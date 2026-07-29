package notify

import (
	"strings"
	"testing"
	"time"
)

func TestRenderUserImpactUsesApprovedHumanLanguageSections(t *testing.T) {
	view := UserImpactView{
		GroupName: "GPT PLUS 内测", Severity: "P1",
		Headline:   "部分请求持续失败",
		UserImpact: "部分用户可能遇到请求失败或需要重试；当前不是完全不可用。",
		Current: []UserImpactFact{
			{Label: "过去 15 分钟", Value: "共 142 次请求，其中 20 次失败，错误率 14.08%。"},
			{Label: "当前容量", Value: "可用账号 2 / 3，仍有服务能力。"},
		},
		Clues: []UserImpactFact{
			{Label: "原生监控", Value: "当前正常；尚未确认具体故障账号。"},
		},
		Action:     "检查该分组最近错误和账号状态，优先确认失败是否集中在单个账号。",
		ObservedAt: time.Date(2026, 7, 29, 13, 3, 0, 0, time.UTC),
	}
	message := RenderUserImpact(view)
	if message.Severity != "P1" ||
		message.Card.Header.Title.Content != "P1｜GPT PLUS 内测分组部分请求持续失败" {
		t.Fatalf("message = %#v", message)
	}
	text := message.RenderedText()
	previous := -1
	for _, section := range []string{"发生了什么", "用户影响", "当前证据", "已知线索", "建议处理", "最新证据"} {
		index := strings.Index(text, section)
		if index <= previous {
			t.Fatalf("section %q is missing or out of order in %s", section, text)
		}
		previous = index
	}
	for _, forbidden := range []string{
		"group #", "new_evidence", "error_rate", "ttft_p95",
		"active but paused", "balance_exhausted",
	} {
		if strings.Contains(message.RenderedText(), forbidden) {
			t.Fatalf("technical term leaked: %s", forbidden)
		}
	}
}

func TestRenderUserImpactProgressUsesHumanTitle(t *testing.T) {
	message := RenderUserImpact(UserImpactView{
		GroupName: "GPT PLUS 内测", Severity: "P0", Progress: true,
		Headline:   "已从部分失败升级为全部失败",
		UserImpact: "该分组用户当前无法正常完成请求。",
		Current:    []UserImpactFact{{Label: "最新情况", Value: "最近 15 分钟 31 次请求全部失败。"}},
		Action:     "立即检查剩余账号请求错误，并恢复至少一个备用账号。",
	})
	if message.Card.Header.Title.Content != "事故进展｜GPT PLUS 内测分组已从部分失败升级为全部失败" {
		t.Fatalf("title = %q", message.Card.Header.Title.Content)
	}
	if strings.Contains(message.RenderedText(), "progressed") {
		t.Fatal("internal transition leaked")
	}
}

func TestRenderUserImpactRecoveryHasNoIncidentActions(t *testing.T) {
	message := RenderUserImpactRecovery(UserImpactRecoveryView{
		GroupName: "GPT PLUS 内测", Result: "最近两个观测均已回到健康范围。",
		Duration: "约 30 分钟", Current: "可用账号 3 / 3，未发现持续用户影响。",
		ObservedAt: time.Date(2026, 7, 29, 13, 30, 0, 0, time.UTC),
	})
	if message.Card.Header.Title.Content != "恢复｜GPT PLUS 内测分组请求已恢复" ||
		message.Severity != "" {
		t.Fatalf("recovery = %#v", message)
	}
	text := message.RenderedText()
	for _, want := range []string{"恢复结果", "影响时长", "当前状态", "后续"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	payload, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"确认并接手", "加急"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("recovery contains %q", forbidden)
		}
	}
}

func TestRenderUserImpactReminderIsConciseAndKeepsSeverity(t *testing.T) {
	message := RenderUserImpactReminder(UserImpactReminderView{
		GroupName: "GPT PLUS 内测", Severity: "P0",
		Headline:   "全部请求持续失败",
		Duration:   "15 分钟",
		LatestFact: "最近 15 分钟 31 次请求全部失败。",
		Capacity:   "当前可用账号 1 / 3。",
	})
	if message.Severity != "P0" ||
		message.Card.Header.Template != "red" ||
		message.Card.Header.Title.Content !=
			"再次提醒｜GPT PLUS 内测分组全部请求持续失败" {
		t.Fatalf("message=%#v", message)
	}
	text := message.RenderedText()
	for _, want := range []string{
		"15 分钟", "尚未有人确认接手",
		"最近 15 分钟 31 次请求全部失败",
		"当前可用账号 1 / 3",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	for _, forbidden := range []string{
		"第 1 次提醒", "发生了什么", "用户影响", "已知线索", "建议处理",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("reminder cloned initial card section %q: %s", forbidden, text)
		}
	}
}
