package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

type accountMonitorAccountRepoStub struct {
	accounts              []Account
	schedulableAccounts   []Account
	listAllStatus         string
	listAllCalled         bool
	listSchedulableCalled bool
}

type accountMonitorRepoStub struct {
	AccountMonitorRepository
	mu                 sync.Mutex
	results            []AccountMonitorProbeResult
	settings           AccountMonitorSettings
	groups             []AccountMonitorGroup
	weights            map[int64]AccountMonitorScoreWeights
	aggregates         map[int64]AccountMonitorAggregate
	windowAggregates   map[int64]AccountMonitorWindowAggregate
	groupAggregates    map[int64]map[int64]AccountMonitorAggregate
	aggregate          AccountMonitorAggregate
	groupAggregate     map[int64]AccountMonitorAggregate
	groupAggregateIDs  map[int64][]int64
	groupAggregatesErr error
	latest             map[int64]AccountMonitorLatest
	timelines          map[int64][]AccountMonitorTimelinePoint
	aggregateIDs       []int64
	probeSince         time.Time
	probeUntil         time.Time
	windowSince        time.Time
	windowUntil        time.Time
}

func (s *accountMonitorRepoStub) InsertResult(_ context.Context, result AccountMonitorProbeResult, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, result)
	return nil
}

func (s *accountMonitorRepoStub) DeleteBefore(context.Context, time.Time) error {
	return nil
}

func (s *accountMonitorRepoStub) ListAggregates(_ context.Context, _ []int64, since, until time.Time) (map[int64]AccountMonitorAggregate, error) {
	s.probeSince = since
	s.probeUntil = until
	return s.aggregates, nil
}

func (s *accountMonitorRepoStub) ListWindowAggregates(_ context.Context, _ []int64, since, until time.Time) (map[int64]AccountMonitorWindowAggregate, error) {
	s.windowSince = since
	s.windowUntil = until
	return s.windowAggregates, nil
}

func (s *accountMonitorRepoStub) ListGroupAggregates(_ context.Context, groupID int64, _ []int64, _ time.Time) (map[int64]AccountMonitorAggregate, error) {
	if s.groupAggregatesErr != nil {
		return nil, s.groupAggregatesErr
	}
	return s.groupAggregates[groupID], nil
}

func (s *accountMonitorRepoStub) LoadAggregate(_ context.Context, accountIDs []int64, _ time.Time) (AccountMonitorAggregate, error) {
	s.aggregateIDs = append([]int64(nil), accountIDs...)
	return s.aggregate, nil
}

func (s *accountMonitorRepoStub) LoadGroupAggregate(_ context.Context, groupID int64, accountIDs []int64, _ time.Time) (AccountMonitorAggregate, error) {
	if s.groupAggregateIDs == nil {
		s.groupAggregateIDs = make(map[int64][]int64)
	}
	s.groupAggregateIDs[groupID] = append([]int64(nil), accountIDs...)
	return s.groupAggregate[groupID], nil
}

func (s *accountMonitorRepoStub) ListLatest(context.Context, []int64) (map[int64]AccountMonitorLatest, error) {
	return s.latest, nil
}

func (s *accountMonitorRepoStub) ListTimelines(context.Context, []int64, int) (map[int64][]AccountMonitorTimelinePoint, error) {
	return s.timelines, nil
}

func (s *accountMonitorRepoStub) LoadSettings(context.Context) (AccountMonitorSettings, error) {
	if s.settings.IntervalSeconds == 0 {
		return AccountMonitorSettings{}, sql.ErrNoRows
	}
	return s.settings, nil
}

func (s *accountMonitorRepoStub) ListGroups(context.Context) ([]AccountMonitorGroup, error) {
	return append([]AccountMonitorGroup(nil), s.groups...), nil
}

func (s *accountMonitorRepoStub) LoadGroupScoreWeights(_ context.Context, groupID int64) (AccountMonitorScoreWeights, error) {
	if weights, ok := s.weights[groupID]; ok {
		return weights, nil
	}
	return AccountMonitorScoreWeights{}, sql.ErrNoRows
}

func (s *accountMonitorRepoStub) SaveGroupScoreWeights(_ context.Context, groupID, actorID int64, weights AccountMonitorScoreWeights) error {
	if s.weights == nil {
		s.weights = make(map[int64]AccountMonitorScoreWeights)
	}
	weights.UpdatedBy = actorID
	s.weights[groupID] = weights
	return nil
}

func (s *accountMonitorRepoStub) ResetGroupScoreWeights(_ context.Context, groupID int64) error {
	delete(s.weights, groupID)
	return nil
}

type accountMonitorMultiplierStub struct {
	mu      sync.Mutex
	calls   []accountMonitorMultiplierCall
	err     error
	result  AccountMonitorMultiplier
	results map[int64]AccountMonitorMultiplier
}

type accountMonitorMultiplierCall struct {
	accountID int64
	force     bool
}

type accountMonitorRunResult struct {
	completed int
	err       error
}

func (s *accountMonitorMultiplierStub) Resolve(account *Account, _ time.Time) AccountMonitorMultiplier {
	if account != nil {
		if result, ok := s.results[account.ID]; ok {
			return result
		}
	}
	return s.result
}

func (s *accountMonitorMultiplierStub) Refresh(_ context.Context, account *Account, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, accountMonitorMultiplierCall{accountID: account.ID, force: force})
	return s.err
}

func accountMonitorConfirmedMultiplier(value float64) *accountMonitorMultiplierStub {
	return &accountMonitorMultiplierStub{result: AccountMonitorMultiplier{
		Value:  &value,
		Status: AccountMonitorMultiplierStatusOK,
	}}
}

func (s *accountMonitorAccountRepoStub) ListSchedulable(context.Context) ([]Account, error) {
	s.listSchedulableCalled = true
	return append([]Account(nil), s.schedulableAccounts...), nil
}

func (s *accountMonitorAccountRepoStub) ListAllWithFilters(
	_ context.Context,
	_ string,
	_ string,
	status string,
	_ string,
	_ int64,
	_ string,
) ([]Account, error) {
	s.listAllCalled = true
	s.listAllStatus = status
	return append([]Account(nil), s.accounts...), nil
}

func TestAccountMonitorListPoolUsesPersistedActiveSchedulableFlags(t *testing.T) {
	future := time.Now().Add(time.Hour)
	repo := &accountMonitorAccountRepoStub{
		accounts: []Account{
			{ID: 9, Status: StatusActive, Schedulable: true, RateLimitResetAt: &future},
			{ID: 2, Status: StatusActive, Schedulable: true, TempUnschedulableUntil: &future},
			{ID: 4, Status: StatusDisabled, Schedulable: true},
			{ID: 6, Status: StatusActive, Schedulable: false},
		},
		schedulableAccounts: []Account{
			{ID: 9, Status: StatusActive, Schedulable: true},
		},
	}
	service := NewAccountMonitorService(nil, repo, nil, nil, nil)

	accounts, err := service.listPool(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 || accounts[0].ID != 2 || accounts[1].ID != 9 {
		t.Fatalf("accounts = %#v", accounts)
	}
	if !repo.listAllCalled || repo.listAllStatus != "" {
		t.Fatalf("ListAllWithFilters must not use the composite active filter: %#v", repo)
	}
	if repo.listSchedulableCalled {
		t.Fatal("account monitor must not use runtime scheduler eligibility")
	}
}

func TestAccountMonitorModelUsesMappedTextModel(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"gpt-4o-mini":            "upstream-small",
			"gpt-5.2-codex":          "upstream-codex",
			"gpt-5.4":                "upstream-latest",
			"gpt-*":                  "upstream-wildcard",
			"gpt-image-1":            "upstream-image",
			"text-embedding-3-large": "upstream-embedding",
		}},
	}

	if got := monitorModelForAccount(account); got != "gpt-5.4" {
		t.Fatalf("monitorModelForAccount() = %q, want gpt-5.4", got)
	}
}

