# T137 账号卡片账号级统一有效观测修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让账号卡片所有质量指标只使用账号级统一有效观测流，并将去重后的真实请求终态与主动探测终态直接相加。

**Architecture:** 在现有账号监控 repository 中建立唯一账号级观测选择规则：真实请求按逻辑请求终态去重，主动探测按运行身份去重，二者不按时间桶互斥。service 只生成一次账号快照，再把同一快照挂到分组卡片；分组只保留上下文、账务和调度投影。前端直接展示后端成功数和总数，不再自行推导。

**Tech Stack:** Go、PostgreSQL SQL、Gin handler、Vue 3、TypeScript、Vitest、Go sqlmock。

**Spec:** `docs/superpowers/specs/2026-09-08-t137-account-card-account-only-observation-design.md`

## Global Constraints

- 账号卡片只展示该账号的数据；分组不得覆盖账号级观测指标。
- 有效观测等于去重后的真实请求终态加去重后的主动探测终态，同一五分钟桶也必须相加。
- 请求成功与 `actual_cost` 解耦；`actual_cost` 只用于账务、成本和利润。
- 主动探测不得进入 `usage_logs`、收入、成本、利润或业务 token 统计。
- 无观测显示 `--`；存在失败观测且无成功观测才显示 `0%`。
- 无数据库迁移、无历史回填、无生产数据写入；只从干净且已推送的根 `main` 发布。

## 文件地图

- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`：统一账号观测 SQL、真实终态去重、探测终态去重、窗口/时间线/累计查询。
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`：锁定 SQL 合同和聚合字段。
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`：单次账号快照投影，禁止分组窗口覆盖账号观测。
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`：稳定成功数、失败数、真实请求数和主动探测数合同。
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`：跨分组一致、质量输入和调度投影。
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go`：若现有响应组装需要透传统一字段，仅做原生字段适配。
- Test: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`：响应恒等式和无来源冲突。
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`：补齐统一计数字段类型。
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`：直接使用后端成功数，移除前端 `total - error_count` 推导。
- Test: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`：卡片数据一致性和空值语义。

### Task 1: Lock the account observation contract in repository tests

**Files:**
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`

**Interfaces:**
- Consumes: existing `accountMonitorRepository` methods and sqlmock helpers.
- Produces: failing cases for zero-cost success, real-plus-probe addition, probe dual-projection deduplication, logical-request final-state deduplication, and the `request_count = success_count + error_count` invariant.

- [ ] **Step 1: Add RED sqlmock cases**

Add cases that assert the query contract contains both terminal probe statuses, does not use `actual_cost > 0` as the success predicate for unified observations, and returns rows for:

```text
real success actual_cost=0 + probe success => request=2, success=2, error=0
real success actual_cost=0 + real failure => request=2, success=1, error=1
same probe run in result and terminal tables => request=1, success=1
retry failure followed by final success => request=1, success=1
```

Use the existing repository test style and return explicit columns for every scanned field.

- [ ] **Step 2: Run the focused repository tests and verify failure**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'Test.*AccountMonitor|Test.*GroupReal' -count=1
```

Expected: FAIL because current SQL still marks success using `actual_cost > 0`, omits the required direct addition behavior, or returns incompatible query text.

- [ ] **Step 3: Commit the RED tests**

```bash
git add upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go
git commit -m "test: define account-level unified observation contract"
```

