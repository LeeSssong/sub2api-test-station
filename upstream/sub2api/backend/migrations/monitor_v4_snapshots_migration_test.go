package migrations

import (
	"regexp"
	"strings"
	"testing"
)

func TestMonitorV4SnapshotsMigration(t *testing.T) {
	sql, err := FS.ReadFile("232_monitor_v4_snapshots.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(sql))
	for _, want := range []string{
		"create table if not exists account_monitor_v4_snapshots",
		`"window" in ('24h', '7d', '30d')`,
		"group_id > 0",
		"request_count >= 0",
		"success_count >= 0",
		"window_start < window_end",
		`unique ("window", group_id)`,
		"snapshot_id uuid",
		"generated_at",
		"create index if not exists",
		"create index if not exists account_monitor_v4_snapshots_window_generated_idx",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("migration missing %q", want)
		}
	}
	if strings.Count(text, "if not exists") < 2 {
		t.Fatal("migration must make both table and index creation idempotent")
	}
	if !regexp.MustCompile(`(?m)^\s*"window"\s+text\b`).MatchString(text) {
		t.Fatal(`migration must quote the PostgreSQL-reserved "window" column`)
	}
	if !strings.Contains(text, `unique ("window", group_id)`) {
		t.Fatal(`migration uniqueness must quote the PostgreSQL-reserved "window" column`)
	}
	if !regexp.MustCompile(`(?s)create index if not exists .*\("window", generated_at desc\)`).MatchString(text) {
		t.Fatal(`migration index must quote the PostgreSQL-reserved "window" column`)
	}
	if !regexp.MustCompile(`(?s)create index if not exists .*window.*generated_at desc`).MatchString(text) {
		t.Error("migration must index window and generated_at descending")
	}
	if strings.Contains(text, "drop table") || strings.Contains(text, "alter table") {
		t.Error("migration must be expand-only")
	}
}
