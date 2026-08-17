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

const accountFinancialUsagePairQueryPattern = `(?s)SELECT.*ul\.group_id.*ul\.account_id.*COUNT\(\*\).*COALESCE\(SUM\(\s*COALESCE\(ul\.input_tokens, 0\)\s*\+ COALESCE\(ul\.output_tokens, 0\)\s*\+ COALESCE\(ul\.cache_creation_tokens, 0\)\s*\+ COALESCE\(ul\.cache_read_tokens, 0\)\s*\), 0\).*COALESCE\(SUM\(CASE WHEN COALESCE\(u\.role, ''\) = 'admin'.*COALESCE\(SUM\(CASE WHEN COALESCE\(u\.role, ''\) <> 'admin'.*FROM usage_logs ul\s+LEFT JOIN users u ON u\.id = ul\.user_id.*GROUP BY ul\.group_id, ul\.account_id`
const accountFinancialProbeQueryPattern = `(?s)SELECT\s+group_id,\s+account_id,\s+COUNT\(\*\)::BIGINT.*FROM account_probe_cost_logs.*WHERE created_at >= \$1 AND created_at < \$2.*GROUP BY group_id, account_id`

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
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "requests", "tokens", "operational_cost", "business_cost", "business_revenue"}).
			AddRow(int64(21), int64(11), int64(2), int64(14), 1.25, 2.0, 5.5).
			AddRow(nil, int64(12), int64(1), int64(4), 0.0, 1.5, 2.0))
	mock.ExpectQuery(accountFinancialProbeQueryPattern).WithArgs(from, to).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "probe_requests", "probe_tokens", "probe_cost", "has_incomplete_cost"}))
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
		Requests: 2, Tokens: 14, Cost: 3.25, UserCost: 5.5, OperationalCost: 1.25, BusinessCost: 2.0, BusinessRevenue: 5.5,
	}, snapshot.Rows[0])
	require.Nil(t, snapshot.Rows[1].GroupID)
	require.Equal(t, "historical-account", snapshot.Rows[1].AccountName)
	require.Equal(t, int64(1), snapshot.Rows[1].Requests)
	require.Equal(t, int64(4), snapshot.Rows[1].Tokens)
	require.InDelta(t, 1.5, snapshot.Rows[1].Cost, 1e-9)
	require.InDelta(t, 2.0, snapshot.Rows[1].UserCost, 1e-9)
	require.InDelta(t, 0.0, snapshot.Rows[1].OperationalCost, 1e-9)
	require.InDelta(t, 1.5, snapshot.Rows[1].BusinessCost, 1e-9)
	require.InDelta(t, 2.0, snapshot.Rows[1].BusinessRevenue, 1e-9)
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
					WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "requests", "tokens", "operational_cost", "business_cost", "business_revenue"}).
						AddRow(nil, "not-an-account-id", int64(1), int64(1), 0.0, 1.0, 1.0))
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
					WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "requests", "tokens", "operational_cost", "business_cost", "business_revenue"}))
				mock.ExpectQuery(accountFinancialProbeQueryPattern).WithArgs(from, to).
					WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "probe_requests", "probe_tokens", "probe_cost", "has_incomplete_cost"}))
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

func TestReadAccountFinancialUsageProbeFailurePreservesNativeSnapshot(t *testing.T) {
	from := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(accountFinancialUsageAccountsQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "platform", "deleted_at"}).AddRow(int64(11), "native", "api_key", "sub", nil))
	mock.ExpectQuery(regexp.QuoteMeta(accountFinancialUsageGroupsQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "deleted_at"}).AddRow(int64(21), "Pro", nil))
	mock.ExpectQuery(regexp.QuoteMeta(accountFinancialUsageBalanceQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(88.5))
	mock.ExpectQuery(accountFinancialUsagePairQueryPattern).WithArgs(from, to).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "requests", "tokens", "operational_cost", "business_cost", "business_revenue"}).
			AddRow(int64(21), int64(11), int64(2), int64(14), 1.25, 2.0, 5.5))
	mock.ExpectQuery(accountFinancialProbeQueryPattern).WithArgs(from, to).WillReturnError(errors.New("probe aggregate unavailable"))
	mock.ExpectRollback()

	snapshot, err := newUsageLogRepositoryWithSQL(nil, db).ReadAccountFinancialUsage(context.Background(), from, to)
	require.NoError(t, err)
	require.True(t, snapshot.ProbeDataError)
	require.NotNil(t, snapshot.ProbeErrorCode)
	require.Equal(t, "probe_aggregate_unavailable", *snapshot.ProbeErrorCode)
	require.Empty(t, snapshot.ProbeRows)
	require.Equal(t, 88.5, snapshot.UserBalanceCNY)
	require.Equal(t, []service.AccountFinancialUsageRow{{
		GroupID: int64Ptr(21), GroupName: "Pro", AccountID: 11, AccountName: "native", AccountType: "api_key", AccountPlatform: "sub",
		Requests: 2, Tokens: 14, Cost: 3.25, UserCost: 5.5, OperationalCost: 1.25, BusinessCost: 2.0, BusinessRevenue: 5.5,
	}}, snapshot.Rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReadAccountFinancialUsageProbeScanFailureReturnsStableProbeError(t *testing.T) {
	from := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(accountFinancialUsageAccountsQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "platform", "deleted_at"}))
	mock.ExpectQuery(regexp.QuoteMeta(accountFinancialUsageGroupsQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "deleted_at"}))
	mock.ExpectQuery(regexp.QuoteMeta(accountFinancialUsageBalanceQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(12.5))
	mock.ExpectQuery(accountFinancialUsagePairQueryPattern).WithArgs(from, to).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "requests", "tokens", "operational_cost", "business_cost", "business_revenue"}).
			AddRow(nil, int64(7), int64(1), int64(2), 0.0, 3.0, 5.0))
	mock.ExpectQuery(accountFinancialProbeQueryPattern).WithArgs(from, to).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "probe_requests", "probe_tokens", "probe_cost", "has_incomplete_cost"}).
			AddRow(nil, "invalid-account", int64(1), int64(2), "0.1", false))
	mock.ExpectRollback()

	snapshot, err := newUsageLogRepositoryWithSQL(nil, db).ReadAccountFinancialUsage(context.Background(), from, to)
	require.NoError(t, err)
	require.True(t, snapshot.ProbeDataError)
	require.Equal(t, "probe_aggregate_unavailable", *snapshot.ProbeErrorCode)
	require.Nil(t, snapshot.ProbeRows)
	require.Equal(t, 12.5, snapshot.UserBalanceCNY)
	require.Equal(t, int64(1), snapshot.Rows[0].Requests)
	require.Equal(t, 3.0, snapshot.Rows[0].Cost)
	require.NoError(t, mock.ExpectationsWereMet())
}

func int64Ptr(v int64) *int64 { return &v }
