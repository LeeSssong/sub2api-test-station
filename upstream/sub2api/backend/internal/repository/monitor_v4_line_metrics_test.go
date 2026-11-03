package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestMonitorV4LineMetricsQueryAndNullableRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	start := time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta(monitorV4LineMetricsSQL)).WithArgs(start, end, sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "requests", "successes", "p50", "samples"}).AddRow(7, 5, 4, 0, 3).AddRow(8, 0, 0, nil, 0))
	repo := &accountMonitorRepository{db: db}
	rows, err := repo.ReadMonitorV4LineMetrics(context.Background(), []int64{7, 8}, start, end)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, 5, rows[0].SLARequestCount)
	require.Equal(t, 0.0, *rows[0].TTFTP50MS)
	require.Nil(t, rows[1].TTFTP50MS)
	require.NoError(t, mock.ExpectationsWereMet())
	for _, fragment := range []string{"PERCENTILE_CONT(0.5)", "first_token_ms >= 0", "LEFT JOIN usage_counts", "LEFT JOIN error_counts", "is_count_tokens = FALSE", opsSLAErrorPredicate} {
		require.Contains(t, monitorV4LineMetricsSQL, fragment)
	}
	require.NotContains(t, monitorV4LineMetricsSQL, "account_monitor_results")
	require.NotContains(t, monitorV4LineMetricsSQL, "AVG(")
}

func TestMonitorV4LineMetricsRepositoryValidationAndErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountMonitorRepository{db: db}
	start := time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	for _, ids := range [][]int64{{0}, {7, 7}, make([]int64, 101)} {
		_, err = repo.ReadMonitorV4LineMetrics(context.Background(), ids, start, end)
		require.Error(t, err)
	}
	_, err = repo.ReadMonitorV4LineMetrics(context.Background(), []int64{7}, start, end.Add(time.Minute))
	require.Error(t, err)
	_, err = repo.ReadMonitorV4LineMetrics(context.Background(), []int64{7}, time.Time{}, end)
	require.Error(t, err)
	rows, err := repo.ReadMonitorV4LineMetrics(context.Background(), nil, start, end)
	require.NoError(t, err)
	require.Empty(t, rows)
	mock.ExpectQuery("WITH groups AS").WillReturnError(errors.New("db unavailable"))
	_, err = repo.ReadMonitorV4LineMetrics(context.Background(), []int64{7}, start, end)
	require.ErrorContains(t, err, "db unavailable")
	mock.ExpectQuery("WITH groups AS").WillReturnRows(sqlmock.NewRows([]string{"id", "requests", "successes", "p50", "samples"}).AddRow(7, -1, 0, nil, 0))
	_, err = repo.ReadMonitorV4LineMetrics(context.Background(), []int64{7}, start, end)
	require.Error(t, err)
	mock.ExpectQuery("WITH groups AS").WillReturnRows(sqlmock.NewRows([]string{"id", "requests", "successes", "p50", "samples"}).AddRow(7, 0, 0, nil, 0).RowError(0, errors.New("row failure")))
	_, err = repo.ReadMonitorV4LineMetrics(context.Background(), []int64{7}, start, end)
	require.ErrorContains(t, err, "row failure")
	require.NoError(t, mock.ExpectationsWereMet())
}
