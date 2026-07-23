package notify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/accountquality"
	"example.invalid/relay-ops-service/internal/feishuapi"
	"example.invalid/relay-ops-service/internal/opsmetrics"
	"example.invalid/relay-ops-service/internal/pricing"
)

const maxCardBytes = 30 << 10

type Link struct{ Label, URL string }

type IncidentView struct {
	Title       string
	Severity    string
	WhatWasDone []string
	Results     []string
	Change      string
	Focus       string
	Links       []Link
	Current     string
	Baseline    string
	Duration    string
	Analysis    string
	Recovery    bool
	Suppressed  bool
}

type CommandView struct {
	Command    string
	ActorID    string
	Status     string
	ErrorCode  string
	GroupName  string
	TargetRole string
	AuditID    string
	DryRun     bool
	Unknown    bool
}

type UpstreamReportView struct {
	Title        string
	Status       string
	QualityScore int
	TotalScore   int
	Direct       string
	Gateway      string
	Models       string
	Pricing      string
	Capacity     string
	Unknowns     []string
	ReportID     string
	ReportHash   string
	Links        []Link
}

// OperationsDigestView is a secret-free projection of the two read-only
// operations domains shown in the daily report.
type OperationsDigestView struct {
	Date           string
	Runtime        opsmetrics.Snapshot
	AccountQuality accountquality.View
	Footer         []string
}

type CardText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type CardConfig struct {
	WideScreenMode bool `json:"wide_screen_mode,omitempty"`
}

type CardHeader struct {
	Title    CardText `json:"title"`
	Template string   `json:"template"`
}

type CardAction struct {
	Tag      string   `json:"tag"`
	Text     CardText `json:"text"`
	Type     string   `json:"type,omitempty"`
	MultiURL *CardURL `json:"multi_url,omitempty"`
}

type CardURL struct {
	URL string `json:"url"`
}

type CardElement struct {
	Tag     string       `json:"tag"`
	Content string       `json:"content,omitempty"`
	Text    *CardText    `json:"text,omitempty"`
	Actions []CardAction `json:"actions,omitempty"`
	Margin  string       `json:"margin,omitempty"`
}

type Card struct {
	Config   CardConfig    `json:"config"`
	Header   CardHeader    `json:"header"`
	Elements []CardElement `json:"elements"`
}

type FeishuMessage struct {
	MsgType string `json:"msg_type"`
	Content struct {
		Text string `json:"text,omitempty"`
	} `json:"content,omitempty"`
	Card *Card `json:"-"`
}

func RenderFeishu(event IncidentView) FeishuMessage {
	if event.Recovery {
		return RenderRecovery(event)
	}
	return RenderAlert(event)
}

func RenderAlert(event IncidentView) FeishuMessage {
	severity := strings.ToUpper(strings.TrimSpace(event.Severity))
	template := "orange"
	if severity == "P0" || severity == "P1" || strings.Contains(strings.ToLower(event.Title), "异常") {
		template = "red"
	}
	sections := []string{
		"**状态** 需要关注",
		"**影响** " + defaultText(event.Current),
		"**基线** " + defaultText(event.Baseline),
		"**已执行** " + joinItems(event.WhatWasDone),
		"**观测结果** " + joinItems(event.Results),
		"**发生变化** " + defaultText(event.Change),
		"**需要关注** " + defaultText(event.Focus),
	}
	if event.Analysis != "" {
		sections = append(sections, "**只读分析** "+safeValue(event.Analysis))
	}
	if event.Suppressed {
		sections = append(sections, "**去重** 重复事件已静默")
	}
	return newCardMessage(event.Title, template, strings.Join(sections, "\n\n"), event.Links)
}

func RenderRecovery(event IncidentView) FeishuMessage {
	sections := []string{
		"**状态** 已恢复",
		"**恢复结果** " + joinItems(event.Results),
		"**持续时间** " + defaultText(event.Duration),
		"**变化** " + defaultText(event.Change),
		"**后续观察** " + defaultText(event.Focus),
	}
	if event.Suppressed {
		sections = append(sections, "**去重** 告警重复已抑制")
	}
	return newCardMessage(event.Title, "green", strings.Join(sections, "\n\n"), event.Links)
}

