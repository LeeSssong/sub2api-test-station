package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/groupimpact"
	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/store"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type fakeGroupReader struct {
	projection      sub2api.AccountMonitorProjection
	err             error
	histories       map[int64][]sub2api.AccountMonitorHistoryEntry
	historyLimits   []int
	historyAccounts []int64
}

func (reader *fakeGroupReader) ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error) {
	return reader.projection, reader.err
}

func (reader *fakeGroupReader) ListAccountMonitorHistory(
	_ context.Context,
	accountID int64,
	limit int,
) ([]sub2api.AccountMonitorHistoryEntry, error) {
	reader.historyLimits = append(reader.historyLimits, limit)
	reader.historyAccounts = append(reader.historyAccounts, accountID)
	return reader.histories[accountID], nil
}

type fakeGroupSignalSink struct {
	signals   []store.GroupSignal
	decisions []store.DecisionRecord
	signalErr error
}

func (sink *fakeGroupSignalSink) UpsertGroupSignal(_ context.Context, signal store.GroupSignal) error {
	sink.signals = append(sink.signals, signal)
	return sink.signalErr
}

func (sink *fakeGroupSignalSink) RecordNotificationDecision(
	_ context.Context,
	decision store.DecisionRecord,
) error {
	sink.decisions = append(sink.decisions, decision)
	return nil
}

func TestRunGroupAvailabilityUpsertsOneFreshSignalPerGroup(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	reader := &fakeGroupReader{projection: groupProjection(
		healthyAccount(20, "Plus-A", "GPT PLUS 内测"),
		healthyAccount(21, "Pro-A", "GPT Pro"),
	)}
	sink := &fakeGroupSignalSink{}
	if err := runGroupAvailability(
		context.Background(), reader, sink, groupCapacityPolicy(), now,
	); err != nil {
		t.Fatal(err)
	}
	if len(sink.signals) != 2 {
		t.Fatalf("signals = %#v", sink.signals)
	}
	for _, signal := range sink.signals {
		if signal.SourceKind != "capacity" || signal.SourceKey != "current" ||
			!signal.SourceObservedAt.Equal(now) || !signal.ExpiresAt.After(now) {
			t.Fatalf("signal = %#v", signal)
		}
	}
	if len(sink.decisions) != 2 {
		t.Fatalf("decisions = %#v", sink.decisions)
	}
}

