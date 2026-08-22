package service

import (
	"context"
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestApplyOpenAISchedulerGroupPolicyWeightedDisablesFairness(t *testing.T) {
	weights := GatewayOpenAIWSSchedulerScoreWeightsView{Priority: 1, Load: 1, Queue: 1, ErrorRate: 1, TTFT: 1, UpstreamCost: 1, Previous: 5, SessionSticky: 3}
	fairness := defaultOpenAISchedulerFairnessSettings()
	policy := OpenAISchedulerGroupPolicy{
		Mode:   OpenAISchedulerGroupPolicyModeWeightedOverride,
		Values: OpenAISchedulerPolicyValues{TopK: 4, Priority: 2, Load: 3, Queue: 1, ErrorRate: 1, TTFT: 1, UpstreamCost: 2, PreviousResponse: 7, SessionSticky: 8},
	}
	gotWeights, gotFairness := applyOpenAISchedulerGroupPolicy(weights, fairness, policy, true)
	require.Equal(t, 2.0, gotWeights.Priority)
	require.Equal(t, 3.0, gotWeights.Load)
	require.Equal(t, 7.0, gotWeights.Previous)
	require.Equal(t, 8.0, gotWeights.SessionSticky)
	require.Equal(t, 0.0, gotFairness.FairnessWeight)
	require.Equal(t, OpenAISchedulerCandidatePoolModeTopK, gotFairness.CandidatePoolMode)
}

func TestApplyOpenAISchedulerGroupPolicyFairKeepsFairness(t *testing.T) {
	weights := GatewayOpenAIWSSchedulerScoreWeightsView{Priority: 1, Load: 1, Queue: 1, ErrorRate: 1, TTFT: 1, UpstreamCost: 1, Previous: 5, SessionSticky: 3}
	fairness := defaultOpenAISchedulerFairnessSettings()
	policy := OpenAISchedulerGroupPolicy{
		Mode:   OpenAISchedulerGroupPolicyModeFair,
		Values: OpenAISchedulerPolicyValues{TopK: 10, Priority: 1.2, Load: 1.4, Queue: 1.2, ErrorRate: 2.5, TTFT: 2, UpstreamCost: 1.5, ExplorationRatio: 40, StarvationThresholdSeconds: 10800, FairnessWeight: 5, CandidatePoolMode: OpenAISchedulerCandidatePoolModeHybrid, PreviousResponse: 5, SessionSticky: 3},
	}
	gotWeights, gotFairness := applyOpenAISchedulerGroupPolicy(weights, fairness, policy, true)
	require.Equal(t, 2.5, gotWeights.ErrorRate)
	require.Equal(t, 2.0, gotWeights.TTFT)
	require.Equal(t, 5.0, gotFairness.FairnessWeight)
	require.Equal(t, 40, gotFairness.ExplorationRatio)
	require.Equal(t, 10800, gotFairness.StarvationThresholdSeconds)
}

func adaptiveCandidate(id int64, score float64) openAIAccountCandidateScore {
	return openAIAccountCandidateScore{
		account:  &Account{ID: id},
		loadInfo: &AccountLoadInfo{AccountID: id},
		score:    score,
	}
}

func TestApplyOpenAIAdaptiveTopKFiltersByScoreGap(t *testing.T) {
	candidates := []openAIAccountCandidateScore{
		adaptiveCandidate(1, 4.0),
		adaptiveCandidate(2, 3.9),
		adaptiveCandidate(3, 3.7),
	}

	selected, threshold, fallback := applyOpenAIAdaptiveTopK(candidates, 7, 7, 0.15)

	require.False(t, fallback)
	require.InDelta(t, 3.85, threshold, 0.000001)
	require.Equal(t, []int64{1, 2}, []int64{selected[0].account.ID, selected[1].account.ID})
}

func TestApplyOpenAIAdaptiveTopKHonorsConfiguredAndAbsoluteCaps(t *testing.T) {
	candidates := []openAIAccountCandidateScore{
		adaptiveCandidate(1, 4.0),
		adaptiveCandidate(2, 3.9),
		adaptiveCandidate(3, 3.8),
	}

	selected, _, fallback := applyOpenAIAdaptiveTopK(candidates, 3, 2, 10)

	require.False(t, fallback)
	require.Len(t, selected, 2)
	require.Equal(t, []int64{1, 2}, []int64{selected[0].account.ID, selected[1].account.ID})
}

func TestApplyOpenAIAdaptiveTopKKeepsAllAvailableCandidatesBelowCap(t *testing.T) {
	candidates := []openAIAccountCandidateScore{
		adaptiveCandidate(1, 4.0),
		adaptiveCandidate(2, 3.9),
	}

	selected, _, fallback := applyOpenAIAdaptiveTopK(candidates, 7, 12, 10)

	require.False(t, fallback)
	require.Len(t, selected, 2)
}

func TestApplyOpenAIAdaptiveTopKFallsBackToBestFiniteCandidate(t *testing.T) {
	candidates := []openAIAccountCandidateScore{
		adaptiveCandidate(1, math.NaN()),
		adaptiveCandidate(2, 3.9),
		adaptiveCandidate(3, math.Inf(1)),
		adaptiveCandidate(4, 4.0),
	}

	selected, threshold, fallback := applyOpenAIAdaptiveTopK(candidates, 7, 7, 0.15)

	require.True(t, fallback)
	require.InDelta(t, 4.0, threshold, 0.000001)
	require.Len(t, selected, 1)
	require.Equal(t, int64(4), selected[0].account.ID)
}

func TestOpenAIAccountSchedulerAdaptiveTopKNarrowsHealthyPool(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	accounts := []Account{
		{ID: 4101, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0},
		{ID: 4102, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		{ID: 4103, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 7
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIScheduler.AdaptiveTopKEnabled = true
	cfg.Gateway.OpenAIScheduler.AdaptiveTopKMax = 7
	cfg.Gateway.OpenAIScheduler.AdaptiveTopKScoreGap = 0.4
	svc := &OpenAIGatewayService{
		accountRepo:      schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:            &schedulerTestGatewayCache{},
		cfg:              cfg,
		rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{
				4101: {AccountID: 4101}, 4102: {AccountID: 4102}, 4103: {AccountID: 4103},
			},
			acquireResults: map[int64]bool{4101: true},
		}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		context.Background(), nil, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(4101), selection.Account.ID)
	require.Equal(t, 3, decision.CandidateCount)
	require.Equal(t, 3, decision.EligibleCount)
	require.Equal(t, 1, decision.EffectiveTopK)
	require.Equal(t, 1, decision.TopK)
	require.Equal(t, openAIAccountScheduleLayerAdaptiveTopK, decision.SelectionLayer)
	require.InDelta(t, 0.6, decision.MinimumScoreThreshold, 0.000001)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIAccountSchedulerDefaultKeepsHealthyPoolWhenAdaptiveTopKDisabled(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	accounts := []Account{
		{ID: 4401, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0},
		{ID: 4402, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
		{ID: 4403, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 7
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIScheduler.AdaptiveTopKEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:      schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:            &schedulerTestGatewayCache{},
		cfg:              cfg,
		rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{
				4401: {AccountID: 4401}, 4402: {AccountID: 4402}, 4403: {AccountID: 4403},
			},
			acquireResults: map[int64]bool{4401: true},
		}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		context.Background(), nil, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, 3, decision.CandidateCount)
	require.Equal(t, 3, decision.EligibleCount)
	require.Equal(t, 3, decision.EffectiveTopK)
	require.Equal(t, 3, decision.TopK)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.SelectionLayer)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIAccountSchedulerAdaptiveTopKNeverReaddsExcludedBestAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	accounts := []Account{
		{ID: 4201, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0},
		{ID: 4202, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 7
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIScheduler.AdaptiveTopKEnabled = true
	cfg.Gateway.OpenAIScheduler.AdaptiveTopKMax = 7
	cfg.Gateway.OpenAIScheduler.AdaptiveTopKScoreGap = 10
	svc := &OpenAIGatewayService{
		accountRepo:      schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:            &schedulerTestGatewayCache{},
		cfg:              cfg,
		rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
			loadMap:        map[int64]*AccountLoadInfo{4202: {AccountID: 4202}},
			acquireResults: map[int64]bool{4202: true},
		}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		context.Background(), nil, "", "", "gpt-5.1", map[int64]struct{}{4201: {}}, OpenAIUpstreamTransportAny, false,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(4202), selection.Account.ID)
	require.Equal(t, 1, decision.CandidateCount)
	require.Equal(t, 1, decision.EligibleCount)
	require.Equal(t, 1, decision.EffectiveTopK)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIAccountSchedulerAdaptiveTopKEscapesWeightedStickyBelowQualityFloor(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	accounts := []Account{
		{ID: 4301, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10},
		{ID: 4302, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 7
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIScheduler.AdaptiveTopKEnabled = true
	cfg.Gateway.OpenAIScheduler.AdaptiveTopKMax = 7
	cfg.Gateway.OpenAIScheduler.AdaptiveTopKScoreGap = 0.1
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:session_hash_adaptive_quality": 4301}}
	svc := &OpenAIGatewayService{
		accountRepo:      schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:            cache,
		cfg:              cfg,
		rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true", "true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
			loadMap:        map[int64]*AccountLoadInfo{4301: {AccountID: 4301}, 4302: {AccountID: 4302}},
			acquireResults: map[int64]bool{4302: true},
		}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		context.Background(), nil, "", "session_hash_adaptive_quality", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(4302), selection.Account.ID)
	require.False(t, decision.StickyKept)
	require.Equal(t, "quality_floor", decision.StickyEscapeReason)
	require.Equal(t, int64(4301), cache.sessionBindings["openai:session_hash_adaptive_quality"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
