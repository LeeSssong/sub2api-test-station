package notify

import (
	"fmt"
)

// GroupAlertAccount carries only fields the caller actually fills. A former
// Duration field was never assigned, and rendering the empty slot left every
// down row with a trailing full-width separator; no data source provides a
// reliable outage duration (the alert window is one hour), so the field is
// gone rather than populated with a misleading value.
type GroupAlertAccount struct {
	Name      string
	ErrorCode string
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
	if !view.Recovery {
		for _, account := range view.Down {
			lines = append(lines, fmt.Sprintf("%s　%s",
				digestValue(account.Name), digestValue(account.ErrorCode)))
		}
		lines = append(lines, "", "建议：为上述账号充值，或临时关闭 schedulable 止血")
	}
	return FeishuMessage{MsgType: "interactive", Card: &Card{
		Config: CardConfig{WideScreenMode: true},
		Header: CardHeader{Title: CardText{Tag: "plain_text", Content: title}, Template: template},
		Elements: []CardElement{
			// fitDigestSection bounds the section at 4 KiB and trims each line.
			// Down has no upper bound — a supplier-wide outage is exactly when
			// this alert matters most, and an oversized card makes CardJSON
			// fail, so the alert would be lost precisely when it is needed.
			{Tag: "div", Text: &CardText{Tag: "lark_md", Content: fitDigestSection(lines)}},
			{Tag: "action", Actions: []CardAction{{
				Tag: "button", Text: CardText{Tag: "plain_text", Content: "运维后台"}, Type: "primary", MultiURL: &CardURL{URL: "/ops"},
			}}},
		},
	}}
}
