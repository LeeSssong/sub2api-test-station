package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyGroupProfitabilityEstimatesFromMultipliersWithoutRealRequests(t *testing.T) {
	upstream := 0.15
	groups := []AccountMonitorGroup{{
		ID:             7,
		Name:           "GPT-Pro",
		RateMultiplier: 0.2,
		Accounts: []AccountMonitorGroupAccount{{
			AccountMonitorAccount: AccountMonitorAccount{
				AccountID: 244,
				Multiplier: AccountMonitorMultiplier{
					Value:  &upstream,
					Status: AccountMonitorMultiplierStatusOK,
				},
			},
		}},
	}}

	applyGroupProfitability(groups, map[int64]AccountMonitorWindowAggregate{})

	profit := groups[0].Accounts[0].GroupProfitability
	require.NotNil(t, profit)
	require.Equal(t, "estimated", profit.Status)
	require.NotNil(t, profit.ProfitRate)
	require.InDelta(t, 0.25, *profit.ProfitRate, 0.000001)
	require.NotNil(t, profit.Rank)
	require.Equal(t, 1, profit.Rank.Rank)
	require.Equal(t, 1, profit.Rank.Total)
}

func TestApplyGroupProfitabilityKeepsMissingMultiplierUnconfirmed(t *testing.T) {
	groups := []AccountMonitorGroup{{
		ID:             7,
		Name:           "GPT-Pro",
		RateMultiplier: 0.2,
		Accounts: []AccountMonitorGroupAccount{{
			AccountMonitorAccount: AccountMonitorAccount{
				AccountID: 244,
				Multiplier: AccountMonitorMultiplier{
					Status: AccountMonitorMultiplierStatusUnavailable,
				},
			},
		}},
	}}

	applyGroupProfitability(groups, map[int64]AccountMonitorWindowAggregate{})

	profit := groups[0].Accounts[0].GroupProfitability
	require.NotNil(t, profit)
	require.Equal(t, "no_real_request", profit.Status)
	require.Nil(t, profit.ProfitRate)
	require.Nil(t, profit.Rank)
}
