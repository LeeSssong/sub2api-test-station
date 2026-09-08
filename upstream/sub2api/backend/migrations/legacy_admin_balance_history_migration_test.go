package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveLegacyAdminBalanceHistoryMigrationTargetsOnlyLegacyRows(t *testing.T) {
	path := filepath.Join("236_remove_legacy_admin_balance_history.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(contents))
	if !strings.Contains(sql, "delete from redeem_codes") || !strings.Contains(sql, "where type = 'admin_balance'") {
		t.Fatalf("migration must delete only redeem_codes rows with type admin_balance: %s", contents)
	}
	for _, forbidden := range []string{"delete from user_quota_grants", "delete from user_quota_adjustments", "update users", "delete from payment_orders"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration must not contain %q", forbidden)
		}
	}
}
