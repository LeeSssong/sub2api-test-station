package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type auditRecorder struct{ entries []*AuditLog }

func (r *auditRecorder) Record(entry *AuditLog) { r.entries = append(r.entries, entry) }

func TestAccountFinancialAuditRedactsAndRecordsMutationContext(t *testing.T) {
	recorder := &auditRecorder{}
	audit := NewAccountFinancialAudit(recorder)
	audit.Record(context.Background(), AccountFinancialAuditEvent{
		Action: "admin.account_financial.review", ActorUserID: 9, RequestID: "req-1", AccountID: 2,
		BusinessDate: "2026-08-13", OldValue: nil, NewValue: floatPtr(3), Cutoff: 88,
		Result:    map[string]int64{"matched": 4, "updated": 3, "skipped": 1},
		Sensitive: map[string]string{"api_key": "secret", "upstream_error": "raw"},
	})
	if len(recorder.entries) != 1 {
		t.Fatalf("entries=%d", len(recorder.entries))
	}
	entry := recorder.entries[0]
	if entry.ActorUserID == nil || *entry.ActorUserID != 9 || entry.RequestID != "req-1" || entry.Action != "admin.account_financial.review" {
		t.Fatalf("audit identity missing: %#v", entry)
	}
	if entry.RequestBody != "" {
		t.Fatalf("financial audit must not retain request body: %q", entry.RequestBody)
	}
	if _, ok := entry.Extra["sensitive"]; ok {
		t.Fatalf("sensitive fields leaked: %#v", entry.Extra)
	}
	if entry.CreatedAt.IsZero() || entry.StatusCode != 200 {
		t.Fatalf("audit result incomplete: %#v", entry)
	}
}

func TestAccountFinancialAuditUsesFixedCreatedAt(t *testing.T) {
	when := time.Date(2026, 8, 13, 8, 0, 0, 0, time.UTC)
	recorder := &auditRecorder{}
	NewAccountFinancialAuditWithClock(recorder, func() time.Time { return when }).Record(context.Background(), AccountFinancialAuditEvent{Action: "test"})
	if !recorder.entries[0].CreatedAt.Equal(when) {
		t.Fatalf("created_at=%v", recorder.entries[0].CreatedAt)
	}
}

func TestAccountFinancialAuditNilRecorderFailsClosed(t *testing.T) {
	if audit := NewAccountFinancialAudit(nil); audit != nil {
		t.Fatalf("nil recorder must not produce a usable audit dependency: %#v", audit)
	}
	if audit := NewAccountFinancialAuditWithClock(nil, time.Now); audit != nil {
		t.Fatalf("nil recorder must fail closed: %#v", audit)
	}
}

func TestAccountFinancialServiceReviewSelectedAuditsCommittedRowsBeforeLaterError(t *testing.T) {
	ctx := context.Background()
	now := beijingTime(t, "2026-08-13 12:00")
	rec := &auditRecorder{}
	repo := &partialReviewFinancialRepoStub{failUsageLogID: 2}
	svc := NewAccountFinancialServiceWithAudit(repo, func() time.Time { return now }, NewAccountFinancialAuditWithClock(rec, func() time.Time { return now }))
	_, err := svc.ReviewSelected(ctx, []UsageCostReviewInput{{UsageLogID: 1, ReviewedBy: 9, RequestID: "selected-1"}, {UsageLogID: 2, ReviewedBy: 9, RequestID: "selected-2"}})
	if err == nil || len(rec.entries) != 2 {
		t.Fatalf("partial batch must audit committed row and failure row: err=%v audits=%d", err, len(rec.entries))
	}
	if rec.entries[0].Extra["updated"] != int64(1) || rec.entries[0].Extra["failed"] != nil {
		t.Fatalf("first committed row audit missing: %#v", rec.entries[0])
	}
}

