package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/feishuapi"
)

func assertReminderOnlyCard(t *testing.T, message FeishuMessage) {
	t.Helper()
	text := message.RenderedText()
	for _, forbidden := range []string{
		"确认并接手", "尚未有人确认接手", "接手状态", "已接手", "处理人",
		"ack_incident", "ack_occurrence",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("card contains retired interaction %q: %s", forbidden, text)
		}
	}
	payload, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	if strings.Contains(body, "ack_incident") || strings.Contains(body, "ack_occurrence") {
		t.Fatalf("card payload contains acknowledgement query: %s", body)
	}
	if !strings.Contains(body, `"url":"/admin/ops"`) {
		t.Fatalf("card payload missing native ops link: %s", body)
	}
	var actionCount int
	for _, element := range message.Card.Elements {
		for _, action := range element.Actions {
			actionCount++
			if action.MultiURL == nil || action.MultiURL.URL != "/admin/ops" {
				t.Fatalf("card contains non-native action: %#v", action)
			}
		}
	}
	if actionCount != 1 {
		t.Fatalf("card action count = %d, want 1: %s", actionCount, body)
	}
}

func TestRenderFeishuIncludesFiveOperatorSections(t *testing.T) {
	t.Parallel()
	message := RenderFeishu(IncidentView{
		Title: "GPT-Pro 倍率变化", WhatWasDone: []string{"读取价格页 1 次"}, Results: []string{"当前倍率 0.10x"},
		Change: "相对 0.07x 上升 42.9%", Focus: "核对利润但不自动改价", Links: []Link{{Label: "事件详情", URL: "https://ops.example/incidents/1"}},
	})
	text := message.RenderedText()
	for _, section := range []string{"已执行", "观测结果", "发生变化", "需要关注"} {
		if !strings.Contains(text, section) {
			t.Fatalf("missing section %q in %s", section, text)
		}
	}
}

func TestRenderSessionExpiredUsesExactLoginLink(t *testing.T) {
	t.Parallel()
	message := RenderSessionExpired("wawazz", "https://wawazz.example/login")
	if !strings.Contains(message.Card.Header.Title.Content, "上游用量读取会话失效") || !strings.Contains(message.RenderedText(), "质量和公开价格监控正常") {
		t.Fatalf("message=%s", message.RenderedText())
	}
	card, err := message.CardJSON()
	if err != nil || !strings.Contains(string(card), "https://wawazz.example/login") {
		t.Fatalf("card=%s err=%v", card, err)
	}
	if message.Severity != "P2" {
		t.Fatalf("severity=%q, want P2 operational event", message.Severity)
	}
}

func TestFeishuClientReadsWebhookSecretWithoutLeakingFailures(t *testing.T) {
	t.Parallel()
	secret := filepath.Join(t.TempDir(), "webhook")
	if err := os.WriteFile(secret, []byte("https://open.feishu.example/hook/secret-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := Client{WebhookFile: secret, Resolver: notifyResolver{}, HTTP: &http.Client{Transport: notifyTransport(func(request *http.Request) *http.Response {
		if strings.Contains(request.URL.String(), "secret-value") {
			return notifyResponse(http.StatusInternalServerError, "secret body")
		}
		return notifyResponse(http.StatusNoContent, "")
	})}}
	err := client.Send(context.Background(), RenderFeishu(IncidentView{Title: "test"}))
	if err == nil || strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "secret body") {
		t.Fatalf("error=%v", err)
	}
}

