package dailyreport

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestServiceSendsIdempotentDailyDigestWithoutIncidentIdentity(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 20, 1, 30, 0, 0, time.UTC)
	reader := reportReader{
		channels: []sub2api.Channel{{
			ID: 9, Status: "active", GroupIDs: []int64{2, 6},
			ModelPricing: []sub2api.ChannelModelPrice{cacheDiscountPricing("gpt-5.6-sol")},
		}},
		groups: []sub2api.Group{
			{ID: 2, Name: "GPT-Pro", Platform: "openai", Status: "active", RateMultiplier: 1},
			{ID: 6, Name: "GPT-Plus", Platform: "openai", Status: "active", RateMultiplier: 1},
			{ID: 7, Name: "private", Platform: "openai", Status: "active", IsExclusive: true},
		},
		ops: map[int64]sub2api.OpsSnapshot{
			2: {Overview: sub2api.OpsOverview{SuccessCount: 99, ErrorCountTotal: 1, RequestCountTotal: 100, SLA: 99, TTFT: sub2api.Percentiles{P95MS: 1400}, Duration: sub2api.Percentiles{P95MS: 3200}}},
			6: {Overview: sub2api.OpsOverview{SuccessCount: 198, ErrorCountTotal: 2, RequestCountTotal: 200, SLA: 99, TTFT: sub2api.Percentiles{P95MS: 1900}, Duration: sub2api.Percentiles{P95MS: 4100}}},
		},
		accounts: []sub2api.Account{{
			ID: 10, Name: "active account", Status: "active",
			Schedulable: true, GroupIDs: []int64{2, 6},
		}},
		usage: map[int64]sub2api.UsageStats{
			2: {TotalRequests: 100, TotalInputTokens: 1_000_000, TotalCacheReadTokens: 3_000_000, TotalCacheCreationTokens: 500_000, CacheMetricsPresent: true, TotalCost: 1.2, TotalActualCost: 0.12, TotalAccountCost: 0.12},
			6: {TotalRequests: 200, TotalInputTokens: 2_000_000, CacheMetricsPresent: true, TotalCost: 2.4, TotalActualCost: 0.12, TotalAccountCost: 0.12},
		},
		monitor: sub2api.AccountMonitorProjection{
			SchemaVersion: 2, ObservedAt: now.Add(-time.Minute),
			Settings: sub2api.AccountMonitorSettings{IntervalSeconds: 300},
			Accounts: []sub2api.AccountMonitorAccount{
				monitorAccount(31, "生产账号 A", 0.70, 450, 1600, 0.12),
				monitorAccount(32, "生产账号 B", 0.96, 120, 500, 0.08),
			},
		},
	}
	summary := &reportSummaryReader{value: DailyNotificationSummary{
		PublicGroups: 2, ActiveP0: 0, ActiveP1: 1, Recovered: 1,
		PricingEvents: 1, FreshCapacityGroups: 2,
		PricingSources: 2, TrackedPricingSources: 2,
	}}
	notifier := &reportNotifier{seen: map[string]bool{}}
	decisions := &reportDecisionRecorder{}
	service := reportService(reader, summary, notifier, decisions, now)

	first, err := service.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	changed := reader.ops[2]
	changed.Overview.SLA = 98
	reader.ops[2] = changed
	service.Reader = reader
	second, err := service.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.ReportDate != "2026-07-20" ||
		first.Notification != "delivered" ||
		second.Notification != "delivered" ||
		first.Groups != 2 {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	if notifier.delivered != 1 || notifier.attempts != 2 {
		t.Fatalf("delivered=%d attempts=%d", notifier.delivered, notifier.attempts)
	}
	identity := notifier.identities[0]
	if identity.Key != "daily-digest:2026-07-20" ||
		identity.Family != "daily_digest" ||
		identity.SourceKind != "daily_report" {
		t.Fatalf("identity=%#v", identity)
	}
	if notifier.messages[0].OccurrenceNo != 0 ||
		notifier.messages[0].Transition != "" {
		t.Fatalf("digest gained incident identity=%#v", notifier.messages[0])
	}
	if len(summary.ranges) != 2 {
		t.Fatalf("summary ranges=%#v", summary.ranges)
	}
	wantFrom := time.Date(2026, 7, 18, 16, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, 7, 19, 16, 0, 0, 0, time.UTC)
	if !summary.ranges[0].from.Equal(wantFrom) ||
		!summary.ranges[0].to.Equal(wantTo) {
		t.Fatalf("summary range=%#v want %s..%s", summary.ranges[0], wantFrom, wantTo)
	}
	card, err := notifier.messages[0].CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	text := string(card)
	for _, want := range []string{
		"中转站晨报｜7月20日", "一句话结论", "用户侧运行",
		"需要处理", "经营情况", "监控完整性",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("card missing %q: %s", want, text)
		}
	}
	for _, forbidden := range []string{
		"候选", "当前账号", "候选账号", "只读分析", "调整建议",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("card contains retired copy %q: %s", forbidden, text)
		}
	}
}

