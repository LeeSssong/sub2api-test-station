package notify

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

func TestRenderOperationsDigestIncludesNativeUpstreamAccountStatus(t *testing.T) {
	t.Parallel()

	message := RenderOperationsDigest(OperationsDigestView{
		UpstreamAccountStatus: UpstreamAccountStatusView{
			Available:     true,
			ObservedAt:    "2026-07-25T07:00:00Z",
			EvidenceState: "fresh",
			Groups: []AccountGroupStatusView{{
				GroupID:            3,
				GroupName:          "GPT-Pro",
				CurrentAccountID:   11,
				CandidateAccountID: 12,
				Decision:           "candidate_better",
				ScoreDelta:         0.128,
				Reasons:            []string{"稳定性更高：96.0% vs 70.0%"},
				Current: AccountStatusView{
					AccountID: 11, Name: "账号 A", SuccessRate: "70.0%", TTFT: "450ms",
					Latency: "1600ms", Multiplier: "0.12x", UsageWindows: "daily 20.0%", Status: "正常",
				},
				Candidate: AccountStatusView{
					AccountID: 12, Name: "账号 B", SuccessRate: "96.0%", TTFT: "120ms",
					Latency: "500ms", Multiplier: "0.08x", UsageWindows: "daily 5.0%", Status: "正常",
				},
			}},
		},
	})
	data, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"上游账号情况", "B 账号综合更佳", "成功率", "TTFT", "倍率", "用量窗口", "账号监控", "只读", "不执行切换"} {
		if !strings.Contains(text, required) {
			t.Fatalf("native account section missing %q: %s", required, text)
		}
	}
	for _, forbidden := range []string{"切换账号", "确认切换", "schedulable", "api_key"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("native account section contains mutation or secret text %q: %s", forbidden, text)
		}
	}
}

func TestRenderOperationsDigestShowsGenerationAndQualityEvidenceTimes(t *testing.T) {
	t.Parallel()
	message := RenderOperationsDigest(OperationsDigestView{
		Date:        "2026-07-23",
		GeneratedAt: time.Date(2026, 7, 23, 8, 5, 0, 0, time.UTC),
		Runtime: opsmetrics.Snapshot{
			CapturedAt: time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC),
		},
		AccountQuality: accountquality.View{
			Available:  true,
			SnapshotID: "quality-20260723-0800",
			ObservedAt: "2026-07-23T08:00:00Z",
		},
	})

	data, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"报告生成时间 2026-07-23 08:05 UTC",
		"站内采集时间 2026-07-23 08:00 UTC",
		"质量证据快照 quality-2026...0800",
		"质量证据时间 2026-07-23T08:00:00Z",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("digest missing %q: %s", required, text)
		}
	}
	if strings.Contains(text, "quality-20260723-0800") {
		t.Fatalf("digest exposed full evidence snapshot ID: %s", text)
	}
}

func TestRenderOperationsDigestDoesNotFabricateZeroRuntimeMetrics(t *testing.T) {
	t.Parallel()

	message := RenderOperationsDigest(OperationsDigestView{Runtime: opsmetrics.Snapshot{
		Groups: []opsmetrics.GroupRuntime{{
			ID: 2, Name: "空分组", RequestCount: 0, SuccessCount: 0, Status: opsmetrics.StatusSampleInsufficient,
		}, {
			ID: 3, Name: "全失败分组", RequestCount: 3, SuccessCount: 0, ErrorRate: 1, SLA: 50,
			DurationP95MS: 500, Status: opsmetrics.StatusSampleInsufficient,
		}},
	}})
	data, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"错误率 未知", "SLA 未知", "TTFT P95 无成功样本"} {
		if !strings.Contains(text, required) {
			t.Fatalf("digest missing %q: %s", required, text)
		}
	}
	if strings.Contains(text, "错误率 0.00%") || strings.Contains(text, "SLA 0.00%") || strings.Contains(text, "TTFT P95 0ms") {
		t.Fatalf("digest fabricated zero metrics: %s", text)
	}
}

func TestRenderOperationsDigestPreservesNativePublicGroupOrder(t *testing.T) {
	t.Parallel()
	message := RenderOperationsDigest(OperationsDigestView{Runtime: opsmetrics.Snapshot{Groups: []opsmetrics.GroupRuntime{
		{ID: 99, Name: "原生顺序-先", RequestCount: 20, Status: opsmetrics.StatusOK},
		{ID: 2, Name: "原生顺序-后", RequestCount: 20, Status: opsmetrics.StatusOK},
	}}})
	data, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	first := strings.Index(string(data), "原生顺序-先")
	second := strings.Index(string(data), "原生顺序-后")
	if first < 0 || second < 0 || first >= second {
		t.Fatalf("native group order was not preserved: %s", data)
	}
}

