package dailyreport

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

// 本测试验证投递去重（同日同证据只发一次）、事件契约，以及脱敏不变量：
// 新健康日报仍会渲染分组名 —— pendingLines 的选型建议行输出
// digestValue(rec.GroupName)，而 Analyze 的输入投影不按 IsExclusive 过滤，
// 所以私有分组名有真实通路进入卡片。fixture 的 monitor 投影特意包含私有
// 分组 "private" 并构造出 candidate_better 建议，使「不含 private」断言
// 真正可失败。
func TestServiceDeduplicatesDailyDeliveryAndKeepsContractStable(t *testing.T) {
	t.Parallel()

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
		accounts: []sub2api.Account{{ID: 10, Name: "active account", Status: "active", Schedulable: true, GroupIDs: []int64{2, 6}}},
		usage: map[int64]sub2api.UsageStats{
			2: {TotalRequests: 100, TotalInputTokens: 1_000_000, TotalCacheReadTokens: 3_000_000, TotalCacheCreationTokens: 500_000, CacheMetricsPresent: true, TotalCost: 1.2, TotalActualCost: 0.12, TotalAccountCost: 0.12},
			6: {TotalRequests: 200, TotalInputTokens: 2_000_000, CacheMetricsPresent: true, TotalCost: 2.4, TotalActualCost: 0.12, TotalAccountCost: 0.12},
		},
		monitor: sub2api.AccountMonitorProjection{
			SchemaVersion: 2, ObservedAt: time.Date(2026, 7, 20, 1, 29, 0, 0, time.UTC),
			Settings: sub2api.AccountMonitorSettings{IntervalSeconds: 300},
			Accounts: []sub2api.AccountMonitorAccount{
				privateMonitorAccount(31, "内部账号 A", 0.70, 450, 1600, 0.12),
				privateMonitorAccount(32, "内部账号 B", 0.96, 120, 500, 0.08),
			},
		},
	}
	incidentStore := &reportIncidentStore{items: []string{"P1 confirmed GPT-Pro monitor error"}, observed: map[string]bool{}}
	notifier := &reportNotifier{seen: map[string]bool{}, incidents: incidentStore}
	analyzer := &reportAnalyzer{}
	service := Service{
		Reader:     reader,
		Candidates: reportCandidates{items: []candidates.Candidate{{ID: domain.UpstreamID(17), Name: "candidate-a", Enabled: true}}},
		Incidents:  incidentStore,
		Agent:      analyzer,
		Notifier:   notifier,
		Timezone:   time.FixedZone("Asia/Shanghai", 8*60*60),
		Now:        func() time.Time { return time.Date(2026, 7, 20, 1, 30, 0, 0, time.UTC) },
	}

	first, err := service.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	changed := reader.ops[2]
	changed.Overview.SLA = 98
	reader.ops[2] = changed
	second, err := service.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.ReportDate != "2026-07-20" || first.Notification != "delivered" || second.Notification != "delivered" {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	if notifier.delivered != 1 || notifier.attempts != 2 || len(analyzer.contracts) != 2 {
		t.Fatalf("delivered=%d attempts=%d analyses=%d", notifier.delivered, notifier.attempts, len(analyzer.contracts))
	}
	card, cardErr := notifier.messages[0].CardJSON()
	if cardErr != nil {
		t.Fatal(cardErr)
	}
	text := string(card)
	// 报告日期由 Now/Timezone 推导后进入卡片标题，是真实的数据流断言；
	// 其余「质量/利润/…」静态标题为无条件输出，断言恒真，不再保留。
	if !strings.Contains(text, "中转站日报 2026-07-20") {
		t.Fatalf("card missing dated title: %s", text)
	}
	// 脱敏不变量：私有（IsExclusive）分组名不得进入日报卡。fixture 的私有
	// 分组 "private" 构造出了 candidate_better 建议行，若渲染链不过滤，
	// 分组名会经 pendingLines 渲染进卡片。
	if strings.Contains(text, "private") {
		t.Fatalf("report leaked private group name: %s", text)
	}
	contract := analyzer.contracts[0]
	if contract.ContractVersion != "relay-ops-incident-v1" || contract.IncidentID != "daily-report:2026-07-20" || contract.Samples != 2 {
		t.Fatalf("contract=%#v", contract)
	}
	if analyzer.contracts[1].IncidentID != contract.IncidentID {
		t.Fatalf("daily report key changed: %#v", analyzer.contracts)
	}
}

