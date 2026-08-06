package migrations

import (
	"strings"
	"testing"
)

func TestAccountProcurementCostMigrationCreatesNullableCostAndEffectiveTime(t *testing.T) {
	sqlBytes, err := FS.ReadFile("196_account_procurement_cost.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(sqlBytes)
	for _, fragment := range []string{
		"procurement_cost_cny numeric(14,2)",
		"procurement_cost_effective_at timestamptz",
		"procurement_cost_cny >= 0",
	} {
		if !strings.Contains(strings.ToLower(sql), fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
