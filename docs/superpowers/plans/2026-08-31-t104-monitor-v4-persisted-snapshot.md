# T104 Monitor V4 Persisted Snapshot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Monitor V4 aggregation to a five-minute, database-backed snapshot worker while preserving the approved logical-request success-rate and successful-request P95 semantics.

**Architecture:** A singleton-owned refresh loop computes all three rolling windows at one UTC `as_of`, then atomically replaces a small PostgreSQL derived table in one transaction. `MonitorV4Service.Snapshot` keeps live group visibility/metadata checks but reads only the latest persisted window rows; it never calls the full-window projection on a page request. `AccountMonitorRunner` owns the refresh loop lifecycle and uses the existing leader-lock helper to avoid multi-worker overlap.

**Tech Stack:** Go 1.27, `database/sql` + PostgreSQL, existing Sub2API service/repository interfaces, Gin, Vue 3/TypeScript, sqlmock and Vitest.

**Spec:** `docs/superpowers/specs/2026-08-31-t104-monitor-v4-persisted-snapshot-design.md`

## Global Constraints

- Work only in `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t104-monitor-v4-persisted-snapshot`; never modify root `main`, `docs/project/project-progress.md`, or `docs/project/native-sub-task-package-queue.md`.
- Do not merge, push, deploy, stop, migrate, restart, switch slots, or touch production from this task.
- T103 is abandoned; preserve the permanent native-only rule and do not add or restore admission, slow-session, or account-level custom concurrency controls.
- Keep `usage_logs`, `ops_error_logs`, `account_monitor_results`, and `account_monitor_bucket_terminals` as the only request/probe facts; the new table is a rebuildable derived cache.
- A real-request bucket suppresses probes; an empty settled bucket contributes exactly one probe logical request, including a failed or fail-closed missing terminal (`0/1`).
- Success rate is selected successful logical requests divided by selected logical requests; explicit client/model-unsupported errors are excluded, final user-visible failures are included, and intermediate failover attempts that end successfully are not counted.
- `ttft_p95_ms` and `latency_p95_ms` keep their names and P95 UI text but contain successful-sample trimmed means after removing `floor(n*0.05)` values from each end independently.
- `cache_hit_rate` remains successful real requests only: `cache_read_tokens / (input_tokens + cache_creation_tokens + cache_read_tokens)`; no new visible fields.
- The existing `contract_version=2`, `24h|7d|30d` windows, live visibility filtering, and frontend layout remain compatible.

---

### Task 1: Persisted Snapshot Storage Contract

**Files:**
- Create: `upstream/sub2api/backend/migrations/232_monitor_v4_snapshots.sql`
- Create: `upstream/sub2api/backend/migrations/monitor_v4_snapshots_migration_test.go`
- Create: `upstream/sub2api/backend/internal/repository/monitor_v4_snapshot_repo.go`
- Create: `upstream/sub2api/backend/internal/repository/monitor_v4_snapshot_repo_test.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v4.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go` (adapter methods only)

**Interfaces:**
- Add `service.MonitorV4StoredWindow` with `Window`, `SnapshotID`, `WindowStart`, `WindowEnd`, `GeneratedAt`, `ContractVersion`, and `map[int64]MonitorV4GroupProjection`.
- Add `service.MonitorV4SnapshotStore`:
  `LoadLatestMonitorV4Snapshot(context.Context, MonitorV4Window) (MonitorV4StoredWindow, error)` and
  `ReplaceMonitorV4Snapshots(context.Context, string, []MonitorV4StoredWindow) error`.
- Add `AccountMonitorService` forwarding methods with the same signatures; they return a clear unavailable error when the underlying account-monitor repository does not implement the store.
- Make `accountMonitorRepository` implement the store without changing the existing `AccountMonitorRepository` interface.

- [x] **Step 1: Write the failing migration and repository tests.**

  Assert that migration `232_monitor_v4_snapshots.sql` is idempotent and contains the `window` check (`24h`, `7d`, `30d`), positive `group_id`, non-negative count checks, `window_start < window_end`, `(window, group_id)` uniqueness, `snapshot_id`, `generated_at`, and the descending `(window, generated_at)` index. In sqlmock tests, assert that replacement starts a transaction, deletes the old derived rows, inserts every window/group row with one UUID, commits, and rolls back on an insert error. Assert that loading returns one `MonitorV4StoredWindow`, rejects an empty result, and rejects rows whose metadata or snapshot UUID differs.

