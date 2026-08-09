package service

import (
	"testing"

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

	before := SnapshotOpenAIResilienceAlertCounters()
	RecordOpenAIResilienceEvent(OpenAIEventAccountModelSoftFailure, 2, "failover_after_failure")
	RecordOpenAIResilienceEvent(OpenAIEventAccountModelCooldownStarted, 2, "failover_after_failure")
	RecordOpenAIResilienceEvent(OpenAIEventStreamUpstreamFailure, 0, "failover_after_failure")
	RecordOpenAIResilienceEvent(OpenAIEventAccountModelHalfOpenProbe, 0, "half_open_probe")
	RecordOpenAIResilienceEvent(OpenAIEventAccountModelCooldownSkippedCache, 0, "sticky")
	after := SnapshotOpenAIResilienceAlertCounters()
	require.Equal(t, before.RepeatedAccountModelFailures+1, after.RepeatedAccountModelFailures)
	require.Equal(t, before.CooldownSaturation+1, after.CooldownSaturation)
	require.Equal(t, before.StreamFailoverDegradation+1, after.StreamFailoverDegradation)
	require.Equal(t, before.PostFailureSelection+1, after.PostFailureSelection)
	require.Equal(t, before.CacheHitFailoverDecline+1, after.CacheHitFailoverDecline)
}
