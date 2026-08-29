package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	coreevents "github.com/Wei-Shaw/sub2api/internal/events"
	"github.com/Wei-Shaw/sub2api/internal/integration"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type accountMonitorRepository struct {
	db     *sql.DB
	outbox *coreevents.Outbox
}

func NewAccountMonitorRepository(db *sql.DB) service.AccountMonitorRepository {
	return &accountMonitorRepository{db: db}
}

func NewAccountMonitorRepositoryWithOutbox(db *sql.DB) service.AccountMonitorRepository {
	return &accountMonitorRepository{db: db, outbox: coreevents.NewOutbox(db)}
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
	if r.outbox == nil {
		_, err := r.db.ExecContext(ctx, `
		INSERT INTO account_monitor_results (
			run_id, account_id, model_id, status, error_code, http_status,
			ttft_ms, latency_ms, checked_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9)
		`, runID, result.AccountID, result.ModelID, result.Status, result.ErrorCode,
			result.HTTPStatus, result.TTFTMS, result.LatencyMS, result.CheckedAt.UTC())
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var previousStatus string
	queryErr := tx.QueryRowContext(ctx, `SELECT status FROM account_monitor_results WHERE account_id = $1 ORDER BY checked_at DESC LIMIT 1`, result.AccountID).Scan(&previousStatus)
	if queryErr != nil && !errors.Is(queryErr, sql.ErrNoRows) {
		return queryErr
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO account_monitor_results (run_id, account_id, model_id, status, error_code, http_status, ttft_ms, latency_ms, checked_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9)
	`, runID, result.AccountID, result.ModelID, result.Status, result.ErrorCode, result.HTTPStatus, result.TTFTMS, result.LatencyMS, result.CheckedAt.UTC()); err != nil {
		return err
	}
	if previousStatus != result.Status {
		payload := integration.HealthChanged{AccountID: result.AccountID, Status: result.Status, ErrorCategory: result.ErrorCode, ObservedAt: result.CheckedAt.UTC(), CheckedAt: result.CheckedAt.UTC(), ProbeVersion: result.ModelID}
		event, err := integration.NewHealthChangedEvent("sub2api-core", result.CheckedAt, payload)
		if err != nil {
			return err
		}
		if err := r.outbox.Append(ctx, tx, event); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *accountMonitorRepository) ProjectMonitorV2Groups(
	ctx context.Context,
	scopes []service.MonitorV2GroupAccountScope,
	start, end, freshSince time.Time,
	bucketSize time.Duration,
) (map[int64]service.MonitorV2NativeGroupProjection, error) {
	out := make(map[int64]service.MonitorV2NativeGroupProjection)
	if len(scopes) == 0 {
		return out, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("nil account monitor repository")
	}
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return nil, errors.New("monitor v2 native projection requires a valid time range")
	}
	if freshSince.IsZero() || bucketSize <= 0 || end.Sub(start) < bucketSize {
		return nil, errors.New("monitor v2 native projection requires valid freshness and bucket bounds")
	}
	seen := make(map[service.MonitorV2GroupAccountScope]struct{}, len(scopes))
	groupIDs := make([]int64, 0, len(scopes))
	accountIDs := make([]int64, 0, len(scopes))
	for _, scope := range scopes {
		if scope.GroupID <= 0 || scope.AccountID <= 0 {
			return nil, errors.New("monitor v2 native projection scope ids must be positive")
		}
		if _, exists := seen[scope]; exists {
			return nil, fmt.Errorf("duplicate monitor v2 native projection scope %d/%d", scope.GroupID, scope.AccountID)
		}
		seen[scope] = struct{}{}
		groupIDs = append(groupIDs, scope.GroupID)
		accountIDs = append(accountIDs, scope.AccountID)
	}

	rows, err := r.db.QueryContext(ctx, `
	WITH scopes AS (
		SELECT group_id, account_id
		FROM unnest($4::bigint[], $5::bigint[]) AS scope(group_id, account_id)
	), buckets AS (
		SELECT bucket_start
		FROM generate_series($1::timestamptz, $2::timestamptz - $6::interval, $6::interval) AS series(bucket_start)
	), scoped_results AS (
		SELECT
			s.group_id,
			r.id,
			r.account_id,
			r.status,
			r.ttft_ms,
			r.latency_ms,
			date_bin($6::interval, r.checked_at, $1::timestamptz) AS bucket_start
		FROM scopes s
		JOIN account_monitor_results r ON r.account_id = s.account_id
		WHERE r.checked_at >= $1
		  AND r.checked_at < $2
	), latest AS (
		SELECT DISTINCT ON (r.account_id)
			r.account_id,
			r.status,
			r.checked_at
		FROM account_monitor_results r
		WHERE r.account_id = ANY($5)
		  AND r.checked_at <= $2
		ORDER BY r.account_id, r.checked_at DESC, r.id DESC
	), current_by_group AS (
		SELECT
			s.group_id,
			BOOL_OR(l.status = 'success' AND l.checked_at >= $3) AS has_fresh_success,
			MAX(l.checked_at) AS source_updated_at
		FROM scopes s
		LEFT JOIN latest l ON l.account_id = s.account_id
		GROUP BY s.group_id
	), metrics AS (
		SELECT
			s.group_id,
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY sr.ttft_ms)
				FILTER (WHERE sr.status = 'success' AND sr.ttft_ms IS NOT NULL) AS ttft_p50_ms,
			AVG(sr.latency_ms) FILTER (WHERE sr.status = 'success' AND sr.latency_ms IS NOT NULL) AS average_latency_ms,
			COUNT(sr.ttft_ms) FILTER (WHERE sr.status = 'success' AND sr.ttft_ms IS NOT NULL)::int AS ttft_sample_count,
			COUNT(sr.latency_ms) FILTER (WHERE sr.status = 'success' AND sr.latency_ms IS NOT NULL)::int AS latency_sample_count
		FROM (SELECT DISTINCT group_id FROM scopes) s
		LEFT JOIN scoped_results sr ON sr.group_id = s.group_id
		GROUP BY s.group_id
	), bucket_metrics AS (
		SELECT
			s.group_id,
			b.bucket_start,
			CASE WHEN COUNT(sr.id) FILTER (WHERE sr.status = 'success') > 0
				THEN 'operational' ELSE 'unavailable' END AS bucket_status,
			COUNT(sr.id) > 0 AS bucket_has_result,
			AVG(sr.latency_ms) FILTER (WHERE sr.status = 'success' AND sr.latency_ms IS NOT NULL) AS bucket_latency_ms
		FROM (SELECT DISTINCT group_id FROM scopes) s
		CROSS JOIN buckets b
		LEFT JOIN scoped_results sr
			ON sr.group_id = s.group_id AND sr.bucket_start = b.bucket_start
		GROUP BY s.group_id, b.bucket_start
	), bucket_summary AS (
		SELECT
			group_id,
			COUNT(*) FILTER (WHERE bucket_status = 'operational')::int AS operational_bucket_count,
			COUNT(*)::int AS total_bucket_count
		FROM bucket_metrics
		GROUP BY group_id
	)
	SELECT
		bm.group_id,
		bm.bucket_start,
		bm.bucket_status,
		bm.bucket_has_result,
		bm.bucket_latency_ms,
		bs.operational_bucket_count,
		bs.total_bucket_count,
		m.ttft_p50_ms,
		m.average_latency_ms,
		m.ttft_sample_count,
		m.latency_sample_count,
		CASE WHEN COALESCE(c.has_fresh_success, FALSE) THEN 'operational' ELSE 'unavailable' END AS current_status,
		c.source_updated_at
	FROM bucket_metrics bm
	JOIN bucket_summary bs ON bs.group_id = bm.group_id
	JOIN metrics m ON m.group_id = bm.group_id
	LEFT JOIN current_by_group c ON c.group_id = bm.group_id
	ORDER BY bm.group_id, bm.bucket_start
	`, start.UTC(), end.UTC(), freshSince.UTC(), pq.Array(groupIDs), pq.Array(accountIDs), bucketSize.String())
	if err != nil {
		return nil, fmt.Errorf("query native monitor v2 groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	seenGroups := make(map[int64]struct{})
	for rows.Next() {
		var (
			groupID                                                       int64
			operationalBuckets, totalBuckets, ttftSamples, latencySamples int
			bucketStart                                                   time.Time
			bucketStatus, currentStatus                                   string
			bucketHasResult                                               bool
			latestCheckedAt                                               sql.NullTime
			bucketLatency, ttftP50, averageLatency                        sql.NullFloat64
		)
		if err := rows.Scan(
			&groupID, &bucketStart, &bucketStatus, &bucketHasResult, &bucketLatency,
			&operationalBuckets, &totalBuckets, &ttftP50, &averageLatency,
			&ttftSamples, &latencySamples, &currentStatus, &latestCheckedAt,
		); err != nil {
			return nil, fmt.Errorf("scan native monitor v2 groups: %w", err)
		}
		if _, exists := seenGroups[groupID]; !exists {
			out[groupID] = service.MonitorV2NativeGroupProjection{
				Status:                 currentStatus,
				SourceUpdatedAt:        accountMonitorNullableTime(latestCheckedAt),
				OperationalBucketCount: operationalBuckets,
				TotalBucketCount:       totalBuckets,
				TTFTP50MS:              accountMonitorRoundedFloat(ttftP50),
				AverageLatencyMS:       accountMonitorRoundedFloat(averageLatency),
				TTFTSampleCount:        ttftSamples,
				LatencySampleCount:     latencySamples,
			}
			seenGroups[groupID] = struct{}{}
		}
		point := service.MonitorV2NativeTimelinePoint{
			BucketStart: bucketStart.UTC(),
			Status:      bucketStatus,
			LatencyMS:   accountMonitorRoundedFloat(bucketLatency),
			HasResult:   bucketHasResult,
		}
		projection := out[groupID]
		projection.Timeline = append(projection.Timeline, point)
		out[groupID] = projection
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate native monitor v2 groups: %w", err)
	}
	return out, nil
}

// ProjectMonitorV4Groups projects request-weighted real traffic, using one
// logical active-probe fallback request for a group/bucket when no real event
// is present. Real traffic always owns its bucket.
func (r *accountMonitorRepository) ProjectMonitorV4Groups(
	ctx context.Context,
	scopes []service.MonitorV2GroupAccountScope,
	start, end time.Time,
	bucketSize time.Duration,
) (map[int64]service.MonitorV4GroupProjection, error) {
	out := make(map[int64]service.MonitorV4GroupProjection)
	if len(scopes) == 0 {
		return out, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("nil account monitor repository")
	}
	if start.IsZero() || end.IsZero() || !end.After(start) || bucketSize <= 0 {
		return nil, errors.New("monitor v4 projection requires a valid time range and bucket")
	}
	seen := make(map[service.MonitorV2GroupAccountScope]struct{}, len(scopes))
	groupIDs := make([]int64, 0, len(scopes))
	accountIDs := make([]int64, 0, len(scopes))
	for _, scope := range scopes {
		if scope.GroupID <= 0 || scope.AccountID <= 0 {
			return nil, errors.New("monitor v4 projection scope ids must be positive")
		}
		if _, exists := seen[scope]; exists {
			return nil, fmt.Errorf("duplicate monitor v4 projection scope %d/%d", scope.GroupID, scope.AccountID)
		}
		seen[scope] = struct{}{}
		groupIDs = append(groupIDs, scope.GroupID)
		accountIDs = append(accountIDs, scope.AccountID)
	}

	rows, err := r.db.QueryContext(ctx, `
WITH scopes AS (
  SELECT group_id, account_id
  FROM unnest($4::bigint[], $5::bigint[]) AS scope(group_id, account_id)
), groups AS (
  SELECT DISTINCT group_id FROM scopes
), usage_candidates AS (
  SELECT s.group_id, u.account_id, u.id::bigint AS source_id, u.created_at AS observed_at,
         date_bin($3::interval, u.created_at, $1::timestamptz) AS bucket_start,
         (u.actual_cost > 0) AS successful,
         u.first_token_ms::double precision AS first_token_ms,
         u.duration_ms::double precision AS duration_ms,
         COALESCE(NULLIF(u.logical_request_id, ''), NULLIF(u.request_id, ''), 'usage:' || u.id::text) AS request_key,
         NULLIF(u.logical_request_id, '') AS logical_request_id,
         NULLIF(u.request_id, '') AS request_id,
         1 AS source_priority
  FROM scopes s
  JOIN usage_logs u ON u.account_id = s.account_id AND u.group_id = s.group_id
  WHERE u.created_at >= $1::timestamptz AND u.created_at < $2::timestamptz
    AND u.usage_completeness IS DISTINCT FROM 'unknown'
), error_candidates AS (
  SELECT s.group_id, o.account_id, o.id::bigint AS source_id, o.created_at AS observed_at,
         date_bin($3::interval, o.created_at, $1::timestamptz) AS bucket_start,
         FALSE AS successful,
         NULL::double precision AS first_token_ms,
         NULL::double precision AS duration_ms,
         COALESCE(matched_usage.request_key, NULLIF(o.request_id, ''), NULLIF(o.client_request_id, ''), 'error:' || o.id::text) AS request_key,
         0 AS source_priority
  FROM scopes s
  JOIN ops_error_logs o ON o.account_id = s.account_id AND o.group_id = s.group_id
  LEFT JOIN LATERAL (
    SELECT u.request_key
    FROM usage_candidates u
    WHERE u.group_id = s.group_id AND u.account_id = o.account_id
      AND (
        (NULLIF(o.request_id, '') IS NOT NULL AND (u.request_id = o.request_id OR u.logical_request_id = o.request_id))
        OR (NULLIF(o.client_request_id, '') IS NOT NULL AND (u.request_id = o.client_request_id OR u.logical_request_id = o.client_request_id))
      )
    ORDER BY u.observed_at DESC, u.source_id DESC
    LIMIT 1
  ) matched_usage ON TRUE
  WHERE o.created_at >= $1::timestamptz AND o.created_at < $2::timestamptz
    AND COALESCE(o.is_count_tokens, FALSE) = FALSE
    AND COALESCE(o.status_code, 0) >= 400
), real_candidates AS (
  SELECT group_id, account_id, source_id, observed_at, bucket_start, successful, first_token_ms, duration_ms, request_key, source_priority
  FROM usage_candidates
  UNION ALL
  SELECT group_id, account_id, source_id, observed_at, bucket_start, successful, first_token_ms, duration_ms, request_key, source_priority
  FROM error_candidates
), real_events AS (
  SELECT group_id, account_id, observed_at, bucket_start, successful, first_token_ms, duration_ms, request_key, 'real'::text AS source
  FROM (
    SELECT rc.*, ROW_NUMBER() OVER (
      PARTITION BY rc.group_id, rc.account_id, rc.request_key
      ORDER BY rc.source_priority ASC, rc.observed_at DESC, rc.source_id DESC
    ) AS position
    FROM real_candidates rc
  ) ranked
  WHERE position = 1
), real_buckets AS (
  SELECT group_id, bucket_start
  FROM real_events
  GROUP BY group_id, bucket_start
), probe_rows AS (
  SELECT s.group_id, r.run_id, r.checked_at AS observed_at,
         date_bin($3::interval, r.checked_at, $1::timestamptz) AS bucket_start,
         (r.status = 'success') AS successful,
         r.ttft_ms,
         r.latency_ms
  FROM scopes s
  JOIN account_monitor_results r ON r.account_id = s.account_id
  WHERE r.checked_at >= $1::timestamptz AND r.checked_at < $2::timestamptz
), probe_runs AS (
  SELECT group_id, bucket_start, run_id,
         BOOL_OR(successful) AS successful,
         MIN(ttft_ms) FILTER (WHERE successful AND ttft_ms IS NOT NULL) AS first_token_ms,
         MIN(latency_ms) FILTER (WHERE successful AND latency_ms IS NOT NULL) AS duration_ms,
         MAX(observed_at) AS observed_at
  FROM probe_rows
  GROUP BY group_id, bucket_start, run_id
), probe_buckets AS (
  SELECT group_id, bucket_start,
         BOOL_OR(successful) AS successful,
         MIN(first_token_ms) FILTER (WHERE successful AND first_token_ms IS NOT NULL) AS first_token_ms,
         MIN(duration_ms) FILTER (WHERE successful AND duration_ms IS NOT NULL) AS duration_ms,
         MAX(observed_at) AS observed_at
  FROM probe_runs
  GROUP BY group_id, bucket_start
), selected_events AS (
  SELECT r.group_id, r.bucket_start, r.observed_at, r.successful, r.first_token_ms, r.duration_ms, r.source
  FROM real_events r
  UNION ALL
  SELECT p.group_id, p.bucket_start, p.observed_at, p.successful, p.first_token_ms, p.duration_ms, 'probe'::text AS source
  FROM probe_buckets p
  LEFT JOIN real_buckets rb ON rb.group_id = p.group_id AND rb.bucket_start = p.bucket_start
  WHERE rb.group_id IS NULL
    AND (
      p.bucket_start + $3::interval <= $2::timestamptz
      OR (
        $2::timestamptz >= p.bucket_start + interval '4 minutes'
        AND $2::timestamptz < p.bucket_start + $3::interval
      )
    )
), latest_selected AS (
  SELECT DISTINCT ON (group_id) group_id, successful
  FROM selected_events
  ORDER BY group_id, bucket_start DESC, observed_at DESC
), aggregate AS (
  SELECT g.group_id,
         CASE WHEN COUNT(s.group_id) = 0 THEN NULL
              ELSE COUNT(*) FILTER (WHERE s.successful)::double precision * 100 / COUNT(s.group_id)
         END AS success_rate,
         COUNT(s.group_id)::int AS request_count,
         COUNT(*) FILTER (WHERE s.successful)::int AS success_count,
         COUNT(*) FILTER (WHERE s.source = 'real')::int AS real_request_count,
         COUNT(*) FILTER (WHERE s.source = 'real' AND s.successful)::int AS real_success_count,
         COUNT(*) FILTER (WHERE s.source = 'probe')::int AS probe_fallback_bucket_count,
         COUNT(*) FILTER (WHERE s.source = 'probe')::int AS probe_fallback_request_count,
         PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY s.first_token_ms)
           FILTER (WHERE s.successful AND s.first_token_ms IS NOT NULL) AS ttft_p95_ms,
         COUNT(*) FILTER (WHERE s.successful AND s.first_token_ms IS NOT NULL)::int AS ttft_sample_count,
         PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY s.duration_ms)
           FILTER (WHERE s.successful AND s.duration_ms IS NOT NULL) AS latency_p95_ms,
         COUNT(*) FILTER (WHERE s.successful AND s.duration_ms IS NOT NULL)::int AS latency_sample_count,
         MAX(s.observed_at) AS source_updated_at,
         COALESCE(BOOL_OR(ls.successful), FALSE) AS current_operational
  FROM groups g
  LEFT JOIN selected_events s ON s.group_id = g.group_id
  LEFT JOIN latest_selected ls ON ls.group_id = g.group_id
  GROUP BY g.group_id
)
SELECT group_id, success_rate, request_count, success_count, real_request_count, real_success_count,
       probe_fallback_bucket_count, probe_fallback_request_count, ttft_p95_ms, ttft_sample_count,
       latency_p95_ms, latency_sample_count, source_updated_at, current_operational
