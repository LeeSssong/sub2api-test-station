package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"example.invalid/relay-ops-service/internal/feishuevents"
)

const (
	RolePrimary = "primary"
	RoleBackup  = "backup"

	ErrorUnknownCommand = "unknown_command"
	ErrorInvalidEvent   = "invalid_event"
)

type ActionKind string

const (
	ActionSwitch ActionKind = "switch"
	ActionStatus ActionKind = "status"
)

type Action struct {
	Kind       ActionKind
	GroupName  string
	TargetRole string
}

type Decision struct {
	Accepted  bool
	Ignore    bool
	Command   string
	Action    Action
	ErrorCode string
}

func Parse(event feishuevents.MessageEvent) Decision {
	if event.EventID == "" || event.MessageID == "" || event.ChatID == "" || event.SenderOpenID == "" ||
		event.EventType != "im.message.receive_v1" || event.ChatType != "group" ||
		event.SenderType != "user" || event.MessageType != "text" {
		return Decision{Ignore: true, ErrorCode: ErrorInvalidEvent}
	}
	var content struct {
		Text string `json:"text"`
	}
	decoder := json.NewDecoder(bytes.NewBufferString(event.Content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&content); err != nil || !atJSONEnd(decoder) {
		return Decision{Ignore: true, ErrorCode: ErrorInvalidEvent}
	}
	text := content.Text
	if len(event.Mentions) > 0 {
		mention := event.Mentions[0]
		if mention.Key != "" && mention.MentionedType == "app" && strings.HasPrefix(text, mention.Key) {
			text = strings.TrimPrefix(text, mention.Key)
		}
	}
	command := strings.TrimSpace(text)
	switch command {
	case "切换 GPT-Pro 到灾备":
		return accepted(command, ActionSwitch, "GPT-Pro", RoleBackup)
	case "切换 GPT-Plus 到灾备":
		return accepted(command, ActionSwitch, "GPT-Plus", RoleBackup)
	case "恢复 GPT-Pro 主分组":
		return accepted(command, ActionSwitch, "GPT-Pro", RolePrimary)
	case "恢复 GPT-Plus 主分组":
		return accepted(command, ActionSwitch, "GPT-Plus", RolePrimary)
	case "查询当前分组状态":
		return accepted(command, ActionStatus, "", "")
	default:
		return Decision{ErrorCode: ErrorUnknownCommand}
	}
}

func accepted(command string, kind ActionKind, groupName, targetRole string) Decision {
	return Decision{
		Accepted: true,
		Command:  command,
		Action: Action{
			Kind:       kind,
			GroupName:  groupName,
			TargetRole: targetRole,
		},
	}
}

func atJSONEnd(decoder *json.Decoder) bool {
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}
