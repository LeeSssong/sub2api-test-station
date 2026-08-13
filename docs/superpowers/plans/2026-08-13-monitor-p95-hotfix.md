# Monitor P95 Hotfix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Make channel monitor P50/P95 values represent exact percentiles across the selected window.

**Architecture:** Reuse the existing raw Ops dashboard query by changing only the query mode selected by `MonitorV2Service`. Keep response contracts, frontend formatting, and general Ops aggregation unchanged.

**Tech Stack:** Go, testify, existing Sub2API service/repository interfaces.

## Global Constraints

- T03-R1 remains active and untouched.
- Only channel monitor card metric reads change.
- Do not modify shared Ops pre-aggregation behavior.
- No migration, backfill, configuration change, or GitHub Actions.
- Run only focused tests and release checks required for this change.

---

### Task 1: Select exact raw percentiles for monitor cards

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2_test.go`

**Interfaces:**
- Consumes: `OpsDashboardFilter.QueryMode` and `OpsQueryModeRaw`.
- Produces: monitor card overview requests that calculate percentiles over the entire selected raw window.

- [ ] **Step 1: Write the failing test**

Add or extend a `MonitorV2Service` test whose fake Ops reader records the received filter, execute a 7-day snapshot, and assert the group overview request uses `OpsQueryModeRaw`. The test must fail against the current `OpsQueryModeAuto` implementation.

- [ ] **Step 2: Verify RED**

Run the single new test in `./internal/service` and confirm it fails because the received mode is `auto`, not `raw`.

- [ ] **Step 3: Implement the minimal fix**

Change the monitor group filter's `QueryMode` from `OpsQueryModeAuto` to `OpsQueryModeRaw`. Do not change repository aggregation code or any other consumer.

- [ ] **Step 4: Verify GREEN**

Run the new test, then the focused monitor-v2 service tests, and `git diff --check`.

- [ ] **Step 5: Commit**

Commit only the specification, plan, ledger entry, service change, and focused test.
