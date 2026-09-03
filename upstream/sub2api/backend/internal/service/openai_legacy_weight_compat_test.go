package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAILegacyWeightOverridesRemainReadableButDoNotChangeRuntimePolicy(t *testing.T) {
	base := openAISchedulerPresetValues(OpenAISchedulerPresetBalanced)
	base.TopK = 4
	base.CandidatePoolMode = OpenAISchedulerCandidatePoolModeHybrid
	base.ExplorationRatio = 20
	base.StarvationThresholdSeconds = 21600
	base.FairnessWeight = 2

	policies := normalizeOpenAISchedulerRuntimeGroupPolicies(
		base.TopK,
		map[string]float64{},
		OpenAISchedulerFairnessSettings{
			CandidatePoolMode:          base.CandidatePoolMode,
			ExplorationRatio:           base.ExplorationRatio,
			StarvationThresholdSeconds: base.StarvationThresholdSeconds,
			FairnessWeight:             base.FairnessWeight,
		},
		`{"11":{"mode":"weighted_override","top_k":7,"weight_overrides":{"upstream_cost":9,"ttft":8,"session_sticky":7,"previous_response":6},"fairness":{"candidate_pool_mode":"all_eligible","exploration_ratio":25}}}`,
	)

	policy := policies[11]
	require.True(t, policy.LegacyWeightOverrideIgnored)
	require.Equal(t, []string{"previous_response", "session_sticky", "ttft", "upstream_cost"}, policy.IgnoredWeightOverrideKeys)
	require.Equal(t, 7, policy.Values.TopK)
	require.Equal(t, OpenAISchedulerCandidatePoolModeAllEligible, policy.Values.CandidatePoolMode)
	require.Equal(t, 25, policy.Values.ExplorationRatio)
	require.Equal(t, base.Priority, policy.Values.Priority)
	require.Equal(t, base.Load, policy.Values.Load)
	require.Equal(t, base.Queue, policy.Values.Queue)
	require.Equal(t, base.ErrorRate, policy.Values.ErrorRate)
	require.Equal(t, base.TTFT, policy.Values.TTFT)
	require.Equal(t, base.UpstreamCost, policy.Values.UpstreamCost)
	require.Equal(t, base.PreviousResponse, policy.Values.PreviousResponse)
	require.Equal(t, base.SessionSticky, policy.Values.SessionSticky)
}

func TestOpenAILegacyWeightOverridesRemainIntactWhenPolicyIsMarshaled(t *testing.T) {
	policy := OpenAISchedulerGroupPolicy{
		WeightOverrides:             map[string]float64{"ttft": 8, "upstream_cost": 9},
		LegacyWeightOverrideIgnored: true,
		IgnoredWeightOverrideKeys:   []string{"ttft", "upstream_cost"},
		Values:                      openAISchedulerPresetValues(OpenAISchedulerPresetBalanced),
	}

	encoded, err := json.Marshal(policy)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(encoded, &raw))
	require.Equal(t, map[string]any{"ttft": 8.0, "upstream_cost": 9.0}, raw["weight_overrides"])
	require.True(t, raw["legacy_weight_override_ignored"].(bool))
	require.Equal(t, []any{"ttft", "upstream_cost"}, raw["ignored_weight_override_keys"])
}

func TestOpenAILegacyWeightOverridesKeepCandidatePoolFairnessAtSelection(t *testing.T) {
	policies := normalizeOpenAISchedulerRuntimeGroupPolicies(
		4,
		map[string]float64{},
		OpenAISchedulerFairnessSettings{
			CandidatePoolMode:          OpenAISchedulerCandidatePoolModeHybrid,
			ExplorationRatio:           20,
			StarvationThresholdSeconds: 21600,
			FairnessWeight:             2,
		},
		`{"11":{"mode":"weighted_override","weight_overrides":{"upstream_cost":9,"ttft":8},"fairness":{"candidate_pool_mode":"all_eligible","exploration_ratio":25,"starvation_threshold_seconds":600,"fairness_weight":4}}}`,
	)
	policy := policies[11]
	weights := GatewayOpenAIWSSchedulerScoreWeightsView{Priority: policy.Values.Priority, Load: policy.Values.Load, Queue: policy.Values.Queue, ErrorRate: policy.Values.ErrorRate, TTFT: policy.Values.TTFT, Reset: policy.Values.Reset, QuotaHeadroom: policy.Values.QuotaHeadroom, UpstreamCost: policy.Values.UpstreamCost, Previous: policy.Values.PreviousResponse, SessionSticky: policy.Values.SessionSticky}

	gotWeights, gotFairness := applyOpenAISchedulerGroupPolicy(weights, defaultOpenAISchedulerFairnessSettings(), policy, true)

	require.Equal(t, OpenAISchedulerCandidatePoolModeAllEligible, gotFairness.CandidatePoolMode)
	require.Equal(t, 25, gotFairness.ExplorationRatio)
	require.Equal(t, 600, gotFairness.StarvationThresholdSeconds)
	require.Equal(t, 4.0, gotFairness.FairnessWeight)
	require.Equal(t, policy.Values.TTFT, gotWeights.TTFT)
	require.Equal(t, policy.Values.UpstreamCost, gotWeights.UpstreamCost)
}
