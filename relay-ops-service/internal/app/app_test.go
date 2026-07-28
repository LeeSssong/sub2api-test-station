package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/config"
	"example.invalid/relay-ops-service/internal/domain"
	"example.invalid/relay-ops-service/internal/feishuapi"
	httpserver "example.invalid/relay-ops-service/internal/http"
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

func TestAcceptanceAnalysisRunnerDoesNotWrapAnUnconfiguredAgentAsANonNilInterface(t *testing.T) {
	runner := acceptanceAnalysisRunner(nil)
	if runner != nil {
		t.Fatalf("unconfigured runner must stay nil: acceptance=%T", runner)
	}

	service := &agent.Service{}
	runner = acceptanceAnalysisRunner(service)
	if runner == nil {
		t.Fatal("configured agent must be wired into acceptance analysis")
	}
}

func TestNotificationClientUsesExistingFeishuAppForConfiguredAlertChat(t *testing.T) {
	chatFile := filepath.Join(t.TempDir(), "feishu-alert-chat-id")
	if err := os.WriteFile(chatFile, []byte("oc_alert_group\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recipientsFile := filepath.Join(t.TempDir(), "feishu-alert-recipients.json")
	if err := os.WriteFile(recipientsFile, []byte(`{"open_ids":["operator-a"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sender := &appTestMessageSender{}
	client, err := notificationClient(config.Config{
		FeishuAlertChatIDFile: chatFile, FeishuAlertRecipientsFile: recipientsFile,
	}, sender)
	if err != nil {
		t.Fatal(err)
	}
	message := notify.RenderFeishu(notify.IncidentView{Title: "合成告警", Severity: "P1"})
	if err := client.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if sender.chatID != "oc_alert_group" || sender.payload.MsgType != "interactive" {
		t.Fatalf("sender=%#v", sender)
	}
	var card notify.Card
	if err := json.Unmarshal(sender.payload.Content, &card); err != nil {
		t.Fatal(err)
	}
	if len(card.Elements) == 0 || card.Elements[0].Text == nil ||
		!strings.Contains(card.Elements[0].Text.Content, "<at id=operator-a></at>") {
		t.Fatal("configured alert recipient was not mentioned")
	}
}

func TestExecuteFastCandidatePersistsQualityReportWithoutNotificationDependencies(t *testing.T) {
	now := time.Date(2026, 7, 22, 3, 0, 0, 0, time.UTC)
	record := []byte(`{"schema_version":1,"run_id":"fast-1","channel_id":"candidate-17","profile_id":"quality-first-fast-v1","job_kind":"health_pulse","recorded_at":"2026-07-22T03:00:00Z","status":"passed","metrics":{"selected_models":["gpt-a"],"direct":{"request_count":2,"success_count":2,"success_rate":1,"latency":{"p95_ms":1200},"ttft":{"p95_ms":500}},"gateway":{"status":"unknown"}},"errors":[]}`)
	runner := &fakeFastRunner{result: probes.FastResult{RunID: "fast-1", JobKind: "health_pulse", Status: "passed", RecordedAt: now, Record: record}}
	sink := &fakeQualitySink{}
	repository := fakeCandidateRepository{items: []candidates.Candidate{{ID: 17, Name: "Candidate", BaseURL: "https://candidate.example/v1", Enabled: true}}}

	err := executeFastCandidate(
		context.Background(), domain.UpstreamID(17), "health_pulse",
		repository, runner, sink,
	)
	if err != nil {
		t.Fatalf("executeFastCandidate: %v", err)
	}
	if runner.candidate.ID != 17 || sink.report.ReportID != "fast-1" || len(sink.report.ReportHash) != 64 {
		t.Fatalf("runner=%#v report=%#v", runner.candidate, sink.report)
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
