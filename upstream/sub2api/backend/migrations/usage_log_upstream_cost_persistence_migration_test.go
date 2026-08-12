package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageLogUpstreamCostPersistenceMigration(t *testing.T) {
	sqlBytes, err := FS.ReadFile("221_usage_log_upstream_cost_persistence.sql")
	require.NoError(t, err)

	normalizedSQL := strings.Join(strings.Fields(strings.ToLower(string(sqlBytes))), " ")
	for _, fragment := range []string{
		"add column if not exists upstream_actual_cost decimal(20, 10)",
		"add column if not exists upstream_cost_status varchar(16)",
		"add column if not exists upstream_cost_reason varchar(64)",
		"add column if not exists profit decimal(20, 10)",
		"add column if not exists upstream_cost_recorded_at timestamptz",
	} {
		require.Contains(t, normalizedSQL, fragment)
	}
	require.NotContains(t, normalizedSQL, "update usage_logs")
	require.NotContains(t, normalizedSQL, "create index")
}