### Task 2: Implement one account-level unified observation query

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`

**Interfaces:**
- Consumes: `usage_logs`, `ops_error_logs`, `account_monitor_results`, `account_monitor_bucket_terminals`.
- Produces: `AccountMonitorWindowAggregate` and timeline rows with `RequestCount`, `SuccessCount`, `ErrorCount`, `SuccessRate`, `RealRequestCount`, `ProbeRequestCount`, `LastObservedAt`.

- [ ] **Step 1: Build the real-request terminal CTE**

Replace every monitoring success predicate based on `u.actual_cost > 0` with a terminal-state projection:

```sql
-- success is derived from the final request outcome, never from cost
successful = TRUE  -- complete usage row with no matching counted terminal error
successful = FALSE -- counted terminal error without a final successful usage row
```

Preserve the existing client-error, unsupported-model, unknown-completeness, and logical-request exclusion rules. Deduplicate by logical request key and prefer the final successful usage row over intermediate errors.

- [ ] **Step 2: Build the active-probe terminal CTE**

Union explicit `success` and `failed` probe results from both native projection tables, deduplicate by stable `run_id + account_id`, and retain every distinct terminal run in the requested window. Do not group probes into one row per five-minute bucket and do not suppress probes when real traffic exists in the same bucket.

- [ ] **Step 3: Union real and probe observations by account**

Create a reusable internal SQL shape or shared query builder for window aggregates and timelines. The union must satisfy:

```sql
request_count = COUNT(*)
success_count = COUNT(*) FILTER (WHERE successful)
error_count = COUNT(*) FILTER (WHERE NOT successful)
success_rate = success_count / NULLIF(request_count, 0)
```

Use cost columns only for `Revenue`, `AccountCost`, `CostComplete`, and profitability; never for `successful`.

- [ ] **Step 4: Apply the same selector to lifetime counts and timelines**

Remove the old “real bucket suppresses probe” behavior from lifetime and timeline queries. Timeline buckets aggregate all selected observations and use success/failed state only for color and TTFT sample eligibility.

- [ ] **Step 5: Run focused repository tests and verify GREEN**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'Test.*AccountMonitor|Test.*GroupReal' -count=1
```

Expected: PASS for all newly added and existing directly related repository tests. If existing sqlmock expectations are stale, update only those expectations to the new contract and retain behavior assertions.

- [ ] **Step 6: Commit the repository implementation**

```bash
git add upstream/sub2api/backend/internal/repository/account_monitor_repo.go upstream/sub2api/backend/internal/service/account_monitor_types.go upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go
git commit -m "fix: unify account monitor real and probe observations"
```

### Task 3: Project one account snapshot into all group cards

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`

**Interfaces:**
- Consumes: the account-level aggregate and timeline from Task 2.
- Produces: top-level account rows and `groups[].accounts[]` rows whose account observation fields are identical for the same `account_id`.

- [ ] **Step 1: Add RED service cases**

Add a service fixture where account `11` belongs to groups `7` and `8`, with one account-level aggregate and different group-level aggregates. Assert both group cards receive the same `request_count`, `success_count`, `error_count`, `success_rate`, TTFT, timeline, quality evidence, and quality score. Assert only group context, profitability, and scheduler rank can differ.

- [ ] **Step 2: Run the focused service tests and verify failure**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'Test.*AccountMonitor' -count=1
```

Expected: FAIL because `projectGroupWindowQuality` currently overwrites account fields from group windows.

- [ ] **Step 3: Remove group metric overwrite and reuse the account snapshot**

Change `projectGroupWindowQuality` so it starts from the already projected account row and only attaches group context. Do not assign group aggregate values to `row.SampleCount`, `row.SuccessRate`, `row.TTFTP50MS`, `row.LatencyP95MS`, `row.Timeline`, `row.RequestCount`, or `row.ErrorCount`.

Use the account-level quality evidence for account quality scoring and scheduler quality input. Keep group profitability sourced from business `usage_logs` fields and keep scheduler ranking returned by `schedulerProjection.Project`.

- [ ] **Step 4: Run service and handler tests GREEN**

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/handler/admin -run 'Test.*AccountMonitor|Test.*Monitor' -count=1
```

Expected: PASS, including response invariants that `request_count = success_count + error_count` and no card changes metrics when group membership context changes.

- [ ] **Step 5: Commit service projection changes**

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/service/account_monitor_service_test.go upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go
git commit -m "fix: keep account monitor cards account-scoped"
```

### Task 4: Make the frontend consume the backend contract directly

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Test: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