func TestAccountFinancialServiceOverrideAuditPersistsMutationKind(t *testing.T) {
	ctx := context.Background()
	now := beijingTime(t, "2026-08-13 12:00")
	rec := &auditRecorder{}
	svc := NewAccountFinancialServiceWithAudit(&overrideKindFinancialRepoStub{}, func() time.Time { return now }, NewAccountFinancialAuditWithClock(rec, func() time.Time { return now }))
	cost := 4.0
	_, err := svc.SetTodayOverride(ctx, TodayOverrideInput{AccountID: 5, BusinessDate: "2026-08-13", CostCNY: &cost, ActorUserID: 9, RequestID: "override-kind"})
	if err != nil || len(rec.entries) != 1 {
		t.Fatalf("override audit missing: err=%v audits=%d", err, len(rec.entries))
	}
	if rec.entries[0].Extra["mutation_kind"] != "cost" {
		t.Fatalf("audit must persist mutation kind: %#v", rec.entries[0].Extra)
	}
}

func TestAccountFinancialServiceAuditsEveryMutation(t *testing.T) {
	ctx := context.Background()
	now := beijingTime(t, "2026-08-13 12:00")
	repo := &mutationFinancialRepoStub{}
	rec := &auditRecorder{}
	svc := NewAccountFinancialServiceWithAudit(repo, func() time.Time { return now }, NewAccountFinancialAuditWithClock(rec, func() time.Time { return now }))
	cost := float64(3)
	_, _ = svc.ReviewOne(ctx, UsageCostReviewInput{UsageLogID: 1, ManualCostCNY: &cost, ReviewedBy: 9, RequestID: "one"})
	_, _ = svc.ReviewSelected(ctx, []UsageCostReviewInput{{UsageLogID: 2, ReviewedBy: 9, RequestID: "selected"}})
	_, _ = svc.ReviewFiltered(ctx, ReviewFilteredInput{MaxUsageLogID: 3, ReviewedBy: 9, RequestID: "filtered"})
	_, _ = svc.SetOAuthDailyCost(ctx, OAuthDailyCostInput{AccountID: 4, BusinessDate: "2026-08-13", CostCNY: &cost, ActorUserID: 9, RequestID: "oauth"})
	_, _ = svc.SetTodayOverride(ctx, TodayOverrideInput{AccountID: 5, BusinessDate: "2026-08-13", CostCNY: &cost, ActorUserID: 9, RequestID: "override"})
	if len(rec.entries) != 5 {
		t.Fatalf("audit entries=%d", len(rec.entries))
	}
	for _, e := range rec.entries {
		if e.RequestID == "" || e.ActorUserID == nil || e.RequestBody != "" {
			t.Fatalf("bad audit=%#v", e)
		}
	}
}

