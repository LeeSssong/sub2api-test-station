package migrations

import (
	"strings"
	"testing"
)

func TestUserQuotaWalletLedgerMigration(t *testing.T) {
	sqlBytes, err := FS.ReadFile("225_user_quota_wallet_ledger.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(sqlBytes))
	for _, fragment := range []string{
		"create table if not exists user_wallets",
		"create table if not exists user_quota_ledger_entries",
		"create table if not exists quota_idempotency_records",
		"cash_balance_cny decimal(20,8)",
		"paid_quota_balance_usd decimal(20,8)",
		"gift_quota_balance_usd decimal(20,8)",
		"check (cash_balance_cny >= 0)",
		"check (paid_quota_balance_usd >= 0)",
		"check (gift_quota_balance_usd >= 0)",
		"on conflict (user_id) do nothing",
		"create index if not exists idx_user_quota_ledger_entries_user_created_at",
		"create index if not exists idx_user_quota_ledger_entries_reference",
		"unique (user_id, idempotency_key)",
		"update user_wallets",
		"set cash_balance_cny = 0",
		"gift_quota_balance_usd = 0",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
	for _, legacy := range []string{"users.balance", "users", "balance"} {
		if !strings.Contains(sql, legacy) {
			t.Fatalf("migration must preserve/read legacy %q", legacy)
		}
	}
	if strings.Contains(sql, "insert into user_quota_ledger_entries") {
		t.Fatal("migration must not backfill historical ledger entries")
	}
}
