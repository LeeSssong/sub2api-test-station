package service

import (
	"context"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestAggregateOpenAISchedulerExperienceGoldenSample(t *testing.T) {
	start := time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	events := []OpenAIResilienceEvent{
		{At: start.Add(time.Minute), CorrelationID: "r1", Name: OpenAIEventSchedulerSelection, AccountID: 1, CanonicalModel: "gpt", EligibleCount: 3, EffectiveTopK: 2, StickyKept: true},
		{At: start.Add(2 * time.Minute), CorrelationID: "r1", Name: OpenAIEventSchedulerRequestOutcome, FinalOutcome: "success"},
		{At: start.Add(3 * time.Minute), CorrelationID: "r2", Name: OpenAIEventSchedulerSelection, AccountID: 2, CanonicalModel: "gpt", EligibleCount: 3, EffectiveTopK: 1, StickyEscapeReason: "ttft", TTFTReportEligible: true},
		{At: start.Add(4 * time.Minute), CorrelationID: "r2", Name: OpenAIEventAccountModelSoftFailure, AccountID: 2, CanonicalModel: "gpt", Outcome: "failure"},
		{At: start.Add(5 * time.Minute), CorrelationID: "r2", Name: OpenAIEventSchedulerSelection, AccountID: 3, CanonicalModel: "gpt", EligibleCount: 2, EffectiveTopK: 1},
		{At: start.Add(6 * time.Minute), CorrelationID: "r2", Name: OpenAIEventSchedulerRequestOutcome, FinalOutcome: "success"},
		{At: start.Add(7 * time.Minute), CorrelationID: "r3", Name: OpenAIEventSchedulerSelection, AccountID: 4, CanonicalModel: "gpt", EligibleCount: 2, EffectiveTopK: 2},
		{At: start.Add(8 * time.Minute), CorrelationID: "r3", Name: OpenAIEventAccountModelSoftFailure, AccountID: 4, CanonicalModel: "gpt", Outcome: "failure"},
		{At: start.Add(9 * time.Minute), CorrelationID: "r3", Name: OpenAIEventSchedulerSelection, AccountID: 4, CanonicalModel: "gpt", SelectionLayer: "adaptive_top_k", EligibleCount: 2, EffectiveTopK: 1},
		{At: start.Add(10 * time.Minute), CorrelationID: "r3", Name: OpenAIEventSchedulerRequestOutcome, FinalOutcome: "failure", RetryBudgetExhausted: true},
		{At: start.Add(11 * time.Minute), CorrelationID: "r4", Name: OpenAIEventSchedulerSelection, AccountID: 5, CanonicalModel: "gpt", SelectionLayer: "half_open_probe", EligibleCount: 1, EffectiveTopK: 1},
		{At: start.Add(12 * time.Minute), CorrelationID: "r4", Name: OpenAIEventSchedulerRequestOutcome, FinalOutcome: "success"},
		{At: start.Add(13 * time.Minute), CorrelationID: "r5", Name: OpenAIEventSchedulerSelection, AccountID: 6, CanonicalModel: "gpt", EligibleCount: 2, EffectiveTopK: 2},
		{At: start.Add(14 * time.Minute), CorrelationID: "r5", Name: OpenAIEventSchedulerRequestOutcome, FinalOutcome: "success"},
	}

	got := aggregateOpenAISchedulerExperience(events, start, end)

	require.Equal(t, int64(5), got.SampleSize)
	require.Equal(t, int64(1), got.Metrics.AutoRecoveryRate.Numerator)
	require.Equal(t, int64(2), got.Metrics.AutoRecoveryRate.Denominator)
	require.Equal(t, OpsSchedulerMetricStatusInsufficientData, got.Metrics.AutoRecoveryRate.Status)
	require.InDelta(t, 1.4, *got.Metrics.AverageAttempts.Value, 0.000001)
	require.Equal(t, 2, *got.Metrics.AverageAttempts.P95)
	require.Equal(t, int64(1), got.Metrics.RepeatedBadAccountRate.Numerator)
	require.Equal(t, int64(2), got.Metrics.RepeatedBadAccountRate.Denominator)
	require.Equal(t, int64(1), got.Metrics.RetryBudgetExhaustedRate.Numerator)
	require.Equal(t, int64(2), got.Metrics.RetryBudgetExhaustedRate.Denominator)
	require.Equal(t, int64(1), got.Metrics.StickyKeptRate.Numerator)
	require.Equal(t, int64(2), got.Metrics.StickyKeptRate.Denominator)
	require.Equal(t, int64(1), got.Metrics.StickyEscapeRate.Numerator)
	require.Equal(t, int64(2), got.Metrics.StickyEscapeRate.Denominator)
	require.Equal(t, int64(5), got.Metrics.TopKFilteredRate.Numerator)
	require.Equal(t, int64(15), got.Metrics.TopKFilteredRate.Denominator)
	require.Equal(t, int64(1), got.Metrics.TTFTReportEligibleRate.Numerator)
	require.Equal(t, int64(7), got.Metrics.TTFTReportEligibleRate.Denominator)
	require.Equal(t, start.Add(14*time.Minute), *got.LatestEventAt)
}

func TestAggregateOpenAISchedulerExperienceNoData(t *testing.T) {
	start := time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC)
	got := aggregateOpenAISchedulerExperience(nil, start, start.Add(time.Hour))

	require.Zero(t, got.SampleSize)
	require.Equal(t, OpsSchedulerMetricStatusNoData, got.Metrics.AutoRecoveryRate.Status)
	require.Nil(t, got.Metrics.AutoRecoveryRate.Value)
	require.Nil(t, got.Metrics.AverageAttempts.Value)
}

