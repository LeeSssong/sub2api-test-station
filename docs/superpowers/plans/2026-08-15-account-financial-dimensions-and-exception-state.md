# Account Financial Dimensions and Exception States Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the native administrator financial page present a fixed whole-site summary, group tabs with group-scoped account rows, and a cost-exception drill-down that always renders loading, empty, data, or retryable error content.

**Architecture:** Extend the existing `account-financial` snapshot and response additively. The backend aggregates group and `(group_id, account_id)` rows from immutable `usage_logs.group_id`; the frontend never derives group money from top-level account rows. Account-level overrides and OAuth daily costs remain whole-site/account facts and are surfaced to groups as unallocated adjustments rather than guessed allocations.

**Tech Stack:** Go, Ent, Gin, Vue 3 `<script setup>`, TypeScript, Tailwind CSS, Vitest, Vue Test Utils, pnpm.

## Global Constraints

- Use only native Sub2API data and APIs; do not introduce `/xingqiao/**`, an external control plane, a second ledger, historical backfill, delayed lookup, upstream retries, FX conversion, or procurement allocation.
- Keep the existing endpoint `GET /api/v1/admin/operations/account-financial?range=today|24h|7d|31d` and add response fields compatibly.
- Group amounts come from the persisted usage row's real `group_id`; top-level accounts remain whole-site account rows, and group account rows are keyed by `(group_id, account_id)`.
- Account-level today overrides and OAuth daily costs are not prorated into groups. Set `has_unallocated_adjustments=true` on affected groups.
- Keep today override and OAuth editing available only in the whole-site account view.
- The exception drill-down defaults to `review=pending` and preserves account and time-range filters.
- Do not add or modify database migrations, billing writes, scheduling behavior, dependencies, or GitHub Actions.
- Work in a dedicated `codex/account-financial-dimensions` worktree created from the latest clean `main`; do not modify global queue/progress files from the feature worktree.
- Each task gets a fresh implementer and an independent read-only task review. Run a fresh whole-branch review before `READY_FOR_ROOT_REVIEW`.

---

## File Responsibility Map

- `backend/internal/service/account_financial.go`: public report DTOs and all whole-site/account/group aggregation semantics.
- `backend/internal/repository/account_financial_repo.go`: one repeatable-read snapshot containing accounts, group identities, usage group identity, evidence, reviews, and daily values.
- `backend/internal/service/account_financial_test.go`: pure aggregation contract tests, including no double counting and unallocated adjustments.
- `backend/internal/repository/account_financial_repo_test.go`: source/query wiring guard for persisted `usage_logs.group_id` and group names.
- `frontend/src/api/admin/accountFinancial.ts`: normalize snake_case/PascalCase reports into typed whole-site, group, and group-account models.
- `frontend/src/views/admin/AccountProfitabilityView.vue`: fixed whole-site summary, group tabs, scoped summary/account table, and pending-exception navigation.
- `frontend/src/components/admin/usage/CostExceptionTable.vue`: loading, data, empty, and retryable error states.
- `frontend/src/views/admin/UsageView.vue`: reactive route restoration and explicit initial exception reload.
- Chinese/English admin locale files: labels for group scope, unassigned rows, unallocated adjustments, exception empty/error states.

---