func RenderCommand(event CommandView) FeishuMessage {
	template := "blue"
	status := strings.ToLower(strings.TrimSpace(event.Status))
	if status == "succeeded" {
		template = "green"
	} else if status == "failed" || event.ErrorCode == "routing_failed" {
		template = "red"
	}
	if event.Unknown {
		event.ErrorCode = "unknown_command"
		event.Command = "未识别命令"
	}
	sections := []string{
		"**命令** " + defaultText(event.Command),
		"**执行者** " + shortIdentity(event.ActorID),
		"**结果** " + defaultText(event.Status),
	}
	if event.GroupName != "" {
		sections = append(sections, "**分组** "+safeValue(event.GroupName))
	}
	if event.TargetRole != "" {
		sections = append(sections, "**目标** "+safeValue(event.TargetRole))
	}
	if event.DryRun {
		sections = append(sections, "**执行模式** dry-run，仅预测，未写入路由")
	}
	if event.ErrorCode != "" {
		sections = append(sections, "**错误码** "+safeValue(event.ErrorCode))
	}
	if event.Unknown {
		sections = append(sections, "**可用命令** 查询当前分组状态；切换 GPT-Pro 到灾备；恢复 GPT-Pro 主分组；切换 GPT-Plus 到灾备；恢复 GPT-Plus 主分组")
	}
	sections = append(sections, "**审计** "+defaultText(event.AuditID))
	return newCardMessage("飞书命令执行结果", template, strings.Join(sections, "\n\n"), nil)
}

func RenderUpstreamReport(report UpstreamReportView) FeishuMessage {
	template := "blue"
	switch report.Status {
	case "blocked":
		template = "red"
	case "eligible_for_manual_switch", "review_recommended":
		template = "green"
	case "needs_evidence":
		template = "orange"
	}
	sections := []string{
		"**状态** " + defaultText(report.Status),
		fmt.Sprintf("**评分** 质量分 %d / 90，总分 %d / 100", report.QualityScore, report.TotalScore),
		"**直连质量** " + defaultText(report.Direct),
		"**网关质量** " + defaultText(report.Gateway),
		"**模型** " + defaultText(report.Models),
		"**价格与余额** " + defaultText(report.Pricing),
		"**容量下界** " + defaultText(report.Capacity),
		"**未知项** " + joinItems(report.Unknowns),
		"**报告** " + defaultText(report.ReportID),
		"**报告哈希** " + shortHash(report.ReportHash),
		"**操作边界** 请在运维后台人工复核；本通知不执行切换",
	}
	return newCardMessage(report.Title, template, strings.Join(sections, "\n\n"), report.Links)
}

func RenderOperationsDigest(view OperationsDigestView) FeishuMessage {
	date := strings.TrimSpace(view.Date)
	if date == "" {
		date = "未提供日期"
	}
	siteLines := []string{"**站内运行**"}
	if !view.Runtime.CapturedAt.IsZero() {
		siteLines = append(siteLines, "采集时间 "+view.Runtime.CapturedAt.UTC().Format("2006-01-02 15:04 UTC"))
	}
	siteLines = append(siteLines, "**公开分组**")
	siteLines = append(siteLines, digestGroupLines(view.Runtime.Groups)...)
	siteLines = append(siteLines, "**当前调度账号**")
	siteLines = append(siteLines, digestAccountLines(view.Runtime.Accounts)...)

	qualityLines := []string{"**上游账号质量**", "数据状态 " + qualityStatus(view.AccountQuality)}
	if view.AccountQuality.ObservedAt != "" {
		qualityLines = append(qualityLines, "采样时间 "+digestValue(view.AccountQuality.ObservedAt))
	}
	qualityLines = append(qualityLines, digestQualityLines(view.AccountQuality.Accounts)...)

	elements := []CardElement{
		{Tag: "div", Text: &CardText{Tag: "lark_md", Content: fitDigestSection(siteLines)}},
		{Tag: "div", Text: &CardText{Tag: "lark_md", Content: fitDigestSection(qualityLines)}},
	}
	if len(view.Footer) > 0 {
		footer := []string{"**补充证据**"}
		footer = append(footer, digestFooterLines(view.Footer)...)
		elements = append(elements, CardElement{Tag: "div", Text: &CardText{Tag: "lark_md", Content: fitDigestSection(footer)}})
	}
	elements = append(elements, CardElement{Tag: "action", Actions: []CardAction{{
		Tag: "button", Text: CardText{Tag: "plain_text", Content: "运维后台"}, Type: "primary", MultiURL: &CardURL{URL: "/ops"},
	}}})
	message := FeishuMessage{MsgType: "interactive", Card: &Card{
		Config:   CardConfig{WideScreenMode: true},
		Header:   CardHeader{Title: CardText{Tag: "plain_text", Content: "relay-ops 每日运营摘要 " + digestValue(date)}, Template: "blue"},
		Elements: elements,
	}}
	// TextProjection is only a local inspection aid; delivery always serializes CardJSON.
	message.Content.Text = message.Card.Header.Title.Content
	return message
}

