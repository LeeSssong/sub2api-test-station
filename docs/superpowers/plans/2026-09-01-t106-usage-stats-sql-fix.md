# T106 用户用量汇总 SQL 修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 `/api/v1/usage/stats` 的 malformed SQL，并锁定同一 scoped 数据集的汇总与 endpoint 聚合契约。

**Architecture:** 保留 `GetStatsWithFilters` 现有过滤构建和 Go 结果映射，只恢复缺失的 scoped `GROUPING SETS` SQL 主体。单元测试在 SQL executor 边界验证查询结构、参数和返回映射。

**Tech Stack:** Go, PostgreSQL SQL, go-sqlmock, testify

**Spec:** `docs/superpowers/specs/2026-09-01-t106-usage-stats-sql-fix-design.md`

## Global Constraints

- 只修改 usage stats repository SQL、直接测试和本任务文档。
- 不修改 `docs/project/project-progress.md` 或 `docs/project/native-sub-task-package-queue.md`。
- 不推送、不部署、不触碰生产数据或凭据，不使用 GitHub Actions。

---

### Task 1: Reproduce malformed stats SQL

**Files:**
- Create: `upstream/sub2api/backend/internal/repository/usage_log_repo_stats_query_test.go`

**Interfaces:**
- Consumes: `usageLogRepository.GetStatsWithFilters(ctx, UsageLogFilters)`
- Produces: repository regression coverage for one scoped aggregate query

- [x] **Step 1: Write the failing test**

Add a sqlmock matcher that rejects `%!`, more than one `FROM usage_logs`, missing `FROM scoped`, missing `GROUPING SETS`, or missing the normal-usage filter. Return representative total and endpoint rows and assert the existing response mapping.

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/repository -run TestUsageLogRepositoryGetStatsWithFiltersUsesSingleScopedAggregate -count=1`

Expected: FAIL because the current SQL contains `%!s(MISSING)` and does not aggregate from `scoped` with grouping sets.

### Task 2: Restore the scoped aggregate SQL

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go`

**Interfaces:**
- Consumes: existing filters, `buildWhere`, `usageLogNormalQueryFilter`
- Produces: the existing 13-column row contract consumed by `GetStatsWithFilters`

- [x] **Step 1: Implement the minimal SQL repair**

Select `GROUPING(inbound_endpoint)`, `GROUPING(upstream_endpoint)`, nullable endpoint dimensions and the existing aggregates from `scoped`; group by total, each endpoint dimension and endpoint path. Sum the CTE's existing `account_cost` projection.

- [x] **Step 2: Format and run focused tests**

Run: `gofmt -w internal/repository/usage_log_repo_stats_query_test.go internal/repository/usage_log_repo_stats.go`

Run: `go test ./internal/repository -run 'TestUsageLogRepositoryGetStatsWithFiltersUsesSingleScopedAggregate|TestUsageLog_GetStatsWithFilters_AggregatesAndEndpoints' -count=1`

Expected: PASS, with integration-tagged tests reported separately if their database is unavailable.

### Task 3: Verify and hand off

**Files:**
- Create: `docs/superpowers/reports/2026-09-01-t106-usage-stats-sql-fix-handoff.md`

- [x] **Step 1: Run direct verification**

Run focused repository tests, `gofmt`, and `git diff --check`; inspect the final diff for scope.

- [x] **Step 2: Record evidence and commit**

Document RED/GREEN evidence, unverified production behavior and root-controller gates. Commit only T106 files on `codex/t106-usage-stats-sql-fix`.
