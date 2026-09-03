package migrations

import (
	"testing"
	"io/fs"

	"github.com/stretchr/testify/require"
)

func TestOpenAISchedulerLogsMigrationCreatesIndexedRetentionTable(t *testing.T) {
	contents, err := fs.ReadFile(FS, "234_openai_scheduler_logs.sql")
	require.NoError(t, err)
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS openai_scheduler_logs",
		"logical_request_id",
		"attempt_id",
		"event_at",
		"CREATE INDEX IF NOT EXISTS idx_openai_scheduler_logs_event_at",
		"CREATE INDEX IF NOT EXISTS idx_openai_scheduler_logs_logical_request",
	} {
		require.Contains(t, sql, fragment)
	}
}
