# T06-R1 Profitability Dark Theme Localization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the T06 account profitability page so the native admin dark theme remains readable and the active range/table labels render through Chinese and English locale entries.

**Architecture:** Keep the page on the existing native admin implementation and consume the established global theme classes: `.card`, `.table-container`, and `.table`. Localize the actual page ranges and table headers through `admin.accountProfitability.*` keys without changing API calls, financial calculations, back-end code, database state, configuration, deployment evidence, or root task ledgers.

**Tech Stack:** Vue 3 SFC, TypeScript, vue-i18n, Vitest, Vue Test Utils, existing Sub2API frontend build chain.

## Global Constraints

- Project date for this task is 2026-08-14.
- Task package is T06-R1, a user-visible independent top-level task.
- Baseline is `651bc2fab27544a8cc131137ab351bf8f2f90f89`; current spec commit is `253676a9c97d22618c7db9ecbae8ebc53fbba610`.
- Do not modify root `main`, `docs/project/project-progress.md`, `docs/project/native-sub-task-package-queue.md`, production state, release records, or deployment evidence.
- Do not merge, push, deploy, or run production verification; final status may only be `READY_FOR_ROOT_REVIEW`.
- Only fix profitability page dark-theme readability, `24h`/`31d` localization, table header localization, and page-level tests.
- Preserve original T06 native API calls, refresh, range switching, today override, OAuth daily cost writes, exception jump, and static no-control-plane/no-`/api/v1/xingqiao/**` guard.
- Do not change financial calculations, interface fields, back end, database migrations, config, dependencies, other pages, T07, GitHub Actions, global `style.css`, or `xingqiao-update-ui.css`.
- Implementation must follow TDD. Each implementation task must use a fresh implementer subagent and an independent read-only reviewer. Run a final whole-branch review before reporting readiness.
- Expected `downtime_required=false`; root release control must confirm this later during authorized release precheck.

---

## File Structure

- Modify `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
  - Owns the page-level regression contract for native API behavior, refresh behavior, range switching, today-only edits, exception jump, static control-plane exclusions, visible localized labels, and theme class usage.
- Modify `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
  - Owns the profitability page template and existing script flow. The implementation should only change the summary card/table classes and table header labels.
- Modify `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
  - Owns Chinese admin locale messages. Add the two actual range labels used by the page.
- Modify `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`
  - Owns English admin locale messages. Add the two actual range labels used by the page.
- Add implementation review artifacts only if the subagent/reviewer workflow needs them under `docs/superpowers/reviews/` or another already-established evidence folder. Do not write global project ledgers.

## Task 1: Localized Range And Table Header Contract

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`

**Interfaces:**
- Consumes: existing `FinancialRange = 'today' | '24h' | '7d' | '31d'`, existing `admin.accountProfitability.columns.*` locale keys, existing `ranges` array in `AccountProfitabilityView.vue`.
- Produces: rendered range labels `今日`, `24 小时`, `7 天`, `31 天`; rendered table headers `账号`, `收入`, `支出`, `盈利`, `利润率`, `异常`, `今日覆盖`; zh/en range entries for `24h` and `31d`.

- [ ] **Step 1: Update the i18n mock test helper**

In `AccountProfitabilityView.spec.ts`, replace the current identity `vue-i18n` mock with this explicit mapping. Keep the same `vi.mock('vue-i18n', ...)` shape.

```ts
const messages: Record<string, string> = {
  'admin.accountProfitability.title': '账号盈利',
  'admin.accountProfitability.description': '按时间范围查看每个账号的实际收入、支出、盈利与利润率。',
  'admin.accountProfitability.ranges.today': '今日',
  'admin.accountProfitability.ranges.24h': '24 小时',
  'admin.accountProfitability.ranges.7d': '7 天',
  'admin.accountProfitability.ranges.31d': '31 天',
  'admin.accountProfitability.summary.revenue': '收入',
  'admin.accountProfitability.summary.expense': '支出',
  'admin.accountProfitability.summary.profit': '盈利',
  'admin.accountProfitability.summary.margin': '利润率',
  'admin.accountProfitability.summary.exceptions': '异常流水',
  'admin.accountProfitability.summary.unconsumedBalance': '用户未消费余额',
  'admin.accountProfitability.columns.account': '账号',
  'admin.accountProfitability.columns.revenue': '收入',
  'admin.accountProfitability.columns.expense': '支出',
  'admin.accountProfitability.columns.profit': '盈利',
  'admin.accountProfitability.columns.margin': '利润率',
  'admin.accountProfitability.columns.exceptions': '异常',
  'admin.accountProfitability.columns.actions': '今日覆盖',
  'common.refresh': '刷新',
}

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
  }),
}))
```

- [ ] **Step 2: Write the failing localized range/header test**

