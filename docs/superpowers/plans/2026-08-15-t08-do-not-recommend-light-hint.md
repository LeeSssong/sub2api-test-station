# T08“暂不建议入组”轻提示实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变推荐算法、接口或卡片布局的前提下，把账号监控卡片的“暂不建议入组”原因改为桌面悬浮/点击、移动端点击查看的一到两行轻提示。

**Architecture:** 复用现有 `HelpTooltip.vue` 的 Teleport、定位和关闭机制，新增一个仅供 T08 使用的 `hover-click` 触发合同；`AccountMonitorCard.vue` 仅在 `group_recommendation.status === 'not_recommended'` 时将原紧凑标签包成可访问触发器，并从现有 `reason_codes[0]` 生成短原因。后端投影、推荐评估、分组迁移、评分和其他卡片区域保持不变。

**Tech Stack:** Vue 3 `<script setup>`、TypeScript、Tailwind CSS、Vitest、Vue Test Utils、Vite、Playwright CLI。

## Global Constraints

- 只处理 `group_recommendation.status = not_recommended` 的原因呈现；保留紧凑标签“暂不建议入组”。
- 原因只复用现有 `reason_codes[0]` 和本地化兜底，不新增字段、不改变推荐算法或 7d 主动探测证据。
- 桌面端支持鼠标悬浮、点击和键盘聚焦；移动端支持点击；外部点击和 `Escape` 关闭。
- 原因浮层最多一到两行，不增加常驻说明区、详情页、抽屉、模态框或迁移按钮。
- 不修改账号分组、优先级、调度状态、评分、真实请求质量数据、数据库、后端接口、配置或路由。
- 现有 `HelpTooltip` 的 `hover` 和 `click` 调用方行为必须保持不变；新增 `hover-click` 只供 T08 使用。
- 使用现有 `Icon` 组件的信息图标，不手绘 SVG，不新增第三方依赖。
- 不修改 `docs/project/project-progress.md`、`docs/project/native-sub-task-package-queue.md`、生产证据或生产状态记录；这些由根总控维护。
- 不修改 `.github/workflows/`，发布继续使用既有本地/宿主蓝绿链。
- 发布预检必须输出 `downtime_required=false`；若输出 `true`，在任何停服、迁移、重启或切换前停止并等待人工确认。

---

## 文件与边界

| 文件 | 责任 |
| --- | --- |
| `upstream/sub2api/frontend/src/components/common/HelpTooltip.vue` | 增加 `hover-click` 触发语义，保持既有 `hover`/`click` 行为 |
| `upstream/sub2api/frontend/src/components/common/__tests__/HelpTooltip.spec.ts` | 验证组合触发状态机、关闭语义、焦点和既有模式回归 |
| `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue` | 仅在 `not_recommended` 分支渲染紧凑可访问触发器和短原因浮层 |
| `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts` | 验证 T08 标签、原因、响应式 class、键盘/桌面/移动交互及其他推荐状态回归 |
| `docs/superpowers/reports/2026-08-15-t08-do-not-recommend-light-hint-implementation.md` | 记录本地实现、测试、浏览器证据、未部署状态和剩余风险 |

不修改 `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`：现有
`AccountMonitorGroupRecommendation` 已包含 T08 所需字段，规格明确禁止 API 契约变化。

## Data and Interaction Contract

### Existing data consumed by the card

```ts
interface AccountMonitorGroupRecommendation {
  status: 'recommended' | 'observe' | 'blocked' | 'not_recommended' | string
  target: string
  target_name: string
  action: 'keep' | 'migrate' | 'hold' | 'none' | string
  reason_codes: string[]
  sample_count: number
  observed_at: string
  source: 'monitor_probe' | string
}
```

T08 只读取 `status` 和 `reason_codes[0]`。未知、缺失或空原因码统一显示：
`原因：主动探测质量不满足目标`。

### New HelpTooltip prop contract

```ts
trigger?: 'hover' | 'click' | 'hover-click'
```

`hover-click` 状态机必须满足：

- `mouseenter` 或触发器获得焦点：打开浮层；
- `mouseleave` 或失去焦点：仅在未被点击锁定时关闭；
- 点击触发器：打开并锁定；再次点击：关闭并解除锁定；
- 点击浮层外部或按 `Escape`：关闭并解除锁定；
- 视口变化时保持现有定位更新；
- 卸载时清理既有 document/window 监听；
- `hover` 和 `click` 的现有行为、关闭按钮和测试继续通过。

