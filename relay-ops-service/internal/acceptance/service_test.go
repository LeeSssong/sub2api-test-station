package acceptance

import (
	"context"
	"testing"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/incidents"
)

func TestRunCompletesLocalDuplicateAndRecoveryCycleWithoutNotification(t *testing.T) {
	repository := &memoryRepository{}
	analysis := &fakeAnalysis{}
	service := Service{
		Incidents: incidents.Machine{Repository: repository, Policy: incidents.DefaultPolicy()},
		Agent:     analysis,
	}
	result, err := service.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "recovered" || result.Transition != "recovered" ||
		result.Notification != "not_configured" || result.ExternalUpstream != "not_accessed" {
		t.Fatalf("result=%#v", result)
	}
	if result.AlertNotification != "not_configured" ||
		result.DuplicateNotification != "suppressed" ||
		result.RecoveryNotification != "not_configured" {
		t.Fatalf("notification phases=%#v", result)
	}
	if analysis.calls != 1 {
		t.Fatalf("analysis calls=%d", analysis.calls)
	}
}

func TestRunDegradesWithoutOptionalIntegrations(t *testing.T) {
	repository := &memoryRepository{}
	service := Service{Incidents: incidents.Machine{Repository: repository, Policy: incidents.DefaultPolicy()}}
	result, err := service.Run(context.Background())
	if err != nil || result.AgentStatus != "fallback" || result.Notification != "not_configured" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	repeated, err := service.Run(context.Background())
	if err != nil || repeated.AgentStatus != "fallback" || repeated.Notification != "not_configured" || repeated.ExternalUpstream != "not_accessed" {
		t.Fatalf("repeated=%#v err=%v", repeated, err)
	}
}

func TestRunReevaluatesOnceWhenOptionalIntegrationsBecomeAvailable(t *testing.T) {
	repository := &memoryRepository{}
	local := Service{Incidents: incidents.Machine{Repository: repository, Policy: incidents.DefaultPolicy()}}
	if _, err := local.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	analysis := &fakeAnalysis{}
	integrated := Service{
		Incidents: incidents.Machine{Repository: repository, Policy: incidents.DefaultPolicy()},
		Agent:     analysis,
	}
	result, err := integrated.Run(context.Background())
	if err != nil || result.AgentStatus != "completed" ||
		result.Notification != "not_configured" || analysis.calls != 1 {
		t.Fatalf("integrated=%#v err=%v analysis=%d", result, err, analysis.calls)
	}
}

type memoryRepository struct {
	record incidents.Record
	found  bool
}

func (r *memoryRepository) Get(context.Context, string) (incidents.Record, bool, error) {
	return r.record, r.found, nil
}
func (r *memoryRepository) Put(_ context.Context, record incidents.Record) error {
	r.record, r.found = record, true
	return nil
}

type fakeAnalysis struct{ calls int }

func (f *fakeAnalysis) AnalyzeOnce(_ context.Context, contract agent.IncidentContractV1) (agent.Analysis, error) {
	f.calls++
	return agent.Fallback(contract), nil
}
