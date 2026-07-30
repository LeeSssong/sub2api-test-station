package nativeopssilence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresReaderMatchesOnlyActiveExactDimensions(t *testing.T) {
	reader, pool := openTestReader(t)
	defer reader.Close()
	ctx := context.Background()
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	ruleID := time.Now().UnixNano()
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM public.ops_alert_silences WHERE rule_id = $1`, ruleID); err != nil {
			t.Errorf("clean up silence rows: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, `
		INSERT INTO public.ops_alert_silences (rule_id, platform, group_id, region, until)
		VALUES
			($1, 'openai', 16, NULL, $2),
			($1, 'openai', 16, 'us', $2),
			($1, 'openai', 17, NULL, $2),
			($1, 'openai', NULL, NULL, $2),
			($1, 'openai', 99, NULL, $3)`, ruleID, now.Add(time.Hour), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		scope Scope
		want  bool
	}{
		{"exact null region", Scope{RuleID: ruleID, Platform: "openai", GroupID: ptr(int64(16))}, true},
		{"different rule ID", Scope{RuleID: ruleID + 1, Platform: "openai", GroupID: ptr(int64(16))}, false},
		{"different platform", Scope{RuleID: ruleID, Platform: "anthropic", GroupID: ptr(int64(16))}, false},
		{"active but different group ID", Scope{RuleID: ruleID, Platform: "openai", GroupID: ptr(int64(18))}, false},
		{"different region", Scope{RuleID: ruleID, Platform: "openai", GroupID: ptr(int64(16)), Region: ptr("eu")}, false},
		{"exact region", Scope{RuleID: ruleID, Platform: "openai", GroupID: ptr(int64(16)), Region: ptr("us")}, true},
		{"null group", Scope{RuleID: ruleID, Platform: "openai"}, true},
		{"expired", Scope{RuleID: ruleID, Platform: "openai", GroupID: ptr(int64(99))}, false},
	}
	for _, test := range tests {
		matched, err := reader.IsSilenced(ctx, test.scope, now)
		if err != nil {
			t.Errorf("%s: IsSilenced: %v", test.name, err)
		}
		if matched != test.want {
			t.Errorf("%s: matched = %v, want %v", test.name, matched, test.want)
		}
	}
}

func TestPostgresReaderRejectsInvalidScope(t *testing.T) {
	reader, _ := openTestReader(t)
	defer reader.Close()
	for _, scope := range []Scope{{Platform: "openai"}, {RuleID: 7}, {RuleID: 7, Platform: " "}} {
		if _, err := reader.IsSilenced(context.Background(), scope, time.Now()); err == nil {
			t.Fatalf("invalid scope %#v was accepted", scope)
		}
	}
}

func TestPostgresReaderQueryFailureDoesNotExposeConnectionString(t *testing.T) {
	reader, pool := openTestReader(t)
	defer reader.Close()
	secret := os.Getenv("RELAY_OPS_TEST_DATABASE_URL")
	ctx := context.Background()
	backupTable := fmt.Sprintf("ops_alert_silences_query_failure_%d", time.Now().UnixNano())
	backupIdentifier := pgx.Identifier{"public", backupTable}.Sanitize()
	if _, err := pool.Exec(ctx, fmt.Sprintf(`ALTER TABLE public.ops_alert_silences RENAME TO %s`, pgx.Identifier{backupTable}.Sanitize())); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s RENAME TO ops_alert_silences`, backupIdentifier)); err != nil {
			t.Errorf("restore silence table: %v", err)
		}
	})
	_, err := reader.IsSilenced(ctx, Scope{RuleID: 7, Platform: "openai"}, time.Now())
	if err == nil {
		t.Fatal("query against missing table unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("query error exposed connection string: %v", err)
	}
}

func openTestReader(t *testing.T) (*PostgresReader, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("RELAY_OPS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("RELAY_OPS_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("open test pool: %v", err)
	}
	t.Cleanup(admin.Close)
	var tableExists bool
	if err := admin.QueryRow(ctx, `SELECT to_regclass('public.ops_alert_silences') IS NOT NULL`).Scan(&tableExists); err != nil {
		t.Fatalf("check silence table: %v", err)
	}
	if !tableExists {
		if _, err := admin.Exec(ctx, `
			CREATE TABLE public.ops_alert_silences (
				rule_id BIGINT NOT NULL,
				platform TEXT NOT NULL,
				group_id BIGINT NULL,
				region TEXT NULL,
				until TIMESTAMPTZ NOT NULL
			)`); err != nil {
			t.Fatalf("create silence table: %v", err)
		}
		t.Cleanup(func() {
			if _, err := admin.Exec(context.Background(), `DROP TABLE public.ops_alert_silences`); err != nil {
				t.Errorf("drop temporary silence table: %v", err)
			}
		})
	}
	secretPath := filepath.Join(t.TempDir(), "database-url")
	if err := os.WriteFile(secretPath, []byte(url), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := Open(ctx, secretPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return reader, admin
}

func ptr[T any](value T) *T { return &value }
