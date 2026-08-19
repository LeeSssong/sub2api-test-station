package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type monitorV2Repository struct {
	db *sql.DB
}

func NewMonitorV2Repository(db *sql.DB) service.MonitorV2Repository {
	return &monitorV2Repository{db: db}
}

func (r *monitorV2Repository) GetPerformanceStats(
	ctx context.Context,
	scopes []service.MonitorV2PerformanceScope,
	start, end time.Time,
) (map[int64]service.MonitorV2PerformanceStats, error) {
	out := make(map[int64]service.MonitorV2PerformanceStats, len(scopes))
	if len(scopes) == 0 {
		return out, nil
	}
	if len(scopes) > 100 {
		return nil, fmt.Errorf("monitor v2 performance query supports at most 100 groups")
	}
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return nil, fmt.Errorf("monitor v2 performance query requires a valid time range")
	}
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil monitor v2 repository")
	}

	groupIDs := make([]int64, 0, len(scopes))
	models := make([]string, 0, len(scopes))
	seen := make(map[int64]struct{}, len(scopes))
	for _, scope := range scopes {
		model := strings.TrimSpace(scope.Model)
		if scope.GroupID <= 0 || model == "" {
			continue
		}
		if _, exists := seen[scope.GroupID]; exists {
			return nil, fmt.Errorf("duplicate monitor v2 performance scope for group %d", scope.GroupID)
		}
		seen[scope.GroupID] = struct{}{}
		groupIDs = append(groupIDs, scope.GroupID)
		models = append(models, model)
	}
	if len(groupIDs) == 0 {
		return out, nil
	}

	rows, err := r.db.QueryContext(ctx, `
	WITH scopes AS (
	  SELECT *
	  FROM unnest($3::bigint[], $4::text[]) AS scope(group_id, model)
	), eligible AS (
	  SELECT
	    ul.group_id,
	    ul.first_token_ms,
	    ul.duration_ms,
	    ul.output_tokens
	  FROM usage_logs ul
	  JOIN scopes scope
	    ON scope.group_id = ul.group_id
	   AND scope.model = ul.model
	  WHERE ul.created_at >= $1
	    AND ul.created_at < $2
	    AND ul.actual_cost > 0
	    AND (
	      ul.billing_mode = 'token'
	      OR (
	        (ul.billing_mode IS NULL OR ul.billing_mode = '')
	        AND COALESCE(ul.image_count, 0) = 0
	        AND COALESCE(ul.video_count, 0) = 0
	        AND COALESCE(ul.image_input_tokens, 0) = 0
	        AND COALESCE(ul.image_output_tokens, 0) = 0
	      )
	    )
	    AND ul.duration_ms > 0
	    AND ul.first_token_ms > 0
	    AND ul.output_tokens > 0
	)
	SELECT
	  scope.group_id,
	  COUNT(eligible.group_id)::bigint AS sample_count,
	  percentile_cont(0.50) WITHIN GROUP (ORDER BY eligible.first_token_ms) AS ttft_p50_ms,
	  percentile_cont(0.95) WITHIN GROUP (ORDER BY eligible.first_token_ms) AS ttft_p95_ms,
	  percentile_cont(0.50) WITHIN GROUP (ORDER BY eligible.duration_ms) AS latency_p50_ms,
	  percentile_cont(0.95) WITHIN GROUP (ORDER BY eligible.duration_ms) AS latency_p95_ms,
	  AVG(eligible.output_tokens * 1000.0 / eligible.duration_ms)::float8 AS avg_tps
	FROM scopes scope
	LEFT JOIN eligible ON eligible.group_id = scope.group_id
	GROUP BY scope.group_id
	ORDER BY scope.group_id
	`, start.UTC(), end.UTC(), pq.Array(groupIDs), pq.Array(models))
	if err != nil {
		return nil, fmt.Errorf("query monitor v2 performance stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			groupID                                  int64
			stats                                    service.MonitorV2PerformanceStats
			ttftP50, ttftP95, latencyP50, latencyP95 sql.NullFloat64
			tps                                      sql.NullFloat64
		)
		if err := rows.Scan(
			&groupID,
			&stats.SampleCount,
			&ttftP50,
			&ttftP95,
			&latencyP50,
			&latencyP95,
			&tps,
		); err != nil {
			return nil, fmt.Errorf("scan monitor v2 performance stats: %w", err)
		}
		stats.TTFTP50MS = monitorV2RoundedInt(ttftP50)
		stats.TTFTP95MS = monitorV2RoundedInt(ttftP95)
		stats.LatencyP50MS = monitorV2RoundedInt(latencyP50)
		stats.LatencyP95MS = monitorV2RoundedInt(latencyP95)
		if tps.Valid {
			value := tps.Float64
			stats.TPS = &value
		}
		out[groupID] = stats
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate monitor v2 performance stats: %w", err)
	}
	return out, nil
}

func monitorV2RoundedInt(value sql.NullFloat64) *int {
	if !value.Valid {
		return nil
	}
	rounded := int(math.Round(value.Float64))
	return &rounded
}
