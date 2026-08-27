package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositoryHasAccountUsageInWindowUsesExclusiveEnd(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	from := time.Date(2026, 8, 27, 2, 5, 0, 0, time.UTC)
	until := from.Add(5 * time.Minute)
	mock.ExpectQuery(regexp.QuoteMeta(activeProbeAccountUsageExistsQuery)).
		WithArgs(int64(7), from, until).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	got, err := newUsageLogRepositoryWithSQL(nil, db).HasAccountUsageInWindow(context.Background(), 7, from, until)
	require.NoError(t, err)
	require.True(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryHasGroupUsageInWindowUsesExclusiveEnd(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	from := time.Date(2026, 8, 27, 2, 5, 0, 0, time.UTC)
	until := from.Add(5 * time.Minute)
	mock.ExpectQuery(regexp.QuoteMeta(activeProbeGroupUsageExistsQuery)).
		WithArgs(int64(9), from, until).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	got, err := newUsageLogRepositoryWithSQL(nil, db).HasGroupUsageInWindow(context.Background(), 9, from, until)
	require.NoError(t, err)
	require.False(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}