Add this test inside the existing `describe('AccountProfitabilityView', ...)` block after `beforeEach`.

```ts
it('renders Chinese range labels and localized table headers without leaking i18n keys', async () => {
  const wrapper = mount(AccountProfitabilityView, {
    global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } },
  })
  await flushPromises()

  expect(wrapper.get('[data-test="range-today"]').text()).toBe('今日')
  expect(wrapper.get('[data-test="range-24h"]').text()).toBe('24 小时')
  expect(wrapper.get('[data-test="range-7d"]').text()).toBe('7 天')
  expect(wrapper.get('[data-test="range-31d"]').text()).toBe('31 天')

  const headers = wrapper.findAll('th').map((header) => header.text())
  expect(headers).toEqual(['账号', '收入', '支出', '盈利', '利润率', '异常', '今日覆盖'])

  expect(wrapper.text()).not.toContain('admin.accountProfitability.ranges.24h')
  expect(wrapper.text()).not.toContain('admin.accountProfitability.ranges.31d')
  expect(headers).not.toEqual(['Account', 'Revenue', 'Expense', 'Profit', 'Margin', 'Exceptions', 'Today override'])
})
```

- [ ] **Step 3: Run the targeted test and verify the expected failure**

Run:

```bash
cd upstream/sub2api/frontend
npm run test:run -- src/views/admin/__tests__/AccountProfitabilityView.spec.ts -t "renders Chinese range labels and localized table headers without leaking i18n keys"
```

Expected: FAIL because the current table headers are hardcoded English. If the test passes before implementation, stop and inspect whether the source has already changed unexpectedly.

- [ ] **Step 4: Localize table headers in the Vue template**

In `AccountProfitabilityView.vue`, replace the current header row:

```vue
<tr><th>Account</th><th>Revenue</th><th>Expense</th><th>Profit</th><th>Margin</th><th>Exceptions</th><th>Today override</th></tr>
```

with:

```vue
<tr>
  <th>{{ t('admin.accountProfitability.columns.account') }}</th>
  <th>{{ t('admin.accountProfitability.columns.revenue') }}</th>
  <th>{{ t('admin.accountProfitability.columns.expense') }}</th>
  <th>{{ t('admin.accountProfitability.columns.profit') }}</th>
  <th>{{ t('admin.accountProfitability.columns.margin') }}</th>
  <th>{{ t('admin.accountProfitability.columns.exceptions') }}</th>
  <th>{{ t('admin.accountProfitability.columns.actions') }}</th>
</tr>
```

Do not change the row data cells, save handlers, range logic, or API calls.

- [ ] **Step 5: Add the actual range entries to Chinese locale**

In `zh/admin/index.ts`, change:

```ts
ranges: { today: '今日', '7d': '7 天', '30d': '30 天', month: '本月' },
```

to:

```ts
ranges: { today: '今日', '24h': '24 小时', '7d': '7 天', '31d': '31 天', '30d': '30 天', month: '本月' },
```

- [ ] **Step 6: Add the actual range entries to English locale**

In `en/admin/index.ts`, change:

```ts
ranges: { today: 'Today', '7d': '7 days', '30d': '30 days', month: 'This month' },
```

to:

```ts
ranges: { today: 'Today', '24h': '24 hours', '7d': '7 days', '31d': '31 days', '30d': '30 days', month: 'This month' },
```

- [ ] **Step 7: Run the targeted test and verify it passes**

Run:

```bash
cd upstream/sub2api/frontend
npm run test:run -- src/views/admin/__tests__/AccountProfitabilityView.spec.ts -t "renders Chinese range labels and localized table headers without leaking i18n keys"
```

Expected: PASS.

- [ ] **Step 8: Run the existing page test file**

Run:

```bash
cd upstream/sub2api/frontend
npm run test:run -- src/views/admin/__tests__/AccountProfitabilityView.spec.ts
```

Expected: PASS. Existing tests for native report loading, manual refresh, 60-second refresh, read-only ranges, exception jump, today override, OAuth daily cost, and control-plane exclusions must still pass.

- [ ] **Step 9: Commit Task 1**

Run:

```bash
git status --short
git add upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts
git commit -m "fix: localize profitability range and headers"
```

- [ ] **Step 10: Independent read-only review for Task 1**

Dispatch a reviewer that only reads the diff and reports findings. Reviewer must verify:

```bash
git show --stat --oneline HEAD
git show -- upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts
```

Reviewer acceptance criteria:

- The test mock maps the target i18n keys to explicit Chinese text.
- The test fails on hardcoded English headers and missing range labels.
- The Vue template uses `t('admin.accountProfitability.columns.*')` for all seven headers.
- zh and en locale files contain `24h` and `31d` under `accountProfitability.ranges`.
- No API path, request payload, financial calculation, timer behavior, router behavior, global CSS, back-end file, migration, config, GitHub Actions file, or project ledger changed.

