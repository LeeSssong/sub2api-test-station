package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
)

func TestRunCompletesAlertDuplicateAndRecoveryCycleWithoutUpstreamAccess(t *testing.T) {
	repository := &memoryRepository{}
	analysis := &fakeAnalysis{}
	notifier := &fakeNotifier{}
	service := Service{Incidents: incidents.Machine{Repository: repository, Policy: incidents.DefaultPolicy()}, Agent: analysis, Notifier: notifier}
	result, err := service.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "recovered" || result.Transition != "recovered" || result.Notification != "delivered" || result.ExternalUpstream != "not_accessed" {
		t.Fatalf("result=%#v", result)
	}
	if result.AlertNotification != "delivered" || result.DuplicateNotification != "suppressed" || result.RecoveryNotification != "delivered" {
		t.Fatalf("notification phases=%#v", result)
	}
	if analysis.calls != 1 || notifier.attempts != 2 || notifier.delivered != 2 || len(notifier.messages) != 2 {
		t.Fatalf("analysis=%d attempts=%d delivered=%d messages=%d", analysis.calls, notifier.attempts, notifier.delivered, len(notifier.messages))
	}
	if !strings.Contains(notifier.messages[0].RenderedText(), "relay-ops 合成告警验收") || !strings.Contains(notifier.messages[1].RenderedText(), "relay-ops 合成告警已恢复") {
		t.Fatalf("unexpected messages: %#v", notifier.messages)
	}
	recoveryText := notifier.messages[1].RenderedText()
	for _, want := range []string{"合成事件已恢复", "重复事件", "已抑制", "真实服务影响", "无", "合成验收"} {
		if !strings.Contains(recoveryText, want) {
			t.Fatalf("recovery missing %q in %q", want, recoveryText)
		}
	}
	for _, message := range notifier.messages {
		if strings.Contains(message.RenderedText(), "api_key") || strings.Contains(message.RenderedText(), "Bearer") {
			t.Fatalf("message contains secret marker: %s", message.RenderedText())
		}
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

func TestRunReportsNotificationFailureWithoutChangingCoreState(t *testing.T) {
	repository := &memoryRepository{}
	notifier := &fakeNotifier{err: errors.New("offline")}
	service := Service{Incidents: incidents.Machine{Repository: repository, Policy: incidents.DefaultPolicy()}, Notifier: notifier}
	result, err := service.Run(context.Background())
	if err != nil || result.Notification != "failed" || result.State != "recovered" || result.AlertNotification != "failed" || result.RecoveryNotification != "failed" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	notifier.err = nil
	retried, err := service.Run(context.Background())
	if err != nil || retried.Notification != "delivered" || notifier.delivered != 2 {
		t.Fatalf("retried=%#v err=%v delivered=%d", retried, err, notifier.delivered)
	}
}

func TestRunReevaluatesOnceWhenOptionalIntegrationsBecomeAvailable(t *testing.T) {
	repository := &memoryRepository{}
	local := Service{Incidents: incidents.Machine{Repository: repository, Policy: incidents.DefaultPolicy()}}
	if _, err := local.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	analysis := &fakeAnalysis{}
	notifier := &fakeNotifier{}
	integrated := Service{Incidents: incidents.Machine{Repository: repository, Policy: incidents.DefaultPolicy()}, Agent: analysis, Notifier: notifier}
	result, err := integrated.Run(context.Background())
	if err != nil || result.AgentStatus != "completed" || result.Notification != "delivered" || analysis.calls != 1 || notifier.delivered != 2 {
		t.Fatalf("integrated=%#v err=%v analysis=%d delivered=%d", result, err, analysis.calls, notifier.delivered)
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

type fakeNotifier struct {
	attempts  int
	delivered int
	messages  []notify.FeishuMessage
	err       error
}

func (f *fakeNotifier) SendIncident(_ context.Context, _, _ string, message notify.FeishuMessage) error {
	f.attempts++
	if f.err != nil {
		return f.err
	}
	f.messages = append(f.messages, message)
	f.delivered++
	return nil
}