func TestAccountMonitorModelFallsBackToNativePlatformDefaults(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     string
	}{
		{name: "anthropic", platform: PlatformAnthropic, want: claude.DefaultTestModel},
		{name: "openai", platform: PlatformOpenAI, want: openai.DefaultTestModel},
		{name: "gemini", platform: PlatformGemini, want: geminicli.DefaultTestModel},
		{name: "grok", platform: PlatformGrok, want: xai.DefaultModels()[0].ID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform: tt.platform,
				Credentials: map[string]any{"model_mapping": map[string]any{
					"*":                      "wildcard",
					"gpt-image-1":            "image",
					"text-embedding-3-small": "embedding",
					"grok-imagine-video-1.5": "video",
				}},
			}
			if got := monitorModelForAccount(account); got != tt.want {
				t.Fatalf("monitorModelForAccount() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAccountMonitorListPoolKeepsOnlyActiveSchedulableAccounts(t *testing.T) {
	service := NewAccountMonitorService(nil, &accountMonitorAccountRepoStub{accounts: []Account{
		{ID: 9, Status: StatusActive, Schedulable: true},
		{ID: 2, Status: StatusActive, Schedulable: true},
		{ID: 4, Status: StatusDisabled, Schedulable: true},
		{ID: 6, Status: StatusActive, Schedulable: false},
	}}, nil, nil, nil)

	accounts, err := service.listPool(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 || accounts[0].ID != 2 || accounts[1].ID != 9 {
		t.Fatalf("accounts = %#v", accounts)
	}
}

func TestAccountMonitorListDeduplicatesAccountsByIDWithStableOrder(t *testing.T) {
	service := NewAccountMonitorService(nil, &accountMonitorAccountRepoStub{accounts: []Account{
		{ID: 9, Name: "second source"},
		{ID: 2, Name: "first source"},
		{ID: 2, Name: "duplicate source"},
		{ID: 9, Name: "duplicate second"},
	}}, nil, nil, nil)

	accounts, err := service.listMonitorAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 || accounts[0].ID != 2 || accounts[1].ID != 9 {
		t.Fatalf("accounts = %#v, want one stable row per ID", accounts)
	}
	if accounts[0].Name != "first source" || accounts[1].Name != "second source" {
		t.Fatalf("deduplication did not preserve first source row: %#v", accounts)
	}
}

func TestAccountMonitorServiceRejectsScoreWeightsThatDoNotSumTo100(t *testing.T) {
	svc := NewAccountMonitorService(&accountMonitorRepoStub{}, &accountMonitorAccountRepoStub{}, nil, nil, nil)
	_, err := svc.UpdateGroupScoreWeights(context.Background(), 7, 3, AccountMonitorScoreWeights{
		Cost: 20, Success: 30, TTFT: 20, Latency: 20,
	})
	if err == nil || !strings.Contains(err.Error(), "sum to 100") {
		t.Fatalf("err = %v, want sum to 100 validation", err)
	}
}

func TestAccountMonitorServiceUpdatesAndResetsGroupScoreWeights(t *testing.T) {
	repo := &accountMonitorRepoStub{}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil)

	updated, err := svc.UpdateGroupScoreWeights(context.Background(), 7, 3, AccountMonitorScoreWeights{
		Cost: 20, Success: 40, TTFT: 20, Latency: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Cost != 20 || updated.Success != 40 || updated.TTFT != 20 || updated.Latency != 20 || updated.UpdatedBy != 3 {
		t.Fatalf("updated weights = %#v", updated)
	}

	reset, err := svc.ResetGroupScoreWeights(context.Background(), 7, 3)
	if err != nil {
		t.Fatal(err)
	}
	if reset != DefaultAccountMonitorScoreWeights {
		t.Fatalf("reset weights = %#v, want %#v", reset, DefaultAccountMonitorScoreWeights)
	}
}

func TestAccountMonitorServiceProjectsNativeGroupVisibilityAndWeights(t *testing.T) {
	updatedAt := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups: []AccountMonitorGroup{
			{ID: 7, Name: "public", RateMultiplier: 1.25, CustomerVisible: true, NativeOrder: 4,
				ScoreWeights: AccountMonitorScoreWeights{Cost: 20, Success: 40, TTFT: 20, Latency: 20, UpdatedBy: 3, UpdatedAt: updatedAt}},
			{ID: 8, Name: "exclusive", RateMultiplier: 2, CustomerVisible: false, NativeOrder: 9,
				ScoreWeights: DefaultAccountMonitorScoreWeights},
		},
	}
	accounts := &accountMonitorAccountRepoStub{accounts: []Account{{ID: 11, Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI}}}
	svc := NewAccountMonitorService(repo, accounts, nil, nil, nil)
	page, err := svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Groups) != 2 || !page.Groups[0].CustomerVisible || page.Groups[1].CustomerVisible {
		t.Fatalf("groups = %#v", page.Groups)
	}
	if page.Groups[0].RateMultiplier != 1.25 || page.Groups[0].NativeOrder != 4 || page.Groups[0].ScoreWeights.Success != 40 {
		t.Fatalf("group projection = %#v", page.Groups[0])
	}
}

func TestAccountMonitorProjectionIgnoresUnprobedPausedAccountsWhenDeterminingStaleness(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.5
	accounts := []Account{
		{ID: 1, Name: "paused-without-probe", Status: StatusDisabled, Schedulable: false, RateMultiplier: &rate},
		{ID: 2, Name: "fresh-enabled", Status: StatusActive, Schedulable: true, RateMultiplier: &rate},
	}
	repo := &accountMonitorRepoStub{
		settings:   AccountMonitorSettings{IntervalSeconds: 300},
		aggregates: map[int64]AccountMonitorAggregate{2: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now}},
		latest:     map[int64]AccountMonitorLatest{2: {Status: "success", CheckedAt: now}},
	}

	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if page.Stale {
		t.Fatalf("paused account without a probe must not make a fresh monitor projection stale: %#v", page)
	}
}

func TestAccountMonitorProjectionSeparatesManagementServiceAndGroupEligibility(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	cheapRate := 0.5
	expensiveRate := 2.0
	confirmedCheap := AccountMonitorMultiplier{Value: &cheapRate, Status: AccountMonitorMultiplierStatusOK}
	confirmedExpensive := AccountMonitorMultiplier{Value: &expensiveRate, Status: AccountMonitorMultiplierStatusOK}
	accounts := []Account{
		{ID: 1, Name: "disabled", Status: StatusDisabled, Schedulable: true, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
		{ID: 2, Name: "unschedulable", Status: StatusActive, Schedulable: false, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
		{ID: 3, Name: "available", Status: StatusActive, Schedulable: true, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
		{ID: 4, Name: "unavailable", Status: StatusActive, Schedulable: true, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
		{ID: 5, Name: "pending", Status: StatusActive, Schedulable: true, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
		{ID: 6, Name: "cost-ineligible", Status: StatusActive, Schedulable: true, RateMultiplier: &expensiveRate, GroupIDs: []int64{7}},
		{ID: 7, Name: "temporarily-paused", Status: StatusActive, Schedulable: true, TempUnschedulableUntil: &future, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
		{ID: 8, Name: "multiplier-pending", Status: StatusActive, Schedulable: true, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "closed", RateMultiplier: 1, CustomerVisible: false}},
		aggregates: map[int64]AccountMonitorAggregate{
			1: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			2: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			3: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			4: {SampleCount: 4, SuccessRate: 0, LastCheckedAt: &now},
			6: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			7: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			8: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
		},
		groupAggregates: map[int64]map[int64]AccountMonitorAggregate{7: {
			1: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			2: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			3: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			4: {SampleCount: 4, SuccessRate: 0, LastCheckedAt: &now},
			6: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			7: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			8: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
		}},
		latest: map[int64]AccountMonitorLatest{
			1: {Status: "success", CheckedAt: now},
			2: {Status: "success", CheckedAt: now},
			3: {Status: "success", CheckedAt: now},
			4: {Status: "failed", CheckedAt: now},
			6: {Status: "success", CheckedAt: now},
			7: {Status: "success", CheckedAt: now},
			8: {Status: "success", CheckedAt: now},
		},
	}
	multiplier := &accountMonitorMultiplierStub{results: map[int64]AccountMonitorMultiplier{
		1: confirmedCheap, 2: confirmedCheap, 3: confirmedCheap, 4: confirmedCheap,
		5: confirmedCheap, 6: confirmedExpensive, 7: confirmedCheap,
		8: {Status: AccountMonitorMultiplierStatusUnavailable},
	}}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, multiplier).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	global := accountMonitorRowsByID(page.Accounts)
	if global[1].ManagementState != "paused" || global[1].ServiceState != "not_monitored" || global[1].GroupEligibility != "not_applicable" || global[1].MonitorBucket != "paused" {
		t.Fatalf("paused global state = %#v", global[1])
	}
	if global[3].ManagementState != "enabled" || global[3].ServiceState != "available" || global[3].GroupEligibility != "not_applicable" || global[3].MonitorBucket != "available" {
		t.Fatalf("available global state = %#v", global[3])
	}
	if global[4].ServiceState != "unavailable" || global[4].MonitorBucket != "unavailable" {
		t.Fatalf("unavailable global state = %#v", global[4])
	}
	if global[5].ServiceState != "pending" || global[5].MonitorBucket != "pending" {
		t.Fatalf("pending global state = %#v", global[5])
	}
	encoded, err := json.Marshal(global[3])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["management_state"] != "enabled" || payload["service_state"] != "available" || payload["group_eligibility"] != "not_applicable" || payload["monitor_bucket"] != "available" {
		t.Fatalf("independent state JSON contract = %#v", payload)
	}
	if page.Health.MonitoringAccounts != 5 || page.Health.AvailableAccounts != 3 || page.Health.UnavailableAccounts != 1 || page.Health.PendingAccounts != 1 || page.Health.PausedAccounts != 3 {
		t.Fatalf("global health = %#v", page.Health)
	}

	group := page.Groups[0]
	rows := accountMonitorGroupRowsByID(group.Accounts)
	if rows[3].ManagementState != "enabled" || rows[3].ServiceState != "available" || rows[3].GroupEligibility != "eligible" || !rows[3].Eligible || rows[3].QualityScore == nil || rows[3].GroupRank == nil {
		t.Fatalf("eligible available group state = %#v", rows[3])
	}
	if rows[4].ServiceState != "unavailable" || rows[4].GroupEligibility != "eligible" || rows[4].Eligible || rows[4].QualityScore != nil || rows[4].GroupRank != nil {
		t.Fatalf("failed account must not rank: %#v", rows[4])
	}
	if rows[6].ServiceState != "available" || rows[6].GroupEligibility != "cost_ineligible" || rows[6].MonitorBucket != "cost_ineligible" || rows[6].Eligible || rows[6].QualityScore != nil || rows[6].GroupRank != nil {
		t.Fatalf("cost-ineligible service state = %#v", rows[6])
	}
	if rows[8].ServiceState != "available" || rows[8].GroupEligibility != "multiplier_pending" || rows[8].MonitorBucket != "pending" || rows[8].Eligible || rows[8].QualityScore != nil || rows[8].GroupRank != nil {
		t.Fatalf("multiplier-pending group state = %#v", rows[8])
	}
	if group.Health.MonitoringAccounts != 5 || group.Health.AvailableAccounts != 3 || group.Health.UnavailableAccounts != 1 || group.Health.PendingAccounts != 1 || group.Health.PausedAccounts != 3 {
		t.Fatalf("group health must not convert cost ineligibility into service failure: %#v", group.Health)
	}
	if group.Health.AvailableAccounts+group.Health.UnavailableAccounts+group.Health.PendingAccounts+group.Health.PausedAccounts != group.Health.TotalAccounts {
		t.Fatalf("group health is not a four-way partition: %#v", group.Health)
	}
}

func accountMonitorRowsByID(rows []AccountMonitorAccount) map[int64]AccountMonitorAccount {
	byID := make(map[int64]AccountMonitorAccount, len(rows))
	for _, row := range rows {
		byID[row.AccountID] = row
	}
	return byID
}

func accountMonitorGroupRowsByID(rows []AccountMonitorGroupAccount) map[int64]AccountMonitorGroupAccount {
	byID := make(map[int64]AccountMonitorGroupAccount, len(rows))
	for _, row := range rows {
		byID[row.AccountID] = row
	}
	return byID
}

func TestAccountMonitorProjectionBucketsEveryAccountAndPreservesClosedGroupAccounts(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	expired := now.Add(-time.Hour)
	cheapRate := 0.5
	expensiveRate := 2.0
	accounts := []Account{
		{ID: 1, Name: "disabled", Status: StatusDisabled, Schedulable: true, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
		{ID: 2, Name: "unschedulable", Status: StatusActive, Schedulable: false, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
		{ID: 3, Name: "available", Status: StatusActive, Schedulable: true, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
		{ID: 4, Name: "unavailable", Status: StatusActive, Schedulable: true, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
		{ID: 5, Name: "pending", Status: StatusActive, Schedulable: true, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
		{ID: 6, Name: "cost-ineligible", Status: StatusActive, Schedulable: true, RateMultiplier: &expensiveRate, GroupIDs: []int64{7}},
		{ID: 7, Name: "temporarily-paused", Status: StatusActive, Schedulable: true, TempUnschedulableUntil: &future, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
		{ID: 8, Name: "expired-auto-pause", Status: StatusActive, Schedulable: true, AutoPauseOnExpired: true, ExpiresAt: &expired, RateMultiplier: &cheapRate, GroupIDs: []int64{7}},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "closed", RateMultiplier: 1, CustomerVisible: false}},
		aggregates: map[int64]AccountMonitorAggregate{
			1: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			2: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			3: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			4: {SampleCount: 4, SuccessRate: 0, LastCheckedAt: &now},
			6: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			7: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			8: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
		},
		groupAggregates: map[int64]map[int64]AccountMonitorAggregate{7: {
			1: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			2: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			3: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			4: {SampleCount: 4, SuccessRate: 0, LastCheckedAt: &now},
			6: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			7: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
			8: {SampleCount: 4, SuccessRate: 1, LastCheckedAt: &now},
		}},
		latest: map[int64]AccountMonitorLatest{
			1: {Status: "success", CheckedAt: now},
			2: {Status: "success", CheckedAt: now},
			3: {Status: "success", CheckedAt: now},
			4: {Status: "failed", CheckedAt: now},
			6: {Status: "success", CheckedAt: now},
			7: {Status: "success", CheckedAt: now},
			8: {Status: "success", CheckedAt: now},
		},
	}

	confirmedCheap := AccountMonitorMultiplier{Value: &cheapRate, Status: AccountMonitorMultiplierStatusOK}
	confirmedExpensive := AccountMonitorMultiplier{Value: &expensiveRate, Status: AccountMonitorMultiplierStatusOK}
	multiplier := &accountMonitorMultiplierStub{results: map[int64]AccountMonitorMultiplier{
		1: confirmedCheap, 2: confirmedCheap, 3: confirmedCheap, 4: confirmedCheap,
		5: confirmedCheap, 6: confirmedExpensive, 7: confirmedCheap, 8: confirmedCheap,
	}}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, multiplier).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Accounts) != 8 {
		t.Fatalf("global accounts = %#v, want every account", page.Accounts)
	}
	globalBuckets := make(map[int64]string, len(page.Accounts))
	for _, account := range page.Accounts {
		globalBuckets[account.AccountID] = account.MonitorBucket
	}
	if want := map[int64]string{1: "paused", 2: "paused", 3: "available", 4: "unavailable", 5: "pending", 6: "available", 7: "paused", 8: "paused"}; !mapsEqual(globalBuckets, want) {
		t.Fatalf("global monitor buckets = %#v, want %#v", globalBuckets, want)
	}
	encoded, err := json.Marshal(page.Accounts[0])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["monitor_bucket"] != "paused" {
		t.Fatalf("monitor bucket JSON contract = %#v", payload)
	}
	if page.Health.AvailableAccounts+page.Health.UnavailableAccounts+page.Health.PendingAccounts+page.Health.PausedAccounts != page.Health.TotalAccounts {
		t.Fatalf("global health is not a four-way partition: %#v", page.Health)
	}

	group := page.Groups[0]
	if group.OperationalState != "closed" || len(group.Accounts) != 8 {
		t.Fatalf("closed group projection = %#v", group)
	}
	groupBuckets := make(map[int64]string, len(group.Accounts))
	for _, account := range group.Accounts {
		groupBuckets[account.AccountID] = account.MonitorBucket
	}
	if want := map[int64]string{1: "paused", 2: "paused", 3: "available", 4: "unavailable", 5: "pending", 6: "cost_ineligible", 7: "paused", 8: "paused"}; !mapsEqual(groupBuckets, want) {
		t.Fatalf("group monitor buckets = %#v, want %#v", groupBuckets, want)
	}
	if group.Health.AvailableAccounts+group.Health.UnavailableAccounts+group.Health.PendingAccounts+group.Health.PausedAccounts != group.Health.TotalAccounts {
		t.Fatalf("group health is not a four-way partition: %#v", group.Health)
	}
	if group.Health.UnavailableAccounts != 1 || group.Health.AvailableAccounts != 2 {
		t.Fatalf("cost-ineligible group account must preserve service health: %#v", group.Health)
	}
}

func mapsEqual(left, right map[int64]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		if rightValue, ok := right[key]; !ok || rightValue != leftValue {
			return false
		}
	}
	return true
}

func TestAccountMonitorProjectionIncludesGlobalAndGroupHealthSummaries(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.02
	paused := Account{ID: 12, Name: "paused", Status: StatusActive, Schedulable: false, GroupIDs: []int64{7}, RateMultiplier: &rate}
	ready := Account{ID: 13, Name: "ready", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate}
	pending := Account{ID: 14, Name: "pending", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		aggregates: map[int64]AccountMonitorAggregate{
			12: {SampleCount: 10, SuccessRate: 0.5, TTFTP50MS: floatPtr(500), LatencyP95MS: floatPtr(1500), LastCheckedAt: &now},
			13: {SampleCount: 10, SuccessRate: 0.9, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(400), LastCheckedAt: &now},
			14: {SampleCount: 0, SuccessRate: 0},
		},
		latest: map[int64]AccountMonitorLatest{
			12: {Status: "success", CheckedAt: now},
			13: {Status: "success", CheckedAt: now},
		},
		groupAggregates: map[int64]map[int64]AccountMonitorAggregate{7: {
			12: {SampleCount: 10, SuccessRate: 0.5, TTFTP50MS: floatPtr(500), LatencyP95MS: floatPtr(1500), LastCheckedAt: &now},
			13: {SampleCount: 10, SuccessRate: 0.9, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(400), LastCheckedAt: &now},
		}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{paused, ready, pending}}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if page.Health.TotalAccounts != 3 || page.Health.AvailableAccounts != 1 || page.Health.PausedAccounts != 1 || page.Health.PendingAccounts != 1 {
		t.Fatalf("global health = %#v", page.Health)
	}
	if page.Health.SuccessRate <= 0.6 || page.Health.TTFTP50MS == nil || page.Health.LatencyP95MS == nil {
		t.Fatalf("global quality health = %#v", page.Health)
	}
	if page.Groups[0].Health.TotalAccounts != 3 || page.Groups[0].Health.AvailableAccounts != 1 || page.Groups[0].Health.PausedAccounts != 1 || page.Groups[0].Health.PendingAccounts != 1 {
		t.Fatalf("group health = %#v", page.Groups[0].Health)
	}
}

func TestAccountMonitorGroupQualityEvidenceUsesGroupCostAndIgnoresPriority(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.02
	groupOne := &Group{ID: 1, Name: "standard", RateMultiplier: 1.0}
	groupTwo := &Group{ID: 2, Name: "discount", RateMultiplier: 0.10}
	account := Account{ID: 41, Name: "shared", Status: StatusActive, Schedulable: true, Priority: 99,
		RateMultiplier: &rate, Platform: PlatformOpenAI, GroupIDs: []int64{1, 2}, Groups: []*Group{groupOne, groupTwo}}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups: []AccountMonitorGroup{
			{ID: 1, Name: "standard", RateMultiplier: 1.0, CustomerVisible: true,
				ScoreWeights: AccountMonitorScoreWeights{Cost: 60, Success: 20, TTFT: 10, Latency: 10}},
			{ID: 2, Name: "discount", RateMultiplier: 0.10, CustomerVisible: true,
				ScoreWeights: AccountMonitorScoreWeights{Cost: 10, Success: 70, TTFT: 10, Latency: 10}},
		},
		aggregates: map[int64]AccountMonitorAggregate{41: {
			SampleCount: 4, SuccessRate: 0.9, LastCheckedAt: &now,
			TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(400),
		}},
		groupAggregates: map[int64]map[int64]AccountMonitorAggregate{
			1: {41: {SampleCount: 4, SuccessRate: 0.9, LastCheckedAt: &now, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(400)}},
			2: {41: {SampleCount: 4, SuccessRate: 0.9, LastCheckedAt: &now, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(400)}},
		},
		latest: map[int64]AccountMonitorLatest{41: {Status: "success", CheckedAt: now}},
	}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{account}}
	page, err := NewAccountMonitorService(repo, accountRepo, nil, nil, accountMonitorConfirmedMultiplier(rate)).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Groups) != 2 || len(page.Groups[0].Accounts) != 1 || len(page.Groups[1].Accounts) != 1 {
		t.Fatalf("group accounts = %#v", page.Groups)
	}
	first, second := page.Groups[0].Accounts[0], page.Groups[1].Accounts[0]
	if first.Priority != second.Priority || first.Priority != 99 {
		t.Fatalf("priority leaked into group projection: first=%d second=%d", first.Priority, second.Priority)
	}
	if first.QualityScore == nil || second.QualityScore == nil || *first.QualityScore <= *second.QualityScore {
		t.Fatalf("cost-weighted quality scores = %v, %v", first.QualityScore, second.QualityScore)
	}
	if !first.Eligible || first.GroupRank == nil || *first.GroupRank != 1 {
		t.Fatalf("first group account = %#v", first)
	}
}

func TestCalculateAccountMonitorQualityScoreUsesConfiguredLinearThresholds(t *testing.T) {
	ttft := 3000.0
	latency := 35000.0
	score := CalculateAccountMonitorQualityScore(1, 1, AccountMonitorScoreWeights{
		Cost: 0, Success: 40, TTFT: 30, Latency: 30,
		TTFTTargetMS: 1000, TTFTLimitMS: 5000,
		LatencyTargetMS: 10000, LatencyLimitMS: 60000,
	}, AccountMonitorQualityEvidence{
		Source: "group", SuccessRate: 0.75, TTFTP50MS: &ttft, LatencyP95MS: &latency,
	})
	if score == nil || *score != 60 {
		t.Fatalf("score = %v, want 60 (30 success + 15 TTFT + 15 latency)", score)
	}

	ttft = 5000
	latency = 60000
	score = CalculateAccountMonitorQualityScore(1, 1, AccountMonitorScoreWeights{
		Cost: 0, Success: 40, TTFT: 30, Latency: 30,
		TTFTTargetMS: 1000, TTFTLimitMS: 5000,
		LatencyTargetMS: 10000, LatencyLimitMS: 60000,
	}, AccountMonitorQualityEvidence{
		Source: "group", SuccessRate: 0.75, TTFTP50MS: &ttft, LatencyP95MS: &latency,
	})
	if score == nil || *score != 30 {
		t.Fatalf("score at upper limits = %v, want success component only", score)
	}
}

func TestAccountMonitorProjectionRoundsScoreAndKeepsMetricSpecificEvidence(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.02
	account := Account{ID: 71, Name: "precise", Status: StatusActive, Schedulable: true,
		RateMultiplier: &rate, Platform: PlatformOpenAI, GroupIDs: []int64{7}}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups: []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true,
			ScoreWeights: AccountMonitorScoreWeights{Cost: 15, Success: 45, TTFT: 20, Latency: 20,
				TTFTTargetMS: 1000, TTFTLimitMS: 5000, LatencyTargetMS: 10000, LatencyLimitMS: 60000}}},
		aggregates: map[int64]AccountMonitorAggregate{71: {
			SampleCount: 7, SuccessSampleCount: 7, TTFTSampleCount: 5, LatencySampleCount: 4,
			SuccessRate: 6.0 / 7.0, TTFTP50MS: floatPtr(2000), LatencyP95MS: floatPtr(20000), LastCheckedAt: &now,
		}},
		groupAggregates: map[int64]map[int64]AccountMonitorAggregate{7: {71: {
			SampleCount: 7, SuccessSampleCount: 7, TTFTSampleCount: 5, LatencySampleCount: 4,
			SuccessRate: 6.0 / 7.0, TTFTP50MS: floatPtr(2000), LatencyP95MS: floatPtr(20000), LastCheckedAt: &now,
		}}},
		latest: map[int64]AccountMonitorLatest{71: {Status: "success", CheckedAt: now}},
		timelines: map[int64][]AccountMonitorTimelinePoint{71: {
			{Status: "failed", CheckedAt: now.Add(-time.Minute)},
			{Status: "success", TTFTMS: floatPtr(2000), LatencyMS: floatPtr(20000), CheckedAt: now},
		}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, accountMonitorConfirmedMultiplier(rate)).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row := page.Groups[0].Accounts[0]
	if row.Evidence.SuccessSampleCount != 7 || row.Evidence.TTFTSampleCount != 5 || row.Evidence.LatencySampleCount != 4 {
		t.Fatalf("metric-specific evidence = %#v", row.Evidence)
	}
	if len(row.Timeline) != 2 || row.Timeline[0].Status != "failed" || row.Timeline[1].Status != "success" {
		t.Fatalf("timeline = %#v", row.Timeline)
	}
	if row.QualityScore == nil || *row.QualityScore != math.Round(*row.QualityScore) {
		t.Fatalf("quality score must be an integer: %v", row.QualityScore)
	}
}

func TestCalculateAccountMonitorQualityScoreRoundsToNearestInteger(t *testing.T) {
	score := CalculateAccountMonitorQualityScore(1, 1, AccountMonitorScoreWeights{
		Cost: 0, Success: 33, TTFT: 0, Latency: 0,
	}, AccountMonitorQualityEvidence{
		Source: "group", SuccessRate: 0.5,
	})
	if score == nil || *score != 17 {
		t.Fatalf("score = %v, want 17 after rounding 16.5", score)
	}

	score = CalculateAccountMonitorQualityScore(1, 1, AccountMonitorScoreWeights{
		Cost: 0, Success: 33, TTFT: 0, Latency: 0,
	}, AccountMonitorQualityEvidence{
		Source: "group", SuccessRate: 0.49,
	})
	if score == nil || *score != 16 {
		t.Fatalf("score = %v, want 16 after rounding 16.17", score)
	}
}

func TestAccountMonitorScoreThresholdsRejectInvalidRanges(t *testing.T) {
	repo := &accountMonitorRepoStub{}
	_, err := NewAccountMonitorService(repo, nil, nil, nil, nil).UpdateGroupScoreWeights(context.Background(), 7, 3, AccountMonitorScoreWeights{
		Cost: 15, Success: 45, TTFT: 20, Latency: 20,
		TTFTTargetMS: 5000, TTFTLimitMS: 5000,
		LatencyTargetMS: 10000, LatencyLimitMS: 60000,
	})
	if err == nil {
		t.Fatal("expected equal TTFT target and limit to be rejected")
	}
}

func TestAccountMonitorGroupQualityEvidenceFallsBackAndMarksStale(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.02
	group := &Group{ID: 7, Name: "public", RateMultiplier: 1.0}
	account := Account{ID: 52, Name: "fallback", Status: StatusActive, Schedulable: true,
		RateMultiplier: &rate, Platform: PlatformOpenAI, GroupIDs: []int64{7}, Groups: []*Group{group}}
	repo := &accountMonitorRepoStub{
		settings:        AccountMonitorSettings{IntervalSeconds: 300},
		groups:          []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1.0, CustomerVisible: true}},
		aggregates:      map[int64]AccountMonitorAggregate{52: {SampleCount: 4, SuccessRate: 0.75, LastCheckedAt: &now, TTFTP50MS: floatPtr(80), LatencyP95MS: floatPtr(300)}},
		groupAggregates: map[int64]map[int64]AccountMonitorAggregate{7: {52: {SampleCount: 1, SuccessRate: 1, LastCheckedAt: &now}}},
		latest:          map[int64]AccountMonitorLatest{52: {Status: "success", CheckedAt: now}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, accountMonitorConfirmedMultiplier(rate)).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row := page.Groups[0].Accounts[0]
	if row.Evidence.Source != "global_fallback" || row.Evidence.SampleCount != 4 || !row.Eligible {
		t.Fatalf("fallback evidence = %#v", row)
	}

	stale := now.Add(-20 * time.Minute)
	repo.aggregates[52] = AccountMonitorAggregate{SampleCount: 4, SuccessRate: 0.75, LastCheckedAt: &stale, TTFTP50MS: floatPtr(80), LatencyP95MS: floatPtr(300)}
	page, err = NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, accountMonitorConfirmedMultiplier(rate)).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row = page.Groups[0].Accounts[0]
	if row.Evidence.Source != "stale" || row.Eligible || row.QualityScore != nil {
		t.Fatalf("stale evidence = %#v", row)
	}
}

