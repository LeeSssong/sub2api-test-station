package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/notify"
)

type Incident struct {
	Key              string
	Severity         string
	OccurrenceNo     int64
	EscalationLevel  int
	FirstDeliveredAt time.Time
	MessagePayload   []byte
}

type Result struct {
	Key              string
	OccurrenceNo     int64
	Level            int
	Succeeded        bool
	NextEscalationAt *time.Time
	RetryAt          time.Time
}

type Repository interface {
	ClaimDueEscalation(context.Context, time.Time) (*Incident, error)
	FinishEscalation(context.Context, Result) error
}

type Sender interface {
	SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

type Service struct {
	Repository Repository
	Sender     Sender
	Clock      func() time.Time
}

func NextEscalationAt(severity string, completedLevel int, firstDeliveredAt time.Time) (time.Time, bool) {
	if firstDeliveredAt.IsZero() || completedLevel < 0 {
		return time.Time{}, false
	}
	switch severity {
	case "P0":
		switch completedLevel {
		case 0:
			return firstDeliveredAt.Add(5 * time.Minute), true
		case 1:
			return firstDeliveredAt.Add(15 * time.Minute), true
		}
	case "P1":
		if completedLevel == 0 {
			return firstDeliveredAt.Add(15 * time.Minute), true
		}
	}
	return time.Time{}, false
}

func (service Service) Run(ctx context.Context) error {
	if service.Repository == nil || service.Sender == nil {
		return fmt.Errorf("incident escalation dependencies are incomplete")
	}
	now := time.Now().UTC()
	if service.Clock != nil {
		now = service.Clock().UTC()
	}
	seen := map[string]struct{}{}
	var failures []error
	for processed := 0; processed < 100; processed++ {
		incident, err := service.Repository.ClaimDueEscalation(ctx, now)
		if err != nil {
			failures = append(failures, err)
			break
		}
		if incident == nil {
			break
		}
		identity := fmt.Sprintf("%s\x00%d", incident.Key, incident.OccurrenceNo)
		if _, duplicate := seen[identity]; duplicate {
			// ClaimDueEscalation has already moved its lease one minute forward.
			continue
		}
		seen[identity] = struct{}{}
		level := incident.EscalationLevel + 1
		message, renderErr := escalationMessage(*incident, level, now)
		sendErr := renderErr
		if sendErr == nil {
			evidence := fmt.Sprintf("occurrence:%d:escalation:%d", incident.OccurrenceNo, level)
			sendErr = service.Sender.SendIncident(ctx, incident.Key, evidence, message)
		}
		result := Result{
			Key: incident.Key, OccurrenceNo: incident.OccurrenceNo, Level: level,
			Succeeded: sendErr == nil,
		}
		if sendErr == nil {
			if next, found := NextEscalationAt(incident.Severity, level, incident.FirstDeliveredAt); found {
				result.NextEscalationAt = &next
			}
		} else {
			result.RetryAt = now.Add(time.Minute)
		}
		if finishErr := service.Repository.FinishEscalation(ctx, result); finishErr != nil {
			failures = append(failures, finishErr)
		}
		if sendErr != nil {
			failures = append(failures, sendErr)
		}
	}
	return errors.Join(failures...)
}

func escalationMessage(incident Incident, level int, now time.Time) (notify.FeishuMessage, error) {
	if incident.Key == "" || incident.OccurrenceNo <= 0 || incident.EscalationLevel < 0 ||
		(incident.Severity != "P0" && incident.Severity != "P1") ||
		incident.FirstDeliveredAt.IsZero() || len(incident.MessagePayload) == 0 {
		return notify.FeishuMessage{}, fmt.Errorf("incident escalation claim is invalid")
	}
	var card notify.Card
	if err := json.Unmarshal(incident.MessagePayload, &card); err != nil {
		return notify.FeishuMessage{}, fmt.Errorf("decode incident escalation payload")
	}
	title := card.Header.Title.Content
	for strings.HasPrefix(title, "再次提醒｜") {
		title = strings.TrimPrefix(title, "再次提醒｜")
	}
	card.Header.Title.Content = "再次提醒｜" + title
	elements := make([]notify.CardElement, 0, len(card.Elements)+1)
	elements = append(elements, notify.CardElement{
		Tag: "div",
		Text: &notify.CardText{
			Tag:     "lark_md",
			Content: fmt.Sprintf("**持续时间** %s\n\n**提醒级别** 第 %d 次", elapsed(now.Sub(incident.FirstDeliveredAt)), level),
		},
	})
	for _, element := range card.Elements {
		if element.Text != nil && strings.Contains(element.Text.Content, "<at id=") {
			continue
		}
		if element.Text != nil && strings.Contains(element.Text.Content, "**持续时间**") &&
			strings.Contains(element.Text.Content, "**提醒级别**") {
			continue
		}
		elements = append(elements, element)
	}
	card.Elements = elements
	message := notify.FeishuMessage{
		MsgType: "interactive", Card: &card, Severity: incident.Severity,
	}
	message = notify.WithDeliveryIdentity(message, incident.OccurrenceNo, fmt.Sprintf("escalation_%d", level))
	if _, err := message.CardJSON(); err != nil {
		return notify.FeishuMessage{}, err
	}
	return message, nil
}

func elapsed(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	minutes := int(duration.Round(time.Minute) / time.Minute)
	if minutes < 60 {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	hours := minutes / 60
	remainder := minutes % 60
	if remainder == 0 {
		return fmt.Sprintf("%d 小时", hours)
	}
	return fmt.Sprintf("%d 小时 %d 分钟", hours, remainder)
}
