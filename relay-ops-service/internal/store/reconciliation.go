package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/reconciliation"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

const upstreamCostAmountScale = 10

func (s *Store) RecordUpstreamCostAttempt(ctx context.Context, raw reconciliation.AttemptInput) (reconciliation.Attempt, bool, error) {
	input, err := reconciliation.ValidateAttempt(raw)
	if err != nil {
		return reconciliation.Attempt{}, false, err
	}
	var attempt reconciliation.Attempt
	var userCharge string
	var upstreamRequestID *string
	var inserted bool
	err = s.pool.QueryRow(ctx, `
		INSERT INTO relay_ops.upstream_cost_attempts (
			attempt_id, local_request_id, account_id, adapter_type, upstream_request_id,
			model, input_tokens, output_tokens, user_charge, currency, request_status, completed_at
		) VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (attempt_id) DO UPDATE SET attempt_id=EXCLUDED.attempt_id
		RETURNING id, attempt_id, local_request_id, account_id, adapter_type, upstream_request_id,
			model, input_tokens, output_tokens, user_charge::text, currency, request_status,
			reconcile_status, completed_at, matched_at, created_at, updated_at, (xmax=0)`,
		input.AttemptID, input.LocalRequestID, input.AccountID, input.AdapterType, input.UpstreamRequestID,
		input.Model, input.InputTokens, input.OutputTokens, input.UserCharge.Round(upstreamCostAmountScale).String(),
		input.Currency, input.RequestStatus, input.CompletedAt,
	).Scan(&attempt.ID, &attempt.AttemptID, &attempt.LocalRequestID, &attempt.AccountID, &attempt.AdapterType,
		&upstreamRequestID, &attempt.Model, &attempt.InputTokens, &attempt.OutputTokens, &userCharge,
		&attempt.Currency, &attempt.RequestStatus, &attempt.ReconcileStatus, &attempt.CompletedAt,
		&attempt.MatchedAt, &attempt.CreatedAt, &attempt.UpdatedAt, &inserted)
	if err != nil {
		return reconciliation.Attempt{}, false, fmt.Errorf("record upstream cost attempt: %w", err)
	}
	attempt.UserCharge, err = decimalFromText(userCharge)
	if err != nil {
		return reconciliation.Attempt{}, false, err
	}
	if upstreamRequestID != nil {
		attempt.UpstreamRequestID = *upstreamRequestID
	}
	if !inserted && !sameAttempt(attempt, input) {
		return reconciliation.Attempt{}, false, ErrConflict
	}
	return attempt, inserted, nil
}

func sameAttempt(stored reconciliation.Attempt, input reconciliation.AttemptInput) bool {
	return stored.AttemptID == input.AttemptID && stored.LocalRequestID == input.LocalRequestID &&
		stored.AccountID == input.AccountID && stored.AdapterType == input.AdapterType &&
		stored.UpstreamRequestID == input.UpstreamRequestID && stored.Model == input.Model &&
		stored.InputTokens == input.InputTokens && stored.OutputTokens == input.OutputTokens &&
		stored.UserCharge.Equal(input.UserCharge.Round(upstreamCostAmountScale)) &&
		stored.Currency == input.Currency && stored.RequestStatus == input.RequestStatus &&
		stored.CompletedAt.Equal(input.CompletedAt)
}

