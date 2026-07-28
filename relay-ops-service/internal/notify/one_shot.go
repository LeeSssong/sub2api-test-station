package notify

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type OneShotIdentity struct {
	Key           string
	Family        string
	PolicyVersion int
	SourceKind    string
}

type OneShotReservation struct {
	NotificationKey string
	Family          string
	PolicyVersion   int
	SourceKind      string
	DedupKey        string
	MessageHash     string
	Payload         []byte
}

type OneShotRepository interface {
	ReserveOneShot(context.Context, OneShotReservation) (int64, bool, error)
	FinishOneShot(context.Context, int64, DeliveryOutcome) error
}

type OneShotSender struct {
	Client     MessageClient
	Repository OneShotRepository
}

func (sender OneShotSender) SendOneShot(
	ctx context.Context,
	identity OneShotIdentity,
	message FeishuMessage,
) error {
	identity.Key = strings.TrimSpace(identity.Key)
	identity.Family = strings.TrimSpace(identity.Family)
	identity.SourceKind = strings.TrimSpace(identity.SourceKind)
	if identity.Key == "" || identity.Family == "" || identity.PolicyVersion <= 0 || identity.SourceKind == "" {
		return fmt.Errorf("one-shot notification identity is incomplete")
	}
	if sender.Client == nil || sender.Repository == nil {
		return fmt.Errorf("one-shot notification dependencies are incomplete")
	}

	// A one-shot event has no incident occurrence, acknowledgement, escalation,
	// or recovery lifecycle. Strip any identity accidentally inherited from a
	// reused renderer before persisting and sending the card.
	message.OccurrenceNo = 0
	message.Transition = ""
	payload, err := message.CardJSON()
	if err != nil {
		return fmt.Errorf("encode one-shot notification message")
	}
	reservation := OneShotReservation{
		NotificationKey: identity.Key,
		Family:          identity.Family,
		PolicyVersion:   identity.PolicyVersion,
		SourceKind:      identity.SourceKind,
		DedupKey:        digest(identity.Family + "\x00" + identity.SourceKind + "\x00" + identity.Key),
		MessageHash:     digest(string(payload)),
		Payload:         payload,
	}
	deliveryID, reserved, err := sender.Repository.ReserveOneShot(ctx, reservation)
	if err != nil {
		return err
	}
	if !reserved {
		return nil
	}

	result := SendResult{
		ResponseCode: http.StatusOK,
		Payload:      payload,
		UrgentStatus: "not_supported",
	}
	if client, ok := sender.Client.(ResultMessageClient); ok {
		result, err = client.SendWithResult(ctx, message)
	} else {
		err = sender.Client.Send(ctx, message)
	}
	if err != nil {
		_ = sender.Repository.FinishOneShot(ctx, deliveryID, DeliveryOutcome{
			Status: "failed", Payload: payload,
		})
		return err
	}
	return sender.Repository.FinishOneShot(ctx, deliveryID, DeliveryOutcome{
		Status:             "delivered",
		MessageID:          result.MessageID,
		ResponseCode:       result.ResponseCode,
		Payload:            payload,
		UrgentStatus:       result.UrgentStatus,
		UrgentResponseCode: result.UrgentResponseCode,
	})
}