func TestServicePolicyDisabledRecordsSuppression(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 20, 1, 30, 0, 0, time.UTC)
	notifier := &reportNotifier{seen: map[string]bool{}}
	decisions := &reportDecisionRecorder{}
	service := reportService(
		reportReader{groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}}, ops: map[int64]sub2api.OpsSnapshot{}},
		&reportSummaryReader{},
		notifier,
		decisions,
		now,
	)
	service.Policy.Feishu.DailyDigestEnabled = false
	result, err := service.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Notification != "suppressed" || len(notifier.messages) != 0 ||
		len(decisions.records) != 1 ||
		decisions.records[0].Decision != "suppressed" ||
		decisions.records[0].Reason != "policy_disabled" {
		t.Fatalf("result=%#v messages=%#v decisions=%#v",
			result, notifier.messages, decisions.records)
	}
}

func TestServiceShadowRecordsWouldDeliverWithoutSending(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 20, 1, 30, 0, 0, time.UTC)
	notifier := &reportNotifier{seen: map[string]bool{}}
	decisions := &reportDecisionRecorder{}
	service := reportService(
		reportReader{groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}}, ops: map[int64]sub2api.OpsSnapshot{}},
		&reportSummaryReader{},
		notifier,
		decisions,
		now,
	)
	service.Policy.Mode = notificationpolicy.ModeShadow
	result, err := service.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Notification != "shadow" || len(notifier.messages) != 0 ||
		len(decisions.records) != 1 ||
		decisions.records[0].Decision != "shadow_would_deliver" ||
		decisions.records[0].Reason != "daily-digest:2026-07-20" {
		t.Fatalf("result=%#v messages=%#v decisions=%#v",
			result, notifier.messages, decisions.records)
	}
}

func TestServiceRendersZeroQualityCountsFromEmptyMonitorProjection(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 20, 1, 30, 0, 0, time.UTC)
	reader := reportReader{
		groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}},
		accounts: []sub2api.Account{{
			ID: 10, Status: "active", Schedulable: true,
		}},
		ops: map[int64]sub2api.OpsSnapshot{},
	}
	notifier := &reportNotifier{seen: map[string]bool{}}
	service := reportService(
		reader,
		&reportSummaryReader{value: DailyNotificationSummary{
			PublicGroups: 1, FreshCapacityGroups: 1,
		}},
		notifier,
		&reportDecisionRecorder{},
		now,
	)
	if _, err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	text := notifier.messages[0].RenderedText()
	if !strings.Contains(text, "0 个稳定｜0 个降级｜0 个不可用") {
		t.Fatalf("quality layer not rendered from monitor projection: %s", text)
	}
}

func TestServiceSendsDigestWithUnavailableMarkWhenMonitorReadFails(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 20, 1, 30, 0, 0, time.UTC)
	reader := failingMonitorReader{reportReader: reportReader{
		groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}},
		ops:    map[int64]sub2api.OpsSnapshot{},
	}}
	notifier := &reportNotifier{seen: map[string]bool{}}
	service := reportService(
		reader,
		&reportSummaryReader{value: DailyNotificationSummary{PublicGroups: 1}},
		notifier,
		&reportDecisionRecorder{},
		now,
	)
	if _, err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(notifier.messages))
	}
	text := notifier.messages[0].RenderedText()
	if !strings.Contains(text, "数据不可用") ||
		!strings.Contains(text, "账号监控数据不可用") ||
		!strings.Contains(text, "监控完整性") {
		t.Fatalf("fail-safe digest missing unavailable mark: %s", text)
	}
}