func TestAccountMonitorGroupQualityEvidenceErrorForcesGlobalFallback(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.02
	account := Account{ID: 53, Name: "group-error", Status: StatusActive, Schedulable: true,
		RateMultiplier: &rate, Platform: PlatformOpenAI, GroupIDs: []int64{7}}
	repo := &accountMonitorRepoStub{
		settings:           AccountMonitorSettings{IntervalSeconds: 300},
		groups:             []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1.0, CustomerVisible: true}},
		aggregates:         map[int64]AccountMonitorAggregate{53: {SampleCount: 4, SuccessRate: 0.75, LastCheckedAt: &now}},
		groupAggregatesErr: errors.New("group evidence unavailable"),
		latest:             map[int64]AccountMonitorLatest{53: {Status: "success", CheckedAt: now}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	evidence := page.Groups[0].Accounts[0].Evidence
	if evidence.Source != "global_fallback" {
		t.Fatalf("evidence source = %q, want global_fallback", evidence.Source)
	}
}

func TestAccountMonitorGroupQualityEvidenceUsesAggregateTimestampForStaleness(t *testing.T) {
	now := time.Now().UTC()
	stale := now.Add(-20 * time.Minute)
	rate := 0.02
	account := Account{ID: 54, Name: "stale-group", Status: StatusActive, Schedulable: true,
		RateMultiplier: &rate, Platform: PlatformOpenAI, GroupIDs: []int64{7}}
	repo := &accountMonitorRepoStub{
		settings:        AccountMonitorSettings{IntervalSeconds: 300},
		groups:          []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1.0, CustomerVisible: true}},
		aggregates:      map[int64]AccountMonitorAggregate{54: {SampleCount: 4, SuccessRate: 0.75, LastCheckedAt: &now}},
		groupAggregates: map[int64]map[int64]AccountMonitorAggregate{7: {54: {SampleCount: 4, SuccessRate: 0.9, LastCheckedAt: &stale}}},
		latest:          map[int64]AccountMonitorLatest{54: {Status: "success", CheckedAt: now}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row := page.Groups[0].Accounts[0]
	if row.Evidence.Source != "stale" || row.Eligible || row.QualityScore != nil {
		t.Fatalf("stale group evidence = %#v", row)
	}
}

