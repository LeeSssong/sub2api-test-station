# T04 账号监控仅使用原生 Sub 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让账号监控页面在所有页面加载与写后重载场景中只读取原生 `/admin/accounts/monitor`，移除控制面状态和 `/xingqiao/**` 请求，同时保持现有卡片与交互无感不变。

**Architecture:** 仅收敛 `AccountMonitorView.vue` 的页面级读取编排：删除控制面导入、状态条、外部投影校验和数据源切换，让现有带 AbortController/generation 保护的 `load()` 直接提交原生 monitor 投影。共享 `controlPlane.ts`、`ReadModelStatus.vue`、`useReadModelFreshness.ts` 及其测试保持不变。

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, Vitest, Vue Test Utils, pnpm.

## Global Constraints

- `/admin/accounts/monitor` 是账号监控页面唯一数据源；首次加载、时间窗切换、用户刷新和写后重载均对 `/xingqiao/**` 零请求。
- 正常状态不新增“原生数据源”“完整性”或其他常驻状态条；仅移除现有控制面状态条。
- 原生首次加载失败且无快照时显示原生错误空态与重试入口；已有快照后失败时保留最后成功快照并显示原生错误与重试入口。
- 保持现有账号卡片、布局、筛选、分组、时间窗、弹窗、按钮、轮询和写操作交互；不修改卡片业务字段或 API 契约。
- T04 不修改 T05/T06/T07/T08/T09 范围，不清理共享控制面文件，不引入 external-primary、relay-ops 主路径或新平行控制面。
- 不修改后端、数据库、迁移、权限、路由、配置或 GitHub Actions；预计 `downtime_required=false`。
- 实施完成后只能报告 `READY_FOR_ROOT_REVIEW`；只有根任务发送包含目标 `main` SHA 的 `AUTHORIZE_MERGE_TO_MAIN` 后才能合并，推送和部署按既有本地/宿主发布链执行。

---

## 文件结构与责任

- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue` — 删除页面级控制面依赖，保留原生加载/失败/交互逻辑；如 RED 测试证明首次失败缺少空态，仅添加最小原生错误空态分支。
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts` — 删除外部 shadow/external-primary/degraded 旧契约，增加原生唯一数据源、零 `/xingqiao/**`、正常无状态条、首次失败和代表性写后重载回归。
- Modify: `docs/project/project-progress.md` — 记录计划已获批准并进入实施，后续记录候选提交、复审和 `READY_FOR_ROOT_REVIEW`。
- Create: `.superpowers/sdd/2026-08-12-t04-account-monitor-native-only/progress.md` — 本计划的 SDD 恢复账本（git-ignored，不进入提交）。
- Create: `.superpowers/sdd/2026-08-12-t04-account-monitor-native-only/task-1-brief.md` — 从计划提取的唯一实现任务简报。
- Create: `.superpowers/sdd/2026-08-12-t04-account-monitor-native-only/task-1-report.md` — implementer 的 TDD、测试和自审报告。

共享文件保持不变：`upstream/sub2api/frontend/src/api/controlPlane.ts`、`upstream/sub2api/frontend/src/components/admin/ReadModelStatus.vue`、`upstream/sub2api/frontend/src/composables/useReadModelFreshness.ts` 及共享 control-plane/client 测试。

## Task 1: 收敛账号监控为原生单读并完成最小验证

**Files:**

- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `docs/project/project-progress.md`

**Interfaces:**

- Consumes: existing `adminAPI.accountMonitor.list(range, { signal })`, `AccountMonitorProjection`, `extractApiErrorMessage`, and existing `load(range, options)` callers.
- Produces: the same `projection`, `activeRange`, `rangeError`, write-operation feedback, card Props, and concurrency behavior as before, with no page-level control-plane calls.

- [ ] **Step 1: Add the RED regression tests before touching production code.**

  In `AccountMonitorView.spec.ts`:

  1. Keep the `controlPlaneAPI` mock as a request spy at the page boundary, but replace the current shadow/external-primary tests with one native-only test that mounts the real page, flushes the initial load, switches to `7d`, and asserts `controlPlaneDecision` and `controlPlaneMonitor` were never called while `list` was called for `24h` and `7d`.
  2. Add a normal-state assertion that the page contains the existing card content but no `data-test="read-model-status"`, `控制面暂时不可用`, `完整性：`, `来源：现有系统`, or `来源：控制面` text.
  3. Add a first-load failure test by making the first native `list` reject with `new Error('initial monitor unavailable')`; after `flushPromises()`, assert the native error text and retry control are visible, no monitor cards are rendered, `controlPlaneDecision`/`controlPlaneMonitor` were not called, and clicking retry issues one more native `list('24h', { signal })` request.
  4. Retain the existing “last complete snapshot” failure test and add the same control-plane-not-called assertions to it.
  5. Retain one representative single-account refresh test and assert its initial load and post-probe reload both call native `list`; do not duplicate equivalent assertions for every write dialog.

  Run the focused RED command from the frontend directory:

  ```bash
  pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts
  ```

  Expected result before the production edit: FAIL because the current page renders the control-plane status or calls the control-plane spies; if the first-load empty-state assertion fails for a separate existing-template reason, keep that failure as the exact signal for the minimal template branch in Step 3.

