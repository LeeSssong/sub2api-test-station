package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestBusinessOverviewMarginIsZeroWhenRevenueIsZero(t *testing.T) {
	profit := 0.0
	revenue := 0.0
	report := BusinessOverviewReport{Summary: BusinessOverviewSummary{RevenueCNY: &revenue, UpstreamCostCNY: &profit}}
	finalizeBusinessOverviewSummary(&report.Summary)
	require.NotNil(t, report.Summary.GrossProfitCNY)
	require.NotNil(t, report.Summary.GrossMargin)
	require.Equal(t, 0.0, *report.Summary.GrossProfitCNY)
	require.Equal(t, 0.0, *report.Summary.GrossMargin)
}

func TestBusinessOverviewUsesActualCostAndZeroDefaults(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	start := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	mock.ExpectQuery("SELECT ul\\.id, ul\\.created_at").WithArgs(start, end, nil).WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at", "group_id", "group_name", "model", "actual_cost", "cost", "usage_completeness"}).
			AddRow(int64(11), start.Add(time.Hour), nil, "", "gpt-test", 23.35, 95.31, "complete"),
	)
	mock.ExpectQuery("SELECT record_type").WithArgs(start, end).WillReturnRows(sqlmock.NewRows([]string{"record_type", "created_at", "cash_delta", "paid_delta", "gift_delta"}))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(cash_balance_cny\\)").WillReturnRows(sqlmock.NewRows([]string{"cash", "paid", "gift"}).AddRow(0, 0, 0))

	report, err := NewBusinessOverviewService(db).GetReport(context.Background(), BusinessOverviewQuery{Start: start, End: end, Timezone: time.UTC})
	require.NoError(t, err)
	require.Equal(t, BusinessOverviewRevenueConfirmed, report.RevenueStatus)
	require.Equal(t, 0, report.Summary.PendingSplitCount)
	require.Equal(t, 0, report.Summary.PendingCostCount)
	require.Equal(t, 23.35, *report.Summary.RevenueCNY)
	require.Equal(t, 95.31, *report.Summary.UpstreamCostCNY)
	require.InDelta(t, -71.96, *report.Summary.GrossProfitCNY, 0.000001)
	require.InDelta(t, -71.96/23.35, *report.Summary.GrossMargin, 0.000001)
	require.Len(t, report.Groups, 1)
	require.Equal(t, int64(1), report.Groups[0].RequestCount)
	require.Equal(t, 23.35, *report.Groups[0].RevenueCNY)
	require.Equal(t, 95.31, *report.Groups[0].UpstreamCostCNY)
	require.InDelta(t, -71.96, *report.Groups[0].GrossProfitCNY, 0.000001)
	require.NotNil(t, report.CashAndBalance.CashRechargeCNY)
	require.Equal(t, 0.0, *report.CashAndBalance.CashRechargeCNY)
	require.Len(t, report.Trend, 1)
	require.Equal(t, 23.35, report.Trend[0].PaidConsumptionCNY)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBusinessOverviewEmptyAndNullValuesBecomeZero(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	start := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	mock.ExpectQuery("SELECT ul\\.id, ul\\.created_at").WithArgs(start, end, nil).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "group_id", "group_name", "model", "actual_cost", "cost", "usage_completeness"}))
	mock.ExpectQuery("SELECT record_type").WithArgs(start, end).WillReturnRows(sqlmock.NewRows([]string{"record_type", "created_at", "cash_delta", "paid_delta", "gift_delta"}))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(cash_balance_cny\\)").WillReturnRows(sqlmock.NewRows([]string{"cash", "paid", "gift"}).AddRow(0, 0, 0))

	report, err := NewBusinessOverviewService(db).GetReport(context.Background(), BusinessOverviewQuery{Start: start, End: end, Timezone: time.UTC})
	require.NoError(t, err)
	require.NotNil(t, report.Summary.RevenueCNY)
	require.NotNil(t, report.Summary.UpstreamCostCNY)
	require.NotNil(t, report.Summary.GrossProfitCNY)
	require.NotNil(t, report.Summary.GrossMargin)
	require.Equal(t, 0.0, *report.Summary.RevenueCNY)
	require.Equal(t, 0.0, *report.Summary.UpstreamCostCNY)
	require.Equal(t, 0.0, *report.Summary.GrossProfitCNY)
	require.Equal(t, 0.0, *report.Summary.GrossMargin)
	require.Equal(t, 0.0, report.Trend[0].CashRechargeCNY)
	require.Equal(t, 0.0, report.Trend[0].PaidConsumptionCNY)
	require.Equal(t, 0.0, report.Trend[0].NetSettlementCNY)
	require.Equal(t, BusinessOverviewBalanceBalanced, report.CashAndBalance.Reconciliation.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBusinessOverviewExcludesUnknownAttempts(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	start := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	mock.ExpectQuery("SELECT ul\\.id, ul\\.created_at").WithArgs(start, end, nil).WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at", "group_id", "group_name", "model", "actual_cost", "cost", "usage_completeness"}).
			AddRow(int64(1), start.Add(time.Hour), nil, "", "ignored", 100.0, 100.0, "unknown").
			AddRow(int64(2), start.Add(2*time.Hour), nil, "", "kept", 2.0, 3.0, "complete"),
	)
	mock.ExpectQuery("SELECT record_type").WithArgs(start, end).WillReturnRows(sqlmock.NewRows([]string{"record_type", "created_at", "cash_delta", "paid_delta", "gift_delta"}))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(cash_balance_cny\\)").WillReturnRows(sqlmock.NewRows([]string{"cash", "paid", "gift"}).AddRow(0, 0, 0))

	report, err := NewBusinessOverviewService(db).GetReport(context.Background(), BusinessOverviewQuery{Start: start, End: end, Timezone: time.UTC})
	require.NoError(t, err)
	require.Equal(t, 1, len(report.Groups))
	require.Equal(t, int64(1), report.Groups[0].RequestCount)
	require.Equal(t, 2.0, *report.Summary.RevenueCNY)
	require.Equal(t, 3.0, *report.Summary.UpstreamCostCNY)
	require.Equal(t, 2.0, report.Trend[0].PaidConsumptionCNY)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBusinessOverviewDatabaseFailureRemainsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	start := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	mock.ExpectQuery("SELECT ul\\.id, ul\\.created_at").WithArgs(start, end, nil).WillReturnError(errors.New("database unavailable"))

	_, err = NewBusinessOverviewService(db).GetReport(context.Background(), BusinessOverviewQuery{Start: start, End: end, Timezone: time.UTC})
	require.EqualError(t, err, "query business overview usage: database unavailable")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBusinessOverviewRangeUsesInclusiveBeijingDates(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	start, end, err := BusinessOverviewDateRange("2026-08-01", "2026-08-01", loc)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, loc), start)
	require.Equal(t, time.Date(2026, 8, 2, 0, 0, 0, 0, loc), end)
}

