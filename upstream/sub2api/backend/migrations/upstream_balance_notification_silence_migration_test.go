package migrations

import (
	"strings"
	"testing"
)

func TestUpstreamBalanceNotificationSilenceMigrationIsAdditiveAndNonSensitive(t *testing.T) {
	contents, err := FS.ReadFile("233_upstream_balance_notification_silence.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.Join(strings.Fields(strings.ToLower(string(contents))), " ")
	for _, fragment := range []string{
		"add column if not exists silenced_until",
		"add column if not exists action_token_hash",
		"create index if not exists idx_ops_alert_events_balance_silence",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"api_key", "login_password", "card_payload", "base_url text"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration persists forbidden field %q", forbidden)
		}
	}
}
