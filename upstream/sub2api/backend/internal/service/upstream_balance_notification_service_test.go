package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	upstreamnotify "github.com/Wei-Shaw/sub2api/internal/notify"
	"github.com/stretchr/testify/require"
)

func TestUpstreamBalanceNotificationServiceEvaluateReReadsCurrentProjectionBeforeSend(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	observedAt := now.Add(-time.Minute)
	first := upstreamBalanceEvaluationFixture("old account", UpstreamBalanceStateLow, 4.5, observedAt)
	latest := upstreamBalanceEvaluationFixture("current account", UpstreamBalanceStateLow, 4.5, observedAt)
	reader := &upstreamBalanceEvaluationReaderStub{results: [][]UpstreamBalanceEvaluation{{first}, {latest}}}
	repo := newUpstreamBalanceEventRepoStub(first, now)
	sender := &upstreamBalanceSenderStub{}
	svc := NewUpstreamBalanceNotificationService(repo, reader, sender, upstreamBalanceLoginLookupStub{}, []string{"ou_recipient"})
	svc.now = func() time.Time { return now }

	err := svc.Evaluate(context.Background())

	require.NoError(t, err)
	require.Len(t, repo.claims, 1)
	require.Equal(t, 30*time.Minute, repo.claims[0].RepeatInterval)
	require.Len(t, sender.inputs, 1)
	require.Equal(t, "current account", sender.inputs[0].Accounts[0].Name)
	require.Equal(t, "registry-user.invalid", sender.inputs[0].LoginAccount)
	require.Equal(t, "registry-secret.invalid", sender.inputs[0].LoginPassword)
	require.Len(t, repo.confirmed, 1)
	require.Equal(t, now, repo.confirmed[0].At)
}

func TestUpstreamBalanceLeaseMatchesMicrosecondTruncatedDatabaseObservation(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	jsonObservedAt := time.Date(2026, 8, 31, 11, 59, 0, 123456789, time.UTC)
	databaseObservedAt := jsonObservedAt.Truncate(time.Microsecond)
	current := &UpstreamBalanceEvent{
		ID: 41, RuleID: 17, Status: OpsAlertStatusFiring, ScopeType: UpstreamBalanceScopeTypeBaseURL,
		ScopeKey: "https://upstream.invalid", NotificationState: UpstreamBalanceNotificationStateLow,
		LastObservedAt: databaseObservedAt, ValueUSD: 4.5, DeliveryGeneration: 1,
		DeliveryLeaseToken: "lease-token", DeliveryLeaseUntil: upstreamBalanceTimePointer(now.Add(time.Minute)),
	}
	lease := UpstreamBalanceDeliveryLease{
		EventID: 41, RuleID: 17, ScopeKey: "https://upstream.invalid", NotificationState: UpstreamBalanceNotificationStateLow,
		ObservedAt: jsonObservedAt, ValueUSD: 4.5, Generation: 1, Token: "lease-token", LeaseUntil: now.Add(time.Minute),
	}

	if !upstreamBalanceLeaseMatches(current, lease, now) {
		t.Fatal("microsecond-truncated database timestamp must match the JSON observation")
	}
}

func TestUpstreamBalanceEvaluationMatchesLeaseMicrosecondTruncation(t *testing.T) {
	jsonObservedAt := time.Date(2026, 8, 31, 11, 59, 0, 123456789, time.UTC)
	value := 4.5
	evaluation := UpstreamBalanceEvaluation{
		ObservedAt: jsonObservedAt, ValueUSD: &value, State: UpstreamBalanceNotificationStateLow,
	}
	lease := UpstreamBalanceDeliveryLease{
		ObservedAt: jsonObservedAt.Truncate(time.Microsecond), ValueUSD: value,
		NotificationState: UpstreamBalanceNotificationStateLow,
	}

	if !upstreamBalanceEvaluationMatchesLease(evaluation, lease) {
		t.Fatal("microsecond-truncated lease timestamp must match the JSON evaluation")
	}
}

