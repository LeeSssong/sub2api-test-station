package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type DeliveryRepository interface {
	ReserveNotification(context.Context, string, string, string) (int64, bool, error)
	FinishNotification(context.Context, int64, string, int) error
}

type MessageClient interface {
	Send(context.Context, FeishuMessage) error
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
	payload, err := deliveryPayload(message)
	if err != nil {
		return fmt.Errorf("encode notification message")
	}
	dedupKey := digest(incidentKey + "\x00" + evidenceHash)
	messageHash := digest(string(payload))
	if s.Repository == nil {
		return s.Client.Send(ctx, message)
	}
	deliveryID, reserved, err := s.Repository.ReserveNotification(ctx, incidentKey, dedupKey, messageHash)
	if err != nil {
		return err
	}
	if !reserved {
		return nil
	}
	if err := s.Client.Send(ctx, message); err != nil {
		_ = s.Repository.FinishNotification(ctx, deliveryID, "failed", 0)
		return err
	}
	if err := s.Repository.FinishNotification(ctx, deliveryID, "delivered", 200); err != nil {
		return err
	}
	return nil
}

func deliveryPayload(message FeishuMessage) ([]byte, error) {
	if message.MsgType == "interactive" {
		return message.CardJSON()
	}
	return json.Marshal(message)
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