func TestOpsServiceGetOpenAISchedulerExperienceFiltersRuntimeLedger(t *testing.T) {
	openAIResilienceEventLedger.Lock()
	previous := append([]OpenAIResilienceEvent(nil), openAIResilienceEventLedger.events...)
	openAIResilienceEventLedger.events = nil
	openAIResilienceEventLedger.Unlock()
	t.Cleanup(func() {
		openAIResilienceEventLedger.Lock()
		openAIResilienceEventLedger.events = previous
		openAIResilienceEventLedger.Unlock()
	})

	start := time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	groupID := int64(11)
	otherGroupID := int64(12)
	RecordOpenAIResilienceOutcome(OpenAIResilienceEvent{
		At: start.Add(time.Minute), Platform: PlatformOpenAI, GroupID: &groupID,
		CorrelationID: "included", Name: OpenAIEventSchedulerSelection, EligibleCount: 2, EffectiveTopK: 1,
	})
	RecordOpenAIResilienceOutcome(OpenAIResilienceEvent{
		At: start.Add(2 * time.Minute), Platform: PlatformOpenAI, GroupID: &groupID,
		CorrelationID: "included", Name: OpenAIEventSchedulerRequestOutcome, FinalOutcome: "success",
	})
	RecordOpenAIResilienceOutcome(OpenAIResilienceEvent{
		At: start.Add(3 * time.Minute), Platform: PlatformOpenAI, GroupID: &otherGroupID,
		CorrelationID: "wrong-group", Name: OpenAIEventSchedulerSelection,
	})
	RecordOpenAIResilienceOutcome(OpenAIResilienceEvent{
		At: start.Add(4 * time.Minute), Platform: PlatformAnthropic, GroupID: &groupID,
		CorrelationID: "wrong-platform", Name: OpenAIEventSchedulerSelection,
	})
	RecordOpenAIResilienceOutcome(OpenAIResilienceEvent{
		At: end.Add(time.Minute), Platform: PlatformOpenAI, GroupID: &groupID,
		CorrelationID: "outside-window", Name: OpenAIEventSchedulerSelection,
	})

	svc := &OpsService{}
	got, err := svc.GetOpenAISchedulerExperience(context.Background(), &OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  PlatformOpenAI,
		GroupID:   &groupID,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.SampleSize)
	require.Equal(t, int64(2), got.Metrics.TopKFilteredRate.Denominator)
	require.Equal(t, start.Add(2*time.Minute), *got.LatestEventAt)
}

func TestOpsServiceGetOpenAISchedulerExperienceValidatesFilter(t *testing.T) {
	now := time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC)
	invalidGroupID := int64(0)
	tests := []struct {
		name       string
		filter     *OpsDashboardFilter
		wantReason string
	}{
		{name: "filter required", wantReason: "OPS_FILTER_REQUIRED"},
		{name: "time required", filter: &OpsDashboardFilter{EndTime: now}, wantReason: "OPS_TIME_RANGE_REQUIRED"},
		{name: "time ordered", filter: &OpsDashboardFilter{StartTime: now, EndTime: now.Add(-time.Minute)}, wantReason: "OPS_TIME_RANGE_INVALID"},
		{name: "group positive", filter: &OpsDashboardFilter{StartTime: now.Add(-time.Hour), EndTime: now, GroupID: &invalidGroupID}, wantReason: "OPS_GROUP_ID_INVALID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&OpsService{}).GetOpenAISchedulerExperience(context.Background(), tt.filter)
			require.Error(t, err)
			require.Equal(t, tt.wantReason, infraerrors.Reason(err))
		})
	}
}
