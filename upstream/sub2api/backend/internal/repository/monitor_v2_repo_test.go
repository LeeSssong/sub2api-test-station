package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestMonitorV2RepositoryGetCacheStatsGroupsRows(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewMonitorV2Repository(db)
	start := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	mock.ExpectQuery(`SELECT\s+group_id,\s+COUNT\(\*\)::bigint AS request_count,\s+COUNT\(\*\) FILTER \(WHERE cache_read_tokens > 0\)::bigint AS hit_count`).
		WithArgs(start, end, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "request_count", "hit_count"}).
			AddRow(int64(7), int64(20), int64(8)).
			AddRow(int64(9), int64(5), int64(0)))

	got, err := repo.GetCacheStats(context.Background(), []int64{7, 9}, start, end)
	require.NoError(t, err)
	require.Equal(t, int64(20), got[7].RequestCount)
	require.Equal(t, int64(8), got[7].HitCount)
	require.Equal(t, int64(5), got[9].RequestCount)
	require.Equal(t, int64(0), got[9].HitCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMonitorV2RepositoryGetCacheStatsBoundsInputs(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewMonitorV2Repository(db)
	start := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	got, err := repo.GetCacheStats(context.Background(), nil, start, end)
	require.NoError(t, err)
	require.Empty(t, got)

	_, err = repo.GetCacheStats(context.Background(), []int64{1}, time.Time{}, end)
	require.ErrorContains(t, err, "valid time range")

	groupIDs := make([]int64, 101)
	_, err = repo.GetCacheStats(context.Background(), groupIDs, start, end)
	require.ErrorContains(t, err, "at most 100 groups")
	require.NoError(t, mock.ExpectationsWereMet())
}
