# T101 Account Monitor Evidence Semantics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render a truthful fixed 24-bucket real-request timeline and distinguish all-site profitability from group-specific profitability.

**Architecture:** The repository owns the fixed timeline contract by preallocating every requested account's buckets and overlaying SQL aggregates by bucket index. The existing Vue card consumes that contract and uses its existing `rankingScope` prop to choose all-site or group profitability semantics; no new API, state store, or component layer is introduced.

**Tech Stack:** Go 1.27, PostgreSQL, sqlmock, Vue 3, TypeScript 5.6, Vitest, Vue Test Utils, Tailwind/scoped CSS.

**Spec:** `docs/superpowers/specs/2026-08-31-t101-account-monitor-evidence-semantics-design.md`

## Global Constraints

- Return exactly `AccountMonitorTimelineLimit = 24` timeline points per requested account for normal account-monitor calls.
- Keep existing real-request deduplication, success/failure rules, TTFT P95 threshold, profitability formulas, scheduling, billing, and probe execution unchanged.
- Do not add migrations, configuration, dependencies, production data writes, or GitHub Actions.
- Preserve the existing quiet, compact operations-console visual language and WCAG 2.1 AA interaction behavior.

---

### Task 1: Fixed repository timeline projection

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go:976`

**Interfaces:**
- Consumes: `ListRealRequestTimelines(ctx context.Context, accountIDs []int64, since, until time.Time, bucketCount int)`.
- Produces: `map[int64][]service.AccountMonitorRealRequestTimelinePoint` with exactly `bucketCount` ordered points for every requested account.

- [x] **Step 1: Write the failing repository test**

Add `TestAccountMonitorRepositoryRealRequestTimelineKeepsEmptyBuckets` with two requested account IDs. Return SQL rows only for account 7 at bucket indexes 3 and 22, then assert both accounts have 24 points, bucket 0 is empty, bucket 3 contains the first aggregate, and bucket 23 ends at `until`.

```go
timelineRepo := NewAccountMonitorRepository(db).(service.AccountMonitorRealRequestTimelineRepository)
mock.ExpectQuery(`(?s)WITH usage_events AS.*bucket_index.*ORDER BY account_id, bucket_index`).
    WithArgs(sqlmock.AnyArg(), since, until, 3600.0).
    WillReturnRows(sqlmock.NewRows([]string{"account_id", "bucket_index", "request_count", "success_count", "failure_count", "ttft_p95_ms"}).
        AddRow(7, 3, 5, 4, 1, 6200.0).
        AddRow(7, 22, 2, 2, 0, 900.0))
got, err := timelineRepo.ListRealRequestTimelines(context.Background(), []int64{7, 8}, since, until, 24)
if err != nil { t.Fatal(err) }
if len(got[7]) != 24 || len(got[8]) != 24 { t.Fatalf("timeline lengths = %d/%d", len(got[7]), len(got[8])) }
if got[7][0].RequestCount != 0 || got[7][0].TTFTP95MS != nil { t.Fatalf("empty bucket = %#v", got[7][0]) }
if got[7][3].RequestCount != 5 || got[7][3].FailureCount != 1 { t.Fatalf("filled bucket = %#v", got[7][3]) }
if !got[7][23].EndAt.Equal(until) { t.Fatalf("last end = %s", got[7][23].EndAt) }
```

- [x] **Step 2: Run the test and verify RED**

Run: `go test -vet=off -p 1 -run '^TestAccountMonitorRepositoryRealRequestTimelineKeepsEmptyBuckets$' -count=1 ./internal/repository`

Expected: FAIL because account 7 has only two points and account 8 has no entry.

- [x] **Step 3: Implement fixed bucket initialization and indexed overlay**

Before the query, initialize `result[id]` for every account. Reuse `bucketSeconds` for boundaries and scanned-index placement. Ignore an impossible out-of-range row instead of panicking.

```go
for _, id := range accountIDs {
    points := make([]service.AccountMonitorRealRequestTimelinePoint, bucketCount)
    for index := range points {
        points[index].StartAt = since.Add(time.Duration(float64(index)*bucketSeconds) * time.Second).UTC()
        points[index].EndAt = since.Add(time.Duration(float64(index+1)*bucketSeconds) * time.Second).UTC()
    }
    result[id] = points
}
// after Scan
if index < 0 || index >= bucketCount { continue }
p.StartAt = result[id][index].StartAt
p.EndAt = result[id][index].EndAt
result[id][index] = p
```

- [x] **Step 4: Run focused repository verification**

Run: `go test -vet=off -p 1 -run '^(TestAccountMonitorRepositoryRealRequestTimelineKeepsEmptyBuckets|TestAccountMonitorRepositoryProjectMonitorV4)' -count=1 ./internal/repository`

Expected: PASS.

- [x] **Step 5: Format and commit the repository slice**

Run: `gofmt -w internal/repository/account_monitor_repo.go internal/repository/account_monitor_repo_test.go && git diff --check`

Commit: `git add upstream/sub2api/backend/internal/repository/account_monitor_repo.go upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go && git commit -m 'fix: preserve account monitor timeline buckets'`

### Task 2: Scope-aware profitability and honest chart controls

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue:33-53,429-443`
- Verify unchanged coverage: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

**Interfaces:**
- Consumes: existing `rankingScope: 'group' | 'global'` and `account.group_profitability`.
- Produces: all-site label `按分组查看`; group-specific percentage behavior; 24 empty fallback bars; chart/probe copy with distinct semantics.

