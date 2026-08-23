# Task 2 report

Status: implemented.

## Changes

- Added `QuotaWalletService` and the wallet coordinator for recharge, refund,
  paid-first usage consumption, and legacy add/subtract/set adjustments.
- Added decimal-based inputs, source-specific deltas, before/after summaries,
  stable business errors, and idempotency result handling.
- Added Ent/SQL repository support for wallet initialization from the legacy
  `users.balance`, `FOR UPDATE` locking, atomic wallet/projection updates,
  ledger insertion, idempotency records, and paginated ledger reads.
- Added focused table-driven service tests for paid-first fallback,
  insufficient balance, recharge/refund rules, and legacy-set validation.

## Verification

`go test ./internal/service -run QuotaWallet -v` — passed.

`go test ./internal/service ./internal/repository -run 'QuotaWallet|UserRepo' -count=1` — passed.

## Concerns / follow-up

- Repository integration/concurrency tests require a live PostgreSQL fixture and
  were not run in this task window.
- Existing legacy `UserRepository` methods remain available; Task 3 should wire
  production call sites and the compatibility endpoints to this coordinator.
- Ent generation and migration/schema files were intentionally not touched.

## Fix round 1

- Idempotency fingerprints now derive from immutable request inputs, so retries
  replay even when the current paid/gift split or refund gift-clearing delta has
  changed. Replay restores paid/gift consumption fields from the ledger.
- Wallet, projection, and ledger DECIMAL values are persisted as exact decimal
  strings; no `InexactFloat64` conversion is used for wallet writes.
- Legacy atomic user-repository balance operations now use the quota coordinator
  when configured by the native repository constructor.
- Added deterministic repository contract/sqlmock tests for projection reads,
  idempotency fingerprint stability, rollback/locking contracts, and replay
  consumption preservation.

Verification:

`go test ./internal/service ./internal/repository -run 'QuotaWallet|UserRepo' -count=1` — passed.

Live PostgreSQL rollback and concurrent-refund integration tests remain pending
because no PostgreSQL fixture was available in this worktree.

## Fix round 2

- Idempotent replay now reads `record_type`; paid/gift consumed fields are
  restored only for `usage_consumption`. Recharge, refund, and legacy replay
  results keep those fields zero.
- `ApplyRedeemBalanceAdjustment` and `DeductAvailableBalance` route through the
  coordinator when configured while retaining redeem floor-at-zero and
  available-deduction min(balance, amount) semantics.
- Added a sqlmock replay test covering usage split restoration and refund
  consumption-field suppression.

Verification:

`go test ./internal/service ./internal/repository -run 'QuotaWallet|UserRepo' -count=1` — passed.
