package migrations

import (
	"strings"
	"testing"
)

func TestUpstreamBalanceEventMigrationAddsScopedNonSensitiveLedger(t *testing.T) {
	sqlBytes, err := FS.ReadFile("231_upstream_baseurl_balance_notifications.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.Join(strings.Fields(strings.ToLower(string(sqlBytes))), " ")

	for _, fragment := range []string{
		"alter table ops_alert_events add column if not exists scope_type",
		"add column if not exists scope_key",
		"add column if not exists notification_state",
		"add column if not exists last_observed_at",
		"add column if not exists last_delivered_at",
		"add column if not exists delivery_generation",
		"add column if not exists delivery_attempt_count",
		"add column if not exists next_attempt_at",
		"add column if not exists delivery_lease_token",
		"add column if not exists delivery_lease_until",
		"add column if not exists last_delivery_error_code",
		"create unique index if not exists idx_ops_alert_events_active_scope_unique on ops_alert_events (rule_id, scope_type, scope_key) where status = 'firing'",
		"'upstream_baseurl_balance_usd_v1'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}

	for _, forbidden := range []string{"login_account", "login_password", "card_payload", "api_key"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration must not persist sensitive field %q", forbidden)
		}
	}
}
