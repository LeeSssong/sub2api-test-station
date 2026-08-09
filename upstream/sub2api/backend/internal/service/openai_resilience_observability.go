package service

import (
	"strings"
	"sync"
	"time"
)

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

// OpenAIResilienceEvent is a bounded process-local event ledger used by Ops
// evaluations. Unlike lifetime counters, callers can scope it by time and
// scheduling dimensions.
type OpenAIResilienceEvent struct {
	At            time.Time
	Platform      string
	GroupID       *int64
	Name          string
	FailureStreak int
	CacheMode     string
	Outcome       string // success, failure, selected, cache_hit
}

var openAIResilienceEventLedger struct {
	sync.Mutex
	events []OpenAIResilienceEvent
}

const openAIResilienceEventLedgerMax = 4096

func RecordOpenAIResilienceOutcome(event OpenAIResilienceEvent) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	event.Platform = strings.TrimSpace(event.Platform)
	event.Name = strings.TrimSpace(event.Name)
	event.CacheMode = strings.TrimSpace(event.CacheMode)
	event.Outcome = strings.TrimSpace(event.Outcome)
	if event.Name == "" {
		return
	}
	openAIResilienceEventLedger.Lock()
	defer openAIResilienceEventLedger.Unlock()
	openAIResilienceEventLedger.events = append(openAIResilienceEventLedger.events, event)
	if len(openAIResilienceEventLedger.events) > openAIResilienceEventLedgerMax {
		openAIResilienceEventLedger.events = append([]OpenAIResilienceEvent(nil), openAIResilienceEventLedger.events[len(openAIResilienceEventLedger.events)-openAIResilienceEventLedgerMax:]...)
	}
}

func RecordOpenAIResilienceEvent(name string, failureStreak int, _ string) {
	RecordOpenAIResilienceOutcome(OpenAIResilienceEvent{Name: name, FailureStreak: failureStreak})
}

func SnapshotOpenAIResilienceAlertCounters() OpenAIResilienceAlertCounters {
	return openAIResilienceCountersForWindow(time.Time{}, time.Time{}, "", nil)
}

func openAIResilienceCountersForWindow(start, end time.Time, platform string, groupID *int64) OpenAIResilienceAlertCounters {
	openAIResilienceEventLedger.Lock()
	defer openAIResilienceEventLedger.Unlock()
	var failures, cooldowns, streams, failoverSuccess, reselections, cacheFailovers int64
	for _, event := range openAIResilienceEventLedger.events {
		if !start.IsZero() && event.At.Before(start) || !end.IsZero() && event.At.After(end) {
			continue
		}
		if platform != "" && event.Platform != platform {
			continue
		}
		if groupID != nil && (event.GroupID == nil || *event.GroupID != *groupID) {
			continue
		}
		switch event.Name {
		case OpenAIEventAccountModelSoftFailure:
			failures++
			if event.FailureStreak >= 2 {
				reselections++
			}
		case OpenAIEventAccountModelCooldownStarted:
			cooldowns++
		case OpenAIEventStreamUpstreamFailure:
			streams++
		case OpenAIEventFailoverAfterStreamFailure:
			if event.Outcome == "success" {
				failoverSuccess++
			}
			if event.Outcome == "selected" {
				reselections++
			}
		case OpenAIEventAccountModelCooldownSkippedCache:
			if event.Outcome == "cache_hit" || event.CacheMode == "failover_after_failure" {
				cacheFailovers++
			}
		}
	}
	denom := failures
	if denom == 0 {
		denom = 1
	}
	streamDenom := streams
	if streamDenom == 0 {
		streamDenom = 1
	}
	return OpenAIResilienceAlertCounters{
		RepeatedAccountModelFailures: reselections,
		CooldownSaturation:           cooldowns * 100 / denom,
		StreamFailoverDegradation:    (streams - failoverSuccess) * 100 / streamDenom,
		PostFailureSelection:         reselections * 100 / denom,
		CacheHitFailoverDecline:      cacheFailovers * 100 / denom,
	}
}
