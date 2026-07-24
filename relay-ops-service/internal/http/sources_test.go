package httpserver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/accountquality"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/d04readiness"
	"example.invalid/relay-ops-service/internal/opsmetrics"
	"example.invalid/relay-ops-service/internal/qualityreports"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestNativePricingFormatsPerMillionAndIncludesIntervals(t *testing.T) {
	input := 0.000005
	output := 0.000030
	cacheRead := 0.0000005
	cacheWrite := 0.00000625
	tierInput := 0.000010
	tierOutput := 0.000045
	reader := pricingReader{
		groups: []sub2api.Group{{ID: 3, Name: "GPT-Pro", Platform: "openai", Status: "active"}},
		channels: []sub2api.Channel{{
			ID: 7, Name: "Neko", Status: "active", GroupIDs: []int64{3},
			ModelPricing: []sub2api.ChannelModelPrice{{
				Models: []string{"gpt-5.6-sol"}, InputPrice: &input, OutputPrice: &output,
				CacheReadPrice: &cacheRead, CacheWritePrice: &cacheWrite,
				Intervals: []sub2api.ChannelModelPriceInterval{{
					MinTokens: 272000, InputPrice: &tierInput, OutputPrice: &tierOutput,
				}},
			}},
		}},
	}
	source := NativePricingSource{Reader: reader, Clock: func() time.Time { return time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC) }}

	groups, err := source.PublicPricing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Models) != 2 {
		t.Fatalf("groups = %#v", groups)
	}
	base := groups[0].Models[0]
	if base.Input != "5.00" || base.Output != "30.00" || base.CacheRead != "0.50" || base.CacheWrite != "6.25" {
		t.Fatalf("base = %#v", base)
	}
	tier := groups[0].Models[1]
	if tier.Tier != ">272k" || tier.Input != "10.00" || tier.Output != "45.00" {
		t.Fatalf("tier = %#v", tier)
	}
}

func TestDatabaseOpsSourceIncludesStoredQualityReports(t *testing.T) {
	repository := qualityOpsRepository{reports: []qualityreports.Report{{
		ReportID: "fast-1", ReportHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		UpstreamName: "Candidate", Status: "needs_evidence", QualityScore: 85, TotalScore: 85,
		Direct: "6/6", Gateway: "unknown", Models: "3 selected", Pricing: "unknown", Capacity: "unknown",
	}}}
	source := DatabaseOpsSource{
		Repository: repository, Quality: repository,
		Native: opsNativeReader{accounts: []sub2api.Account{{ID: 10, Name: "XM PLUS", Status: "active", Schedulable: true, GroupIDs: []int64{6}}}},
	}

	view, err := source.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(view.QualityReports) != 1 || view.QualityReports[0].ReportID != "fast-1" || view.QualityReports[0].QualityScore != 85 {
		t.Fatalf("quality reports = %#v", view.QualityReports)
	}
}

func TestDatabaseOpsSourceProjectsD04ReadinessAndFailsStaleClosed(t *testing.T) {
	t.Parallel()

	evaluatedAt := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	balance := 9.99
	native := opsNativeReader{
		accounts: []sub2api.Account{{ID: 7, Name: "Account", Platform: "openai", Status: "active", Schedulable: true, GroupIDs: []int64{2}}},
		groups:   []sub2api.Group{{ID: 2, Name: "GPT-Pro", Status: "active"}},
	}
	live, err := (d04readiness.Collector{Accounts: native, Clock: func() time.Time { return evaluatedAt }}).Collect(context.Background(), d04readiness.Inputs{SnapshotID: "live"})
	if err != nil {
		t.Fatal(err)
	}
	source := DatabaseOpsSource{
		Repository: qualityOpsRepository{},
		Native:     native,
		Readiness: staticReadinessSource{result: d04readiness.Result{
			PolicyID: "D04-LIGHTWEIGHT-LAUNCH-v3", SnapshotID: "snapshot-1",
			AccountSetSHA256: live.UpstreamDiscovery.AccountSetSHA256,
			EvaluatedAt:      evaluatedAt, Decision: "go",
			BlockingReasons: []string{"upstream_balance_below_minimum"},
			Upstreams: []d04readiness.ResultUpstream{{
				AccountID: 7, DisplayName: "Account", GroupIDs: []int64{2}, Status: "active", Schedulable: true,
				RuntimeAvailable: true, BalanceUSD: &balance, Decision: "no_go",
				BlockingReasons: []string{"upstream_balance_below_minimum"},
			}},
		}},
		Clock: func() time.Time { return evaluatedAt.Add(21 * time.Minute) },
	}

	view, err := source.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if view.D04LaunchReadiness.Decision != "NO-GO" || !view.D04LaunchReadiness.Stale || len(view.D04LaunchReadiness.Upstreams) != 1 {
		t.Fatalf("D04 readiness = %#v", view.D04LaunchReadiness)
	}
	if got := view.D04LaunchReadiness.Upstreams[0].Blockers; got != "余额低于最低门槛" {
		t.Fatalf("blockers = %q", got)
	}
	if got := view.D04LaunchReadiness.Upstreams[0].BlockerCodes; got != "upstream_balance_below_minimum" {
		t.Fatalf("blocker codes = %q", got)
	}
	if got := view.D04LaunchReadiness.Blockers; got != "余额低于最低门槛；门禁结果已过期" {
		t.Fatalf("top-level blockers = %q", got)
	}
}