### New card DOM contract

仅对 `not_recommended` 使用以下稳定测试语义：

- `[data-test="recommendation-reason-trigger"]`：T08 `HelpTooltip` 根触发器；
- `[data-test="recommendation-reason-button"]`：可点击、可聚焦的紧凑按钮；
- `[data-test="group-recommendation"]`：可见标签，文本为“暂不建议入组”；
- `[data-test="group-recommendation-reason"]`：Teleport 后的短原因正文。

正式组迁移仍使用现有：

- `[data-test="recommendation-warning"]`
- `[data-test="group-recommendation-tooltip"]`

不得把 T08 原因正文渲染到卡片常驻元信息中。

---

### Task 1: Add and verify the `HelpTooltip` hover-click trigger contract

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/common/HelpTooltip.vue`
- Test: `upstream/sub2api/frontend/src/components/common/__tests__/HelpTooltip.spec.ts`

**Interfaces:**
- Consumes: existing `content`, `trigger`, `widthClass`, default/trigger slots, Teleport target, document click and keydown listeners.
- Produces: `trigger="hover-click"` behavior described in the interaction contract; existing `trigger="hover"` and `trigger="click"` behavior remains backward compatible.

- [ ] **Step 1: Write failing tests for pointer hover and leave.**

Add a test named `opens and closes hover-click details from pointer enter and leave` in
`HelpTooltip.spec.ts`. Mount with `trigger: 'hover-click'`, attach to `document.body`, trigger
`mouseenter` on `.group`, assert the `[role="tooltip"]` display is not `none`, trigger
`mouseleave`, and assert the display returns to `none`.

- [ ] **Step 2: Run the focused test and verify it fails for the missing trigger contract.**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/common/__tests__/HelpTooltip.spec.ts -t "hover-click"
```

Expected: FAIL before implementation because the current prop type accepts only `hover` and
`click`, and the component has no combined trigger behavior.

- [ ] **Step 3: Write failing tests for click locking, outside click, Escape, and focus.**

Add tests with these exact behaviors:

1. `pins hover-click details on click and toggles on the second click`: click `.group`,
   assert visible; trigger `mouseleave`, assert still visible; click again, assert hidden.
2. `closes a pinned hover-click tooltip on outside click and Escape`: click to pin, dispatch a
   bubbling document `MouseEvent('click')` outside the wrapper, assert hidden; reopen, dispatch
   `KeyboardEvent('keydown', { key: 'Escape' })`, assert hidden.
3. `opens hover-click details from a focusable trigger and closes on blur when unpinned`:
   provide a button trigger slot, focus it, assert visible, blur it, assert hidden.
4. `preserves existing hover and click trigger semantics`: retain the existing hover and click
   tests unchanged and keep their assertions passing.

- [ ] **Step 4: Run the new failing tests.**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/common/__tests__/HelpTooltip.spec.ts -t "hover-click|existing hover|click-to-toggle|keyboard-focusable"
```

Expected: the new hover-click tests fail while the existing hover/click tests remain green.

- [ ] **Step 5: Implement the minimal combined trigger state machine.**

In `HelpTooltip.vue`:

1. Extend the `trigger` prop union with `'hover-click'`.
2. Add one internal boolean for click pinning; it must only affect `hover-click`.
3. Make pointer enter and focus open the tooltip for `hover-click`.
4. Make pointer leave and focus-out close only when the click pin is false.
5. Make wrapper click toggle the click pin and visibility for `hover-click`.
6. Reuse existing outside-click, `Escape`, viewport-change, and unmount cleanup paths;
   outside-click and `Escape` must also clear the click pin.
7. Keep the existing `trigger === 'click'` close button behavior unchanged; if the
   implementation treats `hover-click` as click-capable for the close button, use the same
   existing `aria-label="Close"` and close handler.

Do not add a second Tooltip component, a new dependency, or a card-specific positioning
implementation.

- [ ] **Step 6: Run the focused Tooltip tests and typecheck.**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/common/__tests__/HelpTooltip.spec.ts
pnpm typecheck
```

Expected: all existing and new `HelpTooltip` tests PASS; TypeScript accepts the new prop
union and all existing call sites.

- [ ] **Step 7: Commit the independently reviewable Tooltip change.**

```bash
git add upstream/sub2api/frontend/src/components/common/HelpTooltip.vue \
  upstream/sub2api/frontend/src/components/common/__tests__/HelpTooltip.spec.ts
git commit -m "feat: support hover-click help tooltips"
```

