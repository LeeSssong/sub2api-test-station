package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type RetryDelivery struct {
	ID           int64
	IncidentKey  string
	Severity     string
	OccurrenceNo int64
	Transition   string
	Payload      []byte
}

type DeliveryRetryRepository interface {
	ClaimNotificationRetry(context.Context, time.Time) (*RetryDelivery, error)
	FinishNotification(context.Context, int64, DeliveryOutcome) error
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
		if finishErr := service.Repository.FinishNotification(ctx, delivery.ID, outcome); finishErr != nil {
			failures = append(failures, finishErr)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func retryMessage(delivery RetryDelivery) (FeishuMessage, error) {
	if delivery.ID <= 0 || delivery.IncidentKey == "" || delivery.OccurrenceNo <= 0 ||
		delivery.Transition == "" || (delivery.Severity != "P0" && delivery.Severity != "P1" && delivery.Severity != "P2") ||
		len(delivery.Payload) == 0 {
		return FeishuMessage{}, fmt.Errorf("notification retry claim is invalid")
	}
	var card Card
	if err := json.Unmarshal(delivery.Payload, &card); err != nil {
		return FeishuMessage{}, fmt.Errorf("decode notification retry payload")
	}
	message := FeishuMessage{
		MsgType: "interactive", Card: &card, Severity: delivery.Severity,
		OccurrenceNo: delivery.OccurrenceNo, Transition: delivery.Transition,
	}
	if _, err := message.CardJSON(); err != nil {
		return FeishuMessage{}, err
	}
	return message, nil
}
