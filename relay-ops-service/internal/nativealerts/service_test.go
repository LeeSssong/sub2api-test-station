package nativealerts

import (
	"context"
	"encoding/json"
	"testing"

	"example.invalid/relay-ops-service/internal/groupimpact"
	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestServiceStoresAbnormalMonitorAsGroupEvidenceWithoutPaging(t *testing.T) {
	t.Parallel()
	sink := &recordingSignalSink{}
	service := Service{Signals: sink, Policy: nativeMonitorPolicy()}
	group := sub2api.Group{ID: 7, Name: "GPT PLUS 内测", Status: "active"}
	monitor := sub2api.ChannelMonitor{
		ID: 9, GroupName: group.Name, Enabled: true, PrimaryModel: "gpt-5",
	}
	history := sub2api.MonitorHistory{
		ID: 17, Model: "gpt-5", Status: "error", LatencyMS: 1200,
		CheckedAt: "2026-07-29T01:00:00Z",
	}
	if err := service.ObserveMonitor(context.Background(), group, monitor, history); err != nil {
		t.Fatal(err)
	}
	if len(sink.signals) != 1 || len(sink.decisions) != 1 {
		t.Fatalf("signals=%#v decisions=%#v", sink.signals, sink.decisions)
	}
	var payload groupimpact.NativeMonitorSignalPayload
	if err := json.Unmarshal(sink.signals[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "error" || payload.Model != "gpt-5" ||
		sink.decisions[0].Decision != "evidence_stored" {
		t.Fatalf("payload=%#v decision=%#v", payload, sink.decisions[0])
	}
}

func TestServicePolicyDisabledRecordsSuppressionWithoutSignal(t *testing.T) {
	t.Parallel()
	sink := &recordingSignalSink{}
	policy := nativeMonitorPolicy()
	policy.Feishu.NativeMonitorEvidenceEnabled = false
	service := Service{Signals: sink, Policy: policy}
	group := sub2api.Group{ID: 7, Name: "GPT PLUS 内测", Status: "active"}
	if err := service.ObserveMonitor(context.Background(), group, sub2api.ChannelMonitor{
		ID: 9, GroupName: group.Name, Enabled: true, PrimaryModel: "gpt-5",
	}, sub2api.MonitorHistory{
		Model: "gpt-5", Status: "error", CheckedAt: "2026-07-29T01:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if len(sink.signals) != 0 || len(sink.decisions) != 1 ||
		sink.decisions[0].Decision != "suppressed" ||
		sink.decisions[0].Reason != "policy_disabled" {
		t.Fatalf("signals=%#v decisions=%#v", sink.signals, sink.decisions)
	}
}

func TestServiceRejectsMonitorOutsideMatchedVisibleGroup(t *testing.T) {
	t.Parallel()
	service := Service{Signals: &recordingSignalSink{}, Policy: nativeMonitorPolicy()}
	err := service.ObserveMonitor(
		context.Background(),
		sub2api.Group{ID: 7, Name: "private", Status: "active", IsExclusive: true},
		sub2api.ChannelMonitor{ID: 9, GroupName: "GPT PLUS 内测", Enabled: true},
		sub2api.MonitorHistory{Status: "error", CheckedAt: "2026-07-29T01:00:00Z"},
	)
	if err == nil {
		t.Fatal("unmatched private group was accepted")
	}
}

type recordingSignalSink struct {
	signals   []store.GroupSignal
	decisions []store.DecisionRecord
}

func (sink *recordingSignalSink) UpsertGroupSignal(
	_ context.Context,
	signal store.GroupSignal,
) error {
	sink.signals = append(sink.signals, signal)
	return nil
}

func (sink *recordingSignalSink) RecordNotificationDecision(
	_ context.Context,
	decision store.DecisionRecord,
) error {
	sink.decisions = append(sink.decisions, decision)
	return nil
}

func nativeMonitorPolicy() notificationpolicy.Policy {
	return notificationpolicy.Policy{
		Version: 1,
		Mode:    notificationpolicy.ModeEnabled,
		Feishu: notificationpolicy.FeishuPolicy{
			NativeMonitorEvidenceEnabled: true,
		},
	}
}
