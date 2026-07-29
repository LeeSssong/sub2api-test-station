package groupimpact

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
)

const (
	CapacitySourceKind      = "capacity"
	NativeMonitorSourceKind = "native_monitor"
	groupSignalTTL          = 15 * time.Minute
	maxUnavailableAccounts  = 20
)

type CapacitySignalPayload struct {
	Available   int                  `json:"available"`
	Total       int                  `json:"total"`
	Unavailable []UnavailableAccount `json:"unavailable,omitempty"`
	ObservedAt  time.Time            `json:"observed_at"`
}

type NativeMonitorSignalPayload struct {
	MonitorID  int64     `json:"monitor_id"`
	Model      string    `json:"model"`
	Status     string    `json:"status"`
	LatencyMS  int64     `json:"latency_ms,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}

func CapacitySignal(
	groupName string,
	capacity CapacityEvidence,
	observedAt time.Time,
) (store.GroupSignal, error) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" || observedAt.IsZero() ||
		capacity.Available < 0 || capacity.Total < 0 || capacity.Available > capacity.Total {
		return store.GroupSignal{}, fmt.Errorf("capacity signal is invalid")
	}
	unavailable := make([]UnavailableAccount, 0, min(len(capacity.Unavailable), maxUnavailableAccounts))
	for _, account := range capacity.Unavailable {
		if len(unavailable) == maxUnavailableAccounts {
			break
		}
		name := strings.TrimSpace(account.Name)
		reason := humanCause(account.Reason)
		if name == "" || reason == "" {
			continue
		}
		unavailable = append(unavailable, UnavailableAccount{Name: name, Reason: reason})
	}
	observedAt = observedAt.UTC()
	payload, err := json.Marshal(CapacitySignalPayload{
		Available: capacity.Available, Total: capacity.Total,
		Unavailable: unavailable, ObservedAt: observedAt,
	})
	if err != nil {
		return store.GroupSignal{}, fmt.Errorf("encode capacity signal: %w", err)
	}
	return store.GroupSignal{
		GroupName: groupName, SourceKind: CapacitySourceKind, SourceKey: "current",
		Payload: payload, SourceObservedAt: observedAt, ExpiresAt: observedAt.Add(groupSignalTTL),
	}, nil
}

func NativeMonitorSignal(
	group sub2api.Group,
	monitor sub2api.ChannelMonitor,
	history sub2api.MonitorHistory,
) (store.GroupSignal, error) {
	if !group.CustomerVisible() || !monitor.Enabled ||
		strings.TrimSpace(group.Name) == "" ||
		strings.TrimSpace(monitor.GroupName) != strings.TrimSpace(group.Name) ||
		monitor.ID <= 0 {
		return store.GroupSignal{}, fmt.Errorf("native monitor is not bound to a visible group")
	}
	observedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(history.CheckedAt))
	if err != nil {
		return store.GroupSignal{}, fmt.Errorf("native monitor evidence time is invalid")
	}
	model := strings.TrimSpace(history.Model)
	if model == "" {
		model = strings.TrimSpace(monitor.PrimaryModel)
	}
	if model == "" {
		model = "default"
	}
	status := strings.ToLower(strings.TrimSpace(history.Status))
	if status == "" {
		return store.GroupSignal{}, fmt.Errorf("native monitor status is required")
	}
	observedAt = observedAt.UTC()
	payload, err := json.Marshal(NativeMonitorSignalPayload{
		MonitorID: monitor.ID, Model: model, Status: status,
		LatencyMS: history.LatencyMS, ObservedAt: observedAt,
	})
	if err != nil {
		return store.GroupSignal{}, fmt.Errorf("encode native monitor signal: %w", err)
	}
	return store.GroupSignal{
		GroupName: group.Name, SourceKind: NativeMonitorSourceKind,
		SourceKey: strconv.FormatInt(monitor.ID, 10) + ":" + model,
		Payload:   payload, SourceObservedAt: observedAt, ExpiresAt: observedAt.Add(groupSignalTTL),
	}, nil
}

func DecodeCapacitySignal(signal store.GroupSignal) (CapacityEvidence, error) {
	if signal.SourceKind != CapacitySourceKind {
		return CapacityEvidence{}, fmt.Errorf("group signal is not capacity evidence")
	}
	var payload CapacitySignalPayload
	if err := json.Unmarshal(signal.Payload, &payload); err != nil {
		return CapacityEvidence{}, fmt.Errorf("decode capacity signal: %w", err)
	}
	if payload.Available < 0 || payload.Total < 0 || payload.Available > payload.Total ||
		payload.ObservedAt.IsZero() {
		return CapacityEvidence{}, fmt.Errorf("capacity signal payload is invalid")
	}
	return CapacityEvidence{
		Available: payload.Available, Total: payload.Total,
		Unavailable: append([]UnavailableAccount(nil), payload.Unavailable...),
		ObservedAt:  payload.ObservedAt,
	}, nil
}

func DecodeNativeMonitorSignal(signal store.GroupSignal) (NativeMonitorEvidence, error) {
	if signal.SourceKind != NativeMonitorSourceKind {
		return NativeMonitorEvidence{}, fmt.Errorf("group signal is not native monitor evidence")
	}
	var payload NativeMonitorSignalPayload
	if err := json.Unmarshal(signal.Payload, &payload); err != nil {
		return NativeMonitorEvidence{}, fmt.Errorf("decode native monitor signal: %w", err)
	}
	if payload.MonitorID <= 0 || strings.TrimSpace(payload.Status) == "" || payload.ObservedAt.IsZero() {
		return NativeMonitorEvidence{}, fmt.Errorf("native monitor signal payload is invalid")
	}
	return NativeMonitorEvidence{
		Model: payload.Model, Status: payload.Status, ObservedAt: payload.ObservedAt,
	}, nil
}
