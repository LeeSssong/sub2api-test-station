package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type accountMonitorRepository struct {
	db *sql.DB
}

func NewAccountMonitorRepository(db *sql.DB) service.AccountMonitorRepository {
	return &accountMonitorRepository{db: db}
}

func (r *accountMonitorRepository) LoadSettings(ctx context.Context) (service.AccountMonitorSettings, error) {
	var settings service.AccountMonitorSettings
	err := r.db.QueryRowContext(ctx, `
		SELECT interval_seconds, updated_by, updated_at
		FROM account_monitor_settings
		WHERE id = 1
	`).Scan(&settings.IntervalSeconds, &settings.UpdatedBy, &settings.UpdatedAt)
	if err != nil {
		return service.AccountMonitorSettings{}, err
	}
	return settings, nil
}

func (r *accountMonitorRepository) SaveSettings(ctx context.Context, settings service.AccountMonitorSettings) error {
	if settings.IntervalSeconds < service.AccountMonitorMinIntervalSeconds ||
		settings.IntervalSeconds > service.AccountMonitorMaxIntervalSeconds {
		return fmt.Errorf("interval_seconds must be between %d and %d",
			service.AccountMonitorMinIntervalSeconds, service.AccountMonitorMaxIntervalSeconds)
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO account_monitor_settings (id, interval_seconds, updated_by, updated_at)
		VALUES (1, $1, $2, NOW())
		ON CONFLICT (id) DO UPDATE SET
			interval_seconds = EXCLUDED.interval_seconds,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
	`, settings.IntervalSeconds, settings.UpdatedBy)
	return err
}

func (r *accountMonitorRepository) InsertResult(ctx context.Context, result service.AccountMonitorProbeResult, runID string) error {
	if result.AccountID <= 0 || result.ModelID == "" || result.CheckedAt.IsZero() {
		return errors.New("invalid account monitor result")
	}
	if result.Status == "" {
		result.Status = "unknown"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO account_monitor_results (
			run_id, account_id, model_id, status, error_code, http_status,
			ttft_ms, latency_ms, checked_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9)
	`, runID, result.AccountID, result.ModelID, result.Status, result.ErrorCode,
		result.HTTPStatus, result.TTFTMS, result.LatencyMS, result.CheckedAt.UTC())
	return err
}

func (r *accountMonitorRepository) ListAggregates(
	ctx context.Context,
	accountIDs []int64,
	since time.Time,
) (map[int64]service.AccountMonitorAggregate, error) {
	result := make(map[int64]service.AccountMonitorAggregate, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT
			account_id,
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE status = 'success')::int,
			COUNT(*) FILTER (WHERE status <> 'success')::int,
			COALESCE(
				COUNT(*) FILTER (WHERE status = 'success')::double precision /
				NULLIF(COUNT(*), 0),
				0
			),
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY ttft_ms)
				FILTER (WHERE ttft_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY ttft_ms)
				FILTER (WHERE ttft_ms IS NOT NULL),
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY latency_ms)
				FILTER (WHERE latency_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms)
				FILTER (WHERE latency_ms IS NOT NULL),
			MAX(checked_at)
		FROM account_monitor_results
		WHERE account_id = ANY($1) AND checked_at >= $2
		GROUP BY account_id
	`, pq.Array(accountIDs), since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var accountID int64
		var aggregate service.AccountMonitorAggregate
		if err := rows.Scan(
			&accountID,
			&aggregate.SampleCount,
			&aggregate.SuccessCount,
			&aggregate.ErrorCount,
			&aggregate.SuccessRate,
			&aggregate.TTFTP50MS,
			&aggregate.TTFTP95MS,
			&aggregate.LatencyP50MS,
			&aggregate.LatencyP95MS,
			&aggregate.LastCheckedAt,
		); err != nil {
			return nil, err
		}
		result[accountID] = aggregate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *accountMonitorRepository) LoadAggregate(
	ctx context.Context,
	accountIDs []int64,
	since time.Time,
) (service.AccountMonitorAggregate, error) {
	if len(accountIDs) == 0 {
		return service.AccountMonitorAggregate{}, nil
	}
	return r.loadAggregate(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE status = 'success')::int,
			COUNT(*) FILTER (WHERE status <> 'success')::int,
			COALESCE(
				COUNT(*) FILTER (WHERE status = 'success')::double precision /
				NULLIF(COUNT(*), 0),
				0
			),
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY ttft_ms)
				FILTER (WHERE ttft_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY ttft_ms)
				FILTER (WHERE ttft_ms IS NOT NULL),
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY latency_ms)
				FILTER (WHERE latency_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms)
				FILTER (WHERE latency_ms IS NOT NULL),
			MAX(checked_at)
		FROM account_monitor_results
		WHERE account_id = ANY($1) AND checked_at >= $2
	`, pq.Array(accountIDs), since.UTC())
}

