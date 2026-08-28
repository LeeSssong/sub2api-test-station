package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type accountMonitorAccountRepoStub struct {
	accounts              []Account
	schedulableAccounts   []Account
	listAllStatus         string
	listAllCalled         bool
	listSchedulableCalled bool
}

type accountMonitorAPIKeyRecoveryRepoStub struct {
	accountMonitorAccountRepoStub
	clearTempCalls int
}

type accountMonitorRuntimeBlockerStub struct {
	cleared []int64
}

func (s *accountMonitorRuntimeBlockerStub) BlockAccountScheduling(_ *Account, _ time.Time, _ string) {
}

func (s *accountMonitorRuntimeBlockerStub) ClearAccountSchedulingBlock(id int64) {
	s.cleared = append(s.cleared, id)
}

func (s *accountMonitorAPIKeyRecoveryRepoStub) ClearTempUnschedulable(_ context.Context, id int64) error {
	s.clearTempCalls++
	for i := range s.accounts {
		if s.accounts[i].ID == id {
			s.accounts[i].TempUnschedulableUntil = nil
		}
	}
	return nil
}

type accountMonitorRepoStub struct {
	AccountMonitorRepository
	mu                          sync.Mutex
	results                     []AccountMonitorProbeResult
	settings                    AccountMonitorSettings
	groups                      []AccountMonitorGroup
	weights                     map[int64]AccountMonitorScoreWeights
	globalWeights               AccountMonitorScoreWeights
	globalWeightsErr            error
	globalWeightsSaveErr        error
	globalWeightsResetErr       error
	globalWeightsSaved          []AccountMonitorScoreWeights
	globalWeightsSavedAt        time.Time
	globalWeightsReset          bool
	loadGlobalScoreWeightsCalls int
	aggregates                  map[int64]AccountMonitorAggregate
	windowAggregates            map[int64]AccountMonitorWindowAggregate
	groupWindowAggregates       map[int64]map[int64]AccountMonitorWindowAggregate
	groupWindowAggregatesNil    bool
	groupAggregates             map[int64]map[int64]AccountMonitorAggregate
	aggregate                   AccountMonitorAggregate
	groupAggregate              map[int64]AccountMonitorAggregate
	groupAggregateIDs           map[int64][]int64
	groupAggregatesErr          error
	groupWindowAggregateCalls   []struct {
		groupID    int64
		accountIDs []int64
		since      time.Time
		until      time.Time
	}
	latest                 map[int64]AccountMonitorLatest
	timelines              map[int64][]AccountMonitorTimelinePoint
	timelineIDs            []int64
	timelineLimit          int
	aggregateIDs           []int64
	probeSince             time.Time
	probeUntil             time.Time
	windowSince            time.Time
	windowUntil            time.Time
	aggregateCalls         []time.Time
	aggregateResults       []map[int64]AccountMonitorAggregate
	monitorV2Scopes        []MonitorV2GroupAccountScope
	monitorV2FreshSince    time.Time
	monitorV2Start         time.Time
	monitorV2End           time.Time
	monitorV2BucketSize    time.Duration
	monitorV2Projection    map[int64]MonitorV2NativeGroupProjection
	monitorV2ProjectionErr error
	deleteBeforeCalls      []time.Time
}

func (s *accountMonitorRepoStub) InsertResult(_ context.Context, result AccountMonitorProbeResult, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, result)
	return nil
}

func (s *accountMonitorRepoStub) DeleteBefore(_ context.Context, before time.Time) error {
	s.deleteBeforeCalls = append(s.deleteBeforeCalls, before)
	return nil
}

func (s *accountMonitorRepoStub) ListAggregates(_ context.Context, _ []int64, since, until time.Time) (map[int64]AccountMonitorAggregate, error) {
	if s.probeSince.IsZero() {
		s.probeSince = since
		s.probeUntil = until
	}
	s.aggregateCalls = append(s.aggregateCalls, since)
	if len(s.aggregateResults) > 0 {
		index := len(s.aggregateCalls) - 1
		if index >= len(s.aggregateResults) {
			index = len(s.aggregateResults) - 1
		}
		return s.aggregateResults[index], nil
	}
	return s.aggregates, nil
}

func (s *accountMonitorRepoStub) ListWindowAggregates(_ context.Context, _ []int64, since, until time.Time) (map[int64]AccountMonitorWindowAggregate, error) {
	s.windowSince = since
	s.windowUntil = until
	return s.windowAggregates, nil
}

func (s *accountMonitorRepoStub) ListGroupWindowAggregates(_ context.Context, groupID int64, accountIDs []int64, since, until time.Time) (map[int64]AccountMonitorWindowAggregate, error) {
	s.groupWindowAggregateCalls = append(s.groupWindowAggregateCalls, struct {
		groupID    int64
		accountIDs []int64
		since      time.Time
		until      time.Time
	}{groupID: groupID, accountIDs: append([]int64(nil), accountIDs...), since: since, until: until})
	if s.groupWindowAggregatesNil {
		return nil, nil
	}
	if s.groupWindowAggregates == nil {
		if s.windowAggregates == nil {
			return map[int64]AccountMonitorWindowAggregate{}, nil
		}
		return s.windowAggregates, nil
	}
	return s.groupWindowAggregates[groupID], nil
}

func (s *accountMonitorRepoStub) ListGroupAggregates(_ context.Context, groupID int64, _ []int64, _ time.Time) (map[int64]AccountMonitorAggregate, error) {
	if s.groupAggregatesErr != nil {
		return nil, s.groupAggregatesErr
	}
	if s.groupAggregates == nil {
		return s.aggregates, nil
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

func (s *accountMonitorRepoStub) ListTimelines(_ context.Context, ids []int64, limit int) (map[int64][]AccountMonitorTimelinePoint, error) {
	s.timelineIDs = append([]int64(nil), ids...)
	s.timelineLimit = limit
	return s.timelines, nil
}

func (s *accountMonitorRepoStub) LoadSettings(context.Context) (AccountMonitorSettings, error) {
	if s.settings.IntervalSeconds == 0 {
		return AccountMonitorSettings{}, sql.ErrNoRows
	}
	return s.settings, nil
}

func (s *accountMonitorRepoStub) ProjectMonitorV2Groups(
	_ context.Context,
	scopes []MonitorV2GroupAccountScope,
	start, end, freshSince time.Time,
	bucketSize time.Duration,
) (map[int64]MonitorV2NativeGroupProjection, error) {
	s.monitorV2Scopes = append([]MonitorV2GroupAccountScope(nil), scopes...)
	s.monitorV2Start = start
	s.monitorV2End = end
	s.monitorV2FreshSince = freshSince
	s.monitorV2BucketSize = bucketSize
	if s.monitorV2ProjectionErr != nil {
		return nil, s.monitorV2ProjectionErr
	}
	return s.monitorV2Projection, nil
}

type accountMonitorSchedulerProjectionStub struct {
	byGroup map[int64]*OpenAIAccountSchedulerProjection
	calls   []OpenAIAccountSchedulerProjectionRequest
}

func (s *accountMonitorSchedulerProjectionStub) Project(_ context.Context, req OpenAIAccountSchedulerProjectionRequest) (*OpenAIAccountSchedulerProjection, error) {
	s.calls = append(s.calls, req)
	return s.byGroup[req.GroupID], nil
}

type accountMonitorLegacyRepoAdapter struct {
	AccountMonitorRepository
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

func (s *accountMonitorRepoStub) LoadGlobalScoreWeights(context.Context) (AccountMonitorScoreWeights, error) {
	s.loadGlobalScoreWeightsCalls++
	if s.globalWeightsErr != nil {
		return AccountMonitorScoreWeights{}, s.globalWeightsErr
	}
	return s.globalWeights, nil
}

func (s *accountMonitorRepoStub) SaveGlobalScoreWeights(_ context.Context, actorID int64, weights AccountMonitorScoreWeights) (AccountMonitorScoreWeights, error) {
	if s.globalWeightsSaveErr != nil {
		return AccountMonitorScoreWeights{}, s.globalWeightsSaveErr
	}
	weights.UpdatedBy = actorID
	weights.UpdatedAt = s.globalWeightsSavedAt
	s.globalWeightsSaved = append(s.globalWeightsSaved, weights)
	s.globalWeights = weights
	return weights, nil
}

func (s *accountMonitorRepoStub) ResetGlobalScoreWeights(context.Context) error {
	if s.globalWeightsResetErr != nil {
		return s.globalWeightsResetErr
	}
	s.globalWeightsReset = true
	s.globalWeights = AccountMonitorScoreWeights{}
	s.globalWeightsErr = sql.ErrNoRows
	return nil
}

func TestAccountMonitorServiceGlobalScoreWeightsDefaultAndErrors(t *testing.T) {
	repo := &accountMonitorRepoStub{globalWeightsErr: sql.ErrNoRows}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil)

	got, err := svc.GetGlobalScoreWeights(context.Background())
	if err != nil {
		t.Fatalf("GetGlobalScoreWeights() error = %v", err)
	}
	if !got.IsDefault || got.Cost != 15 || got.Success != 45 || got.TTFT != 20 || got.Latency != 20 || got.UpdatedAt != nil {
		t.Fatalf("default response = %#v", got)
	}

	repo.globalWeightsErr = errors.New("database unavailable")
	if _, err := svc.GetGlobalScoreWeights(context.Background()); err == nil {
		t.Fatal("expected storage error to propagate")
	}
}

func TestAccountMonitorListRestoresLegacyQualityScoreProjection(t *testing.T) {
	now := time.Now().UTC()
	rate := 1.0
	repo := &accountMonitorRepoStub{
		settings:      AccountMonitorSettings{IntervalSeconds: 300},
		globalWeights: DefaultAccountMonitorScoreWeights,
		aggregates: map[int64]AccountMonitorAggregate{
			7: {SampleCount: 6, SuccessCount: 6, SuccessSampleCount: 6, SuccessRate: 1, LastCheckedAt: &now},
		},
		latest: map[int64]AccountMonitorLatest{7: {Status: "success", CheckedAt: now}},
		groups: []AccountMonitorGroup{{ID: 42, RateMultiplier: 1, ScoreWeights: DefaultAccountMonitorScoreWeights}},
	}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{{ID: 7, Status: StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{42}}}}, nil, nil, accountMonitorConfirmedMultiplier(rate))

	page, err := svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Accounts) != 1 || page.Accounts[0].QualityScore == nil || !page.Accounts[0].Eligible {
		t.Fatalf("legacy account projection did not restore score: %#v", page.Accounts)
	}
	if len(page.Groups) != 1 || len(page.Groups[0].Accounts) != 1 || !page.Groups[0].Accounts[0].Evidence.Known {
		t.Fatalf("legacy group evidence lost Known state: %#v", page.Groups)
	}
}

func TestAccountMonitorListHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &accountMonitorRepoStub{settings: AccountMonitorSettings{IntervalSeconds: 300}}
	_, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil).List(ctx)
	if err == nil {
		t.Fatal("List should return the caller context error")
	}
}

func TestAccountMonitorServiceUpdatesGlobalScoreWeightsWithoutThresholdPersistence(t *testing.T) {
	repo := &accountMonitorRepoStub{}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil)
	savedAt := time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC)
	repo.globalWeightsSavedAt = savedAt

	got, err := svc.UpdateGlobalScoreWeights(context.Background(), 12, AccountMonitorScoreWeights{
		Cost: 30, Success: 30, TTFT: 20, Latency: 20,
		TTFTTargetMS: 1, TTFTLimitMS: 2, LatencyTargetMS: 3, LatencyLimitMS: 4,
	})
	if err != nil {
		t.Fatalf("UpdateGlobalScoreWeights() error = %v", err)
	}
	if got.IsDefault || got.Cost != 30 || got.Success != 30 || got.TTFT != 20 || got.Latency != 20 {
		t.Fatalf("saved response = %#v", got)
	}
	if got.UpdatedBy != 12 || got.UpdatedAt == nil || !got.UpdatedAt.Equal(savedAt) {
		t.Fatalf("audit fields = updated_by %d updated_at %#v", got.UpdatedBy, got.UpdatedAt)
	}
	saved := repo.globalWeightsSaved[len(repo.globalWeightsSaved)-1]
	if saved.TTFTTargetMS != 0 || saved.TTFTLimitMS != 0 || saved.LatencyTargetMS != 0 || saved.LatencyLimitMS != 0 {
		t.Fatalf("global save must not persist thresholds: %#v", saved)
	}
}

func TestAccountMonitorServiceGlobalSaveErrorDoesNotReread(t *testing.T) {
	repo := &accountMonitorRepoStub{globalWeightsSaveErr: errors.New("write failed")}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil)
	if _, err := svc.UpdateGlobalScoreWeights(context.Background(), 12, AccountMonitorScoreWeights{Cost: 30, Success: 30, TTFT: 20, Latency: 20}); err == nil {
		t.Fatal("expected save error")
	}
	if repo.loadGlobalScoreWeightsCalls != 0 {
		t.Fatalf("UpdateGlobalScoreWeights reread after save error: %d", repo.loadGlobalScoreWeightsCalls)
	}
}

func TestAccountMonitorServiceRejectsInvalidGlobalScoreWeights(t *testing.T) {
	repo := &accountMonitorRepoStub{}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{}, nil, nil, nil)
	if _, err := svc.UpdateGlobalScoreWeights(context.Background(), 12, AccountMonitorScoreWeights{Cost: 30, Success: 30, TTFT: 20, Latency: 19}); err == nil {
		t.Fatal("expected invalid sum error")
	}
	if _, err := svc.UpdateGlobalScoreWeights(context.Background(), 12, AccountMonitorScoreWeights{Cost: -1, Success: 61, TTFT: 20, Latency: 20}); err == nil {
		t.Fatal("expected negative weight error")
	}
	if _, err := svc.ResetGlobalScoreWeights(context.Background(), 0); err == nil {
		t.Fatal("expected invalid actor error")
	}
	overflowWeight := int(^uint(0) >> 1)
	if _, err := svc.UpdateGlobalScoreWeights(context.Background(), 12, AccountMonitorScoreWeights{
		Cost: overflowWeight, Success: overflowWeight, TTFT: 101, Latency: 1,
	}); !errors.Is(err, ErrAccountMonitorInvalidScoreWeights) {
		t.Fatalf("overflow-sized weights error = %v, want invalid score weights", err)
	}
	if len(repo.globalWeightsSaved) != 0 {
		t.Fatalf("invalid weights reached repository save: %#v", repo.globalWeightsSaved)
	}
}

