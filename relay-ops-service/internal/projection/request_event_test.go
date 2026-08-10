package projection

import (
	"context"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/events"
	"github.com/shopspring/decimal"
)

func TestProfitabilityProjectionConsumesRequestEvent(t *testing.T) {
	p := NewProfitability()
	event := events.Event{EventID: "550e8400-e29b-41d4-a716-446655440001", Type: events.RequestCompleted, OccurredAt: time.Now(), SourceVersion: "core", ContractVersion: events.ContractVersion, Payload: []byte(`{"request_id":"req-1","account_id":7,"model":"gpt-test","prompt_tokens":1,"completion_tokens":1,"user_charge":"1.00","actual_cost":"0.40","currency":"USD"}`)}
	if err := p.Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	revenue, revenueErr := decimal.NewFromString(p.Revenue)
	cost, costErr := decimal.NewFromString(p.Cost)
	if revenueErr != nil || costErr != nil || !revenue.Equal(decimal.RequireFromString("1.00")) || !cost.Equal(decimal.RequireFromString("0.40")) {
		t.Fatalf("unexpected projection: %+v", p)
	}
}
