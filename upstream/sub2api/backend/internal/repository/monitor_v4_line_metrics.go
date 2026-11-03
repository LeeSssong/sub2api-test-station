package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// Raw usage rows are successful requests in the admin SLA contract. Do not
// substitute probe, deduplicated experience, or hourly-percentile projections.
const monitorV4LineMetricsSQL = `WITH groups AS (
 SELECT unnest($3::bigint[]) AS group_id
), usage_counts AS (
 SELECT u.group_id, COUNT(*) AS success_count,
 PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY u.first_token_ms)
   FILTER (WHERE u.first_token_ms >= 0) AS ttft_p50_ms,
 COUNT(*) FILTER (WHERE u.first_token_ms >= 0) AS ttft_sample_count
 FROM usage_logs u JOIN groups g ON g.group_id = u.group_id
 WHERE u.created_at >= $1::timestamptz AND u.created_at < $2::timestamptz
 GROUP BY u.group_id
), error_counts AS (
 SELECT o.group_id, COUNT(*) AS error_count
 FROM ops_error_logs o JOIN groups g ON g.group_id = o.group_id
 WHERE o.created_at >= $1::timestamptz AND o.created_at < $2::timestamptz
   AND o.is_count_tokens = FALSE AND ` + opsSLAErrorPredicate + `
 GROUP BY o.group_id
)
 SELECT g.group_id, COALESCE(u.success_count,0) + COALESCE(e.error_count,0),
 COALESCE(u.success_count,0), u.ttft_p50_ms, COALESCE(u.ttft_sample_count,0)
 FROM groups g LEFT JOIN usage_counts u ON u.group_id = g.group_id
 LEFT JOIN error_counts e ON e.group_id = g.group_id
 ORDER BY g.group_id`

func (r *accountMonitorRepository) ReadMonitorV4LineMetrics(ctx context.Context, ids []int64, start, end time.Time) ([]service.MonitorV4LineMetric, error) {
	if start.IsZero() || end.IsZero() || end.Sub(start) != 24*time.Hour || len(ids) > 100 {
		return nil, fmt.Errorf("invalid line metrics bounds")
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return nil, fmt.Errorf("invalid line metrics group")
		}
		seen[id] = true
	}
	out := make([]service.MonitorV4LineMetric, 0, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("line metrics database unavailable")
	}
	rows, err := r.db.QueryContext(ctx, monitorV4LineMetricsSQL, start.UTC(), end.UTC(), pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var row service.MonitorV4LineMetric
		if err := rows.Scan(&row.ID, &row.SLARequestCount, &row.SLASuccessCount, &row.TTFTP50MS, &row.TTFTSampleCount); err != nil {
			return nil, err
		}
		if err := service.ValidateMonitorV4LineMetric(row); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return out, nil
}
