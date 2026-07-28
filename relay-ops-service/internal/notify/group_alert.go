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
	Severity  string
	Down      []GroupAlertAccount
	Recovery  bool
}

func RenderGroupAlert(view GroupAlertView) FeishuMessage {
	name := digestValue(view.GroupName)
	if view.Recovery {
		return RenderRecoveryCard(RecoveryCardView{
			Title:   "分组可用性已恢复：" + name,
			Summary: "分组可用性已恢复",
			Detail:  "分组账号已重新满足可用性要求",
			Metrics: []RecoveryMetric{
				{Label: "可用账号", Value: fmt.Sprintf("%d / %d", view.Available, view.Total)},
				{Label: "当前状态", Value: "可用性正常"},
				{Label: "健康确认", Value: "1 个完整窗口"},
			},
			Basis:  []string{"当前可用账号数量已回到健康范围"},
			Source: "Sub2API 账号监控分组快照",
			Focus:  "继续观察分组可用账号数量",
			Links:  []Link{{Label: "运维后台", URL: "/ops"}},
		})
	}

	severity := view.Severity
	if severity != "P0" {
		severity = "P1"
	}
	impact := "分组冗余不足，部分请求可能受到影响"
	if severity == "P0" {
		impact = "分组已无可用账号，用户请求将失败"
	}
	lines := []string{
		"**用户影响** " + impact,
		fmt.Sprintf("**剩余容量** 可用 %d / 共 %d", view.Available, view.Total),
	}

	template := "red"
	title := severity + "｜分组可用账号不足：" + name
	for _, account := range view.Down {
		lines = append(lines, fmt.Sprintf("%s　%s",
			digestValue(account.Name), digestValue(account.ErrorCode)))
	}
	lines = append(lines, "", "建议：为上述账号充值，或临时关闭 schedulable 止血")
	return FeishuMessage{MsgType: "interactive", Severity: severity, Card: &Card{
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
