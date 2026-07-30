package notify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/feishuapi"
	"example.invalid/relay-ops-service/internal/pricing"
)

const maxCardBytes = 30 << 10

type Link struct{ Label, URL string }

type IncidentView struct {
	Title         string
	Severity      string
	WhatWasDone   []string
	Results       []string
	Change        string
	Focus         string
	Links         []Link
	CurrentLabel  string
	BaselineLabel string
	Current       string
	Baseline      string
	Duration      string
	Analysis      string
	Recovery      bool
	Suppressed    bool
}

type RecoveryMetric struct {
	Label string
	Value string
}

type RecoveryCardView struct {
	Title      string
	Summary    string
	Detail     string
	Metrics    []RecoveryMetric
	Basis      []string
	Source     string
	Focus      string
	Links      []Link
	Suppressed bool
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

type CardText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type CardField struct {
	IsShort bool     `json:"is_short"`
	Text    CardText `json:"text"`
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
	Fields  []CardField  `json:"fields,omitempty"`
	Actions []CardAction `json:"actions,omitempty"`
	Margin  string       `json:"margin,omitempty"`
}

type Card struct {
	Config   CardConfig    `json:"config"`
	Header   CardHeader    `json:"header"`
	Elements []CardElement `json:"elements"`
}

// FeishuMessage is always an interactive card; both transports send Card and
// nothing else. A plain-text copy of the body used to ride along in a Content
// field, assembled separately from the card — so it could disagree with what
// was actually delivered. RenderedText derives the same view from the card
// instead, which cannot drift.
type FeishuMessage struct {
	MsgType      string `json:"msg_type"`
	Card         *Card  `json:"-"`
	Severity     string `json:"-"`
	OccurrenceNo int64  `json:"-"`
	Transition   string `json:"-"`
}

type SendResult struct {
	MessageID          string
	ResponseCode       int
	Payload            []byte
	UrgentStatus       string
	UrgentResponseCode int
}

func WithDeliveryIdentity(message FeishuMessage, occurrenceNo int64, transition string) FeishuMessage {
	if occurrenceNo <= 0 {
		occurrenceNo = 1
	}
	transition = strings.TrimSpace(transition)
	if transition == "" {
		transition = "confirmed"
	}
	message.OccurrenceNo = occurrenceNo
	message.Transition = transition
	return message
}

// RenderedText flattens the card into the text a reader sees: header title
// followed by every rendered section.
func (m FeishuMessage) RenderedText() string {
	if m.Card == nil {
		return ""
	}
	parts := []string{m.Card.Header.Title.Content}
	for _, element := range m.Card.Elements {
		if element.Text != nil {
			parts = append(parts, element.Text.Content)
		}
		for _, field := range element.Fields {
			parts = append(parts, field.Text.Content)
		}
		if element.Content != "" {
			parts = append(parts, element.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

func RenderFeishu(event IncidentView) FeishuMessage {
	if event.Recovery {
		return RenderRecovery(event)
	}
	return RenderAlert(event)
}

func RenderAlert(event IncidentView) FeishuMessage {
	severity := strings.ToUpper(strings.TrimSpace(event.Severity))
	title := event.Title
	if (severity == "P0" || severity == "P1") && !strings.HasPrefix(title, severity+"｜") {
		title = severity + "｜" + title
	}
	template := "orange"
	if severity == "P0" || severity == "P1" || strings.Contains(strings.ToLower(event.Title), "异常") {
		template = "red"
	}
	currentLabel := strings.TrimSpace(event.CurrentLabel)
	if currentLabel == "" {
		currentLabel = "影响"
	}
	baselineLabel := strings.TrimSpace(event.BaselineLabel)
	if baselineLabel == "" {
		baselineLabel = "基线"
	}
	sections := []string{
		"**状态** 需要关注",
		"**" + currentLabel + "** " + defaultText(event.Current),
		"**" + baselineLabel + "** " + defaultText(event.Baseline),
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
	message := newCardMessage(title, template, strings.Join(sections, "\n\n"), event.Links)
	message.Severity = severity
	return message
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

func RenderRecoveryCard(view RecoveryCardView) FeishuMessage {
	summary := safeValue(view.Summary)
	if summary == "" {
		summary = "已恢复"
	}
	summaryContent := "**" + summary + "**"
	if detail := safeValue(view.Detail); detail != "" {
		summaryContent += "\n" + detail
	}
	elements := []CardElement{{
		Tag:  "div",
		Text: &CardText{Tag: "lark_md", Content: summaryContent},
	}}

	fields := make([]CardField, 0, 4)
	for _, metric := range view.Metrics {
		if len(fields) == 4 {
			break
		}
		label := strings.TrimSpace(metric.Label)
		value := strings.TrimSpace(metric.Value)
		if label == "" || value == "" {
			continue
		}
		label = safeValue(label)
		value = safeValue(value)
		if label == "[已脱敏]" || value == "[已脱敏]" {
			continue
		}
		fields = append(fields, CardField{
			IsShort: true,
			Text: CardText{
				Tag:     "lark_md",
				Content: "**" + label + "**\n" + value,
			},
		})
	}
	if len(fields) == 0 {
		results := make([]string, 0, len(view.Basis)+2)
		if summary := strings.TrimSpace(view.Summary); summary != "" {
			results = append(results, summary)
		}
		results = append(results, view.Basis...)
		if source := strings.TrimSpace(view.Source); source != "" {
			results = append(results, "数据来源："+source)
		}
		return RenderRecovery(IncidentView{
			Title:      view.Title,
			Results:    results,
			Change:     view.Detail,
			Focus:      view.Focus,
			Links:      view.Links,
			Suppressed: view.Suppressed,
		})
	}
	elements = append(elements, CardElement{Tag: "div", Fields: fields})

	evidence := make([]string, 0, 4)
	if len(view.Basis) > 0 {
		evidence = append(evidence, "**判断依据**\n"+joinItems(view.Basis))
	}
	if source := strings.TrimSpace(view.Source); source != "" {
		evidence = append(evidence, "**数据来源**\n"+safeValue(source))
	}
	if focus := strings.TrimSpace(view.Focus); focus != "" {
		evidence = append(evidence, "**后续观察**\n"+safeValue(focus))
	}
	if view.Suppressed {
		evidence = append(evidence, "**去重**\n重复事件已抑制")
	}
	if len(evidence) > 0 {
		elements = append(elements, CardElement{
			Tag:  "div",
			Text: &CardText{Tag: "lark_md", Content: strings.Join(evidence, "\n\n")},
		})
	}
	elements = appendCardActions(elements, view.Links)

	return FeishuMessage{MsgType: "interactive", Card: &Card{
		Config:   CardConfig{WideScreenMode: true},
		Header:   CardHeader{Title: CardText{Tag: "plain_text", Content: safeValue(view.Title)}, Template: "green"},
		Elements: elements,
	}}
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

// Digest sections, measured after JSON escaping, must still fit the 30 KiB
// Feishu card limit. The health digest renders four sections; keeping each
// encoded section bounded at 4 KiB leaves room for the card structure, header,
// and action.
const maxDigestSectionBytes = 4 << 10

func fitDigestSection(lines []string) string {
	const remainder = "- 其余对象请在原生运维后台查看"
	clean := make([]string, len(lines))
	for index, line := range lines {
		clean[index] = trimDigestText(line, 768)
	}
	// Reserve the suffix up front. This makes the result deterministic and lets
	// later abnormal rows displace ordinary rows without changing native order.
	budget := maxDigestSectionBytes - digestLineBytes(remainder)
	criticalBytes := make([]int, len(clean))
	for index, line := range clean {
		if digestLineIsAbnormal(line) {
			criticalBytes[index] = digestLineBytes(line)
		}
	}
	selected := make([]bool, len(clean))
	remainingCritical := make([]int, len(clean)+1)
	for index := len(clean) - 1; index >= 0; index-- {
		remainingCritical[index] = remainingCritical[index+1] + criticalBytes[index]
	}
	used := 0
	for index, line := range clean {
		addition := digestLineBytes(line)
		if criticalBytes[index] > 0 || !strings.HasPrefix(line, "-") || used+addition+remainingCritical[index+1] <= budget {
			if used+addition <= budget {
				selected[index] = true
				used += addition
			}
		}
	}
	var builder strings.Builder
	skipped := false
	for index, line := range clean {
		if !selected[index] {
			skipped = true
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(line)
	}
	if skipped {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(remainder)
	}
	return builder.String()
}

func digestLineBytes(line string) int {
	// The newline becomes a two-byte JSON escape sequence. This intentionally
	// over-counts the first line so section bounds remain safe and deterministic.
	encoded, err := json.Marshal(line)
	if err != nil {
		return maxDigestSectionBytes + 1
	}
	return len(encoded)
}

func digestLineIsAbnormal(line string) bool {
	// Account quality stores result labels in the public projection. Every
	// non-passed supported result stays eligible during truncation, alongside
	// native read/sample and stale-evidence states.
	for _, marker := range []string{
		"读取失败", "样本不足", "证据已过期", "余额不足", "未通过", "异常",
		"账号测试错误", "HTTP 错误", "超时", "流格式错误", "无可用文本模型",
		"account_test_error", "http_error", "timeout", "malformed_stream", "model_unavailable",
	} {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

func trimDigestText(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	maximum -= len("...")
	if maximum <= 0 {
		return "..."
	}
	for maximum > 0 && (value[maximum]&0xc0) == 0x80 {
		maximum--
	}
	return value[:maximum] + "..."
}

func digestValue(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	for _, marker := range []string{"http://", "https://", "base url", "base_url", "api_key", "api key", "api-key", "model response", "response text", "ou-"} {
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
		Severity:    "P2",
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
	card.Elements = appendCardActions(card.Elements, links)
	return FeishuMessage{MsgType: "interactive", Card: card}
}

func appendCardActions(elements []CardElement, links []Link) []CardElement {
	if len(links) > 0 {
		actions := make([]CardAction, 0, len(links))
		for _, link := range links {
			if strings.TrimSpace(link.URL) == "" || strings.TrimSpace(link.Label) == "" {
				continue
			}
			actions = append(actions, CardAction{Tag: "button", Text: CardText{Tag: "plain_text", Content: safeValue(link.Label)}, Type: "primary", MultiURL: &CardURL{URL: link.URL}})
		}
		if len(actions) > 0 {
			elements = append(elements, CardElement{Tag: "action", Actions: actions})
		}
	}
	return elements
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

type Client struct {
	WebhookFile string
	BaseURL     string
	HTTP        *http.Client
	Resolver    pricing.Resolver
}

type MessageSender interface {
	SendMessage(context.Context, string, feishuapi.OutboundMessage) (string, error)
}

type UrgentMessageSender interface {
	UrgentMessage(context.Context, string, []string) (int, error)
}

type AppClient struct {
	Sender           MessageSender
	ChatID           string
	BaseURL          string
	RecipientOpenIDs []string
}

func (c AppClient) Send(ctx context.Context, message FeishuMessage) error {
	_, err := c.SendWithResult(ctx, message)
	return err
}

func (c AppClient) SendWithResult(ctx context.Context, message FeishuMessage) (SendResult, error) {
	if c.Sender == nil || strings.TrimSpace(c.ChatID) == "" {
		return SendResult{}, fmt.Errorf("Feishu App delivery is unavailable")
	}
	prepared, err := prepareStrongReminder(resolveMessageLinks(message, c.BaseURL), c.RecipientOpenIDs)
	if err != nil {
		return SendResult{}, err
	}
	payload, err := prepared.Outbound()
	if err != nil {
		return SendResult{}, err
	}
	messageID, err := c.Sender.SendMessage(ctx, c.ChatID, payload)
	if err != nil {
		return SendResult{}, fmt.Errorf("Feishu App delivery failed")
	}
	result := SendResult{
		MessageID: messageID, ResponseCode: http.StatusOK, Payload: append([]byte(nil), payload.Content...),
		UrgentStatus: "not_required",
	}
	if prepared.Severity != "P0" || len(c.RecipientOpenIDs) == 0 {
		return result, nil
	}
	urgentSender, ok := c.Sender.(UrgentMessageSender)
	if !ok {
		result.UrgentStatus = "failed"
		return result, nil
	}
	status, urgentErr := urgentSender.UrgentMessage(ctx, messageID, normalizedOpenIDs(c.RecipientOpenIDs))
	result.UrgentResponseCode = status
	if urgentErr != nil {
		result.UrgentStatus = "failed"
		return result, nil
	}
	result.UrgentStatus = "delivered"
	return result, nil
}

func prepareStrongReminder(message FeishuMessage, recipients []string) (FeishuMessage, error) {
	if message.Severity != "P0" && message.Severity != "P1" {
		return message, nil
	}
	recipients = normalizedOpenIDs(recipients)
	if len(recipients) == 0 {
		return message, nil
	}
	data, err := message.CardJSON()
	if err != nil {
		return FeishuMessage{}, err
	}
	var card Card
	if err := json.Unmarshal(data, &card); err != nil {
		return FeishuMessage{}, fmt.Errorf("decode notification card")
	}
	mentions := make([]string, 0, len(recipients))
	for _, openID := range recipients {
		mentions = append(mentions, "<at id="+openID+"></at>")
	}
	card.Elements = append([]CardElement{{
		Tag:  "div",
		Text: &CardText{Tag: "lark_md", Content: strings.Join(mentions, " ")},
	}}, card.Elements...)
	message.Card = &card
	return message, nil
}

func normalizedOpenIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == 20 {
			break
		}
	}
	return result
}

func LoadRecipientOpenIDs(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("Feishu alert recipient file is unavailable")
	}
	permissions := info.Mode().Perm()
	if !info.Mode().IsRegular() || (permissions != 0o600 && permissions != 0o640) ||
		info.Size() <= 0 || info.Size() > 8<<10 {
		return nil, fmt.Errorf("Feishu alert recipient file is unsafe")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Feishu alert recipient file is unavailable")
	}
	defer clearNotifySecret(data)
	var document struct {
		OpenIDs []string `json:"open_ids"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("Feishu alert recipient file is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("Feishu alert recipient file is invalid")
	}
	if len(document.OpenIDs) < 1 || len(document.OpenIDs) > 20 {
		return nil, fmt.Errorf("Feishu alert recipient count is invalid")
	}
	recipients := make([]string, 0, len(document.OpenIDs))
	seen := make(map[string]struct{}, len(document.OpenIDs))
	for _, value := range document.OpenIDs {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("Feishu alert recipient value is invalid")
		}
		if _, exists := seen[value]; exists {
			return nil, fmt.Errorf("Feishu alert recipient values must be unique")
		}
		seen[value] = struct{}{}
		recipients = append(recipients, value)
	}
	return recipients, nil
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
	for _, marker := range []string{"sk-", "Bearer ", "Cookie:", "app_secret", "api_key", "api-key"} {
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
