package notify

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxMorningActions = 8

type QualityLine struct {
	Healthy      int
	Degraded     int
	Unavailable  int
	Slow         int
	HealthyDelta *int
	// Median across accounts of each account's TTFT P95, not a median TTFT.
	TTFTP95MedianMS       *float64
	DataUnavailable       bool
	DataUnavailableReason string
}

type ProfitLine struct {
	Revenue                float64
	UpstreamCost           float64
	Gross                  float64
	Margin                 *float64
	Computable             bool
	ExcludedAccounts       int
	UnsupportedAccounts    int
	TotalAccounts          int
	PricedAccounts         int
	UpstreamPricedAccounts int
	NoTraffic              bool
}

type PendingSeverity string

const (
	PendingCritical   PendingSeverity = "critical"
	PendingWarning    PendingSeverity = "warning"
	PendingAccounting PendingSeverity = "accounting"
)

type PendingItem struct {
	AccountID   int64
	AccountName string
	Problem     string
	Detail      string
	Severity    PendingSeverity
}

type TrafficLine struct {
	HasTraffic bool
	Requests   int64
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
	Traffic         TrafficLine
}

func RenderHealthDigest(view HealthDigestView) FeishuMessage {
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
	elements = append(elements, CardElement{Tag: "action", Actions: []CardAction{{
		Tag: "button", Text: CardText{Tag: "plain_text", Content: "运维后台"},
		Type: "primary", MultiURL: &CardURL{URL: "/ops"},
	}}})

	return FeishuMessage{MsgType: "interactive", Card: &Card{
		Config: CardConfig{WideScreenMode: true},
		Header: CardHeader{
			Title:    CardText{Tag: "plain_text", Content: "中转站晨报 · " + morningDate(view.Date)},
			Template: digestTemplate(view),
		},
		Elements: elements,
	}}
}

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

func qualityLines(quality QualityLine) []string {
	lines := []string{"**运行概览**"}
	if quality.DataUnavailable {
		reason := digestValue(quality.DataUnavailableReason)
		if reason == "" {
			reason = "原因未知"
		}
		return append(lines, "数据不可用｜"+reason)
	}

	summary := fmt.Sprintf("%d 个稳定｜%d 个降级｜%d 个不可用",
		quality.Healthy, quality.Degraded, quality.Unavailable)
	if quality.HealthyDelta != nil {
		summary += "｜较昨日 " + signedDelta(*quality.HealthyDelta)
	}
	lines = append(lines, summary)

	detail := "P95 中位 " + formatMillis(quality.TTFTP95MedianMS)
	if quality.Slow > 0 {
		detail += fmt.Sprintf("｜%d 个偏慢", quality.Slow)
	}
	return append(lines, detail)
}

func profitLines(profit ProfitLine, traffic TrafficLine) []string {
	lines := []string{"**经营情况**"}
	switch {
	case profit.NoTraffic:
		lines = append(lines, "今日无有效流量，利润暂不可核算")
	case !profit.Computable:
		lines = append(lines, fmt.Sprintf("利润暂不可核算｜%d 个账号倍率不可用", profit.ExcludedAccounts))
	default:
		lines = append(lines,
			fmt.Sprintf("请求 %d｜收入 %s｜成本 %s",
				traffic.Requests, formatUSD(profit.Revenue), formatUSD(profit.UpstreamCost)),
			fmt.Sprintf("毛利 %s｜毛利率 %s", formatUSD(profit.Gross), formatMargin(profit.Margin)),
		)
	}
	if profit.TotalAccounts > 0 {
		lines = append(lines, fmt.Sprintf("利润覆盖 %d/%d 个账号｜%d 个采用上游公开定价",
			profit.PricedAccounts, profit.TotalAccounts, profit.UpstreamPricedAccounts))
	}
	return lines
}

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

func actionLines(view HealthDigestView) []string {
	items := normalizePending(view.Pending)
	lines := []string{fmt.Sprintf("**需要处理 · %d**", len(items))}
	if len(items) > 0 {
		kept := items
		if len(kept) > maxMorningActions {
			kept = kept[:maxMorningActions]
		}
		for _, item := range kept {
			name := digestValue(item.AccountName)
			if name == "" {
				name = "账号名称不可用"
			}
			title := fmt.Sprintf("**%s｜%s**", severityLabel(item.Severity), name)
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

	clear := view.Profit.TotalAccounts - len(items)
	if clear < 0 {
		clear = 0
	}
	return append(lines, fmt.Sprintf("其余 %d 个账号无待处理项", clear))
}

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
