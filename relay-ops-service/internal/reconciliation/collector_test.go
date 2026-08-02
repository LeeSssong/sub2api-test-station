package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/domain"
)

type collectorSourceProvider struct{ sources []billing.BillingSource }

func (p collectorSourceProvider) ListBillingSources(context.Context) ([]billing.BillingSource, error) {
	return p.sources, nil
}

type collectorAdapter struct {
	transactions []billing.CostTransaction
	snapshot     billing.CostSnapshot
	listErr      error
	snapshotErr  error
}

func (a collectorAdapter) ListTransactions(context.Context, billing.CostQuery) ([]billing.CostTransaction, string, error) {
	if a.listErr != nil {
		return nil, "", a.listErr
	}
	return a.transactions, "", nil
}

func (a collectorAdapter) ReadSnapshot(context.Context) (billing.CostSnapshot, error) {
	return a.snapshot, a.snapshotErr
}

type collectorSnapshotStore struct {
	accountIDs []int64
}

func (s *collectorSnapshotStore) RecordUpstreamCostSnapshot(_ context.Context, accountID int64, _ AdapterType, _ billing.CostSnapshot) error {
	s.accountIDs = append(s.accountIDs, accountID)
	return nil
}

func TestCollectorContinuesAfterOneAccountFails(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	repository := &fakeRepository{attempts: []Attempt{{
		ID:           11,
		AttemptInput: AttemptInput{AttemptID: "attempt-8", LocalRequestID: "local-8", UpstreamRequestID: "upstream-8", AccountID: 8},
	}}}
	snapshots := &collectorSnapshotStore{}
	collector := Collector{
		Sources: collectorSourceProvider{sources: []billing.BillingSource{
			{AccountID: 7, AdapterType: "newapi", BaseURL: "https://failed.example", SecretRef: "file:/run/secrets/upstream-sessions/failed"},
			{AccountID: 8, AdapterType: "sub2api", BaseURL: "https://healthy.example", SecretRef: "file:/run/secrets/upstream-sessions/healthy"},
		}},
		Reconciler: Service{Repository: repository},
		Snapshots:  snapshots,
		AdapterFactory: func(source billing.BillingSource) (billing.CostAdapter, error) {
			if source.AccountID == 7 {
				return collectorAdapter{listErr: errors.New("upstream unavailable"), snapshotErr: errors.New("snapshot unavailable")}, nil
			}
			return collectorAdapter{
				transactions: []billing.CostTransaction{{SourceID: "usage-8", UpstreamRequestID: "upstream-8", Type: "charge", Cost: domain.MicroUSD(82_100), OccurredAt: now}},
				snapshot:     billing.CostSnapshot{ActualCost: domain.MicroUSD(1_000_000), ObservedAt: now},
			}, nil
		},
		Now: func() time.Time { return now },
	}

	result, err := collector.Collect(context.Background(), CollectionRequest{
		Trigger: TriggerPeriodicSweep, Start: now.Add(-time.Hour), End: now.Add(time.Hour),
	})
	if err == nil {
		t.Fatal("Collect succeeded despite one account failure")
	}
	if result.AccountsTotal != 2 || result.AccountsSucceeded != 1 || result.AccountsFailed != 1 || result.TransactionsObserved != 1 || result.Matched != 1 {
		t.Fatalf("result=%#v", result)
	}
	if len(repository.matched) != 1 || len(snapshots.accountIDs) != 1 || snapshots.accountIDs[0] != 8 {
		t.Fatalf("matched=%#v snapshots=%#v", repository.matched, snapshots.accountIDs)
	}
}

func TestCollectorRecordsSnapshotWhenTransactionReconciliationFails(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	snapshots := &collectorSnapshotStore{}
	collector := Collector{
		Sources: collectorSourceProvider{sources: []billing.BillingSource{{
			AccountID: 8, AdapterType: "sub2api", BaseURL: "https://source.example", SecretRef: "file:/run/secrets/upstream-sessions/source",
		}}},
		Reconciler: Service{Repository: &fakeRepository{}},
		Snapshots:  snapshots,
		AdapterFactory: func(billing.BillingSource) (billing.CostAdapter, error) {
			return collectorAdapter{
				listErr:  errors.New("transactions unavailable"),
				snapshot: billing.CostSnapshot{ActualCost: domain.MicroUSD(1_250_000), ObservedAt: now},
			}, nil
		},
	}

	result, err := collector.Collect(context.Background(), CollectionRequest{
		Trigger: TriggerPeriodicSweep, Start: now.Add(-time.Hour), End: now.Add(time.Hour),
	})
	if err == nil || result.AccountsFailed != 1 || result.AccountsSucceeded != 0 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if len(snapshots.accountIDs) != 1 || snapshots.accountIDs[0] != 8 {
		t.Fatalf("snapshots=%#v", snapshots.accountIDs)
	}
}