func (r *accountMonitorRepository) ListGroupAggregates(
	ctx context.Context,
	groupID int64,
	accountIDs []int64,
	since time.Time,
) (map[int64]service.AccountMonitorAggregate, error) {
	result := make(map[int64]service.AccountMonitorAggregate, len(accountIDs))
	if groupID <= 0 || len(accountIDs) == 0 {
		return result, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH group_requests AS (
			SELECT
				u.account_id,
				u.first_token_ms,
				u.duration_ms,
				u.created_at,
				EXISTS (
					SELECT 1
					FROM ops_error_logs e
					WHERE e.request_id = u.request_id
						AND e.account_id = u.account_id
						AND e.group_id = u.group_id
				) AS has_error
			FROM usage_logs u
			WHERE u.group_id = $1
				AND u.account_id = ANY($2)
				AND u.created_at >= $3
		)
		SELECT
			account_id,
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE NOT has_error)::int,
			COUNT(*) FILTER (WHERE has_error)::int,
			COALESCE(
				COUNT(*) FILTER (WHERE NOT has_error)::double precision /
				NULLIF(COUNT(*), 0),
				0
			),
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY first_token_ms)
				FILTER (WHERE first_token_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY first_token_ms)
				FILTER (WHERE first_token_ms IS NOT NULL),
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY duration_ms)
				FILTER (WHERE duration_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms)
				FILTER (WHERE duration_ms IS NOT NULL),
			MAX(created_at)
		FROM group_requests
		GROUP BY account_id
	`, groupID, pq.Array(accountIDs), since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID int64
		var aggregate service.AccountMonitorAggregate
		if err := rows.Scan(
			&accountID,
			&aggregate.SampleCount,
			&aggregate.SuccessCount,
			&aggregate.ErrorCount,
			&aggregate.SuccessRate,
			&aggregate.TTFTP50MS,
			&aggregate.TTFTP95MS,
			&aggregate.LatencyP50MS,
			&aggregate.LatencyP95MS,
			&aggregate.LastCheckedAt,
		); err != nil {
			return nil, err
		}
		result[accountID] = aggregate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *accountMonitorRepository) LoadGroupAggregate(
	ctx context.Context,
	groupID int64,
	accountIDs []int64,
	since time.Time,
) (service.AccountMonitorAggregate, error) {
	if groupID <= 0 || len(accountIDs) == 0 {
		return service.AccountMonitorAggregate{}, nil
	}
	return r.loadAggregate(ctx, `
		WITH group_usage AS (
			SELECT
				u.request_id,
				u.account_id,
				u.first_token_ms,
				u.duration_ms,
				u.created_at
			FROM usage_logs u
			WHERE u.group_id = $1 AND u.account_id = ANY($2) AND u.created_at >= $3
		), group_errors AS (
			SELECT e.request_id, e.account_id, e.created_at
			FROM ops_error_logs e
			WHERE e.group_id = $1
				AND e.account_id = ANY($2)
				AND e.created_at >= $3
				AND COALESCE(e.is_count_tokens, FALSE) = FALSE
				AND COALESCE(e.status_code, 0) >= 400
		), group_requests AS (
			SELECT
				u.first_token_ms,
				u.duration_ms,
				u.created_at,
				EXISTS (
					SELECT 1
					FROM group_errors e
					WHERE e.request_id IS NOT NULL
						AND u.request_id IS NOT NULL
						AND e.request_id = u.request_id
						AND e.account_id = u.account_id
				) AS has_error
			FROM group_usage u
			UNION ALL
			SELECT NULL, NULL, e.created_at, TRUE
			FROM group_errors e
			WHERE NOT EXISTS (
				SELECT 1
				FROM group_usage u
				WHERE u.request_id IS NOT NULL
					AND e.request_id IS NOT NULL
					AND u.request_id = e.request_id
					AND u.account_id = e.account_id
			)
		)
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE NOT has_error)::int,
			COUNT(*) FILTER (WHERE has_error)::int,
			COALESCE(
				COUNT(*) FILTER (WHERE NOT has_error)::double precision /
				NULLIF(COUNT(*), 0),
				0
			),
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY first_token_ms)
				FILTER (WHERE first_token_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY first_token_ms)
				FILTER (WHERE first_token_ms IS NOT NULL),
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY duration_ms)
				FILTER (WHERE duration_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms)
				FILTER (WHERE duration_ms IS NOT NULL),
			MAX(created_at)
		FROM group_requests
		`, groupID, pq.Array(accountIDs), since.UTC())
}

func (r *accountMonitorRepository) loadAggregate(
	ctx context.Context,
	query string,
	args ...any,
) (service.AccountMonitorAggregate, error) {
	var aggregate service.AccountMonitorAggregate
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&aggregate.SampleCount,
		&aggregate.SuccessCount,
		&aggregate.ErrorCount,
		&aggregate.SuccessRate,
		&aggregate.TTFTP50MS,
		&aggregate.TTFTP95MS,
		&aggregate.LatencyP50MS,
		&aggregate.LatencyP95MS,
		&aggregate.LastCheckedAt,
	)
	if err != nil {
		return service.AccountMonitorAggregate{}, err
	}
	return aggregate, nil
}

func (r *accountMonitorRepository) ListLatest(
	ctx context.Context,
	accountIDs []int64,
) (map[int64]service.AccountMonitorLatest, error) {
	result := make(map[int64]service.AccountMonitorLatest, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT ON (account_id)
			account_id, status, error_code, http_status, ttft_ms, latency_ms, checked_at
		FROM account_monitor_results
		WHERE account_id = ANY($1)
		ORDER BY account_id, checked_at DESC, id DESC
	`, pq.Array(accountIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var accountID int64
		var latest service.AccountMonitorLatest
		if err := rows.Scan(
			&accountID,
			&latest.Status,
			&latest.ErrorCode,
			&latest.HTTPStatus,
			&latest.TTFTMS,
			&latest.LatencyMS,
			&latest.CheckedAt,
		); err != nil {
			return nil, err
		}
		result[accountID] = latest
	}
	return result, rows.Err()
}

