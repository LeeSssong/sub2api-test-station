package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAccountSchedulerProjection_UsesGroupPolicyOrderWithoutChangingQualityScore(t *testing.T) {
	scheduler, _, acquiredIDs, releasedIDs := newOpenAIAccountSchedulerProjectionTestScheduler(t, `{"profit":1,"ttft":3,"latency":3}`)
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)

	lessPerformant := projectionTestAccount(1)
	lessPerformant.Priority = 10
	lessPerformant.Extra = projectionTestUpstreamBillingExtra(now, 0.1)
	performant := projectionTestAccount(2)
	performant.Priority = 1
	performant.Extra = projectionTestUpstreamBillingExtra(now, 1.0)

	projection, err := scheduler.Project(context.Background(), OpenAIAccountSchedulerProjectionRequest{
		GroupID:              77,
		Platform:             PlatformOpenAI,
		RequestedModel:       "gpt-5.4-mini",
		RequiredTransport:    OpenAIUpstreamTransportAny,
		UseUpstreamTokenCost: true,
		SnapshotAt:           now,
		Accounts:             []*Account{lessPerformant, performant},
		LoadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 10, WaitingCount: 0},
			2: {AccountID: 2, LoadRate: 0, WaitingCount: 0},
		},
	})

	require.NoError(t, err)
	require.Equal(t, now, projection.SnapshotAt)
	require.Equal(t, "group_policy", projection.PolicyKey)
	require.Equal(t, "利润优先", projection.PolicyLabel)
	require.Equal(t, []int64{1, 2}, projectionAccountIDs(projection.Candidates))
	require.Equal(t, AccountMonitorReasonStrategy, projection.Candidates[0].PrimaryReasonCode)
	require.Equal(t, 3.0, projection.EffectiveWeights["upstream_cost"])
	require.Empty(t, *acquiredIDs)
	require.Empty(t, *releasedIDs)
}

func TestOpenAIAccountSchedulerProjection_MarksCooldownWithoutMutatingSelectionState(t *testing.T) {
	scheduler, cache, acquiredIDs, releasedIDs := newOpenAIAccountSchedulerProjectionTestScheduler(t, "")
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	cooldownUntil := now.Add(time.Hour)
	eligible := projectionTestAccount(11)
	cooldown := projectionTestAccount(12)
	cooldown.TempUnschedulableUntil = &cooldownUntil
	cache.sessionBindings["openai:projection"] = eligible.ID
	beforeBindings, err := json.Marshal(cache.sessionBindings)
	require.NoError(t, err)
	beforeMetrics := scheduler.SnapshotMetrics()
	beforeStatsSize := scheduler.stats.size()

	projection, err := scheduler.Project(context.Background(), OpenAIAccountSchedulerProjectionRequest{
		GroupID:        77,
		Platform:       PlatformOpenAI,
		RequestedModel: "gpt-5.4-mini",
		SnapshotAt:     now,
		Accounts:       []*Account{eligible, cooldown},
		LoadMap: map[int64]*AccountLoadInfo{
			11: {AccountID: 11, LoadRate: 0},
			12: {AccountID: 12, LoadRate: 0},
		},
	})

	require.NoError(t, err)
	require.Equal(t, []int64{11, 12}, projectionAccountIDs(projection.Candidates))
	require.NotNil(t, projection.Candidates[0].Rank)
	require.Nil(t, projection.Candidates[1].Rank)
	require.True(t, projection.Candidates[0].Eligible)
	require.False(t, projection.Candidates[1].Eligible)
	require.Equal(t, AccountMonitorReasonCooldown, projection.Candidates[1].PrimaryReasonCode)
	require.Empty(t, *acquiredIDs)
	require.Empty(t, *releasedIDs)
	require.Equal(t, beforeMetrics, scheduler.SnapshotMetrics())
	require.Equal(t, beforeStatsSize, scheduler.stats.size())
	afterBindings, err := json.Marshal(cache.sessionBindings)
	require.NoError(t, err)
	require.JSONEq(t, string(beforeBindings), string(afterBindings))
}

