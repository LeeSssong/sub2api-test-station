package migrations

import (
	"strings"
	"testing"
)

func TestAccountMonitorScoreThresholdMigrationKeepsApprovedDefaults(t *testing.T) {
	sqlBytes, err := FS.ReadFile("194_account_monitor_score_thresholds.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(sqlBytes)
	for _, fragment := range []string{
		"ttft_target_ms INTEGER NOT NULL DEFAULT 1000",
		"ttft_limit_ms INTEGER NOT NULL DEFAULT 5000",
		"latency_target_ms INTEGER NOT NULL DEFAULT 10000",
		"latency_limit_ms INTEGER NOT NULL DEFAULT 60000",
		"ttft_limit_ms > ttft_target_ms",
		"latency_limit_ms > latency_target_ms",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestOpsMetricsDurationSampleMigrationBackfillsLegacyRows(t *testing.T) {
	sqlBytes, err := FS.ReadFile("195_ops_metrics_duration_sample_count.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(sqlBytes)
	for _, fragment := range []string{
		"UPDATE ops_metrics_hourly",
		"UPDATE ops_metrics_daily",
		"duration_sample_count = success_count",
		"duration_p50_ms IS NOT NULL",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing legacy backfill %q", fragment)
		}
	}
}
