# T36 经营页 CNY/USD 额度关系文案实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在经营页 USD/CNY 切换控件旁增加中英文 i18n 额度关系说明，让运营人员直观看懂本站 1 USD 额度与 1 CNY 额度按 1:1 理解且不是汇率换算。

**Architecture:** 继续使用 `AccountProfitabilityView.vue` 现有 `useI18n()` 和路由懒加载入口，在 segmented view 下方渲染一个稳定的说明节点。文案只放入现有 zh/en `admin.accountProfitability` locale 对象，不引入计算、接口或新组件。

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, vue-i18n, Vitest + Vue Test Utils, Vite/Vue TypeScript build。

**Spec:** `docs/superpowers/specs/2026-08-20-t36-profitability-quota-parity-copy-design.md`

## Global Constraints

- 中文固定值：`额度口径：1 USD 额度 = 1 CNY 额度（仅用于额度关系理解，不是汇率换算）`。
- 英文固定值：`Quota basis: 1 USD quota = 1 CNY quota (for understanding the quota relationship only; not an exchange-rate conversion)`。
- 页面节点固定为 `p[data-test="quota-parity-note"]`，且调用 `t('admin.accountProfitability.quotaParityNote')`。
- 仅修改经营页、zh/en admin locale、直接页面测试与 T36 文档；保留既有路由懒加载、请求、账务和采购行为。
- 不触碰后端、API、SQL、迁移、配置、生产数据、生产状态、全局队列或项目进度总账。

### Task 1: Add the failing component contract test (RED)

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`

**Interfaces:**
- Consumes: `AccountProfitabilityView.vue?raw`, `zhAdmin.accountProfitability`, `enAdmin.accountProfitability`, and the existing `messages` mock.
- Produces: A direct test that fails until the i18n key and visible component node exist.

- [ ] **Step 1: Add exact contract constants and assertions.**

  Add the following test near the existing locale/source contract test, without changing production code:

  ```ts
  it('keeps the CNY/USD quota relationship visible in both locale bundles and the lazy page', async () => {
    const zhCopy = '额度口径：1 USD 额度 = 1 CNY 额度（仅用于额度关系理解，不是汇率换算）'
    const enCopy = 'Quota basis: 1 USD quota = 1 CNY quota (for understanding the quota relationship only; not an exchange-rate conversion)'

    expect(pageSource).toContain("t('admin.accountProfitability.quotaParityNote')")
    expect(zhAdmin.accountProfitability.quotaParityNote).toBe(zhCopy)
    expect(enAdmin.accountProfitability.quotaParityNote).toBe(enCopy)

    const wrapper = mountPage()
    await flushPromises()
    const note = wrapper.get('[data-test="quota-parity-note"]')
    expect(note.text()).toBe(zhCopy)
    await wrapper.get('[data-test="view-cny"]').trigger('click')
    expect(wrapper.get('[data-test="quota-parity-note"]').text()).toBe(zhCopy)
  })
  ```

  Keep the test's current mocked locale as Chinese so the mount assertion proves default visible output; the source and locale assertions cover both production language bundles.

- [ ] **Step 2: Run the focused test and verify the intended RED.**

  Run:

  ```bash
  cd upstream/sub2api/frontend
  pnpm exec vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts
  ```

  Expected: the new test fails because `pageSource` has no `quotaParityNote` call and the locale objects have no `quotaParityNote` field. Existing T33 tests may continue to pass; do not alter them to hide the missing contract.

### Task 2: Implement the i18n copy and visible page node (GREEN)

**Files:**
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts` (only add the new key to the existing `messages` test mock)

**Interfaces:**
- Consumes: the `quotaParityNote` key and exact locale values from Task 1.
- Produces: A visible `p[data-test="quota-parity-note"]` beside the USD/CNY segmented view, backed by `t('admin.accountProfitability.quotaParityNote')`.

- [ ] **Step 1: Add the Chinese locale value.**

  In the `accountProfitability` object in `src/i18n/locales/zh/admin/index.ts`, add:

  ```ts
  quotaParityNote: '额度口径：1 USD 额度 = 1 CNY 额度（仅用于额度关系理解，不是汇率换算）',
  ```

