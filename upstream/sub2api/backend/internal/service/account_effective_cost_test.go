package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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

	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		t.Run(accountType+" is always self owned and ignores upstream rate", func(t *testing.T) {
			account := &Account{
				Type: accountType, EffectiveCostModel: EffectiveCostModelRatioBasedUpstream,
				ProcurementCostCNY: float64Pointer(4), EstimatedUsableQuotaUSD: float64Pointer(100), RateMultiplier: float64Pointer(99),
			}
			cost := EffectiveCostForAccount(account)
			require.Equal(t, EffectiveCostModelSelfOwned, cost.Model)
			require.Equal(t, EffectiveCostStatusReady, cost.Status)
			require.InDelta(t, 0.04, *cost.A, 1e-12)
			require.Nil(t, cost.R)
			require.InDelta(t, 0.04, *cost.U, 1e-12)
		})
	}

	for name, account := range map[string]*Account{
		"ratio missing quota":       {Type: AccountTypeAPIKey, EffectiveCostModel: EffectiveCostModelRatioBasedUpstream, UpstreamActualCost: float64Pointer(1), RateMultiplier: float64Pointer(1)},
		"oauth missing procurement": {Type: AccountTypeOAuth, EstimatedUsableQuotaUSD: float64Pointer(100)},
		"unsupported account type":  {Type: "legacy-cookie", RateMultiplier: float64Pointer(1)},
	} {
		t.Run(name, func(t *testing.T) {
			cost := EffectiveCostForAccount(account)
			require.Equal(t, EffectiveCostStatusUnknown, cost.Status)
			require.Nil(t, cost.U)
		})
	}
}

func TestNormalizeEffectiveCostModelRejectsRatioInputsForOAuth(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		t.Run(accountType, func(t *testing.T) {
			model, err := NormalizeEffectiveCostModel(
				accountType,
				EffectiveCostModelSelfOwned,
				float64Pointer(1),
				float64Pointer(10),
				float64Pointer(99),
			)

			require.ErrorIs(t, err, ErrInvalidEffectiveCostRatio)
			require.True(t, infraerrors.IsBadRequest(err))
			require.Empty(t, model)
		})
	}
}

func TestNormalizeEffectiveCostModelReportsInvalidAdminInputAsBadRequest(t *testing.T) {
	model, err := NormalizeEffectiveCostModel(AccountTypeAPIKey, "unsupported", nil, nil, float64Pointer(1))

	require.ErrorIs(t, err, ErrInvalidEffectiveCostModel)
	require.True(t, infraerrors.IsBadRequest(err))
	require.Empty(t, model)
}

func TestApplyEffectiveCostConfigurationClearsRatioKeysAndPreservesOtherExtra(t *testing.T) {
	account := &Account{
		Type:                  AccountTypeAPIKey,
		RateMultiplier:        float64Pointer(0.4),
		EffectiveCostModel:    EffectiveCostModelRatioBasedUpstream,
		UpstreamActualCost:    float64Pointer(1),
		UpstreamObtainedQuota: float64Pointer(10),
		Extra: map[string]any{
			EffectiveCostModelExtraKey:    EffectiveCostModelRatioBasedUpstream,
			UpstreamActualCostExtraKey:    1.0,
			UpstreamObtainedQuotaExtraKey: 10.0,
			"unrelated":                   "preserved",
		},
	}

	err := applyEffectiveCostConfiguration(account, EffectiveCostModelDirectMultiplier, nil, nil)

	require.NoError(t, err)
	require.Equal(t, EffectiveCostModelDirectMultiplier, account.EffectiveCostModel)
	require.Nil(t, account.UpstreamActualCost)
	require.Nil(t, account.UpstreamObtainedQuota)
	require.NotContains(t, account.Extra, UpstreamActualCostExtraKey)
	require.NotContains(t, account.Extra, UpstreamObtainedQuotaExtraKey)
	require.Equal(t, "preserved", account.Extra["unrelated"])
}

func TestUpdateAccountChangingRatioAPIKeyToOAuthClearsRatioConfiguration(t *testing.T) {
	repo := &procurementCostAccountRepoStub{account: &Account{
		ID:                    95,
		Platform:              PlatformOpenAI,
		Type:                  AccountTypeAPIKey,
		Status:                StatusActive,
		RateMultiplier:        float64Pointer(1.5),
		EffectiveCostModel:    EffectiveCostModelRatioBasedUpstream,
		UpstreamActualCost:    float64Pointer(1),
		UpstreamObtainedQuota: float64Pointer(10),
		Extra: map[string]any{
			EffectiveCostModelExtraKey:    EffectiveCostModelRatioBasedUpstream,
			UpstreamActualCostExtraKey:    1.0,
			UpstreamObtainedQuotaExtraKey: 10.0,
			"unrelated":                   "preserved",
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), 95, &UpdateAccountInput{Type: AccountTypeOAuth})

	require.NoError(t, err)
	require.Equal(t, EffectiveCostModelSelfOwned, updated.EffectiveCostModel)
	require.Nil(t, updated.UpstreamActualCost)
	require.Nil(t, updated.UpstreamObtainedQuota)
	require.NotContains(t, updated.Extra, UpstreamActualCostExtraKey)
	require.NotContains(t, updated.Extra, UpstreamObtainedQuotaExtraKey)
	require.Equal(t, "preserved", updated.Extra["unrelated"])
}

func TestOpenAIProfitControlUsesEffectiveCostAndKeepsUnknownInNativeVeto(t *testing.T) {
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
	require.True(t, vetoed, "unknown normalized cost stays in the native invalid-cost veto partition")
	require.Equal(t, openAIProfitFilterReasonInvalidAccountRate, reason)
}
