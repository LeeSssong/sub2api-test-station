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

const (
	CompletenessEmpty    = events.CompletenessEmpty
	CompletenessPartial  = events.CompletenessPartial
	CompletenessComplete = events.CompletenessComplete

	AccountsCalculationVersion       = "accounts-v1"
	ProfitabilityCalculationVersion  = "profitability-v1"
	AccountingCalculationVersion     = "accounting-v1"
	ReconciliationCalculationVersion = "reconciliation-v1"
)

type Metadata struct {
	GeneratedAt        time.Time `json:"generated_at"`
	SourceWatermark    string    `json:"source_watermark"`
	FreshnessSeconds   int64     `json:"freshness_seconds"`
	Completeness       string    `json:"completeness"`
	CalculationVersion string    `json:"calculation_version"`
}

type AccountRow struct {
	AccountID         int64     `json:"account_id"`
	Status            string    `json:"status"`
	Balance           string    `json:"balance,omitempty"`
	Currency          string    `json:"currency,omitempty"`
	ObservedAt        time.Time `json:"observed_at,omitempty"`
	HealthOccurredAt  time.Time `json:"-"`
	HealthEventID     string    `json:"-"`
	BalanceOccurredAt time.Time `json:"-"`
	BalanceEventID    string    `json:"-"`
	Metadata          Metadata  `json:"metadata"`
}

type Snapshot struct {
	Accounts       []AccountRow       `json:"accounts,omitempty"`
	Profitability  []ProfitabilityRow `json:"profitability,omitempty"`
	Accounting     *Accounting        `json:"accounting,omitempty"`
	Reconciliation *Reconciliation    `json:"reconciliation,omitempty"`
}

type AccountsRepository interface {
	LoadAccountReadModels(context.Context) ([]AccountRow, error)
	UpsertAccountReadModel(context.Context, AccountRow) error
	ReplaceAccountReadModels(context.Context, []AccountRow) error
}

type Accounts struct {
	Rows     map[int64]AccountRow
	Metadata Metadata

	repository  AccountsRepository
	loaded      bool
	watermarkAt time.Time
}

func NewAccounts() *Accounts {
	return &Accounts{
		Rows:     map[int64]AccountRow{},
		Metadata: emptyMetadata(AccountsCalculationVersion),
		loaded:   true,
	}
}

func NewAccountsWithRepository(repository AccountsRepository) *Accounts {
	return &Accounts{Rows: map[int64]AccountRow{}, Metadata: emptyMetadata(AccountsCalculationVersion), repository: repository}
}

func (p *Accounts) Handle(ctx context.Context, event events.Event) error {
	if p == nil {
		return errors.New("accounts projection is nil")
	}
	if err := p.ensureLoaded(ctx); err != nil {
		return err
	}
	row, changed, err := p.apply(event)
	if err != nil || !changed {
		return err
	}
	if p.repository != nil {
		if err := p.repository.UpsertAccountReadModel(ctx, row); err != nil {
			return fmt.Errorf("persist account read model: %w", err)
		}
	}
	return nil
}

func (p *Accounts) apply(event events.Event) (AccountRow, bool, error) {
	switch event.Type {
	case events.AccountHealthChanged:
		var value struct {
			AccountID int64     `json:"account_id"`
			Status    string    `json:"status"`
			CheckedAt time.Time `json:"checked_at"`
		}
		if err := json.Unmarshal(event.Payload, &value); err != nil {
			return AccountRow{}, false, err
		}
		if value.AccountID <= 0 || value.Status == "" || value.CheckedAt.IsZero() {
			return AccountRow{}, false, errors.New("invalid account.health_changed payload")
		}
		row := p.Rows[value.AccountID]
		if events.ComparePosition(event.OccurredAt, event.EventID, row.HealthOccurredAt, row.HealthEventID) <= 0 {
			return row, false, nil
		}
		row.AccountID = value.AccountID
		row.Status = value.Status
		row.HealthOccurredAt = event.OccurredAt.UTC()
		row.HealthEventID = event.EventID
		p.finishRow(&row, event)
		p.Rows[value.AccountID] = row
		return row, true, nil
	case events.AccountBalanceSnapshot:
		var value struct {
			AccountID  int64     `json:"account_id"`
			Balance    string    `json:"balance"`
			Currency   string    `json:"currency"`
			CapturedAt time.Time `json:"captured_at"`
		}
		if err := json.Unmarshal(event.Payload, &value); err != nil {
			return AccountRow{}, false, err
		}
		balance, err := decimal.NewFromString(value.Balance)
		if err != nil || value.AccountID <= 0 || value.Currency == "" || value.CapturedAt.IsZero() {
			return AccountRow{}, false, errors.New("invalid account.balance_snapshot payload")
		}
		row := p.Rows[value.AccountID]
		if events.ComparePosition(event.OccurredAt, event.EventID, row.BalanceOccurredAt, row.BalanceEventID) <= 0 {
			return row, false, nil
		}
		row.AccountID = value.AccountID
		row.Balance = balance.String()
		row.Currency = value.Currency
		row.BalanceOccurredAt = event.OccurredAt.UTC()
		row.BalanceEventID = event.EventID
		p.finishRow(&row, event)
		p.Rows[value.AccountID] = row
		return row, true, nil
	default:
		return AccountRow{}, false, nil
	}
}

