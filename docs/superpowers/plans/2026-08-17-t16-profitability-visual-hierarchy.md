# T16 Profitability Visual Hierarchy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the native profitability report into internal operations and external business economics, then present the five approved result metrics in a responsive hierarchy.

**Architecture:** Extend the existing repeatable-read `usage_logs` aggregation with a read-only join to the current `users.role`, returning internal cost, business cost, and business revenue per immutable group/account pair. The service derives totals and keeps legacy financial fields mathematically aligned; the existing frontend API normalizer and page consume the new fields with backward-compatible fallbacks.

**Tech Stack:** Go, PostgreSQL SQL, sqlmock, Gin JSON handlers, Vue 3, TypeScript, Vitest, Tailwind CSS.

## Global Constraints

- Continue using the Sub native `usage_logs` ledger and the existing effective account cost formula.
- Internal means only current `users.role='admin'`; never infer identity from `user_cost=0`.
- Document that current roles can reclassify historical rows; do not migrate or backfill actor identity.
- Keep `cost/user_cost/profit/margin` compatible and mathematically aligned with the new fields.
- Do not change pricing, multipliers, billing writes, probe collection, or production data.
- Do not add migrations, dependencies, configuration, or GitHub Actions.
- Stop at `READY_FOR_ROOT_REVIEW`; do not merge root `main`, push the candidate, preflight, or deploy.

---

### Task 1: Repository role-aware native aggregation

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go`
- Test: `upstream/sub2api/backend/internal/repository/usage_log_repo_stats_test.go`
- Test: `upstream/sub2api/backend/internal/repository/usage_log_repo_stats_integration_test.go`

**Interfaces:**
- Consumes: `usage_logs.user_id`, `users.id`, `users.role`, and the existing effective account cost expression.
- Produces: `service.AccountFinancialUsageRow{OperationalCost, BusinessCost, BusinessRevenue}` for each group/account pair.

- [ ] **Step 1: Write the failing sqlmock contract**

Require the aggregation query to include `LEFT JOIN users u ON u.id = ul.user_id` and three conditional sums:

```sql
SUM(CASE WHEN COALESCE(u.role, '') = 'admin' THEN effective_account_cost ELSE 0 END)
SUM(CASE WHEN COALESCE(u.role, '') <> 'admin' THEN effective_account_cost ELSE 0 END)
SUM(CASE WHEN COALESCE(u.role, '') <> 'admin' THEN COALESCE(ul.actual_cost, 0) ELSE 0 END)
```

Return one mixed aggregate row and assert all three values are scanned without changing group/account identity.

- [ ] **Step 2: Run the focused test and confirm RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestReadAccountFinancialUsage' -count=1
```

Expected: failure because the old query lacks the role join and the row scanner lacks the new columns.

- [ ] **Step 3: Implement the minimal query and scanner extension**

Add the three fields to `AccountFinancialUsageRow`, update the SQL to compute them from the same effective-cost expression, and scan them. Treat only literal `admin` as internal; all other or missing roles remain business.

- [ ] **Step 4: Update the integration fixture assertions**

Extend existing native aggregation integration rows so an admin and normal user share an account/group, then assert operational cost, business cost, business revenue, and total conservation.

- [ ] **Step 5: Run repository tests and format**

```bash
cd upstream/sub2api/backend
gofmt -w internal/repository/usage_log_repo_stats.go internal/repository/usage_log_repo_stats_test.go internal/repository/usage_log_repo_stats_integration_test.go
go test ./internal/repository -run 'TestReadAccountFinancialUsage' -count=1
```

- [ ] **Step 6: Commit**

```bash
git add upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go upstream/sub2api/backend/internal/repository/usage_log_repo_stats_test.go upstream/sub2api/backend/internal/repository/usage_log_repo_stats_integration_test.go
git commit -m "feat: classify native profitability usage"
```

