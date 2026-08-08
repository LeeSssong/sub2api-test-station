package projection

import (
	"context"
	"encoding/json"
	"time"

	"example.invalid/relay-ops-service/internal/events"
)

type Profitability struct {
	Metadata Metadata `json:"metadata"`
	Revenue  string   `json:"revenue"`
	Cost     string   `json:"cost"`
	Profit   string   `json:"profit"`
	Margin   string   `json:"margin"`
}

func NewProfitability() Profitability {
	return Profitability{Metadata: Metadata{CalculationVersion: "profitability-v1", Completeness: "empty", GeneratedAt: time.Time{}}}
}

func (p *Profitability) Handle(ctx context.Context, e events.Event) error {
	_ = ctx
	if e.Type != events.RequestCompleted {
		return nil
	}
	var value struct {
		UserCharge string `json:"user_charge"`
		ActualCost string `json:"actual_cost"`
		CostUSD    string `json:"cost_usd"`
	}
	if err := json.Unmarshal(e.Payload, &value); err != nil {
		return err
	}
	p.Revenue = value.UserCharge
	if value.CostUSD != "" {
		p.Cost = value.CostUSD
	} else {
		p.Cost = value.ActualCost
	}
	p.Metadata.GeneratedAt = time.Now().UTC()
	p.Metadata.SourceWatermark = e.EventID
	p.Metadata.Completeness = "complete"
	return nil
}
