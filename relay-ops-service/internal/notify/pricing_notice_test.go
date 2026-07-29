package notify

import (
	"strings"
	"testing"
	"time"
)

func TestRenderPricingNoticeIsP2OneShotWithoutIncidentLanguage(t *testing.T) {
	message := RenderPricingNotice(PricingNoticeView{
		Upstream:   "Neko",
		Change:     "公开倍率由 0.07x 变为 0.10x。",
		Review:     "核对该上游公开价格与当前毛利是否仍符合预期。",
		ObservedAt: time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC),
	})
	if message.Severity != "P2" ||
		message.Card.Header.Title.Content != "价格变更｜Neko 公开定价发生变化" {
		t.Fatalf("message = %#v", message)
	}
	text := message.RenderedText()
	for _, want := range []string{
		"发生了什么", "公开倍率由 0.07x 变为 0.10x",
		"系统未做", "没有修改价格、倍率、路由或账号",
		"需要核对", "当前毛利",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	payload, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"确认并接手", "升级", "恢复", "Agent", "第 1 次提醒",
	} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("pricing event contains incident language %q", forbidden)
		}
	}
}
