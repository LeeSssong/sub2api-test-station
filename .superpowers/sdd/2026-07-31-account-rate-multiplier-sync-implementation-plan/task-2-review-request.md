# Task 2 independent review request

Please independently review commit
`30d29a8f612bdf0f24e31a34e382d997b8c5f2b7` for Task 2 of account upstream
rate-multiplier synchronization.

## Review focus

1. The existing probe snapshot CAS, transaction, durable scheduler outbox, and
   post-commit scheduler snapshot behavior remain intact.
2. The repository exclusively uses Task 1's pure policy/decision API and
   preserves snapshots for manual, unchanged, and invalid cases.
3. Managed updates write the multiplier only when the persisted policy and
   expected current multiplier still match; confirm a concurrent manual edit
   cannot be overwritten.
4. The audit event includes old/new values, `native_billing`, timestamp,
   trigger, policy, system actor, and is emitted only after a successful
   multiplier-changing commit.
5. The audit failure contract is appropriate: the existing asynchronous audit
   service cannot roll back a valid committed probe change.
6. Check scope: no lifecycle trigger wiring, no ordinary request probe, and no
   production data/configuration changes.

## Evidence

- Targeted repository tests cover managed update/audit/cache, commit failure,
  manual override, no-op, invalid probe, and lifecycle trigger context.
- `go test ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes -count=1` exited 0.
- `go test ./cmd/server -run '^$' -count=1` passed.
- `git diff --check` passed.
