package migrations

import (
	"regexp"
	"strings"
	"testing"
)

func normalizedUserQuotaWalletLedgerSQL(t *testing.T) string {
	t.Helper()
	sqlBytes, err := FS.ReadFile("227_user_quota_wallet_ledger.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Strip SQL line comments before asserting contracts so commented-out text
	// cannot satisfy the migration contract.
	sql := regexp.MustCompile(`(?m)--[^\r\n]*`).ReplaceAllString(string(sqlBytes), "")
	return strings.Join(strings.Fields(strings.ToLower(sql)), " ")
}

func TestUserQuotaWalletLedgerMigration(t *testing.T) {
	sql := normalizedUserQuotaWalletLedgerSQL(t)
	for _, fragment := range []string{
		"create table if not exists user_wallets (",
		"user_id bigint primary key references users(id) on delete cascade",
		"cash_balance_cny decimal(20,8) not null default 0 check (cash_balance_cny >= 0)",
		"paid_quota_balance_usd decimal(20,8) not null default 0 check (paid_quota_balance_usd >= 0)",
		"gift_quota_balance_usd decimal(20,8) not null default 0 check (gift_quota_balance_usd >= 0)",
		"create table if not exists user_quota_ledger_entries (",
		"user_id bigint not null references users(id) on delete cascade",
		"operator_id bigint references users(id) on delete set null",
		"create table if not exists quota_idempotency_records (",
		"ledger_entry_id bigint references user_quota_ledger_entries(id) on delete set null",
		"constraint quota_idempotency_records_user_id_key_key unique (user_id, idempotency_key)",
		"constraint quota_idempotency_records_ledger_entry_id_key unique (ledger_entry_id)",
		"create index if not exists idx_user_quota_ledger_entries_user_created_at on user_quota_ledger_entries (user_id, created_at desc)",
		"create index if not exists idx_user_quota_ledger_entries_reference on user_quota_ledger_entries (reference_type, reference_id)",
		"insert into user_wallets ( user_id, cash_balance_cny, paid_quota_balance_usd, gift_quota_balance_usd",
		"from users where users.deleted_at is null on conflict (user_id) do nothing",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
	for _, legacy := range []string{"from users where users.deleted_at is null", "users.balance"} {
		if !strings.Contains(sql, legacy) {
			t.Fatalf("migration must preserve/read legacy %q", legacy)
		}
	}
	walletStart := strings.Index(sql, "create table if not exists user_wallets (")
	walletEnd := strings.Index(sql[walletStart:], ");")
	if walletStart < 0 || walletEnd < 0 {
		t.Fatal("user_wallets table definition not found")
	}
	walletSQL := sql[walletStart : walletStart+walletEnd]
	if strings.Contains(walletSQL, "id bigserial primary key") || !strings.Contains(walletSQL, "user_id bigint primary key") {
		t.Fatal("user_wallets must use the Ent-compatible surrogate id with unique user_id")
	}
	if strings.Contains(sql, "insert into user_quota_ledger_entries") {
		t.Fatal("migration must not backfill historical ledger entries")
	}
}
