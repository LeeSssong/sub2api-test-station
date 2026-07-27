# 行动晨报与账号名称告警 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把飞书日报收敛为只展开异常与建议的行动晨报，并让所有账号类站内告警只显示账号名称而保持 ID 事件身份不变。

**Architecture:** `dailyreport` 继续负责健康与利润计算，但改为产出结构化行动等级、利润覆盖数和上游定价覆盖数；`notify` 只渲染运行概览、经营情况、行动项与建议，不再渲染全量账号明细。`opsmonitor` 的内部对象增加展示名称，事件键、baseline key、evidence hash 和去重仍只使用 `{kind,id}`。

**Tech Stack:** Go 1.24、标准库 `testing`、Feishu stable interactive card、现有 `incidents.Machine` 与 `notify` 卡片边界。

## Global Constraints

- 日报定位为“行动晨报”，只展开异常账号与可执行建议。
- 日报标题必须为 `中转站晨报 · M月D日`。
- 账号异常在一张日报内只出现一次；优先级为 `critical > warning > accounting`。
- 偏慢账号只在运行概览计数，不逐条进入行动项。
- 飞书日报不得再渲染全量 `AccountDetailLine`、“明细”、单账号 TTFT/P95/倍率/毛利。
- 经营情况必须显示 `TotalAccounts`、`PricedAccounts`、`UpstreamPricedAccounts` 对应的利润覆盖口径。
- 采用上游定价兜底的账号计入利润覆盖，不进入倍率待处理。
- 监控数据不可用时不得输出零异常或“无待处理项”等假正常结论。
- 用户可见的账号类告警只显示账号名称；空名称显示 `账号名称不可用`，不得回退为 ID。
- incident key、multiplier baseline key、evidence hash、状态机与通知去重继续只使用账号 ID。
- 不修改健康阈值、告警阈值、确认窗口、探测频率或 scheduler。
- 不修改 `/ops` 页面，不引入 Feishu 新版卡片组件。
- 卡片不得超过 30 KiB，不得泄露 API Key、Token、Base URL、原始响应或用户身份。
- 所有 Go 测试在 `relay-ops-service/` 目录运行。

---

### Task 1: 产出结构化行动项与利润覆盖口径

**Files:**
- Modify: `relay-ops-service/internal/notify/digest_v2.go`
- Modify: `relay-ops-service/internal/dailyreport/health.go`
- Test: `relay-ops-service/internal/dailyreport/health_test.go`

**Interfaces:**
- Produces: `type PendingSeverity string`
- Produces: `PendingCritical`、`PendingWarning`、`PendingAccounting`
- Changes: `PendingItem` 增加内部去重字段 `AccountID int64` 和 `Severity PendingSeverity`；渲染不得输出 `AccountID`
- Changes: `ProfitLine` 增加 `TotalAccounts int`、`PricedAccounts int`、`UpstreamPricedAccounts int`
- Changes: `pendingFor(account, sample, verdict, resolved resolvedMultiplier) (notify.PendingItem, bool)`
- Preserves: `BuildHealthDigestWithFallback(projection sub2api.AccountMonitorProjection, histories map[int64][]sub2api.AccountMonitorHistoryEntry, loc *time.Location, now time.Time, fallback func(string) *float64) notify.HealthDigestView`

- [ ] **Step 1: 写利润覆盖与行动等级失败测试**

在 `internal/dailyreport/health_test.go` 追加：

```go
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
		"balance_exhausted": "余额耗尽",
		"http_error":        "HTTP 错误",
		"timeout":           "请求超时",
		"malformed_stream":  "响应格式异常",
		"model_unavailable": "模型不可用",
		"account_test_error": "账号测试失败",
		"unknown_code":      "账号异常",
	}
	for input, want := range cases {
		if got := problemLabel(input); got != want {
			t.Fatalf("problemLabel(%q) = %q, want %q", input, got, want)
		}
	}
}
```

同时给 `health_test.go` 的现有 import block 增加：

```go
"example.invalid/relay-ops-service/internal/accounthealth"
"example.invalid/relay-ops-service/internal/notify"
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/dailyreport/ -run 'Test(BuildHealthDigestCountsPricingCoverage|PendingItemsCarryStructuredSeverity|ProblemLabelUsesOperatorReadableCopy)' -count=1 -v
```

Expected: 编译失败，提示 `ProfitLine` 缺少覆盖字段、`PendingItem` 缺少 `Severity` 或 severity 常量未定义；补齐结构类型但尚未改文案时，`TestProblemLabelUsesOperatorReadableCopy` 必须因仍返回原始错误码而失败。

- [ ] **Step 3: 增加结构化类型**

在 `internal/notify/digest_v2.go` 将视图类型改为：

```go
type PendingSeverity string

const (
	PendingCritical   PendingSeverity = "critical"
	PendingWarning    PendingSeverity = "warning"
	PendingAccounting PendingSeverity = "accounting"
)

type ProfitLine struct {
	Revenue               float64
	UpstreamCost          float64
	Gross                 float64
	Margin                *float64
	Computable            bool
	ExcludedAccounts      int
	UnsupportedAccounts   int
	TotalAccounts         int
	PricedAccounts        int
	UpstreamPricedAccounts int
	NoTraffic             bool
}

type PendingItem struct {
	AccountID   int64
	AccountName string
	Problem     string
	Detail      string
	Severity    PendingSeverity
}
```

字段对齐由 `gofmt` 处理；不要为了兼容渲染而给空 severity 猜默认值，所有构建路径必须显式赋值。

- [ ] **Step 4: 在日报构建层统计覆盖数**

在 `BuildHealthDigestWithFallback` 初始化并递增：

