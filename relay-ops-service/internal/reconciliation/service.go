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
	ListPendingUpstreamCostAttempts(context.Context, int64, time.Time, time.Time, AttemptCursor, int) ([]Attempt, error)
	BindUpstreamRequestID(context.Context, int64, string) error
	CreateAutomaticUpstreamCost(context.Context, AutomaticTransactionInput) (Transaction, bool, error)
	MarkOverdueUpstreamCostExceptions(context.Context, time.Time, time.Duration) (int64, error)
}

type AttemptCursor struct {
	CompletedAt time.Time
	ID          int64
}

type Service struct {
	Repository Repository
	Now        func() time.Time
}

type ReconcileResult struct {
	Scanned              int
	TransactionsObserved int
	Matched              int
	Pending              int
	Conflicts            int
}

func (s Service) ReconcileAccount(ctx context.Context, accountID int64, adapterType AdapterType, adapter billing.CostAdapter, start, end time.Time) (ReconcileResult, error) {
	if s.Repository == nil || adapter == nil {
		return ReconcileResult{}, fmt.Errorf("reconciliation repository and adapter are required")
	}
	if accountID <= 0 || !start.Before(end) {
		return ReconcileResult{}, fmt.Errorf("account and time window are required")
	}
	const pageSize = 1000
	attempts := make([]Attempt, 0, pageSize)
	attemptCursor := AttemptCursor{}
	for {
		page, err := s.Repository.ListPendingUpstreamCostAttempts(ctx, accountID, start, end, attemptCursor, pageSize)
		if err != nil {
			return ReconcileResult{}, err
		}
		attempts = append(attempts, page...)
		if len(page) < pageSize {
			break
		}
		last := page[len(page)-1]
		if last.ID <= 0 || last.CompletedAt.IsZero() ||
			(last.CompletedAt.Equal(attemptCursor.CompletedAt) && last.ID <= attemptCursor.ID) || last.CompletedAt.Before(attemptCursor.CompletedAt) {
			return ReconcileResult{}, fmt.Errorf("local attempt cursor did not advance")
		}
		attemptCursor = AttemptCursor{CompletedAt: last.CompletedAt, ID: last.ID}
	}
	byUpstreamRequestID := make(map[string]Attempt, len(attempts))
	byLocalRequestID := make(map[string]Attempt, len(attempts))
	for _, attempt := range attempts {
		if attempt.UpstreamRequestID != "" {
			byUpstreamRequestID[attempt.UpstreamRequestID] = attempt
		}
		byLocalRequestID[attempt.LocalRequestID] = attempt
	}
	result := ReconcileResult{Scanned: len(attempts)}
	seen := make(map[string]struct{})
	cursor := ""
	for {
		transactions, nextCursor, err := adapter.ListTransactions(ctx, billing.CostQuery{Start: &start, End: &end, Cursor: cursor, Limit: 1000})
		if err != nil {
			return result, err
		}
		for _, transaction := range transactions {
			result.TransactionsObserved++
			providerRequestID := strings.TrimSpace(transaction.UpstreamRequestID)
			attempt, ok := byUpstreamRequestID[providerRequestID]
			if !ok {
				attempt, ok = byLocalRequestID[strings.TrimSpace(transaction.RequestID)]
			}
			if !ok {
				continue
			}
			if providerRequestID != "" && attempt.UpstreamRequestID == "" {
				if err := s.Repository.BindUpstreamRequestID(ctx, attempt.ID, providerRequestID); err != nil {
					return result, err
				}
				attempt.UpstreamRequestID = providerRequestID
				byUpstreamRequestID[providerRequestID] = attempt
			}
			transactionKey := transaction.SourceID + ":" + transaction.Type
			if _, duplicate := seen[transactionKey]; duplicate {
				continue
			}
			seen[transactionKey] = struct{}{}
			sourceType := SourceAutomaticCharge
			if transaction.Type == "refund" {
				sourceType = SourceAutomaticRefund
			}
			amount := decimal.NewFromInt(int64(transaction.Cost)).Div(decimal.NewFromInt(1000000))
			_, _, err := s.Repository.CreateAutomaticUpstreamCost(ctx, AutomaticTransactionInput{
				AttemptID: attempt.ID, AccountID: accountID, SourceType: sourceType,
				SourceRecordID: transaction.SourceID, Amount: amount, Currency: "USD",
				OccurredAt: transaction.OccurredAt,
				// Upstream log IDs are normally scoped to the provider account.
				// Include that identity so the same record number from two accounts
				// cannot close the wrong local attempt.
				IdempotencyKey: fmt.Sprintf("%s:%d:%s:%s", adapterType, accountID, transaction.Type, transaction.SourceID),
			})
			if err != nil {
				return result, err
			}
			result.Matched++
		}
		if nextCursor == "" {
			break
		}
		if nextCursor == cursor {
			return result, fmt.Errorf("upstream billing cursor did not advance")
		}
		if _, repeated := seen[nextCursor]; repeated {
			return result, fmt.Errorf("upstream billing cursor repeated")
		}
		seen[nextCursor] = struct{}{}
		cursor = nextCursor
	}
	for _, attempt := range attempts {
		if attempt.ReconcileStatus == StatusPending || attempt.ReconcileStatus == StatusException {
			result.Pending++
		}
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
