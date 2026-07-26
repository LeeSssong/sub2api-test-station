# 飞书运营报告重设计 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把混装的 `relay-ops 每日运营摘要` 拆成「中转站运营日报」与「分组可用性告警」两条推送，质量优先于利润，并修正账号名、倍率与成本三处口径错误。

**Architecture:** 新增纯函数包 `accounthealth`，把「原始探测记录 → 健康结论」从渲染层彻底剥离；`sub2api.HTTPReader` 扩展历史查询接口供日切片聚合；`notify` 增加两个渲染函数；`dailyreport` 只做编排；`scheduler` 增加 5 分钟间隔的分组可用性作业。

**Tech Stack:** Go 1.24（module `example.invalid/relay-ops-service`）、标准库 `testing`、Ruby（仅 `ops/collect-account-quality-pulse.rb` 清理）。

## Global Constraints

- 账号一律使用 Sub2API 返回的 `Name`，任何面向用户的输出都不得出现数据库 ID。
- 禁止使用 `today_stats.cost`，其乘数为已废弃的 `account_rate_multiplier`（生产实测恒为 1）。上游成本一律用 `standard_cost × 可信倍率`。
- 倍率不可用时毛利标记「无法核算」，**不得以 1 兜底**，且不参与汇总。
- 三档阈值：健康 `success_rate >= 0.95`；降级 `0.50 <= success_rate < 0.95`；不可用 `success_rate < 0.50` 或 `error_code == "balance_exhausted"`。
- 延时标记阈值：`TTFT P95 > 3000ms` 标记偏慢，**偏慢不降档**。
- 分组告警按容量分档：总数 `>= 3` 时可用 `<= 1` 告警；总数 `== 2` 或 `== 1` 时可用 `== 0` 才告警。
- fail-safe：数据不可用时日报明示、告警静默，不得因监控自身故障推送业务故障。
- 卡片渲染输出不得包含 API Key、Base URL、上游原始错误。
- 金额单位沿用 Sub2API 原生口径 USD，以 `$` 呈现，不做汇率换算。
- `sub2api.HTTPReader.doStrict` 启用 `DisallowUnknownFields`，新增响应类型必须精确覆盖服务端返回的全部字段。
- 所有 Go 测试在 `relay-ops-service/` 目录下运行。

---

### Task 1: accounthealth 三档判定与延时标记

**Files:**
- Create: `relay-ops-service/internal/accounthealth/classify.go`
- Test: `relay-ops-service/internal/accounthealth/classify_test.go`

**Interfaces:**
- Produces: `Tier` 常量 `TierHealthy` / `TierDegraded` / `TierUnavailable` / `TierUnknown`
- Produces: `AccountSample{AccountID int64, Name string, GroupNames []string, SuccessRate float64, SampleCount int, TTFTP95MS *float64, ErrorCode string}`
- Produces: `AccountVerdict{AccountID int64, Name string, GroupNames []string, Tier Tier, Slow bool}`
- Produces: `ClassifyAccount(AccountSample) AccountVerdict`
- Produces: 常量 `HealthyMinSuccessRate = 0.95`、`DegradedMinSuccessRate = 0.50`、`SlowTTFTP95MS = 3000.0`、`ErrorCodeBalanceExhausted = "balance_exhausted"`

`SampleCount == 0` 判为 `TierUnknown`——无样本无法断言健康与否，后续分组计算中既不计入分子也不计入分母。

- [ ] **Step 1: 写失败测试**

创建 `relay-ops-service/internal/accounthealth/classify_test.go`：

```go
package accounthealth

import "testing"

func float64Ptr(v float64) *float64 { return &v }

func TestClassifyAccount(t *testing.T) {
	cases := []struct {
		name     string
		sample   AccountSample
		wantTier Tier
		wantSlow bool
	}{
		{"健康下边界", AccountSample{SuccessRate: 0.95, SampleCount: 10}, TierHealthy, false},
		{"健康上方", AccountSample{SuccessRate: 0.951, SampleCount: 10}, TierHealthy, false},
		{"降级上边界", AccountSample{SuccessRate: 0.949, SampleCount: 10}, TierDegraded, false},
		{"降级下边界", AccountSample{SuccessRate: 0.50, SampleCount: 10}, TierDegraded, false},
		{"不可用上边界", AccountSample{SuccessRate: 0.499, SampleCount: 10}, TierUnavailable, false},
		{"余额耗尽直判不可用", AccountSample{SuccessRate: 1.0, SampleCount: 10, ErrorCode: ErrorCodeBalanceExhausted}, TierUnavailable, false},
		{"无样本判未知", AccountSample{SuccessRate: 0, SampleCount: 0}, TierUnknown, false},
		{"延时边界内不标记", AccountSample{SuccessRate: 1.0, SampleCount: 10, TTFTP95MS: float64Ptr(3000)}, TierHealthy, false},
		{"延时超阈值标记", AccountSample{SuccessRate: 1.0, SampleCount: 10, TTFTP95MS: float64Ptr(3000.1)}, TierHealthy, true},
		{"偏慢不降档", AccountSample{SuccessRate: 0.99, SampleCount: 10, TTFTP95MS: float64Ptr(9000)}, TierHealthy, true},
		{"延时缺失不标记", AccountSample{SuccessRate: 1.0, SampleCount: 10, TTFTP95MS: nil}, TierHealthy, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyAccount(tc.sample)
			if got.Tier != tc.wantTier {
				t.Fatalf("Tier = %q, want %q", got.Tier, tc.wantTier)
			}
			if got.Slow != tc.wantSlow {
				t.Fatalf("Slow = %v, want %v", got.Slow, tc.wantSlow)
			}
		})
	}
}

func TestClassifyAccountCarriesIdentity(t *testing.T) {
	got := ClassifyAccount(AccountSample{
		AccountID: 21, Name: "Pro-SHUAI-0.17", GroupNames: []string{"GPT-Pro"},
		SuccessRate: 0.98, SampleCount: 100,
	})
	if got.AccountID != 21 || got.Name != "Pro-SHUAI-0.17" {
		t.Fatalf("identity lost: %+v", got)
	}
	if len(got.GroupNames) != 1 || got.GroupNames[0] != "GPT-Pro" {
		t.Fatalf("GroupNames = %v", got.GroupNames)
	}
}
```

- [ ] **Step 2: 验证测试失败**

Run: `cd relay-ops-service && go test ./internal/accounthealth/ -run TestClassifyAccount -v`
Expected: 编译失败，提示 `undefined: AccountSample`、`undefined: ClassifyAccount`

- [ ] **Step 3: 写最小实现**

创建 `relay-ops-service/internal/accounthealth/classify.go`：

```go
// Package accounthealth turns raw Sub2API account monitor evidence into
// operator-facing health verdicts. Every function here is pure: no IO, no
// clock reads beyond explicitly passed values.
package accounthealth

type Tier string

const (
	TierHealthy     Tier = "healthy"
	TierDegraded    Tier = "degraded"
	TierUnavailable Tier = "unavailable"
	TierUnknown     Tier = "unknown"
)

const (
	HealthyMinSuccessRate     = 0.95
	DegradedMinSuccessRate    = 0.50
	SlowTTFTP95MS             = 3000.0
	ErrorCodeBalanceExhausted = "balance_exhausted"
)

type AccountSample struct {
	AccountID   int64
	Name        string
	GroupNames  []string
	SuccessRate float64
	SampleCount int
	TTFTP95MS   *float64
	ErrorCode   string
}

type AccountVerdict struct {
	AccountID  int64
	Name       string
	GroupNames []string
	Tier       Tier
	Slow       bool
}

func ClassifyAccount(sample AccountSample) AccountVerdict {
	verdict := AccountVerdict{
		AccountID:  sample.AccountID,
		Name:       sample.Name,
		GroupNames: sample.GroupNames,
		Tier:       tierFor(sample),
	}
	verdict.Slow = sample.TTFTP95MS != nil && *sample.TTFTP95MS > SlowTTFTP95MS
	return verdict
}

func tierFor(sample AccountSample) Tier {
	if sample.SampleCount <= 0 {
		return TierUnknown
	}
	if sample.ErrorCode == ErrorCodeBalanceExhausted {
		return TierUnavailable
	}
	switch {
	case sample.SuccessRate >= HealthyMinSuccessRate:
		return TierHealthy
	case sample.SuccessRate >= DegradedMinSuccessRate:
		return TierDegraded
	default:
		return TierUnavailable
	}
}
```

- [ ] **Step 4: 验证测试通过**

Run: `cd relay-ops-service && go test ./internal/accounthealth/ -v`
Expected: PASS，全部子测试通过

- [ ] **Step 5: 提交**

```bash
git add relay-ops-service/internal/accounthealth/classify.go relay-ops-service/internal/accounthealth/classify_test.go
git commit -m "feat: add account health tier classification"
```

---

### Task 2: accounthealth 日切片聚合

**Files:**
- Create: `relay-ops-service/internal/accounthealth/dayslice.go`
- Test: `relay-ops-service/internal/accounthealth/dayslice_test.go`

**Interfaces:**
- Consumes: 无（纯函数，不依赖 Task 1）
- Produces: `HistoryEntry{CheckedAt time.Time, Status string, ErrorCode string, TTFTMS *float64}`
- Produces: `DaySlice{Date string, SampleCount int, SuccessCount int, SuccessRate float64, TTFTP95MS *float64, LastErrorCode string}`
- Produces: `SliceByDay(entries []HistoryEntry, loc *time.Location, now time.Time) (today DaySlice, yesterday DaySlice)`
- Produces: `HistoryLimitFor(intervalSeconds int) int`

`SliceByDay` 按 `loc` 时区的自然日归属。`Status == "success"` 计为成功。`LastErrorCode` 取该日最新一条失败记录的错误码。`TTFTP95MS` 仅由成功记录计算，无成功记录时为 `nil`。

`HistoryLimitFor` 按探测间隔动态计算覆盖 48 小时所需条数并上浮 20%，下限 100、上限 2000；`intervalSeconds <= 0` 时回退为 300 秒。

- [ ] **Step 1: 写失败测试**

创建 `relay-ops-service/internal/accounthealth/dayslice_test.go`：

