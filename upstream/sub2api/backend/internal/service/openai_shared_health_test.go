package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAISharedHealthKeyNormalizesAndHashesUntrustedModel(t *testing.T) {
	key, err := NewOpenAISharedHealthKey(153, " GPT-5.6-SOL ")
	require.NoError(t, err)
	require.Equal(t, int64(153), key.AccountID)
	require.Equal(t, "gpt-5.6-sol", key.CanonicalModel)
	require.NotEmpty(t, key.HashedSuffix())
	require.Len(t, key.HashedSuffix(), 32)
	otherAccountKey, err := NewOpenAISharedHealthKey(154, "gpt-5.6-sol")
	require.NoError(t, err)
	require.Equal(t, key.HashedSuffix(), otherAccountKey.HashedSuffix())
}

func TestOpenAISharedHealthKeyRejectsInvalidInput(t *testing.T) {
	_, err := NewOpenAISharedHealthKey(0, "gpt-5.6-sol")
	require.Error(t, err)
	_, err = NewOpenAISharedHealthKey(1, "")
	require.Error(t, err)
}

func TestDeriveOpenAIFailureDomainsUsesOnlyExplicitFacts(t *testing.T) {
	quotaPool := "org-7"
	account := &Account{ID: 42, Platform: PlatformOpenAI, Extra: map[string]any{"quota_pool_id": quotaPool}}
	domains := DeriveOpenAIFailureDomains(account, 9)
	require.Equal(t, []OpenAIFailureDomain{
		{Type: OpenAIFailureDomainProviderChannel, ID: "openai:channel:9"},
		{Type: OpenAIFailureDomainQuotaPool, ID: "openai:quota_pool:org-7"},
	}, domains)

	unknown := DeriveOpenAIFailureDomains(&Account{ID: 43, Platform: PlatformOpenAI}, 0)
	require.Equal(t, []OpenAIFailureDomain{{Type: OpenAIFailureDomainUnknown, ID: "unknown"}}, unknown)

	numericQuota := DeriveOpenAIFailureDomains(&Account{ID: 44, Platform: PlatformOpenAI, Extra: map[string]any{"quota_pool_id": float64(7)}}, 0)
	require.Equal(t, []OpenAIFailureDomain{{Type: OpenAIFailureDomainQuotaPool, ID: "openai:quota_pool:7"}}, numericQuota)
	fractionalQuota := DeriveOpenAIFailureDomains(&Account{ID: 45, Platform: PlatformOpenAI, Extra: map[string]any{"quota_pool_id": 7.5}}, 0)
	require.Equal(t, []OpenAIFailureDomain{{Type: OpenAIFailureDomainUnknown, ID: "unknown"}}, fractionalQuota)
}

func TestOpenAISharedHealthSnapshotFreshness(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	snapshot := OpenAISharedHealthSnapshot{ObservedAt: now.Add(-10 * time.Second), State: OpenAISharedHealthStateCooldown}
	require.Equal(t, OpenAISharedHealthFresh, snapshot.Freshness(now, 30*time.Second))
	snapshot.ObservedAt = now.Add(-31 * time.Second)
	require.Equal(t, OpenAISharedHealthStale, snapshot.Freshness(now, 30*time.Second))
}

func TestOpenAISharedHealthConfigValidateHardLimits(t *testing.T) {
	cfg := DefaultOpenAISharedHealthConfig()
	require.NoError(t, cfg.Validate())

	cfg.MaxAttempts = 5
	require.Error(t, cfg.Validate())
	cfg = DefaultOpenAISharedHealthConfig()
	cfg.TotalRetryBudgetMS = 5001
	require.Error(t, cfg.Validate())
}
