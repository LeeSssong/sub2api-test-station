# T05 用量页仅使用原生 Sub 数据实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让管理员用量页在初载、刷新、筛选、分页、统计、错误详情和用量详情路径中不再调用外部控制面，并只使用原生 Sub 用量数据。

**Architecture:** 仅收敛 `UsageView.vue` 的页面级统计读取编排：删除 `ReadModelStatus` 状态条、控制面导入、accounting decision/ledger 请求和 external-primary 覆盖统计逻辑，让 `loadStats()` 直接提交原生 stats。共享 `controlPlane.ts`、`ReadModelStatus.vue`、`useReadModelFreshness.ts`、外部化配置和共享测试保持不变。

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, Vitest, Vue Test Utils, pnpm.

## Global Constraints

- 管理员用量页在初载、刷新、筛选、分页、统计重载、错误详情和用量详情路径中，对 `/api/v1/xingqiao/**` 以及其他外部控制面 API 实现零请求。
- `/api/v1/admin/usage` 和 `/api/v1/admin/usage/stats` 始终是用量列表与统计卡片的原生数据源。
- 删除用量页上的外部控制面状态条、完整性信息、外部 ledger 决策、外部 ledger 请求和外部覆盖统计逻辑。
- 不新增“原生数据源”“完整性”或其他替代常驻状态条；正常状态只显示既有用量页内容。
- 保持用量列表、统计卡片、图表、筛选、分页、导出、错误请求、错误详情、管理员用量详情和 T03-R1 成本异常核对入口语义不变。
- 不修改成本数值规则、成本/利润公式、利润页、账号监控页、后端账务模型、调度、external-primary、relay-ops 主路径或 GitHub Actions。
- 不清理共享 `controlPlane.ts`、`ReadModelStatus.vue`、`useReadModelFreshness.ts`、外部化决策配置或共享测试。
- 不修改 `docs/project/project-progress.md` 或 `docs/project/native-sub-task-package-queue.md`。
- 不执行合并 `main`、推送、部署、生产写操作或线上状态修改；本任务最终只报告 `READY_FOR_ROOT_REVIEW`。
- 预计为纯前端页面改动；无数据库迁移、配置变化或停机要求，`downtime_required=false`。

---

## 文件结构与责任

- Modify: `upstream/sub2api/frontend/src/views/admin/UsageView.vue` — 删除页面级外部控制面状态、accounting decision/ledger 调用、外部 ledger 解析和统计覆盖逻辑；保留原生用量列表、统计、图表、错误请求、详情和异常入口。
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts` — 删除旧控制面 mock 与 shadow/external-primary/degraded 测试契约，增加或改写原生唯一数据源和无状态条回归断言。
- Create: `docs/superpowers/reports/2026-08-14-t05-usage-native-only-implementation.md` — 实施报告，记录 RED/GREEN 测试、变更范围、自审、风险和候选 SHA。
- Create: `docs/superpowers/reports/2026-08-14-t05-usage-native-only-task-review.md` — 独立任务 reviewer 报告。
- Create: `docs/superpowers/reports/2026-08-14-t05-usage-native-only-final-review.md` — 整分支 reviewer 报告。

共享文件保持不变：`upstream/sub2api/frontend/src/api/controlPlane.ts`、`upstream/sub2api/frontend/src/components/admin/ReadModelStatus.vue`、`upstream/sub2api/frontend/src/composables/useReadModelFreshness.ts`、`upstream/sub2api/frontend/src/config/externalizationFlags.ts` 及共享 control-plane/client 测试。

## Task 1: 改写用量页测试契约为原生单读

**Files:**

- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts`
- Create: `docs/superpowers/reports/2026-08-14-t05-usage-native-only-implementation.md`

**Interfaces:**