- [x] **Step 1: Write failing card behavior tests**

```ts
it('routes all-site profitability to group views', () => {
  expect(mountCard({ rankingScope: 'global' }).get('[data-test="profit-rate-metric"]').text()).toContain('按分组查看')
  expect(mountCard({ rankingScope: 'group' }).get('[data-test="profit-rate-metric"]').text()).toContain('61.8%')
})

it('renders 24 empty real-request buckets and labels probe refresh separately', () => {
  const wrapper = mountCard({ account: { ...account, real_request_timeline: [] } })
  expect(wrapper.findAll('[data-test="real-request-bar"]')).toHaveLength(24)
  expect(wrapper.get('[data-test="timeline-section"]').text()).toContain('真实性能 · 真实请求')
  expect(wrapper.get('[data-test="refresh-account"]').text()).toContain('刷新探测状态')
  expect(wrapper.get('[data-test="refresh-account"]').attributes('title')).toContain('不生成真实请求样本')
})
```

- [x] **Step 2: Verify existing view scope propagation coverage**

The existing view already passes `rankingScope` to the real card and its 48-test suite covers global/group switching. The proposed stub-only assertion passed without a production change, so no duplicate View test or View edit was added.

```ts
expect(wrapper.findAllComponents(AccountMonitorCardStub)[0].props('rankingScope')).toBe('global')
await wrapper.get('[data-test="group-tab-3"]').trigger('click')
await flushPromises()
expect(wrapper.findAllComponents(AccountMonitorCardStub)[0].props('rankingScope')).toBe('group')
```

- [x] **Step 3: Run the two test files and verify RED/GREEN boundaries**

Run: `./node_modules/.bin/vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`

Observed RED was limited to the new card behavior assertions: the old card showed group profitability in global scope and used the old chart/probe copy. The existing View scope behavior was already green and required no implementation change.

- [x] **Step 4: Implement the minimal Vue changes**

```ts
const profitRateLabel = computed(() => {
  if (props.rankingScope === 'global') return '按分组查看'
  const profit = props.account.group_profitability
  if (!profit || !['confirmed', 'estimated'].includes(profit.status) || profit.profit_rate == null) {
    return profit?.status === 'no_real_request' ? '--' : '待确认'
  }
  return formatPercent(profit.profit_rate)
})
```

Use heading `真实性能 · 真实请求`, button text `刷新探测状态`, and accessible text `刷新主动探测状态，不生成真实请求样本`. Keep the existing event and disabled/loading behavior.

- [x] **Step 5: Run focused frontend tests and commit the UI slice**

Run: `./node_modules/.bin/vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`

Expected: PASS.

Commit: `git add upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts && git commit -m 'fix: clarify account monitor evidence scope'`

### Task 3: Integrated verification and handoff

**Files:**
- Create: `docs/superpowers/reports/2026-08-31-t101-account-monitor-evidence-semantics-verification.md`
- Create: `docs/handoffs/2026-08-31-t101-account-monitor-evidence-semantics-handoff.md`
- Modify: `docs/superpowers/plans/2026-08-31-t101-account-monitor-evidence-semantics.md`

**Interfaces:**
- Consumes: completed backend and frontend slices.
- Produces: reproducible verification evidence and root-integration handoff.

- [x] **Step 1: Run focused and static verification**

Backend commands:

```bash
go test -vet=off -p 1 -run '^(TestAccountMonitorRepositoryRealRequestTimelineKeepsEmptyBuckets|TestAccountMonitorRepositoryProjectMonitorV4)' -count=1 ./internal/repository
go build -p 1 -o /tmp/sub2api-t101-server ./cmd/server
```

Frontend commands:

```bash
./node_modules/.bin/vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
./node_modules/.bin/vue-tsc --noEmit
./node_modules/.bin/vite build
```

- [x] **Step 2: Run scope and diff checks**

Run: `git diff --check && git status --short && git diff main...HEAD --stat && git diff main...HEAD -- .github/workflows upstream/sub2api/backend/migrations`

Expected: clean diff check and no workflow or migration delta.

- [x] **Step 3: Perform browser visual checks**

Verify desktop 1440x900 and mobile 390x844. Confirm 24 stable bars, no text overlap, no full-page horizontal overflow, global/group profitability labels, keyboard focus, and probe action copy. Save screenshots under `output/playwright/t101/` as local evidence; do not commit sensitive sessions or credentials.

- [x] **Step 4: Record verification and handoff**

The report must list exact commands, results, screenshots, known baseline failures, scope exclusions, and release preflight status. The handoff must identify branch/HEAD, base commit, commits, rollback, no-migration/no-config status, and the production authorization gate.

- [x] **Step 5: Mark plan checkboxes and commit documentation**

Run: `git add docs/superpowers/plans/2026-08-31-t101-account-monitor-evidence-semantics.md docs/superpowers/reports/2026-08-31-t101-account-monitor-evidence-semantics-verification.md docs/handoffs/2026-08-31-t101-account-monitor-evidence-semantics-handoff.md && git commit -m 'docs: record t101 verification handoff'`

## Acceptance

- [x] Repository and UI both preserve 24 fixed timeline positions.
- [x] All-site cards show `按分组查看`; group cards retain correct rate semantics.
- [x] Probe refresh copy no longer implies that it creates real-request evidence.
- [x] Focused tests, typecheck, production build, diff check, and visual checks pass.
- [x] No migration, configuration, dependency, GitHub Actions, or production data delta exists.
