# T17 用量详情有效账号成本口径热修实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** 让管理员用量详情的“上游扣费/利润”始终使用 Sub 原生 effective account cost，并把严格 evidence 降级为辅助核验状态。

**Architecture:** 复用现有 `AdminUsageLog` 四个成本字段，在前端 `usageDetail.ts` 增加一个纯函数按原生 COALESCE 公式计算有效账号成本。详情组件用该纯函数计算主金额和利润；`getCostEvidence` 保留为异步辅助状态/原因，不再参与主金额或利润选择。后端 DTO/API 不改，只用现有管理员详情字段和既有证据兼容层。

**Tech Stack:** Vue 3 + TypeScript + Vitest；Go + Gin + existing DTO/handler tests；pnpm；Go fmt/build。

## Global Constraints

- 只修改 T17 直接相关的原生详情组件、公式 helper 和直接相关测试。
- 主成本公式必须是 `COALESCE(account_cost, COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1))`；0 是有效值，不能使用真假判断。
- `usage_upstream_cost_evidence` 只显示严格核验状态/原因；unavailable 或 evidence 请求失败不得隐藏主金额或改变利润。
- 不修改价格、倍率、成本写入、经营页聚合、普通用户 DTO、数据库、生产数据或历史 evidence 表。
- 不增加迁移、配置、依赖、外部控制面或 GitHub Actions。
- 先 RED 后 GREEN；每个新行为测试必须先运行并确认按预期失败。
- 候选完成后停在 READY_FOR_ROOT_REVIEW，不自行合并、推送、部署。

---

### Task 1: 添加并验证 effective account cost 纯函数

**Files:**
- Create: `upstream/sub2api/frontend/src/components/usage/__tests__/usageDetail.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/usage/usageDetail.ts`

**Interfaces:**
- Produces `effectiveAccountCost(input: { account_cost?: number | null; account_stats_cost?: number | null; total_cost: number | null | undefined; account_rate_multiplier?: number | null }): number | null`.
- Uses finite numeric values and nullish fallback; preserves zero.

- [ ] **Step 1: Write failing tests**

Create table-driven Vitest cases covering:
```ts
expect(effectiveAccountCost({ account_cost: 0.00331446, account_stats_cost: 0.01, total_cost: 0.02, account_rate_multiplier: 0.25 })).toBeCloseTo(0.00331446)
expect(effectiveAccountCost({ account_cost: null, account_stats_cost: 0.01, total_cost: 0.02, account_rate_multiplier: 0.25 })).toBeCloseTo(0.0025)
expect(effectiveAccountCost({ account_cost: null, account_stats_cost: null, total_cost: 0.02, account_rate_multiplier: 0.25 })).toBeCloseTo(0.005)
expect(effectiveAccountCost({ account_cost: 0, account_stats_cost: 0.01, total_cost: 0.02, account_rate_multiplier: 0 })).toBe(0)
expect(effectiveAccountCost({ account_cost: null, account_stats_cost: null, total_cost: null, account_rate_multiplier: null })).toBeNull()
```

- [ ] **Step 2: Run RED**

Run from `upstream/sub2api/frontend`:
```bash
pnpm exec vitest run src/components/usage/__tests__/usageDetail.spec.ts
```
Expected: FAIL because `effectiveAccountCost` is not exported.

- [ ] **Step 3: Implement the minimal helper**

In `usageDetail.ts`, add a finite-number guard and implement:
```ts
export function effectiveAccountCost(input: {
  account_cost?: number | null
  account_stats_cost?: number | null
  total_cost?: number | null
  account_rate_multiplier?: number | null
}): number | null {
  const multiplier = Number.isFinite(input.account_rate_multiplier)
    ? input.account_rate_multiplier as number
    : 1
  const source = Number.isFinite(input.account_cost)
    ? input.account_cost as number
    : Number.isFinite(input.account_stats_cost)
      ? input.account_stats_cost as number
      : input.total_cost
  return Number.isFinite(source) ? (source as number) * multiplier : null
}
```
Do not use `||` for numeric selection.

