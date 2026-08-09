# Task 6 Report: Externalized Balance, Billing, Rate Collection, and Controlled Writes

Date: 2026-08-10

## Scope

Implemented only in `.worktrees/fix-official-update-stuck` on
`codex/fix-official-update-stuck`, baseline `370aa1b91`. No push, merge,
deployment, production access, or GitHub Actions use occurred.

## RED Evidence

1. `cd relay-ops-service && go test ./internal/billing ./internal/collection ./internal/upstreams ./internal/adapter -run 'Test(Balance|Rate|Command)' -v`
   exited non-zero before implementation. Missing symbols were
   `BalanceCollector`, `balanceReaderFunc`, `BalanceValue`,
   `ExecuteAccountUpdate`, and `Sub2API.SendAccountUpdateCommand`.
2. `RELAY_OPS_TEST_DATABASE_URL=... go test ./internal/store -run TestBalanceFactsAreIdempotentConflictSafeAndExpire -v`
   initially failed to build because `AppendBalanceSnapshot` and
   `LatestFreshBalanceSnapshot` did not exist.
3. `RELAY_OPS_TEST_DATABASE_URL=... go test ./internal/store -run TestAccountUpdateCommandUsesExistingDurableIdempotencyAudit -v`
   initially failed with `invalid externalization command`: Task 4's durable
   claim primitive only admitted `refresh_account` and correctly rejected the
   new command before its whitelist was extended.
4. `go test ./internal/sub2api -run TestUpdateAccountUsesNarrowOfficialAdminAPI -v`
   initially failed to build because `HTTPReader.UpdateAccount` did not exist.

## GREEN Evidence

1. Focused collector and command tests:
   `go test ./internal/sub2api ./internal/billing ./internal/collection ./internal/upstreams ./internal/adapter -run 'Test(Balance|Rate|Command|UpdateAccount)' -v`
   passed. It proves provider timeout rejection, exact `FreshUntil` expiry,
   duplicate command replay without a second official call, unauthorized/unsafe
   field rejection, adapter forwarding, and official API method/path/header/body
   boundaries.
2. Real PostgreSQL 16 integration database:
   `RELAY_OPS_TEST_DATABASE_URL=postgres://task6:task6@127.0.0.1:55439/relayops_task6?sslmode=disable go test ./internal/store -run 'Test(BalanceFacts|AccountUpdateCommand)' -v`
   passed. It proves append-only balance replay, conflicting same-time fact
   rejection, expiry filtering, durable pending/accepted replay, and actor
   conflict rejection.
3. `cd relay-ops-service && go test ./...` passed.
4. `cd relay-ops-service && go test -race ./internal/billing ./internal/collection ./internal/controlplane` passed.
5. `cd relay-ops-service && go vet ./...` passed.
6. `git diff --check` passed.

## Changed Behavior

- `BalanceCollector` reads a provider balance, produces
  `BalanceSnapshot{AccountID, Amount, Currency, ObservedAt, FreshUntil, Source}`
  and appends only to `relay_ops.balance_snapshots`. Provider errors, including
  deadline expiry, yield no snapshot.
- Balance facts are immutable: identical `(account_id, observed_at)` facts replay
  successfully without a second row; changed values at that identity return
  `store.ErrConflict`; `LatestFreshBalanceSnapshot` ignores facts at or after
  `FreshUntil`.
- Sub2API balance collection reads `/v1/usage`; unsupported adapters fail closed
  in `NewBalanceReader` rather than deriving a balance from cost data.
- `AccountUpdateCommand` permits only `rate_multiplier`, `priority`, and
  `status`. It reuses the Task 4 `ClaimExternalizationCommand` /
  `CompleteExternalizationCommand` protocol with command name `account_update`.
  Pending replay is surfaced, accepted replay does not call the writer, and a
  completion failure is returned rather than ignored.
- The narrow adapter performs an authenticated `PUT
  /api/v1/admin/accounts/:id` with the command idempotency key. It sends only
  the validated field map and has no SQL path to the core database.

## Files

- `relay-ops-service/internal/billing/externalization.go`
- `relay-ops-service/internal/billing/source.go`
- `relay-ops-service/internal/billing/sub2api.go`
- `relay-ops-service/internal/billing/adapter_test.go`
- `relay-ops-service/internal/billing/externalization_test.go`
- `relay-ops-service/internal/controlplane/writes.go`
- `relay-ops-service/internal/adapter/sub2api.go`
- `relay-ops-service/internal/adapter/sub2api_test.go`
- `relay-ops-service/internal/sub2api/client.go`
- `relay-ops-service/internal/sub2api/client_test.go`
- `relay-ops-service/internal/store/postgres.go`
- `relay-ops-service/internal/store/postgres_test.go`
- `docs/project/project-progress.md`

The existing `relay-ops-service/internal/store/migrations/014_externalization_commands.sql`
already supplies the control-plane-only `balance_snapshots` and
`externalization_commands` tables; this task uses it without adding core-schema
migrations.

## Direct-Core-Write Audit

Ran:

```sh
git diff -- relay-ops-service/internal -- '*.go' '*.sql' |
  rg '^[+-].*\\b(INSERT|UPDATE|DELETE)\\b.*\\b(public\\.)?(accounts|groups|usage_logs|balance|billing)\\b'
```

Result: no matches. New SQL targets only `relay_ops.balance_snapshots` and the
existing `relay_ops.externalization_commands` table. Account changes travel
through the official API adapter.

## Self-Review And Residual Concerns

- The task preserves existing pricing snapshots as append-only rate facts;
  their `fetched_at`, content hash, evidence level, and normalized payload
  continue to provide source/watermark provenance. Balance facts preserve their
  observed/fresh/source semantics directly.
- The runtime scheduler is intentionally not rewired here: real-time routing
  and charging remain independent, and enabling collection requires the
  existing control-plane orchestration/feature-flag work before deployment.
- Local verification is complete, but project status remains `进行中` until the
  candidate is reviewed, merged to `main`, pushed, deployed, and verified
  online under the project lifecycle rules.