func TestWebhookSendsInteractiveCardObject(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "webhook")
	if err := os.WriteFile(secret, []byte("https://open.feishu.example/hook/card"), 0o600); err != nil {
		t.Fatal(err)
	}
	var body []byte
	client := Client{WebhookFile: secret, Resolver: notifyResolver{}, HTTP: &http.Client{Transport: notifyTransport(func(request *http.Request) *http.Response {
		body, _ = io.ReadAll(request.Body)
		return notifyResponse(http.StatusNoContent, "")
	})}}
	message := RenderAlert(IncidentView{Title: "倍率异常", Severity: "P1"})
	if err := client.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		MsgType string          `json:"msg_type"`
		Card    json.RawMessage `json:"card"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.MsgType != "interactive" || !json.Valid(payload.Card) {
		t.Fatalf("payload=%s err=%v", body, err)
	}
	want, _ := message.CardJSON()
	if string(payload.Card) != string(want) {
		t.Fatalf("webhook card differs from message card")
	}
}

func TestAppClientSendsRenderedTextToConfiguredChat(t *testing.T) {
	sender := &fakeTextSender{}
	client := AppClient{Sender: sender, ChatID: "oc_alert_group"}
	message := RenderFeishu(IncidentView{Title: "合成告警", Results: []string{"已确认"}})
	if err := client.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if sender.chatID != "oc_alert_group" || sender.payload.MsgType != "interactive" {
		t.Fatalf("sender=%#v message=%#v", sender, message)
	}
}

func TestAppClientMentionsRecipientsAndUrgentsOnlyP0(t *testing.T) {
	tests := []struct {
		name         string
		message      FeishuMessage
		wantMentions bool
		wantUrgent   bool
	}{
		{name: "P0", message: RenderAlert(IncidentView{Title: "公开分组不可用", Severity: "P0"}), wantMentions: true, wantUrgent: true},
		{name: "P1", message: RenderAlert(IncidentView{Title: "账号不可调度", Severity: "P1"}), wantMentions: true},
		{name: "P2", message: RenderAlert(IncidentView{Title: "倍率变化", Severity: "P2"})},
		{name: "recovery", message: RenderRecovery(IncidentView{Title: "已恢复", Severity: "P0"})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender := &fakeTextSender{}
			client := AppClient{
				Sender: sender, ChatID: "oc_alert_group",
				RecipientOpenIDs: []string{"ou-a", "ou-b", "ou-a"},
			}
			result, err := client.SendWithResult(context.Background(), test.message)
			if err != nil {
				t.Fatal(err)
			}
			var card Card
			if err := json.Unmarshal(sender.payload.Content, &card); err != nil {
				t.Fatal(err)
			}
			firstContent := ""
			if len(card.Elements) > 0 && card.Elements[0].Text != nil {
				firstContent = card.Elements[0].Text.Content
			}
			hasMentions := strings.Contains(firstContent, "<at id=ou-a></at>") && strings.Contains(firstContent, "<at id=ou-b></at>")
			if hasMentions != test.wantMentions {
				t.Fatalf("mentions = %v, want %v: %s", hasMentions, test.wantMentions, sender.payload.Content)
			}
			if (sender.urgentCalls == 1) != test.wantUrgent {
				t.Fatalf("urgent calls = %d, want urgent %v", sender.urgentCalls, test.wantUrgent)
			}
			if result.MessageID != "om_alert" || result.ResponseCode != http.StatusOK || !json.Valid(result.Payload) {
				t.Fatalf("send result = %#v", result)
			}
		})
	}
}

func TestAppClientRecordsUrgencyFailureAfterMessageDelivery(t *testing.T) {
	sender := &fakeTextSender{urgentErr: fmt.Errorf("urgent rejected")}
	client := AppClient{
		Sender: sender, ChatID: "oc_alert_group",
		RecipientOpenIDs: []string{"ou-a"},
	}
	result, err := client.SendWithResult(context.Background(), RenderAlert(IncidentView{Title: "不可用", Severity: "P0"}))
	if err != nil {
		t.Fatalf("message delivery should remain successful: %v", err)
	}
	if result.MessageID != "om_alert" || result.UrgentStatus != "failed" || result.UrgentResponseCode != 0 {
		t.Fatalf("send result = %#v", result)
	}
}

func TestLoadRecipientOpenIDsStrictlyValidatesSecretJSON(t *testing.T) {
	t.Parallel()
	validValues := make([]string, 20)
	for index := range validValues {
		validValues[index] = fmt.Sprintf("operator-%02d", index)
	}
	validPayload, err := json.Marshal(map[string]any{"open_ids": validValues})
	if err != nil {
		t.Fatal(err)
	}
	validFile := filepath.Join(t.TempDir(), "recipients.json")
	if err := os.WriteFile(validFile, validPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadRecipientOpenIDs(validFile)
	if err != nil || len(got) != 20 {
		t.Fatalf("recipients count=%d err=%v", len(got), err)
	}

	tests := []struct {
		name    string
		payload []byte
		mode    os.FileMode
	}{
		{name: "empty list", payload: []byte(`{"open_ids":[]}`), mode: 0o600},
		{name: "too many", payload: func() []byte {
			values := append(append([]string(nil), validValues...), "operator-20")
			data, _ := json.Marshal(map[string]any{"open_ids": values})
			return data
		}(), mode: 0o600},
		{name: "duplicate after trim", payload: []byte(`{"open_ids":["operator-a"," operator-a "]}`), mode: 0o600},
		{name: "empty value", payload: []byte(`{"open_ids":["operator-a"," "]}`), mode: 0o600},
		{name: "unknown field", payload: []byte(`{"open_ids":["operator-a"],"chat_id":"not-allowed"}`), mode: 0o600},
		{name: "invalid JSON", payload: []byte(`{"open_ids":`), mode: 0o600},
		{name: "trailing JSON", payload: []byte(`{"open_ids":["operator-a"]}{}`), mode: 0o600},
		{name: "oversized", payload: []byte(`{"open_ids":["` + strings.Repeat("x", 9<<10) + `"]}`), mode: 0o600},
		{name: "unsafe permissions", payload: []byte(`{"open_ids":["operator-a"]}`), mode: 0o644},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "recipients.json")
			if err := os.WriteFile(path, test.payload, test.mode); err != nil {
				t.Fatal(err)
			}
			if values, err := LoadRecipientOpenIDs(path); err == nil {
				t.Fatalf("accepted invalid recipients count=%d", len(values))
			}
		})
	}
	if _, err := LoadRecipientOpenIDs(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing recipient file accepted")
	}
}

func TestDeliverySenderDoesNotInjectAcknowledgementAction(t *testing.T) {
	message := WithDeliveryIdentity(RenderUserImpact(UserImpactView{
		GroupName: "GPT-PLUS-内测", Severity: "P1", Headline: "部分请求持续失败",
	}), 3, "confirmed")
	repository := &recordingDeliveryRepository{}
	client := &recordingMessageClient{}
	sender := DeliverySender{Client: client, Repository: repository}
	if err := sender.SendIncident(context.Background(), "group:1", "evidence", message); err != nil {
		t.Fatal(err)
	}
	assertReminderOnlyCard(t, client.message)
}

type recordingDeliveryRepository = fakeDeliveryRepository

type recordingMessageClient struct {
	message FeishuMessage
}

func (client *recordingMessageClient) Send(_ context.Context, message FeishuMessage) error {
	client.message = message
	return nil
}

func TestAppClientResolvesRelativeOpsLinkBeforeSending(t *testing.T) {
	sender := &fakeTextSender{}
	client := AppClient{Sender: sender, ChatID: "oc_alert_group", BaseURL: "https://ops.example"}
	message := RenderAlert(IncidentView{Title: "告警", Links: []Link{{Label: "运维后台", URL: "/admin/ops"}}})
	if err := client.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sender.payload.Content), "https://ops.example/admin/ops") {
		t.Fatalf("relative link was not resolved: %s", sender.payload.Content)
	}
}

func TestAppClientFiltersUnsafeStructuredRecoveryLinkAndResolvesRelativeLink(t *testing.T) {
	sender := &fakeTextSender{}
	client := AppClient{Sender: sender, ChatID: "oc_alert_group", BaseURL: "https://ops.example"}
	message := RenderRecoveryCard(RecoveryCardView{
		Title:   "恢复",
		Summary: "状态已恢复",
		Metrics: []RecoveryMetric{
			{Label: "当前状态", Value: "正常"},
			{Label: "健康确认", Value: "1 个完整窗口"},
		},
		Links: []Link{
			{Label: "不安全链接", URL: "http://unsafe.example/ops"},
			{Label: "运维后台", URL: "/admin/ops"},
		},
	})
	if err := client.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}

	var card Card
	if err := json.Unmarshal(sender.payload.Content, &card); err != nil {
		t.Fatal(err)
	}
	if len(card.Elements) != 3 || card.Elements[2].Tag != "action" || len(card.Elements[2].Actions) != 1 {
		t.Fatalf("unsafe action was retained: %s", sender.payload.Content)
	}
	action := card.Elements[2].Actions[0]
	if action.MultiURL == nil || action.MultiURL.URL != "https://ops.example/admin/ops" {
		t.Fatalf("relative action was not resolved: %#v", action)
	}
}

func TestRenderAlertRecoveryAndCommandUseInteractiveCards(t *testing.T) {
	t.Parallel()
	alert := RenderAlert(IncidentView{Title: "上游异常", Severity: "P1", Current: "error", Baseline: "operational", Results: []string{"失败率升高"}})
	recovery := RenderRecovery(IncidentView{Title: "上游恢复", Results: []string{"状态恢复"}, Duration: "4m"})
	command := RenderCommand(CommandView{Command: "查询当前分组状态", Status: "succeeded", GroupName: "GPT-Pro", AuditID: "abc123"})
	for name, message := range map[string]FeishuMessage{"alert": alert, "recovery": recovery, "command": command} {
		if message.MsgType != "interactive" || message.Card == nil {
			t.Fatalf("%s message = %#v", name, message)
		}
		data, err := json.Marshal(message.Card)
		if err != nil || len(data) >= 30<<10 {
			t.Fatalf("%s card bytes=%d err=%v", name, len(data), err)
		}
	}
	if alert.Card.Header.Template == recovery.Card.Header.Template || alert.Card.Header.Template == command.Card.Header.Template {
		t.Fatalf("card templates are not visually differentiated")
	}
	if !strings.Contains(alert.RenderedText(), "需要关注") || !strings.Contains(recovery.RenderedText(), "恢复") || !strings.Contains(command.RenderedText(), "审计") {
		t.Fatalf("cards missing semantic sections")
	}
}

func TestRenderAlertKeepsSingleProseDivWithoutStructuredFields(t *testing.T) {
	t.Parallel()
	message := RenderAlert(IncidentView{
		Title:    "上游异常",
		Severity: "P1",
		Current:  "error",
		Results:  []string{"失败率升高"},
	})
	if len(message.Card.Elements) != 1 {
		t.Fatalf("alert elements=%#v", message.Card.Elements)
	}
	element := message.Card.Elements[0]
	if element.Tag != "div" || element.Text == nil || len(element.Fields) != 0 {
		t.Fatalf("alert adopted structured recovery fields: %#v", element)
	}
}

func TestRenderRecoveryCardSeparatesSummaryMetricsEvidenceAndAction(t *testing.T) {
	t.Parallel()
	message := RenderRecoveryCard(RecoveryCardView{
		Title:   "站内运行已恢复：特惠-SHUAI",
		Summary: "已恢复正常调度",
		Detail:  "调度状态已回到健康基线",
		Metrics: []RecoveryMetric{
			{Label: "当前状态", Value: "正常调度"},
			{Label: "健康确认", Value: "1 个完整窗口"},
			{Label: "观测窗口", Value: "15 分钟 / 24 小时"},
			{Label: "证据时间", Value: "05:15 UTC"},
		},
		Basis:  []string{"当前与基线均为可用、可调度"},
		Source: "Sub2API 原生站内运行快照",
		Focus:  "在运维后台查看站内运行证据",
		Links:  []Link{{Label: "运维后台", URL: "/admin/ops"}},
	})

	if got := len(message.Card.Elements); got != 4 {
		t.Fatalf("elements=%d, want summary, metrics, evidence, action", got)
	}
	fields := message.Card.Elements[1].Fields
	if len(fields) != 4 {
		t.Fatalf("fields=%#v", fields)
	}
	for _, field := range fields {
		if !field.IsShort || field.Text.Tag != "lark_md" || strings.TrimSpace(field.Text.Content) == "" {
			t.Fatalf("invalid field=%#v", field)
		}
	}
	for _, want := range []string{"已恢复正常调度", "当前状态", "正常调度", "判断依据", "数据来源", "后续观察"} {
		if !strings.Contains(message.RenderedText(), want) {
			t.Fatalf("missing %q in %q", want, message.RenderedText())
		}
	}
}

func TestRenderRecoveryCardOmitsBlankMetricsAndRedactsValues(t *testing.T) {
	t.Parallel()
	message := RenderRecoveryCard(RecoveryCardView{
		Title: "恢复", Summary: "已恢复",
		Metrics: []RecoveryMetric{
			{Label: "当前状态", Value: "正常"},
			{Label: "", Value: ""},
			{Label: "证据", Value: "x-api-key: secret"},
		},
	})
	fields := message.Card.Elements[1].Fields
	if len(fields) != 1 || strings.Contains(message.RenderedText(), "secret") || strings.Contains(message.RenderedText(), "[已脱敏]") {
		t.Fatalf("fields=%#v text=%q", fields, message.RenderedText())
	}
}

func TestRenderRecoveryCardFallsBackWhenNoUsableMetricsRemain(t *testing.T) {
	t.Parallel()
	message := RenderRecoveryCard(RecoveryCardView{
		Title: "恢复", Summary: "证据已恢复", Detail: "继续观察",
		Metrics: []RecoveryMetric{
			{Label: "", Value: ""},
			{Label: "证据", Value: "x-api-key: secret"},
		},
		Basis:  []string{"读取现有证据"},
		Source: "只读来源",
		Focus:  "关注后续状态",
	})
	for _, element := range message.Card.Elements {
		if len(element.Fields) > 0 {
			t.Fatalf("fallback retained fields: %#v", message.Card.Elements)
		}
	}
	for _, want := range []string{"**恢复结果**", "证据已恢复", "读取现有证据", "数据来源：只读来源"} {
		if !strings.Contains(message.RenderedText(), want) {
			t.Fatalf("fallback missing %q in %q", want, message.RenderedText())
		}
	}
	if strings.Contains(message.RenderedText(), "[已脱敏]") {
		t.Fatalf("fallback retained unusable metric: %q", message.RenderedText())
	}
}

func TestRenderRecoveryCardSupportsTwoThreeAndFourSerializedFields(t *testing.T) {
	t.Parallel()
	for _, count := range []int{2, 3, 4} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			metrics := make([]RecoveryMetric, count)
			for index := range metrics {
				metrics[index] = RecoveryMetric{Label: fmt.Sprintf("指标 %d", index+1), Value: fmt.Sprintf("值 %d", index+1)}
			}
			data, err := RenderRecoveryCard(RecoveryCardView{Title: "恢复", Summary: "已恢复", Metrics: metrics}).CardJSON()
			if err != nil {
				t.Fatal(err)
			}
			var card Card
			if err := json.Unmarshal(data, &card); err != nil {
				t.Fatal(err)
			}
			if len(card.Elements) < 2 || len(card.Elements[1].Fields) != count {
				t.Fatalf("count=%d card=%s", count, data)
			}
		})
	}
}

func TestCardUsesOfficialStableElementShape(t *testing.T) {
	message := RenderAlert(IncidentView{Title: "上游异常", Results: []string{"失败率升高"}})
	if len(message.Card.Elements) == 0 || message.Card.Elements[0].Tag != "div" || message.Card.Elements[0].Text == nil || message.Card.Elements[0].Text.Tag != "lark_md" {
		t.Fatalf("card elements=%#v", message.Card.Elements)
	}
}

func TestCardRedactsLongIdentityAndNeverContainsSecret(t *testing.T) {
	message := RenderCommand(CommandView{Command: "切换 GPT-Pro 到灾备", Status: "succeeded", ActorID: "ou-user-secret", AuditID: "audit-short"})
	data, err := json.Marshal(message.Card)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "ou-user-secret") {
		t.Fatalf("card leaked sensitive value: %s", text)
	}
}

// 连字符形式的 api-key 曾能穿透脱敏：标记表只有 api_key / api key。
// 本项目调用 Sub2API admin API 的 header 就叫 x-api-key，上游错误信息里
// 很容易原样带上它，必须锁定两条脱敏路径都能拦住。
func TestRedactionCatchesHyphenatedAPIKeyMarker(t *testing.T) {
	for _, input := range []string{
		"missing x-api-key header from upstream",
		"X-API-Key: rejected",
		"api-key invalid",
	} {
		if got := digestValue(input); got != "[已脱敏]" {
			t.Fatalf("digestValue(%q) = %q, want [已脱敏]", input, got)
		}
		if got := safeValue(input); got != "[已脱敏]" {
			t.Fatalf("safeValue(%q) = %q, want [已脱敏]", input, got)
		}
	}
}

// 以下三个测试从已删除的 TestRenderOperationsDigest* 迁移而来：被守护的
// 不变量属于共享辅助函数（fitDigestSection / digestValue / shortHash），
// RenderHealthDigest 与 RenderGroupAlert 仍依赖它们，但新渲染的测试没有
// 直接覆盖这些行为。

// 迁移自 TestRenderOperationsDigestDeterministicallyTruncatesAndRetainsAbnormalities
// 与 TestRenderOperationsDigestBoundsEscapedPayloadBelowCardLimit：截断必须
// 确定、必须保留异常行、且字节预算按 JSON 转义后的尺寸计量。
func TestFitDigestSectionIsDeterministicRetainsAbnormalitiesAndBoundsEscapedBytes(t *testing.T) {
	t.Parallel()
	lines := []string{"**明细**"}
	for index := 0; index < 120; index++ {
		lines = append(lines, "- "+strings.Repeat(`普通行<>&"`, 40))
	}
	lines = append(lines, "- 账号 130：读取失败")
	first := fitDigestSection(lines)
	second := fitDigestSection(lines)
	if first != second {
		t.Fatal("fitDigestSection is not deterministic")
	}
	if !strings.Contains(first, "读取失败") {
		t.Fatalf("truncation dropped the trailing abnormal line: %s", first)
	}
	if !strings.Contains(first, "其余对象请在原生运维后台查看") {
		t.Fatal("truncated section missing the native ops remainder notice")
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > maxDigestSectionBytes {
		t.Fatalf("escaped section = %d bytes, exceeds %d", len(encoded), maxDigestSectionBytes)
	}
}

