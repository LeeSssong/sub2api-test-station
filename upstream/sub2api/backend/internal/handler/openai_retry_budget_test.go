package handler

import (
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
