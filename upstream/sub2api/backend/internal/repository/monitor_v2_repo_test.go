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

	mock.ExpectQuery(`SELECT\s+g\.id AS group_id,\s+LOWER\(g\.platform\) IN \('openai', 'anthropic'\) AS evidence_available,\s+COUNT\(ul\.id\) FILTER \(WHERE LOWER\(g\.platform\) IN \('openai', 'anthropic'\)\)::bigint AS request_count,\s+COUNT\(ul\.id\) FILTER \(\s+WHERE LOWER\(g\.platform\) IN \('openai', 'anthropic'\)\s+AND ul\.cache_read_tokens > 0\s+\)::bigint AS hit_count`).
		WithArgs(start, end, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "evidence_available", "request_count", "hit_count"}).
			AddRow(int64(7), true, int64(20), int64(8)).
			AddRow(int64(9), false, int64(0), int64(0)).
			AddRow(int64(11), true, int64(5), int64(0)))

	got, err := repo.GetCacheStats(context.Background(), []int64{7, 9, 11}, start, end)
	require.NoError(t, err)
	require.Equal(t, int64(20), got[7].RequestCount)
	require.Equal(t, int64(8), got[7].HitCount)
	require.True(t, got[7].EvidenceAvailable)
	require.Equal(t, int64(0), got[9].RequestCount)
	require.Equal(t, int64(0), got[9].HitCount)
	require.False(t, got[9].EvidenceAvailable)
	require.True(t, got[11].EvidenceAvailable)
	require.Equal(t, int64(5), got[11].RequestCount)
	require.Equal(t, int64(0), got[11].HitCount)
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
