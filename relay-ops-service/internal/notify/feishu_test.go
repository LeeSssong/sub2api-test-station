package notify

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/accountquality"
	"example.invalid/relay-ops-service/internal/feishuapi"
	"example.invalid/relay-ops-service/internal/opsmetrics"
)

func TestRenderOperationsDigestSeparatesStableDomainsAndRedactsSensitiveValues(t *testing.T) {
	t.Parallel()
	message := RenderOperationsDigest(OperationsDigestView{
		Date: "2026-07-23",
		Runtime: opsmetrics.Snapshot{
			CapturedAt: time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC),
			Groups: []opsmetrics.GroupRuntime{{
				ID: 2, Name: "公开分组 A", RequestCount: 42, SLA: 99.5, TTFTP95MS: 220, Status: opsmetrics.StatusOK,
			}},
			Accounts: []opsmetrics.AccountRuntime{{
				ID: 10, Name: "当前账号 A", GroupIDs: []int64{2}, RequestCount: 42, SLA: 99.5, TTFTP95MS: 240, Status: opsmetrics.StatusOK,
			}},
		},
		AccountQuality: accountquality.View{Available: true, Accounts: []accountquality.AccountView{{
			AccountID: "10", Stability: "成功 6/6", Multiplier: "0.1x", TTFTP95: "260ms", LastResult: "通过",
		}}},
		Footer: []string{"候选站 1", "Base URL https://upstream.example/v1", "api_key=secret", "model response text", "ou-full-user-identity"},
	})

	data, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	previous := 0
	for _, required := range []string{"站内运行", "公开分组", "当前调度账号", "上游账号质量", "倍率", "TTFT P95", "运维后台"} {
		index := strings.Index(text[previous:], required)
		if index < 0 {
			t.Fatalf("missing or out-of-order %q in %s", required, text)
		}
		previous += index + len(required)
	}
	for _, forbidden := range []string{"https://upstream.example", "api_key=secret", "model response text", "ou-full-user-identity"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("digest leaked %q in %s", forbidden, text)
		}
	}
}

func TestRenderFeishuIncludesFiveOperatorSections(t *testing.T) {
	t.Parallel()
	message := RenderFeishu(IncidentView{
		Title: "GPT-Pro 倍率变化", WhatWasDone: []string{"读取价格页 1 次"}, Results: []string{"当前倍率 0.10x"},
		Change: "相对 0.07x 上升 42.9%", Focus: "核对利润但不自动改价", Links: []Link{{Label: "事件详情", URL: "https://ops.example/incidents/1"}},
	})
	text := message.Content.Text
	for _, section := range []string{"已执行", "观测结果", "发生变化", "需要关注"} {
		if !strings.Contains(text, section) {
			t.Fatalf("missing section %q in %s", section, text)
		}
	}
}

func TestRenderSessionExpiredUsesExactLoginLink(t *testing.T) {
	t.Parallel()
	message := RenderSessionExpired("wawazz", "https://wawazz.example/login")
	if !strings.Contains(message.Card.Header.Title.Content, "上游用量读取会话失效") || !strings.Contains(message.Content.Text, "质量和公开价格监控正常") {
		t.Fatalf("message=%s", message.Content.Text)
	}
	card, err := message.CardJSON()
	if err != nil || !strings.Contains(string(card), "https://wawazz.example/login") {
		t.Fatalf("card=%s err=%v", card, err)
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

func TestAppClientResolvesRelativeOpsLinkBeforeSending(t *testing.T) {
	sender := &fakeTextSender{}
	client := AppClient{Sender: sender, ChatID: "oc_alert_group", BaseURL: "https://ops.example"}
	message := RenderAlert(IncidentView{Title: "告警", Links: []Link{{Label: "运维后台", URL: "/ops"}}})
	if err := client.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sender.payload.Content), "https://ops.example/ops") {
		t.Fatalf("relative link was not resolved: %s", sender.payload.Content)
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
	if !strings.Contains(alert.TextProjection(), "需要关注") || !strings.Contains(recovery.TextProjection(), "恢复") || !strings.Contains(command.TextProjection(), "审计") {
		t.Fatalf("text projections missing semantic sections")
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

func TestCardJSONRejectsPayloadOverThirtyKilobytes(t *testing.T) {
	message := RenderAlert(IncidentView{Title: "large", Results: []string{strings.Repeat("x", 31<<10)}})
	if _, err := message.CardJSON(); err == nil {
		t.Fatal("oversized card was accepted")
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
			if tt.unknown && strings.Contains(message.TextProjection(), "untrusted dynamic input") {
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
		Links: []Link{{Label: "运维后台", URL: "/ops"}},
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
	chatID  string
	payload feishuapi.OutboundMessage
}

func (f *fakeTextSender) SendMessage(_ context.Context, chatID string, payload feishuapi.OutboundMessage) (string, error) {
	f.chatID = chatID
	f.payload = payload
	return "om_alert", nil
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