func TestAccountFinancialServiceReviewAuditUsesTruthfulOldAndNewValues(t *testing.T) {
	ctx := context.Background()
	now := beijingTime(t, "2026-08-13 12:00")
	rec := &auditRecorder{}
	svc := NewAccountFinancialServiceWithAudit(&truthfulReviewFinancialRepoStub{}, func() time.Time { return now }, NewAccountFinancialAuditWithClock(rec, func() time.Time { return now }))
	_, err := svc.ReviewOne(ctx, UsageCostReviewInput{UsageLogID: 1, ReviewedBy: 9, RequestID: "repeat"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.entries) != 1 || rec.entries[0].Extra["old_value"] != float64(3) || rec.entries[0].Extra["new_value"] != float64(3) || rec.entries[0].Extra["skipped"] != int64(1) {
		t.Fatalf("audit=%#v", rec.entries)
	}
}

func TestAccountFinancialServiceAuditsValidationFailures(t *testing.T) {
	ctx := context.Background()
	now := beijingTime(t, "2026-08-13 12:00")
	rec := &auditRecorder{}
	svc := NewAccountFinancialServiceWithAudit(&mutationFinancialRepoStub{}, func() time.Time { return now }, NewAccountFinancialAuditWithClock(rec, func() time.Time { return now }))
	invalid := -1.0
	valid := 1.0
	_, _ = svc.ReviewOne(ctx, UsageCostReviewInput{UsageLogID: 1, ManualCostCNY: &invalid, ReviewedBy: 9, RequestID: "one-invalid"})
	_, _ = svc.ReviewSelected(ctx, []UsageCostReviewInput{{UsageLogID: 2, ManualCostCNY: &invalid, ReviewedBy: 9, RequestID: "selected-invalid"}})
	_, _ = svc.ReviewFiltered(ctx, ReviewFilteredInput{ManualCostCNY: &invalid, ReviewedBy: 9, RequestID: "filtered-invalid"})
	_, _ = svc.SetOAuthDailyCost(ctx, OAuthDailyCostInput{AccountID: 4, BusinessDate: "2026-08-13", CostCNY: &invalid, ActorUserID: 9, RequestID: "oauth-invalid"})
	_, _ = svc.SetTodayOverride(ctx, TodayOverrideInput{AccountID: 5, BusinessDate: "2026-08-13", RevenueCNY: &valid, CostCNY: &valid, ActorUserID: 9, RequestID: "override-invalid"})
	if len(rec.entries) != 5 {
		t.Fatalf("audit entries=%d", len(rec.entries))
	}
	for _, e := range rec.entries {
		if e.Extra["failed"] != int64(1) || e.RequestID == "" {
			t.Fatalf("bad failed audit=%#v", e)
		}
	}
}

type mutationFinancialRepoStub struct{ financialRepoStub }

type truthfulReviewFinancialRepoStub struct{ mutationFinancialRepoStub }

type partialReviewFinancialRepoStub struct {
	mutationFinancialRepoStub
	failUsageLogID int64
}

func (r *partialReviewFinancialRepoStub) CreateReview(_ context.Context, in UsageCostReviewInput) (*UsageCostReviewResult, error) {
	if in.UsageLogID == r.failUsageLogID {
		return nil, errors.New("injected review failure")
	}
	return &UsageCostReviewResult{Created: true, UsageLogID: in.UsageLogID, AccountID: 11, ManualCostCNY: 0, ManualProfitCNY: 1, BusinessDate: "2026-08-13"}, nil
}

type overrideKindFinancialRepoStub struct{ mutationFinancialRepoStub }

func (r *overrideKindFinancialRepoStub) SetTodayOverride(_ context.Context, in TodayOverrideInput) (*FinancialMutationResult, error) {
	return &FinancialMutationResult{AccountID: in.AccountID, BusinessDate: in.BusinessDate, NewValue: in.CostCNY, MutationKind: "cost"}, nil
}

func (r *truthfulReviewFinancialRepoStub) CreateReview(_ context.Context, in UsageCostReviewInput) (*UsageCostReviewResult, error) {
	old := 3.0
	return &UsageCostReviewResult{UsageLogID: in.UsageLogID, AccountID: 11, ManualCostCNY: 3, ManualProfitCNY: 1, OldManualCostCNY: &old, BusinessDate: "2026-08-13"}, nil
}

func (r *mutationFinancialRepoStub) CreateReview(_ context.Context, in UsageCostReviewInput) (*UsageCostReviewResult, error) {
	return &UsageCostReviewResult{Created: true, UsageLogID: in.UsageLogID, ManualCostCNY: 0, ManualProfitCNY: 1, AccountID: 11}, nil
}
func (r *mutationFinancialRepoStub) ReviewFiltered(_ context.Context, in ReviewFilteredInput) (*ReviewFilteredResult, error) {
	return &ReviewFilteredResult{Cutoff: in.MaxUsageLogID, MaxUsageLogID: in.MaxUsageLogID, Matched: 1, Updated: 1, Reviews: []UsageCostReviewResult{{Created: true, UsageLogID: 3, AccountID: 11, BusinessDate: "2026-08-13"}}}, nil
}
func (r *mutationFinancialRepoStub) SetOAuthDailyCost(_ context.Context, in OAuthDailyCostInput) (*FinancialMutationResult, error) {
	return &FinancialMutationResult{AccountID: in.AccountID, BusinessDate: in.BusinessDate, NewValue: in.CostCNY, MutationKind: "oauth_cost"}, nil
}
func (r *mutationFinancialRepoStub) SetTodayOverride(_ context.Context, in TodayOverrideInput) (*FinancialMutationResult, error) {
	return &FinancialMutationResult{AccountID: in.AccountID, BusinessDate: in.BusinessDate, NewValue: in.CostCNY, MutationKind: "cost"}, nil
}