Review gate: the reviewer must reject the task if an existing `hover`/`click` caller changes
behavior, if pinned hover-click state cannot be closed by outside click/Escape, or if document
listeners remain after unmount.

---

### Task 2: Render the T08 not-recommended light hint in the account card

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Test: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

**Interfaces:**
- Consumes: existing optional `account.group_recommendation`, existing `recommendationReason`
  mapping, Task 1 `HelpTooltip` prop `trigger="hover-click"`, and existing `Icon` component.
- Produces: the compact `not_recommended` label plus an on-demand, accessible short reason;
  `recommended`, `observe`, `blocked`, formal migration, and missing-object behavior remain
  unchanged.

- [ ] **Step 1: Add failing tests for the default compact card state.**

In `AccountMonitorCard.spec.ts`, add a `not_recommended` fixture with
`reason_codes: ['success_rate_below_special']`, mount it in `GPT-测试分组`, and add tests:

1. `keeps the not-recommended label compact and hides its reason by default`: assert
   `[data-test="group-recommendation"]` contains `暂不建议入组`; assert
   `[data-test="recommendation-reason-button"]` exists; assert
   `[data-test="account-metadata"]` does not contain `原因：`.
2. `uses the primary reason code and a stable fallback`: assert the known code produces
   `原因：探测成功率未达到特惠门槛`; mount with `reason_codes: []` and an unknown code and
   assert `原因：主动探测质量不满足目标`.

- [ ] **Step 2: Run the focused card tests and verify the new tests fail.**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts -t "not-recommended|primary reason|fallback"
```

Expected: FAIL because the current `not_recommended` branch renders a plain span and has no
T08 trigger/reason selectors.

- [ ] **Step 3: Add failing tests for desktop, mobile, keyboard, close, and no-regression behavior.**

Add tests with these exact assertions:

1. `opens the not-recommended reason on hover`: trigger `mouseenter` on
   `[data-test="recommendation-reason-trigger"]`, await `nextTick`, assert the Teleported
   `[data-test="group-recommendation-reason"]` is visible and contains the short reason.
2. `opens and closes the not-recommended reason on click`: click
   `[data-test="recommendation-reason-button"]`, assert visible; click it again, assert hidden.
3. `opens the reason from keyboard focus`: focus the button, assert visible; assert its
   `title` and `aria-label` include both `暂不建议入组` and the short reason.
4. `closes the reason without firing card actions`: open the reason, dispatch an outside click
   and `Escape`, assert hidden; assert `accountInfo`, `accountEdit`, `accountDelete`, and
   `accountMore` mocks were not called.
5. `keeps the other recommendation states unchanged`: retain the existing
   `recommended`/`observe`/`blocked`/formal migration tests and assert the T08 reason selector
   exists only for `not_recommended`.
6. `keeps the hint width and wrapping contract`: assert the reason element has
   `max-w-[min(16rem,calc(100vw-1.5rem))]`, `break-words`, and `whitespace-normal` classes.

- [ ] **Step 4: Run the complete card test file before implementation.**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

Expected: existing tests PASS and the new T08 tests FAIL only on the not-yet-implemented
selectors/interaction.

- [ ] **Step 5: Implement the minimal card rendering branch.**

In `AccountMonitorCard.vue`:

1. Add a computed boolean for `recommendation.value?.status === 'not_recommended'`.
2. Keep `recommendationLabel` and `recommendationReason` as the single sources for the label
   and first reason code.
3. Add a short `recommendationReasonHint` computed value that returns
   `原因：${recommendationReason.value}` only for `not_recommended`.
4. In the existing metadata-row recommendation branch, render a `HelpTooltip` with
   `trigger="hover-click"` and `data-test="recommendation-reason-trigger"` only for
   `not_recommended`.
5. Make the trigger slot a compact `<button type="button">` with
   `data-test="recommendation-reason-button"`, `title`, `aria-label`, visible label
   `暂不建议入组`, and the existing `Icon` component using the existing `infoCircle` icon.
6. Render the Teleported reason as `data-test="group-recommendation-reason"` with
   `max-w-[min(16rem,calc(100vw-1.5rem))] whitespace-normal break-words leading-5`; do not
   include target group, sample count, observation time, or long evidence text.
7. Preserve the existing formal migration `HelpTooltip` branch exactly for `formalMigration`.
8. Preserve the existing plain-span branch for `recommended`, `observe`, and `blocked`.
9. Do not add a second metadata row, new card section, migration action, API call, or state
   persistence.

- [ ] **Step 6: Run the card tests, typecheck, and diff check.**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts
pnpm typecheck
git diff --check
```