### Task 2: Service and API result contract

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_financial.go`
- Test: `upstream/sub2api/backend/internal/service/account_financial_test.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/account_financial_handler_test.go`

**Interfaces:**
- Consumes: repository row fields from Task 1.
- Produces: `FinancialAmounts` JSON fields `operational_cost`, `business_cost`, `business_revenue`, `total_cost`, `net_profit`, and `external_margin` while preserving aligned legacy fields.

- [ ] **Step 1: Write failing service tests**

Use a snapshot containing one mixed row:

```go
AccountFinancialUsageRow{
    AccountID: 1,
    OperationalCost: 2,
    BusinessCost: 3,
    BusinessRevenue: 8,
}
```

Assert all-site, account, group, and group-account amounts equal operational `2`, business cost `3`, business revenue `8`, total cost `5`, net profit `3`, and margin `0.375`. Add a zero-revenue case that returns a nil margin and a negative-profit case.

- [ ] **Step 2: Run service tests and confirm RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountFinancialReport' -count=1
```

Expected: compile failure or missing-field assertion failure.

- [ ] **Step 3: Implement aggregation and finalization**

Extend `FinancialAmounts` and the service `add` function. In `finalizeFinancialAmounts`, derive:

```go
amounts.TotalCost = amounts.OperationalCost + amounts.BusinessCost
amounts.NetProfit = amounts.BusinessRevenue - amounts.TotalCost
amounts.ExternalMargin = nil
if amounts.BusinessRevenue != 0 {
    margin := amounts.NetProfit / amounts.BusinessRevenue
    amounts.ExternalMargin = &margin
}
amounts.Cost = amounts.TotalCost
amounts.UserCost = amounts.BusinessRevenue
amounts.Profit = amounts.NetProfit
amounts.Margin = amounts.ExternalMargin
```

Keep revenue/expense compatibility derived from the finalized legacy values.

- [ ] **Step 4: Extend the handler JSON contract test**

Assert the response serializes all six new snake_case fields and that `cost == total_cost`, `user_cost == business_revenue`, `profit == net_profit`, and `margin == external_margin`.

- [ ] **Step 5: Run focused backend tests**

```bash
cd upstream/sub2api/backend
gofmt -w internal/service/account_financial.go internal/service/account_financial_test.go internal/handler/admin/account_financial_handler_test.go
go test ./internal/service -run 'TestAccountFinancialReport' -count=1
go test ./internal/handler/admin -run 'TestAccountFinancialReport' -count=1
```

- [ ] **Step 6: Commit**

```bash
git add upstream/sub2api/backend/internal/service/account_financial.go upstream/sub2api/backend/internal/service/account_financial_test.go upstream/sub2api/backend/internal/handler/admin/account_financial_handler_test.go
git commit -m "feat: expose profitability result dimensions"
```

### Task 3: Frontend API compatibility

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountFinancial.ts`
- Test: `upstream/sub2api/frontend/src/api/admin/__tests__/accountFinancial.spec.ts`

**Interfaces:**
- Consumes: snake_case or PascalCase backend responses, including legacy responses without T16 fields.
- Produces: typed `FinancialAmounts` with all six T16 fields and finite compatibility values.

- [ ] **Step 1: Write failing normalization tests**

Cover:

```ts
{
  operational_cost: 2,
  business_cost: 3,
  business_revenue: 8,
  total_cost: 5,
  net_profit: 3,
  external_margin: 0.375,
}
```

Also cover PascalCase and a legacy `{ cost: 5, user_cost: 8, profit: 3, margin: 0.375 }` response. Legacy fallback must produce operational `0`, business cost `5`, business revenue `8`, total cost `5`, net profit `3`, and external margin `0.375`.

- [ ] **Step 2: Run the API test and confirm RED**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/api/admin/__tests__/accountFinancial.spec.ts
```

- [ ] **Step 3: Extend types and normalizer**

Add exact numeric fields to `FinancialAmounts`. Read both naming styles, use existing finite-number helpers, and derive absent fields from the legacy contract without changing the HTTP route.

