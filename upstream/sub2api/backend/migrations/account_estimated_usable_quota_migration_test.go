package migrations

import (
	"strings"
	"testing"
)

func TestAccountEstimatedUsableQuotaMigrationCreatesNullablePositiveQuota(t *testing.T) {
	sqlBytes, err := FS.ReadFile("197_account_estimated_usable_quota.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(sqlBytes))
	for _, fragment := range []string{
		"estimated_usable_quota_usd numeric(14,2)",
		"accounts_estimated_usable_quota_usd_positive",
		"estimated_usable_quota_usd > 0",
		"estimated_usable_quota_usd <> 'nan'::numeric",
		"estimated_usable_quota_usd <> 'infinity'::numeric",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
