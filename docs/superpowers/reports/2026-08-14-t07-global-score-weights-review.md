# T07 Global Score Weights Review

## Scope

- Implemented four global score weights only: cost, success, TTFT, latency.
- Did not implement global threshold persistence or editing.
- Group score weights and thresholds remain unchanged.
- Account monitor projection DTO and AccountMonitorSchemaVersion remain unchanged.

## Commits

- 079d3ec76 fix: isolate global score weight mutation sessions
- 0ccce34e6 docs: record T07 migration schema re-review
- 453a5fc20 test: assert global score weight schema
- 780ef8d90 docs: record T07 scoped re-review
- 1b9b28c63 fix: harden global score weight controls
- 18c55ba9f docs: record T07 verification
- 6b4f13d5a Merge remote-tracking branch 'origin/main' into codex/t07-global-score-weights
- a59886002 feat: add global score weight controls
- e7f06158d feat: expose global account monitor score weights
- b9a232c86 feat: persist global account monitor score weights
- d3a0cffac docs: approve T07 implementation plan
- 3e860e2d1 docs: add T07 global score weights plan
- ae8ae2000 docs: add T07 global score weights design
- fc44bde10 docs: record delegated review authority

## Verification

- `go test ./migrations -run 'AccountMonitor.*(GlobalScoreWeights|ScoreThresholds)' -count=1`: PASS.
- `go test ./internal/repository -run 'AccountMonitorRepository.*ScoreWeights' -count=1`: PASS.
- `go test ./internal/repository -run TestMigrationsSchema -count=1`: PASS with `[no tests to run]`; repository integration tests are behind the `integration` build tag.
- `DOCKER_HOST=unix:///Users/gongtengxinwen/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true go test -tags integration ./internal/repository -run TestMigrationsSchema -count=1`: PASS.
- `go test ./internal/service -run 'AccountMonitor.*(GlobalScoreWeights|ScoreWeights|ListWindowUsesPersistedGlobalScoreWeights|QualityScore)' -count=1`: PASS.
- `go test ./internal/handler/admin -run 'AccountMonitorHandler.*ScoreWeights' -count=1`: PASS.
- `go test ./internal/server/routes -run 'AccountMonitor.*Routes' -count=1`: PASS.
- `pnpm vitest run src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts --runInBand`: BLOCKED by Vitest 2.1.9 unsupported option `--runInBand`.
- `pnpm vitest run src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`: PASS, 42 tests.
- `pnpm typecheck`: PASS.
- `go test ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes`: PASS.
- `pnpm build`: PASS.

Notes: frontend commands emit existing environment/build warnings for pnpm overrides, Node localStorage, Browserslist data age, Node child-process deprecation, and Vite dynamic/static import chunking. No warning was caused by a test failure.

## Final Fix Wave Verification