func TestAccountMonitorGroupQualityEvidenceExcludesExpiredAutoPausedAccount(t *testing.T) {
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	rate := 0.02
	account := Account{ID: 55, Name: "expired", Status: StatusActive, Schedulable: true,
		AutoPauseOnExpired: true, ExpiresAt: &expired, RateMultiplier: &rate,
		Platform: PlatformOpenAI, GroupIDs: []int64{7}}
	repo := &accountMonitorRepoStub{
		settings:        AccountMonitorSettings{IntervalSeconds: 300},
		groups:          []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1.0, CustomerVisible: true}},
		aggregates:      map[int64]AccountMonitorAggregate{55: {SampleCount: 4, SuccessRate: 0.9, LastCheckedAt: &now}},
		groupAggregates: map[int64]map[int64]AccountMonitorAggregate{7: {55: {SampleCount: 4, SuccessRate: 0.9, LastCheckedAt: &now}}},
		latest:          map[int64]AccountMonitorLatest{55: {Status: "success", CheckedAt: now}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if page.Groups[0].Accounts[0].Eligible {
		t.Fatalf("expired auto-paused account unexpectedly eligible: %#v", page.Groups[0].Accounts[0])
	}
}

func TestAccountMonitorClosedGroupWithNoAccountsIsNotAFalseRedFailure(t *testing.T) {
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 88, Name: "closed", RateMultiplier: 1.0, CustomerVisible: false}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Groups) != 1 || page.Groups[0].OperationalState != "closed" || len(page.Groups[0].Accounts) != 0 {
		t.Fatalf("closed group = %#v", page.Groups)
	}
}