// 迁移自 TestRenderOperationsDigestSeparatesStableDomainsAndRedactsSensitiveValues：
// 链接、base_url、模型响应文本与飞书身份前缀必须被 digestValue 脱敏。
func TestDigestValueRedactsLinksModelResponsesAndIdentities(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"Base URL https://upstream.example/v1",
		"http://upstream.example/v1",
		"base_url=upstream",
		"model response text",
		"ou-full-user-identity",
	} {
		if got := digestValue(input); got != "[已脱敏]" {
			t.Fatalf("digestValue(%q) = %q, want [已脱敏]", input, got)
		}
	}
}

// 迁移自 TestRenderOperationsDigestShowsGenerationAndQualityEvidenceTimes：
// 超过 16 字节的证据标识必须缩短展示，避免完整快照 ID 进入卡片。
func TestShortHashShortensLongIdentifiers(t *testing.T) {
	t.Parallel()
	if got := shortHash("quality-20260723-0800"); got != "quality-2026...0800" {
		t.Fatalf("shortHash = %q", got)
	}
	if got := shortHash("report-short"); got != "report-short" {
		t.Fatalf("short value must pass through unchanged: %q", got)
	}
}

func TestCardJSONRejectsPayloadOverThirtyKilobytes(t *testing.T) {
	message := RenderAlert(IncidentView{Title: "large", Results: []string{strings.Repeat("x", 31<<10)}})
	if _, err := message.CardJSON(); err == nil {
		t.Fatal("oversized card was accepted")
	}
}

