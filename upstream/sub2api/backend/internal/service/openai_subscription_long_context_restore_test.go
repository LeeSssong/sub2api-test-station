//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestGPT61SolRestoredSubscriptionLongContextBoundary(t *testing.T) {
	for _, n := range []int{272000, 272001, 500000} {
		repo := &openAIRecordUsageLogRepoStub{inserted: true}
		svc := newOpenAIRecordUsageServiceForTest(repo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
		err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{RequestID: "restored", Model: "gpt-6.1-sol", Duration: time.Second, Usage: OpenAIUsage{InputTokens: n, OutputTokens: 1000}},
			APIKey: openAIRecordUsageAPIKeyWithGroup(svc, 1, true), User: &User{ID: 2},
			Account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{openAILongContextBillingEnabledKey: false}},
		})
		require.NoError(t, err)
		require.Equal(t, n > 272000, repo.lastLog.LongContextBillingApplied)
		inputPrice, outputCost := 2e-6, 0.01
		if n > 272000 {
			inputPrice = 4e-6
			outputCost = 0.015
		}
		require.InDelta(t, float64(n)*inputPrice, repo.lastLog.InputCost, 1e-12)
		require.InDelta(t, outputCost, repo.lastLog.OutputCost, 1e-12)
	}
}
