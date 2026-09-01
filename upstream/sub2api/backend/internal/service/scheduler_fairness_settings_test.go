package service

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
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
	require.Equal(t, "体验优先", available[0].Name)
	require.Equal(t, OpenAISchedulerPresetKindBuiltin, available[0].Kind)
	require.Equal(t, "builtin:balanced", available[1].ID)
	require.Equal(t, "体验均衡", available[1].Name)
	require.Equal(t, "builtin:pro", available[2].ID)
	require.Equal(t, "利润优先", available[2].Name)
	require.Equal(t, customID, available[3].ID)
	require.Equal(t, OpenAISchedulerPresetKindCustom, available[3].Kind)
	require.Equal(t, 10, available[3].Values.TopK)
}

func TestNormalizeOpenAISchedulerPresetValuesForReadClampsLegacyValues(t *testing.T) {
	got := normalizeOpenAISchedulerPresetValuesForRead(OpenAISchedulerPolicyValues{
		TopK:                       99,
		Priority:                   -1,
		Load:                       11,
		Queue:                      math.NaN(),
		ErrorRate:                  math.Inf(1),
		TTFT:                       3,
		Reset:                      4,
		QuotaHeadroom:              5,
		UpstreamCost:               6,
		PreviousResponse:           7,
		SessionSticky:              8,
		CandidatePoolMode:          "invalid",
		ExplorationRatio:           101,
		StarvationThresholdSeconds: 60,
		FairnessWeight:             11,
	})

	require.Equal(t, 32, got.TopK)
	require.Equal(t, 0.0, got.Priority)
	require.Equal(t, 10.0, got.Load)
	require.Equal(t, 0.7, got.Queue)
	require.Equal(t, 0.8, got.ErrorRate)
	require.Equal(t, OpenAISchedulerCandidatePoolModeHybrid, got.CandidatePoolMode)
	require.Equal(t, 100, got.ExplorationRatio)
	require.Equal(t, 300, got.StarvationThresholdSeconds)
	require.Equal(t, 10.0, got.FairnessWeight)
}

func TestNormalizeOpenAISchedulerPresetValuesForReadPreservesZeroThreshold(t *testing.T) {
	got := normalizeOpenAISchedulerPresetValuesForRead(OpenAISchedulerPolicyValues{
		TopK:                       1,
		Priority:                   1,
		CandidatePoolMode:          OpenAISchedulerCandidatePoolModeTopK,
		StarvationThresholdSeconds: 0,
	})
	require.Equal(t, 0, got.StarvationThresholdSeconds)
}

func TestNormalizeOpenAISchedulerPresetValuesRejectsOutOfRangeWrites(t *testing.T) {
	valid := openAISchedulerPresetValues(OpenAISchedulerPresetBalanced)
	for name, mutate := range map[string]func(*OpenAISchedulerPolicyValues){
		"top_k":     func(v *OpenAISchedulerPolicyValues) { v.TopK = 33 },
		"weight":    func(v *OpenAISchedulerPolicyValues) { v.Priority = 11 },
		"ratio":     func(v *OpenAISchedulerPolicyValues) { v.ExplorationRatio = 101 },
		"threshold": func(v *OpenAISchedulerPolicyValues) { v.StarvationThresholdSeconds = 60 },
		"fairness":  func(v *OpenAISchedulerPolicyValues) { v.FairnessWeight = 11 },
	} {
		t.Run(name, func(t *testing.T) {
			value := valid
			mutate(&value)
			_, err := normalizeOpenAISchedulerPresetValues(value)
			require.Error(t, err)
		})
	}
}

