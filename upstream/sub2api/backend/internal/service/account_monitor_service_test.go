package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	mu              sync.Mutex
	results         []AccountMonitorProbeResult
	settings        AccountMonitorSettings
	groups          []AccountMonitorGroup
	weights         map[int64]AccountMonitorScoreWeights
	aggregates      map[int64]AccountMonitorAggregate
	groupAggregates map[int64]map[int64]AccountMonitorAggregate
	latest          map[int64]AccountMonitorLatest
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

func (s *accountMonitorRepoStub) ListAggregates(context.Context, []int64, time.Time) (map[int64]AccountMonitorAggregate, error) {
	return s.aggregates, nil
}

func (s *accountMonitorRepoStub) ListGroupAggregates(_ context.Context, groupID int64, _ []int64, _ time.Time) (map[int64]AccountMonitorAggregate, error) {
	return s.groupAggregates[groupID], nil
}

func (s *accountMonitorRepoStub) ListLatest(context.Context, []int64) (map[int64]AccountMonitorLatest, error) {
	return s.latest, nil
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
	mu     sync.Mutex
	calls  []accountMonitorMultiplierCall
	err    error
	result AccountMonitorMultiplier
}

type accountMonitorMultiplierCall struct {
	accountID int64
	force     bool
}

type accountMonitorRunResult struct {
	completed int
	err       error
}

func (s *accountMonitorMultiplierStub) Resolve(*Account, time.Time) AccountMonitorMultiplier {
	return s.result
}

func (s *accountMonitorMultiplierStub) Refresh(_ context.Context, account *Account, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, accountMonitorMultiplierCall{accountID: account.ID, force: force})
	return s.err
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
	page, err := NewAccountMonitorService(repo, accountRepo, nil, nil, nil).List(context.Background())
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
	page, err := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row := page.Groups[0].Accounts[0]
	if row.Evidence.Source != "global_fallback" || row.Evidence.SampleCount != 4 || !row.Eligible {
		t.Fatalf("fallback evidence = %#v", row)
	}

	stale := now.Add(-20 * time.Minute)
	repo.latest[52] = AccountMonitorLatest{Status: "success", CheckedAt: stale}
	page, err = NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row = page.Groups[0].Accounts[0]
	if row.Evidence.Source != "stale" || row.Eligible || row.QualityScore != nil {
		t.Fatalf("stale evidence = %#v", row)
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