func TestAccountMonitorListWindowUsesPersistedGlobalScoreWeights(t *testing.T) {
	rate := 1.0
	accounts := []Account{
		{ID: 1, Name: "cheap", Status: "active", Schedulable: true, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, RateMultiplier: &rate},
		{ID: 2, Name: "fast", Status: "active", Schedulable: true, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, RateMultiplier: &rate},
	}
	ttftCheap := 4000.0
	ttftFast := 500.0
	latency := 1000.0
	repo := &accountMonitorRepoStub{
		settings:         AccountMonitorSettings{IntervalSeconds: AccountMonitorDefaultIntervalSeconds},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{},
		aggregates: map[int64]AccountMonitorAggregate{
			1: {SampleCount: 5, SuccessSampleCount: 5, TTFTSampleCount: 5, LatencySampleCount: 5, SuccessRate: 1, TTFTP50MS: &ttftCheap, LatencyP95MS: &latency, LastCheckedAt: ptrTime(time.Now().UTC())},
			2: {SampleCount: 5, SuccessSampleCount: 5, TTFTSampleCount: 5, LatencySampleCount: 5, SuccessRate: 1, TTFTP50MS: &ttftFast, LatencyP95MS: &latency, LastCheckedAt: ptrTime(time.Now().UTC())},
		},
		groups:        []AccountMonitorGroup{{ID: 7, Name: "Group", RateMultiplier: 1, ScoreWeights: AccountMonitorScoreWeights{Cost: 15, Success: 45, TTFT: 20, Latency: 20}}},
		globalWeights: AccountMonitorScoreWeights{Cost: 0, Success: 0, TTFT: 100, Latency: 0},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatalf("ListWindow() error = %v", err)
	}
	if got := []int64{page.Accounts[0].AccountID, page.Accounts[1].AccountID}; !reflect.DeepEqual(got, []int64{2, 1}) {
		t.Fatalf("global ranking account ids = %v", got)
	}
	if page.SchemaVersion != AccountMonitorSchemaVersion {
		t.Fatalf("schema version changed: %d", page.SchemaVersion)
	}
}

func TestAccountMonitorListWindowKeepsQualityEvidenceAndSchedulerRanksGroupScoped(t *testing.T) {
	rate := 1.0
	now := time.Now().UTC()
	accounts := []Account{
		{ID: 1, Name: "shared", Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, RateMultiplier: &rate, GroupIDs: []int64{7, 8}},
		{ID: 2, Name: "group-seven", Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, RateMultiplier: &rate, GroupIDs: []int64{7}},
		{ID: 3, Name: "group-eight", Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, RateMultiplier: &rate, GroupIDs: []int64{8}},
		{ID: 4, Name: "no-evidence", Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, RateMultiplier: &rate, GroupIDs: []int64{7}},
	}
	probe := func(ttft, latency float64) AccountMonitorAggregate {
		return AccountMonitorAggregate{
			SampleCount: 3, SuccessCount: 3, SuccessSampleCount: 3, SuccessRate: 1,
			TTFTSampleCount: 3, LatencySampleCount: 3, TTFTP50MS: &ttft, LatencyP95MS: &latency,
			LastCheckedAt: &now,
		}
	}
	window := func(ttft, latency float64) AccountMonitorWindowAggregate {
		return AccountMonitorWindowAggregate{
			RequestCount: 3, SuccessCount: 3, SuccessRate: 1, TTFTSampleCount: 3, LatencySampleCount: 3,
			TTFTP50MS: &ttft, LatencyP95MS: &latency, LastObservedAt: &now,
		}
	}
	rank := func(value int) *int { return &value }
	scheduler := &accountMonitorSchedulerProjectionStub{byGroup: map[int64]*OpenAIAccountSchedulerProjection{
		7: {SnapshotAt: now, PolicyKey: "group_policy", PolicyLabel: "利润优先", EffectiveWeights: map[string]float64{"priority": 1}, CandidateCount: 3, Candidates: []OpenAIAccountSchedulerProjectionCandidate{
			{AccountID: 1, Rank: rank(1), Eligible: true, PrimaryReasonCode: AccountMonitorReasonStrategy},
			{AccountID: 2, Rank: rank(2), Eligible: true},
			{AccountID: 4, Eligible: false, PrimaryReasonCode: AccountMonitorReasonNotEligible},
		}},
		8: {SnapshotAt: now, PolicyKey: "group_policy", PolicyLabel: "体验优先", EffectiveWeights: map[string]float64{"priority": 2}, CandidateCount: 2, Candidates: []OpenAIAccountSchedulerProjectionCandidate{
			{AccountID: 3, Rank: rank(1), Eligible: true},
			{AccountID: 1, Rank: rank(2), Eligible: true, PrimaryReasonCode: AccountMonitorReasonTieBreak},
		}},
	}}
	repo := &accountMonitorRepoStub{
		settings:      AccountMonitorSettings{IntervalSeconds: 300},
		globalWeights: AccountMonitorScoreWeights{Cost: 0, Success: 45, TTFT: 35, Latency: 20},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			1: window(400, 800), 2: window(100, 200), 3: window(200, 300),
		},
		groupWindowAggregates: map[int64]map[int64]AccountMonitorWindowAggregate{
			7: {1: window(100, 200), 2: window(4000, 50000)},
			8: {1: window(5000, 60000), 3: window(100, 200)},
		},
		aggregates: map[int64]AccountMonitorAggregate{1: probe(100, 200), 2: probe(100, 200), 3: probe(100, 200), 4: probe(100, 200)},
		groupAggregates: map[int64]map[int64]AccountMonitorAggregate{
			7: {1: probe(100, 200), 2: probe(4000, 50000)},
			8: {1: probe(5000, 60000), 3: probe(100, 200)},
		},
		latest: map[int64]AccountMonitorLatest{
			1: {Status: "success", CheckedAt: now}, 2: {Status: "success", CheckedAt: now}, 3: {Status: "success", CheckedAt: now}, 4: {Status: "success", CheckedAt: now},
		},
		groups: []AccountMonitorGroup{
			{ID: 7, Name: "seven", Platform: PlatformOpenAI, RateMultiplier: 1, RequirePrivacySet: true, ScoreWeights: DefaultAccountMonitorScoreWeights},
			{ID: 8, Name: "eight", Platform: PlatformOpenAI, RateMultiplier: 1, ScoreWeights: DefaultAccountMonitorScoreWeights},
		},
	}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, accountMonitorConfirmedMultiplier(rate))
	svc.SetOpenAIAccountSchedulerProjectionProvider(scheduler)

	page, err := svc.ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatalf("ListWindow() error = %v", err)
	}
	if len(scheduler.calls) != 2 || len(repo.groupWindowAggregateCalls) != 2 {
		t.Fatalf("projection calls = %d, group window calls = %d", len(scheduler.calls), len(repo.groupWindowAggregateCalls))
	}
	if !scheduler.calls[0].RequirePrivacySet || !reflect.DeepEqual(scheduler.calls[0].QualityOrder, []int64{1, 2}) || !reflect.DeepEqual(scheduler.calls[1].QualityOrder, []int64{3, 1}) {
		t.Fatalf("scheduler projection context = %#v", scheduler.calls)
	}
	if !scheduler.calls[0].SnapshotAt.Equal(page.ObservedAt) || !repo.groupWindowAggregateCalls[0].until.Equal(page.ObservedAt) || !repo.groupWindowAggregateCalls[1].until.Equal(page.ObservedAt) {
		t.Fatalf("snapshot timestamps drifted: page=%s scheduler=%s group_until=%s/%s", page.ObservedAt, scheduler.calls[0].SnapshotAt, repo.groupWindowAggregateCalls[0].until, repo.groupWindowAggregateCalls[1].until)
	}
	byGroup := make(map[int64]AccountMonitorGroup)
	for _, group := range page.Groups {
		byGroup[group.ID] = group
	}
	sharedSeven := findAccountMonitorGroupAccount(t, byGroup[7].Accounts, 1)
	sharedEight := findAccountMonitorGroupAccount(t, byGroup[8].Accounts, 1)
	if sharedSeven.Evidence.TTFTP50MS == nil || sharedEight.Evidence.TTFTP50MS == nil || *sharedSeven.Evidence.TTFTP50MS == *sharedEight.Evidence.TTFTP50MS || sharedSeven.Evidence.LatencyP95MS == nil || sharedEight.Evidence.LatencyP95MS == nil || *sharedSeven.Evidence.LatencyP95MS == *sharedEight.Evidence.LatencyP95MS {
		t.Fatalf("shared account evidence was not group-scoped: seven=%#v eight=%#v", sharedSeven.Evidence, sharedEight.Evidence)
	}
	if sharedSeven.QualityRank == nil || sharedEight.QualityRank == nil || *sharedSeven.QualityRank == *sharedEight.QualityRank {
		t.Fatalf("shared account quality ranks = %#v and %#v, want different group ranks", sharedSeven.QualityRank, sharedEight.QualityRank)
	}
	if sharedSeven.SchedulerRank == nil || *sharedSeven.SchedulerRank != 1 || sharedEight.SchedulerRank == nil || *sharedEight.SchedulerRank != 2 {
		t.Fatalf("shared account scheduler ranks = %#v and %#v", sharedSeven.SchedulerRank, sharedEight.SchedulerRank)
	}
	if sharedSeven.SchedulerRankTotal != 2 || sharedEight.SchedulerRankTotal != 2 || sharedSeven.SchedulerExplanation == nil || sharedSeven.SchedulerExplanation.RankTotal != 2 || sharedSeven.SchedulerExplanation.CandidateTotal != 3 {
		t.Fatalf("scheduler rank totals = seven=%#v eight=%#v", sharedSeven.SchedulerExplanation, sharedEight.SchedulerExplanation)
	}
	if sharedSeven.SchedulerExplanation == nil || sharedSeven.SchedulerExplanation.PolicyLabel != "利润优先" || sharedEight.SchedulerExplanation == nil || sharedEight.SchedulerExplanation.PolicyLabel != "体验优先" {
		t.Fatalf("scheduler explanations = %#v / %#v", sharedSeven.SchedulerExplanation, sharedEight.SchedulerExplanation)
	}
	if sharedSeven.QualityExplanation == nil || sharedSeven.QualityExplanation.Breakdown.Success.Max != 45 || sharedSeven.QualityExplanation.Breakdown.TTFT.Max != 20 || sharedSeven.QualityExplanation.Breakdown.Latency.Max != 20 {
		t.Fatalf("quality explanation maxima = %#v", sharedSeven.QualityExplanation)
	}
	noEvidence := findAccountMonitorGroupAccount(t, byGroup[7].Accounts, 4)
	if noEvidence.QualityScore != nil || noEvidence.QualityRank != nil || noEvidence.SchedulerRank != nil {
		t.Fatalf("account without evidence fabricated projection: %#v", noEvidence)
	}
	global := findAccountMonitorAccount(t, page.Accounts, 1)
	if global.QualityRank == nil || global.SchedulerRank != nil {
		t.Fatalf("full-site row left global quality path: %#v", global)
	}
}

func TestAccountMonitorListWindowDoesNotLeakGlobalQualityIntoLegacyGroupRows(t *testing.T) {
	rate := 1.0
	now := time.Now().UTC()
	accounts := []Account{{ID: 1, Name: "shared", Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, RateMultiplier: &rate, GroupIDs: []int64{7}}}
	windowTTFT, windowLatency := 100.0, 200.0
	probeTTFT, probeLatency := 100.0, 200.0
	baseRepo := &accountMonitorRepoStub{
		settings:      AccountMonitorSettings{IntervalSeconds: 300},
		globalWeights: AccountMonitorScoreWeights{Cost: 0, Success: 45, TTFT: 35, Latency: 20},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{1: {
			RequestCount: 3, SuccessCount: 3, SuccessRate: 1, TTFTSampleCount: 3, LatencySampleCount: 3,
			TTFTP50MS: &windowTTFT, LatencyP95MS: &windowLatency, LastObservedAt: &now,
		}},
		aggregates: map[int64]AccountMonitorAggregate{1: {
			SampleCount: 3, SuccessCount: 3, SuccessSampleCount: 3, SuccessRate: 1,
			TTFTSampleCount: 3, LatencySampleCount: 3, TTFTP50MS: &probeTTFT, LatencyP95MS: &probeLatency,
			LastCheckedAt: &now,
		}},
		latest: map[int64]AccountMonitorLatest{1: {Status: "success", CheckedAt: now}},
		groups: []AccountMonitorGroup{{ID: 7, Name: "seven", Platform: PlatformOpenAI, RateMultiplier: 1, ScoreWeights: DefaultAccountMonitorScoreWeights}},
	}
	svc := NewAccountMonitorService(&accountMonitorLegacyRepoAdapter{AccountMonitorRepository: baseRepo}, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, accountMonitorConfirmedMultiplier(rate))

	page, err := svc.ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatalf("ListWindow() error = %v", err)
	}
	row := findAccountMonitorGroupAccount(t, page.Groups[0].Accounts, 1)
	if row.Evidence.SampleCount != 0 || row.QualityScore != nil || row.QualityRank != nil || row.QualityExplanation != nil {
		t.Fatalf("legacy group row leaked global quality evidence: %#v", row)
	}
}

func TestAccountMonitorListWindowSeparatesGroupRequestsFromProbeEvidence(t *testing.T) {
	rate := 1.0
	now := time.Now().UTC()
	accounts := []Account{
		{ID: 101, Name: "overlapping-request", Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, RateMultiplier: &rate, GroupIDs: []int64{7}},
		{ID: 102, Name: "request-only", Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, RateMultiplier: &rate, GroupIDs: []int64{7}},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		aggregates: map[int64]AccountMonitorAggregate{
			// This is the only distinct probe sample for account 101. The two
			// group request rows below must not be treated as more probes.
			101: {SampleCount: 1, SuccessCount: 1, SuccessSampleCount: 1, SuccessRate: 1, LastCheckedAt: &now},
		},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			101: {RequestCount: 2, SuccessCount: 2, SuccessRate: 1, LastObservedAt: &now},
			102: {RequestCount: 100, SuccessCount: 100, SuccessRate: 1, LastObservedAt: &now},
		},
		groupWindowAggregates: map[int64]map[int64]AccountMonitorWindowAggregate{
			7: {
				101: {RequestCount: 2, SuccessCount: 2, SuccessRate: 1, LastObservedAt: &now},
				102: {RequestCount: 100, SuccessCount: 100, SuccessRate: 1, LastObservedAt: &now},
			},
		},
		// These fixtures represent the same group-scoped request domain as
		// groupWindowAggregates. They must never become probe evidence.
		groupAggregates: map[int64]map[int64]AccountMonitorAggregate{
			7: {
				101: {SampleCount: 2, SuccessCount: 2, SuccessSampleCount: 2, SuccessRate: 1, LastCheckedAt: &now},
				102: {SampleCount: 100, SuccessCount: 100, SuccessSampleCount: 100, SuccessRate: 1, LastCheckedAt: &now},
			},
		},
		latest: map[int64]AccountMonitorLatest{
			101: {Status: "success", CheckedAt: now},
			102: {Status: "success", CheckedAt: now},
		},
		groups: []AccountMonitorGroup{{ID: 7, Name: "openai", Platform: PlatformOpenAI, RateMultiplier: 1, ScoreWeights: DefaultAccountMonitorScoreWeights}},
	}

	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	rows := make(map[int64]AccountMonitorGroupAccount, len(page.Groups[0].Accounts))
	for _, row := range page.Groups[0].Accounts {
		rows[row.AccountID] = row
	}

	if got := rows[101].Evidence.SampleCount; got != 3 {
		t.Fatalf("overlapping request/probe evidence sample count = %d, want 3 distinct samples", got)
	}
	if got := rows[101].Evidence.SuccessSampleCount; got != 3 {
		t.Fatalf("overlapping request/probe evidence success count = %d, want 3 distinct samples", got)
	}
	if rows[101].QualityRank == nil {
		t.Fatalf("account with one real probe and group requests should remain rankable: %#v", rows[101])
	}
	if rows[102].Evidence.Source != "stale" || rows[102].Evidence.SampleCount != 0 || rows[102].QualityRank != nil || rows[102].QualityScore != nil {
		t.Fatalf("request-only group traffic bypassed the probe gate: %#v", rows[102])
	}
}

