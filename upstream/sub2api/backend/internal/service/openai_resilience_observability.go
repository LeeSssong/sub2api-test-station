package service

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	OpenAIEventStreamUpstreamFailure            = "openai.stream_upstream_failure"
	OpenAIEventAccountModelSoftFailure          = "openai.account_model_soft_failure"
	OpenAIEventAccountModelCooldownStarted      = "openai.account_model_cooldown_started"
	OpenAIEventAccountModelCooldownSkippedCache = "openai.account_model_cooldown_skipped_for_cache"
	OpenAIEventAccountModelPostFailureSelected  = "openai.account_model_post_failure_selected"
	OpenAIEventFailoverAfterStreamFailure       = "openai.failover_after_stream_failure"
	OpenAIEventAccountModelHalfOpenProbe        = "openai.account_model_half_open_probe"
	OpenAIEventRetryBillingReconciled           = "openai.retry_billing_reconciled"
	OpenAIEventSchedulerSelection               = "openai.scheduler_selection"
	OpenAIEventSchedulerRequestOutcome          = "openai.scheduler_request_outcome"
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
	At                    time.Time
	Platform              string
	GroupID               *int64
	CorrelationID         string
	Name                  string
	AccountID             int64
	CanonicalModel        string
	AttemptID             string
	AttemptNumber         int
	StatusCode            int
	OutputStarted         bool
	UsageProduced         bool
	FailureStreak         int
	CacheMode             string
	CooldownSeconds       int
	RetryAfterSeconds     int
	Outcome               string // success, failure, selected, cache_hit, started
	SelectionLayer        string
	CandidateCount        int
	EligibleCount         int
	EffectiveTopK         int
	MinimumScoreThreshold float64
	StickyKept            bool
	StickyEscapeReason    string
	TTFTReportEligible    bool
	RetryBudgetExhausted  bool
	FinalOutcome          string
}

type openAIResilienceCacheModeContextKey struct{}
type openAIResilienceCorrelationIDContextKey struct{}

func WithOpenAIResilienceCacheMode(ctx context.Context, mode string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIResilienceCacheModeContextKey{}, strings.TrimSpace(mode))
}

func openAIResilienceCacheModeFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	mode, _ := ctx.Value(openAIResilienceCacheModeContextKey{}).(string)
	return strings.TrimSpace(mode)
}

func WithOpenAIResilienceCorrelationID(ctx context.Context, correlationID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIResilienceCorrelationIDContextKey{}, strings.TrimSpace(correlationID))
}

func openAIResilienceCorrelationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	correlationID, _ := ctx.Value(openAIResilienceCorrelationIDContextKey{}).(string)
	return strings.TrimSpace(correlationID)
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
	event.CorrelationID = strings.TrimSpace(event.CorrelationID)
	event.Name = strings.TrimSpace(event.Name)
	event.CanonicalModel = normalizeOpenAIAccountModelTransientModel(event.CanonicalModel)
	event.AttemptID = strings.TrimSpace(event.AttemptID)
	event.CacheMode = strings.TrimSpace(event.CacheMode)
	event.Outcome = strings.TrimSpace(event.Outcome)
	event.SelectionLayer = strings.TrimSpace(event.SelectionLayer)
	event.StickyEscapeReason = strings.TrimSpace(event.StickyEscapeReason)
	event.FinalOutcome = strings.TrimSpace(event.FinalOutcome)
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

func RecordOpenAISchedulerSelection(ctx context.Context, platform string, groupID *int64, decision OpenAIAccountScheduleDecision) {
	RecordOpenAIResilienceOutcomeWithContext(ctx, OpenAIResilienceEvent{
		Platform: platform, GroupID: groupID, Name: OpenAIEventSchedulerSelection, Outcome: "selected",
		SelectionLayer: decision.SelectionLayer, CandidateCount: decision.CandidateCount,
		EligibleCount: decision.EligibleCount, EffectiveTopK: decision.EffectiveTopK,
		MinimumScoreThreshold: decision.MinimumScoreThreshold, StickyKept: decision.StickyKept,
		StickyEscapeReason: decision.StickyEscapeReason, TTFTReportEligible: decision.TTFTReportEligible,
	})
}

func RecordOpenAISchedulerRequestOutcome(ctx context.Context, platform string, groupID *int64, outcome string, retryBudgetExhausted bool) {
	finalOutcome := strings.TrimSpace(outcome)
	if finalOutcome == "" {
		finalOutcome = "failure"
	}
	RecordOpenAIResilienceOutcomeWithContext(ctx, OpenAIResilienceEvent{
		Platform: platform, GroupID: groupID, Name: OpenAIEventSchedulerRequestOutcome,
		Outcome: finalOutcome, FinalOutcome: finalOutcome, RetryBudgetExhausted: retryBudgetExhausted,
	})
}

// RecordOpenAIResilienceOutcomeWithContext is the common request-path emitter.
// It fills immutable attempt and correlation fields without making every
// transition duplicate the same context plumbing.
func RecordOpenAIResilienceOutcomeWithContext(ctx context.Context, event OpenAIResilienceEvent) {
	if metadata, ok := OpenAIRequestAttemptMetadataFromContext(ctx); ok {
		if event.CorrelationID == "" {
			event.CorrelationID = metadata.LogicalRequestID
		}
		if event.AccountID == 0 {
			event.AccountID = metadata.AccountID
		}
		if event.CanonicalModel == "" {
			event.CanonicalModel = metadata.CanonicalModel
		}
		if event.AttemptID == "" {
			event.AttemptID = metadata.AttemptID
		}
		if event.AttemptNumber == 0 {
			event.AttemptNumber = metadata.AttemptNumber
		}
		if event.CacheMode == "" {
			event.CacheMode = metadata.CachePreservationMode
		}
	}
	if event.CorrelationID == "" {
		event.CorrelationID = openAIResilienceCorrelationIDFromContext(ctx)
	}
	RecordOpenAIResilienceOutcome(event)
}

