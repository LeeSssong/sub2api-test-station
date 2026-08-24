# T57 DOCX Business Analysis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the DOCX经营总览 as a native administrator operations page backed by a read-only CNY business-overview API.

**Architecture:** Add a DOCX-specific service and DTO without changing the existing USD account-financial contract. The service aggregates native `usage_logs` cost/group data and conditionally reads the T55 wallet/ledger read model; missing T55 tables or unresolved consumption splits produce explicit pending statuses and null financial values. Add a native admin route, a focused Vue page with CNY cards/trend/group table, and direct contract tests.

**Tech Stack:** Go, Gin, `database/sql`, sqlmock, Vue 3, TypeScript, Vitest, existing Chart.js/chart components, native admin middleware and router.

**Spec:** `docs/superpowers/specs/2026-08-24-t57-docx-business-analysis-design.md`

## Global Constraints

- `usage_logs` remains the only usage and upstream-cost fact source.
- T55 owns wallet/ledger writes; T57 is read-only and must not modify the T55 worktree.
- Unknown paid/gift split, missing cost, and unavailable wallet data use `null`/pending status, never fabricated zeroes.
- Keep `/admin/operations/account-financial` and `/admin/operations/account-profitability` USD contracts unchanged.
- Do not expose upstream names, suppliers, rankings, credentials, or a parallel control plane.
- All date buckets use `Asia/Shanghai`; request dates are inclusive and service intervals are half-open.
- No migration or production data writes are expected from T57; deployment still requires root release gates.

### Task 1: Define the business-overview domain contract

**Files:**
- Create: `upstream/sub2api/backend/internal/service/business_overview.go`
- Test: `upstream/sub2api/backend/internal/service/business_overview_test.go`

**Interfaces:**
- Produces `BusinessOverviewService`, `BusinessOverviewReport`, `BusinessOverviewSummary`, `BusinessOverviewCashBalance`, `BusinessOverviewTrend`, and `BusinessOverviewGroup` with JSON tags matching the spec.
- Produces `ParseBusinessOverviewRange` or an equivalent service-facing range helper used by the handler.

- [ ] **Step 1: Write failing tests for pure financial rules.** Cover `grossProfit`, zero-revenue `nil` margin, pending split null revenue, net settlement, balance reconciliation status, and inclusive Beijing date conversion.
- [ ] **Step 2: Run the focused service tests and confirm they fail for missing types/functions.**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'BusinessOverview' -count=1`
Expected: FAIL because the new contract is not defined.

- [ ] **Step 3: Implement the DTOs and pure helpers.** Use pointer numeric fields where the spec distinguishes unknown from zero. Keep status constants explicit: `confirmed`, `pending_split`, `pending`, `unavailable`, `balanced`, `unbalanced`.
- [ ] **Step 4: Run the focused service tests.**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'BusinessOverview' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit the domain contract.**

```bash
git add upstream/sub2api/backend/internal/service/business_overview.go upstream/sub2api/backend/internal/service/business_overview_test.go
git commit -m "feat: define business overview contract"
```

### Task 2: Implement read-only native aggregation with T55 degradation

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/business_overview.go`
- Test: `upstream/sub2api/backend/internal/service/business_overview_test.go`
- Create if needed: `upstream/sub2api/backend/internal/repository/business_overview_queries.go`

**Interfaces:**
- `NewBusinessOverviewService(db *sql.DB)` returns the service used by the admin handler.
- `GetReport(ctx context.Context, input BusinessOverviewQuery) (*BusinessOverviewReport, error)` reads native usage and optional T55 tables only.
- SQL uses `usage_logs.group_id`, the existing upstream cost expression, and stable T55 ledger fields. It never writes.