### Task 1: Add Native Group Financial Projections

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_financial.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_financial_repo.go`
- Test: `upstream/sub2api/backend/internal/service/account_financial_test.go`
- Test: `upstream/sub2api/backend/internal/repository/account_financial_repo_test.go`

**Interfaces:**
- Consumes: existing `AccountFinancialRepository.ReadSnapshot(ctx, query)` and persisted `UsageLog.GroupID *int64`.
- Produces: `AccountFinancialReport.Groups []AccountFinancialGroupReport`.
- Produces: group rows whose `Accounts` are scoped to `(group_id, account_id)`, never copied from top-level `report.Accounts`.
- Produces: `Complete` and `HasUnallocatedAdjustments` flags for honest group reconciliation.

- [ ] **Step 1: Write failing service tests for real group attribution and no double counting**

Add a test shaped as follows:

```go
func TestAccountFinancialGroupsUseUsageGroupWithoutDoubleCountingWholeSite(t *testing.T) {
    now := beijingTime(t, "2026-08-15 12:00")
    report, err := NewAccountFinancialService(&financialRepoStub{snapshot: &AccountFinancialSnapshot{
        GeneratedAt: now,
        EnabledAt: now.Add(-time.Hour),
        Groups: []AccountFinancialSnapshotGroup{{ID: 10, Name: "Pro"}, {ID: 20, Name: "Plus"}},
        Accounts: []AccountFinancialSnapshotAccount{{ID: 7, Name: "shared", Type: "api_key"}},
        Entries: []AccountFinancialSnapshotEntry{
            {UsageLogID: 1, AccountID: 7, GroupID: financialInt64Ptr(10), BusinessDate: "2026-08-15", RevenueCNY: 10, EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(4)},
            {UsageLogID: 2, AccountID: 7, GroupID: financialInt64Ptr(20), BusinessDate: "2026-08-15", RevenueCNY: 20, EvidenceStatus: "confirmed", EvidenceCostCNY: financialFloatPtr(8)},
        },
    }}, func() time.Time { return now }).GetReport(context.Background(), AccountFinancialRangeToday)
    require.NoError(t, err)
    assertFinancialAmounts(t, report.Summary, 30, 12, 18, .6)
    require.Len(t, report.Groups, 2)
    assertFinancialAmounts(t, report.Groups[0].Amounts, 10, 4, 6, .6)
    assertFinancialAmounts(t, report.Groups[1].Amounts, 20, 8, 12, .6)
    require.Equal(t, int64(7), report.Groups[0].Accounts[0].ID)
}
```

Also add `TestAccountFinancialGroupsKeepNilGroupUnassigned` and assert that a `GroupID:nil` entry appears once under `Unassigned:true`, while the whole-site summary still counts it once.

- [ ] **Step 2: Run the focused tests and capture RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountFinancialGroups' -count=1
```

Expected: compile failure because `AccountFinancialSnapshotGroup`, `Groups`, and `GroupID` do not exist.

- [ ] **Step 3: Add explicit snapshot and response types**

Add these responsibilities to `account_financial.go`:

```go
type AccountFinancialSnapshotGroup struct {
    ID int64
    Name string
}

type AccountFinancialGroupReport struct {
    ID int64
    Name string
    Unassigned bool
    Complete bool
    HasUnallocatedAdjustments bool
    Amounts FinancialAmounts
    Accounts []*AccountFinancialAccountReport
    ExceptionCount int
    AffectedRevenueCNY float64
}

type AccountFinancialReport struct {
    GeneratedAt time.Time
    Range AccountFinancialRange
    Summary FinancialAmounts
    Accounts []*AccountFinancialAccountReport
    ExceptionCount int
    AffectedRevenueCNY float64
    UserBalanceCNY float64
    Groups []*AccountFinancialGroupReport
}
```

Extend `AccountFinancialSnapshot` with `Groups []AccountFinancialSnapshotGroup`, and extend `AccountFinancialSnapshotEntry` with `GroupID *int64` and `GroupName string`.

- [ ] **Step 4: Implement group aggregation from entries**

Add a private accumulator keyed by group identity and account ID. Its required behavior is:

```go
type financialGroupKey struct {
    ID int64
    Unassigned bool
}

// During the same entry loop used for whole-site account facts:
// 1. Resolve key from e.GroupID; nil becomes {ID:0, Unassigned:true}.
// 2. Create a group-scoped account row for e.AccountID.
// 3. For non-OAuth rows, reuse includeEntry(e).
// 4. Pending evidence increments group/account exception counts and affected revenue.
// 5. Confirmed/reviewed rows add revenue and cost once.
// 6. OAuth rows mark Complete=false and HasUnallocatedAdjustments=true; do not guess a group cost.
```

Finalize profit and margin only from included group-scoped amounts. Preserve `Margin=nil` when revenue is zero. Sort configured groups by their snapshot order, append historical referenced groups not in the current list, and append the unassigned projection last when it has activity.

- [ ] **Step 5: Write failing tests for account-level adjustments**

Add a test with two groups for one account plus a today override and assert:

```go
assertFinancialAmounts(t, report.Accounts[0].Amounts, 100, 40, 60, .6)
require.True(t, report.Groups[0].HasUnallocatedAdjustments)
require.True(t, report.Groups[1].HasUnallocatedAdjustments)
require.False(t, report.Groups[0].Complete)
require.False(t, report.Groups[1].Complete)
```

The group amounts must remain based on their original confirmed rows; the override delta must not be prorated or assigned to the unassigned group. Add the same expectation for literal OAuth daily cost.

- [ ] **Step 6: Mark affected groups without allocating adjustments**

For each in-range `AccountFinancialDailyValue` containing `RevenueOverrideCNY`, `CostOverrideCNY`, or `OAuthCostCNY`, find groups that contain an entry for the same account and business date. Mark those group and group-account rows incomplete with `HasUnallocatedAdjustments=true`. Do not change their group amounts.

