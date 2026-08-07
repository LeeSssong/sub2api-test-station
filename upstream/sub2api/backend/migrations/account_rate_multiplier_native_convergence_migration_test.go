package migrations

import (
	"strings"
	"testing"
)

func TestAccountRateMultiplierNativeConvergenceMigration(t *testing.T) {
	sqlBytes, err := FS.ReadFile("198_account_rate_multiplier_native_convergence.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.Join(strings.Fields(strings.ToLower(string(sqlBytes))), " ")
	for _, fragment := range []string{
		"when extra ->> 'upstream_billing_rate_multiplier_policy' = 'manual_override' then 'false'::jsonb",
		"when extra ->> 'upstream_billing_rate_multiplier_policy' = 'upstream_managed' then 'true'::jsonb",
		"jsonb_set( extra, '{upstream_billing_rate_sync_enabled}'",
		"- 'upstream_billing_rate_multiplier_policy'",
		"- 'account_monitor_multiplier_measurement'",
		"where extra ? 'upstream_billing_rate_multiplier_policy'",
		"or extra ? 'account_monitor_multiplier_measurement'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
	if strings.Contains(sql, "set rate_multiplier") {
		t.Fatal("migration must preserve accounts.rate_multiplier")
	}
}