// 曾经名为 TestServiceDoesNotRenderLegacyAccountQualityInHealthDigest：旧
// accountquality 注入点已随 Task 9 整体移除，泄漏断言失去通路后改为守住
// 剩余的真实数据流 —— 空监控投影必须渲染出全零计数，而不是「数据不可用」。
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
	incidentStore := &reportIncidentStore{observed: map[string]bool{}}
	notifier := &reportNotifier{seen: map[string]bool{}, incidents: incidentStore}
	service := Service{
		Reader: reader, Incidents: incidentStore, Notifier: notifier,
		Now: func() time.Time { return now },
	}

	if _, err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.messages) != 1 {
		t.Fatalf("messages = %#v", notifier.messages)
	}
	data, err := notifier.messages[0].CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	// 「质量」是静态标题，contains("质量") 恒真。断言质量层的计数行确实来自
	// 监控投影：本 fixture 的投影为空，只有走了投影路径才会打出全零计数。
	if !strings.Contains(text, "稳定 0 / 降级 0 / 不可用 0") {
		t.Fatalf("quality layer not rendered from monitor projection: %s", text)
	}
}

// 本任务的核心安全属性：账号监控读取失败时，日报仍要发出，且质量层明确
// 标注「数据不可用」及原因，而不是伪装成一切正常或整卡缺席。
func TestServiceSendsDigestWithUnavailableMarkWhenMonitorReadFails(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 20, 1, 30, 0, 0, time.UTC)
	reader := failingMonitorReader{reportReader: reportReader{
		groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}},
		ops:    map[int64]sub2api.OpsSnapshot{},
	}}
	incidentStore := &reportIncidentStore{observed: map[string]bool{}}
	notifier := &reportNotifier{seen: map[string]bool{}, incidents: incidentStore}
	service := Service{
		Reader: reader, Incidents: incidentStore, Notifier: notifier,
		Now: func() time.Time { return now },
	}

	if _, err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.messages) != 1 {
		t.Fatalf("messages = %d, want 1（监控失败不得吞掉日报）", len(notifier.messages))
	}
	data, err := notifier.messages[0].CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "数据不可用") || !strings.Contains(text, "账号监控数据不可用") {
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
	incidentStore := &reportIncidentStore{observed: map[string]bool{}}
	notifier := &reportNotifier{seen: map[string]bool{}, incidents: incidentStore}
	service := Service{
		Reader: reader, Incidents: incidentStore, Notifier: notifier, Now: func() time.Time { return now },
	}

	if _, err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := notifier.messages[0].CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"稳定 1 / 降级 1 / 不可用 0", "账号 A", "账号 B", "综合优于当前"} {
		if !strings.Contains(text, required) {
			t.Fatalf("native projection missing %q: %s", required, text)
		}
	}
	if strings.Contains(text, "账号 11") || strings.Contains(text, "账号 12") {
		t.Fatalf("health digest leaked database ids: %s", text)
	}
}

func TestCacheReportLineMarksUnconfirmedAndPricingBlockers(t *testing.T) {
	t.Parallel()

	ready := cacheReportLine(sub2api.UsageStats{}, true, 1, 1, nil)
	if !strings.Contains(ready, "缓存统计不可确认") || !strings.Contains(ready, "缓存优惠 1/1 模型") {
		t.Fatalf("ready line = %q", ready)
	}

	blocked := cacheReportLine(sub2api.UsageStats{CacheMetricsPresent: true}, false, 1, 0, []string{"cache_read_price_missing:group=2:model=gpt-5.6-sol:tier=base"})
	if !strings.Contains(blocked, "缓存优惠未就绪") || !strings.Contains(blocked, "cache_read_price_missing") {
		t.Fatalf("blocked line = %q", blocked)
	}
}

// Service.Run discovers the monitor reader through a runtime type assertion,
// so a missing method on this double would silently take the else branch and
// the tests would stop covering the digest path. Fail the build instead.
var _ sub2api.AccountMonitorReader = reportReader{}

type reportReader struct {
	channels []sub2api.Channel
	groups   []sub2api.Group
	accounts []sub2api.Account
	ops      map[int64]sub2api.OpsSnapshot
	usage    map[int64]sub2api.UsageStats
	monitor  sub2api.AccountMonitorProjection
}

func (r reportReader) ListChannels(context.Context) ([]sub2api.Channel, error) {
	return r.channels, nil
}
func (r reportReader) ListGroups(context.Context) ([]sub2api.Group, error) { return r.groups, nil }
func (r reportReader) ListAccounts(context.Context) ([]sub2api.Account, error) {
	return r.accounts, nil
}
func (r reportReader) ListChannelMonitors(context.Context) ([]sub2api.ChannelMonitor, error) {
	return nil, nil
}
func (r reportReader) GetChannelMonitorHistory(context.Context, int64, string, int) ([]sub2api.MonitorHistory, error) {
	return nil, nil
}
func (r reportReader) GetOpsSnapshot(_ context.Context, query sub2api.OpsQuery) (sub2api.OpsSnapshot, error) {
	return r.ops[query.GroupID], nil
}
func (r reportReader) GetUsageStats(_ context.Context, query sub2api.UsageQuery) (sub2api.UsageStats, error) {
	return r.usage[query.GroupID], nil
}
func (r reportReader) ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error) {
	return r.monitor, nil
}
func (r reportReader) ListAccountMonitorHistory(context.Context, int64, int) ([]sub2api.AccountMonitorHistoryEntry, error) {
	return nil, nil
}

