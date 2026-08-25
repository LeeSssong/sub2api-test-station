# T63 Business Overview Zero-Default Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the native admin business overview always calculate from `usage_logs`, remove visible pending semantics, and render all missing numeric data as zero.

**Architecture:** Keep the existing `/admin/operations/business-overview` endpoint and `BusinessOverviewService`. Change its read model so `usage_logs.actual_cost` is the revenue and trend-consumption fact, keep the approved effective account-cost expression for upstream cost, and treat empty/NULL numeric inputs as zero. Keep T55 ledger/wallet reads only for current recharge and snapshot details, with missing optional rows/tables projected as zero and no pending status. Update the existing Vue view and parser without creating a second accounting source.

**Tech Stack:** Go service + `sqlmock`/`testify`, Gin admin handler contract, Vue 3 + TypeScript, Vitest + Vue Test Utils, pnpm typecheck/build.

**Spec:** `docs/superpowers/specs/2026-08-25-t63-business-overview-zero-default-design.md`

## Global Constraints

- Use only Sub native `usage_logs`, T55 wallet/ledger reads, existing admin route and existing page; do not add a parallel ledger or external control plane.
- Revenue is `usage_logs.actual_cost`; upstream cost is `COALESCE(account_cost, COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1))`, with missing/non-finite row values treated as 0.
- No historical recharge backfill, ledger writes, migrations, configuration changes, production-data writes, or GitHub Actions.
- Visible business-overview output must not contain “待确认”, “口径待确认”, `pending_split`, `pending_cost`, or `unavailable` business-result semantics.
- Real database/query failures remain errors; empty result sets and NULL numeric values become 0.
- Candidate worktree only; do not modify root `main`, queue, progress ledger, release evidence, or production state from the candidate.

### Task 1: Replace backend pending-gated aggregation with zero-default native usage aggregation

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/business_overview.go`
- Test: `upstream/sub2api/backend/internal/service/business_overview_test.go`

**Interfaces:**
- Consumes: existing `BusinessOverviewQuery` and `BusinessOverviewService.GetReport`.
- Produces: the same `BusinessOverviewReport` JSON shape, with compatibility fields fixed to `revenue_status="confirmed"`, `pending_split_count=0`, and `pending_cost_count=0`; all numeric result fields are finite values.

- [ ] **Step 1: Add RED service tests for the new contract.**

  Replace the tests that assert pending revenue with focused cases for:

  ```go
  func TestBusinessOverviewUsesActualCostAndZeroDefaults(t *testing.T) {
      // usage row: actual_cost=23.35, account cost=95.31, no matching ledger row
      // expect revenue=23.35, cost=95.31, profit=-71.96, margin≈-3.0818,
      // status="confirmed", pending counters 0, group values equal summary values.
  }

  func TestBusinessOverviewEmptyAndNullValuesBecomeZero(t *testing.T) {
      // empty usage result plus optional empty ledger/wallet results
      // expect every numeric summary/cash/trend value to be 0 and balanced reconciliation.
  }

  func TestBusinessOverviewExcludesUnknownAttempts(t *testing.T) {
      // return one unknown attempt and one complete row; expect only complete row in
      // request/revenue/cost/group/trend totals.
  }

  func TestBusinessOverviewDatabaseFailureRemainsError(t *testing.T) {
      // usage query error must be returned instead of a zero report.
  }
  ```

- [ ] **Step 2: Run the focused Go tests and verify RED.**

  Run:

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/service -run 'TestBusinessOverview' -count=1
  ```

  Expected: FAIL because the current service still returns `pending_split`/nil values and does not select `actual_cost` or filter `usage_completeness='unknown'`.

