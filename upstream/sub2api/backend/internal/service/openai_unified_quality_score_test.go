package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Production break caught: changing a fixed quality coefficient, or folding
// P50/P90 back into an adjustable combined TTFT weight, changes this score.
func TestOpenAIUnifiedQualityScoreUsesVersionedFixedComponents(t *testing.T) {
	quality := OpenAIAccountQuality{AccountID: 19, Windows: map[OpenAIQualityWindow]OpenAIQualityWindowMetrics{
		OpenAIQualityWindow1H: {
			AttemptCount: 20, SuccessCount: 20, SuccessRate: floatPtr(1),
			TTFTSampleCount: 20, TTFTP50MS: floatPtr(2000), TTFTP90MS: floatPtr(5000),
			OutputRateSampleCount: 20, OutputRateTokensPerSecond: floatPtr(32),
		},
		OpenAIQualityWindow24H: {
			AttemptCount: 100, SuccessCount: 100, SuccessRate: floatPtr(1),
			TTFTSampleCount: 100, TTFTP50MS: floatPtr(2000), TTFTP90MS: floatPtr(5000),
			OutputRateSampleCount: 100, OutputRateTokensPerSecond: floatPtr(32),
		},
		OpenAIQualityWindow7D: {
			AttemptCount: 300, SuccessCount: 300, SuccessRate: floatPtr(1),
			TTFTSampleCount: 300, TTFTP50MS: floatPtr(2000), TTFTP90MS: floatPtr(5000),
			OutputRateSampleCount: 300, OutputRateTokensPerSecond: floatPtr(32),
		},
	}}

	got := calculateOpenAIUnifiedQualityScore(quality, &AccountLoadInfo{LoadRate: 17}, nil, 19)

	require.Equal(t, "t122-v1", OpenAIUnifiedQualityScoreVersion)
	require.InDelta(t, 100, got.SuccessScore, .001)
	require.InDelta(t, 100, got.P50TTFTScore, .001)
	require.InDelta(t, 80, got.P90TTFTScore, .001)
	require.InDelta(t, 55, got.OutputRateScore, .001)
	require.InDelta(t, 83, got.LiveLoadScore, .001)
	// 100*.40 + 100*.24 + 80*.16 + 55*.10 + 83*.10.
	require.InDelta(t, 90.6, got.QualityScore, .001)
	require.InDelta(t, 1, got.Confidence, .001)
	require.InDelta(t, 1, got.Windows[OpenAIQualityWindow1H].Confidence, .001)
}

// Production break caught: treating an unavailable quality snapshot as a
// failed account would turn an otherwise eligible candidate into a zero score.
func TestOpenAIUnifiedQualityScoreUsesNeutralValuesForMissingEvidence(t *testing.T) {
	got := calculateOpenAIUnifiedQualityScore(OpenAIAccountQuality{AccountID: 19}, nil, nil, 19)

	require.InDelta(t, 50, got.QualityScore, .001)
	require.InDelta(t, 50, got.SuccessScore, .001)
	require.InDelta(t, 50, got.P50TTFTScore, .001)
	require.InDelta(t, 50, got.P90TTFTScore, .001)
	require.InDelta(t, 50, got.OutputRateScore, .001)
	require.InDelta(t, 50, got.LiveLoadScore, .001)
	require.InDelta(t, 0, got.Confidence, .001)
	for _, window := range []OpenAIQualityWindow{OpenAIQualityWindow1H, OpenAIQualityWindow24H, OpenAIQualityWindow7D} {
		require.InDelta(t, 0, got.Windows[window].Confidence, .001)
	}
}

