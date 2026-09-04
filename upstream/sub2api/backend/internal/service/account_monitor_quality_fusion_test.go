package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFuseAccountMonitorQualityEvidenceKeepsWindowStatsWhenObservationIsStale(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	observed := now.Add(-30 * time.Minute)
	evidence := fuseAccountMonitorQualityEvidence(AccountMonitorWindowAggregate{
		RequestCount: 63, SuccessCount: 63, TTFTSampleCount: 63,
		TTFTP50MS: ptrFloat(14558), LastObservedAt: &observed,
	}, AccountMonitorAggregate{}, AccountMonitorLatest{}, AccountMonitorSettings{IntervalSeconds: 300}, now)

	require.True(t, evidence.Known)
	require.Equal(t, accountMonitorQualityFreshnessStale, evidence.Freshness)
	require.Equal(t, 63, evidence.SampleCount)
	require.Equal(t, 63, evidence.SuccessSampleCount)
	require.Equal(t, 1.0, evidence.SuccessRate)
}

func ptrFloat(value float64) *float64 { return &value }
