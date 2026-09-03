# T128 Monitor Contract Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore Monitor V4 snapshot refresh and Channel Monitor batch reads by aligning SQL result columns with Go scan contracts, with regression tests.

**Architecture:** Keep the existing native repository queries and snapshot worker unchanged. Add only the missing V4 projection columns, correct the independent Channel Monitor batch query contract, and lock both contracts with focused sqlmock tests plus existing service tests.

**Tech Stack:** Go, `database/sql`, `sqlmock`, PostgreSQL SQL, existing Sub2API repository/service tests.

**Spec:** `docs/superpowers/specs/2026-09-03-t128-monitor-metrics-and-failover-correction-design.md`

## Global Constraints

- Do not modify the root `main` worktree, merge, push, deploy, or contact production during this task.
- Work only in `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t128-monitor-contract-fix` on branch `codex/t128-monitor-contract-fix`.
- Do not modify Sub native Monitor V2 semantics, native control-panel aggregation, native accounting, or native whole-site cache aggregation.
- No database migration, historical backfill, snapshot-table rebuild, or production data write.
- Preserve atomic all-window snapshot replacement and existing error behavior.
- Run focused tests before any broader build; retain unrelated baseline failures as evidence.

### Task 1: Monitor V4 SELECT/Scan Contract

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`

**Interfaces:**
- Consumes: Existing `ProjectMonitorV4GroupsForGroups` query and `MonitorV4GroupProjection`.
- Produces: A 19-column result projection whose order is identical in SQL, `rows.Scan`, and test expectations.

- [ ] Step 1: Add a failing repository contract assertion for the final V4 SELECT column list.
- [ ] Step 2: Run the focused repository test and confirm it fails against the current 16-column SELECT.
- [ ] Step 3: Add `cache_read_tokens`, `cache_creation_tokens`, and `cache_hit_denominator` to the final SELECT in the existing order.
- [ ] Step 4: Update the sqlmock row columns/values and assertions so the test exercises all 19 result columns.
- [ ] Step 5: Run `go test -vet=off -count=1 -run 'TestAccountMonitorRepositoryProjectMonitorV4' ./internal/repository`.
- [ ] Step 6: Commit the isolated change:
  `git add upstream/sub2api/backend/internal/repository/account_monitor_repo.go upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go && git commit -m "fix: align monitor v4 select and scan columns"`

### Task 2: Channel Monitor Batch Contract

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/channel_monitor_repo.go`
- Test: `upstream/sub2api/backend/internal/repository/channel_monitor_repo_test.go` (create only if no focused test exists)

**Interfaces:**
- Consumes: Existing `ListLatestForMonitorIDs` query.
- Produces: A batch latest query whose result columns exactly match `monitor_id, model, status, latency_ms, ping_latency_ms, checked_at, quota` and its `rows.Scan` call.

- [ ] Step 1: Add a failing sqlmock test that supplies the query's actual six columns and verifies the seven-value scan contract fails.
- [ ] Step 2: Run the focused Channel Monitor repository test and confirm the failure reproduces `expected 6 destination arguments in Scan, not 7`.
- [ ] Step 3: Make the minimal contract correction by adding the persisted `quota` expression to the batch SELECT if the schema supports it; otherwise remove the quota scan and assignment only after confirming the batch API does not promise quota.
- [ ] Step 4: Add/adjust the regression test for the corrected seven-column result, including a valid JSON quota snapshot when quota is selected.
- [ ] Step 5: Run the focused Channel Monitor repository tests and verify no `Scan 6/7` error.
- [ ] Step 6: Commit the isolated change:
  `git add upstream/sub2api/backend/internal/repository/channel_monitor_repo.go upstream/sub2api/backend/internal/repository/channel_monitor_repo_test.go && git commit -m "fix: align channel monitor batch scan columns"`

### Task 3: Integrated Verification

**Files:**
- Modify: None unless test-only fixture updates are required.
- Test: Existing repository/service tests.

- [ ] Step 1: Run `go test -vet=off -count=1 -run 'TestMonitorV4|TestAccountMonitorRepositoryProjectMonitorV4|TestMonitorV4Snapshots|TestListLatestForMonitorIDs' ./internal/repository ./internal/service`.
- [ ] Step 2: Run `go build ./cmd/server` from the backend module.
- [ ] Step 3: Run `gofmt -w` only on changed Go files and then `git diff --check`.
- [ ] Step 4: Review `git diff --stat`, changed-file scope, and verify no migration/config/native-control-panel files changed.
- [ ] Step 5: Report candidate branch, commits, tests, build result, migration/config status, and that no root `main`/deployment action occurred.