func TestAccountMonitorListWindowDoesNotUseAccountScopedEvidenceWhenGroupWindowProviderReturnsNil(t *testing.T) {
	rate := 1.0
	now := time.Now().UTC()
	account := Account{ID: 104, Name: "group-window-unavailable", Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, RateMultiplier: &rate, GroupIDs: []int64{7}}
	repo := &accountMonitorRepoStub{
		settings:                 AccountMonitorSettings{IntervalSeconds: 300},
		groupWindowAggregatesNil: true,
		windowAggregates:         map[int64]AccountMonitorWindowAggregate{104: {RequestCount: 10, SuccessCount: 10, SuccessRate: 1, LastObservedAt: &now}},
		aggregates:               map[int64]AccountMonitorAggregate{104: {SampleCount: 4, SuccessSampleCount: 4, SuccessRate: 1, LastCheckedAt: &now}},
		latest:                   map[int64]AccountMonitorLatest{104: {Status: "success", CheckedAt: now}},
		groups:                   []AccountMonitorGroup{{ID: 7, Name: "openai", Platform: PlatformOpenAI, RateMultiplier: 1, ScoreWeights: DefaultAccountMonitorScoreWeights}},
	}

	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	row := findAccountMonitorGroupAccount(t, page.Groups[0].Accounts, account.ID)
	if row.Evidence.SampleCount != 0 || row.Evidence.Source != "unknown" || row.QualityScore != nil || row.QualityRank != nil {
		t.Fatalf("group row used account-scoped evidence after nil group-window projection: %#v", row)
	}
}

func TestAccountMonitorListWindowSkipsSchedulerProjectionForNonOpenAIGroups(t *testing.T) {
	rate := 1.0
	now := time.Now().UTC()
	account := Account{ID: 201, Name: "shared", Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, RateMultiplier: &rate, GroupIDs: []int64{7, 8, 9}}
	rank := 1
	projection := func() *OpenAIAccountSchedulerProjection {
		return &OpenAIAccountSchedulerProjection{
			SnapshotAt:     now,
			PolicyKey:      "group_policy",
			PolicyLabel:    "policy",
			CandidateCount: 1,
			Candidates:     []OpenAIAccountSchedulerProjectionCandidate{{AccountID: account.ID, Rank: &rank, Eligible: true}},
		}
	}
	repo := &accountMonitorRepoStub{
		settings:         AccountMonitorSettings{IntervalSeconds: 300},
		aggregates:       map[int64]AccountMonitorAggregate{201: {SampleCount: 1, SuccessCount: 1, SuccessSampleCount: 1, SuccessRate: 1, LastCheckedAt: &now}},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{201: {RequestCount: 1, SuccessCount: 1, SuccessRate: 1, LastObservedAt: &now}},
		groupWindowAggregates: map[int64]map[int64]AccountMonitorWindowAggregate{
			7: {201: {RequestCount: 1, SuccessCount: 1, SuccessRate: 1, LastObservedAt: &now}},
			8: {201: {RequestCount: 1, SuccessCount: 1, SuccessRate: 1, LastObservedAt: &now}},
			9: {201: {RequestCount: 1, SuccessCount: 1, SuccessRate: 1, LastObservedAt: &now}},
		},
		latest: map[int64]AccountMonitorLatest{201: {Status: "success", CheckedAt: now}},
		groups: []AccountMonitorGroup{
			{ID: 7, Name: "default-anthropic", Platform: "", RateMultiplier: 1, ScoreWeights: DefaultAccountMonitorScoreWeights},
			{ID: 8, Name: "anthropic", Platform: PlatformAnthropic, RateMultiplier: 1, ScoreWeights: DefaultAccountMonitorScoreWeights},
			{ID: 9, Name: "openai", Platform: PlatformOpenAI, RateMultiplier: 1, ScoreWeights: DefaultAccountMonitorScoreWeights},
		},
	}
	scheduler := &accountMonitorSchedulerProjectionStub{byGroup: map[int64]*OpenAIAccountSchedulerProjection{
		7: projection(),
		8: projection(),
		9: projection(),
	}}
	svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, accountMonitorConfirmedMultiplier(rate))
	svc.SetOpenAIAccountSchedulerProjectionProvider(scheduler)

	page, err := svc.ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduler.calls) != 1 || scheduler.calls[0].GroupID != 9 || scheduler.calls[0].Platform != PlatformOpenAI {
		t.Fatalf("scheduler projection calls = %#v, want only OpenAI group 9", scheduler.calls)
	}
	for _, group := range page.Groups {
		row := findAccountMonitorGroupAccount(t, group.Accounts, account.ID)
		if group.ID == 9 {
			if row.SchedulerExplanation == nil || row.SchedulerRank == nil {
				t.Fatalf("OpenAI group lost scheduler projection: %#v", row)
			}
			continue
		}
		if row.SchedulerExplanation != nil || row.SchedulerRank != nil {
			t.Fatalf("non-OpenAI group received scheduler metadata: group=%d row=%#v", group.ID, row)
		}
	}
}

func findAccountMonitorGroupAccount(t *testing.T, rows []AccountMonitorGroupAccount, accountID int64) AccountMonitorGroupAccount {
	for _, row := range rows {
		if row.AccountID == accountID {
			return row
		}
	}
	t.Fatalf("account %d not found in group rows %#v", accountID, rows)
	return AccountMonitorGroupAccount{}
}

func findAccountMonitorAccount(t *testing.T, rows []AccountMonitorAccount, accountID int64) AccountMonitorAccount {
	for _, row := range rows {
		if row.AccountID == accountID {
			return row
		}
	}
	t.Fatalf("account %d not found in rows %#v", accountID, rows)
	return AccountMonitorAccount{}
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
	options   AccountMonitorRefreshOptions
}

type accountMonitorRunResult struct {
	completed int
	err       error
}

func TestAccountMonitorProbeProjectionUsesOnlyFreshProbeEvidence(t *testing.T) {
	now := time.Date(2026, time.August, 7, 8, 0, 0, 0, time.UTC)
	settings := AccountMonitorSettings{IntervalSeconds: 300}
	tests := []struct {
		name         string
		management   string
		aggregate    AccountMonitorAggregate
		latest       AccountMonitorLatest
		timeline     []AccountMonitorTimelinePoint
		availability string
		scoreStatus  string
		eligible     bool
	}{
		{
			name:         "zero of twenty four probes is unavailable",
			aggregate:    AccountMonitorAggregate{SampleCount: 24, SuccessCount: 0, ErrorCount: 24, SuccessRate: 0, LastCheckedAt: timePtr(now.Add(-time.Minute))},
			latest:       AccountMonitorLatest{Status: "failed", ErrorCode: "upstream_error", CheckedAt: now.Add(-time.Minute)},
			timeline:     []AccountMonitorTimelinePoint{{Status: "failed"}, {Status: "failed"}, {Status: "failed"}},
			availability: accountMonitorAvailabilityUnavailable, scoreStatus: accountMonitorScoreIneligible,
		},
		{
			name:         "one fresh failure is abnormal and score capped",
			aggregate:    AccountMonitorAggregate{SampleCount: 24, SuccessCount: 23, ErrorCount: 1, SuccessRate: 23.0 / 24.0, LastCheckedAt: timePtr(now.Add(-time.Minute))},
			latest:       AccountMonitorLatest{Status: "failed", ErrorCode: "upstream_error", CheckedAt: now.Add(-time.Minute)},
			timeline:     []AccountMonitorTimelinePoint{{Status: "success"}, {Status: "failed"}},
			availability: accountMonitorAvailabilityAbnormal, scoreStatus: accountMonitorScoreCapped, eligible: true,
		},
		{
			name:         "twenty four successful probes is normal",
			aggregate:    AccountMonitorAggregate{SampleCount: 24, SuccessCount: 24, SuccessRate: 1, SuccessSampleCount: 24, LastCheckedAt: timePtr(now.Add(-time.Minute))},
			latest:       AccountMonitorLatest{Status: "success", CheckedAt: now.Add(-time.Minute)},
			availability: accountMonitorAvailabilityNormal, scoreStatus: accountMonitorScoreEligible, eligible: true,
		},
		{
			name:         "three consecutive failures is unavailable",
			aggregate:    AccountMonitorAggregate{SampleCount: 24, SuccessCount: 21, ErrorCount: 3, SuccessRate: 0.875, LastCheckedAt: timePtr(now.Add(-time.Minute))},
			latest:       AccountMonitorLatest{Status: "failed", ErrorCode: "upstream_error", CheckedAt: now.Add(-time.Minute)},
			timeline:     []AccountMonitorTimelinePoint{{Status: "failed"}, {Status: "failed"}, {Status: "failed"}},
			availability: accountMonitorAvailabilityUnavailable, scoreStatus: accountMonitorScoreIneligible,
		},
		{
			name:         "fatal authentication error is unavailable immediately",
			aggregate:    AccountMonitorAggregate{SampleCount: 1, ErrorCount: 1, LastCheckedAt: timePtr(now.Add(-time.Minute))},
			latest:       AccountMonitorLatest{Status: "failed", ErrorCode: "invalid_api_key", CheckedAt: now.Add(-time.Minute)},
			availability: accountMonitorAvailabilityUnavailable, scoreStatus: accountMonitorScoreIneligible,
		},
		{
			name:         "stale probes cannot score",
			aggregate:    AccountMonitorAggregate{SampleCount: 24, SuccessCount: 24, SuccessRate: 1, LastCheckedAt: timePtr(now.Add(-11 * time.Minute))},
			latest:       AccountMonitorLatest{Status: "success", CheckedAt: now.Add(-11 * time.Minute)},
			availability: accountMonitorAvailabilityStale, scoreStatus: accountMonitorScoreIneligible,
		},
		{
			name:         "no samples cannot score",
			aggregate:    AccountMonitorAggregate{},
			availability: accountMonitorAvailabilityStale, scoreStatus: accountMonitorScoreIneligible,
		},
		{
			name:         "paused account with fresh success still scores",
			management:   accountMonitorManagementPaused,
			aggregate:    AccountMonitorAggregate{SampleCount: 24, SuccessCount: 24, SuccessRate: 1, LastCheckedAt: timePtr(now.Add(-time.Minute))},
			latest:       AccountMonitorLatest{Status: "success", CheckedAt: now.Add(-time.Minute)},
			availability: accountMonitorAvailabilityNormal, scoreStatus: accountMonitorScoreEligible, eligible: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			management := tt.management
			if management == "" {
				management = accountMonitorManagementEnabled
			}
			row := AccountMonitorAccount{RequestCount: 33, SuccessRate: 1, QualityScore: floatPtr(100), GroupRank: intPtr(1)}
			projectAccountMonitorProbe(&row, tt.aggregate, tt.latest, tt.timeline, settings, now, management)
			if row.AvailabilityStatus != tt.availability || row.ScoreStatus != tt.scoreStatus || row.Eligible != tt.eligible {
				t.Fatalf("availability=%q score_status=%q eligible=%v", row.AvailabilityStatus, row.ScoreStatus, row.Eligible)
			}
			if row.ProbeSampleCount != tt.aggregate.SampleCount || row.SampleCount != tt.aggregate.SampleCount || row.SuccessRate != tt.aggregate.SuccessRate {
				t.Fatalf("probe aliases = samples %d/%d success_rate %.3f", row.ProbeSampleCount, row.SampleCount, row.SuccessRate)
			}
			if !tt.eligible && row.GroupRank != nil {
				t.Fatalf("ineligible row retained rank %v", *row.GroupRank)
			}
		})
	}
}

func TestAccountMonitorWindowScoreProjectionSeparatesCurrentStateFromScoreEligibility(t *testing.T) {
	now := time.Now().UTC()
	valid := AccountMonitorQualityEvidence{
		Source: "stale", SampleCount: 24, SuccessSampleCount: 21,
		SuccessRate: 21.0 / 24.0, ObservedAt: now.Add(-20 * time.Minute),
	}
	tests := []struct {
		name       string
		account    Account
		status     string
		evidence   AccountMonitorQualityEvidence
		wantSource string
		wantStatus string
		want       bool
	}{
		{name: "schedulable unavailable retains selected-window score", account: Account{Status: StatusActive, Schedulable: true}, status: accountMonitorScoreIneligible, evidence: valid, wantSource: "monitor_probe", wantStatus: accountMonitorScoreEligible, want: true},
		{name: "schedulable stale retains selected-window score", account: Account{Status: StatusActive, Schedulable: true}, status: accountMonitorScoreIneligible, evidence: valid, wantSource: "monitor_probe", wantStatus: accountMonitorScoreEligible, want: true},
		{name: "pure failure stays unscored", account: Account{Status: StatusActive, Schedulable: true}, status: accountMonitorScoreIneligible, evidence: AccountMonitorQualityEvidence{Source: "monitor_probe", SampleCount: 24}, wantSource: "monitor_probe", wantStatus: accountMonitorScoreIneligible},
		{name: "no sample stays unscored", account: Account{Status: StatusActive, Schedulable: true}, status: accountMonitorScoreIneligible, evidence: AccountMonitorQualityEvidence{Source: "stale"}, wantSource: "stale", wantStatus: accountMonitorScoreIneligible},
		{name: "future cooldown uses native schedulability", account: Account{Status: StatusActive, Schedulable: true, TempUnschedulableUntil: timePtr(now.Add(time.Hour))}, status: accountMonitorScoreIneligible, evidence: valid, wantSource: "stale", wantStatus: accountMonitorScoreIneligible},
		{name: "t32 paused eligible remains eligible", account: Account{Status: StatusActive, Schedulable: false}, status: accountMonitorScoreEligible, evidence: valid, wantSource: "monitor_probe", wantStatus: accountMonitorScoreEligible, want: true},
		{name: "t32 abnormal cap remains capped", account: Account{Status: StatusActive, Schedulable: false}, status: accountMonitorScoreCapped, evidence: valid, wantSource: "monitor_probe", wantStatus: accountMonitorScoreCapped, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvidence, gotStatus, gotEligible := accountMonitorWindowScoreProjection(tt.account, tt.status, tt.evidence)
			if gotEvidence.Source != tt.wantSource || gotStatus != tt.wantStatus || gotEligible != tt.want {
				t.Fatalf("source=%q status=%q eligible=%v", gotEvidence.Source, gotStatus, gotEligible)
			}
		})
	}
}

