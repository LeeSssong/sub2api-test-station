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

func TestOpenAISchedulerAvailablePresetsIncludeImmutableBuiltInsAndCustomPresets(t *testing.T) {
	customID := "custom:550e8400-e29b-41d4-a716-446655440000"
	presets, err := normalizeOpenAISchedulerCustomPresets(map[string]OpenAISchedulerCustomPreset{
		customID: {ID: customID, Name: "晚高峰稳态", Values: openAISchedulerPresetValues(OpenAISchedulerPresetPro)},
	})
	require.NoError(t, err)

	available := openAISchedulerAvailablePresets(presets)
	require.Len(t, available, 4)
	require.Equal(t, "builtin:special_offer", available[0].ID)
	require.Equal(t, OpenAISchedulerPresetKindBuiltin, available[0].Kind)
	require.Equal(t, "builtin:balanced", available[1].ID)
	require.Equal(t, "builtin:pro", available[2].ID)
	require.Equal(t, customID, available[3].ID)
	require.Equal(t, OpenAISchedulerPresetKindCustom, available[3].Kind)
	require.Equal(t, 10, available[3].Values.TopK)
}

func TestNormalizeOpenAISchedulerGroupPoliciesConvertsLegacyModesAndPersistsSnapshots(t *testing.T) {
	global := openAISchedulerPresetValues(OpenAISchedulerPresetBalanced)
	policies, err := normalizeOpenAISchedulerGroupPoliciesWithPresets(map[int64]OpenAISchedulerGroupPolicy{
		10: {Mode: OpenAISchedulerGroupPolicyModeWeightedOverride, WeightOverrides: map[string]float64{"priority": 2}},
		20: {Mode: OpenAISchedulerGroupPolicyModeFair, Preset: OpenAISchedulerPresetPro},
	}, global, nil, nil)
	require.NoError(t, err)
	require.Equal(t, OpenAISchedulerGroupPolicyModeCustom, policies[10].Mode)
	require.Equal(t, 2.0, policies[10].Values.Priority)
	require.Equal(t, OpenAISchedulerGroupPolicyModePreset, policies[20].Mode)
	require.Equal(t, "builtin:pro", policies[20].PresetID)
	require.Equal(t, openAISchedulerPresetValues(OpenAISchedulerPresetPro), policies[20].Values)
}

func TestNormalizeOpenAISchedulerCustomPresetStateRejectsReferencedDeletionAndNormalizesTemporaryID(t *testing.T) {
	previousID := "custom:550e8400-e29b-41d4-a716-446655440000"
	previous := map[string]OpenAISchedulerCustomPreset{
		previousID: {ID: previousID, Name: "保留", Values: openAISchedulerPresetValues(OpenAISchedulerPresetBalanced)},
	}
	policies := map[int64]OpenAISchedulerGroupPolicy{
		1: {Mode: OpenAISchedulerGroupPolicyModePreset, PresetID: previousID, Values: openAISchedulerPresetValues(OpenAISchedulerPresetBalanced)},
	}
	_, _, err := normalizeOpenAISchedulerPresetState(map[string]OpenAISchedulerCustomPreset{}, policies, previous, openAISchedulerPresetValues(OpenAISchedulerPresetBalanced), nil)
	require.Error(t, err)

	newPresets, _, err := normalizeOpenAISchedulerPresetState(map[string]OpenAISchedulerCustomPreset{
		"custom:new:browser-1": {ID: "custom:new:browser-1", Name: "新预设", Values: openAISchedulerPresetValues(OpenAISchedulerPresetBalanced)},
	}, map[int64]OpenAISchedulerGroupPolicy{}, nil, openAISchedulerPresetValues(OpenAISchedulerPresetBalanced), nil)
	require.NoError(t, err)
	require.Len(t, newPresets, 1)
	for id, preset := range newPresets {
		require.Regexp(t, `^custom:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, id)
		require.Equal(t, id, preset.ID)
	}
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
	tooLarge := 11.0
	_, err = normalizeOpenAISchedulerGroupPolicies(map[int64]OpenAISchedulerGroupPolicy{
		3: {Mode: OpenAISchedulerGroupPolicyModeWeightedOverride, WeightOverrides: map[string]float64{"priority": tooLarge}},
	}, OpenAISchedulerPolicyValues{TopK: 7, Priority: 1}, map[int64]struct{}{3: {}})
	require.Error(t, err)
	tooLargeTopK := OpenAISchedulerPolicyValues{TopK: 33, Priority: 1}
	_, err = normalizeOpenAISchedulerGroupPolicies(map[int64]OpenAISchedulerGroupPolicy{
		4: {Mode: OpenAISchedulerGroupPolicyModeWeightedOverride},
	}, tooLargeTopK, map[int64]struct{}{4: {}})
	require.Error(t, err)
	customFairness := OpenAISchedulerFairnessOverride{ExplorationRatio: fairnessIntPtr(101)}
	_, err = normalizeOpenAISchedulerGroupPolicies(map[int64]OpenAISchedulerGroupPolicy{
		5: {Mode: OpenAISchedulerGroupPolicyModeFair, Preset: OpenAISchedulerPresetBalanced, Fairness: &customFairness},
	}, OpenAISchedulerPolicyValues{TopK: 7, Priority: 1}, map[int64]struct{}{5: {}})
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
