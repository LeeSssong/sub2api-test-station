package service

import "sync/atomic"

const (
	OpenAIEventStreamUpstreamFailure            = "openai.stream_upstream_failure"
	OpenAIEventAccountModelSoftFailure          = "openai.account_model_soft_failure"
	OpenAIEventAccountModelCooldownStarted      = "openai.account_model_cooldown_started"
	OpenAIEventAccountModelCooldownSkippedCache = "openai.account_model_cooldown_skipped_for_cache"
	OpenAIEventFailoverAfterStreamFailure       = "openai.failover_after_stream_failure"
	OpenAIEventAccountModelHalfOpenProbe        = "openai.account_model_half_open_probe"
	OpenAIEventRetryBillingReconciled           = "openai.retry_billing_reconciled"
)

// OpenAIResilienceAlertCounters are process-local inputs for existing Ops
// alert rules. They count structured recovery events, not customer requests.
type OpenAIResilienceAlertCounters struct {
	RepeatedAccountModelFailures int64
	CooldownSaturation           int64
	StreamFailoverDegradation    int64
	PostFailureSelection         int64
	CacheHitFailoverDecline      int64
}

var openAIResilienceCounters struct {
	repeatedFailures atomic.Int64
	cooldown         atomic.Int64
	streamFailover   atomic.Int64
	postFailure      atomic.Int64
	cacheDecline     atomic.Int64
}

func RecordOpenAIResilienceEvent(name string, failureStreak int, _ string) {
	switch name {
	case OpenAIEventAccountModelSoftFailure:
		if failureStreak >= 2 {
			openAIResilienceCounters.repeatedFailures.Add(1)
		}
	case OpenAIEventAccountModelCooldownStarted:
		openAIResilienceCounters.cooldown.Add(1)
	case OpenAIEventStreamUpstreamFailure, OpenAIEventFailoverAfterStreamFailure:
		openAIResilienceCounters.streamFailover.Add(1)
	case OpenAIEventAccountModelHalfOpenProbe:
		openAIResilienceCounters.postFailure.Add(1)
	case OpenAIEventAccountModelCooldownSkippedCache:
		openAIResilienceCounters.cacheDecline.Add(1)
	}
}

func SnapshotOpenAIResilienceAlertCounters() OpenAIResilienceAlertCounters {
	return OpenAIResilienceAlertCounters{
		RepeatedAccountModelFailures: openAIResilienceCounters.repeatedFailures.Load(),
		CooldownSaturation:           openAIResilienceCounters.cooldown.Load(),
		StreamFailoverDegradation:    openAIResilienceCounters.streamFailover.Load(),
		PostFailureSelection:         openAIResilienceCounters.postFailure.Load(),
		CacheHitFailoverDecline:      openAIResilienceCounters.cacheDecline.Load(),
	}
}
