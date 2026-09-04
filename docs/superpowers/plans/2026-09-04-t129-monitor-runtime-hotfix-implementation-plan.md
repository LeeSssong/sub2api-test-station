# T129 Monitor Runtime Hotfix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore account-monitor queries and Monitor V4 snapshot refreshes by correcting two PostgreSQL projection contracts.

**Architecture:** Keep the existing repository and service boundaries. Correct SQL generation at the repository layer while preserving the service's fail-closed projection validator.

**Tech Stack:** Go 1.27, PostgreSQL, `go-sqlmock`, existing blue-green and independent-test-station release scripts.

**Spec:** `docs/superpowers/specs/2026-09-04-t129-monitor-runtime-hotfix-design.md`

## Global Constraints

- No migration, configuration, dependency, historical backfill, or production business-data write.
- Deploy only from clean root `main` whose commit/tree equal pushed `origin/main`.
- Stop before downtime when preflight reports `downtime_required=true`.
- Main station and independent acceptance station must run the same source commit/tree.

---

### Task 1: Account monitor timeline SQL

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`

**Interfaces:**
- Consumes: `ListRealRequestTimelines(ctx, accountIDs, since, until, bucketCount)`.
- Produces: the unchanged timeline result contract without an invalid `bucket_start` reference.

- [x] Add a regression expectation that rejects the old unaliased `date_bin(...)` projection and requires an explicit projected `bucket_start` expression.
- [x] Run the focused repository test and verify it fails against the current SQL.
- [x] Apply the minimal SQL correction in `real_buckets`.
- [x] Re-run the focused repository test and verify it passes.

### Task 2: Monitor V4 cache denominator

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`

**Interfaces:**
- Consumes: successful selected request events and their cache token fields.
- Produces: `CacheHitDenominator == CacheReadTokens + CacheCreationTokens` and `CacheHitRate == CacheReadTokens / CacheHitDenominator`.

- [x] Tighten the repository query expectation so including `input_tokens` in the cache denominator fails.
- [x] Run the focused repository test and verify it fails against the current SQL.
- [x] Remove `input_tokens` from both cache denominator expressions.
- [x] Re-run repository and service Monitor V4 tests and verify they pass.

### Task 3: Integrate and release

**Files:**
- Modify: `docs/project/project-progress.md`
- Modify: `docs/project/native-sub-task-package-queue.md`
- Create: a 0600 release evidence JSON outside the repository.

**Interfaces:**
- Consumes: reviewed candidate commit and existing release scripts.
- Produces: pushed root `main`, same-version main and acceptance deployments, health and target-interface evidence.

- [ ] Run focused repository/service tests, build, formatting, and diff checks.
- [ ] Commit the candidate, merge it into root `main`, repeat the direct checks, and push `origin/main`.
- [ ] Run production preflight; proceed only when `downtime_required=false`.
- [ ] Deploy main, verify health and authenticated monitor endpoints, and confirm the two log errors stop recurring.
- [ ] Deploy the same commit/tree to the independent acceptance station and verify health, source identity, and authenticated monitor endpoints.
- [ ] Record final evidence and mark the task `DONE` only after both deployments and online verification succeed.
