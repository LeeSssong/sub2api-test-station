package service

import (
	"testing"

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
	require.InDelta(t, 50, got.OutputRateScore, .001)
	require.InDelta(t, 83, got.LiveLoadScore, .001)
	// 100*.40 + 100*.24 + 80*.16 + 50*.10 + 83*.10.
	require.InDelta(t, 90.1, got.QualityScore, .001)
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
