package nativealerts

import (
	"context"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/agent"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestServiceConfirmsSuppressesAddsEvidenceAndRecovers(t *testing.T) {
	t.Parallel()

	repository := &memoryIncidentRepository{records: map[string]incidents.Record{}}
	analyzer := &recordingAnalyzer{}
	notifier := &recordingNotifier{}
	service := Service{
		Incidents: incidents.Machine{Repository: repository, Policy: incidents.DefaultPolicy()},
		Agent:     analyzer,
		Notifier:  notifier,
	}
	errorSample := Observation{
		Monitor: sub2api.ChannelMonitor{ID: 9, Name: "GPT-Pro monitor", GroupName: "GPT-Pro", PrimaryModel: "gpt-5.6-sol"},
		History: sub2api.MonitorHistory{ID: 101, Model: "gpt-5.6-sol", Status: "error", LatencyMS: 570, CheckedAt: "2026-07-20T14:55:00Z"},
	}

	first, err := service.Observe(context.Background(), errorSample)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != "observed" || first.Notification != "suppressed" || len(notifier.messages) != 0 {
		t.Fatalf("first=%#v messages=%d", first, len(notifier.messages))
	}

	confirmed, err := service.Observe(context.Background(), errorSample)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Transition != "confirmed" || confirmed.Notification != "delivered" || len(notifier.messages) != 1 || len(analyzer.contracts) != 1 {
		t.Fatalf("confirmed=%#v messages=%d analyses=%d", confirmed, len(notifier.messages), len(analyzer.contracts))
	}
	if !strings.Contains(notifier.messages[0].RenderedText(), "GPT-Pro") || !strings.Contains(notifier.messages[0].RenderedText(), "gpt-5.6-sol") {
		t.Fatalf("notification missing group/model: %q", notifier.messages[0].RenderedText())
	}

	duplicate, err := service.Observe(context.Background(), errorSample)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Notification != "suppressed" || len(notifier.messages) != 1 || len(analyzer.contracts) != 1 {
		t.Fatalf("duplicate=%#v messages=%d analyses=%d", duplicate, len(notifier.messages), len(analyzer.contracts))
	}

	degraded := errorSample
	degraded.History.Status = "degraded"
	degraded.History.ID = 102
	degraded.History.CheckedAt = "2026-07-20T15:00:00Z"
	changed, err := service.Observe(context.Background(), degraded)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Transition != "new_evidence" || changed.Notification != "delivered" || len(notifier.messages) != 2 || len(analyzer.contracts) != 2 {
		t.Fatalf("changed=%#v messages=%d analyses=%d", changed, len(notifier.messages), len(analyzer.contracts))
	}

	recovered := errorSample
	recovered.History.Status = "operational"
	recovered.History.ID = 103
	recovered.History.LatencyMS = 1396
	recovered.History.CheckedAt = "2026-07-20T15:05:00Z"
	recovery, err := service.Observe(context.Background(), recovered)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.Transition != "recovered" || recovery.Notification != "delivered" || len(notifier.messages) != 3 || len(analyzer.contracts) != 3 {
		t.Fatalf("recovery=%#v messages=%d analyses=%d", recovery, len(notifier.messages), len(analyzer.contracts))
	}
	recoveryText := notifier.messages[2].RenderedText()
	for _, want := range []string{"原生监控已恢复", "运行正常", "gpt-5.6-sol", "1396ms", "Sub2API 原生 Channel Monitor 最新状态"} {
		if !strings.Contains(recoveryText, want) {
			t.Fatalf("recovery message missing %q: %q", want, recoveryText)
		}
	}
	for _, forbidden := range []string{"**恢复结果**", "状态：operational"} {
		if strings.Contains(recoveryText, forbidden) {
			t.Fatalf("recovery message exposes legacy prose %q: %q", forbidden, recoveryText)
		}
	}

	healthy, err := service.Observe(context.Background(), recovered)
	if err != nil {
		t.Fatal(err)
	}
	if healthy.Notification != "suppressed" || len(notifier.messages) != 3 || len(analyzer.contracts) != 3 {
		t.Fatalf("healthy=%#v messages=%d analyses=%d", healthy, len(notifier.messages), len(analyzer.contracts))
	}
	for _, contract := range analyzer.contracts {
		if contract.ContractVersion != "relay-ops-incident-v1" || len(contract.EvidenceRefs) != 1 || strings.Contains(strings.Join(contract.EvidenceRefs, " "), "http") {
			t.Fatalf("unsafe or incomplete contract: %#v", contract)
		}
	}
}

type memoryIncidentRepository struct{ records map[string]incidents.Record }

func (r *memoryIncidentRepository) Get(_ context.Context, key string) (incidents.Record, bool, error) {
	record, ok := r.records[key]
	return record, ok, nil
}

func (r *memoryIncidentRepository) Put(_ context.Context, record incidents.Record) error {
	r.records[record.Key] = record
	return nil
}

type recordingAnalyzer struct{ contracts []agent.IncidentContractV1 }

func (a *recordingAnalyzer) AnalyzeOnce(_ context.Context, contract agent.IncidentContractV1) (agent.Analysis, error) {
	a.contracts = append(a.contracts, contract)
	return agent.Analysis{Summary: "只读分析", Change: "状态变化", Focus: "人工复核"}, nil
}

type recordingNotifier struct {
	keys     []string
	evidence []string
	messages []notify.FeishuMessage
}

func (n *recordingNotifier) SendIncident(_ context.Context, key, evidence string, message notify.FeishuMessage) error {
	n.keys = append(n.keys, key)
	n.evidence = append(n.evidence, evidence)
	n.messages = append(n.messages, message)
	return nil
}
