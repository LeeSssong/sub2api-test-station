# Monitor V2 Long-Window Buckets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent Monitor V2 7-day and 30-day requests from falling back to
minute buckets and exceeding the 64-point contract limit.

**Architecture:** Extend the shared ops trend repository bucket contract with
6-hour and 24-hour intervals. Both throughput and error trends use one
normalization helper and interval-specific PostgreSQL expressions, while
existing fill and label logic remains unchanged.

**Tech Stack:** Go, PostgreSQL SQL expressions, `go-sqlmock`, Testify.

## Global Constraints

- Preserve the Monitor V2 response contract and 64-point limit.
- Preserve existing 1-minute, 5-minute, and 1-hour behavior.
- Unsupported bucket sizes continue to normalize to 60 seconds.
- Do not deploy or modify production.

---

### Task 1: Add Long-Window Repository Regression Tests

**Files:**
- Create: `upstream/sub2api/backend/internal/repository/ops_repo_trends_bucket_test.go`

**Interfaces:**
- Consumes: `(*opsRepository).GetThroughputTrend`, `(*opsRepository).GetErrorTrend`
- Produces: regression coverage for 21,600-second and 86,400-second buckets

- [x] **Step 1: Write failing consumer-level tests**

Add table-driven throughput and error trend tests using an empty SQL result.
Require SQL to contain the requested bucket interval, then assert literal
labels and point counts: `6h`/`28` for seven days and `24h`/`30` for thirty
days.

- [x] **Step 2: Run the focused tests and verify RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestOpsRepositoryPreservesLongTrendBuckets' -count=1
```

Expected: FAIL because the repository rewrites 21,600 and 86,400 seconds to
60 seconds, so the SQL expectations do not match.

### Task 2: Preserve Supported Long Buckets

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/ops_repo_trends.go`

**Interfaces:**
- Produces: `normalizeOpsTrendBucketSeconds(int) int`
- Preserves: existing `GetThroughputTrend` and `GetErrorTrend` signatures

- [x] **Step 1: Implement the minimal fix**

Add 21,600 and 86,400 to a shared supported-bucket normalizer. Use it in both
repository methods. Generate epoch-floor SQL expressions for 300, 21,600, and
86,400 seconds; keep the existing hourly and minute expressions.

- [x] **Step 2: Run the focused tests and verify GREEN**

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestOpsRepositoryPreservesLongTrendBuckets' -count=1
```

Expected: PASS.

### Task 3: Regression Verification and Review

**Files:**
- Verify: `upstream/sub2api/backend/internal/repository`
- Verify: `upstream/sub2api/backend/internal/service`
- Verify: `upstream/sub2api/backend/internal/handler`

- [x] **Step 1: Run related packages**

```bash
cd upstream/sub2api/backend
go test ./internal/repository ./internal/service ./internal/handler
```

- [x] **Step 2: Run formatting and inspect the final diff**

```bash
gofmt -w internal/repository/ops_repo_trends.go \
  internal/repository/ops_repo_trends_bucket_test.go
git diff --check
git diff -- upstream/sub2api/backend/internal/repository \
  docs/superpowers/specs/2026-07-30-monitor-v2-long-window-buckets-design.md \
  docs/superpowers/plans/2026-07-30-monitor-v2-long-window-buckets.md
```

- [x] **Step 3: Confirm acceptance**

The focused tests and related package tests pass, no formatting errors remain,
and the diff contains no frontend, API-contract, deployment, or production
configuration changes.
