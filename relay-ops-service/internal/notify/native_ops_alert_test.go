package notify

import (
	"strings"
	"testing"
	"time"
)

func TestRenderNativeOpsAlertCardsIncludeLifecycleAndSafeDimensions(t *testing.T) {
	view := NativeOpsAlertView{
		EventID: 41, RuleID: 7, Title: "错误率过高", Severity: "P0", Status: "firing",
		MetricValue: ptrFloat(80), ThresholdValue: ptrFloat(5),
		Dimensions: map[string]string{
			"platform": "openai", "group_id": "16", "region": "us",
			"model": "gpt-5", "account_id": "acct-1",
			"api_key": "sk-secret", "cookie": "session-secret", "unknown": "private",
		},
		FiredAt: time.Date(2026, 7, 30, 13, 48, 0, 0, time.UTC),
	}
	message := RenderNativeOpsAlert(view)
	if !strings.HasPrefix(message.Card.Header.Title.Content, "P0｜Sub2API 原生告警｜错误率过高") {
		t.Fatalf("title=%q", message.Card.Header.Title.Content)
	}
	text := message.RenderedText()
	for _, want := range []string{"事件 ID**：41", "规则 ID**：7", "openai", "group_id：16", "当前值**：80", "阈值**：5"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text missing %q: %s", want, text)
		}
	}
	for _, forbidden := range []string{"sk-secret", "session-secret", "private"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("secret/private dimension leaked: %q", forbidden)
		}
	}
	if _, err := message.CardJSON(); err != nil {
		t.Fatal(err)
	}
}

func TestRenderNativeOpsAlertTerminalSummaryAndReminder(t *testing.T) {
	resolved := time.Date(2026, 7, 30, 13, 55, 0, 0, time.UTC)
	view := NativeOpsAlertView{
		EventID: 41, RuleID: 7, Title: strings.Repeat("x", 5000), Severity: "P1",
		Status: "resolved", FiredAt: resolved.Add(-7 * time.Minute), ResolvedAt: &resolved,
		TerminalSummary: true,
	}
	recovery := RenderNativeOpsAlertRecovery(view)
	if !strings.Contains(recovery.Card.Header.Title.Content, "已恢复｜Sub2API 原生告警") ||
		!strings.Contains(recovery.RenderedText(), "轮询间短时告警") {
		t.Fatalf("recovery=%s", recovery.RenderedText())
	}
	reminder := RenderNativeOpsAlertReminder(NativeOpsAlertView{Title: "持续异常", Severity: "P1", ReminderLevel: 1})
	if !strings.HasPrefix(reminder.Card.Header.Title.Content, "P1｜仍在告警｜Sub2API 原生告警｜持续异常") {
		t.Fatalf("reminder title=%q", reminder.Card.Header.Title.Content)
	}
	if _, err := recovery.CardJSON(); err != nil {
		t.Fatal(err)
	}
}

func ptrFloat(value float64) *float64 { return &value }
