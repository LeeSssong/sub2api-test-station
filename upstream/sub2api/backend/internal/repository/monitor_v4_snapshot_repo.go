package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

func validateMonitorV4StoredWindow(snapshot service.MonitorV4StoredWindow, snapshotID string) error {
	if snapshot.Window != service.MonitorV4Window24H && snapshot.Window != service.MonitorV4Window7D && snapshot.Window != service.MonitorV4Window30D {
		return fmt.Errorf("unsupported monitor v4 window %q", snapshot.Window)
	}
	if snapshot.SnapshotID != snapshotID || snapshotID == "" {
		return errors.New("monitor v4 snapshot id mismatch")
	}
	if _, err := uuid.Parse(snapshotID); err != nil {
		return fmt.Errorf("invalid monitor v4 snapshot id: %w", err)
	}
	if snapshot.WindowStart.IsZero() || snapshot.WindowEnd.IsZero() || !snapshot.WindowStart.Before(snapshot.WindowEnd) || snapshot.GeneratedAt.IsZero() || snapshot.ContractVersion == "" {
		return errors.New("invalid monitor v4 snapshot metadata")
	}
	for groupID, projection := range snapshot.Groups {
		if groupID <= 0 || projection.RequestCount < 0 || projection.SuccessCount < 0 || projection.RealRequestCount < 0 || projection.RealSuccessCount < 0 || projection.ProbeFallbackBucketCount < 0 || projection.ProbeFallbackRequestCount < 0 || projection.MissingProbeTerminalCount < 0 || projection.TTFTSampleCount < 0 || projection.LatencySampleCount < 0 {
			return fmt.Errorf("invalid monitor v4 snapshot counts for group %d", groupID)
		}
	}
	return nil
}