// Production break caught: applying group policy, price, or profit fields in
// the quality builder would make a shared account score differ by text group.
func TestOpenAIUnifiedQualityScoreIgnoresGroupAndCommercialFields(t *testing.T) {
	quality := OpenAIAccountQuality{AccountID: 19, Windows: map[OpenAIQualityWindow]OpenAIQualityWindowMetrics{
		OpenAIQualityWindow1H: {
			AttemptCount: 20, SuccessCount: 19, SuccessRate: floatPtr(.95),
			TTFTSampleCount: 20, TTFTP50MS: floatPtr(3000), TTFTP90MS: floatPtr(7000),
		},
	}}
	lowMultiplier, highMultiplier := .1, 50.0
	lowCost, highCost := .01, 1000.0
	cheap := &Account{ID: 19, Concurrency: 4, RateMultiplier: &lowMultiplier, UpstreamActualCost: &lowCost}
	expensive := &Account{ID: 19, Concurrency: 4, RateMultiplier: &highMultiplier, UpstreamActualCost: &highCost}
	qualities := map[int64]OpenAIAccountQuality{19: quality}
	loads := map[int64]*AccountLoadInfo{19: {AccountID: 19, CurrentConcurrency: 1}}

	group2 := buildOpenAIQualityBreakdowns([]*Account{cheap}, qualities, loads, nil)[19]
	group20 := buildOpenAIQualityBreakdowns([]*Account{expensive}, qualities, loads, nil)[19]

	require.Equal(t, group2, group20)
}

// Production break caught: leaving output rate neutral causes materially
// faster visible output to have no influence on quality score or rank.
func TestOpenAIUnifiedQualityScoreRanksConfidenceWeightedOutputRate(t *testing.T) {
	quality := func(accountID int64, outputRate float64) OpenAIAccountQuality {
		return OpenAIAccountQuality{AccountID: accountID, Windows: map[OpenAIQualityWindow]OpenAIQualityWindowMetrics{
			OpenAIQualityWindow1H: {
				AttemptCount: 20, SuccessCount: 19, SuccessRate: floatPtr(.95),
				TTFTSampleCount: 20, TTFTP50MS: floatPtr(3000), TTFTP90MS: floatPtr(5000),
				OutputRateSampleCount: 20, OutputRateTokensPerSecond: floatPtr(outputRate),
			},
			OpenAIQualityWindow24H: {
				AttemptCount: 100, SuccessCount: 95, SuccessRate: floatPtr(.95),
				TTFTSampleCount: 100, TTFTP50MS: floatPtr(3000), TTFTP90MS: floatPtr(5000),
				OutputRateSampleCount: 100, OutputRateTokensPerSecond: floatPtr(outputRate),
			},
			OpenAIQualityWindow7D: {
				AttemptCount: 300, SuccessCount: 285, SuccessRate: floatPtr(.95),
				TTFTSampleCount: 300, TTFTP50MS: floatPtr(3000), TTFTP90MS: floatPtr(5000),
				OutputRateSampleCount: 300, OutputRateTokensPerSecond: floatPtr(outputRate),
			},
		}}
	}
	accounts := []*Account{{ID: 1}, {ID: 2}}
	breakdowns := buildOpenAIQualityBreakdowns(accounts, map[int64]OpenAIAccountQuality{
		1: quality(1, 10),
		2: quality(2, 50),
	}, nil, nil)

	require.InDelta(t, 0, breakdowns[1].OutputRateScore, .001)
	require.InDelta(t, 100, breakdowns[2].OutputRateScore, .001)
	require.Less(t, breakdowns[1].QualityScore, breakdowns[2].QualityScore)
	ordered := sortOpenAIUnifiedQualityCandidates([]openAIUnifiedQualityCandidate{
		{account: accounts[0], quality: breakdowns[1]},
		{account: accounts[1], quality: breakdowns[2]},
	})
	require.Equal(t, []int64{2, 1}, unifiedCandidateIDs(ordered))
}