func TestAccountMonitorHealthUsesExactCombinedPercentiles(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.02
	accounts := []Account{
		{ID: 81, Name: "fast", Status: StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{9}},
		{ID: 82, Name: "slow", Status: StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{9}},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 9, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		aggregates: map[int64]AccountMonitorAggregate{
			81: {SampleCount: 90, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastCheckedAt: &now},
			82: {SampleCount: 10, SuccessRate: 0.5, TTFTP50MS: floatPtr(900), LatencyP95MS: floatPtr(1500), LastCheckedAt: &now},
		},
		groupAggregates: map[int64]map[int64]AccountMonitorAggregate{9: {
			81: {SampleCount: 90, SuccessRate: 1, TTFTP50MS: floatPtr(120), LatencyP95MS: floatPtr(240), LastCheckedAt: &now},
			82: {SampleCount: 10, SuccessRate: 0.5, TTFTP50MS: floatPtr(800), LatencyP95MS: floatPtr(1400), LastCheckedAt: &now},
		}},
		aggregate: AccountMonitorAggregate{SampleCount: 100, SuccessRate: 0.95, TTFTP50MS: floatPtr(110), LatencyP95MS: floatPtr(260)},
		groupAggregate: map[int64]AccountMonitorAggregate{
			9: {SampleCount: 100, SuccessRate: 0.9, TTFTP50MS: floatPtr(130), LatencyP95MS: floatPtr(300)},
		},
		latest: map[int64]AccountMonitorLatest{81: {Status: "success", CheckedAt: now}, 82: {Status: "success", CheckedAt: now}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if page.Health.TTFTP50MS == nil || *page.Health.TTFTP50MS != 110 || page.Health.LatencyP95MS == nil || *page.Health.LatencyP95MS != 260 {
		t.Fatalf("global percentiles must come from the combined request set: %#v", page.Health)
	}
	groupHealth := page.Groups[0].Health
	if groupHealth.TTFTP50MS == nil || *groupHealth.TTFTP50MS != 130 || groupHealth.LatencyP95MS == nil || *groupHealth.LatencyP95MS != 300 {
		t.Fatalf("group percentiles must come from the combined request set: %#v", groupHealth)
	}
}

func TestAccountMonitorCombinedHealthExcludesPausedAccounts(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.02
	accounts := []Account{
		{ID: 91, Name: "paused", Status: StatusActive, Schedulable: false, RateMultiplier: &rate},
		{ID: 92, Name: "active", Status: StatusActive, Schedulable: true, RateMultiplier: &rate},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		aggregates: map[int64]AccountMonitorAggregate{
			91: {SampleCount: 100, SuccessRate: 0.1, LastCheckedAt: &now},
			92: {SampleCount: 10, SuccessRate: 1, LastCheckedAt: &now},
		},
		aggregate: AccountMonitorAggregate{SampleCount: 10, SuccessRate: 1, LastCheckedAt: &now},
		latest:    map[int64]AccountMonitorLatest{92: {Status: "success", CheckedAt: now}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.aggregateIDs) != 1 || repo.aggregateIDs[0] != 92 || page.Health.SuccessRate != 1 {
		t.Fatalf("combined health scope includes paused accounts: ids=%v health=%#v", repo.aggregateIDs, page.Health)
	}
}

func TestAccountMonitorGroupCombinedHealthExcludesPausedHistory(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.02
	accounts := []Account{
		{ID: 91, Name: "paused", Status: StatusActive, Schedulable: false, RateMultiplier: &rate, GroupIDs: []int64{9}},
		{ID: 92, Name: "active", Status: StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{9}},
	}
	repo := &accountMonitorRepoStub{
		settings:       AccountMonitorSettings{IntervalSeconds: 300},
		groups:         []AccountMonitorGroup{{ID: 9, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		aggregates:     map[int64]AccountMonitorAggregate{91: {SampleCount: 100, SuccessRate: 0.1, LastCheckedAt: &now}, 92: {SampleCount: 10, SuccessRate: 1, LastCheckedAt: &now}},
		groupAggregate: map[int64]AccountMonitorAggregate{9: {SampleCount: 10, SuccessRate: 1, LastCheckedAt: &now}},
		latest:         map[int64]AccountMonitorLatest{92: {Status: "success", CheckedAt: now}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := repo.groupAggregateIDs[9]; len(got) != 1 || got[0] != 92 {
		t.Fatalf("group combined health scope includes paused accounts: ids=%v", got)
	}
	if page.Groups[0].Health.SuccessRate != 1 {
		t.Fatalf("group health includes paused history: %#v", page.Groups[0].Health)
	}
}

func TestAccountMonitorClosedGroupWithRequestsKeepsClosedStateAndAccounts(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.02
	account := Account{ID: 89, Name: "failed", Status: StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{88}}
	repo := &accountMonitorRepoStub{
		settings:       AccountMonitorSettings{IntervalSeconds: 300},
		groups:         []AccountMonitorGroup{{ID: 88, Name: "closed", RateMultiplier: 1, CustomerVisible: false}},
		aggregates:     map[int64]AccountMonitorAggregate{89: {SampleCount: 4, SuccessRate: 0, LastCheckedAt: &now}},
		groupAggregate: map[int64]AccountMonitorAggregate{88: {SampleCount: 1, ErrorCount: 1, SuccessRate: 0, LastCheckedAt: &now}},
		latest:         map[int64]AccountMonitorLatest{89: {Status: "success", CheckedAt: now}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	group := page.Groups[0]
	if group.OperationalState != "closed" || len(group.Accounts) != 1 || group.Health.UnavailableAccounts != 1 {
		t.Fatalf("closed group with requests must remain inspectable: %#v", group)
	}
}

func TestAccountMonitorClosedGroupWithSuccessfulTrafficRemainsClosed(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.02
	account := Account{ID: 90, Name: "healthy", Status: StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{88}}
	repo := &accountMonitorRepoStub{
		settings:   AccountMonitorSettings{IntervalSeconds: 300},
		groups:     []AccountMonitorGroup{{ID: 88, Name: "closed", RateMultiplier: 1, CustomerVisible: false}},
		aggregates: map[int64]AccountMonitorAggregate{90: {SampleCount: 4, SuccessCount: 4, SuccessRate: 1, LastCheckedAt: &now}},
		groupAggregate: map[int64]AccountMonitorAggregate{
			88: {SampleCount: 4, SuccessCount: 4, SuccessRate: 1, LastCheckedAt: &now},
		},
		latest: map[int64]AccountMonitorLatest{90: {Status: "failed", CheckedAt: now}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	group := page.Groups[0]
	if group.OperationalState != "closed" || len(group.Accounts) != 1 {
		t.Fatalf("closed group with successful traffic must remain inspectable: %#v", group)
	}
}

func TestAccountMonitorWindowCostUsesProcurementOverlapAndOneToOneMultiplier(t *testing.T) {
	windowStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	effectiveAt := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	purchaseCost := 24.0

	got := accountMonitorWindowCost(Account{
		ProcurementCostCNY:         &purchaseCost,
		ProcurementCostEffectiveAt: &effectiveAt,
		ExpiresAt:                  &expiresAt,
	}, windowStart, windowEnd, 8)
	if got.Mode != "procurement" || got.WindowCost != 8 || got.EffectiveMultiplier == nil || *got.EffectiveMultiplier != 1 {
		t.Fatalf("procurement window cost = %#v, want CNY 8 and 1x", got)
	}
}

func TestAccountMonitorListWindowProjectsNativeProcurementCostFields(t *testing.T) {
	now := time.Now().UTC()
	purchaseCost := 120.50
	effectiveAt := now.Add(-24 * time.Hour)
	expiresAt := now.Add(29 * 24 * time.Hour)
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			113: {RequestCount: 3, BaseCost: 10, SuccessRate: 1, LastObservedAt: &now},
		},
	}

	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{{
		ID: 113, Name: "procurement", Status: StatusActive, Schedulable: true,
		ProcurementCostCNY: &purchaseCost, ProcurementCostEffectiveAt: &effectiveAt, ExpiresAt: &expiresAt,
	}}}, nil, nil, nil).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Accounts) != 1 {
		t.Fatalf("accounts = %#v, want one account", page.Accounts)
	}
	row := page.Accounts[0]
	if row.ProcurementCostCNY == nil || *row.ProcurementCostCNY != purchaseCost {
		t.Fatalf("procurement_cost_cny = %#v, want %v", row.ProcurementCostCNY, purchaseCost)
	}
	if row.ProcurementCostEffectiveAt == nil || !row.ProcurementCostEffectiveAt.Equal(effectiveAt) {
		t.Fatalf("procurement_cost_effective_at = %#v, want %s", row.ProcurementCostEffectiveAt, effectiveAt)
	}
	if row.ExpiresAt == nil || !row.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expires_at = %#v, want %s", row.ExpiresAt, expiresAt)
	}
}

func TestAccountMonitorWindowCostReturnsZeroCostScoreForInvalidCostInputs(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	effectiveAt := start.Add(-24 * time.Hour)
	purchaseCost := 24.0

	procurement := accountMonitorWindowCost(Account{
		ProcurementCostCNY:         &purchaseCost,
		ProcurementCostEffectiveAt: &effectiveAt,
	}, start, end, 8)
	if procurement.CostScoreEligible || procurement.EffectiveMultiplier != nil {
		t.Fatalf("missing expiry must have zero cost score: %#v", procurement)
	}

	multiplier := accountMonitorWindowCost(Account{}, start, end, 8)
	if multiplier.Mode != "multiplier" || multiplier.CostScoreEligible || multiplier.EffectiveMultiplier != nil {
		t.Fatalf("missing multiplier must have zero cost score: %#v", multiplier)
	}
}

func TestAccountMonitorWindowRankingKeepsCostInvalidAccountEligible(t *testing.T) {
	now := time.Now().UTC()
	cheap := 0.5
	accounts := []Account{
		{ID: 9, Name: "cost-missing", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}},
		{ID: 10, Name: "priced", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &cheap},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			9:  {RequestCount: 3, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
			10: {RequestCount: 3, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
		},
		latest: map[int64]AccountMonitorLatest{9: {Status: "success", CheckedAt: now}, 10: {Status: "success", CheckedAt: now}},
	}
	multiplier := &accountMonitorMultiplierStub{results: map[int64]AccountMonitorMultiplier{
		9:  {Status: AccountMonitorMultiplierStatusUnavailable},
		10: {Value: &cheap, Status: AccountMonitorMultiplierStatusOK},
	}}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, multiplier).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	rows := page.Groups[0].Accounts
	if len(rows) != 2 || !rows[0].Eligible || rows[0].CostScore != 0 || rows[0].GroupRank == nil || *rows[0].GroupRank != 1 {
		t.Fatalf("cost-invalid account must remain eligible and rankable: %#v", rows)
	}
}

func TestAccountMonitorWindowRankingUsesScoreThenAccountIDAndPlacesUnrankedLast(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.5
	accounts := []Account{
		{ID: 30, Name: "tie-later", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
		{ID: 10, Name: "best", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
		{ID: 20, Name: "tie-earlier", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
		{ID: 40, Name: "unranked", Status: StatusDisabled, Schedulable: false, GroupIDs: []int64{7}, RateMultiplier: &rate},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			10: {RequestCount: 3, BaseCost: 1, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
			20: {RequestCount: 3, BaseCost: 1, SuccessRate: 0.5, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
			30: {RequestCount: 3, BaseCost: 1, SuccessRate: 0.5, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
		},
		latest: map[int64]AccountMonitorLatest{10: {Status: "success", CheckedAt: now}, 20: {Status: "success", CheckedAt: now}, 30: {Status: "success", CheckedAt: now}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	rows := page.Groups[0].Accounts
	if got := []int64{rows[0].AccountID, rows[1].AccountID, rows[2].AccountID, rows[3].AccountID}; !slicesEqual(got, []int64{10, 20, 30, 40}) {
		t.Fatalf("rank order = %v, want [10 20 30 40]", got)
	}
	if rows[0].GroupRank == nil || *rows[0].GroupRank != 1 || rows[1].GroupRank == nil || *rows[1].GroupRank != 2 || rows[2].GroupRank == nil || *rows[2].GroupRank != 3 || rows[3].GroupRank != nil {
		t.Fatalf("ranks = %#v", rows)
	}
}

func TestAccountMonitorWindowEvidenceUsesProbesOnlyBelowThreeRealRequests(t *testing.T) {
	now := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	probe := AccountMonitorAggregate{SampleCount: 4, SuccessSampleCount: 4, SuccessRate: 0.75, LastCheckedAt: &now}
	latest := AccountMonitorLatest{Status: "success", CheckedAt: now}
	settings := AccountMonitorSettings{IntervalSeconds: 300}

	withTwoRequests := accountMonitorWindowEvidence(
		AccountMonitorWindowAggregate{RequestCount: 2, SuccessRate: 1, LastObservedAt: &now}, probe, latest, settings, now,
	)
	if withTwoRequests.Source != "monitor_probe" || withTwoRequests.SampleCount != 4 || withTwoRequests.SuccessRate != 0.75 {
		t.Fatalf("two real requests should use probe evidence: %#v", withTwoRequests)
	}

	withThreeRequests := accountMonitorWindowEvidence(
		AccountMonitorWindowAggregate{RequestCount: 3, SuccessRate: 1, LastObservedAt: &now}, probe, latest, settings, now,
	)
	if withThreeRequests.Source != "real_requests" || withThreeRequests.SampleCount != 3 || withThreeRequests.SuccessRate != 1 {
		t.Fatalf("three real requests must not use probe evidence: %#v", withThreeRequests)
	}
}

func TestAccountMonitorWindowProbeFallbackUsesSelectedUpperBound(t *testing.T) {
	repo := &accountMonitorRepoStub{settings: AccountMonitorSettings{IntervalSeconds: 300}}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil)

	page, err := svc.ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	if !repo.probeSince.Equal(repo.windowSince) || !repo.probeUntil.Equal(repo.windowUntil) {
		t.Fatalf("probe window = [%s, %s), request window = [%s, %s)", repo.probeSince, repo.probeUntil, repo.windowSince, repo.windowUntil)
	}
	if !repo.probeUntil.Equal(page.ObservedAt) {
		t.Fatalf("probe upper bound = %s, observed_at = %s", repo.probeUntil, page.ObservedAt)
	}
	if got := repo.probeUntil.Sub(repo.probeSince); got != 24*time.Hour {
		t.Fatalf("probe window duration = %s, want 24h", got)
	}
}

func slicesEqual(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func floatPtr(value float64) *float64 { return &value }

func TestAccountMonitorUsageWindowNormalizesNativePercentage(t *testing.T) {
	progress := &UsageProgress{
		Utilization: 42,
		WindowStats: &WindowStats{Requests: 12, Tokens: 340},
	}
	window, ok := accountMonitorUsageWindow("5h", progress)
	if !ok {
		t.Fatal("expected usage window")
	}
	if window.Utilization != 0.42 || window.Requests != 12 || window.Tokens != 340 {
		t.Fatalf("window = %#v", window)
	}
}

func TestAccountMonitorProjectionIncludesReusableTodayStatsWithoutSecrets(t *testing.T) {
	row := AccountMonitorAccount{
		AccountID:   7,
		AccountType: "oauth",
		TodayStats:  &WindowStats{Requests: 3, Tokens: 120, Cost: 0.5, UserCost: 0.8},
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, expected := range []string{`"account_type":"oauth"`, `"today_stats":`, `"tokens":120`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("projection %s missing %s", text, expected)
		}
	}
	for _, forbidden := range []string{"credential", "token\"", "cookie", "authorization", "password", "secret"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("projection contains forbidden field %q: %s", forbidden, text)
		}
	}
}

func TestAccountMonitorProjectionSerializesUngroupedAccountListsAsEmptyArrays(t *testing.T) {
	service := NewAccountMonitorService(
		&accountMonitorRepoStub{settings: AccountMonitorSettings{IntervalSeconds: 300}},
		&accountMonitorAccountRepoStub{accounts: []Account{{
			ID:          91,
			Name:        "ungrouped",
			Status:      StatusActive,
			Schedulable: true,
			Platform:    PlatformOpenAI,
		}}},
		nil,
		nil,
		nil,
	)

	page, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, expected := range []string{`"group_ids":[]`, `"group_names":[]`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("projection %s missing %s", text, expected)
		}
	}
}

func TestAccountMonitorServiceRunAllRefreshesDueMultiplierWithoutFailingConnectivity(t *testing.T) {
	monitorRepo := &accountMonitorRepoStub{}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{
		ID:          21,
		Status:      StatusActive,
		Schedulable: true,
		Platform:    PlatformOpenAI,
	}}}
	multiplier := &accountMonitorMultiplierStub{err: errors.New("measurement failed")}
	service := NewAccountMonitorService(monitorRepo, accountRepo, nil, nil, multiplier)

	completed, err := service.RunAll(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if completed != 1 || len(monitorRepo.results) != 1 {
		t.Fatalf("completed=%d results=%#v", completed, monitorRepo.results)
	}
	if len(multiplier.calls) != 1 || multiplier.calls[0] != (accountMonitorMultiplierCall{accountID: 21, force: false}) {
		t.Fatalf("multiplier calls = %#v", multiplier.calls)
	}
}

func TestAccountMonitorServiceRunOneForcesMultiplierWithoutFailingConnectivity(t *testing.T) {
	monitorRepo := &accountMonitorRepoStub{}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{
		ID:          23,
		Status:      StatusActive,
		Schedulable: true,
		Platform:    PlatformOpenAI,
	}}}
	multiplier := &accountMonitorMultiplierStub{err: errors.New("measurement failed")}
	service := NewAccountMonitorService(monitorRepo, accountRepo, nil, nil, multiplier)

	result, err := service.RunOne(context.Background(), 1, 23)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountID != 23 || len(monitorRepo.results) != 1 {
		t.Fatalf("result=%#v persisted=%#v", result, monitorRepo.results)
	}
	if len(multiplier.calls) != 1 || multiplier.calls[0] != (accountMonitorMultiplierCall{accountID: 23, force: true}) {
		t.Fatalf("multiplier calls = %#v", multiplier.calls)
	}
}

func TestAccountMonitorServiceRunAllBoundsBlockingProbe(t *testing.T) {
	monitorRepo := &accountMonitorRepoStub{}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{
		ID:          31,
		Status:      StatusActive,
		Schedulable: true,
		Platform:    PlatformOpenAI,
	}}}
	service := NewAccountMonitorService(monitorRepo, accountRepo, nil, nil, nil)
	service.probeTimeout = 20 * time.Millisecond
	service.probeConnection = func(
		ctx context.Context,
		_ int64,
		_ string,
		_ string,
		_ string,
	) (AccountMonitorProbeResult, error) {
		<-ctx.Done()
		return AccountMonitorProbeResult{}, ctx.Err()
	}

	for attempt := 0; attempt < 2; attempt++ {
		completed, err := service.RunAll(context.Background(), 1)
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt+1, err)
		}
		if completed != 1 {
			t.Fatalf("attempt %d completed = %d", attempt+1, completed)
		}
	}

	if len(monitorRepo.results) != 2 {
		t.Fatalf("persisted results = %#v", monitorRepo.results)
	}
	for _, result := range monitorRepo.results {
		if result.Status != "failed" || result.ErrorCode != "timeout" {
			t.Fatalf("timed-out result = %#v", result)
		}
	}
}

func TestAccountMonitorServiceConcurrentRunAllJoinsInFlightRun(t *testing.T) {
	monitorRepo := &accountMonitorRepoStub{}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{
		ID:          37,
		Status:      StatusActive,
		Schedulable: true,
		Platform:    PlatformOpenAI,
	}}}
	service := NewAccountMonitorService(monitorRepo, accountRepo, nil, nil, nil)
	started := make(chan struct{})
	release := make(chan struct{})
	var probeCalls int
	var probeMu sync.Mutex
	service.probeConnection = func(
		context.Context,
		int64,
		string,
		string,
		string,
	) (AccountMonitorProbeResult, error) {
		probeMu.Lock()
		probeCalls++
		call := probeCalls
		probeMu.Unlock()
		if call == 1 {
			close(started)
			<-release
		}
		return AccountMonitorProbeResult{Status: "success", CheckedAt: time.Now().UTC()}, nil
	}

	first := make(chan accountMonitorRunResult, 1)
	second := make(chan accountMonitorRunResult, 1)
	go func() {
		completed, err := service.RunAll(context.Background(), 1)
		first <- accountMonitorRunResult{completed: completed, err: err}
	}()
	<-started
	go func() {
		completed, err := service.RunAll(context.Background(), 2)
		second <- accountMonitorRunResult{completed: completed, err: err}
	}()

	select {
	case result := <-second:
		t.Fatalf("joiner returned before leader completed: %#v", result)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)

	for name, resultCh := range map[string]<-chan accountMonitorRunResult{
		"leader": first,
		"joiner": second,
	} {
		result := <-resultCh
		if result.err != nil || result.completed != 1 {
			t.Fatalf("%s result = %#v", name, result)
		}
	}
	probeMu.Lock()
	defer probeMu.Unlock()
	if probeCalls != 1 {
		t.Fatalf("physical probe calls = %d", probeCalls)
	}
}