```go
package accounthealth

import (
	"testing"
	"time"
)

func TestHistoryLimitFor(t *testing.T) {
	cases := []struct {
		interval int
		want     int
	}{
		{300, 691},  // ceil(172800/300)=576, *1.2=691.2 -> 691
		{0, 691},    // 非法值回退 300 秒
		{-5, 691},   // 非法值回退 300 秒
		{60, 2000},  // ceil(172800/60)=2880, *1.2=3456 -> 上限 2000
		{86400, 100}, // ceil(172800/86400)=2, *1.2=2.4 -> 下限 100
	}
	for _, tc := range cases {
		if got := HistoryLimitFor(tc.interval); got != tc.want {
			t.Fatalf("HistoryLimitFor(%d) = %d, want %d", tc.interval, got, tc.want)
		}
	}
}

func TestSliceByDaySplitsOnLocalMidnight(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, loc)
	entries := []HistoryEntry{
		{CheckedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, loc), Status: "success", TTFTMS: float64Ptr(1000)},
		{CheckedAt: time.Date(2026, 7, 27, 8, 0, 0, 0, loc), Status: "failed", ErrorCode: "http_error"},
		{CheckedAt: time.Date(2026, 7, 26, 23, 59, 59, 0, loc), Status: "success", TTFTMS: float64Ptr(2000)},
		{CheckedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, loc), Status: "success", TTFTMS: float64Ptr(3000)},
	}
	today, yesterday := SliceByDay(entries, loc, now)
	if today.Date != "2026-07-27" || today.SampleCount != 2 || today.SuccessCount != 1 {
		t.Fatalf("today = %+v", today)
	}
	if today.SuccessRate != 0.5 {
		t.Fatalf("today.SuccessRate = %v, want 0.5", today.SuccessRate)
	}
	if today.LastErrorCode != "http_error" {
		t.Fatalf("today.LastErrorCode = %q", today.LastErrorCode)
	}
	if yesterday.Date != "2026-07-26" || yesterday.SampleCount != 1 {
		t.Fatalf("yesterday = %+v", yesterday)
	}
	// 7-25 的记录既不属于今天也不属于昨天
}

func TestSliceByDayTTFTFromSuccessesOnly(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, loc)
	entries := []HistoryEntry{
		{CheckedAt: time.Date(2026, 7, 27, 1, 0, 0, 0, loc), Status: "success", TTFTMS: float64Ptr(100)},
		{CheckedAt: time.Date(2026, 7, 27, 2, 0, 0, 0, loc), Status: "success", TTFTMS: float64Ptr(900)},
		{CheckedAt: time.Date(2026, 7, 27, 3, 0, 0, 0, loc), Status: "failed", ErrorCode: "timeout", TTFTMS: float64Ptr(99999)},
	}
	today, _ := SliceByDay(entries, loc, now)
	if today.TTFTP95MS == nil {
		t.Fatal("TTFTP95MS is nil")
	}
	if *today.TTFTP95MS != 900 {
		t.Fatalf("TTFTP95MS = %v, want 900 (失败记录必须排除)", *today.TTFTP95MS)
	}
}

func TestSliceByDayEmptyAndNoSuccess(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, loc)

	today, yesterday := SliceByDay(nil, loc, now)
	if today.SampleCount != 0 || yesterday.SampleCount != 0 {
		t.Fatalf("empty input produced samples: %+v %+v", today, yesterday)
	}
	if today.TTFTP95MS != nil {
		t.Fatal("empty input must yield nil TTFTP95MS")
	}

	onlyFailures := []HistoryEntry{
		{CheckedAt: time.Date(2026, 7, 27, 1, 0, 0, 0, loc), Status: "failed", ErrorCode: "balance_exhausted"},
	}
	today, _ = SliceByDay(onlyFailures, loc, now)
	if today.SuccessRate != 0 || today.TTFTP95MS != nil {
		t.Fatalf("all-failure slice = %+v", today)
	}
	if today.LastErrorCode != "balance_exhausted" {
		t.Fatalf("LastErrorCode = %q", today.LastErrorCode)
	}
}
```

- [ ] **Step 2: 验证测试失败**

Run: `cd relay-ops-service && go test ./internal/accounthealth/ -run 'TestHistoryLimitFor|TestSliceByDay' -v`
Expected: 编译失败，提示 `undefined: HistoryEntry`、`undefined: SliceByDay`、`undefined: HistoryLimitFor`

- [ ] **Step 3: 写最小实现**

创建 `relay-ops-service/internal/accounthealth/dayslice.go`：

```go
package accounthealth

import (
	"math"
	"sort"
	"time"
)

const (
	historyWindowSeconds  = 48 * 60 * 60
	historyLimitSlack     = 1.2
	historyLimitMin       = 100
	historyLimitMax       = 2000
	defaultIntervalSecond = 300
	statusSuccess         = "success"
	dateLayout            = "2006-01-02"
)

type HistoryEntry struct {
	CheckedAt time.Time
	Status    string
	ErrorCode string
	TTFTMS    *float64
}

type DaySlice struct {
	Date          string
	SampleCount   int
	SuccessCount  int
	SuccessRate   float64
	TTFTP95MS     *float64
	LastErrorCode string
}

// HistoryLimitFor sizes the history page so that a full 48-hour window is
// covered at the current probe interval. Hard-coding the count would silently
// drop yesterday's samples once an administrator shortens the interval.
func HistoryLimitFor(intervalSeconds int) int {
	if intervalSeconds <= 0 {
		intervalSeconds = defaultIntervalSecond
	}
	needed := int(math.Ceil(float64(historyWindowSeconds) / float64(intervalSeconds)))
	limit := int(float64(needed) * historyLimitSlack)
	if limit < historyLimitMin {
		return historyLimitMin
	}
	if limit > historyLimitMax {
		return historyLimitMax
	}
	return limit
}

func SliceByDay(entries []HistoryEntry, loc *time.Location, now time.Time) (DaySlice, DaySlice) {
	if loc == nil {
		loc = time.UTC
	}
	todayDate := now.In(loc).Format(dateLayout)
	yesterdayDate := now.In(loc).AddDate(0, 0, -1).Format(dateLayout)

	buckets := map[string][]HistoryEntry{}
	for _, entry := range entries {
		date := entry.CheckedAt.In(loc).Format(dateLayout)
		if date == todayDate || date == yesterdayDate {
			buckets[date] = append(buckets[date], entry)
		}
	}
	return summarize(todayDate, buckets[todayDate]), summarize(yesterdayDate, buckets[yesterdayDate])
}

func summarize(date string, entries []HistoryEntry) DaySlice {
	slice := DaySlice{Date: date, SampleCount: len(entries)}
	if len(entries) == 0 {
		return slice
	}
	ordered := append([]HistoryEntry(nil), entries...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].CheckedAt.Before(ordered[j].CheckedAt)
	})

	ttfts := make([]float64, 0, len(ordered))
	for _, entry := range ordered {
		if entry.Status == statusSuccess {
			slice.SuccessCount++
			if entry.TTFTMS != nil {
				ttfts = append(ttfts, *entry.TTFTMS)
			}
			continue
		}
		if entry.ErrorCode != "" {
			slice.LastErrorCode = entry.ErrorCode
		}
	}
	slice.SuccessRate = float64(slice.SuccessCount) / float64(slice.SampleCount)
	slice.TTFTP95MS = percentile(ttfts, 0.95)
	return slice
}

func percentile(values []float64, q float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(q*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	value := sorted[index]
	return &value
}
```

- [ ] **Step 4: 验证测试通过**

Run: `cd relay-ops-service && go test ./internal/accounthealth/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add relay-ops-service/internal/accounthealth/dayslice.go relay-ops-service/internal/accounthealth/dayslice_test.go
git commit -m "feat: add per-day account history slicing"
```

---

### Task 3: accounthealth 分组可用性与告警判定

**Files:**
- Create: `relay-ops-service/internal/accounthealth/group.go`
- Test: `relay-ops-service/internal/accounthealth/group_test.go`

**Interfaces:**
- Consumes: `AccountVerdict`、`Tier` 常量（Task 1）
- Produces: `GroupAvailability{GroupName string, Total int, Available int, Alerting bool, Down []AccountVerdict}`
- Produces: `GroupAvailabilities(verdicts []AccountVerdict) []GroupAvailability`
- Produces: `ShouldAlert(total, available int) bool`

`Available` = 该分组内 `TierHealthy` + `TierDegraded` 账号数。`Total` 排除 `TierUnknown` 账号（无样本不参与容量判定）。`Down` 列出该分组内 `TierUnavailable` 账号。结果按 `GroupName` 升序，保证输出稳定。

- [ ] **Step 1: 写失败测试**

创建 `relay-ops-service/internal/accounthealth/group_test.go`：

```go
package accounthealth

import "testing"

func TestShouldAlertByCapacity(t *testing.T) {
	cases := []struct {
		total, available int
		want             bool
	}{
		{4, 2, false}, {4, 1, true}, {4, 0, true},
		{3, 2, false}, {3, 1, true}, {3, 0, true},
		{2, 2, false}, {2, 1, false}, {2, 0, true},
		{1, 1, false}, {1, 0, true},
		{0, 0, false},
	}
	for _, tc := range cases {
		if got := ShouldAlert(tc.total, tc.available); got != tc.want {
			t.Fatalf("ShouldAlert(%d,%d) = %v, want %v", tc.total, tc.available, got, tc.want)
		}
	}
}

func TestGroupAvailabilitiesAggregates(t *testing.T) {
	verdicts := []AccountVerdict{
		{Name: "Plus-TK-0.08", GroupNames: []string{"GPT-Plus"}, Tier: TierDegraded},
		{Name: "Plus-XN-0.09", GroupNames: []string{"GPT-Plus"}, Tier: TierUnavailable},
		{Name: "Plus-XM-0.1", GroupNames: []string{"GPT-Plus"}, Tier: TierUnavailable},
		{Name: "Pro-SHEN-0.16", GroupNames: []string{"GPT-Pro"}, Tier: TierHealthy},
		{Name: "Pro-TK-0.15", GroupNames: []string{"GPT-Pro"}, Tier: TierHealthy},
		{Name: "Pro-SHUAI-0.17", GroupNames: []string{"GPT-Pro"}, Tier: TierHealthy},
	}
	groups := GroupAvailabilities(verdicts)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].GroupName != "GPT-Plus" || groups[1].GroupName != "GPT-Pro" {
		t.Fatalf("groups not sorted: %+v", groups)
	}
	plus := groups[0]
	if plus.Total != 3 || plus.Available != 1 || !plus.Alerting {
		t.Fatalf("GPT-Plus = %+v, want total 3 available 1 alerting", plus)
	}
	if len(plus.Down) != 2 {
		t.Fatalf("GPT-Plus Down = %+v, want 2", plus.Down)
	}
	pro := groups[1]
	if pro.Total != 3 || pro.Available != 3 || pro.Alerting {
		t.Fatalf("GPT-Pro = %+v", pro)
	}
}

func TestGroupAvailabilitiesSingleAccountGroupNotAlerting(t *testing.T) {
	verdicts := []AccountVerdict{
		{Name: "GPT特惠-TK-0.08", GroupNames: []string{"GPT特惠", "GPT-PLUS-内测"}, Tier: TierDegraded},
	}
	groups := GroupAvailabilities(verdicts)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	for _, group := range groups {
		if group.Total != 1 || group.Available != 1 {
			t.Fatalf("%s = %+v", group.GroupName, group)
		}
		if group.Alerting {
			t.Fatalf("%s 单账号分组可用时不得告警", group.GroupName)
		}
	}
}

func TestGroupAvailabilitiesExcludesUnknown(t *testing.T) {
	verdicts := []AccountVerdict{
		{Name: "A", GroupNames: []string{"G"}, Tier: TierHealthy},
		{Name: "B", GroupNames: []string{"G"}, Tier: TierUnknown},
	}
	groups := GroupAvailabilities(verdicts)
	if groups[0].Total != 1 || groups[0].Available != 1 {
		t.Fatalf("unknown 账号必须排除在容量之外: %+v", groups[0])
	}
	if groups[0].Alerting {
		t.Fatal("单账号分组且可用，不得告警")
	}
}

func TestGroupAvailabilitiesIgnoresAccountsWithoutGroup(t *testing.T) {
	verdicts := []AccountVerdict{{Name: "orphan", GroupNames: nil, Tier: TierHealthy}}
	if groups := GroupAvailabilities(verdicts); len(groups) != 0 {
		t.Fatalf("groups = %+v, want empty", groups)
	}
}
```

