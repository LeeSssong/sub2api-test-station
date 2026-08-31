package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func float64Pointer(value float64) *float64 { return &value }

func TestEffectiveCostProvider(t *testing.T) {
	t.Run("api key defaults to direct multiplier", func(t *testing.T) {
		account := &Account{Type: AccountTypeAPIKey, RateMultiplier: float64Pointer(0.35)}
		cost := EffectiveCostForAccount(account)
		require.Equal(t, EffectiveCostModelDirectMultiplier, cost.Model)
		require.Equal(t, EffectiveCostStatusReady, cost.Status)
		require.InDelta(t, 1, *cost.A, 1e-12)
		require.InDelta(t, 0.35, *cost.R, 1e-12)
		require.InDelta(t, 0.35, *cost.U, 1e-12)
	})

	t.Run("ratio based api key multiplies ratio by upstream rate", func(t *testing.T) {
		account := &Account{
			Type: AccountTypeAPIKey, EffectiveCostModel: EffectiveCostModelRatioBasedUpstream,
			UpstreamActualCost: float64Pointer(1), UpstreamObtainedQuota: float64Pointer(10), RateMultiplier: float64Pointer(1.5),
		}
		cost := EffectiveCostForAccount(account)
		require.Equal(t, EffectiveCostStatusReady, cost.Status)
		require.InDelta(t, 0.1, *cost.A, 1e-12)
		require.InDelta(t, 1.5, *cost.R, 1e-12)
		require.InDelta(t, 0.15, *cost.U, 1e-12)
	})

	t.Run("oauth is always self owned and ignores upstream rate", func(t *testing.T) {
		account := &Account{
			Type: AccountTypeOAuth, EffectiveCostModel: EffectiveCostModelRatioBasedUpstream,
			ProcurementCostCNY: float64Pointer(4), EstimatedUsableQuotaUSD: float64Pointer(100), RateMultiplier: float64Pointer(99),
		}
		cost := EffectiveCostForAccount(account)
		require.Equal(t, EffectiveCostModelSelfOwned, cost.Model)
		require.Equal(t, EffectiveCostStatusReady, cost.Status)
		require.InDelta(t, 0.04, *cost.A, 1e-12)
		require.Nil(t, cost.R)
		require.InDelta(t, 0.04, *cost.U, 1e-12)
	})

	for name, account := range map[string]*Account{
		"ratio missing quota":       {Type: AccountTypeAPIKey, EffectiveCostModel: EffectiveCostModelRatioBasedUpstream, UpstreamActualCost: float64Pointer(1), RateMultiplier: float64Pointer(1)},
		"oauth missing procurement": {Type: AccountTypeOAuth, EstimatedUsableQuotaUSD: float64Pointer(100)},
		"unsupported account type":  {Type: AccountTypeSetupToken, RateMultiplier: float64Pointer(1)},
	} {
		t.Run(name, func(t *testing.T) {
			cost := EffectiveCostForAccount(account)
			require.Equal(t, EffectiveCostStatusUnknown, cost.Status)
			require.Nil(t, cost.U)
		})
	}
}

func TestOpenAIProfitControlUsesEffectiveCostAndFailsOpenWhenUnknown(t *testing.T) {
	ratio := &Account{
		Type: AccountTypeAPIKey, EffectiveCostModel: EffectiveCostModelRatioBasedUpstream,
		UpstreamActualCost: float64Pointer(1), UpstreamObtainedQuota: float64Pointer(10), RateMultiplier: float64Pointer(5),
	}
	ctx := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{threshold: 0.4})
	vetoed, reason := openAIProfitControlVetoReasonReadOnly(ctx, ratio)
	require.True(t, vetoed)
	require.Equal(t, openAIProfitFilterReasonThreshold, reason)

	ratio.UpstreamObtainedQuota = nil
	vetoed, reason = openAIProfitControlVetoReasonReadOnly(ctx, ratio)
	require.False(t, vetoed, "unknown normalized cost preserves official availability-first behavior")
	require.Empty(t, reason)
}
