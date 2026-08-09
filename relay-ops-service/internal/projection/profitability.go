package projection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"example.invalid/relay-ops-service/internal/events"
	"github.com/shopspring/decimal"
)

type ProfitabilityRow struct {
	AccountID        int64     `json:"account_id"`
	Requests         int64     `json:"requests"`
	Revenue          string    `json:"revenue"`
	Cost             string    `json:"cost"`
	Profit           string    `json:"profit"`
	Margin           string    `json:"margin"`
	Rank             int       `json:"rank"`
	Metadata         Metadata  `json:"metadata"`
	SourceOccurredAt time.Time `json:"source_occurred_at,omitempty"`
}

type ProfitabilityRepository interface {
	LoadProfitabilityReadModels(context.Context) ([]ProfitabilityRow, error)
	ReplaceProfitabilityReadModels(context.Context, []ProfitabilityRow) error
}

type Profitability struct {
	Metadata Metadata                   `json:"metadata"`
	Requests int64                      `json:"requests"`
	Revenue  string                     `json:"revenue"`
	Cost     string                     `json:"cost"`
	Profit   string                     `json:"profit"`
	Margin   string                     `json:"margin"`
	Rows     map[int64]ProfitabilityRow `json:"rows"`

	repository  ProfitabilityRepository
	loaded      bool
	seen        map[string]struct{}
	watermarkAt time.Time
}

func NewProfitability() *Profitability {
	return &Profitability{Metadata: emptyMetadata(ProfitabilityCalculationVersion), Rows: map[int64]ProfitabilityRow{}, seen: map[string]struct{}{}, loaded: true}
}

func NewProfitabilityWithRepository(repository ProfitabilityRepository) *Profitability {
	return &Profitability{Metadata: emptyMetadata(ProfitabilityCalculationVersion), Rows: map[int64]ProfitabilityRow{}, seen: map[string]struct{}{}, repository: repository}
}

func (p *Profitability) Handle(ctx context.Context, event events.Event) error {
	if p == nil {
		return errors.New("profitability projection is nil")
	}
	if event.Type != events.RequestCompleted {
		return nil
	}
	if p.repository != nil {
		return p.handlePersistent(ctx, event)
	}
	if err := p.ensureLoaded(ctx); err != nil {
		return err
	}
	if _, exists := p.seen[event.EventID]; exists {
		return nil
	}
	value, err := decodeRequestCompleted(event)
	if err != nil {
		return err
	}
	p.seen[event.EventID] = struct{}{}
	row := p.Rows[value.AccountID]
	row.AccountID = value.AccountID
	row.Requests++
	row.Revenue, err = addDecimal(row.Revenue, value.UserCharge)
	if err != nil {
		return err
	}
	row.Cost, err = addDecimal(row.Cost, value.ActualCost)
	if err != nil {
		return err
	}
	p.Rows[value.AccountID] = row
	p.recalculate(event)
	if p.repository != nil {
		if err := p.repository.ReplaceProfitabilityReadModels(ctx, p.profitabilityRows()); err != nil {
			return fmt.Errorf("persist profitability read models: %w", err)
		}
	}
	return nil
}

func (p *Profitability) handlePersistent(ctx context.Context, event events.Event) error {
	rows, err := p.repository.LoadProfitabilityReadModels(ctx)
	if err != nil {
		return fmt.Errorf("load profitability read models: %w", err)
	}
	working := NewProfitability()
	for _, input := range rows {
		row, err := normalizeProfitabilityRow(input)
		if err != nil {
			return err
		}
		working.Rows[row.AccountID] = row
		if events.ComparePosition(row.SourceOccurredAt, row.Metadata.SourceWatermark, working.watermarkAt, working.Metadata.SourceWatermark) > 0 {
			working.watermarkAt, working.Metadata = row.SourceOccurredAt, row.Metadata
		}
	}
	working.recalculateTotals()
	if err := working.Handle(ctx, event); err != nil {
		return err
	}
	if err := p.repository.ReplaceProfitabilityReadModels(ctx, working.profitabilityRows()); err != nil {
		return fmt.Errorf("persist profitability read models: %w", err)
	}
	return nil
}

