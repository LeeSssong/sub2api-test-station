# T13 NewAPI Rate Multiplier Registration Implementation

## Status

- Candidate branch: `codex/newapi-rate-multiplier-registration`.
- Approved refreshed baseline: `45de05dffa560f8d2f92695258d4928e6d18ac34`.
- Pre-Task-2/3 branch tip: `0730603c806779ebdedff9ed18ecac6be4134f4a`.
- Root `main` at handoff time: `92db09644`; this dirty candidate was not
  refreshed with that main and must be reconciled by the root controller before
  integration.
- Final candidate SHA is recorded in the task handoff after the implementation
  commit is created.

## Implemented Scope

- Parses only the top-level NewAPI log `other.group_ratio` from an exact usage
  match and rejects refunds, fuzzy matches, invalid ratios, OAuth, non-API-key
  accounts, and accounts with a successful native billing probe.
- Uses a five-minute Claim/Complete/Release CAS lease and a Beijing natural-day
  refresh key. Completion atomically updates `accounts.rate_multiplier`, the
  sanitized registration snapshot, and the scheduler outbox.
- Keeps the generic usage-cost evidence identity unchanged: an unsupported-only
  probe is not sufficient for that ledger. T13 registration uses a narrow proof
  that accepts either an explicit NewAPI balance source or an unsupported native
  probe, and still completes only after an exact valid NewAPI log match.
- Reuses the single NewAPI log lookup for registration and cost evidence while
  preserving lookup failure reasons.
- Shows the current registered multiplier and a compact administrator-only
  registration badge, safe timestamps, and the manual-overwrite warning.
- Clears registration metadata when account identity changes or a successful
  native billing probe supersedes the NewAPI-derived value.

## Verification

- PASS: three directly related service tests for generic-ledger exclusion,
  unsupported-plus-exact-log registration, and single-query reuse.
- PASS: two directly related repository CAS contract tests.
- PASS: compile-only checks for `internal/service`, `internal/repository`, and
  `cmd/server`.
- PASS: `gofmt -d` produced no output.
- PASS: `git diff --check` exited 0.
- Not run: frontend Vitest. `node_modules/.bin/vitest` is absent, and the prior
  dependency installation attempt failed with `ENOTFOUND registry.npmjs.org`.
  No frontend test pass is claimed.
- Not run by explicit user scope: full repository tests, typecheck, frontend
  build, integration database execution, extra reviews, production checks, or
  deployment validation.

## Release Properties

- Database migrations: none.
- Configuration changes: none.
- Dependency or lockfile changes: none.
- GitHub Actions changes: none.
- Production data changes during implementation: none.
- Expected `downtime_required=false` because this candidate changes application
  code and existing JSON metadata only. The authoritative value remains the root
  release-chain preflight result.

## Rollback And Risks

- Before deployment, drop or revert the T13 candidate commits from the root
  integration candidate.
- After deployment, revert the T13 code commits and redeploy the previous
  verified main. Do not destructively rewrite account data: multipliers and
  sanitized registration snapshots already written by successful requests must
  be handled by an explicitly approved data operation if reversal is required.
- Remaining risks: frontend component tests were not executable in this
  worktree; repository SQL is covered by focused mock contracts rather than a
  live integration database; root main has advanced and requires controlled
  refresh/conflict handling before merge.
