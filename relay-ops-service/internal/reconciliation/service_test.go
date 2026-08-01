package reconciliation

import (
	"context"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/domain"
	"github.com/shopspring/decimal"
)

type fakeAdapter struct{ transactions []billing.CostTransaction }

func (f fakeAdapter) ListTransactions(context.Context, billing.CostQuery) ([]billing.CostTransaction, string, error) {
	return f.transactions, "", nil
}
func (f fakeAdapter) ReadSnapshot(context.Context) (billing.CostSnapshot, error) {
	return billing.CostSnapshot{}, nil
}

type fakeRepository struct {
	attempts []Attempt
	matched  []AutomaticTransactionInput
}

func (f *fakeRepository) ListPendingUpstreamCostAttempts(context.Context, int64, time.Time, time.Time, int) ([]Attempt, error) {
	return f.attempts, nil
}
func (f *fakeRepository) CreateAutomaticUpstreamCost(_ context.Context, input AutomaticTransactionInput) (Transaction, bool, error) {
	f.matched = append(f.matched, input)
	return Transaction{}, true, nil
}
func (f *fakeRepository) MarkOverdueUpstreamCostExceptions(context.Context, time.Time, time.Duration) (int64, error) {
	return 0, nil
}

func TestServiceReconcileAccountMatchesUpstreamRequestID(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{attempts: []Attempt{{ID: 11, AttemptInput: AttemptInput{AttemptID: "a", LocalRequestID: "local", UpstreamRequestID: "upstream", AccountID: 7}}}}
	service := Service{Repository: repo}
	result, err := service.ReconcileAccount(context.Background(), 7, AdapterNewAPI, fakeAdapter{transactions: []billing.CostTransaction{{SourceID: "log-1", UpstreamRequestID: "upstream", Type: "charge", Cost: domain.MicroUSD(82000), OccurredAt: now}}}, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("ReconcileAccount: %v", err)
	}
	if result.Matched != 1 || len(repo.matched) != 1 || !repo.matched[0].Amount.Equal(decimal.RequireFromString("0.082")) {
		t.Fatalf("result=%#v matched=%#v", result, repo.matched)
	}
}