- [ ] **Step 2: 验证测试失败**

Run: `cd relay-ops-service && go test ./internal/accounthealth/ -run 'TestShouldAlert|TestGroupAvailabilities' -v`
Expected: 编译失败，提示 `undefined: ShouldAlert`、`undefined: GroupAvailabilities`

- [ ] **Step 3: 写最小实现**

创建 `relay-ops-service/internal/accounthealth/group.go`：

```go
package accounthealth

import "sort"

type GroupAvailability struct {
	GroupName string
	Total     int
	Available int
	Alerting  bool
	Down      []AccountVerdict
}

// ShouldAlert encodes capacity-tiered thresholds. A group with three or more
// accounts alerts once it loses redundancy; smaller groups only alert when
// nothing is left, so single-account groups never alert while they work.
func ShouldAlert(total, available int) bool {
	if total <= 0 {
		return false
	}
	if total >= 3 {
		return available <= 1
	}
	return available == 0
}

func GroupAvailabilities(verdicts []AccountVerdict) []GroupAvailability {
	byGroup := map[string]*GroupAvailability{}
	for _, verdict := range verdicts {
		if verdict.Tier == TierUnknown {
			continue
		}
		for _, name := range verdict.GroupNames {
			if name == "" {
				continue
			}
			group, ok := byGroup[name]
			if !ok {
				group = &GroupAvailability{GroupName: name}
				byGroup[name] = group
			}
			group.Total++
			switch verdict.Tier {
			case TierHealthy, TierDegraded:
				group.Available++
			case TierUnavailable:
				group.Down = append(group.Down, verdict)
			}
		}
	}
	groups := make([]GroupAvailability, 0, len(byGroup))
	for _, group := range byGroup {
		group.Alerting = ShouldAlert(group.Total, group.Available)
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].GroupName < groups[j].GroupName })
	return groups
}
```

- [ ] **Step 4: 验证测试通过**

Run: `cd relay-ops-service && go test ./internal/accounthealth/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add relay-ops-service/internal/accounthealth/group.go relay-ops-service/internal/accounthealth/group_test.go
git commit -m "feat: add group availability and capacity-tiered alerting"
```

---

### Task 4: accounthealth 利润计算

**Files:**
- Create: `relay-ops-service/internal/accounthealth/profit.go`
- Test: `relay-ops-service/internal/accounthealth/profit_test.go`

**Interfaces:**
- Produces: `ProfitInput{StandardCost float64, UserCost float64, Multiplier *float64}`
- Produces: `Profit{Revenue float64, UpstreamCost float64, Gross float64, Margin *float64, Computable bool}`
- Produces: `ComputeProfit(ProfitInput) Profit`
- Produces: `SumProfit(inputs []ProfitInput) (total Profit, excluded int)`

`Multiplier == nil` 表示倍率不可用，`Computable = false`，此时 `UpstreamCost` 与 `Gross` 保持 `0` 且不得参与汇总。`UserCost == 0` 时 `Margin = nil`（不做除零）。`SumProfit` 返回被排除的账号数量供卡片注明。

- [ ] **Step 1: 写失败测试**

创建 `relay-ops-service/internal/accounthealth/profit_test.go`：

```go
package accounthealth

import (
	"math"
	"testing"
)

func TestComputeProfitWithMultiplier(t *testing.T) {
	got := ComputeProfit(ProfitInput{StandardCost: 100, UserCost: 40, Multiplier: float64Ptr(0.15)})
	if !got.Computable {
		t.Fatal("Computable = false, want true")
	}
	if math.Abs(got.UpstreamCost-15) > 1e-9 {
		t.Fatalf("UpstreamCost = %v, want 15", got.UpstreamCost)
	}
	if math.Abs(got.Gross-25) > 1e-9 {
		t.Fatalf("Gross = %v, want 25", got.Gross)
	}
	if got.Margin == nil || math.Abs(*got.Margin-0.625) > 1e-9 {
		t.Fatalf("Margin = %v, want 0.625", got.Margin)
	}
}

func TestComputeProfitWithoutMultiplierIsNotComputable(t *testing.T) {
	got := ComputeProfit(ProfitInput{StandardCost: 100, UserCost: 40, Multiplier: nil})
	if got.Computable {
		t.Fatal("倍率缺失时不得可核算")
	}
	if got.UpstreamCost != 0 || got.Gross != 0 {
		t.Fatalf("倍率缺失时不得以 1 兜底: %+v", got)
	}
	if got.Margin != nil {
		t.Fatalf("Margin = %v, want nil", got.Margin)
	}
}

func TestComputeProfitZeroRevenueHasNoMargin(t *testing.T) {
	got := ComputeProfit(ProfitInput{StandardCost: 0, UserCost: 0, Multiplier: float64Ptr(0.2)})
	if !got.Computable {
		t.Fatal("Computable = false, want true")
	}
	if got.Margin != nil {
		t.Fatalf("Margin = %v, want nil (不得除零)", got.Margin)
	}
}

func TestSumProfitExcludesIncomputable(t *testing.T) {
	total, excluded := SumProfit([]ProfitInput{
		{StandardCost: 100, UserCost: 40, Multiplier: float64Ptr(0.15)},
		{StandardCost: 200, UserCost: 60, Multiplier: float64Ptr(0.25)},
		{StandardCost: 500, UserCost: 999, Multiplier: nil},
	})
	if excluded != 1 {
		t.Fatalf("excluded = %d, want 1", excluded)
	}
	if math.Abs(total.Revenue-100) > 1e-9 {
		t.Fatalf("Revenue = %v, want 100 (排除项的收入不得计入)", total.Revenue)
	}
	if math.Abs(total.UpstreamCost-65) > 1e-9 {
		t.Fatalf("UpstreamCost = %v, want 65", total.UpstreamCost)
	}
	if math.Abs(total.Gross-35) > 1e-9 {
		t.Fatalf("Gross = %v, want 35", total.Gross)
	}
	if total.Margin == nil || math.Abs(*total.Margin-0.35) > 1e-9 {
		t.Fatalf("Margin = %v, want 0.35", total.Margin)
	}
}

func TestSumProfitAllIncomputable(t *testing.T) {
	total, excluded := SumProfit([]ProfitInput{{StandardCost: 10, UserCost: 5, Multiplier: nil}})
	if excluded != 1 || total.Computable {
		t.Fatalf("total = %+v, excluded = %d", total, excluded)
	}
}
```

- [ ] **Step 2: 验证测试失败**

Run: `cd relay-ops-service && go test ./internal/accounthealth/ -run 'TestComputeProfit|TestSumProfit' -v`
Expected: 编译失败，提示 `undefined: ComputeProfit`、`undefined: SumProfit`

- [ ] **Step 3: 写最小实现**

创建 `relay-ops-service/internal/accounthealth/profit.go`：

```go
package accounthealth

type ProfitInput struct {
	StandardCost float64
	UserCost     float64
	Multiplier   *float64
}

type Profit struct {
	Revenue      float64
	UpstreamCost float64
	Gross        float64
	Margin       *float64
	Computable   bool
}

// ComputeProfit derives upstream cost from the official standard cost and the
// trustworthy schema v2 multiplier. It never falls back to 1: the deprecated
// accounts.rate_multiplier is fixed at 1 in production and using it inflates
// upstream cost by 4x-20x.
func ComputeProfit(in ProfitInput) Profit {
	if in.Multiplier == nil {
		return Profit{Computable: false}
	}
	profit := Profit{
		Revenue:      in.UserCost,
		UpstreamCost: in.StandardCost * *in.Multiplier,
		Computable:   true,
	}
	profit.Gross = profit.Revenue - profit.UpstreamCost
	profit.Margin = marginOf(profit.Revenue, profit.Gross)
	return profit
}

func SumProfit(inputs []ProfitInput) (Profit, int) {
	total := Profit{}
	excluded := 0
	for _, in := range inputs {
		one := ComputeProfit(in)
		if !one.Computable {
			excluded++
			continue
		}
		total.Computable = true
		total.Revenue += one.Revenue
		total.UpstreamCost += one.UpstreamCost
	}
	if !total.Computable {
		return Profit{Computable: false}, excluded
	}
	total.Gross = total.Revenue - total.UpstreamCost
	total.Margin = marginOf(total.Revenue, total.Gross)
	return total, excluded
}

func marginOf(revenue, gross float64) *float64 {
	if revenue == 0 {
		return nil
	}
	margin := gross / revenue
	return &margin
}
```

- [ ] **Step 4: 验证测试通过**

Run: `cd relay-ops-service && go test ./internal/accounthealth/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add relay-ops-service/internal/accounthealth/profit.go relay-ops-service/internal/accounthealth/profit_test.go
git commit -m "feat: compute gross profit from trustworthy multiplier"
```

---

### Task 5: sub2api 客户端历史查询接口

**Files:**
- Modify: `relay-ops-service/internal/sub2api/types.go`（在 `AccountMonitorProjection` 定义之后追加类型，并扩展第 23 行附近的 `AccountMonitorReader` 接口）
- Modify: `relay-ops-service/internal/sub2api/client.go`（在 `ListAccountMonitors` 之后追加方法）
- Test: `relay-ops-service/internal/sub2api/client_history_test.go`

**Interfaces:**
- Produces: `AccountMonitorHistoryEntry{AccountID int64, ModelID string, Status string, ErrorCode string, HTTPStatus *int, TTFTMS *float64, LatencyMS *float64, CheckedAt time.Time}`
- Produces: `(*HTTPReader).ListAccountMonitorHistory(ctx context.Context, accountID int64, limit int) ([]AccountMonitorHistoryEntry, error)`
- Produces: `AccountMonitorReader` 接口新增 `ListAccountMonitorHistory(context.Context, int64, int) ([]AccountMonitorHistoryEntry, error)`

生产实测响应（`GET /api/v1/admin/account-monitors/:account_id/history?limit=N`）：

```json
{"code":0,"message":"success","data":{"items":[
  {"account_id":21,"model_id":"gpt-5.6-terra","status":"success","ttft_ms":1255.694,"latency_ms":1453.25,"checked_at":"2026-07-26T11:23:50Z"},
  {"account_id":22,"model_id":"gpt-5.6-terra","status":"failed","error_code":"balance_exhausted","latency_ms":542.088,"checked_at":"2026-07-26T11:28:55Z"}
]}}
```

`doStrict` 会剥掉 `data` 外层并启用 `DisallowUnknownFields`，因此类型必须精确覆盖上述字段。

- [ ] **Step 1: 写失败测试**

创建 `relay-ops-service/internal/sub2api/client_history_test.go`：

