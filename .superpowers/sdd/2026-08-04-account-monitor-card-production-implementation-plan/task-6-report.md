# Task 6 report — 账号监控 V3 设计还原

- status: DONE
- commit SHA: `e85dad1645efe000a3169e62529a8632ef7aa368`
- push/deploy: 未执行。

## Root-cause confirmation

Confirmed. Commit `2ea3ef167` replaced the card with a simplified upper-card implementation and removed the service-only recent-probe section, selected-window call disclosure, checked-at/statistics-cutoff footer, and per-card refresh button. It also left the platform selector in `AccountMonitorFilters.vue`. The previous focused tests protected the new score/metric upper region but did not render and assert the complete lower-card structure, so the regression passed.

## Files changed

- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.spec.ts`
- `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

No QA harness, project-progress file, acceptance contract, prototype, backend, deployment, production environment, or controller-owned temporary file was edited or committed.

## RED

Command:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorFilters.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
```

Expected failure: the rejected implementation has no green V3 shell/lower modules, retains two selectors including platform, and lacks the constrained monitor-page shell.

Exact initial RED summary:

```text
Test Files  3 failed (3)
Tests  5 failed | 17 passed (22)
```

The failures specifically reported the missing `border-l-4`/green-card shell, missing call disclosure, two rendered selects instead of one, and missing `account-monitor-page`.

Additional RED cycles were run after browser inspection and independent review. They caught the inline procurement action layout, optional-account-range disclosure bug, and actual save-button focus path before their fixes.

## GREEN

Focused frontend command:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorFilters.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
```

Exact summary:

```text
Test Files  3 passed (3)
Tests  24 passed (24)
```

Type check:

```bash
cd upstream/sub2api/frontend
pnpm typecheck
```

Exact summary: `vue-tsc --noEmit` exited 0.

Production build:

```bash
cd upstream/sub2api/frontend
pnpm build
```

Exact summary: `vue-tsc -b && vite build` exited 0 and transformed 994 modules.

Whitespace check:

```bash
git diff --check -- <the six Task 6 files>
```

Exact summary: exited 0 with no output.

## Acceptance-contract self-review

### 来源与视觉真值

Read the approved desktop/mobile target and interactive prototype. Local browser checks used the controller-owned QA harness without editing it: selected-group screenshots were captured at 1440x1000 and 390x844 in ignored `.playwright-cli/` artifacts. The 1440px result has exactly two cards in one row; the 390px result is one column with no horizontal overflow. Browser console: 0 errors, 0 warnings.

### 当前职责边界

The restored UI contains only service-quality, score/rank, priority, cost, concurrency, probes, real calls, check/cutoff time, and refresh. It adds no revenue, profit, operations, accounting, ledger, reconciliation, history, adjustment, or exception UI/API; real-rendered tests assert this boundary.

### 页面结构

The page has the title/description/refresh-all header, native group tabs, exactly search + one status selector + range control, selected-group seven-field summary, constrained `max-w-[1240px]` content, and the existing deterministic responsive `grid-cols-1 lg:grid-cols-2` card grid. The platform selector and its filtering state/emits were removed.

### 分组汇总

The existing group summary remains exactly the seven native fields (`status`, `platform`, `rate_multiplier`, `rpm_limit`, `account_count`, `active_account_count`, `rate_limited_account_count`); the multiplier display was made screenshot-faithful with `×`.

### 账号卡片完整结构

Normal cards have a green left border and pale-green header; the score/rank/priority band shows `第 N / 可排名数`; five separate green/blue/yellow/purple/neutral tiles remain; timeline-only 24-bar probes, selected-window calls, check/cutoff footer, and per-card refresh were restored. Cost edit/clear actions are below the cost detail to preserve the five-tile visual hierarchy. No settings/history buttons were restored.

### 数据与交互合同

The committed view range is passed explicitly to cards, so `24h`/`7d`/`30d` disclosure labels remain coherent even when an optional child `account.range` is stale or absent. Probe totals derive only from the last 24 `timeline` points; real request/error counts remain call-only. Existing promise-backed priority/procurement saving, failure draft retention, real browser-focus-safe error recovery, stable ordering, successful-snapshot range behavior, and visible-only 5-second batch concurrency polling are retained and tested.

### 防漂移门禁

Tests were RED before implementation and now exercise real mounted card/filter components plus a real-card view integration test. Desktop/mobile browser evidence was checked, focused tests/typecheck/build are green, and an independent re-review reported no unresolved Critical/Important finding. No push/deploy/progress status update was performed.

## Concerns

None. The local QA fixture intentionally supplies an empty `timeline`, so its visual run shows 24 neutral placeholder bars; the real component test supplies 24 timeline points and verifies green/red bars and the `23 成功 · 1 失败` summary.

---

## Task 6 fix round 1 — 2026-08-04

- status: DONE_WITH_CONCERNS
- implementation commit SHA: `a34342740`
- push/deploy/production access: 未执行。

### Root cause