func TestAccountMonitorListWindowRetainsSchedulableUnavailableAndStaleNativeScores(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.5
	accounts := []Account{
		{ID: 501, Name: "latest-unavailable", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
		{ID: 502, Name: "stale-current", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
		{ID: 503, Name: "pure-failure", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
		{ID: 504, Name: "no-sample", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
	}
	repo := &accountMonitorRepoStub{
		settings:      AccountMonitorSettings{IntervalSeconds: 300},
		globalWeights: AccountMonitorScoreWeights{Cost: 15, Success: 45, TTFT: 20, Latency: 20},
		groups:        []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		aggregates: map[int64]AccountMonitorAggregate{
			501: {SampleCount: 24, SuccessCount: 21, SuccessSampleCount: 21, ErrorCount: 3, SuccessRate: 21.0 / 24.0, ConsecutiveFailed: 3, LastCheckedAt: &now},
			502: {SampleCount: 24, SuccessCount: 24, SuccessSampleCount: 24, SuccessRate: 1, LastCheckedAt: timePtr(now.Add(-20 * time.Minute))},
			503: {SampleCount: 24, ErrorCount: 24, SuccessRate: 0, ConsecutiveFailed: 24, LastCheckedAt: &now},
		},
		latest: map[int64]AccountMonitorLatest{
			501: {Status: "failed", HTTPStatus: intPtr(http.StatusBadGateway), CheckedAt: now},
			502: {Status: "success", CheckedAt: now.Add(-20 * time.Minute)},
			503: {Status: "failed", HTTPStatus: intPtr(http.StatusBadGateway), CheckedAt: now},
		},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	global := accountMonitorRowsByID(page.Accounts)
	if row := global[501]; row.AvailabilityStatus != accountMonitorAvailabilityUnavailable || row.QualityScore == nil || row.GroupRank == nil || !row.Eligible {
		t.Fatalf("latest unavailable row = %#v, want unavailable with retained score/rank", row)
	}
	if row := global[502]; row.AvailabilityStatus != accountMonitorAvailabilityStale || !row.Stale || row.QualityScore == nil || row.GroupRank == nil || !row.Eligible {
		t.Fatalf("stale row = %#v, want stale with retained score/rank", row)
	}
	for _, id := range []int64{503, 504} {
		if row := global[id]; row.QualityScore != nil || row.GroupRank != nil || row.Eligible {
			t.Fatalf("account %d unexpectedly scored: %#v", id, row)
		}
	}
	byGroup := accountMonitorGroupRowsByID(page.Groups[0].Accounts)
	for _, id := range []int64{501, 502} {
		if row := byGroup[id]; row.QualityScore != nil || row.GroupRank != nil || row.Eligible || row.Evidence.Source != "stale" {
			t.Fatalf("group account %d used account-scoped evidence without a group-window projection: %#v", id, row)
		}
	}
	for _, id := range []int64{503, 504} {
		if row := byGroup[id]; row.QualityScore != nil || row.GroupRank != nil || row.Eligible {
			t.Fatalf("group account %d unexpectedly scored: %#v", id, row)
		}
	}
}

func TestAccountMonitorListWindowFallsBackToLatestHistoricalScoreWhenCurrentWindowHasNoScore(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.5
	account := Account{ID: 601, Name: "history-score", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate}
	historicalAt := now.Add(-2 * time.Hour)
	repo := &accountMonitorRepoStub{
		settings:      AccountMonitorSettings{IntervalSeconds: AccountMonitorDefaultIntervalSeconds},
		globalWeights: AccountMonitorScoreWeights{Cost: 15, Success: 45, TTFT: 20, Latency: 20},
		groups:        []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		aggregateResults: []map[int64]AccountMonitorAggregate{
			{},
			{601: {SampleCount: 12, SuccessSampleCount: 12, SuccessRate: 1, LastCheckedAt: &historicalAt}},
		},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{},
	}

	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	row := accountMonitorRowsByID(page.Accounts)[601]
	if row.QualityScore == nil || !row.Eligible || row.EvidenceSource != "historical_final" {
		t.Fatalf("row = %#v, want historical score fallback", row)
	}
	if row.SampleCount != 0 {
		t.Fatalf("current sample count = %d, want current window unchanged", row.SampleCount)
	}
}

func (s *accountMonitorMultiplierStub) Resolve(account *Account, _ time.Time) AccountMonitorMultiplier {
	if account != nil {
		if result, ok := s.results[account.ID]; ok {
			return result
		}
	}
	return s.result
}

func (s *accountMonitorMultiplierStub) Refresh(_ context.Context, account *Account, options AccountMonitorRefreshOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, accountMonitorMultiplierCall{accountID: account.ID, options: options})
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
	if len(accounts) != 3 || accounts[0].ID != 2 || accounts[1].ID != 6 || accounts[2].ID != 9 {
		t.Fatalf("accounts = %#v", accounts)
	}
	if !repo.listAllCalled || repo.listAllStatus != "" {
		t.Fatalf("ListAllWithFilters must not use the composite active filter: %#v", repo)
	}
	if repo.listSchedulableCalled {
		t.Fatal("account monitor must not use runtime scheduler eligibility")
	}
}

func TestAccountMonitorProjectMonitorV2GroupsUsesSchedulableScopes(t *testing.T) {
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)
	accounts := []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, GroupIDs: []int64{7, 7, 8}},
		{ID: 2, Status: StatusDisabled, Schedulable: true, GroupIDs: []int64{7}},
		{ID: 3, Status: StatusActive, Schedulable: false, GroupIDs: []int64{7}},
		{ID: 4, Status: StatusActive, Schedulable: true, AutoPauseOnExpired: true, ExpiresAt: &past, GroupIDs: []int64{7}},
		{ID: 5, Status: StatusActive, Schedulable: true, TempUnschedulableUntil: &future, GroupIDs: []int64{7}},
		{ID: 6, Status: StatusActive, Schedulable: true, RateLimitResetAt: &future, GroupIDs: []int64{7}},
		{ID: 7, Status: StatusActive, Schedulable: true, OverloadUntil: &future, GroupIDs: []int64{7}},
		{ID: 8, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey, Extra: map[string]any{"quota_limit": 1.0, "quota_used": 1.0}, GroupIDs: []int64{7}},
		{ID: 10, Status: StatusActive, Schedulable: true, GroupIDs: []int64{8}},
	}
	repo := &accountMonitorRepoStub{settings: AccountMonitorSettings{IntervalSeconds: 60}}
	accountRepo := &accountMonitorAccountRepoStub{accounts: accounts}
	svc := NewAccountMonitorService(repo, accountRepo, nil, nil, nil)
	start := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	_, err := svc.ProjectMonitorV2Groups(context.Background(), []int64{7, 8}, start, end, time.Hour)

	if err != nil {
		t.Fatalf("ProjectMonitorV2Groups() error = %v", err)
	}
	wantScopes := []MonitorV2GroupAccountScope{{GroupID: 7, AccountID: 1}, {GroupID: 8, AccountID: 1}, {GroupID: 8, AccountID: 10}}
	if !reflect.DeepEqual(repo.monitorV2Scopes, wantScopes) {
		t.Fatalf("native scopes = %#v, want %#v", repo.monitorV2Scopes, wantScopes)
	}
	if got, want := repo.monitorV2FreshSince, end.Add(-2*time.Minute); !got.Equal(want) {
		t.Fatalf("freshSince = %s, want %s", got, want)
	}
	if !repo.monitorV2Start.Equal(start) || !repo.monitorV2End.Equal(end) || repo.monitorV2BucketSize != time.Hour {
		t.Fatalf("native query bounds = start %s end %s bucket %s", repo.monitorV2Start, repo.monitorV2End, repo.monitorV2BucketSize)
	}
}

func TestAccountMonitorProjectMonitorV2GroupsPropagatesNativeRepositoryError(t *testing.T) {
	repo := &accountMonitorRepoStub{
		settings:               AccountMonitorSettings{IntervalSeconds: 60},
		monitorV2ProjectionErr: errors.New("native projection unavailable"),
	}
	accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{
		ID: 1, Status: StatusActive, Schedulable: true, GroupIDs: []int64{7},
	}}}
	svc := NewAccountMonitorService(repo, accountRepo, nil, nil, nil)

	_, err := svc.ProjectMonitorV2Groups(
		context.Background(),
		[]int64{7},
		time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC),
		time.Hour,
	)

	if err == nil || !strings.Contains(err.Error(), "native projection unavailable") {
		t.Fatalf("error = %v, want native projection error", err)
	}
}