Expected: all card tests PASS; the existing formal migration Tooltip remains accessible; no
type or whitespace errors are reported.

- [ ] **Step 7: Commit the independently reviewable card change.**

```bash
git add upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue \
  upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts
git commit -m "feat: add on-demand not-recommended reason"
```

Review gate: the reviewer must reject the task if the reason is visible without interaction,
if the T08 branch changes formal migration behavior, if the label leaves the metadata row, if
the tooltip exposes long evidence text, or if any account operation receives an unintended
click.

---

### Task 3: Run full validation, browser evidence, and handoff review

**Files:**
- Create: `docs/superpowers/reports/2026-08-15-t08-do-not-recommend-light-hint-implementation.md`
- Do not modify: `docs/project/project-progress.md`
- Do not modify: `docs/project/native-sub-task-package-queue.md`

**Interfaces:**
- Consumes: Task 1 and Task 2 commits, the approved design specification, and the current
  account-monitor browser route `/admin/accounts/monitor`.
- Produces: local validation evidence and a `READY_FOR_ROOT_REVIEW` handoff report; no merge,
  push, deployment, production write, or global ledger update.

- [ ] **Step 1: Run the focused combined frontend test set.**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run \
  src/components/common/__tests__/HelpTooltip.spec.ts \
  src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

Expected: all Tooltip and card tests PASS, including existing hover/click behavior, formal
migration behavior, T08 reason behavior, and outside/Escape closure.

- [ ] **Step 2: Run the frontend typecheck, build, and whitespace checks.**

Run:

```bash
cd upstream/sub2api/frontend
pnpm typecheck
pnpm build
cd ../..
git diff --check
```

Expected: all commands exit 0; no backend, migration, config, dependency, or workflow file
appears in the diff.

- [ ] **Step 3: Run the complete frontend regression suite.**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run
```

Expected: the complete Vitest suite passes. Existing warnings may be recorded, but no new
T08-related failure, console error, or snapshot mismatch may remain.

- [ ] **Step 4: Start the local frontend for browser verification.**

Run:

```bash
cd upstream/sub2api/frontend
pnpm dev --host 127.0.0.1 --port 4173
```

Use the existing authenticated/local mock account-monitor route at
`http://127.0.0.1:4173/admin/accounts/monitor`. Do not submit account edits, cost changes,
priority changes, group changes, refresh actions, or probe actions from the browser.

- [ ] **Step 5: Capture desktop and mobile browser evidence.**

Use the bundled Playwright CLI wrapper:

```bash
PWCLI="$HOME/.codex/skills/playwright/scripts/playwright_cli.sh"
```

At `1440x1000`, verify:

- the `not_recommended` card shows only `暂不建议入组` before interaction;
- hover opens a Teleported reason with one or two wrapped lines;
- click toggles the reason;
- keyboard focus opens the reason and the trigger has a useful accessible name;
- outside click and `Escape` close it;
- formal migration Tooltip still shows only for explicit formal migration.

At `390x844`, verify:

- tapping the compact label opens the reason;
- the floating panel stays within the viewport;
- `document.documentElement.scrollWidth === document.documentElement.clientWidth`;
- the account name, status, metadata, and operation buttons do not overlap or become
  unreachable.

Save local-only screenshots and DOM notes under:

```text
output/playwright/t08-do-not-recommend-light-hint/desktop.png
output/playwright/t08-do-not-recommend-light-hint/mobile.png
```

Do not commit screenshots unless the root task explicitly requests them as release evidence.

- [ ] **Step 6: Run the scope and source-of-truth diff audit.**

Run:

```bash
git diff --name-only 5fa37dbfd..HEAD
git diff --check 5fa37dbfd..HEAD
```

The only allowed application files are `HelpTooltip.vue`, its test, `AccountMonitorCard.vue`,
and its test, plus the T08 handoff report. Reject the candidate if it changes:

- backend recommendation evaluation or API types;
- database migrations, configuration, dependencies, routes, schedulers, or production data;
- account groups, priorities, dispatch state, score weights, or usage evidence;
- `docs/project/project-progress.md`, `docs/project/native-sub-task-package-queue.md`, or
  `.github/workflows/`.

- [ ] **Step 7: Write the implementation handoff report.**

