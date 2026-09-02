package migrations

import (
	"strings"
	"testing"
)

func TestMonitorV4Windows1HMigration(t *testing.T) {
	sql, err := FS.ReadFile("233_monitor_v4_windows_1h.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(sql))
	for _, want := range []string{
		"drop constraint if exists account_monitor_v4_snapshots_window_check",
		"delete from account_monitor_v4_snapshots where \"window\" = '30d'",
		`check ("window" in ('1h', '24h', '7d'))`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("migration missing %q", want)
		}
	}
}
