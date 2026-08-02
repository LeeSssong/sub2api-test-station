package reconciliation

import (
	"context"
	"strconv"
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
	attempts    []Attempt
	matched     []AutomaticTransactionInput
	cursorCalls []AttemptCursor
}

func (f *fakeRepository) ListPendingUpstreamCostAttempts(_ context.Context, _ int64, _ time.Time, _ time.Time, after AttemptCursor, limit int) ([]Attempt, error) {
	f.cursorCalls = append(f.cursorCalls, after)
	start := 0
	if after.ID != 0 {
		for start < len(f.attempts) && (f.attempts[start].CompletedAt.Before(after.CompletedAt) ||
			(f.attempts[start].CompletedAt.Equal(after.CompletedAt) && f.attempts[start].ID <= after.ID)) {
			start++
		}
	}
	end := start + limit
	if end > len(f.attempts) {
		end = len(f.attempts)
	}
	return f.attempts[start:end], nil
}

func TestServiceReconcileAccountReadsAllAttemptPages(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	attempts := make([]Attempt, 1001)
	for index := range attempts {
		id := int64(index + 1)
		attempts[index] = Attempt{ID: id, AttemptInput: AttemptInput{
			AttemptID:         "attempt-" + strconv.FormatInt(id, 10),
			LocalRequestID:    "request-" + strconv.FormatInt(id, 10),
			UpstreamRequestID: "upstream-" + strconv.FormatInt(id, 10),
			AccountID:         7,
			CompletedAt:       now.Add(time.Duration(index) * time.Microsecond),
		}}
	}
	repo := &fakeRepository{attempts: attempts}
	service := Service{Repository: repo}
	_, err := service.ReconcileAccount(context.Background(), 7, AdapterSub2API, fakeAdapter{transactions: []billing.CostTransaction{{
		SourceID: "usage-final", UpstreamRequestID: "upstream-1001", Type: "charge", Cost: domain.MicroUSD(1), OccurredAt: now,
	}}}, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.cursorCalls) != 2 || repo.cursorCalls[1].ID != 1000 || len(repo.matched) != 1 || repo.matched[0].AttemptID != 1001 {
		t.Fatalf("cursor calls=%#v matched=%#v", repo.cursorCalls, repo.matched)
	}
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

func TestServiceReconcileAccountFallsBackToRequestIDWhenUpstreamRequestIDDoesNotMatch(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{attempts: []Attempt{
		{ID: 11, AttemptInput: AttemptInput{
			AttemptID: "a", LocalRequestID: "usage-request-id", AccountID: 7,
		}},
		{ID: 12, AttemptInput: AttemptInput{
			AttemptID: "b", LocalRequestID: "other-local-request-id", UpstreamRequestID: "usage-request-id", AccountID: 7,
		}},
	}}
	service := Service{Repository: repo}

	result, err := service.ReconcileAccount(context.Background(), 7, AdapterNewAPI, fakeAdapter{transactions: []billing.CostTransaction{{
		SourceID: "log-1", UpstreamRequestID: "unmatched-upstream-id", RequestID: "usage-request-id",
		Type: "charge", Cost: domain.MicroUSD(82000), OccurredAt: now,
	}}}, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("ReconcileAccount: %v", err)
	}
	if result.Matched != 1 || len(repo.matched) != 1 || repo.matched[0].AttemptID != 11 {
		t.Fatalf("result=%#v matched=%#v", result, repo.matched)
	}
}
