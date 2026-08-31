package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRetryBudgetBoundsAttemptsAndAccountSwitches(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 4, MaxAccountSwitches: 2, MaxFailureDomains: 2, Total: 5 * time.Second}, func() time.Time { return now })

	require.True(t, budget.ConsumeAttempt(11))
	require.True(t, budget.ConsumeAttempt(11), "same-account retry does not consume an account switch")
	require.True(t, budget.ConsumeAttempt(12))
	require.True(t, budget.ConsumeAttempt(13))
	require.False(t, budget.ConsumeAttempt(14), "the third account switch is rejected")
}

func TestOpenAIRetryBudgetRejectsFourthAccountSwitch(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 8, MaxAccountSwitches: 2, MaxFailureDomains: 2, Total: 5 * time.Second}, func() time.Time { return now })

	for _, accountID := range []int64{11, 12, 13} {
		require.True(t, budget.ConsumeAttempt(accountID))
	}
	require.False(t, budget.CanSwitch(14, false, false))
	require.False(t, budget.ConsumeAttempt(14))
}

func TestOpenAIRetryBudgetCountsUnknownAsOneFailureDomain(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 4, MaxAccountSwitches: 3, MaxFailureDomains: 2, Total: 5 * time.Second}, func() time.Time { return now })

	require.True(t, budget.ObserveDomain([]service.OpenAIFailureDomain{{Type: service.OpenAIFailureDomainUnknown, ID: "unknown-a"}}))
	require.True(t, budget.ObserveDomain([]service.OpenAIFailureDomain{{Type: service.OpenAIFailureDomainUnknown, ID: "unknown-b"}}))
	require.True(t, budget.ObserveDomain([]service.OpenAIFailureDomain{{Type: service.OpenAIFailureDomainProviderChannel, ID: "openai:channel:7"}}))
	require.False(t, budget.ObserveDomain([]service.OpenAIFailureDomain{{Type: service.OpenAIFailureDomainQuotaPool, ID: "openai:quota_pool:pro"}}))
}

func TestOpenAIRetryBudgetStopsAtDeadlineAndUnsafeReplay(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 4, MaxAccountSwitches: 3, MaxFailureDomains: 2, Total: 5 * time.Second}, func() time.Time { return now })
	require.True(t, budget.ConsumeAttempt(11))
	require.False(t, budget.CanSwitch(12, true, false))
	require.False(t, budget.CanSwitch(12, false, true))

	now = now.Add(5 * time.Second)
	require.True(t, budget.DeadlineReached())
	require.Zero(t, budget.Remaining())
	require.False(t, budget.ConsumeAttempt(12))
}

func TestOpenAIRetryBudgetHonorsRetryAfterDeltaAndHTTPDate(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 4, MaxAccountSwitches: 3, MaxFailureDomains: 2, Total: 5 * time.Second, BackoffInitial: 120 * time.Millisecond, BackoffMax: 2 * time.Second}, func() time.Time { return now })

	delay, ok := budget.RetryDelay(&service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, ResponseHeaders: http.Header{"Retry-After": []string{"3"}}}, 1)
	require.True(t, ok)
	require.Equal(t, 3*time.Second, delay)

	delay, ok = budget.RetryDelay(&service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, ResponseHeaders: http.Header{"Retry-After": []string{now.Add(4 * time.Second).Format(http.TimeFormat)}}}, 1)
	require.True(t, ok)
	require.Equal(t, 4*time.Second, delay)

	_, ok = budget.RetryDelay(&service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, ResponseHeaders: http.Header{"Retry-After": []string{"6"}}}, 1)
	require.False(t, ok, "Retry-After beyond the total request budget must exhaust instead of sleeping past the deadline")
}

func TestOpenAIRetryBudgetBoundsExponentialBackoff(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 4, MaxAccountSwitches: 3, MaxFailureDomains: 2, Total: 5 * time.Second, BackoffInitial: 120 * time.Millisecond, BackoffMax: 2 * time.Second}, func() time.Time { return now })
	failure := &service.UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable}

	for attempt, base := range []time.Duration{120 * time.Millisecond, 240 * time.Millisecond, 480 * time.Millisecond, 960 * time.Millisecond, 1920 * time.Millisecond, 2 * time.Second} {
		delay, ok := budget.RetryDelay(failure, attempt+1)
		require.True(t, ok)
		require.GreaterOrEqual(t, delay, base)
		require.LessOrEqual(t, delay, minDuration(2*time.Second, base+base/5))
		repeated, repeatedOK := budget.RetryDelay(failure, attempt+1)
		require.True(t, repeatedOK)
		require.Equal(t, delay, repeated, "jitter must be stable for the same attempt")
	}
}

