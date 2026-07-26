package notify

import (
	"strings"
	"testing"
)

func TestRenderGroupAlertContent(t *testing.T) {
	view := GroupAlertView{
		GroupName: "GPT-Plus", Available: 1, Total: 3,
		Down: []GroupAlertAccount{
			{Name: "Plus-XN-0.09", ErrorCode: "balance_exhausted", Duration: "已持续 3 天"},
			{Name: "Plus-XM-0.1", ErrorCode: "balance_exhausted", Duration: "已持续 5 天"},
		},
	}
	payload, err := RenderGroupAlert(view).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	text := string(payload)
	for _, want := range []string{"GPT-Plus", "可用 1 / 共 3", "Plus-XN-0.09", "balance_exhausted", "已持续 5 天", "red"} {
		if !strings.Contains(text, want) {
			t.Fatalf("缺少 %q: %s", want, text)
		}
	}
	if !strings.Contains(text, "建议") {
		t.Fatal("必须给出建议动作")
	}
}

func TestRenderGroupAlertRecovery(t *testing.T) {
	view := GroupAlertView{GroupName: "GPT-Plus", Available: 3, Total: 3, Recovery: true}
	payload, err := RenderGroupAlert(view).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, "green") {
		t.Fatalf("恢复卡应为绿色: %s", text)
	}
	if !strings.Contains(text, "已恢复") {
		t.Fatal("恢复卡应说明已恢复")
	}
}

func TestRenderGroupAlertLeaksNoSecrets(t *testing.T) {
	view := GroupAlertView{
		GroupName: "GPT-Plus", Available: 0, Total: 2,
		Down: []GroupAlertAccount{{Name: "Plus-XN-0.09", ErrorCode: "balance_exhausted", Duration: "已持续 1 天"}},
	}
	payload, err := RenderGroupAlert(view).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	text := string(payload)
	for _, forbidden := range []string{"http://", "https://api.", "sk-", "x-api-key"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("卡片泄露敏感内容 %q: %s", forbidden, text)
		}
	}
}
