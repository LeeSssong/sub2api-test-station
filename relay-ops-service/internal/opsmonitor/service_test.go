package opsmonitor

import (
	"context"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notificationpolicy"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestServiceDoesNotCreateRuntimePausedOrBalanceIncidents(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	reader := &fakeReader{
		groups: []sub2api.Group{{ID: 7, Name: "GPT PLUS 内测", Status: "active"}},
		accounts: []sub2api.Account{{
			ID: 20, Name: "Plus-A", Status: "active", Schedulable: false,
		}},
		ops: map[snapshotKey]sub2api.OpsSnapshot{
			{groupID: 7, rangeName: "15m"}: snapshot(20, 0, 1, 0),
			{groupID: 7, rangeName: "24h"}: snapshot(500, 490, .02, 1000),
		},
	}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repository.records) != 0 || len(notifier.sent) != 0 {
		t.Fatalf("legacy incident paths remain active: records=%#v sent=%#v",
			repository.records, notifier.sent)
	}
}

func TestServiceKeepsMultiplierWatcherUntilPricingEventMigration(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	value := .05
	reader := &fakeReader{accounts: []sub2api.Account{{
		ID: 20, Name: "Plus-A", Status: "active", Schedulable: true,
	}}}
	multipliers := &fakeMultiplierSource{projection: multiplierProjection(20, &value, "ok")}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)
	service.Multipliers = multipliers
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Fatalf("first trustworthy value must only establish baseline: %#v", notifier.sent)
	}
	value = .10
	multipliers.projection = multiplierProjection(20, &value, "ok")
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 ||
		notifier.sent[0].key != "site:account:20:multiplier" ||
		notifier.sent[0].message.Severity != "P2" {
		t.Fatalf("multiplier delivery = %#v", notifier.sent)
	}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("unchanged multiplier resent: %#v", notifier.sent)
	}
}

func TestServiceUsesFallbackOnlyWhenNativeMultiplierIsUnavailable(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	reader := &fakeReader{accounts: []sub2api.Account{{
		ID: 20, Name: "Plus-A", Status: "active", Schedulable: true,
	}}}
	multipliers := &fakeMultiplierSource{projection: multiplierProjection(20, nil, "failed")}
	repository := newMemoryRepository()
	service := newService(reader, repository, &fakeNotifier{}, now)
	service.Multipliers = multipliers
	service.Fallback = func(context.Context, string) *float64 {
		value := .07
		return &value
	}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	record, found, err := repository.Get(context.Background(), "site:account:20:multiplier_baseline")
	if err != nil || !found || record.CurrentValue != "0.07x" {
		t.Fatalf("fallback baseline = %#v found=%v err=%v", record, found, err)
	}
}

func TestServicePricingPolicyDisabledDoesNothing(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{accounts: []sub2api.Account{{
		ID: 20, Name: "Plus-A", Status: "active", Schedulable: true,
	}}}
	value := .05
	repository := newMemoryRepository()
	service := newService(reader, repository, &fakeNotifier{}, time.Now().UTC())
	service.Multipliers = &fakeMultiplierSource{projection: multiplierProjection(20, &value, "ok")}
	service.Policy.Feishu.PricingNoticeEnabled = false
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repository.records) != 0 {
		t.Fatalf("disabled pricing policy wrote state: %#v", repository.records)
	}
}

type fakeMultiplierSource struct {
	projection sub2api.AccountMonitorProjection
	err        error
}

func (source *fakeMultiplierSource) ListAccountMonitors(
	context.Context,
) (sub2api.AccountMonitorProjection, error) {
	return source.projection, source.err
}

func multiplierProjection(
	accountID int64,
	value *float64,
	status string,
) sub2api.AccountMonitorProjection {
	return sub2api.AccountMonitorProjection{
		SchemaVersion: 2,
		Accounts: []sub2api.AccountMonitorAccount{{
			AccountID: accountID, Name: "Plus-A",
			Multiplier: sub2api.AccountMonitorMultiplier{
				Value: value, Source: "declared", Status: status,
			},
		}},
	}
}

func newService(
	reader *fakeReader,
	repository *memoryRepository,
	notifier *fakeNotifier,
	now time.Time,
) Service {
	return Service{
		Reader: reader, Incidents: &incidents.Machine{
			Repository: repository, Policy: incidents.DefaultPolicy(),
		},
		Notifier: notifier, Policy: notificationpolicy.Policy{
			Version: 1, Mode: notificationpolicy.ModeEnabled,
			Feishu: notificationpolicy.FeishuPolicy{PricingNoticeEnabled: true},
		},
		Now: func() time.Time { return now },
	}
}

type snapshotKey struct {
	groupID, accountID int64
	rangeName          string
}

type fakeReader struct {
	groups   []sub2api.Group
	accounts []sub2api.Account
	ops      map[snapshotKey]sub2api.OpsSnapshot
}

func (reader *fakeReader) ListGroups(context.Context) ([]sub2api.Group, error) {
	return reader.groups, nil
}

func (reader *fakeReader) ListAccounts(context.Context) ([]sub2api.Account, error) {
	return reader.accounts, nil
}

func (reader *fakeReader) GetOpsSnapshot(
	_ context.Context,
	query sub2api.OpsQuery,
) (sub2api.OpsSnapshot, error) {
	return reader.ops[snapshotKey{
		groupID: query.GroupID, accountID: query.AccountID, rangeName: query.TimeRange,
	}], nil
}

type memoryRepository struct {
	records map[string]incidents.Record
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{records: map[string]incidents.Record{}}
}

func (repository *memoryRepository) Get(
	_ context.Context,
	key string,
) (incidents.Record, bool, error) {
	record, found := repository.records[key]
	return record, found, nil
}

func (repository *memoryRepository) Put(
	_ context.Context,
	record incidents.Record,
) error {
	repository.records[record.Key] = record
	return nil
}

type sentMessage struct {
	key     string
	message notify.FeishuMessage
}

type fakeNotifier struct {
	sent []sentMessage
}

func (notifier *fakeNotifier) SendIncident(
	_ context.Context,
	key string,
	_ string,
	message notify.FeishuMessage,
) error {
	notifier.sent = append(notifier.sent, sentMessage{key: key, message: message})
	return nil
}

func snapshot(
	requests int64,
	successes int64,
	errorRate float64,
	ttftP95 float64,
) sub2api.OpsSnapshot {
	return sub2api.OpsSnapshot{Overview: sub2api.OpsOverview{
		RequestCountTotal: requests, SuccessCount: successes, ErrorRate: errorRate,
		TTFT: sub2api.Percentiles{P95MS: ttftP95},
	}}
}
