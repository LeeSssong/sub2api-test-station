package migrations

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

func normalizedQuotaAccountingFoundationSQL(t *testing.T) string {
	t.Helper()
	b, err := FS.ReadFile("233_quota_accounting_foundation.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := regexp.MustCompile(`(?m)--[^\r\n]*`).ReplaceAllString(string(b), "")
	return strings.Join(strings.Fields(strings.ToLower(sql)), " ")
}

func TestQuotaAccountingFoundationMigrationContracts(t *testing.T) {
	sql := normalizedQuotaAccountingFoundationSQL(t)
	required := []string{
		"create table if not exists user_quota_grants",
		"create table if not exists user_quota_adjustments",
		"user_quota_grants_source_check",
		"user_quota_adjustments_refund_method_check",
		"user_quota_adjustments_actor_check",
		"numeric(20,8)",
		"grant_type = 'migration_opening'",
		"gift_quota_usd = 0",
		"create unique index if not exists user_quota_grants_payment_order_unique",
		"create unique index if not exists user_quota_grants_user_idempotency_unique",
		"create index if not exists user_quota_grants_fifo",
		"create unique index if not exists user_quota_adjustments_user_idempotency_unique",
		"create unique index if not exists user_quota_adjustments_provider_request_unique",
		"create unique index if not exists user_quota_adjustments_provider_refund_unique",
		"alter table payment_orders add column if not exists paid_quota_usd",
		"payment_orders_provider_trade_unique",
		"payment_orders_admin_recharge_trade_unique",
		"alter table billing_usage_entries add column if not exists paid_quota_delta_usd",
		"alter table billing_usage_entries add column if not exists attempted_quota_usd",
		"alter table redeem_codes add column if not exists code_kind",
		"alter table promo_codes add column if not exists benefit_mode",
		"alter table promo_code_usages add column if not exists grant_id",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestQuotaAccountingFoundationMigrationRequiresRefundMethod(t *testing.T) {
	sql := normalizedQuotaAccountingFoundationSQL(t)
	if !strings.Contains(sql, "refund_method in ('original_channel','manual_transfer')") {
		t.Fatal("refund recovery must require an explicit supported refund method")
	}
	if !strings.Contains(sql, "refund_method is not null") {
		t.Fatal("refund recovery must reject a missing refund method")
	}
}

func TestQuotaAccountingFoundationMigrationPreservesLegacyData(t *testing.T) {
	sql := normalizedQuotaAccountingFoundationSQL(t)
	for _, forbidden := range []string{
		"drop table user_quota_ledger_entries",
		"drop column",
		"truncate",
		"delete from",
		"check (paid_quota_balance_usd >= 0)",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("additive migration contains forbidden fragment %q", forbidden)
		}
	}
	if !strings.Contains(sql, "billing_usage_entries_usage_log_id_unique") {
		t.Fatal("migration must preserve the existing usage_log_id uniqueness contract")
	}
}

func TestQuotaAccountingFoundationMigrationIsEmbeddedInOrderAndTransactional(t *testing.T) {
	entries, err := FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	idx := sort.SearchStrings(names, "233_quota_accounting_foundation.sql")
	if idx == len(names) || names[idx] != "233_quota_accounting_foundation.sql" {
		t.Fatal("migration is not embedded")
	}
	if idx > 0 && names[idx-1] >= names[idx] {
		t.Fatal("migration order is not strictly increasing")
	}
	sql := normalizedQuotaAccountingFoundationSQL(t)
	if strings.Contains(sql, "create index concurrently") {
		t.Fatal("transactional migration must not use CREATE INDEX CONCURRENTLY")
	}
	if !strings.Contains(sql, "create table if not exists") || !strings.Contains(sql, "if not exists") {
		t.Fatal("migration must remain repeatable")
	}
}

func TestQuotaAccountingFoundationMigrationDeclaresRollbackBoundary(t *testing.T) {
	sql := normalizedQuotaAccountingFoundationSQL(t)
	if strings.Contains(sql, "drop table") || strings.Contains(sql, "drop column") || strings.Contains(sql, "delete from") {
		t.Fatal("migration must be rolled back by release-level revert, not destructive SQL")
	}
}