func TestNormalizeOpenAISchedulerOverridesRejectsOutOfRangeWrites(t *testing.T) {
	svc := NewSettingService(nil, &config.Config{})
	tooLargeTopK := &SystemSettings{OpenAIAdvancedSchedulerLBTopK: "33"}
	require.Error(t, svc.normalizeOpenAIAdvancedSchedulerOverrides(tooLargeTopK))

	tooLargeWeight := &SystemSettings{OpenAIAdvancedSchedulerWeightPriority: "10.01"}
	require.Error(t, svc.normalizeOpenAIAdvancedSchedulerOverrides(tooLargeWeight))
}

func TestNormalizeOpenAISchedulerCustomPresetsForReadKeepsValidIDsAndClampsValues(t *testing.T) {
	customID := "custom:550e8400-e29b-41d4-a716-446655440000"
	got := normalizeOpenAISchedulerCustomPresetsForRead(map[string]OpenAISchedulerCustomPreset{
		customID: {
			ID:   customID,
			Name: "旧配置",
			Values: OpenAISchedulerPolicyValues{
				TopK:              0,
				Priority:          99,
				CandidatePoolMode: OpenAISchedulerCandidatePoolModeHybrid,
			},
		},
	})
	require.Len(t, got, 1)
	require.Equal(t, customID, got[customID].ID)
	require.Equal(t, 1, got[customID].Values.TopK)
	require.Equal(t, 10.0, got[customID].Values.Priority)
}

func TestSettingServiceParseSettingsClampsLegacySchedulerValues(t *testing.T) {
	svc := NewSettingService(nil, &config.Config{})
	got := svc.parseSettings(map[string]string{
		SettingKeyOpenAIAdvancedSchedulerLBTopK:                     "99",
		SettingKeyOpenAIAdvancedSchedulerWeightPriority:             "-1",
		SettingKeyOpenAIAdvancedSchedulerWeightLoad:                 "11",
		SettingKeyOpenAIAdvancedSchedulerCandidatePoolMode:          "invalid",
		SettingKeyOpenAIAdvancedSchedulerExplorationRatio:           "101",
		SettingKeyOpenAIAdvancedSchedulerStarvationThresholdSeconds: "60",
		SettingKeyOpenAIAdvancedSchedulerFairnessWeight:             "NaN",
	})
	require.Equal(t, "32", got.OpenAIAdvancedSchedulerLBTopK)
	require.Equal(t, "0", got.OpenAIAdvancedSchedulerWeightPriority)
	require.Equal(t, "10", got.OpenAIAdvancedSchedulerWeightLoad)
	require.Equal(t, OpenAISchedulerCandidatePoolModeHybrid, got.OpenAIAdvancedSchedulerCandidatePoolMode)
	require.Equal(t, 100, got.OpenAIAdvancedSchedulerExplorationRatio)
	require.Equal(t, 300, got.OpenAIAdvancedSchedulerStarvationThresholdSeconds)
	require.Equal(t, 2.0, got.OpenAIAdvancedSchedulerFairnessWeight)
}

