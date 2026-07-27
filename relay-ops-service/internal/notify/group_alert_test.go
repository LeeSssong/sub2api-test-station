package notify

import (
	"strings"
	"testing"
)

func TestRenderGroupAlertContent(t *testing.T) {
	view := GroupAlertView{
		GroupName: "GPT-Plus", Available: 1, Total: 3,
		Down: []GroupAlertAccount{
			{Name: "Plus-XN-0.09", ErrorCode: "余额耗尽"},
			{Name: "Plus-XM-0.1", ErrorCode: "余额耗尽"},
		},
	}
	payload, err := RenderGroupAlert(view).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	text := string(payload)
	for _, want := range []string{"GPT-Plus", "可用 1 / 共 3", "Plus-XN-0.09", "余额耗尽", "red"} {
		if !strings.Contains(text, want) {
			t.Fatalf("缺少 %q: %s", want, text)
		}
	}
	// 曾经的 Duration 字段从未被赋值，渲染出「名字　error_code　」这种尾随
	// 全角分隔符；字段删除后不得再出现。
	if strings.Contains(text, "余额耗尽　") {
		t.Fatalf("故障行存在尾随分隔符: %s", text)
	}
	if !strings.Contains(text, "建议") {
		t.Fatal("必须给出建议动作")
	}
}

func TestRenderGroupAlertRecovery(t *testing.T) {
	view := GroupAlertView{GroupName: "GPT-Plus", Available: 3, Total: 3, Recovery: true}
	message := RenderGroupAlert(view)
	payload, err := message.CardJSON()
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
	for _, want := range []string{"可用账号", "3 / 3", "当前状态", "可用性正常", "数据来源", "Sub2API 账号监控分组快照"} {
		if !strings.Contains(message.RenderedText(), want) {
			t.Fatalf("恢复卡缺少 %q: %s", want, message.RenderedText())
		}
	}
	if len(message.Card.Elements) < 3 || len(message.Card.Elements[1].Fields) != 3 {
		t.Fatalf("恢复卡未使用结构化指标: %#v", message.Card.Elements)
	}
}

// 对抗性输入：必须真的把敏感串喂进去，否则断言「产物不含 sk-」只是因为
// 输入本身干净而碰巧通过，抓不住任何真实泄露。
func TestRenderGroupAlertLeaksNoSecrets(t *testing.T) {
	view := GroupAlertView{
		GroupName: "https://api.shuaiapi.com/v1", Available: 0, Total: 2,
		Down: []GroupAlertAccount{
			{Name: "http://internal.host/admin", ErrorCode: "sk-abc123 leaked"},
			{Name: "Plus-XN-0.09", ErrorCode: "api_key rejected"},
		},
	}
	payload, err := RenderGroupAlert(view).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	text := string(payload)
	for _, forbidden := range []string{"http://", "https://api.", "sk-", "x-api-key", "api_key"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("卡片泄露敏感内容 %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "[已脱敏]") {
		t.Fatalf("敏感输入必须被替换为脱敏标记: %s", text)
	}
}

func TestRenderGroupAlertBoundsOversizedDownList(t *testing.T) {
	// 上游供应商级故障会让整组账号同时不可用。卡片超过 maxCardBytes 会让
	// CardJSON 报错，告警就在最该送达的时刻丢失。
	down := make([]GroupAlertAccount, 600)
	for i := range down {
		down[i] = GroupAlertAccount{
			Name:      strings.Repeat("A", 60),
			ErrorCode: "余额耗尽",
		}
	}
	payload, err := RenderGroupAlert(GroupAlertView{
		GroupName: "GPT-Plus", Available: 0, Total: 600, Down: down,
	}).CardJSON()
	if err != nil {
		t.Fatalf("大规模故障时告警必须仍能渲染: %v", err)
	}
	if len(payload) > maxCardBytes {
		t.Fatalf("card size = %d, exceeds %d", len(payload), maxCardBytes)
	}
}

func TestRenderGroupAlertRecoveryOmitsDownList(t *testing.T) {
	payload, err := RenderGroupAlert(GroupAlertView{
		GroupName: "GPT-Plus", Available: 3, Total: 3, Recovery: true,
		Down: []GroupAlertAccount{{Name: "Plus-XN-0.09", ErrorCode: "balance_exhausted"}},
	}).CardJSON()
	if err != nil {
		t.Fatalf("CardJSON: %v", err)
	}
	if strings.Contains(string(payload), "Plus-XN-0.09") {
		t.Fatalf("恢复卡不得再列出已不可用账号: %s", payload)
	}
}
