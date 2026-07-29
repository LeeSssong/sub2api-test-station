package groupimpact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type RuntimeReader interface {
	ListGroups(context.Context) ([]sub2api.Group, error)
	GetOpsSnapshot(context.Context, sub2api.OpsQuery) (sub2api.OpsSnapshot, error)
}

type SignalRepository interface {
	ListFreshGroupSignals(context.Context, string, time.Time) ([]store.GroupSignal, error)
}

type IncidentObserver interface {
	Observe(context.Context, incidents.Observation) (incidents.Transition, error)
}

type IncidentSender interface {
	SendIncident(context.Context, string, string, notify.FeishuMessage) error
}

type DecisionRecorder interface {
	RecordNotificationDecision(context.Context, store.DecisionRecord) error
}

type Service struct {
	Reader    RuntimeReader
	Signals   SignalRepository
	Incidents IncidentObserver
	Notifier  IncidentSender
	Policy    notificationpolicy.Policy
	Decisions DecisionRecorder
	Now       func() time.Time
}

func (service Service) Run(ctx context.Context) error {
	if service.Policy.Version <= 0 {
		return nil
	}
	if service.Decisions == nil {
		return fmt.Errorf("group impact decision recorder is required")
	}
	now := time.Now().UTC()
	if service.Now != nil {
		now = service.Now().UTC()
	}
	if !service.Policy.Enabled(notificationpolicy.FamilyGroupRuntime) ||
		service.Policy.Mode == notificationpolicy.ModeDisabled {
		return service.recordDecision(ctx, 0, "", "suppressed", "policy_disabled", "", Impact{}, now)
	}
	if service.Reader == nil || service.Signals == nil || service.Incidents == nil {
		return fmt.Errorf("group impact service dependencies are incomplete")
	}
	groups, err := service.Reader.ListGroups(ctx)
	if err != nil {
		return service.recordDecision(ctx, 0, "", "suppressed", "runtime_unavailable", "", Impact{}, now)
	}

	var failures []error
	for _, group := range groups {
		if !group.CustomerVisible() {
			continue
		}
		if err := service.runGroup(ctx, group, now); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (service Service) runGroup(
	ctx context.Context,
	group sub2api.Group,
	now time.Time,
) error {
	window, err := service.Reader.GetOpsSnapshot(ctx, sub2api.OpsQuery{
		TimeRange: "15m", GroupID: group.ID,
	})
	if err != nil {
		return service.recordDecision(
			ctx, group.ID, group.Name, "suppressed", "runtime_unavailable", "", Impact{}, now,
		)
	}
	baseline, err := service.Reader.GetOpsSnapshot(ctx, sub2api.OpsQuery{
		TimeRange: "24h", GroupID: group.ID,
	})
	if err != nil {
		return service.recordDecision(
			ctx, group.ID, group.Name, "suppressed", "runtime_unavailable", "", Impact{}, now,
		)
	}
	signals, err := service.Signals.ListFreshGroupSignals(ctx, group.Name, now)
	if err != nil {
		return service.recordDecision(
			ctx, group.ID, group.Name, "suppressed", "signal_source_unavailable", "", Impact{}, now,
		)
	}

	snapshot := Snapshot{
		GroupID: group.ID, GroupName: group.Name, ObservedAt: now,
		Runtime: &RuntimeEvidence{
			Requests:          window.Overview.RequestCountTotal,
			Successes:         window.Overview.SuccessCount,
			ErrorRate:         window.Overview.ErrorRate,
			TTFTP95MS:         window.Overview.TTFT.P95MS,
			TTFTBaselineP95MS: baseline.Overview.TTFT.P95MS,
		},
	}
	for _, signal := range signals {
		switch signal.SourceKind {
		case CapacitySourceKind:
			if !service.Policy.Enabled(notificationpolicy.FamilyGroupCapacity) {
				continue
			}
			capacity, decodeErr := DecodeCapacitySignal(signal)
			if decodeErr == nil {
				snapshot.Capacity = &capacity
			}
		case NativeMonitorSourceKind:
			if !service.Policy.Enabled(notificationpolicy.FamilyNativeMonitorEvidence) {
				continue
			}
			monitor, decodeErr := DecodeNativeMonitorSignal(signal)
			if decodeErr == nil {
				snapshot.NativeMonitor = append(snapshot.NativeMonitor, monitor)
			}
		}
	}
	impact := Evaluate(snapshot)
	incidentKey := "group:" + strconv.FormatInt(group.ID, 10) + ":user-impact"
	if service.Policy.Mode == notificationpolicy.ModeShadow {
		incidentKey = "shadow:" + incidentKey
	}

	message := service.render(group.Name, impact, "", now)
	payload, err := message.CardJSON()
	if err != nil {
		return fmt.Errorf("render group impact card for %d: %w", group.ID, err)
	}
	currentValue, err := json.Marshal(reminderSnapshot(group.Name, impact, now))
	if err != nil {
		return fmt.Errorf("encode group impact reminder snapshot: %w", err)
	}
	severity := impact.Severity
	if severity == "" {
		severity = "P1"
	}
	confirmationWindows := 2
	if severity == "P0" {
		confirmationWindows = 1
	}
	transition, err := service.Incidents.Observe(ctx, incidents.Observation{
		Key: incidentKey, Family: string(notificationpolicy.FamilyGroupRuntime),
		PolicyVersion: service.Policy.Version, SourceKind: "site_monitor",
		Severity: severity, Failing: impact.Failing,
		EvidenceHash: impact.EvidenceHash, MaterialHash: impact.MaterialHash,
		CurrentValue: string(currentValue), LatestPayload: payload,
		ConfirmationWindows: confirmationWindows, RecoveryWindows: 2,
	})
	if err != nil {
		return fmt.Errorf("observe group impact for %d: %w", group.ID, err)
	}
	if !transition.Notify {
		return service.recordDecision(
			ctx, group.ID, group.Name, "evidence_stored", "no_lifecycle_transition",
			transition.Kind, impact, now,
		)
	}
	if service.Policy.Mode == notificationpolicy.ModeShadow {
		return service.recordDecision(
			ctx, group.ID, group.Name, "shadow_would_deliver", transition.Kind,
			transition.Kind, impact, now,
		)
	}
	if !service.Policy.ShouldDeliver(notificationpolicy.FamilyGroupRuntime) {
		return service.recordDecision(
			ctx, group.ID, group.Name, "suppressed", "policy_disabled",
			transition.Kind, impact, now,
		)
	}
	if service.Notifier == nil {
		return service.recordDecision(
			ctx, group.ID, group.Name, "suppressed", "transport_unavailable",
			transition.Kind, impact, now,
		)
	}

	message = service.render(group.Name, impact, transition.Kind, now)
	message = notify.WithDeliveryIdentity(message, transition.OccurrenceNo, transition.Kind)
	if err := service.Notifier.SendIncident(
		ctx,
		incidentKey,
		transition.Kind+":"+impact.EvidenceHash,
		message,
	); err != nil {
		_ = service.recordDecision(
			ctx, group.ID, group.Name, "delivery_failed", "transport_error",
			transition.Kind, impact, now,
		)
		return err
	}
	return service.recordDecision(
		ctx, group.ID, group.Name, "delivered", transition.Kind,
		transition.Kind, impact, now,
	)
}

func (service Service) render(
	groupName string,
	impact Impact,
	transition string,
	now time.Time,
) notify.FeishuMessage {
	if transition == "recovered" {
		return notify.RenderUserImpactRecovery(notify.UserImpactRecoveryView{
			GroupName:  groupName,
			Result:     "最近两个观测均已回到健康范围。",
			Duration:   "以事故首次发现时间为准",
			Current:    currentCapacity(impact),
			ObservedAt: now,
		})
	}
	progress := transition == "progressed" || transition == "escalated"
	headline := impact.Headline
	userImpact := impact.UserImpact
	if progress && impact.Primary == PrimaryAllRequestsFailed {
		headline = "已从部分失败升级为全部失败"
		userImpact = impact.Summary
	}
	current := make([]notify.UserImpactFact, 0, len(impact.Current))
	for _, fact := range impact.Current {
		current = append(current, notify.UserImpactFact{
			Label: fact.Label, Value: fact.Value, Confirmed: fact.Confirmed,
		})
	}
	clues := make([]notify.UserImpactFact, 0, len(impact.Clues))
	for _, fact := range impact.Clues {
		clues = append(clues, notify.UserImpactFact{
			Label: fact.Label, Value: fact.Value, Confirmed: fact.Confirmed,
		})
	}
	return notify.RenderUserImpact(notify.UserImpactView{
		GroupName: groupName, Severity: impact.Severity,
		Headline: headline, Summary: impact.Summary, UserImpact: userImpact,
		Current: current, Clues: clues, Action: impact.Action,
		ObservedAt: impact.ObservedAt, Progress: progress,
	})
}

func reminderSnapshot(
	groupName string,
	impact Impact,
	observedAt time.Time,
) ReminderSnapshot {
	return ReminderSnapshot{
		GroupName: groupName, Headline: impact.Headline,
		LatestFact: impact.Summary, Capacity: currentCapacity(impact),
		ObservedAt: observedAt,
	}
}

func currentCapacity(impact Impact) string {
	for _, fact := range impact.Current {
		if fact.Label == "当前容量" {
			return fact.Value
		}
	}
	if impact.Failing {
		return "当前容量尚未取得可靠证据。"
	}
	return "未发现持续用户影响。"
}

func (service Service) recordDecision(
	ctx context.Context,
	groupID int64,
	groupName string,
	decision string,
	reason string,
	transition string,
	impact Impact,
	observedAt time.Time,
) error {
	details, _ := json.Marshal(struct {
		GroupID    int64  `json:"group_id,omitempty"`
		GroupName  string `json:"group_name,omitempty"`
		Severity   string `json:"severity,omitempty"`
		Primary    string `json:"primary,omitempty"`
		Transition string `json:"transition,omitempty"`
	}{
		GroupID: groupID, GroupName: groupName, Severity: impact.Severity,
		Primary: impact.Primary, Transition: transition,
	})
	key := "group-impact:global:last"
	if groupID > 0 {
		key = "group-impact:" + strconv.FormatInt(groupID, 10) + ":last"
	}
	return service.Decisions.RecordNotificationDecision(ctx, store.DecisionRecord{
		DecisionKey:   key,
		Family:        string(notificationpolicy.FamilyGroupRuntime),
		PolicyVersion: service.Policy.Version,
		SourceKind:    "site_monitor",
		Decision:      strings.TrimSpace(decision),
		Reason:        strings.TrimSpace(reason),
		Details:       details,
		ObservedAt:    observedAt,
	})
}
