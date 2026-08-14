package migrations

import (
	"strings"
	"testing"
)

func TestAccountMonitorGlobalScoreWeightsMigrationCreatesFourWeightSingleton(t *testing.T) {
	sqlBytes, err := FS.ReadFile("223_account_monitor_global_score_weights.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(sqlBytes))
	for _, fragment := range []string{
		"create table if not exists account_monitor_global_score_weights",
		"singleton boolean primary key default true check (singleton)",
		"cost_weight smallint not null check (cost_weight >= 0)",
		"success_weight smallint not null check (success_weight >= 0)",
		"ttft_weight smallint not null check (ttft_weight >= 0)",
		"latency_weight smallint not null check (latency_weight >= 0)",
		"updated_by bigint not null",
		"updated_at timestamptz not null default now()",
		"cost_weight + success_weight + ttft_weight + latency_weight = 100",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"ttft_target_ms", "ttft_limit_ms", "latency_target_ms", "latency_limit_ms"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("global table must not include threshold column %s", forbidden)
		}
	}
}
