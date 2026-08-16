package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

type financialRepoStub struct {
	snapshot          *AccountFinancialSnapshot
	readSnapshotCalls int
}

func (r *financialRepoStub) ReadSnapshot(context.Context, AccountFinancialSnapshotQuery) (*AccountFinancialSnapshot, error) {
	r.readSnapshotCalls++
	return r.snapshot, nil
}
func (r *financialRepoStub) CreateReview(context.Context, UsageCostReviewInput) (*UsageCostReviewResult, error) {
	panic("unexpected CreateReview")
}
func (r *financialRepoStub) FreezeReviewFilter(context.Context, ReviewFilter) (int64, error) {
	panic("unexpected FreezeReviewFilter")
}
func (r *financialRepoStub) ReviewFiltered(context.Context, ReviewFilteredInput) (*ReviewFilteredResult, error) {
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

type financialUsageReaderStub struct {
	snapshot *AccountFinancialUsageSnapshot
	err      error
	from     time.Time
	to       time.Time
	calls    int
}

func (r *financialUsageReaderStub) ReadAccountFinancialUsage(_ context.Context, from, to time.Time) (*AccountFinancialUsageSnapshot, error) {
	r.calls++
	r.from, r.to = from, to
	return r.snapshot, r.err
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

func TestAccountFinancialReportFoldsNativePairsAndConservesTotals(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	group10, group20, group30 := int64(10), int64(20), int64(30)
	reader := &financialUsageReaderStub{snapshot: &AccountFinancialUsageSnapshot{
		UserBalanceCNY: 90,
		Accounts: []AccountFinancialUsageAccount{
			{ID: 1, Name: "shared", Type: "api_key", Platform: "sub", Active: true},
			{ID: 2, Name: "zero", Type: "api_key", Platform: "sub", Active: true},
			{ID: 3, Name: "archived-account", Type: "api_key", Active: false},
		},
		Groups: []AccountFinancialUsageGroup{
			{ID: group10, Name: "Pro", Active: true},
			{ID: 11, Name: "Empty", Active: true},
			{ID: group20, Name: "Soft deleted", Active: false},
		},
		Rows: []AccountFinancialUsageRow{
			{GroupID: &group10, GroupName: "Pro", AccountID: 1, AccountName: "shared", AccountType: "api_key", AccountPlatform: "sub", Requests: 2, Tokens: 100, Cost: 1, UserCost: 3},
			{GroupID: &group20, GroupName: "Soft deleted", AccountID: 1, AccountName: "shared", AccountType: "api_key", AccountPlatform: "sub", Requests: 3, Tokens: 200, Cost: 2, UserCost: 4},
			{GroupID: &group30, AccountID: 4, Requests: 4, Tokens: 400, Cost: 4, UserCost: 5},
			{AccountID: 3, Requests: 1, Tokens: 50, Cost: .5, UserCost: 1},
		},
	}}
	repo := &financialRepoStub{}
	report, err := NewAccountFinancialService(repo, reader, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 || !reader.from.Equal(beijingTime(t, "2026-08-13 00:00")) || !reader.to.Equal(now) {
		t.Fatalf("reader call=%d from=%v to=%v", reader.calls, reader.from, reader.to)
	}
	if report.GeneratedAt != now || report.Currency != "USD" || report.UserBalanceCNY != 90 {
		t.Fatalf("report metadata=%#v", report)
	}
	assertNativeAmounts(t, report.Summary, 10, 750, 7.5, 13, 5.5/13)
	if len(report.Accounts) != 4 || report.Accounts[0].ID != 1 || report.Accounts[1].ID != 2 || !report.Accounts[2].Historical || !report.Accounts[3].Historical {
		t.Fatalf("account order/history=%#v", report.Accounts)
	}
	assertNativeAmounts(t, report.Accounts[0].Amounts, 5, 300, 3, 7, 4.0/7.0)
	assertNativeAmounts(t, report.Accounts[1].Amounts, 0, 0, 0, 0, 0)
	if report.Accounts[1].Amounts.Margin != nil {
		t.Fatal("active zero account margin must be nil")
	}
	if len(report.Groups) != 5 || report.Groups[0].ID != group10 || report.Groups[1].ID != 11 || !report.Groups[2].Historical || !report.Groups[3].Historical || !report.Groups[4].Unassigned {
		t.Fatalf("group order/history=%#v", report.Groups)
	}
	assertNativeAmounts(t, report.Groups[0].Amounts, 2, 100, 1, 3, 2.0/3.0)
	assertNativeAmounts(t, report.Groups[1].Amounts, 0, 0, 0, 0, 0)
	assertNativeAmounts(t, report.Groups[2].Amounts, 3, 200, 2, 4, .5)
	assertNativeAmounts(t, report.Groups[3].Amounts, 4, 400, 4, 5, .2)
	assertNativeAmounts(t, report.Groups[4].Amounts, 1, 50, .5, 1, .5)
	assertNativeAmounts(t, report.Groups[0].Accounts[0].Amounts, 2, 100, 1, 3, 2.0/3.0)
	assertNativeAmounts(t, report.Groups[2].Accounts[0].Amounts, 3, 200, 2, 4, .5)
	assertNativeAmounts(t, report.Groups[3].Accounts[0].Amounts, 4, 400, 4, 5, .2)
	assertNativeAmounts(t, report.Groups[4].Accounts[0].Amounts, 1, 50, .5, 1, .5)
	if report.Groups[3].Name != "分组 #30" || report.Accounts[2].Name != "账号 #4" {
		t.Fatalf("stable fallbacks group=%q account=%q", report.Groups[3].Name, report.Accounts[2].Name)
	}
	if report.Accounts[0].Amounts.Revenue != report.Accounts[0].Amounts.UserCost || report.Accounts[0].Amounts.Expense != report.Accounts[0].Amounts.Cost {
		t.Fatalf("deprecated aliases diverged: %#v", report.Accounts[0].Amounts)
	}
	if repo.readSnapshotCalls != 0 {
		t.Fatalf("GetReport must not read legacy snapshot: calls=%d", repo.readSnapshotCalls)
	}
}

func TestAccountFinancialReportPreservesActiveZeroRowsAndHistoricalUsage(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	historicalAccount, historicalGroup := int64(9), int64(99)
	reader := &financialUsageReaderStub{snapshot: &AccountFinancialUsageSnapshot{
		Accounts: []AccountFinancialUsageAccount{{ID: 1, Active: true}, {ID: historicalAccount, Active: false}},
		Groups:   []AccountFinancialUsageGroup{{ID: 10, Active: true}, {ID: historicalGroup, Active: false}},
		Rows:     []AccountFinancialUsageRow{{GroupID: &historicalGroup, AccountID: historicalAccount, Requests: 1, Tokens: 2, UserCost: 3}},
	}}
	report, err := NewAccountFinancialService(&financialRepoStub{}, reader, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRange7D)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Accounts) != 2 || report.Accounts[0].ID != 1 || report.Accounts[0].Historical || !report.Accounts[1].Historical {
		t.Fatalf("active zero/historical accounts=%#v", report.Accounts)
	}
	if report.Accounts[0].Amounts.Requests != 0 || report.Accounts[0].Amounts.Margin != nil {
		t.Fatalf("active zero account must remain visible with nil margin=%#v", report.Accounts[0])
	}
	if len(report.Groups) != 2 || report.Groups[0].ID != 10 || report.Groups[0].Historical || report.Groups[1].ID != historicalGroup || !report.Groups[1].Historical {
		t.Fatalf("active zero/historical groups=%#v", report.Groups)
	}
	if len(report.Groups[1].Accounts) != 1 || report.Groups[1].Accounts[0].ID != historicalAccount {
		t.Fatalf("historical pair must retain scoped account=%#v", report.Groups[1])
	}
}

func TestAccountFinancialReportUsesNullMarginForZeroUserCost(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	reader := &financialUsageReaderStub{snapshot: &AccountFinancialUsageSnapshot{Rows: []AccountFinancialUsageRow{{AccountID: 1, Requests: 1, Tokens: 2, Cost: 2, UserCost: 0}}}}
	report, err := NewAccountFinancialService(&financialRepoStub{}, reader, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Margin != nil || report.Summary.Profit != -2 || report.Summary.Revenue != 0 || report.Summary.Expense != 2 {
		t.Fatalf("zero user cost must produce nil margin and aliases=%#v", report.Summary)
	}
}

func TestAccountFinancialReportProbeUnavailableForSuccessfulUserOnlySnapshot(t *testing.T) {
	now := beijingTime(t, "2026-08-16 12:00")
	groupID := int64(10)
	reader := &financialUsageReaderStub{snapshot: &AccountFinancialUsageSnapshot{
		Accounts: []AccountFinancialUsageAccount{{ID: 1, Active: true}},
		Groups:   []AccountFinancialUsageGroup{{ID: groupID, Active: true}},
		Rows:     []AccountFinancialUsageRow{{GroupID: &groupID, AccountID: 1, Requests: 2, Tokens: 3, Cost: 4, UserCost: 6}},
	}}
	report, err := NewAccountFinancialService(&financialRepoStub{}, reader, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	for name, amounts := range map[string]FinancialAmounts{
		"summary": report.Summary, "account": report.Accounts[0].Amounts, "group": report.Groups[0].Amounts, "group account": report.Groups[0].Accounts[0].Amounts,
	} {
		assertProbeAmounts(t, name, amounts, 0, 0, "0", "unavailable")
	}
	assertNativeAmounts(t, report.Summary, 2, 3, 4, 6, 1.0/3.0)
	if report.ProbeDataError || report.ProbeErrorCode != nil {
		t.Fatalf("successful empty probe query marked failed: %#v", report)
	}
}

func TestAccountFinancialReportProbeConservesImmutableGroupsAndExactDecimals(t *testing.T) {
	now := beijingTime(t, "2026-08-16 12:00")
	group10, group20 := int64(10), int64(20)
	costA := decimal.RequireFromString("0.1000000001")
	costB := decimal.RequireFromString("0.2000000002")
	costC := decimal.RequireFromString("0.3000000003")
	reader := &financialUsageReaderStub{snapshot: &AccountFinancialUsageSnapshot{
		Accounts: []AccountFinancialUsageAccount{{ID: 1, Name: "shared", Active: true}},
		Groups:   []AccountFinancialUsageGroup{{ID: group10, Name: "Current", Active: true}},
		Rows:     []AccountFinancialUsageRow{{GroupID: &group10, AccountID: 1, Requests: 1, Tokens: 2, Cost: 3, UserCost: 5}},
		ProbeRows: []AccountProbeCostAggregate{
			{GroupID: &group10, AccountID: 1, ProbeRequests: 1, ProbeTokens: 10, ProbeCost: &costA},
			{GroupID: &group20, AccountID: 1, ProbeRequests: 2, ProbeTokens: 20, ProbeCost: &costB},
			{AccountID: 1, ProbeRequests: 3, ProbeTokens: 30, ProbeCost: &costC},
		},
	}}
	report, err := NewAccountFinancialService(&financialRepoStub{}, reader, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	assertProbeAmounts(t, "summary", report.Summary, 6, 60, "0.6000000006", "confirmed")
	assertProbeAmounts(t, "account", report.Accounts[0].Amounts, 6, 60, "0.6000000006", "confirmed")
	assertProbeAmounts(t, "current group", report.Groups[0].Amounts, 1, 10, "0.1000000001", "confirmed")
	assertProbeAmounts(t, "historical group", report.Groups[1].Amounts, 2, 20, "0.2000000002", "confirmed")
	assertProbeAmounts(t, "unassigned", report.Groups[2].Amounts, 3, 30, "0.3000000003", "confirmed")
	if !report.Groups[1].Historical || !report.Groups[2].Unassigned {
		t.Fatalf("probe snapshot dimensions lost: %#v", report.Groups)
	}
	assertNativeAmounts(t, report.Summary, 1, 2, 3, 5, .4)
}

func TestAccountFinancialReportProbePartialNullsCostWithoutChangingNativeAmounts(t *testing.T) {
	now := beijingTime(t, "2026-08-16 12:00")
	complete := decimal.RequireFromString("0.25")
	reader := &financialUsageReaderStub{snapshot: &AccountFinancialUsageSnapshot{
		Rows: []AccountFinancialUsageRow{{AccountID: 1, Requests: 4, Tokens: 8, Cost: 2, UserCost: 3}},
		ProbeRows: []AccountProbeCostAggregate{
			{AccountID: 1, ProbeRequests: 1, ProbeTokens: 10, ProbeCost: &complete},
			{AccountID: 1, ProbeRequests: 2, ProbeTokens: 20, HasIncompleteCost: true},
		},
	}}
	report, err := NewAccountFinancialService(&financialRepoStub{}, reader, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	assertProbeAmounts(t, "summary", report.Summary, 3, 30, "", "incomplete")
	assertNativeAmounts(t, report.Summary, 4, 8, 2, 3, 1.0/3.0)
}

func TestAccountFinancialReportProbeFailureReturnsNativeAmountsAndNullProbeFields(t *testing.T) {
	now := beijingTime(t, "2026-08-16 12:00")
	code := "probe_aggregate_unavailable"
	reader := &financialUsageReaderStub{snapshot: &AccountFinancialUsageSnapshot{
		Rows:           []AccountFinancialUsageRow{{AccountID: 1, Requests: 4, Tokens: 8, Cost: 2, UserCost: 3}},
		ProbeDataError: true, ProbeErrorCode: &code,
	}}
	report, err := NewAccountFinancialService(&financialRepoStub{}, reader, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ProbeDataError || report.ProbeErrorCode == nil || *report.ProbeErrorCode != code {
		t.Fatalf("probe failure metadata=%#v", report)
	}
	if report.Summary.ProbeRequests != nil || report.Summary.ProbeTokens != nil || report.Summary.ProbeCost != nil || report.Summary.ProbeCostStatus != nil {
		t.Fatalf("probe failure was zero-masked: %#v", report.Summary)
	}
	assertNativeAmounts(t, report.Summary, 4, 8, 2, 3, 1.0/3.0)
}

func TestAccountFinancialReportBuildsToday24H7D31DHalfOpenWindows(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:34")
	wants := []struct {
		r    AccountFinancialRange
		from time.Time
	}{
		{AccountFinancialRangeToday, beijingTime(t, "2026-08-13 00:00")},
		{AccountFinancialRange24H, beijingTime(t, "2026-08-12 12:34")},
		{AccountFinancialRange7D, beijingTime(t, "2026-08-07 00:00")},
		{AccountFinancialRange31D, beijingTime(t, "2026-07-14 00:00")},
	}
	for _, want := range wants {
		t.Run(string(want.r), func(t *testing.T) {
			reader := &financialUsageReaderStub{snapshot: &AccountFinancialUsageSnapshot{}}
			report, err := NewAccountFinancialService(&financialRepoStub{}, reader, func() time.Time { return now }).GetReport(context.Background(), want.r)
			if err != nil {
				t.Fatal(err)
			}
			if report.Range != want.r || !reader.from.Equal(want.from) || !reader.to.Equal(now) {
				t.Fatalf("range=%q from=%v to=%v", report.Range, reader.from, reader.to)
			}
		})
	}
}

func TestAccountFinancialReportFailsClosedWithoutUsageReader(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	_, err := NewAccountFinancialService(&financialRepoStub{}, nil, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if err == nil {
		t.Fatal("nil usage reader must fail closed")
	}
}

func TestAccountFinancialReportPropagatesUsageReaderError(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	want := errors.New("usage reader unavailable")
	_, err := NewAccountFinancialService(&financialRepoStub{}, &financialUsageReaderStub{err: want}, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want=%v", err, want)
	}
}

func TestAccountFinancialReportDoesNotReadLegacySnapshot(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	repo := &financialRepoStub{snapshot: &AccountFinancialSnapshot{GeneratedAt: now}}
	reader := &financialUsageReaderStub{snapshot: &AccountFinancialUsageSnapshot{}}
	if _, err := NewAccountFinancialService(repo, reader, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday); err != nil {
		t.Fatal(err)
	}
	if repo.readSnapshotCalls != 0 {
		t.Fatalf("legacy snapshot was read %d times", repo.readSnapshotCalls)
	}
}

func assertNativeAmounts(t *testing.T, got FinancialAmounts, requests, tokens int64, cost, userCost, margin float64) {
	t.Helper()
	if got.Requests != requests || got.Tokens != tokens || got.Cost != cost || got.UserCost != userCost || got.Profit != userCost-cost || got.Revenue != userCost || got.Expense != cost {
		t.Fatalf("amounts got %#v want requests=%d tokens=%d cost=%v user_cost=%v", got, requests, tokens, cost, userCost)
	}
	if userCost == 0 {
		if got.Margin != nil {
			t.Fatalf("zero user_cost margin=%v", *got.Margin)
		}
		return
	}
	if got.Margin == nil || math.Abs(*got.Margin-margin) > 1e-12 {
		t.Fatalf("margin got=%v want=%v", got.Margin, margin)
	}
}

func assertProbeAmounts(t *testing.T, name string, got FinancialAmounts, requests, tokens int64, cost, status string) {
	t.Helper()
	if got.ProbeRequests == nil || *got.ProbeRequests != requests || got.ProbeTokens == nil || *got.ProbeTokens != tokens || got.ProbeCostStatus == nil || *got.ProbeCostStatus != status {
		t.Fatalf("%s probe amounts=%#v", name, got)
	}
	if cost == "" {
		if got.ProbeCost != nil {
			t.Fatalf("%s probe cost=%s want nil", name, got.ProbeCost.String())
		}
		return
	}
	if got.ProbeCost == nil || got.ProbeCost.String() != cost {
		t.Fatalf("%s probe cost=%v want %s", name, got.ProbeCost, cost)
	}
}

func TestAccountFinancialListExceptionsFiltersPaginatesAndPreservesTrace(t *testing.T) {
	now := beijingTime(t, "2026-08-13 12:00")
	sub := float64(0)
	repo := &financialRepoStub{snapshot: &AccountFinancialSnapshot{GeneratedAt: now, EnabledAt: now.Add(-time.Hour), Accounts: []AccountFinancialSnapshotAccount{{ID: 1, Type: "api_key"}}, Entries: []AccountFinancialSnapshotEntry{
		{UsageLogID: 1, AccountID: 1, RequestID: "req-alpha", Model: "gpt", CreatedAt: now, RevenueCNY: 10, EvidenceID: financialInt64Ptr(3), EvidenceStatus: "unavailable", ReasonCode: "record_not_found", SubActualCost: &sub},
		{UsageLogID: 2, AccountID: 1, RequestID: "req-beta", Model: "claude", CreatedAt: now, RevenueCNY: 20, EvidenceStatus: "unavailable"},
	}}}
	list, err := NewAccountFinancialService(repo, nil, func() time.Time { return now }).ListExceptions(context.Background(), ReviewFilter{Page: 1, PageSize: 1, Search: "alpha", EvidenceStatus: "unavailable", ReviewStatus: "pending"})
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
	list, err := NewAccountFinancialService(repo, nil, func() time.Time { return now }).ListExceptions(context.Background(), ReviewFilter{ReviewStatus: "reviewed"})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].UsageLogID != 2 || list.Items[0].ReviewStatus != "reviewed" {
		t.Fatalf("reviewed list=%#v", list)
	}
}