func TestOpenAIAccountSchedulerProjection_UsesStableComparatorForEqualScores(t *testing.T) {
	scheduler, _, _, _ := newOpenAIAccountSchedulerProjectionTestScheduler(t, "")
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	first := projectionTestAccount(20)
	second := projectionTestAccount(19)

	projection, err := scheduler.Project(context.Background(), OpenAIAccountSchedulerProjectionRequest{
		GroupID:        77,
		Platform:       PlatformOpenAI,
		RequestedModel: "gpt-5.4-mini",
		SnapshotAt:     now,
		Accounts:       []*Account{first, second},
		LoadMap: map[int64]*AccountLoadInfo{
			20: {AccountID: 20, LoadRate: 0, WaitingCount: 0},
			19: {AccountID: 19, LoadRate: 0, WaitingCount: 0},
		},
	})

	require.NoError(t, err)
	require.Equal(t, []int64{19, 20}, projectionAccountIDs(projection.Candidates))
	require.Equal(t, AccountMonitorReasonTieBreak, projection.Candidates[1].PrimaryReasonCode)
}

func TestOpenAIAccountSchedulerProjection_UsesOperatorStrategyLabels(t *testing.T) {
	tests := []struct {
		name   string
		preset OpenAISchedulerPreset
		label  string
	}{
		{name: "special offer", preset: OpenAISchedulerPresetSpecialOffer, label: "体验优先"},
		{name: "balanced", preset: OpenAISchedulerPresetBalanced, label: "体验均衡"},
		{name: "pro", preset: OpenAISchedulerPresetPro, label: "利润优先"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.label, schedulerProjectionPolicyLabel(OpenAISchedulerGroupPolicy{Preset: tt.preset}, true))
		})
	}
}

func TestOpenAIAccountSchedulerProjection_UsesSnapshotForReadOnlyRuntimeEligibility(t *testing.T) {
	scheduler, _, _, _ := newOpenAIAccountSchedulerProjectionTestScheduler(t, "")
	service := scheduler.service
	snapshot := time.Now().UTC().Add(-time.Hour)
	blockedUntil := snapshot.Add(30 * time.Minute)
	proxyID := int64(901)
	account := projectionTestAccount(90)
	account.ProxyID = &proxyID

	service.openaiAccountRuntimeBlockUntil.Store(account.ID, blockedUntil)
	service.openaiModelTransient = newOpenAIAccountModelTransientState(16)
	service.openaiModelTransient.entries[openAIAccountModelKey{AccountID: account.ID, Model: "gpt-5.4-mini"}] = openAIAccountModelTransientEntry{
		failureStreak: 2,
		lastFailure:   snapshot,
		blockUntil:    blockedUntil,
		lastTouched:   snapshot,
	}
	service.openaiProxyStreamCircuit = newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		failureThreshold: 2,
		failureWindow:    time.Hour,
		quarantineTTL:    time.Hour,
		maxEntries:       16,
	})
	service.openaiProxyStreamCircuit.entries[proxyID] = openAIProxyStreamCircuitEntry{
		blockedUntil: blockedUntil,
		lastTouched:  snapshot,
	}

	beforeTransient := service.openaiModelTransient.entries[openAIAccountModelKey{AccountID: account.ID, Model: "gpt-5.4-mini"}]
	beforeProxy := service.openaiProxyStreamCircuit.entries[proxyID]

	projection, err := scheduler.Project(context.Background(), OpenAIAccountSchedulerProjectionRequest{
		GroupID:        77,
		Platform:       PlatformOpenAI,
		RequestedModel: "gpt-5.4-mini",
		SnapshotAt:     snapshot,
		Accounts:       []*Account{account},
		LoadMap:        map[int64]*AccountLoadInfo{account.ID: {AccountID: account.ID}},
	})

	require.NoError(t, err)
	require.False(t, projection.Candidates[0].Eligible)
	require.Equal(t, AccountMonitorReasonCooldown, projection.Candidates[0].PrimaryReasonCode)
	_, runtimeBlockStillPresent := service.openaiAccountRuntimeBlockUntil.Load(account.ID)
	require.True(t, runtimeBlockStillPresent)
	require.Equal(t, beforeTransient, service.openaiModelTransient.entries[openAIAccountModelKey{AccountID: account.ID, Model: "gpt-5.4-mini"}])
	require.Equal(t, beforeProxy, service.openaiProxyStreamCircuit.entries[proxyID])
}

