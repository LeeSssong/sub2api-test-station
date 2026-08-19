# T32 Account Score Regression Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore native probe-driven scores and ranks for paused or schedulable-closed accounts while stopping only after a closed account's latest native probe returns HTTP 4xx/5xx.

**Architecture:** Keep `AccountMonitorService` and `AccountMonitorRepository` as the only facts and projection boundary. Add one pure stop predicate over `Account` + `AccountMonitorLatest`, use it before physical probes, and remove management-paused status from the score eligibility gate while preserving existing formula, evidence freshness, cost, and ordering.

**Tech Stack:** Go, `testing`, existing account monitor service/repository stubs, PostgreSQL-backed native probe result schema (unchanged).

**Spec:** `docs/superpowers/specs/2026-08-19-t32-account-score-regression-design.md`

## Global Constraints

- Scores, current status, and ranks use only Sub native active-probe results.
- Paused accounts with fresh probe evidence remain scoreable and rankable.
- Only closed scheduling plus latest probe HTTP status 400..599 stops probing and removes ranking.
- No score formula, weights, billing, scheduler policy, database fact source, migration, or unrelated page changes.
- Do not synthesize defaults for missing, stale, or failed evidence.

### Task 1: Add RED regression coverage

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_probe_test.go` (only if pure predicate coverage is placed with probe tests)

**Interfaces:**
- Consume existing `AccountMonitorService.List`, `ListWindow`, `RunAll`, `RunOne`, repository stubs, and probe hook.
- Produce failing assertions for `accountMonitorProbeShouldStop`, paused projection, and closed-success continuation.

- [ ] **Step 1: Write the failing stop-gate test**

Add table-driven cases asserting the pure predicate is true only for `Schedulable=false` (or non-active scheduling closure) plus HTTP 400, 401, 500, or 599; assert false for schedulable-open 500, closed timeout/no status, closed success, and closed non-HTTP error.

- [ ] **Step 2: Write the failing paused scoring/ranking test**

Create two accounts in the same group, one `StatusActive/Schedulable=false` and one schedulable, each with fresh successful probe aggregates and latest success. Call `ListWindow`; assert the paused row has `AvailabilityStatus=normal`, `QualityScore != nil`, `Eligible=true`, and a deterministic `GroupRank`, without changing existing score values.

- [ ] **Step 3: Write the failing paused 4xx/5xx and no-evidence tests**

For a paused account with latest 401/500, assert no rank and `AvailabilityStatus=unavailable` or `abnormal` according to the existing native error classification. For a paused account with no aggregate/latest, assert pending/stale with nil score/rank.

- [ ] **Step 4: Write the failing closed-success continuation test**

Use a closed account with latest successful probe, install a probe hook that records calls, run `RunAll` twice, and assert both runs physically probe and persist the result. Add a closed-4xx case with the same hook and assert the run skips the physical probe and does not insert a new result.

- [ ] **Step 5: Run RED tests**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitor(T32|ProbeShouldStop)' -count=1
```

Expected: FAIL because paused rows are currently disabled/unranked and `listPool` excludes closed accounts.

### Task 2: Implement the minimal native projection and runner fix

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go:940-1038, 1450-1562, 1859-1906`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go` only to update assertions that directly contradict the approved T32 contract

**Interfaces:**
- Add `accountMonitorProbeShouldStop(account Account, latest AccountMonitorLatest) bool` as a pure internal helper.
- Keep `listPool(ctx)` returning the native account pool; apply the stop predicate using one `ListLatest` read before dispatch.

- [ ] **Step 1: Add the stop predicate**

Implement HTTP range checking with `status >= 400 && status <= 599`, require scheduling closed semantics, and require latest status to be a failed probe with an HTTP status. Do not treat timeout, error-code text, or missing status as the stop trigger.

- [ ] **Step 2: Apply the predicate before `RunAll` probes**

Load latest results for the pool once, skip only rows satisfying the predicate, and leave all other accounts (including schedulable-closed) in the probe fan-out. Preserve completed-count and existing persistence/error handling.

- [ ] **Step 3: Apply the same predicate to `RunOne`**

Resolve the target's latest result before probing; return the existing service error shape with a precise stopped reason when the stop gate matches. A skipped run must not write a synthetic result.

- [ ] **Step 4: Remove paused-as-disabled score gating**

Make `accountMonitorAvailabilityStatus` evaluate evidence and latest probe first. A paused account with fresh latest success is normal and score-eligible; a paused account with fresh latest non-HTTP failure remains abnormal/capped according to existing formula; only the stop predicate produces unavailable for a closed account. Preserve stale/no-evidence behavior.

- [ ] **Step 5: Preserve group score eligibility for paused rows**

Do not discard paused rows' probe aggregates before `accountMonitorEvidence`; keep health summaries' existing paused accounting, but let score/rank use the same probe evidence and cost eligibility as other rows. Remove only the management-state boolean from the ranking gate, not group cost checks or sort/tie-break rules.

- [ ] **Step 6: Run focused GREEN tests**

Run:

```bash
cd upstream/sub2api/backend
gofmt -w internal/service/account_monitor_service.go internal/service/account_monitor_service_test.go internal/service/account_monitor_probe_test.go
go test ./internal/service -run 'TestAccountMonitor(T32|ProbeShouldStop|Window|Classifier)' -count=1
```

Expected: PASS, with no default score for missing/stale evidence.

### Task 3: Direct regression verification and handoff

**Files:**
- Create: `docs/superpowers/reports/2026-08-19-t32-account-score-regression-implementation.md`
- Create: `docs/handoffs/2026-08-19-t32-account-score-regression-handoff.md`

**Interfaces:**
- Consume candidate source and focused test evidence.
- Produce READY_FOR_ROOT_REVIEW handoff; no edits to global queue, project progress, main, release evidence, or production state.

- [ ] **Step 1: Run direct backend tests and static checks**

Run `go test ./internal/service ./internal/repository ./internal/handler`, `go vet ./internal/service ./internal/repository ./internal/handler`, and `git diff --check`. Record exact commands and results.

- [ ] **Step 2: Review the diff against the spec**

Confirm only native service/tests plus task-local docs changed; verify no score formula, weight, cost, scheduler, schema, migration, config, or frontend page changes.

- [ ] **Step 3: Record handoff metadata**

Record baseline SHA `dc51b37c9dbf73a87cccceab5815f129882812c5`, candidate commit SHA, changed files, tests, unverified production items, migration/config (`none`), `downtime_required` (`false` pending root precheck), rollback SHA, and remaining risks. Set task state to `READY_FOR_ROOT_REVIEW` only.