**Interfaces:**
- Consumes: `success_count`, `request_count`, `error_count`, `success_rate`, and unified timeline from the admin monitor API.
- Produces: card metadata and success-rate display that do not infer success from error subtraction.

- [ ] **Step 1: Add RED component cases**

Add tests where `request_count=10`, `success_count=10`, `error_count=0`, `success_rate=100`, and where `request_count=2`, `success_count=0`, `error_count=2`, `success_rate=0`. Assert metadata is `10/10` and `0/2`, respectively. Add a zero-sample case asserting `--`.

- [ ] **Step 2: Run frontend tests and verify failure**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

Expected: FAIL because the component currently derives successful count using `total - error_count`.

- [ ] **Step 3: Implement direct field projection**

Replace the computed successful count with:

```ts
const successfulRequestCount = computed(() => Math.max(0, Number(props.account.success_count) || 0))
```

Keep zero-sample handling in `successRate`: only show `0%` when `request_count > 0`; show `--` when there are no observations. Update TypeScript interfaces without changing public route names.

- [ ] **Step 4: Run frontend tests, typecheck, and build**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts
pnpm typecheck
pnpm build
```

Expected: PASS with no TypeScript or production build errors.

- [ ] **Step 5: Commit frontend changes**

```bash
git add upstream/sub2api/frontend/src/api/admin/accountMonitor.ts upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts
git commit -m "fix: render account monitor counts from unified fields"
```

### Task 5: Direct verification, integration handoff, and release preparation

**Files:**
- Modify: `docs/project/project-progress.md` only in the root release-control phase.
- Modify: `docs/project/native-sub-task-package-queue.md` only in the root release-control phase.
- Create: direct test evidence under `/Users/gongtengxinwen/.codex/release-evidence/sub2api/` after verification.

**Interfaces:**
- Consumes: T137 spec, all implementation commits, direct test outputs, and release preflight.
- Produces: one candidate commit/tree suitable for root integration; no deployment from the feature worktree.

- [ ] **Step 1: Run direct backend verification**

```bash
cd upstream/sub2api/backend
go test ./internal/repository ./internal/service ./internal/handler/admin -run 'Test.*AccountMonitor|Test.*Monitor' -count=1
go build ./cmd/server
gofmt -w internal/repository/account_monitor_repo.go internal/service/account_monitor_service.go internal/service/account_monitor_types.go
```

- [ ] **Step 2: Run direct frontend verification**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor
pnpm typecheck
pnpm build
```

- [ ] **Step 3: Run scope and diff checks**

```bash
git diff --check
 git status --short
```

Expected: only T137 implementation files and its tests are changed in the candidate worktree; no migrations, credentials, production data, or release-control files are changed in the candidate.

- [ ] **Step 4: Create a handoff summary**

Record baseline SHA, candidate SHA, changed files, direct tests, build/typecheck results, migration/config/data status, `downtime_required` unknown until root preflight, rollback method, and remaining risks. Candidate ends at `READY_FOR_ROOT_REVIEW`.

- [ ] **Step 5: Root integration and production release**

Only after candidate verification and root release-control review:

```bash
git fetch origin
git log --oneline <candidate> ^origin/main
git diff --stat origin/main...<candidate>
git diff --name-status origin/main...<candidate>
```

Merge to a clean root `main`, run direct regressions on root `main`, push `origin/main`, then run the reviewed local/host blue-green release chain from root `main`. Use the user-authorized path “快速部署主站，不同步测试站”; if preflight returns `downtime_required=true`, stop before any stop/restart/switch and request explicit downtime authorization.

- [ ] **Step 6: Online verification and ledger closeout**

Verify public health endpoints, running source commit/tree/image identity, admin monitor API, and the following scenarios without generating artificial user traffic: same account in two group tabs has identical account metrics; zero-cost successful requests count as success; same-bucket real plus probe observations are both counted; probe duplicate projections count once; billing/profit fields do not change from probes.

After production verification, update the root ledger with production commit/tree, image digest, release record, test-station source/tree unchanged, explicit version difference, rollback slot, and unresolved risks.
