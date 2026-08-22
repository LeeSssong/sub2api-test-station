package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
