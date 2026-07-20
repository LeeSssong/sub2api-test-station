package commands

import (
	"fmt"
	"testing"

	"example.invalid/relay-ops-service/internal/feishuevents"
)

func TestParseAcceptsOnlyFixedCommands(t *testing.T) {
	tests := []struct {
		text       string
		kind       ActionKind
		group      string
		targetRole string
	}{
		{"切换 GPT-Pro 到灾备", ActionSwitch, "GPT-Pro", RoleBackup},
		{"切换 GPT-Plus 到灾备", ActionSwitch, "GPT-Plus", RoleBackup},
		{"恢复 GPT-Pro 主分组", ActionSwitch, "GPT-Pro", RolePrimary},
		{"恢复 GPT-Plus 主分组", ActionSwitch, "GPT-Plus", RolePrimary},
		{"查询当前分组状态", ActionStatus, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			decision := Parse(validEvent(tt.text))
			if !decision.Accepted || decision.Ignore || decision.Command != tt.text {
				t.Fatalf("decision = %#v", decision)
			}
			if decision.Action.Kind != tt.kind || decision.Action.GroupName != tt.group || decision.Action.TargetRole != tt.targetRole {
				t.Fatalf("action = %#v", decision.Action)
			}
		})
	}
}

func TestParseNormalizesOnlyLeadingAppMentionAndOuterWhitespace(t *testing.T) {
	event := validEvent("@_user_1\u3000切换 GPT-Pro 到灾备\n")
	event.Mentions = []feishuevents.Mention{{Key: "@_user_1", OpenID: "ou-bot", MentionedType: "app", Name: "星桥AI监控Agent"}}

	decision := Parse(event)
	if !decision.Accepted || decision.Command != "切换 GPT-Pro 到灾备" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestParseRejectsUnknownCombinedOrParameterizedText(t *testing.T) {
	tests := []string{
		"切换GPT-Pro到灾备",
		"切换 GPT-Pro 到灾备 现在",
		"切换 GPT-Pro 到灾备\n查询当前分组状态",
		"请切换 GPT-Pro 到灾备",
		"`切换 GPT-Pro 到灾备`",
		"https://example.com/切换 GPT-Pro 到灾备",
		"@_user_1 切换 GPT-Pro 到灾备",
	}
	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			decision := Parse(validEvent(text))
			if decision.Accepted || decision.Ignore || decision.ErrorCode != ErrorUnknownCommand {
				t.Fatalf("decision = %#v", decision)
			}
		})
	}
}

func TestParseDoesNotRemoveUserOrNonLeadingMention(t *testing.T) {
	tests := []feishuevents.MessageEvent{
		func() feishuevents.MessageEvent {
			event := validEvent("@_user_1 切换 GPT-Pro 到灾备")
			event.Mentions = []feishuevents.Mention{{Key: "@_user_1", MentionedType: "user"}}
			return event
		}(),
		func() feishuevents.MessageEvent {
			event := validEvent("切换 GPT-Pro 到灾备 @_user_1")
			event.Mentions = []feishuevents.Mention{{Key: "@_user_1", MentionedType: "app"}}
			return event
		}(),
	}
	for i, event := range tests {
		decision := Parse(event)
		if decision.Accepted || decision.ErrorCode != ErrorUnknownCommand {
			t.Fatalf("case %d decision = %#v", i, decision)
		}
	}
}

func TestParseAllowsEveryGroupAndRejectsNonGroupActors(t *testing.T) {
	for _, chatID := range []string{"chat-one", "chat-two", "another-tenant-chat"} {
		event := validEvent("查询当前分组状态")
		event.ChatID = chatID
		if decision := Parse(event); !decision.Accepted {
			t.Fatalf("chat %q rejected: %#v", chatID, decision)
		}
	}

	tests := []struct {
		name   string
		mutate func(*feishuevents.MessageEvent)
	}{
		{"private", func(event *feishuevents.MessageEvent) { event.ChatType = "p2p" }},
		{"topic group", func(event *feishuevents.MessageEvent) { event.ChatType = "topic_group" }},
		{"app sender", func(event *feishuevents.MessageEvent) { event.SenderType = "app" }},
		{"bot sender", func(event *feishuevents.MessageEvent) { event.SenderType = "bot" }},
		{"system sender", func(event *feishuevents.MessageEvent) { event.SenderType = "system" }},
		{"image", func(event *feishuevents.MessageEvent) { event.MessageType = "image" }},
		{"missing event id", func(event *feishuevents.MessageEvent) { event.EventID = "" }},
		{"missing message id", func(event *feishuevents.MessageEvent) { event.MessageID = "" }},
		{"missing sender", func(event *feishuevents.MessageEvent) { event.SenderOpenID = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := validEvent("查询当前分组状态")
			tt.mutate(&event)
			decision := Parse(event)
			if !decision.Ignore || decision.Accepted {
				t.Fatalf("decision = %#v", decision)
			}
		})
	}
}

func TestParseRejectsMalformedTextContent(t *testing.T) {
	event := validEvent("查询当前分组状态")
	event.Content = `{"text":"查询当前分组状态","extra":true}`
	decision := Parse(event)
	if !decision.Ignore || decision.ErrorCode != ErrorInvalidEvent {
		t.Fatalf("decision = %#v", decision)
	}
}

func validEvent(text string) feishuevents.MessageEvent {
	return feishuevents.MessageEvent{
		EventID:      "evt-1",
		EventType:    "im.message.receive_v1",
		AppID:        "cli-test",
		MessageID:    "msg-1",
		ChatID:       "chat-1",
		ChatType:     "group",
		MessageType:  "text",
		Content:      fmt.Sprintf(`{"text":%q}`, text),
		SenderOpenID: "ou-user",
		SenderType:   "user",
	}
}