FROM aggregate
ORDER BY group_id
`, start.UTC(), end.UTC(), bucketSize.String(), pq.Array(groupIDs), pq.Array(accountIDs))
	if err != nil {
		return nil, fmt.Errorf("query hybrid monitor v4 groups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			groupID, requestCount, successCount, realRequestCount, realSuccessCount          int
			probeFallbackBuckets, probeFallbackRequests, ttftSampleCount, latencySampleCount int
			successRate, ttftP95, latencyP95                                                 sql.NullFloat64
			currentOperational                                                               bool
			sourceUpdatedAt                                                                  sql.NullTime
		)
		if err := rows.Scan(&groupID, &successRate, &requestCount, &successCount, &realRequestCount, &realSuccessCount, &probeFallbackBuckets, &probeFallbackRequests, &ttftP95, &ttftSampleCount, &latencyP95, &latencySampleCount, &sourceUpdatedAt, &currentOperational); err != nil {
			return nil, fmt.Errorf("scan hybrid monitor v4 groups: %w", err)
		}
		var successRatePtr, ttftP95Ptr, latencyP95Ptr *float64
		if successRate.Valid {
			value := successRate.Float64
			successRatePtr = &value
		}
		if ttftP95.Valid {
			value := ttftP95.Float64
			ttftP95Ptr = &value
		}
		if latencyP95.Valid {
			value := latencyP95.Float64
			latencyP95Ptr = &value
		}
		out[int64(groupID)] = service.MonitorV4GroupProjection{
			SuccessRate:               successRatePtr,
			RequestCount:              requestCount,
			SuccessCount:              successCount,
			RealRequestCount:          realRequestCount,
			RealSuccessCount:          realSuccessCount,
			ProbeFallbackBucketCount:  probeFallbackBuckets,
			ProbeFallbackRequestCount: probeFallbackRequests,
			TTFTP95MS:                 ttftP95Ptr,
			TTFTSampleCount:           ttftSampleCount,
			LatencyP95MS:              latencyP95Ptr,
			LatencySampleCount:        latencySampleCount,
			SourceUpdatedAt:           accountMonitorNullableTime(sourceUpdatedAt),
			CurrentOperational:        currentOperational,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hybrid monitor v4 groups: %w", err)
	}
	return out, nil
}

func accountMonitorNullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	checkedAt := value.Time.UTC()
	return &checkedAt
}

func accountMonitorRoundedFloat(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	rounded := math.Round(value.Float64)
	return &rounded
}

func (r *accountMonitorRepository) ListAggregates(
	ctx context.Context,
	accountIDs []int64,
	since, until time.Time,
) (map[int64]service.AccountMonitorAggregate, error) {
	result := make(map[int64]service.AccountMonitorAggregate, len(accountIDs))
	if len(accountIDs) == 0 || !until.After(since) {
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
			COUNT(*) FILTER (WHERE status = 'success')::int,
			COUNT(ttft_ms) FILTER (WHERE status = 'success')::int,
			COUNT(latency_ms) FILTER (WHERE status = 'success')::int,
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY ttft_ms)
				FILTER (WHERE status = 'success' AND ttft_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY ttft_ms)
				FILTER (WHERE status = 'success' AND ttft_ms IS NOT NULL),
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY latency_ms)
				FILTER (WHERE status = 'success' AND latency_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms)
				FILTER (WHERE status = 'success' AND latency_ms IS NOT NULL),
			MAX(checked_at)
		FROM account_monitor_results
		WHERE account_id = ANY($1) AND checked_at >= $2 AND checked_at < $3
		GROUP BY account_id
	`, pq.Array(accountIDs), since.UTC(), until.UTC())
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
			&aggregate.SuccessSampleCount,
			&aggregate.TTFTSampleCount,
			&aggregate.LatencySampleCount,
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

