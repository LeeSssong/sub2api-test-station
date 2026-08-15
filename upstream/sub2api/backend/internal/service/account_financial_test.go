package service

import (
	"context"
	"math"
	"testing"
	"time"
)

type financialRepoStub struct {
	snapshot *AccountFinancialSnapshot
}

func (r *financialRepoStub) ReadSnapshot(context.Context, AccountFinancialSnapshotQuery) (*AccountFinancialSnapshot, error) {
	return r.snapshot, nil
}
func (r *financialRepoStub) CreateReview(context.Context, UsageCostReviewInput) (*UsageCostReviewResult, error) {
	panic("unexpected CreateReview")
}
func (r *financialRepoStub) FreezeReviewFilter(context.Context, ReviewFilter) (int64, error) {
	panic("unexpected FreezeReviewFilter")
}
func (r *financialRepoStub) ReviewFiltered(context.Context, ReviewFilteredInput) (*UsageCostReviewBatchResult, error) {
	panic("unexpected ReviewFiltered")
}
func (r *financialRepoStub) SetOAuthDailyCost(context.Context, OAuthDailyCostInput) (*FinancialMutationResult, error) {
	panic("unexpected SetOAuthDailyCost")
}
func (r *financialRepoStub) SetTodayOverride(context.Context, TodayOverrideInput) (*FinancialMutationResult, error) {
	panic("unexpected SetTodayOverride")
}
func (r *financialRepoStub) GetUsageEvidence(context.Context, int64) (*UsageFinancialEvidence, error) {
	panic("unexpected GetUsageEvidence")
}

