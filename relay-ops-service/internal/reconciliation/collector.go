package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/billing"
)

const defaultCollectionAccountTimeout = 15 * time.Second

type CollectionTrigger string

const (
	TriggerPeriodicSweep CollectionTrigger = "periodic_sweep"
	TriggerAdminRefresh  CollectionTrigger = "admin_refresh"
	TriggerDailyClose    CollectionTrigger = "daily_close"
)

type CollectionRequest struct {
	Trigger   CollectionTrigger
	AccountID int64
	Start     time.Time
	End       time.Time
}

type CollectionResult struct {
	AccountsTotal        int
	AccountsSucceeded    int
	AccountsFailed       int
	TransactionsObserved int
	Scanned              int
	Matched              int
	Pending              int
	Conflicts            int
}

// BillingSourceProvider returns source configuration without exposing a
// credential. The adapter factory resolves the secret only during collection.
type BillingSourceProvider interface {
	ListBillingSources(context.Context) ([]billing.BillingSource, error)
}

type SnapshotRepository interface {
	RecordUpstreamCostSnapshot(context.Context, int64, AdapterType, billing.CostSnapshot) error
}

type AdapterFactory func(billing.BillingSource) (billing.CostAdapter, error)

// Collector pulls billing evidence account-by-account. A failed upstream is
// isolated so the remaining accounts can still be reconciled in the same run.
type Collector struct {
	Sources        BillingSourceProvider
	Reconciler     Service
	Snapshots      SnapshotRepository
	AdapterFactory AdapterFactory
	HTTPClient     *http.Client
	Now            func() time.Time
	AccountTimeout time.Duration
}

func (c Collector) Collect(ctx context.Context, request CollectionRequest) (CollectionResult, error) {
	if c.Sources == nil || c.Reconciler.Repository == nil || c.Snapshots == nil {
		return CollectionResult{}, fmt.Errorf("billing collector dependencies are required")
	}
	if request.Trigger != TriggerPeriodicSweep && request.Trigger != TriggerAdminRefresh && request.Trigger != TriggerDailyClose {
		return CollectionResult{}, fmt.Errorf("billing collection trigger is invalid")
	}
	if request.AccountID < 0 || !request.Start.Before(request.End) {
		return CollectionResult{}, fmt.Errorf("billing collection request is invalid")
	}
	sources, err := c.Sources.ListBillingSources(ctx)
	if err != nil {
		return CollectionResult{}, fmt.Errorf("list billing sources: %w", err)
	}
	result := CollectionResult{}
	var failures []error
	for _, source := range sources {
		if request.AccountID != 0 && source.AccountID != request.AccountID {
			continue
		}
		result.AccountsTotal++
		accountResult, err := c.collectAccount(ctx, source, request.Start, request.End)
		result.TransactionsObserved += accountResult.TransactionsObserved
		result.Scanned += accountResult.Scanned
		result.Matched += accountResult.Matched
		result.Pending += accountResult.Pending
		result.Conflicts += accountResult.Conflicts
		if err != nil {
			result.AccountsFailed++
			failures = append(failures, fmt.Errorf("collect billing for account %d: %w", source.AccountID, err))
			continue
		}
		result.AccountsSucceeded++
	}
	return result, errors.Join(failures...)
}

func (c Collector) collectAccount(ctx context.Context, source billing.BillingSource, start, end time.Time) (ReconcileResult, error) {
	adapterType, err := collectionAdapterType(source.AdapterType)
	if err != nil {
		return ReconcileResult{}, err
	}
	accountCtx, cancel := context.WithTimeout(ctx, c.accountTimeout())
	defer cancel()
	factory := c.AdapterFactory
	if factory == nil {
		factory = func(config billing.BillingSource) (billing.CostAdapter, error) {
			return billing.NewBillingAdapter(config, c.HTTPClient)
		}
	}
	adapter, err := factory(source)
	if err != nil {
		return ReconcileResult{}, err
	}
	// Snapshot and request-level matching use independent upstream endpoints.
	// Persist the authoritative cumulative evidence even when a paginated
	// transaction fetch or local match fails in this collection run.
	var snapshotErr error
	snapshot, err := adapter.ReadSnapshot(accountCtx)
	if err != nil {
		snapshotErr = err
	} else {
		if snapshot.ObservedAt.IsZero() {
			snapshot.ObservedAt = c.now()
		}
		snapshotErr = c.Snapshots.RecordUpstreamCostSnapshot(accountCtx, source.AccountID, adapterType, snapshot)
	}
	result, reconcileErr := c.Reconciler.ReconcileAccount(accountCtx, source.AccountID, adapterType, adapter, start, end)
	return result, errors.Join(snapshotErr, reconcileErr)
}

func (c Collector) accountTimeout() time.Duration {
	if c.AccountTimeout > 0 {
		return c.AccountTimeout
	}
	return defaultCollectionAccountTimeout
}

func (c Collector) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

func collectionAdapterType(value string) (AdapterType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(AdapterNewAPI):
		return AdapterNewAPI, nil
	case string(AdapterSub2API):
		return AdapterSub2API, nil
	default:
		return "", fmt.Errorf("billing adapter type is unsupported")
	}
}