`ListWindow` read window aggregates, probe aggregates, and the latest point, but never called `ListTimelines`; its top-level accounts therefore initialized `timeline` as an empty array. The same top-level projection did not calculate a global score/rank, while group-only rows did. The frontend then rendered the all-site tab with group labels and missing rank semantics. Separately, the card inherited the shared 16px `.card` shell, used localized millisecond grouping, truncated the identity, and formatted the cutoff as a full date/time.

### Changes by finding

1. `ListWindow` now batch-queries `ListTimelines(ctx, ids, 24)` and copies the native points to each top-level account; the group projection inherits those points.
2. Added top-level global quality/rank projection using selected-window evidence, `DefaultAccountMonitorScoreWeights`, account cost evidence, score-desc/account-ID-asc order, and rankable-only denominators. Missing cost evidence gives zero cost contribution without excluding an otherwise normal account.
3. Restored prototype sizing: summary unit `min-height:82px`, card `rounded-lg` (8px), desktop header `16px 18px`, score cells `min-height:121px`, and all five metrics `min-height:116px`.
4. Milliseconds use rounded plain digits plus `ms` and `whitespace-nowrap`.
5. Account identity uses safe word breaking instead of ellipsis truncation.
6. Statistics cutoff uses a short Shanghai `HH:mm` formatter; the ≤430px footer stacks and right-aligns the refresh control.
7. Updated API-shaped service/handler tests and frontend projection fixtures with timeline, global score/rank, concurrency, and timestamp fields; status default is now `全部状态`.

### RED

```bash
cd upstream/sub2api/backend
go test -count=1 -run 'TestAccountMonitor(ListWindowProjectsRecentTimelineAndGlobalRankings|HandlerReturnsCompleteWindowTimelineAndGlobalRanking)' -v ./internal/service ./internal/handler/admin
```

Expected failures observed: service stub recorded no timeline query (`ids [] limit 0`); handler response contained `timeline:[]` and no global score/rank.

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorFilters.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
```

Expected failures observed: grouped `1,018 ms`, inherited `card` shell without `rounded-lg`, default `common.all` rather than `全部状态`, and all-site label `账号分组评分` rather than `账号服务评分` (5 failing tests).

### GREEN / final verification

```bash
cd upstream/sub2api/backend
go test -count=1 ./internal/repository ./internal/service ./internal/handler/admin ./internal/server/routes
```

Passed (repository output reported `ok`; command exited 0 for all four requested packages).

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorFilters.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
```

Passed: 3 files, 26 tests.

```bash
pnpm typecheck
pnpm build
```

Both exited 0; build transformed 994 modules. `git diff --check` exited 0.

### Residual risks / concerns

- No browser screenshot comparison was run in this repair round; deployment and production validation were explicitly out of scope.
- Existing frontend toolchain warnings remain (obsolete Browserslist data, dynamic/static import chunk warnings, and Node DEP0190); none was introduced by this change.

---

## Task 6 fix round 2 — 2026-08-04

- status: READY_FOR_RE-REVIEW (local test repair only)
- test commit SHA: `8d690a6e2`
- production logic / QA harness / project progress / push / deploy: 未修改或执行。

### Root cause

The all-site production projection already sorts equal quality scores by account ID and assigns consecutive ranks. Its regression test supplied only different scores, so it could not exercise the equal-score branch. The view fixture separately represented equal-score accounts 10 and 11 as duplicate rank 1, which is not a production API response after the stable ranking projection.

### Changes

1. Reworked the focused `ListWindow` service case into three equally scored eligible accounts (10, 11, 20) plus one unranked account. It asserts top-level `page.Accounts` ID order, equal source scores, continuous `1, 2, 3` ranks, unranked-last behavior, and the JSON response shape.
2. Replaced the View all-site fixture's duplicate ranks with unique continuous ranks `1, 2, 3, null`; the unranked row now uses a production-possible disabled/non-eligible state.
3. The real-card view assertion now verifies visible ID order and every global rank label. The unrelated concurrency filter expectation was updated from renamed account 20's old fixture label to its new label.

### RED

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorFilters.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
```

Observed after tightening the assertion against the old fixture: `3` files ran; Card and Filters passed, while View failed (`25 passed, 1 failed`) because cards 10 and 11 both rendered `全站排名第 1 / 3` and account 20 rendered `第 2 / 3`, instead of the required continuous `1, 2, 3` sequence.

### GREEN

```bash
cd upstream/sub2api/backend
go test -count=1 -run TestAccountMonitorListWindowProjectsRecentTimelineAndRanksGlobalScoreTiesByAccountID -v ./internal/service
```

Passed: the top-level same-score ordering/rank regression test passed.

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorFilters.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
```

Passed: `3` files, `26` tests. `git diff --check` also exited 0.

### Residual risks / concerns

- This round intentionally did not change production code, run QA harness/browser checks, push, deploy, or validate production.
- The frontend command retained pre-existing pnpm metadata, Node localStorage, and stale Browserslist warnings; no test failures remained.
