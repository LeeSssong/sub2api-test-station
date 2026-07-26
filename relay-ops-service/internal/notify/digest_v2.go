package notify

import (
	"fmt"
	"strconv"
)

type QualityLine struct {
	Healthy      int
	Degraded     int
	Unavailable  int
	Slow         int
	HealthyDelta *int
	// Median across accounts of each account's TTFT P95 — not a median TTFT.
	// The old name said "median" alone and invited exactly that misreading.
	TTFTP95MedianMS       *float64
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
	// UnsupportedAccounts counts, among ExcludedAccounts, the accounts whose
	// upstream simply cannot be auto-measured (measured+failed). They are
	// excluded from profit all the same, but the copy must say why instead of
	// implying someone can fix it.
	UnsupportedAccounts int
	NoTraffic           bool
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
	// Each layer must go through fitDigestSection rather than a bare
	// strings.Join: the pending layer grows linearly with the number of
	// accounts (one row per abnormal account plus recommendations) and is
	// otherwise unbounded. An oversized section makes CardJSON return an
	// error, which drops the entire digest instead of degrading gracefully.
	// Four sections capped at maxDigestSectionBytes (4 KiB) each stay well
	// below the maxCardBytes (30 KiB) limit.
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
	detail := "延时 P95 中位 " + formatMillis(quality.TTFTP95MedianMS)
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
		switch {
		case profit.UnsupportedAccounts >= profit.ExcludedAccounts:
			lines = append(lines, fmt.Sprintf("%d 个账号上游不支持自动测算，未计入", profit.ExcludedAccounts))
		case profit.UnsupportedAccounts > 0:
			lines = append(lines, fmt.Sprintf("%d 个账号因倍率不可用未计入（其中 %d 个上游不支持自动测算）",
				profit.ExcludedAccounts, profit.UnsupportedAccounts))
		default:
			lines = append(lines, fmt.Sprintf("%d 个账号因倍率不可用未计入", profit.ExcludedAccounts))
		}
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
	// The truncation notice leads the account rows: fitDigestSection consumes
	// its byte budget in order, so a trailing notice would be the first line
	// dropped exactly when truncation happened and the notice matters most.
	if truncated > 0 {
		lines = append(lines, fmt.Sprintf("（已截断 %d 个账号，完整明细见运维后台）", truncated))
	}
	for _, account := range kept {
		lines = append(lines, fmt.Sprintf("- %s：成功率 %s · TTFT %s · 延迟 P95 %s · 倍率 %s · 毛利 %s",
			digestValue(account.Name), digestValue(account.SuccessRate), digestValue(account.TTFTP50),
			digestValue(account.LatencyP95), digestValue(account.Multiplier), digestValue(account.GrossContribution)))
	}
	if len(kept) == 0 {
		lines = append(lines, "无账号明细")
	}
	if view.Traffic.HasTraffic {
		lines = append(lines, fmt.Sprintf("站内流量：请求 %s · 错误率 %s · SLA %s",
			strconv.FormatInt(view.Traffic.Requests, 10), digestValueOrDash(view.Traffic.ErrorRate), digestValueOrDash(view.Traffic.SLA)))
	} else {
		lines = append(lines, "站内流量：今日无真实调用")
	}
	return lines
}

// fitAccountLines caps the detail layer at a fixed number of accounts and
// reports how many were dropped so the section can carry an explicit
// "已截断 N 个账号" notice. It does not provide any byte-level guarantee —
// that comes from fitDigestSection, which every rendered layer goes through.
func fitAccountLines(accounts []AccountDetailLine) ([]AccountDetailLine, int) {
	const maxDetailAccounts = 40
	if len(accounts) <= maxDetailAccounts {
		return accounts, 0
	}
	return accounts[:maxDetailAccounts], len(accounts) - maxDetailAccounts
}

// digestValueOrDash renders an absent metric as an explicit "—" instead of a
// blank: `错误率  · SLA ` reads like a rendering bug, `错误率 — · SLA —` reads
// like the metric is not collected.
func digestValueOrDash(value string) string {
	if rendered := digestValue(value); rendered != "" {
		return rendered
	}
	return "—"
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