- Consumes: existing hoisted mocks `list`, `getStats`, `getSnapshotV2`, `getById`, `listErrorLogs`, `routeQuery`, and mount helper `mountRouteFilteredUsageView()`.
- Produces: tests that require `UsageView.vue` to stop importing/calling `controlPlaneAPI.decision` and `controlPlaneAPI.ledger`, preserve native stats/list behavior, and render no control-plane status text.

- [ ] **Step 1: Remove the page-level control-plane mock from the test setup.**

  In `UsageView.spec.ts`, edit the hoisted mock destructuring so it no longer defines `controlPlaneLedger` or `controlPlaneDecision`:

  ```ts
  const { list, exportList, listCostExceptions, getStats, getSnapshotV2, getById, getModelStats, listErrorLogs, routeQuery, aoaToSheet, sheetAddAoa, saveAs, xlsxWrite } = vi.hoisted(() => {
  ```

  Inside the hoisted return object, remove:

  ```ts
  controlPlaneLedger: vi.fn(),
  controlPlaneDecision: vi.fn(),
  ```

  Delete the `vi.mock('@/api/controlPlane', ...)` block entirely. This makes any remaining production import of `@/api/controlPlane` use the real module and gives the RED failure if the page still calls the external control plane during the test environment.

- [ ] **Step 2: Remove control-plane reset defaults from `beforeEach()`.**

  In `beforeEach()`, delete:

  ```ts
  controlPlaneLedger.mockReset().mockResolvedValue({
    items: {},
    freshness: { completeness: 'complete', calculation_version: 'accounting-v1' },
  })
  controlPlaneDecision.mockReset().mockResolvedValue({ page: 'accounting', requested_mode: 'legacy_only', effective_mode: 'legacy_only', use_external: false, degraded: false, reason: 'legacy_default' })
  ```

- [ ] **Step 3: Replace the old shadow-mode test with a native-only normal-state test.**

  Replace the test named `keeps legacy usage pagination and rows in shadow mode while loading ledger freshness` with:

  ```ts
  it('uses native usage stats and rows without rendering control-plane status', async () => {
    getStats.mockResolvedValueOnce({
      total_requests: 7, total_input_tokens: 10, total_output_tokens: 20, total_cache_tokens: 0,
      total_cache_creation_tokens: 0, total_cache_read_tokens: 0, total_tokens: 30,
      total_cost: 2, total_actual_cost: 1, total_account_cost: 1, average_duration_ms: 100,
    })

    const wrapper = mountRouteFilteredUsageView()
    await flushPromises()

    expect(list).toHaveBeenCalledWith(expect.objectContaining({ page: 1 }), expect.anything())
    expect(getStats).toHaveBeenCalledWith(expect.objectContaining({ start_date: expect.any(String), end_date: expect.any(String) }))
    expect((wrapper.vm as any).usageStats).toMatchObject({
      total_requests: 7,
      total_cost: 2,
      total_actual_cost: 1,
      total_tokens: 30,
    })
    expect(wrapper.text()).not.toContain('控制面暂时不可用')
    expect(wrapper.text()).not.toContain('完整性')
    expect(wrapper.text()).not.toContain('来源：现有系统')
    expect(wrapper.text()).not.toContain('来源：控制面')
  })
  ```

- [ ] **Step 4: Delete the old control-plane degradation and external-primary tests.**

  Remove these tests completely:

  ```ts
  it.each([401, 403])('keeps usage local when the control plane returns %s', async (status) => {
    ...
  })
  ```

  ```ts
  it('applies trusted accounting totals while preserving legacy detail rows and filters', async () => {
    ...
  })
  ```

  Do not replace them with tests that mock `@/api/controlPlane`; T05's page contract is that the page has no control-plane dependency.

