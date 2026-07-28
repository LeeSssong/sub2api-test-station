package repository

import (
	"context"
	"database/sql"
	"fmt"
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

func (r *monitorV2Repository) GetCacheStats(
	ctx context.Context,
	groupIDs []int64,
	start, end time.Time,
) (map[int64]service.MonitorV2CacheStats, error) {
	out := make(map[int64]service.MonitorV2CacheStats, len(groupIDs))
	if len(groupIDs) == 0 {
		return out, nil
	}
	if len(groupIDs) > 100 {
		return nil, fmt.Errorf("monitor v2 cache query supports at most 100 groups")
	}
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return nil, fmt.Errorf("monitor v2 cache query requires a valid time range")
	}
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil monitor v2 repository")
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT
  group_id,
  COUNT(*)::bigint AS request_count,
  COUNT(*) FILTER (WHERE cache_read_tokens > 0)::bigint AS hit_count
FROM usage_logs
WHERE created_at >= $1
  AND created_at < $2
  AND group_id = ANY($3)
GROUP BY group_id
`, start.UTC(), end.UTC(), pq.Array(groupIDs))
	if err != nil {
		return nil, fmt.Errorf("query monitor v2 cache stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			groupID int64
			stats   service.MonitorV2CacheStats
		)
		if err := rows.Scan(&groupID, &stats.RequestCount, &stats.HitCount); err != nil {
			return nil, fmt.Errorf("scan monitor v2 cache stats: %w", err)
		}
		out[groupID] = stats
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate monitor v2 cache stats: %w", err)
	}
	return out, nil
}