func TestUpstreamBalanceNotificationServiceUsesFiveMinuteZeroCadence(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	evaluation := upstreamBalanceEvaluationFixture("zero account", UpstreamBalanceStateZero, 0, now.Add(-time.Minute))
	reader := &upstreamBalanceEvaluationReaderStub{results: [][]UpstreamBalanceEvaluation{{evaluation}, {evaluation}}}
	repo := newUpstreamBalanceEventRepoStub(evaluation, now)
	svc := NewUpstreamBalanceNotificationService(repo, reader, &upstreamBalanceSenderStub{}, upstreamBalanceLoginLookupStub{}, []string{"ou_recipient"})
	svc.now = func() time.Time { return now }

	require.NoError(t, svc.Evaluate(context.Background()))
	require.Len(t, repo.claims, 1)
	require.Equal(t, 5*time.Minute, repo.claims[0].RepeatInterval)
}

func TestUpstreamBalanceNotificationServiceHealthyResolvesWithoutSend(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	evaluation := upstreamBalanceEvaluationFixture("healthy account", UpstreamBalanceStateHealthy, 5, now.Add(-time.Minute))
	reader := &upstreamBalanceEvaluationReaderStub{results: [][]UpstreamBalanceEvaluation{{evaluation}}}
	repo := newUpstreamBalanceEventRepoStub(evaluation, now)
	sender := &upstreamBalanceSenderStub{}
	svc := NewUpstreamBalanceNotificationService(repo, reader, sender, upstreamBalanceLoginLookupStub{}, nil)
	svc.now = func() time.Time { return now }

	require.NoError(t, svc.Evaluate(context.Background()))
	require.Len(t, repo.resolved, 1)
	require.Empty(t, repo.claims)
	require.Empty(t, sender.inputs)
}

func TestUpstreamBalanceNotificationServiceReadFailureDoesNotMutateEvent(t *testing.T) {
	repo := &upstreamBalanceEventRepoStub{ruleID: 17}
	reader := &upstreamBalanceEvaluationReaderStub{err: errors.New("projection unavailable")}
	svc := NewUpstreamBalanceNotificationService(repo, reader, &upstreamBalanceSenderStub{}, upstreamBalanceLoginLookupStub{}, nil)

	err := svc.Evaluate(context.Background())

	require.EqualError(t, err, "read upstream balance projection")
	require.Empty(t, repo.claims)
	require.Empty(t, repo.resolved)
}

func TestUpstreamBalanceNotificationServiceRejectsStaleGenerationBeforeSend(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	evaluation := upstreamBalanceEvaluationFixture("stale account", UpstreamBalanceStateLow, 3, now.Add(-time.Minute))
	reader := &upstreamBalanceEvaluationReaderStub{results: [][]UpstreamBalanceEvaluation{{evaluation}, {evaluation}}}
	repo := newUpstreamBalanceEventRepoStub(evaluation, now)
	repo.current.DeliveryGeneration++
	sender := &upstreamBalanceSenderStub{}
	svc := NewUpstreamBalanceNotificationService(repo, reader, sender, upstreamBalanceLoginLookupStub{}, nil)
	svc.now = func() time.Time { return now }

	require.NoError(t, svc.Evaluate(context.Background()))
	require.Empty(t, sender.inputs)
	require.Empty(t, repo.confirmed)
}

func TestUpstreamBalanceNotificationServiceRecordsFiveMinuteFailureBackoff(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	evaluation := upstreamBalanceEvaluationFixture("retry account", UpstreamBalanceStateLow, 2, now.Add(-time.Minute))
	reader := &upstreamBalanceEvaluationReaderStub{results: [][]UpstreamBalanceEvaluation{{evaluation}, {evaluation}}}
	repo := newUpstreamBalanceEventRepoStub(evaluation, now)
	repo.current.DeliveryAttemptCount = 2
	sender := &upstreamBalanceSenderStub{err: errors.New("transport unavailable")}
	svc := NewUpstreamBalanceNotificationService(repo, reader, sender, upstreamBalanceLoginLookupStub{}, nil)
	svc.now = func() time.Time { return now }

	err := svc.Evaluate(context.Background())

	require.EqualError(t, err, "deliver upstream balance notification")
	require.Len(t, repo.failed, 1)
	require.Equal(t, now.Add(5*time.Minute), repo.failed[0].NextAttemptAt)
	require.Equal(t, "feishu_send_failed", repo.failed[0].ErrorCode)
	require.Empty(t, repo.confirmed)
}