func TestRenderOperationsDigestDeterministicallyTruncatesAndRetainsAbnormalities(t *testing.T) {
	t.Parallel()
	groups := make([]opsmetrics.GroupRuntime, 0, 128)
	for id := int64(1); id <= 127; id++ {
		groups = append(groups, opsmetrics.GroupRuntime{
			ID: id, Name: strings.Repeat("普通分组", 180), RequestCount: 20, Status: opsmetrics.StatusOK,
		})
	}
	groups = append(groups, opsmetrics.GroupRuntime{
		ID: 128, Name: "末尾异常分组", Status: opsmetrics.StatusReadFailed,
	})
	view := OperationsDigestView{Runtime: opsmetrics.Snapshot{Groups: groups}}
	first := RenderOperationsDigest(view)
	second := RenderOperationsDigest(view)
	firstData, err := first.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	secondData, err := second.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(firstData) >= maxCardBytes {
		t.Fatalf("card size = %d", len(firstData))
	}
	if string(firstData) != string(secondData) {
		t.Fatal("digest truncation is not deterministic")
	}
	for _, required := range []string{"末尾异常分组", "读取失败", "其余对象请在 /ops 查看"} {
		if !strings.Contains(string(firstData), required) {
			t.Fatalf("truncated digest missing %q: %s", required, firstData)
		}
	}
}

func TestRenderOperationsDigestBoundsEscapedPayloadBelowCardLimit(t *testing.T) {
	t.Parallel()
	escaped := strings.Repeat("<>&", 250)
	groups := make([]opsmetrics.GroupRuntime, 12)
	quality := make([]accountquality.AccountView, 12)
	footer := make([]string, 12)
	for index := range groups {
		groups[index] = opsmetrics.GroupRuntime{ID: int64(index + 1), Name: escaped, RequestCount: 20, Status: opsmetrics.StatusOK}
		quality[index] = accountquality.AccountView{AccountID: strings.Repeat("<>&", 250), Stability: escaped, Multiplier: "0.1x", TTFTP95: "200ms", LastResult: "通过"}
		footer[index] = escaped
	}
	message := RenderOperationsDigest(OperationsDigestView{
		Date: "2026-07-23", Runtime: opsmetrics.Snapshot{Groups: groups}, AccountQuality: accountquality.View{Available: true, Accounts: quality}, Footer: footer,
	})
	data, err := message.CardJSON()
	if err != nil {
		t.Fatalf("escaped digest must remain deliverable: %v", err)
	}
	if len(data) >= maxCardBytes {
		t.Fatalf("escaped card size = %d", len(data))
	}
	if len(message.Card.Elements) == 0 || message.Card.Elements[len(message.Card.Elements)-1].Tag != "action" {
		t.Fatalf("card lost its action: %#v", message.Card.Elements)
	}
}

func TestRenderOperationsDigestRetainsAccountQualityFailuresDuringTruncation(t *testing.T) {
	t.Parallel()
	accounts := make([]accountquality.AccountView, 0, 130)
	for id := 1; id <= 128; id++ {
		accounts = append(accounts, accountquality.AccountView{
			AccountID: strconv.Itoa(id), Stability: strings.Repeat("稳定", 220), Multiplier: "0.1x", TTFTP95: "200ms", LastResult: "通过",
		})
	}
	accounts = append(accounts,
		accountquality.AccountView{AccountID: "129", Stability: "成功 0/1", Multiplier: "0.1x", TTFTP95: "无成功样本", LastResult: "HTTP 错误"},
		accountquality.AccountView{AccountID: "130", Stability: "成功 0/1", Multiplier: "0.1x", TTFTP95: "无成功样本", LastResult: "超时"},
	)
	message := RenderOperationsDigest(OperationsDigestView{AccountQuality: accountquality.View{Available: true, Accounts: accounts}})
	data, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"账号 129", "HTTP 错误", "账号 130", "超时", "其余对象请在 /ops 查看"} {
		if !strings.Contains(text, required) {
			t.Fatalf("truncated quality digest missing %q: %s", required, text)
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