- [ ] **Step 5: Add a refresh/date-range regression if no existing test covers stats reload.**

  If the spec file already has a test that triggers `onDateRangeChange`, `refreshData`, or the filters' refresh event and asserts native `getStats`/`list` calls, add the no-status assertions from Step 3 to that existing test. If no such test exists, add this test near the route-filter tests:

  ```ts
  it('refreshes usage data through native endpoints without control-plane status', async () => {
    const wrapper = mountRouteFilteredUsageView()
    await flushPromises()
    getStats.mockClear()
    list.mockClear()

    await wrapper.getComponent(UsageFiltersStub).vm.$emit('refresh')
    await flushPromises()

    expect(list).toHaveBeenCalledWith(expect.objectContaining({ page: 1 }), expect.anything())
    expect(getStats).toHaveBeenCalledWith(expect.objectContaining({ start_date: expect.any(String), end_date: expect.any(String) }))
    expect(wrapper.text()).not.toContain('控制面暂时不可用')
    expect(wrapper.text()).not.toContain('完整性')
  })
  ```

  If `UsageFiltersStub` exposes a helper method instead of `$emit('refresh')`, use the existing helper in the same file and keep the assertions exactly focused on native list/stats and absent status text.

- [ ] **Step 6: Run the focused RED test.**

  From `upstream/sub2api/frontend`, run:

  ```bash
  pnpm exec vitest run src/views/admin/__tests__/UsageView.spec.ts
  ```

  Expected before Task 2: FAIL because `UsageView.vue` still imports or calls control-plane code, still renders `ReadModelStatus`, or the removed mocks expose a test/runtime failure tied to that dependency. A syntax or fixture failure means fix the test edit before proceeding.

- [ ] **Step 7: Start the implementation report.**

  Create `docs/superpowers/reports/2026-08-14-t05-usage-native-only-implementation.md` with this exact structure:

  ```md
  # T05 用量页仅使用原生 Sub 数据实施报告

  ## Baseline

  - Baseline SHA: 4c5f0d1587004cfb4d7386d0c947f157678d8803
  - Branch: codex/t05-usage-native-only

  ## RED

  - Command: `cd upstream/sub2api/frontend && pnpm exec vitest run src/views/admin/__tests__/UsageView.spec.ts`
  - Result: RED evidence must be filled with the actual failing assertion summary before the report is committed.

  ## GREEN

  - Not run yet; Task 2 must replace this line with focused and neighboring GREEN evidence before commit.

  ## Scope Review

  - Not reviewed yet; Task 2 must replace this line with the final scope review before commit.
  ```

  Replace only the RED result line with the actual focused failure summary. Do not leave angle-bracket text in the committed report.

## Task 2: 删除 UsageView 外部控制面状态和 accounting ledger 逻辑

**Files:**

- Modify: `upstream/sub2api/frontend/src/views/admin/UsageView.vue`
- Modify: `docs/superpowers/reports/2026-08-14-t05-usage-native-only-implementation.md`

**Interfaces:**

- Consumes: existing `adminAPI.usage.getStats(params)`, `adminAPI.usage.list(params, options)`, `UsageStatsCards`, chart props, filters, error tab, usage detail dialog, and cost exception table.
- Produces: same `usageStats`, `usageLogs`, endpoint stats arrays, chart state, filters, tabs, detail modal state, and exception table behavior, with no `ReadModelStatus`, no `controlPlaneAPI`, and no external stats overwrite.

- [ ] **Step 1: Remove the `ReadModelStatus` template block.**

  Delete this block from the top of `UsageView.vue`:

  ```vue
  <ReadModelStatus
    v-if="controlPlaneResponse || controlPlaneDegraded"
    :generated-at="readModel.generatedAt.value"
    :completeness="readModel.completeness.value"
    :calculation-version="readModel.calculationVersion.value"
    :degraded="controlPlaneDegraded || readModel.degraded.value"
    :source-label="renderSource === 'external' ? '控制面' : '现有系统'"
    @retry="loadStats(true)"
  />
  ```

