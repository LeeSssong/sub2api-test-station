# T34 Monitor V2 Native Probe Verification

Status: READY_FOR_ROOT_REVIEW
Branch: `codex/t34-native-probe-monitor-v2`

## Scope

- Monitor V2 contract v7 projects only native `account_monitor_results` data through `AccountMonitorService`.
- Fixed windows: 24h/24x1h, 7d/28x6h, 30d/30x24h.
- Group state uses a fresh native success within two monitor intervals; historical buckets are operational when any schedulable account succeeds.
- Metrics are native success-sample TTFT P50 and average latency; legacy flagship/TPS/P95/usage-log repository path is removed.
- Native result retention is 30 days while existing account-monitor aggregation remains 7 days.

## TDD Evidence

- Task 1 RED: service projection tests failed before native projection types/methods existed.
- Task 1 GREEN: `go test ./internal/service -run 'TestAccountMonitorProjectMonitorV2Groups' -count=1`.
- Task 2 RED: repository projection tests failed before the native batch SQL projection existed.
- Task 2 GREEN: `go test ./internal/repository ./internal/service -run 'TestAccountMonitorRepositoryProjectMonitorV2Groups|TestAccountMonitorRunAllUsesThirtyDayResultRetention' -count=1`.
- Task 3/4 GREEN: `go test ./internal/service ./internal/repository ./internal/handler ./cmd/server -run 'TestMonitorV2|TestAccountMonitor' -count=1`.

## Frontend Evidence

- Existing `frontend/src/features/monitor-v2/__tests__/api.spec.ts` updated in place for v7; no duplicate API contract file added.
- `pnpm vitest run src/features/monitor-v2/__tests__` passed: 8 files, 36 tests.
- `pnpm typecheck` passed.
- `pnpm build` passed.
- Correction pass RED: Monitor V2 API tests rejected the newly specified `not_provided` fixture only after the RED assertion was added; the pre-fix parser accepted it.
- Correction pass GREEN: TTFT label is now `首字速度：` (English `First token speed: `); metric state is limited to `available | insufficient_data`; old TTFT P95/TPS/latency/P95/not_provided/baseRate locale keys were removed.
- Correction pass: `pnpm vitest run src/features/monitor-v2/__tests__` passed (8 files, 36 tests), `pnpm typecheck` passed, and `pnpm build` passed again.
- Source guard confirmed no Monitor V2 consumer references the removed locale keys.
- The local Vite route redirected to login because no authenticated fixture/session was available. Authenticated desktop and 390px screenshots, overflow checks, and pixel-level visual acceptance remain **UNVERIFIED**; login redirect is not counted as page acceptance. Component-level Monitor V2 view tests cover the v7 card layout and fixed-width behavior.

## Source Guards

- `monitor_v2.go` has no `usage_logs` or `ChannelMonitorService` dependency.
- `ProvideMonitorV2Service` accepts `AccountMonitorService` native projection and settings only.
- `internal/repository/wire.go` removes `NewMonitorV2Repository` while preserving legacy Channel Monitor and Channel Monitor V2 providers.
- `cmd/server/wire_gen.go` constructs Monitor V2 after `AccountMonitorService` and keeps legacy channel monitor wiring intact.
- `git diff --check` passed.

## Root Review Fixes

- Timeline bucket matching now truncates both native points and service bounds to microseconds; regression uses non-zero nanosecond `now` and a microsecond native point.
- Frontend validation now requires exactly 24/28/30 timeline points for 24h/7d/30d; rejection coverage is in the existing API contract test.
- Group card status is visible in the first-line badge, while the second line retains the three metrics without repeating the availability badge text.

## Refresh Verification

- Pre-refresh candidate: `2e4ac36bc118b27c4c40a554284e09cfd90e207d`; branch and worktree were clean.
- Final refresh baseline/target: `main@9d2fed6ebae4163fca546cc54f31a1018ceeb488`.
- Final refresh merge: `ff12adaf082587b583c5374c6428d6ff17a16086`, parents `dc9150e8ad7107aec05c78c3825e06bdf6c6e901` and `9d2fed6ebae4163fca546cc54f31a1018ceeb488`.
- Merge completed without conflicts. Relative to the prior `f96297c631548c56d89da3b7162d9007dd2c7a36` target, latest main added only the T35 production-close progress entry; no task-authored edit was made to the global ledger.
- Backend refresh test passed: `go test ./internal/service ./internal/repository ./internal/handler ./cmd/server -run 'TestMonitorV2|TestAccountMonitor' -count=1`; `go build ./cmd/server` passed.
- Frontend refresh test passed: `pnpm vitest run src/features/monitor-v2/__tests__` (8 files, 36 tests).
- `pnpm typecheck` and `pnpm build` passed after refresh.
- Source guard and both `git diff --check 9d2fed6ebae4163fca546cc54f31a1018ceeb488 HEAD` / working-tree `git diff --check` passed.
- Relative to target main, the candidate contains only the approved T34 design/plan/report/handoff, native Account Monitor projection, Monitor V2 v7 service/handler/wiring, and Monitor V2 card/API/type/locale/test paths. T35 code remains in the main baseline and is not part of the T34 delta. No migration, configuration, production, or GitHub Actions path is present.
- Migration change: none. Configuration change: none. Expected `downtime_required=false`; root release preflight remains authoritative.
- Rollback: before integration, retain target main unchanged and discard this candidate; after a root-authorized integration, revert the T34 integration commit and redeploy the prior blue-green source/image.

No merge into root main, push, deployment, production-state edit, or task-authored queue/progress-ledger edit was performed.