func digestGroupLines(groups []opsmetrics.GroupRuntime) []string {
	rows := append([]opsmetrics.GroupRuntime(nil), groups...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ID != rows[j].ID {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].Name < rows[j].Name
	})
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Status == opsmetrics.StatusReadFailed {
			lines = append(lines, fmt.Sprintf("- %s：读取失败", digestValue(row.Name)))
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s：请求 %d，错误率 %.2f%%，SLA %.2f%%，TTFT P95 %.0fms，%s",
			digestValue(row.Name), row.RequestCount, row.ErrorRate*100, row.SLA, row.TTFTP95MS, runtimeStatus(row.Status)))
	}
	if len(lines) == 0 {
		return []string{"- 暂无公开分组运行数据"}
	}
	return lines
}

func digestAccountLines(accounts []opsmetrics.AccountRuntime) []string {
	rows := append([]opsmetrics.AccountRuntime(nil), accounts...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		label := "账号 " + strconv.FormatInt(row.ID, 10)
		if row.Status == opsmetrics.StatusReadFailed {
			lines = append(lines, label+"：读取失败")
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s：请求 %d，错误率 %.2f%%，SLA %.2f%%，TTFT P95 %.0fms，%s",
			label, row.RequestCount, row.ErrorRate*100, row.SLA, row.TTFTP95MS, runtimeStatus(row.Status)))
	}
	if len(lines) == 0 {
		return []string{"- 当前没有调度账号"}
	}
	return lines
}

func digestQualityLines(accounts []accountquality.AccountView) []string {
	rows := append([]accountquality.AccountView(nil), accounts...)
	sort.SliceStable(rows, func(i, j int) bool {
		left, leftErr := strconv.ParseInt(rows[i].AccountID, 10, 64)
		right, rightErr := strconv.ParseInt(rows[j].AccountID, 10, 64)
		if leftErr == nil && rightErr == nil && left != right {
			return left < right
		}
		return rows[i].AccountID < rows[j].AccountID
	})
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf("- 账号 %s：稳定性 %s，倍率 %s，TTFT P95 %s，最近结果 %s",
			digestValue(row.AccountID), digestValue(row.Stability), digestValue(row.Multiplier), digestValue(row.TTFTP95), digestValue(row.LastResult)))
	}
	if len(lines) == 0 {
		return []string{"- 尚无账号质量证据"}
	}
	return lines
}

func digestFooterLines(values []string) []string {
	lines := make([]string, 0, len(values))
	for _, value := range values {
		if value = digestValue(value); value != "" && value != "[已脱敏]" {
			lines = append(lines, "- "+value)
		}
	}
	if len(lines) == 0 {
		return []string{"- 无"}
	}
	return lines
}

func runtimeStatus(status string) string {
	switch status {
	case opsmetrics.StatusOK:
		return "正常"
	case opsmetrics.StatusSampleInsufficient:
		return "样本不足"
	case opsmetrics.StatusReadFailed:
		return "读取失败"
	default:
		return "状态未知"
	}
}