```go
view := notify.HealthDigestView{Date: now.In(loc).Format("2006-01-02")}
totalAccounts := len(projection.Accounts)
pricedAccounts := 0
upstreamPricedAccounts := 0
```

在每个账号调用 `resolveMultiplier` 后：

```go
multiplier := resolveMultiplier(account, fallback)
if multiplier.value != nil {
	pricedAccounts++
}
if multiplier.fromUpstream {
	upstreamPricedAccounts++
}
```

构造 `ProfitLine` 时写入：

```go
view.Profit = notify.ProfitLine{
	Revenue: total.Revenue, UpstreamCost: total.UpstreamCost, Gross: total.Gross,
	Margin: total.Margin, Computable: computable, ExcludedAccounts: excluded,
	UnsupportedAccounts: unsupported,
	TotalAccounts: totalAccounts, PricedAccounts: pricedAccounts,
	UpstreamPricedAccounts: upstreamPricedAccounts,
	NoTraffic: !hasTraffic,
}
```

- [ ] **Step 5: 让行动项消费最终倍率结果**

将调用改为：

```go
if item, ok := pendingFor(account, sample, verdict, multiplier); ok {
	view.Pending = append(view.Pending, item)
}
```

将函数改为：

```go
func pendingFor(
	account sub2api.AccountMonitorAccount,
	sample accounthealth.AccountSample,
	verdict accounthealth.AccountVerdict,
	resolved resolvedMultiplier,
) (notify.PendingItem, bool) {
	switch {
	case verdict.Tier == accounthealth.TierUnavailable:
		return notify.PendingItem{
			AccountID:   account.AccountID,
			AccountName: account.Name,
			Problem:     problemLabel(sample.ErrorCode),
			Severity:    notify.PendingCritical,
		}, true
	case verdict.Tier == accounthealth.TierDegraded:
		detail := ""
		if sample.ErrorCode != "" {
			detail = problemLabel(sample.ErrorCode)
		}
		return notify.PendingItem{
			AccountID:   account.AccountID,
			AccountName: account.Name,
			Problem:     fmt.Sprintf("成功率 %.0f%% ↓", sample.SuccessRate*100),
			Detail:      detail,
			Severity:    notify.PendingWarning,
		}, true
	case resolved.value == nil:
		if unsupportedMeasurement(account.Multiplier) {
			return notify.PendingItem{}, false
		}
		return notify.PendingItem{
			AccountID:   account.AccountID,
			AccountName: account.Name,
			Problem:     "倍率不可用",
			Detail:      "利润未核算",
			Severity:    notify.PendingAccounting,
		}, true
	}
	return notify.PendingItem{}, false
}
```

`problemLabel("")` 会返回“不可用”，但降级账号没有错误码时不应显示“不可用”，因此 warning 分支必须按上面的显式空值判断保留空详情。

把 `problemLabel` 扩展为完整运营文案映射：

```go
func problemLabel(errorCode string) string {
	switch errorCode {
	case accounthealth.ErrorCodeBalanceExhausted:
		return "余额耗尽"
	case "http_error":
		return "HTTP 错误"
	case "timeout":
		return "请求超时"
	case "malformed_stream":
		return "响应格式异常"
	case "model_unavailable":
		return "模型不可用"
	case "account_test_error":
		return "账号测试失败"
	case "":
		return "不可用"
	default:
		return "账号异常"
	}
}
```

把 `TestBuildHealthDigestJudgesByTodayWindowNotCumulative` 中的待处理断言从原始 `http_error` 更新为批准文案 `HTTP 错误`；把 `TestBuildGroupAvailabilityAlertsOnRollingWindowFailures` 的下线原因断言从原始 `timeout` 更新为 `请求超时`。其余按原始错误码构造判定的测试输入保持不变。

- [ ] **Step 6: 运行日报构建测试**

Run:

```bash
gofmt -w internal/notify/digest_v2.go internal/dailyreport/health.go internal/dailyreport/health_test.go
go test ./internal/dailyreport/ -count=1
```

Expected: PASS。若旧断言仍期待“倍率测不出 / 利润无法核算”，同步更新为批准文案“倍率不可用 / 利润未核算”，不得删除行为断言。

- [ ] **Step 7: 提交**

```bash
git add internal/notify/digest_v2.go internal/dailyreport/health.go internal/dailyreport/health_test.go
git commit -m "feat: structure morning digest actions and pricing coverage"
```

---

### Task 2: 渲染行动晨报并移除全量明细

**Files:**
- Modify: `relay-ops-service/internal/notify/digest_v2.go`
- Test: `relay-ops-service/internal/notify/digest_v2_test.go`
- Test: `relay-ops-service/internal/dailyreport/service_test.go`

**Interfaces:**
- Consumes: Task 1 的 `PendingSeverity`、利润覆盖字段
- Produces: `digestTemplate(view HealthDigestView) string`
- Produces: `morningDate(value string) string`
- Produces: `normalizePending(items []PendingItem) []PendingItem`
- Removes: `AccountDetailLine`、`HealthDigestView.Accounts`、`detailLines`、`fitAccountLines`、`digestValueOrDash`
- Preserves: `RenderHealthDigest(view HealthDigestView) FeishuMessage`

- [ ] **Step 1: 用行动晨报期望替换旧四层测试**

在 `internal/notify/digest_v2_test.go`：