func TestUpstreamBalanceNotificationServiceConcurrentEvaluateGetsOneClaim(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	evaluation := upstreamBalanceEvaluationFixture("shared account", UpstreamBalanceStateLow, 1, now.Add(-time.Minute))
	reader := &upstreamBalanceEvaluationReaderStub{fallback: []UpstreamBalanceEvaluation{evaluation}}
	repo := newUpstreamBalanceEventRepoStub(evaluation, now)
	sender := &upstreamBalanceSenderStub{}
	svc := NewUpstreamBalanceNotificationService(repo, reader, sender, upstreamBalanceLoginLookupStub{}, nil)
	svc.now = func() time.Time { return now }

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.Evaluate(context.Background())
		}()
	}
	wg.Wait()

	require.Len(t, sender.snapshot(), 1)
}

func TestUpstreamBalanceNotificationServiceRunDueIgnoresNewScopes(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	active := upstreamBalanceEvaluationFixture("active event account", UpstreamBalanceStateLow, 4, now.Add(-time.Minute))
	newScope := upstreamBalanceEvaluationFixture("new scope account", UpstreamBalanceStateLow, 3, now.Add(-time.Minute))
	newScope.NormalizedBaseURL = "https://new-scope.invalid"
	newScope.Accounts[0].BaseURL = newScope.NormalizedBaseURL
	reader := &upstreamBalanceEvaluationReaderStub{results: [][]UpstreamBalanceEvaluation{{active, newScope}, {active, newScope}}}
	repo := newUpstreamBalanceEventRepoStub(active, now)
	sender := &upstreamBalanceSenderStub{}
	svc := NewUpstreamBalanceNotificationService(repo, reader, sender, upstreamBalanceLoginLookupStub{}, nil)
	svc.now = func() time.Time { return now }

	require.NoError(t, svc.RunDue(context.Background()))
	require.Len(t, repo.claims, 1)
	require.Equal(t, active.NormalizedBaseURL, repo.claims[0].ScopeKey)
	require.Len(t, sender.inputs, 1)
}

func TestUpstreamBalanceNotificationServiceRunDueRefreshesZeroScopeBeforeProjection(t *testing.T) {
	now := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	zero := upstreamBalanceEvaluationFixture("zero account", UpstreamBalanceStateZero, 0, now.Add(-time.Minute))
	healthy := upstreamBalanceEvaluationFixture("recharged account", UpstreamBalanceStateHealthy, 8, now)
	reader := &upstreamBalanceRefreshingReaderStub{
		upstreamBalanceEvaluationReaderStub: upstreamBalanceEvaluationReaderStub{results: [][]UpstreamBalanceEvaluation{{healthy}}},
	}
	repo := newUpstreamBalanceEventRepoStub(zero, now)
	sender := &upstreamBalanceSenderStub{}
	svc := NewUpstreamBalanceNotificationService(repo, reader, sender, upstreamBalanceLoginLookupStub{}, nil)
	svc.now = func() time.Time { return now }

	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, []string{"https://upstream.invalid"}, reader.refreshedScopes)
	require.Equal(t, []string{"https://upstream.invalid"}, repo.resolved)
	require.Empty(t, sender.inputs)
}

func TestUpstreamBalanceNotificationServiceRunDueRefreshesLowScopeBeforeProjection(t *testing.T) {
	now := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	low := upstreamBalanceEvaluationFixture("low account", UpstreamBalanceStateLow, 2, now.Add(-time.Minute))
	reader := &upstreamBalanceRefreshingReaderStub{
		upstreamBalanceEvaluationReaderStub: upstreamBalanceEvaluationReaderStub{results: [][]UpstreamBalanceEvaluation{{low}}},
	}
	repo := newUpstreamBalanceEventRepoStub(low, now)
	svc := NewUpstreamBalanceNotificationService(repo, reader, &upstreamBalanceSenderStub{}, upstreamBalanceLoginLookupStub{}, nil)
	svc.now = func() time.Time { return now }

	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, []string{"https://upstream.invalid"}, reader.refreshedScopes)
}

