package billing

import (
	"context"
	"time"

	"example.invalid/relay-ops-service/internal/domain"
)

// CostTransaction is one upstream billing event. Cost is expressed in USD
// with six decimal places and is positive for charges, negative for refunds.
type CostTransaction struct {
	SourceID          string
	RequestID         string
	UpstreamRequestID string
	Type              string
	Cost              domain.MicroUSD
	OccurredAt        time.Time
	Model             string
	PromptTokens      int64
	CompletionTokens  int64
}

type CostQuery struct {
	Start  *time.Time
	End    *time.Time
	Cursor string
	Limit  int
}

type CostSnapshot struct {
	ActualCost domain.MicroUSD
	ObservedAt time.Time
}

// CostAdapter reads immutable upstream billing evidence.
type CostAdapter interface {
	ListTransactions(context.Context, CostQuery) ([]CostTransaction, string, error)
	ReadSnapshot(context.Context) (CostSnapshot, error)
}
