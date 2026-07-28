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

type DigestNotificationSummary struct {
	ActiveP0              int
	ActiveP1              int
	Recovered             int
	PricingEvents         int
	TrackedPublicGroups   int
	FreshCapacityGroups   int
	PricingSources        int
	TrackedPricingSources int
}

type HealthDigestView struct {
	Date         string
	PublicGroups int
	Summary      DigestNotificationSummary
	Quality      QualityLine
	Profit       ProfitLine
	Pending      []PendingItem
	Traffic      TrafficLine
}

func RenderHealthDigest(view HealthDigestView) FeishuMessage {
	elements := []CardElement{
		{Tag: "div", Text: &CardText{
			Tag: "lark_md", Content: fitDigestSection(conclusionLines(view)),
		}},
		{Tag: "div", Text: &CardText{
			Tag: "lark_md", Content: fitDigestSection(userRuntimeLines(view)),
		}},
		{Tag: "div", Text: &CardText{
			Tag: "lark_md", Content: fitDigestSection(actionLines(view)),
		}},
		{Tag: "div", Text: &CardText{
			Tag: "lark_md", Content: fitDigestSection(profitLines(view)),
		}},
		{Tag: "div", Text: &CardText{
			Tag: "lark_md", Content: fitDigestSection(monitoringLines(view)),
		}},
	}
	elements = append(elements, CardElement{Tag: "action", Actions: []CardAction{{
		Tag: "button", Text: CardText{Tag: "plain_text", Content: "运维后台"},
		Type: "primary", MultiURL: &CardURL{URL: "/ops"},
	}}})

	return FeishuMessage{MsgType: "interactive", Card: &Card{
		Config: CardConfig{WideScreenMode: true},
		Header: CardHeader{
			Title:    CardText{Tag: "plain_text", Content: "中转站晨报｜" + morningDate(view.Date)},
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
	case view.Summary.ActiveP0 > 0 || view.Quality.Unavailable > 0:
		return "red"
	case view.Quality.DataUnavailable ||
		view.Summary.ActiveP1 > 0 ||
		view.Quality.Degraded > 0 ||
		len(view.Pending) > 0 ||
		view.Summary.PricingEvents > 0 ||
		!monitoringComplete(view):
		return "orange"
	default:
		return "green"
	}
}

func conclusionLines(view HealthDigestView) []string {
	active := view.Summary.ActiveP0 + view.Summary.ActiveP1
	needs := len(normalizePending(view.Pending)) + view.Summary.PricingEvents
	return []string{
		"**一句话结论**",
		fmt.Sprintf("%d 个公开分组，%d 起进行中用户事故，%d 项需要处理。",
			view.PublicGroups, active, needs),
	}
}

func userRuntimeLines(view HealthDigestView) []string {
	lines := []string{
		"**用户侧运行**",
		fmt.Sprintf("公开分组 %d｜P0 %d｜P1 %d",
			view.PublicGroups, view.Summary.ActiveP0, view.Summary.ActiveP1),
		fmt.Sprintf("昨日恢复 %d 起", view.Summary.Recovered),
	}
	if view.Quality.DataUnavailable {
		reason := digestValue(view.Quality.DataUnavailableReason)
		if reason == "" {
			reason = "原因未知"
		}
		return append(lines, "账号质量：数据不可用｜"+reason)
	}
	summary := fmt.Sprintf("账号状态：%d 个稳定｜%d 个降级｜%d 个不可用",
		view.Quality.Healthy, view.Quality.Degraded, view.Quality.Unavailable)
	if view.Quality.HealthyDelta != nil {
		summary += "｜较昨日 " + signedDelta(*view.Quality.HealthyDelta)
	}
	lines = append(lines, summary)
	detail := "响应首字节 P95 中位 " + formatMillis(view.Quality.TTFTP95MedianMS)
	if view.Quality.Slow > 0 {
		detail += fmt.Sprintf("｜%d 个偏慢", view.Quality.Slow)
	}
	return append(lines, detail)
}

func profitLines(view HealthDigestView) []string {
	lines := []string{"**经营情况**"}
	if view.Quality.DataUnavailable {
		return append(lines, "账号监控数据不可用，经营数据暂不可核算。")
	}
	profit := view.Profit
	traffic := view.Traffic
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
	total := len(items) + view.Summary.PricingEvents
	lines := []string{fmt.Sprintf("**需要处理 · %d**", total)}
	if view.Summary.PricingEvents > 0 {
		lines = append(lines, fmt.Sprintf(
			"**价格核对｜公开定价变化 %d 项**\n请核对当前售价与毛利。",
			view.Summary.PricingEvents,
		))
	}
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
	if !view.Quality.DataUnavailable && view.Profit.TotalAccounts > 0 {
		lines = append(lines, clearAccountLine(view))
	}

	return lines
}

func clearAccountLine(view HealthDigestView) string {
	clear := view.Profit.TotalAccounts - len(normalizePending(view.Pending))
	if clear < 0 {
		clear = 0
	}
	return fmt.Sprintf("其余 %d 个账号无待处理项", clear)
}

func monitoringLines(view HealthDigestView) []string {
	lines := []string{"**监控完整性**"}
	issues := make([]string, 0, 4)
	if view.Quality.DataUnavailable {
		reason := digestValue(view.Quality.DataUnavailableReason)
		if reason == "" {
			reason = "账号质量来源不可用"
		}
		issues = append(issues, reason)
	}
	if view.Summary.TrackedPublicGroups != view.PublicGroups {
		issues = append(issues, fmt.Sprintf(
			"公开分组同步 %d/%d",
			view.Summary.TrackedPublicGroups,
			view.PublicGroups,
		))
	}
	if view.Summary.FreshCapacityGroups < view.PublicGroups {
		issues = append(issues, fmt.Sprintf(
			"容量证据 %d/%d 为新鲜状态",
			view.Summary.FreshCapacityGroups,
			view.PublicGroups,
		))
	}
	if view.Summary.TrackedPricingSources < view.Summary.PricingSources {
		issues = append(issues, fmt.Sprintf(
			"生产定价来源 %d/%d 已建立可靠基线",
			view.Summary.TrackedPricingSources,
			view.Summary.PricingSources,
		))
	}
	if len(issues) == 0 {
		return append(lines, "真实流量、账号质量、容量证据和生产定价来源均已读取。")
	}
	return append(lines, issues...)
}

func monitoringComplete(view HealthDigestView) bool {
	return !view.Quality.DataUnavailable &&
		view.Summary.TrackedPublicGroups == view.PublicGroups &&
		view.Summary.FreshCapacityGroups >= view.PublicGroups &&
		view.Summary.TrackedPricingSources >= view.Summary.PricingSources
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
