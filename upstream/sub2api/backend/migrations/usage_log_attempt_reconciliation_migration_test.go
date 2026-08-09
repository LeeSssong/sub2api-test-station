package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageLogAttemptReconciliationMigration(t *testing.T) {
	sqlBytes, err := FS.ReadFile("200_usage_log_attempt_reconciliation.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(sqlBytes))

	for _, column := range []string{
		"logical_request_id",
		"attempt_id",
		"usage_completeness",
		"reconciliation_required",
		"unsafe_to_replay",
	} {
		require.Contains(t, sql, "add column if not exists "+column)
	}
	require.Contains(t, sql, "check (usage_completeness in ('complete', 'partial', 'unknown'))")
	require.Contains(t, sql, "where reconciliation_required = true")
}