func TestOpenAIRetryBudgetSharedHealthDegradationAllowsOnlyOneRemainingSwitch(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 4, MaxAccountSwitches: 3, MaxFailureDomains: 2, Total: 5 * time.Second}, func() time.Time { return now })
	require.True(t, budget.ConsumeAttempt(11))

	budget.NarrowForSharedHealthDegraded()
	require.True(t, budget.ConsumeAttempt(12))
	require.False(t, budget.ConsumeAttempt(13))
	require.Equal(t, openAIRetryReasonAccountSwitchLimit, budget.Reason())
}

func TestOpenAIRetryBudgetExposesStableExhaustionReasons(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 1, MaxAccountSwitches: 3, MaxFailureDomains: 2, Total: 5 * time.Second}, func() time.Time { return now })
	require.True(t, budget.ConsumeAttempt(11))
	require.False(t, budget.ConsumeAttempt(11))
	require.Equal(t, openAIRetryReasonAttemptLimit, budget.Reason())

	unsafe := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 4, MaxAccountSwitches: 3, MaxFailureDomains: 2, Total: 5 * time.Second}, func() time.Time { return now })
	require.True(t, unsafe.ConsumeAttempt(11))
	require.False(t, unsafe.CanSwitch(12, true, false))
	require.Equal(t, openAIRetryReasonUnsafeToReplay, unsafe.Reason())

	domains := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 4, MaxAccountSwitches: 3, MaxFailureDomains: 1, Total: 5 * time.Second}, func() time.Time { return now })
	require.True(t, domains.ObserveDomain([]service.OpenAIFailureDomain{{Type: service.OpenAIFailureDomainProviderChannel, ID: "openai:channel:7"}}))
	require.False(t, domains.ObserveDomain([]service.OpenAIFailureDomain{{Type: service.OpenAIFailureDomainQuotaPool, ID: "openai:quota_pool:pro"}}))
	require.Equal(t, openAIRetryReasonFailureDomainLimit, domains.Reason())

	deadline := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 4, MaxAccountSwitches: 3, MaxFailureDomains: 2, Total: time.Second}, func() time.Time { return now })
	_, ok := deadline.RetryDelay(&service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, ResponseHeaders: http.Header{"Retry-After": []string{"2"}}}, 1)
	require.False(t, ok)
	require.Equal(t, openAIRetryReasonRetryDeadline, deadline.Reason())
}

func TestOpenAIRetryBudgetRetainsObservedFailureDomainsForSchedulerPreference(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 4, MaxAccountSwitches: 3, MaxFailureDomains: 2, Total: 5 * time.Second}, func() time.Time { return now })
	account := &service.Account{ID: 11, Platform: service.PlatformOpenAI, Extra: map[string]any{"quota_pool_id": "shared"}}

	want := service.DeriveOpenAIFailureDomains(account, 7)
	require.True(t, budget.ObserveDomain(openAIRetryFailureDomains(account, 7)))
	require.Equal(t, want, budget.ObservedDomains())
}

func TestOpenAITTFTReportEligibleRequiresSafePreOutputCapacity(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 4, MaxAccountSwitches: 3, MaxFailureDomains: 2, Total: 5 * time.Second}, func() time.Time { return now })

	require.True(t, openAITTFTReportEligible(true, false, false, 2, budget))
	require.False(t, openAITTFTReportEligible(false, false, false, 2, budget))
	require.False(t, openAITTFTReportEligible(true, true, false, 2, budget))
	require.False(t, openAITTFTReportEligible(true, false, true, 2, budget))
	require.False(t, openAITTFTReportEligible(true, false, false, 1, budget))

	require.True(t, budget.ConsumeAttempt(11))
	require.True(t, budget.ConsumeAttempt(11))
	require.True(t, budget.ConsumeAttempt(11))
	require.True(t, budget.ConsumeAttempt(11))
	require.False(t, openAITTFTReportEligible(true, false, false, 2, budget))
}

func TestOpenAISameAccountRetryLimitCaps502503ToOne(t *testing.T) {
	require.Equal(t, 1, openAISameAccountRetryLimit(http.StatusBadGateway, 3))
	require.Equal(t, 1, openAISameAccountRetryLimit(http.StatusServiceUnavailable, 2))
	require.Equal(t, 3, openAISameAccountRetryLimit(http.StatusTooManyRequests, 3))
}

func TestOpenAIUnifiedRetryBudgetCountsOnlyDifferentForwardStarts(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	for extra, wantAttempts := range map[int]int{0: 1, 1: 2, 2: 3, 3: 4} {
		budget := newOpenAIUnifiedRetryBudget(extra, func() time.Time { return now })
		for attempt := 0; attempt < wantAttempts; attempt++ {
			require.True(t, budget.CanStartForward(int64(attempt+1)), "extra=%d attempt=%d", extra, attempt)
			require.True(t, budget.RecordForwardStarted(int64(attempt+1)))
		}
		require.False(t, budget.CanStartForward(99))
		require.Equal(t, extra, budget.ExtraUsed())
	}
}