- [ ] **Step 2: Verify the RED output identifies the intended behavior gap.**

  Re-run the same focused command if needed until it fails with an assertion about control-plane calls/status or the missing first-load error empty state, not a syntax, fixture, or environment error. Record the command and concise failure reason in `task-1-report.md` after implementation; do not modify production code to make a test pass before this RED evidence exists.

- [ ] **Step 3: Implement the minimal native-only page change.**

  In `AccountMonitorView.vue`:

  1. Remove the `ReadModelStatus` template block.
  2. Remove imports and state for `controlPlaneAPI`, `ControlPlaneResponse`, `ReadModelStatus`, `useReadModelFreshness`, `resolveTrustedPageDecision`, `controlPlaneResponse`, `controlPlaneDegraded`, `renderSource`, and `readModel`.
  3. Remove the external projection validation helpers and `loadControlPlane()` because they are no longer reachable from the page.
  4. In `load()`, await only `adminAPI.accountMonitor.list(range, { signal: controller.signal })`; retain abort/generation checks, range consistency validation, projection commit, active-group correction, error extraction, snapshot preservation, and generation-scoped cleanup exactly as currently used.
  5. If the RED test demonstrates that a first native failure leaves no explicit empty-state/retry surface, add a minimal `v-else-if="rangeError && !projection"` block in the existing content area with `data-test="account-monitor-error-empty"`, the error message, and a button that calls `load(activeRange)`. Do not alter the successful card grid, skeleton, filters, cards, dialogs, or write handlers.
  6. Do not edit `controlPlane.ts`, `ReadModelStatus.vue`, `useReadModelFreshness.ts`, externalization config, or shared tests.

- [ ] **Step 4: Run the focused GREEN tests.**

  From `upstream/sub2api/frontend`, run:

  ```bash
  pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts
  ```

  Expected result: the full AccountMonitorView spec passes with zero failures, including native-only calls, no status row, first-load error/retry, snapshot retention, range protection, card interactions, and representative write reload.

- [ ] **Step 5: Run the minimum MVP static verification.**

  From `upstream/sub2api/frontend`, run:

  ```bash
  pnpm typecheck
  pnpm build
  ```

  From the worktree root, run:

  ```bash
  git diff --check
  rg -n "controlPlaneAPI|ControlPlaneResponse|ReadModelStatus|useReadModelFreshness|resolveTrustedPageDecision|loadControlPlane|/xingqiao/" upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue
  git status --short
  ```

  Expected result: typecheck, build, and diff check exit successfully; the final `rg` returns no matches in `AccountMonitorView.vue`; only the two scoped frontend files and the progress ledger are changed.

- [ ] **Step 6: Self-review, commit, and report the task.**

  Review the diff against the approved spec and confirm: no card/layout/interaction rewrite, no shared control-plane edits, no new API/config/migration, no external request path, and native failure semantics remain intact. Append RED/GREEN commands and outputs, files changed, self-review, and any unverified item to `task-1-report.md`. Then commit only the scoped implementation and ledger changes:

  ```bash
  git add upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue \
    upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts \
    docs/project/project-progress.md
  git commit -m "fix: make account monitor native-only"
  git status --short --branch
  ```

  The task stops after a clean candidate commit and reports its SHA for independent task review; it does not merge, push, deploy, or mark the ledger complete until review gates pass.

## Task 1 review and final handoff

- [ ] Generate a review package with `scripts/review-package` using the pre-task baseline SHA and candidate HEAD, then dispatch an independent task reviewer for both spec compliance and code quality.
- [ ] If the reviewer reports Critical/Important findings, have the implementer fix them, append a fix report, and run a scoped re-review; do not proceed with open load-bearing findings.
- [ ] After the task review is clean, dispatch a fresh final whole-branch reviewer on the complete candidate diff. Address any Critical/Important findings with one fix round and scoped re-review.
- [ ] Run fresh verification on the final HEAD (`vitest`, `pnpm typecheck`, `pnpm build`, `git diff --check`, and scope checks), update `project-progress.md` with candidate SHA and evidence, and report `READY_FOR_ROOT_REVIEW` with baseline SHA, candidate commit, changed files, tests, unverified items, migration/config changes, `downtime_required=false`, rollback, and risks.

## Rollback

Revert the candidate commit(s) through the reviewed local/host release chain. No database or configuration rollback is required. Do not merge or publish until the root task sends `AUTHORIZE_MERGE_TO_MAIN` with the exact target `main` SHA.

## Done when

- AccountMonitorView performs zero `/xingqiao/**` requests and uses only native monitor data in initial load, range changes, and representative post-write reload.
- Normal state has no control-plane status row and existing cards/interactions remain covered by tests.
- Native first-load and snapshot-retaining failure semantics are covered and passing.
- Candidate is clean, independently task-reviewed, whole-branch reviewed, and reported as `READY_FOR_ROOT_REVIEW`; no merge/push/deploy has happened without root authorization.
