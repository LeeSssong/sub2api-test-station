package nativealerts

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/groupimpact"
	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type SignalSink interface {
	UpsertGroupSignal(context.Context, store.GroupSignal) error
	RecordNotificationDecision(context.Context, store.DecisionRecord) error
}

type Service struct {
	Signals SignalSink
	Policy  notificationpolicy.Policy
}

func (service Service) ObserveMonitor(
	ctx context.Context,
	group sub2api.Group,
	monitor sub2api.ChannelMonitor,
	history sub2api.MonitorHistory,
) error {
	if service.Policy.Version <= 0 {
		return nil
	}
	if service.Signals == nil {
		return fmt.Errorf("native monitor signal sink is required")
	}
	if !group.CustomerVisible() || !monitor.Enabled ||
		strings.TrimSpace(group.Name) == "" ||
		strings.TrimSpace(monitor.GroupName) != strings.TrimSpace(group.Name) {
		return fmt.Errorf("native monitor is not bound to the visible group")
	}
	model := strings.TrimSpace(history.Model)
	if model == "" {
		model = strings.TrimSpace(monitor.PrimaryModel)
	}
	decisionKey := "native-monitor:" + strconv.FormatInt(monitor.ID, 10) + ":" + model
	if !service.Policy.Enabled(notificationpolicy.FamilyNativeMonitorEvidence) {
		return service.Signals.RecordNotificationDecision(ctx, nativeMonitorDecision(
			decisionKey,
			service.Policy,
			"suppressed",
			"policy_disabled",
			group,
			monitor,
			history,
		))
	}
	signal, err := groupimpact.NativeMonitorSignal(group, monitor, history)
	if err != nil {
		return err
	}
	if err := service.Signals.UpsertGroupSignal(ctx, signal); err != nil {
		return fmt.Errorf("store native monitor signal: %w", err)
	}
	return service.Signals.RecordNotificationDecision(ctx, nativeMonitorDecision(
		decisionKey,
		service.Policy,
		"evidence_stored",
		"fresh_native_monitor_signal",
		group,
		monitor,
		history,
	))
}

func nativeMonitorDecision(
	key string,
	policy notificationpolicy.Policy,
	decision string,
	reason string,
	group sub2api.Group,
	monitor sub2api.ChannelMonitor,
	history sub2api.MonitorHistory,
) store.DecisionRecord {
	details, _ := json.Marshal(struct {
		GroupName string `json:"group_name"`
		MonitorID int64  `json:"monitor_id"`
		Model     string `json:"model"`
		Status    string `json:"status"`
	}{
		GroupName: group.Name,
		MonitorID: monitor.ID,
		Model:     history.Model,
		Status:    strings.ToLower(strings.TrimSpace(history.Status)),
	})
	return store.DecisionRecord{
		DecisionKey:   key,
		Family:        string(notificationpolicy.FamilyNativeMonitorEvidence),
		PolicyVersion: policy.Version,
		SourceKind:    groupimpact.NativeMonitorSourceKind,
		Decision:      decision,
		Reason:        reason,
		Details:       details,
		ObservedAt:    signalObservedAt(history),
	}
}

func signalObservedAt(history sub2api.MonitorHistory) time.Time {
	observedAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(history.CheckedAt))
	return observedAt.UTC()
}
