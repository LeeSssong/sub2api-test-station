package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type RetryDelivery struct {
	Kind            string
	ID              int64
	IncidentKey     string
	NotificationKey string
	Severity        string
	OccurrenceNo    int64
	Transition      string
	Payload         []byte
}

type DeliveryRetryRepository interface {
	ClaimNotificationRetry(context.Context, time.Time) (*RetryDelivery, error)
	FinishNotification(context.Context, int64, DeliveryOutcome) error
	FinishOneShot(context.Context, int64, DeliveryOutcome) error
}

type DeliveryRetryService struct {
	Repository DeliveryRetryRepository
	Client     MessageClient
	Clock      func() time.Time
}

func (service DeliveryRetryService) Run(ctx context.Context) error {
	if service.Repository == nil || service.Client == nil {
		return fmt.Errorf("notification retry dependencies are incomplete")
	}
	now := time.Now().UTC()
	if service.Clock != nil {
		now = service.Clock().UTC()
	}
	var failures []error
	for processed := 0; processed < 100; processed++ {
		delivery, err := service.Repository.ClaimNotificationRetry(ctx, now)
		if err != nil {
			failures = append(failures, err)
			break
		}
		if delivery == nil {
			break
		}
		message, err := retryMessage(*delivery)
		result := SendResult{ResponseCode: http.StatusOK, UrgentStatus: "not_supported"}
		if err == nil {
			if client, ok := service.Client.(ResultMessageClient); ok {
				result, err = client.SendWithResult(ctx, message)
			} else {
				err = service.Client.Send(ctx, message)
			}
		}
		outcome := DeliveryOutcome{Status: "failed", Payload: append([]byte(nil), delivery.Payload...)}
		if err == nil {
			outcome.Status = "delivered"
			outcome.MessageID = result.MessageID
			outcome.ResponseCode = result.ResponseCode
			outcome.UrgentStatus = result.UrgentStatus
			outcome.UrgentResponseCode = result.UrgentResponseCode
		}
		if finishErr := service.finish(ctx, *delivery, outcome); finishErr != nil {
			failures = append(failures, finishErr)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (service DeliveryRetryService) finish(
	ctx context.Context,
	delivery RetryDelivery,
	outcome DeliveryOutcome,
) error {
	switch delivery.Kind {
	case "incident":
		return service.Repository.FinishNotification(ctx, delivery.ID, outcome)
	case "one_shot":
		return service.Repository.FinishOneShot(ctx, delivery.ID, outcome)
	default:
		return fmt.Errorf("notification retry kind is invalid")
	}
}

func retryMessage(delivery RetryDelivery) (FeishuMessage, error) {
	if delivery.ID <= 0 ||
		(delivery.Severity != "P0" && delivery.Severity != "P1" && delivery.Severity != "P2") ||
		len(delivery.Payload) == 0 {
		return FeishuMessage{}, fmt.Errorf("notification retry claim is invalid")
	}
	switch delivery.Kind {
	case "incident":
		if delivery.IncidentKey == "" || delivery.NotificationKey != "" ||
			delivery.OccurrenceNo <= 0 || delivery.Transition == "" {
			return FeishuMessage{}, fmt.Errorf("notification retry claim is invalid")
		}
	case "one_shot":
		if delivery.NotificationKey == "" || delivery.IncidentKey != "" ||
			delivery.OccurrenceNo != 0 || delivery.Transition != "" ||
			delivery.Severity != "P2" {
			return FeishuMessage{}, fmt.Errorf("notification retry claim is invalid")
		}
	default:
		return FeishuMessage{}, fmt.Errorf("notification retry claim is invalid")
	}
	var card Card
	if err := json.Unmarshal(delivery.Payload, &card); err != nil {
		return FeishuMessage{}, fmt.Errorf("decode notification retry payload")
	}
	normalizeRetryCard(&card)
	message := FeishuMessage{
		MsgType: "interactive", Card: &card, Severity: delivery.Severity,
	}
	if delivery.Kind == "incident" {
		message.OccurrenceNo = delivery.OccurrenceNo
		message.Transition = delivery.Transition
	}
	if _, err := message.CardJSON(); err != nil {
		return FeishuMessage{}, err
	}
	return message, nil
}

func normalizeRetryCard(card *Card) {
	if card == nil {
		return
	}
	elements := make([]CardElement, 0, len(card.Elements)+1)
	for _, element := range card.Elements {
		if element.Text != nil {
			element.Text.Content = normalizeRetryText(element.Text.Content)
		}
		for index := range element.Fields {
			element.Fields[index].Text.Content = normalizeRetryText(element.Fields[index].Text.Content)
		}
		element.Content = normalizeRetryText(element.Content)
		element.Actions = nil
		if element.Tag != "action" {
			elements = append(elements, element)
		}
	}
	card.Elements = append(elements, operationsAction())
}

func normalizeRetryText(value string) string {
	legacyStatusLine := "**接手" + "状态**：尚未有人确认" + "接手。"
	return strings.ReplaceAll(
		value,
		legacyStatusLine,
		"**提醒状态**：该异常仍在持续。",
	)
}