func TestServiceRendersHealthDigestFromMonitorProjection(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 25, 7, 30, 0, 0, time.UTC)
	reader := reportReader{
		groups: []sub2api.Group{{ID: 3, Name: "GPT-Pro", Status: "active"}},
		ops:    map[int64]sub2api.OpsSnapshot{},
		monitor: sub2api.AccountMonitorProjection{
			SchemaVersion: 2, ObservedAt: now.Add(-time.Minute),
			Settings: sub2api.AccountMonitorSettings{IntervalSeconds: 300},
			Accounts: []sub2api.AccountMonitorAccount{
				monitorAccount(11, "账号 A", 0.70, 450, 1600, 0.12),
				monitorAccount(12, "账号 B", 0.96, 120, 500, 0.08),
			},
		},
	}
	notifier := &reportNotifier{seen: map[string]bool{}}
	service := reportService(
		reader,
		&reportSummaryReader{value: DailyNotificationSummary{
			PublicGroups: 1, FreshCapacityGroups: 1,
		}},
		notifier,
		&reportDecisionRecorder{},
		now,
	)
	if _, err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	text := notifier.messages[0].RenderedText()
	for _, required := range []string{
		"1 个稳定｜1 个降级｜0 个不可用",
		"需要处理 · 1",
		"注意｜账号 A",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("native projection missing %q: %s", required, text)
		}
	}
	for _, forbidden := range []string{"调整建议", "候选账号", "账号 11", "账号 12"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("health digest contains forbidden detail %q: %s", forbidden, text)
		}
	}
}

func TestCacheReportLineMarksUnconfirmedAndPricingBlockers(t *testing.T) {
	t.Parallel()
	ready := cacheReportLine(sub2api.UsageStats{}, true, 1, 1, nil)
	if !strings.Contains(ready, "缓存统计不可确认") ||
		!strings.Contains(ready, "缓存优惠 1/1 模型") {
		t.Fatalf("ready line = %q", ready)
	}
	blocked := cacheReportLine(
		sub2api.UsageStats{CacheMetricsPresent: true},
		false,
		1,
		0,
		[]string{"cache_read_price_missing:group=2:model=gpt-5.6-sol:tier=base"},
	)
	if !strings.Contains(blocked, "缓存优惠未就绪") ||
		!strings.Contains(blocked, "cache_read_price_missing") {
		t.Fatalf("blocked line = %q", blocked)
	}
}

var _ sub2api.AccountMonitorReader = reportReader{}

type reportReader struct {
	channels []sub2api.Channel
	groups   []sub2api.Group
	accounts []sub2api.Account
	ops      map[int64]sub2api.OpsSnapshot
	usage    map[int64]sub2api.UsageStats
	monitor  sub2api.AccountMonitorProjection
}

func (reader reportReader) ListChannels(context.Context) ([]sub2api.Channel, error) {
	return reader.channels, nil
}

func (reader reportReader) ListGroups(context.Context) ([]sub2api.Group, error) {
	return reader.groups, nil
}

func (reader reportReader) ListAccounts(context.Context) ([]sub2api.Account, error) {
	return reader.accounts, nil
}

func (reader reportReader) ListChannelMonitors(context.Context) ([]sub2api.ChannelMonitor, error) {
	return nil, nil
}

func (reader reportReader) GetChannelMonitorHistory(
	context.Context,
	int64,
	string,
	int,
) ([]sub2api.MonitorHistory, error) {
	return nil, nil
}

func (reader reportReader) GetOpsSnapshot(
	_ context.Context,
	query sub2api.OpsQuery,
) (sub2api.OpsSnapshot, error) {
	return reader.ops[query.GroupID], nil
}

func (reader reportReader) GetUsageStats(
	_ context.Context,
	query sub2api.UsageQuery,
) (sub2api.UsageStats, error) {
	return reader.usage[query.GroupID], nil
}

func (reader reportReader) ListAccountMonitors(
	context.Context,
) (sub2api.AccountMonitorProjection, error) {
	return reader.monitor, nil
}