func TestStructuredRecoveryCardRejectsPayloadOverThirtyKilobytes(t *testing.T) {
	message := RenderRecoveryCard(RecoveryCardView{
		Title:   "large recovery",
		Summary: strings.Repeat("x", 31<<10),
		Metrics: []RecoveryMetric{
			{Label: "当前状态", Value: "正常"},
			{Label: "健康确认", Value: "1 个完整窗口"},
		},
	})
	if _, err := message.CardJSON(); err == nil {
		t.Fatal("oversized structured recovery card was accepted")
	}
}

func TestCommandCardStylesOutcomesAndHidesUnknownInput(t *testing.T) {
	tests := []struct {
		name, status, code, template string
		unknown                      bool
	}{
		{"success", "succeeded", "", "green", false},
		{"failure", "failed", "routing_failed", "red", false},
		{"unknown", "rejected", "unknown_command", "blue", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := RenderCommand(CommandView{Command: "untrusted dynamic input", Status: tt.status, ErrorCode: tt.code, Unknown: tt.unknown, AuditID: "abc123"})
			if message.Card.Header.Template != tt.template {
				t.Fatalf("template=%q", message.Card.Header.Template)
			}
			if tt.unknown && strings.Contains(message.RenderedText(), "untrusted dynamic input") {
				t.Fatal("unknown command echoed untrusted input")
			}
		})
	}
}