func TestUpstreamBalanceFailureDelay(t *testing.T) {
	tests := []struct {
		attempt int
		state   string
		want    time.Duration
	}{
		{attempt: 0, state: UpstreamBalanceNotificationStateLow, want: time.Minute},
		{attempt: 1, state: UpstreamBalanceNotificationStateLow, want: 2 * time.Minute},
		{attempt: 2, state: UpstreamBalanceNotificationStateLow, want: 5 * time.Minute},
		{attempt: 3, state: UpstreamBalanceNotificationStateLow, want: 10 * time.Minute},
		{attempt: 4, state: UpstreamBalanceNotificationStateLow, want: 30 * time.Minute},
		{attempt: 4, state: UpstreamBalanceNotificationStateZero, want: 5 * time.Minute},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, upstreamBalanceFailureDelay(tt.attempt, tt.state))
	}
}

func TestBuildUpstreamBalanceEvaluationsPreservesBaseURLPathAndGroupRanks(t *testing.T) {
	observedAt := time.Date(2026, 8, 31, 11, 59, 0, 0, time.UTC)
	value := 4.25
	rank := 2
	fingerprint := accountMonitorBalanceCredentialFingerprint("test-key")
	raw := []Account{{
		ID: 9, Name: "native account", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"base_url": "HTTPS://Upstream.Invalid/native/v1/", "api_key": "test-key"},
	}}
	page := AccountMonitorPage{AccountMonitorProjection: AccountMonitorProjection{
		ObservedAt: observedAt.Add(time.Minute),
		Accounts: []AccountMonitorAccount{{
			AccountID: 9, Balance: &AccountMonitorBalance{
				Version: 1, Status: AccountMonitorBalanceStatusOK, Source: AccountMonitorBalanceSourceSub2API,
				ValueUSD: &value, ObservedAt: &observedAt, CredentialFingerprint: fingerprint,
			},
		}},
		Groups: []AccountMonitorGroup{{
			ID: 7, Name: "Primary", Accounts: []AccountMonitorGroupAccount{{
				AccountMonitorAccount: AccountMonitorAccount{AccountID: 9, SchedulerRank: &rank},
			}},
		}},
	}}

	evaluations, err := buildUpstreamBalanceEvaluations(raw, page)

	require.NoError(t, err)
	require.Len(t, evaluations, 1)
	require.Equal(t, "https://upstream.invalid/native/v1", evaluations[0].NormalizedBaseURL)
	require.Equal(t, "Primary", evaluations[0].Accounts[0].Ranks[0].GroupName)
	require.Equal(t, 2, *evaluations[0].Accounts[0].Ranks[0].Rank)
}

func TestReadUpstreamBalanceEvaluationsUsesWindowSchedulerRanks(t *testing.T) {
	now := time.Now().UTC()
	value := 8.25
	observedAt := now.Add(-time.Minute)
	rank := 3
	account := Account{
		ID: 9, Name: "ranked account", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, GroupIDs: []int64{7},
		Credentials: map[string]any{"base_url": "https://upstream.invalid/v1", "api_key": "test-key"},
		Extra: map[string]any{AccountMonitorBalanceExtraKey: AccountMonitorBalance{
			Version: AccountMonitorBalanceVersion, Status: AccountMonitorBalanceStatusOK,
			Source:   AccountMonitorBalanceSourceSub2API,
			ValueUSD: &value, ObservedAt: &observedAt, CredentialFingerprint: accountMonitorBalanceCredentialFingerprint("test-key"),
		}},
	}
	repo := &accountMonitorRepoStub{
		settings:         AccountMonitorSettings{IntervalSeconds: 300},
		globalWeights:    DefaultAccountMonitorScoreWeights,
		windowAggregates: map[int64]AccountMonitorWindowAggregate{9: {RequestCount: 1, SuccessCount: 1, SuccessRate: 1, LastObservedAt: &now}},
		aggregates:       map[int64]AccountMonitorAggregate{9: {SampleCount: 1, SuccessCount: 1, SuccessRate: 1, LastCheckedAt: &now}},
		latest:           map[int64]AccountMonitorLatest{9: {Status: "success", CheckedAt: now}},
		timelines:        map[int64][]AccountMonitorTimelinePoint{},
		groups:           []AccountMonitorGroup{{ID: 7, Name: "Primary", Platform: PlatformOpenAI, RateMultiplier: 1}},
	}
	scheduler := &accountMonitorSchedulerProjectionStub{byGroup: map[int64]*OpenAIAccountSchedulerProjection{
		7: {SnapshotAt: now, CandidateCount: 3, Candidates: []OpenAIAccountSchedulerProjectionCandidate{{AccountID: 9, Rank: &rank, Eligible: true}}},
	}}
	monitor := NewAccountMonitorService(repo, &accountMonitorAccountRepoStub{accounts: []Account{account}}, nil, nil, NewAccountMultiplierService(nil, nil, nil))
	monitor.SetOpenAIAccountSchedulerProjectionProvider(scheduler)

	evaluations, err := monitor.ReadUpstreamBalanceEvaluations(context.Background())

	require.NoError(t, err)
	require.Len(t, evaluations, 1)
	require.Len(t, evaluations[0].Accounts, 1)
	require.Len(t, evaluations[0].Accounts[0].Ranks, 1)
	require.Equal(t, "Primary", evaluations[0].Accounts[0].Ranks[0].GroupName)
	require.Equal(t, rank, *evaluations[0].Accounts[0].Ranks[0].Rank)

	card := upstreamBalanceCardInput(evaluations[0], "login", "password", nil)
	require.Len(t, card.Accounts[0].Ranks, 1)
	require.Equal(t, rank, *card.Accounts[0].Ranks[0].Rank)
	require.NotNil(t, card.Accounts[0].BalanceUSD)
	require.Equal(t, value, *card.Accounts[0].BalanceUSD)
}

