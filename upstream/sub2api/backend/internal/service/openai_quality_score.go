package service

import (
	"math"
	"sort"
)

const OpenAIUnifiedQualityScoreVersion = "t122-v1"

const (
	openAIUnifiedQualitySuccessWeight    = 0.40
	openAIUnifiedQualityP50TTFTWeight    = 0.24
	openAIUnifiedQualityP90TTFTWeight    = 0.16
	openAIUnifiedQualityOutputRateWeight = 0.10
	openAIUnifiedQualityLiveLoadWeight   = 0.10
)

type OpenAIQualityWindowEvidence struct {
	SampleCount     int64
	Confidence      float64
	EffectiveWeight float64
}

type openAIWindowScoreInput struct {
	Score       float64
	SampleCount int64
	Target      int64
}

type openAIBlendedScore struct {
	Score         float64
	NeutralWeight float64
	Windows       map[OpenAIQualityWindow]OpenAIQualityWindowEvidence
}

type OpenAIQualityBreakdown struct {
	QualityScoreVersion  string
	QualityScore         float64
	SuccessScore         float64
	P50TTFTScore         float64
	P90TTFTScore         float64
	FirstOutputScore     float64
	OutputRateScore      float64
	LiveLoadScore        float64
	Confidence           float64
	P50TTFTMS            *float64
	P90TTFTMS            *float64
	OutputRate           *float64
	Windows              map[OpenAIQualityWindow]OpenAIQualityWindowEvidence
	WindowProvenance     map[OpenAIQualityWindow]OpenAIQualityWindowEvidence
	FirstOutputSlowCount int
	SlowEvidenceReplaced bool
}

type openAIScorePoint struct{ x, y float64 }

func scoreOpenAISuccessRate(rate float64) float64 {
	return interpolateOpenAIScore(rate, []openAIScorePoint{{0, 0}, {.8, 5}, {.9, 25}, {.95, 60}, {.97, 80}, {.99, 95}, {1, 100}}, 50)
}

func scoreOpenAITTFT(ms float64) float64 {
	return interpolateOpenAIScore(ms, []openAIScorePoint{{0, 100}, {2000, 100}, {5000, 80}, {10000, 50}, {20000, 20}, {40000, 5}, {60000, 0}}, 50)
}

func interpolateOpenAIScore(value float64, points []openAIScorePoint, neutral float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || len(points) == 0 {
		return neutral
	}
	if value <= points[0].x {
		return points[0].y
	}
	for i := 1; i < len(points); i++ {
		if value <= points[i].x {
			left, right := points[i-1], points[i]
			if right.x == left.x {
				return right.y
			}
			ratio := (value - left.x) / (right.x - left.x)
			return clampOpenAIScore(left.y + ratio*(right.y-left.y))
		}
	}
	return points[len(points)-1].y
}

func blendOpenAIWindowScores(inputs map[OpenAIQualityWindow]openAIWindowScoreInput, neutral float64) openAIBlendedScore {
	base := map[OpenAIQualityWindow]float64{OpenAIQualityWindow1H: .5, OpenAIQualityWindow24H: .3, OpenAIQualityWindow7D: .2}
	ordered := []OpenAIQualityWindow{OpenAIQualityWindow1H, OpenAIQualityWindow24H, OpenAIQualityWindow7D}
	result := openAIBlendedScore{Windows: make(map[OpenAIQualityWindow]OpenAIQualityWindowEvidence)}
	carry := 0.0
	for _, window := range ordered {
		input := inputs[window]
		available := base[window] + carry
		confidence := 0.0
		if input.Target > 0 && input.SampleCount > 0 {
			confidence = math.Min(1, float64(input.SampleCount)/float64(input.Target))
		}
		weight := available * confidence
		result.Score += clampOpenAIScore(input.Score) * weight
		result.Windows[window] = OpenAIQualityWindowEvidence{SampleCount: input.SampleCount, Confidence: confidence, EffectiveWeight: weight}
		carry = available - weight
	}
	result.NeutralWeight = carry
	result.Score += clampOpenAIScore(neutral) * carry
	return result
}

func scoreOpenAIOutputRate(value, p20, p80 float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || p80 <= p20 {
		return 50
	}
	return clampOpenAIScore((value - p20) / (p80 - p20) * 100)
}

func scoreOpenAILiveLoad(load *AccountLoadInfo, maxConcurrency, minWait, maxWait int) float64 {
	if load == nil || maxConcurrency <= 0 {
		return 50
	}
	remaining := clamp01(float64(maxConcurrency-load.CurrentConcurrency)/float64(maxConcurrency)) * 100
	waitScore := 100.0
	if maxWait > minWait {
		waitScore = clamp01(1-float64(load.WaitingCount-minWait)/float64(maxWait-minWait)) * 100
	}
	return .7*remaining + .3*waitScore
}