func (p *Profitability) Rebuild(ctx context.Context, snapshot Snapshot, stream []events.Event) error {
	if p == nil {
		return errors.New("profitability projection is nil")
	}
	p.Rows = map[int64]ProfitabilityRow{}
	p.seen = map[string]struct{}{}
	p.Metadata = emptyMetadata(ProfitabilityCalculationVersion)
	p.watermarkAt = time.Time{}
	p.loaded = true
	for _, input := range snapshot.Profitability {
		row, err := normalizeProfitabilityRow(input)
		if err != nil {
			return fmt.Errorf("snapshot profitability account %d: %w", input.AccountID, err)
		}
		p.Rows[row.AccountID] = row
		if row.SourceOccurredAt.IsZero() {
			row.SourceOccurredAt = row.Metadata.GeneratedAt
			p.Rows[row.AccountID] = row
		}
		if events.ComparePosition(row.SourceOccurredAt, row.Metadata.SourceWatermark, p.watermarkAt, p.Metadata.SourceWatermark) > 0 {
			p.watermarkAt, p.Metadata = row.SourceOccurredAt, row.Metadata
		}
	}
	p.recalculateTotals()
	snapshotAt, snapshotEventID := p.watermarkAt, p.Metadata.SourceWatermark
	repository := p.repository
	p.repository = nil
	for _, event := range sortedUniqueEvents(stream) {
		if !snapshotAt.IsZero() && events.ComparePosition(event.OccurredAt, event.EventID, snapshotAt, snapshotEventID) <= 0 {
			continue
		}
		if err := p.Handle(ctx, event); err != nil {
			p.repository = repository
			return err
		}
	}
	p.repository = repository
	if repository != nil {
		if err := repository.ReplaceProfitabilityReadModels(ctx, p.profitabilityRows()); err != nil {
			return fmt.Errorf("replace profitability read models: %w", err)
		}
	}
	return nil
}

func (p *Profitability) ensureLoaded(ctx context.Context) error {
	if p.loaded {
		return nil
	}
	p.loaded = true
	if p.repository == nil {
		return nil
	}
	rows, err := p.repository.LoadProfitabilityReadModels(ctx)
	if err != nil {
		p.loaded = false
		return fmt.Errorf("load profitability read models: %w", err)
	}
	for _, input := range rows {
		row, err := normalizeProfitabilityRow(input)
		if err != nil {
			return err
		}
		p.Rows[row.AccountID] = row
		if row.SourceOccurredAt.IsZero() {
			row.SourceOccurredAt = row.Metadata.GeneratedAt
			p.Rows[row.AccountID] = row
		}
		if events.ComparePosition(row.SourceOccurredAt, row.Metadata.SourceWatermark, p.watermarkAt, p.Metadata.SourceWatermark) > 0 {
			p.watermarkAt, p.Metadata = row.SourceOccurredAt, row.Metadata
		}
	}
	p.recalculateTotals()
	return nil
}

func (p *Profitability) recalculate(event events.Event) {
	if events.ComparePosition(event.OccurredAt, event.EventID, p.watermarkAt, p.Metadata.SourceWatermark) > 0 {
		p.watermarkAt = event.OccurredAt.UTC()
		p.Metadata = completeMetadata(event.OccurredAt, event.EventID, ProfitabilityCalculationVersion)
	}
	p.recalculateTotals()
}

