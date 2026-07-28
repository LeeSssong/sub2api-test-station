package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type DeliveryRepository interface {
	ReserveNotification(context.Context, Reservation) (int64, bool, error)
	FinishNotification(context.Context, int64, DeliveryOutcome) error
}

type MessageClient interface {
	Send(context.Context, FeishuMessage) error
}

type ResultMessageClient interface {
	SendWithResult(context.Context, FeishuMessage) (SendResult, error)
}

type Reservation struct {
	IncidentKey  string
	DedupKey     string
	MessageHash  string
	Transition   string
	OccurrenceNo int64
}

type DeliveryOutcome struct {
	Status             string
	MessageID          string
	ResponseCode       int
	Payload            []byte
	UrgentStatus       string
	UrgentResponseCode int
}

type DeliverySender struct {
	Client     MessageClient
	Repository DeliveryRepository
}

func (s DeliverySender) SendIncident(ctx context.Context, incidentKey, evidenceHash string, message FeishuMessage) error {
	if incidentKey == "" {
		return fmt.Errorf("incident key is required")
	}
	if s.Client == nil {
		return fmt.Errorf("notification client is required")
	}
	// The dedup fingerprint is taken over the card that will actually be sent,
	// so a message whose rendering changed counts as a different message.
	payload, err := message.CardJSON()
	if err != nil {
		return fmt.Errorf("encode notification message")
	}
	occurrenceNo := message.OccurrenceNo
	if occurrenceNo <= 0 {
		occurrenceNo = 1
	}
	transition := message.Transition
	if transition == "" {
		transition = "confirmed"
	}
	dedupKey := digest(fmt.Sprintf("%s\x00%d\x00%s\x00%s", incidentKey, occurrenceNo, transition, evidenceHash))
	messageHash := digest(string(payload))
	if s.Repository == nil {
		if client, ok := s.Client.(ResultMessageClient); ok {
			_, err := client.SendWithResult(ctx, message)
			return err
		}
		return s.Client.Send(ctx, message)
	}
	deliveryID, reserved, err := s.Repository.ReserveNotification(ctx, Reservation{
		IncidentKey: incidentKey, DedupKey: dedupKey, MessageHash: messageHash,
		Transition: transition, OccurrenceNo: occurrenceNo,
	})
	if err != nil {
		return err
	}
	if !reserved {
		return nil
	}
	result := SendResult{ResponseCode: 200, Payload: payload, UrgentStatus: "not_supported"}
	if client, ok := s.Client.(ResultMessageClient); ok {
		result, err = client.SendWithResult(ctx, message)
	} else {
		err = s.Client.Send(ctx, message)
	}
	if err != nil {
		_ = s.Repository.FinishNotification(ctx, deliveryID, DeliveryOutcome{Status: "failed"})
		return err
	}
	outcome := DeliveryOutcome{
		Status: "delivered", MessageID: result.MessageID, ResponseCode: result.ResponseCode,
		Payload: result.Payload, UrgentStatus: result.UrgentStatus,
		UrgentResponseCode: result.UrgentResponseCode,
	}
	if err := s.Repository.FinishNotification(ctx, deliveryID, outcome); err != nil {
		return err
	}
	return nil
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