func (s *Store) CreateAutomaticUpstreamCost(ctx context.Context, raw reconciliation.AutomaticTransactionInput) (reconciliation.Transaction, bool, error) {
	input, err := reconciliation.ValidateAutomaticTransaction(raw)
	if err != nil {
		return reconciliation.Transaction{}, false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return reconciliation.Transaction{}, false, fmt.Errorf("begin automatic upstream cost: %w", err)
	}
	defer tx.Rollback(ctx)
	var accountID int64
	var status string
	if err := tx.QueryRow(ctx, `SELECT account_id, reconcile_status FROM relay_ops.upstream_cost_attempts WHERE id=$1 FOR UPDATE`, input.AttemptID).Scan(&accountID, &status); err != nil {
		return reconciliation.Transaction{}, false, fmt.Errorf("lock upstream cost attempt: %w", err)
	}
	if accountID != input.AccountID {
		return reconciliation.Transaction{}, false, ErrConflict
	}
	effective := status != string(reconciliation.StatusManual) && status != string(reconciliation.StatusConflict)
	transaction, inserted, err := insertUpstreamCostTransaction(ctx, tx, input.AttemptID, input.AccountID,
		input.SourceType, input.SourceRecordID, input.Amount, input.Currency, effective, input.OccurredAt,
		input.IdempotencyKey, "", nil)
	if err != nil {
		return reconciliation.Transaction{}, false, err
	}
	if effective {
		if _, err := tx.Exec(ctx, `
			UPDATE relay_ops.upstream_cost_attempts
			SET reconcile_status='matched', matched_at=$2, updated_at=NOW()
			WHERE id=$1;
			UPDATE relay_ops.upstream_reconciliation_exceptions
			SET resolved_at=$2, resolution_type='automatic', last_checked_at=$2
			WHERE attempt_id=$1 AND resolved_at IS NULL`, input.AttemptID, time.Now().UTC()); err != nil {
			return reconciliation.Transaction{}, false, fmt.Errorf("mark automatic upstream cost matched: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE relay_ops.upstream_cost_attempts SET reconcile_status='conflict', updated_at=NOW() WHERE id=$1;
			INSERT INTO relay_ops.upstream_reconciliation_exceptions
				(attempt_id, reason_code, details, first_detected_at, last_checked_at)
			VALUES ($1, 'late_automatic_after_manual', $2, NOW(), NOW())
			ON CONFLICT (attempt_id) DO UPDATE SET reason_code=EXCLUDED.reason_code,
				details=EXCLUDED.details, last_checked_at=NOW(), resolved_at=NULL, resolution_type=NULL`,
			input.AttemptID, "automatic record "+input.SourceRecordID+" arrived after manual adjustment"); err != nil {
			return reconciliation.Transaction{}, false, fmt.Errorf("mark upstream cost conflict: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return reconciliation.Transaction{}, false, fmt.Errorf("commit automatic upstream cost: %w", err)
	}
	return transaction, inserted, nil
}

func (s *Store) CreateManualUpstreamCost(ctx context.Context, raw reconciliation.ManualAdjustmentInput) (reconciliation.Transaction, bool, error) {
	input, err := reconciliation.ValidateManualAdjustment(raw)
	if err != nil {
		return reconciliation.Transaction{}, false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return reconciliation.Transaction{}, false, fmt.Errorf("begin manual upstream cost: %w", err)
	}
	defer tx.Rollback(ctx)
	var accountID int64
	var currency string
	var status string
	if err := tx.QueryRow(ctx, `SELECT account_id, currency, reconcile_status FROM relay_ops.upstream_cost_attempts WHERE id=$1 FOR UPDATE`, input.AttemptID).Scan(&accountID, &currency, &status); err != nil {
		return reconciliation.Transaction{}, false, fmt.Errorf("lock manual upstream cost attempt: %w", err)
	}
	if status == string(reconciliation.StatusMatched) || status == string(reconciliation.StatusConflict) {
		return reconciliation.Transaction{}, false, ErrConflict
	}
	actorID := input.ActorUserID
	transaction, inserted, err := insertUpstreamCostTransaction(ctx, tx, input.AttemptID, accountID,
		reconciliation.SourceManualAdjustment, "", input.Amount, currency, true, time.Now().UTC(),
		input.IdempotencyKey, input.Notes, &actorID)
	if err != nil {
		return reconciliation.Transaction{}, false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE relay_ops.upstream_cost_attempts
		SET reconcile_status='manual', matched_at=NOW(), updated_at=NOW() WHERE id=$1;
		UPDATE relay_ops.upstream_reconciliation_exceptions
		SET resolved_at=NOW(), resolution_type='manual', last_checked_at=NOW()
		WHERE attempt_id=$1 AND resolved_at IS NULL`, input.AttemptID); err != nil {
		return reconciliation.Transaction{}, false, fmt.Errorf("mark manual upstream cost matched: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return reconciliation.Transaction{}, false, fmt.Errorf("commit manual upstream cost: %w", err)
	}
	return transaction, inserted, nil
}

type reconciliationExecer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func insertUpstreamCostTransaction(ctx context.Context, executor reconciliationExecer, attemptID, accountID int64,
	sourceType reconciliation.TransactionSource, sourceRecordID string, amount decimal.Decimal, currency string,
	effective bool, occurredAt time.Time, idempotencyKey, notes string, actorID *int64,
) (reconciliation.Transaction, bool, error) {
	var transaction reconciliation.Transaction
	var amountText string
	var attemptIDValue *int64
	var sourceRecordIDValue *string
	var inserted bool
	err := executor.QueryRow(ctx, `
		INSERT INTO relay_ops.upstream_cost_transactions (
			attempt_id, account_id, source_type, source_record_id, amount, currency, effective,
			occurred_at, idempotency_key, notes, created_by_user_id
		) VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key
		RETURNING id, attempt_id, account_id, source_type, source_record_id, amount::text, currency,
			effective, occurred_at, idempotency_key, notes, created_by_user_id, created_at, (xmax=0)`,
		attemptID, accountID, sourceType, strings.TrimSpace(sourceRecordID), amount.Round(upstreamCostAmountScale).String(),
		currency, effective, occurredAt.UTC(), idempotencyKey, notes, actorID,
	).Scan(&transaction.ID, &attemptIDValue, &transaction.AccountID, &transaction.SourceType,
		&sourceRecordIDValue, &amountText, &transaction.Currency, &transaction.Effective,
		&transaction.OccurredAt, &transaction.IdempotencyKey, &transaction.Notes,
		&transaction.CreatedByUserID, &transaction.CreatedAt, &inserted)
	if err != nil {
		return reconciliation.Transaction{}, false, fmt.Errorf("insert upstream cost transaction: %w", err)
	}
	transaction.AttemptID = attemptIDValue
	if sourceRecordIDValue != nil {
		transaction.SourceRecordID = *sourceRecordIDValue
	}
	transaction.Amount, err = decimalFromText(amountText)
	if err != nil {
		return reconciliation.Transaction{}, false, err
	}
	return transaction, inserted, nil
}

func (s *Store) MarkOverdueUpstreamCostExceptions(ctx context.Context, now time.Time, grace time.Duration) (int64, error) {
	if grace <= 0 {
		return 0, fmt.Errorf("grace duration must be positive")
	}
	result, err := s.pool.Exec(ctx, `
		WITH overdue AS (
			UPDATE relay_ops.upstream_cost_attempts
			SET reconcile_status='exception', updated_at=NOW()
			WHERE reconcile_status='pending' AND completed_at <= $1
			RETURNING id
		)
		INSERT INTO relay_ops.upstream_reconciliation_exceptions
			(attempt_id, reason_code, details, retry_count, first_detected_at, last_checked_at)
		SELECT id, 'upstream_record_missing', '', 0, $2, $2 FROM overdue
		ON CONFLICT (attempt_id) DO UPDATE SET retry_count=relay_ops.upstream_reconciliation_exceptions.retry_count+1,
			last_checked_at=$2
	`, now.UTC().Add(-grace), now.UTC())
	if err != nil {
		return 0, fmt.Errorf("mark overdue upstream cost exceptions: %w", err)
	}
	return result.RowsAffected(), nil
}

func (s *Store) ListPendingUpstreamCostAttempts(ctx context.Context, accountID int64, start, end time.Time, limit int) ([]reconciliation.Attempt, error) {
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, attempt_id, local_request_id, account_id, adapter_type, COALESCE(upstream_request_id,''),
			model, input_tokens, output_tokens, user_charge::text, currency, request_status,
			reconcile_status, completed_at, matched_at, created_at, updated_at
		FROM relay_ops.upstream_cost_attempts
		WHERE account_id=$1 AND completed_at >= $2 AND completed_at < $3
			AND reconcile_status IN ('pending','exception','matched','manual')
		ORDER BY completed_at, id LIMIT $4`, accountID, start.UTC(), end.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list pending upstream cost attempts: %w", err)
	}
	defer rows.Close()
	attempts := make([]reconciliation.Attempt, 0)
	for rows.Next() {
		var item reconciliation.Attempt
		var amount string
		if err := rows.Scan(&item.ID, &item.AttemptID, &item.LocalRequestID, &item.AccountID, &item.AdapterType,
			&item.UpstreamRequestID, &item.Model, &item.InputTokens, &item.OutputTokens, &amount, &item.Currency,
			&item.RequestStatus, &item.ReconcileStatus, &item.CompletedAt, &item.MatchedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan pending upstream cost attempt: %w", err)
		}
		item.UserCharge, err = decimalFromText(amount)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending upstream cost attempts: %w", err)
	}
	return attempts, nil
}

func (s *Store) ReadReconciliationSummary(ctx context.Context, accountID int64, start, end time.Time, currency string) (reconciliation.Summary, error) {
	if !start.Before(end) {
		return reconciliation.Summary{}, fmt.Errorf("summary time window is invalid")
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return reconciliation.Summary{}, fmt.Errorf("currency must be a three-letter code")
	}
	var summary reconciliation.Summary
	var coverageText, upstreamText, userText string
	err := s.pool.QueryRow(ctx, `
		WITH attempts AS (
			SELECT id, user_charge, reconcile_status
			FROM relay_ops.upstream_cost_attempts
			WHERE ($1=0 OR account_id=$1) AND currency=$4 AND completed_at >= $2 AND completed_at < $3
		), costs AS (
			SELECT COALESCE(SUM(t.amount) FILTER (WHERE t.effective), 0) AS upstream_cost
			FROM relay_ops.upstream_cost_transactions t JOIN attempts a ON a.id=t.attempt_id
		)
		SELECT COUNT(*), COUNT(*) FILTER (WHERE reconcile_status IN ('matched','manual')),
			COUNT(*) FILTER (WHERE reconcile_status IN ('pending','exception')),
			COUNT(*) FILTER (WHERE reconcile_status='conflict'),
			CASE WHEN COUNT(*)=0 THEN 1 ELSE COUNT(*) FILTER (WHERE reconcile_status IN ('matched','manual'))::numeric/COUNT(*) END::text,
			COALESCE((SELECT upstream_cost FROM costs),0)::text, COALESCE(SUM(user_charge),0)::text
		FROM attempts`, accountID, start.UTC(), end.UTC(), currency).Scan(
		&summary.TotalAttempts, &summary.MatchedAttempts, &summary.PendingAttempts, &summary.ConflictAttempts,
		&coverageText, &upstreamText, &userText)
	if err != nil {
		return reconciliation.Summary{}, fmt.Errorf("read reconciliation summary: %w", err)
	}
	var parseErr error
	if summary.CoverageRatio, parseErr = decimalFromText(coverageText); parseErr != nil {
		return reconciliation.Summary{}, parseErr
	}
	if summary.UpstreamCost, parseErr = decimalFromText(upstreamText); parseErr != nil {
		return reconciliation.Summary{}, parseErr
	}
	if summary.UserCharge, parseErr = decimalFromText(userText); parseErr != nil {
		return reconciliation.Summary{}, parseErr
	}
	summary.PaperProfit = summary.UserCharge.Sub(summary.UpstreamCost)
	summary.Currency = currency
	summary.ObservedAt = time.Now().UTC()
	return summary, nil
}

func (s *Store) ListUpstreamCostExceptions(ctx context.Context, accountID int64, limit int) ([]reconciliation.Exception, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.reason_code, e.details, e.retry_count, e.first_detected_at, e.last_checked_at,
			a.id, a.attempt_id, a.local_request_id, a.account_id, a.adapter_type,
			COALESCE(a.upstream_request_id,''), a.model, a.input_tokens, a.output_tokens,
			a.user_charge::text, a.currency, a.request_status, a.reconcile_status,
			a.completed_at, a.matched_at, a.created_at, a.updated_at
		FROM relay_ops.upstream_reconciliation_exceptions e
		JOIN relay_ops.upstream_cost_attempts a ON a.id=e.attempt_id
		WHERE ($1=0 OR a.account_id=$1) AND e.resolved_at IS NULL
		ORDER BY e.last_checked_at DESC, e.id DESC LIMIT $2`, accountID, limit)
	if err != nil {
		return nil, fmt.Errorf("list upstream cost exceptions: %w", err)
	}
	defer rows.Close()
	items := make([]reconciliation.Exception, 0)
	for rows.Next() {
		var item reconciliation.Exception
		var amount string
		if err := rows.Scan(&item.ID, &item.ReasonCode, &item.Details, &item.RetryCount,
			&item.FirstDetectedAt, &item.LastCheckedAt, &item.Attempt.ID, &item.Attempt.AttemptID,
			&item.Attempt.LocalRequestID, &item.Attempt.AccountID, &item.Attempt.AdapterType,
			&item.Attempt.UpstreamRequestID, &item.Attempt.Model, &item.Attempt.InputTokens,
			&item.Attempt.OutputTokens, &amount, &item.Attempt.Currency, &item.Attempt.RequestStatus,
			&item.Attempt.ReconcileStatus, &item.Attempt.CompletedAt, &item.Attempt.MatchedAt,
			&item.Attempt.CreatedAt, &item.Attempt.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan upstream cost exception: %w", err)
		}
		item.Attempt.UserCharge, err = decimalFromText(amount)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream cost exceptions: %w", err)
	}
	return items, nil
}

func (s *Store) RefreshReconciliation(ctx context.Context, accountID int64, start, end time.Time, currency string) (reconciliation.Summary, error) {
	if _, err := s.MarkOverdueUpstreamCostExceptions(ctx, time.Now().UTC(), 10*time.Minute); err != nil {
		return reconciliation.Summary{}, err
	}
	return s.ReadReconciliationSummary(ctx, accountID, start, end, currency)
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
