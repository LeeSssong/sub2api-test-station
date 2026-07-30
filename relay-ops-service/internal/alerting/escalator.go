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
	ClaimToken       string
	FirstDeliveredAt time.Time
	CurrentValue     string
}

type Result struct {
	Key              string
	OccurrenceNo     int64
	Level            int
	ClaimToken       string
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
		if completedLevel == 0 {
			return firstDeliveredAt.Add(5 * time.Minute), true
		}
		if completedLevel == 1 {
			return firstDeliveredAt.Add(15 * time.Minute), true
		}
		return firstDeliveredAt.Add(15*time.Minute + time.Duration(completedLevel-1)*30*time.Minute), true
	case "P1":
		if completedLevel == 0 {
			return firstDeliveredAt.Add(15 * time.Minute), true
		}
		return firstDeliveredAt.Add(15*time.Minute + time.Duration(completedLevel)*60*time.Minute), true
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
			ClaimToken: incident.ClaimToken, Succeeded: sendErr == nil,
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
		incident.ClaimToken == "" ||
		(incident.Severity != "P0" && incident.Severity != "P1") ||
		incident.FirstDeliveredAt.IsZero() || strings.TrimSpace(incident.CurrentValue) == "" {
		return notify.FeishuMessage{}, fmt.Errorf("incident escalation claim is invalid")
	}
	var snapshot struct {
		GroupName  string    `json:"group_name"`
		Headline   string    `json:"headline"`
		LatestFact string    `json:"latest_fact"`
		Capacity   string    `json:"capacity"`
		ObservedAt time.Time `json:"observed_at"`
	}
	if err := json.Unmarshal([]byte(incident.CurrentValue), &snapshot); err != nil {
		return notify.FeishuMessage{}, fmt.Errorf("decode incident reminder snapshot")
	}
	if strings.TrimSpace(snapshot.GroupName) == "" ||
		strings.TrimSpace(snapshot.Headline) == "" ||
		strings.TrimSpace(snapshot.LatestFact) == "" {
		return notify.FeishuMessage{}, fmt.Errorf("decode incident reminder snapshot")
	}
	message := notify.RenderUserImpactReminder(notify.UserImpactReminderView{
		GroupName:  snapshot.GroupName,
		Severity:   incident.Severity,
		Headline:   snapshot.Headline,
		Duration:   elapsed(now.Sub(incident.FirstDeliveredAt)),
		LatestFact: snapshot.LatestFact,
		Capacity:   snapshot.Capacity,
	})
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
