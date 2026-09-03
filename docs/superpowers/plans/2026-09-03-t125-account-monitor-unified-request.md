# T125 Account Monitor Unified Request Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make account monitoring treat one latest active-probe terminal as an ordinary request only when its 5-minute bucket has no real logical request, while keeping the UI source-agnostic and restoring the native edit entry.

**Architecture:** Keep `usage_logs`, `ops_error_logs`, and `account_monitor_results` as the existing raw facts. Add one repository-level unified-request projection that applies per-account/per-5-minute source selection, then feed its aggregates and timelines into the existing Account Monitor service and scheduler projection. Keep accounting fields sourced only from business usage logs, and remove public source-split fields from the monitor response without changing the endpoint or card layout.

**Tech Stack:** Go, PostgreSQL SQL, `sqlmock`, Vue 3, TypeScript, Vitest, pnpm.

**Spec:** `docs/superpowers/specs/2026-09-03-t125-account-monitor-unified-request-design.md`

## Global Constraints

- Implement only in the T125 worktree; do not modify the root `main`, global queue, project ledger, other worktrees, production data, or deployment state.
- The public monitor contract does not distinguish probe and real request sources; no source count, source label, or source-specific color is exposed.
- A real logical request wins its 5-minute bucket; otherwise one latest completed probe terminal is one ordinary request; an empty bucket is excluded from denominators.
- TTFT P95 uses raw selected successful TTFT samples, never a percentile of percentiles.
- Scheduler rank comes from the native scheduler projection; the monitor page does not recompute rank.
- Revenue, cost, profit, and token accounting remain native `usage_logs` facts; probes do not create accounting rows.
- No migration, configuration change, historical backfill, GitHub Actions workflow, merge, push, deployment, or production verification is part of this plan.
- Run direct tests, required build/type checks, and `git diff --check`; do not broaden verification to unrelated failures.

---

## File Map

- Modify `upstream/sub2api/backend/internal/service/account_monitor_types.go`: define the source-agnostic aggregate/timeline contracts while preserving only necessary internal compatibility aliases.
- Modify `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`: implement the unified request SQL for account aggregates, group aggregates, lifetime counts, and timelines.
- Modify `upstream/sub2api/backend/internal/service/account_monitor_service.go`: consume unified aggregates for cards, current state, group evidence, quality, and scheduler input; preserve accounting projection boundaries.
- Modify `upstream/sub2api/backend/internal/service/account_monitor_quality_fusion.go`: remove additive real/probe semantics from the monitor-quality path and accept the already-selected unified aggregate.
- Inspect `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go`; modify it only when the handler explicitly constructs source-split fields after the service/type changes. If it only serializes service rows, leave it unchanged and record that result in the handoff.
- Modify `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`: remove source-split timeline fields from the public type and rename only local semantic aliases where needed.
- Modify `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`: render one request metric/timeline and remove source-specific labels/logic without changing layout.
- Modify `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`: lock source-agnostic display and edit entry behavior.
- Inspect `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`; add the view-level modal assertion when the existing card event test does not cover `showEditAccountDialog` becoming true, otherwise leave the view test unchanged and record the existing coverage.
- Modify `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`: cover SQL projection and scan contracts.
- Modify `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`: cover unified state/quality/ranking behavior.
- Inspect `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go` and add the response-level assertion there because the public JSON contract must be locked at the handler boundary.

## Interfaces

The repository's existing typed methods remain the integration boundary to minimize wiring changes:

```go
type AccountMonitorWindowAggregate struct {
    RequestCount int64
    SuccessCount int64
    ErrorCount int64
    BaseCost float64
    Revenue float64
    AccountCost float64
    CostComplete bool
    SuccessRate float64
    TTFTSampleCount int
    LatencySampleCount int
    TTFTP50MS *float64
    TTFTP95MS *float64
    LatencyP95MS *float64
    OutputRateTokensPerSecond *float64
    OutputRateSampleCount int
    LastObservedAt *time.Time
}

type AccountMonitorRealRequestTimelinePoint struct {
    StartAt time.Time
    EndAt time.Time
    RequestCount int64
    SuccessCount int64
    FailureCount int64
    TTFTP95MS *float64
}
```

The methods may retain their Go names temporarily for compatibility, but their returned values must obey the unified contract:

