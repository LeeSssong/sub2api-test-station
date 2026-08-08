package service

import (
	"context"
	"database/sql"
	"math"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestAccountProfitabilityServiceAggregatesRevenueAndRelayExpense(t *testing.T) {
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC)
	db, mock := newAccountProfitabilityDB(t)

	mock.ExpectQuery(accountProfitabilityQueryPattern()).
		WithArgs(start, end).
		WillReturnRows(accountProfitabilityRows().
			AddRow(int64(11), "Sub relay", PlatformOpenAI, AccountTypeAPIKey, StatusActive,
				balanceEvidenceJSON(AccountMonitorBalanceSourceSub2API), nil, nil, nil,
				12.5, 4.25, int64(3), int64(900)).
			AddRow(int64(12), "New relay", PlatformOpenAI, AccountTypeAPIKey, StatusActive,
				balanceEvidenceJSON(AccountMonitorBalanceSourceNewAPI), nil, nil, nil,
				7.5, 2.5, int64(2), int64(500)))

	report, err := NewAccountProfitabilityService(db).GetReport(context.Background(), start, end)
	require.NoError(t, err)
	require.Len(t, report.Rows, 2)
	require.Equal(t, AccountProfitabilitySourceSub2API, report.Rows[0].Source)
	require.Equal(t, AccountProfitabilitySourceNewAPI, report.Rows[1].Source)
	require.Equal(t, AccountProfitabilityExpenseAvailable, report.Rows[0].ExpenseStatus)
	require.Equal(t, AccountProfitabilityCostBasisUsage, report.Rows[0].CostBasis)
	require.Equal(t, 12.5, report.Rows[0].Revenue)
	require.NotNil(t, report.Rows[0].Expense)
	require.InDelta(t, 4.25, *report.Rows[0].Expense, 1e-12)
	require.NotNil(t, report.Rows[0].Profit)
	require.InDelta(t, 8.25, *report.Rows[0].Profit, 1e-12)
	require.NotNil(t, report.Rows[0].Margin)
	require.InDelta(t, 0.66, *report.Rows[0].Margin, 1e-12)
	require.Equal(t, int64(3), report.Rows[0].RequestCount)
	require.Equal(t, int64(900), report.Rows[0].Tokens)
	require.Equal(t, 20.0, report.Summary.Revenue)
	require.NotNil(t, report.Summary.Expense)
	require.InDelta(t, 6.75, *report.Summary.Expense, 1e-12)
	require.NotNil(t, report.Summary.Profit)
	require.InDelta(t, 13.25, *report.Summary.Profit, 1e-12)
	require.NotNil(t, report.Summary.Margin)
	require.InDelta(t, 0.6625, *report.Summary.Margin, 1e-12)
	require.Equal(t, 2, report.Summary.AccountCount)
	require.Zero(t, report.Summary.PendingCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountProfitabilityServiceAllocatesSelfPurchasedCostByActiveOverlap(t *testing.T) {
	start := time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	effective := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	expires := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	db, mock := newAccountProfitabilityDB(t)

	mock.ExpectQuery(accountProfitabilityQueryPattern()).
		WithArgs(start, end).
		WillReturnRows(accountProfitabilityRows().
			AddRow(int64(21), "Purchased", PlatformOpenAI, AccountTypeOAuth, StatusActive,
				`{}`, 31.0, effective, expires, 9.0, 8.0, int64(4), int64(1200)))

	report, err := NewAccountProfitabilityService(db).GetReport(context.Background(), start, end)
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	row := report.Rows[0]
	require.Equal(t, AccountProfitabilitySourceSelfPurchased, row.Source)
	require.Equal(t, AccountProfitabilityExpenseAvailable, row.ExpenseStatus)
	require.Equal(t, AccountProfitabilityCostBasisProcurement, row.CostBasis)
	require.Equal(t, "CNY", row.ExpenseCurrency)
	require.NotNil(t, row.Expense)
	require.InDelta(t, 2.0, *row.Expense, 1e-12)
	require.NotNil(t, row.ProcurementExpenseCNY)
	require.Nil(t, row.Profit)
	require.Nil(t, report.Summary.Profit)
	require.Equal(t, 1, report.Summary.PendingCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountProfitabilityServiceMarksUnknownCostPending(t *testing.T) {
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC)
	db, mock := newAccountProfitabilityDB(t)

	mock.ExpectQuery(accountProfitabilityQueryPattern()).
		WithArgs(start, end).
		WillReturnRows(accountProfitabilityRows().
			AddRow(int64(31), "Unknown", PlatformAnthropic, AccountTypeOAuth, StatusActive,
				`{}`, nil, nil, nil, 5.0, 4.0, int64(1), int64(100)))

	report, err := NewAccountProfitabilityService(db).GetReport(context.Background(), start, end)
	require.NoError(t, err)
	row := report.Rows[0]
	require.Equal(t, AccountProfitabilitySourcePending, row.Source)
	require.Equal(t, AccountProfitabilityExpensePending, row.ExpenseStatus)
	require.Equal(t, AccountProfitabilityCostBasisMissing, row.CostBasis)
	require.Nil(t, row.Expense)
	require.Nil(t, row.Profit)
	require.Nil(t, row.Margin)
	require.Equal(t, 1, report.Summary.PendingCount)
	require.Nil(t, report.Summary.Expense)
	require.Nil(t, report.Summary.Profit)
	require.Nil(t, report.Summary.Margin)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountProfitabilityServiceMarksIncompleteProcurementDatesPending(t *testing.T) {
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC)
	db, mock := newAccountProfitabilityDB(t)

	mock.ExpectQuery(accountProfitabilityQueryPattern()).
		WithArgs(start, end).
		WillReturnRows(accountProfitabilityRows().
			AddRow(int64(32), "Undated purchase", PlatformAnthropic, AccountTypeOAuth, StatusActive,
				`{}`, 30.0, nil, nil, 5.0, 4.0, int64(1), int64(100)))

	report, err := NewAccountProfitabilityService(db).GetReport(context.Background(), start, end)
	require.NoError(t, err)
	row := report.Rows[0]
	require.Equal(t, AccountProfitabilitySourceSelfPurchased, row.Source)
	require.Equal(t, AccountProfitabilityExpensePending, row.ExpenseStatus)
	require.Nil(t, row.Expense)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountProfitabilityServiceUsesZeroMarginWhenRevenueIsZero(t *testing.T) {
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC)
	db, mock := newAccountProfitabilityDB(t)

	mock.ExpectQuery(accountProfitabilityQueryPattern()).
		WithArgs(start, end).
		WillReturnRows(accountProfitabilityRows().
			AddRow(int64(41), "Idle relay", PlatformOpenAI, AccountTypeAPIKey, StatusActive,
				balanceEvidenceJSON(AccountMonitorBalanceSourceSub2API), nil, nil, nil,
				0.0, 0.0, int64(0), int64(0)))

	report, err := NewAccountProfitabilityService(db).GetReport(context.Background(), start, end)
	require.NoError(t, err)
	row := report.Rows[0]
	require.NotNil(t, row.Margin)
	require.False(t, math.IsNaN(*row.Margin))
	require.Equal(t, 0.0, *row.Margin)
	require.NotNil(t, report.Summary.Margin)
	require.Equal(t, 0.0, *report.Summary.Margin)
	require.NoError(t, mock.ExpectationsWereMet())
}

func newAccountProfitabilityDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

func accountProfitabilityRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"account_id", "name", "platform", "account_type", "status", "extra",
		"procurement_cost_cny", "procurement_cost_effective_at", "expires_at",
		"revenue", "relay_expense", "request_count", "tokens",
	})
}

func accountProfitabilityQueryPattern() string {
	return regexp.QuoteMeta("SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1))")
}

func balanceEvidenceJSON(source string) string {
	return `{"account_monitor_balance":{"version":1,"source":"` + source + `","status":"ok"}}`
}