- [ ] **Step 2: Remove control-plane imports and state.**

  Delete these imports from the `<script setup>` block:

  ```ts
  import { controlPlaneAPI, type ControlPlaneResponse } from '@/api/controlPlane'
  import { useReadModelFreshness } from '@/composables/useReadModelFreshness'
  import { resolveTrustedPageDecision } from '@/config/externalizationFlags'
  import ReadModelStatus from '@/components/admin/ReadModelStatus.vue'
  ```

  Delete these state declarations:

  ```ts
  const controlPlaneResponse = ref<ControlPlaneResponse<unknown> | null>(null)
  const controlPlaneDegraded = ref(false)
  const renderSource = ref<'legacy' | 'external'>('legacy')
  const readModel = useReadModelFreshness(controlPlaneResponse)
  ```

- [ ] **Step 3: Make `loadStats()` native-only.**

  In `loadStats()`, remove:

  ```ts
  await loadControlPlaneLedger()
  ```

  Keep the rest of the success path intact:

  ```ts
  usageStats.value = s
  inboundEndpointStats.value = s.endpoints || []
  upstreamEndpointStats.value = s.upstream_endpoints || []
  endpointPathStats.value = s.endpoint_paths || []
  ```

  Do not change cost values, token values, average duration, endpoint stats, request type conversion, `force ? { nocache: 1 } : {}`, or `statsReqSeq` protection.

- [ ] **Step 4: Delete external ledger helpers.**

  Delete the entire `loadControlPlaneLedger()` function and the entire `accountingLedger()` function. After deletion, `UsageView.vue` must contain no references to:

  ```text
  controlPlaneAPI
  ControlPlaneResponse
  ReadModelStatus
  useReadModelFreshness
  resolveTrustedPageDecision
  controlPlaneResponse
  controlPlaneDegraded
  renderSource
  loadControlPlaneLedger
  accountingLedger
  ```

- [ ] **Step 5: Run the focused GREEN test.**

  From `upstream/sub2api/frontend`, run:

  ```bash
  pnpm exec vitest run src/views/admin/__tests__/UsageView.spec.ts
  ```

  Expected result: PASS. If it fails outside the intended page dependency removal, inspect the failing assertion and keep the fix inside `UsageView.vue` or `UsageView.spec.ts`; do not edit shared control-plane files.

- [ ] **Step 6: Run the T05 neighboring regression suite.**

  From `upstream/sub2api/frontend`, run:

  ```bash
  pnpm exec vitest run \
    src/views/admin/__tests__/UsageView.spec.ts \
    src/components/admin/usage/__tests__/UsageTable.spec.ts \
    src/components/usage/__tests__/UsageDetailDialog.spec.ts \
    src/api/admin/__tests__/usageDetail.spec.ts \
    src/components/admin/usage/__tests__/CostExceptionTable.spec.ts \
    src/views/admin/ops/components/__tests__/OpsErrorLogTable.spec.ts \
    src/views/admin/ops/components/__tests__/OpsErrorDetailModal.spec.ts \
    src/api/admin/__tests__/errorDetailResponse.spec.ts \
    src/api/admin/__tests__/admin.usage.spec.ts
  ```

  Expected result: PASS for all files.

- [ ] **Step 7: Run static and scope checks.**

  From `upstream/sub2api/frontend`, run:

  ```bash
  pnpm typecheck
  ```

  From the worktree root, run:

  ```bash
  git diff --check
  rg -n "controlPlaneAPI|ControlPlaneResponse|ReadModelStatus|useReadModelFreshness|resolveTrustedPageDecision|controlPlaneResponse|controlPlaneDegraded|renderSource|loadControlPlaneLedger|accountingLedger|/api/v1/xingqiao|/xingqiao" upstream/sub2api/frontend/src/views/admin/UsageView.vue upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts
  git status --short --branch
  ```

  Expected result: `pnpm typecheck` and `git diff --check` pass; the `rg` command returns no matches in `UsageView.vue` or `UsageView.spec.ts`; changed files remain scoped to `UsageView.vue`, `UsageView.spec.ts`, the T05 spec/plan, and T05 reports.

