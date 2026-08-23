# Task 1 report — Ent wallet/ledger schema and expand-only migration

- Status: implementation complete; ready for root review/integration.
- Commit: `feat: add native quota wallet schema` (see commit SHA in handoff message).
- Added Ent schemas for `UserWallet`, `UserQuotaLedgerEntry`, and `QuotaIdempotencyRecord`, including decimal(20,8) balances/deltas/snapshots, user/operator relations, record-type validation, idempotency scope, and reference/time indexes.
- Added expand-only migration `225_user_quota_wallet_ledger.sql`. It preserves `users.balance`, creates the three tables and nonnegative balance checks, initializes only non-deleted users with cash/gift zero and paid quota copied from `users.balance`, and uses `ON CONFLICT (user_id) DO NOTHING` without historical ledger backfill.
- Added migration contract coverage in `migrations/user_quota_wallet_ledger_migration_test.go`.
- No configuration changes and no production data writes; migration is expand-only and expected `downtime_required=false` subject to root release preflight.

## Validation

- `go test ./migrations -run UserQuotaWalletLedger -v` — passed.
- `go test ./ent/... ./migrations/...` — passed.
- `git diff --check` — passed.

Remaining validation: root must run its normal merged-main migration/release preflight and deployment verification. No frontend or service behavior is included in this task.
