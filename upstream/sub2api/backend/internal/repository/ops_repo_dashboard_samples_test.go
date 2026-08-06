package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryQueryUsageLatencyPublishesMetricSampleCounts(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mock.ExpectQuery(`COUNT\(duration_ms\) AS duration_sample_count`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"duration_p50",
			"duration_p90",
			"duration_p95",
			"duration_p99",
			"duration_avg",
			"duration_max",
			"ttft_p50",
			"ttft_p90",
			"ttft_p95",
			"ttft_p99",
			"ttft_avg",
			"ttft_max",
			"duration_sample_count",
			"ttft_sample_count",
		}).AddRow(
			1000.0,
			1200.0,
			1500.0,
			1800.0,
			1100.0,
			int64(2000),
			300.0,
			400.0,
			500.0,
			600.0,
			350.0,
			int64(700),
			int64(4),
			int64(3),
		))

	duration, ttft, durationSampleCount, ttftSampleCount, err := repo.queryUsageLatency(
		context.Background(),
		&service.OpsDashboardFilter{},
		start,
		end,
	)

	require.NoError(t, err)
	require.Equal(t, int64(4), durationSampleCount)
	require.Equal(t, int64(4), duration.SampleCount)
	require.Equal(t, int64(3), ttftSampleCount)
	require.Equal(t, int64(3), ttft.SampleCount)
	require.NoError(t, mock.ExpectationsWereMet())
}
