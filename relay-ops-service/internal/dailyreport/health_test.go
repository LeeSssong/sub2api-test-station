package dailyreport

import (
	"fmt"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/accounthealth"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func f64(v float64) *float64 { return &v }

func TestBuildHealthDigestCountsPricingCoverage(t *testing.T) {
	projection, histories, loc, now := fixture()
	fallback := func(name string) *float64 {
		if name == "Pro-SHUAI-0.17" {
			value := 0.17
			return &value
		}
		return nil
	}

	view := BuildHealthDigestWithFallback(projection, histories, loc, now, fallback)

	if view.Profit.TotalAccounts != len(projection.Accounts) {
		t.Fatalf("TotalAccounts = %d, want %d", view.Profit.TotalAccounts, len(projection.Accounts))
	}
	if view.Profit.PricedAccounts != len(projection.Accounts) {
		t.Fatalf("PricedAccounts = %d, want %d", view.Profit.PricedAccounts, len(projection.Accounts))
	}
	if view.Profit.UpstreamPricedAccounts != 1 {
		t.Fatalf("UpstreamPricedAccounts = %d, want 1", view.Profit.UpstreamPricedAccounts)
	}
	for _, item := range view.Pending {
		if item.AccountName == "Pro-SHUAI-0.17" {
			t.Fatalf("fallback-priced account must not be pending: %+v", item)
		}
	}
}

func TestPendingItemsCarryStructuredSeverity(t *testing.T) {
	projection, histories, loc, now := fixture()
	view := BuildHealthDigest(projection, histories, loc, now)

	severityByName := map[string]notify.PendingSeverity{}
	for _, item := range view.Pending {
		severityByName[item.AccountName] = item.Severity
	}
	if severityByName["Plus-XN-0.09"] != notify.PendingCritical {
		t.Fatalf("balance-exhausted severity = %q, want critical", severityByName["Plus-XN-0.09"])
	}
	warning, ok := pendingFor(
		sub2api.AccountMonitorAccount{AccountID: 41, Name: "warning"},
		accounthealth.AccountSample{SuccessRate: 0.76, SampleCount: 10, ErrorCode: "http_error"},
		accounthealth.AccountVerdict{Tier: accounthealth.TierDegraded},
		resolvedMultiplier{value: f64(0.2)},
	)
	if !ok || warning.Severity != notify.PendingWarning {
		t.Fatalf("degraded severity = %q, ok = %v, want warning", warning.Severity, ok)
	}
	accounting, ok := pendingFor(
		sub2api.AccountMonitorAccount{
			AccountID: 42, Name: "accounting",
			Multiplier: sub2api.AccountMonitorMultiplier{Source: "declared", Status: "failed"},
		},
		accounthealth.AccountSample{SuccessRate: 1, SampleCount: 10},
		accounthealth.AccountVerdict{Tier: accounthealth.TierHealthy},
		resolvedMultiplier{},
	)
	if !ok || accounting.Severity != notify.PendingAccounting {
		t.Fatalf("unpriced severity = %q, ok = %v, want accounting", accounting.Severity, ok)
	}
	for _, item := range view.Pending {
		if item.Severity == "" {
			t.Fatalf("pending item has empty severity: %+v", item)
		}
	}
}

func TestProblemLabelUsesOperatorReadableCopy(t *testing.T) {
	cases := map[string]string{
		"balance_exhausted":  "余额耗尽",
		"http_error":         "HTTP 错误",
		"timeout":            "请求超时",
		"malformed_stream":   "响应格式异常",
		"model_unavailable":  "模型不可用",
		"account_test_error": "账号测试失败",
		"unknown_code":       "账号异常",
	}
	for input, want := range cases {
		if got := problemLabel(input); got != want {
			t.Fatalf("problemLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

func fixture() (sub2api.AccountMonitorProjection, map[int64][]sub2api.AccountMonitorHistoryEntry, *time.Location, time.Time) {
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, loc)
	projection := sub2api.AccountMonitorProjection{
		SchemaVersion: 2,
		Settings:      sub2api.AccountMonitorSettings{IntervalSeconds: 300},
		Accounts: []sub2api.AccountMonitorAccount{
			{
				AccountID: 22, Name: "Plus-XN-0.09", GroupIDs: []int64{6}, GroupNames: []string{"GPT-Plus"},
				SuccessRate: 0, SampleCount: 200, ErrorCode: "balance_exhausted",
				Multiplier: sub2api.AccountMonitorMultiplier{Value: f64(0.25), Source: "declared", Status: "ok"},
				TodayStats: &sub2api.AccountMonitorTodayStats{StandardCost: 0, UserCost: 0},
			},
			{
				AccountID: 21, Name: "Pro-SHUAI-0.17", GroupIDs: []int64{7}, GroupNames: []string{"GPT-Pro"},
				SuccessRate: 0.98, SampleCount: 200, TTFTP95MS: f64(3777),
				Multiplier: sub2api.AccountMonitorMultiplier{Status: "failed", Source: "measured"},
				TodayStats: &sub2api.AccountMonitorTodayStats{StandardCost: 100, UserCost: 40},
			},
			{
				AccountID: 26, Name: "Pro-SHEN-0.16", GroupIDs: []int64{7}, GroupNames: []string{"GPT-Pro"},
				SuccessRate: 1, SampleCount: 200, TTFTP95MS: f64(2000),
				Multiplier: sub2api.AccountMonitorMultiplier{Value: f64(0.16), Source: "declared", Status: "ok"},
				TodayStats: &sub2api.AccountMonitorTodayStats{StandardCost: 200, UserCost: 100},
			},
		},
	}
	histories := map[int64][]sub2api.AccountMonitorHistoryEntry{
		26: {
			{AccountID: 26, Status: "success", TTFTMS: f64(1500), CheckedAt: time.Date(2026, 7, 27, 8, 0, 0, 0, loc)},
			{AccountID: 26, Status: "failed", ErrorCode: "http_error", CheckedAt: time.Date(2026, 7, 26, 8, 0, 0, 0, loc)},
		},
	}
	return projection, histories, loc, now
}

func TestBuildHealthDigestUsesNamesAndTiers(t *testing.T) {
	projection, histories, loc, now := fixture()
	view := BuildHealthDigest(projection, histories, loc, now)

	if view.Quality.Unavailable != 1 {
		t.Fatalf("Unavailable = %d, want 1", view.Quality.Unavailable)
	}
	if view.Quality.Healthy != 2 {
		t.Fatalf("Healthy = %d, want 2", view.Quality.Healthy)
	}
	if view.Quality.Slow != 1 {
		t.Fatalf("Slow = %d, want 1 (仅 Pro-SHUAI-0.17 的 3777ms 超阈值)", view.Quality.Slow)
	}
	for _, account := range view.Accounts {
		if account.Name == "" {
			t.Fatal("明细行缺少账号名")
		}
	}
}

func TestBuildHealthDigestExcludesIncomputableProfit(t *testing.T) {
	projection, histories, loc, now := fixture()
	view := BuildHealthDigest(projection, histories, loc, now)

	if view.Profit.ExcludedAccounts != 1 {
		t.Fatalf("ExcludedAccounts = %d, want 1 (Pro-SHUAI-0.17 倍率 failed)", view.Profit.ExcludedAccounts)
	}
	// 上游成本 = 200 * 0.16 = 32；收入 = 100（SHUAI 的 40 必须排除）
	if view.Profit.UpstreamCost != 32 {
		t.Fatalf("UpstreamCost = %v, want 32", view.Profit.UpstreamCost)
	}
	if view.Profit.Revenue != 100 {
		t.Fatalf("Revenue = %v, want 100", view.Profit.Revenue)
	}
}

func TestBuildHealthDigestListsPendingItems(t *testing.T) {
	projection, histories, loc, now := fixture()
	// 夹具里 Pro-SHUAI 是 measured+failed（上游不支持自动测算，不进待办）。
	// 这里改成 declared+failed —— 上游声明了倍率却给不出有效值，是真问题，
	// 必须与余额耗尽一起出现在待处理层。
	projection.Accounts[1].Multiplier = sub2api.AccountMonitorMultiplier{
		Source: "declared", Status: "failed",
	}
	view := BuildHealthDigest(projection, histories, loc, now)

	var names []string
	for _, item := range view.Pending {
		names = append(names, item.AccountName)
	}
	if len(names) < 2 {
		t.Fatalf("待处理项 = %v，应至少含余额耗尽与倍率不可用两项", names)
	}
}

func TestBuildHealthDigestTreatsNonPositiveMultiplierAsUnusable(t *testing.T) {
	projection, histories, loc, now := fixture()
	// 上游报 status=ok 但倍率为 0：Sub2API 只拒绝 value<0，0 会漏过来。
	// 若当作可核算，会算出「成本 0、毛利率 100%」这种误导性数字。
	projection.Accounts[2].Multiplier = sub2api.AccountMonitorMultiplier{
		Value: f64(0), Source: "declared", Status: "ok",
	}
	view := BuildHealthDigest(projection, histories, loc, now)

	if view.Profit.ExcludedAccounts != 2 {
		t.Fatalf("ExcludedAccounts = %d, want 2 (倍率 failed 与倍率 0 都不可核算)", view.Profit.ExcludedAccounts)
	}
	if view.Profit.Computable {
		t.Fatal("全部账号倍率不可用时不得标记为可核算")
	}
}

func TestBuildHealthDigestRecommendationsAreComplete(t *testing.T) {
	projection, histories, loc, now := fixture()
	view := BuildHealthDigest(projection, histories, loc, now)
	for _, rec := range view.Recommendations {
		if rec.GroupName == "" || rec.CurrentName == "" || rec.CandidateName == "" {
			t.Fatalf("建议行字段不完整，缺候选或当前账号名: %+v", rec)
		}
	}
}

func TestBuildGroupAvailabilityReportsEveryGroup(t *testing.T) {
	projection, _, _, now := fixture()
	views := BuildGroupAvailability(projection, nil, now)

	// 健康分组也必须返回，否则状态机永远观测不到 Failing=false，
	// 恢复分支不可达，告警只会成功发出一次。
	if len(views) != 2 {
		t.Fatalf("views = %+v, want 2 (GPT-Plus 与 GPT-Pro 都要在)", views)
	}
	byName := map[string]GroupAvailabilityView{}
	for _, view := range views {
		byName[view.Alert.GroupName] = view
	}

	plus, ok := byName["GPT-Plus"]
	if !ok {
		t.Fatal("缺少 GPT-Plus")
	}
	if !plus.Alerting || plus.Alert.Available != 0 || plus.Alert.Total != 1 {
		t.Fatalf("GPT-Plus = %+v, want alerting 0/1", plus)
	}
	if len(plus.Alert.Down) != 1 || plus.Alert.Down[0].Name != "Plus-XN-0.09" {
		t.Fatalf("Down = %+v", plus.Alert.Down)
	}
	// 告警卡与日报待处理层统一走 problemLabel：不得把裸的 balance_exhausted
	// 发进运维群。
	if plus.Alert.Down[0].ErrorCode != "余额耗尽" {
		t.Fatalf("ErrorCode = %q, want 余额耗尽", plus.Alert.Down[0].ErrorCode)
	}

	pro, ok := byName["GPT-Pro"]
	if !ok {
		t.Fatal("缺少 GPT-Pro")
	}
	if pro.Alerting {
		t.Fatalf("GPT-Pro 2/2 健康不应告警: %+v", pro)
	}
}

func TestBuildGroupAvailabilityKeepsGroupsWhoseAccountsLostAllSamples(t *testing.T) {
	projection, _, _, now := fixture()
	// GPT-Plus 唯一账号失去全部探测样本（TierUnknown）。GroupAvailabilities
	// 会跳过 unknown 账号，若分组因此整个消失，就不再被 Observe，incident
	// 卡在 confirmed，恢复卡与下一次告警都发不出来。
	// ErrorCode 也要清掉：balance_exhausted 短路先于零样本判定，带着它的
	// 零样本账号会判 Unavailable 而不是 Unknown。
	projection.Accounts[0].SampleCount = 0
	projection.Accounts[0].ErrorCode = ""
	views := BuildGroupAvailability(projection, nil, now)

	byName := map[string]GroupAvailabilityView{}
	for _, view := range views {
		byName[view.Alert.GroupName] = view
	}
	plus, ok := byName["GPT-Plus"]
	if !ok {
		t.Fatalf("失去样本的分组不得从结果里消失: %+v", views)
	}
	if plus.Alerting {
		t.Fatalf("无样本分组应以 Alerting=false 观测（fail-safe）: %+v", plus)
	}
	if len(views) != 2 {
		t.Fatalf("views = %+v, want 2", views)
	}
	// 结果必须按分组名排序，投递顺序才可复现。
	if views[0].Alert.GroupName != "GPT-Plus" || views[1].Alert.GroupName != "GPT-Pro" {
		t.Fatalf("views 未按分组名排序: %+v", views)
	}
}

func TestBuildHealthDigestOmitsDeltaWithoutComparableHistory(t *testing.T) {
	projection, _, loc, now := fixture()
	// 历史为空：不得凭空得出「健康账号较昨日 ↑N」。
	view := BuildHealthDigest(projection, map[int64][]sub2api.AccountMonitorHistoryEntry{}, loc, now)
	if view.Quality.HealthyDelta != nil {
		t.Fatalf("HealthyDelta = %v, want nil（无可比历史时不给同比）", *view.Quality.HealthyDelta)
	}
}

// 阻断项回归：判定必须消费当日切片，而不是 projection 的 7 天累计口径。
// 账号今天硬挂（当日全失败），但 7 天累计成功率仍高达 98.6%——修复前它会
// 被判 Healthy，三档计数与待处理层整天报「基本健康」。
// 反向验证：把 classify 输入换回 projection.SuccessRate，本测试必须变红。
func TestBuildHealthDigestJudgesByTodayWindowNotCumulative(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, loc)
	projection := sub2api.AccountMonitorProjection{
		SchemaVersion: 2,
		Settings:      sub2api.AccountMonitorSettings{IntervalSeconds: 300},
		Accounts: []sub2api.AccountMonitorAccount{{
			AccountID: 22, Name: "Plus-XN-0.09", GroupIDs: []int64{6}, GroupNames: []string{"GPT-Plus"},
			SuccessRate: 0.986, SampleCount: 2016, ErrorCode: "http_error",
		}},
	}
	entries := make([]sub2api.AccountMonitorHistoryEntry, 0, 12)
	for i := 0; i < 12; i++ {
		entries = append(entries, sub2api.AccountMonitorHistoryEntry{
			AccountID: 22, Status: "failed", ErrorCode: "http_error",
			CheckedAt: now.Add(-time.Duration(i) * 5 * time.Minute),
		})
	}
	view := BuildHealthDigest(projection, map[int64][]sub2api.AccountMonitorHistoryEntry{22: entries}, loc, now)

	if view.Quality.Unavailable != 1 || view.Quality.Healthy != 0 {
		t.Fatalf("当日全失败必须判不可用: healthy=%d degraded=%d unavailable=%d",
			view.Quality.Healthy, view.Quality.Degraded, view.Quality.Unavailable)
	}
	if len(view.Accounts) != 1 || view.Accounts[0].SuccessRate != "0.0%" {
		t.Fatalf("明细成功率必须是当日口径 0.0%%: %+v", view.Accounts)
	}
	if len(view.Pending) != 1 || view.Pending[0].Problem != "HTTP 错误" {
		t.Fatalf("待处理项应携带当日窗口的错误码: %+v", view.Pending)
	}
}

// 审查员实测缺陷回归：给多个账号喂完全相同的今昨两日历史，HealthyDelta
// 曾算出非零（今天走完整 tier 规则、昨天只比裸成功率，两套口径）。
// 两侧统一走 ClassifyAccount 后，历史相同必须恒为 0。projection 的累计
// 成功率故意与当日口径矛盾（0.6），换回 projection 口径本测试必须变红。
func TestBuildHealthDigestDeltaZeroWhenHistoriesIdentical(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, loc)
	accounts := []sub2api.AccountMonitorAccount{}
	histories := map[int64][]sub2api.AccountMonitorHistoryEntry{}
	// 三种档位各一个账号，今昨两日历史逐条相同（仅日期不同）。
	patterns := map[int64][]string{
		1: {"success", "success", "success", "success"}, // 稳定
		2: {"success", "failed", "success", "failed"},   // 降级 0.5
		3: {"failed", "failed", "failed", "failed"},     // 不可用
	}
	for id, statuses := range patterns {
		accounts = append(accounts, sub2api.AccountMonitorAccount{
			AccountID: id, Name: fmt.Sprintf("acct-%d", id), GroupNames: []string{"G"},
			SuccessRate: 0.6, SampleCount: 2016,
		})
		for day := 0; day < 2; day++ {
			base := now.AddDate(0, 0, -day).Add(-8 * time.Hour)
			for i, status := range statuses {
				entry := sub2api.AccountMonitorHistoryEntry{
					AccountID: id, Status: status,
					CheckedAt: base.Add(time.Duration(i) * 5 * time.Minute),
				}
				if status == "failed" {
					entry.ErrorCode = "http_error"
				}
				histories[id] = append(histories[id], entry)
			}
		}
	}
	projection := sub2api.AccountMonitorProjection{
		SchemaVersion: 2,
		Settings:      sub2api.AccountMonitorSettings{IntervalSeconds: 300},
		Accounts:      accounts,
	}
	view := BuildHealthDigest(projection, histories, loc, now)
	if view.Quality.HealthyDelta == nil {
		t.Fatal("今昨都有样本时必须给出同比")
	}
	if *view.Quality.HealthyDelta != 0 {
		t.Fatalf("HealthyDelta = %d, want 0（两日历史完全相同）", *view.Quality.HealthyDelta)
	}
}

// 阻断项回归：分组告警必须按最近 1 小时滚动窗判定。账号 7 天累计成功率
// 100%，但最近 1 小时全部失败——修复前 classify 读 projection 直接判
// Healthy，告警对 http_error/timeout 类故障的响应时间是天级。
// 窗口外（1 小时前）的成功探测必须被排除，否则会稀释失败率导致不告警。
func TestBuildGroupAvailabilityAlertsOnRollingWindowFailures(t *testing.T) {
	now := time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC)
	projection := sub2api.AccountMonitorProjection{
		SchemaVersion: 2,
		Settings:      sub2api.AccountMonitorSettings{IntervalSeconds: 300},
		Accounts: []sub2api.AccountMonitorAccount{{
			AccountID: 26, Name: "Pro-SHEN-0.16", GroupIDs: []int64{7}, GroupNames: []string{"GPT-Pro"},
			SuccessRate: 1, SampleCount: 2016,
		}},
	}
	entries := []sub2api.AccountMonitorHistoryEntry{}
	// 最近 1 小时：12 次探测全失败
	for i := 0; i < 12; i++ {
		entries = append(entries, sub2api.AccountMonitorHistoryEntry{
			AccountID: 26, Status: "failed", ErrorCode: "timeout",
			CheckedAt: now.Add(-time.Duration(i)*5*time.Minute - time.Minute),
		})
	}
	// 窗口之前：30 次成功。若被错误计入，成功率升到 0.71 判降级，不再告警。
	for i := 0; i < 30; i++ {
		entries = append(entries, sub2api.AccountMonitorHistoryEntry{
			AccountID: 26, Status: "success",
			CheckedAt: now.Add(-time.Hour).Add(-time.Duration(i+1) * 2 * time.Minute),
		})
	}
	views := BuildGroupAvailability(projection, map[int64][]sub2api.AccountMonitorHistoryEntry{26: entries}, now)
	if len(views) != 1 {
		t.Fatalf("views = %+v", views)
	}
	pro := views[0]
	if !pro.Alerting || pro.Alert.Available != 0 || pro.Alert.Total != 1 {
		t.Fatalf("1 小时窗内全失败必须告警: %+v", pro)
	}
	if len(pro.Alert.Down) != 1 || pro.Alert.Down[0].ErrorCode != "请求超时" {
		t.Fatalf("Down = %+v", pro.Alert.Down)
	}
}

func TestBuildHealthDigestOmitsUnsupportedMeasurementFromPending(t *testing.T) {
	projection, histories, loc, now := fixture()
	// Pro-SHUAI-0.17 是 measured+failed：上游不支持自动测算，不该每天进待办
	view := BuildHealthDigest(projection, histories, loc, now)
	for _, item := range view.Pending {
		if item.AccountName == "Pro-SHUAI-0.17" {
			t.Fatalf("上游不支持自动测算的账号不得进入待处理: %+v", item)
		}
	}
	if view.Profit.UnsupportedAccounts != 1 {
		t.Fatalf("UnsupportedAccounts = %d, want 1", view.Profit.UnsupportedAccounts)
	}
	if view.Profit.ExcludedAccounts < 1 {
		t.Fatal("利润仍必须排除它，不得估算")
	}
}

func TestBuildHealthDigestKeepsOtherMultiplierFailuresInPending(t *testing.T) {
	projection, histories, loc, now := fixture()
	// declared + failed 是真问题（上游声明了却给不出有效值），必须留在待办
	projection.Accounts[1].Multiplier = sub2api.AccountMonitorMultiplier{
		Source: "declared", Status: "failed",
	}
	view := BuildHealthDigest(projection, histories, loc, now)
	found := false
	for _, item := range view.Pending {
		if item.AccountName == projection.Accounts[1].Name {
			found = true
		}
	}
	if !found {
		t.Fatal("declared+failed 是真问题，必须留在待处理")
	}
	if view.Profit.UnsupportedAccounts != 0 {
		t.Fatalf("UnsupportedAccounts = %d, want 0", view.Profit.UnsupportedAccounts)
	}
}

func TestBuildHealthDigestUsesUpstreamPricingFallback(t *testing.T) {
	projection, histories, loc, now := fixture()
	// Pro-SHUAI-0.17 的 Sub2API 倍率是 measured/failed，靠上游定价兜底
	fallback := func(name string) *float64 {
		if name == "Pro-SHUAI-0.17" {
			v := 0.17
			return &v
		}
		return nil
	}
	view := BuildHealthDigestWithFallback(projection, histories, loc, now, fallback)

	if view.Profit.UnsupportedAccounts != 0 {
		t.Fatalf("UnsupportedAccounts = %d, want 0（已被兜底，不再算不支持）", view.Profit.UnsupportedAccounts)
	}
	var line string
	for _, account := range view.Accounts {
		if account.Name == "Pro-SHUAI-0.17" {
			line = account.Multiplier
		}
	}
	if line != "0.17x（上游定价）" {
		t.Fatalf("Multiplier = %q, want 0.17x（上游定价）—— 来源必须可见", line)
	}
}

func TestBuildHealthDigestFallbackDoesNotOverrideTrustworthy(t *testing.T) {
	projection, histories, loc, now := fixture()
	// Pro-SHEN-0.16 有 declared/ok 倍率，兜底不得覆盖它
	fallback := func(string) *float64 { v := 9.99; return &v }
	view := BuildHealthDigestWithFallback(projection, histories, loc, now, fallback)

	for _, account := range view.Accounts {
		if account.Name == "Pro-SHEN-0.16" && account.Multiplier != "0.16x" {
			t.Fatalf("Multiplier = %q, want 0.16x（declared 优先于兜底）", account.Multiplier)
		}
	}
}

func TestBuildHealthDigestFallbackIncludesAccountInProfit(t *testing.T) {
	projection, histories, loc, now := fixture()
	// 兜底成功后 Pro-SHUAI-0.17 的利润必须可核算：
	// 收入 = 100 + 40 = 140，上游成本 = 200*0.16 + 100*0.17 = 49
	fallback := func(name string) *float64 {
		if name == "Pro-SHUAI-0.17" {
			v := 0.17
			return &v
		}
		return nil
	}
	view := BuildHealthDigestWithFallback(projection, histories, loc, now, fallback)

	if view.Profit.ExcludedAccounts != 0 {
		t.Fatalf("ExcludedAccounts = %d, want 0（兜底后全部可核算）", view.Profit.ExcludedAccounts)
	}
	if view.Profit.Revenue != 140 {
		t.Fatalf("Revenue = %v, want 140", view.Profit.Revenue)
	}
	if view.Profit.UpstreamCost != 49 {
		t.Fatalf("UpstreamCost = %v, want 49", view.Profit.UpstreamCost)
	}
}

func TestBuildHealthDigestAccumulatesTrafficRequests(t *testing.T) {
	projection, histories, loc, now := fixture()
	projection.Accounts[1].TodayStats.Requests = 150
	projection.Accounts[2].TodayStats.Requests = 350
	view := BuildHealthDigest(projection, histories, loc, now)

	// 请求数缺失时卡片会同时打出「收入 $100」和「请求 0」，自相矛盾。
	if view.Traffic.Requests != 500 {
		t.Fatalf("Traffic.Requests = %d, want 500", view.Traffic.Requests)
	}
	if !view.Traffic.HasTraffic {
		t.Fatal("有真实调用时 HasTraffic 应为 true")
	}
}