- 用下方 `TestRenderHealthDigestActionMorningOrder` 替换 `TestRenderHealthDigestLayerOrder` 和只覆盖全量明细的 `TestRenderHealthDigestUsesAccountNameNotID`；
- 删除 `TestRenderHealthDigestFitsCardLimitWithManyPendingItems`，由 Step 8 的唯一账号行动项预算测试统一覆盖；
- 用 Step 2 的新测试替换 `TestRenderHealthDigestNoTrafficCollapses`、`TestRenderHealthDigestDataUnavailable` 和 `TestRenderHealthDigestExplainsUnsupportedMeasurement`；
- 更新 `TestRenderHealthDigestShowsRecommendations` 的标题断言为“调整建议”，更新 `TestRenderHealthDigestOmitsEmptyRecommendations` 的禁止标题为“调整建议”。

写入：

```go
func TestRenderHealthDigestActionMorningOrder(t *testing.T) {
	view := HealthDigestView{
		Date: "2026-07-27",
		Quality: QualityLine{
			Healthy: 8, Degraded: 1, Unavailable: 1, Slow: 3,
			HealthyDelta: intPtr(-1), TTFTP95MedianMS: f64Ptr(3800),
		},
		Profit: ProfitLine{
			Revenue: 140, UpstreamCost: 49, Gross: 91, Margin: f64Ptr(0.65),
			Computable: true, TotalAccounts: 10, PricedAccounts: 9,
			UpstreamPricedAccounts: 3,
		},
		Pending: []PendingItem{
			{AccountID: 31, AccountName: "特惠-XM-0.045", Problem: "成功率 76% ↓", Detail: "HTTP 错误", Severity: PendingWarning},
			{AccountID: 32, AccountName: "Pro20x-XN-0.25", Problem: "余额耗尽", Severity: PendingCritical},
			{AccountID: 33, AccountName: "claude-SHUAI", Problem: "倍率不可用", Detail: "利润未核算", Severity: PendingAccounting},
		},
		Recommendations: []RecommendationLine{
			{GroupName: "GPT-Pro", CurrentName: "A", CandidateName: "B", Reason: "成功率更高"},
		},
		Traffic: TrafficLine{HasTraffic: true, Requests: 57},
	}

	message := RenderHealthDigest(view)
	text := renderText(t, message)
	for _, want := range []string{
		"中转站晨报 · 7月27日",
		"运行概览", "8 个稳定｜1 个降级｜1 个不可用",
		"经营情况", "请求 57｜收入 $140.00｜成本 $49.00",
		"利润覆盖 9/10 个账号｜3 个采用上游公开定价",
		"需要处理 · 3", "严重｜Pro20x-XN-0.25",
		"注意｜特惠-XM-0.045", "核算｜claude-SHUAI",
		"调整建议", "GPT-Pro：建议由 A 切换到 B｜成功率更高",
		"其余 7 个账号无待处理项",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	if message.Card.Header.Template != "red" {
		t.Fatalf("template = %q, want red", message.Card.Header.Template)
	}
	if strings.Contains(text, "明细") || strings.Contains(text, "TTFT P50") {
		t.Fatalf("action morning must not render full details: %s", text)
	}
}
```

- [ ] **Step 2: 写颜色、去重、降级与无流量失败测试**

追加表驱动测试：

```go
func TestRenderHealthDigestTemplate(t *testing.T) {
	cases := []struct {
		name string
		view HealthDigestView
		want string
	}{
		{"healthy", HealthDigestView{Quality: QualityLine{Healthy: 3}, Profit: ProfitLine{TotalAccounts: 3, PricedAccounts: 3}}, "green"},
		{"degraded", HealthDigestView{Quality: QualityLine{Healthy: 2, Degraded: 1}, Profit: ProfitLine{TotalAccounts: 3, PricedAccounts: 3}}, "orange"},
		{"accounting", HealthDigestView{Quality: QualityLine{Healthy: 3}, Profit: ProfitLine{TotalAccounts: 3, PricedAccounts: 2}, Pending: []PendingItem{{AccountName: "A", Severity: PendingAccounting}}}, "orange"},
		{"unavailable", HealthDigestView{Quality: QualityLine{Unavailable: 1}, Profit: ProfitLine{TotalAccounts: 1}}, "red"},
		{"data unavailable", HealthDigestView{Quality: QualityLine{DataUnavailable: true, DataUnavailableReason: "read failed"}}, "orange"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RenderHealthDigest(tc.view).Card.Header.Template; got != tc.want {
				t.Fatalf("template = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderHealthDigestDeduplicatesPendingBySeverity(t *testing.T) {
	view := HealthDigestView{
		Date: "2026-07-27",
		Quality: QualityLine{Healthy: 1, Unavailable: 1},
		Profit: ProfitLine{TotalAccounts: 2, PricedAccounts: 1},
		Pending: []PendingItem{
			{AccountID: 41, AccountName: "A", Problem: "倍率不可用", Severity: PendingAccounting},
			{AccountID: 41, AccountName: "A", Problem: "余额耗尽", Severity: PendingCritical},
			{AccountID: 42, AccountName: "B", Problem: "HTTP 错误", Severity: PendingWarning},
		},
	}
	text := renderText(t, RenderHealthDigest(view))
	if strings.Count(text, "｜A") != 1 || !strings.Contains(text, "余额耗尽") {
		t.Fatalf("highest-severity item must win once: %s", text)
	}
	if strings.Index(text, "严重｜A") > strings.Index(text, "注意｜B") {
		t.Fatalf("critical must render before warning: %s", text)
	}
}

func TestRenderHealthDigestDataUnavailableDoesNotClaimHealthy(t *testing.T) {
	view := HealthDigestView{
		Date: "2026-07-27",
		Quality: QualityLine{DataUnavailable: true, DataUnavailableReason: "Sub2API 不可达"},
		Profit: ProfitLine{TotalAccounts: 11},
	}
	text := renderText(t, RenderHealthDigest(view))
	for _, forbidden := range []string{
		"0 个不可用", "经营情况", "需要处理", "无待处理", "其余 11 个账号",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("data-unavailable card contains fake normal %q: %s", forbidden, text)
		}
	}
}

func TestRenderHealthDigestNoTrafficIsCompact(t *testing.T) {
	view := HealthDigestView{
		Date: "2026-07-27",
		Quality: QualityLine{Healthy: 3},
		Profit: ProfitLine{
			NoTraffic: true, TotalAccounts: 3, PricedAccounts: 3,
			UpstreamPricedAccounts: 1,
		},
	}
	text := renderText(t, RenderHealthDigest(view))
	if !strings.Contains(text, "今日无有效流量，利润暂不可核算") {
		t.Fatalf("missing compact no-traffic conclusion: %s", text)
	}
	for _, forbidden := range []string{"错误率", "SLA", "站内流量"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("no-traffic card contains noise %q: %s", forbidden, text)
		}
	}
}

func TestRenderHealthDigestUncomputableProfitIsExplicit(t *testing.T) {
	view := HealthDigestView{
		Date: "2026-07-27",
		Quality: QualityLine{Healthy: 1, Degraded: 2},
		Profit: ProfitLine{
			Computable: false, ExcludedAccounts: 2,
			TotalAccounts: 3, PricedAccounts: 1,
		},
		Traffic: TrafficLine{HasTraffic: true, Requests: 17},
	}
	text := renderText(t, RenderHealthDigest(view))
	for _, want := range []string{
		"利润暂不可核算｜2 个账号倍率不可用",
		"利润覆盖 1/3 个账号｜0 个采用上游公开定价",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run:

```bash
go test ./internal/notify/ -run 'TestRenderHealthDigest(ActionMorningOrder|Template|DeduplicatesPendingBySeverity|DataUnavailableDoesNotClaimHealthy|NoTrafficIsCompact|UncomputableProfitIsExplicit)' -count=1 -v
```

Expected: FAIL；旧标题、蓝色模板、四层明细和未排序行动项不符合新断言。

- [ ] **Step 4: 实现日期与标题颜色**

在 `digest_v2.go` 引入 `sort`、`strings`、`time`，实现：

```go
func morningDate(value string) string {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return digestValue(value)
	}
	return parsed.Format("1月2日")
}