func (reader reportReader) ListAccountMonitorHistory(
	context.Context,
	int64,
	int,
) ([]sub2api.AccountMonitorHistoryEntry, error) {
	return nil, nil
}

type failingMonitorReader struct{ reportReader }

func (failingMonitorReader) ListAccountMonitors(
	context.Context,
) (sub2api.AccountMonitorProjection, error) {
	return sub2api.AccountMonitorProjection{}, fmt.Errorf("monitor endpoint unavailable")
}

func cacheDiscountPricing(model string) sub2api.ChannelModelPrice {
	input, read, write := 5e-6, 0.5e-6, 6.25e-6
	return sub2api.ChannelModelPrice{
		Models: []string{model}, InputPrice: &input,
		CacheReadPrice: &read, CacheWritePrice: &write,
	}
}

func float64ptr(value float64) *float64 { return &value }

func monitorAccount(
	id int64,
	name string,
	success float64,
	ttft float64,
	latency float64,
	multiplier float64,
) sub2api.AccountMonitorAccount {
	return sub2api.AccountMonitorAccount{
		AccountID: id, Name: name, Platform: "openai",
		Status: "active", Schedulable: true,
		GroupIDs: []int64{3}, GroupNames: []string{"GPT-Pro"},
		ModelID: "gpt-5.6-sol", LatestStatus: "passed",
		SampleCount: 4, SuccessRate: success,
		TTFTP95MS: float64ptr(ttft), LatencyP95MS: float64ptr(latency),
		Multiplier: sub2api.AccountMonitorMultiplier{
			Value: float64ptr(multiplier), Source: "declared", Status: "ok",
			ObservedAt: timePtr(time.Date(2026, 7, 25, 7, 28, 0, 0, time.UTC)),
		},
		CheckedAt: timePtr(time.Date(2026, 7, 25, 7, 29, 0, 0, time.UTC)),
		UsageWindows: []sub2api.AccountMonitorUsageWindow{{
			Name: "daily", Utilization: 0.2,
		}},
	}
}

func timePtr(value time.Time) *time.Time { return &value }

type reportSummaryReader struct {
	value  DailyNotificationSummary
	ranges []summaryRange
}

type summaryRange struct {
	from time.Time
	to   time.Time
}

func (reader *reportSummaryReader) ReadDailyNotificationSummary(
	_ context.Context,
	from time.Time,
	to time.Time,
) (DailyNotificationSummary, error) {
	reader.ranges = append(reader.ranges, summaryRange{from: from, to: to})
	return reader.value, nil
}

type reportNotifier struct {
	seen       map[string]bool
	attempts   int
	delivered  int
	identities []notify.OneShotIdentity
	messages   []notify.FeishuMessage
}

func (notifier *reportNotifier) SendOneShot(
	_ context.Context,
	identity notify.OneShotIdentity,
	message notify.FeishuMessage,
) error {
	notifier.attempts++
	if notifier.seen[identity.Key] {
		return nil
	}
	notifier.seen[identity.Key] = true
	notifier.delivered++
	notifier.identities = append(notifier.identities, identity)
	notifier.messages = append(notifier.messages, message)
	return nil
}

type reportDecisionRecorder struct {
	records []store.DecisionRecord
}

func (recorder *reportDecisionRecorder) RecordNotificationDecision(
	_ context.Context,
	record store.DecisionRecord,
) error {
	recorder.records = append(recorder.records, record)
	return nil
}

func reportService(
	reader opsReader,
	summary *reportSummaryReader,
	notifier *reportNotifier,
	decisions *reportDecisionRecorder,
	now time.Time,
) Service {
	return Service{
		Reader: reader, Summary: summary, Notifier: notifier, Decisions: decisions,
		Policy: notificationpolicy.Policy{
			Version: 1, Mode: notificationpolicy.ModeEnabled,
			Feishu: notificationpolicy.FeishuPolicy{DailyDigestEnabled: true},
		},
		Timezone: time.FixedZone("Asia/Shanghai", 8*60*60),
		Now:      func() time.Time { return now },
	}
}

type opsReader interface {
	sub2api.Reader
	sub2api.AccountReader
}
