package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.invalid/relay-ops-service/internal/events"
)

type AccountingRepository interface {
	LoadAccountingReadModel(context.Context) (Accounting, bool, error)
	SaveAccountingReadModel(context.Context, Accounting) error
}

type Accounting struct {
	Metadata         Metadata  `json:"metadata"`
	Requests         int64     `json:"requests"`
	Revenue          string    `json:"revenue"`
	Cost             string    `json:"cost"`
	SourceOccurredAt time.Time `json:"-"`

	repository AccountingRepository
	loaded     bool
	seen       map[string]struct{}
}

func NewAccounting() *Accounting {
	return &Accounting{Metadata: emptyMetadata(AccountingCalculationVersion), seen: map[string]struct{}{}, loaded: true}
}

func NewAccountingWithRepository(repository AccountingRepository) *Accounting {
	return &Accounting{Metadata: emptyMetadata(AccountingCalculationVersion), seen: map[string]struct{}{}, repository: repository}
}

func (p *Accounting) Handle(ctx context.Context, event events.Event) error {
	if p == nil {
		return errors.New("accounting projection is nil")
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
	p.Requests++
	p.Revenue, err = addDecimal(p.Revenue, value.UserCharge)
	if err != nil {
		return err
	}
	p.Cost, err = addDecimal(p.Cost, value.ActualCost)
	if err != nil {
		return err
	}
	if events.ComparePosition(event.OccurredAt, event.EventID, p.SourceOccurredAt, p.Metadata.SourceWatermark) > 0 {
		p.SourceOccurredAt = event.OccurredAt.UTC()
		p.Metadata = completeMetadata(event.OccurredAt, event.EventID, AccountingCalculationVersion)
	}
	if p.repository != nil {
		if err := p.repository.SaveAccountingReadModel(ctx, *p); err != nil {
			return fmt.Errorf("persist accounting read model: %w", err)
		}
	}
	return nil
}

func (p *Accounting) handlePersistent(ctx context.Context, event events.Event) error {
	stored, found, err := p.repository.LoadAccountingReadModel(ctx)
	if err != nil {
		return fmt.Errorf("load accounting read model: %w", err)
	}
	working := NewAccounting()
	if found {
		working.Metadata = stored.Metadata
		working.Requests = stored.Requests
		working.Revenue = stored.Revenue
		working.Cost = stored.Cost
		working.SourceOccurredAt = stored.SourceOccurredAt
	}
	if err := working.Handle(ctx, event); err != nil {
		return err
	}
	if err := p.repository.SaveAccountingReadModel(ctx, *working); err != nil {
		return fmt.Errorf("persist accounting read model: %w", err)
	}
	return nil
}

func (p *Accounting) Rebuild(ctx context.Context, snapshot Snapshot, stream []events.Event) error {
	if p == nil {
		return errors.New("accounting projection is nil")
	}
	repository := p.repository
	*p = *NewAccounting()
	p.repository = nil
	if snapshot.Accounting != nil {
		p.Requests = snapshot.Accounting.Requests
		p.Revenue = zeroDecimal(snapshot.Accounting.Revenue)
		p.Cost = zeroDecimal(snapshot.Accounting.Cost)
		p.Metadata = snapshot.Accounting.Metadata
		p.SourceOccurredAt = snapshot.Accounting.SourceOccurredAt
	}
	for _, event := range sortedUniqueEvents(stream) {
		if err := p.Handle(ctx, event); err != nil {
			p.repository = repository
			return err
		}
	}
	p.repository = repository
	if repository != nil {
		return repository.SaveAccountingReadModel(ctx, *p)
	}
	return nil
}

func (p *Accounting) ensureLoaded(ctx context.Context) error {
	if p.loaded {
		return nil
	}
	p.loaded = true
	if p.repository == nil {
		return nil
	}
	stored, found, err := p.repository.LoadAccountingReadModel(ctx)
	if err != nil {
		p.loaded = false
		return err
	}
	if found {
		p.Metadata, p.Requests, p.Revenue, p.Cost, p.SourceOccurredAt = stored.Metadata, stored.Requests, stored.Revenue, stored.Cost, stored.SourceOccurredAt
	}
	return nil
}
