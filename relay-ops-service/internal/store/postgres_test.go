package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/events"
	"example.invalid/relay-ops-service/internal/projection"
	"example.invalid/relay-ops-service/internal/qualityreports"
	"example.invalid/relay-ops-service/internal/sub2api"
	"example.invalid/relay-ops-service/internal/upstreams"
)

func int64Pointer(value int64) *int64 { return &value }

func TestQualityReportsAreAppendOnlyAndRetrievable(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	upstreamID, err := st.CreateUpstream(ctx, Upstream{Name: "candidate", Role: "candidate", BaseURL: "https://candidate.example/v1", AdapterType: "openai", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	report := qualityreports.Report{
		ReportID: "fast-1", ReportHash: strings.Repeat("a", 64), UpstreamID: upstreamID, UpstreamName: "candidate",
		JobKind: "health_pulse", Status: "needs_evidence", QualityScore: 85, TotalScore: 85,
		Direct: "6/6", Gateway: "unknown", Models: "3 selected", Pricing: "unknown", Capacity: "unknown",
		Record: json.RawMessage(`{"run_id":"fast-1"}`), RecordedAt: time.Date(2026, 7, 22, 3, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 7, 22, 3, 30, 0, 0, time.UTC),
	}
	if err := st.PutQualityReport(ctx, report); err != nil {
		t.Fatalf("PutQualityReport: %v", err)
	}
	if err := st.PutQualityReport(ctx, report); err != nil {
		t.Fatalf("idempotent PutQualityReport: %v", err)
	}
	changed := report
	changed.ReportHash = strings.Repeat("b", 64)
	if err := st.PutQualityReport(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed report error = %v", err)
	}
	got, found, err := st.GetQualityReport(ctx, report.ReportID)
	if err != nil || !found || got.ReportHash != report.ReportHash || got.QualityScore != 85 {
		t.Fatalf("GetQualityReport = %#v, %v, %v", got, found, err)
	}
	items, err := st.ListQualityReports(ctx, 10)
	if err != nil || len(items) != 1 || items[0].ReportID != report.ReportID {
		t.Fatalf("ListQualityReports = %#v, %v", items, err)
	}
}

func TestMigrateIsIdempotentAndUpstreamIdentityIsUnique(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	first := Upstream{Name: "Neko", Role: "production", BaseURL: "https://api.neko.example/v1", AdapterType: "sub2api", Enabled: true}
	if _, err := st.CreateUpstream(ctx, first); err != nil {
		t.Fatalf("CreateUpstream: %v", err)
	}
	if _, err := st.CreateUpstream(ctx, first); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate name error = %v, want ErrConflict", err)
	}
	second := first
	second.Name = "Neko duplicate"
	if _, err := st.CreateUpstream(ctx, second); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate base URL error = %v, want ErrConflict", err)
	}
}

func TestBalanceFactsAreIdempotentConflictSafeAndExpire(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC)
	snapshot := billing.BalanceSnapshot{AccountID: 71, Amount: "12.50", Currency: "USD", ObservedAt: observedAt, FreshUntil: observedAt.Add(time.Minute), Source: "sub2api"}
	inserted, err := st.AppendBalanceSnapshot(ctx, snapshot)
	if err != nil || !inserted {
		t.Fatalf("first append inserted=%v err=%v", inserted, err)
	}
	inserted, err = st.AppendBalanceSnapshot(ctx, snapshot)
	if err != nil || inserted {
		t.Fatalf("replay inserted=%v err=%v", inserted, err)
	}
	conflicting := snapshot
	conflicting.Amount = "13.50"
	if _, err := st.AppendBalanceSnapshot(ctx, conflicting); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting replay error=%v, want ErrConflict", err)
	}
	fresh, found, err := st.LatestFreshBalanceSnapshot(ctx, snapshot.AccountID, observedAt.Add(59*time.Second))
	if err != nil || !found || fresh.Amount != "12.50" || fresh.Source != "sub2api" {
		t.Fatalf("fresh snapshot=%+v found=%v err=%v", fresh, found, err)
	}
	if _, found, err := st.LatestFreshBalanceSnapshot(ctx, snapshot.AccountID, snapshot.FreshUntil); err != nil || found {
		t.Fatalf("expired snapshot found=%v err=%v", found, err)
	}
}

func TestAccountUpdateCommandUsesExistingDurableIdempotencyAudit(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	dispatch, result, err := st.ClaimExternalizationCommand(ctx, 9, 71, "account:71:priority:1", "account_update", 1)
	if err != nil || !dispatch || result != "pending" {
		t.Fatalf("initial claim dispatch=%v result=%q err=%v", dispatch, result, err)
	}
	dispatch, result, err = st.ClaimExternalizationCommand(ctx, 9, 71, "account:71:priority:1", "account_update", 1)
	if err != nil || dispatch || result != "pending" {
		t.Fatalf("pending replay dispatch=%v result=%q err=%v", dispatch, result, err)
	}
	if err := st.CompleteExternalizationCommand(ctx, 9, 71, "account:71:priority:1", "accepted", 1); err != nil {
		t.Fatal(err)
	}
	dispatch, result, err = st.ClaimExternalizationCommand(ctx, 9, 71, "account:71:priority:1", "account_update", 1)
	if err != nil || dispatch || result != "accepted" {
		t.Fatalf("accepted replay dispatch=%v result=%q err=%v", dispatch, result, err)
	}
	if _, _, err := st.ClaimExternalizationCommand(ctx, 10, 71, "account:71:priority:1", "account_update", 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("actor mismatch error=%v, want ErrConflict", err)
	}
}

func TestAccountUpdateCommandDurablyBindsCommandIDPayloadActorAndVersion(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	command := billing.AccountUpdateCommand{CommandID: "command-durable-1", ActorID: 9, AccountID: 71, IdempotencyKey: "account:71:priority:durable", Fields: map[string]any{"priority": 2}}
	hash, err := command.CanonicalPayloadHash()
	if err != nil {
		t.Fatal(err)
	}
	dispatch, result, err := st.ClaimAccountUpdateCommand(ctx, command, hash, 1)
	if err != nil || !dispatch || result != "pending" {
		t.Fatalf("initial dispatch=%v result=%q err=%v", dispatch, result, err)
	}
	for _, changed := range []billing.AccountUpdateCommand{
		{CommandID: "command-durable-2", ActorID: 9, AccountID: 71, IdempotencyKey: command.IdempotencyKey, Fields: map[string]any{"priority": 2}},
		{CommandID: command.CommandID, ActorID: 10, AccountID: 71, IdempotencyKey: command.IdempotencyKey, Fields: map[string]any{"priority": 2}},
		{CommandID: command.CommandID, ActorID: 9, AccountID: 71, IdempotencyKey: command.IdempotencyKey, Fields: map[string]any{"priority": 3}},
	} {
		changedHash, hashErr := changed.CanonicalPayloadHash()
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		if _, _, claimErr := st.ClaimAccountUpdateCommand(ctx, changed, changedHash, 1); !errors.Is(claimErr, ErrConflict) {
			t.Fatalf("changed=%+v err=%v, want ErrConflict", changed, claimErr)
		}
	}
	if err := st.CompleteAccountUpdateCommand(ctx, command, "failed", 1); err != nil {
		t.Fatal(err)
	}
	dispatch, result, err = st.ClaimAccountUpdateCommand(ctx, command, hash, 1)
	if err != nil || dispatch || result != "failed" {
		t.Fatalf("failed replay dispatch=%v result=%q err=%v", dispatch, result, err)
	}
}