- [ ] **Step 1: Add sqlmock RED tests for native usage aggregation.** Assert cost expression, group aggregation, unassigned grouping, request/token counts, and absence of upstream names in the returned DTO.
- [ ] **Step 2: Add RED tests for T55 states.** Cover all-paid, all-gift, mixed request references, missing reference, missing T55 tables, and invalid/unknown cost. Assert `pending_split`/`pending` and null revenue/profit rather than zero.
- [ ] **Step 3: Add RED tests for trend, cash/ledger totals, opening/closing balances, `group_id` filtering, and Beijing day buckets.**
- [ ] **Step 4: Implement the read-only SQL and aggregation.** Query T55 through narrow read-model queries; detect `sql.Err` for missing tables/columns and convert it to `pending` only for the wallet portion. Preserve real usage cost when wallet data is unavailable.
- [ ] **Step 5: Implement status propagation and pointer-valued calculations.** A group becomes pending if any included usage cannot be split or cost cannot be confirmed; income and derived profit/margin remain null in that case.
- [ ] **Step 6: Run the focused backend tests and `gofmt`.**

Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/repository -run 'BusinessOverview' -count=1 && gofmt -w internal/service/business_overview.go internal/service/business_overview_test.go internal/repository/business_overview_queries.go`
Expected: PASS and no formatting diff.

- [ ] **Step 7: Commit the aggregation.**

```bash
git add upstream/sub2api/backend/internal/service/business_overview.go upstream/sub2api/backend/internal/service/business_overview_test.go upstream/sub2api/backend/internal/repository/business_overview_queries.go
git commit -m "feat: aggregate native business overview"
```

### Task 3: Wire the authenticated admin API

**Files:**
- Create: `upstream/sub2api/backend/internal/handler/admin/business_overview_handler.go`
- Create: `upstream/sub2api/backend/internal/handler/admin/business_overview_handler_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/handlers.go` or the existing handler container file
- Modify: `upstream/sub2api/backend/internal/handler/wire.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go`

**Interfaces:**
- `GET /api/v1/admin/operations/business-overview` returns `response.Success(c, report)` under the existing administrator middleware.
- Handler accepts `range`, `start_date`, `end_date`, `timezone`, and optional `group_id`; malformed inputs return 400.

- [ ] **Step 1: Write handler RED tests for valid custom range, shortcut ranges, invalid dates/timezone/group, and service error mapping.**
- [ ] **Step 2: Add an unauthenticated route contract test proving the route remains behind the existing admin group.**
- [ ] **Step 3: Implement the handler and dependency wiring following `AccountFinancialHandler` patterns.** Do not alter existing operations handlers.
- [ ] **Step 4: Run focused handler tests and a backend build.**

Run: `cd upstream/sub2api/backend && go test ./internal/handler/admin -run 'BusinessOverview' -count=1 && go build ./cmd/server`
Expected: PASS and build success.

- [ ] **Step 5: Commit the API wiring.**

```bash
git add upstream/sub2api/backend/internal/handler upstream/sub2api/backend/internal/service/wire.go upstream/sub2api/backend/internal/server/routes/admin.go
git commit -m "feat: expose business overview admin api"
```

### Task 4: Add the native Vue business overview page

**Files:**
- Create: `upstream/sub2api/frontend/src/api/admin/businessOverview.ts`
- Create: `upstream/sub2api/frontend/src/views/admin/BusinessOverviewView.vue`
- Create: `upstream/sub2api/frontend/src/views/admin/__tests__/BusinessOverviewView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/router/index.ts`
- Modify: `upstream/sub2api/frontend/src/components/layout/AppSidebar.vue`
- Modify: the existing admin locale files containing `admin.accountProfitability`/navigation keys

**Interfaces:**
- API client exports `businessOverviewAPI.getReport(params)` with typed `BusinessOverviewReport`.
- Route is `/admin/operations/business-overview`; page renders only native admin data and does not call the old USD endpoint.

- [ ] **Step 1: Write Vitest RED tests for API normalization and page structure.** Assert CNY currency, Q label, four fixed summary cards, pending split copy, no upstream labels, all date controls, trend series, group table, and refresh/error states.
- [ ] **Step 2: Implement typed API normalization without converting unknown numeric values to zero.** Keep `null` values intact.
- [ ] **Step 3: Implement the page with fixed section order, CNY formatting, explicit “Q is internal quota, not USD” copy, pending states, and a responsive trend chart/table.** Reuse existing layout and chart conventions; do not add a separate card-within-card page shell.
- [ ] **Step 4: Register the API, route, sidebar label, and locale keys.** Keep old profitability navigation and page behavior unchanged.
- [ ] **Step 5: Run focused Vitest, typecheck, and build.**

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/BusinessOverviewView.spec.ts && pnpm typecheck && pnpm build`
Expected: PASS, typecheck success, production build success.

- [ ] **Step 6: Commit the frontend slice.**

```bash
git add upstream/sub2api/frontend/src/api/admin/businessOverview.ts upstream/sub2api/frontend/src/views/admin/BusinessOverviewView.vue upstream/sub2api/frontend/src/views/admin/__tests__/BusinessOverviewView.spec.ts upstream/sub2api/frontend/src/api/admin/index.ts upstream/sub2api/frontend/src/router/index.ts upstream/sub2api/frontend/src/components/layout/AppSidebar.vue upstream/sub2api/frontend/src/locales
git commit -m "feat: add business overview admin page"
```

### Task 5: Candidate-level verification and handoff

**Files:**
- Create: `docs/superpowers/reports/2026-08-24-t57-docx-business-analysis-handoff.md`
- Modify: `docs/superpowers/specs/2026-08-24-t57-docx-business-analysis-design.md` only if the stable T55 contract requires a documented refresh

- [ ] **Step 1: Refresh the candidate onto the latest `main` that contains the finalized T55 contract.** Do not merge or modify the T55 worktree; resolve only T57-scope conflicts.
- [ ] **Step 2: Run direct backend tests, frontend tests, `go build ./cmd/server`, `pnpm typecheck`, `pnpm build`, and `git diff --check`.**
- [ ] **Step 3: Verify the diff contains no migration, wallet writes, payment-order writes, upstream names, credentials, or external control-plane dependency.**
- [ ] **Step 4: Record baseline SHA, candidate SHA, changed files, tests, T55 contract version, migration/config status, `downtime_required` expectation, rollback path, and remaining production verification in the handoff.**
- [ ] **Step 5: Commit the handoff and report `READY_FOR_ROOT_REVIEW`; do not push, merge, deploy, or mark DONE from this worktree.**

### Task 6: Root-controlled integration and production validation

**Files:**
- Root-only: `docs/project/project-progress.md`, `docs/project/native-sub-task-package-queue.md`, release evidence and production records

- [ ] **Step 1: Wait until T55 is merged, pushed, deployed, and online-verified; record the stable read contract.**
- [ ] **Step 2: Root refreshes T57 candidate to the post-T55 `main` and repeats Task 5 direct verification.**
- [ ] **Step 3: Root authorizes the single candidate merge, verifies merged `main`, and runs the required release preflight.**
- [ ] **Step 4: If `downtime_required=false`, continue the reviewed local/host blue-green chain; if true, stop before any stop/restart/switch and request explicit authorization.**
- [ ] **Step 5: Verify `/healthz`, `/readyz`, `/health`, authenticated business-overview API, pending/confirmed semantics, CNY/Q labels, date range, trend and group view against the deployed source.**
- [ ] **Step 6: Only after pushed, deployed, and verified evidence, update the root ledgers to `DONE` and archive/delete the candidate worktree according to the recovery rules.**