func TestAccountMonitorRunAllUsesThirtyDayResultRetention(t *testing.T) {
	repo := &accountMonitorRepoStub{}
	accountRepo := &accountMonitorAccountRepoStub{}
	svc := NewAccountMonitorService(repo, accountRepo, nil, nil, nil)
	started := time.Now()
	if _, err := svc.RunAll(context.Background(), 1); err != nil {
		t.Fatalf("RunAll() error = %v", err)
	}
	if len(repo.deleteBeforeCalls) != 1 {
		t.Fatalf("DeleteBefore calls = %d, want 1", len(repo.deleteBeforeCalls))
	}
	cutoffAge := time.Since(repo.deleteBeforeCalls[0])
	if cutoffAge < 29*24*time.Hour || cutoffAge > 31*24*time.Hour {
		t.Fatalf("retention cutoff age = %s, want approximately 30 days (started %s)", cutoffAge, started)
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

func TestAccountMonitorListPoolKeepsActiveAccountsIncludingPaused(t *testing.T) {
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
	if len(accounts) != 3 || accounts[0].ID != 2 || accounts[1].ID != 6 || accounts[2].ID != 9 {
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
	if global[1].ManagementState != "paused" || global[1].ServiceState != "available" || global[1].AvailabilityStatus != accountMonitorAvailabilityNormal || global[1].GroupEligibility != "not_applicable" || global[1].MonitorBucket != "paused" || !global[1].Eligible {
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

func TestAccountMonitorClassifierFatalErrorsAreUnavailableImmediately(t *testing.T) {
	now := time.Date(2026, time.August, 7, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		message      string
		availability string
	}{
		{name: "balance exhausted", message: "insufficient quota remaining", availability: accountMonitorAvailabilityUnavailable},
		{name: "http unauthorized", message: "API returned 401: invalid credentials", availability: accountMonitorAvailabilityUnavailable},
		{name: "antigravity chinese unauthorized", message: "API 返回 401: request rejected", availability: accountMonitorAvailabilityUnavailable},
		{name: "antigravity chinese payment required", message: "API 返回 402: request rejected", availability: accountMonitorAvailabilityUnavailable},
		{name: "antigravity chinese forbidden", message: "API 返回 403: request rejected", availability: accountMonitorAvailabilityUnavailable},
		{name: "antigravity chinese server error", message: "API 返回 500: upstream unavailable", availability: accountMonitorAvailabilityAbnormal},
		{name: "missing api key", message: "No API key available", availability: accountMonitorAvailabilityUnavailable},
		{name: "authentication failed", message: "Chat Completions authentication failed", availability: accountMonitorAvailabilityUnavailable},
		{name: "http server error", message: "API returned 500: upstream unavailable", availability: accountMonitorAvailabilityAbnormal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAccountMonitorProbeResult(
				7,
				"gpt-4o-mini",
				now.Add(-time.Second),
				now,
				&accountMonitorProbeObserver{},
				errors.New(tt.message),
			)
			latest := AccountMonitorLatest{
				Status: result.Status, ErrorCode: result.ErrorCode, HTTPStatus: result.HTTPStatus, CheckedAt: result.CheckedAt,
			}
			got := accountMonitorAvailabilityStatus(accountMonitorManagementEnabled, false, 1, 1, latest)
			if got != tt.availability {
				t.Fatalf("classified result = %#v, availability = %q, want %q", result, got, tt.availability)
			}
		})
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

func TestAccountMonitorGroupQualityEvidenceStaysGroupScopedAndMarksStale(t *testing.T) {
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
	if row.Evidence.Source != "stale" || row.Evidence.SampleCount != 1 || row.Evidence.Known || row.QualityScore != nil {
		t.Fatalf("group evidence = %#v", row)
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

func TestAccountMonitorGroupQualityEvidenceErrorStaysUnknown(t *testing.T) {
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
	if evidence.Source != "unknown" || evidence.Known || evidence.SampleCount != 0 {
		t.Fatalf("evidence = %#v, want unknown group-isolated evidence", evidence)
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
		t.Fatalf("expired auto-paused account without cost evidence must remain ineligible: %#v", page.Groups[0].Accounts[0])
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

func TestAccountMonitorHealthUsesProbeWeightedPercentiles(t *testing.T) {
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
	if page.Health.TTFTP50MS == nil || *page.Health.TTFTP50MS != 180 || page.Health.LatencyP95MS == nil || *page.Health.LatencyP95MS != 330 {
		t.Fatalf("global percentiles must come from probe samples: %#v", page.Health)
	}
	groupHealth := page.Groups[0].Health
	if groupHealth.TTFTP50MS == nil || *groupHealth.TTFTP50MS != 130 || groupHealth.LatencyP95MS == nil || *groupHealth.LatencyP95MS != 300 {
		t.Fatalf("group percentiles must come from the combined request set: %#v", groupHealth)
	}
}

func TestAccountMonitorProbeHealthExcludesPausedAccounts(t *testing.T) {
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
	if len(repo.aggregateIDs) != 0 || page.Health.SuccessRate != 1 {
		t.Fatalf("probe health included paused accounts or queried request aggregate: ids=%v health=%#v", repo.aggregateIDs, page.Health)
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

func TestAccountMonitorEffectiveCostKeepsLegacyProcurementWindowCalculationForNonOpenAI(t *testing.T) {
	windowStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	effectiveAt := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	purchaseCost := 24.0

	got := accountMonitorEffectiveCost(Account{
		Platform:                   PlatformAnthropic,
		ProcurementCostCNY:         &purchaseCost,
		ProcurementCostEffectiveAt: &effectiveAt,
		ExpiresAt:                  &expiresAt,
	}, windowStart, windowEnd, 8)
	if got.Mode != "procurement" || got.WindowCost != 8 || got.EffectiveMultiplier == nil || *got.EffectiveMultiplier != 1 {
		t.Fatalf("procurement window cost = %#v, want CNY 8 and 1x", got)
	}
}

func TestAccountMonitorEffectiveCostUsesOpenAIAccountTypePrecedence(t *testing.T) {
	windowStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(24 * time.Hour)
	for _, tt := range []struct {
		name    string
		account Account
		want    float64
	}{
		{
			name: "procurement 4 CNY over 60 USD",
			account: Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				ProcurementCostCNY: floatPtr(4), EstimatedUsableQuotaUSD: floatPtr(60)},
			want: 4.0 / 60.0,
		},
		{
			name: "procurement 4 CNY over 120 USD",
			account: Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				ProcurementCostCNY: floatPtr(4), EstimatedUsableQuotaUSD: floatPtr(120)},
			want: 4.0 / 120.0,
		},
		{
			name: "API key ignores stale procurement fields",
			account: Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				RateMultiplier: floatPtr(0.75), ProcurementCostCNY: floatPtr(4), EstimatedUsableQuotaUSD: floatPtr(60)},
			want: 0.75,
		},
		{
			name: "non API key ignores stale rate multiplier",
			account: Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				RateMultiplier: floatPtr(0.75), ProcurementCostCNY: floatPtr(4), EstimatedUsableQuotaUSD: floatPtr(60)},
			want: 4.0 / 60.0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := accountMonitorEffectiveCost(tt.account, windowStart, windowEnd, 0)
			if got.EffectiveMultiplier == nil || math.Abs(*got.EffectiveMultiplier-tt.want) > 1e-9 {
				t.Fatalf("effective multiplier = %#v, want %.12f", got, tt.want)
			}
		})
	}
}

func TestRateMultiplierSingleSourceIgnoresLegacyProjectionEvidenceForCost(t *testing.T) {
	nativeRate := 0.25
	legacyProjectedRate := 0.75
	cost := accountMonitorProjectedEffectiveCost(Account{
		Platform:       PlatformOpenAI,
		Type:           AccountTypeAPIKey,
		RateMultiplier: &nativeRate,
	}, AccountMonitorMultiplier{
		Value:  &legacyProjectedRate,
		Source: "measured",
		Status: AccountMonitorMultiplierStatusOK,
	}, time.Time{}, time.Time{}, 0)

	if cost.EffectiveMultiplier == nil || math.Abs(*cost.EffectiveMultiplier-0.25) > 1e-9 {
		t.Fatalf("effective multiplier = %#v, want native account rate 0.25", cost.EffectiveMultiplier)
	}
	if score := accountMonitorCostScore(1, cost.EffectiveMultiplier, DefaultAccountMonitorScoreWeights); score != 11.25 {
		t.Fatalf("cost score = %v, want 11.25 from native account rate", score)
	}
}

func TestAccountMonitorMissingOpenAIProcurementQuotaRetainsQualityRanking(t *testing.T) {
	now := time.Now().UTC()
	accounts := []Account{
		{ID: 9, Name: "cost pending", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, ProcurementCostCNY: floatPtr(4)},
		{ID: 10, Name: "priced API key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: floatPtr(0.5)},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		aggregates: map[int64]AccountMonitorAggregate{
			9: freshSuccessfulProbeAggregate(now), 10: freshSuccessfulProbeAggregate(now),
		},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			9:  {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
			10: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
		},
		latest: map[int64]AccountMonitorLatest{9: {Status: "success", CheckedAt: now}, 10: {Status: "success", CheckedAt: now}},
	}
	activeMultiplier := 0.5
	multiplier := &accountMonitorMultiplierStub{results: map[int64]AccountMonitorMultiplier{
		9:  {Status: AccountMonitorMultiplierStatusUnavailable},
		10: {Value: &activeMultiplier, Status: AccountMonitorMultiplierStatusOK},
	}}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, multiplier).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	rows := page.Groups[0].Accounts
	byID := map[int64]AccountMonitorGroupAccount{rows[0].AccountID: rows[0], rows[1].AccountID: rows[1]}
	missing := byID[9]
	if missing.EffectiveMultiplier != nil || missing.CostScore != 0 || missing.QualityScore == nil || missing.GroupRank == nil {
		t.Fatalf("missing quota must retain quality ranking with zero cost score: %#v", missing)
	}
	if *missing.GroupRank != 2 {
		t.Fatalf("missing quota row rank = %d, want 2 behind lower-cost API key", *missing.GroupRank)
	}
}

func TestAccountMonitorMixedGroupRanksUsingNativeAPIKeyMultiplier(t *testing.T) {
	now := time.Now().UTC()
	persistedMultiplier := 1.0
	procurementCost := 4.0
	estimatedQuota := 60.0
	accounts := []Account{
		{
			ID: 20, Name: "measured API key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &persistedMultiplier,
			Extra: map[string]any{
				UpstreamBillingProbeExtraKey:   UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported},
				legacyMultiplierMeasurementKey: map[string]any{"value": 0.05},
			},
		},
		{
			ID: 21, Name: "procurement", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
			Status: StatusActive, Schedulable: true, GroupIDs: []int64{7},
			ProcurementCostCNY: &procurementCost, EstimatedUsableQuotaUSD: &estimatedQuota,
		},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "mixed", RateMultiplier: 1, CustomerVisible: true}},
		aggregates: map[int64]AccountMonitorAggregate{
			20: freshSuccessfulProbeAggregate(now), 21: freshSuccessfulProbeAggregate(now),
		},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			20: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
			21: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
		},
		latest: map[int64]AccountMonitorLatest{
			20: {Status: "success", CheckedAt: now},
			21: {Status: "success", CheckedAt: now},
		},
	}
	service := NewAccountMonitorService(
		repo,
		&accountMonitorAccountRepoStub{accounts: accounts},
		nil,
		nil,
		NewAccountMultiplierService(nil, nil, nil),
	)

	page, err := service.ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	rows := page.Groups[0].Accounts
	if len(rows) != 2 {
		t.Fatalf("group rows = %#v, want both accounts", rows)
	}
	byID := map[int64]AccountMonitorGroupAccount{rows[0].AccountID: rows[0], rows[1].AccountID: rows[1]}
	apiKey := byID[20]
	procurement := byID[21]
	if apiKey.Multiplier.Value == nil || math.Abs(*apiKey.Multiplier.Value-persistedMultiplier) > 1e-9 {
		t.Fatalf("card multiplier = %#v, want native %.4fx", apiKey.Multiplier, persistedMultiplier)
	}
	if apiKey.EffectiveMultiplier == nil || math.Abs(*apiKey.EffectiveMultiplier-persistedMultiplier) > 1e-9 {
		t.Fatalf("scoring multiplier = %#v, want native %.4fx", apiKey.EffectiveMultiplier, persistedMultiplier)
	}
	if procurement.EffectiveMultiplier == nil || math.Abs(*procurement.EffectiveMultiplier-4.0/60.0) > 1e-9 {
		t.Fatalf("procurement multiplier = %#v, want 4/60", procurement.EffectiveMultiplier)
	}
	if procurement.GroupRank == nil || *procurement.GroupRank != 1 || apiKey.GroupRank == nil || *apiKey.GroupRank != 2 {
		t.Fatalf("mixed ranking = %#v, want lower-cost procurement account first", rows)
	}
}

func TestAccountMonitorAPIKeyCostScoringIgnoresResolverEvidence(t *testing.T) {
	now := time.Now().UTC()
	persistedMultiplier := 1.0
	activeMultiplier := 0.2
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		aggregates: map[int64]AccountMonitorAggregate{
			20: freshSuccessfulProbeAggregate(now),
		},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			20: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
		},
		latest: map[int64]AccountMonitorLatest{20: {Status: "success", CheckedAt: now}},
	}
	service := NewAccountMonitorService(
		repo,
		&accountMonitorAccountRepoStub{accounts: []Account{{
			ID: 20, Name: "API key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, RateMultiplier: &persistedMultiplier,
		}}},
		nil,
		nil,
		&accountMonitorMultiplierStub{result: AccountMonitorMultiplier{
			Value: &activeMultiplier, Source: AccountMonitorMultiplierSourceDeclared, Status: AccountMonitorMultiplierStatusFailed,
		}},
	)

	page, err := service.ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	row := page.Accounts[0]
	if row.EffectiveMultiplier == nil || math.Abs(*row.EffectiveMultiplier-persistedMultiplier) > 1e-9 {
		t.Fatalf("scoring multiplier = %#v, want native %.4fx", row.EffectiveMultiplier, persistedMultiplier)
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
			113: {RequestCount: 3, SuccessCount: 3, BaseCost: 10, SuccessRate: 1, LastObservedAt: &now},
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

func TestAccountMonitorListWindowUsesFixedSevenDayProbeEvidenceForRecommendation(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.1
	testGroup := &Group{ID: 2, Name: "GPT-测试分组"}
	account := Account{ID: 201, Name: "test-account", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{2}, Groups: []*Group{testGroup}}
	day24 := freshProbeAggregate(now, 0.70)
	day7 := freshProbeAggregate(now, 0.99)
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups: []AccountMonitorGroup{
			{ID: 1, Name: "GPT-Pro", Status: StatusActive, RateMultiplier: 1},
			{ID: 2, Name: "GPT-测试分组", Status: StatusActive, RateMultiplier: 1},
		},
		aggregateResults: []map[int64]AccountMonitorAggregate{{201: day24}, {201: day7}},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{201: {RequestCount: 4}},
		latest:           map[int64]AccountMonitorLatest{201: {Status: "success", CheckedAt: now}},
	}

	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.aggregateCalls) != 2 || repo.aggregateCalls[0].Sub(repo.aggregateCalls[1]) < 6*24*time.Hour {
		t.Fatalf("aggregate calls = %#v, want requested 24h plus fixed 7d recommendation window", repo.aggregateCalls)
	}
	row := page.Accounts[0]
	if row.ProbeSuccessRate != day24.SuccessRate || row.SampleCount != day24.SampleCount+4 || row.EvidenceSource != "hybrid" {
		t.Fatalf("existing 24h metrics changed: probe_success=%.2f samples=%d source=%s", row.ProbeSuccessRate, row.SampleCount, row.EvidenceSource)
	}
	if row.GroupRecommendation == nil || row.GroupRecommendation.Target != AccountMonitorGroupRecommendationTargetPro {
		t.Fatalf("recommendation = %#v, want Pro from 7d probe evidence", row.GroupRecommendation)
	}
}

func TestAccountMonitorListWindowRecommendationScope(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.1
	groups := []*Group{
		{ID: 1, Name: "GPT-Pro"},
		{ID: 2, Name: "GPT-Plus"},
		{ID: 3, Name: "GPT-测试分组"},
		{ID: 4, Name: "malformed"},
	}
	accounts := []Account{
		{ID: 211, Name: "matching-pro", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{1}, Groups: []*Group{groups[0]}},
		{ID: 212, Name: "migrate-plus", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{2}, Groups: []*Group{groups[1]}},
		{ID: 213, Name: "disabled-plus", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusDisabled, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{2}, Groups: []*Group{groups[1]}},
		{ID: 214, Name: "test-account", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{3}, Groups: []*Group{groups[2]}},
		{ID: 215, Name: "malformed-group", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, RateMultiplier: &rate, GroupIDs: []int64{4}, Groups: []*Group{groups[3]}},
		{ID: 216, Name: "unschedulable-plus", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: false, RateMultiplier: &rate, GroupIDs: []int64{2}, Groups: []*Group{groups[1]}},
	}
	probes := make(map[int64]AccountMonitorAggregate, len(accounts))
	for _, account := range accounts {
		probes[account.ID] = freshProbeAggregate(now, 0.99)
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups: []AccountMonitorGroup{
			{ID: 1, Name: "GPT-Pro", Status: StatusActive, RateMultiplier: 1},
			{ID: 2, Name: "GPT-Plus", Status: StatusActive, RateMultiplier: 1},
			{ID: 3, Name: "GPT-测试分组", Status: StatusActive, RateMultiplier: 1},
			{ID: 4, Name: "malformed", Status: StatusActive, RateMultiplier: 1},
		},
		aggregates:       probes,
		aggregateResults: []map[int64]AccountMonitorAggregate{probes, probes},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{},
		latest:           map[int64]AccountMonitorLatest{211: {Status: "success", CheckedAt: now}, 212: {Status: "success", CheckedAt: now}, 213: {Status: "success", CheckedAt: now}, 214: {Status: "success", CheckedAt: now}, 215: {Status: "success", CheckedAt: now}, 216: {Status: "success", CheckedAt: now}},
	}

	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(originalLogger)
	service := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, accountMonitorConfirmedMultiplier(rate))
	service.recommend = func(account Account, currentGroupNames []string, groups []AccountMonitorGroup, evidence AccountMonitorQualityEvidence, latest AccountMonitorLatest, now time.Time) *AccountMonitorGroupRecommendation {
		if account.ID == 215 {
			panic("malformed recommendation input")
		}
		return EvaluateAccountMonitorGroupRecommendation(account, currentGroupNames, groups, evidence, latest, now)
	}
	page, err := service.ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[int64]AccountMonitorAccount, len(page.Accounts))
	for _, row := range page.Accounts {
		byID[row.AccountID] = row
	}
	if byID[211].GroupRecommendation != nil {
		t.Fatalf("matching formal group must remain nil: %#v", byID[211].GroupRecommendation)
	}
	if rec := byID[212].GroupRecommendation; rec == nil || rec.Action != AccountMonitorGroupRecommendationActionMigrate || rec.Target != AccountMonitorGroupRecommendationTargetPro {
		t.Fatalf("formal alternate target recommendation = %#v", rec)
	}
	if byID[213].GroupRecommendation != nil {
		t.Fatalf("disabled formal account must remain nil: %#v", byID[213].GroupRecommendation)
	}
	if byID[216].GroupRecommendation != nil {
		t.Fatalf("unschedulable formal account must remain nil: %#v", byID[216].GroupRecommendation)
	}
	if rec := byID[214].GroupRecommendation; rec == nil || rec.Status != AccountMonitorGroupRecommendationStatusRecommended {
		t.Fatalf("test group recommendation = %#v", rec)
	}
	if byID[215].GroupRecommendation != nil || len(page.Accounts) != len(accounts) {
		t.Fatalf("malformed group must be isolated to one row: row=%#v account_count=%d", byID[215].GroupRecommendation, len(page.Accounts))
	}
	if !strings.Contains(logs.String(), "account_id=215") || !strings.Contains(logs.String(), "recommendation evaluation failed") {
		t.Fatalf("isolated evaluator failure must emit warning log: %s", logs.String())
	}
}