```go
ListRealRequestAggregates(ctx context.Context, accountIDs []int64, since, until time.Time) (map[int64]AccountMonitorWindowAggregate, error)
ListGroupRealRequestAggregates(ctx context.Context, groupIDs, accountIDs []int64, since, until time.Time) (map[int64]map[int64]AccountMonitorWindowAggregate, error)
ListLifetimeRealRequestCounts(ctx context.Context, accountIDs []int64) (map[int64]int64, error)
ListRealRequestTimelines(ctx context.Context, accountIDs []int64, since, until time.Time, bucketCount int) (map[int64][]AccountMonitorRealRequestTimelinePoint, error)
```

## Task 1: Add failing repository tests for unified bucket selection

**Files:**
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- Reference: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`

**Interfaces:** Consumes the existing repository interfaces above. Produces exact SQL/scan expectations for later implementation.

- [ ] **Step 1: Write failing sqlmock tests for account aggregates.** Add cases that expect the query to select all real logical requests when a bucket contains real traffic, select exactly one latest probe when the bucket has no real traffic, and leave an empty bucket out of the aggregate denominator. Assert returned `RequestCount`, `SuccessCount`, `ErrorCount`, `TTFTSampleCount`, `TTFTP95MS`, and `LastObservedAt`.
- [ ] **Step 2: Write failing sqlmock tests for group aggregates.** Cover a group/account scope with no real row but one probe row, and assert the group result is present instead of being marked stale or missing by the service.
- [ ] **Step 3: Write failing sqlmock tests for timelines and lifetime counts.** Change the expected timeline columns to only unified request fields and assert a probe-only bucket returns `RequestCount=1`, `SuccessCount=1`, `FailureCount=0`; assert a real bucket ignores its probe and lifetime count includes selected probe fallback events.
- [ ] **Step 4: Run the focused repository tests and confirm failure.**

Run from `upstream/sub2api`:

```bash
go test ./backend/internal/repository -run 'TestAccountMonitorRepository(RealRequest|Group|Lifetime).*' -count=1
```

Expected: FAIL because current SQL returns real-only aggregates and source-split timeline columns.

- [ ] **Step 5: Commit the failing tests.**

```bash
git add upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go
git commit -m "test: define unified account monitor request buckets"
```

## Task 2: Implement one repository unified-request projection

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`

**Interfaces:** Consumes raw usage/error/probe tables. Produces the existing aggregate/timeline method results under the unified contract for Tasks 3 and 4.

- [ ] **Step 1: Add a shared SQL selection shape.** Build CTEs with explicit columns for real candidates and probe candidates. Deduplicate real candidates by the existing canonical logical request key. Bucket by the same 5-minute origin used by the timeline.
- [ ] **Step 2: Apply source precedence.** For each account/source bucket, retain every deduplicated real request if at least one exists; otherwise retain only the latest probe terminal. Do not sum or return probe-source counters.
- [ ] **Step 3: Aggregate metrics.** Calculate request/error/success counts, success rate, raw successful TTFT P95, latency P95, and latest selected observation. Keep revenue/account cost/cost completeness populated only from real usage rows; probe rows contribute zero to accounting fields and never make cost complete.
- [ ] **Step 4: Apply the same selector to group aggregates and lifetime counts.** Preserve immutable `usage_logs.group_id` attribution for real business rows. For probe fallback, use the account's monitor scope/group association already supplied by the service; do not infer a business accounting group from a probe.
- [ ] **Step 5: Apply the same selector to timeline buckets.** Return only request count, success count, failure count, TTFT P95, and fixed time boundaries. Keep empty buckets as zero/null points; remove source counters from the response scan and type.
- [ ] **Step 6: Run focused repository tests and confirm pass.**

```bash
go test ./backend/internal/repository -run 'TestAccountMonitorRepository(RealRequest|Group|Lifetime).*' -count=1
```

Expected: PASS for all new and updated unified-selection cases.

- [ ] **Step 7: Commit repository implementation.**

```bash
git add upstream/sub2api/backend/internal/repository/account_monitor_repo.go upstream/sub2api/backend/internal/service/account_monitor_types.go upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go
git commit -m "feat: unify account monitor request observations"
```

