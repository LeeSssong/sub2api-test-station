package service

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIQualityScoreCurvesInterpolateAndClamp(t *testing.T) {
	for _, test := range []struct {
		name string
		got  float64
		want float64
	}{
		{"success 100", scoreOpenAISuccessRate(1), 100},
		{"success 99", scoreOpenAISuccessRate(.99), 95},
		{"success 98", scoreOpenAISuccessRate(.98), 87.5},
		{"success below", scoreOpenAISuccessRate(.5), 3.125},
		{"ttft 2s", scoreOpenAITTFT(2000), 100},
		{"ttft 5s", scoreOpenAITTFT(5000), 80},
		{"ttft 15s", scoreOpenAITTFT(15000), 35},
		{"ttft 60s", scoreOpenAITTFT(60000), 0},
	} {
		t.Run(test.name, func(t *testing.T) { require.InDelta(t, test.want, test.got, .001) })
	}
	require.Equal(t, float64(50), scoreOpenAITTFT(math.NaN()))
}

func TestOpenAIQualityConfidenceTransfersToOlderWindowsAndNeutral(t *testing.T) {
	result := blendOpenAIWindowScores(map[OpenAIQualityWindow]openAIWindowScoreInput{
		OpenAIQualityWindow5M:  {Score: 100, SampleCount: 5, Target: 5},
		OpenAIQualityWindow55M: {Score: 80, SampleCount: 25, Target: 50},
	}, 50)

	require.InDelta(t, 79, result.Score, .001)
	require.InDelta(t, .4, result.Windows[OpenAIQualityWindow5M].EffectiveWeight, .001)
	require.InDelta(t, .3, result.Windows[OpenAIQualityWindow55M].EffectiveWeight, .001)
	require.InDelta(t, .3, result.NeutralWeight, .001)
}

func TestOpenAIOutputRateAndLiveLoadScores(t *testing.T) {
	require.Equal(t, float64(0), scoreOpenAIOutputRate(10, 10, 50))
	require.Equal(t, float64(50), scoreOpenAIOutputRate(30, 10, 50))
	require.Equal(t, float64(100), scoreOpenAIOutputRate(50, 10, 50))
	require.Equal(t, float64(50), scoreOpenAIOutputRate(30, 30, 30))

	load := scoreOpenAILiveLoad(&AccountLoadInfo{CurrentConcurrency: 1, WaitingCount: 2}, 4, 0, 4)
	require.InDelta(t, 67.5, load, .001)
	require.Equal(t, float64(50), scoreOpenAILiveLoad(nil, 4, 0, 4))
}