- [ ] **Step 2: Add the English locale value.**

  In the matching object in `src/i18n/locales/en/admin/index.ts`, add:

  ```ts
  quotaParityNote: 'Quota basis: 1 USD quota = 1 CNY quota (for understanding the quota relationship only; not an exchange-rate conversion)',
  ```

- [ ] **Step 3: Add the page node next to the segmented view.**

  Immediately after the existing `role="group"` div containing `view-usd` and `view-cny`, add:

  ```vue
  <p class="text-xs text-gray-500" data-test="quota-parity-note">
    {{ t('admin.accountProfitability.quotaParityNote') }}
  </p>
  ```

  The node must be outside the `activeView` branches so it stays visible while either view is selected and while CNY is loading. Do not add a new computed property or API call.

- [ ] **Step 4: Add the locale key to the existing Vitest mock.**

  In the `messages` object of `AccountProfitabilityView.spec.ts`, add:

  ```ts
  'admin.accountProfitability.quotaParityNote': '额度口径：1 USD 额度 = 1 CNY 额度（仅用于额度关系理解，不是汇率换算）',
  ```

- [ ] **Step 5: Run the focused test and verify GREEN.**

  Run:

  ```bash
  cd upstream/sub2api/frontend
  pnpm exec vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts
  ```

  Expected: all tests in the file pass, including the new quota-parity contract. The test must still observe the same API call counts and CNY view behavior as before.

### Task 3: Direct verification and candidate handoff

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts` only if verification exposes a test-only cleanup needed for the new key.
- Create: `docs/handoffs/2026-08-20-t36-profitability-quota-parity-handoff.md`

**Interfaces:**
- Consumes: the green page and locale contract from Task 2.
- Produces: A `READY_FOR_ROOT_REVIEW` handoff bound to the candidate HEAD, with direct test/build evidence and explicit scope checks.

- [ ] **Step 1: Run direct component and locale tests.**

  Run:

  ```bash
  cd upstream/sub2api/frontend
  pnpm exec vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts src/i18n/__tests__/localesMessageCompile.spec.ts
  ```

  Expected: the page contract and both locale message compilation cases pass.

- [ ] **Step 2: Run required frontend checks.**

  Run:

  ```bash
  cd upstream/sub2api/frontend
  pnpm typecheck
  pnpm build
  cd ../../..
  git diff --check
  ```

  Expected: typecheck, production build, and diff check exit successfully. Existing Vite chunk/dynamic-import warnings may remain; no new route or manual chunk is introduced.

- [ ] **Step 3: Confirm the diff boundary.**

  Run:

  ```bash
  git diff --name-only
  git diff -- upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts
  ```

  Expected: only the four direct frontend files plus the T36 spec/plan/handoff are changed; no backend, SQL, migration, configuration, global queue, progress ledger, production evidence, or deployment files appear.

- [ ] **Step 4: Write the handoff and record candidate state.**

  The handoff must include task `T36`, refreshed baseline `main` SHA `180ddd25b`, the refreshed candidate integration SHA/tree, changed files, direct test/typecheck/build/diff results, no migration/config/data changes, expected `downtime_required=false`, rollback to the previous verified `main` image, and the remaining risk that root still owns merge/deploy/online verification.

- [ ] **Step 5: Commit the candidate and stop at the root-review gate.**

  ```bash
  git add upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue \
    upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts \
    upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts \
    upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts \
    docs/superpowers/specs/2026-08-20-t36-profitability-quota-parity-copy-design.md \
    docs/superpowers/plans/2026-08-20-t36-profitability-quota-parity-copy.md \
    docs/handoffs/2026-08-20-t36-profitability-quota-parity-handoff.md
  git commit -m "fix: clarify profitability quota parity"
  ```

  Report `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, modify `main`, or edit global queue/progress files.

## Plan Self-Review

- **Spec coverage:** The fixed copy, visible node, bilingual locale values, lazy route boundary, no-data-flow change, direct RED/GREEN test, locale compilation, typecheck, build, diff boundary, rollback and root-only deployment gate are each covered above.
- **Placeholder scan:** No `TBD`, `TODO`, or unspecified implementation step remains. The only runtime values are the exact strings and existing `t()` key named in the spec.
- **Type consistency:** `quotaParityNote` is a string property on both locale `accountProfitability` objects; the component consumes it through `t()`; the test reads the same key from both locale objects and the rendered node.
