# T85 Monitor V4 真实请求成功率与探测兜底去重 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (or superpowers:subagent-driven-development) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Monitor V4 改为真实请求成功率主指标，并让主动探测兜底每个分组/5 分钟桶只贡献一个逻辑请求，同时跳过已知不可调度账号。

**Architecture:** 复用现有 `usage_logs`、`ops_error_logs` 和 `account_monitor_results`。仓储 SQL 先构造真实请求事件，再对探测按 run/bucket 聚合并执行真实优先仲裁；服务层使用 `Account.IsSchedulableAt` 收紧自动探测池；handler/frontend 将 V4 合同升级为请求数、成功数和独立 P95 字段。

**Tech Stack:** Go、database/sql、PostgreSQL CTE/PERCENTILE_CONT、sqlmock、Vue 3、TypeScript、Vitest、pnpm。

**Spec:** `docs/superpowers/specs/2026-08-29-t85-monitor-v4-real-request-probe-dedup-design.md`

## Global Constraints

- 保留 5 分钟桶、真实请求优先、最后一分钟主动探测兜底、同桶不混用。
- 真实成功定义保持 `actual_cost > 0` 且 `usage_completeness <> 'unknown'`；真实错误进入分母。
- 探测全失败的已关闭兜底桶计为一次失败逻辑请求；探测自身无结果/未知不选中。
- 不新增迁移、表、配置、账务写入、历史回填或 GitHub Actions。
- 所有发布动作遵守验收站与主站授权门禁；本计划只负责本地实现和直接相关验证。

### Task 1: Replace V4 repository projection with request-weighted source arbitration

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go:281-428`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go:318-352`

**Interfaces:**
- Consumes: `[]service.MonitorV2GroupAccountScope`, window bounds and bucket size.
- Produces: `MonitorV4GroupProjection` with `SuccessRate`, `RequestCount`, `SuccessCount`, source counts, independent P95 values/sample counts, and current status.

- [ ] **Step 1: Write RED sqlmock cases**

Add repository tests that expect `real_events`, `error_events`, `probe_runs`, `probe_buckets`, `real_request_count`, `success_count`, and the last-minute/current-bucket guard. Return rows in the new column order and assert that real-failure buckets suppress probes, mixed probe buckets return one request, all-failed probes return zero success, and TTFT/latency sample counts are independent.

- [ ] **Step 2: Run the focused repository tests and confirm RED**

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestAccountMonitorRepository.*V4|TestAccountMonitorRepositoryProjectMonitorV4' -count=1
```

Expected: FAIL because the current projection still scans v1 availability columns and has no request-weighted fields.

- [ ] **Step 3: Change the projection type**

Replace the old availability-only fields in `MonitorV4GroupProjection` with `SuccessRate *float64`, `RequestCount`, `SuccessCount`, `RealRequestCount`, `RealSuccessCount`, `ProbeFallbackBucketCount`, `ProbeFallbackRequestCount`, nullable `TTFTP95MS`/`LatencyP95MS`, their sample counts, `SourceUpdatedAt`, and `CurrentOperational`.

- [ ] **Step 4: Implement the CTE projection**

Keep the existing scope validation and arguments. Build CTEs in this order: `scopes`, `groups`, `buckets`; `usage_events` with non-unknown usage and success=`actual_cost > 0`; `error_events` with non-token status >= 400 errors not matching a usage request; `real_events` deduplicated by group/account/logical request key; `real_buckets`; `probe_rows`; `probe_runs` (one result per group/bucket/run, any success wins and successful timings use minimum non-null values); `probe_buckets` (one result per group/bucket); `selected_events` (real source wins, otherwise one probe logical event only for closed or final-minute current buckets); and `aggregate` for request/success counts and independent P95s. Return nullable SQL P95 values and scan with `sql.NullFloat64`; do not coalesce no-sample P95 to zero.

- [ ] **Step 5: Run focused repository tests and confirm GREEN**

Run the command from Step 2. Expected: PASS, including `mock.ExpectationsWereMet()`.

- [ ] **Step 6: Commit the repository projection**

```bash
git add upstream/sub2api/backend/internal/repository/account_monitor_repo.go upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go upstream/sub2api/backend/internal/service/account_monitor_types.go
git commit -m "feat: weight monitor v4 by logical requests"
```

### Task 2: Stop automatic probes for accounts already blocked by native scheduling gates

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go:2097-2145,1218-1220`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- Consumes: existing `Account` snapshots and `Account.IsSchedulableAt(time.Time)`.
- Produces: automatic monitor runs that call `probeAccount` only for accounts currently eligible under native scheduling gates.

- [ ] **Step 1: Write RED unit coverage**

Add a focused `listPool` test using active, schedulable, probe-enabled accounts. Assert that a future `TempUnschedulableUntil` and API-key quota exhaustion are excluded while an eligible account remains.

