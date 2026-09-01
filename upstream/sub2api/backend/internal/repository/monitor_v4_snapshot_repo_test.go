package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func monitorV4StoredWindowFixture() service.MonitorV4StoredWindow {
	return service.MonitorV4StoredWindow{
		Window: service.MonitorV4Window24H, SnapshotID: "7d4b56d2-8223-4f77-8d22-f6a93d818980",
		WindowStart: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC), WindowEnd: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		GeneratedAt: time.Date(2026, 8, 31, 0, 1, 0, 0, time.UTC), ContractVersion: service.MonitorV4ContractVersion,
		Groups: map[int64]service.MonitorV4GroupProjection{7: {SuccessRate: floatPtr(75), RequestCount: 4, SuccessCount: 3, RealRequestCount: 4, RealSuccessCount: 3, CurrentOperational: true}},
	}
}

func floatPtr(v float64) *float64 { return &v }

func TestMonitorV4SnapshotReplaceIsAtomic(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db).(*accountMonitorRepository)
	window := monitorV4StoredWindowFixture()
	window7 := window
	window7.Window = service.MonitorV4Window7D
	window30 := window
	window30.Window = service.MonitorV4Window30D
	window.Groups[8] = service.MonitorV4GroupProjection{RequestCount: 1, SuccessCount: 1, RealRequestCount: 1, RealSuccessCount: 1}
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM account_monitor_v4_snapshots`).WillReturnResult(sqlmock.NewResult(0, 1))
	for _, snapshot := range []service.MonitorV4StoredWindow{window, window7, window30} {
		for _, groupID := range []int64{7, 8} {
			projection := snapshot.Groups[groupID]
			mock.ExpectExec(`INSERT INTO account_monitor_v4_snapshots`).WithArgs(snapshot.Window, groupID, window.SnapshotID, snapshot.GeneratedAt, snapshot.WindowStart, snapshot.WindowEnd, snapshot.ContractVersion, projection.SuccessRate, projection.RequestCount, projection.SuccessCount, projection.RealRequestCount, projection.RealSuccessCount, projection.ProbeFallbackBucketCount, projection.ProbeFallbackRequestCount, projection.MissingProbeTerminalCount, projection.TTFTP95MS, projection.TTFTSampleCount, projection.LatencyP95MS, projection.LatencySampleCount, projection.CacheHitRate, projection.SourceUpdatedAt, projection.CurrentOperational).WillReturnResult(sqlmock.NewResult(0, 1))
		}
	}
	mock.ExpectCommit()
	if err := repo.ReplaceMonitorV4Snapshots(context.Background(), window.SnapshotID, []service.MonitorV4StoredWindow{window, window7, window30}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMonitorV4SnapshotReplaceRollsBackOnInsertError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db).(*accountMonitorRepository)
	window := monitorV4StoredWindowFixture()
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM account_monitor_v4_snapshots`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO account_monitor_v4_snapshots`).WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()
	if err := repo.ReplaceMonitorV4Snapshots(context.Background(), window.SnapshotID, []service.MonitorV4StoredWindow{window}); err == nil {
		t.Fatal("expected insert error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMonitorV4SnapshotLoadValidatesMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db).(*accountMonitorRepository)
	window := monitorV4StoredWindowFixture()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "window", group_id, snapshot_id, generated_at, window_start, window_end, contract_version`)).
		WithArgs(window.Window).WillReturnRows(sqlmock.NewRows([]string{
		"window", "group_id", "snapshot_id", "generated_at", "window_start", "window_end", "contract_version", "success_rate", "request_count", "success_count", "real_request_count", "real_success_count", "probe_fallback_bucket_count", "probe_fallback_request_count", "missing_probe_terminal_count", "ttft_p95_ms", "ttft_sample_count", "latency_p95_ms", "latency_sample_count", "cache_hit_rate", "source_updated_at", "current_operational",
	}).AddRow(window.Window, 7, window.SnapshotID, window.GeneratedAt, window.WindowStart, window.WindowEnd, window.ContractVersion, 75.0, 4, 3, 4, 3, 0, 0, 0, nil, 0, nil, 0, nil, nil, true))
	got, err := repo.LoadLatestMonitorV4Snapshot(context.Background(), window.Window)
	if err != nil {
		t.Fatal(err)
	}
	if got.SnapshotID != window.SnapshotID || got.Groups[7].RequestCount != 4 {
		t.Fatalf("loaded snapshot = %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMonitorV4SnapshotLoadRejectsEmptyOrInconsistentRows(t *testing.T) {
	metadataRows := sqlmock.NewRows([]string{
		"window", "group_id", "snapshot_id", "generated_at", "window_start", "window_end", "contract_version",
		"success_rate", "request_count", "success_count", "real_request_count", "real_success_count",
		"probe_fallback_bucket_count", "probe_fallback_request_count", "missing_probe_terminal_count",
		"ttft_p95_ms", "ttft_sample_count", "latency_p95_ms", "latency_sample_count", "cache_hit_rate", "source_updated_at", "current_operational",
	}).AddRow("24h", 7, "different", time.Now(), time.Now().Add(-time.Hour), time.Now(), "2", nil, 0, 0, 0, 0, 0, 0, 0, nil, 0, nil, 0, nil, nil, false)
	for _, tc := range []struct {
		name string
		rows *sqlmock.Rows
	}{
		{name: "empty", rows: sqlmock.NewRows([]string{"window", "group_id"})},
		{name: "metadata", rows: metadataRows},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			repo := NewAccountMonitorRepository(db).(*accountMonitorRepository)
			mock.ExpectQuery(`SELECT "window", group_id, snapshot_id`).WithArgs(service.MonitorV4Window24H).WillReturnRows(tc.rows)
			if _, err := repo.LoadLatestMonitorV4Snapshot(context.Background(), service.MonitorV4Window24H); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestMonitorV4SnapshotLoadRejectsWindowBoundsGeneratedAtAndContractMismatch(t *testing.T) {
	base := monitorV4StoredWindowFixture()
	columns := []string{"window", "group_id", "snapshot_id", "generated_at", "window_start", "window_end", "contract_version", "success_rate", "request_count", "success_count", "real_request_count", "real_success_count", "probe_fallback_bucket_count", "probe_fallback_request_count", "missing_probe_terminal_count", "ttft_p95_ms", "ttft_sample_count", "latency_p95_ms", "latency_sample_count", "cache_hit_rate", "source_updated_at", "current_operational"}
	for _, tc := range []struct {
		name                  string
		window                string
		generated, start, end time.Time
		contract              string
	}{
		{name: "window", window: "7d", generated: base.GeneratedAt, start: base.WindowStart, end: base.WindowEnd, contract: base.ContractVersion},
		{name: "bounds", window: "24h", generated: base.GeneratedAt, start: base.WindowEnd, end: base.WindowStart, contract: base.ContractVersion},
		{name: "generated_at", window: "24h", generated: base.GeneratedAt.Add(time.Minute), start: base.WindowStart, end: base.WindowEnd, contract: base.ContractVersion},
		{name: "contract_version", window: "24h", generated: base.GeneratedAt, start: base.WindowStart, end: base.WindowEnd, contract: "3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			rows := sqlmock.NewRows(columns).
				AddRow("24h", 7, base.SnapshotID, base.GeneratedAt, base.WindowStart, base.WindowEnd, base.ContractVersion, 75.0, 4, 3, 4, 3, 0, 0, 0, nil, 0, nil, 0, nil, nil, true).
				AddRow(tc.window, 8, base.SnapshotID, tc.generated, tc.start, tc.end, tc.contract, 75.0, 4, 3, 4, 3, 0, 0, 0, nil, 0, nil, 0, nil, nil, true)
			mock.ExpectQuery(`SELECT "window", group_id, snapshot_id`).WithArgs(service.MonitorV4Window24H).WillReturnRows(rows)
			if _, err := NewAccountMonitorRepository(db).(*accountMonitorRepository).LoadLatestMonitorV4Snapshot(context.Background(), service.MonitorV4Window24H); err == nil {
				t.Fatal("expected metadata validation error")
			}
		})
	}
}

func TestMonitorV4SnapshotReplaceRejectsCountInvariantBeforeTransaction(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewAccountMonitorRepository(db).(*accountMonitorRepository)
	window := monitorV4StoredWindowFixture()
	window.Groups[7] = service.MonitorV4GroupProjection{RequestCount: 1, SuccessCount: 2}
	if err := repo.ReplaceMonitorV4Snapshots(context.Background(), window.SnapshotID, []service.MonitorV4StoredWindow{window}); err == nil {
		t.Fatal("expected count invariant error")
	}
}
