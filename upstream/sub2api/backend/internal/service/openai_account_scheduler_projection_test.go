package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAccountSchedulerProjection_UsesGroupPolicyOrderWithoutChangingQualityScore(t *testing.T) {
	scheduler, _, acquiredIDs, releasedIDs := newOpenAIAccountSchedulerProjectionTestScheduler(t, `{"profit":1,"ttft":3,"latency":3}`)
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)

	lessPerformant := projectionTestAccount(1)
	lessPerformant.Extra = projectionTestUpstreamBillingExtra(now, 0.1)
	performant := projectionTestAccount(2)
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
			1: {AccountID: 1, LoadRate: 0, WaitingCount: 0},
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
	require.True(t, schedulerProjectionUsesStrategy(OpenAISchedulerGroupPolicy{
		Priority: OpenAISchedulerBusinessPriority{Profit: 3, TTFT: 1, Latency: 2},
	}, true))
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
			"status":            UpstreamBillingProbeStatusOK,
			"peak_rate_enabled": false,
			"received_at":       now.Add(-10 * time.Minute).Format(time.RFC3339Nano),
			"fresh_until":       now.Add(time.Hour).Format(time.RFC3339Nano),
			"next_probe_at":     now.Add(20 * time.Minute).Format(time.RFC3339Nano),
			"data": map[string]any{
				"billing_scope":             "token",
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
