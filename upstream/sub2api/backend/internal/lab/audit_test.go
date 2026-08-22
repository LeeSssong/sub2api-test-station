package lab

import (
	"testing"
	"time"
)

func TestAuditEventRequiresLabOnlyAndRedactsSecrets(t *testing.T) {
	evt, err := NewAuditEvent("seed", map[string]any{"token": "secret", "request_id": "req-lab-1"})
	if err != nil {
		t.Fatal(err)
	}
	if evt.Source != "LAB_ONLY" {
		t.Fatalf("source=%q", evt.Source)
	}
	if evt.Payload["token"] != "[REDACTED]" {
		t.Fatalf("token=%v", evt.Payload["token"])
	}
	if evt.Payload["request_id"] != "req-lab-1" {
		t.Fatalf("request_id=%v", evt.Payload["request_id"])
	}
	if evt.OccurredAt.IsZero() {
		t.Fatal("missing timestamp")
	}
}

func TestAuditEventRejectsEmptyAction(t *testing.T) {
	if _, err := NewAuditEvent("", nil); err == nil {
		t.Fatal("expected error")
	}
}

var _ = time.Time{}