## Task 3: Make service state, quality, and scheduler input use the unified aggregate

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_quality_fusion.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:** Consumes unified repository aggregates/timelines. Produces card rows, group evidence, current availability, and quality facts that share one request contract; scheduler rank remains the native projection result.

- [ ] **Step 1: Add failing service tests.** Cover: probe-only window produces one ordinary request; real-plus-probe bucket does not add the probe; empty buckets do not lower the success denominator; recent real success clears stale/pending state; group quality uses probe fallback when no real group row exists; scheduler rank remains the projection's rank.
- [ ] **Step 2: Run focused service tests to verify failure.**

```bash
go test ./backend/internal/service -run 'TestAccountMonitor(ListWindow|Window|Probe|Projection|Group).*' -count=1
```

Expected: FAIL against current probe-gated/additive behavior.

- [ ] **Step 3: Replace split projection calls with one unified evidence path.** Use the repository aggregate/timeline result as the source for `request_count`, `error_count`, `success_rate`, TTFT, timeline, and quality. Do not call `fuseAccountMonitorQualityEvidence` to add independent real and probe samples after repository selection.
- [ ] **Step 4: Correct current-state freshness.** Determine freshness from the latest selected request and apply the existing failure/disabled/paused rules. A fresh real request or probe fallback can be normal; stale/missing selected evidence remains “待确认”. Preserve management-state and fatal-failure semantics.
- [ ] **Step 5: Correct group projection.** Remove the `hasGroupWindow` branch that discards probe evidence when there is no real group row. Generate group evidence from the unified group aggregate while keeping profitability fields sourced only from real usage aggregates.
- [ ] **Step 6: Preserve native scheduler rank.** Continue `schedulerProjection.Project` and copy its `Rank`, `RankTotal`, `Eligible`, `SnapshotAt`, and reason fields without recomputing rank from card metrics.
- [ ] **Step 7: Run focused service tests and confirm pass.**

```bash
go test ./backend/internal/service -run 'TestAccountMonitor(ListWindow|Window|Probe|Projection|Group).*' -count=1
```

Expected: PASS for unified state/quality/rank assertions; existing tests whose names assert the obsolete split contract must be updated to the approved contract.

- [ ] **Step 8: Commit service implementation.**

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/service/account_monitor_quality_fusion.go upstream/sub2api/backend/internal/service/account_monitor_service_test.go
git commit -m "feat: project unified account monitor quality"
```

## Task 4: Lock the handler contract and accounting boundary

**Files:**
- Inspect `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go`; modify it only when the handler explicitly constructs source-split fields after the service/type changes. If it only serializes service rows, leave it unchanged and record that result in the handoff.
- Test: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`
- Reference: `upstream/sub2api/backend/internal/service/account_monitor_types.go`

**Interfaces:** Consumes service rows from Task 3. Produces the existing monitor endpoint response with one request semantic and no source-split public fields.

- [ ] **Step 1: Add failing handler contract assertions.** Marshal a monitor response containing a probe-only bucket and assert the JSON has ordinary request metrics, no `probe_count`, `probe_success_count`, `probe_failure_count`, or `source`, and unchanged scheduler rank fields. Assert profitability/accounting remains zero/unchanged for probe-only evidence.
- [ ] **Step 2: Run the focused handler test.**

```bash
go test ./backend/internal/handler/admin -run 'TestAccountMonitorHandler.*' -count=1
```

Expected: FAIL while source-split timeline fields remain serialized or accounting is coupled to monitor samples.

- [ ] **Step 3: Implement the narrow response shaping change.** Remove only source-split JSON fields and keep endpoint, authentication, no-store headers, scheduler explanation, profitability status, and compatibility fields otherwise unchanged.
- [ ] **Step 4: Run the focused handler tests.**

```bash
go test ./backend/internal/handler/admin -run 'TestAccountMonitorHandler.*' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the contract change.**

```bash
git add upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go upstream/sub2api/backend/internal/service/account_monitor_types.go
git commit -m "fix: hide account monitor request source split"
```

## Task 5: Update the existing card without changing layout and restore edit entry

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Test: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Modify/Test: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts` only if card-level event coverage cannot prove modal opening.

**Interfaces:** Consumes the unified monitor API from Task 4. Produces the same card layout with source-agnostic labels/colors and a visible `accountEdit` button that opens the existing modal through `AccountMonitorView`.