## Task 2: Native Dark Theme Class Contract

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`

**Interfaces:**
- Consumes: existing global `.card`, `.table-container`, and `.table` definitions from `upstream/sub2api/frontend/src/style.css`.
- Produces: six summary cards with `card p-4`, one table wrapper with `table-container`, one table with `table`, and no fixed `bg-white` on the profitability summary cards or table wrapper.

- [ ] **Step 1: Write the failing theme class test**

Add this test inside the existing `describe('AccountProfitabilityView', ...)` block after the localized range/header test.

```ts
it('uses native admin theme classes for summary cards and the account table', async () => {
  const wrapper = mount(AccountProfitabilityView, {
    global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } },
  })
  await flushPromises()

  const cardKeys = ['revenue', 'expense', 'profit', 'margin', 'exceptions', 'unconsumed-balance']
  for (const key of cardKeys) {
    const classes = wrapper.get(`[data-test="summary-${key}"]`).classes()
    expect(classes).toContain('card')
    expect(classes).toContain('p-4')
    expect(classes).not.toContain('bg-white')
  }

  const tableWrapper = wrapper.get('[data-test="account-financial-table"]')
  expect(tableWrapper.classes()).toContain('table-container')
  expect(tableWrapper.classes()).not.toContain('bg-white')

  const table = tableWrapper.get('table')
  expect(table.classes()).toContain('table')
  expect(table.classes()).not.toContain('min-w-full')
})
```

- [ ] **Step 2: Run the targeted test and verify the expected failure**

Run:

```bash
cd upstream/sub2api/frontend
npm run test:run -- src/views/admin/__tests__/AccountProfitabilityView.spec.ts -t "uses native admin theme classes for summary cards and the account table"
```

Expected: FAIL because summary cards and the table wrapper currently include fixed `bg-white`, the wrapper lacks `data-test="account-financial-table"`, and the table lacks `table`.

- [ ] **Step 3: Update summary card classes**

In `AccountProfitabilityView.vue`, replace:

```vue
class="rounded-xl border bg-white p-4"
```

with:

```vue
class="card p-4"
```

Do not change `data-test`, card labels, card values, or the `cards` computed value.

- [ ] **Step 4: Update table wrapper and table classes**

In `AccountProfitabilityView.vue`, replace:

```vue
<section class="overflow-x-auto rounded-xl border bg-white"><table class="min-w-full text-sm">
```

with:

```vue
<section class="table-container" data-test="account-financial-table"><table class="table">
```

Do not change table rows, cells, inputs, buttons, or handlers.

- [ ] **Step 5: Run the targeted test and verify it passes**

Run:

```bash
cd upstream/sub2api/frontend
npm run test:run -- src/views/admin/__tests__/AccountProfitabilityView.spec.ts -t "uses native admin theme classes for summary cards and the account table"
```

Expected: PASS.

- [ ] **Step 6: Run the full page test file**

Run:

```bash
cd upstream/sub2api/frontend
npm run test:run -- src/views/admin/__tests__/AccountProfitabilityView.spec.ts
```

Expected: PASS. This also rechecks the localized range/header contract and all pre-existing T06 behavior.

- [ ] **Step 7: Commit Task 2**

Run:

```bash
git status --short
git add upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue
git commit -m "fix: use native theme classes for profitability table"
```

- [ ] **Step 8: Independent read-only review for Task 2**

Dispatch a reviewer that only reads the diff and reports findings. Reviewer must verify:

```bash
git show --stat --oneline HEAD
git show -- upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue
```

Reviewer acceptance criteria:

- Summary articles consume `.card` and retain `p-4`.
- Table wrapper consumes `.table-container` and has `data-test="account-financial-table"`.
- Table consumes `.table`.
- Fixed `bg-white` is absent from the target profitability card/table containers.
- Existing T06 runtime behavior remains untouched.
- No global CSS, back-end, migration, config, other page, GitHub Actions, production evidence, or project ledger changed.

## Task 3: Final Verification And Handoff Evidence

**Files:**
- Read: all files changed since `651bc2fab27544a8cc131137ab351bf8f2f90f89`
- Optional create: `docs/superpowers/reviews/2026-08-14-t06-r1-final-review.md` if the final reviewer produces a file-backed report

**Interfaces:**
- Consumes: Task 1 and Task 2 commits, their reviewer findings, and the spec.
- Produces: candidate SHA, verified command results, diff scope, migration/config assessment, downtime expectation, rollback statement, and final `READY_FOR_ROOT_REVIEW` report.

- [ ] **Step 1: Run the targeted page test file**

Run:

```bash
cd upstream/sub2api/frontend
npm run test:run -- src/views/admin/__tests__/AccountProfitabilityView.spec.ts
```

Expected: PASS.

- [ ] **Step 2: Run frontend typecheck**

Run:

```bash
cd upstream/sub2api/frontend
npm run typecheck
```

Expected: PASS.

- [ ] **Step 3: Run production build**

Run:

```bash
cd upstream/sub2api/frontend
npm run build
```

Expected: PASS.

- [ ] **Step 4: Run static scope checks**

Run:

```bash
git diff --name-only 651bc2fab27544a8cc131137ab351bf8f2f90f89...HEAD
rg -n "controlPlaneAPI|ControlPlaneResponse|ReadModelStatus|useReadModelFreshness|resolveTrustedPageDecision|controlPlaneResponse|controlPlaneDegraded|renderSource|unknown|degraded|integrity|/api/v1/xingqiao|/xingqiao" upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue
rg -n "bg-white" upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue
rg -n "Account|Revenue|Expense|Profit|Margin|Exceptions|Today override" upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue
```

Expected:

- `git diff --name-only` contains only the allowed runtime/test files plus task evidence docs.
- The control-plane/xingqiao/static status search returns no matches in `AccountProfitabilityView.vue`.
- The `bg-white` search returns no matches in `AccountProfitabilityView.vue`.
- The English header literal search returns no table header literals in `AccountProfitabilityView.vue`; if `Account` appears only inside i18n keys such as `accountProfitability`, that is acceptable.

- [ ] **Step 5: Inspect migration, config, and release evidence scope**

Run:

```bash
git diff --name-only 651bc2fab27544a8cc131137ab351bf8f2f90f89...HEAD | rg -n "(\.github/workflows|migration|migrations|alembic|schema|config|env|release-records|project-progress|native-sub-task-package-queue)"
```

Expected: no matches. If any match appears, stop and review whether the plan scope was violated.

- [ ] **Step 6: Run final whole-branch read-only review**

Dispatch a reviewer with the full branch diff. Reviewer must read:

```bash
git diff 651bc2fab27544a8cc131137ab351bf8f2f90f89...HEAD -- upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts docs/superpowers/specs/2026-08-14-t06-r1-profitability-dark-theme-localization-design.md docs/superpowers/plans/2026-08-14-t06-r1-profitability-dark-theme-localization.md
```

Reviewer acceptance criteria:

- Spec scope is fully implemented.
- No non-goal code path changed.
- Tests meaningfully fail on the original dark-theme and localization regressions.
- Build/typecheck/test evidence is sufficient for root review.
- Final report can truthfully state no migrations and no config changes.

- [ ] **Step 7: Apply verification-before-completion before final status**

Read and follow `superpowers:verification-before-completion` before claiming completion or readiness. Confirm the exact final command outputs and current `git status --short`.

- [ ] **Step 8: Commit final evidence if a review artifact was created**

If a final review file was created, run:

```bash
git add docs/superpowers/reviews/2026-08-14-t06-r1-final-review.md
git commit -m "docs: review T06-R1 profitability fix"
```

If no final review file was created, do not create an empty commit.

- [ ] **Step 9: Report only READY_FOR_ROOT_REVIEW**

The final response must include:

- `READY_FOR_ROOT_REVIEW`
- Task package `T06-R1 利润页深色主题与中文本地化修复`
- Baseline SHA `651bc2fab27544a8cc131137ab351bf8f2f90f89`
- Candidate SHA from `git rev-parse HEAD`
- Changed files
- Test/typecheck/build commands and results
- Unverified items, including no production/admin-login verification by this task
- Migration/config changes, expected as none
- `downtime_required=false` as task expectation
- Rollback: root release control reverts the T06-R1 candidate commit(s) and redeploys through the reviewed local/host release chain
- Remaining risks, if any

Do not merge, push, deploy, update root ledgers, update release evidence, or perform production operations.

## Self-Review

- Spec coverage: Task 1 covers `24h`/`31d` zh/en locale entries and seven localized table headers. Task 2 covers dark-theme readability through existing native admin theme classes and fixed `bg-white` regression protection. Task 3 covers page tests, typecheck, build, scope checks, migration/config checks, final whole-branch review, and the required `READY_FOR_ROOT_REVIEW` handoff.
- Placeholder scan: The plan contains no unresolved placeholder markers or unspecified test/implementation steps.
- Type consistency: The plan uses the existing `FinancialRange` values, existing `admin.accountProfitability.columns.*` keys, existing `data-test` summary keys, and one new `data-test="account-financial-table"` that is defined in Task 2 before final verification relies on it.
- Scope check: Runtime changes are limited to `AccountProfitabilityView.vue` and zh/en admin locale files; tests are limited to the page spec. The plan does not touch global CSS, back end, migrations, config, other pages, T07, GitHub Actions, root `main`, project ledgers, production state, or release records.
