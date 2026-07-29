package groupimpact

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestServiceCorrelatesRuntimeCapacityAndMonitorIntoOneIncident(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	group := sub2api.Group{ID: 7, Name: "GPT PLUS 内测", Status: "active"}
	reader := impactReader(group, runtimeSnapshot(20, 18, .1, 1200), runtimeSnapshot(500, 490, .02, 1000))
	signals := &impactSignalRepository{byGroup: map[string][]store.GroupSignal{
		group.Name: {
			mustCapacitySignal(t, group.Name, 2, 3, now),
			mustNativeSignal(t, group, "error", now),
		},
	}}
	service, incidentRepository, sender, _ := impactService(reader, signals, now, notificationpolicy.ModeEnabled)

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 || sender.sent[0].key != "group:7:user-impact" {
		t.Fatalf("deliveries = %#v", sender.sent)
	}
	if len(incidentRepository.records) != 1 {
		t.Fatalf("incident records = %#v", incidentRepository.records)
	}
	text := sender.sent[0].message.RenderedText()
	for _, want := range []string{"GPT PLUS 内测", "部分请求持续失败", "可用账号 2 / 3", "原生监控"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
}

func TestServiceUsesStableGroupIDKeyAndCurrentGroupName(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	group := sub2api.Group{ID: 7, Name: "旧名称", Status: "active"}
	reader := impactReader(group, runtimeSnapshot(3, 0, 1, 0), runtimeSnapshot(500, 490, .02, 1000))
	service, repository, sender, _ := impactService(reader, &impactSignalRepository{}, now, notificationpolicy.ModeEnabled)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	reader.groups[0].Name = "GPT PLUS 内测"
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repository.records) != 1 || len(sender.sent) != 1 ||
		sender.sent[0].key != "group:7:user-impact" {
		t.Fatalf("records=%#v sent=%#v", repository.records, sender.sent)
	}
	record := repository.records["group:7:user-impact"]
	if !strings.Contains(record.CurrentValue, "GPT PLUS 内测") {
		t.Fatalf("current snapshot did not adopt current group name: %s", record.CurrentValue)
	}
}

func TestServiceStoresEvidenceChangeWithoutSending(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	group := sub2api.Group{ID: 7, Name: "GPT PLUS 内测", Status: "active"}
	reader := impactReader(group, runtimeSnapshot(31, 0, 1, 0), runtimeSnapshot(500, 490, .02, 1000))
	service, repository, sender, _ := impactService(reader, &impactSignalRepository{}, now, notificationpolicy.ModeEnabled)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	firstEvidence := repository.records["group:7:user-impact"].EvidenceHash
	reader.snapshots[impactSnapshotKey{groupID: 7, timeRange: "15m"}] = runtimeSnapshot(40, 0, 1, 0)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("numeric update resent card: %#v", sender.sent)
	}
	if got := repository.records["group:7:user-impact"].EvidenceHash; got == firstEvidence {
		t.Fatal("latest evidence was not stored")
	}
}

func TestServiceSendsProgressOnlyForMaterialChange(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	group := sub2api.Group{ID: 7, Name: "GPT PLUS 内测", Status: "active"}
	reader := impactReader(group, runtimeSnapshot(20, 18, .1, 1200), runtimeSnapshot(500, 490, .02, 1000))
	service, _, sender, _ := impactService(reader, &impactSignalRepository{}, now, notificationpolicy.ModeEnabled)
	for range 2 {
		if err := service.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	reader.snapshots[impactSnapshotKey{groupID: 7, timeRange: "15m"}] = runtimeSnapshot(31, 0, 1, 0)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 2 ||
		!strings.HasPrefix(sender.sent[1].message.Card.Header.Title.Content, "事故进展｜") {
		t.Fatalf("deliveries = %#v", sender.sent)
	}
}

func TestServiceRequiresTwoHealthyObservationsForRecovery(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	group := sub2api.Group{ID: 7, Name: "GPT PLUS 内测", Status: "active"}
	reader := impactReader(group, runtimeSnapshot(3, 0, 1, 0), runtimeSnapshot(500, 490, .02, 1000))
	service, repository, sender, _ := impactService(reader, &impactSignalRepository{}, now, notificationpolicy.ModeEnabled)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	reader.snapshots[impactSnapshotKey{groupID: 7, timeRange: "15m"}] = runtimeSnapshot(20, 20, 0, 1200)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 || repository.records["group:7:user-impact"].State == "recovered" {
		t.Fatalf("first healthy observation recovered incident: %#v", sender.sent)
	}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 2 ||
		!strings.HasPrefix(sender.sent[1].message.Card.Header.Title.Content, "恢复｜") ||
		repository.records["group:7:user-impact"].State != "recovered" {
		t.Fatalf("recovery deliveries=%#v record=%#v", sender.sent, repository.records["group:7:user-impact"])
	}
}

func TestServiceFailsSafeWhenRuntimeIsUnavailable(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	group := sub2api.Group{ID: 7, Name: "GPT PLUS 内测", Status: "active"}
	reader := impactReader(group, runtimeSnapshot(20, 0, 1, 0), runtimeSnapshot(500, 490, .02, 1000))
	reader.errors = map[impactSnapshotKey]error{
		{groupID: 7, timeRange: "15m"}: errors.New("runtime unavailable"),
	}
	service, repository, sender, decisions := impactService(reader, &impactSignalRepository{}, now, notificationpolicy.ModeEnabled)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repository.records) != 0 || len(sender.sent) != 0 ||
		len(decisions.records) != 1 ||
		decisions.records[0].Reason != "runtime_unavailable" {
		t.Fatalf("records=%#v sent=%#v decisions=%#v", repository.records, sender.sent, decisions.records)
	}
}

