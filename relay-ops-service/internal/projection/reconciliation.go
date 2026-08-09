package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.invalid/relay-ops-service/internal/events"
	"github.com/shopspring/decimal"
)

type ReconciliationRepository interface {
	LoadReconciliationReadModel(context.Context) (Reconciliation, bool, error)
	SaveReconciliationReadModel(context.Context, Reconciliation) error
}

type Reconciliation struct {
	Metadata         Metadata  `json:"metadata"`
	Matched          int64     `json:"matched"`
	Exceptions       int64     `json:"exceptions"`
	SourceOccurredAt time.Time `json:"-"`

	repository ReconciliationRepository
	loaded     bool
	seen       map[string]struct{}
}

func NewReconciliation() *Reconciliation {
	return &Reconciliation{Metadata: emptyMetadata(ReconciliationCalculationVersion), seen: map[string]struct{}{}, loaded: true}
}

func NewReconciliationWithRepository(repository ReconciliationRepository) *Reconciliation {
	return &Reconciliation{Metadata: emptyMetadata(ReconciliationCalculationVersion), seen: map[string]struct{}{}, repository: repository}
}

func (p *Reconciliation) Handle(ctx context.Context, event events.Event) error {
	if p == nil {
		return errors.New("reconciliation projection is nil")
	}
	if event.Type != events.RequestCompleted {
		return nil
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
	actual, _ := decimal.NewFromString(value.ActualCost)
	if value.CostUSD == "" {
		p.Exceptions++
	} else {
		evidence, _ := decimal.NewFromString(value.CostUSD)
		if actual.Equal(evidence) {
			p.Matched++
		} else {
			p.Exceptions++
		}
	}
	if events.ComparePosition(event.OccurredAt, event.EventID, p.SourceOccurredAt, p.Metadata.SourceWatermark) > 0 {
		p.SourceOccurredAt = event.OccurredAt.UTC()
		p.Metadata = completeMetadata(event.OccurredAt, event.EventID, ReconciliationCalculationVersion)
	}
	if p.repository != nil {
		if err := p.repository.SaveReconciliationReadModel(ctx, *p); err != nil {
			return fmt.Errorf("persist reconciliation read model: %w", err)
		}
	}
	return nil
}

func (p *Reconciliation) Rebuild(ctx context.Context, snapshot Snapshot, stream []events.Event) error {
	if p == nil {
		return errors.New("reconciliation projection is nil")
	}
	repository := p.repository
	*p = *NewReconciliation()
	p.repository = nil
	if snapshot.Reconciliation != nil {
		p.Metadata = snapshot.Reconciliation.Metadata
		p.Matched = snapshot.Reconciliation.Matched
		p.Exceptions = snapshot.Reconciliation.Exceptions
		p.SourceOccurredAt = snapshot.Reconciliation.SourceOccurredAt
	}
	for _, event := range sortedUniqueEvents(stream) {
		if err := p.Handle(ctx, event); err != nil {
			p.repository = repository
			return err
		}
	}
	p.repository = repository
	if repository != nil {
		return repository.SaveReconciliationReadModel(ctx, *p)
	}
	return nil
}

func (p *Reconciliation) ensureLoaded(ctx context.Context) error {
	if p.loaded {
		return nil
	}
	p.loaded = true
	if p.repository == nil {
		return nil
	}
	stored, found, err := p.repository.LoadReconciliationReadModel(ctx)
	if err != nil {
		p.loaded = false
		return err
	}
	if found {
		p.Metadata, p.Matched, p.Exceptions, p.SourceOccurredAt = stored.Metadata, stored.Matched, stored.Exceptions, stored.SourceOccurredAt
	}
	return nil
}
