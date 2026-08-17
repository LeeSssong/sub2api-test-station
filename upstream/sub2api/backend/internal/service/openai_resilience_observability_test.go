package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIResilienceEventContractAndAlertCounters(t *testing.T) {
	require.Equal(t, "openai.stream_upstream_failure", OpenAIEventStreamUpstreamFailure)
	require.Equal(t, "openai.account_model_soft_failure", OpenAIEventAccountModelSoftFailure)
	require.Equal(t, "openai.account_model_cooldown_started", OpenAIEventAccountModelCooldownStarted)
	require.Equal(t, "openai.account_model_cooldown_skipped_for_cache", OpenAIEventAccountModelCooldownSkippedCache)
	require.Equal(t, "openai.failover_after_stream_failure", OpenAIEventFailoverAfterStreamFailure)
	require.Equal(t, "openai.account_model_half_open_probe", OpenAIEventAccountModelHalfOpenProbe)
	require.Equal(t, "openai.retry_billing_reconciled", OpenAIEventRetryBillingReconciled)

	at := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	groupID := int64(700)
	for _, event := range []OpenAIResilienceEvent{
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-1", Name: OpenAIEventAccountModelSoftFailure, FailureStreak: 2, Outcome: "failure"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-1", Name: OpenAIEventAccountModelCooldownStarted, Outcome: "failure"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-1", Name: OpenAIEventStreamUpstreamFailure, Outcome: "failure"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-1", Name: OpenAIEventAccountModelPostFailureSelected, Outcome: "selected"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-1", Name: OpenAIEventAccountModelCooldownSkippedCache, CacheMode: "failover_after_failure", Outcome: "cache_hit"},
	} {
		RecordOpenAIResilienceOutcome(event)
	}
	counters := openAIResilienceCountersForWindow(at, at, PlatformOpenAI, &groupID)
	require.Equal(t, int64(1), counters.RepeatedAccountModelFailures)
	require.Equal(t, int64(1), counters.CooldownSaturation)
	require.Equal(t, int64(1), counters.StreamFailoverDegradation)
	require.Equal(t, int64(1), counters.PostFailureSelection)
	require.Equal(t, int64(1), counters.CacheHitFailoverDecline)
}

func TestRecordOpenAIRetryBillingReconciledEmitsCompletedAttemptEvent(t *testing.T) {
	now := time.Now().UTC()
	groupID := int64(799)
	ctx := WithOpenAIRequestAttemptMetadata(context.Background(), OpenAIRequestAttemptMetadata{
		LogicalRequestID: "request-reconciled", AttemptID: "request-reconciled:2", AttemptNumber: 2,
		AccountID: 92, CanonicalModel: "gpt-5.5", CachePreservationMode: "failover_after_failure",
	})

	RecordOpenAIRetryBillingReconciled(ctx, PlatformOpenAI, &groupID, 503, true, true, 10)

	events := openAIResilienceEventsForWindow(now.Add(-time.Second), now.Add(time.Second), PlatformOpenAI, &groupID)
	var got *OpenAIResilienceEvent
	for i := range events {
		if events[i].CorrelationID == "request-reconciled" {
			got = &events[i]
			break
		}
	}
	require.NotNil(t, got)
	require.Equal(t, OpenAIEventRetryBillingReconciled, got.Name)
	require.Equal(t, "success", got.Outcome)
	require.Equal(t, int64(92), got.AccountID)
	require.Equal(t, "gpt-5.5", got.CanonicalModel)
	require.Equal(t, "request-reconciled:2", got.AttemptID)
	require.Equal(t, 2, got.AttemptNumber)
	require.Equal(t, 503, got.StatusCode)
	require.True(t, got.OutputStarted)
	require.True(t, got.UsageProduced)
	require.Equal(t, 10, got.RetryAfterSeconds)
}

func TestRecordOpenAISchedulerSelectionAndOutcomeUseAttemptContext(t *testing.T) {
	now := time.Now().UTC()
	groupID := int64(812)
	ctx := WithOpenAIRequestAttemptMetadata(context.Background(), OpenAIRequestAttemptMetadata{
		LogicalRequestID: "scheduler-request-1", AttemptID: "scheduler-request-1:2", AttemptNumber: 2,
		AccountID: 93, CanonicalModel: "gpt-5.6-sol", CachePreservationMode: "failover_after_failure",
	})
	decision := OpenAIAccountScheduleDecision{
		SelectionLayer: "adaptive_top_k", CandidateCount: 7, EligibleCount: 5, EffectiveTopK: 3,
		MinimumScoreThreshold: 2.75, StickyKept: false, StickyEscapeReason: "quality_floor", TTFTReportEligible: true,
	}

	RecordOpenAISchedulerSelection(ctx, PlatformOpenAI, &groupID, decision)
	RecordOpenAISchedulerRequestOutcome(ctx, PlatformOpenAI, &groupID, "success", false)

	events := openAIResilienceEventsForWindow(now.Add(-time.Second), now.Add(time.Second), PlatformOpenAI, &groupID)
	var selection, outcome *OpenAIResilienceEvent
	for i := range events {
		if events[i].CorrelationID != "scheduler-request-1" {
			continue
		}
		switch events[i].Name {
		case OpenAIEventSchedulerSelection:
			selection = &events[i]
		case OpenAIEventSchedulerRequestOutcome:
			outcome = &events[i]
		}
	}
	require.NotNil(t, selection)
	require.Equal(t, int64(93), selection.AccountID)
	require.Equal(t, "gpt-5.6-sol", selection.CanonicalModel)
	require.Equal(t, "adaptive_top_k", selection.SelectionLayer)
	require.Equal(t, 7, selection.CandidateCount)
	require.Equal(t, 5, selection.EligibleCount)
	require.Equal(t, 3, selection.EffectiveTopK)
	require.InDelta(t, 2.75, selection.MinimumScoreThreshold, 0.000001)
	require.Equal(t, "quality_floor", selection.StickyEscapeReason)
	require.True(t, selection.TTFTReportEligible)
	require.NotNil(t, outcome)
	require.Equal(t, "success", outcome.FinalOutcome)
	require.False(t, outcome.RetryBudgetExhausted)
}