- [x] **Step 2: Run the new tests to prove the contract is red.**

  Run from `upstream/sub2api/backend`:

  ```bash
  go test -vet=off -count=1 -run 'TestMonitorV4Snapshot|TestAccountMonitorV4Snapshot' ./internal/repository
  go test -count=1 -run 'TestMonitorV4SnapshotsMigration' ./migrations
  ```

  Expected result: compile failures for the not-yet-defined store methods and migration fixture.

- [x] **Step 3: Add the expand-only migration.**

  Create `account_monitor_v4_snapshots` with one current row per `(window, group_id)`, the scalar projection columns, nullable metric/timestamp columns, non-null `current_operational`, `snapshot_id UUID`, and `contract_version`. Use `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS`; do not alter or delete any existing table.

- [x] **Step 4: Implement atomic replace and validated load.**

  In `monitor_v4_snapshot_repo.go`, validate window/time/count invariants before opening the write transaction. `ReplaceMonitorV4Snapshots` must execute `DELETE FROM account_monitor_v4_snapshots`, insert all rows using the supplied `snapshotID`, and commit only after every insert succeeds. `LoadLatestMonitorV4Snapshot` must query the requested window ordered by `generated_at DESC, group_id`, scan nullable floats/timestamps, and fail closed if rows disagree on `snapshot_id`, `window_start`, `window_end`, `generated_at`, or `contract_version`.

- [x] **Step 5: Implement the service adapter and run the tests green.**

  Add the two forwarding methods to `AccountMonitorService`, run the Task 1 commands again, and run `gofmt` on changed Go files. Expected result: all migration and repository snapshot tests pass.

- [x] **Step 6: Commit the storage slice.**

  ```bash
  git add upstream/sub2api/backend/migrations/232_monitor_v4_snapshots.sql \
    upstream/sub2api/backend/migrations/monitor_v4_snapshots_migration_test.go \
    upstream/sub2api/backend/internal/repository/monitor_v4_snapshot_repo.go \
    upstream/sub2api/backend/internal/repository/monitor_v4_snapshot_repo_test.go \
    upstream/sub2api/backend/internal/service/monitor_v4.go \
    upstream/sub2api/backend/internal/service/account_monitor_service.go
  git commit -m "feat: persist monitor v4 snapshots"
  ```

### Task 2: Snapshot Refresh and Read Semantics

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/monitor_v4.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v4_test.go`
- Create: `upstream/sub2api/backend/internal/service/monitor_v4_snapshot_service_test.go`

**Interfaces:**
- Add `service.MonitorV4SnapshotRefresher`:
  `RefreshMonitorV4Snapshots(context.Context, time.Time) error`.
- `MonitorV4Service.SetSnapshotStore(MonitorV4SnapshotStore)` attaches the store for tests and alternate providers.
- `MonitorV4Service.RefreshMonitorV4Snapshots` enumerates active/configured group IDs, calls the existing `MonitorV4ProjectionReader` once per window using one minute-truncated `as_of`, then calls `ReplaceMonitorV4Snapshots` once.

- [x] **Step 1: Add failing service/read-path tests.**

  Extend the service tests with a fake snapshot store and native reader. Assert that `Snapshot` returns the stored `GeneratedAt` and projection while the native reader call count remains zero; visible exclusive groups are still filtered using the current user; a missing store row returns an error; and a stale stored row is returned with its original timestamp rather than triggering live projection. Add refresh tests asserting all three windows use the same `as_of`, active/configured groups are passed, one replacement call is made, and a projection/store error leaves the fake store unchanged.

- [x] **Step 2: Change the missing-probe SQL test first.**

  Update `TestAccountMonitorRepositoryProjectMonitorV4FailClosesMissingProbeBucket` to expect one selected failed probe event (`request_count=1`, `success_count=0`, `success_rate=0`, both probe fallback counts `1`, `missing_probe_terminal_count=1`) instead of a normal empty projection. Add a SQL assertion that the `selected_events` probe branch no longer filters `probe_missing IS FALSE`.

- [x] **Step 3: Implement the read path and refresh method.**

  `MonitorV4Service.Snapshot` must validate the requested window, load active/configured/available groups exactly as today, call `LoadLatestMonitorV4Snapshot`, validate stored metadata/count invariants, and pass stored projections plus stored `GeneratedAt` to `snapshotWithGroups`. It must never call `native.ProjectMonitorV4Groups`. `RefreshMonitorV4Snapshots` must use all active groups allowed by the channel-monitor group allow-list (empty allow-list means all active groups), call the native reader for `24h`, `7d`, and `30d` with a shared minute-truncated end, assign a UUID, and publish only after all windows succeed.

- [x] **Step 4: Make missing settled probe buckets fail closed.**

  In `account_monitor_repo.go`, change the probe branch of `selected_events` to include every `bucket_matrix` row where `has_real IS NOT TRUE`; use `COALESCE(probe_successful, FALSE)` and preserve `probe_missing` for the internal warning count. Keep current-bucket-before-final-minute exclusion and all real/error/model/client filters unchanged.

- [x] **Step 5: Run service and repository tests green.**

  ```bash
  go test -vet=off -count=1 -run 'TestMonitorV4|TestAccountMonitorRepositoryProjectMonitorV4' ./internal/service ./internal/repository
  gofmt -w internal/service/monitor_v4.go internal/service/monitor_v4_test.go internal/service/monitor_v4_snapshot_service_test.go internal/repository/account_monitor_repo.go internal/repository/account_monitor_repo_test.go
  git diff --check
  ```

- [x] **Step 6: Commit the refresh/read slice.**

  ```bash
  git add upstream/sub2api/backend/internal/service/monitor_v4.go \
    upstream/sub2api/backend/internal/service/monitor_v4_test.go \
    upstream/sub2api/backend/internal/service/monitor_v4_snapshot_service_test.go \
    upstream/sub2api/backend/internal/repository/account_monitor_repo.go \
    upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go
  git commit -m "fix: read monitor v4 from persisted snapshots"
  ```

### Task 3: Five-Minute Worker and Wiring

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_runner.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_runner_test.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/backend/cmd/server/wire_gen.go`

