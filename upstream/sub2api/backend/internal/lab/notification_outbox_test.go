package lab

import (
	"context"
	"testing"
)

func TestNotificationOutboxFailsClosedOutsideLab(t *testing.T) {
	if _, err := NewNotificationOutbox("0"); err == nil {
		t.Fatal("expected non-lab notification outbox construction to fail")
	}
}

func TestNotificationOutboxRecordsRedactedLabOnlyEvents(t *testing.T) {
	outbox, err := NewNotificationOutbox("1")
	if err != nil {
		t.Fatal(err)
	}

	event, err := outbox.Enqueue(context.Background(), NotificationEvent{
		Type: "payment.succeeded",
		Payload: map[string]any{
			"order_id": "lab_pay_success_001",
			"token":    "must-not-escape",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Source != "LAB_ONLY" || event.Sequence != 1 {
		t.Fatalf("unexpected lab event: %#v", event)
	}
	if event.Payload["token"] != "[REDACTED]" {
		t.Fatalf("secret escaped event outbox: %#v", event.Payload)
	}

	events := outbox.Events()
	if len(events) != 1 || events[0].Type != "payment.succeeded" {
		t.Fatalf("unexpected events: %#v", events)
	}
	events[0].Payload["order_id"] = "mutated"
	if outbox.Events()[0].Payload["order_id"] != "lab_pay_success_001" {
		t.Fatal("outbox leaked mutable event payload")
	}
}

func TestNotificationOutboxRejectsEmptyEventType(t *testing.T) {
	outbox, err := NewNotificationOutbox("1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Enqueue(context.Background(), NotificationEvent{}); err == nil {
		t.Fatal("expected empty notification type rejection")
	}
}