- [ ] **Step 3: Extend the usage row and SQL projection.**

  Add `ActualCost sql.NullFloat64` and `UsageCompleteness sql.NullString` to `businessOverviewUsageRow`. Change the usage query to select:

  ```sql
  ul.id,
  ul.created_at,
  ul.group_id,
  COALESCE(g.name, ''),
  COALESCE(ul.model, ''),
  COALESCE(ul.actual_cost, 0),
  COALESCE(ul.account_cost,
    COALESCE(ul.account_stats_cost, ul.total_cost)
      * COALESCE(ul.account_rate_multiplier, 1),
    0),
  COALESCE(ul.usage_completeness, 'complete')
  ```

  Add `AND COALESCE(ul.usage_completeness, 'complete') <> 'unknown'` to the WHERE clause. Update sqlmock row columns in the service tests.

- [ ] **Step 4: Implement minimal native aggregation.**

  In the usage loop, add `actual_cost` to site and group revenue, add the SQL-projected cost to site and group upstream cost, and default invalid/non-finite values to zero. Remove the `byReference`/`pendingSplit` gate from revenue computation. Keep compatibility fields fixed to confirmed/zero. Make `finalizeBusinessOverviewSummary` always set gross profit and set gross margin to 0 when revenue is 0.

- [ ] **Step 5: Make ledger/wallet reads non-blocking for empty/missing optional data.**

  Retain current-period recharge events and wallet snapshot reads for cash/balance cards, but treat a missing T55 relation or empty result as an empty event/snapshot (all zeros), never as `pending`. Propagate non-schema SQL errors. Set reconciliation to `balanced` with zero difference and “本期无变动” when there are no current events; otherwise use the existing reconciliation helper.

- [ ] **Step 6: Derive consumption trend from usage rows.**

  Pass the filtered usage rows into `buildReport` (or build the trend before calling it). For every Beijing date in the selected range, sum `ActualCost`; combine current recharge events for `cash_recharge_cny`; calculate `net_settlement_cny = recharge - actual_cost`. Fill dates with zeros.

- [ ] **Step 7: Run the focused Go tests and verify GREEN.**

  Run:

  ```bash
  cd upstream/sub2api/backend
  gofmt -w internal/service/business_overview.go internal/service/business_overview_test.go
  go test ./internal/service -run 'TestBusinessOverview' -count=1
  ```

  Expected: PASS with no pending assertions and the new numeric contract covered.

- [ ] **Step 8: Commit the backend slice.**

  ```bash
  git add upstream/sub2api/backend/internal/service/business_overview.go \
    upstream/sub2api/backend/internal/service/business_overview_test.go
  git commit -m "fix: make business overview zero-default native usage"
  ```

### Task 2: Remove visible pending rendering and normalize the frontend contract

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/businessOverview.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/BusinessOverviewView.vue`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/BusinessOverviewView.spec.ts`

**Interfaces:**
- Consumes: the existing `BusinessOverviewReport` endpoint, including compatibility status fields.
- Produces: a page that renders numeric zeros, negative profit, 0.00% margin, `—` only for missing optional preset configuration, and no pending banner/text.

- [ ] **Step 1: Replace the existing pending UI test with RED tests.**

  Update the fixture to contain numeric revenue/cost/profit/margin and zero compatibility counters. Add assertions that:

  ```ts
  expect(wrapper.find('[data-test="business-card-revenue"]').text()).toContain('¥23.35')
  expect(wrapper.find('[data-test="business-card-profit"]').text()).toContain('-¥71.96')
  expect(wrapper.find('[data-test="business-card-margin"]').text()).toContain('-308.18%')
  expect(wrapper.find('[data-test="business-pending"]').exists()).toBe(false)
  expect(wrapper.text()).not.toContain('待确认')
  expect(wrapper.text()).not.toContain('口径待确认')
  ```

  Add an empty-report case where all numeric values are 0 and the page contains `¥0.00` and `0.00%`.

- [ ] **Step 2: Run the focused Vitest file and verify RED.**

  Run from `upstream/sub2api/frontend`:

  ```bash
  pnpm vitest run src/views/admin/__tests__/BusinessOverviewView.spec.ts
  ```

  Expected: FAIL because the current component renders the pending banner and converts null amounts to “口径待确认”.