- Fix-wave base: `18c55ba9fc5431882bf60a3a3ffd3b3e39e479f0`.
- Frontend RED: `pnpm vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts` failed 3 focused regressions: switching to a group after global GET resolution left mode global, switching before GET resolution allowed the stale request to reopen global mode, and reset plus projection reload failure retained old custom weights.
- Backend service RED: `go test ./internal/service -run TestAccountMonitorServiceRejectsInvalidGlobalScoreWeights -count=1` accepted overflow-sized weights and returned no error.
- Backend handler RED: `go test ./internal/handler/admin -run TestAccountMonitorHandlerRejectsOverflowSizedGlobalScoreWeights -count=1` returned `200` and reached the repository save stub.
- Backend repository RED: `go test ./internal/repository -run TestAccountMonitorRepositoryRejectsOverflowSizedGlobalScoreWeights -count=1` attempted the SQL insert instead of rejecting locally.
- Frontend GREEN: `pnpm vitest run src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`: PASS, 45 tests.
- Backend GREEN: focused service, handler, routes, and repository score-weight suites all PASS.
- `pnpm typecheck`: PASS.
- Route success assertions now verify GET/PUT/DELETE status and payload in addition to step-up invocation counts.
- Scoped post-fix re-review (Ampere, agent `01a00217-2b83-7d01-a0cd-d3dd2c28c45d`): all three final-review findings are addressed, with no new Critical/Important breakage.
- Post scoped re-review verification:
  - `git diff --check 18c55ba9f..HEAD`: PASS.
  - `go test ./internal/service -run 'AccountMonitor.*(GlobalScoreWeights|ScoreWeights|ListWindowUsesPersistedGlobalScoreWeights|QualityScore)' -count=1`: PASS.
  - `go test ./internal/handler/admin -run 'AccountMonitorHandler.*ScoreWeights' -count=1`: PASS.
  - `go test ./internal/server/routes -run 'AccountMonitor.*Routes|TestAccountMonitorGlobalScoreWeightRoutesUseStepUpForWritesOnly' -count=1`: PASS.
  - `go test ./internal/repository -run 'AccountMonitorRepository.*ScoreWeights' -count=1`: PASS.
  - `pnpm vitest run src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`: PASS, 45 tests.
  - `pnpm typecheck`: PASS.

## Final Review Migration Schema Fix

- Fix base: `780ef8d90f8221946ada8da085e8df06c6bfe383`.
- Fix commit: `453a5fc20 test: assert global score weight schema`.
- Added PostgreSQL catalog assertions in `TestMigrationsSchema` for the `account_monitor_global_score_weights` table, singleton boolean/NOT NULL/default/primary-key/check contract, the exact four `SMALLINT NOT NULL` weight columns, audit columns/default, the four-weight sum check, and absence of all four threshold columns.
- Untagged verification: `go test ./internal/repository -run TestMigrationsSchema -count=1` PASS with `[no tests to run]`, as the schema test is behind the `integration` build tag.
- RED mutation proof: temporarily changed the migration sum check from `= 100` to `= 99`; the PostgreSQL-backed test failed at the new sum-contract assertion with `expected four-weight sum CHECK constraint`. The migration was restored before commit.
- GREEN verification: `DOCKER_HOST=unix:///Users/gongtengxinwen/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true go test -tags integration ./internal/repository -run TestMigrationsSchema -count=1` PASS after restoration.
- Scoped re-review by Kant (agent `01a00239-d0c6-7482-9971-74d83433ab77`): `TestMigrationsSchema` global score weights schema coverage is ADDRESSED; no new Critical/Important breakage.

## Root Final Review Fix Wave

- Fix base: `0ccce34e620493a3734690901058fe4719b34094`.
- Fix commit: `079d3ec76 fix: isolate global score weight mutation sessions`.
- I1 root cause: global PUT/DELETE handlers read the live `scoreDialogMode` after awaited API/reload calls, so a canceled global dialog followed by a newly opened group dialog could be closed or receive the wrong completion label from the stale global mutation.
- I1 fix: every opened dialog owns an immutable dialog generation, mode, group ID, and mutation generation. Save/reset use only that captured scope, and may update dialog state, success/error messages, returned reset weights, or saving state only while the exact session still owns the dialog. Cancel, scope switch, or opening another dialog invalidates the old mutation without canceling an already accepted API write.
- Frontend RED: `pnpm vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts` failed exactly two new race regressions; both stale global PUT and stale global DELETE completions closed the newly opened group dialog (`expected true, received false`).
- Frontend GREEN: `pnpm vitest run src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts` PASS, 47 tests; `pnpm typecheck` PASS.
- M1 fix: `TestMigrationsSchema` now asserts the PostgreSQL catalog contains a separate `>= 0` CHECK for each of `cost_weight`, `success_weight`, `ttft_weight`, and `latency_weight`.
- M1 RED mutation proof: temporarily changed only `latency_weight CHECK (latency_weight >= 0)` to `>= 1`; the real PostgreSQL integration test failed with `expected non-negative CHECK constraint for latency_weight`. The migration was restored immediately and has no final diff.
- M1 GREEN: `DOCKER_HOST=unix:///Users/gongtengxinwen/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true go test -tags integration ./internal/repository -run TestMigrationsSchema -count=1` PASS; focused migration tests PASS.