func TestProvideUpstreamBalanceNotificationServiceIsDisabledByDefault(t *testing.T) {
	t.Setenv("SUB2API_UPSTREAM_BALANCE_NOTIFICATION_ENABLED", "")

	svc := ProvideUpstreamBalanceNotificationService(nil, nil, nil)

	require.NotNil(t, svc)
	require.False(t, svc.started)
}

func TestProvideUpstreamBalanceNotificationServiceSecretFailureDoesNotBlockCoreStartup(t *testing.T) {
	t.Setenv("SUB2API_UPSTREAM_BALANCE_NOTIFICATION_ENABLED", "1")
	t.Setenv("SUB2API_UPSTREAM_BALANCE_FEISHU_APP_ID_FILE", "")
	t.Setenv("SUB2API_UPSTREAM_BALANCE_FEISHU_APP_SECRET_FILE", "")
	t.Setenv("SUB2API_UPSTREAM_BALANCE_FEISHU_CHAT_ID_FILE", "")
	t.Setenv("SUB2API_UPSTREAM_BALANCE_FEISHU_RECIPIENTS_FILE", "")
	t.Setenv("SUB2API_UPSTREAM_BALANCE_LOGIN_REGISTRY_FILE", "")

	svc := ProvideUpstreamBalanceNotificationService(nil, nil, nil)

	require.NotNil(t, svc)
	require.False(t, svc.started)
}

type upstreamBalanceEvaluationReaderStub struct {
	mu       sync.Mutex
	results  [][]UpstreamBalanceEvaluation
	fallback []UpstreamBalanceEvaluation
	err      error
}

type upstreamBalanceRefreshingReaderStub struct {
	upstreamBalanceEvaluationReaderStub
	refreshedScopes []string
}

func (s *upstreamBalanceRefreshingReaderStub) RefreshUpstreamBalanceScopes(_ context.Context, scopes map[string]struct{}) error {
	for scope := range scopes {
		s.refreshedScopes = append(s.refreshedScopes, scope)
	}
	return nil
}

func (s *upstreamBalanceEvaluationReaderStub) ReadUpstreamBalanceEvaluations(context.Context) ([]UpstreamBalanceEvaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	if len(s.results) == 0 {
		return append([]UpstreamBalanceEvaluation(nil), s.fallback...), nil
	}
	result := s.results[0]
	s.results = s.results[1:]
	return append([]UpstreamBalanceEvaluation(nil), result...), nil
}

type upstreamBalanceSenderStub struct {
	mu     sync.Mutex
	inputs []upstreamnotify.UpstreamBalanceCardInput
	err    error
}

func (s *upstreamBalanceSenderStub) Send(_ context.Context, input upstreamnotify.UpstreamBalanceCardInput) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inputs = append(s.inputs, input)
	if s.err != nil {
		return "", s.err
	}
	return "message-id", nil
}

func (s *upstreamBalanceSenderStub) snapshot() []upstreamnotify.UpstreamBalanceCardInput {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]upstreamnotify.UpstreamBalanceCardInput(nil), s.inputs...)
}

