package controlplane

import (
	"context"
	"sync"
	"time"

	"example.invalid/relay-ops-service/internal/projection"
)

type MemoryReader struct {
	mu     sync.RWMutex
	Models map[string]ReadModel
}

func NewMemoryReader() *MemoryReader { return &MemoryReader{Models: map[string]ReadModel{}} }
func (r *MemoryReader) Read(_ context.Context, name string, _ map[string]string) (ReadModel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.Models[name]
	if !ok {
		return ReadModel{Items: []any{}, Freshness: Freshness{Completeness: "empty"}}, nil
	}
	return m, nil
}
func (r *MemoryReader) Set(name string, m ReadModel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Models[name] = m
}

// ProjectionStoreReader is deliberately limited to relay-owned read-model
// loaders. It cannot access core accounts, groups, usage, or billing tables.
type ProjectionStoreReader interface {
	LoadAccountReadModels(context.Context) ([]projection.AccountRow, error)
	LoadProfitabilityReadModels(context.Context) ([]projection.ProfitabilityRow, error)
	LoadAccountingReadModel(context.Context) (projection.Accounting, bool, error)
	LoadReconciliationReadModel(context.Context) (projection.Reconciliation, bool, error)
}

type StoreReader struct {
	Store ProjectionStoreReader
	Now   func() time.Time
}

func (r StoreReader) Read(ctx context.Context, name string, _ map[string]string) (ReadModel, error) {
	if r.Store == nil {
		return ReadModel{}, context.Canceled
	}
	switch name {
	case "accounts/monitor":
		rows, err := r.Store.LoadAccountReadModels(ctx)
		if err != nil {
			return ReadModel{}, err
		}
		return rowsModel(rows, func(row projection.AccountRow) projection.Metadata { return row.Metadata }), nil
	case "operations/profitability":
		rows, err := r.Store.LoadProfitabilityReadModels(ctx)
		if err != nil {
			return ReadModel{}, err
		}
		return rowsModel(rows, func(row projection.ProfitabilityRow) projection.Metadata { return row.Metadata }), nil
	case "accounting/ledger":
		row, found, err := r.Store.LoadAccountingReadModel(ctx)
		if err != nil {
			return ReadModel{}, err
		}
		if !found {
			return ReadModel{Items: []any{}, Freshness: Freshness{Completeness: "empty", CalculationVersion: "accounting-v1"}}, nil
		}
		return ReadModel{Items: row, Total: 1, Freshness: fromMetadata(row.Metadata)}, nil
	case "reconciliation":
		row, found, err := r.Store.LoadReconciliationReadModel(ctx)
		if err != nil {
			return ReadModel{}, err
		}
		if !found {
			return ReadModel{Items: []any{}, Freshness: Freshness{Completeness: "empty", CalculationVersion: "reconciliation-v1"}}, nil
		}
		return ReadModel{Items: row, Total: 1, Freshness: fromMetadata(row.Metadata)}, nil
	default:
		return ReadModel{}, context.Canceled
	}
}

func rowsModel[T any](rows []T, metadata func(T) projection.Metadata) ReadModel {
	model := ReadModel{Items: rows, Total: len(rows)}
	for _, row := range rows {
		value := metadata(row)
		if value.GeneratedAt.After(model.Freshness.GeneratedAt) {
			model.Freshness = fromMetadata(value)
		}
	}
	if model.Freshness.CalculationVersion == "" && len(rows) == 0 {
		model.Freshness.Completeness = "empty"
	}
	return model
}

func fromMetadata(value projection.Metadata) Freshness {
	return Freshness{GeneratedAt: value.GeneratedAt, SourceWatermark: value.SourceWatermark, FreshnessSeconds: value.FreshnessSeconds, Completeness: value.Completeness, CalculationVersion: value.CalculationVersion}
}
