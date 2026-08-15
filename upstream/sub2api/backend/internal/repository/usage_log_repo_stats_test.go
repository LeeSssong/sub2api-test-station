package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

var _ service.AccountFinancialUsageReader = (*usageLogRepository)(nil)

const accountFinancialUsagePairQueryPattern = `(?s)SELECT.*ul\.group_id.*ul\.account_id.*COUNT\(\*\).*COALESCE\(SUM\(\s*COALESCE\(ul\.input_tokens, 0\)\s*\+ COALESCE\(ul\.output_tokens, 0\)\s*\+ COALESCE\(ul\.cache_creation_tokens, 0\)\s*\+ COALESCE\(ul\.cache_read_tokens, 0\)\s*\), 0\).*COALESCE\(SUM\(COALESCE\(\s*ul\.account_cost,\s*COALESCE\(ul\.account_stats_cost, ul\.total_cost\)\s*\* COALESCE\(ul\.account_rate_multiplier, 1\)\s*\)\), 0\).*COALESCE\(SUM\(COALESCE\(ul\.actual_cost, 0\)\), 0\).*FROM usage_logs ul.*WHERE ul\.created_at >= \$1 AND ul\.created_at < \$2.*GROUP BY ul\.group_id, ul\.account_id`

func TestReadAccountFinancialUsageUsesHalfOpenNativeAggregation(t *testing.T) {
	ctx := context.Background()
	from := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, type, platform, deleted_at FROM accounts ORDER BY id`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "platform", "deleted_at"}).
			AddRow(int64(11), "active-account", "api_key", "openai", nil).
			AddRow(int64(12), "historical-account", "oauth", "anthropic", from))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, deleted_at FROM groups ORDER BY id`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "deleted_at"}).
			AddRow(int64(21), "active-group", nil).
			AddRow(int64(22), "historical-group", from))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(SUM(balance), 0) FROM users WHERE deleted_at IS NULL`)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(12.5))
	mock.ExpectQuery(accountFinancialUsagePairQueryPattern).
		WithArgs(from, to).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "requests", "tokens", "cost", "user_cost"}).
			AddRow(int64(21), int64(11), int64(2), int64(14), 3.25, 5.5).
			AddRow(nil, int64(12), int64(1), int64(4), 1.5, 2.0))
	mock.ExpectCommit()

	snapshot, err := newUsageLogRepositoryWithSQL(nil, db).ReadAccountFinancialUsage(ctx, from, to)
	require.NoError(t, err)
	require.InDelta(t, 12.5, snapshot.UserBalanceCNY, 1e-9)
	require.Equal(t, []service.AccountFinancialUsageAccount{
		{ID: 11, Name: "active-account", Type: "api_key", Platform: "openai", Active: true},
		{ID: 12, Name: "historical-account", Type: "oauth", Platform: "anthropic", Active: false},
	}, snapshot.Accounts)
	require.Equal(t, []service.AccountFinancialUsageGroup{
		{ID: 21, Name: "active-group", Active: true},
		{ID: 22, Name: "historical-group", Active: false},
	}, snapshot.Groups)
	require.Len(t, snapshot.Rows, 2)
	require.Equal(t, int64(21), *snapshot.Rows[0].GroupID)
	require.Equal(t, service.AccountFinancialUsageRow{
		GroupID: int64Ptr(21), GroupName: "active-group", AccountID: 11,
		AccountName: "active-account", AccountType: "api_key", AccountPlatform: "openai",
		Requests: 2, Tokens: 14, Cost: 3.25, UserCost: 5.5,
	}, snapshot.Rows[0])
	require.Nil(t, snapshot.Rows[1].GroupID)
	require.Equal(t, "historical-account", snapshot.Rows[1].AccountName)
	require.Equal(t, int64(1), snapshot.Rows[1].Requests)
	require.Equal(t, int64(4), snapshot.Rows[1].Tokens)
	require.InDelta(t, 1.5, snapshot.Rows[1].Cost, 1e-9)
	require.InDelta(t, 2.0, snapshot.Rows[1].UserCost, 1e-9)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReadAccountFinancialUsagePropagatesQueryScanAndCommitErrors(t *testing.T) {
	from := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	accountsQuery := regexp.QuoteMeta(`SELECT id, name, type, platform, deleted_at FROM accounts ORDER BY id`)
	groupsQuery := regexp.QuoteMeta(`SELECT id, name, deleted_at FROM groups ORDER BY id`)
	balanceQuery := regexp.QuoteMeta(`SELECT COALESCE(SUM(balance), 0) FROM users WHERE deleted_at IS NULL`)

	for _, tt := range []struct {
		name  string
		setup func(sqlmock.Sqlmock)
	}{
		{
			name: "account query",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(accountsQuery).WillReturnError(errors.New("accounts query failed"))
			},
		},
		{
			name: "pair scan",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(accountsQuery).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "platform", "deleted_at"}))
				mock.ExpectQuery(groupsQuery).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "deleted_at"}))
				mock.ExpectQuery(balanceQuery).WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(0))
				mock.ExpectQuery(accountFinancialUsagePairQueryPattern).WithArgs(from, to).
					WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "requests", "tokens", "cost", "user_cost"}).
						AddRow(nil, "not-an-account-id", int64(1), int64(1), 1.0, 1.0))
			},
		},
		{
			name: "commit",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(accountsQuery).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "platform", "deleted_at"}))
				mock.ExpectQuery(groupsQuery).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "deleted_at"}))
				mock.ExpectQuery(balanceQuery).WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(0))
				mock.ExpectQuery(accountFinancialUsagePairQueryPattern).WithArgs(from, to).
					WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "requests", "tokens", "cost", "user_cost"}))
				mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			tt.setup(mock)

			_, err = newUsageLogRepositoryWithSQL(nil, db).ReadAccountFinancialUsage(context.Background(), from, to)
			require.Error(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func int64Ptr(v int64) *int64 { return &v }
