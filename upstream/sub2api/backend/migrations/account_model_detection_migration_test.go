package migrations

import (
	"strings"
	"testing"
)

func TestAccountModelDetectionMigration(t *testing.T) {
	sqlBytes, err := FS.ReadFile("225_account_model_detection.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(sqlBytes))
	for _, fragment := range []string{
		"create table if not exists account_model_detection_settings",
		"account_id bigint primary key references accounts(id) on delete cascade",
		"connection_probe_model text not null default ''",
		"model_detection_model text not null default ''",
		"create table if not exists account_model_detection_runs",
		"id uuid primary key",
		"account_id bigint not null references accounts(id) on delete cascade",
		"trigger_kind in ('manual', 'scheduled')",
		"status in ('queued', 'running', 'normal', 'abnormal', 'insufficient', 'failed')",
		"juice_summary jsonb",
		"fingerprint_similarity jsonb",
		"unique (account_id, slot_key)",
		"where status in ('queued', 'running')",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"api_key", "base_url", "prompt", "full_output", "usage_logs", "account_monitor_results"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("model detection migration must not contain %q", forbidden)
		}
	}
}