- [ ] **Step 7: Persist real group identity in the snapshot**

In `ReadSnapshot`:

```go
groups, err := client.Group.Query().All(mixins.SkipSoftDelete(ctx))
if err != nil { return nil, err }
groupNames := make(map[int64]string, len(groups))
for _, g := range groups {
    groupNames[g.ID] = g.Name
    if g.DeletedAt == nil {
        s.Groups = append(s.Groups, service.AccountFinancialSnapshotGroup{ID: g.ID, Name: g.Name})
    }
}
```

When projecting each usage row, copy `u.GroupID` and resolve `GroupName` from `groupNames`. Never infer group identity from the account's current group memberships.

- [ ] **Step 8: Add repository guards and run GREEN**

Add repository tests that guard reading `UsageLog.GroupID` and group identity under the same repeatable-read transaction. Then run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountFinancial' -count=1
go test ./internal/repository -run 'TestAccountFinancial' -count=1
go test ./internal/handler/admin -run 'TestAccountFinancial' -count=1
```

Expected: PASS. If PostgreSQL integration tests require an unavailable Docker daemon, record them as unverified; do not silently replace them with mocks.

- [ ] **Step 9: Commit Task 1**

```bash
git add upstream/sub2api/backend/internal/service/account_financial.go \
  upstream/sub2api/backend/internal/service/account_financial_test.go \
  upstream/sub2api/backend/internal/repository/account_financial_repo.go \
  upstream/sub2api/backend/internal/repository/account_financial_repo_test.go
git commit -m "feat: add native group financial projections"
```

---

### Task 2: Build the Whole-Site, Group, and Account Page

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountFinancial.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/api/__tests__/admin.accountFinancial.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`

**Interfaces:**
- Consumes: additive `groups` response from Task 1.
- Produces: `FinancialGroup` and normalized group-scoped `FinancialAccount` rows.
- Produces: `jump(accountId)` route with `tab=cost-exceptions`, `review=pending`, `range`, and `account_id`.

- [ ] **Step 1: Write failing API normalization tests**

Add a mixed PascalCase/snake_case response containing one configured group and one unassigned group. Assert:

```ts
expect(report.groups[0]).toMatchObject({ id: 10, name: 'Pro', complete: true })
expect(report.groups[0].accounts[0].amounts.revenue).toBe(10)
expect(report.groups[1]).toMatchObject({ id: 0, unassigned: true })
```

Run:

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/api/__tests__/admin.accountFinancial.spec.ts
```

Expected: FAIL because `groups` is not normalized.

- [ ] **Step 2: Extend typed normalization**

Add:

```ts
export interface FinancialGroup {
  id: number
  name: string
  unassigned: boolean
  complete: boolean
  has_unallocated_adjustments: boolean
  amounts: FinancialAmounts
  accounts: FinancialAccount[]
  exception_count: number
  affected_revenue: number
}
```

Add `groups: FinancialGroup[]` to `AccountFinancialReport`. Extract a shared `normalizeAccount()` helper so top-level and group-scoped rows use identical field normalization. Default missing `groups` to `[]` for compatibility.

- [ ] **Step 3: Write failing page tests for the three-layer interaction**

Mount the page with a report containing Pro, Plus, and unassigned groups. Assert:

```ts
expect(wrapper.get('[data-test="scope-all"]').text()).toContain('全站')
expect(wrapper.get('[data-test="scope-group-10"]').text()).toContain('Pro')
await wrapper.get('[data-test="scope-group-10"]').trigger('click')
expect(wrapper.find('[data-test="group-summary-10"]').exists()).toBe(true)
expect(wrapper.find('[data-test="account-financial-7"]').exists()).toBe(true)
expect(wrapper.find('[data-test="account-financial-8"]').exists()).toBe(false)
```

Also assert that the six whole-site cards remain visible after switching groups, unallocated adjustments render a visible notice, and today override inputs exist only under `scope-all`.

- [ ] **Step 4: Implement stable scope controls and selected data**

Use a stable scope model:

```ts
type FinancialScope = { kind: 'all' } | { kind: 'group'; id: number; unassigned: boolean }
const activeScope = ref<FinancialScope>({ kind: 'all' })
const selectedGroup = computed(() => activeScope.value.kind === 'group'
  ? report.value.groups.find(group => group.id === activeScope.value.id && group.unassigned === activeScope.value.unassigned)
  : undefined)
