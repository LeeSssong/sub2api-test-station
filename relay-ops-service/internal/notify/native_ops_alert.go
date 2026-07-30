package notify

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type NativeOpsAlertView struct {
	EventID         int64
	RuleID          int64
	Title           string
	Severity        string
	Status          string
	MetricValue     *float64
	ThresholdValue  *float64
	Dimensions      map[string]string
	FiredAt         time.Time
	ResolvedAt      *time.Time
	ReminderLevel   int
	SilenceExpired  bool
	TerminalSummary bool
}

func RenderNativeOpsAlert(view NativeOpsAlertView) FeishuMessage {
	severity := nativeAlertSeverity(view.Severity)
	return nativeOpsAlertMessage(view, severity+"｜Sub2API 原生告警｜"+nativeAlertTitle(view.Title), "red", "告警")
}

func RenderNativeOpsAlertRecovery(view NativeOpsAlertView) FeishuMessage {
	message := nativeOpsAlertMessage(view, "已恢复｜Sub2API 原生告警｜"+nativeAlertTitle(view.Title), "green", "已恢复")
	return message
}

func RenderNativeOpsAlertManualClose(view NativeOpsAlertView) FeishuMessage {
	return nativeOpsAlertMessage(view, "人工关闭｜Sub2API 原生告警｜"+nativeAlertTitle(view.Title), "blue", "人工关闭")
}

func RenderNativeOpsAlertReminder(view NativeOpsAlertView) FeishuMessage {
	severity := nativeAlertSeverity(view.Severity)
	return nativeOpsAlertMessage(view, severity+"｜仍在告警｜Sub2API 原生告警｜"+nativeAlertTitle(view.Title), "orange", "仍在告警")
}

func nativeOpsAlertMessage(view NativeOpsAlertView, title, template, status string) FeishuMessage {
	lines := []string{
		"**状态**：" + status,
		fmt.Sprintf("**事件 ID**：%d", view.EventID),
		fmt.Sprintf("**规则 ID**：%d", view.RuleID),
	}
	if view.MetricValue != nil {
		lines = append(lines, "**当前值**："+formatNativeFloat(*view.MetricValue))
	}
	if view.ThresholdValue != nil {
		lines = append(lines, "**阈值**："+formatNativeFloat(*view.ThresholdValue))
	}
	if dimensions := nativeAlertDimensions(view.Dimensions); len(dimensions) > 0 {
		lines = append(lines, "**维度**")
		for _, key := range []string{"platform", "group_id", "region", "model", "account_id"} {
			if value, ok := dimensions[key]; ok {
				lines = append(lines, key+"："+value)
			}
		}
	}
	if !view.FiredAt.IsZero() {
		lines = append(lines, "**触发时间**："+formatObservedAt(view.FiredAt))
	}
	if view.ResolvedAt != nil && !view.ResolvedAt.IsZero() {
		lines = append(lines, "**结束时间**："+formatObservedAt(*view.ResolvedAt))
	}
	if view.TerminalSummary {
		lines = append(lines, "**说明**：轮询间短时告警，触发与结束均已记录。")
	}
	if view.SilenceExpired {
		lines = append(lines, "**静默**：静默已到期，告警重新通知。")
	}
	if view.ReminderLevel > 0 {
		lines = append(lines, fmt.Sprintf("**提醒级别**：%d", view.ReminderLevel))
	}
	elements := []CardElement{
		{Tag: "div", Text: &CardText{Tag: "lark_md", Content: fitDigestSection(lines)}},
		operationsAction(),
	}
	return FeishuMessage{
		MsgType:  "interactive",
		Severity: nativeAlertSeverity(view.Severity),
		Card: &Card{
			Config:   CardConfig{WideScreenMode: true},
			Header:   CardHeader{Title: CardText{Tag: "plain_text", Content: truncateNativeText(title, 240)}, Template: template},
			Elements: elements,
		},
	}
}

func nativeAlertDimensions(input map[string]string) map[string]string {
	allowed := map[string]struct{}{
		"platform": {}, "group_id": {}, "region": {}, "model": {}, "account_id": {},
	}
	output := make(map[string]string)
	for key, value := range input {
		if _, ok := allowed[key]; ok {
			value = truncateNativeText(humanText(value, ""), 120)
			if value != "" {
				output[key] = value
			}
		}
	}
	return output
}

func nativeAlertSeverity(value string) string {
	if strings.TrimSpace(value) == "P0" {
		return "P0"
	}
	return "P1"
}

func nativeAlertTitle(value string) string {
	return truncateNativeText(humanText(value, "需要关注"), 120)
}

func truncateNativeText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len([]rune(value)) <= max {
		return value
	}
	return string([]rune(value)[:max-1]) + "…"
}

func formatNativeFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
