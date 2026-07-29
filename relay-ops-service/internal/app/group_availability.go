package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"example.invalid/relay-ops-service/internal/accounthealth"
	"example.invalid/relay-ops-service/internal/dailyreport"
	"example.invalid/relay-ops-service/internal/groupimpact"
	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type groupMonitorReader interface {
	ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error)
	ListAccountMonitorHistory(context.Context, int64, int) ([]sub2api.AccountMonitorHistoryEntry, error)
}

type groupSignalSink interface {
	UpsertGroupSignal(context.Context, store.GroupSignal) error
	RecordNotificationDecision(context.Context, store.DecisionRecord) error
}

func runGroupAvailability(
	ctx context.Context,
	reader groupMonitorReader,
	sink groupSignalSink,
	policy notificationpolicy.Policy,
	now time.Time,
) error {
	if policy.Version <= 0 {
		return nil
	}
	if sink == nil {
		return fmt.Errorf("group availability signal sink is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if !policy.Enabled(notificationpolicy.FamilyGroupCapacity) {
		return sink.RecordNotificationDecision(ctx, capacityDecision(
			"capacity:policy-disabled",
			policy,
			"suppressed",
			"policy_disabled",
			now,
			map[string]string{"source": "account_monitor"},
		))
	}

	projection, readErr := reader.ListAccountMonitors(ctx)
	if readErr != nil {
		log.Printf("group availability: monitor read failed: %v", readErr)
		return sink.RecordNotificationDecision(ctx, capacityDecision(
			"capacity:source-unavailable",
			policy,
			"suppressed",
			"source_unavailable",
			now,
			map[string]string{"source": "account_monitor"},
		))
	}
	if projection.Stale {
		return sink.RecordNotificationDecision(ctx, capacityDecision(
			"capacity:source-stale",
			policy,
			"suppressed",
			"source_stale",
			now,
			map[string]string{"source": "account_monitor"},
		))
	}

	limit := accounthealth.RollingWindowLimitFor(projection.Settings.IntervalSeconds)
	histories := make(map[int64][]sub2api.AccountMonitorHistoryEntry, len(projection.Accounts))
	for _, account := range projection.Accounts {
		entries, historyErr := reader.ListAccountMonitorHistory(ctx, account.AccountID, limit)
		if historyErr != nil {
			log.Printf("group availability: history read failed for account %d: %v", account.AccountID, historyErr)
			continue
		}
		histories[account.AccountID] = entries
	}

	var failures []error
	for _, item := range dailyreport.BuildGroupAvailability(projection, histories, now) {
		groupName := item.Alert.GroupName
		if !item.Reliable {
			if err := sink.RecordNotificationDecision(ctx, capacityDecision(
				"capacity:"+groupName+":source-unavailable",
				policy,
				"suppressed",
				"source_unavailable",
				now,
				map[string]string{"group_name": groupName},
			)); err != nil {
				failures = append(failures, err)
			}
			continue
		}
		signal, err := groupimpact.CapacitySignal(groupName, item.Capacity, now)
		if err == nil {
			err = sink.UpsertGroupSignal(ctx, signal)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("store capacity signal for %s: %w", groupName, err))
			continue
		}
		if err := sink.RecordNotificationDecision(ctx, capacityDecision(
			"capacity:"+groupName+":current",
			policy,
			"evidence_stored",
			"fresh_capacity_signal",
			now,
			map[string]string{"group_name": groupName},
		)); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func capacityDecision(
	key string,
	policy notificationpolicy.Policy,
	decision string,
	reason string,
	observedAt time.Time,
	details map[string]string,
) store.DecisionRecord {
	payload, _ := json.Marshal(details)
	return store.DecisionRecord{
		DecisionKey: key, Family: string(notificationpolicy.FamilyGroupCapacity),
		PolicyVersion: policy.Version, SourceKind: groupimpact.CapacitySourceKind,
		Decision: decision, Reason: reason, Details: payload, ObservedAt: observedAt,
	}
}