func TestRenderUpstreamReportIsNotificationOnly(t *testing.T) {
	t.Parallel()
	message := RenderUpstreamReport(UpstreamReportView{
		Title: "上游质量评测：Local Sub 73", Status: "review_recommended",
		QualityScore: 88, TotalScore: 94, Direct: "成功 6/6，TTFT P95 820ms",
		Gateway: "成功 6/6，TTFT P95 940ms", Models: "可开放 15，阻塞 0",
		Pricing: "multiplier_only，待管理员确认余额", Capacity: "并发至少 5，RPM 至少 20",
		Unknowns: []string{"转售条款"}, ReportID: "report-20260722-73", ReportHash: strings.Repeat("a", 64),
		Links: []Link{{Label: "运维后台", URL: "/admin/ops"}},
	})

	data, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"质量分 88", "总分 94", "成功 6/6", "可开放 15", "report-20260722-73", "运维后台"} {
		if !strings.Contains(text, required) {
			t.Fatalf("missing %q in %s", required, text)
		}
	}
	for _, forbidden := range []string{"切换上游", "确认切换", "重试探测"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("notification card contains action %q: %s", forbidden, text)
		}
	}
	if len(message.Card.Elements) != 2 || message.Card.Elements[1].Tag != "action" || len(message.Card.Elements[1].Actions) != 1 || message.Card.Elements[1].Actions[0].Text.Content != "运维后台" {
		t.Fatalf("actions=%#v", message.Card.Elements)
	}
}

type fakeTextSender struct {
	chatID      string
	payload     feishuapi.OutboundMessage
	urgentCalls int
	urgentErr   error
}

func (f *fakeTextSender) SendMessage(_ context.Context, chatID string, payload feishuapi.OutboundMessage) (string, error) {
	f.chatID = chatID
	f.payload = payload
	return "om_alert", nil
}

func (f *fakeTextSender) UrgentMessage(_ context.Context, messageID string, openIDs []string) (int, error) {
	f.urgentCalls++
	if messageID != "om_alert" || fmt.Sprint(openIDs) != "[ou-a ou-b]" && fmt.Sprint(openIDs) != "[ou-a]" {
		return 0, fmt.Errorf("unexpected urgent input")
	}
	if f.urgentErr != nil {
		return 0, f.urgentErr
	}
	return http.StatusOK, nil
}

type notifyResolver struct{}

func (notifyResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("203.0.113.40")}}, nil
}

type notifyTransport func(*http.Request) *http.Response

func (fn notifyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request), nil
}
func notifyResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
