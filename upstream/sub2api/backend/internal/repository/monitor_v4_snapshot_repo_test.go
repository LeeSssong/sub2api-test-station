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
		Groups: map[int64]service.MonitorV4GroupProjection{7: {SuccessRate: floatPtr(75), RequestCount: 4, SuccessCount: 3, RealRequestCount: 2, RealSuccessCount: 1, CurrentOperational: true}},
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
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM account_monitor_v4_snapshots`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO account_monitor_v4_snapshots`).WithArgs(
		window.Window, int64(7), window.SnapshotID, window.GeneratedAt, window.WindowStart, window.WindowEnd,
		window.ContractVersion, float64(75), 4, 3, 2, 1, 0, 0, 0, nil, 0, nil, 0, nil, nil, true,
	).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.ReplaceMonitorV4Snapshots(context.Background(), window.SnapshotID, []service.MonitorV4StoredWindow{window}); err != nil {
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
	mock.ExpectQuery(regexp.QuoteMeta("SELECT window, group_id, snapshot_id, generated_at, window_start, window_end, contract_version")).
		WithArgs(window.Window).WillReturnRows(sqlmock.NewRows([]string{
		"window", "group_id", "snapshot_id", "generated_at", "window_start", "window_end", "contract_version", "success_rate", "request_count", "success_count", "real_request_count", "real_success_count", "probe_fallback_bucket_count", "probe_fallback_request_count", "missing_probe_terminal_count", "ttft_p95_ms", "ttft_sample_count", "latency_p95_ms", "latency_sample_count", "cache_hit_rate", "source_updated_at", "current_operational",
	}).AddRow(window.Window, 7, window.SnapshotID, window.GeneratedAt, window.WindowStart, window.WindowEnd, window.ContractVersion, 75.0, 4, 3, 2, 1, 0, 0, 0, nil, 0, nil, 0, nil, nil, true))
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
			mock.ExpectQuery("SELECT window, group_id, snapshot_id").WithArgs(service.MonitorV4Window24H).WillReturnRows(tc.rows)
			if _, err := repo.LoadLatestMonitorV4Snapshot(context.Background(), service.MonitorV4Window24H); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