func TestAccountMonitorServiceJoiningRunAllHonorsCallerCancellation(t *testing.T) {
	monitorRepo := &accountMonitorRepoStub{}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{
		ID:          41,
		Status:      StatusActive,
		Schedulable: true,
		Platform:    PlatformOpenAI,
	}}}
	service := NewAccountMonitorService(monitorRepo, accountRepo, nil, nil, nil)
	started := make(chan struct{})
	release := make(chan struct{})
	service.probeConnection = func(
		context.Context,
		int64,
		string,
		string,
		string,
	) (AccountMonitorProbeResult, error) {
		close(started)
		<-release
		return AccountMonitorProbeResult{Status: "success", CheckedAt: time.Now().UTC()}, nil
	}

	leader := make(chan accountMonitorRunResult, 1)
	go func() {
		completed, err := service.RunAll(context.Background(), 1)
		leader <- accountMonitorRunResult{completed: completed, err: err}
	}()
	<-started

	waiterCtx, cancel := context.WithCancel(context.Background())
	cancel()
	completed, err := service.RunAll(waiterCtx, 2)
	if completed != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled joiner completed=%d err=%v", completed, err)
	}

	close(release)
	result := <-leader
	if result.err != nil || result.completed != 1 {
		t.Fatalf("leader result = %#v", result)
	}
}