func TestOpenAIAccountSchedulerProjection_UsesSnapshotForQuotaEligibility(t *testing.T) {
	scheduler, _, _, _ := newOpenAIAccountSchedulerProjectionTestScheduler(t, "")
	snapshot := time.Now().UTC().Add(-time.Hour)
	account := projectionTestAccount(901)
	account.Extra = map[string]any{
		"codex_5h_used_percent":  95,
		"codex_5h_reset_at":      snapshot.Add(30 * time.Minute).Format(time.RFC3339Nano),
		"codex_usage_updated_at": snapshot.Format(time.RFC3339Nano),
	}
	ctx := withOpenAIQuotaAutoPauseSettings(context.Background(), OpsOpenAIAccountQuotaAutoPauseSettings{DefaultThreshold5h: 0.9})

	projection, err := scheduler.Project(ctx, OpenAIAccountSchedulerProjectionRequest{
		GroupID:        77,
		Platform:       PlatformOpenAI,
		RequestedModel: "gpt-5.4-mini",
		SnapshotAt:     snapshot,
		Accounts:       []*Account{account},
		LoadMap:        map[int64]*AccountLoadInfo{account.ID: {AccountID: account.ID}},
	})

	require.NoError(t, err)
	require.False(t, projection.Candidates[0].Eligible)
	require.Equal(t, AccountMonitorReasonNotEligible, projection.Candidates[0].PrimaryReasonCode)
}

func TestOpenAIAccountSchedulerProjection_DoesNotObserveProfitControl(t *testing.T) {
	scheduler, _, _, _ := newOpenAIAccountSchedulerProjectionTestScheduler(t, "")
	groupID := int64(907)
	stats := openAIProfitControlObserverInstance.stats(groupID, PlatformOpenAI)
	before := stats.vetoInvalidRate.Load()
	threshold := 0.5
	ctx := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{
		groupID:   groupID,
		platform:  PlatformOpenAI,
		threshold: threshold,
	})
	rate := 1.0
	account := projectionTestAccount(91)
	account.RateMultiplier = &rate

	projection, err := scheduler.Project(ctx, OpenAIAccountSchedulerProjectionRequest{
		GroupID:        groupID,
		Platform:       PlatformOpenAI,
		RequestedModel: "gpt-5.4-mini",
		SnapshotAt:     time.Now().UTC(),
		Accounts:       []*Account{account},
		LoadMap:        map[int64]*AccountLoadInfo{account.ID: {AccountID: account.ID}},
	})

	require.NoError(t, err)
	require.False(t, projection.Candidates[0].Eligible)
	require.Equal(t, before, stats.vetoInvalidRate.Load())
}

func TestOpenAIAccountSchedulerProjection_ReturnsPolicyReadFailure(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	repo := &projectionFailingSettingRepo{
		openAIAdvancedSchedulerSettingRepoStub: &openAIAdvancedSchedulerSettingRepoStub{},
		err:                                    errors.New("settings unavailable"),
	}
	cfg := &config.Config{}
	service := &OpenAIGatewayService{
		cfg:              cfg,
		rateLimitService: &RateLimitService{settingService: NewSettingService(repo, cfg)},
	}
	scheduler := &defaultOpenAIAccountScheduler{service: service, stats: newOpenAIAccountRuntimeStats()}

	_, err := scheduler.Project(context.Background(), OpenAIAccountSchedulerProjectionRequest{
		GroupID:        77,
		Platform:       PlatformOpenAI,
		RequestedModel: "gpt-5.4-mini",
		SnapshotAt:     time.Now().UTC(),
		Accounts:       []*Account{projectionTestAccount(92)},
	})

	require.ErrorIs(t, err, repo.err)
	require.Nil(t, openAIAdvancedSchedulerSettingCache.Load())
}

