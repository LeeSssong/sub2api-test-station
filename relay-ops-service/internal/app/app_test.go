package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/config"
	"example.invalid/relay-ops-service/internal/dailyreport"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/feishuapi"
	httpserver "example.invalid/relay-ops-service/internal/http"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/nativealerts"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/probes"
	"example.invalid/relay-ops-service/internal/qualityreports"
)

func TestCandidateServiceUsesConfiguredManagedSecretDirectory(t *testing.T) {
	service := configuredCandidateService(config.Config{CandidateSecretDir: "/var/lib/relay-ops/candidate-keys"}, nil)
	store, ok := service.SecretStore.(candidates.FileSecretStore)
	if !ok {
		t.Fatalf("SecretStore = %T", service.SecretStore)
	}
	if store.Directory != "/var/lib/relay-ops/candidate-keys" {
		t.Fatalf("Directory = %q", store.Directory)
	}
}

func TestAnalysisRunnersDoNotWrapAnUnconfiguredAgentAsANonNilInterface(t *testing.T) {
	collectorRunner, acceptanceRunner := analysisRunners(nil)
	if collectorRunner != nil || acceptanceRunner != nil {
		t.Fatalf("unconfigured runners must stay nil: collector=%T acceptance=%T", collectorRunner, acceptanceRunner)
	}

	service := &agent.Service{}
	collectorRunner, acceptanceRunner = analysisRunners(service)
	if collectorRunner == nil || acceptanceRunner == nil {
		t.Fatal("configured agent must be wired into both analysis paths")
	}
}

func TestOperationalAnalysisRunnersStayNilUntilAgentIsConfigured(t *testing.T) {
	nativeRunner, reportRunner := operationalAnalysisRunners(nil)
	if nativeRunner != nil || reportRunner != nil {
		t.Fatalf("unconfigured operational runners must stay nil: native=%T report=%T", nativeRunner, reportRunner)
	}

	service := &agent.Service{}
	nativeRunner, reportRunner = operationalAnalysisRunners(service)
	if nativeRunner == nil || reportRunner == nil {
		t.Fatal("configured agent must be wired into native alerts and daily reports")
	}
	var _ nativealerts.AnalysisRunner = nativeRunner
	var _ dailyreport.AnalysisRunner = reportRunner
}

func TestConfiguredSiteMonitorSharesTheIncidentMachine(t *testing.T) {
	machine := &incidents.Machine{}
	monitor := configuredSiteMonitor(nil, nil, nil, machine, nil)
	if monitor.Incidents != machine {
		t.Fatalf("incident machine = %p, want %p", monitor.Incidents, machine)
	}
}