func openAIQualityPercentile(values []float64, percentile float64) float64 {
	finite := make([]float64, 0, len(values))
	for _, value := range values {
		if !math.IsNaN(value) && !math.IsInf(value, 0) {
			finite = append(finite, value)
		}
	}
	if len(finite) == 0 {
		return math.NaN()
	}
	sort.Float64s(finite)
	index := int(math.Ceil(percentile*float64(len(finite)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(finite) {
		index = len(finite) - 1
	}
	return finite[index]
}

func clampOpenAIScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func calculateOpenAIUnifiedQualityScore(quality OpenAIAccountQuality, load *AccountLoadInfo, slow *OpenAIFirstOutputSlowTracker, accountID int64) OpenAIQualityBreakdown {
	breakdown := OpenAIQualityBreakdown{
		QualityScoreVersion: OpenAIUnifiedQualityScoreVersion,
		SuccessScore:        50,
		P50TTFTScore:        50,
		P90TTFTScore:        50,
		OutputRateScore:     50,
		LiveLoadScore:       scoreOpenAIUnifiedLiveLoad(load),
		Windows:             make(map[OpenAIQualityWindow]OpenAIQualityWindowEvidence),
	}
	breakdown.WindowProvenance = breakdown.Windows

	metricScore := func(metric func(OpenAIQualityWindowMetrics) (float64, bool), sampleCount func(OpenAIQualityWindowMetrics) int64, score func(float64) float64) (float64, float64) {
		inputs := make(map[OpenAIQualityWindow]openAIWindowScoreInput, len(quality.Windows))
		for window, metrics := range quality.Windows {
			value, ok := metric(metrics)
			if !ok {
				continue
			}
			inputs[window] = openAIWindowScoreInput{Score: score(value), SampleCount: sampleCount(metrics), Target: openAIQualityWindowTarget(window)}
		}
		blended := blendOpenAIWindowScores(inputs, 50)
		return blended.Score, blended.NeutralWeight
	}

	// Attempt counts represent success evidence; metric-specific sample counts
	// keep missing TTFT/output-rate evidence from borrowing request confidence.
	successInputs := make(map[OpenAIQualityWindow]openAIWindowScoreInput, len(quality.Windows))
	for window, metrics := range quality.Windows {
		if finiteQualityValue(metrics.SuccessRate) {
			successInputs[window] = openAIWindowScoreInput{Score: scoreOpenAISuccessRate(*metrics.SuccessRate), SampleCount: metrics.AttemptCount, Target: openAIQualityWindowTarget(window)}
		}
	}
	successBlended := blendOpenAIWindowScores(successInputs, 50)
	breakdown.SuccessScore = successBlended.Score
	breakdown.Confidence = 1 - successBlended.NeutralWeight
	breakdown.Windows = successBlended.Windows
	breakdown.WindowProvenance = breakdown.Windows

	breakdown.P50TTFTScore, _ = metricScore(func(metrics OpenAIQualityWindowMetrics) (float64, bool) {
		if !finiteQualityValue(metrics.TTFTP50MS) {
			return 0, false
		}
		return *metrics.TTFTP50MS, true
	}, func(metrics OpenAIQualityWindowMetrics) int64 { return metrics.TTFTSampleCount }, scoreOpenAITTFT)
	breakdown.P90TTFTScore, _ = metricScore(func(metrics OpenAIQualityWindowMetrics) (float64, bool) {
		if !finiteQualityValue(metrics.TTFTP90MS) {
			return 0, false
		}
		return *metrics.TTFTP90MS, true
	}, func(metrics OpenAIQualityWindowMetrics) int64 { return metrics.TTFTSampleCount }, scoreOpenAITTFT)
	// Output-rate normalization is candidate-relative and therefore remains
	// neutral for this single-account calculator.
	breakdown.FirstOutputScore = .6*breakdown.P50TTFTScore + .4*breakdown.P90TTFTScore
	breakdown.LiveLoadScore = scoreOpenAIUnifiedLiveLoad(load)
	breakdown.QualityScore = openAIUnifiedQualitySuccessWeight*breakdown.SuccessScore +
		openAIUnifiedQualityP50TTFTWeight*breakdown.P50TTFTScore +
		openAIUnifiedQualityP90TTFTWeight*breakdown.P90TTFTScore +
		openAIUnifiedQualityOutputRateWeight*breakdown.OutputRateScore +
		openAIUnifiedQualityLiveLoadWeight*breakdown.LiveLoadScore

	for _, window := range []OpenAIQualityWindow{OpenAIQualityWindow1H, OpenAIQualityWindow24H, OpenAIQualityWindow7D} {
		metrics := quality.Windows[window]
		if breakdown.P50TTFTMS == nil && finiteQualityValue(metrics.TTFTP50MS) {
			breakdown.P50TTFTMS = floatPointer(*metrics.TTFTP50MS)
		}
		if breakdown.P90TTFTMS == nil && finiteQualityValue(metrics.TTFTP90MS) {
			breakdown.P90TTFTMS = floatPointer(*metrics.TTFTP90MS)
		}
		if breakdown.OutputRate == nil && finiteQualityValue(metrics.OutputRateTokensPerSecond) {
			breakdown.OutputRate = floatPointer(*metrics.OutputRateTokensPerSecond)
		}
	}
	if slow != nil {
		view := slow.viewAccount(accountID)
		breakdown.FirstOutputSlowCount = view.SlowCount
		breakdown.SlowEvidenceReplaced = view.Replaced
	}
	return breakdown
}

func scoreOpenAIUnifiedLiveLoad(load *AccountLoadInfo) float64 {
	if load == nil {
		return 50
	}
	if load.LoadRate >= 0 {
		return clampOpenAIScore(100 - float64(load.LoadRate))
	}
	return 50
}

func buildOpenAIQualityBreakdowns(accounts []*Account, qualities map[int64]OpenAIAccountQuality, loads map[int64]*AccountLoadInfo, slow *OpenAIFirstOutputSlowTracker) map[int64]OpenAIQualityBreakdown {
	result := make(map[int64]OpenAIQualityBreakdown, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		load := loads[account.ID]
		result[account.ID] = calculateOpenAIUnifiedQualityScore(qualities[account.ID], load, slow, account.ID)
	}
	return result
}

func openAIQualityWindowTarget(window OpenAIQualityWindow) int64 {
	switch window {
	case OpenAIQualityWindow1H:
		return 20
	case OpenAIQualityWindow24H:
		return 100
	default:
		return 300
	}
}