func beijingTime(t *testing.T, value string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func financialFloatPtr(value float64) *float64 { return &value }
func financialInt64Ptr(value int64) *int64     { return &value }

func TestAccountFinancialReportAppliesIndependentOverrideCutoffs(t *testing.T) {
	now := beijingTime(t, "2026-08-13 16:00")
	repo := &financialRepoStub{snapshot: &AccountFinancialSnapshot{
		GeneratedAt: now,
		EnabledAt:   beijingTime(t, "2026-08-01 00:00"),
		Accounts:    []AccountFinancialSnapshotAccount{{ID: 1, Name: "Sub", Type: "api_key"}},
		Entries: []AccountFinancialSnapshotEntry{
			{UsageLogID: 1, AccountID: 1, BusinessDate: "2026-08-13", RevenueCNY: 100, EvidenceID: financialInt64Ptr(10), EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(40)},
			{UsageLogID: 2, AccountID: 1, BusinessDate: "2026-08-13", RevenueCNY: 20, EvidenceID: financialInt64Ptr(11), EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(8)},
			{UsageLogID: 3, AccountID: 1, BusinessDate: "2026-08-13", RevenueCNY: 10, EvidenceStatus: "unavailable", ReviewID: financialInt64Ptr(21), ReviewCostCNY: financialFloatPtr(3)},
		},
		DailyValues: []AccountFinancialDailyValue{{
			AccountID: 1, BusinessDate: "2026-08-13",
			RevenueOverrideCNY: financialFloatPtr(95), RevenueEvidenceCutoffID: financialInt64Ptr(10), RevenueReviewCutoffID: financialInt64Ptr(20),
			CostOverrideCNY: financialFloatPtr(35), CostEvidenceCutoffID: financialInt64Ptr(10), CostReviewCutoffID: financialInt64Ptr(20),
		}},
	}}

	report, err := NewAccountFinancialService(repo, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	assertFinancialAmounts(t, report.Summary, 125, 46, 79, 79.0/125.0)
	assertFinancialAmounts(t, report.Accounts[0].Amounts, 125, 46, 79, 79.0/125.0)
	if !report.GeneratedAt.Equal(now) || !report.Accounts[0].GeneratedAt.Equal(now) {
		t.Fatalf("all report values must share generated_at %v: %#v", now, report)
	}
}

func TestAccountFinancialPendingIsolationOAuthLiteralAndBalance(t *testing.T) {
	now := beijingTime(t, "2026-08-13 16:00")
	repo := &financialRepoStub{snapshot: &AccountFinancialSnapshot{
		GeneratedAt: now, EnabledAt: beijingTime(t, "2026-08-01 00:00"), UserBalanceCNY: 15,
		Accounts: []AccountFinancialSnapshotAccount{
			{ID: 1, Name: "API", Type: "api_key"},
			{ID: 2, Name: "OAuth", Type: "oauth"},
			{ID: 3, Name: "OAuth-like", Type: "api_key", Platform: "oauth"},
		},
		Entries: []AccountFinancialSnapshotEntry{
			{UsageLogID: 1, AccountID: 1, BusinessDate: "2026-08-13", RevenueCNY: 30, EvidenceStatus: "confirmed_zero"},
			{UsageLogID: 2, AccountID: 2, BusinessDate: "2026-08-13", RevenueCNY: 50},
			{UsageLogID: 3, AccountID: 3, BusinessDate: "2026-08-13", RevenueCNY: 7, EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(2)},
		},
	}}

	report, err := NewAccountFinancialService(repo, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	assertFinancialAmounts(t, report.Summary, 7, 2, 5, 5.0/7.0)
	if report.Summary.ExceptionCount != 1 || report.Summary.AffectedRevenueCNY != 30 {
		t.Fatalf("pending exception must be isolated but counted: %#v", report.Summary)
	}
	if report.UserBalanceCNY != 15 {
		t.Fatalf("balance = SUM(nondeleted balance), got %v", report.UserBalanceCNY)
	}
	if report.Accounts[1].Complete || report.Accounts[1].Amounts.Margin != nil {
		t.Fatalf("literal oauth without daily cost must be incomplete: %#v", report.Accounts[1])
	}
}

func TestAccountFinancial24HIgnoresOverridesAnd7D31DAggregateByBeijingDay(t *testing.T) {
	now := beijingTime(t, "2026-08-13 00:30")
	snapshot := &AccountFinancialSnapshot{
		GeneratedAt: now, EnabledAt: beijingTime(t, "2026-07-01 00:00"),
		Accounts: []AccountFinancialSnapshotAccount{{ID: 1, Name: "Sub", Type: "api_key"}},
		Entries: []AccountFinancialSnapshotEntry{
			{UsageLogID: 1, AccountID: 1, CreatedAt: beijingTime(t, "2026-08-12 01:00"), BusinessDate: "2026-08-12", RevenueCNY: 10, EvidenceID: financialInt64Ptr(1), EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(4)},
			{UsageLogID: 2, AccountID: 1, CreatedAt: beijingTime(t, "2026-08-12 23:45"), BusinessDate: "2026-08-12", RevenueCNY: 20, EvidenceID: financialInt64Ptr(2), EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(8)},
			{UsageLogID: 3, AccountID: 1, CreatedAt: beijingTime(t, "2026-08-13 00:10"), BusinessDate: "2026-08-13", RevenueCNY: 5, EvidenceID: financialInt64Ptr(3), EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(2)},
		},
		DailyValues: []AccountFinancialDailyValue{{AccountID: 1, BusinessDate: "2026-08-12", RevenueOverrideCNY: financialFloatPtr(99), RevenueEvidenceCutoffID: financialInt64Ptr(2), CostOverrideCNY: financialFloatPtr(88), CostEvidenceCutoffID: financialInt64Ptr(2)}},
	}
	service := NewAccountFinancialService(&financialRepoStub{snapshot: snapshot}, func() time.Time { return now })

	report24, err := service.GetReport(context.Background(), AccountFinancialRange24H)
	if err != nil {
		t.Fatal(err)
	}
	assertFinancialAmounts(t, report24.Summary, 35, 14, 21, .6)
	report7, err := service.GetReport(context.Background(), AccountFinancialRange7D)
	if err != nil {
		t.Fatal(err)
	}
	assertFinancialAmounts(t, report7.Summary, 104, 90, 14, 14.0/104.0)
	report31, err := service.GetReport(context.Background(), AccountFinancialRange31D)
	if err != nil {
		t.Fatal(err)
	}
	assertFinancialAmounts(t, report31.Summary, 104, 90, 14, 14.0/104.0)
}

func TestAccountFinancialZeroRevenueMarginIsNull(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	report, err := NewAccountFinancialService(&financialRepoStub{snapshot: &AccountFinancialSnapshot{
		GeneratedAt: now, EnabledAt: now.Add(-time.Hour), Accounts: []AccountFinancialSnapshotAccount{{ID: 1, Type: "api_key"}},
	}}, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Margin != nil {
		t.Fatalf("zero revenue margin must be nil: %v", *report.Summary.Margin)
	}
}

func TestAccountFinancialOAuthSevenDaysExcludesOnlyMissingDays(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	report, err := NewAccountFinancialService(&financialRepoStub{snapshot: &AccountFinancialSnapshot{
		GeneratedAt: now, EnabledAt: now.Add(-24 * time.Hour),
		Accounts:    []AccountFinancialSnapshotAccount{{ID: 2, Type: "oauth"}},
		Entries:     []AccountFinancialSnapshotEntry{{UsageLogID: 1, AccountID: 2, BusinessDate: "2026-08-12", RevenueCNY: 20}},
		DailyValues: []AccountFinancialDailyValue{{AccountID: 2, BusinessDate: "2026-08-12", OAuthCostCNY: financialFloatPtr(5)}},
	}}, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRange7D)
	if err != nil {
		t.Fatal(err)
	}
	assertFinancialAmounts(t, report.Summary, 20, 5, 15, .75)
}

func TestAccountFinancialBeijingTodayStartsAtMidnight(t *testing.T) {
	now := beijingTime(t, "2026-08-13 00:30")
	repo := &capturingFinancialRepo{financialRepoStub: financialRepoStub{snapshot: &AccountFinancialSnapshot{GeneratedAt: now}}}
	_, err := NewAccountFinancialService(repo, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	want := beijingTime(t, "2026-08-13 00:00")
	if !repo.query.From.Equal(want) {
		t.Fatalf("from=%v want=%v", repo.query.From, want)
	}
}

type capturingFinancialRepo struct {
	financialRepoStub
	query AccountFinancialSnapshotQuery
}

func (r *capturingFinancialRepo) ReadSnapshot(c context.Context, q AccountFinancialSnapshotQuery) (*AccountFinancialSnapshot, error) {
	r.query = q
	return r.snapshot, nil
}

func TestAccountFinancialOAuthTodayIncludesRevenueAndOverride(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	report, err := NewAccountFinancialService(&financialRepoStub{snapshot: &AccountFinancialSnapshot{GeneratedAt: now, EnabledAt: now.Add(-time.Hour), Accounts: []AccountFinancialSnapshotAccount{{ID: 2, Type: "oauth"}}, Entries: []AccountFinancialSnapshotEntry{{UsageLogID: 1, AccountID: 2, BusinessDate: "2026-08-13", RevenueCNY: 20}}, DailyValues: []AccountFinancialDailyValue{{AccountID: 2, BusinessDate: "2026-08-13", OAuthCostCNY: financialFloatPtr(5), RevenueOverrideCNY: financialFloatPtr(18)}}}}, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	assertFinancialAmounts(t, report.Summary, 18, 5, 13, 13.0/18)
}

func TestAccountFinancialOAuthOverrideKeepsLaterRevenueForTodayAndSevenDays(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	overrideAt := beijingTime(t, "2026-08-13 10:00")
	snapshot := &AccountFinancialSnapshot{
		GeneratedAt: now, EnabledAt: now.Add(-time.Hour),
		Accounts: []AccountFinancialSnapshotAccount{{ID: 2, Type: "oauth"}},
		Entries: []AccountFinancialSnapshotEntry{
			{UsageLogID: 1, AccountID: 2, CreatedAt: beijingTime(t, "2026-08-13 09:00"), BusinessDate: "2026-08-13", RevenueCNY: 20},
			{UsageLogID: 2, AccountID: 2, CreatedAt: beijingTime(t, "2026-08-13 11:00"), BusinessDate: "2026-08-13", RevenueCNY: 3},
		},
		DailyValues: []AccountFinancialDailyValue{{
			AccountID: 2, BusinessDate: "2026-08-13", OAuthCostCNY: financialFloatPtr(5),
			RevenueOverrideCNY: financialFloatPtr(18), RevenueOverrideAt: &overrideAt,
		}},
	}
	service := NewAccountFinancialService(&financialRepoStub{snapshot: snapshot}, func() time.Time { return now })
	for _, r := range []AccountFinancialRange{AccountFinancialRangeToday, AccountFinancialRange7D} {
		report, err := service.GetReport(context.Background(), r)
		if err != nil {
			t.Fatal(err)
		}
		assertFinancialAmounts(t, report.Summary, 21, 5, 16, 16.0/21)
	}
}

func TestAccountFinancialGroupsUseUsageIdentityAndKeepUnassignedLast(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	group10, group20, historical := int64(10), int64(20), int64(30)
	report, err := NewAccountFinancialService(&financialRepoStub{snapshot: &AccountFinancialSnapshot{
		GeneratedAt: now,
		EnabledAt:   now.Add(-time.Hour),
		Groups: []AccountFinancialSnapshotGroup{
			{ID: group20, Name: "Plus"},
			{ID: group10, Name: "Pro"},
		},
		Accounts: []AccountFinancialSnapshotAccount{
			{ID: 1, Name: "shared", Type: "api_key"},
			{ID: 2, Name: "second", Type: "api_key"},
		},
		Entries: []AccountFinancialSnapshotEntry{
			{UsageLogID: 1, AccountID: 1, GroupID: &group10, GroupName: "Pro", BusinessDate: "2026-08-13", RevenueCNY: 40, EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(10)},
			{UsageLogID: 2, AccountID: 1, GroupID: &group20, GroupName: "Plus", BusinessDate: "2026-08-13", RevenueCNY: 30, EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(9)},
			{UsageLogID: 3, AccountID: 2, GroupID: &historical, GroupName: "Archived", BusinessDate: "2026-08-13", RevenueCNY: 20, EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(5)},
			{UsageLogID: 4, AccountID: 2, BusinessDate: "2026-08-13", RevenueCNY: 10, EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(3)},
			{UsageLogID: 5, AccountID: 2, GroupID: &group10, GroupName: "Pro", BusinessDate: "2026-08-13", RevenueCNY: 5, EvidenceStatus: "unavailable"},
		},
	}}, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Groups) != 4 {
		t.Fatalf("groups=%#v", report.Groups)
	}
	if report.Groups[0].ID != group20 || report.Groups[1].ID != group10 || report.Groups[2].ID != historical || !report.Groups[3].Unassigned {
		t.Fatalf("configured, historical, unassigned order not preserved: %#v", report.Groups)
	}
	assertFinancialAmounts(t, report.Groups[0].Amounts, 30, 9, 21, .7)
	assertFinancialAmounts(t, report.Groups[1].Amounts, 40, 10, 30, .75)
	if len(report.Groups[1].Accounts) != 2 || report.Groups[1].Accounts[0].ID != 1 || report.Groups[1].Accounts[1].ID != 2 {
		t.Fatalf("group accounts must use group-scoped usage facts: %#v", report.Groups[1].Accounts)
	}
	if report.Groups[1].Complete || report.Groups[1].ExceptionCount != 1 || report.Groups[1].AffectedRevenueCNY != 5 {
		t.Fatalf("pending group evidence must remain isolated and counted: %#v", report.Groups[1])
	}
	assertFinancialAmounts(t, report.Groups[3].Amounts, 10, 3, 7, .7)
	assertFinancialAmounts(t, report.Summary, 100, 27, 73, .73)
}

func TestAccountFinancialGroupAdjustmentsAreMarkedButNeverAllocated(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	group10, group20 := int64(10), int64(20)
	report, err := NewAccountFinancialService(&financialRepoStub{snapshot: &AccountFinancialSnapshot{
		GeneratedAt: now,
		EnabledAt:   now.Add(-time.Hour),
		Groups:      []AccountFinancialSnapshotGroup{{ID: group10, Name: "Pro"}, {ID: group20, Name: "Plus"}},
		Accounts: []AccountFinancialSnapshotAccount{
			{ID: 1, Name: "api", Type: "api_key"},
			{ID: 2, Name: "oauth", Type: "oauth"},
		},
		Entries: []AccountFinancialSnapshotEntry{
			{UsageLogID: 1, AccountID: 1, GroupID: &group10, BusinessDate: "2026-08-13", RevenueCNY: 60, EvidenceID: financialInt64Ptr(1), EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(24)},
			{UsageLogID: 2, AccountID: 1, GroupID: &group20, BusinessDate: "2026-08-13", RevenueCNY: 30, EvidenceID: financialInt64Ptr(2), EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(12)},
			{UsageLogID: 3, AccountID: 2, GroupID: &group10, BusinessDate: "2026-08-13", RevenueCNY: 10},
		},
		DailyValues: []AccountFinancialDailyValue{
			{AccountID: 1, BusinessDate: "2026-08-13", RevenueOverrideCNY: financialFloatPtr(100), RevenueEvidenceCutoffID: financialInt64Ptr(2), CostOverrideCNY: financialFloatPtr(40), CostEvidenceCutoffID: financialInt64Ptr(2)},
			{AccountID: 2, BusinessDate: "2026-08-13", OAuthCostCNY: financialFloatPtr(3)},
		},
	}}, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	assertFinancialAmounts(t, report.Accounts[0].Amounts, 100, 40, 60, .6)
	assertFinancialAmounts(t, report.Groups[0].Amounts, 60, 24, 36, .6)
	assertFinancialAmounts(t, report.Groups[1].Amounts, 30, 12, 18, .6)
	for _, group := range report.Groups {
		if !group.HasUnallocatedAdjustments || group.Complete {
			t.Fatalf("affected group must disclose unallocated adjustments: %#v", group)
		}
	}
	if !report.Groups[0].Accounts[0].HasUnallocatedAdjustments || report.Groups[0].Accounts[0].Complete {
		t.Fatalf("group account must disclose unallocated adjustment: %#v", report.Groups[0].Accounts[0])
	}
	if !report.Groups[0].Accounts[1].HasUnallocatedAdjustments || report.Groups[0].Accounts[1].Complete {
		t.Fatalf("oauth group account must disclose literal daily cost: %#v", report.Groups[0].Accounts[1])
	}
}

func TestAccountFinancialOAuthGroupIsIncompleteWithoutDailyCost(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	groupID := int64(10)
	report, err := NewAccountFinancialService(&financialRepoStub{snapshot: &AccountFinancialSnapshot{
		GeneratedAt: now,
		EnabledAt:   now.Add(-time.Hour),
		Groups:      []AccountFinancialSnapshotGroup{{ID: groupID, Name: "Pro"}},
		Accounts:    []AccountFinancialSnapshotAccount{{ID: 2, Name: "oauth", Type: "oauth"}},
		Entries:     []AccountFinancialSnapshotEntry{{UsageLogID: 1, AccountID: 2, GroupID: &groupID, BusinessDate: "2026-08-13", RevenueCNY: 10}},
	}}, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Groups) != 1 || report.Groups[0].Complete || !report.Groups[0].HasUnallocatedAdjustments {
		t.Fatalf("oauth group must remain incomplete without an allocatable daily cost: %#v", report.Groups)
	}
	if len(report.Groups[0].Accounts) != 1 || report.Groups[0].Accounts[0].Complete || !report.Groups[0].Accounts[0].HasUnallocatedAdjustments {
		t.Fatalf("oauth group account must disclose unallocated cost: %#v", report.Groups[0].Accounts)
	}
}

func TestAccountFinancialListExceptionsFiltersPaginatesAndPreservesTrace(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	sub := float64(0)
	repo := &financialRepoStub{snapshot: &AccountFinancialSnapshot{GeneratedAt: now, EnabledAt: now.Add(-time.Hour), Accounts: []AccountFinancialSnapshotAccount{{ID: 1, Type: "api_key"}}, Entries: []AccountFinancialSnapshotEntry{
		{UsageLogID: 1, AccountID: 1, RequestID: "req-alpha", Model: "gpt", CreatedAt: now, RevenueCNY: 10, EvidenceID: financialInt64Ptr(3), EvidenceStatus: "unavailable", ReasonCode: "record_not_found", SubActualCost: &sub},
		{UsageLogID: 2, AccountID: 1, RequestID: "req-beta", Model: "claude", CreatedAt: now, RevenueCNY: 20, EvidenceStatus: "unavailable"},
	}}}
	list, err := NewAccountFinancialService(repo, func() time.Time { return now }).ListExceptions(context.Background(), ReviewFilter{Page: 1, PageSize: 1, Search: "alpha", EvidenceStatus: "unavailable", ReviewStatus: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ReasonCode != "record_not_found" || list.Items[0].ReviewStatus != "pending" || list.Items[0].CostTrace.SubActualCost == nil {
		t.Fatalf("list=%#v", list)
	}
}

func TestAccountFinancialListExceptionsFiltersReviewedRows(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	repo := &financialRepoStub{snapshot: &AccountFinancialSnapshot{GeneratedAt: now, EnabledAt: now.Add(-time.Hour), Accounts: []AccountFinancialSnapshotAccount{{ID: 1, Type: "api_key"}}, Entries: []AccountFinancialSnapshotEntry{
		{UsageLogID: 1, AccountID: 1, RequestID: "pending", CreatedAt: now, EvidenceStatus: "unavailable", ReasonCode: "record_not_found"},
		{UsageLogID: 2, AccountID: 1, RequestID: "reviewed", CreatedAt: now, EvidenceStatus: "confirmed_zero", ReasonCode: "record_not_found", ReviewID: financialInt64Ptr(7), ReviewCostCNY: financialFloatPtr(0)},
	}}}
	list, err := NewAccountFinancialService(repo, func() time.Time { return now }).ListExceptions(context.Background(), ReviewFilter{ReviewStatus: "reviewed"})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].UsageLogID != 2 || list.Items[0].ReviewStatus != "reviewed" {
		t.Fatalf("reviewed list=%#v", list)
	}
}

func assertFinancialAmounts(t *testing.T, got FinancialAmounts, revenue, cost, profit, margin float64) {
	t.Helper()
	if got.RevenueCNY != revenue || got.CostCNY != cost || got.ProfitCNY != profit || got.Margin == nil || math.Abs(*got.Margin-margin) > 1e-12 {
		t.Fatalf("amounts got %#v want revenue=%v cost=%v profit=%v margin=%v", got, revenue, cost, profit, margin)
	}
}
