package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/pricing"
	"example.invalid/relay-ops-service/internal/reconciliation"
	"example.invalid/relay-ops-service/internal/sub2api"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

type testBillingSources struct{ items []billing.BillingSource }

func (s testBillingSources) ListBillingSources(context.Context) ([]billing.BillingSource, error) {
	return s.items, nil
}

type testUsageLogReader struct{ logs []sub2api.UsageLog }

func (r testUsageLogReader) ListUsageLogs(context.Context, sub2api.UsageLogQuery) ([]sub2api.UsageLog, error) {
	return r.logs, nil
}

type testCostAdapter struct{ transactions []billing.CostTransaction }

func (a testCostAdapter) ListTransactions(context.Context, billing.CostQuery) ([]billing.CostTransaction, string, error) {
	return a.transactions, "", nil
}

func (testCostAdapter) ReadSnapshot(context.Context) (billing.CostSnapshot, error) {
	return billing.CostSnapshot{}, nil
}

func TestCreateAutomaticUpstreamCostMatchesAndResolvesException(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	attempt, exceptionID := createManualAdjustmentException(t, st, ctx, "automatic-effective")
	transaction, inserted, err := st.CreateAutomaticUpstreamCost(ctx, reconciliation.AutomaticTransactionInput{
		AttemptID: attempt.ID, AccountID: attempt.AccountID,
		SourceType: reconciliation.SourceAutomaticCharge, SourceRecordID: "provider-charge-effective",
		Amount: decimal.RequireFromString("1.25"), Currency: "USD",
		OccurredAt: time.Now().UTC(), IdempotencyKey: "automatic:effective:8123",
	})
	if err != nil {
		t.Fatalf("CreateAutomaticUpstreamCost effective: %v", err)
	}
	if !inserted || !transaction.Effective {
		t.Fatalf("automatic effective transaction = %#v inserted %v, want inserted effective transaction", transaction, inserted)
	}

	var transactionCount, effectiveCount int
	if err := st.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE effective)
		FROM relay_ops.upstream_cost_transactions WHERE attempt_id=$1`, attempt.ID).Scan(&transactionCount, &effectiveCount); err != nil {
		t.Fatalf("read effective transaction state: %v", err)
	}
	if transactionCount != 1 || effectiveCount != 1 {
		t.Fatalf("effective transaction state = count %d effective %d, want 1/1", transactionCount, effectiveCount)
	}

	var reconcileStatus string
	var resolvedAt *time.Time
	var resolutionType *string
	if err := st.pool.QueryRow(ctx, `
		SELECT a.reconcile_status, e.resolved_at, e.resolution_type
		FROM relay_ops.upstream_cost_attempts a
		JOIN relay_ops.upstream_reconciliation_exceptions e ON e.id=$2
		WHERE a.id=$1`, attempt.ID, exceptionID).Scan(&reconcileStatus, &resolvedAt, &resolutionType); err != nil {
		t.Fatalf("read effective reconciliation state: %v", err)
	}
	if reconcileStatus != string(reconciliation.StatusMatched) || resolvedAt == nil || resolutionType == nil || *resolutionType != "automatic" {
		t.Fatalf("effective reconciliation state = status %q resolved %v type %v, want matched/resolved/automatic", reconcileStatus, resolvedAt, resolutionType)
	}
}

func TestReadRequestCostDetailMatchesExactLocalAndUpstreamIDs(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	now := time.Now().UTC()
	attempt, _, err := st.RecordUpstreamCostAttempt(ctx, reconciliation.AttemptInput{
		AttemptID: "request-cost-exact", LocalRequestID: "local-cost-exact", UpstreamRequestID: "upstream-cost-exact",
		AccountID: 8123, AdapterType: reconciliation.AdapterSub2API, Model: "gpt-test", InputTokens: 11, OutputTokens: 7,
		UserCharge: decimal.RequireFromString("0.12"), SiteStandardCost: decimal.RequireFromString("0.15"), Currency: "USD",
		RequestStatus: "success", CompletedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateAutomaticUpstreamCost(ctx, reconciliation.AutomaticTransactionInput{
		AttemptID: attempt.ID, AccountID: attempt.AccountID, SourceType: reconciliation.SourceAutomaticCharge,
		SourceRecordID: "native-log-1", Amount: decimal.RequireFromString("0.0821"), Currency: "USD",
		OccurredAt: now, IdempotencyKey: "request-cost:native-log-1",
	}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []reconciliation.RequestCostQuery{{LocalRequestID: "local-cost-exact"}, {UpstreamRequestID: "upstream-cost-exact"}} {
		detail, err := st.ReadRequestCostDetail(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		if detail.SourceID != "native-log-1" || detail.CostSource != "上游逐笔账单" || detail.Confidence != "confirmed" ||
			!detail.UpstreamActualCost.Equal(decimal.RequireFromString("0.0821")) || detail.LocalRequestID != "local-cost-exact" {
			t.Fatalf("detail=%#v", detail)
		}
	}
}

func TestRequestCostDetailPersistsProviderIDAcrossImportReconcileAndRead(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	importer := reconciliation.UsageImporter{
		Sources: testBillingSources{items: []billing.BillingSource{{AccountID: 8123, AdapterType: "newapi"}}},
		Reader: testUsageLogReader{logs: []sub2api.UsageLog{{
			ID: 17, AccountID: 8123, RequestID: "local-imported", Model: "gpt-test",
			InputTokens: 10, OutputTokens: 4, TotalCost: 0.12, CreatedAt: now,
		}}},
		Attempts: st,
	}
	if result, err := importer.Import(ctx, 8123, now.Add(-time.Hour), now.Add(time.Hour)); err != nil || result.Inserted != 1 {
		t.Fatalf("import result=%#v err=%v", result, err)
	}
	service := reconciliation.Service{Repository: st}
	if result, err := service.ReconcileAccount(ctx, 8123, reconciliation.AdapterNewAPI, testCostAdapter{transactions: []billing.CostTransaction{{
		SourceID: "native-log-17", RequestID: "local-imported", UpstreamRequestID: "provider-17", Type: "charge",
		Cost: domain.MicroUSD(82_100), OccurredAt: now,
	}}}, now.Add(-time.Hour), now.Add(time.Hour)); err != nil || result.Matched != 1 {
		t.Fatalf("reconcile result=%#v err=%v", result, err)
	}
	detail, err := st.ReadRequestCostDetail(ctx, reconciliation.RequestCostQuery{UpstreamRequestID: "provider-17"})
	if err != nil {
		t.Fatal(err)
	}
	if detail.LocalRequestID != "local-imported" || detail.UpstreamRequestID != "provider-17" || detail.SourceID != "native-log-17" ||
		!detail.UpstreamActualCost.Equal(decimal.RequireFromString("0.0821")) || detail.Confidence != "confirmed" {
		t.Fatalf("detail=%#v", detail)
	}
}

func TestReadRequestCostDetailNetsNativeChargeAndRefundRows(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	now := time.Now().UTC()
	attempt, _, err := st.RecordUpstreamCostAttempt(ctx, reconciliation.AttemptInput{
		AttemptID: "request-cost-net", LocalRequestID: "local-cost-net", AccountID: 8123,
		AdapterType: reconciliation.AdapterNewAPI, Model: "gpt-test", Currency: "USD", RequestStatus: "success", CompletedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []reconciliation.AutomaticTransactionInput{
		{AttemptID: attempt.ID, AccountID: 8123, SourceType: reconciliation.SourceAutomaticCharge, SourceRecordID: "native-charge", Amount: decimal.RequireFromString("0.08"), Currency: "USD", OccurredAt: now, IdempotencyKey: "native-charge"},
		{AttemptID: attempt.ID, AccountID: 8123, SourceType: reconciliation.SourceAutomaticRefund, SourceRecordID: "native-refund", Amount: decimal.RequireFromString("-0.02"), Currency: "USD", OccurredAt: now.Add(time.Second), IdempotencyKey: "native-refund"},
	} {
		if _, _, err := st.CreateAutomaticUpstreamCost(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	detail, err := st.ReadRequestCostDetail(ctx, reconciliation.RequestCostQuery{LocalRequestID: "local-cost-net"})
	if err != nil {
		t.Fatal(err)
	}
	if !detail.UpstreamActualCost.Equal(decimal.RequireFromString("0.06")) || detail.SourceID != "native-charge,native-refund" ||
		detail.CostSource != "上游逐笔账单" || detail.Confidence != "confirmed" {
		t.Fatalf("net native detail=%#v", detail)
	}
}

func TestReadRequestCostDetailUsesStoredUpstreamPriceTableEvidence(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	now := time.Now().UTC()
	var upstreamID int64
	if err := st.pool.QueryRow(ctx, `
		INSERT INTO relay_ops.upstreams (display_name, role, base_url, adapter_type)
		VALUES ('price-evidence-upstream', 'production', 'https://price-evidence.example/v1', 'newapi')
		RETURNING id`).Scan(&upstreamID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO relay_ops.auth_sessions (upstream_id, secret_ref, auth_mode, status, login_url, scope, billing_account_id)
		VALUES ($1, 'file:/price-evidence-secret', 'bearer', 'active', 'https://price-evidence.example/login', 'billing_read', 8123)`, upstreamID); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(pricing.Evidence{
		SchemaVersion: pricing.EvidenceSchemaVersion, Confidence: "structured_json", SourceURL: "https://price-evidence.example/pricing",
		Models: []pricing.ModelPrice{{ModelID: "gpt-test", Input: "1.25", Output: "10"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshotID, err := st.AppendPricingSnapshot(ctx, PricingSnapshot{
		UpstreamID: domain.UpstreamID(upstreamID), SourceURL: "https://price-evidence.example/pricing", SourceType: "public_page",
		FetchedAt: now.Add(-time.Minute), ContentHash: "price-evidence-hash", NormalizedJSON: payload, EvidenceLevel: "structured_json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.RecordUpstreamCostAttempt(ctx, reconciliation.AttemptInput{
		AttemptID: "request-cost-price", LocalRequestID: "local-cost-price", AccountID: 8123,
		AdapterType: reconciliation.AdapterNewAPI, Model: "gpt-test", InputTokens: 1_000_000, OutputTokens: 500_000,
		UserCharge: decimal.RequireFromString("99"), SiteStandardCost: decimal.RequireFromString("99"),
		Currency: "USD", RequestStatus: "success", CompletedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	detail, err := st.ReadRequestCostDetail(ctx, reconciliation.RequestCostQuery{LocalRequestID: "local-cost-price"})
	if err != nil {
		t.Fatal(err)
	}
	if detail.SourceID != "pricing-snapshot:"+strconv.FormatInt(snapshotID, 10) || detail.CostSource != reconciliation.RequestCostSourceUpstreamPriceTable ||
		detail.Confidence != "estimated" || detail.Status != reconciliation.StatusPending || !detail.UpstreamActualCost.IsZero() ||
		!detail.UpstreamStandardCost.Equal(decimal.RequireFromString("6.25")) {
		t.Fatalf("price table detail=%#v", detail)
	}
}

func TestReadRequestCostDetailKeepsAmbiguousUpstreamIDPending(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	now := time.Now().UTC()
	for _, suffix := range []string{"a", "b"} {
		_, _, err := st.RecordUpstreamCostAttempt(ctx, reconciliation.AttemptInput{
			AttemptID: "ambiguous-" + suffix, LocalRequestID: "ambiguous-local-" + suffix, UpstreamRequestID: "ambiguous-upstream",
			AccountID: 8123, AdapterType: reconciliation.AdapterNewAPI, UserCharge: decimal.RequireFromString("0.10"),
			Currency: "USD", RequestStatus: "success", CompletedAt: now.Add(time.Duration(len(suffix)) * time.Microsecond),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	detail, err := st.ReadRequestCostDetail(ctx, reconciliation.RequestCostQuery{UpstreamRequestID: "ambiguous-upstream"})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != reconciliation.StatusPending || detail.Confidence != "pending" || detail.CostSource != "待对账" || !detail.UpstreamActualCost.IsZero() {
		t.Fatalf("ambiguous detail=%#v", detail)
	}
}

func TestRecordUpstreamCostAttemptPreservesGroupScopeAndRejectsConflict(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	groupID := int64(3)
	attempt, inserted, err := st.RecordUpstreamCostAttempt(ctx, reconciliation.AttemptInput{
		AttemptID:      "group-scope-attempt",
		LocalRequestID: "group-scope-local-request",
		AccountID:      8123,
		AdapterType:    reconciliation.AdapterSub2API,
		GroupID:        &groupID,
		Model:          "gpt-test",
		UserCharge:     decimal.RequireFromString("2.00"),
		Currency:       "USD",
		RequestStatus:  "success",
		CompletedAt:    time.Now().UTC(),
	})
	if err != nil || !inserted {
		t.Fatalf("RecordUpstreamCostAttempt = %#v inserted %v err %v", attempt, inserted, err)
	}
	if attempt.GroupID == nil || *attempt.GroupID != groupID {
		t.Fatalf("stored group_id = %v, want 3", attempt.GroupID)
	}

	retry, inserted, err := st.RecordUpstreamCostAttempt(ctx, attempt.AttemptInput)
	if err != nil || inserted || retry.GroupID == nil || *retry.GroupID != groupID {
		t.Fatalf("retry = %#v inserted %v err %v, want idempotent same group", retry, inserted, err)
	}

	changed := attempt.AttemptInput
	changed.GroupID = ptrInt64(8)
	if _, _, err := st.RecordUpstreamCostAttempt(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed group_id error = %v, want ErrConflict", err)
	}

	nilGroup := attempt.AttemptInput
	nilGroup.AttemptID = "nil-group-scope-attempt"
	nilGroup.LocalRequestID = "nil-group-scope-local-request"
	nilGroup.GroupID = nil
	storedNil, inserted, err := st.RecordUpstreamCostAttempt(ctx, nilGroup)
	if err != nil || !inserted || storedNil.GroupID != nil {
		t.Fatalf("nil group attempt = %#v inserted %v err %v, want nil group", storedNil, inserted, err)
	}
}

func TestOperationsSummaryAndDailyHistoryScopeUniqueAttemptsAndUnattributed(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	dayOne := time.Date(2026, 8, 1, 4, 0, 0, 0, time.UTC)
	dayTwo := time.Date(2026, 8, 2, 4, 0, 0, 0, time.UTC)
	records := []struct {
		attemptID string
		groupID   *int64
		charge    string
		cost      string
		status    string
		completed time.Time
	}{
		{attemptID: "ops-group-3", groupID: ptrInt64(3), charge: "2.00", cost: "0.20", status: "success", completed: dayOne},
		{attemptID: "ops-group-8", groupID: ptrInt64(8), charge: "3.00", cost: "0.30", status: "success", completed: dayOne.Add(time.Minute)},
		{attemptID: "ops-unattributed", groupID: nil, charge: "1.00", cost: "0.10", status: "success", completed: dayOne.Add(2 * time.Minute)},
		{attemptID: "ops-failed-billed", groupID: ptrInt64(8), charge: "0.00", cost: "0.20", status: "failed", completed: dayTwo},
	}
	for _, record := range records {
		attempt, inserted, err := st.RecordUpstreamCostAttempt(ctx, reconciliation.AttemptInput{
			AttemptID: record.attemptID, LocalRequestID: record.attemptID + "-request", AccountID: 8123,
			AdapterType: reconciliation.AdapterSub2API, GroupID: record.groupID, Model: "gpt-test",
			UserCharge: decimal.RequireFromString(record.charge), Currency: "USD", RequestStatus: record.status,
			CompletedAt: record.completed,
		})
		if err != nil || !inserted {
			t.Fatalf("RecordUpstreamCostAttempt %s = %#v inserted %v err %v", record.attemptID, attempt, inserted, err)
		}
		if _, inserted, err := st.CreateAutomaticUpstreamCost(ctx, reconciliation.AutomaticTransactionInput{
			AttemptID: attempt.ID, AccountID: 8123, SourceType: reconciliation.SourceAutomaticCharge,
			SourceRecordID: record.attemptID + "-charge", Amount: decimal.RequireFromString(record.cost),
			Currency: "USD", OccurredAt: record.completed, IdempotencyKey: "ops:" + record.attemptID,
		}); err != nil || !inserted {
			t.Fatalf("CreateAutomaticUpstreamCost %s inserted %v err %v", record.attemptID, inserted, err)
		}
	}

	start := dayOne.Add(-time.Hour)
	end := dayTwo.Add(24 * time.Hour)
	global, err := st.ReadOperationsSummary(ctx, reconciliation.OperationsScope{
		Start: start, End: end, Currency: "USD", Timezone: "UTC",
	})
	if err != nil {
		t.Fatalf("ReadOperationsSummary global: %v", err)
	}
	if global.TotalAttempts != 4 || global.MatchedAttempts != 4 || !global.CoverageKnown ||
		!global.UpstreamCost.Equal(decimal.RequireFromString("0.80")) ||
		!global.UserCharge.Equal(decimal.RequireFromString("6.00")) ||
		global.UnattributedAttempts != 1 ||
		!global.UnattributedUserCharge.Equal(decimal.RequireFromString("1.00")) ||
		!global.UnattributedUpstreamCost.Equal(decimal.RequireFromString("0.10")) {
		t.Fatalf("global operations = %#v", global)
	}

	groupID := int64(3)
	groupThree, err := st.ReadOperationsSummary(ctx, reconciliation.OperationsScope{
		GroupID: &groupID, Start: start, End: end, Currency: "USD", Timezone: "UTC",
	})
	if err != nil {
		t.Fatalf("ReadOperationsSummary group 3: %v", err)
	}
	if groupThree.TotalAttempts != 1 || !groupThree.UserCharge.Equal(decimal.RequireFromString("2.00")) ||
		!groupThree.UpstreamCost.Equal(decimal.RequireFromString("0.20")) || groupThree.UnattributedAttempts != 0 {
		t.Fatalf("group 3 operations = %#v", groupThree)
	}

	daily, err := st.ListOperationsDaily(ctx, reconciliation.OperationsScope{
		Start: start, End: end, Currency: "USD", Timezone: "UTC",
	})
	if err != nil {
		t.Fatalf("ListOperationsDaily: %v", err)
	}
	if len(daily) != 2 || daily[0].Day != "2026-08-01" || daily[1].Day != "2026-08-02" ||
		!daily[0].UpstreamCost.Equal(decimal.RequireFromString("0.60")) ||
		!daily[1].UpstreamCost.Equal(decimal.RequireFromString("0.20")) ||
		daily[1].ProfitMargin != nil {
		t.Fatalf("daily operations = %#v", daily)
	}

	groupDaily, err := st.ListOperationsDaily(ctx, reconciliation.OperationsScope{
		GroupID: &groupID, Start: start, End: end, Currency: "USD", Timezone: "UTC",
	})
	if err != nil {
		t.Fatalf("ListOperationsDaily group 3: %v", err)
	}
	if len(groupDaily) != 1 || groupDaily[0].Day != "2026-08-01" ||
		!groupDaily[0].UserCharge.Equal(decimal.RequireFromString("2.00")) {
		t.Fatalf("group daily operations = %#v", groupDaily)
	}
}

func TestCreateAutomaticUpstreamCostAfterManualCreatesConflictException(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	attempt, exceptionID := createManualAdjustmentException(t, st, ctx, "automatic-late-manual")
	manual, inserted, err := st.CreateManualUpstreamCost(ctx, reconciliation.ManualAdjustmentInput{
		AttemptID: attempt.ID, Amount: decimal.RequireFromString("1.25"), ActorUserID: 7,
		IdempotencyKey: "manual:automatic-late:8123",
	})
	if err != nil || !inserted || !manual.Effective {
		t.Fatalf("CreateManualUpstreamCost = %#v inserted %v err %v, want inserted effective manual transaction", manual, inserted, err)
	}

	transaction, inserted, err := st.CreateAutomaticUpstreamCost(ctx, reconciliation.AutomaticTransactionInput{
		AttemptID: attempt.ID, AccountID: attempt.AccountID,
		SourceType: reconciliation.SourceAutomaticCharge, SourceRecordID: "provider-charge-late",
		Amount: decimal.RequireFromString("1.25"), Currency: "USD",
		OccurredAt: time.Now().UTC(), IdempotencyKey: "automatic:late-manual:8123",
	})
	if err != nil {
		t.Fatalf("CreateAutomaticUpstreamCost after manual: %v", err)
	}
	if !inserted || transaction.Effective {
		t.Fatalf("automatic late transaction = %#v inserted %v, want inserted ineffective transaction", transaction, inserted)
	}

	var transactionCount, effectiveCount int
	if err := st.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE effective)
		FROM relay_ops.upstream_cost_transactions WHERE attempt_id=$1`, attempt.ID).Scan(&transactionCount, &effectiveCount); err != nil {
		t.Fatalf("read conflict transaction state: %v", err)
	}
	if transactionCount != 2 || effectiveCount != 1 {
		t.Fatalf("conflict transaction state = count %d effective %d, want 2/1", transactionCount, effectiveCount)
	}

	var reconcileStatus, reasonCode string
	var resolvedAt *time.Time
	var resolutionType *string
	if err := st.pool.QueryRow(ctx, `
		SELECT a.reconcile_status, e.reason_code, e.resolved_at, e.resolution_type
		FROM relay_ops.upstream_cost_attempts a
		JOIN relay_ops.upstream_reconciliation_exceptions e ON e.id=$2
		WHERE a.id=$1`, attempt.ID, exceptionID).Scan(&reconcileStatus, &reasonCode, &resolvedAt, &resolutionType); err != nil {
		t.Fatalf("read conflict reconciliation state: %v", err)
	}
	if reconcileStatus != string(reconciliation.StatusConflict) || reasonCode != "late_automatic_after_manual" || resolvedAt != nil || resolutionType != nil {
		t.Fatalf("conflict reconciliation state = status %q reason %q resolved %v type %v, want conflict/late_automatic_after_manual/unresolved", reconcileStatus, reasonCode, resolvedAt, resolutionType)
	}
}

func TestCreateManualUpstreamCostForExceptionAllowsResolvedExceptionIdempotentRetry(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	completedAt := time.Now().UTC().Add(-2 * time.Hour)
	attempt, inserted, err := st.RecordUpstreamCostAttempt(ctx, reconciliation.AttemptInput{
		AttemptID:      "manual-retry-attempt",
		LocalRequestID: "manual-retry-local-request",
		AccountID:      8123,
		AdapterType:    reconciliation.AdapterSub2API,
		Model:          "gpt-test",
		Currency:       "USD",
		RequestStatus:  "success",
		CompletedAt:    completedAt,
	})
	if err != nil || !inserted {
		t.Fatalf("RecordUpstreamCostAttempt = %#v inserted %v err %v", attempt, inserted, err)
	}
	if _, err := st.MarkOverdueUpstreamCostExceptions(ctx, time.Now().UTC(), time.Hour); err != nil {
		t.Fatalf("MarkOverdueUpstreamCostExceptions: %v", err)
	}

	var exceptionID int64
	if err := st.pool.QueryRow(ctx, `
		SELECT id FROM relay_ops.upstream_reconciliation_exceptions WHERE attempt_id=$1`, attempt.ID).Scan(&exceptionID); err != nil {
		t.Fatalf("find exception: %v", err)
	}

	input := reconciliation.ManualAdjustmentInput{
		Amount:         decimal.RequireFromString("1.25"),
		ActorUserID:    7,
		IdempotencyKey: "manual:exception:8123:retry",
	}
	first, created, err := st.CreateManualUpstreamCostForException(ctx, exceptionID, input)
	if err != nil || !created {
		t.Fatalf("first manual adjustment = %#v created %v err %v", first, created, err)
	}

	normalizedRetry := input
	normalizedRetry.Amount = decimal.RequireFromString("1.25000000001")
	retry, created, err := st.CreateManualUpstreamCostForException(ctx, exceptionID, normalizedRetry)
	if err != nil {
		t.Fatalf("same-key retry returned error: %v", err)
	}
	if created || retry.ID != first.ID {
		t.Fatalf("same-key retry = %#v created %v, want existing transaction id %d", retry, created, first.ID)
	}

	mismatch := input
	mismatch.Amount = decimal.RequireFromString("2.50")
	if _, _, err := st.CreateManualUpstreamCostForException(ctx, exceptionID, mismatch); !errors.Is(err, ErrConflict) {
		t.Fatalf("mismatched same-key retry error = %v, want ErrConflict", err)
	}
	wrongKey := input
	wrongKey.IdempotencyKey = "manual:exception:8123:different-retry"
	if _, _, err := st.CreateManualUpstreamCostForException(ctx, exceptionID, wrongKey); !errors.Is(err, ErrConflict) {
		t.Fatalf("different-key retry error = %v, want ErrConflict", err)
	}

	var transactionCount int
	if err := st.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM relay_ops.upstream_cost_transactions WHERE attempt_id=$1`, attempt.ID).Scan(&transactionCount); err != nil {
		t.Fatalf("count manual transactions: %v", err)
	}
	if transactionCount != 1 {
		t.Fatalf("manual transaction count = %d, want 1", transactionCount)
	}
	var reconcileStatus, resolutionType string
	var resolvedAt *time.Time
	if err := st.pool.QueryRow(ctx, `
		SELECT a.reconcile_status, e.resolved_at, e.resolution_type
		FROM relay_ops.upstream_cost_attempts a
		JOIN relay_ops.upstream_reconciliation_exceptions e ON e.attempt_id=a.id
		WHERE a.id=$1`, attempt.ID).Scan(&reconcileStatus, &resolvedAt, &resolutionType); err != nil {
		t.Fatalf("read adjustment state: %v", err)
	}
	if reconcileStatus != string(reconciliation.StatusManual) || resolvedAt == nil || resolutionType != "manual" {
		t.Fatalf("adjustment state = status %q resolved %v type %q, want manual/resolved/manual", reconcileStatus, resolvedAt, resolutionType)
	}
}

func TestCreateManualUpstreamCostForExceptionLocksAttemptBeforeExceptionDuringConcurrentAdjustment(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	attempt, exceptionID := createManualAdjustmentException(t, st, ctx, "manual-lock-order")
	blocker, err := st.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin attempt blocker: %v", err)
	}
	defer blocker.Rollback(ctx)
	var lockedAttemptID int64
	if err := blocker.QueryRow(ctx, `
		SELECT id FROM relay_ops.upstream_cost_attempts WHERE id=$1 FOR UPDATE`, attempt.ID).Scan(&lockedAttemptID); err != nil {
		t.Fatalf("lock attempt blocker: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, _, err := st.CreateManualUpstreamCostForException(ctx, exceptionID, reconciliation.ManualAdjustmentInput{
			Amount: decimal.RequireFromString("1.25"), ActorUserID: 7,
			IdempotencyKey: "manual:exception:lock-order",
		})
		result <- err
	}()

	waitForTransactionLockWaiter(t, st, ctx)
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		locked, err := exceptionIsLockedNowait(ctx, st, exceptionID)
		if err != nil {
			t.Fatalf("probe exception lock: %v", err)
		}
		if locked {
			t.Fatal("manual adjustment locked exception while waiting for attempt; this reverses the shared attempt-to-exception lock order")
		}
		time.Sleep(10 * time.Millisecond)
	}

	var lockedExceptionID int64
	if err := blocker.QueryRow(ctx, `
		SELECT id FROM relay_ops.upstream_reconciliation_exceptions WHERE id=$1 FOR UPDATE`, exceptionID).Scan(&lockedExceptionID); err != nil {
		t.Fatalf("lock exception after attempt: %v", err)
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("commit attempt blocker: %v", err)
	}

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("manual adjustment after concurrent lock sequence: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manual adjustment did not complete after attempt and exception locks were released")
	}
}

func waitForTransactionLockWaiter(t *testing.T, st *Store, ctx context.Context) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var waiters int
		if err := st.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM pg_locks WHERE locktype='transactionid' AND NOT granted`).Scan(&waiters); err != nil {
			t.Fatalf("inspect transaction lock waiters: %v", err)
		}
		if waiters > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("manual adjustment never reached the blocked attempt lock")
}

func createManualAdjustmentException(t *testing.T, st *Store, ctx context.Context, suffix string) (reconciliation.Attempt, int64) {
	t.Helper()
	attempt, inserted, err := st.RecordUpstreamCostAttempt(ctx, reconciliation.AttemptInput{
		AttemptID:      suffix + "-attempt",
		LocalRequestID: suffix + "-local-request",
		AccountID:      8123,
		AdapterType:    reconciliation.AdapterSub2API,
		Model:          "gpt-test",
		Currency:       "USD",
		RequestStatus:  "success",
		CompletedAt:    time.Now().UTC().Add(-2 * time.Hour),
	})
	if err != nil || !inserted {
		t.Fatalf("RecordUpstreamCostAttempt = %#v inserted %v err %v", attempt, inserted, err)
	}
	if _, err := st.MarkOverdueUpstreamCostExceptions(ctx, time.Now().UTC(), time.Hour); err != nil {
		t.Fatalf("MarkOverdueUpstreamCostExceptions: %v", err)
	}
	var exceptionID int64
	if err := st.pool.QueryRow(ctx, `
		SELECT id FROM relay_ops.upstream_reconciliation_exceptions WHERE attempt_id=$1`, attempt.ID).Scan(&exceptionID); err != nil {
		t.Fatalf("find exception: %v", err)
	}
	return attempt, exceptionID
}

func exceptionIsLockedNowait(ctx context.Context, st *Store, exceptionID int64) (bool, error) {
	var id int64
	err := st.pool.QueryRow(ctx, `
		SELECT id FROM relay_ops.upstream_reconciliation_exceptions WHERE id=$1 FOR UPDATE NOWAIT`, exceptionID).Scan(&id)
	if err == nil {
		return false, nil
	}
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) && postgresErr.Code == "55P03" {
		return true, nil
	}
	return false, err
}

func ptrInt64(value int64) *int64 { return &value }