func TestNotificationClientUsesExistingFeishuAppForConfiguredAlertChat(t *testing.T) {
	chatFile := filepath.Join(t.TempDir(), "feishu-alert-chat-id")
	if err := os.WriteFile(chatFile, []byte("oc_alert_group\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sender := &appTestMessageSender{}
	client, err := notificationClient(config.Config{FeishuAlertChatIDFile: chatFile}, sender)
	if err != nil {
		t.Fatal(err)
	}
	message := notify.RenderFeishu(notify.IncidentView{Title: "合成告警"})
	if err := client.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if sender.chatID != "oc_alert_group" || sender.payload.MsgType != "interactive" {
		t.Fatalf("sender=%#v", sender)
	}
}

func TestExecuteFastCandidatePersistsHashBoundQualityReport(t *testing.T) {
	now := time.Date(2026, 7, 22, 3, 0, 0, 0, time.UTC)
	record := []byte(`{"schema_version":1,"run_id":"fast-1","channel_id":"candidate-17","profile_id":"quality-first-fast-v1","job_kind":"health_pulse","recorded_at":"2026-07-22T03:00:00Z","status":"passed","metrics":{"selected_models":["gpt-a"],"direct":{"request_count":2,"success_count":2,"success_rate":1,"latency":{"p95_ms":1200},"ttft":{"p95_ms":500}},"gateway":{"status":"unknown"}},"errors":[]}`)
	runner := &fakeFastRunner{result: probes.FastResult{RunID: "fast-1", JobKind: "health_pulse", Status: "passed", RecordedAt: now, Record: record}}
	sink := &fakeQualitySink{}
	incidentStore := &fakeQualityIncidentStore{}
	notifier := &fakeQualityNotifier{}
	repository := fakeCandidateRepository{items: []candidates.Candidate{{ID: 17, Name: "Candidate", BaseURL: "https://candidate.example/v1", Enabled: true}}}

	err := executeFastCandidate(context.Background(), domain.UpstreamID(17), "health_pulse", repository, runner, sink, incidentStore, notifier)
	if err != nil {
		t.Fatalf("executeFastCandidate: %v", err)
	}
	if runner.candidate.ID != 17 || sink.report.ReportID != "fast-1" || len(sink.report.ReportHash) != 64 {
		t.Fatalf("runner=%#v report=%#v", runner.candidate, sink.report)
	}
	if notifier.key != "quality-report:17:health_pulse" || notifier.evidence != qualityNotificationEvidence(sink.report) {
		t.Fatalf("notification identity = %q %q", notifier.key, notifier.evidence)
	}
	if incidentStore.record.Key != notifier.key || incidentStore.record.EvidenceHash != notifier.evidence || incidentStore.record.State != "confirmed" {
		t.Fatalf("incident state = %#v", incidentStore.record)
	}
	if notifier.message.MsgType != "interactive" || notifier.message.Card == nil || !strings.Contains(notifier.message.Content.Text, "本通知不执行切换") {
		t.Fatalf("notification = %#v", notifier.message)
	}
}

func TestQualityNotificationEvidenceDeduplicatesEquivalentRuns(t *testing.T) {
	first := qualityreports.Report{
		ReportID: "run-1", ReportHash: strings.Repeat("a", 64), UpstreamID: 17, JobKind: "health_pulse",
		Status: "needs_evidence", QualityScore: 80, TotalScore: 80,
		Direct: "6/6", Gateway: "unknown", Models: "3 selected", Pricing: "unknown", Capacity: "unknown",
	}
	second := first
	second.ReportID = "run-2"
	second.ReportHash = strings.Repeat("b", 64)

	if qualityNotificationEvidence(first) != qualityNotificationEvidence(second) {
		t.Fatal("equivalent reports must share notification evidence")
	}
	second.Status = "blocked"
	if qualityNotificationEvidence(first) == qualityNotificationEvidence(second) {
		t.Fatal("a changed quality decision must produce new notification evidence")
	}
}

func TestQualityReviewAdapterMapsStaleEvidenceWithoutWrites(t *testing.T) {
	now := time.Date(2026, 7, 22, 3, 0, 0, 0, time.UTC)
	repository := &fakeQualityRepository{report: qualityreports.Report{
		ReportID: "fast-1", ReportHash: strings.Repeat("a", 64), ExpiresAt: now.Add(time.Minute), Status: "needs_evidence",
	}}
	adapter := qualityReviewAdapter{Service: qualityreports.Service{Repository: repository, Clock: func() time.Time { return now }}}

	preview, err := adapter.Preview(context.Background(), domain.AdminActor{UserID: 1}, httpserver.QualityPreviewInput{ReportID: "fast-1", ReportHash: repository.report.ReportHash})
	if err != nil || preview.Writes != 0 || preview.Status != "dry_run" {
		t.Fatalf("Preview = %#v, %v", preview, err)
	}
	_, err = adapter.Preview(context.Background(), domain.AdminActor{UserID: 1}, httpserver.QualityPreviewInput{ReportID: "fast-1", ReportHash: strings.Repeat("b", 64)})
	if !errors.Is(err, httpserver.ErrQualityReportStale) {
		t.Fatalf("stale error = %v", err)
	}
}

type fakeCandidateRepository struct{ items []candidates.Candidate }

func (f fakeCandidateRepository) ListCandidates(context.Context) ([]candidates.Candidate, error) {
	return f.items, nil
}

type fakeFastRunner struct {
	result    probes.FastResult
	candidate candidates.Candidate
}

func (f *fakeFastRunner) Fast(_ context.Context, candidate candidates.Candidate, _ string) (probes.FastResult, error) {
	f.candidate = candidate
	return f.result, nil
}

type fakeQualitySink struct{ report qualityreports.Report }

func (f *fakeQualitySink) PutQualityReport(_ context.Context, report qualityreports.Report) error {
	f.report = report
	return nil
}

type fakeQualityNotifier struct {
	key      string
	evidence string
	message  notify.FeishuMessage
}

type fakeQualityIncidentStore struct{ record incidents.Record }

func (f *fakeQualityIncidentStore) Put(_ context.Context, record incidents.Record) error {
	f.record = record
	return nil
}

func (f *fakeQualityNotifier) SendIncident(_ context.Context, key, evidence string, message notify.FeishuMessage) error {
	f.key = key
	f.evidence = evidence
	f.message = message
	return nil
}

type fakeQualityRepository struct{ report qualityreports.Report }

func (f *fakeQualityRepository) Get(context.Context, string) (qualityreports.Report, bool, error) {
	return f.report, true, nil
}
func (f *fakeQualityRepository) Put(context.Context, qualityreports.Report) error { return nil }

type appTestMessageSender struct {
	chatID  string
	payload feishuapi.OutboundMessage
}

func (s *appTestMessageSender) SendMessage(_ context.Context, chatID string, payload feishuapi.OutboundMessage) (string, error) {
	s.chatID = chatID
	s.payload = payload
	return "om_alert", nil
}
