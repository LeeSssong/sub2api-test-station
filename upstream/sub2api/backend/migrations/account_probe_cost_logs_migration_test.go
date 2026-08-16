package migrations

import (
	"strings"
	"testing"
)

func TestAccountProbeCostLogsMigration(t *testing.T) {
	sqlBytes, err := FS.ReadFile("224_account_probe_cost_logs.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(sqlBytes))
	for _, fragment := range []string{
		"create table if not exists account_probe_cost_logs",
		"account_id bigint not null references accounts(id) on delete restrict",
		"probe_kind in ('monitor', 'scheduled', 'manual')",
		"usage_completeness in ('complete', 'partial', 'unknown')",
		"probe_outcome in ('success', 'failure')",
		"account_cost decimal(20,10)",
		"unique (probe_run_id)",
		"create index if not exists idx_account_probe_cost_logs_created_at",
		"create index if not exists idx_account_probe_cost_logs_account_created_at",
		"create index if not exists idx_account_probe_cost_logs_group_created_at",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
	if strings.Contains(sql, "alter table usage_logs") || strings.Contains(sql, "insert into usage_logs") {
		t.Fatal("probe migration must not mutate usage_logs")
	}
	for _, forbidden := range []string{"user_id", "api_key_id", "on delete cascade"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("probe migration must not contain %q", forbidden)
		}
	}
}