## Scope Guards

- `git diff --check efa0ef54cb432e784796add380727bc5366d2d06..HEAD`: PASS.
- `git diff --name-only efa0ef54cb432e784796add380727bc5366d2d06...HEAD`: reviewed. Files are limited to T07 spec/plan, T07 account-monitor backend/frontend files, one migration and migration test, this report, and root governance files brought in by REFRESH_REQUIRED.
- Threshold guard: PASS. Threshold field strings appear only in existing group request/repository paths or editable group types; `223_account_monitor_global_score_weights.sql` and the global request type do not contain threshold fields.
- Scheduler/external-primary/GitHub Actions/root governance guard: PASS for T07-owned changes. No `.github/workflows`, `external-primary`, scheduler, Top-K, or quota files are changed by T07. Root governance files changed only through the REFRESH_REQUIRED merge from `origin/main`.
- AccountMonitorSchemaVersion guard: PASS. Value remains `5`; branch diff does not change the schema-version constant.

## REFRESH_REQUIRED

- Integrated main SHA: `d3a0cffac3220f51d4872be62c094442246b5249`.
- Merge commit: `6b4f13d5a`.
- Conflict summary: none.
- Root governance files preserved from `origin/main`: `docs/project/native-sub-incremental-delivery-constraints.md`, `docs/project/native-sub-task-package-queue.md`, and `docs/project/project-progress.md`.
- Post-refresh backend focused tests: PASS.
- Post-refresh PostgreSQL/testcontainers migration schema test with `DOCKER_HOST` and `TESTCONTAINERS_RYUK_DISABLED=true`: PASS.
- Post-refresh frontend focused tests and typecheck: PASS.

## Review Findings

- Task 1 reviewer Boole: spec compliant, task quality approved, no findings.
- Task 2 reviewer Kuhn: spec compliant, task quality approved. The deferred route-test strength note is resolved in the final fix wave.
- Task 3 reviewer Laplace: spec compliant, task quality approved, no findings.
- Final whole-branch reviewer found three blocking issues: global dialog scope/race, discarded reset response on projection reload failure, and overflow-prone global weight validation. All three have focused RED/GREEN regression coverage and implementation fixes in this wave.
- Scoped post-fix re-review by Ampere: all three findings addressed; no new Critical/Important breakage.
- Final reviewer Carver found one additional Important issue: `TestMigrationsSchema` lacked PostgreSQL assertions for `account_monitor_global_score_weights`. Fix commit `453a5fc20` adds the schema coverage, and scoped re-review by Kant marked it addressed with no new Critical/Important breakage.
- Root final review found Important I1 (stale global mutation completion could affect a later group dialog) and Minor M1 (per-weight non-negative PostgreSQL CHECK constraints were not individually asserted). Fix commit `079d3ec76` adds focused RED/GREEN race coverage, immutable mutation ownership, and real-PostgreSQL per-weight constraint coverage. A refreshed whole-branch reviewer is still required before handoff.

## Release Precheck And Rollback Notes

- Migration is expand-only and creates only `account_monitor_global_score_weights`.
- App rollback leaves the singleton table unused by old code.
- `downtime_required`: no.
- Stop or roll back if global and group weights overwrite each other, PUT/DELETE bypass step-up, storage errors fall back silently, global thresholds appear, or account monitor ranking diverges from backend projection.

## Status

REVIEWING. Final reviewer findings are fixed, scoped re-reviewed, and locally verified; pending refreshed final whole-branch reviewer before `READY_FOR_ROOT_REVIEW`.