- [ ] **Step 4: Run API tests**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/api/admin/__tests__/accountFinancial.spec.ts
```

- [ ] **Step 5: Commit**

```bash
git add upstream/sub2api/frontend/src/api/admin/accountFinancial.ts upstream/sub2api/frontend/src/api/admin/__tests__/accountFinancial.spec.ts
git commit -m "feat: normalize profitability dimensions"
```

### Task 4: Responsive profitability hierarchy

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin.ts`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`

**Interfaces:**
- Consumes: normalized `FinancialAmounts` from Task 3 and existing range/group selection state.
- Produces: five-card summary, current-role caveat, five-metric account table, and stable metric sorting.

- [ ] **Step 1: Replace test fixtures with the T16 contract and write failing view tests**

Assert:

- exactly five summary cards;
- internal-cost card copy says it is included in total cost;
- account table contains operational cost, business cost, business revenue, total cost, and net profit;
- default all-site view includes every account;
- default order is net profit descending and every metric sorts in both directions;
- negative net profit has a warning class;
- main has `overflow-x-hidden`, summary has two mobile columns, and the table wrapper owns `overflow-x-auto`.

- [ ] **Step 2: Run the view test and confirm RED**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts
```

- [ ] **Step 3: Implement the summary and table**

Keep range buttons, group tabs, refresh/error/empty states, and account identity. Replace account cards with one semantic table inside an `overflow-x-auto` wrapper. Use stable fixed/minimum column widths so labels and values never resize the layout.

- [ ] **Step 4: Add localized labels and the role-history note**

Add Chinese and English labels for the five metrics, the “included in total cost” note, and the current-role historical classification caveat. Remove no existing translation keys needed by other consumers.

- [ ] **Step 5: Run view, API, type, and build gates**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/api/admin/__tests__/accountFinancial.spec.ts src/views/admin/__tests__/AccountProfitabilityView.spec.ts
pnpm typecheck
pnpm build
```

- [ ] **Step 6: Commit**

```bash
git add upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts upstream/sub2api/frontend/src/i18n/locales/zh/admin.ts upstream/sub2api/frontend/src/i18n/locales/en/admin.ts
git commit -m "feat: redesign profitability result hierarchy"
```

### Task 5: Candidate verification and handoff

**Files:**
- Create: `docs/handoffs/2026-08-17-t16-profitability-visual-hierarchy-handoff.md`

**Interfaces:**
- Consumes: Tasks 1-4 candidate commits.
- Produces: one clean `READY_FOR_ROOT_REVIEW` candidate with exact verification evidence.

- [ ] **Step 1: Run direct backend verification**

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestReadAccountFinancialUsage' -count=1
go test ./internal/service -run 'TestAccountFinancialReport' -count=1
go test ./internal/handler/admin -run 'TestAccountFinancialReport' -count=1
go test ./internal/repository ./internal/service ./internal/handler/admin ./internal/server/routes -run '^$'
```

- [ ] **Step 2: Run direct frontend verification**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/api/admin/__tests__/accountFinancial.spec.ts src/views/admin/__tests__/AccountProfitabilityView.spec.ts
pnpm typecheck
pnpm build
```

- [ ] **Step 3: Run repository hygiene checks**

```bash
git diff --check main...HEAD
git status --short
git diff --name-only main...HEAD
```

Confirm there are no migrations, dependencies, configuration, workflow, release, or unrelated task files.

- [ ] **Step 4: Write and commit the handoff**

Record baseline, final commit/tree, changed files, exact test commands/results, migration/config status, `downtime_required=unverified`, rollback, historical-role risk, and the explicit no-release state.

```bash
git add docs/handoffs/2026-08-17-t16-profitability-visual-hierarchy-handoff.md
git commit -m "docs: hand off T16 profitability candidate"
```

## Plan Self-Review

- Every specification requirement maps to a repository, service/API, frontend contract, view, or verification task.
- The plan has no implementation placeholders or unresolved product decisions.
- Field names and formulas are consistent across Go, JSON, TypeScript, and Vue tasks.
- The task remains a single vertical feature and stops before integration or deployment.