func TestBusinessOverviewReconciliation(t *testing.T) {
	got := reconcileBusinessOverview(10, 5, 2, 5, 12)
	require.Equal(t, BusinessOverviewBalanceBalanced, got.Status)
	require.Equal(t, 0.0, got.DifferenceCNY)
	got = reconcileBusinessOverview(10, 5, 2, 0, 12)
	require.Equal(t, BusinessOverviewBalanceUnbalanced, got.Status)
	require.Equal(t, 5.0, got.DifferenceCNY)
}

func TestBusinessOverviewWithoutT55TablesUsesZeroDefaults(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	start := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	mock.ExpectQuery("SELECT ul\\.id, ul\\.created_at").WithArgs(start, end, nil).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "group_id", "group_name", "model", "actual_cost", "cost", "usage_completeness"}).AddRow(int64(11), start.Add(time.Hour), nil, "", "gpt-test", 4.5, 4.5, "complete"))
	mock.ExpectQuery("SELECT record_type").WithArgs(start, end).WillReturnError(errors.New("relation user_quota_ledger_entries does not exist"))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(cash_balance_cny\\)").WillReturnError(errors.New("relation user_wallets does not exist"))

	report, err := NewBusinessOverviewService(db).GetReport(context.Background(), BusinessOverviewQuery{Start: start, End: end, Timezone: time.UTC})
	require.NoError(t, err)
	require.Equal(t, BusinessOverviewRevenueConfirmed, report.RevenueStatus)
	require.Equal(t, 4.5, *report.Summary.RevenueCNY)
	require.NotNil(t, report.Summary.UpstreamCostCNY)
	require.Equal(t, 4.5, *report.Summary.UpstreamCostCNY)
	require.Len(t, report.Groups, 1)
	require.Equal(t, int64(1), report.Groups[0].RequestCount)
	require.NoError(t, mock.ExpectationsWereMet())
}