func (r *accountMonitorRepository) ListWindowAggregates(
	ctx context.Context,
	accountIDs []int64,
	since, until time.Time,
) (map[int64]service.AccountMonitorWindowAggregate, error) {
	result := make(map[int64]service.AccountMonitorWindowAggregate, len(accountIDs))
	if len(accountIDs) == 0 || !until.After(since) {
		return result, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		WITH window_usage AS (
			SELECT
				u.account_id,
				u.first_token_ms,
				u.duration_ms,
				u.output_tokens,
				u.total_cost,
				u.created_at,
				EXISTS (
					SELECT 1
					FROM ops_error_logs e
					WHERE e.account_id = u.account_id
						AND e.request_id IS NOT NULL
						AND u.request_id IS NOT NULL
						AND e.request_id = u.request_id
						AND e.created_at >= $2
						AND e.created_at < $3
						AND COALESCE(e.is_count_tokens, FALSE) = FALSE
						AND COALESCE(e.status_code, 0) >= 400
				) AS has_error
			FROM usage_logs u
			WHERE u.account_id = ANY($1)
				AND u.created_at >= $2
				AND u.created_at < $3
		)
		SELECT
			account_id,
			COUNT(*)::bigint,
			COUNT(*) FILTER (WHERE NOT has_error)::bigint,
			COUNT(*) FILTER (WHERE has_error)::bigint,
			COALESCE(SUM(total_cost), 0)::double precision,
			COALESCE(
				COUNT(*) FILTER (WHERE NOT has_error)::double precision / NULLIF(COUNT(*), 0),
				0
			),
			COUNT(first_token_ms)::int,
			COUNT(duration_ms)::int,
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY first_token_ms)
				FILTER (WHERE first_token_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms)
				FILTER (WHERE duration_ms IS NOT NULL),
			COUNT(*) FILTER (WHERE NOT has_error AND output_tokens > 0 AND first_token_ms IS NOT NULL AND duration_ms > first_token_ms)::int,
			COALESCE(SUM(output_tokens) FILTER (WHERE NOT has_error AND output_tokens > 0 AND first_token_ms IS NOT NULL AND duration_ms > first_token_ms), 0)::bigint,
			COALESCE(SUM(duration_ms - first_token_ms) FILTER (WHERE NOT has_error AND output_tokens > 0 AND first_token_ms IS NOT NULL AND duration_ms > first_token_ms), 0)::double precision,
			MAX(created_at)
		FROM window_usage
		GROUP BY account_id
	`, pq.Array(accountIDs), since.UTC(), until.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var accountID int64
		var aggregate service.AccountMonitorWindowAggregate
		var outputTokens int64
		var generationMS float64
		if err := rows.Scan(
			&accountID,
			&aggregate.RequestCount,
			&aggregate.SuccessCount,
			&aggregate.ErrorCount,
			&aggregate.BaseCost,
			&aggregate.SuccessRate,
			&aggregate.TTFTSampleCount,
			&aggregate.LatencySampleCount,
			&aggregate.TTFTP50MS,
			&aggregate.LatencyP95MS,
			&aggregate.OutputRateSampleCount,
			&outputTokens,
			&generationMS,
			&aggregate.LastObservedAt,
		); err != nil {
			return nil, err
		}
		aggregate.OutputRateTokensPerSecond = service.AccountMonitorOutputRateTokensPerSecond(outputTokens, generationMS, aggregate.OutputRateSampleCount)
		result[accountID] = aggregate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *accountMonitorRepository) ListGroupWindowAggregates(
	ctx context.Context,
	groupID int64,
	accountIDs []int64,
	since, until time.Time,
) (map[int64]service.AccountMonitorWindowAggregate, error) {
	result := make(map[int64]service.AccountMonitorWindowAggregate, len(accountIDs))
	if groupID <= 0 || len(accountIDs) == 0 || !until.After(since) {
		return result, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		WITH group_usage AS (
			SELECT
				u.request_id,
				u.account_id,
				u.first_token_ms,
				u.duration_ms,
				u.output_tokens,
				u.total_cost,
				u.created_at
			FROM usage_logs u
			WHERE u.group_id = $1
				AND u.account_id = ANY($2)
				AND u.created_at >= $3
				AND u.created_at < $4
		), group_errors AS (
			SELECT e.request_id, e.account_id, e.created_at
			FROM ops_error_logs e
			WHERE e.group_id = $1
				AND e.account_id = ANY($2)
				AND e.created_at >= $3
				AND e.created_at < $4
				AND COALESCE(e.is_count_tokens, FALSE) = FALSE
				AND COALESCE(e.status_code, 0) >= 400
		), group_requests AS (
			SELECT
				u.account_id,
				u.first_token_ms,
				u.duration_ms,
				u.output_tokens,
				u.total_cost,
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
			SELECT e.account_id, NULL, NULL, NULL, NULL, e.created_at, TRUE
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
			account_id,
			COUNT(*)::bigint,
			COUNT(*) FILTER (WHERE NOT has_error)::bigint,
			COUNT(*) FILTER (WHERE has_error)::bigint,
			COALESCE(SUM(total_cost), 0)::double precision,
			COALESCE(
				COUNT(*) FILTER (WHERE NOT has_error)::double precision / NULLIF(COUNT(*), 0),
				0
			),
			COUNT(first_token_ms)::int,
			COUNT(duration_ms)::int,
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY first_token_ms)
				FILTER (WHERE first_token_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms)
				FILTER (WHERE duration_ms IS NOT NULL),
			COUNT(*) FILTER (WHERE NOT has_error AND output_tokens > 0 AND first_token_ms IS NOT NULL AND duration_ms > first_token_ms)::int,
			COALESCE(SUM(output_tokens) FILTER (WHERE NOT has_error AND output_tokens > 0 AND first_token_ms IS NOT NULL AND duration_ms > first_token_ms), 0)::bigint,
			COALESCE(SUM(duration_ms - first_token_ms) FILTER (WHERE NOT has_error AND output_tokens > 0 AND first_token_ms IS NOT NULL AND duration_ms > first_token_ms), 0)::double precision,
			MAX(created_at)
		FROM group_requests
		GROUP BY account_id
	`, groupID, pq.Array(accountIDs), since.UTC(), until.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var accountID int64
		var aggregate service.AccountMonitorWindowAggregate
		var outputTokens int64
		var generationMS float64
		if err := rows.Scan(
			&accountID,
			&aggregate.RequestCount,
			&aggregate.SuccessCount,
			&aggregate.ErrorCount,
			&aggregate.BaseCost,
			&aggregate.SuccessRate,
			&aggregate.TTFTSampleCount,
			&aggregate.LatencySampleCount,
			&aggregate.TTFTP50MS,
			&aggregate.LatencyP95MS,
			&aggregate.OutputRateSampleCount,
			&outputTokens,
			&generationMS,
			&aggregate.LastObservedAt,
		); err != nil {
			return nil, err
		}
		aggregate.OutputRateTokensPerSecond = service.AccountMonitorOutputRateTokensPerSecond(outputTokens, generationMS, aggregate.OutputRateSampleCount)
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
			COUNT(*) FILTER (WHERE status = 'success')::int,
			COUNT(ttft_ms) FILTER (WHERE status = 'success')::int,
			COUNT(latency_ms) FILTER (WHERE status = 'success')::int,
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY ttft_ms)
				FILTER (WHERE status = 'success' AND ttft_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY ttft_ms)
				FILTER (WHERE status = 'success' AND ttft_ms IS NOT NULL),
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY latency_ms)
				FILTER (WHERE status = 'success' AND latency_ms IS NOT NULL),
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms)
				FILTER (WHERE status = 'success' AND latency_ms IS NOT NULL),
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
		WITH group_usage AS (
			SELECT
				u.request_id,
				u.account_id,
				u.first_token_ms,
				u.duration_ms,
				u.created_at
			FROM usage_logs u
			WHERE u.group_id = $1
				AND u.account_id = ANY($2)
				AND u.created_at >= $3
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
				u.account_id,
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
			SELECT e.account_id, NULL, NULL, e.created_at, TRUE
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
			account_id,
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE NOT has_error)::int,
			COUNT(*) FILTER (WHERE has_error)::int,
			COALESCE(
				COUNT(*) FILTER (WHERE NOT has_error)::double precision /
				NULLIF(COUNT(*), 0),
				0
			),
			COUNT(*)::int,
			COUNT(first_token_ms)::int,
			COUNT(duration_ms)::int,
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
			&aggregate.SuccessSampleCount,
			&aggregate.TTFTSampleCount,
			&aggregate.LatencySampleCount,
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
			COUNT(*)::int,
			COUNT(first_token_ms)::int,
			COUNT(duration_ms)::int,
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
		&aggregate.SuccessSampleCount,
		&aggregate.TTFTSampleCount,
		&aggregate.LatencySampleCount,
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
			account_id, model_id, status, error_code, http_status, ttft_ms, latency_ms, checked_at
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
			&latest.ModelID,
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