- [ ] **Step 8: Complete the implementation report and commit.**

  Update `docs/superpowers/reports/2026-08-14-t05-usage-native-only-implementation.md` so it has no placeholders and includes:

  ```md
  ## GREEN

  - Focused UsageView: `cd upstream/sub2api/frontend && pnpm exec vitest run src/views/admin/__tests__/UsageView.spec.ts` — record the actual pass/fail summary.
  - Neighboring regression suite: record the exact multi-file Vitest command from Step 6 and its actual pass/fail summary.
  - Typecheck: `cd upstream/sub2api/frontend && pnpm typecheck` — record the actual pass/fail summary.
  - Scope checks: record the exact `git diff --check`, `rg`, and `git status --short --branch` results.

  ## Changed Files

  - `upstream/sub2api/frontend/src/views/admin/UsageView.vue`
  - `upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts`
  - `docs/superpowers/reports/2026-08-14-t05-usage-native-only-implementation.md`

  ## Scope Review

  - No shared control-plane files changed.
  - No docs/project progress or queue files changed.
  - No backend, migration, config, GitHub Actions, cost formula, profit page, account monitor, scheduler, external-primary, or relay-ops main-path changes.
  - `downtime_required=false`.
  ```

  Replace each angle-bracket segment with actual evidence. Then commit only the scoped implementation and report files:

  ```bash
  git add upstream/sub2api/frontend/src/views/admin/UsageView.vue \
    upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts \
    docs/superpowers/reports/2026-08-14-t05-usage-native-only-implementation.md
  git commit -m "fix: make usage page native-only"
  git status --short --branch
  ```

## Task 3: Independent task review, final review, and handoff evidence

**Files:**

- Create: `docs/superpowers/reports/2026-08-14-t05-usage-native-only-task-review.md`
- Create: `docs/superpowers/reports/2026-08-14-t05-usage-native-only-final-review.md`

**Interfaces:**

- Consumes: approved spec `docs/superpowers/specs/2026-08-14-t05-usage-native-only-design.md`, this plan, baseline SHA `4c5f0d1587004cfb4d7386d0c947f157678d8803`, and candidate implementation commit from Task 2.
- Produces: independent review reports and final `READY_FOR_ROOT_REVIEW` handoff evidence. It does not produce a merge, push, deployment, production write, or project progress update.

- [ ] **Step 1: Dispatch an independent task reviewer.**

  Use a fresh GPT-5.5 / medium reviewer agent with read-only instructions. Ask it to inspect:

  ```text
  Baseline: 4c5f0d1587004cfb4d7386d0c947f157678d8803
  Candidate: HEAD
  Spec: docs/superpowers/specs/2026-08-14-t05-usage-native-only-design.md
  Plan: docs/superpowers/plans/2026-08-14-t05-usage-native-only.md
  Files: UsageView.vue, UsageView.spec.ts, implementation report
  ```

  Required findings format:

  ```md
  # T05 Task Review

  ## Verdict

  PASS or FAIL

  ## Findings

  - Severity: Critical/Important/Minor
  - File:
  - Line:
  - Issue:
  - Required change:

  ## Checks

  - Zero UsageView control-plane imports/calls:
  - No shared control-plane edits:
  - Native usage/error/detail/exception paths preserved:
  - Tests adequate:
  ```

- [ ] **Step 2: Save the task review report and fix load-bearing findings.**

  Save the reviewer output to `docs/superpowers/reports/2026-08-14-t05-usage-native-only-task-review.md`. If the verdict is FAIL or it includes Critical/Important findings, make the smallest scoped fix, rerun the focused and neighboring tests from Task 2, update the implementation report with a fix section, commit the fix, and request a scoped re-review before continuing.

