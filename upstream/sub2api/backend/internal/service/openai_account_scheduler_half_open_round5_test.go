package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIHalfOpenSchedulerDisabled_PublicSelectionProbesAtCooldownExpiry(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)

	groupID := int64(49001)
	account := openAIHalfOpenRound4Account(49011, groupID)
	acquiredIDs := make([]int64, 0, 1)
	releasedIDs := make([]int64, 0, 1)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	svc := &OpenAIGatewayService{
		accountRepo:          openAIHalfOpenRound4Repo{schedulerTestOpenAIAccountRepo{accounts: []Account{account}}},
		cfg:                  cfg,
		rateLimitService:     newOpenAIAdvancedSchedulerRateLimitService("false"),
		concurrencyService:   NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquiredIDs, releasedIDs: &releasedIDs}),
		openaiModelTransient: newOpenAIAccountModelTransientState(32),
	}
	require.False(t, svc.isOpenAIAdvancedSchedulerEnabled(context.Background()))

	now := time.Now()
	canonicalModel := canonicalOpenAIAccountSchedulingModel(&account, "gpt-5.5")
	for _, failedAt := range []time.Time{now.Add(-12 * time.Second), now.Add(-11 * time.Second)} {
		svc.RecordOpenAIAccountModelFailure(context.Background(), OpenAIAccountModelFailureEvent{
			AccountID: account.ID, CanonicalModel: canonicalModel, StatusCode: 502, Now: failedAt,
		})
	}
	require.True(t, svc.getOpenAIAccountModelTransientState().isBlocked(account.ID, canonicalModel, now))

	selectAccount := func() (*AccountSelectionResult, error) {
		selection, _, err := svc.SelectAccountWithSchedulerForCapability(
			context.Background(), &groupID, "", "", "gpt-5.5", nil,
			OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityChatCompletions,
			false, false, true,
		)
		return selection, err
	}

	selection, err := selectAccount()
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, account.ID, selection.Account.ID)
	require.True(t, selection.Acquired)
	require.True(t, selection.HalfOpenProbe)
	require.Nil(t, selection.WaitPlan)
	require.NotNil(t, selection.halfOpenLease)
	require.Equal(t, canonicalModel, selection.halfOpenLease.canonicalModel)
	require.Equal(t, []int64{account.ID}, acquiredIDs)
	runtimeSnapshot := svc.SnapshotOpenAIAccountModelRuntime(time.Now())
	require.Len(t, runtimeSnapshot, 1)
	require.True(t, runtimeSnapshot[0].HalfOpenInFlight)

	second, secondErr := selectAccount()
	require.Error(t, secondErr)
	require.Nil(t, second)
	require.Equal(t, []int64{account.ID}, acquiredIDs, "only the lease owner may acquire a slot")

	selection.CompleteHalfOpenProbe(true)
	selection.CompleteHalfOpenProbe(true)
	selection.ReleaseFunc()
	selection.ReleaseFunc()
	require.Equal(t, []int64{account.ID}, releasedIDs)
	require.Empty(t, svc.SnapshotOpenAIAccountModelRuntime(time.Now()))
	require.Nil(t, svc.openaiScheduler, "the half-open fallback must not enable advanced scheduling globally")
	require.Equal(t, OpenAIAccountSchedulerMetricsSnapshot{}, svc.SnapshotOpenAIAccountSchedulerMetrics())
}