func TestProjectionStorePersistsConsumerCheckpointDeadLettersAndReadModels(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	event := events.Event{
		EventID:         "550e8400-e29b-41d4-a716-446655440001",
		Type:            events.RequestCompleted,
		OccurredAt:      at,
		SourceVersion:   "sub2api-v1",
		ContractVersion: events.ContractVersion,
		Payload:         []byte(`{"request_id":"request-1","account_id":7,"model":"gpt-test","prompt_tokens":1,"completion_tokens":1,"user_charge":"1.25","actual_cost":"0.40","cost_usd":"0.40","currency":"USD"}`),
	}
	profitability := projection.NewProfitabilityWithRepository(st)
	accounting := projection.NewAccountingWithRepository(st)
	consumer, err := events.NewPersistentConsumer(st, profitability, accounting)
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.Handle(ctx, event); err != nil {
		t.Fatal(err)
	}

	restarted, err := events.NewPersistentConsumer(st, events.HandlerFunc(func(context.Context, events.Event) error {
		t.Fatal("persisted duplicate was dispatched after restart")
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Handle(ctx, event); err != nil {
		t.Fatalf("restart duplicate: %v", err)
	}
	next := event
	next.EventID = "550e8400-e29b-41d4-a716-446655440002"
	next.OccurredAt = at.Add(time.Second)
	next.Payload = []byte(`{"request_id":"request-2","account_id":7,"model":"gpt-test","prompt_tokens":1,"completion_tokens":1,"user_charge":"0.75","actual_cost":"0.10","cost_usd":"0.10","currency":"USD"}`)
	resumedProfitability := projection.NewProfitabilityWithRepository(st)
	resumedAccounting := projection.NewAccountingWithRepository(st)
	resumed, err := events.NewPersistentConsumer(st, resumedProfitability, resumedAccounting)
	if err != nil {
		t.Fatal(err)
	}
	if err := resumed.Handle(ctx, next); err != nil {
		t.Fatalf("resume projection from persisted state: %v", err)
	}
	w, found, err := restarted.LoadWatermark(ctx, "sub2api-v1")
	if err != nil || !found || w.LastEventID != next.EventID || w.Completeness != events.CompletenessComplete {
		t.Fatalf("watermark=%+v found=%v err=%v", w, found, err)
	}

	var requestCount int64
	var revenue, cost, profit, completeness, version string
	if err := st.pool.QueryRow(ctx, `
		SELECT requests, revenue::text, cost::text, profit::text, completeness, calculation_version
		FROM relay_ops.profitability_read_models WHERE account_id=7`).Scan(
		&requestCount, &revenue, &cost, &profit, &completeness, &version,
	); err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 || revenue != "2" || cost != "0.5" || profit != "1.5" || completeness != projection.CompletenessComplete || version != projection.ProfitabilityCalculationVersion {
		t.Fatalf("persisted profitability requests=%d revenue=%s cost=%s profit=%s completeness=%s version=%s", requestCount, revenue, cost, profit, completeness, version)
	}
	if err := st.pool.QueryRow(ctx, `SELECT requests FROM relay_ops.accounting_read_models WHERE scope='all'`).Scan(&requestCount); err != nil || requestCount != 2 {
		t.Fatalf("persisted accounting requests=%d err=%v", requestCount, err)
	}
	accountEvent := events.Event{
		EventID:         "550e8400-e29b-41d4-a716-446655440003",
		Type:            events.AccountHealthChanged,
		OccurredAt:      at.Add(2 * time.Second),
		SourceVersion:   "sub2api-v1",
		ContractVersion: events.ContractVersion,
		Payload:         []byte(`{"account_id":7,"status":"healthy","checked_at":"2026-08-09T00:00:02Z"}`),
	}
	accounts := projection.NewAccountsWithRepository(st)
	accountConsumer, err := events.NewPersistentConsumer(st, accounts)
	if err != nil {
		t.Fatal(err)
	}
	if err := accountConsumer.Handle(ctx, accountEvent); err != nil {
		t.Fatalf("persist account projection: %v", err)
	}
	var status, sourceWatermark string
	var generatedAt time.Time
	var freshness int64
	if err := st.pool.QueryRow(ctx, `
		SELECT status, generated_at, source_watermark, freshness_seconds
		FROM relay_ops.account_read_models WHERE account_id=7`).Scan(&status, &generatedAt, &sourceWatermark, &freshness); err != nil {
		t.Fatal(err)
	}
	if status != "healthy" || generatedAt.IsZero() || sourceWatermark != accountEvent.EventID || freshness < 0 {
		t.Fatalf("persisted account status=%s generated_at=%s watermark=%s freshness=%d", status, generatedAt, sourceWatermark, freshness)
	}

	deadEvent := events.Event{
		EventID:         "550e8400-e29b-41d4-a716-446655440004",
		Type:            events.AccountHealthChanged,
		OccurredAt:      at.Add(time.Second),
		SourceVersion:   "sub2api-v1",
		ContractVersion: events.ContractVersion,
		Payload:         []byte(`{"account_id":7,"status":"healthy","checked_at":"2026-08-09T00:00:01Z"}`),
	}
	failing, err := events.NewPersistentConsumer(st, events.HandlerFunc(func(context.Context, events.Event) error {
		return errors.New("projection rejected event")
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := failing.Handle(ctx, deadEvent); err == nil {
		t.Fatal("expected projection failure")
	}
	dead, err := restarted.ListDeadLetters(ctx)
	if err != nil || len(dead) != 1 || dead[0].Event.EventID != deadEvent.EventID || dead[0].Error != "projection rejected event" {
		t.Fatalf("dead letters=%+v err=%v", dead, err)
	}
}

func TestConsumerAtomicProjectionRollbackFencingAndCompleteness(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC)
	event := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440011", at, "1.00", "0.25")
	consumer, err := events.NewPersistentConsumer(st,
		projection.NewProfitabilityWithRepository(st),
		events.HandlerFunc(func(context.Context, events.Event) error { return errors.New("second projection failed") }),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.Handle(ctx, event); err == nil {
		t.Fatal("expected second handler failure")
	}
	var profitabilityRows int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.profitability_read_models`).Scan(&profitabilityRows); err != nil {
		t.Fatal(err)
	}
	if profitabilityRows != 0 {
		t.Fatalf("partial projection rows=%d, want 0", profitabilityRows)
	}
	var status string
	if err := st.pool.QueryRow(ctx, `SELECT status FROM relay_ops.externalization_events WHERE event_id=$1`, event.EventID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "dead" {
		t.Fatalf("failed event status=%q", status)
	}

	good := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440012", at.Add(time.Second), "0.50", "0.10")
	goodConsumer, err := events.NewPersistentConsumer(st, projection.NewProfitabilityWithRepository(st))
	if err != nil {
		t.Fatal(err)
	}
	if err := goodConsumer.Handle(ctx, good); err != nil {
		t.Fatal(err)
	}
	watermark, found, err := goodConsumer.LoadWatermark(ctx, "sub2api-v1")
	if err != nil || !found {
		t.Fatalf("watermark found=%v err=%v", found, err)
	}
	if watermark.Completeness != events.CompletenessPartial {
		t.Fatalf("watermark completeness=%q with dead gap, want partial", watermark.Completeness)
	}

	fenced := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440013", at.Add(2*time.Second), "0.25", "0.05")
	oldClaim, err := st.ClaimEvent(ctx, fenced)
	if err != nil || !oldClaim.Acquired {
		t.Fatalf("old claim=%+v err=%v", oldClaim, err)
	}
	if _, err := st.pool.Exec(ctx, `UPDATE relay_ops.externalization_events SET lease_until=NOW()-INTERVAL '1 second' WHERE event_id=$1`, fenced.EventID); err != nil {
		t.Fatal(err)
	}
	newClaim, err := st.ClaimEvent(ctx, fenced)
	if err != nil || !newClaim.Acquired || newClaim.Generation <= oldClaim.Generation || newClaim.Token == oldClaim.Token {
		t.Fatalf("new claim=%+v old=%+v err=%v", newClaim, oldClaim, err)
	}
	_, err = st.ApplyEvent(ctx, fenced, oldClaim, time.Now().UTC(), func(context.Context) error { return nil })
	if !errors.Is(err, events.ErrStaleClaim) {
		t.Fatalf("old worker apply error=%v, want ErrStaleClaim", err)
	}
	if _, err := st.ApplyEvent(ctx, fenced, newClaim, time.Now().UTC(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("new worker apply: %v", err)
	}
}

func TestConsumerCompleteFailureRollsBackProjectionAndRetryDoesNotDoubleCount(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION relay_ops.reject_externalization_watermark() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'forced watermark failure'; END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER reject_externalization_watermark
		BEFORE INSERT OR UPDATE ON relay_ops.externalization_watermarks
		FOR EACH ROW EXECUTE FUNCTION relay_ops.reject_externalization_watermark();`); err != nil {
		t.Fatal(err)
	}
	event := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440021", time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC), "1.00", "0.40")
	consumer, err := events.NewPersistentConsumer(st, projection.NewProfitabilityWithRepository(st))
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.Handle(ctx, event); err == nil {
		t.Fatal("expected forced completion failure")
	}
	var rows int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.profitability_read_models`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("projection rows after completion rollback=%d err=%v", rows, err)
	}
	if _, err := st.pool.Exec(ctx, `
		DROP TRIGGER reject_externalization_watermark ON relay_ops.externalization_watermarks;
		DROP FUNCTION relay_ops.reject_externalization_watermark();`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `
		UPDATE relay_ops.externalization_events SET lease_until=NOW()-INTERVAL '1 second' WHERE event_id=$1`, event.EventID); err != nil {
		t.Fatal(err)
	}
	retry, err := events.NewPersistentConsumer(st, projection.NewProfitabilityWithRepository(st))
	if err != nil {
		t.Fatal(err)
	}
	if err := retry.Handle(ctx, event); err != nil {
		t.Fatal(err)
	}
	var requests int64
	var revenue string
	if err := st.pool.QueryRow(ctx, `SELECT requests, revenue::text FROM relay_ops.profitability_read_models WHERE account_id=7`).Scan(&requests, &revenue); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || revenue != "1" {
		t.Fatalf("retried projection requests=%d revenue=%s", requests, revenue)
	}
}

func TestConsumerConcurrentInstancesSerializeProjectionUpdates(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)
	input := []events.Event{
		externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440031", at, "0.10", "0.01"),
		externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440032", at, "0.20", "0.02"),
	}
	start := make(chan struct{})
	result := make(chan error, len(input))
	for _, event := range input {
		event := event
		go func() {
			consumer, err := events.NewPersistentConsumer(st, projection.NewProfitabilityWithRepository(st))
			if err == nil {
				<-start
				err = consumer.Handle(ctx, event)
			}
			result <- err
		}()
	}
	close(start)
	for range input {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
	var requests int64
	var revenue string
	if err := st.pool.QueryRow(ctx, `SELECT requests, revenue::text FROM relay_ops.profitability_read_models WHERE account_id=7`).Scan(&requests, &revenue); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || revenue != "0.3" {
		t.Fatalf("concurrent projection requests=%d revenue=%s", requests, revenue)
	}
	watermark, found, err := st.LoadWatermark(ctx, "sub2api-v1")
	if err != nil || !found || watermark.Completeness != events.CompletenessComplete {
		t.Fatalf("completed watermark=%+v found=%v err=%v", watermark, found, err)
	}
	pending := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440033", at.Add(time.Second), "0.30", "0.03")
	claim, err := st.ClaimEvent(ctx, pending)
	if err != nil || !claim.Acquired {
		t.Fatalf("processing gap claim=%+v err=%v", claim, err)
	}
	watermark, found, err = st.LoadWatermark(ctx, "sub2api-v1")
	if err != nil || !found || watermark.Completeness != events.CompletenessPartial {
		t.Fatalf("processing-gap watermark=%+v found=%v err=%v", watermark, found, err)
	}
}

func TestProjectionCompletenessTracksProcessingAndDeadGapsInSameTransaction(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 10, 3, 30, 0, 0, time.UTC)
	accounts := projection.NewAccountsWithRepository(st)
	profitability := projection.NewProfitabilityWithRepository(st)
	accounting := projection.NewAccountingWithRepository(st)
	reconciliation := projection.NewReconciliationWithRepository(st)
	consumer, err := events.NewPersistentConsumer(st, accounts, profitability, accounting, reconciliation)
	if err != nil {
		t.Fatal(err)
	}
	accountEvent := events.Event{
		EventID: "550e8400-e29b-41d4-a716-446655440034", Type: events.AccountHealthChanged,
		OccurredAt: at, SourceVersion: "sub2api-v1", ContractVersion: events.ContractVersion,
		Payload: []byte(`{"account_id":7,"status":"healthy","checked_at":"2026-08-10T03:30:00Z"}`),
	}
	requestEvent := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440035", at.Add(time.Second), "1.00", "0.25")
	if err := consumer.Handle(ctx, accountEvent); err != nil {
		t.Fatal(err)
	}
	if err := consumer.Handle(ctx, requestEvent); err != nil {
		t.Fatal(err)
	}
	assertExternalizationCompleteness(t, st, events.CompletenessComplete)

	processing := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440036", at.Add(2*time.Second), "0.50", "0.10")
	claim, err := st.ClaimEvent(ctx, processing)
	if err != nil || !claim.Acquired {
		t.Fatalf("processing claim=%+v err=%v", claim, err)
	}
	assertExternalizationCompleteness(t, st, events.CompletenessPartial)
	if _, err := st.ApplyEvent(ctx, processing, claim, time.Now().UTC(), func(applyCtx context.Context) error {
		for _, handler := range []events.Handler{accounts, profitability, accounting, reconciliation} {
			if err := handler.Handle(applyCtx, processing); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertExternalizationCompleteness(t, st, events.CompletenessComplete)

	dead := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440037", at.Add(3*time.Second), "0.25", "0.05")
	deadClaim, err := st.ClaimEvent(ctx, dead)
	if err != nil || !deadClaim.Acquired {
		t.Fatalf("dead claim=%+v err=%v", deadClaim, err)
	}
	if err := st.FailEvent(ctx, dead, deadClaim, time.Now().UTC(), errors.New("permanent projection failure")); err != nil {
		t.Fatal(err)
	}
	assertExternalizationCompleteness(t, st, events.CompletenessPartial)
}

func TestGlobalProjectionCompletenessIncludesGapsFromOtherSources(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 10, 3, 40, 0, 0, time.UTC)
	accounts := projection.NewAccountsWithRepository(st)
	profitability := projection.NewProfitabilityWithRepository(st)
	accounting := projection.NewAccountingWithRepository(st)
	reconciliation := projection.NewReconciliationWithRepository(st)
	consumer, err := events.NewPersistentConsumer(st, accounts, profitability, accounting, reconciliation)
	if err != nil {
		t.Fatal(err)
	}
	accountEvent := events.Event{
		EventID: "550e8400-e29b-41d4-a716-446655440051", Type: events.AccountHealthChanged,
		OccurredAt: at, SourceVersion: "source-a", ContractVersion: events.ContractVersion,
		Payload: []byte(`{"account_id":7,"status":"healthy","checked_at":"2026-08-10T03:40:00Z"}`),
	}
	seed := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440052", at.Add(time.Second), "1.00", "0.25")
	seed.SourceVersion = "source-a"
	if err := consumer.Handle(ctx, accountEvent); err != nil {
		t.Fatal(err)
	}
	if err := consumer.Handle(ctx, seed); err != nil {
		t.Fatal(err)
	}

	sourceB := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440053", at.Add(2*time.Second), "0.50", "0.10")
	sourceB.SourceVersion = "source-b"
	sourceBClaim, err := st.ClaimEvent(ctx, sourceB)
	if err != nil || !sourceBClaim.Acquired {
		t.Fatalf("source B claim=%+v err=%v", sourceBClaim, err)
	}
	sourceAProcessingGap := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440054", at.Add(3*time.Second), "0.25", "0.05")
	sourceAProcessingGap.SourceVersion = "source-a"
	if err := consumer.Handle(ctx, sourceAProcessingGap); err != nil {
		t.Fatal(err)
	}
	watermark, found, err := st.LoadWatermark(ctx, "source-a")
	if err != nil || !found || watermark.Completeness != events.CompletenessComplete {
		t.Fatalf("source A watermark=%+v found=%v err=%v, want complete", watermark, found, err)
	}
	assertProjectionRowsCompleteness(t, st, events.CompletenessPartial)

	if err := st.FailEvent(ctx, sourceB, sourceBClaim, time.Now().UTC(), errors.New("source B permanent failure")); err != nil {
		t.Fatal(err)
	}
	sourceADeadGap := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440055", at.Add(4*time.Second), "0.25", "0.05")
	sourceADeadGap.SourceVersion = "source-a"
	if err := consumer.Handle(ctx, sourceADeadGap); err != nil {
		t.Fatal(err)
	}
	watermark, found, err = st.LoadWatermark(ctx, "source-a")
	if err != nil || !found || watermark.Completeness != events.CompletenessComplete {
		t.Fatalf("source A watermark with source B dead=%+v found=%v err=%v, want complete", watermark, found, err)
	}
	assertProjectionRowsCompleteness(t, st, events.CompletenessPartial)
}

func TestClaimAndFirstWatermarkCompletionShareProjectionLock(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 10, 3, 45, 0, 0, time.UTC)
	first := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440038", at, "1.00", "0.25")
	firstClaim, err := st.ClaimEvent(ctx, first)
	if err != nil || !firstClaim.Acquired {
		t.Fatalf("first claim=%+v err=%v", firstClaim, err)
	}

	blocker, err := st.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	blockerTx, err := blocker.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blockerTx.Rollback(ctx)
	if _, err := blockerTx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('relay-ops:externalization-projections', 0))`); err != nil {
		t.Fatal(err)
	}

	applyResult := make(chan error, 1)
	go func() {
		_, err := st.ApplyEvent(ctx, first, firstClaim, time.Now().UTC(), func(context.Context) error { return nil })
		applyResult <- err
	}()
	waitForAdvisoryWaiters(t, st, 1)

	second := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440039", at.Add(time.Second), "0.50", "0.10")
	type claimResult struct {
		claim events.Claim
		err   error
	}
	secondResult := make(chan claimResult, 1)
	go func() {
		claim, err := st.ClaimEvent(ctx, second)
		secondResult <- claimResult{claim: claim, err: err}
	}()
	waitForAdvisoryWaiters(t, st, 2)
	if err := blockerTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-applyResult; err != nil {
		t.Fatal(err)
	}
	claimed := <-secondResult
	if claimed.err != nil || !claimed.claim.Acquired {
		t.Fatalf("second claim=%+v err=%v", claimed.claim, claimed.err)
	}
	watermark, found, err := st.LoadWatermark(ctx, "sub2api-v1")
	if err != nil || !found || watermark.Completeness != events.CompletenessPartial {
		t.Fatalf("watermark=%+v found=%v err=%v, want partial", watermark, found, err)
	}
	var status string
	if err := st.pool.QueryRow(ctx, `SELECT status FROM relay_ops.externalization_events WHERE event_id=$1`, second.EventID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "processing" {
		t.Fatalf("second event status=%q, want processing", status)
	}
}

func assertExternalizationCompleteness(t *testing.T, st *Store, want string) {
	t.Helper()
	ctx := context.Background()
	watermark, found, err := st.LoadWatermark(ctx, "sub2api-v1")
	if err != nil || !found || watermark.Completeness != want {
		t.Fatalf("watermark=%+v found=%v err=%v, want completeness %q", watermark, found, err, want)
	}
	assertProjectionRowsCompleteness(t, st, want)
}

func assertProjectionRowsCompleteness(t *testing.T, st *Store, want string) {
	t.Helper()
	ctx := context.Background()
	for _, table := range []string{
		"account_read_models", "profitability_read_models", "accounting_read_models", "reconciliation_read_models",
	} {
		var values []string
		rows, err := st.pool.Query(ctx, `SELECT completeness FROM relay_ops.`+table)
		if err != nil {
			t.Fatalf("query %s completeness: %v", table, err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			values = append(values, value)
		}
		rows.Close()
		if len(values) == 0 {
			t.Fatalf("%s has no projection rows", table)
		}
		for _, value := range values {
			if value != want {
				t.Fatalf("%s completeness=%q, want %q", table, value, want)
			}
		}
	}
}

func waitForAdvisoryWaiters(t *testing.T, st *Store, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		var waiting int
		if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM pg_locks WHERE locktype='advisory' AND NOT granted`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("advisory lock waiters=%d, want at least %d", waiting, want)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestProjectionMigrationUpgradesPlaceholderSchemaInPlace(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := st.pool.Exec(ctx, `
		CREATE SCHEMA relay_ops;
		CREATE TABLE relay_ops.externalization_events (
		 event_id TEXT PRIMARY KEY, source_version TEXT NOT NULL, event_type TEXT NOT NULL,
		 occurred_at TIMESTAMPTZ NOT NULL, processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), payload JSONB NOT NULL
		);
		CREATE TABLE relay_ops.externalization_watermarks (
		 source TEXT PRIMARY KEY, last_event_id TEXT NOT NULL, occurred_at TIMESTAMPTZ NOT NULL,
		 processed_at TIMESTAMPTZ NOT NULL, completeness TEXT NOT NULL, calculation_version TEXT NOT NULL
		);
		CREATE TABLE relay_ops.externalization_dead_letters (
		 event_id TEXT PRIMARY KEY, error TEXT NOT NULL, payload JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE relay_ops.account_read_models (
		 account_id BIGINT PRIMARY KEY, status TEXT NOT NULL DEFAULT 'unknown', balance NUMERIC, currency TEXT,
		 observed_at TIMESTAMPTZ, generated_at TIMESTAMPTZ NOT NULL, source_watermark TEXT NOT NULL,
		 freshness_seconds BIGINT NOT NULL DEFAULT 0, completeness TEXT NOT NULL, calculation_version TEXT NOT NULL
		);
		INSERT INTO relay_ops.externalization_events
			(event_id, source_version, event_type, occurred_at, payload)
		VALUES ('legacy-event', 'legacy-v1', 'request.completed', '2026-08-09T00:00:00Z', '{"legacy":true}');
		INSERT INTO relay_ops.externalization_watermarks
			(source, last_event_id, occurred_at, processed_at, completeness, calculation_version)
		VALUES ('legacy-v1', 'legacy-event', '2026-08-09T00:00:00Z', '2026-08-09T00:00:01Z', 'complete', 'legacy-v1');
		INSERT INTO relay_ops.externalization_dead_letters
			(event_id, error, payload)
		VALUES ('legacy-dead', 'legacy failure', '{"legacy":true}');
			INSERT INTO relay_ops.account_read_models
				(account_id, status, balance, currency, observed_at, generated_at, source_watermark, completeness, calculation_version)
			VALUES (99, 'healthy', 12.50, 'USD', '2026-08-09T00:00:00Z', '2026-08-09T00:00:01Z', 'legacy-event', 'complete', 'accounts-v0');`); err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("upgrade placeholder schema: %v", err)
	}
	var legacyStatus string
	var legacyGeneration int64
	if err := st.pool.QueryRow(ctx, `
		SELECT status, claim_generation FROM relay_ops.externalization_events WHERE event_id='legacy-event'`).Scan(&legacyStatus, &legacyGeneration); err != nil {
		t.Fatal(err)
	}
	if legacyStatus != "processed" || legacyGeneration != 0 {
		t.Fatalf("legacy event status=%s generation=%d", legacyStatus, legacyGeneration)
	}
	var legacyAccounts, legacyDead int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.account_read_models WHERE account_id=99`).Scan(&legacyAccounts); err != nil {
		t.Fatal(err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.externalization_dead_letters WHERE event_id='legacy-dead'`).Scan(&legacyDead); err != nil {
		t.Fatal(err)
	}
	if legacyAccounts != 1 || legacyDead != 1 {
		t.Fatalf("legacy rows accounts=%d dead=%d", legacyAccounts, legacyDead)
	}
	var healthAt, balanceAt *time.Time
	var healthEventID, balanceEventID *string
	if err := st.pool.QueryRow(ctx, `
		SELECT health_occurred_at, health_event_id, balance_occurred_at, balance_event_id
		FROM relay_ops.account_read_models WHERE account_id=99`).Scan(&healthAt, &healthEventID, &balanceAt, &balanceEventID); err != nil {
		t.Fatal(err)
	}
	legacyObservedAt := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	if healthAt == nil || !healthAt.Equal(legacyObservedAt) || healthEventID == nil || *healthEventID != "legacy-event" ||
		balanceAt == nil || !balanceAt.Equal(legacyObservedAt) || balanceEventID == nil || *balanceEventID != "legacy-event" {
		t.Fatalf("legacy positions health=(%v,%v) balance=(%v,%v)", healthAt, healthEventID, balanceAt, balanceEventID)
	}
	accounts := projection.NewAccountsWithRepository(st)
	accountConsumer, err := events.NewPersistentConsumer(st, accounts)
	if err != nil {
		t.Fatal(err)
	}
	for _, delayed := range []events.Event{
		{
			EventID: "legacy-health-delayed", Type: events.AccountHealthChanged,
			OccurredAt: legacyObservedAt.Add(-time.Second), SourceVersion: "legacy-v1", ContractVersion: events.ContractVersion,
			Payload: []byte(`{"account_id":99,"status":"unhealthy","checked_at":"2026-08-08T23:59:59Z"}`),
		},
		{
			EventID: "legacy-balance-delayed", Type: events.AccountBalanceSnapshot,
			OccurredAt: legacyObservedAt.Add(-time.Second), SourceVersion: "legacy-v1", ContractVersion: events.ContractVersion,
			Payload: []byte(`{"account_id":99,"balance":"1.25","currency":"USD","captured_at":"2026-08-08T23:59:59Z"}`),
		},
	} {
		if err := accountConsumer.Handle(ctx, delayed); err != nil {
			t.Fatalf("consume delayed legacy account event: %v", err)
		}
	}
	var legacyAccountStatus, legacyBalance string
	if err := st.pool.QueryRow(ctx, `SELECT status, balance::text FROM relay_ops.account_read_models WHERE account_id=99`).Scan(&legacyAccountStatus, &legacyBalance); err != nil {
		t.Fatal(err)
	}
	if legacyAccountStatus != "healthy" || legacyBalance != "12.50" {
		t.Fatalf("delayed event overwrote legacy account status=%q balance=%q", legacyAccountStatus, legacyBalance)
	}
	event := externalizationRequestEvent("550e8400-e29b-41d4-a716-446655440041", time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC), "1.00", "0.25")
	consumer, err := events.NewPersistentConsumer(st, projection.NewProfitabilityWithRepository(st))
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.Handle(ctx, event); err != nil {
		t.Fatalf("consume after placeholder migration upgrade: %v", err)
	}
}

func externalizationRequestEvent(id string, occurredAt time.Time, charge, cost string) events.Event {
	return events.Event{
		EventID: id, Type: events.RequestCompleted, OccurredAt: occurredAt, SourceVersion: "sub2api-v1", ContractVersion: events.ContractVersion,
		Payload: []byte(`{"request_id":"request-` + id + `","account_id":7,"model":"gpt-test","prompt_tokens":1,"completion_tokens":1,"user_charge":"` + charge + `","actual_cost":"` + cost + `","cost_usd":"` + cost + `","currency":"USD"}`),
	}
}

func TestMigrateRejectsDuplicateActiveBillingAccountMappings(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := st.pool.Exec(ctx, initialMigration); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	if _, err := st.pool.Exec(ctx, `ALTER TABLE relay_ops.auth_sessions ADD COLUMN billing_account_id BIGINT`); err != nil {
		t.Fatalf("add legacy billing account mapping column: %v", err)
	}
	for _, name := range []string{"duplicate-billing-one", "duplicate-billing-two"} {
		upstreamID, err := st.CreateUpstream(ctx, Upstream{
			Name: name, Role: "production", BaseURL: "https://" + name + ".example/v1", AdapterType: "newapi", Enabled: true,
		})
		if err != nil {
			t.Fatalf("CreateUpstream %q: %v", name, err)
		}
		if _, err := st.pool.Exec(ctx, `
			INSERT INTO relay_ops.auth_sessions (upstream_id, secret_ref, auth_mode, status, login_url, scope, billing_account_id)
			VALUES ($1, 'file:/run/secrets/upstream-sessions/redacted', 'bearer', 'active', 'https://billing.example/login', 'billing_read', 8123)`, upstreamID); err != nil {
			t.Fatalf("insert legacy duplicate mapping %q: %v", name, err)
		}
	}

	err := st.Migrate(ctx)
	if err == nil {
		t.Fatal("Migrate succeeded with duplicate active billing mappings")
	}
	for _, want := range []string{"migration 010 blocked", "billing account 8123", "2 active billing_read mappings"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Migrate error = %q, want actionable diagnostic containing %q", err, want)
		}
	}
	var count int
	if err := st.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM relay_ops.auth_sessions
		WHERE billing_account_id=8123 AND status='active' AND scope='billing_read'`).Scan(&count); err != nil {
		t.Fatalf("count legacy duplicate mappings: %v", err)
	}
	if count != 2 {
		t.Fatalf("legacy duplicate mappings = %d, want 2 (preflight must not mutate data)", count)
	}
}

func TestProvisionBillingSourceIsAtomicIdempotentAndRejectsConflicts(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO relay_ops.public_groups
			(group_id, name, enabled, customer_visible, user_multiplier_bps, source_revision, last_seen_at)
		VALUES (71, 'billing-public', TRUE, TRUE, 10000, 'test', NOW())`); err != nil {
		t.Fatal(err)
	}
	record := billing.BillingProvisionRecord{
		Production: upstreams.ProductionRecord{
			Source: upstreams.Source{
				Name: "billing-primary", Role: upstreams.RoleProduction, BaseURL: "https://billing-primary.example/v1",
				PricingURL: "https://billing-primary.example/pricing", UsageURL: "https://billing-primary.example/usage",
				AdapterType: upstreams.AdapterSub2API, GroupIDs: []int64{71}, Enabled: true,
			},
			Audit: upstreams.AuditEvent{ActorUserID: 42, Action: "upstream.production.create", ObjectType: "upstream", AfterSummary: map[string]string{"name": "billing-primary"}},
		},
		Session: billing.BillingReadSessionRecord{
			LoginURL: "https://billing-primary.example/login", BillingAccountID: 8123,
			Secret: billing.SessionSecretRef{SecretRef: "file:/run/secrets/upstream-sessions/billing-primary", Kind: "upstream_usage_session", Fingerprint: strings.Repeat("a", 64), LastFour: "token"},
			Audit:  billing.SessionAuditEvent{ActorUserID: 42, Action: "upstream.billing_session.provision", ObjectType: "auth_session", AfterSummary: map[string]string{"scope": "billing_read"}},
		},
	}
	first, err := st.ProvisionBillingSource(ctx, record)
	if err != nil || first.AlreadyConfigured || first.UpstreamID <= 0 || first.BillingAccountID != 8123 {
		t.Fatalf("first provision=%#v err=%v", first, err)
	}
	second, err := st.ProvisionBillingSource(ctx, record)
	if err != nil || !second.AlreadyConfigured || second.UpstreamID != first.UpstreamID {
		t.Fatalf("idempotent provision=%#v err=%v", second, err)
	}
	rotatedSecret := record
	rotatedSecret.Session.Secret.Fingerprint = strings.Repeat("d", 64)
	if _, err := st.ProvisionBillingSource(ctx, rotatedSecret); !errors.Is(err, billing.ErrBillingProvisionConflict) {
		t.Fatalf("rotated bearer declaration error=%v, want billing provision conflict", err)
	}
	var sessions, audits int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.auth_sessions WHERE billing_account_id=8123`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.audit_events WHERE object_id=$1`, strconv.FormatInt(int64(first.UpstreamID), 10)).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 || audits != 2 {
		t.Fatalf("sessions=%d audits=%d", sessions, audits)
	}
	conflict := record
	conflict.Production.Source.Name = "billing-secondary"
	conflict.Production.Source.BaseURL = "https://billing-secondary.example/v1"
	conflict.Production.Source.PricingURL = "https://billing-secondary.example/pricing"
	conflict.Production.Source.UsageURL = "https://billing-secondary.example/usage"
	conflict.Session.Secret.SecretRef = "file:/run/secrets/upstream-sessions/billing-secondary"
	conflict.Session.Secret.Fingerprint = strings.Repeat("b", 64)
	if _, err := st.ProvisionBillingSource(ctx, conflict); !errors.Is(err, billing.ErrBillingProvisionConflict) {
		t.Fatalf("duplicate billing account error=%v", err)
	}
	secretConflict := record
	secretConflict.Production.Source.Name = "billing-secret-reuse"
	secretConflict.Production.Source.BaseURL = "https://billing-secret-reuse.example/v1"
	secretConflict.Production.Source.PricingURL = "https://billing-secret-reuse.example/pricing"
	secretConflict.Production.Source.UsageURL = "https://billing-secret-reuse.example/usage"
	secretConflict.Session.BillingAccountID = 9123
	if _, err := st.ProvisionBillingSource(ctx, secretConflict); !errors.Is(err, billing.ErrBillingProvisionConflict) {
		t.Fatalf("reused billing secret reference error=%v", err)
	}
}

func TestProvisionBillingSourceConcurrentSecretReferenceConflicts(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION relay_ops.delay_billing_secret_insert() RETURNS trigger AS $$
		BEGIN
			IF NEW.secret_ref = 'file:/run/secrets/upstream-sessions/shared-billing-token' THEN
				PERFORM pg_sleep(0.2);
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER delay_billing_secret_insert
			BEFORE INSERT ON relay_ops.secret_refs
			FOR EACH ROW EXECUTE FUNCTION relay_ops.delay_billing_secret_insert();`); err != nil {
		t.Fatalf("install concurrent secret reference test barrier: %v", err)
	}

	record := func(name string, billingAccountID int64) billing.BillingProvisionRecord {
		return billing.BillingProvisionRecord{
			Production: upstreams.ProductionRecord{
				Source: upstreams.Source{
					Name: name, Role: upstreams.RoleProduction, BaseURL: "https://" + name + ".example/v1",
					PricingURL: "https://" + name + ".example/pricing", UsageURL: "https://" + name + ".example/usage",
					AdapterType: upstreams.AdapterSub2API, Enabled: true,
				},
				Audit: upstreams.AuditEvent{ActorUserID: 42, Action: "upstream.production.create", ObjectType: "upstream", AfterSummary: map[string]string{"name": name}},
			},
			Session: billing.BillingReadSessionRecord{
				LoginURL: "https://" + name + ".example/login", BillingAccountID: billingAccountID,
				Secret: billing.SessionSecretRef{SecretRef: "file:/run/secrets/upstream-sessions/shared-billing-token", Kind: "upstream_usage_session", Fingerprint: strings.Repeat("c", 64), LastFour: "token"},
				Audit:  billing.SessionAuditEvent{ActorUserID: 42, Action: "upstream.billing_session.provision", ObjectType: "auth_session", AfterSummary: map[string]string{"scope": "billing_read"}},
			},
		}
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, input := range []billing.BillingProvisionRecord{record("billing-concurrent-one", 8123), record("billing-concurrent-two", 9123)} {
		go func(input billing.BillingProvisionRecord) {
			<-start
			_, err := st.ProvisionBillingSource(ctx, input)
			results <- err
		}(input)
	}
	close(start)

	var success, conflicts int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			success++
		case errors.Is(err, billing.ErrBillingProvisionConflict):
			conflicts++
		default:
			t.Fatalf("concurrent billing provision error = %v", err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("concurrent shared secret results: success=%d conflicts=%d, want one of each", success, conflicts)
	}
	var sessions int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.auth_sessions WHERE secret_ref=$1`, "file:/run/secrets/upstream-sessions/shared-billing-token").Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 {
		t.Fatalf("sessions using shared secret reference=%d, want 1", sessions)
	}
}

func TestBillingProvisionLockKeysUseSeparateNamespaces(t *testing.T) {
	secret := billingProvisionLockKey("secret", "X")
	source := billingProvisionLockKey("source", "X")
	if secret == source {
		t.Fatalf("secret and source lock keys must differ: %q", secret)
	}
	if secret != "billing-provision:secret:X" || source != "billing-provision:source:X" {
		t.Fatalf("billing lock keys = %q, %q", secret, source)
	}
}

func TestPricingSnapshotsAreAppendOnly(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	upstreamID, err := st.CreateUpstream(ctx, Upstream{Name: "wawazz", Role: "candidate", BaseURL: "https://wawazz.example/v1", AdapterType: "newapi", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	fetchedAt := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	for _, hash := range []string{"hash-one", "hash-two"} {
		if _, err := st.AppendPricingSnapshot(ctx, PricingSnapshot{UpstreamID: upstreamID, SourceURL: "https://wawazz.example/pricing", SourceType: "public_page", FetchedAt: fetchedAt, ContentHash: hash, NormalizedJSON: []byte(`{"multiplier_bps":500}`), EvidenceLevel: "public_page"}); err != nil {
			t.Fatalf("AppendPricingSnapshot: %v", err)
		}
	}
	count, err := st.CountPricingSnapshots(ctx, upstreamID)
	if err != nil || count != 2 {
		t.Fatalf("snapshot count = %d, err = %v", count, err)
	}
}

func TestNativeSyncPreservesQualificationAndDeduplicatesMetricRefs(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	seenAt := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	record := sub2api.PublicGroupRecord{
		GroupID: 3, Name: "GPT-Pro", Platform: "openai", Enabled: true, CustomerVisible: true,
		UserMultiplierBPS: 10_000, ChannelIDs: []int64{7}, MonitorIDs: []int64{9},
		HealthGate: "pending", SourceRevision: "revision-one", LastSeenAt: seenAt,
	}
	if err := st.UpsertPublicGroup(ctx, record); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `UPDATE relay_ops.public_groups SET health_gate = 'qualified' WHERE group_id = 3`); err != nil {
		t.Fatal(err)
	}
	record.SourceRevision = "revision-two"
	if err := st.UpsertPublicGroup(ctx, record); err != nil {
		t.Fatal(err)
	}
	var gate, revision string
	if err := st.pool.QueryRow(ctx, `SELECT health_gate, source_revision FROM relay_ops.public_groups WHERE group_id = 3`).Scan(&gate, &revision); err != nil {
		t.Fatal(err)
	}
	if gate != "qualified" || revision != "revision-two" {
		t.Fatalf("gate=%q revision=%q", gate, revision)
	}

	ref := sub2api.MetricRef{SourceKind: "sub2api_native_monitor", ExternalID: "monitor:9", WindowStart: seenAt, WindowEnd: seenAt.Add(time.Minute), PayloadHash: "abc", SchemaVersion: sub2api.NativeMetricSchemaV1}
	if err := st.AppendMetricRef(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendMetricRef(ctx, ref); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.metric_refs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("metric ref count = %d", count)
	}
}

func TestCreateCandidatePersistsSecretMetadataAndAuditAtomically(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	record := candidates.CreateRecord{
		Candidate: candidates.Candidate{Name: "candidate", BaseURL: "https://candidate.example/v1", PricingURL: "https://candidate.example/pricing", UsageURL: "https://candidate.example/usage", ProbeSecretRef: "file:/run/secrets/candidate"},
		SecretRef: candidates.SecretRef{SecretRef: "file:/run/secrets/candidate", Kind: "candidate_probe_key", OwnerScope: "candidate", Fingerprint: "fingerprint", LastFour: "5678"},
		Audit:     candidates.AuditEvent{ActorUserID: 42, Action: "candidate.create", ObjectType: "upstream", AfterSummary: map[string]string{"name": "candidate"}},
	}
	id, err := st.CreateCandidate(ctx, record)
	if err != nil || id == 0 {
		t.Fatalf("CreateCandidate = %d, %v", id, err)
	}
	var secretRef, fingerprint, lastFour string
	if err := st.pool.QueryRow(ctx, `SELECT secret_ref, fingerprint, last_four FROM relay_ops.secret_refs WHERE owner_scope = 'candidate'`).Scan(&secretRef, &fingerprint, &lastFour); err != nil {
		t.Fatal(err)
	}
	if secretRef != "file:/run/secrets/candidate" || fingerprint != "fingerprint" || lastFour != "5678" {
		t.Fatalf("secret metadata = %q %q %q", secretRef, fingerprint, lastFour)
	}
	var auditCount int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.audit_events WHERE actor_user_id = 42 AND action = 'candidate.create'`).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("audit count = %d, %v", auditCount, err)
	}
	if _, err := st.CreateCandidate(ctx, record); !errors.Is(err, candidates.ErrConflict) {
		t.Fatalf("duplicate error = %v", err)
	}
	second := record
	second.Candidate.Name = "candidate-with-reused-key"
	second.Candidate.BaseURL = "https://candidate-2.example/v1"
	second.Candidate.ProbeSecretRef = "file:/run/secrets/candidate-2"
	second.SecretRef.SecretRef = "file:/run/secrets/candidate-2"
	second.SecretRef.OwnerScope = "candidate-with-reused-key"
	second.Audit.AfterSummary = map[string]string{"name": second.Candidate.Name}
	if _, err := st.CreateCandidate(ctx, second); !errors.Is(err, candidates.ErrConflict) {
		t.Fatalf("reused fingerprint error = %v, want ErrConflict", err)
	}
}

func TestCreateProductionUpstreamLinksOnlyCustomerVisibleGroups(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	visible := sub2api.PublicGroupRecord{
		GroupID: 3, Name: "GPT-Pro", Platform: "openai", Enabled: true, CustomerVisible: true,
		UserMultiplierBPS: 10_000, ChannelIDs: []int64{7}, MonitorIDs: []int64{9},
		HealthGate: "qualified", SourceRevision: "visible", LastSeenAt: time.Now().UTC(),
	}
	if err := st.UpsertPublicGroup(ctx, visible); err != nil {
		t.Fatal(err)
	}
	record := upstreams.ProductionRecord{
		Source: upstreams.Source{
			Name: "Neko", Role: upstreams.RoleProduction, BaseURL: "https://neko.example/v1",
			PricingURL: "https://neko.example/pricing", UsageURL: "https://neko.example/usage",
			MonitorID: 9, GroupIDs: []int64{3}, Enabled: true,
		},
		Audit: upstreams.AuditEvent{ActorUserID: 42, Action: "upstream.production.create", ObjectType: "upstream", AfterSummary: map[string]string{"name": "Neko"}},
	}
	id, err := st.CreateProduction(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := st.ListProduction(ctx)
	if err != nil || len(sources) != 1 || sources[0].ID != id || len(sources[0].GroupIDs) != 1 || sources[0].GroupIDs[0] != 3 {
		t.Fatalf("sources = %#v, err = %v", sources, err)
	}
	var secretCount int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.secret_refs WHERE owner_scope = 'Neko'`).Scan(&secretCount); err != nil || secretCount != 0 {
		t.Fatalf("secret count = %d, err = %v", secretCount, err)
	}

	record.Source.Name = "Missing group"
	record.Source.BaseURL = "https://missing.example/v1"
	record.Source.GroupIDs = []int64{404}
	if _, err := st.CreateProduction(ctx, record); !errors.Is(err, upstreams.ErrGroupUnavailable) {
		t.Fatalf("missing group error = %v", err)
	}
}

func TestListAndDisableCandidatePersistOnlyStateAndAudit(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	record := candidates.CreateRecord{
		Candidate: candidates.Candidate{Name: "candidate", BaseURL: "https://candidate.example/v1", PricingURL: "https://candidate.example/pricing", UsageURL: "https://candidate.example/usage", ProbeSecretRef: "file:/run/secrets/candidate"},
		SecretRef: candidates.SecretRef{SecretRef: "file:/run/secrets/candidate", Kind: "candidate_probe_key", OwnerScope: "candidate-list", Fingerprint: "fingerprint", LastFour: "5678"},
		Audit:     candidates.AuditEvent{ActorUserID: 42, Action: "candidate.create", ObjectType: "upstream", AfterSummary: map[string]string{"name": "candidate"}},
	}
	id, err := st.CreateCandidate(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListCandidates(ctx)
	if err != nil || len(listed) != 1 || listed[0].ID != id || listed[0].ProbeSecretRef != record.SecretRef.SecretRef {
		t.Fatalf("ListCandidates = %#v, %v", listed, err)
	}
	audit := candidates.AuditEvent{ActorUserID: 99, Action: "candidate.disable", ObjectType: "upstream", AfterSummary: map[string]string{"enabled": "false"}}
	if err := st.DisableCandidate(ctx, id, audit); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	if err := st.pool.QueryRow(ctx, `SELECT enabled FROM relay_ops.upstreams WHERE id = $1`, id).Scan(&enabled); err != nil || enabled {
		t.Fatalf("enabled=%v err=%v", enabled, err)
	}
	var auditCount int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.audit_events WHERE actor_user_id = 99 AND action = 'candidate.disable'`).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("audit count=%d err=%v", auditCount, err)
	}
}

func TestUsageSessionExpiryUpdatesStatusAndCostsAppend(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateUpstream(ctx, Upstream{Name: "usage-upstream", Role: "candidate", BaseURL: "https://usage-upstream.example/v1", AdapterType: "newapi", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `INSERT INTO relay_ops.auth_sessions (upstream_id, secret_ref, auth_mode, status, login_url, scope) VALUES ($1, 'file:/secret', 'cookie', 'active', 'https://usage-upstream.example/login', 'usage_read')`, id); err != nil {
		t.Fatal(err)
	}
	observed := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	for _, at := range []time.Time{observed, observed.Add(time.Hour), observed.Add(25 * time.Hour)} {
		if err := st.RecordExpired(ctx, id, "https://usage-upstream.example/login", at); err != nil {
			t.Fatalf("RecordExpired(%s): %v", at, err)
		}
	}
	if err := st.RecordHealthy(ctx, id, observed.Add(26*time.Hour)); err != nil {
		t.Fatal(err)
	}
	evidence := billing.UsageEvidence{UpstreamID: id, ObservedAt: observed, StandardCost: 10_000_000, ActualCost: 1_000_000, HasActualCost: true, EffectiveMultiplier: 1_000, Note: "辅助证据"}
	if err := st.AppendCostObservation(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	var status string
	var costCount int
	if err := st.pool.QueryRow(ctx, `SELECT status FROM relay_ops.auth_sessions WHERE upstream_id=$1`, id).Scan(&status); err != nil || status != "active" {
		t.Fatalf("status=%q err=%v", status, err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM relay_ops.cost_observations WHERE upstream_id=$1`, id).Scan(&costCount); err != nil || costCount != 1 {
		t.Fatalf("cost count=%d err=%v", costCount, err)
	}
}

func TestListBillingSourcesUsesEnabledProductionActiveSessions(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	eligible, err := st.CreateUpstream(ctx, Upstream{Name: "billing-active", Role: "production", BaseURL: "https://billing-active.example/v1", AdapterType: "sub2api", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := st.CreateUpstream(ctx, Upstream{Name: "billing-disabled", Role: "production", BaseURL: "https://billing-disabled.example/v1", AdapterType: "newapi", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	unsupported, err := st.CreateUpstream(ctx, Upstream{Name: "billing-unsupported", Role: "production", BaseURL: "https://billing-unsupported.example/v1", AdapterType: "openai", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	cookieSession, err := st.CreateUpstream(ctx, Upstream{Name: "billing-cookie", Role: "production", BaseURL: "https://billing-cookie.example/v1", AdapterType: "newapi", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	usageBearer, err := st.CreateUpstream(ctx, Upstream{Name: "billing-usage-only", Role: "production", BaseURL: "https://billing-usage-only.example/v1", AdapterType: "newapi", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id               domain.UpstreamID
		status           string
		authMode         string
		ref              string
		scope            string
		billingAccountID *int64
	}{
		{eligible, "active", "bearer", "file:/run/secrets/upstream-sessions/active", "billing_read", int64Pointer(731)},
		{disabled, "active", "bearer", "file:/run/secrets/upstream-sessions/disabled", "billing_read", int64Pointer(732)},
		{unsupported, "active", "bearer", "file:/run/secrets/upstream-sessions/unsupported", "billing_read", int64Pointer(733)},
		{cookieSession, "active", "cookie", "file:/run/secrets/upstream-sessions/cookie", "usage_read", nil},
		{usageBearer, "active", "bearer", "file:/run/secrets/upstream-sessions/usage-only", "usage_read", int64Pointer(734)},
	} {
		if _, err := st.pool.Exec(ctx, `
			INSERT INTO relay_ops.auth_sessions (upstream_id, secret_ref, auth_mode, status, login_url, scope, billing_account_id)
			VALUES ($1, $2, $3, $4, 'https://billing.example/login', $5, $6)`, row.id, row.ref, row.authMode, row.status, row.scope, row.billingAccountID); err != nil {
			t.Fatal(err)
		}
	}
	sources, err := st.ListBillingSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("sources=%#v", sources)
	}
	got := sources[0]
	if got.AccountID != 731 || got.BaseURL != "https://billing-active.example/v1" || got.AdapterType != "sub2api" || got.SecretRef != "file:/run/secrets/upstream-sessions/active" {
		t.Fatalf("source=%#v", got)
	}
}

func TestSchedulerClaimIsConcurrentAndRestartSafe(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	results := make(chan bool, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			claimed, err := st.Claim(ctx, "candidate-cycle:17", now, 6*time.Hour)
			if err != nil {
				t.Errorf("Claim: %v", err)
			}
			results <- claimed
		}()
	}
	wait.Wait()
	close(results)
	claimedCount := 0
	for claimed := range results {
		if claimed {
			claimedCount++
		}
	}
	if claimedCount != 1 {
		t.Fatalf("claimed count=%d", claimedCount)
	}
	if claimed, err := st.Claim(ctx, "candidate-cycle:17", now.Add(5*time.Hour), 6*time.Hour); err != nil || claimed {
		t.Fatalf("early claim=%v err=%v", claimed, err)
	}
	if claimed, err := st.Claim(ctx, "candidate-cycle:17", now.Add(6*time.Hour), 6*time.Hour); err != nil || !claimed {
		t.Fatalf("due claim=%v err=%v", claimed, err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("RELAY_OPS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("RELAY_OPS_TEST_DATABASE_URL is not set")
	}
	secret := filepath.Join(t.TempDir(), "database-url")
	if err := os.WriteFile(secret, []byte(url), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := Open(context.Background(), secret)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(st.Close)
	if _, err := st.pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS relay_ops CASCADE"); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	return st
}

var _ domain.UpstreamID