- [ ] **Step 3: Dispatch a final whole-branch reviewer.**

  After task review passes, use a fresh GPT-5.5 / medium reviewer agent with read-only instructions. Ask it to review the whole branch since baseline `0c26c9afc28419daa42fe54f3fc2d3bbf7bef2ea`, including the spec, plan, implementation, tests, and reports. It must verify the branch is suitable for `READY_FOR_ROOT_REVIEW` and that no forbidden actions or files are included.

- [ ] **Step 4: Save final review and resolve load-bearing findings.**

  Save the reviewer output to `docs/superpowers/reports/2026-08-14-t05-usage-native-only-final-review.md`. If the final reviewer reports Critical/Important findings, make the smallest scoped fix, rerun relevant tests and checks, update reports, commit the fix, and request one scoped re-review.

- [ ] **Step 5: Run final fresh verification on the completed branch.**

  From `upstream/sub2api/frontend`, run:

  ```bash
  pnpm exec vitest run \
    src/views/admin/__tests__/UsageView.spec.ts \
    src/components/admin/usage/__tests__/UsageTable.spec.ts \
    src/components/usage/__tests__/UsageDetailDialog.spec.ts \
    src/api/admin/__tests__/usageDetail.spec.ts \
    src/components/admin/usage/__tests__/CostExceptionTable.spec.ts \
    src/views/admin/ops/components/__tests__/OpsErrorLogTable.spec.ts \
    src/views/admin/ops/components/__tests__/OpsErrorDetailModal.spec.ts \
    src/api/admin/__tests__/errorDetailResponse.spec.ts \
    src/api/admin/__tests__/admin.usage.spec.ts
  pnpm typecheck
  ```

  From the worktree root, run:

  ```bash
  git diff --check HEAD~3..HEAD
  rg -n "controlPlaneAPI|ControlPlaneResponse|ReadModelStatus|useReadModelFreshness|resolveTrustedPageDecision|controlPlaneResponse|controlPlaneDegraded|renderSource|loadControlPlaneLedger|accountingLedger|/api/v1/xingqiao|/xingqiao" upstream/sub2api/frontend/src/views/admin/UsageView.vue upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts
  git status --short --branch
  ```

  Expected result: tests and typecheck pass; `git diff --check` passes; `rg` returns no matches; branch contains only T05 spec, plan, reports, `UsageView.vue`, and `UsageView.spec.ts`.

- [ ] **Step 6: Report `READY_FOR_ROOT_REVIEW`.**

  Final handoff must include:

  ```text
  READY_FOR_ROOT_REVIEW
  Baseline SHA: 0c26c9afc28419daa42fe54f3fc2d3bbf7bef2ea
  Spec/plan baseline after design: 4c5f0d1587004cfb4d7386d0c947f157678d8803
  Candidate SHA: use the final `git rev-parse HEAD` value.
  Tests: list each actual command and observed result from final fresh verification.
  Changed files: list the actual scoped files from `git diff --name-only 0c26c9afc28419daa42fe54f3fc2d3bbf7bef2ea..HEAD`.
  migration/config: none
  downtime_required: false
  rollback: revert T05 candidate commits through the reviewed local/host release chain; no DB/config/data rollback
  risks: native stats become the only displayed usage totals; production network zero-request verification remains for root release validation
  ```

## Rollback

Revert the T05 candidate commits through the reviewed local/host release chain. No database, migration, configuration, or production data rollback is required. Do not merge, push, deploy, or touch production from this feature task.

## Done when

- `UsageView.vue` has no page-level control-plane imports, state, status row, decision/ledger request, or external stats overwrite.
- Initial load, refresh, filtering, pagination, stats, errors, error detail, usage detail, and cost exceptions continue to use native/local paths and pass scoped tests.
- Shared control-plane files, docs/project queue/progress files, backend, config, GitHub Actions, cost formula, profit page, account monitor, scheduler, external-primary, and relay-ops main path are untouched.
- Implementation is committed, independently task-reviewed, whole-branch reviewed, and reported as `READY_FOR_ROOT_REVIEW`; no merge, push, deploy, or production write has happened.