func TestOpenAIAccountSchedulerProjection_DoesNotLabelMatchingQualityOrderAsStrategy(t *testing.T) {
	scheduler, _, _, _ := newOpenAIAccountSchedulerProjectionTestScheduler(t, `{"profit":1,"ttft":3,"latency":3}`)
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	first := projectionTestAccount(93)
	second := projectionTestAccount(94)

	projection, err := scheduler.Project(context.Background(), OpenAIAccountSchedulerProjectionRequest{
		GroupID:        77,
		Platform:       PlatformOpenAI,
		RequestedModel: "gpt-5.4-mini",
		SnapshotAt:     now,
		Accounts:       []*Account{first, second},
		LoadMap: map[int64]*AccountLoadInfo{
			first.ID:  {AccountID: first.ID, LoadRate: 0},
			second.ID: {AccountID: second.ID, LoadRate: 0},
		},
	})

	require.NoError(t, err)
	require.Equal(t, []int64{93, 94}, projectionAccountIDs(projection.Candidates))
	require.NotEqual(t, AccountMonitorReasonStrategy, projection.Candidates[0].PrimaryReasonCode)
}

type projectionFailingSettingRepo struct {
	*openAIAdvancedSchedulerSettingRepoStub
	err error
}

func (r *projectionFailingSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, r.err
}

func newOpenAIAccountSchedulerProjectionTestScheduler(t *testing.T, priorityJSON string) (*defaultOpenAIAccountScheduler, *schedulerTestGatewayCache, *[]int64, *[]int64) {
	t.Helper()
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)

	values := map[string]string{openAIAdvancedSchedulerSettingKey: "true"}
	if priorityJSON != "" {
		values[SettingKeyOpenAIAdvancedSchedulerGroupOverrides] = `{"77":{"priority":` + priorityJSON + `,"operations":{"balance":"standard","peak_protection":"strict","session_continuity":"standard"}}}`
	}
	repo := &openAIAdvancedSchedulerSettingRepoStub{values: values}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 1
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{}}
	acquiredIDs := []int64{}
	releasedIDs := []int64{}
	service := &OpenAIGatewayService{
		cache:              cache,
		cfg:                cfg,
		rateLimitService:   &RateLimitService{settingService: NewSettingService(repo, cfg)},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquiredIDs, releasedIDs: &releasedIDs}),
	}
	return &defaultOpenAIAccountScheduler{service: service, stats: newOpenAIAccountRuntimeStats()}, cache, &acquiredIDs, &releasedIDs
}

func projectionTestAccount(id int64) *Account {
	return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
}

func projectionTestUpstreamBillingExtra(now time.Time, rate float64) map[string]any {
	return map[string]any{
		UpstreamBillingProbeExtraKey: map[string]any{
			"status":        UpstreamBillingProbeStatusOK,
			"received_at":   now.Add(-10 * time.Minute).Format(time.RFC3339Nano),
			"fresh_until":   now.Add(time.Hour).Format(time.RFC3339Nano),
			"next_probe_at": now.Add(20 * time.Minute).Format(time.RFC3339Nano),
			"data": map[string]any{
				"billing_scope":             "token",
				"peak_rate_enabled":         false,
				"resolved_rate_multiplier":  rate,
				"effective_rate_multiplier": rate,
			},
		},
	}
}

func projectionAccountIDs(candidates []OpenAIAccountSchedulerProjectionCandidate) []int64 {
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.AccountID)
	}
	return ids
}