func qualityStatus(view accountquality.View) string {
	if view.Stale {
		return "证据已过期"
	}
	if !view.Available {
		return "等待证据"
	}
	return "巡检正常"
}

const maxDigestSectionBytes = 8 << 10

func fitDigestSection(lines []string) string {
	var builder strings.Builder
	for index, line := range lines {
		line = trimDigestText(line, 768)
		addition := len(line)
		if index > 0 {
			addition++
		}
		if builder.Len()+addition > maxDigestSectionBytes {
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString("- 其余对象请在 /ops 查看")
			break
		}
		if index > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(line)
	}
	return builder.String()
}

func trimDigestText(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum] + "..."
}

func digestValue(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	for _, marker := range []string{"http://", "https://", "base url", "base_url", "api_key", "api key", "model response", "response text", "ou-"} {
		if strings.Contains(lower, marker) {
			return "[已脱敏]"
		}
	}
	return safeValue(value)
}

func shortHash(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 16 {
		return safeValue(value)
	}
	return safeValue(value[:12] + "..." + value[len(value)-4:])
}

func RenderSessionExpired(upstream, loginURL string) FeishuMessage {
	return RenderAlert(IncidentView{
		Title:       "上游用量读取会话失效：" + upstream,
		Severity:    "P1",
		WhatWasDone: []string{"读取上游用量页面并在 401 后重试 1 次"},
		Results:     []string{"质量和公开价格监控正常；真实消费核对暂停"},
		Change:      "登录会话已失效",
		Focus:       "打开登录链接并重新登录一次",
		Links:       []Link{{Label: "重新登录", URL: loginURL}},
	})
}

func newCardMessage(title, template, markdown string, links []Link) FeishuMessage {
	card := &Card{
		Config:   CardConfig{WideScreenMode: true},
		Header:   CardHeader{Title: CardText{Tag: "plain_text", Content: safeValue(title)}, Template: template},
		Elements: []CardElement{{Tag: "div", Text: &CardText{Tag: "lark_md", Content: markdown}}},
	}
	if len(links) > 0 {
		actions := make([]CardAction, 0, len(links))
		for _, link := range links {
			if strings.TrimSpace(link.URL) == "" || strings.TrimSpace(link.Label) == "" {
				continue
			}
			actions = append(actions, CardAction{Tag: "button", Text: CardText{Tag: "plain_text", Content: safeValue(link.Label)}, Type: "primary", MultiURL: &CardURL{URL: link.URL}})
		}
		if len(actions) > 0 {
			card.Elements = append(card.Elements, CardElement{Tag: "action", Actions: actions})
		}
	}
	message := FeishuMessage{MsgType: "interactive", Card: card}
	message.Content.Text = safeValue(title) + "\n\n" + markdown
	return message
}

func (m FeishuMessage) CardJSON() ([]byte, error) {
	if m.MsgType != "interactive" || m.Card == nil || len(m.Card.Elements) == 0 {
		return nil, fmt.Errorf("interactive Feishu card is required")
	}
	data, err := json.Marshal(m.Card)
	if err != nil {
		return nil, fmt.Errorf("encode Feishu card")
	}
	if len(data) == 0 || len(data) > maxCardBytes {
		return nil, fmt.Errorf("Feishu card exceeds size limit")
	}
	return data, nil
}

func (m FeishuMessage) Outbound() (feishuapi.OutboundMessage, error) {
	data, err := m.CardJSON()
	if err != nil {
		return feishuapi.OutboundMessage{}, err
	}
	return feishuapi.OutboundMessage{MsgType: m.MsgType, Content: data}, nil
}

func (m FeishuMessage) TextProjection() string { return m.Content.Text }

type Client struct {
	WebhookFile string
	BaseURL     string
	HTTP        *http.Client
	Resolver    pricing.Resolver
}

type MessageSender interface {
	SendMessage(context.Context, string, feishuapi.OutboundMessage) (string, error)
}

type AppClient struct {
	Sender  MessageSender
	ChatID  string
	BaseURL string
}

