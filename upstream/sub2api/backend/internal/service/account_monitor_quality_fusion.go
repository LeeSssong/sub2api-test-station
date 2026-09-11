package service

import "time"

const (
	accountMonitorQualityFreshnessFresh   = "fresh"
	accountMonitorQualityFreshnessStale   = "stale"
	accountMonitorQualityFreshnessUnknown = "unknown"
	accountMonitorQualitySourceReal       = "real_request"
	accountMonitorQualitySourceProbe      = "monitor_probe"
	accountMonitorQualitySourceHybrid     = "hybrid"
	accountMonitorQualitySourceUnified    = "unified"
	accountMonitorQualitySourceUnknown    = "unknown"
)

// fuseAccountMonitorQualityEvidence preserves compatibility for repositories
// that still return real requests and probes separately. Callers backed by the
// unified request repository pass an empty probe aggregate.
func fuseAccountMonitorQualityEvidence(real AccountMonitorWindowAggregate, probe AccountMonitorAggregate, latest AccountMonitorLatest, settings AccountMonitorSettings, now time.Time) AccountMonitorQualityEvidence {
	now = now.UTC()
	realSamples := clampNonNegativeInt64(real.RequestCount)
	probeSamples := clampNonNegativeInt(probe.SampleCount)
	realSuccesses := clampCount(real.SuccessCount, realSamples)
	probeSuccesses := clampCount(int64(legacyAggregateSuccessSamples(probe)), int64(probeSamples))
	realAt := accountMonitorWindowObservedAt(real)
	probeAt := accountMonitorProbeObservedAt(probe, latest)
	ttl := time.Duration(settings.IntervalSeconds*2) * time.Second
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	// Legacy window aggregate adapters do not always carry MAX(created_at).
	// Their rows are still scoped to the requested current window, but the
	// absence of a timestamp must not be exposed as an invented observation.
	realAvailable := realSamples > 0
	probeAvailable := probeSamples > 0
	realFresh := realAvailable && (realAt.IsZero() || isAccountMonitorEvidenceFresh(realAt, now, ttl))
	probeFresh := probeAvailable && isAccountMonitorEvidenceFresh(probeAt, now, ttl)
	if !realAvailable && !probeAvailable {
		return accountMonitorUnknownQualityEvidence("missing")
	}
	useReal, useProbe := realFresh, probeFresh
	freshness := accountMonitorQualityFreshnessFresh
	if !useReal && !useProbe {
		useReal, useProbe = realAvailable, probeAvailable
		freshness = accountMonitorQualityFreshnessStale
	}

	evidence := AccountMonitorQualityEvidence{
		Known: true, Freshness: freshness,
		ObservedAt: latestEvidenceTime(realAt, probeAt),
	}
	if useReal {
		evidence.RealRequestSamples = int(realSamples)
		evidence.SampleCount += int(realSamples)
		evidence.SuccessSampleCount += int(realSuccesses)
	}
	if useProbe {
		evidence.ProbeSamples = probeSamples
		evidence.SampleCount += probeSamples
		evidence.SuccessSampleCount += int(probeSuccesses)
	}
	switch {
	case useReal && useProbe:
		evidence.Source = accountMonitorQualitySourceHybrid
	case useReal:
		evidence.Source = accountMonitorQualitySourceReal
	case useProbe:
		evidence.Source = accountMonitorQualitySourceProbe
	}
	if evidence.SampleCount > 0 {
		evidence.RealRequestWeight = float64(evidence.RealRequestSamples) / float64(evidence.SampleCount)
		evidence.ProbeWeight = float64(evidence.ProbeSamples) / float64(evidence.SampleCount)
		evidence.SuccessRate = float64(evidence.SuccessSampleCount) / float64(evidence.SampleCount)
	}
	if useReal && real.TTFTSampleCount > 0 && real.TTFTP50MS != nil {
		evidence.TTFTSampleCount = real.TTFTSampleCount
		evidence.TTFTP50MS = real.TTFTP50MS
	} else if useProbe && !useReal {
		evidence.TTFTSampleCount = probe.TTFTSampleCount
		evidence.TTFTP50MS = probe.TTFTP50MS
	}
	if useReal && real.LatencySampleCount > 0 && real.LatencyP95MS != nil {
		evidence.LatencySampleCount = real.LatencySampleCount
		evidence.LatencyP95MS = real.LatencyP95MS
	} else if useProbe && !useReal {
		evidence.LatencySampleCount = probe.LatencySampleCount
		evidence.LatencyP95MS = probe.LatencyP95MS
	}
	if useReal && real.OutputRateSampleCount > 0 && validAccountMonitorOutputRate(real.OutputRateTokensPerSecond) {
		evidence.OutputRateTokensPerSecond = real.OutputRateTokensPerSecond
		evidence.OutputRateSampleCount = real.OutputRateSampleCount
	}
	return evidence
}

func accountMonitorUnknownQualityEvidence(reason string) AccountMonitorQualityEvidence {
	return accountMonitorUnknownQualityEvidenceWithFreshness(reason, false)
}

func accountMonitorUnknownQualityEvidenceWithFreshness(reason string, stale bool) AccountMonitorQualityEvidence {
	freshness := accountMonitorQualityFreshnessUnknown
	if stale {
		freshness = accountMonitorQualityFreshnessStale
	}
	return AccountMonitorQualityEvidence{Source: accountMonitorQualitySourceUnknown, Freshness: freshness, UnknownReason: reason}
}

func isAccountMonitorEvidenceFresh(observedAt, now time.Time, ttl time.Duration) bool {
	return !observedAt.IsZero() && !observedAt.After(now) && now.Sub(observedAt) <= ttl
}

func latestEvidenceTime(values ...time.Time) time.Time {
	latest := time.Time{}
	for _, value := range values {
		if value.After(latest) {
			latest = value.UTC()
		}
	}
	return latest
}

func clampNonNegativeInt64(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func clampNonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func clampCount(value, max int64) int64 {
	if value < 0 {
		return 0
	}
	if value > max {
		return max
	}
	return value
}

func maxInt64(value, fallback int64) int64 {
	if value > fallback {
		return value
	}
	return fallback
}

func maxNonZeroInt(value, fallback int) int {
	if value > fallback {
		return value
	}
	return fallback
}
