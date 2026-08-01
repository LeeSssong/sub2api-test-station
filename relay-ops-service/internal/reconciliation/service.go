package reconciliation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/billing"
	"github.com/shopspring/decimal"
)

type Repository interface {
	ListPendingUpstreamCostAttempts(context.Context, int64, time.Time, time.Time, int) ([]Attempt, error)
	CreateAutomaticUpstreamCost(context.Context, AutomaticTransactionInput) (Transaction, bool, error)
	MarkOverdueUpstreamCostExceptions(context.Context, time.Time, time.Duration) (int64, error)
}

type Service struct {
	Repository Repository
	Now        func() time.Time
}

type ReconcileResult struct {
	Scanned   int
	Matched   int
	Pending   int
	Conflicts int
}

func (s Service) ReconcileAccount(ctx context.Context, accountID int64, adapterType AdapterType, adapter billing.CostAdapter, start, end time.Time) (ReconcileResult, error) {
	if s.Repository == nil || adapter == nil {
		return ReconcileResult{}, fmt.Errorf("reconciliation repository and adapter are required")
	}
	if accountID <= 0 || !start.Before(end) {
		return ReconcileResult{}, fmt.Errorf("account and time window are required")
	}
	attempts, err := s.Repository.ListPendingUpstreamCostAttempts(ctx, accountID, start, end, 5000)
	if err != nil {
		return ReconcileResult{}, err
	}
	byKey := make(map[string]Attempt, len(attempts)*2)
	for _, attempt := range attempts {
		if attempt.UpstreamRequestID != "" {
			byKey[attempt.UpstreamRequestID] = attempt
		}
		byKey[attempt.LocalRequestID] = attempt
	}
	transactions, _, err := adapter.ListTransactions(ctx, billing.CostQuery{Start: &start, End: &end, Limit: 1000})
	if err != nil {
		return ReconcileResult{}, err
	}
	result := ReconcileResult{Scanned: len(attempts)}
	seen := make(map[int64]struct{})
	for _, transaction := range transactions {
		key := strings.TrimSpace(transaction.UpstreamRequestID)
		if key == "" {
			key = strings.TrimSpace(transaction.RequestID)
		}
		attempt, ok := byKey[key]
		if !ok {
			continue
		}
		if _, duplicate := seen[attempt.ID]; duplicate {
			continue
		}
		seen[attempt.ID] = struct{}{}
		sourceType := SourceAutomaticCharge
		if transaction.Type == "refund" {
			sourceType = SourceAutomaticRefund
		}
		amount := decimal.NewFromInt(int64(transaction.Cost)).Div(decimal.NewFromInt(1000000))
		_, _, err := s.Repository.CreateAutomaticUpstreamCost(ctx, AutomaticTransactionInput{
			AttemptID: attempt.ID, AccountID: accountID, SourceType: sourceType,
			SourceRecordID: transaction.SourceID, Amount: amount, Currency: "USD",
			OccurredAt: transaction.OccurredAt, IdempotencyKey: fmt.Sprintf("%s:%s", adapterType, transaction.SourceID),
		})
		if err != nil {
			return result, err
		}
		result.Matched++
	}
	result.Pending = len(attempts) - result.Matched
	if result.Pending < 0 {
		result.Pending = 0
	}
	return result, nil
}

func (s Service) SweepOverdue(ctx context.Context, grace time.Duration) (int64, error) {
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	return s.Repository.MarkOverdueUpstreamCostExceptions(ctx, now, grace)
}