- [ ] **Step 3: Implement the minimal frontend change.**

  In `BusinessOverviewView.vue`:

  - remove `isPending`, the pending `<p>`, and `formatMoneyOrPending`;
  - use `formatMoney(value: number) => ¥...` for every required monetary field;
  - use `formatPercent(value: number) => ...%`, including `0` as `0.00%`;
  - keep `preset_margin` nullable and render `—` only when the preset is absent;
  - render reconciliation status labels as “已平衡” or “本期无变动” instead of pending;
  - leave the existing CNY/Q explanatory copy and group/table structure unchanged.

  In `businessOverview.ts`, keep compatibility fields parseable but normalize all numeric result fields with `numberOrZero`; type required summary/cash/group amounts as `number`, leaving only `preset_margin` and optional configuration fields nullable.

- [ ] **Step 4: Run the focused Vitest file and verify GREEN.**

  ```bash
  cd upstream/sub2api/frontend
  pnpm vitest run src/views/admin/__tests__/BusinessOverviewView.spec.ts
  ```

  Expected: PASS; no pending text remains in the rendered page.

- [ ] **Step 5: Commit the frontend slice.**

  ```bash
  git add upstream/sub2api/frontend/src/api/admin/businessOverview.ts \
    upstream/sub2api/frontend/src/views/admin/BusinessOverviewView.vue \
    upstream/sub2api/frontend/src/views/admin/__tests__/BusinessOverviewView.spec.ts
  git commit -m "fix: remove pending state from business overview"
  ```

### Task 3: Direct validation, scope audit, and handoff

**Files:**
- Modify: `docs/superpowers/reports/2026-08-25-t63-business-overview-zero-default-handoff.md`

**Interfaces:**
- Consumes: the two implementation commits and their focused test evidence.
- Produces: a `READY_FOR_ROOT_REVIEW` handoff; no merge, push, deploy, or production verification from this candidate.

- [ ] **Step 1: Run the directly related backend checks.**

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/service -run 'TestBusinessOverview' -count=1
  go build ./cmd/server
  gofmt -w internal/service/business_overview.go internal/service/business_overview_test.go
  ```

- [ ] **Step 2: Run the directly related frontend checks.**

  ```bash
  cd upstream/sub2api/frontend
  pnpm vitest run src/views/admin/__tests__/BusinessOverviewView.spec.ts
  pnpm typecheck
  pnpm build
  ```

- [ ] **Step 3: Run scope and diff checks.**

  ```bash
  git diff --check HEAD~2..HEAD
  git diff --name-only HEAD~2..HEAD
  ! git diff --name-only HEAD~2..HEAD | rg '^\.github/workflows/'
  ! git diff HEAD~2..HEAD -- upstream/sub2api/backend/migrations
  ! git diff HEAD~2..HEAD -- docs/project/project-progress.md docs/project/native-sub-task-package-queue.md
  ```

  Expected: only the approved service, frontend, test, spec/plan/report files are changed; no migration, production-data, queue, or GitHub Actions changes occur in the candidate.

- [ ] **Step 4: Write the handoff report.**

  Record baseline `main@fbe32c725`, candidate commits, changed files, exact test output, no migration/config/data changes, expected `downtime_required=false`, rollback by previous verified blue/green slot, and remaining risk that production login-state verification is still root-controlled.

- [ ] **Step 5: Commit the handoff and stop at `READY_FOR_ROOT_REVIEW`.**

  ```bash
  git add docs/superpowers/reports/2026-08-25-t63-business-overview-zero-default-handoff.md
  git commit -m "docs: hand off business overview zero-default candidate"
  ```

## Plan self-review

- Spec coverage: the plan covers native source selection, zero defaults, compatibility status fields, group/trend aggregation, error semantics, UI copy, no migration/backfill, focused tests, build/typecheck, scope audit, release gates, rollback, and handoff.
- Placeholder scan: no TBD/TODO or unspecified implementation steps remain.
- Type consistency: backend continues returning `BusinessOverviewReport`; frontend parser and view consume the same endpoint, with required numeric fields normalized to `number` and only preset configuration remaining nullable.