func digestTemplate(view HealthDigestView) string {
	switch {
	case view.Quality.DataUnavailable:
		return "orange"
	case view.Quality.Unavailable > 0:
		return "red"
	case view.Quality.Degraded > 0 || len(view.Pending) > 0 || len(view.Recommendations) > 0:
		return "orange"
	default:
		return "green"
	}
}
```

标题改为：

```go
Header: CardHeader{
	Title: CardText{Tag: "plain_text", Content: "中转站晨报 · " + morningDate(view.Date)},
	Template: digestTemplate(view),
},
```

`RenderHealthDigest` 必须先处理数据不可用分支；该分支只渲染运行概览和按钮，不渲染经营情况、行动项或建议：

```go
elements := []CardElement{
	{Tag: "div", Text: &CardText{
		Tag: "lark_md", Content: fitDigestSection(qualityLines(view.Quality)),
	}},
}
if !view.Quality.DataUnavailable {
	elements = append(elements,
		CardElement{Tag: "div", Text: &CardText{
			Tag: "lark_md", Content: fitDigestSection(profitLines(view.Profit, view.Traffic)),
		}},
		CardElement{Tag: "div", Text: &CardText{
			Tag: "lark_md", Content: fitDigestSection(actionLines(view)),
		}},
	)
	if lines := recommendationLines(view.Recommendations); len(lines) > 0 {
		elements = append(elements, CardElement{Tag: "div", Text: &CardText{
			Tag: "lark_md", Content: fitDigestSection(lines),
		}})
	}
}
```

- [ ] **Step 5: 实现运行概览与经营情况**

将 `qualityLines` 标题和分隔调整为：

```go
lines := []string{"**运行概览**"}
summary := fmt.Sprintf("%d 个稳定｜%d 个降级｜%d 个不可用",
	quality.Healthy, quality.Degraded, quality.Unavailable)
```

第二行：

```go
detail := "P95 中位 " + formatMillis(quality.TTFTP95MedianMS)
if quality.Slow > 0 {
	detail += fmt.Sprintf("｜%d 个偏慢", quality.Slow)
}
```

数据不可用时仅返回标题和错误结论。

把 `profitLines` 签名改为：

```go
func profitLines(profit ProfitLine, traffic TrafficLine) []string
```

实现分支：

```go
lines := []string{"**经营情况**"}
switch {
case profit.NoTraffic:
	lines = append(lines, "今日无有效流量，利润暂不可核算")
case !profit.Computable:
	lines = append(lines, fmt.Sprintf("利润暂不可核算｜%d 个账号倍率不可用", profit.ExcludedAccounts))
default:
	lines = append(lines,
		fmt.Sprintf("请求 %d｜收入 %s｜成本 %s", traffic.Requests, formatUSD(profit.Revenue), formatUSD(profit.UpstreamCost)),
		fmt.Sprintf("毛利 %s｜毛利率 %s", formatUSD(profit.Gross), formatMargin(profit.Margin)),
	)
}
if profit.TotalAccounts > 0 {
	lines = append(lines, fmt.Sprintf("利润覆盖 %d/%d 个账号｜%d 个采用上游公开定价",
		profit.PricedAccounts, profit.TotalAccounts, profit.UpstreamPricedAccounts))
}
return lines
```

- [ ] **Step 6: 实现行动项排序、去重与上限**

增加：

```go
func severityRank(value PendingSeverity) int {
	switch value {
	case PendingCritical:
		return 0
	case PendingWarning:
		return 1
	case PendingAccounting:
		return 2
	default:
		return 3
	}
}

