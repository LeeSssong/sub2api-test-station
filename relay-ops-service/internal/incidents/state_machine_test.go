package incidents

import (
	"context"
	"testing"
)

func TestMachineConfirmsDeduplicatesEscalatesAndRecovers(t *testing.T) {
	t.Parallel()
	repository := newMemoryRepository()
	machine := Machine{Repository: repository, Policy: DefaultPolicy()}
	ctx := context.Background()
	key := "upstream:neko:multiplier"

	first, _ := machine.Observe(ctx, Observation{Key: key, Severity: "P1", Failing: true, EvidenceHash: "a", CurrentValue: "0.10"})
	if first.Notify || first.State != "observed" {
		t.Fatalf("first=%#v", first)
	}
	confirmed, _ := machine.Observe(ctx, Observation{Key: key, Severity: "P1", Failing: true, EvidenceHash: "a", CurrentValue: "0.10"})
	if !confirmed.Notify || confirmed.Kind != "confirmed" {
		t.Fatalf("confirmed=%#v", confirmed)
	}
	duplicate, _ := machine.Observe(ctx, Observation{Key: key, Severity: "P1", Failing: true, EvidenceHash: "a", CurrentValue: "0.10"})
	if duplicate.Notify {
		t.Fatalf("duplicate=%#v", duplicate)
	}
	newEvidence, _ := machine.Observe(ctx, Observation{Key: key, Severity: "P1", Failing: true, EvidenceHash: "b", CurrentValue: "0.11"})
	if newEvidence.Notify || newEvidence.Kind != "" {
		t.Fatalf("new evidence=%#v", newEvidence)
	}
	escalated, _ := machine.Observe(ctx, Observation{Key: key, Severity: "P0", Failing: true, EvidenceHash: "c", CurrentValue: "unavailable"})
	if !escalated.Notify || escalated.Kind != "escalated" {
		t.Fatalf("escalated=%#v", escalated)
	}
	recovered, _ := machine.Observe(ctx, Observation{Key: key, Severity: "P0", Failing: false, EvidenceHash: "recovered", CurrentValue: "healthy"})
	if !recovered.Notify || recovered.Kind != "recovered" || recovered.RelatedKey != key {
		t.Fatalf("recovered=%#v", recovered)
	}
}

func TestEvidenceChangeUpdatesStateWithoutNotification(t *testing.T) {
	t.Parallel()
	repository := newMemoryRepository()
	machine := Machine{Repository: repository, Policy: DefaultPolicy()}
	first := Observation{
		Key: "group:7:user-impact", Family: "group_runtime", PolicyVersion: 1,
		SourceKind: "site_monitor", Severity: "P1", Failing: true,
		EvidenceHash: "requests:20:errors:2", MaterialHash: "partial-errors:gpt-5",
		CurrentValue: `{"latest_fact":"错误率 10%"}`, LatestPayload: []byte(`{"card":"first"}`),
		ConfirmationWindows: 1, RecoveryWindows: 2,
	}
	if transition, err := machine.Observe(context.Background(), first); err != nil || !transition.Notify {
		t.Fatalf("first transition=%#v err=%v", transition, err)
	}
	changed := first
	changed.EvidenceHash = "requests:25:errors:2"
	changed.CurrentValue = `{"latest_fact":"错误率 8%"}`
	changed.LatestPayload = []byte(`{"card":"updated"}`)
	transition, err := machine.Observe(context.Background(), changed)
	if err != nil || transition.Notify || transition.Kind != "" {
		t.Fatalf("evidence transition=%#v err=%v", transition, err)
	}
	record := repository.records[first.Key]
	if record.EvidenceHash != changed.EvidenceHash ||
		record.CurrentValue != changed.CurrentValue ||
		string(record.LatestPayload) != string(changed.LatestPayload) {
		t.Fatalf("stored record=%#v", record)
	}
}

func TestMaterialChangeProducesProgressedNotification(t *testing.T) {
	t.Parallel()
	repository := newMemoryRepository()
	machine := Machine{Repository: repository, Policy: DefaultPolicy()}
	observation := Observation{
		Key: "group:7:user-impact", Severity: "P1", Failing: true,
		EvidenceHash: "one", MaterialHash: "lost-redundancy",
		ConfirmationWindows: 1,
	}
	if _, err := machine.Observe(context.Background(), observation); err != nil {
		t.Fatal(err)
	}
	observation.EvidenceHash = "two"
	observation.MaterialHash = "partial-request-failures"
	transition, err := machine.Observe(context.Background(), observation)
	if err != nil || !transition.Notify || transition.Kind != "progressed" {
		t.Fatalf("material transition=%#v err=%v", transition, err)
	}
}