func TestAccountMonitorServiceRunOneWaitsForInFlightRunAll(t *testing.T) {
	monitorRepo := &accountMonitorRepoStub{}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{
		ID:          43,
		Status:      StatusActive,
		Schedulable: true,
		Platform:    PlatformOpenAI,
	}}}
	service := NewAccountMonitorService(monitorRepo, accountRepo, nil, nil, nil)
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var probeCalls int
	var probeMu sync.Mutex
	service.probeConnection = func(
		context.Context,
		int64,
		string,
		string,
		string,
	) (AccountMonitorProbeResult, error) {
		probeMu.Lock()
		probeCalls++
		call := probeCalls
		probeMu.Unlock()
		if call == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		return AccountMonitorProbeResult{Status: "success", CheckedAt: time.Now().UTC()}, nil
	}

	fullRun := make(chan accountMonitorRunResult, 1)
	go func() {
		completed, err := service.RunAll(context.Background(), 1)
		fullRun <- accountMonitorRunResult{completed: completed, err: err}
	}()
	<-firstStarted

	singleRun := make(chan error, 1)
	go func() {
		_, err := service.RunOne(context.Background(), 2, 43)
		singleRun <- err
	}()

	select {
	case err := <-singleRun:
		t.Fatalf("single run returned before full run completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseFirst)

	if result := <-fullRun; result.err != nil || result.completed != 1 {
		t.Fatalf("full run result = %#v", result)
	}
	if err := <-singleRun; err != nil {
		t.Fatalf("single run error = %v", err)
	}
	probeMu.Lock()
	defer probeMu.Unlock()
	if probeCalls != 2 {
		t.Fatalf("physical probe calls = %d", probeCalls)
	}
}
