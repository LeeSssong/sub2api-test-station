package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/billing"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type importerSources struct{ items []billing.BillingSource }

func (s importerSources) ListBillingSources(context.Context) ([]billing.BillingSource, error) {
	return s.items, nil
}

type importerReader struct {
	logs map[int64][]sub2api.UsageLog
	errs map[int64]error
}

func (r importerReader) ListUsageLogs(_ context.Context, query sub2api.UsageLogQuery) ([]sub2api.UsageLog, error) {
	if err := r.errs[query.AccountID]; err != nil {
		return nil, err
	}
	return r.logs[query.AccountID], nil
}

type importerAttempts struct {
	items map[string]AttemptInput
}

func (r *importerAttempts) RecordUpstreamCostAttempt(_ context.Context, input AttemptInput) (Attempt, bool, error) {
	if _, exists := r.items[input.AttemptID]; exists {
		return Attempt{}, false, nil
	}
	r.items[input.AttemptID] = input
	return Attempt{AttemptInput: input}, true, nil
}

func TestUsageImporterCreatesIdempotentAttemptsAndContinuesAfterSourceFailure(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	attempts := &importerAttempts{items: make(map[string]AttemptInput)}
	importer := UsageImporter{
		Sources: importerSources{items: []billing.BillingSource{
			{AccountID: 7, AdapterType: "newapi"},
			{AccountID: 8, AdapterType: "sub2api"},
		}},
		Reader: importerReader{
			errs: map[int64]error{7: errors.New("billing unavailable")},
			logs: map[int64][]sub2api.UsageLog{8: {{
				ID: 81, AccountID: 8, RequestID: "request-81", Model: "gpt-5.6-sol",
				InputTokens: 120, OutputTokens: 60, TotalCost: 0.1234, CreatedAt: start.Add(time.Minute),
				GroupID: ptrInt64(3),
			}}},
		},
		Attempts: attempts,
	}

	result, err := importer.Import(context.Background(), 0, start, start.Add(time.Hour))
	if err == nil || result.SourcesTotal != 2 || result.SourcesFailed != 1 || result.Observed != 1 || result.Inserted != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	created, ok := attempts.items["sub2api-usage:8:81"]
	if !ok || created.LocalRequestID != "request-81" || created.AdapterType != AdapterSub2API || created.UserCharge.String() != "0.1234" || created.SiteStandardCost.String() != "0.1234" {
		t.Fatalf("attempt=%#v", created)
	}
	if created.GroupID == nil || *created.GroupID != 3 {
		t.Fatalf("attempt group_id = %v, want 3", created.GroupID)
	}

	result, err = importer.Import(context.Background(), 8, start, start.Add(time.Hour))
	if result.SourcesTotal != 1 || result.Inserted != 0 || err != nil {
		t.Fatalf("repeat result=%#v err=%v", result, err)
	}
}

func TestUsageImporterPreservesNilGroupScope(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	attempts := &importerAttempts{items: make(map[string]AttemptInput)}
	importer := UsageImporter{
		Sources: importerSources{items: []billing.BillingSource{{AccountID: 8, AdapterType: "sub2api"}}},
		Reader: importerReader{logs: map[int64][]sub2api.UsageLog{8: {{
			ID: 82, AccountID: 8, RequestID: "request-82", Model: "gpt-5.6-sol",
			InputTokens: 10, OutputTokens: 5, TotalCost: 0.02, CreatedAt: start.Add(time.Minute),
			GroupID: nil,
		}}}},
		Attempts: attempts,
	}

	result, err := importer.Import(context.Background(), 8, start, start.Add(time.Hour))
	if err != nil || result.Inserted != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	created := attempts.items["sub2api-usage:8:82"]
	if created.GroupID != nil {
		t.Fatalf("attempt group_id = %v, want nil", *created.GroupID)
	}
}

func ptrInt64(value int64) *int64 { return &value }