func (p *Profitability) recalculateTotals() {
	revenue, cost := decimal.Zero, decimal.Zero
	rows := p.profitabilityRows()
	for index := range rows {
		rowRevenue, _ := decimal.NewFromString(zeroDecimal(rows[index].Revenue))
		rowCost, _ := decimal.NewFromString(zeroDecimal(rows[index].Cost))
		profit := rowRevenue.Sub(rowCost)
		rows[index].Profit = profit.String()
		rows[index].Margin = decimalRatio(profit, rowRevenue)
		revenue = revenue.Add(rowRevenue)
		cost = cost.Add(rowCost)
	}
	sort.Slice(rows, func(i, j int) bool {
		left, _ := decimal.NewFromString(zeroDecimal(rows[i].Profit))
		right, _ := decimal.NewFromString(zeroDecimal(rows[j].Profit))
		if !left.Equal(right) {
			return left.GreaterThan(right)
		}
		return rows[i].AccountID < rows[j].AccountID
	})
	p.Requests = 0
	for index := range rows {
		rows[index].Rank = index + 1
		rows[index].Metadata = p.Metadata
		rows[index].SourceOccurredAt = p.watermarkAt
		p.Rows[rows[index].AccountID] = rows[index]
		p.Requests += rows[index].Requests
	}
	p.Revenue = revenue.String()
	p.Cost = cost.String()
	p.Profit = revenue.Sub(cost).String()
	p.Margin = decimalRatio(revenue.Sub(cost), revenue)
}

func (p *Profitability) profitabilityRows() []ProfitabilityRow {
	rows := make([]ProfitabilityRow, 0, len(p.Rows))
	for _, row := range p.Rows {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].AccountID < rows[j].AccountID })
	return rows
}

func normalizeProfitabilityRow(input ProfitabilityRow) (ProfitabilityRow, error) {
	revenue, err := decimal.NewFromString(zeroDecimal(input.Revenue))
	if err != nil {
		return ProfitabilityRow{}, err
	}
	cost, err := decimal.NewFromString(zeroDecimal(input.Cost))
	if err != nil {
		return ProfitabilityRow{}, err
	}
	input.Revenue, input.Cost = revenue.String(), cost.String()
	input.Profit = revenue.Sub(cost).String()
	input.Margin = decimalRatio(revenue.Sub(cost), revenue)
	return input, nil
}

type requestCompletedPayload struct {
	RequestID  string `json:"request_id"`
	AccountID  int64  `json:"account_id"`
	Model      string `json:"model"`
	UserCharge string `json:"user_charge"`
	ActualCost string `json:"actual_cost"`
	CostUSD    string `json:"cost_usd"`
}

func decodeRequestCompleted(event events.Event) (requestCompletedPayload, error) {
	var value requestCompletedPayload
	if err := json.Unmarshal(event.Payload, &value); err != nil {
		return value, err
	}
	if value.RequestID == "" || value.AccountID <= 0 || value.Model == "" {
		return value, errors.New("invalid request.completed payload identity")
	}
	if _, err := decimal.NewFromString(value.UserCharge); err != nil {
		return value, fmt.Errorf("invalid user_charge: %w", err)
	}
	if _, err := decimal.NewFromString(value.ActualCost); err != nil {
		return value, fmt.Errorf("invalid actual_cost: %w", err)
	}
	if value.CostUSD != "" {
		if _, err := decimal.NewFromString(value.CostUSD); err != nil {
			return value, fmt.Errorf("invalid cost_usd: %w", err)
		}
	}
	return value, nil
}

func addDecimal(left, right string) (string, error) {
	l, err := decimal.NewFromString(zeroDecimal(left))
	if err != nil {
		return "", err
	}
	r, err := decimal.NewFromString(zeroDecimal(right))
	if err != nil {
		return "", err
	}
	return l.Add(r).String(), nil
}

func zeroDecimal(value string) string {
	if value == "" {
		return "0"
	}
	return value
}

func decimalRatio(numerator, denominator decimal.Decimal) string {
	if denominator.IsZero() {
		return "0"
	}
	return numerator.DivRound(denominator, 6).String()
}