// RecordOpenAIFailoverAfterStreamFailure records a completed post-output
// account switch. Callers invoke it only after the replacement attempt has
// succeeded, never when returning a terminal recovery envelope.
func RecordOpenAIFailoverAfterStreamFailure(ctx context.Context, platform string, groupID *int64, statusCode int, usageProduced bool, retryAfterSeconds int) {
	RecordOpenAIResilienceOutcomeWithContext(ctx, OpenAIResilienceEvent{
		Platform: platform, GroupID: groupID, Name: OpenAIEventFailoverAfterStreamFailure,
		StatusCode: statusCode, OutputStarted: true, UsageProduced: usageProduced,
		RetryAfterSeconds: retryAfterSeconds, Outcome: "success",
	})
}

// RecordOpenAIRetryBillingReconciled records a successful, durable billing
// completion for a retry whose earlier attempt exposed partial usage. It must
// be called after RecordUsage returns nil, not when reconciliation is queued.
func RecordOpenAIRetryBillingReconciled(ctx context.Context, platform string, groupID *int64, statusCode int, outputStarted, usageProduced bool, retryAfterSeconds int) {
	RecordOpenAIResilienceOutcomeWithContext(ctx, OpenAIResilienceEvent{
		Platform: platform, GroupID: groupID, Name: OpenAIEventRetryBillingReconciled,
		StatusCode: statusCode, OutputStarted: outputStarted, UsageProduced: usageProduced,
		RetryAfterSeconds: retryAfterSeconds, Outcome: "success",
	})
}

func RecordOpenAIResilienceEvent(name string, failureStreak int, cacheMode string) {
	RecordOpenAIResilienceOutcome(OpenAIResilienceEvent{Name: name, FailureStreak: failureStreak, CacheMode: cacheMode})
}

func SnapshotOpenAIResilienceAlertCounters() OpenAIResilienceAlertCounters {
	return openAIResilienceCountersForWindow(time.Time{}, time.Time{}, "", nil)
}

func openAIResilienceEventsForWindow(start, end time.Time, platform string, groupID *int64) []OpenAIResilienceEvent {
	openAIResilienceEventLedger.Lock()
	defer openAIResilienceEventLedger.Unlock()
	events := make([]OpenAIResilienceEvent, 0)
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
		events = append(events, event)
	}
	return events
}

// openAIResilienceCorrelationKey keeps alert joins inside the exact
// platform/group/request dimension. Logical request IDs are client supplied,
// so a bare ID cannot safely correlate traffic from different tenants/groups.
func openAIResilienceCorrelationKey(event OpenAIResilienceEvent) string {
	correlationID := strings.TrimSpace(event.CorrelationID)
	if correlationID == "" {
		return ""
	}
	groupID := "none"
	if event.GroupID != nil {
		groupID = strconv.FormatInt(*event.GroupID, 10)
	}
	return strings.TrimSpace(event.Platform) + "\x00" + groupID + "\x00" + correlationID
}

func openAIResilienceCountersForWindow(start, end time.Time, platform string, groupID *int64) OpenAIResilienceAlertCounters {
	events := openAIResilienceEventsForWindow(start, end, platform, groupID)
	var repeatedFailures, cooldowns, postFailureSelections, cacheFailovers int64
	streamFailures := make(map[string]int64)
	streamFailoverSuccess := make(map[string]int64)
	failureContexts := make(map[string]struct{})
	postFailureContexts := make(map[string]struct{})
	for _, event := range events {
		correlationKey := openAIResilienceCorrelationKey(event)
		switch event.Name {
		case OpenAIEventAccountModelSoftFailure:
			if event.FailureStreak >= 2 {
				repeatedFailures++
			}
			if correlationKey != "" {
				failureContexts[correlationKey] = struct{}{}
			}
		case OpenAIEventAccountModelCooldownStarted:
			if _, failed := failureContexts[correlationKey]; failed {
				cooldowns++
			}
		case OpenAIEventStreamUpstreamFailure:
			if correlationKey != "" {
				streamFailures[correlationKey]++
			}
		case OpenAIEventFailoverAfterStreamFailure:
			if event.Outcome == "success" && correlationKey != "" {
				streamFailoverSuccess[correlationKey]++
			}
		case OpenAIEventAccountModelPostFailureSelected:
			if event.Outcome == "selected" {
				if _, failed := failureContexts[correlationKey]; !failed {
					continue
				}
				postFailureSelections++
				if correlationKey != "" {
					postFailureContexts[correlationKey] = struct{}{}
				}
			}
		}
	}
	for _, event := range events {
		if event.Name != OpenAIEventAccountModelCooldownSkippedCache || event.Outcome != "cache_hit" {
			continue
		}
		if _, ok := postFailureContexts[openAIResilienceCorrelationKey(event)]; ok {
			cacheFailovers++
		}
	}
	var streamDegradation int64
	for correlationID, failures := range streamFailures {
		recovered := streamFailoverSuccess[correlationID]
		if recovered < failures {
			streamDegradation += failures - recovered
		}
	}
	return OpenAIResilienceAlertCounters{
		RepeatedAccountModelFailures: repeatedFailures,
		CooldownSaturation:           cooldowns,
		StreamFailoverDegradation:    streamDegradation,
		PostFailureSelection:         postFailureSelections,
		CacheHitFailoverDecline:      cacheFailovers,
	}
}
