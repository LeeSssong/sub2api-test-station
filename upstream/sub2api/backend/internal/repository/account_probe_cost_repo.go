package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type accountProbeCostRepository struct {
	db *sql.DB
}

func NewAccountProbeCostRepository(db *sql.DB) service.AccountProbeCostRepository {
	return &accountProbeCostRepository{db: db}
}

func (r *accountProbeCostRepository) Append(ctx context.Context, log service.AccountProbeCostLog) error {
	if r == nil || r.db == nil {
		return errors.New("account probe cost repository unavailable")
	}

	var insertedRunID string
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO account_probe_cost_logs (
			account_id, group_id, probe_run_id, probe_kind, model,
			input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
			account_cost, usage_completeness, probe_outcome, error_code, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (probe_run_id) DO NOTHING
		RETURNING probe_run_id
	`, log.AccountID, log.GroupID, log.ProbeRunID, log.ProbeKind, log.Model,
		log.InputTokens, log.OutputTokens, log.CacheCreationTokens, log.CacheReadTokens,
		log.AccountCost, log.UsageCompleteness, log.ProbeOutcome, log.ErrorCode, log.CreatedAt).Scan(&insertedRunID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var stored service.AccountProbeCostLog
	var storedGroupID sql.NullInt64
	var storedCost sql.NullFloat64
	var storedErrorCode sql.NullString
	var probeKind, completeness, outcome string
	err = r.db.QueryRowContext(ctx, `
		SELECT probe_run_id, account_id, group_id, probe_kind, model,
		       input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
		       account_cost, usage_completeness, probe_outcome, error_code, created_at
		FROM account_probe_cost_logs
		WHERE probe_run_id = $1
	`, log.ProbeRunID).Scan(
		&stored.ProbeRunID, &stored.AccountID, &storedGroupID, &probeKind, &stored.Model,
		&stored.InputTokens, &stored.OutputTokens, &stored.CacheCreationTokens, &stored.CacheReadTokens,
		&storedCost, &completeness, &outcome, &storedErrorCode, &stored.CreatedAt)
	if err != nil {
		return err
	}
	if storedGroupID.Valid {
		stored.GroupID = &storedGroupID.Int64
	}
	if storedCost.Valid {
		stored.AccountCost = &storedCost.Float64
	}
	if storedErrorCode.Valid {
		stored.ErrorCode = &storedErrorCode.String
	}
	stored.ProbeKind = service.ProbeKind(probeKind)
	stored.UsageCompleteness = service.ProbeUsageCompleteness(completeness)
	stored.ProbeOutcome = service.ProbeOutcome(outcome)
	if !sameAccountProbeCostLog(log, stored) {
		return fmt.Errorf("%w: probe_run_id=%s", service.ErrAccountProbeCostConflict, log.ProbeRunID)
	}
	return nil
}

func (r *accountProbeCostRepository) ReadWindow(ctx context.Context, from, to time.Time) ([]service.AccountProbeCostAggregate, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("account probe cost repository unavailable")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			group_id,
			account_id,
			COUNT(*)::BIGINT AS probe_requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0)::BIGINT AS probe_tokens,
			CASE WHEN BOOL_AND(account_cost IS NOT NULL AND usage_completeness = 'complete')
			     THEN SUM(account_cost) ELSE NULL END AS probe_cost,
			BOOL_OR(account_cost IS NULL OR usage_completeness <> 'complete') AS has_incomplete_cost
		FROM account_probe_cost_logs
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY group_id, account_id
		ORDER BY group_id NULLS FIRST, account_id
	`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []service.AccountProbeCostAggregate
	for rows.Next() {
		var row service.AccountProbeCostAggregate
		var groupID sql.NullInt64
		var cost sql.NullFloat64
		if err := rows.Scan(&groupID, &row.AccountID, &row.ProbeRequests, &row.ProbeTokens, &cost, &row.HasIncompleteCost); err != nil {
			return nil, err
		}
		if groupID.Valid {
			row.GroupID = &groupID.Int64
		}
		if cost.Valid {
			row.ProbeCost = &cost.Float64
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, rows.Close()
}

func sameAccountProbeCostLog(a, b service.AccountProbeCostLog) bool {
	return a.ProbeRunID == b.ProbeRunID && a.AccountID == b.AccountID &&
		sameInt64Ptr(a.GroupID, b.GroupID) && a.ProbeKind == b.ProbeKind && a.Model == b.Model &&
		a.InputTokens == b.InputTokens && a.OutputTokens == b.OutputTokens &&
		a.CacheCreationTokens == b.CacheCreationTokens && a.CacheReadTokens == b.CacheReadTokens &&
		sameFloat64Ptr(a.AccountCost, b.AccountCost) && a.UsageCompleteness == b.UsageCompleteness &&
		a.ProbeOutcome == b.ProbeOutcome && sameStringPtr(a.ErrorCode, b.ErrorCode) && a.CreatedAt.Equal(b.CreatedAt)
}

func sameInt64Ptr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameFloat64Ptr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