func (r *accountMonitorRepository) ListHistory(
	ctx context.Context,
	accountID int64,
	limit int,
) ([]service.AccountMonitorProbeResult, error) {
	if accountID <= 0 {
		return nil, errors.New("invalid account id")
	}
	if limit <= 0 || limit > 200 {
		limit = service.AccountMonitorDefaultHistoryLimit
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT account_id, model_id, status, error_code, http_status,
			ttft_ms, latency_ms, checked_at
		FROM account_monitor_results
		WHERE account_id = $1
		ORDER BY checked_at DESC, id DESC
		LIMIT $2
	`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]service.AccountMonitorProbeResult, 0, limit)
	for rows.Next() {
		var result service.AccountMonitorProbeResult
		if err := rows.Scan(
			&result.AccountID,
			&result.ModelID,
			&result.Status,
			&result.ErrorCode,
			&result.HTTPStatus,
			&result.TTFTMS,
			&result.LatencyMS,
			&result.CheckedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func (r *accountMonitorRepository) DeleteBefore(ctx context.Context, before time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM account_monitor_results
		WHERE checked_at < $1
	`, before.UTC())
	return err
}

func (r *accountMonitorRepository) ListGroups(ctx context.Context) ([]service.AccountMonitorGroup, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			g.id,
			g.name,
			g.rate_multiplier,
			(g.status = 'active' AND NOT g.is_exclusive) AS customer_visible,
			g.sort_order AS native_order,
			COALESCE(w.cost_weight, 15),
			COALESCE(w.success_weight, 45),
			COALESCE(w.ttft_weight, 20),
			COALESCE(w.latency_weight, 20),
			COALESCE(w.updated_by, 0),
			w.updated_at
		FROM groups g
		LEFT JOIN account_monitor_group_score_weights w ON w.group_id = g.id
		ORDER BY g.sort_order ASC, g.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]service.AccountMonitorGroup, 0)
	for rows.Next() {
		var group service.AccountMonitorGroup
		var updatedAt sql.NullTime
		if err := rows.Scan(
			&group.ID,
			&group.Name,
			&group.RateMultiplier,
			&group.CustomerVisible,
			&group.NativeOrder,
			&group.ScoreWeights.Cost,
			&group.ScoreWeights.Success,
			&group.ScoreWeights.TTFT,
			&group.ScoreWeights.Latency,
			&group.ScoreWeights.UpdatedBy,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		if updatedAt.Valid {
			group.ScoreWeights.UpdatedAt = updatedAt.Time
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (r *accountMonitorRepository) LoadGroupScoreWeights(ctx context.Context, groupID int64) (service.AccountMonitorScoreWeights, error) {
	if groupID <= 0 {
		return service.AccountMonitorScoreWeights{}, errors.New("invalid group id")
	}
	var weights service.AccountMonitorScoreWeights
	err := r.db.QueryRowContext(ctx, `
		SELECT cost_weight, success_weight, ttft_weight, latency_weight, updated_by, updated_at
		FROM account_monitor_group_score_weights
		WHERE group_id = $1
	`, groupID).Scan(
		&weights.Cost,
		&weights.Success,
		&weights.TTFT,
		&weights.Latency,
		&weights.UpdatedBy,
		&weights.UpdatedAt,
	)
	if err != nil {
		return service.AccountMonitorScoreWeights{}, err
	}
	return weights, nil
}

func (r *accountMonitorRepository) SaveGroupScoreWeights(
	ctx context.Context,
	groupID, actorID int64,
	weights service.AccountMonitorScoreWeights,
) error {
	if groupID <= 0 || actorID <= 0 {
		return errors.New("invalid group or actor id")
	}
	if err := validateGroupScoreWeights(weights); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO account_monitor_group_score_weights (
			group_id, cost_weight, success_weight, ttft_weight, latency_weight, updated_by, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (group_id) DO UPDATE SET
			cost_weight = EXCLUDED.cost_weight,
			success_weight = EXCLUDED.success_weight,
			ttft_weight = EXCLUDED.ttft_weight,
			latency_weight = EXCLUDED.latency_weight,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
	`, groupID, weights.Cost, weights.Success, weights.TTFT, weights.Latency, actorID)
	return err
}

func (r *accountMonitorRepository) ResetGroupScoreWeights(ctx context.Context, groupID int64) error {
	if groupID <= 0 {
		return errors.New("invalid group id")
	}
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM account_monitor_group_score_weights
		WHERE group_id = $1
	`, groupID)
	return err
}

func validateGroupScoreWeights(weights service.AccountMonitorScoreWeights) error {
	if weights.Cost < 0 || weights.Success < 0 || weights.TTFT < 0 || weights.Latency < 0 {
		return errors.New("score weights must be non-negative")
	}
	if weights.Cost+weights.Success+weights.TTFT+weights.Latency != 100 {
		return errors.New("score weights must sum to 100")
	}
	return nil
}

var _ service.AccountMonitorRepository = (*accountMonitorRepository)(nil)
