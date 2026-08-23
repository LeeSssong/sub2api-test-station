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
