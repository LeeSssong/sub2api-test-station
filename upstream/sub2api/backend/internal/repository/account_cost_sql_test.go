//go:build unit

package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectiveAccountCostSQL(t *testing.T) {
	require.Equal(t,
		"COALESCE(account_cost, COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1))",
		effectiveAccountCostSQL(""),
	)
	require.Equal(t,
		"COALESCE(ul.account_cost, COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1))",
		effectiveAccountCostSQL("ul"),
	)
}
