package groupimpact

import (
	"encoding/json"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestCapacitySignalUsesStableIdentityAndFreshBoundedPayload(t *testing.T) {
	t.Parallel()
	observedAt := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	unavailable := make([]UnavailableAccount, 25)
	for index := range unavailable {
		unavailable[index] = UnavailableAccount{Name: "账号", Reason: "余额已耗尽"}
	}
	signal, err := CapacitySignal("GPT PLUS 内测", CapacityEvidence{
		Available: 0, Total: 25, Unavailable: unavailable,
	}, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if signal.GroupName != "GPT PLUS 内测" ||
		signal.SourceKind != "capacity" ||
		signal.SourceKey != "current" ||
		!signal.SourceObservedAt.Equal(observedAt) ||
		!signal.ExpiresAt.Equal(observedAt.Add(15*time.Minute)) {
		t.Fatalf("signal = %#v", signal)
	}
	var payload CapacitySignalPayload
	if err := json.Unmarshal(signal.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Available != 0 || payload.Total != 25 || len(payload.Unavailable) != 20 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestNativeMonitorSignalUsesMatchedGroupAndLatestEvidenceTime(t *testing.T) {
	t.Parallel()
	group := sub2api.Group{ID: 7, Name: "GPT PLUS 内测", Status: "active"}
	monitor := sub2api.ChannelMonitor{ID: 9, GroupName: group.Name, Enabled: true, PrimaryModel: "gpt-5"}
	history := sub2api.MonitorHistory{
		ID: 17, Model: "gpt-5", Status: "error", LatencyMS: 1200,
		CheckedAt: "2026-07-29T01:00:00Z",
	}
	signal, err := NativeMonitorSignal(group, monitor, history)
	if err != nil {
		t.Fatal(err)
	}
	if signal.GroupName != group.Name ||
		signal.SourceKind != "native_monitor" ||
		signal.SourceKey != "9:gpt-5" ||
		!signal.SourceObservedAt.Equal(time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)) {
		t.Fatalf("signal = %#v", signal)
	}
	var payload NativeMonitorSignalPayload
	if err := json.Unmarshal(signal.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "error" || payload.Model != "gpt-5" || payload.MonitorID != 9 {
		t.Fatalf("payload = %#v", payload)
	}
}
