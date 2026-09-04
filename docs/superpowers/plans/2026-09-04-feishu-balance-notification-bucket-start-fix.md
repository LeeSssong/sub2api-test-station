# Feishu Balance Notification Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the native account-monitor projection used by Feishu balance alerts by fixing the PostgreSQL `bucket_start` grouping expression, without changing cache metrics or notification semantics.

**Architecture:** Keep `UpstreamBalanceNotificationService` and its existing event/lease/sender flow unchanged. Fix only the `real_buckets` CTE in `account_monitor_repo.go` so the selected `date_bin` expression is explicitly projected and grouped; prove the contract with repository SQL tests and service notification tests.

**Tech Stack:** Go 1.27, PostgreSQL SQL, `sqlmock`, existing Sub2API service/repository tests.

**Spec:** `docs/superpowers/specs/2026-09-04-feishu-balance-notification-bucket-start-fix-design.md`

## Global Constraints

- Do not modify `input_tokens`, cache hit rate, cache denominator, or any other Monitor V4 metric.
- Do not modify balance thresholds, BaseURL aggregation, repeat intervals, credentials, event schema, or sender behavior.
- No migration, configuration change, production data write, real upstream probe, or real Feishu send.
- Root `main` is protected; implementation occurs only in this candidate worktree.
- Candidate must be rebased/merged only through the root release controller after direct tests pass.

### Task 1: Add the failing SQL regression contract

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- Test target: `TestAccountMonitorRepositoryRealRequestTimelineUsesUnifiedRequestFields`

**Interfaces:**
- Consumes: `AccountMonitorRealRequestTimelineRepository.ListRealRequestTimelines(ctx, accountIDs, since, until, bucketCount)`.
- Produces: A test assertion requiring `real_buckets` to contain `date_bin(...) AS bucket_start` and group by the same expression.

- [ ] **Step 1: Strengthen the existing test matcher before changing production SQL.**

  Replace the permissive timeline query matcher with an assertion that the normalized SQL contains exactly:

  ```text
  SELECT account_id, date_bin('5 minutes'::interval, created_at, $2::timestamptz) AS bucket_start FROM real_candidates WHERE rn = 1 GROUP BY account_id, date_bin('5 minutes'::interval, created_at, $2::timestamptz)
  ```

- [ ] **Step 2: Run the focused test and verify it fails against the production baseline.**

  Run from `upstream/sub2api/backend`:

  ```bash
  go test ./internal/repository -run 'TestAccountMonitorRepositoryRealRequestTimelineUsesUnifiedRequestFields' -count=1
  ```

  Expected: FAIL because the current SQL groups by the unavailable same-level alias `bucket_start` and does not explicitly project/group the expression.

### Task 2: Implement the minimal production SQL fix

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go:1112`

**Interfaces:**
- Consumes: Existing timeline query parameters and CTE column contract.
- Produces: `real_buckets(account_id, bucket_start)` with PostgreSQL-valid expression projection and grouping.

- [ ] **Step 1: Change only the `real_buckets` CTE.**

  Use this exact SQL shape:

  ```sql
  real_buckets (account_id, bucket_start) AS (
      SELECT account_id,
             date_bin('5 minutes'::interval, created_at, $2::timestamptz) AS bucket_start
      FROM real_candidates
      WHERE rn = 1
      GROUP BY account_id,
               date_bin('5 minutes'::interval, created_at, $2::timestamptz)
  )
  ```

  Do not alter neighboring CTEs, selected columns, cache fields, or timeline semantics.

- [ ] **Step 2: Run the focused repository test and verify it passes.**

  ```bash
  go test ./internal/repository -run 'TestAccountMonitorRepositoryRealRequestTimelineUsesUnifiedRequestFields' -count=1
  ```

  Expected: PASS.

### Task 3: Verify Feishu notification path and scope

**Files:**
- Test: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service_test.go`
- Inspect only: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service.go`

**Interfaces:**
- Consumes: Existing reader, event repository, registry, and sender fakes.
- Produces: Evidence that a readable projection permits claim/delivery and that projection errors remain fail-closed.

- [ ] **Step 1: Run existing direct notification tests without changing service code.**

  ```bash
  go test ./internal/service -run 'TestUpstreamBalanceNotification|Test.*Balance.*Notification' -count=1
  ```

  Expected: PASS; no service implementation change is needed because the existing service already sends after a successful projection read.

- [ ] **Step 2: Confirm the existing service suite covers delivery and fail-closed behavior.**

  Inspect the matching tests for a readable `UpstreamBalanceEvaluation`, active low/zero event claim, sender invocation, delivery confirmation, and reader-error no-send behavior. Do not add a new service test or production interface because this SQL-only fix does not change that contract.

- [ ] **Step 3: Run the focused service tests again.**

  ```bash
  go test ./internal/service -run 'TestUpstreamBalanceNotification|Test.*Balance.*Notification' -count=1
  ```

  Expected: PASS.

### Task 4: Candidate verification and commit

**Files:**
- Inspect: all candidate diffs
- Modify: none beyond Tasks 1-3

- [ ] **Step 1: Run the direct repository and service tests.**

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/repository -run 'TestAccountMonitorRepository' -count=1
  go test ./internal/service -run 'TestUpstreamBalanceNotification|Test.*Balance.*Notification' -count=1
  go build ./cmd/server
  cd ../..
  git diff --check
  ```

- [ ] **Step 2: Prove the diff is Feishu-only.**

  Confirm `git diff --name-only main...HEAD` contains only the spec/plan history plus `account_monitor_repo.go` and its direct tests; reject any diff containing `input_tokens`, `cache_hit_rate`, `cache_hit_denominator`, migrations, config, secrets, or deployment scripts.

- [ ] **Step 3: Commit the candidate.**

  ```bash
  git add upstream/sub2api/backend/internal/repository/account_monitor_repo.go \
          upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go \
          upstream/sub2api/backend/internal/service/upstream_balance_notification_service_test.go
  git commit -m "fix: restore Feishu balance notification projection"
  ```

- [ ] **Step 4: Report candidate handoff.**

  Record baseline `main@9e4c70884`, candidate commit, changed files, direct test results, no migration/config/data changes, and the required root release authorization. Do not merge, push, deploy, or send real Feishu traffic from this worktree.