func severityLabel(value PendingSeverity) string {
	switch value {
	case PendingCritical:
		return "严重"
	case PendingWarning:
		return "注意"
	case PendingAccounting:
		return "核算"
	default:
		return "注意"
	}
}

func normalizePending(items []PendingItem) []PendingItem {
	indexByAccount := make(map[string]int, len(items))
	normalized := make([]PendingItem, 0, len(items))
	for _, item := range items {
		key := "name:" + strings.TrimSpace(item.AccountName)
		if item.AccountID != 0 {
			key = "id:" + strconv.FormatInt(item.AccountID, 10)
		}
		if index, ok := indexByAccount[key]; ok {
			if severityRank(item.Severity) < severityRank(normalized[index].Severity) {
				normalized[index] = item
			}
			continue
		}
		indexByAccount[key] = len(normalized)
		normalized = append(normalized, item)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return severityRank(normalized[i].Severity) < severityRank(normalized[j].Severity)
	})
	return normalized
}
```

行动区最多显示 8 个唯一账号：

```go
const maxMorningActions = 8

func actionLines(view HealthDigestView) []string {
	items := normalizePending(view.Pending)
	lines := []string{fmt.Sprintf("**需要处理 · %d**", len(items))}
	if len(items) > 0 {
		kept := items
		if len(kept) > maxMorningActions {
			kept = kept[:maxMorningActions]
		}
		for _, item := range kept {
			title := fmt.Sprintf("**%s｜%s**", severityLabel(item.Severity), digestValue(item.AccountName))
			detail := digestValue(item.Problem)
			if rendered := digestValue(item.Detail); rendered != "" {
				detail += "｜" + rendered
			}
			lines = append(lines, title+"\n"+detail)
		}
		if hidden := len(items) - len(kept); hidden > 0 {
			lines = append(lines, fmt.Sprintf("其余 %d 项见运维后台", hidden))
		}
	}
	if !view.Quality.DataUnavailable {
		clear := view.Profit.TotalAccounts - len(items)
		if clear < 0 {
			clear = 0
		}
		lines = append(lines, fmt.Sprintf("其余 %d 个账号无待处理项", clear))
	}
	return lines
}
```

- [ ] **Step 7: 分离调整建议并删除明细**

实现：

```go
func recommendationLines(items []RecommendationLine) []string {
	if len(items) == 0 {
		return nil
	}
	lines := []string{"**调整建议**"}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("%s：建议由 %s 切换到 %s｜%s",
			digestValue(item.GroupName), digestValue(item.CurrentName),
			digestValue(item.CandidateName), digestValue(item.Reason)))
	}
	return lines
}
```

`RenderHealthDigest` 使用 Step 4 的条件化 elements，并始终在末尾保留“运维后台” action。

删除：

- `AccountDetailLine`
- `HealthDigestView.Accounts`
- `detailLines`
- `fitAccountLines`
- `digestValueOrDash`

同时删除 `health.go` 中向 `view.Accounts` 追加全量账号明细的语句，以及 `resolvedMultiplierLabel`、`multiplierLabel`、`grossLabel`、`millis`；确认这些函数无其他调用后再删除。

- [ ] **Step 8: 更新卡片体积与 service 回归测试**

把原 `TestRenderHealthDigestFitsCardLimit` 改为以下测试，创建 320 个账号名唯一的混合严重度 `PendingItem`：

```go
func TestRenderHealthDigestFitsCardLimit(t *testing.T) {
	pending := make([]PendingItem, 320)
	for index := range pending {
		severity := PendingAccounting
		if index%3 == 0 {
			severity = PendingCritical
		} else if index%3 == 1 {
			severity = PendingWarning
		}
		pending[index] = PendingItem{
			AccountID:   int64(index + 1),
			AccountName: fmt.Sprintf("账号-%03d-%s", index, strings.Repeat("A", 40)),
			Problem:     "需要运营处理",
			Detail:      "受控详情",
			Severity:    severity,
		}
	}
	view := HealthDigestView{
		Date: "2026-07-27",
		Quality: QualityLine{Unavailable: 107, Degraded: 107, Healthy: 106},
		Profit: ProfitLine{TotalAccounts: 320, PricedAccounts: 213},
		Pending: pending,
		Traffic: TrafficLine{HasTraffic: true, Requests: 320},
	}

	payload, err := RenderHealthDigest(view).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	if len(payload) > maxCardBytes {
		t.Fatalf("card size = %d, exceeds %d", len(payload), maxCardBytes)
	}
	text := string(payload)
	if !strings.Contains(text, "其余 312 项见运维后台") {
		t.Fatalf("missing exact truncation count: %s", text)
	}
	if !strings.Contains(text, "严重｜") {
		t.Fatalf("critical items must survive truncation: %s", text)
	}
}
```

给测试文件增加标准库 `fmt` import。

更新 `internal/dailyreport/service_test.go`：

- 把 `TestServiceDeduplicatesDailyDeliveryAndKeepsContractStable` 的标题断言改为 `中转站晨报 · 7月20日`；
- 把 `TestServiceRendersZeroQualityCountsFromEmptyMonitorProjection` 的计数断言改为 `0 个稳定｜0 个降级｜0 个不可用`；
- 把 `TestServiceRendersHealthDigestFromMonitorProjection` 的必需内容改为 `1 个稳定｜1 个降级｜0 个不可用`、`需要处理 · 1`、异常账号 `账号 A` 和 `调整建议`，删除要求健康账号逐条出现的断言；
- 保留私有分组脱敏和公开选型建议断言；
- 增加“不包含 `**明细**`”断言。

- [ ] **Step 9: 运行 Task 2 测试**

Run:

```bash
gofmt -w internal/notify/digest_v2.go internal/notify/digest_v2_test.go internal/dailyreport/health.go internal/dailyreport/health_test.go internal/dailyreport/service_test.go
go test ./internal/notify/ ./internal/dailyreport/ -count=1
```

Expected: PASS。

- [ ] **Step 10: 提交**

```bash
git add internal/notify/digest_v2.go internal/notify/digest_v2_test.go \
  internal/dailyreport/health.go internal/dailyreport/health_test.go \
  internal/dailyreport/service_test.go