func TestNormalizeOpenAISchedulerGroupPoliciesForReadClampsLegacySnapshots(t *testing.T) {
	topK := 99
	priority := 11.0
	ratio := 101
	policies := normalizeOpenAISchedulerGroupPoliciesForRead(map[int64]OpenAISchedulerGroupPolicy{
		7: {
			TopK:            &topK,
			WeightOverrides: map[string]float64{"priority": priority},
			Fairness:        &OpenAISchedulerFairnessOverride{ExplorationRatio: &ratio},
		},
	})
	require.Equal(t, 32, *policies[7].TopK)
	require.Equal(t, 10.0, policies[7].WeightOverrides["priority"])
	require.Equal(t, 100, *policies[7].Fairness.ExplorationRatio)
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

func TestOpenAISchedulerGroupPolicyExtraRetryCountRoundTripAndValidation(t *testing.T) {
	raw := `{"11":{"extra_retry_count":3,"mode":"custom","priority":{"profit":1,"ttft":2,"latency":3},"operations":{"balance":"high","peak_protection":"strict","session_continuity":"keep"},"compiled_snapshot":{"top_k":7,"weight_overrides":{"priority":1},"fairness":{"candidate_pool_mode":"hybrid","exploration_ratio":20,"starvation_threshold_seconds":21600,"fairness_weight":2}}}}`
	policies, err := parseOpenAISchedulerGroupPolicies(raw)
	require.NoError(t, err)
	require.NotNil(t, policies[11].ExtraRetryCount)
	require.Equal(t, 3, *policies[11].ExtraRetryCount)
	require.Equal(t, 1, policies[11].Priority.Profit)
	require.Equal(t, "high", policies[11].Operations.Balance)

	encoded, err := json.Marshal(policies)
	require.NoError(t, err)
	var roundTrip map[string]map[string]any
	require.NoError(t, json.Unmarshal(encoded, &roundTrip))
	require.Equal(t, float64(3), roundTrip["11"]["extra_retry_count"])
	require.NotNil(t, roundTrip["11"]["priority"])
	require.NotNil(t, roundTrip["11"]["operations"])
	require.NotNil(t, roundTrip["11"]["compiled_snapshot"])

	for _, invalid := range []string{
		`{"11":{"extra_retry_count":-1}}`,
		`{"11":{"extra_retry_count":4}}`,
		`{"11":{"extra_retry_count":1.5}}`,
	} {
		_, err := parseOpenAISchedulerGroupPolicies(invalid)
		require.Error(t, err, invalid)
	}

	legacy, err := parseOpenAISchedulerGroupPolicies(`{"11":{"candidate_pool_mode":"all_eligible","extra_retry_count":2}}`)
	require.NoError(t, err)
	require.Equal(t, 2, resolveOpenAIExtraRetryCount(legacy[11]))
	require.Equal(t, 0, resolveOpenAIExtraRetryCount(OpenAISchedulerGroupPolicy{}))
	clamped := normalizeOpenAISchedulerGroupPoliciesForRead(map[int64]OpenAISchedulerGroupPolicy{
		12: {ExtraRetryCount: intPtrForTest(9)},
	})
	require.Equal(t, 3, resolveOpenAIExtraRetryCount(clamped[12]))
}

func TestRecommendedOpenAISchedulerBusinessPolicyUsesApprovedPriorityDefaults(t *testing.T) {
	tests := []struct {
		name string
		want OpenAISchedulerBusinessPriority
	}{
		{name: "GPT-特惠", want: OpenAISchedulerBusinessPriority{Profit: 1, TTFT: 2, Latency: 3}},
		{name: "GPT-Plus", want: OpenAISchedulerBusinessPriority{Profit: 1, TTFT: 1, Latency: 1}},
		{name: "GPT-Pro", want: OpenAISchedulerBusinessPriority{Profit: 3, TTFT: 1, Latency: 2}},
		{name: "【专属】GPT-PRO", want: OpenAISchedulerBusinessPriority{Profit: 3, TTFT: 1, Latency: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := recommendedOpenAISchedulerBusinessPolicy(tt.name)
			require.Equal(t, tt.want, policy.Priority)
			require.Equal(t, OpenAISchedulerOperations{Balance: "standard", PeakProtection: "strict", SessionContinuity: "standard"}, policy.Operations)
		})
	}
}

func TestNormalizeOpenAISchedulerBusinessPriorityPreservesEqualPriorityTiers(t *testing.T) {
	got, err := normalizeOpenAISchedulerBusinessPriority(OpenAISchedulerBusinessPriority{Profit: 1, TTFT: 1, Latency: 1})
	require.NoError(t, err)
	require.Equal(t, OpenAISchedulerBusinessPriority{Profit: 1, TTFT: 1, Latency: 1}, got)

	for _, value := range []OpenAISchedulerBusinessPriority{
		{Profit: 0, TTFT: 1, Latency: 2},
		{Profit: 1, TTFT: 4, Latency: 2},
	} {
		_, err := normalizeOpenAISchedulerBusinessPriority(value)
		require.Error(t, err)
	}
}

func TestNormalizeOpenAISchedulerRuntimeGroupPoliciesCompilesBusinessPolicy(t *testing.T) {
	raw := `{"7":{"mode":"custom","priority":{"profit":1,"ttft":2,"latency":3},"operations":{"balance":"high","peak_protection":"strict","session_continuity":"keep"},"compiled_snapshot":{"top_k":32,"weight_overrides":{"upstream_cost":99}}}}`
	got := normalizeOpenAISchedulerRuntimeGroupPolicies(7, nil, defaultOpenAISchedulerFairnessSettings(), raw)
	require.Contains(t, got, int64(7))
	policy := got[7]
	require.Equal(t, OpenAISchedulerBusinessPriority{Profit: 1, TTFT: 2, Latency: 3}, policy.Priority)
	require.NotEqual(t, 99.0, policy.Values.UpstreamCost)
	require.Equal(t, 0, policy.Values.ExplorationRatio)
	require.Equal(t, OpenAISchedulerCandidatePoolModeAllEligible, policy.Values.CandidatePoolMode)
}

func TestNormalizeOpenAISchedulerOperationsValidatesEnumsAndDefaultsMissingValues(t *testing.T) {
	got, err := normalizeOpenAISchedulerOperations(OpenAISchedulerOperations{})
	require.NoError(t, err)
	require.Equal(t, OpenAISchedulerOperations{Balance: "standard", PeakProtection: "strict", SessionContinuity: "standard"}, got)

	valid := []OpenAISchedulerOperations{
		{Balance: "low", PeakProtection: "strict", SessionContinuity: "keep"},
		{Balance: "standard", PeakProtection: "standard", SessionContinuity: "standard"},
		{Balance: "high", PeakProtection: "open", SessionContinuity: "switch"},
	}
	for _, value := range valid {
		_, err := normalizeOpenAISchedulerOperations(value)
		require.NoError(t, err)
	}
	for _, value := range []OpenAISchedulerOperations{
		{Balance: "burst", PeakProtection: "strict", SessionContinuity: "keep"},
		{Balance: "low", PeakProtection: "relaxed", SessionContinuity: "keep"},
		{Balance: "low", PeakProtection: "strict", SessionContinuity: "random"},
	} {
		_, err := normalizeOpenAISchedulerOperations(value)
		require.Error(t, err)
	}
}

func TestParseOpenAISchedulerBusinessPolicyReadsLegacyPayloadWithoutChangingCompiledSnapshot(t *testing.T) {
	legacy := OpenAISchedulerGroupPolicy{
		Mode:            OpenAISchedulerGroupPolicyModeWeightedOverride,
		TopK:            intPtrForTest(7),
		WeightOverrides: map[string]float64{"priority": 2, "ttft": 0.5, "upstream_cost": 2.5},
		Fairness:        &OpenAISchedulerFairnessOverride{CandidatePoolMode: schedulerStringPtr(OpenAISchedulerCandidatePoolModeHybrid), ExplorationRatio: fairnessIntPtr(25), FairnessWeight: floatPtr(3)},
	}
	legacyValues := openAISchedulerPresetValues(OpenAISchedulerPresetBalanced)
	legacy.Values = applyOpenAISchedulerGroupPolicySnapshot(legacyValues, legacy)

	got, err := parseOpenAISchedulerBusinessPolicy(legacy)
	require.NoError(t, err)
	require.Equal(t, legacy.Values, got.CompiledSnapshot)
	require.Equal(t, OpenAISchedulerOperations{Balance: "standard", PeakProtection: "strict", SessionContinuity: "standard"}, got.Operations)
	require.Equal(t, OpenAISchedulerBusinessPriority{Profit: 1, TTFT: 1, Latency: 1}, got.Priority)
}

func schedulerStringPtr(value string) *string { return &value }

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