func TestServiceShadowRunsLifecycleWithoutSending(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	group := sub2api.Group{ID: 7, Name: "GPT PLUS 内测", Status: "active"}
	reader := impactReader(group, runtimeSnapshot(3, 0, 1, 0), runtimeSnapshot(500, 490, .02, 1000))
	service, repository, sender, decisions := impactService(reader, &impactSignalRepository{}, now, notificationpolicy.ModeShadow)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("shadow sent messages: %#v", sender.sent)
	}
	if _, found := repository.records["shadow:group:7:user-impact"]; !found {
		t.Fatalf("shadow incident missing: %#v", repository.records)
	}
	if len(decisions.records) != 1 || decisions.records[0].Decision != "shadow_would_deliver" {
		t.Fatalf("decisions = %#v", decisions.records)
	}
}

type impactSnapshotKey struct {
	groupID   int64
	timeRange string
}

type impactRuntimeReader struct {
	groups    []sub2api.Group
	snapshots map[impactSnapshotKey]sub2api.OpsSnapshot
	errors    map[impactSnapshotKey]error
}

func impactReader(
	group sub2api.Group,
	window sub2api.OpsSnapshot,
	baseline sub2api.OpsSnapshot,
) *impactRuntimeReader {
	return &impactRuntimeReader{
		groups: []sub2api.Group{group},
		snapshots: map[impactSnapshotKey]sub2api.OpsSnapshot{
			{groupID: group.ID, timeRange: "15m"}: window,
			{groupID: group.ID, timeRange: "24h"}: baseline,
		},
	}
}

func (reader *impactRuntimeReader) ListGroups(context.Context) ([]sub2api.Group, error) {
	return append([]sub2api.Group(nil), reader.groups...), nil
}

func (reader *impactRuntimeReader) GetOpsSnapshot(
	_ context.Context,
	query sub2api.OpsQuery,
) (sub2api.OpsSnapshot, error) {
	key := impactSnapshotKey{groupID: query.GroupID, timeRange: query.TimeRange}
	if err := reader.errors[key]; err != nil {
		return sub2api.OpsSnapshot{}, err
	}
	return reader.snapshots[key], nil
}

type impactSignalRepository struct {
	byGroup map[string][]store.GroupSignal
}

func (repository *impactSignalRepository) ListFreshGroupSignals(
	_ context.Context,
	groupName string,
	_ time.Time,
) ([]store.GroupSignal, error) {
	return append([]store.GroupSignal(nil), repository.byGroup[groupName]...), nil
}

type impactIncidentRepository struct {
	records map[string]incidents.Record
}

func (repository *impactIncidentRepository) Get(
	_ context.Context,
	key string,
) (incidents.Record, bool, error) {
	record, found := repository.records[key]
	return record, found, nil
}

func (repository *impactIncidentRepository) Put(
	_ context.Context,
	record incidents.Record,
) error {
	repository.records[record.Key] = record
	return nil
}

type impactSender struct {
	sent []impactDelivery
}

type impactDelivery struct {
	key      string
	evidence string
	message  notify.FeishuMessage
}

func (sender *impactSender) SendIncident(
	_ context.Context,
	key string,
	evidence string,
	message notify.FeishuMessage,
) error {
	sender.sent = append(sender.sent, impactDelivery{key: key, evidence: evidence, message: message})
	return nil
}

type impactDecisionRecorder struct {
	records []store.DecisionRecord
}

func (recorder *impactDecisionRecorder) RecordNotificationDecision(
	_ context.Context,
	record store.DecisionRecord,
) error {
	recorder.records = append(recorder.records, record)
	return nil
}

func impactService(
	reader *impactRuntimeReader,
	signals *impactSignalRepository,
	now time.Time,
	mode notificationpolicy.DeliveryMode,
) (Service, *impactIncidentRepository, *impactSender, *impactDecisionRecorder) {
	repository := &impactIncidentRepository{records: map[string]incidents.Record{}}
	sender := &impactSender{}
	decisions := &impactDecisionRecorder{}
	policy := notificationpolicy.Policy{
		Version: 1,
		Mode:    mode,
		Feishu: notificationpolicy.FeishuPolicy{
			GroupRuntimeEnabled:          true,
			GroupCapacityEnabled:         true,
			NativeMonitorEvidenceEnabled: true,
		},
	}
	return Service{
		Reader: reader, Signals: signals,
		Incidents: incidents.Machine{Repository: repository, Policy: incidents.DefaultPolicy()},
		Notifier:  sender, Policy: policy, Decisions: decisions,
		Now: func() time.Time { return now },
	}, repository, sender, decisions
}

func runtimeSnapshot(
	requests int64,
	successes int64,
	errorRate float64,
	ttftP95 float64,
) sub2api.OpsSnapshot {
	return sub2api.OpsSnapshot{Overview: sub2api.OpsOverview{
		RequestCountTotal: requests,
		SuccessCount:      successes,
		ErrorRate:         errorRate,
		TTFT:              sub2api.Percentiles{P95MS: ttftP95},
	}}
}

func mustCapacitySignal(
	t *testing.T,
	groupName string,
	available int,
	total int,
	now time.Time,
) store.GroupSignal {
	t.Helper()
	signal, err := CapacitySignal(groupName, CapacityEvidence{
		Available: available, Total: total,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return signal
}

func mustNativeSignal(
	t *testing.T,
	group sub2api.Group,
	status string,
	now time.Time,
) store.GroupSignal {
	t.Helper()
	signal, err := NativeMonitorSignal(group, sub2api.ChannelMonitor{
		ID: 9, GroupName: group.Name, Enabled: true, PrimaryModel: "gpt-5",
	}, sub2api.MonitorHistory{
		Model: "gpt-5", Status: status, CheckedAt: now.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	return signal
}
