package projection

import (
	"context"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/events"
)

func TestProfitabilityProjectionConsumesRequestEvent(t *testing.T) {
	p := NewProfitability()
	event := events.Event{EventID: "req-1", Type: events.RequestCompleted, OccurredAt: time.Now(), SourceVersion: "core", ContractVersion: events.ContractVersion, Payload: []byte(`{"user_charge":"1.00","actual_cost":"0.40"}`)}
	if err := p.Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if p.Revenue != "1.00" || p.Cost != "0.40" {
		t.Fatalf("unexpected projection: %+v", p)
	}
}
