package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
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
	return regexp.QuoteMeta("SUM(COALESCE(ul.account_cost, COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)))")
}

func balanceEvidenceJSON(source string) string {
	return `{"account_monitor_balance":{"version":1,"source":"` + source + `","status":"ok"}}`
}

func TestCalculateProcurementMetricsCapsAndSettlesLoss(t *testing.T) {
	c, p, l, u, profit, margin := calculateProcurementMetrics(120, 60, 90, 100, false)
	require.Equal(t, 120.0, c)
	require.Zero(t, p)
	require.Zero(t, l)
	require.Equal(t, 1.0, *u)
	require.Equal(t, -20.0, *profit)
	require.Equal(t, -.2, *margin)
	c, p, l, _, _, _ = calculateProcurementMetrics(120, 60, 30, 100, true)
	require.Equal(t, 60.0, c)
	require.Zero(t, p)
	require.Equal(t, 60.0, l)
}

func TestSelfPurchasedReportScansTimestampsAndAggregatesVersions(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	db, mock := newAccountProfitabilityDB(t)
	columns := []string{"id", "name", "platform", "type", "status", "version_no", "cost_cny", "estimated_usable_quota_usd", "effective_at", "ended_at", "version_status", "settled_at", "loss_cny", "standard_consumed", "revenue"}
	ended := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	settled := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("WITH versions AS").WithArgs(start, end).WillReturnRows(sqlmock.NewRows(columns).
		AddRow(int64(7), "purchase", PlatformOpenAI, AccountTypeOAuth, StatusActive, 1, 100.0, 50.0, start, ended, "ended", nil, 0.0, 25.0, 60.0).
		AddRow(int64(7), "purchase", PlatformOpenAI, AccountTypeOAuth, StatusActive, 2, 50.0, 25.0, ended, settled, "settled", settled, 20.0, 10.0, 30.0))
	report, err := NewAccountProfitabilityService(db).GetSelfPurchasedReport(context.Background(), start, end)
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	row := report.Rows[0]
	require.Equal(t, 70.0, row.ConfirmedCostCNY)
	require.Zero(t, row.PendingCostCNY, "ended versions must not retain cost that rolled into a later version")
	require.Equal(t, 20.0, row.LossCNY)
	require.Equal(t, 90.0, row.RevenueCNY)
	require.Equal(t, -0.0, *row.NetProfitCNY)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSelfPurchasedReportKeepsCurrentCostPendingAccountUnpriced(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	db, mock := newAccountProfitabilityDB(t)
	columns := []string{"id", "name", "platform", "type", "status", "version_no", "cost_cny", "estimated_usable_quota_usd", "effective_at", "ended_at", "version_status", "settled_at", "loss_cny", "standard_consumed", "revenue"}
	changedAt := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("WITH versions AS").WithArgs(start, end).WillReturnRows(sqlmock.NewRows(columns).
		AddRow(int64(7), "purchase", PlatformOpenAI, AccountTypeOAuth, StatusActive, 1, 100.0, 50.0, start, changedAt, "ended", nil, 0.0, 10.0, 20.0).
		AddRow(int64(7), "purchase", PlatformOpenAI, AccountTypeOAuth, StatusActive, 2, nil, nil, changedAt, nil, "cost_pending", nil, 0.0, 0.0, 15.0))

	report, err := NewAccountProfitabilityService(db).GetSelfPurchasedReport(context.Background(), start, end)
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.Equal(t, ProcurementStatusCostPending, report.Rows[0].CostStatus)
	require.Nil(t, report.Rows[0].ProcurementCostCNY)
	require.Nil(t, report.Rows[0].EstimatedQuotaUSD)
	require.Nil(t, report.Rows[0].NetProfitCNY)
	require.Nil(t, report.Rows[0].Margin)
	require.Equal(t, 1, report.Summary.AccountCount)
	require.Nil(t, report.Summary.NetProfitCNY)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSelfPurchasedReportIncludesPartialLegacyProjectionAsCostPending(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	db, mock := newAccountProfitabilityDB(t)
	columns := []string{"id", "name", "platform", "type", "status", "version_no", "cost_cny", "estimated_usable_quota_usd", "effective_at", "ended_at", "version_status", "settled_at", "loss_cny", "standard_consumed", "revenue"}
	mock.ExpectQuery("WITH versions AS").WithArgs(start, end).WillReturnRows(sqlmock.NewRows(columns).
		AddRow(int64(8), "partial legacy purchase", PlatformOpenAI, AccountTypeOAuth, StatusActive, 0, 40.0, nil, start, nil, "active", nil, 0.0, 12.0, 18.0))

	report, err := NewAccountProfitabilityService(db).GetSelfPurchasedReport(context.Background(), start, end)
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.Equal(t, ProcurementStatusCostPending, report.Rows[0].CostStatus)
	require.Nil(t, report.Rows[0].ProcurementCostCNY)
	require.Nil(t, report.Rows[0].NetProfitCNY)
	require.Equal(t, 12.0, report.Rows[0].StandardConsumedUSD)
	require.Equal(t, 18.0, report.Rows[0].RevenueCNY)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSelfPurchasedReportDoesNotReturnNaNUtilizationForZeroCostSettlement(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	db, mock := newAccountProfitabilityDB(t)
	columns := []string{"id", "name", "platform", "type", "status", "version_no", "cost_cny", "estimated_usable_quota_usd", "effective_at", "ended_at", "version_status", "settled_at", "loss_cny", "standard_consumed", "revenue"}
	settledAt := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("WITH versions AS").WithArgs(start, end).WillReturnRows(sqlmock.NewRows(columns).
		AddRow(int64(7), "free purchase", PlatformOpenAI, AccountTypeOAuth, StatusError, 1, 0.0, 50.0, start, settledAt, "settled", settledAt, 0.0, 0.0, 0.0))

	report, err := NewAccountProfitabilityService(db).GetSelfPurchasedReport(context.Background(), start, end)
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.Nil(t, report.Rows[0].Utilization)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSelfPurchasedReportOwnershipComesOnlyFromLedgerOrProjection(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		lower := strings.ToLower(actual)
		if strings.Count(lower, "a.type = 'oauth'")+strings.Count(lower, "a.type='oauth'") < 2 {
			return errors.New("self-purchased report must restrict accounts to oauth")
		}
		for _, required := range []string{"account_procurement_cost_versions", "billing_mode", "usage_completeness", "image_count", "video_count", "request_type"} {
			if !strings.Contains(actual, required) {
				return fmt.Errorf("missing procurement report filter %s", required)
			}
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("").WithArgs(start, end).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "platform", "type", "status", "version_no", "cost_cny", "estimated_usable_quota_usd", "effective_at", "ended_at", "version_status", "settled_at", "loss_cny", "standard_consumed", "revenue"}))

	report, err := NewAccountProfitabilityService(db).GetSelfPurchasedReport(context.Background(), start, end)
	require.NoError(t, err)
	require.Empty(t, report.Rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateProcurementConfigRepricesCostPendingVersionWithoutScanningNulls(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	createdAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	cost, quota := 7.7, 60.0
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions WHERE request_id").WithArgs("req-pending-reprice").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT created_at FROM accounts").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(createdAt))
	mock.ExpectQuery("SELECT id,cost_cny").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"id", "cost_cny", "estimated_usable_quota_usd", "effective_at"}).AddRow(int64(3), nil, nil, now.Add(-time.Hour)))
	mock.ExpectExec("UPDATE account_procurement_cost_versions SET ended_at").WithArgs(int64(3), now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version_no\\),0\\)\\+1").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"next"}).AddRow(2))
	mock.ExpectExec("INSERT INTO account_procurement_cost_versions").WithArgs(int64(9), 2, cost, quota, now, int64(77), "req-pending-reprice", now).WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("UPDATE accounts SET procurement_cost_cny").WithArgs(int64(9), cost, quota, now, now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(int64(77), "/admin/accounts/9/procurement", "req-pending-reprice", int64(9), cost, quota).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	svc := NewAccountProfitabilityService(db)
	svc.now = func() time.Time { return now }
	require.NoError(t, svc.UpdateProcurementConfig(context.Background(), ProcurementConfigInput{AccountID: 9, CostCNY: &cost, QuotaUSD: &quota, ActorUserID: 77, RequestID: "req-pending-reprice"}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateProcurementConfigFirstVersionUsesCreatedAtAndAuditsActor(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	createdAt := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	cost, quota := 120.0, 60.0
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions WHERE request_id").WithArgs("req-first").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT created_at FROM accounts").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(createdAt))
	mock.ExpectQuery("SELECT id,cost_cny").WithArgs(int64(9)).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version_no\\),0\\)\\+1").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"next"}).AddRow(1))
	mock.ExpectExec("INSERT INTO account_procurement_cost_versions").WithArgs(int64(9), 1, cost, quota, createdAt, int64(77), "req-first", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE accounts SET procurement_cost_cny").WithArgs(int64(9), cost, quota, createdAt, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(int64(77), "/admin/accounts/9/procurement", "req-first", int64(9), cost, quota).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	err := NewAccountProfitabilityService(db).UpdateProcurementConfig(context.Background(), ProcurementConfigInput{AccountID: 9, CostCNY: &cost, QuotaUSD: &quota, ActorUserID: 77, RequestID: "req-first"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateProcurementConfigIdempotentReplayDoesNotWriteAgain(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	cost, quota := 120.0, 60.0
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions WHERE request_id").WithArgs("req-replay").WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(9)))
	mock.ExpectCommit()
	err := NewAccountProfitabilityService(db).UpdateProcurementConfig(context.Background(), ProcurementConfigInput{AccountID: 9, CostCNY: &cost, QuotaUSD: &quota, ActorUserID: 77, RequestID: "req-replay"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateProcurementConfigSubsequentVersionStoresRemainingValues(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	createdAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	effectiveAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	cost, quota := 120.0, 60.0
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions WHERE request_id").WithArgs("req-next").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT created_at FROM accounts").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(createdAt))
	mock.ExpectQuery("SELECT id,cost_cny").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"id", "cost_cny", "estimated_usable_quota_usd", "effective_at"}).AddRow(int64(3), 100.0, 50.0, effectiveAt))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(CASE").WithArgs(int64(9), effectiveAt, now).WillReturnRows(sqlmock.NewRows([]string{"consumed"}).AddRow(10.0))
	mock.ExpectExec("UPDATE account_procurement_cost_versions SET ended_at").WithArgs(int64(3), now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version_no\\),0\\)\\+1").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"next"}).AddRow(2))
	mock.ExpectExec("INSERT INTO account_procurement_cost_versions").WithArgs(int64(9), 2, 100.0, 50.0, now, int64(77), "req-next", now).WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("UPDATE accounts SET procurement_cost_cny").WithArgs(int64(9), 100.0, 50.0, now, now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(int64(77), "/admin/accounts/9/procurement", "req-next", int64(9), 100.0, 50.0).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	svc := NewAccountProfitabilityService(db)
	svc.now = func() time.Time { return now }
	err := svc.UpdateProcurementConfig(context.Background(), ProcurementConfigInput{AccountID: 9, CostCNY: &cost, QuotaUSD: &quota, ActorUserID: 77, RequestID: "req-next"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateProcurementConfigClearCreatesCostPendingVersion(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	createdAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	effectiveAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions WHERE request_id").WithArgs("req-clear").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT created_at FROM accounts").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(createdAt))
	mock.ExpectQuery("SELECT id,cost_cny").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"id", "cost_cny", "estimated_usable_quota_usd", "effective_at"}).AddRow(int64(3), 100.0, 50.0, effectiveAt))
	mock.ExpectExec("UPDATE account_procurement_cost_versions SET ended_at").WithArgs(int64(3), now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version_no\\),0\\)\\+1").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"next"}).AddRow(2))
	mock.ExpectExec("UPDATE accounts SET procurement_cost_cny=NULL").WithArgs(int64(9), now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO account_procurement_cost_versions").WithArgs(int64(9), 2, now, int64(77), "req-clear", now).WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(int64(77), "/admin/accounts/9/procurement", "req-clear", int64(9)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	svc := NewAccountProfitabilityService(db)
	svc.now = func() time.Time { return now }
	err := svc.UpdateProcurementConfig(context.Background(), ProcurementConfigInput{AccountID: 9, ActorUserID: 77, RequestID: "req-clear"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateProcurementConfigRejectsRequestIDReusedByAnotherAccount(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	cost, quota := 120.0, 60.0
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions WHERE request_id").WithArgs("req-conflict").WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(8)))
	mock.ExpectRollback()
	err := NewAccountProfitabilityService(db).UpdateProcurementConfig(context.Background(), ProcurementConfigInput{AccountID: 9, CostCNY: &cost, QuotaUSD: &quota, ActorUserID: 77, RequestID: "req-conflict"})
	require.ErrorContains(t, err, "idempotency key conflict")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettleProcurementUsesPermanentLossAndSettlementIdempotency(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	effective := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id,account_id FROM account_procurement_cost_versions WHERE settlement_request_id").WithArgs("settle-1").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT v.id,v.cost_cny,v.estimated_usable_quota_usd,v.effective_at,v.status,a.status,a.expires_at").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"id", "cost_cny", "estimated_usable_quota_usd", "effective_at", "version_status", "account_status", "expires_at"}).AddRow(int64(3), 100.0, 50.0, effective, "active", StatusError, nil))
	mock.ExpectExec("UPDATE account_procurement_cost_versions v SET status='settled'").WithArgs(int64(3), "settle-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(int64(77), "/admin/accounts/9/procurement/settle", "settle-1", int64(9), "administrator_confirmed_expired").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	ok, err := NewAccountProfitabilityService(db).SettleProcurement(context.Background(), ProcurementSettlementInput{AccountID: 9, RequestID: "settle-1", Reason: "administrator_confirmed_expired", ActorUserID: 77})
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettleProcurementRepeatAfterSettlementIsIdempotent(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	effective := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id,account_id FROM account_procurement_cost_versions WHERE settlement_request_id").WithArgs("settle-2").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT v.id,v.cost_cny,v.estimated_usable_quota_usd,v.effective_at,v.status,a.status,a.expires_at.*ORDER BY v.version_no DESC").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"id", "cost_cny", "estimated_usable_quota_usd", "effective_at", "version_status", "account_status", "expires_at"}).AddRow(int64(3), 100.0, 50.0, effective, "settled", StatusError, nil))
	mock.ExpectCommit()
	ok, err := NewAccountProfitabilityService(db).SettleProcurement(context.Background(), ProcurementSettlementInput{AccountID: 9, RequestID: "settle-2", Reason: "administrator_confirmed_expired", ActorUserID: 77})
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettleProcurementRejectsHealthyUnexpiredAccount(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	effective := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id,account_id FROM account_procurement_cost_versions WHERE settlement_request_id").WithArgs("settle-active").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT v.id,v.cost_cny,v.estimated_usable_quota_usd,v.effective_at,v.status,a.status,a.expires_at").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"id", "cost_cny", "estimated_usable_quota_usd", "effective_at", "version_status", "account_status", "expires_at"}).AddRow(int64(3), 100.0, 50.0, effective, "active", StatusActive, now.Add(time.Hour)))
	mock.ExpectRollback()
	svc := NewAccountProfitabilityService(db)
	svc.now = func() time.Time { return now }
	ok, err := svc.SettleProcurement(context.Background(), ProcurementSettlementInput{AccountID: 9, RequestID: "settle-active", Reason: "administrator_confirmed_expired", ActorUserID: 77})
	require.False(t, ok)
	require.ErrorContains(t, err, "not permanently unavailable")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettleProcurementRejectsRequestIDReusedByAnotherAccount(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id,account_id FROM account_procurement_cost_versions WHERE settlement_request_id").WithArgs("settle-conflict").WillReturnRows(sqlmock.NewRows([]string{"id", "account_id"}).AddRow(int64(3), int64(8)))
	mock.ExpectRollback()
	ok, err := NewAccountProfitabilityService(db).SettleProcurement(context.Background(), ProcurementSettlementInput{AccountID: 9, RequestID: "settle-conflict", Reason: "expired", ActorUserID: 77})
	require.False(t, ok)
	require.ErrorContains(t, err, "idempotency key conflict")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSelfPurchasedReportUsesAllUndeletedOAuthAccountsAsCandidates(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		lower := strings.ToLower(actual)
		for _, required := range []string{"from accounts a", "left join", "a.deleted_at is null", "a.type = 'oauth'"} {
			if !strings.Contains(lower, required) {
				return fmt.Errorf("missing all-oauth candidate contract %q", required)
			}
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	columns := []string{"id", "name", "platform", "type", "status", "version_no", "cost_cny", "estimated_usable_quota_usd", "effective_at", "ended_at", "version_status", "settled_at", "loss_cny", "standard_consumed", "revenue"}
	mock.ExpectQuery("").WithArgs(start, end).WillReturnRows(sqlmock.NewRows(columns).
		AddRow(int64(41), "No cost", PlatformOpenAI, AccountTypeOAuth, StatusActive, 0, nil, nil, start, nil, string(ProcurementStatusCostPending), nil, 0.0, 0.0, 0.0))

	report, err := NewAccountProfitabilityService(db).GetSelfPurchasedReport(context.Background(), start, end)
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.Equal(t, int64(41), report.Rows[0].AccountID)
	require.Equal(t, ProcurementStatusCostPending, report.Rows[0].CostStatus)
	require.Nil(t, report.Rows[0].ProcurementCostCNY)
	require.Nil(t, report.Rows[0].EstimatedQuotaUSD)
	require.Zero(t, report.Rows[0].StandardConsumedUSD)
	require.Zero(t, report.Rows[0].RevenueCNY)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProcurementAuditSQLUsesPostgreSQLParameterTypes(t *testing.T) {
	source, err := os.ReadFile("account_procurement_profitability.go")
	require.NoError(t, err)
	sourceText := string(source)
	require.Equal(t, 3, strings.Count(sourceText, "LEFT($3,64)"), "all procurement audit request ids must stay bounded to audit_logs.request_id VARCHAR(64)")
	require.Contains(t, sourceText, "jsonb_build_object('account_id',$4::bigint,'reason',$5::text)")
	require.Contains(t, sourceText, "jsonb_build_object('account_id',$4::bigint,'cleared',true)")
	require.Contains(t, sourceText, "jsonb_build_object('account_id',$4::bigint,'cost_cny',$5::double precision,'quota_usd',$6::double precision)")
	require.NotContains(t, sourceText, "jsonb_build_object('account_id',$4,'reason',$5)")
	require.NotContains(t, sourceText, "jsonb_build_object('account_id',$4,'cleared',true)")
	require.NotContains(t, sourceText, "jsonb_build_object('account_id',$4,'cost_cny',$5,'quota_usd',$6)")
}

func TestUpdateProcurementConfigRollsBackWhenAuditInsertFails(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	createdAt := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	cost, quota := 120.0, 60.0
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions WHERE request_id").WithArgs("req-audit-fail").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT created_at FROM accounts").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(createdAt))
	mock.ExpectQuery("SELECT id,cost_cny").WithArgs(int64(9)).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version_no\\),0\\)\\+1").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"next"}).AddRow(1))
	mock.ExpectExec("INSERT INTO account_procurement_cost_versions").WithArgs(int64(9), 1, cost, quota, createdAt, int64(77), "req-audit-fail", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE accounts SET procurement_cost_cny").WithArgs(int64(9), cost, quota, createdAt, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(int64(77), "/admin/accounts/9/procurement", "req-audit-fail", int64(9), cost, quota).WillReturnError(errors.New("audit write failed"))
	mock.ExpectRollback()

	err := NewAccountProfitabilityService(db).UpdateProcurementConfig(context.Background(), ProcurementConfigInput{AccountID: 9, CostCNY: &cost, QuotaUSD: &quota, ActorUserID: 77, RequestID: "req-audit-fail"})
	require.ErrorContains(t, err, "audit write failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateProcurementConfigClearRollsBackWhenAuditInsertFails(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	createdAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	effectiveAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions WHERE request_id").WithArgs("req-clear-audit-fail").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT created_at FROM accounts").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(createdAt))
	mock.ExpectQuery("SELECT id,cost_cny").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"id", "cost_cny", "estimated_usable_quota_usd", "effective_at"}).AddRow(int64(3), 100.0, 50.0, effectiveAt))
	mock.ExpectExec("UPDATE account_procurement_cost_versions SET ended_at").WithArgs(int64(3), now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version_no\\),0\\)\\+1").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"next"}).AddRow(2))
	mock.ExpectExec("UPDATE accounts SET procurement_cost_cny=NULL").WithArgs(int64(9), now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO account_procurement_cost_versions").WithArgs(int64(9), 2, now, int64(77), "req-clear-audit-fail", now).WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(int64(77), "/admin/accounts/9/procurement", "req-clear-audit-fail", int64(9)).WillReturnError(errors.New("clear audit write failed"))
	mock.ExpectRollback()

	svc := NewAccountProfitabilityService(db)
	svc.now = func() time.Time { return now }
	err := svc.UpdateProcurementConfig(context.Background(), ProcurementConfigInput{AccountID: 9, ActorUserID: 77, RequestID: "req-clear-audit-fail"})
	require.ErrorContains(t, err, "clear audit write failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettleProcurementRollsBackWhenAuditInsertFails(t *testing.T) {
	db, mock := newAccountProfitabilityDB(t)
	effective := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id,account_id FROM account_procurement_cost_versions WHERE settlement_request_id").WithArgs("settle-audit-fail").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT v.id,v.cost_cny,v.estimated_usable_quota_usd,v.effective_at,v.status,a.status,a.expires_at").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"id", "cost_cny", "estimated_usable_quota_usd", "effective_at", "version_status", "account_status", "expires_at"}).AddRow(int64(3), 100.0, 50.0, effective, "active", StatusError, nil))
	mock.ExpectExec("UPDATE account_procurement_cost_versions v SET status='settled'").WithArgs(int64(3), "settle-audit-fail").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(int64(77), "/admin/accounts/9/procurement/settle", "settle-audit-fail", int64(9), "administrator_confirmed_expired").WillReturnError(errors.New("settle audit write failed"))
	mock.ExpectRollback()

	ok, err := NewAccountProfitabilityService(db).SettleProcurement(context.Background(), ProcurementSettlementInput{AccountID: 9, RequestID: "settle-audit-fail", Reason: "administrator_confirmed_expired", ActorUserID: 77})
	require.False(t, ok)
	require.ErrorContains(t, err, "settle audit write failed")
	require.NoError(t, mock.ExpectationsWereMet())
}
