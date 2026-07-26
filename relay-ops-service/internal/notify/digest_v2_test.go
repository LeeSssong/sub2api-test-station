package notify

import (
	"strings"
	"testing"
)

func intPtr(v int) *int         { return &v }
func f64Ptr(v float64) *float64 { return &v }

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

func TestRenderHealthDigestFitsCardLimitWithManyPendingItems(t *testing.T) {
	pending := make([]PendingItem, 200)
	for i := range pending {
		pending[i] = PendingItem{
			AccountName: strings.Repeat("P", 60),
			Problem:     "余额耗尽",
			Detail:      "已持续 3 天，需要人工介入处理",
		}
	}
	view := HealthDigestView{
		Date: "2026-07-27", Quality: QualityLine{Healthy: 200},
		Profit:  ProfitLine{NoTraffic: true},
		Pending: pending,
		Traffic: TrafficLine{HasTraffic: false},
	}
	payload, err := RenderHealthDigest(view).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	if len(payload) > maxCardBytes {
		t.Fatalf("card size = %d, exceeds %d", len(payload), maxCardBytes)
	}
	if !strings.Contains(string(payload), "其余对象请在 /ops 查看") {
		t.Fatal("待处理层被截断时必须提示去 /ops 查看")
	}
}
