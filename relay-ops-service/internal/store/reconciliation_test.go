package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/reconciliation"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

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