func TestDatabaseOpsSourceUsesLiveAccountsWhenReadinessSetChanged(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 22, 16, 30, 0, 0, time.UTC)
	native := opsNativeReader{
		accounts: []sub2api.Account{
			{ID: 11, Name: "XM PRO", Platform: "openai", Status: "active", Schedulable: true, GroupIDs: []int64{2}},
			{ID: 7, Name: "disabled old", Platform: "openai", Status: "disabled", Schedulable: false, GroupIDs: []int64{2}},
			{ID: 10, Name: "XM PLUS", Platform: "openai", Status: "active", Schedulable: true, GroupIDs: []int64{6}},
			{ID: 12, Name: "paused", Platform: "openai", Status: "active", Schedulable: false, GroupIDs: []int64{6}},
		},
		groups: []sub2api.Group{{ID: 2, Name: "GPT-Pro", Status: "active"}, {ID: 6, Name: "GPT-Plus", Status: "active"}},
	}
	expected, err := (d04readiness.Collector{Accounts: native, Clock: func() time.Time { return now }}).Collect(context.Background(), d04readiness.Inputs{SnapshotID: "live"})
	if err != nil {
		t.Fatal(err)
	}
	source := DatabaseOpsSource{
		Repository: qualityOpsRepository{}, Native: native,
		Readiness: staticReadinessSource{result: d04readiness.Result{
			PolicyID: "D04-LIGHTWEIGHT-LAUNCH-v3", SnapshotID: "old",
			AccountSetSHA256: strings.Repeat("a", 64), EvaluatedAt: now.Add(-time.Minute), Decision: "go",
			Upstreams: []d04readiness.ResultUpstream{{AccountID: 7, DisplayName: "old", GroupIDs: []int64{2}, Status: "active", Schedulable: true, RuntimeAvailable: true, Decision: "go"}},
		}},
		Clock: func() time.Time { return now },
	}

	view, err := source.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	readiness := view.D04LaunchReadiness
	if readiness.Decision != "NO-GO" || readiness.AccountSetSHA256 != expected.UpstreamDiscovery.AccountSetSHA256 || len(readiness.Upstreams) != 2 {
		t.Fatalf("readiness = %#v", readiness)
	}
	if readiness.Upstreams[0].AccountID != "10" || readiness.Upstreams[0].DisplayName != "XM PLUS" || readiness.Upstreams[0].Groups != "GPT-Plus" || readiness.Upstreams[1].AccountID != "11" || readiness.Upstreams[1].Groups != "GPT-Pro" {
		t.Fatalf("live upstreams = %#v", readiness.Upstreams)
	}
	if strings.Contains(readiness.Blockers, "old") || !strings.Contains(readiness.BlockerCodes, "upstream_account_set_changed") {
		t.Fatalf("blockers=%q codes=%q", readiness.Blockers, readiness.BlockerCodes)
	}
	if view.RefreshedAt != "2026-07-22 16:30 UTC" {
		t.Fatalf("refreshed_at=%q", view.RefreshedAt)
	}
}

