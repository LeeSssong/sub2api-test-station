package notify

import (
	"fmt"
	"strings"
)

type GroupAlertAccount struct {
	Name      string
	ErrorCode string
	Duration  string
}

type GroupAlertView struct {
	GroupName string
	Available int
	Total     int
	Down      []GroupAlertAccount
	Recovery  bool
}

func RenderGroupAlert(view GroupAlertView) FeishuMessage {
	name := digestValue(view.GroupName)
	lines := []string{fmt.Sprintf("可用 %d / 共 %d", view.Available, view.Total)}

	template := "red"
	title := "⚠️ 分组可用账号不足：" + name
	if view.Recovery {
		template = "green"
		title = "分组可用性已恢复：" + name
	}
	for _, account := range view.Down {
		lines = append(lines, fmt.Sprintf("%s　%s　%s",
			digestValue(account.Name), digestValue(account.ErrorCode), digestValue(account.Duration)))
	}
	if !view.Recovery {
		lines = append(lines, "", "建议：为上述账号充值，或临时关闭 schedulable 止血")
	}
	return FeishuMessage{MsgType: "interactive", Card: &Card{
		Config: CardConfig{WideScreenMode: true},
		Header: CardHeader{Title: CardText{Tag: "plain_text", Content: title}, Template: template},
		Elements: []CardElement{
			{Tag: "div", Text: &CardText{Tag: "lark_md", Content: strings.Join(lines, "\n")}},
			{Tag: "action", Actions: []CardAction{{
				Tag: "button", Text: CardText{Tag: "plain_text", Content: "运维后台"}, Type: "primary", MultiURL: &CardURL{URL: "/ops"},
			}}},
		},
	}}
}
