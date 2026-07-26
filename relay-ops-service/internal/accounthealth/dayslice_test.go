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