- [ ] **Step 1: Update failing component tests.** Replace “窗口真实请求” assertions with “窗口请求” or the existing neutral Chinese request label. Add a probe-only timeline case that renders as one ordinary request, has no source text/legend, and uses normal status color. Assert `account-edit` is visible and emits `accountEdit`.
- [ ] **Step 2: Run the focused Vitest file and confirm failure.**

```bash
pnpm exec vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts --pool=forks --poolOptions.forks.singleFork=true
```

Expected: FAIL because current tests and component expose real/probe wording and CSS hides the edit button.

- [ ] **Step 3: Simplify frontend types and computed values.** Remove public probe/source fields from `AccountMonitorRealRequestTimelinePoint`; make card request count and timeline consume unified fields. Keep `refresh-account` action wording functional, but do not claim it creates no request sample in the monitor display contract.
- [ ] **Step 4: Simplify chart classification.** Use only request count, success/failure, and TTFT threshold. A probe fallback is rendered exactly like any other selected request; no source branch or source-specific color remains.
- [ ] **Step 5: Restore the edit button.** Delete the narrow legacy CSS selector that hides `[data-test="account-edit"]`; leave the existing event, modal loading, modal component, and other account actions unchanged.
- [ ] **Step 6: Run focused frontend tests.**

```bash
pnpm exec vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts --pool=forks --poolOptions.forks.singleFork=true
```

Expected: PASS, including visible edit entry and native modal path.

- [ ] **Step 7: Commit frontend changes.**

```bash
git add upstream/sub2api/frontend/src/api/admin/accountMonitor.ts upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts
git commit -m "fix: show unified account monitor requests and edit action"
```

## Task 6: Integrated direct verification

**Files:**
- No new implementation files.
- Verify all changed files and commits from Tasks 1-5.

**Interfaces:** Confirms the unified request contract survives the full service, handler, and frontend path.

- [ ] **Step 1: Run all directly related Go tests.**

```bash
cd upstream/sub2api
go test ./backend/internal/repository ./backend/internal/service ./backend/internal/handler/admin ./backend/internal/server/routes -run 'AccountMonitor|OpenAIUnifiedQuality' -count=1
```

Expected: PASS for the T125-related tests. Record unrelated baseline failures without expanding scope.

- [ ] **Step 2: Run frontend account-monitor tests.**

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/components/admin/account-monitor src/views/admin/__tests__/AccountMonitorView.spec.ts --pool=forks --poolOptions.forks.singleFork=true
```

Expected: PASS.

- [ ] **Step 3: Run required type/build checks.**

```bash
cd upstream/sub2api/frontend
pnpm typecheck
pnpm build
cd ..
go build ./cmd/server
```

Expected: PASS; do not run deployment or production commands.

- [ ] **Step 4: Check the diff and source split removal.**

```bash
git diff --check
rg -n "probe_count|probe_success_count|probe_failure_count|source.*(real|probe)|窗口真实请求|不生成真实请求样本" upstream/sub2api/frontend/src/api/admin/accountMonitor.ts upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue upstream/sub2api/backend/internal/service/account_monitor_types.go
```

Expected: no public source-split fields or source-specific display wording in the changed monitor contract; internal raw-table source handling may remain in repository SQL.

- [ ] **Step 5: Review accounting and deployment boundaries.** Confirm no migration/configuration changes, no writes outside tests, no `.github/workflows` changes, and no release command was run.

- [ ] **Step 6: Record verification in the T125 handoff without a separate notes commit.** Do not create a release or mark the global task complete from this worktree; report the candidate as `READY_FOR_ROOT_REVIEW` only after all direct checks pass.

## Plan Self-Review

- Spec coverage: unified selection, raw P95, empty denominators, current-state freshness, group quality, native scheduler rank, accounting isolation, source-agnostic API/UI, edit regression, tests, and release gates are all assigned above.
- Placeholder scan: no `TBD`, `TODO`, or vague “handle later” steps; every step names files, behavior, command, and expected result.
- Type consistency: the existing aggregate/timeline method signatures remain stable while returned semantics change; handler and frontend consume the same source-agnostic fields.
- Scope check: this is one vertical Account Monitor task with one independent UI regression in the same screen, not unrelated subsystems.
