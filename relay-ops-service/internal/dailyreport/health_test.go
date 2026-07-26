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

func TestBuildGroupAlertsOnlyForAlertingGroups(t *testing.T) {
	projection, _, loc, now := fixture()
	alerts := BuildGroupAlerts(projection, loc, now)

	// GPT-Plus 单账号且不可用 -> 可用 0，告警；GPT-Pro 2/2 健康 -> 不告警
	if len(alerts) != 1 {
		t.Fatalf("alerts = %+v, want 1", alerts)
	}
	if alerts[0].GroupName != "GPT-Plus" || alerts[0].Available != 0 || alerts[0].Total != 1 {
		t.Fatalf("alert = %+v", alerts[0])
	}
	if len(alerts[0].Down) != 1 || alerts[0].Down[0].Name != "Plus-XN-0.09" {
		t.Fatalf("Down = %+v", alerts[0].Down)
	}
}
