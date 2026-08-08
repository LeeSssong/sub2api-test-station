package projection

import (
	"context"
	"encoding/json"
	"time"

	"example.invalid/relay-ops-service/internal/events"
)

type Metadata struct {
	GeneratedAt        time.Time `json:"generated_at"`
	SourceWatermark    string    `json:"source_watermark"`
	FreshnessSeconds   int64     `json:"freshness_seconds"`
	Completeness       string    `json:"completeness"`
	CalculationVersion string    `json:"calculation_version"`
}
type AccountRow struct {
	AccountID  int64     `json:"account_id"`
	Status     string    `json:"status"`
	Balance    string    `json:"balance,omitempty"`
	Currency   string    `json:"currency,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
	Metadata   Metadata  `json:"metadata"`
}

type Accounts struct {
	Rows     map[int64]AccountRow
	Metadata Metadata
}

func NewAccounts() *Accounts {
	return &Accounts{Rows: map[int64]AccountRow{}, Metadata: Metadata{CalculationVersion: "accounts-v1", Completeness: "empty"}}
}
func (p *Accounts) Handle(ctx context.Context, e events.Event) error {
	_ = ctx
	if p == nil {
		return nil
	}
	switch e.Type {
	case events.AccountHealthChanged:
		var v struct {
			AccountID int64     `json:"account_id"`
			Status    string    `json:"status"`
			CheckedAt time.Time `json:"checked_at"`
		}
		if err := json.Unmarshal(e.Payload, &v); err != nil {
			return err
		}
		row := p.Rows[v.AccountID]
		row.AccountID = v.AccountID
		row.Status = v.Status
		row.ObservedAt = v.CheckedAt
		p.Rows[v.AccountID] = row
	case events.AccountBalanceSnapshot:
		var v struct {
			AccountID  int64     `json:"account_id"`
			Balance    string    `json:"balance"`
			Currency   string    `json:"currency"`
			CapturedAt time.Time `json:"captured_at"`
		}
		if err := json.Unmarshal(e.Payload, &v); err != nil {
			return err
		}
		row := p.Rows[v.AccountID]
		row.AccountID = v.AccountID
		row.Balance = v.Balance
		row.Currency = v.Currency
		row.ObservedAt = v.CapturedAt
		p.Rows[v.AccountID] = row
	}
	p.Metadata.GeneratedAt = time.Now().UTC()
	p.Metadata.SourceWatermark = e.EventID
	p.Metadata.Completeness = "complete"
	return nil
}
func (p *Accounts) Rebuild(ctx context.Context, snapshot []events.Event) error {
	p.Rows = map[int64]AccountRow{}
	for _, event := range snapshot {
		if err := p.Handle(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