func TestOpenAIUnifiedRetryBudgetRejectsSameAccountAndIgnoresDomains(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIUnifiedRetryBudget(3, func() time.Time { return now })
	require.True(t, budget.RecordForwardStarted(11))
	require.False(t, budget.RecordForwardStarted(11), "ordinary text must never replay the same account")
	budget.RecordObservedDomains([]service.OpenAIFailureDomain{{Type: service.OpenAIFailureDomainProviderChannel, ID: "a"}})
	budget.RecordObservedDomains([]service.OpenAIFailureDomain{{Type: service.OpenAIFailureDomainQuotaPool, ID: "b"}})
	budget.RecordObservedDomains([]service.OpenAIFailureDomain{{Type: service.OpenAIFailureDomainUnknown, ID: "unknown"}})
	for _, id := range []int64{12, 13, 14} {
		require.True(t, budget.RecordForwardStarted(id))
	}
	require.Equal(t, 4, budget.Attempts())
	require.Len(t, budget.ObservedDomains(), 3)
}

func TestOpenAIUnifiedRetryBudgetHasNoIndependentDeadlineOrFailureDomainLimit(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIUnifiedRetryBudget(3, func() time.Time { return now })
	now = now.Add(24 * time.Hour)
	require.False(t, budget.DeadlineReached())
	require.True(t, budget.CanSwitch(12, false, false))
	delay, ok := budget.RetryDelay(&service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, ResponseHeaders: http.Header{"Retry-After": []string{"300"}}}, 1)
	require.True(t, ok)
	require.Equal(t, time.Duration(0), delay)
}

func TestOpenAIUnifiedFailureSafetyBlocksNonReplayableAttempts(t *testing.T) {
	base := service.OpenAIUpstreamFailureClass{Transient: true, SafeToReplay: true}
	allowed := &service.UpstreamFailoverError{NextAccountAction: service.NextAccountRetry}
	require.True(t, openAIUnifiedFailureSafeToReplay(base, allowed, false))
	for _, tc := range []struct {
		name    string
		failure service.OpenAIUpstreamFailureClass
		usage   bool
		err     *service.UpstreamFailoverError
	}{
		{name: "output", failure: service.OpenAIUpstreamFailureClass{Transient: true, SafeToReplay: true, OutputStarted: true}, err: allowed},
		{name: "side effect", failure: service.OpenAIUpstreamFailureClass{Transient: true, SafeToReplay: true, HasSideEffect: true}, err: allowed},
		{name: "usage", failure: base, usage: true, err: allowed},
		{name: "unknown billing", failure: base, err: &service.UpstreamFailoverError{NextAccountAction: service.NextAccountRetry, UsageKnown: true}},
		{name: "stop action", failure: base, err: &service.UpstreamFailoverError{NextAccountAction: service.NextAccountStop}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, openAIUnifiedFailureSafeToReplay(tc.failure, tc.err, tc.usage))
		})
	}
}

func TestOpenAIUnifiedOAuth429KeepsNativeCooldownAndStopSemantics(t *testing.T) {
	account := &service.Account{ID: 9001, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	failure := &service.UpstreamFailoverError{
		StatusCode:        http.StatusTooManyRequests,
		NextAccountAction: service.NextAccountRetry,
		ResponseHeaders:   http.Header{"Retry-After": []string{"1"}},
		ResponseBody:      []byte(`{"error":{"type":"rate_limit_error","message":"rate limited"}}`),
	}
	state := &service.OpenAIOAuth429FailoverState{}
	gateway := &recordingOpenAIUnifiedOAuth429Gateway{}

	require.False(t, handleOpenAIUnifiedOAuth429(gateway, context.Background(), account, failure, 0, state))
	require.False(t, handleOpenAIUnifiedOAuth429(gateway, context.Background(), account, failure, 1, state))
	require.True(t, handleOpenAIUnifiedOAuth429(gateway, context.Background(), account, failure, 3, state))
	require.Equal(t, 3, gateway.persistCalls)
	require.Equal(t, account.ID, gateway.lastAccountID)
	require.Equal(t, 3, gateway.lastFailedSwitches)
}

type recordingOpenAIUnifiedOAuth429Gateway struct {
	persistCalls       int
	lastAccountID      int64
	lastFailedSwitches int
}

func (g *recordingOpenAIUnifiedOAuth429Gateway) PersistOpenAIOAuth429Cooldown(_ context.Context, account *service.Account, _ http.Header, _ []byte) {
	g.persistCalls++
	if account != nil {
		g.lastAccountID = account.ID
	}
}

func (g *recordingOpenAIUnifiedOAuth429Gateway) ShouldStopOpenAIOAuth429Failover(_ *service.Account, _ int, failedSwitches int, _ *service.OpenAIOAuth429FailoverState) bool {
	g.lastFailedSwitches = failedSwitches
	return failedSwitches >= 3
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}