func (r *accountMonitorRepository) ListTimelines(
	ctx context.Context,
	accountIDs []int64,
	perAccountLimit int,
) (map[int64][]service.AccountMonitorTimelinePoint, error) {
	result := make(map[int64][]service.AccountMonitorTimelinePoint, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
	}
	if perAccountLimit <= 0 || perAccountLimit > 200 {
		perAccountLimit = service.AccountMonitorTimelineLimit
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT ranked.account_id, ranked.status, ranked.error_code, ranked.http_status,
			ranked.ttft_ms, ranked.latency_ms, ranked.checked_at
		FROM (
			SELECT account_id, status, error_code, http_status, ttft_ms, latency_ms, checked_at,
				ROW_NUMBER() OVER (PARTITION BY account_id ORDER BY checked_at DESC, id DESC) AS position
			FROM account_monitor_results
			WHERE account_id = ANY($1)
		) ranked
		WHERE ranked.position <= $2
		ORDER BY account_id, checked_at ASC
	`, pq.Array(accountIDs), perAccountLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID int64
		var point service.AccountMonitorTimelinePoint
		if err := rows.Scan(&accountID, &point.Status, &point.ErrorCode, &point.HTTPStatus, &point.TTFTMS, &point.LatencyMS, &point.CheckedAt); err != nil {
			return nil, err
		}
		result[accountID] = append(result[accountID], point)
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
			g.status,
			g.platform,
			g.rate_multiplier,
			g.rpm_limit,
			COALESCE(c.account_count, 0),
			COALESCE(c.active_account_count, 0),
			COALESCE(c.rate_limited_account_count, 0),
			(g.status = 'active' AND NOT g.is_exclusive) AS customer_visible,
			g.require_privacy_set,
			g.sort_order AS native_order,
			COALESCE(w.cost_weight, 15),
			COALESCE(w.success_weight, 45),
			COALESCE(w.ttft_weight, 20),
			COALESCE(w.latency_weight, 20),
			COALESCE(w.ttft_target_ms, 1000),
			COALESCE(w.ttft_limit_ms, 5000),
			COALESCE(w.latency_target_ms, 10000),
			COALESCE(w.latency_limit_ms, 60000),
			COALESCE(w.updated_by, 0),
			w.updated_at
		FROM groups g
		LEFT JOIN account_monitor_group_score_weights w ON w.group_id = g.id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (WHERE a.deleted_at IS NULL)::bigint AS account_count,
				COUNT(*) FILTER (WHERE a.deleted_at IS NULL
					AND a.status = 'active' AND a.schedulable = TRUE
					AND (a.expires_at IS NULL OR a.expires_at > NOW() OR a.auto_pause_on_expired = FALSE)
					AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW())
					AND (a.overload_until IS NULL OR a.overload_until <= NOW())
					AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW()))::bigint AS active_account_count,
				COUNT(*) FILTER (WHERE a.deleted_at IS NULL
					AND a.status = 'active' AND a.schedulable = TRUE
					AND (a.expires_at IS NULL OR a.expires_at > NOW() OR a.auto_pause_on_expired = FALSE)
					AND (a.rate_limit_reset_at > NOW() OR a.overload_until > NOW() OR a.temp_unschedulable_until > NOW()))::bigint AS rate_limited_account_count
			FROM account_groups ag
			JOIN accounts a ON a.id = ag.account_id
			WHERE ag.group_id = g.id
		) c ON TRUE
		WHERE g.deleted_at IS NULL
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
			&group.Status,
			&group.Platform,
			&group.RateMultiplier,
			&group.RPMLimit,
			&group.AccountCount,
			&group.ActiveAccountCount,
			&group.RateLimitedAccountCount,
			&group.CustomerVisible,
			&group.RequirePrivacySet,
			&group.NativeOrder,
			&group.ScoreWeights.Cost,
			&group.ScoreWeights.Success,
			&group.ScoreWeights.TTFT,
			&group.ScoreWeights.Latency,
			&group.ScoreWeights.TTFTTargetMS,
			&group.ScoreWeights.TTFTLimitMS,
			&group.ScoreWeights.LatencyTargetMS,
			&group.ScoreWeights.LatencyLimitMS,
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
		SELECT cost_weight, success_weight, ttft_weight, latency_weight,
			ttft_target_ms, ttft_limit_ms, latency_target_ms, latency_limit_ms,
			updated_by, updated_at
		FROM account_monitor_group_score_weights
		WHERE group_id = $1
	`, groupID).Scan(
		&weights.Cost,
		&weights.Success,
		&weights.TTFT,
		&weights.Latency,
		&weights.TTFTTargetMS,
		&weights.TTFTLimitMS,
		&weights.LatencyTargetMS,
		&weights.LatencyLimitMS,
		&weights.UpdatedBy,
		&weights.UpdatedAt,
	)
	if err != nil {
		return service.AccountMonitorScoreWeights{}, err
	}
	return weights, nil
}

