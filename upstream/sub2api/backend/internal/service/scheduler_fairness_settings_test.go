package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAISchedulerGroupPoliciesExpandsPresetsAndInheritsGlobal(t *testing.T) {
	global := OpenAISchedulerPolicyValues{TopK: 7, Priority: 1, Load: 1, Queue: .7, ErrorRate: .8, TTFT: .5, PreviousResponse: 5, SessionSticky: 3, CandidatePoolMode: OpenAISchedulerCandidatePoolModeHybrid, ExplorationRatio: 25, StarvationThresholdSeconds: 21600, FairnessWeight: 3}
	policies, err := normalizeOpenAISchedulerGroupPolicies(map[int64]OpenAISchedulerGroupPolicy{
		10: {Mode: OpenAISchedulerGroupPolicyModeFair, Preset: OpenAISchedulerPresetPro},
		20: {Mode: OpenAISchedulerGroupPolicyModeWeightedOverride, WeightOverrides: map[string]float64{"priority": 2}},
	}, global, map[int64]struct{}{10: {}, 20: {}})
	require.NoError(t, err)
	pro := policies[10]
	require.Equal(t, OpenAISchedulerPresetPro, pro.Preset)
	require.Equal(t, 10, pro.Values.TopK)
	require.Equal(t, 40, pro.Values.ExplorationRatio)
	require.Equal(t, 5.0, pro.Values.FairnessWeight)
	require.Equal(t, 2.0, policies[20].Values.Priority)
	require.Equal(t, global.Load, policies[20].Values.Load)
}

func TestNormalizeOpenAISchedulerGroupPoliciesLegacyJSON(t *testing.T) {
	var raw = `{"10":{"candidate_pool_mode":"all_eligible","exploration_ratio":40}}`
	policies, err := parseOpenAISchedulerGroupPolicies(raw)
	require.NoError(t, err)
	require.Equal(t, OpenAISchedulerGroupPolicyModeWeightedOverride, policies[10].Mode)
	require.NotNil(t, policies[10].LegacyFairness.ExplorationRatio)
	encoded, err := json.Marshal(policies)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "exploration_ratio")
}

func TestNormalizeOpenAISchedulerGroupPoliciesRejectsInvalidPayload(t *testing.T) {
	_, err := normalizeOpenAISchedulerGroupPolicies(map[int64]OpenAISchedulerGroupPolicy{
		1: {Mode: OpenAISchedulerGroupPolicyModeFair, Preset: "unknown"},
	}, OpenAISchedulerPolicyValues{TopK: 7, Priority: 1}, map[int64]struct{}{1: {}})
	require.Error(t, err)
	_, err = normalizeOpenAISchedulerGroupPolicies(map[int64]OpenAISchedulerGroupPolicy{
		2: {Mode: OpenAISchedulerGroupPolicyModeWeightedOverride, WeightOverrides: map[string]float64{"priority": 0}},
	}, OpenAISchedulerPolicyValues{TopK: 7, Priority: 1}, map[int64]struct{}{2: {}})
	require.Error(t, err)
}

func TestNormalizeOpenAISchedulerFairnessSettingsDefaultsMissingValues(t *testing.T) {
	got, err := normalizeOpenAISchedulerFairnessSettings(OpenAISchedulerFairnessSettings{})
	require.NoError(t, err)
	require.Equal(t, OpenAISchedulerCandidatePoolModeHybrid, got.CandidatePoolMode)
	require.Equal(t, 20, got.ExplorationRatio)
	require.Equal(t, 21600, got.StarvationThresholdSeconds)
	require.Equal(t, 2.0, got.FairnessWeight)
	require.Empty(t, got.GroupOverrides)
}

func TestNormalizeOpenAISchedulerFairnessSettingsRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*OpenAISchedulerFairnessSettings)
	}{
		{name: "mode", mutate: func(v *OpenAISchedulerFairnessSettings) { v.CandidatePoolMode = "random" }},
		{name: "ratio", mutate: func(v *OpenAISchedulerFairnessSettings) { v.ExplorationRatio = 101 }},
		{name: "threshold", mutate: func(v *OpenAISchedulerFairnessSettings) { v.StarvationThresholdSeconds = 60 }},
		{name: "weight", mutate: func(v *OpenAISchedulerFairnessSettings) { v.FairnessWeight = 11 }},
		{name: "group id", mutate: func(v *OpenAISchedulerFairnessSettings) {
			v.GroupOverrides = map[int64]OpenAISchedulerFairnessOverride{0: {ExplorationRatio: fairnessIntPtr(20)}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := OpenAISchedulerFairnessSettings{}
			tt.mutate(&value)
			_, err := normalizeOpenAISchedulerFairnessSettings(value)
			require.Error(t, err)
		})
	}
}

func TestResolveOpenAISchedulerFairnessOverrideFallsBackToGlobal(t *testing.T) {
	global := OpenAISchedulerFairnessSettings{
		CandidatePoolMode:          OpenAISchedulerCandidatePoolModeHybrid,
		ExplorationRatio:           20,
		StarvationThresholdSeconds: 21600,
		FairnessWeight:             2,
		GroupOverrides: map[int64]OpenAISchedulerFairnessOverride{
			20: {ExplorationRatio: fairnessIntPtr(40)},
		},
	}
	got := resolveOpenAISchedulerFairnessForGroup(global, 20)
	require.Equal(t, OpenAISchedulerCandidatePoolModeHybrid, got.CandidatePoolMode)
	require.Equal(t, 40, got.ExplorationRatio)
	require.Equal(t, 21600, got.StarvationThresholdSeconds)
	require.Equal(t, 2.0, got.FairnessWeight)
}

func fairnessIntPtr(value int) *int { return &value }

func TestOpenAIFairnessFactorRanksLeastRecentlyUsedAccount(t *testing.T) {
	now := time.Now()
	recent := now.Add(-5 * time.Minute)
	old := now.Add(-2 * time.Hour)
	candidates := []openAIAccountCandidateScore{
		{account: &Account{ID: 1, LastUsedAt: &recent}},
		{account: &Account{ID: 2, LastUsedAt: &old}},
	}
	recentFactor := openAIFairnessFactor(candidates[0].account, candidates, now)
	oldFactor := openAIFairnessFactor(candidates[1].account, candidates, now)
	require.Greater(t, oldFactor, recentFactor)
	require.InDelta(t, 1, oldFactor, 0.000001)
}

func TestAppendOldestStarvedOpenAICandidateAddsOnlyOne(t *testing.T) {
	now := time.Now()
	oldest := now.Add(-4 * time.Hour)
	older := now.Add(-3 * time.Hour)
	selected := []openAIAccountCandidateScore{{account: &Account{ID: 1}}}
	eligible := []openAIAccountCandidateScore{
		selected[0],
		{account: &Account{ID: 2, LastUsedAt: &older}},
		{account: &Account{ID: 3, LastUsedAt: &oldest}},
	}
	got := appendOldestStarvedOpenAICandidate(selected, eligible, 300, now)
	require.Len(t, got, 2)
	require.Equal(t, int64(3), got[1].account.ID)
}