const selectedAccounts = computed(() => selectedGroup.value?.accounts ?? report.value.accounts)
const selectedAmounts = computed(() => selectedGroup.value?.amounts ?? report.value.summary)
```

Render the existing six whole-site cards before the scope tabs. Below the tabs, render a compact selected-scope summary and the selected account rows. Use group-provided account rows; do not filter `report.accounts` by current membership.

- [ ] **Step 5: Preserve editing and add honest group notices**

Keep today override and OAuth inputs only when `activeRange === 'today' && activeScope.kind === 'all'`. For group rows with `has_unallocated_adjustments`, render localized text equivalent to “账号级覆盖或 OAuth 日成本未按比例分摊；分组仅显示可由真实流水归属的金额。”

- [ ] **Step 6: Fix the exception navigation contract**

Change the route push to:

```ts
router.push({
  path: '/admin/usage',
  query: {
    tab: 'cost-exceptions',
    review: 'pending',
    range: activeRange.value,
    account_id: String(accountId),
  },
})
```

Test the complete query object, not only `expect.objectContaining` for the tab.

- [ ] **Step 7: Add localized labels and responsive layout guards**

Add Chinese and English copy for: whole site, unassigned, group summary, incomplete group data, unallocated account adjustment, and scoped account count. Keep cards at the existing native radius/theme, tabs horizontally scrollable on narrow screens, and table content in an overflow container without changing shared layout classes.

- [ ] **Step 8: Run frontend GREEN and commit Task 2**

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/api/__tests__/admin.accountFinancial.spec.ts src/views/admin/__tests__/AccountProfitabilityView.spec.ts
pnpm typecheck
```

Expected: PASS.

```bash
git add upstream/sub2api/frontend/src/api/admin/accountFinancial.ts \
  upstream/sub2api/frontend/src/api/__tests__/admin.accountFinancial.spec.ts \
  upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue \
  upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts \
  upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts \
  upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts
git commit -m "feat: add account financial scope views"
```

---

### Task 3: Make Cost-Exception Drill-Down Always Visible

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/admin/usage/CostExceptionTable.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/usage/__tests__/CostExceptionTable.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/UsageView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`

**Interfaces:**
- Consumes: route query `tab`, `range`, `account_id`, `evidence`, and `review`.
- Produces: exactly one visible table-body state: loading, rows, empty, or error/retry.
- Produces: `reload(): Promise<void>` exposed to `UsageView`.

- [ ] **Step 1: Write failing component state tests**

Add three tests:

```ts
it('shows loading while the list is pending', async () => { /* deferred promise; assert data-test=cost-exceptions-loading */ })
it('shows an explicit empty state for zero rows', async () => { /* items:[], total:0; assert data-test=cost-exceptions-empty */ })
it('shows a retryable error and reloads', async () => { /* reject once, resolve next; click data-test=cost-exceptions-retry */ })
```

Run:

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/components/admin/usage/__tests__/CostExceptionTable.spec.ts
```

Expected: FAIL because these states do not exist and `reload` does not catch errors.

- [ ] **Step 2: Implement an explicit reload state machine**

Add `loading` and `loadError` refs. Implement:

```ts
const reload = async () => {
  loading.value = true
  loadError.value = ''
  try {
    const res = await adminUsageAPI.listCostExceptions(query.value)
    items.value = res.items
    total.value = res.total
    page.value = res.page
    pageSize.value = res.page_size
    selectedIds.value = []
  } catch {
    loadError.value = t('admin.costExceptions.loadError')
  } finally {
    loading.value = false
  }
}
```

In the table body, render one `colspan="13"` row for loading, error/retry, or empty before the normal `v-for`. Preserve existing rows and pagination behavior.

- [ ] **Step 3: Synchronize routed filter controls**

When `props.filters.review_status` or `evidence_status` changes, mirror it into the visible select values before reloading so the controls match the actual API query. An explicit user selection overrides the routed default until the route itself changes.

- [ ] **Step 4: Write failing UsageView route-mount test**

Use the real `CostExceptionTable` or an exposed reload spy. Start with:

```ts
routeQuery.tab = 'cost-exceptions'
routeQuery.range = 'today'
routeQuery.account_id = '42'
routeQuery.review = 'pending'
```

Assert that `listCostExceptions` is called exactly once with `account_id:42`, `review_status:'pending'`, and RFC3339 start/end times after initial mount.

- [ ] **Step 5: Apply routed state before the first child render**

Extract `normalizeDetailTab()` and `applyRouteState()`. Register a synchronous immediate watcher before `onMounted`:

```ts
const normalizeDetailTab = (value: unknown): DetailTab => {
  const tab = getSingleQueryValue(value as string | string[] | null | undefined)
  return tab === 'errors' || tab === 'cost-exceptions' || tab === 'ranking' ? tab : 'usage'
}

const applyRouteState = () => {
  applyRouteQueryFilters()
  activeTab.value = normalizeDetailTab(route.query.tab)
  if (activeTab.value === 'ranking') rankingMounted.value = true
}

watch(
  () => [route.query.tab, route.query.range, route.query.account_id, route.query.evidence, route.query.review],
  applyRouteState,
  { immediate: true, flush: 'sync' },
)
```

Remove the duplicate route-tab assignment from `onMounted`. Because `activeTab` is already `cost-exceptions` during the first render, `CostExceptionTable` mounts once and its existing `onMounted(reload)` owns the single initial request. Later query-filter changes flow through the component's existing filter watcher; do not add a second explicit initial reload.

- [ ] **Step 6: Run GREEN and commit Task 3**

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/components/admin/usage/__tests__/CostExceptionTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts
pnpm typecheck
```

Expected: PASS.

```bash
git add upstream/sub2api/frontend/src/components/admin/usage/CostExceptionTable.vue \
  upstream/sub2api/frontend/src/components/admin/usage/__tests__/CostExceptionTable.spec.ts \
  upstream/sub2api/frontend/src/views/admin/UsageView.vue \
  upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts \
  upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts \
  upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts
git commit -m "fix: render cost exception route states"
```

---

### Task 4: Integrated Verification and Handoff

**Files:**
- Create: `docs/superpowers/reports/2026-08-15-account-financial-dimensions-task-review.md`
- Create: `docs/superpowers/reports/2026-08-15-account-financial-dimensions-final-review.md`
- Create: `docs/superpowers/reports/2026-08-15-account-financial-dimensions-handoff.md`

**Interfaces:**
- Consumes: Tasks 1-3 committed candidate.
- Produces: a `READY_FOR_ROOT_REVIEW` handoff; does not merge, push, or deploy.

- [ ] **Step 1: Run the complete focused backend matrix**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountFinancial' -count=1
go test ./internal/repository -run 'TestAccountFinancial' -count=1
go test ./internal/handler/admin -run 'TestAccountFinancial' -count=1
go test ./internal/server/routes -run 'Test.*Financial|Test.*CostException' -count=1
```

- [ ] **Step 2: Run the complete focused frontend matrix**

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run \
  src/api/__tests__/admin.accountFinancial.spec.ts \
  src/views/admin/__tests__/AccountProfitabilityView.spec.ts \
  src/components/admin/usage/__tests__/CostExceptionTable.spec.ts \
  src/views/admin/__tests__/UsageView.spec.ts
pnpm typecheck
pnpm build
```

- [ ] **Step 3: Run repository and scope guards**

```bash
git diff --check
git status --short
git diff --name-only <baseline-main-sha>...HEAD
rg -n '/xingqiao|controlPlaneAPI|ReadModelStatus|external-primary' \
  upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue \
  upstream/sub2api/frontend/src/components/admin/usage/CostExceptionTable.vue \
  upstream/sub2api/backend/internal/service/account_financial.go
git diff <baseline-main-sha>...HEAD -- upstream/sub2api/backend/migrations .github/workflows
```

Expected: no forbidden control-plane symbols and no migration or GitHub Actions delta.

- [ ] **Step 4: Perform responsive visual QA**

Start the local frontend against a controlled mocked/native API fixture. Capture desktop `1440x1000` and mobile `390x844` screenshots with the repository Playwright workflow. Verify whole-site cards remain visible, tabs are usable without page-wide horizontal overflow, table text does not overlap, the group notice is readable, and loading/empty/error states occupy the content area below filters.

- [ ] **Step 5: Run independent reviews**

Require each task's reviewer to check spec compliance and code quality read-only. Then dispatch a fresh whole-branch reviewer over `<baseline-main-sha>...HEAD`. Fix every P0-P2 finding in the candidate and repeat affected tests/reviews.

- [ ] **Step 6: Write the handoff and commit evidence**

The handoff must include task name, baseline `main` SHA, candidate SHA, changed files, exact test results, visual evidence paths, unverified checks, migration/config/dependency changes, `downtime_required` expectation, rollback, and residual risk.

```bash
git add docs/superpowers/reports/2026-08-15-account-financial-dimensions-*.md
git commit -m "docs: hand off account financial dimension fix"
```

Stop at `READY_FOR_ROOT_REVIEW`. Do not merge `main`, push, deploy, or modify global queue/progress files from the feature worktree.