func TestAccountMonitorListWindowProjectsRecentTimelineAndRanksGlobalScoreTiesByAccountID(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.5
	accounts := []Account{
		{ID: 10, Name: "global-tie-one", Status: StatusActive, Schedulable: true, RateMultiplier: &rate},
		{ID: 11, Name: "global-tie-two", Status: StatusActive, Schedulable: true, RateMultiplier: &rate},
		{ID: 20, Name: "global-tie-three", Status: StatusActive, Schedulable: true, RateMultiplier: &rate},
		{ID: 30, Name: "global-unranked", Status: StatusDisabled, Schedulable: false, RateMultiplier: &rate},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		aggregates: map[int64]AccountMonitorAggregate{
			10: freshSuccessfulProbeAggregate(now), 11: freshSuccessfulProbeAggregate(now), 20: freshSuccessfulProbeAggregate(now),
		},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			10: {RequestCount: 3, SuccessCount: 3, BaseCost: 1, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
			11: {RequestCount: 3, SuccessCount: 3, BaseCost: 1, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
			20: {RequestCount: 3, SuccessCount: 3, BaseCost: 1, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
		},
		latest: map[int64]AccountMonitorLatest{
			10: {Status: "success", CheckedAt: now},
			11: {Status: "success", CheckedAt: now},
			20: {Status: "success", CheckedAt: now},
		},
		timelines: map[int64][]AccountMonitorTimelinePoint{
			10: {{Status: "success", CheckedAt: now.Add(-time.Minute)}, {Status: "failed", CheckedAt: now}},
		},
	}

	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	if !slicesEqual(repo.timelineIDs, []int64{10, 11, 20, 30}) || repo.timelineLimit != AccountMonitorTimelineLimit {
		t.Fatalf("timeline query = ids %v limit %d, want ids [10 11 20 30] limit %d", repo.timelineIDs, repo.timelineLimit, AccountMonitorTimelineLimit)
	}
	if len(page.Accounts[0].Timeline) != 2 || page.Accounts[0].Timeline[1].Status != "failed" {
		t.Fatalf("timeline projection = %#v, want latest native points", page.Accounts[0].Timeline)
	}
	if len(page.Accounts) != 4 {
		t.Fatalf("top-level page accounts = %#v, want four accounts", page.Accounts)
	}
	if !slicesEqual([]int64{
		page.Accounts[0].AccountID,
		page.Accounts[1].AccountID,
		page.Accounts[2].AccountID,
		page.Accounts[3].AccountID,
	}, []int64{10, 11, 20, 30}) {
		t.Fatalf("top-level page accounts = %#v, want tied accounts ordered by account ID with unranked last", page.Accounts)
	}
	if page.Accounts[0].QualityScore == nil || page.Accounts[1].QualityScore == nil || page.Accounts[2].QualityScore == nil ||
		*page.Accounts[0].QualityScore != *page.Accounts[1].QualityScore || *page.Accounts[1].QualityScore != *page.Accounts[2].QualityScore {
		t.Fatalf("top-level quality scores = %#v, want equal scores for account IDs 10, 11, and 20", page.Accounts)
	}
	for index, wantRank := range []int{1, 2, 3} {
		if page.Accounts[index].GroupRank == nil || *page.Accounts[index].GroupRank != wantRank {
			t.Fatalf("top-level rank for account %d = %#v, want %d", page.Accounts[index].AccountID, page.Accounts[index].GroupRank, wantRank)
		}
	}
	if page.Accounts[3].GroupRank != nil {
		t.Fatalf("top-level unranked account = %#v, want nil rank", page.Accounts[3])
	}

	payload, err := json.Marshal(page.Accounts)
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(payload, &rows); err != nil {
		t.Fatal(err)
	}
	if rows[0]["quality_score"] == nil || rows[0]["group_rank"] != float64(1) || rows[1]["group_rank"] != float64(2) || rows[2]["group_rank"] != float64(3) || rows[3]["group_rank"] != nil {
		t.Fatalf("global score/rank JSON projection = %#v, want stable rankings 1, 2, 3, nil", rows)
	}
}

func TestAccountMonitorWindowCostReturnsZeroCostScoreForInvalidCostInputs(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	purchaseCost := 24.0

	procurement := accountMonitorEffectiveCost(Account{
		Platform:           PlatformOpenAI,
		Type:               AccountTypeOAuth,
		ProcurementCostCNY: &purchaseCost,
	}, start, end, 8)
	if procurement.CostScoreEligible || procurement.EffectiveMultiplier != nil {
		t.Fatalf("missing quota must have zero cost score: %#v", procurement)
	}

	multiplier := accountMonitorEffectiveCost(Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, start, end, 8)
	if multiplier.Mode != "multiplier" || !multiplier.CostScoreEligible || multiplier.EffectiveMultiplier == nil || *multiplier.EffectiveMultiplier != 1 {
		t.Fatalf("legacy nil multiplier must use BillingRateMultiplier default: %#v", multiplier)
	}
}

func TestAccountMonitorWindowIgnoresLegacyMeasurementForCostProjection(t *testing.T) {
	now := time.Now().UTC()
	value := 0.25
	account := Account{
		ID: 10, Name: "new-api", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &value,
		Extra: map[string]any{
			UpstreamBillingProbeExtraKey:   UpstreamBillingProbeSnapshot{Status: UpstreamBillingProbeStatusUnsupported},
			legacyMultiplierMeasurementKey: map[string]any{"value": 0.75, "status": "failed"},
		},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		aggregates: map[int64]AccountMonitorAggregate{
			10: freshSuccessfulProbeAggregate(now),
		},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			10: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
		},
		latest: map[int64]AccountMonitorLatest{10: {Status: "success", CheckedAt: now}},
	}
	page, err := NewAccountMonitorService(
		repo,
		&accountMonitorAccountRepoStub{accounts: []Account{account}},
		nil,
		nil,
		NewAccountMultiplierService(nil, nil, nil),
	).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	row := page.Groups[0].Accounts[0]
	if row.Multiplier.Status != AccountMonitorMultiplierStatusOK || row.Multiplier.Value == nil || *row.Multiplier.Value != value {
		t.Fatalf("projection must use native multiplier: %#v", row.Multiplier)
	}
	if row.EffectiveMultiplier == nil || *row.EffectiveMultiplier != value || row.CostScore <= 0 {
		t.Fatalf("native multiplier must remain in cost scoring: %#v", row)
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
		aggregates: map[int64]AccountMonitorAggregate{
			9: freshSuccessfulProbeAggregate(now), 10: freshSuccessfulProbeAggregate(now),
		},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			9:  {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
			10: {RequestCount: 3, SuccessCount: 3, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
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
	if len(rows) != 2 {
		t.Fatalf("group rows = %#v, want both accounts", rows)
	}
	byID := map[int64]AccountMonitorGroupAccount{rows[0].AccountID: rows[0], rows[1].AccountID: rows[1]}
	missing := byID[9]
	priced := byID[10]
	if !missing.Eligible || missing.CostScore != 0 || missing.EffectiveMultiplier == nil || *missing.EffectiveMultiplier != 1 || missing.GroupRank == nil {
		t.Fatalf("native default multiplier account must remain eligible and rankable: %#v", missing)
	}
	if !priced.Eligible || priced.CostScore <= 0 || priced.GroupRank == nil || *priced.GroupRank != 1 {
		t.Fatalf("priced account should receive multiplier cost score and rank first: %#v", priced)
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
		aggregates: map[int64]AccountMonitorAggregate{
			10: freshSuccessfulProbeAggregate(now),
			20: freshProbeAggregate(now, 0.5),
			30: freshProbeAggregate(now, 0.5),
		},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			10: {RequestCount: 3, SuccessCount: 3, BaseCost: 1, SuccessRate: 1, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
			20: {RequestCount: 3, SuccessCount: 1, BaseCost: 1, SuccessRate: 0.5, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
			30: {RequestCount: 3, SuccessCount: 1, BaseCost: 1, SuccessRate: 0.5, TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastObservedAt: &now},
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

func TestAccountMonitorWindowEvidenceCombinesRealRequestsAndProbes(t *testing.T) {
	now := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	probeTTFT := 900.0
	probeLatency := 5000.0
	realTTFT := 120.0
	realLatency := 800.0
	realOutputRate := 48.0
	probe := AccountMonitorAggregate{
		SampleCount: 4, SuccessSampleCount: 4, SuccessRate: 0.75,
		TTFTSampleCount: 4, LatencySampleCount: 4, TTFTP50MS: &probeTTFT, LatencyP95MS: &probeLatency,
		LastCheckedAt: &now,
	}
	latest := AccountMonitorLatest{Status: "success", CheckedAt: now}
	settings := AccountMonitorSettings{IntervalSeconds: 300}

	combined := accountMonitorWindowEvidence(
		AccountMonitorWindowAggregate{
			RequestCount: 2, SuccessCount: 2, SuccessRate: 1,
			TTFTSampleCount: 2, LatencySampleCount: 2, TTFTP50MS: &realTTFT, LatencyP95MS: &realLatency,
			OutputRateTokensPerSecond: &realOutputRate, OutputRateSampleCount: 2,
			LastObservedAt: &now,
		}, probe, latest, settings, now,
	)
	if combined.Source != "hybrid" || combined.SampleCount != 6 || combined.SuccessSampleCount != 6 || combined.SuccessRate != 1 {
		t.Fatalf("real requests and probes should be combined: %#v", combined)
	}
	if combined.TTFTSampleCount != 2 || combined.TTFTP50MS == nil || *combined.TTFTP50MS != realTTFT || combined.LatencySampleCount != 2 || combined.LatencyP95MS == nil || *combined.LatencyP95MS != realLatency {
		t.Fatalf("real request latency metrics should take precedence when present: %#v", combined)
	}
	if combined.OutputRateTokensPerSecond == nil || *combined.OutputRateTokensPerSecond != realOutputRate || combined.OutputRateSampleCount != 2 {
		t.Fatalf("real request output rate should take precedence when present: %#v", combined)
	}

	probeFallback := accountMonitorWindowEvidence(
		AccountMonitorWindowAggregate{RequestCount: 2, SuccessCount: 2, LastObservedAt: &now}, probe, latest, settings, now,
	)
	if probeFallback.TTFTSampleCount != probe.TTFTSampleCount || probeFallback.TTFTP50MS == nil || *probeFallback.TTFTP50MS != probeTTFT || probeFallback.LatencySampleCount != probe.LatencySampleCount || probeFallback.LatencyP95MS == nil || *probeFallback.LatencyP95MS != probeLatency {
		t.Fatalf("probe latency metrics should fill missing real request metrics: %#v", probeFallback)
	}
	if probeFallback.OutputRateTokensPerSecond != nil || probeFallback.OutputRateSampleCount != 0 {
		t.Fatalf("probe-only evidence must not invent an output rate: %#v", probeFallback)
	}

	realOnly := accountMonitorWindowEvidence(
		AccountMonitorWindowAggregate{RequestCount: 3, SuccessCount: 2, SuccessRate: 2.0 / 3.0, LastObservedAt: &now}, AccountMonitorAggregate{}, AccountMonitorLatest{}, settings, now,
	)
	if realOnly.Source != "stale" || realOnly.SampleCount != 0 || realOnly.SuccessSampleCount != 0 {
		t.Fatalf("real request evidence without a fresh probe should remain gated: %#v", realOnly)
	}
}

func TestAccountMonitorOutputRateTokensPerSecondUsesOnlyValidGenerationEvidence(t *testing.T) {
	tests := []struct {
		name         string
		outputTokens int64
		generationMS float64
		sampleCount  int
		want         *float64
	}{
		{name: "normal", outputTokens: 240, generationMS: 4000, sampleCount: 2, want: floatPtr(60)},
		{name: "no output tokens", generationMS: 4000, sampleCount: 2},
		{name: "no valid samples", outputTokens: 240, generationMS: 4000},
		{name: "nonpositive generation", outputTokens: 240, sampleCount: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AccountMonitorOutputRateTokensPerSecond(tt.outputTokens, tt.generationMS, tt.sampleCount)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("output rate = %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Fatalf("output rate = %v, want %v", got, *tt.want)
			}
		})
	}
}

func TestAccountMonitorQualityScorePrefersOutputRateAndFallsBackToLatency(t *testing.T) {
	weights := AccountMonitorScoreWeights{Cost: 0, Success: 1, TTFT: 1, Latency: 10, TTFTTargetMS: 100, TTFTLimitMS: 1000, LatencyTargetMS: 1000, LatencyLimitMS: 5000}
	fastRate := 256.0
	slowRate := 64.0
	ttft := 100.0
	latency := 2000.0
	base := AccountMonitorQualityEvidence{Known: true, SampleCount: 2, SuccessRate: 1, TTFTP50MS: &ttft, LatencyP95MS: &latency}
	fast := base
	fast.OutputRateTokensPerSecond = &fastRate
	fast.OutputRateSampleCount = 2
	slow := base
	slow.OutputRateTokensPerSecond = &slowRate
	slow.OutputRateSampleCount = 2

	fastScore := CalculateAccountMonitorQualityScore(1, 1, weights, fast)
	slowScore := CalculateAccountMonitorQualityScore(1, 1, weights, slow)
	legacyScore := CalculateAccountMonitorQualityScore(1, 1, weights, base)
	if fastScore == nil || slowScore == nil || legacyScore == nil || *fastScore <= *slowScore {
		t.Fatalf("higher output rate should win: fast=%v slow=%v legacy=%v", fastScore, slowScore, legacyScore)
	}
	if *legacyScore != 10 { // success + TTFT + latency compatibility fallback (8).
		t.Fatalf("legacy latency fallback score = %v, want 10", *legacyScore)
	}
}

func TestAccountMonitorQualityEvidenceOutputRateJSONCompatibilityAndExplanationLabel(t *testing.T) {
	rate := 32.0
	known := AccountMonitorQualityEvidence{OutputRateTokensPerSecond: &rate, OutputRateSampleCount: 3}
	body, err := json.Marshal(known)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"output_rate_tokens_per_second":32`) || !strings.Contains(string(body), `"output_rate_sample_count":3`) {
		t.Fatalf("known output rate JSON = %s", body)
	}
	unknown, err := json.Marshal(AccountMonitorQualityEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(unknown), "output_rate") {
		t.Fatalf("unknown output rate should remain omitted: %s", unknown)
	}
	if got := accountMonitorExperienceLabel(known); got != "生成体验（输出速率）" {
		t.Fatalf("known experience label = %q", got)
	}
	if got := accountMonitorExperienceLabel(AccountMonitorQualityEvidence{LatencyP95MS: &rate}); got != "生成体验（完整响应 P95 兼容回退）" {
		t.Fatalf("fallback experience label = %q", got)
	}
}

func TestFuseAccountMonitorQualityEvidenceUsesSampleProportionalWeightsAndUnknownSemantics(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	settings := AccountMonitorSettings{IntervalSeconds: 300}
	probeAt := now.Add(-time.Minute)
	staleAt := now.Add(-20 * time.Minute)

	tests := []struct {
		name        string
		real        AccountMonitorWindowAggregate
		probe       AccountMonitorAggregate
		wantSource  string
		wantFresh   string
		wantSamples int
		wantRealWt  float64
		wantProbeWt float64
		wantRate    float64
		wantKnown   bool
		wantReason  string
	}{
		{
			name:       "real only",
			real:       AccountMonitorWindowAggregate{RequestCount: 5, SuccessCount: 4, LastObservedAt: &probeAt},
			wantSource: "real_request", wantFresh: "fresh", wantSamples: 5, wantRealWt: 1, wantRate: .8, wantKnown: true,
		},
		{
			name:       "probe only",
			probe:      AccountMonitorAggregate{SampleCount: 4, SuccessSampleCount: 3, LastCheckedAt: &probeAt},
			wantSource: "monitor_probe", wantFresh: "fresh", wantSamples: 4, wantProbeWt: 1, wantRate: .75, wantKnown: true,
		},
		{
			name:       "real dominates after threshold",
			real:       AccountMonitorWindowAggregate{RequestCount: 5, SuccessCount: 5, LastObservedAt: &probeAt},
			probe:      AccountMonitorAggregate{SampleCount: 20, SuccessSampleCount: 0, LastCheckedAt: &probeAt},
			wantSource: "hybrid", wantFresh: "fresh", wantSamples: 25, wantRealWt: .2, wantProbeWt: .8, wantRate: .2, wantKnown: true,
		},
		{
			name:       "probe supplements sparse real traffic",
			real:       AccountMonitorWindowAggregate{RequestCount: 1, SuccessCount: 1, LastObservedAt: &probeAt},
			probe:      AccountMonitorAggregate{SampleCount: 4, SuccessSampleCount: 0, LastCheckedAt: &probeAt},
			wantSource: "hybrid", wantFresh: "fresh", wantSamples: 5, wantRealWt: .2, wantProbeWt: .8, wantRate: .2, wantKnown: true,
		},
		{
			name:       "stale evidence is unknown",
			real:       AccountMonitorWindowAggregate{RequestCount: 5, SuccessCount: 5, LastObservedAt: &staleAt},
			wantSource: "unknown", wantFresh: "stale", wantSamples: 0, wantKnown: false, wantReason: "stale",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuseAccountMonitorQualityEvidence(tt.real, tt.probe, AccountMonitorLatest{}, settings, now)
			if got.Source != tt.wantSource || got.Freshness != tt.wantFresh || got.SampleCount != tt.wantSamples || got.Known != tt.wantKnown || got.UnknownReason != tt.wantReason {
				t.Fatalf("evidence = %#v", got)
			}
			if got.RealRequestWeight != tt.wantRealWt || got.ProbeWeight != tt.wantProbeWt {
				t.Fatalf("weights = %v/%v, want %v/%v", got.RealRequestWeight, got.ProbeWeight, tt.wantRealWt, tt.wantProbeWt)
			}
			if got.SuccessRate != tt.wantRate {
				t.Fatalf("success rate = %v, want %v", got.SuccessRate, tt.wantRate)
			}
		})
	}
}

func TestAccountMonitorQualityEvidenceReadErrorIsUnknown(t *testing.T) {
	evidence := accountMonitorUnknownQualityEvidence("read_error")
	if evidence.Known || evidence.Source != "unknown" || evidence.Freshness != "unknown" || evidence.UnknownReason != "read_error" {
		t.Fatalf("read error evidence = %#v", evidence)
	}
}

func TestAccountMonitorQualityEvidenceDoesNotIntroduceModelDimension(t *testing.T) {
	evidence := fuseAccountMonitorQualityEvidence(
		AccountMonitorWindowAggregate{RequestCount: 3, SuccessCount: 3},
		AccountMonitorAggregate{},
		AccountMonitorLatest{},
		AccountMonitorSettings{IntervalSeconds: 300},
		time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC),
	)
	body, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "model_id") || strings.Contains(string(body), "model") {
		t.Fatalf("quality evidence unexpectedly has model dimension: %s", body)
	}
}

func TestAccountMonitorWindowStateIgnoresRealRequestsAndUsesProbeTime(t *testing.T) {
	now := time.Now().UTC()
	observedAt := now.Add(-5 * time.Minute)
	latestCheckedAt := now.Add(-time.Minute)
	rate := 0.5
	accounts := []Account{
		{ID: 118, Name: "real-success-78", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
		{ID: 119, Name: "real-success-742", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
		{ID: 120, Name: "real-all-failed", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
		{ID: 121, Name: "probe-fallback", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			118: {RequestCount: 78, SuccessCount: 78, ErrorCount: 0, SuccessRate: 1, LastObservedAt: &observedAt},
			119: {RequestCount: 742, SuccessCount: 742, ErrorCount: 0, SuccessRate: 1, LastObservedAt: &observedAt},
			120: {RequestCount: 3, SuccessCount: 0, ErrorCount: 3, SuccessRate: 0, LastObservedAt: &observedAt},
			121: {RequestCount: 2, SuccessCount: 2, ErrorCount: 0, SuccessRate: 1, LastObservedAt: &observedAt},
		},
		aggregates: map[int64]AccountMonitorAggregate{
			121: {SampleCount: 4, SuccessCount: 4, SuccessSampleCount: 4, SuccessRate: 1, LastCheckedAt: &latestCheckedAt},
		},
		latest: map[int64]AccountMonitorLatest{
			118: {Status: "failed", CheckedAt: latestCheckedAt},
			119: {Status: "failed", CheckedAt: latestCheckedAt},
			120: {Status: "success", CheckedAt: latestCheckedAt},
			121: {Status: "success", CheckedAt: latestCheckedAt},
		},
		timelines: map[int64][]AccountMonitorTimelinePoint{
			118: {{Status: "failed", CheckedAt: latestCheckedAt}},
			119: {{Status: "failed", CheckedAt: latestCheckedAt}},
		},
	}

	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}

	globalByID := make(map[int64]AccountMonitorAccount, len(page.Accounts))
	for _, row := range page.Accounts {
		globalByID[row.AccountID] = row
	}
	for _, accountID := range []int64{118, 119, 120} {
		row := globalByID[accountID]
		if row.ServiceState != accountMonitorServicePending || row.MonitorBucket != accountMonitorServicePending || row.Eligible || row.GroupRank != nil {
			t.Fatalf("global request-only account %d = %#v, want pending and unranked", accountID, row)
		}
	}
	if row := globalByID[121]; row.ServiceState != accountMonitorServiceAvailable || !row.Eligible || row.GroupRank == nil {
		t.Fatalf("global probe fallback account = %#v, want existing latest-probe success behavior", row)
	}

	groupByID := make(map[int64]AccountMonitorGroupAccount, len(page.Groups[0].Accounts))
	for _, row := range page.Groups[0].Accounts {
		groupByID[row.AccountID] = row
	}
	for _, accountID := range []int64{118, 119, 120} {
		row := groupByID[accountID]
		if row.ServiceState != accountMonitorServicePending || row.MonitorBucket != accountMonitorServicePending || row.Eligible || row.GroupRank != nil {
			t.Fatalf("group request-only account %d = %#v, want pending and unranked", accountID, row)
		}
	}
	if row := groupByID[121]; row.Evidence.Source != "hybrid" || row.ServiceState != accountMonitorServiceAvailable || !row.Eligible || row.GroupRank == nil {
		t.Fatalf("group probe fallback account = %#v, want hybrid and available", row)
	}
}

func TestAccountMonitorWindowEvidenceWithoutProbesIsStale(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	latestCheckedAt := now.Add(-time.Minute)
	evidence := accountMonitorWindowEvidence(
		AccountMonitorWindowAggregate{RequestCount: 3, SuccessCount: 0, SuccessRate: 0.5},
		AccountMonitorAggregate{},
		AccountMonitorLatest{Status: "success", CheckedAt: latestCheckedAt},
		AccountMonitorSettings{IntervalSeconds: 300},
		now,
	)
	if evidence.Source != "unknown" || evidence.SuccessSampleCount != 0 {
		t.Fatalf("missing probe aggregate evidence = %#v, want unknown", evidence)
	}
	if evidence.ObservedAt.IsZero() || !evidence.ObservedAt.Equal(latestCheckedAt) {
		t.Fatalf("probe fallback observed_at = %s, want latest probe time %s", evidence.ObservedAt, latestCheckedAt)
	}
}

func TestAccountMonitorWindowWithoutProbeSamplesStaysPending(t *testing.T) {
	now := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	if got := accountMonitorAvailabilityStatus(accountMonitorManagementEnabled, true, 0, 0, AccountMonitorLatest{}); got != accountMonitorAvailabilityStale {
		t.Fatalf("missing probe samples = %q, want stale", got)
	}
	_ = now
}

func TestAccountMonitorWindowThresholdQualifiedRealRequestsIgnoreAbsentLatestInGlobalAndGroupProjections(t *testing.T) {
	now := time.Now().UTC()
	observedAt := now.Add(-time.Hour)
	rate := 0.5
	accounts := []Account{
		{ID: 201, Name: "threshold-success", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
		{ID: 202, Name: "threshold-failed", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
	}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			201: {RequestCount: 3, SuccessCount: 3, ErrorCount: 0, SuccessRate: 1, LastObservedAt: &observedAt},
			202: {RequestCount: 3, SuccessCount: 0, ErrorCount: 3, SuccessRate: 0, LastObservedAt: &observedAt},
		},
	}

	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}

	globalByID := make(map[int64]AccountMonitorAccount, len(page.Accounts))
	for _, row := range page.Accounts {
		globalByID[row.AccountID] = row
	}
	if row := globalByID[201]; row.ServiceState != accountMonitorServicePending || row.Eligible || row.GroupRank != nil || row.Latest != nil {
		t.Fatalf("global request-only success = %#v, want pending and unranked without probe", row)
	}
	if row := globalByID[202]; row.ServiceState != accountMonitorServicePending || row.Eligible || row.GroupRank != nil || row.Latest != nil {
		t.Fatalf("global request-only failure = %#v, want pending and unranked without probe", row)
	}

	groupByID := make(map[int64]AccountMonitorGroupAccount, len(page.Groups[0].Accounts))
	for _, row := range page.Groups[0].Accounts {
		groupByID[row.AccountID] = row
	}
	if row := groupByID[201]; row.ServiceState != accountMonitorServicePending || row.Eligible || row.GroupRank != nil || row.Latest != nil {
		t.Fatalf("group request-only success = %#v, want pending and unranked without probe", row)
	}
	if row := groupByID[202]; row.ServiceState != accountMonitorServicePending || row.Eligible || row.GroupRank != nil || row.Latest != nil {
		t.Fatalf("group request-only failure = %#v, want pending and unranked without probe", row)
	}
}

func TestAccountMonitorWindowSubthresholdRealRequestsKeepLatestProbeGateInGlobalAndGroupProjections(t *testing.T) {
	now := time.Now().UTC()
	observedAt := now.Add(-time.Hour)
	rate := 0.5
	account := Account{ID: 203, Name: "subthreshold-success", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			203: {RequestCount: 2, SuccessCount: 2, ErrorCount: 0, SuccessRate: 1, LastObservedAt: &observedAt},
		},
		latest: map[int64]AccountMonitorLatest{
			203: {Status: "failed", CheckedAt: now},
		},
	}

	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	if row := page.Accounts[0]; row.ServiceState != accountMonitorServicePending || row.Eligible || row.GroupRank != nil || row.LatestStatus != "failed" {
		t.Fatalf("global request-only success = %#v, want pending and unranked", row)
	}
	if row := page.Groups[0].Accounts[0]; row.ServiceState != accountMonitorServicePending || row.Eligible || row.GroupRank != nil || row.LatestStatus != "failed" {
		t.Fatalf("group request-only success = %#v, want pending and unranked", row)
	}
}

func TestSummarizeHealthSamplesWeightsSuccessRateByRequestSamples(t *testing.T) {
	summary := summarizeHealthSamples([]accountMonitorHealthSample{
		{serviceState: accountMonitorServiceAvailable, sampleCount: 100, successSampleCount: 1, successRate: 0.01},
		{serviceState: accountMonitorServiceAvailable, sampleCount: 1, successSampleCount: 1, successRate: 1},
	})
	if summary.SuccessSampleCount != 2 {
		t.Fatalf("success sample count = %d, want 2", summary.SuccessSampleCount)
	}
	if summary.SuccessRate < 0.019 || summary.SuccessRate > 0.020 {
		t.Fatalf("success rate = %v, want approximately 2/101", summary.SuccessRate)
	}
}

func TestSummarizeHealthSamplesPreservesZeroSuccessForAllFailedWindow(t *testing.T) {
	summary := summarizeHealthSamples([]accountMonitorHealthSample{
		{serviceState: accountMonitorServiceUnavailable, sampleCount: 4, successSampleCount: 0, successRate: 0},
	})
	if summary.SuccessSampleCount != 0 || summary.SuccessRate != 0 {
		t.Fatalf("all-failed summary = %#v, want zero successful samples and rate", summary)
	}
}

func TestAccountMonitorWindowCostKeepsNativeMultiplierWithoutWindowBaseCost(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	rate := 0.75

	got := accountMonitorEffectiveCost(Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, RateMultiplier: &rate}, start, end, 0)
	if got.Mode != "multiplier" || !got.CostScoreEligible || got.EffectiveMultiplier == nil || *got.EffectiveMultiplier != rate {
		t.Fatalf("native multiplier with no real request cost = %#v, want effective multiplier %.2f", got, rate)
	}
}

func TestAccountMonitorEffectiveCostProcurementDoesNotDependOnWindowBaseCost(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	purchaseCost := 24.0
	quota := 60.0

	got := accountMonitorEffectiveCost(Account{
		Platform:                PlatformOpenAI,
		Type:                    AccountTypeOAuth,
		ProcurementCostCNY:      &purchaseCost,
		EstimatedUsableQuotaUSD: &quota,
	}, start, end, 0)
	if got.Mode != "procurement" || !got.CostScoreEligible || got.EffectiveMultiplier == nil || *got.EffectiveMultiplier != 0.4 {
		t.Fatalf("procurement zero-base-cost result = %#v, want 0.4x", got)
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

func floatPtr(value float64) *float64    { return &value }
func timePtr(value time.Time) *time.Time { return &value }

func freshSuccessfulProbeAggregate(now time.Time) AccountMonitorAggregate {
	return freshProbeAggregate(now, 1)
}

func freshProbeAggregate(now time.Time, successRate float64) AccountMonitorAggregate {
	samples := 24
	successes := int(math.Round(float64(samples) * successRate))
	return AccountMonitorAggregate{
		SampleCount: samples, SuccessCount: successes, SuccessSampleCount: successes,
		ErrorCount: samples - successes, SuccessRate: successRate,
		TTFTSampleCount: successes, LatencySampleCount: successes,
		TTFTP50MS: floatPtr(100), LatencyP95MS: floatPtr(200), LastCheckedAt: timePtr(now),
	}
}

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

func TestAccountMonitorProjectionExposesExplainableQualityAndSchedulerRanks(t *testing.T) {
	qualityRank, schedulerRank, legacyRank := 2, 1, 2
	row := AccountMonitorAccount{
		AccountID:            7,
		QualityRank:          &qualityRank,
		QualityRankTotal:     5,
		SchedulerRank:        &schedulerRank,
		SchedulerRankTotal:   4,
		GroupRank:            &legacyRank,
		QualityExplanation:   &AccountMonitorQualityExplanation{Window: "24h", SampleCount: 12},
		SchedulerExplanation: &AccountMonitorSchedulerExplanation{PolicyLabel: "利润优先", PrimaryReasonCode: "strategy"},
	}
	body, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["quality_rank"] != float64(2) {
		t.Fatalf("quality_rank = %v", payload["quality_rank"])
	}
	if payload["quality_rank_total"] != float64(5) {
		t.Fatalf("quality_rank_total = %v", payload["quality_rank_total"])
	}
	if payload["scheduler_rank"] != float64(1) {
		t.Fatalf("scheduler_rank = %v", payload["scheduler_rank"])
	}
	if payload["scheduler_rank_total"] != float64(4) {
		t.Fatalf("scheduler_rank_total = %v", payload["scheduler_rank_total"])
	}
	if payload["group_rank"] != float64(2) {
		t.Fatalf("group_rank = %v", payload["group_rank"])
	}
	if payload["quality_explanation"] == nil {
		t.Fatal("quality_explanation missing")
	}
	if payload["scheduler_explanation"] == nil {
		t.Fatal("scheduler_explanation missing")
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
	service.SetActiveProbeUsageReader(&modelDetectionUsageStub{})

	completed, err := service.RunAll(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if completed != 1 || len(monitorRepo.results) != 1 {
		t.Fatalf("completed=%d results=%#v", completed, monitorRepo.results)
	}
	if len(multiplier.calls) != 1 || multiplier.calls[0] != (accountMonitorMultiplierCall{accountID: 21, options: AccountMonitorRefreshOptions{
		RefreshDeclaration: true, RefreshBalance: true,
	}}) {
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
	if len(multiplier.calls) != 1 || multiplier.calls[0] != (accountMonitorMultiplierCall{accountID: 23, options: AccountMonitorRefreshOptions{
		RefreshDeclaration: true, RefreshBalance: true,
	}}) {
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
	service.SetActiveProbeUsageReader(&modelDetectionUsageStub{})
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
	service.SetActiveProbeUsageReader(&modelDetectionUsageStub{})
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
	service.SetActiveProbeUsageReader(&modelDetectionUsageStub{})
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
	service.SetActiveProbeUsageReader(&modelDetectionUsageStub{})
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

func TestAccountMonitorProbeShouldStopOnlyForClosedAccountHTTP4xxOr5xx(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name     string
		account  Account
		latest   AccountMonitorLatest
		wantStop bool
	}{
		{name: "paused success continues", account: Account{Status: StatusActive, Schedulable: false}, latest: AccountMonitorLatest{Status: "success", CheckedAt: now}},
		{name: "paused 401 stops", account: Account{Status: StatusActive, Schedulable: false}, latest: AccountMonitorLatest{Status: "failed", HTTPStatus: intPtr(http.StatusUnauthorized), CheckedAt: now}, wantStop: true},
		{name: "paused 500 stops", account: Account{Status: StatusActive, Schedulable: false}, latest: AccountMonitorLatest{Status: "failed", HTTPStatus: intPtr(http.StatusInternalServerError), CheckedAt: now}, wantStop: true},
		{name: "schedulable 500 continues", account: Account{Status: StatusActive, Schedulable: true}, latest: AccountMonitorLatest{Status: "failed", HTTPStatus: intPtr(http.StatusInternalServerError), CheckedAt: now}},
		{name: "paused timeout continues", account: Account{Status: StatusActive, Schedulable: false}, latest: AccountMonitorLatest{Status: "failed", ErrorCode: "timeout", CheckedAt: now}},
		{name: "paused without evidence continues", account: Account{Status: StatusActive, Schedulable: false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := accountMonitorProbeShouldStop(tt.account, tt.latest); got != tt.wantStop {
				t.Fatalf("stop=%v, want %v", got, tt.wantStop)
			}
		})
	}
}

func TestAccountMonitorPausedProbeProjectionScoresRanksAndKeepsNoEvidencePending(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.5
	pausedSuccess := Account{ID: 401, Name: "paused-success", Status: StatusActive, Schedulable: false, GroupIDs: []int64{7}, RateMultiplier: &rate}
	pausedHTTPError := Account{ID: 402, Name: "paused-http-error", Status: StatusActive, Schedulable: false, GroupIDs: []int64{7}, RateMultiplier: &rate}
	pausedMissing := Account{ID: 403, Name: "paused-missing", Status: StatusActive, Schedulable: false, GroupIDs: []int64{7}, RateMultiplier: &rate}
	activeSuccess := Account{ID: 404, Name: "active-success", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		aggregates: map[int64]AccountMonitorAggregate{
			401: {SampleCount: 8, SuccessCount: 8, SuccessSampleCount: 8, SuccessRate: 1, LastCheckedAt: &now},
			402: {SampleCount: 8, ErrorCount: 8, LastCheckedAt: &now},
			404: {SampleCount: 8, SuccessCount: 8, SuccessSampleCount: 8, SuccessRate: 1, LastCheckedAt: &now},
		},
		latest: map[int64]AccountMonitorLatest{
			401: {Status: "success", CheckedAt: now},
			402: {Status: "failed", HTTPStatus: intPtr(http.StatusBadGateway), CheckedAt: now},
			404: {Status: "success", CheckedAt: now},
		},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{pausedSuccess, pausedHTTPError, pausedMissing, activeSuccess}}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[int64]AccountMonitorAccount, len(page.Accounts))
	for _, row := range page.Accounts {
		byID[row.AccountID] = row
	}
	if row := byID[401]; row.AvailabilityStatus != accountMonitorAvailabilityNormal || row.QualityScore == nil || !row.Eligible || row.GroupRank == nil {
		t.Fatalf("paused successful probe projection = %#v, want normal/scored/ranked", row)
	}
	if row := byID[402]; row.GroupRank != nil || row.QualityScore != nil || row.AvailabilityStatus != accountMonitorAvailabilityUnavailable {
		t.Fatalf("paused HTTP error projection = %#v, want unavailable/unranked", row)
	}
	if row := byID[403]; row.GroupRank != nil || row.QualityScore != nil || row.ServiceState != accountMonitorServicePending {
		t.Fatalf("paused missing probe projection = %#v, want pending/unranked", row)
	}
	if row := byID[404]; row.QualityScore == nil || row.GroupRank == nil {
		t.Fatalf("active successful probe projection = %#v, want scored/ranked", row)
	}
}

func TestAccountMonitorRunAllSkipsUnschedulableAccounts(t *testing.T) {
	makeService := func(latest AccountMonitorLatest) (*AccountMonitorService, *accountMonitorRepoStub, *int) {
		repo := &accountMonitorRepoStub{
			latest:   map[int64]AccountMonitorLatest{405: latest},
			settings: AccountMonitorSettings{IntervalSeconds: 300},
		}
		accountRepo := &accountMonitorAccountRepoStub{accounts: []Account{{ID: 405, Status: StatusActive, Schedulable: false}}}
		service := NewAccountMonitorService(repo, accountRepo, nil, nil, nil)
		calls := 0
		service.probeConnection = func(context.Context, int64, string, string, string) (AccountMonitorProbeResult, error) {
			calls++
			return AccountMonitorProbeResult{Status: "success", CheckedAt: time.Now().UTC()}, nil
		}
		return service, repo, &calls
	}

	service, repo, calls := makeService(AccountMonitorLatest{Status: "success", CheckedAt: time.Now().UTC()})
	service.SetActiveProbeUsageReader(&modelDetectionUsageStub{})
	if completed, err := service.RunAll(context.Background(), 1); err != nil {
		t.Fatal(err)
	} else if completed != 0 {
		t.Fatalf("closed success run completed=%d, want no probe", completed)
	}
	if *calls != 0 || len(repo.results) != 0 {
		t.Fatalf("closed success run = calls %d results %d, want skipped", *calls, len(repo.results))
	}

	service, repo, calls = makeService(AccountMonitorLatest{Status: "failed", HTTPStatus: intPtr(http.StatusBadGateway), CheckedAt: time.Now().UTC()})
	service.SetActiveProbeUsageReader(&modelDetectionUsageStub{})
	if completed, err := service.RunAll(context.Background(), 1); err != nil {
		t.Fatal(err)
	} else if completed != 0 {
		t.Fatalf("closed HTTP error run completed=%d, want no probe", completed)
	}
	if *calls != 0 || len(repo.results) != 0 {
		t.Fatalf("closed HTTP error run = calls %d results %d, want stopped", *calls, len(repo.results))
	}
}

func TestAccountMonitorRunAllOnlyProbesEmptyUsageBucket(t *testing.T) {
	account := Account{ID: 407, Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI}
	newService := func(reader ActiveProbeUsageWindowReader) (*AccountMonitorService, *accountMonitorRepoStub, *int) {
		repo := &accountMonitorRepoStub{}
		svc := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, nil)
		svc.SetActiveProbeUsageReader(reader)
		calls := 0
		svc.probeConnection = func(context.Context, int64, string, string, string) (AccountMonitorProbeResult, error) {
			calls++
			return AccountMonitorProbeResult{Status: "success", CheckedAt: time.Now().UTC()}, nil
		}
		return svc, repo, &calls
	}

	t.Run("no reader fails closed", func(t *testing.T) {
		svc, repo, calls := newService(nil)
		completed, err := svc.RunAll(context.Background(), 1)
		require.NoError(t, err)
		require.Equal(t, 0, completed)
		require.Zero(t, *calls)
		require.Empty(t, repo.results)
	})

	t.Run("real request in bucket skips probe", func(t *testing.T) {
		svc, repo, calls := newService(&modelDetectionUsageStub{accountUsed: true})
		completed, err := svc.RunAll(context.Background(), 1)
		require.NoError(t, err)
		require.Equal(t, 0, completed)
		require.Zero(t, *calls)
		require.Empty(t, repo.results)
	})

	t.Run("empty bucket permits one probe", func(t *testing.T) {
		svc, repo, calls := newService(&modelDetectionUsageStub{})
		completed, err := svc.RunAll(context.Background(), 1)
		require.NoError(t, err)
		require.Equal(t, 1, completed)
		require.Equal(t, 1, *calls)
		require.Len(t, repo.results, 1)
	})
}

func TestAccountMonitorRunAllProbesAPIKey401RecoveryAndClearsTempState(t *testing.T) {
	repo := &accountMonitorRepoStub{
		latest:   map[int64]AccountMonitorLatest{406: {Status: "failed", HTTPStatus: intPtr(http.StatusUnauthorized)}},
		settings: AccountMonitorSettings{IntervalSeconds: 300},
	}
	until := time.Now().UTC().Add(time.Hour)
	accountRepo := &accountMonitorAPIKeyRecoveryRepoStub{accountMonitorAccountRepoStub: accountMonitorAccountRepoStub{
		accounts: []Account{{ID: 406, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey, TempUnschedulableUntil: &until}},
	}}
	svc := NewAccountMonitorService(repo, accountRepo, nil, nil, nil)
	svc.SetActiveProbeUsageReader(&modelDetectionUsageStub{})
	runtimeBlocker := &accountMonitorRuntimeBlockerStub{}
	svc.SetAccountRuntimeBlocker(runtimeBlocker)
	calls := 0
	svc.probeConnection = func(context.Context, int64, string, string, string) (AccountMonitorProbeResult, error) {
		calls++
		return AccountMonitorProbeResult{Status: "success", CheckedAt: time.Now().UTC()}, nil
	}

	_, err := svc.RunAll(context.Background(), 0)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, 1, accountRepo.clearTempCalls)
	require.Equal(t, []int64{406}, runtimeBlocker.cleared)
}
func TestAccountMonitorWindowScoreBreakdownSumsToRoundedQualityScore(t *testing.T) {
	now := time.Now().UTC()
	rate := 0.5
	accounts := []Account{{ID: 301, Name: "probe-scored", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate}}
	repo := &accountMonitorRepoStub{
		settings: AccountMonitorSettings{IntervalSeconds: 300},
		groups:   []AccountMonitorGroup{{ID: 7, Name: "public", RateMultiplier: 1, CustomerVisible: true}},
		windowAggregates: map[int64]AccountMonitorWindowAggregate{
			301: {RequestCount: 96, SuccessCount: 96, SuccessRate: 1, LastObservedAt: &now},
		},
		aggregates: map[int64]AccountMonitorAggregate{
			301: {SampleCount: 9, SuccessCount: 8, SuccessSampleCount: 8, SuccessRate: 8.0 / 9.0, TTFTSampleCount: 9, LatencySampleCount: 9, TTFTP50MS: floatPtr(1600), LatencyP95MS: floatPtr(12000), LastCheckedAt: &now},
		},
		latest: map[int64]AccountMonitorLatest{301: {Status: "success", CheckedAt: now}},
	}
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: accounts}, nil, nil, accountMonitorConfirmedMultiplier(rate)).ListWindow(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	row := page.Accounts[0]
	if row.EvidenceSource != "hybrid" {
		t.Fatalf("evidence source = %q, want hybrid", row.EvidenceSource)
	}
	if row.RequestCount != 96 || row.SuccessRate != 104.0/105.0 {
		t.Fatalf("request disclosure/quality = request_count %d success_rate %v, want 96 and hybrid rate", row.RequestCount, row.SuccessRate)
	}
	if row.QualityScore == nil {
		t.Fatal("quality score is nil")
	}
	components := row.ScoreBreakdown.Cost + row.ScoreBreakdown.Success + row.ScoreBreakdown.TTFT + row.ScoreBreakdown.Latency
	if math.Round(components) != *row.QualityScore {
		t.Fatalf("score breakdown sum = %v, quality_score = %v", components, *row.QualityScore)
	}
}