```go
package sub2api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// NewHTTPReader takes an admin-key FILE path (not the key itself) and returns
// an error, so tests must materialise a key file first.
func newHistoryTestReader(t *testing.T, baseURL string) *HTTPReader {
	t.Helper()
	keyFile := filepath.Join(t.TempDir(), "admin-key")
	if err := os.WriteFile(keyFile, []byte("test-admin-key\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	reader, err := NewHTTPReader(baseURL, keyFile)
	if err != nil {
		t.Fatalf("NewHTTPReader: %v", err)
	}
	return reader
}

func TestListAccountMonitorHistoryDecodesProductionShape(t *testing.T) {
	const body = `{"code":0,"message":"success","data":{"items":[
		{"account_id":21,"model_id":"gpt-5.6-terra","status":"success","ttft_ms":1255.694,"latency_ms":1453.25,"checked_at":"2026-07-26T11:23:50Z"},
		{"account_id":21,"model_id":"gpt-5.6-terra","status":"failed","error_code":"balance_exhausted","latency_ms":542.088,"checked_at":"2026-07-26T11:28:55Z"}
	]}}`
	var gotPath, gotQuery, gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotKey = r.URL.Path, r.URL.RawQuery, r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	reader := newHistoryTestReader(t, server.URL)
	entries, err := reader.ListAccountMonitorHistory(context.Background(), 21, 692)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/admin/account-monitors/21/history" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotQuery != "limit=692" {
		t.Fatalf("query = %q", gotQuery)
	}
	if gotKey != "test-admin-key" {
		t.Fatalf("x-api-key = %q", gotKey)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].TTFTMS == nil || *entries[0].TTFTMS != 1255.694 {
		t.Fatalf("entries[0].TTFTMS = %v", entries[0].TTFTMS)
	}
	if entries[1].ErrorCode != "balance_exhausted" {
		t.Fatalf("entries[1].ErrorCode = %q", entries[1].ErrorCode)
	}
	if entries[1].TTFTMS != nil {
		t.Fatalf("失败记录不应有 ttft_ms: %v", entries[1].TTFTMS)
	}
	if entries[0].CheckedAt.IsZero() {
		t.Fatal("CheckedAt not parsed")
	}
}

func TestListAccountMonitorHistoryRejectsBadInput(t *testing.T) {
	reader := newHistoryTestReader(t, "http://127.0.0.1:1")
	if _, err := reader.ListAccountMonitorHistory(context.Background(), 0, 10); err == nil {
		t.Fatal("accountID 0 must be rejected")
	}
	if _, err := reader.ListAccountMonitorHistory(context.Background(), 1, 0); err == nil {
		t.Fatal("limit 0 must be rejected")
	}
}

func TestListAccountMonitorHistoryRejectsUnknownField(t *testing.T) {
	const body = `{"data":{"items":[{"account_id":1,"model_id":"m","status":"success","checked_at":"2026-07-26T11:23:50Z","surprise":1}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	reader := newHistoryTestReader(t, server.URL)
	if _, err := reader.ListAccountMonitorHistory(context.Background(), 1, 10); err == nil {
		t.Fatal("未知字段必须触发 schema mismatch")
	}
}
```

- [ ] **Step 2: 验证测试失败**

Run: `cd relay-ops-service && go test ./internal/sub2api/ -run TestListAccountMonitorHistory -v`
Expected: 编译失败，提示 `reader.ListAccountMonitorHistory undefined`

- [ ] **Step 3: 添加类型并扩展接口**

在 `relay-ops-service/internal/sub2api/types.go` 的 `AccountMonitorProjection` 结构体之后追加：

```go
type AccountMonitorHistoryEntry struct {
	AccountID  int64     `json:"account_id"`
	ModelID    string    `json:"model_id"`
	Status     string    `json:"status"`
	ErrorCode  string    `json:"error_code,omitempty"`
	HTTPStatus *int      `json:"http_status,omitempty"`
	TTFTMS     *float64  `json:"ttft_ms,omitempty"`
	LatencyMS  *float64  `json:"latency_ms,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

type accountMonitorHistoryPage struct {
	Items []AccountMonitorHistoryEntry `json:"items"`
}
```

在同文件第 23 行的 `AccountMonitorReader` 接口中追加一行：

```go
	ListAccountMonitorHistory(context.Context, int64, int) ([]AccountMonitorHistoryEntry, error)
```

- [ ] **Step 4: 实现客户端方法**

在 `relay-ops-service/internal/sub2api/client.go` 的 `ListAccountMonitors` 方法之后追加：

```go
func (c *HTTPReader) ListAccountMonitorHistory(ctx context.Context, accountID int64, limit int) ([]AccountMonitorHistoryEntry, error) {
	if accountID <= 0 || limit <= 0 {
		return nil, errSchemaMismatch
	}
	path := "/api/v1/admin/account-monitors/" + strconv.FormatInt(accountID, 10) + "/history"
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	var page accountMonitorHistoryPage
	if err := c.getStrict(ctx, path, query, &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}
```

- [ ] **Step 5: 验证测试通过**

Run: `cd relay-ops-service && go test ./internal/sub2api/ -v`
Expected: PASS。若其他实现 `AccountMonitorReader` 的测试替身报「未实现方法」，为其补上同签名的空实现（返回 `nil, nil`）。

- [ ] **Step 6: 提交**

```bash
git add relay-ops-service/internal/sub2api/types.go relay-ops-service/internal/sub2api/client.go relay-ops-service/internal/sub2api/client_history_test.go
git commit -m "feat: read account monitor probe history"
```

---

### Task 6: 日报卡片渲染

**Files:**
- Create: `relay-ops-service/internal/notify/digest_v2.go`
- Test: `relay-ops-service/internal/notify/digest_v2_test.go`

**Interfaces:**
- Consumes: `accounthealth.Tier`、`accounthealth.Profit`、`accounthealth.GroupAvailability`
- Produces: `HealthDigestView{Date string, Quality QualityLine, Profit ProfitLine, Pending []PendingItem, Recommendations []RecommendationLine, Accounts []AccountDetailLine, Traffic TrafficLine}`
- Produces: `RecommendationLine{GroupName, CurrentName, CandidateName, Reason string}`
- Produces: `QualityLine{Healthy, Degraded, Unavailable, Slow int, HealthyDelta *int, TTFTMedianMS *float64, DataUnavailable bool, DataUnavailableReason string}`
- Produces: `ProfitLine{Revenue, UpstreamCost, Gross float64, Margin *float64, Computable bool, ExcludedAccounts int, NoTraffic bool}`
- Produces: `PendingItem{AccountName, Problem, Detail string}`
- Produces: `AccountDetailLine{Name, SuccessRate, TTFTP50, LatencyP95, Multiplier, GrossContribution string}`
- Produces: `TrafficLine{HasTraffic bool, Requests int64, ErrorRate, SLA string}`
- Produces: `RenderHealthDigest(HealthDigestView) FeishuMessage`

四层顺序固定为质量 → 利润 → 待处理 → 明细。金额以 `$` 呈现。

- [ ] **Step 1: 写失败测试**

创建 `relay-ops-service/internal/notify/digest_v2_test.go`：

```go
package notify

import (
	"strings"
	"testing"
)

func intPtr(v int) *int          { return &v }
func f64Ptr(v float64) *float64  { return &v }

func renderText(t *testing.T, message FeishuMessage) string {
	t.Helper()
	payload, err := message.CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	return string(payload)
}

func TestRenderHealthDigestLayerOrder(t *testing.T) {
	view := HealthDigestView{
		Date: "2026-07-27",
		Quality: QualityLine{
			Healthy: 6, Degraded: 2, Unavailable: 3, Slow: 2,
			HealthyDelta: intPtr(-1), TTFTMedianMS: f64Ptr(3900),
		},
		Profit: ProfitLine{NoTraffic: true},
		Pending: []PendingItem{
			{AccountName: "Plus-XN-0.09", Problem: "余额耗尽", Detail: "已持续 3 天"},
		},
		Accounts: []AccountDetailLine{
			{Name: "Pro-SHEN-0.16", SuccessRate: "100%", TTFTP50: "1467ms", LatencyP95: "4970ms", Multiplier: "0.16x", GrossContribution: "$0.00"},
		},
		Traffic: TrafficLine{HasTraffic: false},
	}
	text := renderText(t, RenderHealthDigest(view))

	qualityAt := strings.Index(text, "质量")
	profitAt := strings.Index(text, "利润")
	pendingAt := strings.Index(text, "待处理")
	detailAt := strings.Index(text, "明细")
	if qualityAt < 0 || profitAt < 0 || pendingAt < 0 || detailAt < 0 {
		t.Fatalf("四层标题缺失: %s", text)
	}
	if !(qualityAt < profitAt && profitAt < pendingAt && pendingAt < detailAt) {
		t.Fatalf("层级顺序错误 quality=%d profit=%d pending=%d detail=%d", qualityAt, profitAt, pendingAt, detailAt)
	}
	if !strings.Contains(text, "2026-07-27") {
		t.Fatal("日期缺失")
	}
}

func TestRenderHealthDigestUsesAccountNameNotID(t *testing.T) {
	view := HealthDigestView{
		Date:     "2026-07-27",
		Quality:  QualityLine{Healthy: 1},
		Profit:   ProfitLine{NoTraffic: true},
		Accounts: []AccountDetailLine{{Name: "Plus-XM-0.1", SuccessRate: "0%", TTFTP50: "—", LatencyP95: "511ms", Multiplier: "0.10x", GrossContribution: "—"}},
		Traffic:  TrafficLine{HasTraffic: false},
	}
	text := renderText(t, RenderHealthDigest(view))
	if !strings.Contains(text, "Plus-XM-0.1") {
		t.Fatal("必须显示用户命名的账号名")
	}
	if strings.Contains(text, "账号 2") {
		t.Fatalf("不得输出数据库 ID 形式: %s", text)
	}
}

func TestRenderHealthDigestNoTrafficCollapses(t *testing.T) {
	view := HealthDigestView{
		Date: "2026-07-27", Quality: QualityLine{Healthy: 1},
		Profit: ProfitLine{NoTraffic: true}, Traffic: TrafficLine{HasTraffic: false},
	}
	text := renderText(t, RenderHealthDigest(view))
	if !strings.Contains(text, "今日无真实调用") {
		t.Fatalf("无流量时应折叠为一行: %s", text)
	}
	if strings.Contains(text, "错误率") {
		t.Fatal("无流量时不得展开流量明细")
	}
}

func TestRenderHealthDigestProfitFormatting(t *testing.T) {
	view := HealthDigestView{
		Date: "2026-07-27", Quality: QualityLine{Healthy: 1},
		Profit: ProfitLine{
			Revenue: 100, UpstreamCost: 65, Gross: 35,
			Margin: f64Ptr(0.35), Computable: true, ExcludedAccounts: 2,
		},
		Traffic: TrafficLine{HasTraffic: true, Requests: 1200, ErrorRate: "0.8%", SLA: "99.2%"},
	}
	text := renderText(t, RenderHealthDigest(view))
	for _, want := range []string{"$100.00", "$65.00", "$35.00", "35.0%", "2 个账号因倍率不可用未计入"} {
		if !strings.Contains(text, want) {
			t.Fatalf("缺少 %q: %s", want, text)
		}
	}
	if !strings.Contains(text, "1200") {
		t.Fatal("有流量时应展开请求数")
	}
}

func TestRenderHealthDigestShowsRecommendations(t *testing.T) {
	view := HealthDigestView{
		Date: "2026-07-27", Quality: QualityLine{Healthy: 3},
		Profit: ProfitLine{NoTraffic: true},
		Recommendations: []RecommendationLine{
			{GroupName: "GPT-Pro", CurrentName: "Pro-TK-0.15", CandidateName: "Pro-SHEN-0.16", Reason: "成功率更高且延时更低"},
		},
		Traffic: TrafficLine{HasTraffic: false},
	}
	text := renderText(t, RenderHealthDigest(view))
	for _, want := range []string{"选型建议", "GPT-Pro", "Pro-SHEN-0.16", "Pro-TK-0.15", "成功率更高且延时更低"} {
		if !strings.Contains(text, want) {
			t.Fatalf("缺少 %q: %s", want, text)
		}
	}
}

func TestRenderHealthDigestOmitsEmptyRecommendations(t *testing.T) {
	view := HealthDigestView{
		Date: "2026-07-27", Quality: QualityLine{Healthy: 3},
		Profit: ProfitLine{NoTraffic: true}, Traffic: TrafficLine{HasTraffic: false},
	}
	text := renderText(t, RenderHealthDigest(view))
	if strings.Contains(text, "选型建议") {
		t.Fatalf("无建议时不得输出该小节: %s", text)
	}
}

func TestRenderHealthDigestDataUnavailable(t *testing.T) {
	view := HealthDigestView{
		Date:    "2026-07-27",
		Quality: QualityLine{DataUnavailable: true, DataUnavailableReason: "Sub2API 不可达"},
	}
	text := renderText(t, RenderHealthDigest(view))
	if !strings.Contains(text, "数据不可用") || !strings.Contains(text, "Sub2API 不可达") {
		t.Fatalf("必须明示数据不可用: %s", text)
	}
	if strings.Contains(text, "健康 0") {
		t.Fatal("数据不可用时不得输出伪造的健康计数")
	}
}

func TestRenderHealthDigestFitsCardLimit(t *testing.T) {
	accounts := make([]AccountDetailLine, 200)
	for i := range accounts {
		accounts[i] = AccountDetailLine{
			Name: strings.Repeat("A", 60), SuccessRate: "99.9%", TTFTP50: "1500ms",
			LatencyP95: "4000ms", Multiplier: "0.25x", GrossContribution: "$12.34",
		}
	}
	view := HealthDigestView{
		Date: "2026-07-27", Quality: QualityLine{Healthy: 200},
		Profit: ProfitLine{NoTraffic: true}, Accounts: accounts,
		Traffic: TrafficLine{HasTraffic: false},
	}
	payload, err := RenderHealthDigest(view).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	if len(payload) > maxCardBytes {
		t.Fatalf("card size = %d, exceeds %d", len(payload), maxCardBytes)
	}
	if !strings.Contains(string(payload), "已截断") {
		t.Fatal("超限截断时必须注明")
	}
}
```

- [ ] **Step 2: 验证测试失败**

Run: `cd relay-ops-service && go test ./internal/notify/ -run TestRenderHealthDigest -v`
Expected: 编译失败，提示 `undefined: HealthDigestView`、`undefined: RenderHealthDigest`

- [ ] **Step 3: 写实现**

创建 `relay-ops-service/internal/notify/digest_v2.go`：

```go
package notify

import (
	"fmt"
	"strconv"
	"strings"
)

type QualityLine struct {
	Healthy               int
	Degraded              int
	Unavailable           int
	Slow                  int
	HealthyDelta          *int
	TTFTMedianMS          *float64
	DataUnavailable       bool
	DataUnavailableReason string
}

type ProfitLine struct {
	Revenue          float64
	UpstreamCost     float64
	Gross            float64
	Margin           *float64
	Computable       bool
	ExcludedAccounts int
	NoTraffic        bool
}

type PendingItem struct {
	AccountName string
	Problem     string
	Detail      string
}

type AccountDetailLine struct {
	Name              string
	SuccessRate       string
	TTFTP50           string
	LatencyP95        string
	Multiplier        string
	GrossContribution string
}

type TrafficLine struct {
	HasTraffic bool
	Requests   int64
	ErrorRate  string
	SLA        string
}

type RecommendationLine struct {
	GroupName     string
	CurrentName   string
	CandidateName string
	Reason        string
}

type HealthDigestView struct {
	Date            string
	Quality         QualityLine
	Profit          ProfitLine
	Pending         []PendingItem
	Recommendations []RecommendationLine
	Accounts        []AccountDetailLine
	Traffic         TrafficLine
}

func RenderHealthDigest(view HealthDigestView) FeishuMessage {
	// fitDigestSection gives every layer its own 4 KiB budget, trims each line
	// to 768 bytes, and keeps abnormal rows during truncation. Joining raw
	// would leave the pending layer unbounded, and a card over maxCardBytes
	// makes CardJSON fail — which drops the whole report instead of degrading.
	elements := []CardElement{
		{Tag: "div", Text: &CardText{Tag: "lark_md", Content: fitDigestSection(qualityLines(view.Quality))}},
		{Tag: "div", Text: &CardText{Tag: "lark_md", Content: fitDigestSection(profitLines(view.Profit))}},
		{Tag: "div", Text: &CardText{Tag: "lark_md", Content: fitDigestSection(pendingLines(view.Pending, view.Recommendations))}},
		{Tag: "div", Text: &CardText{Tag: "lark_md", Content: fitDigestSection(detailLines(view))}},
	}
	elements = append(elements, CardElement{Tag: "action", Actions: []CardAction{{
		Tag: "button", Text: CardText{Tag: "plain_text", Content: "运维后台"}, Type: "primary", MultiURL: &CardURL{URL: "/ops"},
	}}})
	return FeishuMessage{MsgType: "interactive", Card: &Card{
		Config:   CardConfig{WideScreenMode: true},
		Header:   CardHeader{Title: CardText{Tag: "plain_text", Content: "relay-ops 中转站日报 " + digestValue(view.Date)}, Template: "blue"},
		Elements: elements,
	}}
}

func qualityLines(quality QualityLine) []string {
	lines := []string{"**质量**"}
	if quality.DataUnavailable {
		reason := digestValue(quality.DataUnavailableReason)
		if reason == "" {
			reason = "原因未知"
		}
		return append(lines, "数据不可用："+reason)
	}
	summary := fmt.Sprintf("稳定 %d / 降级 %d / 不可用 %d", quality.Healthy, quality.Degraded, quality.Unavailable)
	if quality.HealthyDelta != nil {
		summary += "　健康账号较昨日 " + signedDelta(*quality.HealthyDelta)
	}
	lines = append(lines, summary)
	detail := "延时 P95 中位 " + formatMillis(quality.TTFTMedianMS)
	if quality.Slow > 0 {
		detail += fmt.Sprintf(" ｜ %d 个账号偏慢", quality.Slow)
	}
	return append(lines, detail)
}

func profitLines(profit ProfitLine) []string {
	lines := []string{"**利润**"}
	if profit.NoTraffic {
		return append(lines, "今日无真实调用")
	}
	if !profit.Computable {
		return append(lines, "无法核算：所有账号倍率不可用")
	}
	lines = append(lines, fmt.Sprintf("今日收入 %s　上游成本 %s　毛利 %s（%s）",
		formatUSD(profit.Revenue), formatUSD(profit.UpstreamCost), formatUSD(profit.Gross), formatMargin(profit.Margin)))
	if profit.ExcludedAccounts > 0 {
		lines = append(lines, fmt.Sprintf("%d 个账号因倍率不可用未计入", profit.ExcludedAccounts))
	}
	return lines
}

func pendingLines(items []PendingItem, recommendations []RecommendationLine) []string {
	lines := []string{"**待处理**"}
	if len(items) == 0 {
		lines = append(lines, "无")
	}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s　%s　%s",
			digestValue(item.AccountName), digestValue(item.Problem), digestValue(item.Detail)))
	}
	if len(recommendations) > 0 {
		lines = append(lines, "**选型建议**")
		for _, rec := range recommendations {
			lines = append(lines, fmt.Sprintf("- %s：%s 综合优于当前 %s（%s）",
				digestValue(rec.GroupName), digestValue(rec.CandidateName),
				digestValue(rec.CurrentName), digestValue(rec.Reason)))
		}
	}
	return lines
}

func detailLines(view HealthDigestView) []string {
	lines := []string{"**明细**"}
	kept, truncated := fitAccountLines(view.Accounts)
	for _, account := range kept {
		lines = append(lines, fmt.Sprintf("- %s：成功率 %s · TTFT %s · 延迟 P95 %s · 倍率 %s · 毛利 %s",
			digestValue(account.Name), digestValue(account.SuccessRate), digestValue(account.TTFTP50),
			digestValue(account.LatencyP95), digestValue(account.Multiplier), digestValue(account.GrossContribution)))
	}
	if len(kept) == 0 {
		lines = append(lines, "无账号明细")
	}
	if truncated > 0 {
		lines = append(lines, fmt.Sprintf("（已截断 %d 个账号，完整明细见运维后台）", truncated))
	}
	if view.Traffic.HasTraffic {
		lines = append(lines, fmt.Sprintf("站内流量：请求 %s · 错误率 %s · SLA %s",
			strconv.FormatInt(view.Traffic.Requests, 10), digestValue(view.Traffic.ErrorRate), digestValue(view.Traffic.SLA)))
	} else {
		lines = append(lines, "站内流量：今日无真实调用")
	}
	return lines
}

// fitAccountLines caps the detail layer so the first three layers always
// survive the 30 KiB card limit.
func fitAccountLines(accounts []AccountDetailLine) ([]AccountDetailLine, int) {
	const maxDetailAccounts = 40
	if len(accounts) <= maxDetailAccounts {
		return accounts, 0
	}
	return accounts[:maxDetailAccounts], len(accounts) - maxDetailAccounts
}

func signedDelta(delta int) string {
	switch {
	case delta > 0:
		return "↑" + strconv.Itoa(delta)
	case delta < 0:
		return "↓" + strconv.Itoa(-delta)
	default:
		return "持平"
	}
}

func formatUSD(value float64) string { return fmt.Sprintf("$%.2f", value) }

func formatMargin(margin *float64) string {
	if margin == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", *margin*100)
}

func formatMillis(value *float64) string {
	if value == nil {
		return "—"
	}
	return fmt.Sprintf("%.1fs", *value/1000)
}
```

- [ ] **Step 4: 验证测试通过**

Run: `cd relay-ops-service && go test ./internal/notify/ -v`
Expected: PASS。若 `digestValue`、`maxCardBytes`、`CardElement` 等符号签名与 `feishu.go` 不符，以 `feishu.go` 现有定义为准调整调用处。

- [ ] **Step 5: 提交**

```bash
git add relay-ops-service/internal/notify/digest_v2.go relay-ops-service/internal/notify/digest_v2_test.go
git commit -m "feat: render quality-first operations digest card"
```

---

### Task 7: 分组可用性告警卡片渲染

**Files:**
- Create: `relay-ops-service/internal/notify/group_alert.go`
- Test: `relay-ops-service/internal/notify/group_alert_test.go`

**Interfaces:**
- Produces: `GroupAlertView{GroupName string, Available int, Total int, Down []GroupAlertAccount, Recovery bool}`
- Produces: `GroupAlertAccount{Name, ErrorCode, Duration string}`
- Produces: `RenderGroupAlert(GroupAlertView) FeishuMessage`

告警卡为红色模板，恢复卡为绿色模板。卡片不得包含凭据、Base URL 或上游原始错误。

- [ ] **Step 1: 写失败测试**

创建 `relay-ops-service/internal/notify/group_alert_test.go`：

```go
package notify

import (
	"strings"
	"testing"
)

func TestRenderGroupAlertContent(t *testing.T) {
	view := GroupAlertView{
		GroupName: "GPT-Plus", Available: 1, Total: 3,
		Down: []GroupAlertAccount{
			{Name: "Plus-XN-0.09", ErrorCode: "balance_exhausted", Duration: "已持续 3 天"},
			{Name: "Plus-XM-0.1", ErrorCode: "balance_exhausted", Duration: "已持续 5 天"},
		},
	}
	payload, err := RenderGroupAlert(view).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	text := string(payload)
	for _, want := range []string{"GPT-Plus", "可用 1 / 共 3", "Plus-XN-0.09", "balance_exhausted", "已持续 5 天", "red"} {
		if !strings.Contains(text, want) {
			t.Fatalf("缺少 %q: %s", want, text)
		}
	}
	if !strings.Contains(text, "建议") {
		t.Fatal("必须给出建议动作")
	}
}

func TestRenderGroupAlertRecovery(t *testing.T) {
	view := GroupAlertView{GroupName: "GPT-Plus", Available: 3, Total: 3, Recovery: true}
	payload, err := RenderGroupAlert(view).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, "green") {
		t.Fatalf("恢复卡应为绿色: %s", text)
	}
	if !strings.Contains(text, "已恢复") {
		t.Fatal("恢复卡应说明已恢复")
	}
}

// 对抗性输入：必须真的把敏感串喂进去，否则断言「产物不含 sk-」只是因为
// 输入本身干净而碰巧通过，抓不住任何真实泄露。
func TestRenderGroupAlertLeaksNoSecrets(t *testing.T) {
	view := GroupAlertView{
		GroupName: "https://api.shuaiapi.com/v1", Available: 0, Total: 2,
		Down: []GroupAlertAccount{
			{Name: "http://internal.host/admin", ErrorCode: "sk-abc123 leaked", Duration: "x-api-key: secret"},
			{Name: "Plus-XN-0.09", ErrorCode: "api_key rejected", Duration: "已持续 1 天"},
		},
	}
	payload, err := RenderGroupAlert(view).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	text := string(payload)
	for _, forbidden := range []string{"http://", "https://api.", "sk-", "x-api-key", "api_key"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("卡片泄露敏感内容 %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "[已脱敏]") {
		t.Fatalf("敏感输入必须被替换为脱敏标记: %s", text)
	}
}

func TestRenderGroupAlertBoundsOversizedDownList(t *testing.T) {
	// 上游供应商级故障会让整组账号同时不可用。卡片超过 maxCardBytes 会让
	// CardJSON 报错，告警就在最该送达的时刻丢失。
	down := make([]GroupAlertAccount, 600)
	for i := range down {
		down[i] = GroupAlertAccount{
			Name:      strings.Repeat("A", 60),
			ErrorCode: "balance_exhausted",
			Duration:  "已持续 3 天",
		}
	}
	payload, err := RenderGroupAlert(GroupAlertView{
		GroupName: "GPT-Plus", Available: 0, Total: 600, Down: down,
	}).CardJSON()
	if err != nil {
		t.Fatalf("大规模故障时告警必须仍能渲染: %v", err)
	}
	if len(payload) > maxCardBytes {
		t.Fatalf("card size = %d, exceeds %d", len(payload), maxCardBytes)
	}
}

func TestRenderGroupAlertRecoveryOmitsDownList(t *testing.T) {
	payload, err := RenderGroupAlert(GroupAlertView{
		GroupName: "GPT-Plus", Available: 3, Total: 3, Recovery: true,
		Down: []GroupAlertAccount{{Name: "Plus-XN-0.09", ErrorCode: "balance_exhausted"}},
	}).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	if strings.Contains(string(payload), "Plus-XN-0.09") {
		t.Fatalf("恢复卡不得再列出已不可用账号: %s", payload)
	}
}
```

- [ ] **Step 2: 验证测试失败**

Run: `cd relay-ops-service && go test ./internal/notify/ -run TestRenderGroupAlert -v`
Expected: 编译失败，提示 `undefined: GroupAlertView`、`undefined: RenderGroupAlert`

- [ ] **Step 3: 写实现**

创建 `relay-ops-service/internal/notify/group_alert.go`：

```go
package notify

import (
	"fmt"
	"strings"
)

type GroupAlertAccount struct {
	Name      string
	ErrorCode string
	Duration  string
}

type GroupAlertView struct {
	GroupName string
	Available int
	Total     int
	Down      []GroupAlertAccount
	Recovery  bool
}

func RenderGroupAlert(view GroupAlertView) FeishuMessage {
	name := digestValue(view.GroupName)
	lines := []string{fmt.Sprintf("可用 %d / 共 %d", view.Available, view.Total)}

	template := "red"
	title := "⚠️ 分组可用账号不足：" + name
	if view.Recovery {
		template = "green"
		title = "分组可用性已恢复：" + name
	}
	if !view.Recovery {
		for _, account := range view.Down {
			lines = append(lines, fmt.Sprintf("%s　%s　%s",
				digestValue(account.Name), digestValue(account.ErrorCode), digestValue(account.Duration)))
		}
		lines = append(lines, "", "建议：为上述账号充值，或临时关闭 schedulable 止血")
	}
	return FeishuMessage{MsgType: "interactive", Card: &Card{
		Config: CardConfig{WideScreenMode: true},
		Header: CardHeader{Title: CardText{Tag: "plain_text", Content: title}, Template: template},
		Elements: []CardElement{
			// fitDigestSection bounds the section at 4 KiB and trims each line.
			// Down has no upper bound — a supplier-wide outage is exactly when
			// this alert matters most, and an oversized card makes CardJSON
			// fail, so the alert would be lost precisely when it is needed.
			{Tag: "div", Text: &CardText{Tag: "lark_md", Content: fitDigestSection(lines)}},
			{Tag: "action", Actions: []CardAction{{
				Tag: "button", Text: CardText{Tag: "plain_text", Content: "运维后台"}, Type: "primary", MultiURL: &CardURL{URL: "/ops"},
			}}},
		},
	}}
}
```

- [ ] **Step 4: 验证测试通过**

Run: `cd relay-ops-service && go test ./internal/notify/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add relay-ops-service/internal/notify/group_alert.go relay-ops-service/internal/notify/group_alert_test.go
git commit -m "feat: render group availability alert card"
```

---

### Task 8: 日报编排改造与告警作业接线

**Files:**
- Modify: `relay-ops-service/internal/dailyreport/service.go:65-145`（`Run` 方法的账号区块）
- Create: `relay-ops-service/internal/dailyreport/health.go`
- Modify: `relay-ops-service/internal/scheduler/scheduler.go:38`（`Scheduler` 结构体）与 `:119-133`（`Tick` 作业注册）
- Modify: `relay-ops-service/internal/app/app.go:283-289`（`dailyReportService` 构造）与 `:390-396`（`scheduled` 构造）
- Test: `relay-ops-service/internal/dailyreport/health_test.go`

**Interfaces:**
- Consumes: `accounthealth.ClassifyAccount`、`accounthealth.SliceByDay`、`accounthealth.HistoryLimitFor`、`accounthealth.GroupAvailabilities`、`accounthealth.SumProfit`（Task 1–4）
- Consumes: `accountrecommendation.Analyze(projection) Result`，其 `Result.Groups[].{GroupName, Decision, CandidateAccountID, Reasons, Current.Name, Candidate.Name}` 用于填充选型建议；仅当 `Decision == "candidate_better"` 且 `CandidateAccountID != 0` 时才产出建议行
- Consumes: `sub2api.AccountMonitorReader.ListAccountMonitorHistory`（Task 5）
- Consumes: `notify.RenderHealthDigest`、`notify.RenderGroupAlert`（Task 6–7）
- Produces: `BuildHealthDigest(projection sub2api.AccountMonitorProjection, histories map[int64][]sub2api.AccountMonitorHistoryEntry, loc *time.Location, now time.Time) notify.HealthDigestView`
- Produces: `GroupAvailabilityView{Alert notify.GroupAlertView, Alerting bool}` 与 `BuildGroupAvailability(projection sub2api.AccountMonitorProjection) []GroupAvailabilityView`（返回全部分组，不只告警中的）
- Produces: `Scheduler.GroupAvailability func(context.Context) error` 字段，注册为 `group-availability` 作业，间隔 5 分钟

- [ ] **Step 1: 写失败测试**

创建 `relay-ops-service/internal/dailyreport/health_test.go`：

```go
package dailyreport

import (
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/sub2api"
)

func f64(v float64) *float64 { return &v }

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
	projection, _, _, _ := fixture()
	views := BuildGroupAvailability(projection)

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
	if plus.Alert.Down[0].ErrorCode != "balance_exhausted" {
		t.Fatalf("ErrorCode = %q, want balance_exhausted", plus.Alert.Down[0].ErrorCode)
	}

	pro, ok := byName["GPT-Pro"]
	if !ok {
		t.Fatal("缺少 GPT-Pro")
	}
	if pro.Alerting {
		t.Fatalf("GPT-Pro 2/2 健康不应告警: %+v", pro)
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
```

- [ ] **Step 2: 验证测试失败**

Run: `cd relay-ops-service && go test ./internal/dailyreport/ -run 'TestBuildHealthDigest|TestBuildGroupAvailability' -v`
Expected: 编译失败，提示 `undefined: BuildHealthDigest`、`undefined: BuildGroupAvailability`

- [ ] **Step 3: 实现构建函数**

创建 `relay-ops-service/internal/dailyreport/health.go`：

```go
package dailyreport

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/accounthealth"
	"example.invalid/relay-ops-service/internal/accountrecommendation"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func classify(projection sub2api.AccountMonitorProjection) []accounthealth.AccountVerdict {
	verdicts := make([]accounthealth.AccountVerdict, 0, len(projection.Accounts))
	for _, account := range projection.Accounts {
		verdicts = append(verdicts, accounthealth.ClassifyAccount(accounthealth.AccountSample{
			AccountID:   account.AccountID,
			Name:        account.Name,
			GroupNames:  account.GroupNames,
			SuccessRate: account.SuccessRate,
			SampleCount: account.SampleCount,
			TTFTP95MS:   account.TTFTP95MS,
			ErrorCode:   account.ErrorCode,
		}))
	}
	return verdicts
}

// trustworthyMultiplier returns the schema v2 multiplier only when it is
// usable. The deprecated accounts.rate_multiplier must never be substituted.
//
// A non-positive multiplier is treated as unusable even when the upstream
// reports status=ok: Sub2API only rejects value < 0, so a zero can reach us,
// and a zero multiplier would report 100% margin on a real cost. Production
// multipliers sit in 0.05x-0.25x, so non-positive means bad data.
func trustworthyMultiplier(account sub2api.AccountMonitorAccount) *float64 {
	if account.Multiplier.Status != "ok" || account.Multiplier.Value == nil {
		return nil
	}
	if *account.Multiplier.Value <= 0 {
		return nil
	}
	return account.Multiplier.Value
}

func BuildHealthDigest(
	projection sub2api.AccountMonitorProjection,
	histories map[int64][]sub2api.AccountMonitorHistoryEntry,
	loc *time.Location,
	now time.Time,
) notify.HealthDigestView {
	verdicts := classify(projection)

	view := notify.HealthDigestView{Date: now.In(loc).Format("2006-01-02")}
	profitInputs := make([]accounthealth.ProfitInput, 0, len(projection.Accounts))
	comparableAccounts, todayHealthyComparable, yesterdayHealthy := 0, 0, 0

	for index, account := range projection.Accounts {
		verdict := verdicts[index]
		switch verdict.Tier {
		case accounthealth.TierHealthy:
			view.Quality.Healthy++
		case accounthealth.TierDegraded:
			view.Quality.Degraded++
		case accounthealth.TierUnavailable:
			view.Quality.Unavailable++
		}
		if verdict.Slow {
			view.Quality.Slow++
		}

		multiplier := trustworthyMultiplier(account)
		standardCost, userCost := 0.0, 0.0
		if account.TodayStats != nil {
			standardCost, userCost = account.TodayStats.StandardCost, account.TodayStats.UserCost
		}
		input := accounthealth.ProfitInput{StandardCost: standardCost, UserCost: userCost, Multiplier: multiplier}
		profitInputs = append(profitInputs, input)

		view.Accounts = append(view.Accounts, notify.AccountDetailLine{
			Name:              account.Name,
			SuccessRate:       fmt.Sprintf("%.1f%%", account.SuccessRate*100),
			TTFTP50:           millis(account.TTFTP50MS),
			LatencyP95:        millis(account.LatencyP95MS),
			Multiplier:        multiplierLabel(account.Multiplier),
			GrossContribution: grossLabel(accounthealth.ComputeProfit(input)),
		})

		if item, ok := pendingFor(account, verdict); ok {
			view.Pending = append(view.Pending, item)
		}

		if entries := histories[account.AccountID]; len(entries) > 0 {
			_, yesterday := accounthealth.SliceByDay(toHistoryEntries(entries), loc, now)
			if yesterday.SampleCount > 0 {
				comparableAccounts++
				if yesterday.SuccessRate >= accounthealth.HealthyMinSuccessRate {
					yesterdayHealthy++
				}
				if verdict.Tier == accounthealth.TierHealthy {
					todayHealthyComparable++
				}
			}
		}
	}

	view.Recommendations = buildRecommendations(projection)

	// 同比只能在「今天和昨天都有样本」的账号子集上计算。若拿全部账号的今日
	// 健康数去减「仅有历史记录的账号」的昨日健康数，两个总体不一致，delta 会
	// 系统性偏高；历史为空时更会凭空得出「较昨日 ↑N」。宁可不给同比。
	if comparableAccounts > 0 {
		delta := todayHealthyComparable - yesterdayHealthy
		view.Quality.HealthyDelta = &delta
	}
	view.Quality.TTFTMedianMS = medianTTFT(projection.Accounts)

	total, excluded := accounthealth.SumProfit(profitInputs)
	view.Profit = notify.ProfitLine{
		Revenue: total.Revenue, UpstreamCost: total.UpstreamCost, Gross: total.Gross,
		Margin: total.Margin, Computable: total.Computable, ExcludedAccounts: excluded,
		NoTraffic: !hasTraffic,
	}
	// 所有有流量的账号都因倍率不可用被排除时，利润数字没有意义。此时报
	// 「可核算、收入 $0」等于凭空造出一个 100% 毛利的一天。
	if hasTraffic && total.Revenue == 0 && total.UpstreamCost == 0 {
		view.Profit.Computable = false
	}
	view.Traffic = notify.TrafficLine{HasTraffic: hasTraffic, Requests: totalRequests}
	return view
}

// buildRecommendations surfaces only actionable switches: a group is listed
// solely when the analyzer concluded the candidate is better and named it.
func buildRecommendations(projection sub2api.AccountMonitorProjection) []notify.RecommendationLine {
	recommendations := []notify.RecommendationLine{}
	for _, group := range accountrecommendation.Analyze(projection).Groups {
		if group.Decision != "candidate_better" || group.CandidateAccountID == 0 {
			continue
		}
		if group.Current.Name == "" || group.Candidate.Name == "" {
			continue
		}
		recommendations = append(recommendations, notify.RecommendationLine{
			GroupName:     group.GroupName,
			CurrentName:   group.Current.Name,
			CandidateName: group.Candidate.Name,
			Reason:        strings.Join(group.Reasons, "、"),
		})
	}
	return recommendations
}

// GroupAvailabilityView pairs a renderable alert with the alerting flag.
//
// Every group must be reported, not just the alerting ones: the incident state
// machine only emits a recovery when it observes Failing=false. Returning just
// the alerting groups would leave a recovered group stuck in `confirmed`
// forever, so its next real outage carries an unchanged evidence hash and is
// silently suppressed — the alert would fire exactly once, ever.
type GroupAvailabilityView struct {
	Alert    notify.GroupAlertView
	Alerting bool
}

func BuildGroupAvailability(projection sub2api.AccountMonitorProjection) []GroupAvailabilityView {
	errorCodes := make(map[int64]string, len(projection.Accounts))
	for _, account := range projection.Accounts {
		errorCodes[account.AccountID] = account.ErrorCode
	}
	views := []GroupAvailabilityView{}
	seen := map[string]bool{}
	for _, group := range accounthealth.GroupAvailabilities(classify(projection)) {
		alert := notify.GroupAlertView{GroupName: group.GroupName, Available: group.Available, Total: group.Total}
		for _, down := range group.Down {
			alert.Down = append(alert.Down, notify.GroupAlertAccount{
				Name:      down.Name,
				ErrorCode: errorCodes[down.AccountID],
			})
		}
		seen[group.GroupName] = true
		views = append(views, GroupAvailabilityView{Alert: alert, Alerting: group.Alerting})
	}
	// GroupAvailabilities 会跳过 TierUnknown 账号，因此某个分组的账号全部失去
	// 样本时，该分组会整个从结果里消失，于是也不再被 Observe，incident 卡在
	// confirmed —— 与「告警只发一次」同源。这里把缺席的分组补回来，以
	// Alerting=false 观测，让状态机能够走完恢复路径。
	for _, name := range groupNamesIn(projection) {
		if !seen[name] {
			views = append(views, GroupAvailabilityView{
				Alert: notify.GroupAlertView{GroupName: name},
			})
		}
	}
	sort.SliceStable(views, func(i, j int) bool { return views[i].Alert.GroupName < views[j].Alert.GroupName })
	return views
}

func groupNamesIn(projection sub2api.AccountMonitorProjection) []string {
	names := []string{}
	seen := map[string]bool{}
	for _, account := range projection.Accounts {
		for _, name := range account.GroupNames {
			if name != "" && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

func pendingFor(account sub2api.AccountMonitorAccount, verdict accounthealth.AccountVerdict) (notify.PendingItem, bool) {
	switch {
	case verdict.Tier == accounthealth.TierUnavailable:
		return notify.PendingItem{AccountName: account.Name, Problem: problemLabel(account.ErrorCode), Detail: ""}, true
	case verdict.Tier == accounthealth.TierDegraded:
		return notify.PendingItem{
			AccountName: account.Name,
			Problem:     fmt.Sprintf("成功率 %.0f%% ↓", account.SuccessRate*100),
			Detail:      account.ErrorCode,
		}, true
	case trustworthyMultiplier(account) == nil:
		return notify.PendingItem{AccountName: account.Name, Problem: "倍率测不出", Detail: "利润无法核算"}, true
	}
	return notify.PendingItem{}, false
}

func problemLabel(errorCode string) string {
	if errorCode == accounthealth.ErrorCodeBalanceExhausted {
		return "余额耗尽"
	}
	if errorCode == "" {
		return "不可用"
	}
	return errorCode
}

func multiplierLabel(multiplier sub2api.AccountMonitorMultiplier) string {
	if multiplier.Status != "ok" || multiplier.Value == nil {
		return "—"
	}
	return fmt.Sprintf("%.2fx", *multiplier.Value)
}

func grossLabel(profit accounthealth.Profit) string {
	if !profit.Computable {
		return "—"
	}
	return fmt.Sprintf("$%.2f", profit.Gross)
}

func millis(value *float64) string {
	if value == nil {
		return "—"
	}
	return fmt.Sprintf("%.0fms", *value)
}

func medianTTFT(accounts []sub2api.AccountMonitorAccount) *float64 {
	values := make([]float64, 0, len(accounts))
	for _, account := range accounts {
		if account.TTFTP95MS != nil {
			values = append(values, *account.TTFTP95MS)
		}
	}
	if len(values) == 0 {
		return nil
	}
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
	median := values[len(values)/2]
	return &median
}

func toHistoryEntries(entries []sub2api.AccountMonitorHistoryEntry) []accounthealth.HistoryEntry {
	converted := make([]accounthealth.HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		converted = append(converted, accounthealth.HistoryEntry{
			CheckedAt: entry.CheckedAt, Status: entry.Status,
			ErrorCode: entry.ErrorCode, TTFTMS: entry.TTFTMS,
		})
	}
	return converted
}
```

- [ ] **Step 4: 验证构建函数测试通过**

Run: `cd relay-ops-service && go test ./internal/dailyreport/ -run 'TestBuildHealthDigest|TestBuildGroupAvailability' -v`
Expected: PASS

- [ ] **Step 5: 接线到 Run 与调度器**

在 `relay-ops-service/internal/dailyreport/service.go` 的 `Run` 方法中，把下面这段现有代码（`accountStatus` 构造 + `RenderOperationsDigest` 调用）：

```go
	accountStatus := notify.UpstreamAccountStatusView{}
	if monitorReader, ok := s.Reader.(sub2api.AccountMonitorReader); ok {
		if projection, readErr := monitorReader.ListAccountMonitors(ctx); readErr == nil {
			analysis := accountrecommendation.Analyze(projection)
			accountStatus = upstreamAccountStatusView(projection, analysis)
		}
	}
	message := notify.RenderOperationsDigest(notify.OperationsDigestView{
		Date: date, GeneratedAt: now, Runtime: runtime, AccountQuality: quality, UpstreamAccountStatus: accountStatus, Footer: footer,
	})
```

整体替换为：

```go
	view := notify.HealthDigestView{
		Date: date,
		Quality: notify.QualityLine{
			DataUnavailable:       true,
			DataUnavailableReason: "账号监控数据不可用",
		},
	}
	if monitorReader, ok := s.Reader.(sub2api.AccountMonitorReader); ok {
		if projection, readErr := monitorReader.ListAccountMonitors(ctx); readErr == nil {
			limit := accounthealth.HistoryLimitFor(projection.Settings.IntervalSeconds)
			histories := make(map[int64][]sub2api.AccountMonitorHistoryEntry, len(projection.Accounts))
			for _, account := range projection.Accounts {
				entries, historyErr := monitorReader.ListAccountMonitorHistory(ctx, account.AccountID, limit)
				if historyErr != nil {
					// 单账号历史失败只影响该账号的同比，其余账号照常出数
					continue
				}
				histories[account.AccountID] = entries
			}
			view = BuildHealthDigest(projection, histories, location, now)
		}
	}
	message := notify.RenderHealthDigest(view)
```

同步调整 `service.go` 的 import：新增 `example.invalid/relay-ops-service/internal/accounthealth`，删除 `accountrecommendation` 与 `notify` 中已不再用到的引用（`accountrecommendation` 改由 `health.go` 引用）。同时删除 `service.go` 中的 `upstreamAccountStatusView` 函数（第 170 行起）——它唯一的调用点正是本步骤替换掉的那段。`go build ./...` 会报未使用的 import，据此收敛。

变量 `quality`、`footer`、`evidenceRefs`、`runtime` 仍被同方法内的 `contract`、`analysis` 与后续 `Observe`/`SendIncident` 逻辑使用，**不要删除它们的赋值**；本步骤只替换卡片渲染部分。

在 `relay-ops-service/internal/scheduler/scheduler.go` 的 `Scheduler` 结构体追加字段：

```go
	GroupAvailability func(context.Context) error
```

在 `Tick` 方法中 `SiteMonitor` 注册之后追加：

```go
	if s.GroupAvailability != nil {
		if err := s.runDue(ctx, "group-availability", now, 5*time.Minute, s.GroupAvailability); err != nil {
			failures = append(failures, err)
		}
	}
```

在 `relay-ops-service/internal/app/app.go` 中，把 `scheduled` 构造里的 `SiteMonitor: siteMonitor.Run,` 一行替换为：

```go
		SiteMonitor: siteMonitor.Run,
		GroupAvailability: func(runCtx context.Context) error {
			return runGroupAvailability(runCtx, reader, incidentMachine, notifier, cfg.Timezone, time.Now().UTC())
		},
```

`reader` 是第 234 行构造的 `*sub2api.HTTPReader` 具体类型，无需类型断言。`sha256`、`hex`、`fmt`、`time`、`incidents`、`notify`、`dailyreport` 在 app.go 中均已 import。

告警主体提取为 `app` 包内的可测函数（新建 `relay-ops-service/internal/app/group_availability.go`），而不是写成匿名闭包 —— 闭包无法被测试触达，而 C1 的全部修复逻辑都活在这里：

```go
type groupMonitorReader interface {
	ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error)
}

type groupIncidentObserver interface {
	Observe(context.Context, incidents.Observation) (incidents.Transition, error)
}

type groupAlertSender interface {
	SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

func runGroupAvailability(
	ctx context.Context,
	reader groupMonitorReader,
	machine groupIncidentObserver,
	sender groupAlertSender,
	loc *time.Location,
	now time.Time,
) error {
	projection, readErr := reader.ListAccountMonitors(ctx)
	if readErr != nil {
		// fail-safe：监控自身故障不得伪装成业务故障，静默跳过本轮
		log.Printf("group availability: skip round, monitor read failed: %v", readErr)
		return nil
	}
	if loc == nil {
		loc = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	var failures []error
	for _, item := range dailyreport.BuildGroupAvailability(projection) {
		alert := item.Alert
		key := "group:" + alert.GroupName + ":availability"
		// 投递证据必须带时间桶。notification_deliveries.dedup_key 有 UNIQUE
		// 约束且成功投递的记录永不复用，而可用比会重复出现（单账号分组每次
		// 故障都是 0/1）。不带时间桶的话，同一分组第二次故障会撞上第一次的
		// dedup_key 被静默丢弃——告警只会成功发出一次。
		//
		// 已知残留：桶内「故障→恢复→再故障」的第二次故障与其恢复卡仍会被
		// 投递层吞掉，静默窗口 <= 1 小时。根因是同一个 evidenceHash 被两个
		// 目的复用——状态机用它判断「取值是否变化」，投递层用它做「事件轮次
		// 幂等键」，二者语义冲突。彻底修法是为投递另行派生带事件轮次判别量
		// 的证据（需改 incidents 包暴露轮次），本次先用小时桶把窗口压到 1
		// 小时。不要把这个残留描述成「有意的防刷屏」——防重复由状态机的
		// 转移判定和 ConfirmationWindows 负责，时间桶抑制的是全新事件。
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d/%d:%s",
			alert.GroupName, alert.Available, alert.Total, now.In(loc).Format("2006-01-02T15"))))
		evidence := hex.EncodeToString(hash[:])
		// 健康分组同样要 Observe（Failing=false），否则状态机永远走不到恢复
		// 分支，分组恢复后 incident 卡在 confirmed。
		transition, observeErr := machine.Observe(ctx, incidents.Observation{
			Key:                 key,
			Severity:            "P1",
			Failing:             item.Alerting,
			EvidenceHash:        evidence,
			CurrentValue:        fmt.Sprintf("可用 %d / 共 %d", alert.Available, alert.Total),
			ConfirmationWindows: 2,
		})
		if observeErr != nil || !transition.Notify || sender == nil {
			continue
		}
		alert.Recovery = !item.Alerting
		// 单个分组投递失败不得挡住其余分组的告警
		if sendErr := sender.SendIncident(ctx, key, evidence, notify.RenderGroupAlert(alert)); sendErr != nil {
			failures = append(failures, sendErr)
		}
	}
	return errors.Join(failures...)
}
```

必须为它写一个「故障 → 恢复 → 再次故障」的三阶段测试，断言三张卡都投递出去（红、绿、红），且第二次故障的 dedup 证据与第一次不同。再补一个 reader 返回 error 时静默跳过、不产生任何投递的测试。

- [ ] **Step 6: 验证全量测试通过**

Run: `cd relay-ops-service && go build ./... && go test ./... 2>&1 | tail -20`
Expected: 全部包 `ok` 或 `no test files`，无编译错误

- [ ] **Step 7: 提交**

```bash
git add relay-ops-service/internal/dailyreport/ relay-ops-service/internal/scheduler/scheduler.go relay-ops-service/internal/app/app.go
git commit -m "feat: wire health digest and group availability alerting"
```

---

### Task 9: 删除旧渲染链并清理废弃倍率字段

**Files:**
- Modify: `relay-ops-service/internal/notify/feishu.go`（删除 `RenderOperationsDigest` 及其专属辅助函数与视图类型）
- Modify: `relay-ops-service/internal/notify/feishu_test.go`（删除 8 个 `TestRenderOperationsDigest*` 测试）
- Modify: `ops/collect-account-quality-pulse.rb:333-335`、`:345`、`:356-364`、`:366-368`、`:392`
- Modify: `relay-ops-service/internal/accountquality/result.go:41`、`:63`、`:115`、`:166`、`:226`
- Test: `relay-ops-service/internal/accountquality/result_test.go`（既有文件，删除倍率相关断言）

**Interfaces:**
- Removes: `notify.RenderOperationsDigest`、`notify.OperationsDigestView`、`notify.UpstreamAccountStatusView`、`notify.AccountGroupStatusView`、`notify.AccountStatusView`
- Removes: `notify.digestAccountLines`、`digestQualityLines`、`accountStatusLines`、`accountStatusLine`、`digestGroupLines`、`accountEvidenceStatus`、`qualityStatus`（仅在确认无其他调用者后删除）
- Removes: `accountquality.AccountRecord.RateMultiplier`、`accountquality.AccountView.Multiplier`、`accountquality.formatMultiplier`
- Removes: Ruby 端 `rate_multiplier` 方法及其在 sample / summary 中的输出

Task 8 已把日报渲染切到 `RenderHealthDigest`，旧链在生产上再无调用者。`accountrecommendation` 包**必须保留** —— Task 8 的 `buildRecommendations` 仍在使用它。

`ops/collect-account-quality-pulse.rb` 生成的证据 JSON 将不再包含 `rate_multiplier` 键。`accountquality` 的校验逻辑必须同步移除对该键的要求，否则历史证据文件解析失败。

- [ ] **Step 1: 确认旧链无生产调用者**

Run:

```bash
cd relay-ops-service && grep -rn "RenderOperationsDigest\|OperationsDigestView\|UpstreamAccountStatusView" --include="*.go" . | grep -v "_test.go" | grep -v "internal/notify/feishu.go"
```

Expected: 无输出。若有输出，说明 Task 8 未完成或存在计划外调用者，**停止并上报**，不要继续删除。

- [ ] **Step 2: 删除旧渲染链**

在 `relay-ops-service/internal/notify/feishu.go` 中删除：`RenderOperationsDigest` 函数、`OperationsDigestView` / `UpstreamAccountStatusView` / `AccountGroupStatusView` / `AccountStatusView` 类型定义，以及仅被它们调用的辅助函数 `digestGroupLines`、`digestAccountLines`、`digestQualityLines`、`accountStatusLines`、`accountStatusLine`、`accountEvidenceStatus`、`qualityStatus`。

删每个辅助函数前先确认它没有别的调用者：

```bash
cd relay-ops-service && grep -rn "<函数名>" --include="*.go" .
```

`digestValue`、`fitDigestSection`、`shortHash`、`digestTimestamp`、`maxCardBytes` 等被 Task 6/7 的新渲染或其他卡片复用，**不要删除**。

在 `relay-ops-service/internal/notify/feishu_test.go` 中删除全部 8 个 `TestRenderOperationsDigest*` 测试函数。若删除后该文件残留仅供这些测试使用的 helper，一并删除。

- [ ] **Step 3: 验证删除后仍可编译**

Run: `cd relay-ops-service && go build ./... && go vet ./internal/notify/`
Expected: 无输出（编译与静态检查均通过）

- [ ] **Step 4: 移除 Go 端倍率字段**

在 `relay-ops-service/internal/accountquality/result.go`：删除第 41 行 `RateMultiplier *float64` 字段、第 63 行 `AccountView` 中的 `Multiplier`、第 115 行的 `Multiplier: formatMultiplier(...)` 赋值、第 166 行校验条件中的 `(account.RateMultiplier != nil && !finiteNonNegative(*account.RateMultiplier)) ||` 片段，以及第 226 行起的 `formatMultiplier` 函数。删除 `result_test.go` 中所有引用 `RateMultiplier` 或 `Multiplier` 的断言。

若 `finiteNonNegative` 在删除该片段后再无调用者，一并删除；先用 `grep -rn "finiteNonNegative" --include="*.go" .` 确认。

- [ ] **Step 5: 移除 Ruby 端倍率采集**

在 `ops/collect-account-quality-pulse.rb`：删除 `collect_account` 中的 `multiplier = nil` 与 `multiplier = rate_multiplier(@client.account(account_id))` 两行、sample 哈希中的 `"rate_multiplier" => multiplier,`、`rate_multiplier` 方法定义、`failure_sample` 的 `multiplier` 形参及其 `"rate_multiplier" => multiplier,` 输出（调用处同步改为 `failure_sample(account_id)`），以及 summary 哈希中的 `"rate_multiplier" => current.fetch("rate_multiplier"),`。

- [ ] **Step 6: 验证全量测试通过**

Run: `cd relay-ops-service && go build ./... && go test ./... 2>&1 | tail -20`
Expected: 全部通过

Run: `ruby -c ops/collect-account-quality-pulse.rb`
Expected: `Syntax OK`

- [ ] **Step 7: 提交**

```bash
git add ops/collect-account-quality-pulse.rb relay-ops-service/internal/accountquality/ relay-ops-service/internal/notify/
git commit -m "refactor: remove legacy digest renderer and deprecated multiplier"
```

---

## 完成后的验收

全部任务完成后运行：

```bash
cd relay-ops-service && go build ./... && go test ./... 2>&1 | tail -30
```

Expected: 所有包 `ok` 或 `no test files`。

部署前请人工确认三项：日报卡片首屏为质量层且账号显示为用户命名；单账号分组在可用时不产生告警；倍率不可用的账号在利润区显示「无法核算」而非数值。