**Interfaces:**
- `AccountMonitorRunner.SetMonitorV4SnapshotRefresher(MonitorV4SnapshotRefresher)`.
- `AccountMonitorRunner.SetMonitorV4SnapshotCoordination(LeaderLockCache, *sql.DB)`.
- Internal constants/vars: five-minute refresh interval, ten-minute leader-lock TTL, and a fixed lock key `account-monitor-v4-snapshot`.

- [x] **Step 1: Add failing runner lifecycle tests.**

  Use a fake refresher and a short test interval variable to assert: `Start` performs one immediate refresh and one refresh per ticker; a second tick does not overlap a blocked first refresh; a peer-held leader lock skips the refresh; `Stop` returns and no refresh occurs afterward; and a nil refresher does not add a goroutine. Keep existing account-monitor and detector loop assertions unchanged.

- [x] **Step 2: Implement the snapshot loop.**

  Add an optional third goroutine to `AccountMonitorRunner`. It uses a `snapshotMu` try-lock, a bounded context, `tryAcquireSingletonLeaderLock(ctx, lockCache, db, "account-monitor-v4-snapshot", instanceID, 10*time.Minute)`, calls the refresher with `time.Now().UTC()`, logs only sanitized phase/error metadata, and waits on a five-minute ticker. `Stop` must cancel and join it through the existing wait group.

- [x] **Step 3: Wire the existing services.**

  `NewMonitorV4Service` should auto-detect the `MonitorV4SnapshotStore` implemented by the native `AccountMonitorService`; `ProvideAccountMonitorRunner` should attach the monitor V4 refresher, existing `leaderLockCache`, and `db` before `Start`. Update the generated call in `cmd/server/wire_gen.go`; do not alter any admission/slow-session wiring.

- [x] **Step 4: Run runner and compile checks.**

  ```bash
  go test -vet=off -count=1 -run 'TestAccountMonitorRunner|TestMonitorV4SnapshotRunner' ./internal/service
  gofmt -w internal/service/account_monitor_runner.go internal/service/account_monitor_runner_test.go internal/service/wire.go ../../cmd/server/wire_gen.go
  git diff --check
  ```

  Do not run the full `cmd/server` test/build in this fast iteration; wiring is covered by the focused runner/provider compile path.

- [x] **Step 5: Commit the worker slice.**

  ```bash
  git add upstream/sub2api/backend/internal/service/account_monitor_runner.go \
    upstream/sub2api/backend/internal/service/account_monitor_runner_test.go \
    upstream/sub2api/backend/internal/service/wire.go \
    upstream/sub2api/backend/cmd/server/wire_gen.go
  git commit -m "feat: refresh monitor v4 snapshots periodically"
  ```

