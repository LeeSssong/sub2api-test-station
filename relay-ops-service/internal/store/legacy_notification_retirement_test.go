package store

import (
	"context"
	"os"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/legacyretirement"
)

var retiredNotificationTables = legacyretirement.Tables

func TestRuntimeMigrationDoesNotReferenceRetiredNotificationTables(t *testing.T) {
	lowerMigration := strings.ToLower(initialMigration)
	for _, table := range retiredNotificationTables {
		if strings.Contains(lowerMigration, "relay_ops."+table) {
			t.Fatalf("runtime migration still references retired table %s", table)
		}
	}
}

func TestMigrateDoesNotRecreateRetiredNotificationTables(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range retiredNotificationTables {
		var exists bool
		if err := st.pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, "relay_ops."+table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatalf("runtime migration recreated retired table %s", table)
		}
	}
}

func TestRetirementMigrationDropsOnlyAuthorizedTablesInOrder(t *testing.T) {
	raw, err := os.ReadFile("migrations/015_retire_legacy_notifications.sql")
	if err != nil {
		t.Fatal(err)
	}
	lowerSQL := strings.ToLower(string(raw))
	position := -1
	for _, table := range retiredNotificationTables {
		needle := "drop table if exists relay_ops." + table
		next := strings.Index(lowerSQL, needle)
		if next < 0 {
			t.Fatalf("retirement migration does not drop %s", table)
		}
		if next <= position {
			t.Fatalf("retirement migration drops %s out of order", table)
		}
		position = next
	}
	if strings.Count(lowerSQL, "drop table") != len(retiredNotificationTables) {
		t.Fatalf("retirement migration must drop exactly %d tables", len(retiredNotificationTables))
	}
}
