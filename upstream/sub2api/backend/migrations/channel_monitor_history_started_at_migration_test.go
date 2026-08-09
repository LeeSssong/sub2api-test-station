package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration201AddsChannelMonitorHistoryStartedAt(t *testing.T) {
	content, err := FS.ReadFile("201_channel_monitor_history_started_at.sql")
	require.NoError(t, err)
	sql := string(content)
	require.Contains(t, sql, "ALTER TABLE channel_monitors")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS history_started_at TIMESTAMPTZ NOT NULL DEFAULT NOW()")
	require.Contains(t, sql, "UPDATE channel_monitors")
	require.Contains(t, sql, "SET history_started_at = NOW()")
}
