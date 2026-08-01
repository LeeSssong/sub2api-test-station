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
		{300, 691},   // ceil(172800/300)=576, *1.2=691.2 -> 691
		{0, 691},     // 非法值回退 300 秒
		{-5, 691},    // 非法值回退 300 秒
		{60, 2000},   // ceil(172800/60)=2880, *1.2=3456 -> 上限 2000
		{86400, 100}, // ceil(172800/86400)=2, *1.2=2.4 -> 下限 100
	}
	for _, tc := range cases {
		if got := HistoryLimitFor(tc.interval); got != tc.want {
			t.Fatalf("HistoryLimitFor(%d) = %d, want %d", tc.interval, got, tc.want)
		}
	}
}

func TestRollingWindowLimitFor(t *testing.T) {
	cases := []struct {
		interval int
		want     int
	}{
		{300, 18},  // ceil(3600/300)=12, *1.5=18
		{0, 18},    // 非法值回退 300 秒
		{-5, 18},   // 非法值回退 300 秒
		{60, 90},   // ceil(3600/60)=60, *1.5=90
		{7200, 12}, // ceil(3600/7200)=1, *1.5=1.5 -> 2 -> 下限 12
		{250, 23},  // ceil(3600/250)=15, *1.5=22.5 -> 23
	}
	for _, tc := range cases {
		if got := RollingWindowLimitFor(tc.interval); got != tc.want {
			t.Fatalf("RollingWindowLimitFor(%d) = %d, want %d", tc.interval, got, tc.want)
		}
	}
}

func TestRollingWindowLimitForIsClamped(t *testing.T) {
	cases := []struct {
		name     string
		interval int
		want     int
	}{
		{"当前生产间隔", 300, 18},
		{"非法值回退 300 秒", 0, 18},
		{"负值回退 300 秒", -5, 18},
		{"密集探测撞上限", 1, 200},
		// 简报原值为 72，但按简报自身实现（slack=1.5）ceil(60*1.5)=90；
		// 72 只在 slack=1.2 时成立，而那会把 300 秒间隔算成 15、
		// 破坏 internal/app 端到端测试断言的 limit==18。取 90。
		{"60 秒间隔在区间内", 60, 90},
		{"18 秒间隔恰好触顶", 18, 200},
		{"稀疏探测撞下限", 3600, 12},
		{"超长间隔仍给下限", 86400, 12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RollingWindowLimitFor(tc.interval); got != tc.want {
				t.Fatalf("RollingWindowLimitFor(%d) = %d, want %d", tc.interval, got, tc.want)
			}
		})
	}
}

func TestAggregateUsesHalfOpenWindow(t *testing.T) {
	from := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	entries := []HistoryEntry{
		{CheckedAt: from.Add(-time.Second), Status: "success"},                    // 窗口前，排除
		{CheckedAt: from, Status: "success", TTFTMS: float64Ptr(1000)},            // 左闭，计入
		{CheckedAt: to.Add(-time.Second), Status: "failed", ErrorCode: "timeout"}, // 窗口内，计入
		{CheckedAt: to, Status: "failed", ErrorCode: "http_error"},                // 右开，排除
	}
	slice := Aggregate(entries, from, to)
	if slice.SampleCount != 2 || slice.SuccessCount != 1 {
		t.Fatalf("slice = %+v, want 2 samples 1 success", slice)
	}
	if slice.SuccessRate != 0.5 {
		t.Fatalf("SuccessRate = %v, want 0.5", slice.SuccessRate)
	}
	if slice.LastErrorCode != "timeout" {
		t.Fatalf("LastErrorCode = %q, want timeout（窗口外的 http_error 不得混入）", slice.LastErrorCode)
	}
}

