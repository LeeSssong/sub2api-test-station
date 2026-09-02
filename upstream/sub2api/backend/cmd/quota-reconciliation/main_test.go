package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSnapshotUsesDecimalStrings(t *testing.T) {
	snapshot, err := parseSnapshot([]byte(`{
  "wallets":[{"user_id":1,"paid_balance_usd":"4.00000000","gift_balance_usd":"1.00000000"}],
  "grants":[{"id":1,"user_id":1,"paid_granted_usd":"5","gift_granted_usd":"1","paid_consumed_usd":"1"}],
  "usage":[{"id":"u1","user_id":1,"grant_id":1,"paid_delta_usd":"1","gift_delta_usd":"0","allocation_valid":true}]
}`))
	require.NoError(t, err)
	require.Equal(t, "4", snapshot.Wallets[0].PaidBalance.String())
	require.Equal(t, "1", snapshot.Usage[0].PaidDelta.String())
}

func TestParseSnapshotRejectsFloatJSONNumber(t *testing.T) {
	_, err := parseSnapshot([]byte(`{"wallets":[{"user_id":1,"paid_balance_usd":4.2}]}`))
	require.Error(t, err)
}