git commit -m "feat: render a compact action morning digest"
```

---

### Task 3: 账号类告警只显示账号名称

**Files:**
- Modify: `relay-ops-service/internal/opsmonitor/service.go`
- Test: `relay-ops-service/internal/opsmonitor/service_test.go`

**Interfaces:**
- Changes: `object` 增加 `name string`
- Produces: `(object).displayLabel() string`
- Changes: `evaluateMultipliers(ctx context.Context, active map[int64]string, now time.Time) error`
- Preserves: `(object).key(metric string) string`
- Preserves: `evidenceHash(item object, metric, current, baseline string) string` 的 `{kind,id}` 输入语义

- [ ] **Step 1: 写暂停告警名称与内部键失败测试**

扩展现有 active-but-paused 测试或新增：

```go
func TestPausedAlertDisplaysAccountNameButKeepsIDKey(t *testing.T) {
	now := time.Date(2026, 7, 27, 2, 0, 0, 0, time.UTC)
	reader := &fakeReader{
		accounts: []sub2api.Account{
			{ID: 35, Name: "特惠-SHUAI", Status: "active", Schedulable: false},
		},
	}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(notifier.sent))
	}
	if notifier.sent[0].key != "site:account:35:paused" {
		t.Fatalf("key = %q", notifier.sent[0].key)
	}
	text := notifier.sent[0].message.RenderedText()
	if !strings.Contains(text, "站内运行告警：特惠-SHUAI") ||
		!strings.Contains(text, "对象：特惠-SHUAI") {
		t.Fatalf("account name missing: %s", text)
	}
	if strings.Contains(text, "account #35") || strings.Contains(text, "账号 #35") {
		t.Fatalf("database label leaked: %s", text)
	}
}
```

使用标准库 `strings`，不要保留测试文件自制的 `contains` 循环用于新断言。

- [ ] **Step 2: 写展示名不影响身份与空名失败测试**

追加纯函数测试：

```go
func TestObjectDisplayNameDoesNotChangeIdentity(t *testing.T) {
	first := object{kind: "account", id: 35, name: "特惠-SHUAI"}
	renamed := object{kind: "account", id: 35, name: "特惠-SHUAI-新名称"}

	if first.key("paused") != renamed.key("paused") {
		t.Fatal("display name changed incident key")
	}
	firstHash := evidenceHash(first, "paused", "active but paused", "active && schedulable")
	renamedHash := evidenceHash(renamed, "paused", "active but paused", "active && schedulable")
	if firstHash != renamedHash {
		t.Fatal("display name changed evidence hash")
	}
}

func TestRenderAccountWithEmptyNameDoesNotExposeID(t *testing.T) {
	message := render(
		object{kind: "account", id: 35},
		"paused", "active but paused", "active && schedulable",
		1, time.Date(2026, 7, 27, 2, 0, 0, 0, time.UTC), "confirmed",
	)
	text := message.RenderedText()
	if !strings.Contains(text, "账号名称不可用") {
		t.Fatalf("missing empty-name fallback: %s", text)
	}
	if strings.Contains(text, "#35") {
		t.Fatalf("database ID leaked: %s", text)
	}
}
```

- [ ] **Step 3: 写倍率、余额与恢复路径名称失败测试**

写表驱动测试直接覆盖 `render`：

```go
func TestRenderAccountMetricsUseDisplayName(t *testing.T) {
	cases := []struct {
		metric     string
		transition string
		wantTitle string
	}{
		{"availability", "confirmed", "站内运行告警：Pro-SHUAI-0.17"},
		{"balance_exhausted", "confirmed", "上游账号质量告警：Pro-SHUAI-0.17"},
		{"multiplier", "confirmed", "上游账号质量告警：Pro-SHUAI-0.17"},
		{"paused", "recovered", "站内运行已恢复：Pro-SHUAI-0.17"},
	}
	for _, tc := range cases {
		t.Run(tc.metric+"/"+tc.transition, func(t *testing.T) {
			message := render(
				object{kind: "account", id: 21, name: "Pro-SHUAI-0.17"},
				tc.metric, "current", "baseline", 1,
				time.Date(2026, 7, 27, 2, 0, 0, 0, time.UTC), tc.transition,
			)
			text := message.RenderedText()
			if !strings.Contains(text, tc.wantTitle) ||
				!strings.Contains(text, "对象：Pro-SHUAI-0.17") {
				t.Fatalf("name missing: %s", text)
			}
		})
	}
}
```

- [ ] **Step 4: 运行测试确认失败**

Run:

```bash
go test ./internal/opsmonitor/ -run 'Test(PausedAlertDisplaysAccountNameButKeepsIDKey|ObjectDisplayNameDoesNotChangeIdentity|RenderAccountWithEmptyNameDoesNotExposeID|RenderAccountMetricsUseDisplayName)' -count=1 -v
```

Expected: 编译失败（`object.name` / `displayLabel` 不存在）或断言看到 `account #35`。

- [ ] **Step 5: 实现展示标签而不改变身份**

给 `object` 增加名称并实现：

