# T104 Monitor V4 Persisted Snapshot Verification

Date: 2026-08-31 (Asia/Shanghai)

## Scope

Task 4 adds direct contract coverage only. Runtime behavior and response fields are unchanged. The tests cover persisted `generated_at` passthrough, the existing response field set, all supported windows (`24h`, `7d`, `30d`), selected-window requests, and retention of the last successful window after a read error.

Baseline: `main@5e6ccee143f07ee34017c25e75979b74b6bcfc77` (tree `42dda8e317725a710340b5624bbda887cd1f6a50`).

Candidate implementation commits already present:

- `23997a946` persist Monitor V4 snapshots
- `f2bf70309` strengthen snapshot evidence tests
- `3040f6901` read Monitor V4 from persisted snapshots
- `969d87fa3` harden snapshot invariants
- `e134d50bf` validate refresh projections
- `65d6fc97a` refresh snapshots periodically

## Changes

- `upstream/sub2api/backend/internal/handler/monitor_v4_handler_test.go`: assert stored snapshot `generated_at` is returned and the response envelope contains only the established fields.
- `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/api.spec.ts`: validate all three persisted windows.
- `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceView.spec.ts`: verify selected-window request and restoration to the last successful window after a failed read.

No migration, configuration, production data, or runtime source changes were made by Task 4.

## Verification

`go test -vet=off -p 1 -run 'TestAccountMonitorRepositoryProjectMonitorV4|TestMonitorV4|TestAccountMonitorRunner|TestMonitorV4Snapshots' ./internal/repository ./internal/service ./internal/handler`

- Repository: PASS (`ok`, 0.811s)
- Service: PASS (`ok`, 1.166s)
- Handler: BLOCKED before test execution by pre-existing compile errors:
  - `internal/handler/handler_wiring_test.go:11`: `ProvideHandlers` call has too few arguments
  - `internal/handler/openai_gateway_handler_test.go:1791-1800`: undefined `openAIAccountScheduleModel`

`./node_modules/.bin/vitest run src/features/monitor-v4/__tests__/api.spec.ts src/features/monitor-v4/__tests__/HybridPerformanceView.spec.ts src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts`

- BLOCKED: `upstream/sub2api/frontend/node_modules` is absent (`no such file or directory`).
- `pnpm install --offline --frozen-lockfile` was attempted and refused by the existing lockfile/override mismatch (`ERR_PNPM_LOCKFILE_CONFIG_MISMATCH`); no files were changed.

`git diff --check`: PASS.

`bash ops/assert-native-openai-concurrency-only.sh --worktree "$PWD"`: PASS (`native_concurrency_guard status=passed mode=native_account_concurrency_only`).

The required admission/slow-session search found no T104 additions in `monitor_v4.go` or `account_monitor_runner.go`.

## Boundary and risks

This candidate remains isolated. No merge, push, deployment, migration, or production claim was made. The handler package and frontend focused suite remain unverified due to the blockers above; repository/service coverage and source/diff guards are verified. Rollback is by retaining the candidate and excluding its Task 4 commit from root integration; no database rollback is needed for these test/report-only changes.

Status: `READY_FOR_ROOT_REVIEW`.