- [ ] **Step 2: Run the focused service test and confirm RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitor.*Pool|TestListPool' -count=1
```

Expected: FAIL because `listPool` currently checks only the boolean `Schedulable` field.

- [ ] **Step 3: Implement native gate reuse**

Capture one `now := time.Now().UTC()` in `listPool` and require `account.IsSchedulableAt(now)` before the existing account/group active-probe switches. Preserve existing status and group membership behavior.

- [ ] **Step 4: Run focused service tests**

```bash
go test ./internal/service -run 'TestAccountMonitor.*(Pool|Run|Probe)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the probe-pool gate**

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/service/account_monitor_service_test.go
git commit -m "fix: skip blocked accounts in monitor probe pool"
```

### Task 3: Upgrade the V4 service, handler, frontend contract, and card

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/monitor_v4.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v4_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/monitor_v4_handler.go`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/types.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/api.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/HybridPerformanceGroupCard.vue`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/HybridPerformanceView.vue`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/api.spec.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/channelMonitorV2.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/channelMonitorV2.ts`

**Interfaces:**
- Consumes: new `MonitorV4GroupProjection` fields from Task 1.
- Produces: API contract version `2` and a single user-visible “成功率” metric.

- [ ] **Step 1: Write RED service and frontend contract tests**

Update service tests to assert null success rate for zero requests and exact percentage conversion for `success_count/request_count`. Update frontend fixtures to use `contract_version: '2'`, `success_rate`, request counts, nullable P95 fields, and reject a v1 availability-only response.

- [ ] **Step 2: Run focused tests and confirm RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestMonitorV4' -count=1
cd ../../frontend
pnpm vitest run src/features/monitor-v4/__tests__/api.spec.ts src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts src/features/monitor-v4/__tests__/HybridPerformanceView.spec.ts
```

Expected: FAIL because the current service/handler/frontend still expose contract version `1` and availability fields.

- [ ] **Step 3: Map the v2 backend contract**

Set `MonitorV4ContractVersion` to `2`. Compute a nullable percentage in `snapshotWithGroups`; preserve null when `RequestCount == 0`. Map nullable P95s and all source/request counters into the service group and handler DTO. Remove v1 availability, bucket totals, and metric fallback fields from the v2 JSON response.

- [ ] **Step 4: Update frontend validation and rendering**

Change `MONITOR_V4_CONTRACT_VERSION` to `'2'`. Validate nullable `success_rate` in 0–100, non-negative integer request/success counters with success <= request, source counters with real/probe invariants, and nullable P95 fields. Render the ring label as “成功率”, show `成功 N/M 次请求`, and render `--` for null P95. Keep window switching and refresh behavior unchanged.

- [ ] **Step 5: Run focused tests and confirm GREEN**

Run the commands from Step 2. Expected: PASS, including old-contract rejection and null P95 cases.

- [ ] **Step 6: Commit the API/frontend contract**

```bash
git add upstream/sub2api/backend/internal/service/monitor_v4.go upstream/sub2api/backend/internal/service/monitor_v4_test.go upstream/sub2api/backend/internal/handler/monitor_v4_handler.go upstream/sub2api/frontend/src/features/monitor-v4 upstream/sub2api/frontend/src/i18n/locales/zh/channelMonitorV2.ts upstream/sub2api/frontend/src/i18n/locales/en/channelMonitorV2.ts
git commit -m "feat: expose monitor v4 request success rate"
```

### Task 4: Full direct verification and handoff

**Files:**
- Create: `docs/handoffs/2026-08-29-t85-monitor-v4-real-request-probe-dedup-handoff.md`

- [ ] **Step 1: Format and diff-check**

```bash
cd upstream/sub2api/backend
gofmt -w internal/repository/account_monitor_repo.go internal/repository/account_monitor_repo_test.go internal/service/account_monitor_types.go internal/service/account_monitor_service.go internal/service/account_monitor_service_test.go internal/service/monitor_v4.go internal/service/monitor_v4_test.go internal/handler/monitor_v4_handler.go
git diff --check
```

- [ ] **Step 2: Run direct backend verification**

```bash
go test ./internal/repository -run 'TestAccountMonitorRepository.*V4|TestAccountMonitorRepositoryProjectMonitorV4' -count=1
go test ./internal/service -run 'TestMonitorV4|TestAccountMonitor.*(Pool|Run|Probe)' -count=1
go build ./cmd/server
```

- [ ] **Step 3: Run direct frontend verification**

```bash
cd ../frontend
pnpm vitest run src/features/monitor-v4
pnpm typecheck
pnpm build
```

- [ ] **Step 4: Self-review the diff and contract**

Check that no V4 response contains v1 availability fields, SQL has no window-outside fallback, empty/current-ineligible buckets are excluded, probe failures are aggregated once, and no migration/config/GitHub Actions files changed.

- [ ] **Step 5: Write the handoff and commit verification evidence**

Record baseline `main` SHA, candidate commits, changed files, test output summaries, no-migration/config status, `downtime_required=unverified until root preflight`, rollback, and remaining production risks in the handoff, then commit.

## Done When

- V4 contract version `2` returns request-weighted success rate and independent P95 values.
- Real buckets always suppress probes; eligible probe fallback buckets count once.
- Known blocked accounts no longer receive automatic monitor probes.
- Direct Go/Vitest tests, server build, frontend typecheck/build, gofmt, and diff-check pass.
- Candidate remains isolated and `READY_FOR_ROOT_REVIEW`; no production deployment is claimed.
