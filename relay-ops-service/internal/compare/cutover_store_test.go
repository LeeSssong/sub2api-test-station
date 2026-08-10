package compare

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeCutoverAuthorityEnforcesOrderAndPersistsIdempotentRollback(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	reports := NewJSONLReportSetRepository(filepath.Join(t.TempDir(), "report-sets.jsonl"))
	for index, page := range []Page{PageAccountMonitor, PageProfitability, PageAccounting, PageReconciliation} {
		input := completeSetInput(page, now)
		input.SetID = "set-" + string(page)
		input.RunID = "run-" + string(page)
		for comparisonIndex := range input.Comparisons {
			input.Comparisons[comparisonIndex].Legacy.RunID = input.RunID
			input.Comparisons[comparisonIndex].External.RunID = input.RunID
		}
		if _, err := NewReportSetService(reports).CompareAndPersistSet(context.Background(), input); err != nil {
			t.Fatalf("page %d: %v", index, err)
		}
	}
	statePath := filepath.Join(t.TempDir(), "cutover-state.jsonl")
	authority := NewJSONLCutoverAuthority(statePath, reports, func() time.Time { return now }, 10*time.Minute)

	if _, err := authority.SetMode(context.Background(), PageProfitability, ExternalPrimary, 42, "profit-before-monitor", nil); !errors.Is(err, ErrCutoverPredecessor) {
		t.Fatalf("out-of-order error=%v", err)
	}
	monitor, err := authority.SetMode(context.Background(), PageAccountMonitor, ExternalPrimary, 42, "monitor-primary", nil)
	if err != nil || monitor.EffectiveMode != ExternalPrimary || monitor.ReportSetID != "set-monitor" || monitor.ActorID != 42 {
		t.Fatalf("monitor=%#v err=%v", monitor, err)
	}
	replay, err := authority.SetMode(context.Background(), PageAccountMonitor, ExternalPrimary, 42, "monitor-primary", nil)
	if err != nil || replay.AuditID != monitor.AuditID {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
	if _, err := authority.SetMode(context.Background(), PageAccountMonitor, DualReadComparing, 42, "monitor-primary", nil); !errors.Is(err, ErrCutoverIdempotencyConflict) {
		t.Fatalf("conflicting replay error=%v", err)
	}
	for _, page := range []Page{PageProfitability, PageAccounting, PageReconciliation} {
		if _, err := authority.SetMode(context.Background(), page, ExternalPrimary, 42, "promote-"+string(page), nil); err != nil {
			t.Fatalf("promote %s: %v", page, err)
		}
	}
	rollback, err := authority.SetMode(context.Background(), PageAccountMonitor, LegacyOnly, 42, "monitor-rollback", nil)
	if err != nil || rollback.EffectiveMode != LegacyOnly || rollback.Result != "rolled_back" {
		t.Fatalf("rollback=%#v err=%v", rollback, err)
	}

	reloaded := NewJSONLCutoverAuthority(statePath, reports, func() time.Time { return now }, 10*time.Minute)
	decision, err := reloaded.Decision(context.Background(), PageAccountMonitor)
	if err != nil || decision.EffectiveMode != LegacyOnly || decision.UseExternal {
		t.Fatalf("reloaded decision=%#v err=%v", decision, err)
	}
	downstream, err := reloaded.Decision(context.Background(), PageReconciliation)
	if err != nil || downstream.EffectiveMode != LegacyOnly || downstream.UseExternal || !downstream.Degraded {
		t.Fatalf("downstream decision=%#v err=%v", downstream, err)
	}
	records, err := reloaded.AuditRecords(context.Background())
	if err != nil || len(records) != 5 || records[4].IdempotencyKey != "monitor-rollback" {
		t.Fatalf("records=%#v err=%v", records, err)
	}
}

func TestRuntimeCutoverAuthorityRejectsPromotionWithoutValidReportSet(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	authority := NewJSONLCutoverAuthority(filepath.Join(t.TempDir(), "state.jsonl"), NewJSONLReportSetRepository(filepath.Join(t.TempDir(), "sets.jsonl")), func() time.Time { return now }, 10*time.Minute)
	if _, err := authority.SetMode(context.Background(), PageAccountMonitor, ExternalPrimary, 42, "monitor-primary", nil); !errors.Is(err, ErrCutoverEvidence) {
		t.Fatalf("missing evidence error=%v", err)
	}
	records, err := authority.AuditRecords(context.Background())
	if err != nil || len(records) != 0 {
		t.Fatalf("failed promotion persisted records=%#v err=%v", records, err)
	}
}