### Task 4: Contract and Regression Verification

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/monitor_v4_handler_test.go`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/api.spec.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/api.ts` only if a strict snapshot timestamp validation gap is found
- Create: `docs/superpowers/reports/2026-08-31-t104-monitor-v4-persisted-snapshot-verification.md`

- [x] **Step 1: Add contract tests.**

  Keep the handler response fields unchanged and assert `generated_at` is the stored snapshot time. Assert the existing frontend validator still accepts `24h`, `7d`, and `30d`, and the view sends the selected window while retaining the last successful window on a read error. Add a test proving no new response field is required for snapshot freshness.

- [x] **Step 2: Run the focused backend verification.**

  ```bash
  go test -vet=off -p 1 -run 'TestAccountMonitorRepositoryProjectMonitorV4|TestMonitorV4|TestAccountMonitorRunner|TestMonitorV4Snapshots' ./internal/repository ./internal/service ./internal/handler
  ```

  Do not run a server build or unrelated package tests. If the focused package still hits a pre-existing compile blocker, record the exact error and keep the functional tests that can run.

- [ ] **Step 3: Run the focused frontend verification.** (blocked: candidate has no `node_modules`; see verification report)

  ```bash
  ./node_modules/.bin/vitest run src/features/monitor-v4/__tests__/api.spec.ts src/features/monitor-v4/__tests__/HybridPerformanceView.spec.ts src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts
  ```

  Do not run frontend typecheck or production build in this fast iteration.

- [x] **Step 4: Run scope and source-guard checks.**

  ```bash
  git diff --check
  bash ops/assert-native-openai-concurrency-only.sh --worktree "$PWD"
  rg -n 'AcquireOpenAIAdmission|RecordOpenAISlowSessionGuard|slow-session|admission' upstream/sub2api/backend/internal/service/monitor_v4.go upstream/sub2api/backend/internal/service/account_monitor_runner.go || true
  ```

  The final search must find no new T104 admission/slow-session code.

- [x] **Step 5: Write the verification report and update the candidate handoff.**

  Record baseline `main@5e6ccee143f07ee34017c25e75979b74b6bcfc77`, candidate commits, changed files, migration status, all commands/results, known environment blockers, no-production boundary, rollback, and residual risks. Update `docs/superpowers/handoffs/2026-08-31-t104-monitor-v4-persisted-snapshot-recovery.md` with the latest pointer and next action.

- [x] **Step 6: Commit the verification artifacts.**

  ```bash
  git add upstream/sub2api/backend/internal/handler/monitor_v4_handler_test.go \
    upstream/sub2api/frontend/src/features/monitor-v4/__tests__/api.spec.ts \
    upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceView.spec.ts \
    docs/superpowers/reports/2026-08-31-t104-monitor-v4-persisted-snapshot-verification.md \
    docs/superpowers/handoffs/2026-08-31-t104-monitor-v4-persisted-snapshot-recovery.md
  git commit -m "test: verify persisted monitor v4 snapshots"
  ```

## Acceptance

- [x] Page/window requests issue only snapshot reads; no request path calls `ProjectMonitorV4Groups`.
- [x] A singleton worker refreshes all `24h`, `7d`, and `30d` snapshots every five minutes with one shared `as_of`, and `Stop` is leak-free.
- [x] Snapshot replacement is atomic; a failed refresh leaves the previous successful generation readable.
- [x] Current visibility and group metadata remain live and exclusive-group safe.
- [x] Missing settled probe terminals are counted as one failed logical request, while real buckets suppress probes and probe attempts are never multiplied.
- [x] Success rate, trimmed P95-labelled TTFT/latency, cache hit rate, unsupported/client exclusion, and final-failure inclusion match the approved contract.
- [x] Direct backend functional coverage and touched-file checks pass; handler/frontend suites remain explicitly blocked by documented environment/baseline issues. Full package tests, server builds, frontend typecheck/build, and unrelated review are intentionally out of scope for this fast iteration.
- [x] Candidate remains isolated and is reported `READY_FOR_ROOT_REVIEW`; no merge, push, deployment, or production claim is made.

## Risks

- Migration number `232` may collide with another candidate before root integration; root must resolve the filename under the serial merge gate without changing schema semantics.
- A stopped worker leaves a stale snapshot; the API intentionally serves the last successful generation and exposes its age through the existing `generated_at` field rather than fabricating current data.
- Three full-window projections run per five-minute refresh; failures preserve the old generation, and post-merge root verification must observe query latency on the production-sized dataset.