- [ ] **Step 4: Run GREEN**

Run the same Vitest command and expect all helper cases PASS.

- [ ] **Step 5: Commit**

```bash
git add src/components/usage/usageDetail.ts src/components/usage/__tests__/usageDetail.spec.ts
git commit -m "feat: centralize effective account cost formula"
```

---

### Task 2: Make UsageDetailDialog use native cost and preserve evidence as auxiliary

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue`
- Modify: `upstream/sub2api/frontend/src/components/usage/__tests__/UsageDetailDialog.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/usage/__tests__/UsageDetailDialog.compat.spec.ts` only if compatibility assertions need updated fixtures

**Interfaces:**
- Consumes `effectiveAccountCost(AdminUsageLog)`.
- Keeps `adminUsageAPI.getCostEvidence(id)` and its normalized PascalCase/snake_case response.
- Produces `upstreamActualCostValue = formatCost(effectiveAccountCost(adminDetail))` and `profitValue = formatCost(actual_cost - effectiveAccountCost)`.

- [ ] **Step 1: Write failing component regressions**

Add cases to `UsageDetailDialog.spec.ts`:
1. `account_cost` is present and evidence is unavailable: with `actual_cost=0.00688`, `account_cost=0.00331446`, evidence `{ evidence_status: 'unavailable', reason_code: 'endpoint_unavailable' }`, expect upstream `$0.003314` and profit `$0.003566`, while unavailable helper text remains.
2. `account_cost` is null and historical fallback uses `account_stats_cost * account_rate_multiplier`: with stats `0.01`, multiplier `0.25`, expect `$0.002500` and profit `$0.004380`.
3. evidence request rejects after detail succeeds: expect the same native cost/profit rather than `-`.

Use the existing `adminGetById` and `adminGetCostEvidence` hoisted stubs; do not add a second HTTP client or synthetic production data.

- [ ] **Step 2: Run RED**

Run:
```bash
pnpm exec vitest run src/components/usage/__tests__/UsageDetailDialog.spec.ts
```
Expected: the new cases fail because the component still uses `normalized_cost_cny` and returns `-` for unavailable evidence.

- [ ] **Step 3: Implement minimal component change**

Import `effectiveAccountCost` and replace `confirmedActualCost`/profit selection with:
```ts
const effectiveAccountCostValue = computed(() => (
  adminDetail.value ? effectiveAccountCost(adminDetail.value) : null
))
const upstreamActualCostValue = computed(() => (
  effectiveAccountCostValue.value == null ? '-' : formatCost(effectiveAccountCostValue.value)
))
const profitValue = computed(() => {
  if (effectiveAccountCostValue.value == null || !adminDetail.value) return '-'
  return formatCost(adminDetail.value.actual_cost - effectiveAccountCostValue.value)
})
```
Delete only the now-unused evidence-as-primary computed values. Keep evidence status/reason message and `adminCostDetail` rendering. When `getCostEvidence` rejects, leave `adminCostDetail=null` and do not touch `detail`.

- [ ] **Step 4: Run GREEN**

Run:
```bash
pnpm exec vitest run src/components/usage/__tests__/UsageDetailDialog.spec.ts src/components/usage/__tests__/UsageDetailDialog.compat.spec.ts
```
Expected: all existing and new cases PASS, including PascalCase compatibility.

- [ ] **Step 5: Commit**

```bash
git add src/components/usage/UsageDetailDialog.vue src/components/usage/__tests__/UsageDetailDialog.spec.ts src/components/usage/__tests__/UsageDetailDialog.compat.spec.ts
git commit -m "fix: use native account cost in usage detail"
```

---

### Task 3: Confirm API/DTO contract and direct regression coverage

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/admin/usage_handler_detail_test.go` only to add explicit cost-field assertions if absent.
- Modify: `upstream/sub2api/backend/internal/handler/dto/mappers_usage_test.go` only if the admin mapper lacks a direct account-cost serialization assertion.
- No production backend file should change unless a test demonstrates an actual missing field.

