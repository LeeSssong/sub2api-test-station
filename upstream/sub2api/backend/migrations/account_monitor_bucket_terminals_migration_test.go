package migrations

import (
	"strings"
	"testing"
)

func TestAccountMonitorBucketTerminalsMigrationCreatesIdempotentGroupBucketLedger(t *testing.T) {
	content, err := FS.ReadFile("230_account_monitor_bucket_terminals.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(content))
	for _, fragment := range []string{
		"create table if not exists account_monitor_bucket_terminals",
		"group_id bigint not null",
		"bucket_start timestamptz not null",
		"unique (group_id, bucket_start)",
		"status in ('success', 'failed')",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
