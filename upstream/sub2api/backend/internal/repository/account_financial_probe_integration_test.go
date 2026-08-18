package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestAccountFinancialProbeReadsExactRowsInFinancialSnapshot(t *testing.T) {
	from := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, name, type, platform, deleted_at FROM accounts").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "platform", "deleted_at"}).AddRow(int64(17), "probe", "api_key", "sub", nil))
	mock.ExpectQuery("SELECT id, name, deleted_at FROM groups").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "deleted_at"}).AddRow(int64(9), "Pro", nil))
	mock.ExpectQuery(accountFinancialUsageMembershipQueryPattern).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "group_id"}).AddRow(int64(17), int64(9)))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(balance\\), 0\\) FROM users").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(7.5))
	mock.ExpectQuery(accountFinancialUsagePairQueryPattern).WithArgs(from, to).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "requests", "tokens", "cost", "user_cost"}))
	mock.ExpectQuery(accountFinancialProbeQueryPattern).WithArgs(from, to).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "probe_requests", "probe_tokens", "probe_cost", "has_incomplete_cost"}).
			AddRow(int64(9), int64(17), int64(2), int64(37), "1234567890.1234567890", false).
			AddRow(nil, int64(17), int64(1), int64(3), nil, true))
	mock.ExpectCommit()

	snapshot, err := newUsageLogRepositoryWithSQL(nil, db).ReadAccountFinancialUsage(context.Background(), from, to)
	require.NoError(t, err)
	require.False(t, snapshot.ProbeDataError)
	require.Nil(t, snapshot.ProbeErrorCode)
	require.Len(t, snapshot.ProbeRows, 2)
	require.Equal(t, "1234567890.1234567890", snapshot.ProbeRows[0].ProbeCost.StringFixed(10))
	require.Nil(t, snapshot.ProbeRows[1].GroupID)
	require.True(t, snapshot.ProbeRows[1].HasIncompleteCost)
	require.NoError(t, mock.ExpectationsWereMet())
}