**Interfaces:**
- Admin endpoint remains `GET /api/v1/admin/usage/:id`.
- Admin JSON contains `account_rate_multiplier`, `account_stats_cost`, `account_cost`, `actual_cost`, `total_cost`.
- User endpoint remains forbidden from exposing admin cost fields.

- [ ] **Step 1: Inspect existing API contract tests**

Run:
```bash
go test ./internal/handler/admin -run 'TestAdminUsageGetByIDReturnsAdminProjection' -count=1
go test ./internal/handler/dto -run 'Test.*Usage.*Mapper|Test.*Admin.*Usage' -count=1
```
If assertions for all four fields already exist and pass, retain them as the API regression evidence and do not modify backend production code.

- [ ] **Step 2: Add only missing assertions**

If an assertion is missing, extend the existing test fixture with non-null `account_stats_cost` and `account_cost`, then assert JSON values. Because this is a guard for an already-existing contract, record that it is a characterization test; it must not introduce a new endpoint or source.

- [ ] **Step 3: Run focused backend tests**

Run:
```
go test ./internal/handler/admin -run 'TestAdminUsageGetByID|TestAdminUsageGetUpstreamCost' -count=1
go test ./internal/handler/dto -run 'Test.*Usage.*' -count=1
go test ./internal/server -run '^$' -count=1
```
Expected: PASS or an existing unrelated compile blocker documented without broadening scope.

- [ ] **Step 4: Commit if changed**

```bash
git add internal/handler/admin/usage_handler_detail_test.go internal/handler/dto/mappers_usage_test.go
git commit -m "test: guard admin usage cost field contract"
```

---

### Task 4: Final focused validation and T17 handoff

**Files:**
- Create: `docs/superpowers/reports/2026-08-17-t17-effective-account-cost-hotfix-verification.md`
- Create: `docs/handoffs/2026-08-17-t17-effective-account-cost-hotfix-handoff.md`

- [ ] **Step 1: Run the complete direct validation set**

Frontend:
```bash
pnpm exec vitest run src/components/usage/__tests__/usageDetail.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts src/components/usage/__tests__/UsageDetailDialog.compat.spec.ts
pnpm typecheck
pnpm build
```

Backend:
```bash
go test ./internal/handler/admin -run 'TestAdminUsageGetByID|TestAdminUsageGetUpstreamCost' -count=1
go test ./internal/handler/dto -run 'Test.*Usage.*' -count=1
go test ./internal/server -run '^$' -count=1
go build ./cmd/server
gofmt -w internal/handler/admin/usage_handler_detail_test.go internal/handler/dto/mappers_usage_test.go
```

Scope:
```bash
git diff --check main..HEAD
git diff --name-only main..HEAD -- .github/workflows | wc -l
git diff --name-only main..HEAD -- upstream/sub2api/backend/migrations | wc -l
```
Expected: focused tests/typecheck/build pass; workflow and migration diff counts 0.

- [ ] **Step 2: Write verification report**

Bind the report to the final candidate SHA and list:
- T17 scope and baseline;
- changed files and exact test outputs;
- formula matrix and evidence-unavailable behavior;
- migration/config/dependency/workflow deltas (all zero);
- unverified full-suite/pressure/soak/mutation/browser items;
- `downtime_required=unverified until root preflight`;
- functional rollback (disable no runtime flag; binary rollback to previous immutable image).

- [ ] **Step 3: Write handoff**

State status `READY_FOR_ROOT_REVIEW`, candidate SHA, clean worktree, no production changes, and explicit root instructions:
`AUTHORIZE_MERGE_TO_MAIN` -> merge -> focused root gates -> push -> release preflight -> if false direct blue-green; if true pause for authorization.

- [ ] **Step 4: Commit and stop**

```bash
git add docs/superpowers/reports/2026-08-17-t17-effective-account-cost-hotfix-verification.md docs/handoffs/2026-08-17-t17-effective-account-cost-hotfix-handoff.md
git commit -m "docs: hand off T17 effective account cost hotfix"
git status --short --branch
```

Stop at `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or modify global queue/ledger.

