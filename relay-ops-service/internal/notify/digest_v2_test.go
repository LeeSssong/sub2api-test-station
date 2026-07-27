package notify

import (
	"fmt"
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
	if strings.Contains(text, "明细") || strings.Contains(text, "TTFT P50") ||
		strings.Contains(text, "account #31") || strings.Contains(text, "账号 #31") {
		t.Fatalf("action morning contains forbidden detail or ID: %s", text)
	}
}

func TestRenderHealthDigestTemplate(t *testing.T) {
	cases := []struct {
		name string
		view HealthDigestView
		want string
	}{
		{"healthy", HealthDigestView{Quality: QualityLine{Healthy: 3}, Profit: ProfitLine{TotalAccounts: 3, PricedAccounts: 3}}, "green"},
		{"degraded", HealthDigestView{Quality: QualityLine{Healthy: 2, Degraded: 1}, Profit: ProfitLine{TotalAccounts: 3, PricedAccounts: 3}}, "orange"},
		{"accounting", HealthDigestView{Quality: QualityLine{Healthy: 3}, Profit: ProfitLine{TotalAccounts: 3, PricedAccounts: 2}, Pending: []PendingItem{{AccountName: "A", Severity: PendingAccounting}}}, "orange"},
		{"recommendation", HealthDigestView{Quality: QualityLine{Healthy: 3}, Profit: ProfitLine{TotalAccounts: 3, PricedAccounts: 3}, Recommendations: []RecommendationLine{{GroupName: "G", CurrentName: "A", CandidateName: "B"}}}, "orange"},
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
		Date:    "2026-07-27",
		Quality: QualityLine{Healthy: 1, Unavailable: 1},
		Profit:  ProfitLine{TotalAccounts: 2, PricedAccounts: 1},
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

func TestRenderHealthDigestKeepsDifferentAccountsWithSameName(t *testing.T) {
	view := HealthDigestView{
		Date:    "2026-07-27",
		Quality: QualityLine{Unavailable: 2},
		Profit:  ProfitLine{TotalAccounts: 2},
		Pending: []PendingItem{
			{AccountID: 41, AccountName: "同名账号", Problem: "余额耗尽", Severity: PendingCritical},
			{AccountID: 42, AccountName: "同名账号", Problem: "HTTP 错误", Severity: PendingWarning},
		},
	}
	text := renderText(t, RenderHealthDigest(view))
	if strings.Count(text, "｜同名账号") != 2 || !strings.Contains(text, "需要处理 · 2") {
		t.Fatalf("different account IDs were merged: %s", text)
	}
}

func TestRenderHealthDigestDataUnavailableDoesNotClaimHealthy(t *testing.T) {
	view := HealthDigestView{
		Date:    "2026-07-27",
		Quality: QualityLine{DataUnavailable: true, DataUnavailableReason: "Sub2API 不可达"},
		Profit:  ProfitLine{TotalAccounts: 11},
	}
	text := renderText(t, RenderHealthDigest(view))
	if !strings.Contains(text, "数据不可用｜Sub2API 不可达") {
		t.Fatalf("missing unavailable reason: %s", text)
	}
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
		Date:    "2026-07-27",
		Quality: QualityLine{Healthy: 3},
		Profit: ProfitLine{
			NoTraffic: true, TotalAccounts: 3, PricedAccounts: 3,
			UpstreamPricedAccounts: 1,
		},
	}
	text := renderText(t, RenderHealthDigest(view))
	for _, want := range []string{
		"今日无有效流量，利润暂不可核算",
		"利润覆盖 3/3 个账号｜1 个采用上游公开定价",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing compact no-traffic conclusion %q: %s", want, text)
		}
	}
	for _, forbidden := range []string{"错误率", "SLA", "站内流量"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("no-traffic card contains noise %q: %s", forbidden, text)
		}
	}
}

func TestRenderHealthDigestUncomputableProfitIsExplicit(t *testing.T) {
	view := HealthDigestView{
		Date:    "2026-07-27",
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

func TestRenderHealthDigestOmitsEmptyRecommendations(t *testing.T) {
	view := HealthDigestView{
		Date:    "2026-07-27",
		Quality: QualityLine{Healthy: 3},
		Profit:  ProfitLine{NoTraffic: true, TotalAccounts: 3, PricedAccounts: 3},
	}
	text := renderText(t, RenderHealthDigest(view))
	if strings.Contains(text, "调整建议") {
		t.Fatalf("empty recommendations rendered a section: %s", text)
	}
}

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
		Date:    "2026-07-27",
		Quality: QualityLine{Unavailable: 107, Degraded: 107, Healthy: 106},
		Profit:  ProfitLine{TotalAccounts: 320, PricedAccounts: 213},
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
