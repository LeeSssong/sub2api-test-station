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
	if !newEvidence.Notify || newEvidence.Kind != "new_evidence" {
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