// failingMonitorReader simulates the Sub2API monitor endpoint being down while
// the rest of the reader keeps working.
type failingMonitorReader struct{ reportReader }

func (failingMonitorReader) ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error) {
	return sub2api.AccountMonitorProjection{}, fmt.Errorf("monitor endpoint unavailable")
}

func cacheDiscountPricing(model string) sub2api.ChannelModelPrice {
	input, read, write := 5e-6, 0.5e-6, 6.25e-6
	return sub2api.ChannelModelPrice{Models: []string{model}, InputPrice: &input, CacheReadPrice: &read, CacheWritePrice: &write}
}

type reportCandidates struct{ items []candidates.Candidate }

func (r reportCandidates) ListCandidates(context.Context) ([]candidates.Candidate, error) {
	return r.items, nil
}

func float64ptr(value float64) *float64 { return &value }

func monitorAccount(id int64, name string, success, ttft, latency, multiplier float64) sub2api.AccountMonitorAccount {
	return sub2api.AccountMonitorAccount{
		AccountID: id, Name: name, Platform: "openai", Status: "active", Schedulable: true,
		GroupIDs: []int64{3}, GroupNames: []string{"GPT-Pro"}, ModelID: "gpt-5.6-sol",
		LatestStatus: "passed", SampleCount: 4, SuccessRate: success,
		TTFTP95MS: float64ptr(ttft), LatencyP95MS: float64ptr(latency),
		Multiplier: sub2api.AccountMonitorMultiplier{
			Value:      float64ptr(multiplier),
			Source:     "declared",
			Status:     "ok",
			ObservedAt: timePtr(time.Date(2026, 7, 25, 7, 28, 0, 0, time.UTC)),
		},
		CheckedAt:    timePtr(time.Date(2026, 7, 25, 7, 29, 0, 0, time.UTC)),
		UsageWindows: []sub2api.AccountMonitorUsageWindow{{Name: "daily", Utilization: 0.2}},
	}
}

func timePtr(value time.Time) *time.Time { return &value }

// privateMonitorAccount belongs to the exclusive group "private"（上方 fixture
// 里 ID 7、IsExclusive: true 的那个分组），用于让脱敏断言真正跑起来。
func privateMonitorAccount(id int64, name string, success, ttft, latency, multiplier float64) sub2api.AccountMonitorAccount {
	account := monitorAccount(id, name, success, ttft, latency, multiplier)
	account.GroupIDs = []int64{7}
	account.GroupNames = []string{"private"}
	observed := time.Date(2026, 7, 20, 1, 28, 0, 0, time.UTC)
	account.Multiplier.ObservedAt = &observed
	account.CheckedAt = &observed
	return account
}

type reportIncidentStore struct {
	items    []string
	observed map[string]bool
}

func (r *reportIncidentStore) ListIncidentSummaries(context.Context, int) ([]string, error) {
	return r.items, nil
}

func (r *reportIncidentStore) Observe(_ context.Context, observation incidents.Observation) (incidents.Transition, error) {
	r.observed[observation.Key] = true
	return incidents.Transition{State: "confirmed", Kind: "confirmed", Notify: true, RelatedKey: observation.Key}, nil
}

type reportAnalyzer struct{ contracts []agent.IncidentContractV1 }

func (a *reportAnalyzer) AnalyzeOnce(_ context.Context, contract agent.IncidentContractV1) (agent.Analysis, error) {
	a.contracts = append(a.contracts, contract)
	return agent.Analysis{Summary: "日报只读分析", Change: "无重大变化", Focus: "继续观察"}, nil
}

type reportNotifier struct {
	seen      map[string]bool
	incidents *reportIncidentStore
	attempts  int
	delivered int
	messages  []notify.FeishuMessage
}

func (n *reportNotifier) SendIncident(_ context.Context, key, evidence string, message notify.FeishuMessage) error {
	if n.incidents == nil || !n.incidents.observed[key] {
		return fmt.Errorf("incident not found for notification")
	}
	n.attempts++
	dedup := key + "\x00" + evidence
	if n.seen[dedup] {
		return nil
	}
	n.seen[dedup] = true
	n.delivered++
	n.messages = append(n.messages, message)
	return nil
}
