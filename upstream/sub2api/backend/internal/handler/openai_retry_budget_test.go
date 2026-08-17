package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRetryBudgetBoundsAttemptsAndAccountSwitches(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 4, MaxAccountSwitches: 3, MaxFailureDomains: 2, Total: 5 * time.Second}, func() time.Time { return now })

	require.True(t, budget.ConsumeAttempt(11))
	require.True(t, budget.ConsumeAttempt(11), "same-account retry does not consume an account switch")
	require.True(t, budget.ConsumeAttempt(12))
	require.True(t, budget.ConsumeAttempt(13))
	require.False(t, budget.ConsumeAttempt(14), "the fifth real upstream attempt is rejected")
}

func TestOpenAIRetryBudgetRejectsFourthAccountSwitch(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	budget := newOpenAIRetryBudget(openAIRetryBudgetConfig{MaxAttempts: 8, MaxAccountSwitches: 3, MaxFailureDomains: 2, Total: 5 * time.Second}, func() time.Time { return now })

	for _, accountID := range []int64{11, 12, 13, 14} {
		require.True(t, budget.ConsumeAttempt(accountID))
	}
	require.False(t, budget.CanSwitch(15, false, false))
	require.False(t, budget.ConsumeAttempt(15))
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

	for attempt, want := range []time.Duration{120 * time.Millisecond, 240 * time.Millisecond, 480 * time.Millisecond, 960 * time.Millisecond, 1920 * time.Millisecond, 2 * time.Second} {
		delay, ok := budget.RetryDelay(failure, attempt+1)
		require.True(t, ok)
		require.Equal(t, want, delay)
	}
}
