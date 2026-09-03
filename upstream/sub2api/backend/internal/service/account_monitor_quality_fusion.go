package service

import "time"

const (
	accountMonitorQualityFreshnessFresh   = "fresh"
	accountMonitorQualityFreshnessStale   = "stale"
	accountMonitorQualityFreshnessUnknown = "unknown"
	accountMonitorQualitySourceUnified    = "unified"
	accountMonitorQualitySourceUnknown    = "unknown"
)

// fuseAccountMonitorQualityEvidence projects the repository-selected request
// stream. Repository selection already applies the five-minute real/probe
// fallback, so service code must not add probe aggregates a second time.
func fuseAccountMonitorQualityEvidence(window AccountMonitorWindowAggregate, _ AccountMonitorAggregate, _ AccountMonitorLatest, settings AccountMonitorSettings, now time.Time) AccountMonitorQualityEvidence {
	now = now.UTC()
	samples := clampNonNegativeInt64(window.RequestCount)
	successes := clampCount(window.SuccessCount, samples)
	observedAt := accountMonitorWindowObservedAt(window)
	ttl := time.Duration(settings.IntervalSeconds*2) * time.Second
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	// Legacy window aggregate adapters do not always carry MAX(created_at).
	// Their rows are still scoped to the requested current window, but the
	// absence of a timestamp must not be exposed as an invented observation.
	if samples == 0 {
		return accountMonitorUnknownQualityEvidenceWithFreshness("missing", false)
	}
	if observedAt.IsZero() || !isAccountMonitorEvidenceFresh(observedAt, now, ttl) {
		return accountMonitorUnknownQualityEvidenceWithFreshness("stale", true)
	}
	evidence := AccountMonitorQualityEvidence{
		Source: accountMonitorQualitySourceUnified, Known: true,
		Freshness: accountMonitorQualityFreshnessFresh, ObservedAt: observedAt,
		SampleCount: int(samples), SuccessSampleCount: int(successes),
		SuccessRate: float64(successes) / float64(samples),
	}
	if window.TTFTSampleCount > 0 && window.TTFTP50MS != nil {
		evidence.TTFTSampleCount = window.TTFTSampleCount
		evidence.TTFTP50MS = window.TTFTP50MS
	}
	if window.LatencySampleCount > 0 && window.LatencyP95MS != nil {
		evidence.LatencySampleCount = window.LatencySampleCount
		evidence.LatencyP95MS = window.LatencyP95MS
	}
	if window.OutputRateSampleCount > 0 && validAccountMonitorOutputRate(window.OutputRateTokensPerSecond) {
		evidence.OutputRateTokensPerSecond = window.OutputRateTokensPerSecond
		evidence.OutputRateSampleCount = window.OutputRateSampleCount
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
