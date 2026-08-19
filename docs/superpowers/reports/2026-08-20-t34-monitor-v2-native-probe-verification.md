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
- `pnpm vitest run src/features/monitor-v2/__tests__` passed: 8 files, 35 tests.
- `pnpm typecheck` passed.
- `pnpm build` passed.
- Desktop and 390px viewport checks were exercised against the local Vite route; the route correctly redirects to login without an authenticated fixture. Component-level Monitor V2 view tests cover the v7 card layout and fixed-width behavior.

## Source Guards

- `monitor_v2.go` has no `usage_logs` or `ChannelMonitorService` dependency.
- `ProvideMonitorV2Service` accepts `AccountMonitorService` native projection and settings only.
- `internal/repository/wire.go` removes `NewMonitorV2Repository` while preserving legacy Channel Monitor and Channel Monitor V2 providers.
- `cmd/server/wire_gen.go` constructs Monitor V2 after `AccountMonitorService` and keeps legacy channel monitor wiring intact.
- `git diff --check` passed.

No merge, push, deployment, production-state, main, queue, or progress-ledger changes were performed.