func TestRunGroupAvailabilityCountsActiveUnschedulableAccountAsUnavailable(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	reader := &fakeGroupReader{projection: groupProjection(
		healthyAccount(20, "Plus-A", "GPT PLUS 内测"),
		sub2api.AccountMonitorAccount{
			AccountID: 21, Name: "Plus-B", GroupNames: []string{"GPT PLUS 内测"},
			Status: "active", Schedulable: false, SampleCount: 20, SuccessRate: 1,
		},
	)}
	sink := &fakeGroupSignalSink{}
	if err := runGroupAvailability(
		context.Background(), reader, sink, groupCapacityPolicy(), now,
	); err != nil {
		t.Fatal(err)
	}
	if len(sink.signals) != 1 {
		t.Fatalf("signals = %#v", sink.signals)
	}
	var payload groupimpact.CapacitySignalPayload
	if err := json.Unmarshal(sink.signals[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Available != 1 || payload.Total != 2 ||
		len(payload.Unavailable) != 1 ||
		payload.Unavailable[0].Reason != "当前未参与调度" {
		t.Fatalf("capacity payload = %#v", payload)
	}
}

func TestRunGroupAvailabilityTranslatesBalanceExhaustedCause(t *testing.T) {
	t.Parallel()
	reader := &fakeGroupReader{projection: groupProjection(
		downAccount(22, "Plus-XN", "GPT PLUS 内测"),
	)}
	sink := &fakeGroupSignalSink{}
	if err := runGroupAvailability(
		context.Background(), reader, sink, groupCapacityPolicy(), time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	var payload groupimpact.CapacitySignalPayload
	if err := json.Unmarshal(sink.signals[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Unavailable) != 1 || payload.Unavailable[0].Reason != "余额耗尽" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestRunGroupAvailabilityRecordsStableSuppressionWhenSourceFails(t *testing.T) {
	t.Parallel()
	reader := &fakeGroupReader{err: errors.New("monitor endpoint unavailable")}
	sink := &fakeGroupSignalSink{}
	for index := 0; index < 2; index++ {
		if err := runGroupAvailability(
			context.Background(), reader, sink, groupCapacityPolicy(),
			time.Date(2026, 7, 29, 1, index, 0, 0, time.UTC),
		); err != nil {
			t.Fatal(err)
		}
	}
	if len(sink.signals) != 0 || len(sink.decisions) != 2 {
		t.Fatalf("signals=%#v decisions=%#v", sink.signals, sink.decisions)
	}
	if sink.decisions[0].DecisionKey != sink.decisions[1].DecisionKey ||
		sink.decisions[0].Decision != "suppressed" ||
		sink.decisions[0].Reason != "source_unavailable" {
		t.Fatalf("decisions = %#v", sink.decisions)
	}
}

func TestRunGroupAvailabilityPolicyDisabledStoresSuppressionOnly(t *testing.T) {
	t.Parallel()
	reader := &fakeGroupReader{projection: groupProjection(
		healthyAccount(20, "Plus-A", "GPT PLUS 内测"),
	)}
	sink := &fakeGroupSignalSink{}
	policy := groupCapacityPolicy()
	policy.Feishu.GroupCapacityEnabled = false
	if err := runGroupAvailability(
		context.Background(), reader, sink, policy, time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	if len(sink.signals) != 0 || len(sink.decisions) != 1 ||
		sink.decisions[0].Reason != "policy_disabled" {
		t.Fatalf("signals=%#v decisions=%#v", sink.signals, sink.decisions)
	}
}

func TestRunGroupAvailabilityStaleProjectionStoresSuppressionOnly(t *testing.T) {
	t.Parallel()
	projection := groupProjection(healthyAccount(20, "Plus-A", "GPT PLUS 内测"))
	projection.Stale = true
	reader := &fakeGroupReader{projection: projection}
	sink := &fakeGroupSignalSink{}
	if err := runGroupAvailability(
		context.Background(), reader, sink, groupCapacityPolicy(), time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	if len(sink.signals) != 0 || len(sink.decisions) != 1 ||
		sink.decisions[0].Reason != "source_stale" {
		t.Fatalf("signals=%#v decisions=%#v", sink.signals, sink.decisions)
	}
}

func TestRunGroupAvailabilityKeepsRollingHourHistoryLimit(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	reader := &fakeGroupReader{
		projection: groupProjection(healthyAccount(20, "Plus-A", "GPT PLUS 内测")),
		histories: map[int64][]sub2api.AccountMonitorHistoryEntry{
			20: {{AccountID: 20, Status: "success", CheckedAt: now.Add(-time.Minute)}},
		},
	}
	if err := runGroupAvailability(
		context.Background(), reader, &fakeGroupSignalSink{}, groupCapacityPolicy(), now,
	); err != nil {
		t.Fatal(err)
	}
	if len(reader.historyLimits) != 1 || reader.historyLimits[0] != 18 {
		t.Fatalf("history limits = %#v", reader.historyLimits)
	}
}

func groupProjection(accounts ...sub2api.AccountMonitorAccount) sub2api.AccountMonitorProjection {
	return sub2api.AccountMonitorProjection{
		SchemaVersion: 2,
		Settings:      sub2api.AccountMonitorSettings{IntervalSeconds: 300},
		Accounts:      accounts,
	}
}

func downAccount(id int64, name, group string) sub2api.AccountMonitorAccount {
	return sub2api.AccountMonitorAccount{
		AccountID: id, Name: name, GroupIDs: []int64{6}, GroupNames: []string{group},
		Status: "active", Schedulable: true,
		SampleCount: 20, SuccessRate: 0, ErrorCode: "balance_exhausted",
	}
}

func healthyAccount(id int64, name, group string) sub2api.AccountMonitorAccount {
	return sub2api.AccountMonitorAccount{
		AccountID: id, Name: name, GroupIDs: []int64{6}, GroupNames: []string{group},
		Status: "active", Schedulable: true, SampleCount: 20, SuccessRate: 1,
	}
}

func groupCapacityPolicy() notificationpolicy.Policy {
	return notificationpolicy.Policy{
		Version: 1,
		Mode:    notificationpolicy.ModeEnabled,
		Feishu: notificationpolicy.FeishuPolicy{
			GroupCapacityEnabled: true,
		},
	}
}