func TestDatabaseOpsSourceFailsClosedWhenSub2APIAccountsCannotBeRead(t *testing.T) {
	t.Parallel()
	source := DatabaseOpsSource{Repository: qualityOpsRepository{}, Native: opsNativeReader{err: errors.New("unavailable")}}
	if _, err := source.Snapshot(context.Background()); err == nil || !strings.Contains(err.Error(), "list active Sub2API accounts") {
		t.Fatalf("error=%v", err)
	}
}

func TestDatabaseOpsSourceProjectsAccountQualityResult(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	accountSetHash, hashErr := accountquality.CanonicalAccountSetSHA256([]int64{10})
	if hashErr != nil {
		t.Fatal(hashErr)
	}
	source := DatabaseOpsSource{
		Repository: qualityOpsRepository{},
		Native: opsNativeReader{accounts: []sub2api.Account{{
			ID: 10, Status: "active", Schedulable: true,
		}}},
		AccountQuality: staticAccountQualitySource{result: accountquality.Result{
			SchemaVersion: 1, SnapshotID: "ACCOUNT-QUALITY-1", ObservedAt: now,
			AccountSetSHA256: accountSetHash,
			Accounts: []accountquality.Account{{
				AccountID: 10, ModelID: "gpt-5.6-sol", RateMultiplier: float64Pointer(0.05),
				SampleCount: 4, SuccessCount: 3, SuccessRate: 0.75, LastResult: "passed", LastObservedAt: now,
			}},
		}},
		Clock: func() time.Time { return now },
	}

	view, err := source.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !view.AccountQuality.Available || len(view.AccountQuality.Accounts) != 1 || view.AccountQuality.Accounts[0].AccountID != "10" {
		t.Fatalf("account quality = %#v", view.AccountQuality)
	}
}

func TestDatabaseOpsSourceRejectsAccountQualityForDifferentActiveAccountSet(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	source := DatabaseOpsSource{
		Repository: qualityOpsRepository{},
		Native: opsNativeReader{accounts: []sub2api.Account{{
			ID: 10, Status: "active", Schedulable: true,
		}}},
		AccountQuality: staticAccountQualitySource{result: accountquality.Result{
			SchemaVersion: 1, SnapshotID: "ACCOUNT-QUALITY-OLD", ObservedAt: now,
			AccountSetSHA256: "95bce61a71a78185f4b6f8f25fc6986108043727fe9d9c19dbe44b0081ef928a",
			Accounts: []accountquality.Account{{
				AccountID: 10, ModelID: "gpt-5.6-sol", RateMultiplier: float64Pointer(0.05),
				SampleCount: 1, SuccessCount: 1, SuccessRate: 1, LastResult: "passed", LastObservedAt: now,
			}},
		}},
		Clock: func() time.Time { return now },
	}

	view, err := source.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if view.AccountQuality.Available || !view.AccountQuality.AccountSetMismatch || len(view.AccountQuality.Accounts) != 0 {
		t.Fatalf("mismatched quality = %#v", view.AccountQuality)
	}
}

func TestDatabaseOpsSourceProjectsSiteRuntime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 23, 6, 0, 0, 0, time.UTC)
	native := runtimeNativeReader{
		groups: []sub2api.Group{
			{ID: 2, Name: "Public Pro", Status: "active"},
			{ID: 4, Name: "Public Plus", Status: "active"},
		},
		accounts: []sub2api.Account{
			{ID: 7, Name: "Disabled", Status: "disabled", Schedulable: true},
			{ID: 8, Name: "Paused", Status: "active", Schedulable: false},
			{ID: 10, Name: "Current", Status: "active", Schedulable: true, GroupIDs: []int64{2, 4}},
		},
		ops: map[runtimeObject]sub2api.OpsSnapshot{
			{groupID: 2}:    runtimeOpsSnapshot(20),
			{groupID: 4}:    runtimeOpsSnapshot(10),
			{accountID: 10}: runtimeOpsSnapshot(20),
		},
	}
	source := DatabaseOpsSource{
		Repository: qualityOpsRepository{}, Native: native,
		AccountQuality: staticAccountQualitySource{result: accountquality.Result{
			SchemaVersion: 1, SnapshotID: "quality", ObservedAt: now,
			AccountSetSHA256: "41164c427f4cf681cc1b50eaf1a175e3574a1d015c6c90fbdaf77aea39d24086",
		}},
		Clock: func() time.Time { return now },
	}

	view, err := source.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(view.SiteRuntime.Groups) != 2 || view.SiteRuntime.Groups[1].Status != opsmetrics.StatusSampleInsufficient {
		t.Fatalf("site runtime groups = %#v", view.SiteRuntime.Groups)
	}
	if len(view.SiteRuntime.Accounts) != 1 || view.SiteRuntime.Accounts[0].ID != 10 || view.SiteRuntime.Accounts[0].Status != opsmetrics.StatusOK {
		t.Fatalf("site runtime accounts = %#v", view.SiteRuntime.Accounts)
	}
	if view.D04LaunchReadiness.Available != true || view.AccountQuality.Available != true {
		t.Fatalf("existing projections = readiness:%#v quality:%#v", view.D04LaunchReadiness, view.AccountQuality)
	}
}

