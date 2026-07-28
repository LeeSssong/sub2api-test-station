package notify

import (
	"fmt"
	"strings"
	"time"
)

type UserImpactFact struct {
	Label     string
	Value     string
	Confirmed bool
}

type UserImpactView struct {
	GroupName  string
	Severity   string
	Headline   string
	Summary    string
	UserImpact string
	Current    []UserImpactFact
	Clues      []UserImpactFact
	Action     string
	ObservedAt time.Time
	Progress   bool
}

type UserImpactRecoveryView struct {
	GroupName  string
	Result     string
	Duration   string
	Current    string
	ObservedAt time.Time
}

type UserImpactReminderView struct {
	GroupName  string
	Severity   string
	Headline   string
	Duration   string
	LatestFact string
	Capacity   string
}

func RenderUserImpact(view UserImpactView) FeishuMessage {
	groupName := userImpactGroupName(view.GroupName)
	severity := strings.ToUpper(strings.TrimSpace(view.Severity))
	if severity != "P0" {
		severity = "P1"
	}
	headline := humanText(view.Headline, "需要关注")
	title := severity + "｜" + groupName + "分组" + headline
	template := "orange"
	if severity == "P0" {
		template = "red"
	}

	var elements []CardElement
	if view.Progress {
		title = "事故进展｜" + groupName + "分组" + headline
		elements = impactSections([]section{
			{Title: "影响变化", Lines: []string{humanText(view.UserImpact, "用户影响发生变化。")}},
			{Title: "最新线索", Lines: factLines(view.Current, view.Clues)},
			{Title: "建议处理", Lines: []string{humanText(view.Action, "请查看运维后台。")}},
			{Title: "最新证据", Lines: []string{formatObservedAt(view.ObservedAt)}},
		})
	} else {
		clueTitle := "已知线索"
		for _, clue := range view.Clues {
			if clue.Confirmed {
				clueTitle = "已知原因"
				break
			}
		}
		elements = impactSections([]section{
			{Title: "发生了什么", Lines: []string{humanText(view.Summary, firstFactValue(view.Current))}},
			{Title: "用户影响", Lines: []string{humanText(view.UserImpact, "当前用户影响尚待确认。")}},
			{Title: "当前证据", Lines: factLines(view.Current, nil)},
			{Title: clueTitle, Lines: factLines(view.Clues, nil)},
			{Title: "建议处理", Lines: []string{humanText(view.Action, "请查看运维后台。")}},
			{Title: "最新证据", Lines: []string{formatObservedAt(view.ObservedAt)}},
		})
	}
	elements = append(elements, operationsAction())
	return FeishuMessage{
		MsgType:  "interactive",
		Severity: severity,
		Card: &Card{
			Config:   CardConfig{WideScreenMode: true},
			Header:   CardHeader{Title: CardText{Tag: "plain_text", Content: title}, Template: template},
			Elements: elements,
		},
	}
}

func RenderUserImpactRecovery(view UserImpactRecoveryView) FeishuMessage {
	groupName := userImpactGroupName(view.GroupName)
	elements := impactSections([]section{
		{Title: "恢复结果", Lines: []string{humanText(view.Result, "最近两个观测均已回到健康范围。")}},
		{Title: "影响时长", Lines: []string{humanText(view.Duration, "影响时长待确认。")}},
		{Title: "当前状态", Lines: []string{humanText(view.Current, "未发现持续用户影响。")}},
		{Title: "后续", Lines: []string{"无需立即操作，继续观察下一小时运行情况。"}},
		{Title: "最新证据", Lines: []string{formatObservedAt(view.ObservedAt)}},
	})
	elements = append(elements, operationsAction())
	return FeishuMessage{
		MsgType: "interactive",
		Card: &Card{
			Config: CardConfig{WideScreenMode: true},
			Header: CardHeader{
				Title:    CardText{Tag: "plain_text", Content: "恢复｜" + groupName + "分组请求已恢复"},
				Template: "green",
			},
			Elements: elements,
		},
	}
}

func RenderUserImpactReminder(view UserImpactReminderView) FeishuMessage {
	groupName := userImpactGroupName(view.GroupName)
	severity := strings.ToUpper(strings.TrimSpace(view.Severity))
	template := "orange"
	if severity == "P0" {
		template = "red"
	} else {
		severity = "P1"
	}
	lines := []string{
		"**持续时间**：" + humanText(view.Duration, "一段时间"),
		"**接手状态**：尚未有人确认接手。",
		"",
		"**最新情况**",
		humanText(view.LatestFact, "最新运行情况待确认。"),
	}
	if strings.TrimSpace(view.Capacity) != "" {
		lines = append(lines, humanText(view.Capacity, ""))
	}
	return FeishuMessage{
		MsgType:  "interactive",
		Severity: severity,
		Card: &Card{
			Config: CardConfig{WideScreenMode: true},
			Header: CardHeader{
				Title: CardText{
					Tag:     "plain_text",
					Content: "再次提醒｜" + groupName + "分组" + humanText(view.Headline, "需要关注"),
				},
				Template: template,
			},
			Elements: []CardElement{
				{Tag: "div", Text: &CardText{Tag: "lark_md", Content: fitDigestSection(lines)}},
				operationsAction(),
			},
		},
	}
}

type section struct {
	Title string
	Lines []string
}

func impactSections(sections []section) []CardElement {
	elements := make([]CardElement, 0, len(sections))
	for _, item := range sections {
		lines := []string{"**" + item.Title + "**"}
		hasContent := false
		for _, line := range item.Lines {
			line = humanText(line, "")
			if line == "" {
				continue
			}
			hasContent = true
			lines = append(lines, line)
		}
		if !hasContent {
			lines = append(lines, "尚未确认。")
		}
		elements = append(elements, CardElement{
			Tag: "div", Text: &CardText{Tag: "lark_md", Content: fitDigestSection(lines)},
		})
	}
	return elements
}

func factLines(primary, additional []UserImpactFact) []string {
	facts := append(append([]UserImpactFact(nil), primary...), additional...)
	lines := make([]string, 0, len(facts))
	for _, fact := range facts {
		label := humanText(fact.Label, "")
		value := humanText(fact.Value, "")
		switch {
		case label != "" && value != "":
			lines = append(lines, fmt.Sprintf("%s：%s", label, value))
		case value != "":
			lines = append(lines, value)
		}
		if len(lines) == 8 {
			break
		}
	}
	return lines
}

func operationsAction() CardElement {
	return CardElement{Tag: "action", Actions: []CardAction{{
		Tag: "button", Text: CardText{Tag: "plain_text", Content: "查看运维后台"},
		Type: "primary", MultiURL: &CardURL{URL: "/ops"},
	}}}
}

func userImpactGroupName(value string) string {
	value = humanText(value, "公开")
	if strings.Contains(strings.ToLower(value), "group #") {
		value = "公开"
	}
	value = strings.TrimSpace(strings.TrimSuffix(value, "分组"))
	if value == "" {
		return "公开"
	}
	return value
}

func firstFactValue(facts []UserImpactFact) string {
	if len(facts) == 0 {
		return "当前运行情况需要关注。"
	}
	return facts[0].Value
}

func humanText(value, fallback string) string {
	value = digestValue(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func formatObservedAt(value time.Time) string {
	if value.IsZero() {
		return "时间待确认"
	}
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	return value.In(shanghai).Format("1月2日 15:04")
}
