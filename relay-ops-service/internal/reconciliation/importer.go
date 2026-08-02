package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/sub2api"
	"github.com/shopspring/decimal"
)

// UsageImporter turns Sub2API's durable usage ledger into local reconciliation
// attempts. Importing is idempotent, allowing both the one-minute sweep and an
// administrator refresh to repair a missed run without duplicate entries.
type UsageImporter struct {
	Sources  BillingSourceProvider
	Reader   sub2api.UsageLogReader
	Attempts AttemptRecorder
}

type AttemptRecorder interface {
	RecordUpstreamCostAttempt(context.Context, AttemptInput) (Attempt, bool, error)
}

type ImportResult struct {
	SourcesTotal  int
	SourcesFailed int
	Observed      int
	Inserted      int
}

func (i UsageImporter) Import(ctx context.Context, accountID int64, start, end time.Time) (ImportResult, error) {
	if i.Sources == nil || i.Reader == nil || i.Attempts == nil {
		return ImportResult{}, fmt.Errorf("usage importer dependencies are required")
	}
	if accountID < 0 || !start.Before(end) {
		return ImportResult{}, fmt.Errorf("usage import request is invalid")
	}
	sources, err := i.Sources.ListBillingSources(ctx)
	if err != nil {
		return ImportResult{}, fmt.Errorf("list billing sources for usage import: %w", err)
	}
	var result ImportResult
	var failures []error
	for _, source := range sources {
		if accountID != 0 && source.AccountID != accountID {
			continue
		}
		result.SourcesTotal++
		if err := i.importSource(ctx, source, start, end, &result); err != nil {
			result.SourcesFailed++
			failures = append(failures, fmt.Errorf("import usage for account %d: %w", source.AccountID, err))
		}
	}
	return result, errors.Join(failures...)
}

func (i UsageImporter) importSource(ctx context.Context, source billing.BillingSource, start, end time.Time, result *ImportResult) error {
	adapterType, err := collectionAdapterType(source.AdapterType)
	if err != nil {
		return err
	}
	logs, err := i.Reader.ListUsageLogs(ctx, sub2api.UsageLogQuery{AccountID: source.AccountID, Start: start, End: end})
	if err != nil {
		return err
	}
	for _, log := range logs {
		result.Observed++
		localRequestID := strings.TrimSpace(log.RequestID)
		if localRequestID == "" {
			localRequestID = fmt.Sprintf("sub2api-usage:%d:%d", source.AccountID, log.ID)
		}
		_, inserted, err := i.Attempts.RecordUpstreamCostAttempt(ctx, AttemptInput{
			AttemptID:      fmt.Sprintf("sub2api-usage:%d:%d", source.AccountID, log.ID),
			LocalRequestID: localRequestID,
			AccountID:      source.AccountID,
			AdapterType:    adapterType,
			Model:          log.Model,
			InputTokens:    log.InputTokens,
			OutputTokens:   log.OutputTokens,
			UserCharge:     decimal.NewFromFloat(log.TotalCost),
			Currency:       "USD",
			RequestStatus:  "success",
			CompletedAt:    log.CreatedAt,
		})
		if err != nil {
			return err
		}
		if inserted {
			result.Inserted++
		}
	}
	return nil
}
