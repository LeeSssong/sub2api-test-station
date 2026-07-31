# Account Upstream Rate Multiplier Synchronization Implementation Plan

## Scope and constraints

Implement the approved design in the isolated worktree only. Do not modify
production accounts, production secrets, or deployment state. Keep the
project progress ledger at `进行中` until code is pushed, deployed, and
verified in production.

## Task 1 — Synchronization policy and pure logic

Owner: fresh implementer subagent.

- Inspect the account model and existing `extra`/configuration conventions.
- Reuse an existing manual multiplier/override setting when available; add a
  backward-compatible managed/override policy only if necessary.
- Add a small pure decision/validation function that consumes a probe
  snapshot, current account, and policy and returns either a validated update
  or a no-op/error reason.
- Add table-driven tests for valid, invalid, unchanged, manual override, and
  missing-field cases before implementation (RED -> GREEN).
- Produce a task report and review package.

## Task 2 — Repository update, audit, and cache coherence

Owner: fresh implementer subagent.

- Extend the account repository transaction used by
  `UpdateUpstreamBillingProbeSnapshot` (or introduce a focused method) to
  persist the validated multiplier for managed accounts.
- Preserve the snapshot even when synchronization is skipped or invalid.
- Write the required audit fields using the project's existing audit-log
  repository/service.
- Ensure account cache invalidation and
  `syncSchedulerAccountSnapshot` happen after a committed multiplier change;
  keep the operation idempotent.
- Add repository/service integration tests covering commit failure, no-op,
  manual override, and cache/scheduler refresh behavior.
- Produce a task report and review package.

## Task 3 — Lifecycle and periodic probe integration

Owner: fresh implementer subagent.

- Locate account create/edit/enable handlers and the periodic billing probe
  runner.
- Invoke one forced native billing probe at eligible lifecycle transitions.
- Route scheduled probe success through the same synchronization method.
- Do not add billing calls to the ordinary request forwarding path.
- Add regression tests for lifecycle triggering, scheduled triggering,
  failed-probe preservation, and usage-log multiplier selection.
- Produce a task report and review package.

## Task 4 — Independent reviews and fix loops

- After each task, a separate reviewer checks the diff, tests, error handling,
  migration/backward compatibility, and scope boundaries.
- Any Critical/Important finding is fixed by the implementer and reviewed
  again before the task is marked complete.
- Run a final whole-branch review after all task reports are green.

## Validation and handoff

Run at minimum:

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes -count=1
```

Then run targeted package tests and, if resources permit, `go test ./... -count=1`.
Inspect the final diff for accidental production/config changes. Update the
project progress ledger to reflect that the item remains `进行中` until the
user separately authorizes push, deployment, and live verification.
