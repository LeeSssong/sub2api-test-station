# Account Profitability Operations Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin operations page that lists per-account expense, actual billed revenue, profit amount, and profit margin for Sub2API, NewAPI, and self-purchased accounts across selectable date ranges.

**Architecture:** Add a single admin API endpoint that aggregates usage logs by account for the requested date range and enriches rows with account metadata, procurement fields, and stored balance-source evidence. The Vue page consumes that endpoint, renders summary KPIs, filters, a responsive table, and CSV export. Revenue is `SUM(actual_cost)`; relay expense is usage `account_cost` in USD. Self-purchased procurement expense remains explicitly in CNY because no configured FX rate exists; it is shown in CNY but excluded from USD profit/margin until conversion is configured. Rows missing a defensible cost or comparable currency are marked pending rather than treated as zero.

**Tech Stack:** Go/Gin admin handler, existing usage-log repository SQL, Vue 3 `<script setup>`, TypeScript, Tailwind, Vitest.

## Global Constraints

- Keep `docs/project/project-progress.md` status as 进行中 until pushed, deployed, and production-verified.
- Use existing admin auth, API client, AppLayout, sidebar, i18n, and DataTable conventions.
- Support ranges today, 7d, 30d, month, and custom dates; default to month.
- Classify relay source from stored balance evidence (`sub2api` or `newapi`); otherwise treat as self-purchased when procurement data exists; otherwise pending.
- Never expose credentials or silently convert unknown expense to zero.

---

### Task 1: Add the account profitability aggregation contract

**Files:**
- Create: `upstream/sub2api/backend/internal/service/account_profitability.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go` and handler wiring as required by existing constructor patterns
- Modify: `upstream/sub2api/backend/internal/handler/admin/dashboard_handler.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go`
- Test: `upstream/sub2api/backend/internal/service/account_profitability_test.go`

**Interfaces:**
- Produces `GET /api/v1/admin/operations/account-profitability?start_date=YYYY-MM-DD&end_date=YYYY-MM-DD&timezone=Asia/Shanghai`.
- Response shape: `{start_date,end_date,generated_at,summary:{revenue,expense,profit,margin,account_count,pending_count},rows:[{account_id,name,platform,account_type,source,status,revenue,expense,profit,margin,expense_status,request_count,tokens,cost_basis}]}`.
- `source` values: `sub2api`, `newapi`, `self_purchased`, `pending`.

- [ ] Write failing service tests for revenue aggregation, source classification, procurement allocation, pending-cost behavior, and zero-revenue margin.
- [ ] Run the focused Go test and confirm it fails because the service is absent.
- [ ] Implement repository/service query using one grouped usage-log query joined to accounts, with `SUM(actual_cost)` and `SUM(COALESCE(account_stats_cost,total_cost) * COALESCE(account_rate_multiplier,1))`; allocate self-purchased procurement cost over the selected window using effective/expiry dates.
- [ ] Add handler parsing inclusive date range and timezone, route registration, and dependency wiring.
- [ ] Run focused service and handler tests.

### Task 2: Add admin API client and operations page

**Files:**
- Create: `upstream/sub2api/frontend/src/api/admin/accountProfitability.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/index.ts`
- Create: `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- Modify: `upstream/sub2api/frontend/src/router/index.ts`
- Modify: `upstream/sub2api/frontend/src/components/layout/AppSidebar.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts` and `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`

**Interfaces:**
- Page route: `/admin/operations/account-profitability`.
- API client method `adminAPI.accountProfitability.get(params)` returns the response contract from Task 1.

- [ ] Write failing component tests for default month range, source/status filters, summary totals, pending-cost display, and CSV export.
- [ ] Run the focused Vitest test and confirm failure.
- [ ] Implement API client and page using existing `AppLayout`, `TablePageLayout`/`DataTable`, `Icon`, and i18n patterns.
- [ ] Add date-range segmented controls, source filter, search, summary strip, dense desktop table, mobile stacked rows, loading/error/empty states, and CSV export.
- [ ] Add sidebar item under an Operations section using the existing chart/finance icon convention.
- [ ] Run the focused component test and TypeScript check.

### Task 3: Whole-feature validation and review

**Files:**
- Modify: `docs/project/project-progress.md` only for ongoing verification notes

- [ ] Run backend focused tests and frontend focused tests.
- [ ] Run frontend production build and existing relevant lint/type checks.
- [ ] Review route, API contract, date boundaries, source classification, and unknown-cost behavior against this plan.
- [ ] Run a local smoke check of the page and capture responsive screenshots if the dev server is available.
- [ ] Record that deployment and production verification remain outstanding; do not mark completed prematurely.