func (c AppClient) Send(ctx context.Context, message FeishuMessage) error {
	if c.Sender == nil || strings.TrimSpace(c.ChatID) == "" {
		return fmt.Errorf("Feishu App delivery is unavailable")
	}
	payload, err := resolveMessageLinks(message, c.BaseURL).Outbound()
	if err != nil {
		return err
	}
	if _, err := c.Sender.SendMessage(ctx, c.ChatID, payload); err != nil {
		return fmt.Errorf("Feishu App delivery failed")
	}
	return nil
}

func (c Client) Send(ctx context.Context, message FeishuMessage) error {
	webhookBytes, err := readNotifySecret(c.WebhookFile)
	if err != nil {
		return err
	}
	defer clearNotifySecret(webhookBytes)
	webhook := string(webhookBytes)
	resolver := c.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if err := pricing.ValidateRemoteURL(ctx, resolver, webhook); err != nil {
		return fmt.Errorf("Feishu webhook URL is unsafe")
	}
	card, cardErr := resolveMessageLinks(message, c.BaseURL).CardJSON()
	if cardErr != nil {
		return cardErr
	}
	payload := struct {
		MsgType string          `json:"msg_type"`
		Card    json.RawMessage `json:"card"`
	}{MsgType: "interactive", Card: card}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Feishu message")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("build Feishu request")
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Feishu delivery failed")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Feishu delivery returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func resolveMessageLinks(message FeishuMessage, baseURL string) FeishuMessage {
	if message.Card == nil {
		return message
	}
	copyMessage := message
	copyCard := *message.Card
	copyElements := make([]CardElement, 0, len(message.Card.Elements))
	for _, element := range message.Card.Elements {
		copyElement := element
		copyElement.Actions = append([]CardAction(nil), element.Actions...)
		if len(copyElement.Actions) > 0 {
			filtered := copyElement.Actions[:0]
			for _, action := range copyElement.Actions {
				if action.MultiURL == nil {
					filtered = append(filtered, action)
					continue
				}
				url := action.MultiURL.URL
				if strings.HasPrefix(url, "/") {
					if strings.TrimSpace(baseURL) == "" {
						continue
					}
					url = strings.TrimRight(baseURL, "/") + url
				}
				if !strings.HasPrefix(strings.ToLower(url), "https://") {
					continue
				}
				copyAction := action
				copyAction.MultiURL = &CardURL{URL: url}
				filtered = append(filtered, copyAction)
			}
			copyElement.Actions = filtered
		}
		if len(copyElement.Actions) > 0 || element.Tag != "action" {
			copyElements = append(copyElements, copyElement)
		}
	}
	copyCard.Elements = copyElements
	copyMessage.Card = &copyCard
	return copyMessage
}

func readNotifySecret(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("Feishu webhook secret is unavailable")
	}
	permissions := info.Mode().Perm()
	if !info.Mode().IsRegular() || (permissions != 0o600 && permissions != 0o640) || info.Size() <= 0 || info.Size() > 8<<10 {
		return nil, fmt.Errorf("Feishu webhook secret is unsafe")
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Feishu webhook secret is unavailable")
	}
	value = bytes.TrimSpace(value)
	return value, nil
}

func clearNotifySecret(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func joinItems(items []string) string {
	if len(items) == 0 {
		return "无"
	}
	safe := make([]string, 0, len(items))
	for _, item := range items {
		safe = append(safe, safeValue(item))
	}
	return strings.Join(safe, "；")
}

func defaultText(value string) string {
	if strings.TrimSpace(value) == "" {
		return "无"
	}
	return safeValue(value)
}

func safeValue(value string) string {
	value = strings.TrimSpace(value)
	for _, marker := range []string{"sk-", "Bearer ", "Cookie:", "app_secret", "api_key"} {
		if strings.Contains(strings.ToLower(value), strings.ToLower(marker)) {
			return "[已脱敏]"
		}
	}
	return value
}

func shortIdentity(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "未提供"
	}
	sum := sha256.Sum256([]byte(value))
	return "#" + hex.EncodeToString(sum[:4])
}