func TestRecoveryRequiresConfiguredHealthyWindows(t *testing.T) {
	t.Parallel()
	repository := newMemoryRepository()
	machine := Machine{Repository: repository, Policy: DefaultPolicy()}
	failing := Observation{
		Key: "group:7:user-impact", Severity: "P1", Failing: true,
		EvidenceHash: "failing", MaterialHash: "partial-request-failures",
		ConfirmationWindows: 1, RecoveryWindows: 2,
	}
	if _, err := machine.Observe(context.Background(), failing); err != nil {
		t.Fatal(err)
	}
	healthy := failing
	healthy.Failing = false
	healthy.EvidenceHash = "healthy"
	healthy.MaterialHash = "healthy"
	first, err := machine.Observe(context.Background(), healthy)
	if err != nil || first.Notify || first.Kind != "" || first.State != "confirmed" {
		t.Fatalf("first healthy=%#v err=%v", first, err)
	}
	second, err := machine.Observe(context.Background(), healthy)
	if err != nil || !second.Notify || second.Kind != "recovered" || second.State != "recovered" {
		t.Fatalf("second healthy=%#v err=%v", second, err)
	}
}

func TestFailureResetsRecoveryCount(t *testing.T) {
	t.Parallel()
	repository := newMemoryRepository()
	machine := Machine{Repository: repository, Policy: DefaultPolicy()}
	failing := Observation{
		Key: "group:7:user-impact", Severity: "P1", Failing: true,
		EvidenceHash: "failing", MaterialHash: "partial-request-failures",
		ConfirmationWindows: 1, RecoveryWindows: 2,
	}
	if _, err := machine.Observe(context.Background(), failing); err != nil {
		t.Fatal(err)
	}
	healthy := failing
	healthy.Failing = false
	if _, err := machine.Observe(context.Background(), healthy); err != nil {
		t.Fatal(err)
	}
	if repository.records[failing.Key].RecoveryCount != 1 {
		t.Fatalf("recovery count=%d", repository.records[failing.Key].RecoveryCount)
	}
	if transition, err := machine.Observe(context.Background(), failing); err != nil || transition.Notify {
		t.Fatalf("renewed failure transition=%#v err=%v", transition, err)
	}
	if repository.records[failing.Key].RecoveryCount != 0 {
		t.Fatalf("recovery count was not reset: %d", repository.records[failing.Key].RecoveryCount)
	}
}

func TestSeverityEscalationStillNotifies(t *testing.T) {
	t.Parallel()
	repository := newMemoryRepository()
	machine := Machine{Repository: repository, Policy: DefaultPolicy()}
	if _, err := machine.Observe(context.Background(), Observation{
		Key: "group:7:user-impact", Severity: "P1", Failing: true,
		EvidenceHash: "p1", MaterialHash: "partial-request-failures",
		ConfirmationWindows: 1,
	}); err != nil {
		t.Fatal(err)
	}
	transition, err := machine.Observe(context.Background(), Observation{
		Key: "group:7:user-impact", Severity: "P0", Failing: true,
		EvidenceHash: "p0", MaterialHash: "all-requests-failed",
	})
	if err != nil || !transition.Notify || transition.Kind != "escalated" {
		t.Fatalf("escalation=%#v err=%v", transition, err)
	}
}

func TestMachineUsesSeveritySpecificConfirmationWindows(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		severity string
		windows  int
	}{{"P0", 1}, {"P1", 2}, {"P2", 3}} {
		repository := newMemoryRepository()
		machine := Machine{Repository: repository, Policy: DefaultPolicy()}
		var transition Transition
		for index := 0; index < test.windows; index++ {
			transition, _ = machine.Observe(context.Background(), Observation{Key: "key:" + test.severity, Severity: test.severity, Failing: true, EvidenceHash: "same"})
		}
		if transition.Kind != "confirmed" || !transition.Notify {
			t.Fatalf("severity=%s transition=%#v", test.severity, transition)
		}
	}
}

func TestMachineAllowsHighConfidenceSingleWindowConfirmation(t *testing.T) {
	t.Parallel()
	machine := Machine{Repository: newMemoryRepository(), Policy: DefaultPolicy()}
	transition, err := machine.Observe(context.Background(), Observation{Key: "price-change", Severity: "P1", Failing: true, EvidenceHash: "snapshot", ConfirmationWindows: 1})
	if err != nil || !transition.Notify || transition.Kind != "confirmed" {
		t.Fatalf("transition=%#v err=%v", transition, err)
	}
}