```go
type object struct {
	kind string
	id   int64
	name string
}

func (o object) displayLabel() string {
	if o.kind == "account" {
		if name := strings.TrimSpace(o.name); name != "" {
			return name
		}
		return "账号名称不可用"
	}
	return o.kind + " #" + strconv.FormatInt(o.id, 10)
}
```

`key` 保持原样：

```go
func (o object) key(metric string) string {
	return "site:" + o.kind + ":" + strconv.FormatInt(o.id, 10) + ":" + metric
}
```

`evidenceHash` 保持只拼接 `kind` 和 `id`，不得加入 `name`。

把 `render` 第一行改为：

```go
label := item.displayLabel()
```

- [ ] **Step 6: 统一从 ListAccounts 传递账号名**

把 active 集合改成名称映射：

```go
active := make(map[int64]string, len(accounts))
```

账号循环内统一创建：

```go
item := object{kind: "account", id: account.ID, name: account.Name}
```

并用于 paused 和 runtime：

```go
if err := s.observe(
	ctx, item, "paused", !account.Schedulable,
	"active && schedulable", schedulingValue(account), 1, now,
); err != nil {
	return err
}
if !account.Schedulable {
	continue
}
active[account.ID] = account.Name

window, baseline, available := s.snapshots(ctx, sub2api.OpsQuery{AccountID: account.ID})
if !available {
	continue
}
if err := s.evaluateRuntime(ctx, item, window, baseline, now); err != nil {
	return err
}
```

质量结果路径：

```go
name, ok := active[account.AccountID]
if !ok {
	continue
}
item := object{kind: "account", id: account.AccountID, name: name}
```

倍率路径签名改为：

```go
func (s Service) evaluateMultipliers(ctx context.Context, active map[int64]string, now time.Time) error
```

倍率投影循环只用投影名称做 fallback lookup；展示名称来自 active 映射：

```go
displayName, ok := active[account.AccountID]
if !ok {
	continue
}
// resolve value using account.Name as before
item := object{kind: "account", id: account.AccountID, name: displayName}
```

`activeIDs` 继续按排序后的 `ListAccounts` 生成，account-quality set hash 行为不变。

- [ ] **Step 7: 运行 opsmonitor 全量测试**

Run:

```bash
gofmt -w internal/opsmonitor/service.go internal/opsmonitor/service_test.go
go test ./internal/opsmonitor/ -count=1
```

Expected: PASS。现有 group 对象仍允许 `group #7`；不要把私有分组名称传入通用告警。

- [ ] **Step 8: 提交**

```bash
git add internal/opsmonitor/service.go internal/opsmonitor/service_test.go
git commit -m "fix: show account names in runtime alerts"
```

---

### Task 4: 全量回归、生产部署与验收

**Files:**
- Modify only if a regression test exposes a scoped defect in Tasks 1-3
- Verify: `relay-ops-service/**`
- Verify: `tests/relay_ops/validate_relay_ops_contract.sh`
- Verify: `tests/infra/validate-baseline.sh`

**Interfaces:**
- Consumes: Tasks 1-3 的已提交实现
- Produces: 通过门禁的 `feat/action-morning-digest`
- Produces: 只重建 `relay-ops` 的生产镜像

- [ ] **Step 1: 运行格式与静态差异检查**

Run:

```bash
gofmt -w internal/notify/digest_v2.go internal/notify/digest_v2_test.go \
  internal/dailyreport/health.go internal/dailyreport/health_test.go \
  internal/dailyreport/service_test.go \
  internal/opsmonitor/service.go internal/opsmonitor/service_test.go
git diff --check
git status --short
```

Expected: `git diff --check` 无输出；`git status --short` 只列出本计划涉及且尚未提交的文件。若 Tasks 1-3 已逐项提交，应无输出。

- [ ] **Step 2: 运行完整本地门禁**

Run from `relay-ops-service/`:

```bash
go build ./...
go vet ./...
go test ./... -count=1
```

Run from repository root:

```bash
bash tests/relay_ops/validate_relay_ops_contract.sh
bash tests/infra/validate-baseline.sh
```

Expected:

- build exit 0；
- vet exit 0；
- 所有 Go package PASS；
- `PASS: relay-ops container and routing contracts`；
- `PASS: infrastructure baseline contracts`。

- [ ] **Step 3: 审查分支范围**

Run:

```bash
git log --oneline main..HEAD
git diff --stat main...HEAD
git diff --name-status main...HEAD
git status --short --branch
```

Expected:

- 设计提交、行动数据提交、行动渲染提交、账号名告警提交；
- 代码范围只包含规格、计划、`dailyreport`、`notify`、`opsmonitor` 及相应测试；
- worktree 干净。

- [ ] **Step 4: 按项目约定合并回 main**

在主工作区：

```bash
git checkout main
git pull --ff-only
git merge --ff-only feat/action-morning-digest
```

合并后从主工作区 `relay-ops-service/` 再运行：

```bash
go build ./...
go vet ./...
go test ./... -count=1
```

Expected: 全部 exit 0。任何失败都停止部署，保留 worktree 和分支用于修复。

- [ ] **Step 5: 清理已合并 worktree**

仅在合并后测试通过时：

```bash
git worktree remove /Users/gongtengxinwen/Documents/sub2api-action-morning-digest
git branch -d feat/action-morning-digest
git worktree list
git branch --format='%(refname:short)'
```

Expected: 只剩主工作区和 `main`。

- [ ] **Step 6: 记录生产部署前基线**

Run:

```bash
ssh sub2api-prod 'sudo docker ps --format "{{.Names}}={{.ID}}" | sort'
ssh sub2api-prod 'sudo docker inspect sub2api-relay-ops-1 --format "image={{.Config.Image}} health={{.State.Health.Status}} restarts={{.RestartCount}}"'
```

Expected:

- `sub2api-relay-ops-1` 健康；
- 记录 Sub2API、PostgreSQL、Redis、Caddy 的容器 ID，部署后必须相同。

- [ ] **Step 7: 从 Git 树同步 relay-ops 源码**

从主工作区执行：

```bash
git archive --format=tar main relay-ops-service \
  | ssh sub2api-prod 'tee /tmp/action-morning-deploy.tar >/dev/null'
```

在生产机解包：

```bash
ssh sub2api-prod 'sudo bash -s' <<'REMOTE'
set -eu
rm -rf /tmp/action-morning-deploy
mkdir -p /tmp/action-morning-deploy
tar -xf /tmp/action-morning-deploy.tar -C /tmp/action-morning-deploy
rsync -a --delete /tmp/action-morning-deploy/relay-ops-service/ /opt/sub2api/production/relay-ops-service/
rm -rf /tmp/action-morning-deploy /tmp/action-morning-deploy.tar
REMOTE
```

不得 rsync 本地工作区。

- [ ] **Step 8: 构建唯一生产镜像**

在本地取得实现提交短哈希并直接构建唯一标签：

```bash
ACTION_DIGEST_REV=$(git rev-parse --short=7 main)
ACTION_DIGEST_IMAGE="sub2api-relay-ops:action-morning-${ACTION_DIGEST_REV}-20260727-v1"
printf '%s\n' "$ACTION_DIGEST_IMAGE"
ssh sub2api-prod "cd /opt/sub2api/production && sudo docker build \
  -f infra/Dockerfile.relay-ops \
  -t '${ACTION_DIGEST_IMAGE}' ."
```

禁止使用 `latest`。构建命令必须 exit 0，且输出的最终镜像标签与 `ACTION_DIGEST_IMAGE` 一致。

- [ ] **Step 9: 备份 Compose 并只重建 relay-ops**

重新计算本次镜像标签，并在生产机执行带前置条件的最小替换：

```bash
ACTION_DIGEST_REV=$(git rev-parse --short=7 main)
ACTION_DIGEST_IMAGE="sub2api-relay-ops:action-morning-${ACTION_DIGEST_REV}-20260727-v1"
ssh sub2api-prod "sudo bash -s -- '${ACTION_DIGEST_IMAGE}'" <<'REMOTE'
set -eu
NEW_IMAGE=$1
ROOT=/opt/sub2api/production
BACKUP=$ROOT/compose.yaml.bak-before-action-morning-20260727
cd "$ROOT"
test ! -e "$BACKUP"
test "$(grep -Fc 'image: sub2api-relay-ops:upstream-group-mapping-8ecc6ce-20260727-v1' compose.yaml)" -eq 1
cp compose.yaml "$BACKUP"
sed -i "s|image: sub2api-relay-ops:upstream-group-mapping-8ecc6ce-20260727-v1|image: ${NEW_IMAGE}|" compose.yaml
docker compose --env-file .env -f compose.yaml config --quiet
docker compose --env-file .env -f compose.yaml up -d --no-deps --no-build --force-recreate relay-ops
REMOTE
```

备份已存在、旧镜像行不是唯一匹配或 Compose 解析失败时脚本必须停止。若解析失败发生在替换后，恢复命令为：

```bash
ssh sub2api-prod 'cd /opt/sub2api/production && sudo cp compose.yaml.bak-before-action-morning-20260727 compose.yaml'
```

- [ ] **Step 10: 验证生产运行与无意外告警**

Run:

```bash
ssh sub2api-prod 'sudo docker inspect sub2api-relay-ops-1 --format "image={{.Config.Image}} health={{.State.Health.Status}} restarts={{.RestartCount}}"'
ssh sub2api-prod 'sudo docker exec sub2api-caddy-1 wget -qO- http://relay-ops:8100/healthz'
ssh sub2api-prod 'sudo docker ps --format "{{.Names}}={{.ID}}" | sort'
ssh sub2api-prod 'sudo docker logs --since 10m sub2api-relay-ops-1 2>&1 | tail -120'
```

Expected:

- 新 action-morning 镜像；
- health `healthy`、restarts `0`；
- healthz `{"status":"alive"}`；
- 只有 relay-ops 容器 ID 改变；
- 日志无 panic、CardJSON 超限或通知渲染错误。

不要修改 incident 记录、不要制造 paused/余额/倍率故障、不要绕过去重发送重复日报。

- [ ] **Step 11: 验证下一次自然通知**

下一次自然账号告警必须满足：

- 标题使用账号名称；
- 正文“对象”使用账号名称；
- 数据库 incident key 仍为实际 ID 形式，例如 `site:account:35:paused`。

下一次正常 09:02 日报必须满足：

- 标题为 `中转站晨报 · M月D日`；
- 首屏可见运行概览、经营情况和行动项数量；
- 无“明细”全量列表；
- 异常账号只出现一次；
- 上游定价账号数可见；
- 移动端无需滚动长账号清单才能读到结论。

时间条件未到时，明确记录为“待自然通知验证”，不得以单元测试冒充飞书真实渲染已验收。

## 回滚

仅回滚 relay-ops：

```bash
ssh sub2api-prod 'cd /opt/sub2api/production && sudo cp compose.yaml.bak-before-action-morning-20260727 compose.yaml && sudo docker compose --env-file .env -f compose.yaml up -d --no-deps --no-build --force-recreate relay-ops'
```

回滚后验证：

```bash
ssh sub2api-prod 'sudo docker inspect sub2api-relay-ops-1 --format "image={{.Config.Image}} health={{.State.Health.Status}} restarts={{.RestartCount}}"'
ssh sub2api-prod 'sudo docker exec sub2api-caddy-1 wget -qO- http://relay-ops:8100/healthz'
```