func TestAccountSampleFromMapsSliceFields(t *testing.T) {
	ttft := 1234.0
	slice := DaySlice{
		Date: "2026-07-27", SampleCount: 12, SuccessCount: 6,
		SuccessRate: 0.5, TTFTP95MS: &ttft, LastErrorCode: "timeout",
	}
	sample := AccountSampleFrom(slice, 22, "Plus-XN-0.09", []string{"GPT-Plus"})
	if sample.AccountID != 22 || sample.Name != "Plus-XN-0.09" {
		t.Fatalf("identity lost: %+v", sample)
	}
	if len(sample.GroupNames) != 1 || sample.GroupNames[0] != "GPT-Plus" {
		t.Fatalf("GroupNames = %v", sample.GroupNames)
	}
	if sample.SuccessRate != 0.5 || sample.SampleCount != 12 || sample.ErrorCode != "timeout" {
		t.Fatalf("sample = %+v", sample)
	}
	if sample.TTFTP95MS == nil || *sample.TTFTP95MS != 1234 {
		t.Fatalf("TTFTP95MS = %v", sample.TTFTP95MS)
	}
	verdict := ClassifyAccount(sample)
	if verdict.Tier != TierDegraded {
		t.Fatalf("Tier = %q, want degraded（聚合结果可直接进入判定）", verdict.Tier)
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

func TestAggregateUsesLatestProbeForErrorCode(t *testing.T) {
	from := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)

	latestSuccess := Aggregate([]HistoryEntry{
		{CheckedAt: from.Add(10 * time.Minute), Status: "failed", ErrorCode: ErrorCodeBalanceExhausted},
		{CheckedAt: from.Add(20 * time.Minute), Status: "success"},
	}, from, to)
	if latestSuccess.LastErrorCode != "" {
		t.Fatalf("LastErrorCode = %q, want empty after latest success", latestSuccess.LastErrorCode)
	}
	if latestSuccess.LatestStatus != statusSuccess {
		t.Fatalf("LatestStatus = %q, want success", latestSuccess.LatestStatus)
	}

	latestFailure := Aggregate([]HistoryEntry{
		{CheckedAt: from.Add(10 * time.Minute), Status: "success"},
		{CheckedAt: from.Add(20 * time.Minute), Status: "failed", ErrorCode: ErrorCodeBalanceExhausted},
	}, from, to)
	if latestFailure.LastErrorCode != ErrorCodeBalanceExhausted {
		t.Fatalf("LastErrorCode = %q, want %q after latest failure", latestFailure.LastErrorCode, ErrorCodeBalanceExhausted)
	}
	if latestFailure.LatestStatus != "failed" {
		t.Fatalf("LatestStatus = %q, want failed", latestFailure.LatestStatus)
	}

	latestFailureWithoutCode := Aggregate([]HistoryEntry{
		{CheckedAt: from.Add(10 * time.Minute), Status: "failed", ErrorCode: ErrorCodeBalanceExhausted},
		{CheckedAt: from.Add(20 * time.Minute), Status: "failed"},
	}, from, to)
	if latestFailureWithoutCode.LastErrorCode != "" {
		t.Fatalf("LastErrorCode = %q, want empty when latest failure has no code", latestFailureWithoutCode.LastErrorCode)
	}
}

func TestAggregateKeepsFirstInputAsNewestWhenTimestampsTie(t *testing.T) {
	checkedAt := time.Date(2026, 7, 27, 8, 20, 0, 0, time.UTC)
	slice := Aggregate([]HistoryEntry{
		// 上游历史接口按 checked_at DESC, id DESC 返回；时间相同时，较新的
		// id 在前。聚合必须保留第一个结果，不能因稳定升序排序被旧记录覆盖。
		{CheckedAt: checkedAt, Status: "success"},
		{CheckedAt: checkedAt, Status: "failed", ErrorCode: ErrorCodeBalanceExhausted},
	}, checkedAt.Add(-time.Hour), checkedAt.Add(time.Hour))
	if slice.LatestStatus != statusSuccess || slice.LastErrorCode != "" {
		t.Fatalf("slice = %+v, want first same-time entry to remain latest success", slice)
	}
}