func (r *accountMonitorRepository) LoadGlobalScoreWeights(ctx context.Context) (service.AccountMonitorScoreWeights, error) {
	var weights service.AccountMonitorScoreWeights
	err := r.db.QueryRowContext(ctx, `
		SELECT cost_weight, success_weight, ttft_weight, latency_weight, updated_by, updated_at
		FROM account_monitor_global_score_weights
		WHERE singleton = TRUE
	`).Scan(&weights.Cost, &weights.Success, &weights.TTFT, &weights.Latency, &weights.UpdatedBy, &weights.UpdatedAt)
	if err != nil {
		return service.AccountMonitorScoreWeights{}, err
	}
	return weights, nil
}

func (r *accountMonitorRepository) SaveGlobalScoreWeights(ctx context.Context, actorID int64, weights service.AccountMonitorScoreWeights) (service.AccountMonitorScoreWeights, error) {
	if actorID <= 0 {
		return service.AccountMonitorScoreWeights{}, errors.New("invalid actor id")
	}
	if err := validateFourScoreWeights(weights); err != nil {
		return service.AccountMonitorScoreWeights{}, err
	}
	var saved service.AccountMonitorScoreWeights
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO account_monitor_global_score_weights (
			singleton, cost_weight, success_weight, ttft_weight, latency_weight, updated_by, updated_at
		) VALUES (TRUE, $1, $2, $3, $4, $5, NOW())
		ON CONFLICT (singleton) DO UPDATE SET
			cost_weight = EXCLUDED.cost_weight,
			success_weight = EXCLUDED.success_weight,
			ttft_weight = EXCLUDED.ttft_weight,
			latency_weight = EXCLUDED.latency_weight,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
		RETURNING cost_weight, success_weight, ttft_weight, latency_weight, updated_by, updated_at
	`, weights.Cost, weights.Success, weights.TTFT, weights.Latency, actorID).Scan(
		&saved.Cost, &saved.Success, &saved.TTFT, &saved.Latency, &saved.UpdatedBy, &saved.UpdatedAt,
	)
	if err != nil {
		return service.AccountMonitorScoreWeights{}, err
	}
	return saved, nil
}

func (r *accountMonitorRepository) ResetGlobalScoreWeights(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM account_monitor_global_score_weights
		WHERE singleton = TRUE
	`)
	return err
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
			group_id, cost_weight, success_weight, ttft_weight, latency_weight,
			ttft_target_ms, ttft_limit_ms, latency_target_ms, latency_limit_ms,
			updated_by, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (group_id) DO UPDATE SET
			cost_weight = EXCLUDED.cost_weight,
			success_weight = EXCLUDED.success_weight,
			ttft_weight = EXCLUDED.ttft_weight,
			latency_weight = EXCLUDED.latency_weight,
			ttft_target_ms = EXCLUDED.ttft_target_ms,
			ttft_limit_ms = EXCLUDED.ttft_limit_ms,
			latency_target_ms = EXCLUDED.latency_target_ms,
			latency_limit_ms = EXCLUDED.latency_limit_ms,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
	`, groupID, weights.Cost, weights.Success, weights.TTFT, weights.Latency,
		weights.TTFTTargetMS, weights.TTFTLimitMS, weights.LatencyTargetMS, weights.LatencyLimitMS, actorID)
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
	if weights.TTFTTargetMS < 0 || weights.TTFTLimitMS <= weights.TTFTTargetMS {
		return errors.New("ttft target must be non-negative and less than limit")
	}
	if weights.LatencyTargetMS < 0 || weights.LatencyLimitMS <= weights.LatencyTargetMS {
		return errors.New("latency target must be non-negative and less than limit")
	}
	return nil
}

func validateFourScoreWeights(weights service.AccountMonitorScoreWeights) error {
	if weights.Cost < 0 || weights.Cost > 100 ||
		weights.Success < 0 || weights.Success > 100 ||
		weights.TTFT < 0 || weights.TTFT > 100 ||
		weights.Latency < 0 || weights.Latency > 100 {
		return errors.New("score weights must be between 0 and 100")
	}
	if weights.Cost+weights.Success+weights.TTFT+weights.Latency != 100 {
		return errors.New("score weights must sum to 100")
	}
	return nil
}

var _ service.AccountMonitorRepository = (*accountMonitorRepository)(nil)
