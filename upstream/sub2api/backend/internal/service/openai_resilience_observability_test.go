package service

import (
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
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, Name: OpenAIEventAccountModelSoftFailure, FailureStreak: 2, Outcome: "failure"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, Name: OpenAIEventAccountModelCooldownStarted, Outcome: "failure"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, Name: OpenAIEventStreamUpstreamFailure, Outcome: "failure"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, Name: OpenAIEventAccountModelPostFailureSelected, Outcome: "selected"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, Name: OpenAIEventAccountModelCooldownSkippedCache, CacheMode: "failover_after_failure", Outcome: "cache_hit"},
	} {
		RecordOpenAIResilienceOutcome(event)
	}
	counters := openAIResilienceCountersForWindow(at, at, PlatformOpenAI, &groupID)
	require.Equal(t, int64(1), counters.RepeatedAccountModelFailures)
	require.Equal(t, int64(100), counters.CooldownSaturation)
	require.Equal(t, int64(100), counters.StreamFailoverDegradation)
	require.Equal(t, int64(100), counters.PostFailureSelection)
	require.Equal(t, int64(100), counters.CacheHitFailoverDecline)
}

func TestRecordOpenAIAccountModelFailureRecordsDimensionedOutcome(t *testing.T) {
	at := time.Date(2032, 1, 1, 0, 0, 0, 0, time.UTC)
	groupID := int64(703)
	svc := &OpenAIGatewayService{}
	svc.RecordOpenAIAccountModelFailure(nil, OpenAIAccountModelFailureEvent{
		AccountID: 91, CanonicalModel: "gpt-5.5", StatusCode: 502, ErrorType: "transient_upstream",
		Platform: PlatformOpenAI, GroupID: &groupID, CacheMode: "failover_after_failure", Now: at,
	})

	counters := openAIResilienceCountersForWindow(at, at, PlatformOpenAI, &groupID)
	require.Equal(t, int64(0), counters.RepeatedAccountModelFailures)
	require.Equal(t, int64(0), counters.PostFailureSelection)
	require.Equal(t, int64(0), counters.CooldownSaturation)
}

func TestOpenAIResilienceAlertCountersHonorWindowDimensionsAndCorrelation(t *testing.T) {
	at := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)
	groupID := int64(701)
	otherGroupID := int64(702)
	for _, event := range []OpenAIResilienceEvent{
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, Name: OpenAIEventAccountModelSoftFailure, FailureStreak: 1},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, Name: OpenAIEventAccountModelSoftFailure, FailureStreak: 2},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, Name: OpenAIEventAccountModelCooldownStarted},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, Name: OpenAIEventStreamUpstreamFailure},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, Name: OpenAIEventFailoverAfterStreamFailure, Outcome: "success"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, Name: OpenAIEventAccountModelPostFailureSelected, Outcome: "selected"},
		{At: at, Platform: PlatformOpenAI, GroupID: &groupID, Name: OpenAIEventAccountModelCooldownSkippedCache, Outcome: "cache_hit", CacheMode: "failover_after_failure"},
		{At: at, Platform: PlatformOpenAI, GroupID: &otherGroupID, Name: OpenAIEventAccountModelCooldownStarted},
	} {
		RecordOpenAIResilienceOutcome(event)
	}

	counters := openAIResilienceCountersForWindow(at.Add(-time.Second), at.Add(time.Second), PlatformOpenAI, &groupID)
	require.Equal(t, int64(50), counters.CooldownSaturation)
	require.Equal(t, int64(0), counters.StreamFailoverDegradation)
	require.Equal(t, int64(50), counters.PostFailureSelection)
	require.Equal(t, int64(50), counters.CacheHitFailoverDecline)
	require.Equal(t, int64(1), counters.RepeatedAccountModelFailures)
	require.Equal(t, OpenAIResilienceAlertCounters{}, openAIResilienceCountersForWindow(at.Add(2*time.Second), at.Add(3*time.Second), PlatformOpenAI, &groupID))
}
