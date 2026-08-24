package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestBusinessOverviewMarginIsNilWhenRevenueIsZero(t *testing.T) {
	profit := 0.0
	revenue := 0.0
	report := BusinessOverviewReport{Summary: BusinessOverviewSummary{RevenueCNY: &revenue, UpstreamCostCNY: &profit}}
	finalizeBusinessOverviewSummary(&report.Summary)
	require.Nil(t, report.Summary.GrossProfitCNY)
	require.Nil(t, report.Summary.GrossMargin)
}

func TestBusinessOverviewPendingSplitDoesNotTreatUsageAsRevenue(t *testing.T) {
	revenue := 12.0
	cost := 4.0
	summary := BusinessOverviewSummary{RevenueStatus: BusinessOverviewRevenuePendingSplit, RevenueCNY: &revenue, UpstreamCostCNY: &cost}
	markBusinessOverviewPendingSplit(&summary)
	require.Equal(t, BusinessOverviewRevenuePendingSplit, summary.RevenueStatus)
	require.Nil(t, summary.RevenueCNY)
	require.Nil(t, summary.GrossProfitCNY)
	require.Nil(t, summary.GrossMargin)
	require.Equal(t, 4.0, *summary.UpstreamCostCNY)
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

func TestBusinessOverviewWithoutT55TablesPreservesUsageCostAndMarksPending(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	start := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	mock.ExpectQuery("SELECT ul\\.id, ul\\.created_at").WithArgs(start, end, nil).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "group_id", "group_name", "model", "cost"}).AddRow(int64(11), start.Add(time.Hour), nil, "", "gpt-test", 4.5))
	mock.ExpectQuery("SELECT COALESCE\\(reference_id").WithArgs(start, end).WillReturnError(errors.New("relation user_quota_ledger_entries does not exist"))
	mock.ExpectQuery("SELECT record_type").WithArgs(start, end).WillReturnError(errors.New("relation user_quota_ledger_entries does not exist"))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(cash_balance_cny\\)").WillReturnError(errors.New("relation user_wallets does not exist"))

	report, err := NewBusinessOverviewService(db).GetReport(context.Background(), BusinessOverviewQuery{Start: start, End: end, Timezone: time.UTC})
	require.NoError(t, err)
	require.Equal(t, BusinessOverviewRevenuePending, report.RevenueStatus)
	require.Nil(t, report.Summary.RevenueCNY)
	require.NotNil(t, report.Summary.UpstreamCostCNY)
	require.Equal(t, 4.5, *report.Summary.UpstreamCostCNY)
	require.Len(t, report.Groups, 1)
	require.Equal(t, int64(1), report.Groups[0].RequestCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBusinessOverviewMissingLedgerOnlyMatchesT55Tables(t *testing.T) {
	require.True(t, isBusinessOverviewMissingLedger(errors.New(`relation "user_quota_ledger_entries" does not exist`)))
	require.True(t, isBusinessOverviewMissingLedger(errors.New(`relation "user_wallets" does not exist`)))
	require.False(t, isBusinessOverviewMissingLedger(errors.New(`relation "usage_logs" does not exist`)))
	require.False(t, isBusinessOverviewMissingLedger(errors.New("permission denied for table user_wallets")))
}