type upstreamBalanceLoginLookupStub struct{}

func (upstreamBalanceLoginLookupStub) Lookup(string) (string, string, bool) {
	return "registry-user.invalid", "registry-secret.invalid", true
}

type upstreamBalanceEventRepoStub struct {
	mu        sync.Mutex
	ruleID    int64
	claimed   bool
	lease     UpstreamBalanceDeliveryLease
	current   *UpstreamBalanceEvent
	claims    []UpstreamBalanceClaimInput
	confirmed []UpstreamBalanceDeliveryResult
	failed    []UpstreamBalanceDeliveryFailure
	resolved  []string
}

func newUpstreamBalanceEventRepoStub(evaluation UpstreamBalanceEvaluation, now time.Time) *upstreamBalanceEventRepoStub {
	lease := UpstreamBalanceDeliveryLease{
		EventID: 41, RuleID: 17, ScopeKey: evaluation.NormalizedBaseURL,
		NotificationState: evaluation.State, ObservedAt: evaluation.ObservedAt,
		ValueUSD: *evaluation.ValueUSD, Generation: 1, Token: "lease-token", LeaseUntil: now.Add(time.Minute),
	}
	return &upstreamBalanceEventRepoStub{
		ruleID: 17, lease: lease,
		current: &UpstreamBalanceEvent{
			ID: 41, RuleID: 17, Status: OpsAlertStatusFiring, ScopeType: UpstreamBalanceScopeTypeBaseURL,
			ScopeKey: evaluation.NormalizedBaseURL, NotificationState: evaluation.State,
			LastObservedAt: evaluation.ObservedAt, ValueUSD: *evaluation.ValueUSD,
			DeliveryGeneration: 1, DeliveryLeaseToken: "lease-token", DeliveryLeaseUntil: upstreamBalanceTimePointer(now.Add(time.Minute)),
		},
	}
}

func (s *upstreamBalanceEventRepoStub) GetRuleID(context.Context) (int64, error) {
	return s.ruleID, nil
}

func (s *upstreamBalanceEventRepoStub) Claim(_ context.Context, input UpstreamBalanceClaimInput) (UpstreamBalanceDeliveryLease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claims = append(s.claims, input)
	if s.claimed {
		return UpstreamBalanceDeliveryLease{}, false, nil
	}
	s.claimed = true
	return s.lease, true, nil
}

func (s *upstreamBalanceEventRepoStub) GetCurrent(context.Context, int64) (*UpstreamBalanceEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return nil, nil
	}
	copy := *s.current
	return &copy, nil
}

func (s *upstreamBalanceEventRepoStub) ConfirmDelivery(_ context.Context, result UpstreamBalanceDeliveryResult) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.confirmed = append(s.confirmed, result)
	return true, nil
}

func (s *upstreamBalanceEventRepoStub) RecordFailure(_ context.Context, failure UpstreamBalanceDeliveryFailure) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = append(s.failed, failure)
	return true, nil
}

func (s *upstreamBalanceEventRepoStub) Resolve(_ context.Context, _ int64, scopeKey string, _ time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolved = append(s.resolved, scopeKey)
	return true, nil
}

func (s *upstreamBalanceEventRepoStub) ListActive(context.Context, int) ([]UpstreamBalanceEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return nil, nil
	}
	return []UpstreamBalanceEvent{*s.current}, nil
}

func (s *upstreamBalanceEventRepoStub) WithScopeLock(ctx context.Context, _ int64, _ string, fn func(context.Context) error) (bool, error) {
	return true, fn(ctx)
}

func upstreamBalanceEvaluationFixture(name, state string, value float64, observedAt time.Time) UpstreamBalanceEvaluation {
	return UpstreamBalanceEvaluation{
		NormalizedBaseURL: "https://upstream.invalid", ValueUSD: &value, ObservedAt: observedAt, State: state,
		Accounts: []UpstreamBalanceAccount{{
			AccountID: 9, Name: name, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, BaseURL: "https://upstream.invalid",
			Ranks: []UpstreamBalanceAccountRank{{GroupName: "Primary", Rank: upstreamBalanceIntPointer(1)}},
		}},
	}
}

func upstreamBalanceIntPointer(value int) *int              { return &value }
func upstreamBalanceTimePointer(value time.Time) *time.Time { return &value }