type staticReadinessSource struct {
	result d04readiness.Result
	err    error
}

func (s staticReadinessSource) Read() (d04readiness.Result, error) { return s.result, s.err }

type staticAccountQualitySource struct {
	result accountquality.Result
	err    error
}

func (s staticAccountQualitySource) Read(time.Time) (accountquality.Result, error) {
	return s.result, s.err
}

type qualityOpsRepository struct{ reports []qualityreports.Report }

func (qualityOpsRepository) ListCandidates(context.Context) ([]candidates.Candidate, error) {
	return nil, nil
}
func (qualityOpsRepository) ListPublicGroupNames(context.Context) ([]string, error) { return nil, nil }
func (r qualityOpsRepository) ListQualityReports(context.Context, int) ([]qualityreports.Report, error) {
	return r.reports, nil
}

type opsNativeReader struct {
	accounts []sub2api.Account
	groups   []sub2api.Group
	err      error
}

func (r opsNativeReader) ListAccounts(context.Context) ([]sub2api.Account, error) {
	return r.accounts, r.err
}

func (r opsNativeReader) ListGroups(context.Context) ([]sub2api.Group, error) {
	return r.groups, r.err
}

func (opsNativeReader) GetOpsSnapshot(context.Context, sub2api.OpsQuery) (sub2api.OpsSnapshot, error) {
	return sub2api.OpsSnapshot{}, nil
}

type runtimeObject struct {
	groupID   int64
	accountID int64
}

type runtimeNativeReader struct {
	groups   []sub2api.Group
	accounts []sub2api.Account
	ops      map[runtimeObject]sub2api.OpsSnapshot
}

func (r runtimeNativeReader) ListGroups(context.Context) ([]sub2api.Group, error) {
	return r.groups, nil
}

func (r runtimeNativeReader) ListAccounts(context.Context) ([]sub2api.Account, error) {
	return r.accounts, nil
}

func (r runtimeNativeReader) GetOpsSnapshot(_ context.Context, query sub2api.OpsQuery) (sub2api.OpsSnapshot, error) {
	return r.ops[runtimeObject{groupID: query.GroupID, accountID: query.AccountID}], nil
}

func runtimeOpsSnapshot(requestCount int64) sub2api.OpsSnapshot {
	return sub2api.OpsSnapshot{Overview: sub2api.OpsOverview{RequestCountTotal: requestCount, SuccessCount: requestCount, SLA: 99, TTFT: sub2api.Percentiles{P95MS: 1200}, Duration: sub2api.Percentiles{P95MS: 2400}}}
}

func float64Pointer(value float64) *float64 { return &value }

type pricingReader struct {
	channels []sub2api.Channel
	groups   []sub2api.Group
}

func (r pricingReader) ListChannels(context.Context) ([]sub2api.Channel, error) {
	return r.channels, nil
}
func (r pricingReader) ListGroups(context.Context) ([]sub2api.Group, error) { return r.groups, nil }
func (pricingReader) ListChannelMonitors(context.Context) ([]sub2api.ChannelMonitor, error) {
	return nil, nil
}
func (pricingReader) GetChannelMonitorHistory(context.Context, int64, string, int) ([]sub2api.MonitorHistory, error) {
	return nil, nil
}
func (pricingReader) GetOpsSnapshot(context.Context, sub2api.OpsQuery) (sub2api.OpsSnapshot, error) {
	return sub2api.OpsSnapshot{}, nil
}
func (pricingReader) GetUsageStats(context.Context, sub2api.UsageQuery) (sub2api.UsageStats, error) {
	return sub2api.UsageStats{}, nil
}
