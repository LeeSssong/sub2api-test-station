# T49 失败尝试从正常流水列表隔离 Implementation Plan

> **For agentic workers:** Execute the bounded plan in this worktree and leave the candidate at `READY_FOR_ROOT_REVIEW`; root controls merge, push, deploy, and production verification.

**Goal:** Prevent retry-failure `unknown` usage rows from appearing in normal usage lists and their matching statistics.

**Architecture:** Reuse the existing native usage repository. Add one shared SQL predicate to the normal list and filtered-stat query builders; preserve the existing audit storage and billing path.

**Tech Stack:** Go, PostgreSQL SQL builders, `go-sqlmock`, Gin handler contract tests.

**Spec:** `docs/superpowers/specs/2026-08-22-t49-hide-failed-usage-attempts-design.md`

## Tasks

### Task 1: Lock the regression

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_request_type_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/usage_handler_request_type_test.go`

- [x] Add a repository test proving the normal list query includes the `unknown` exclusion.
- [x] Reuse the existing handler contract suite; no new public filter field is needed for this repository-internal default.
- [x] Run the focused tests and observe the repository test fail before production changes.

### Task 2: Implement the shared native filter

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_query.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go`

- [x] Define the `COALESCE(usage_completeness, 'complete') <> 'unknown'` predicate beside the existing usage predicates.
- [x] Append it to `ListWithFilters` and `GetStatsWithFilters` while retaining all caller filters and argument ordering; endpoint breakdowns use the same predicate.
- [x] Keep `ListBy*`, time-range audit reads, writes, billing, and cleanup behavior unchanged.

### Task 3: Verify and hand off

- [x] Run focused repository and admin handler tests.
- [x] Run `gofmt` and `git diff --check`.
- [x] Run the affected backend compile/build command.
- [x] Record candidate SHA, tests, no-migration status, `downtime_required` assumption, rollback, and residual risk in a handoff.