Create `docs/superpowers/reports/2026-08-15-t08-do-not-recommend-light-hint-implementation.md`
with these exact sections:

1. Scope and approved specification path;
2. Baseline SHA, implementation commit SHAs, and changed files;
3. TDD tests and exact command results;
4. Desktop/mobile screenshot and DOM evidence paths;
5. Interface, migration, dependency, configuration, and GitHub Actions status;
6. `downtime_required=false` precondition;
7. Rollback: restore the previous verified frontend image/tree, with no data rollback;
8. Unverified items: production deployment and online verification;
9. Remaining risks and independent review result.

Keep the report status `READY_FOR_ROOT_REVIEW`; do not claim `DONE` or deployed.

- [ ] **Step 8: Commit the validation handoff only after all checks pass.**

```bash
git add docs/superpowers/reports/2026-08-15-t08-do-not-recommend-light-hint-implementation.md
git commit -m "docs: record T08 light hint validation"
```

Review gate: an independent whole-branch reviewer must review findings first and block handoff
on any scope, accessibility, responsive overflow, existing Tooltip regression, or source-of-
truth violation. The top-level task then reports `READY_FOR_ROOT_REVIEW` and waits for the root
total to send `AUTHORIZE_MERGE_TO_MAIN`.

---

## Root merge, release, and rollback gates

These gates are part of the implementation plan but are executed only by the root total after
an explicit authorization:

1. Confirm the candidate worktree is clean and still based on the latest approved `main`.
2. If `main` moved after implementation, mark the candidate `REFRESH_REQUIRED`; refresh from
   the new `main`, rerun Tasks 1–3, and obtain a new independent review.
3. Merge only after `READY_FOR_ROOT_REVIEW` and a root instruction containing the exact target
   `main` SHA: `AUTHORIZE_MERGE_TO_MAIN`.
4. On merged `main`, rerun the focused Tooltip/card tests, full frontend tests, typecheck,
   build, `git diff --check`, and release preflight.
5. Require release preflight output `downtime_required=false`. Stop before any production
   action if it is `true`, if migration state changes unexpectedly, or if conflict output is
   non-empty.
6. Push and deploy only from the verified root `main` through the reviewed local/host blue-green
   chain. Do not use GitHub Actions.
7. Verify production admin login state, T08 desktop/mobile behavior, no horizontal overflow,
   unchanged account operations, `/healthz`, `/readyz`, `/health`, and deployment identity.
8. If deployment or online verification fails, retain this candidate worktree, failure evidence,
   and rollback basis; repair on the same candidate and repeat merge, regression, push, deploy,
   and online verification.
9. Roll back by promoting the previous verified frontend image/tree through the same host chain.
   No database, account, group, priority, or usage-data rollback is allowed because T08 has no
   data migration or production write.
10. Only after `main` is pushed, deployment succeeds, and online verification is effective may
    the root total update global records and remove the candidate worktree/branch.

## Plan self-review

### Spec coverage

- Compact label and no permanent reason: Task 2 steps 1 and 5.
- Desktop hover/click, mobile click, keyboard focus, outside click, and `Escape`: Task 1
  steps 1–5 and Task 2 steps 3 and 5.
- One-to-two-line reason and viewport-safe Teleport: Task 2 steps 1, 3, and 5; Task 3 step 5.
- Existing recommendation states and formal migration regression: Task 2 steps 3 and 6; Task 3
  step 1.
- No API/algorithm/layout/data changes: Global Constraints, file boundary table, Task 2 step 5,
  and Task 3 step 6.
- TDD and independent review: every implementation task writes failing tests first, runs them,
  implements the minimum change, reruns focused tests, and commits separately.
- Release, downtime, root authorization, rollback, and failure retention: Root merge/release/
  rollback gates.

### Placeholder and ambiguity scan

- No placeholder markers, deferred implementation notes, or unbounded error-handling
  instructions appear in any task step.
- The new prop name is fixed as `hover-click`.
- The exact selectors, reason fallback, tests, commands, commits, report path, allowed files,
  and release stop conditions are defined.
- The browser evidence is local-only and explicitly excludes production writes.

### Type and boundary consistency

- Task 1 produces the `HelpTooltip` prop consumed by Task 2.
- Task 2 consumes the existing `AccountMonitorGroupRecommendation` contract without changing it.
- Task 3 consumes only the two frontend commits and produces a handoff report.
- No task modifies global project ledger files, backend code, database schema, or production state.

Plan conclusion: `READY_FOR_IMPLEMENTATION`.
