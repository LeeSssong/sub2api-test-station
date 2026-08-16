# T12 Task 1 Implementation Report

## Scope

Implemented the isolated, append-only `account_probe_cost_logs` migration and its service/repository contract. `usage_logs` remains untouched; the ledger has no user/API-key identity columns, uses `account_id ... ON DELETE RESTRICT`, and does not backfill historical rows.

## Changes

- Added migration `224_account_probe_cost_logs.sql` with the probe-kind, completeness, and outcome checks; nullable `DECIMAL(20,10)` account cost; unique probe run IDs; and created/account/group time indexes.
- Added migration contract coverage for required clauses, forbidden `usage_logs` mutation, forbidden placeholder identities, and cascade deletion.
- Added service DTOs/enums and `AccountProbeCostRepository` interface.
- Added SQL repository with parameterized append, immutable `probe_run_id` idempotency, typed conflict errors, half-open `[from,to)` aggregation, and nullable aggregate cost semantics.
- Added SQL-mock coverage for complete and partial rows, identical/conflicting duplicates, aggregation, empty windows, and query error propagation.

## Verification

- `go test ./migrations -run TestAccountProbeCostLogsMigration -count=1` — PASS
- `go test ./internal/repository -run 'TestAccountProbeCost' -count=1` — PASS (7 tests)
- `git diff --check` — PASS

## Red-state evidence

Before implementation, the migration test failed because `224_account_probe_cost_logs.sql` was missing, and the repository test failed to compile because the service contract and repository constructor were absent.

## Migration/release posture

Add-only migration; no historical rows or data rewrites. No configuration changes. `downtime_required` was not evaluated in this isolated task and no deployment is authorized from this worktree. Rollback is to stop probe writes/reads while retaining the table and rows.

## Concerns

- Aggregate cost is represented as `nil` if any grouped row has missing/non-complete cost; downstream tasks must map that to the specified incomplete status rather than `$0.00`.
- Production migration precheck and deployment remain root-task responsibilities.

## Review round 1 fixes

- Canonicalized `CreatedAt` to UTC PostgreSQL microsecond precision before insert and during immutable duplicate comparison; the duplicate regression test supplies nanoseconds and mocks the microsecond database read-back.
- Replaced nullable `float64` cost values with nullable `shopspring/decimal.Decimal`, including read-back comparison and aggregate output; added exact `1234567890.1234567890` round-trip/aggregation coverage.
- Strengthened all window SQL mocks to require both `created_at >= $1` and `created_at < $2`.

Fix verification: migration contract PASS; repository focused tests PASS (8); `git diff --check` PASS.
