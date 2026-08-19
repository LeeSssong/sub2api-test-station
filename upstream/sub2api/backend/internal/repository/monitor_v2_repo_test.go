package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMonitorV2RepositoryGetPerformanceStatsUsesOneEligibleSampleSet(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewMonitorV2Repository(db)
	start := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	mock.ExpectQuery(
		`(?s)WITH scopes AS \(\s+SELECT \*\s+FROM unnest\(\$3::bigint\[\], \$4::text\[\]\)`+
			`.*scope\.model = ul\.model`+
			`.*ul\.actual_cost > 0`+
			`.*ul\.billing_mode = 'token'`+
			`.*ul\.duration_ms > 0`+
			`.*ul\.first_token_ms > 0`+
			`.*ul\.output_tokens > 0`+
			`.*COUNT\(eligible\.group_id\)::bigint AS sample_count`+
			`.*AVG\(eligible\.output_tokens \* 1000\.0 / eligible\.duration_ms\)::float8 AS avg_tps`,
	).
		WithArgs(start, end, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"group_id", "sample_count", "ttft_p50_ms", "ttft_p95_ms", "latency_p50_ms", "latency_p95_ms", "avg_tps",
		}).AddRow(int64(7), int64(20), 420.4, 880.2, 1320.1, 2400.4, 46.5))

	got, err := repo.GetPerformanceStats(context.Background(), []service.MonitorV2PerformanceScope{{GroupID: 7, Model: "gpt-5.6-sol"}}, start, end)

	require.NoError(t, err)
	require.Equal(t, int64(20), got[7].SampleCount)
	require.Equal(t, 420, *got[7].TTFTP50MS)
	require.Equal(t, 880, *got[7].TTFTP95MS)
	require.Equal(t, 1320, *got[7].LatencyP50MS)
	require.Equal(t, 2400, *got[7].LatencyP95MS)
	require.InDelta(t, 46.5, *got[7].TPS, 0.0001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMonitorV2RepositoryGetPerformanceStatsBoundsInputs(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewMonitorV2Repository(db)
	start := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	got, err := repo.GetPerformanceStats(context.Background(), nil, start, end)
	require.NoError(t, err)
	require.Empty(t, got)

	_, err = repo.GetPerformanceStats(context.Background(), []service.MonitorV2PerformanceScope{{GroupID: 1, Model: "gpt"}}, time.Time{}, end)
	require.ErrorContains(t, err, "valid time range")

	scopes := make([]service.MonitorV2PerformanceScope, 101)
	_, err = repo.GetPerformanceStats(context.Background(), scopes, start, end)
	require.ErrorContains(t, err, "at most 100 groups")

	_, err = repo.GetPerformanceStats(context.Background(), []service.MonitorV2PerformanceScope{
		{GroupID: 1, Model: "gpt-a"},
		{GroupID: 1, Model: "gpt-b"},
	}, start, end)
	require.ErrorContains(t, err, "duplicate")
	require.NoError(t, mock.ExpectationsWereMet())
}