// Production break caught: candidate-pool-relative output normalization makes
// the same account score differently when only its eligible peers change.
func TestOpenAIUnifiedQualityScoreOutputRateIsInvariantAcrossCandidatePools(t *testing.T) {
	quality := func(accountID int64, outputRate float64) OpenAIAccountQuality {
		return OpenAIAccountQuality{AccountID: accountID, Windows: map[OpenAIQualityWindow]OpenAIQualityWindowMetrics{
			OpenAIQualityWindow1H: {
				AttemptCount: 20, SuccessCount: 19, SuccessRate: floatPtr(.95),
				TTFTSampleCount: 20, TTFTP50MS: floatPtr(3000), TTFTP90MS: floatPtr(5000),
				OutputRateSampleCount: 20, OutputRateTokensPerSecond: floatPtr(outputRate),
			},
			OpenAIQualityWindow24H: {
				AttemptCount: 100, SuccessCount: 95, SuccessRate: floatPtr(.95),
				TTFTSampleCount: 100, TTFTP50MS: floatPtr(3000), TTFTP90MS: floatPtr(5000),
				OutputRateSampleCount: 100, OutputRateTokensPerSecond: floatPtr(outputRate),
			},
			OpenAIQualityWindow7D: {
				AttemptCount: 300, SuccessCount: 285, SuccessRate: floatPtr(.95),
				TTFTSampleCount: 300, TTFTP50MS: floatPtr(3000), TTFTP90MS: floatPtr(5000),
				OutputRateSampleCount: 300, OutputRateTokensPerSecond: floatPtr(outputRate),
			},
		}}
	}
	target := quality(19, 30)
	poolWithSlowerPeer := buildOpenAIQualityBreakdowns([]*Account{{ID: 19}, {ID: 20}}, map[int64]OpenAIAccountQuality{
		19: target,
		20: quality(20, 10),
	}, nil, nil)[19]
	poolWithFasterPeer := buildOpenAIQualityBreakdowns([]*Account{{ID: 19}, {ID: 21}}, map[int64]OpenAIAccountQuality{
		19: target,
		21: quality(21, 50),
	}, nil, nil)[19]

	require.InDelta(t, poolWithSlowerPeer.OutputRateScore, poolWithFasterPeer.OutputRateScore, .001)
	require.InDelta(t, poolWithSlowerPeer.QualityScore, poolWithFasterPeer.QualityScore, .001)
}

// Production break caught: recording the 60-second observation only for
// diagnostics leaves a repeatedly slow account indistinguishable in rank.
func TestOpenAIUnifiedQualityScoreUsesSlowEvidenceForLaterRanking(t *testing.T) {
	now := time.Date(2026, 9, 3, 3, 0, 0, 0, time.UTC)
	var timers []*manualSlowTimer
	tracker := newOpenAIFirstOutputSlowTracker(func() time.Time { return now }, func(_ time.Duration, fn func()) openAISlowTimer {
		timer := &manualSlowTimer{fn: fn}
		timers = append(timers, timer)
		return timer
	})
	observation := tracker.Start(2, 1, "slow-attempt", now)
	require.NotNil(t, observation)
	require.Len(t, timers, 1)
	timers[0].Fire()

	quality := func(accountID int64) OpenAIAccountQuality {
		return OpenAIAccountQuality{AccountID: accountID, Windows: map[OpenAIQualityWindow]OpenAIQualityWindowMetrics{
			OpenAIQualityWindow1H: {
				AttemptCount: 20, SuccessCount: 19, SuccessRate: floatPtr(.95),
				TTFTSampleCount: 20, TTFTP50MS: floatPtr(3000), TTFTP90MS: floatPtr(5000),
			},
		}}
	}
	accounts := []*Account{{ID: 1}, {ID: 2}}
	breakdowns := buildOpenAIQualityBreakdowns(accounts, map[int64]OpenAIAccountQuality{
		1: quality(1),
		2: quality(2),
	}, nil, tracker)

	require.Equal(t, 1, breakdowns[1].FirstOutputSlowCount)
	require.Less(t, breakdowns[1].QualityScore, breakdowns[2].QualityScore)
	ordered := sortOpenAIUnifiedQualityCandidates([]openAIUnifiedQualityCandidate{
		{account: accounts[0], quality: breakdowns[1]},
		{account: accounts[1], quality: breakdowns[2]},
	})
	require.Equal(t, []int64{2, 1}, unifiedCandidateIDs(ordered))
}