func (r *accountMonitorRepository) ReplaceMonitorV4Snapshots(ctx context.Context, snapshotID string, snapshots []service.MonitorV4StoredWindow) error {
	if snapshotID == "" {
		return errors.New("monitor v4 snapshot id is required")
	}
	for _, snapshot := range snapshots {
		if err := validateMonitorV4StoredWindow(snapshot, snapshotID); err != nil {
			return err
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := func(e error) error { _ = tx.Rollback(); return e }
	if _, err := tx.ExecContext(ctx, `DELETE FROM account_monitor_v4_snapshots`); err != nil {
		return rollback(err)
	}
	for _, snapshot := range snapshots {
		for groupID, projection := range snapshot.Groups {
			if _, err := tx.ExecContext(ctx, `INSERT INTO account_monitor_v4_snapshots (
				window, group_id, snapshot_id, generated_at, window_start, window_end, contract_version,
				success_rate, request_count, success_count, real_request_count, real_success_count,
				probe_fallback_bucket_count, probe_fallback_request_count, missing_probe_terminal_count,
				ttft_p95_ms, ttft_sample_count, latency_p95_ms, latency_sample_count, cache_hit_rate, source_updated_at, current_operational
			) VALUES ($1, $2, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)`,
				snapshot.Window, groupID, snapshotID, snapshot.GeneratedAt.UTC(), snapshot.WindowStart.UTC(), snapshot.WindowEnd.UTC(), snapshot.ContractVersion,
				projection.SuccessRate, projection.RequestCount, projection.SuccessCount, projection.RealRequestCount, projection.RealSuccessCount,
				projection.ProbeFallbackBucketCount, projection.ProbeFallbackRequestCount, projection.MissingProbeTerminalCount,
				projection.TTFTP95MS, projection.TTFTSampleCount, projection.LatencyP95MS, projection.LatencySampleCount, projection.CacheHitRate, projection.SourceUpdatedAt, projection.CurrentOperational); err != nil {
				return rollback(err)
			}
		}
	}
	return tx.Commit()
}

func (r *accountMonitorRepository) LoadLatestMonitorV4Snapshot(ctx context.Context, window service.MonitorV4Window) (service.MonitorV4StoredWindow, error) {
	if window != service.MonitorV4Window24H && window != service.MonitorV4Window7D && window != service.MonitorV4Window30D {
		return service.MonitorV4StoredWindow{}, fmt.Errorf("unsupported monitor v4 window %q", window)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT window, group_id, snapshot_id, generated_at, window_start, window_end, contract_version, success_rate, request_count, success_count, real_request_count, real_success_count, probe_fallback_bucket_count, probe_fallback_request_count, missing_probe_terminal_count, ttft_p95_ms, ttft_sample_count, latency_p95_ms, latency_sample_count, cache_hit_rate, source_updated_at, current_operational FROM account_monitor_v4_snapshots WHERE window = $1 ORDER BY generated_at DESC, group_id`, window)
	if err != nil {
		return service.MonitorV4StoredWindow{}, err
	}
	defer rows.Close()
	var result service.MonitorV4StoredWindow
	result.Groups = make(map[int64]service.MonitorV4GroupProjection)
	for rows.Next() {
		var rowWindow string
		var groupID int64
		var snapshotID, contract string
		var generatedAt, start, end time.Time
		var successRate, ttft, latency, cache sql.NullFloat64
		var source sql.NullTime
		var requestCount, successCount, realRequestCount, realSuccessCount, probeBuckets, probeRequests, missing, ttftSamples, latencySamples int
		var operational bool
		if err := rows.Scan(&rowWindow, &groupID, &snapshotID, &generatedAt, &start, &end, &contract, &successRate, &requestCount, &successCount, &realRequestCount, &realSuccessCount, &probeBuckets, &probeRequests, &missing, &ttft, &ttftSamples, &latency, &latencySamples, &cache, &source, &operational); err != nil {
			return service.MonitorV4StoredWindow{}, err
		}
		if result.SnapshotID == "" {
			result = service.MonitorV4StoredWindow{Window: service.MonitorV4Window(rowWindow), SnapshotID: snapshotID, WindowStart: start, WindowEnd: end, GeneratedAt: generatedAt, ContractVersion: contract, Groups: make(map[int64]service.MonitorV4GroupProjection)}
		} else if result.SnapshotID != snapshotID || !result.WindowStart.Equal(start) || !result.WindowEnd.Equal(end) || !result.GeneratedAt.Equal(generatedAt) || result.ContractVersion != contract || result.Window != service.MonitorV4Window(rowWindow) {
			return service.MonitorV4StoredWindow{}, errors.New("inconsistent monitor v4 snapshot metadata")
		}
		p := service.MonitorV4GroupProjection{RequestCount: requestCount, SuccessCount: successCount, RealRequestCount: realRequestCount, RealSuccessCount: realSuccessCount, ProbeFallbackBucketCount: probeBuckets, ProbeFallbackRequestCount: probeRequests, MissingProbeTerminalCount: missing, TTFTSampleCount: ttftSamples, LatencySampleCount: latencySamples, CurrentOperational: operational}
		if successRate.Valid {
			p.SuccessRate = &successRate.Float64
		}
		if ttft.Valid {
			p.TTFTP95MS = &ttft.Float64
		}
		if latency.Valid {
			p.LatencyP95MS = &latency.Float64
		}
		if cache.Valid {
			p.CacheHitRate = &cache.Float64
		}
		if source.Valid {
			p.SourceUpdatedAt = &source.Time
		}
		result.Groups[groupID] = p
	}
	if err := rows.Err(); err != nil {
		return service.MonitorV4StoredWindow{}, err
	}
	if result.SnapshotID == "" {
		return service.MonitorV4StoredWindow{}, errors.New("monitor v4 snapshot unavailable")
	}
	if err := validateMonitorV4StoredWindow(result, result.SnapshotID); err != nil {
		return service.MonitorV4StoredWindow{}, err
	}
	return result, nil
}

var _ service.MonitorV4SnapshotStore = (*accountMonitorRepository)(nil)
