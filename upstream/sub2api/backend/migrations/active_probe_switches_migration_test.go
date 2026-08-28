package migrations

import (
	"strings"
	"testing"
)

func TestActiveProbeSwitchMigrationAddsGroupGate(t *testing.T) {
	content, err := FS.ReadFile("229_active_probe_switches.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(content))
	for _, fragment := range []string{"alter table groups", "active_probe_enabled", "default true"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