func TestRecordOpenAIAccountModelFailureRecordsDimensionedOutcome(t *testing.T) {
	at := time.Date(2032, 1, 1, 0, 0, 0, 0, time.UTC)
	groupID := int64(703)
	svc := &OpenAIGatewayService{}
	ctx := WithOpenAIRequestAttemptMetadata(nil, OpenAIRequestAttemptMetadata{
		LogicalRequestID: "request-schema", AttemptID: "request-schema:2", AttemptNumber: 2,
		AccountID: 91, CanonicalModel: "gpt-5.5", CachePreservationMode: "failover_after_failure",
	})
	svc.RecordOpenAIAccountModelFailure(ctx, OpenAIAccountModelFailureEvent{
		AccountID: 91, CanonicalModel: "gpt-5.5", StatusCode: 502, ErrorType: "transient_upstream",
		OutputStarted: true, UsageKnown: true, Platform: PlatformOpenAI, GroupID: &groupID,
		CacheMode: "failover_after_failure", Now: at,
	})

	counters := openAIResilienceCountersForWindow(at, at, PlatformOpenAI, &groupID)
	require.Equal(t, int64(0), counters.RepeatedAccountModelFailures)
	require.Equal(t, int64(0), counters.PostFailureSelection)
	require.Equal(t, int64(0), counters.CooldownSaturation)

	events := openAIResilienceEventsForWindow(at, at, PlatformOpenAI, &groupID)
	require.Len(t, events, 1)
	require.Equal(t, OpenAIResilienceEvent{
		At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-schema",
		Name: OpenAIEventAccountModelSoftFailure, AccountID: 91, CanonicalModel: "gpt-5.5",
		AttemptID: "request-schema:2", AttemptNumber: 2, StatusCode: 502, OutputStarted: true,
		UsageProduced: true, FailureStreak: 1, CacheMode: "failover_after_failure", Outcome: "failure",
	}, events[0])
}

func TestOpenAIResilienceAlertCountersHonorWindowDimensionsAndCorrelation(t *testing.T) {
	at := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)
	groupID := int64(701)
	otherGroupID := int64(702)
	for _, event := range []OpenAIResilienceEvent{
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-a", Name: OpenAIEventAccountModelSoftFailure, FailureStreak: 1},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-b", Name: OpenAIEventAccountModelSoftFailure, FailureStreak: 2},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-b", Name: OpenAIEventAccountModelCooldownStarted},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-c", Name: OpenAIEventStreamUpstreamFailure},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-c", Name: OpenAIEventFailoverAfterStreamFailure, Outcome: "success"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-d", Name: OpenAIEventAccountModelSoftFailure, FailureStreak: 1},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-d", Name: OpenAIEventAccountModelPostFailureSelected, Outcome: "selected"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "request-d", Name: OpenAIEventAccountModelCooldownSkippedCache, Outcome: "cache_hit", CacheMode: "failover_after_failure"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "unrelated", Name: OpenAIEventAccountModelCooldownSkippedCache, Outcome: "cache_hit", CacheMode: "failover_after_failure"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, CorrelationID: "unrelated", Name: OpenAIEventAccountModelPostFailureSelected, Outcome: "selected"},
		{At: at, Platform: PlatformOpenAI, GroupID: &otherGroupID, Name: OpenAIEventAccountModelCooldownStarted},
	} {
		RecordOpenAIResilienceOutcome(event)
	}

	counters := openAIResilienceCountersForWindow(at.Add(-time.Second), at.Add(time.Second), PlatformOpenAI, &groupID)
	require.Equal(t, int64(1), counters.CooldownSaturation)
	require.Equal(t, int64(0), counters.StreamFailoverDegradation)
	require.Equal(t, int64(1), counters.PostFailureSelection)
	require.Equal(t, int64(1), counters.CacheHitFailoverDecline, "an unrelated cache skip must not be counted as a failover decline")
	require.Equal(t, int64(1), counters.RepeatedAccountModelFailures)
	require.Equal(t, OpenAIResilienceAlertCounters{}, openAIResilienceCountersForWindow(at.Add(2*time.Second), at.Add(3*time.Second), PlatformOpenAI, &groupID))
}