func (p *Accounts) finishRow(row *AccountRow, event events.Event) {
	latestAt, latestID := row.HealthOccurredAt, row.HealthEventID
	if events.ComparePosition(row.BalanceOccurredAt, row.BalanceEventID, latestAt, latestID) > 0 {
		latestAt, latestID = row.BalanceOccurredAt, row.BalanceEventID
	}
	row.ObservedAt = latestAt
	row.Metadata = completeMetadata(latestAt, latestID, AccountsCalculationVersion)
	if events.ComparePosition(latestAt, latestID, p.watermarkAt, p.Metadata.SourceWatermark) > 0 {
		p.watermarkAt = latestAt
		p.Metadata = row.Metadata
	}
}

func (p *Accounts) Rebuild(ctx context.Context, snapshot Snapshot, stream []events.Event) error {
	if p == nil {
		return errors.New("accounts projection is nil")
	}
	p.Rows = map[int64]AccountRow{}
	p.Metadata = emptyMetadata(AccountsCalculationVersion)
	p.watermarkAt = time.Time{}
	p.loaded = true
	for _, input := range snapshot.Accounts {
		row := input
		if row.Balance != "" {
			value, err := decimal.NewFromString(row.Balance)
			if err != nil {
				return fmt.Errorf("snapshot account %d balance: %w", row.AccountID, err)
			}
			row.Balance = value.String()
		}
		if row.HealthOccurredAt.IsZero() {
			row.HealthOccurredAt, row.HealthEventID = row.ObservedAt, row.Metadata.SourceWatermark
		}
		if row.BalanceOccurredAt.IsZero() {
			row.BalanceOccurredAt, row.BalanceEventID = row.ObservedAt, row.Metadata.SourceWatermark
		}
		p.Rows[row.AccountID] = row
		if events.ComparePosition(row.ObservedAt, row.Metadata.SourceWatermark, p.watermarkAt, p.Metadata.SourceWatermark) > 0 {
			p.watermarkAt, p.Metadata = row.ObservedAt, row.Metadata
		}
	}
	repository := p.repository
	p.repository = nil
	for _, event := range sortedUniqueEvents(stream) {
		if err := p.Handle(ctx, event); err != nil {
			p.repository = repository
			return err
		}
	}
	p.repository = repository
	if repository != nil {
		if err := repository.ReplaceAccountReadModels(ctx, p.accountRows()); err != nil {
			return fmt.Errorf("replace account read models: %w", err)
		}
	}
	return nil
}

func (p *Accounts) ensureLoaded(ctx context.Context) error {
	if p.loaded {
		return nil
	}
	p.loaded = true
	if p.repository == nil {
		return nil
	}
	rows, err := p.repository.LoadAccountReadModels(ctx)
	if err != nil {
		p.loaded = false
		return fmt.Errorf("load account read models: %w", err)
	}
	for _, row := range rows {
		p.Rows[row.AccountID] = row
		if events.ComparePosition(row.ObservedAt, row.Metadata.SourceWatermark, p.watermarkAt, p.Metadata.SourceWatermark) > 0 {
			p.watermarkAt, p.Metadata = row.ObservedAt, row.Metadata
		}
	}
	return nil
}

func (p *Accounts) accountRows() []AccountRow {
	rows := make([]AccountRow, 0, len(p.Rows))
	for _, row := range p.Rows {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].AccountID < rows[j].AccountID })
	return rows
}

func sortedUniqueEvents(stream []events.Event) []events.Event {
	ordered := append([]events.Event(nil), stream...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return events.ComparePosition(ordered[i].OccurredAt, ordered[i].EventID, ordered[j].OccurredAt, ordered[j].EventID) < 0
	})
	seen := make(map[string]struct{}, len(ordered))
	result := ordered[:0]
	for _, event := range ordered {
		if _, exists := seen[event.EventID]; exists {
			continue
		}
		seen[event.EventID] = struct{}{}
		result = append(result, event)
	}
	return result
}

func emptyMetadata(version string) Metadata {
	return Metadata{Completeness: CompletenessEmpty, FreshnessSeconds: -1, CalculationVersion: version}
}

func completeMetadata(occurredAt time.Time, eventID, version string) Metadata {
	now := time.Now().UTC()
	freshness := int64(now.Sub(occurredAt).Seconds())
	if freshness < 0 {
		freshness = 0
	}
	return Metadata{
		GeneratedAt: now, SourceWatermark: eventID, FreshnessSeconds: freshness,
		Completeness: CompletenessComplete, CalculationVersion: version,
	}
}