func TestMachineAssignsANewOccurrenceAfterRecovery(t *testing.T) {
	t.Parallel()
	repository := newMemoryRepository()
	machine := Machine{Repository: repository, Policy: DefaultPolicy()}
	ctx := context.Background()
	observation := Observation{
		Key:                 "group:GPT-Plus:availability",
		Severity:            "P1",
		Failing:             true,
		EvidenceHash:        "available:0/1",
		CurrentValue:        "可用 0 / 共 1",
		ConfirmationWindows: 1,
	}

	first, err := machine.Observe(ctx, observation)
	if err != nil || first.OccurrenceNo != 1 || first.Kind != "confirmed" {
		t.Fatalf("first transition = %#v, %v", first, err)
	}
	repeated, err := machine.Observe(ctx, observation)
	if err != nil || repeated.OccurrenceNo != 1 || repeated.Notify {
		t.Fatalf("repeated transition = %#v, %v", repeated, err)
	}
	recovered, err := machine.Observe(ctx, Observation{
		Key:                 observation.Key,
		Severity:            "P1",
		Failing:             false,
		EvidenceHash:        "available:1/1",
		CurrentValue:        "可用 1 / 共 1",
		ConfirmationWindows: 1,
	})
	if err != nil || recovered.OccurrenceNo != 1 || recovered.Kind != "recovered" {
		t.Fatalf("recovery transition = %#v, %v", recovered, err)
	}
	second, err := machine.Observe(ctx, observation)
	if err != nil || second.OccurrenceNo != 2 || second.Kind != "confirmed" {
		t.Fatalf("second transition = %#v, %v", second, err)
	}
	if record := repository.records[observation.Key]; record.OccurrenceNo != 2 {
		t.Fatalf("stored occurrence = %d, want 2", record.OccurrenceNo)
	}
}

func TestMachineSuppressesAndResumesSameOccurrence(t *testing.T) {
	repository := newMemoryRepository()
	machine := Machine{Repository: repository, Policy: DefaultPolicy()}
	ctx := context.Background()
	observation := Observation{Key: "native:7:hash", Family: "native_ops_alert", SourceKind: "sub2api", Severity: "P0", Failing: true, EvidenceHash: "one", ConfirmationWindows: 1}
	first, err := machine.Observe(ctx, observation)
	if err != nil || !first.Notify || first.OccurrenceNo != 1 {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	if err := machine.Suppress(ctx, observation.Key); err != nil {
		t.Fatal(err)
	}
	record := repository.records[observation.Key]
	if record.State != "suppressed" || record.OccurrenceNo != 1 {
		t.Fatalf("suppressed record=%#v", record)
	}
	resumed, err := machine.Observe(ctx, observation)
	if err != nil || !resumed.Notify || resumed.OccurrenceNo != 1 || resumed.Kind != "resumed" {
		t.Fatalf("resumed=%#v err=%v", resumed, err)
	}
}

func TestMachineClosesResolvedAndManualResolved(t *testing.T) {
	for _, reason := range []string{"resolved", "manual_resolved"} {
		t.Run(reason, func(t *testing.T) {
			repository := newMemoryRepository()
			machine := Machine{Repository: repository, Policy: DefaultPolicy()}
			ctx := context.Background()
			observation := Observation{Key: "native:" + reason, Severity: "P0", Failing: true, EvidenceHash: "one", ConfirmationWindows: 1}
			if _, err := machine.Observe(ctx, observation); err != nil {
				t.Fatal(err)
			}
			transition, err := machine.Close(ctx, observation.Key, reason)
			if err != nil || !transition.Notify || transition.OccurrenceNo != 1 {
				t.Fatalf("transition=%#v err=%v", transition, err)
			}
			wantState, wantKind := "recovered", "recovered"
			if reason == "manual_resolved" {
				wantState, wantKind = "closed", "manual_resolved"
			}
			if transition.State != wantState || transition.Kind != wantKind {
				t.Fatalf("transition=%#v want state=%q kind=%q", transition, wantState, wantKind)
			}
		})
	}
}

type memoryRepository struct{ records map[string]Record }

func newMemoryRepository() *memoryRepository { return &memoryRepository{records: map[string]Record{}} }
func (r *memoryRepository) Get(_ context.Context, key string) (Record, bool, error) {
	value, ok := r.records[key]
	return value, ok, nil
}
func (r *memoryRepository) Put(_ context.Context, record Record) error {
	r.records[record.Key] = record
	return nil
}
