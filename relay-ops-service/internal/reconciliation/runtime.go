package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// CollectionRunner permits the runtime API and scheduler to share the same
// collection path without coupling either caller to a concrete collector.
type CollectionRunner interface {
	Collect(context.Context, CollectionRequest) (CollectionResult, error)
}

// UsageImportRunner imports durable local usage records before upstream costs
// are fetched, so every ledger row has a reconciliation attempt to match.
type UsageImportRunner interface {
	Import(context.Context, int64, time.Time, time.Time) (ImportResult, error)
}

type RuntimeRepository interface {
	ReadReconciliationSummary(context.Context, int64, time.Time, time.Time, string) (Summary, error)
	ReadOperationsSummary(context.Context, OperationsScope) (OperationsSummary, error)
	ListOperationsDaily(context.Context, OperationsScope) ([]OperationsDailyRow, error)
	ListUpstreamCostExceptions(context.Context, int64, int) ([]Exception, error)
	CreateManualUpstreamCostForException(context.Context, int64, ManualAdjustmentInput) (Transaction, bool, error)
	MarkOverdueUpstreamCostExceptions(context.Context, time.Time, time.Duration) (int64, error)
}

// RuntimeService is the control-plane facade. Refreshes always collect first,
// then read the persisted state so an operator sees the newest completed work.
type RuntimeService struct {
	Repository RuntimeRepository
	Importer   UsageImportRunner
	Collector  CollectionRunner
	Now        func() time.Time
	Grace      time.Duration
}

func (s RuntimeService) ReadReconciliationSummary(ctx context.Context, accountID int64, start, end time.Time, currency string) (Summary, error) {
	if s.Repository == nil {
		return Summary{}, fmt.Errorf("reconciliation repository is required")
	}
	return s.Repository.ReadReconciliationSummary(ctx, accountID, start, end, currency)
}

func (s RuntimeService) ReadOperationsSummary(ctx context.Context, scope OperationsScope) (OperationsSummary, error) {
	if s.Repository == nil {
		return OperationsSummary{}, fmt.Errorf("reconciliation repository is required")
	}
	return s.Repository.ReadOperationsSummary(ctx, scope)
}

func (s RuntimeService) ListOperationsDaily(ctx context.Context, scope OperationsScope) ([]OperationsDailyRow, error) {
	if s.Repository == nil {
		return nil, fmt.Errorf("reconciliation repository is required")
	}
	return s.Repository.ListOperationsDaily(ctx, scope)
}

func (s RuntimeService) ListUpstreamCostExceptions(ctx context.Context, accountID int64, limit int) ([]Exception, error) {
	if s.Repository == nil {
		return nil, fmt.Errorf("reconciliation repository is required")
	}
	return s.Repository.ListUpstreamCostExceptions(ctx, accountID, limit)
}

func (s RuntimeService) CreateManualUpstreamCostForException(ctx context.Context, exceptionID int64, input ManualAdjustmentInput) (Transaction, bool, error) {
	if s.Repository == nil {
		return Transaction{}, false, fmt.Errorf("reconciliation repository is required")
	}
	return s.Repository.CreateManualUpstreamCostForException(ctx, exceptionID, input)
}

func (s RuntimeService) RefreshReconciliation(ctx context.Context, accountID int64, start, end time.Time, currency string) (Summary, error) {
	if s.Repository == nil || s.Collector == nil {
		return Summary{}, fmt.Errorf("reconciliation runtime dependencies are required")
	}
	var importErr error
	if s.Importer != nil {
		_, importErr = s.Importer.Import(ctx, accountID, start, end)
	}
	_, collectionErr := s.Collector.Collect(ctx, CollectionRequest{
		Trigger: TriggerAdminRefresh, AccountID: accountID, Start: start, End: end,
	})
	if _, err := s.Repository.MarkOverdueUpstreamCostExceptions(ctx, s.now(), s.grace()); err != nil {
		return Summary{}, err
	}
	summary, err := s.Repository.ReadReconciliationSummary(ctx, accountID, start, end, currency)
	if err != nil {
		return Summary{}, err
	}
	summary.CollectionPartial = importErr != nil || collectionErr != nil
	return summary, nil
}

func (s RuntimeService) Sweep(ctx context.Context, start, end time.Time) (CollectionResult, error) {
	if s.Repository == nil || s.Collector == nil {
		return CollectionResult{}, fmt.Errorf("reconciliation runtime dependencies are required")
	}
	var importErr error
	if s.Importer != nil {
		_, importErr = s.Importer.Import(ctx, 0, start, end)
	}
	result, collectionErr := s.Collector.Collect(ctx, CollectionRequest{
		Trigger: TriggerPeriodicSweep, Start: start, End: end,
	})
	_, overdueErr := s.Repository.MarkOverdueUpstreamCostExceptions(ctx, s.now(), s.grace())
	if importErr != nil || collectionErr != nil {
		return result, errors.Join(importErr, collectionErr)
	}
	return result, overdueErr
}

// DailyClose uses the same source and matching logic as an operator refresh,
// with an explicit trigger so daily accounting can be audited separately.
func (s RuntimeService) DailyClose(ctx context.Context, start, end time.Time) (CollectionResult, error) {
	if s.Repository == nil || s.Collector == nil {
		return CollectionResult{}, fmt.Errorf("reconciliation runtime dependencies are required")
	}
	var importErr error
	if s.Importer != nil {
		_, importErr = s.Importer.Import(ctx, 0, start, end)
	}
	result, collectionErr := s.Collector.Collect(ctx, CollectionRequest{
		Trigger: TriggerDailyClose, Start: start, End: end,
	})
	_, overdueErr := s.Repository.MarkOverdueUpstreamCostExceptions(ctx, s.now(), s.grace())
	if importErr != nil || collectionErr != nil {
		return result, errors.Join(importErr, collectionErr)
	}
	return result, overdueErr
}

func (s RuntimeService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s RuntimeService) grace() time.Duration {
	if s.Grace > 0 {
		return s.Grace
	}
	return 10 * time.Minute
}
