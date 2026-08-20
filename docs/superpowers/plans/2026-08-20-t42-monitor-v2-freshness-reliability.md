# T42 Monitor V2 Freshness and Refresh Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the latest native probe timestamp per Monitor V2 group, label the current fixed bucket, and retry failed GET refreshes without abandoning the existing snapshot.

**Architecture:** Reuse the existing account-monitor SQL latest-probe CTE and carry `checked_at` through service and handler as optional `source_updated_at`; keep contract version 7 and bucket generation unchanged. Add a small retry scheduler in `MonitorV2View` that only affects GET reads and is cancelled by visibility/unmount lifecycle.

**Tech Stack:** Go service/repository/handler, Vue 3 + TypeScript, Vitest, Go test.

**Spec:** `docs/superpowers/specs/2026-08-20-t42-monitor-v2-freshness-reliability-design.md`

## Global Constraints

- Preserve Monitor V2 contract version `7` and fixed timeline lengths `24/28/30`.
- Reuse native `account_monitor_results.checked_at`; no new migration, probe, backfill, or production setting.
- Do not edit `main`, global queue, or project progress from this worktree.
- Run only direct Monitor V2 tests plus required typecheck/build and diff check.

### Task 1: Backend freshness projection (TDD)

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/handler/monitor_v2_handler.go`
- Test: `upstream/sub2api/backend/internal/service/monitor_v2_test.go`
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- Test: `upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go`

- [ ] Write failing tests asserting `SourceUpdatedAt` is carried from the native projection into the service snapshot and handler JSON.
- [ ] Run `go test ./internal/service ./internal/handler ./internal/repository -run 'MonitorV2|monitor v2'` and observe failure because the field is absent.
- [ ] Add optional `SourceUpdatedAt *time.Time` fields and aggregate `MAX(l.checked_at)` in `current_by_group`; scan and serialize with `omitempty`.
- [ ] Re-run the focused Go tests and format changed Go files with `gofmt`.

### Task 2: Frontend freshness contract and card label (TDD)

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/types.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/api.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2GroupCard.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`
- Test: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/api.spec.ts`
- Test: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`

- [ ] Add failing parser and card assertions for optional `source_updated_at`.
- [ ] Run the API and View Vitest files and confirm the expected failures.
- [ ] Implement nullable RFC3339 validation and freshness text without changing timeline files or bucket arrays.
- [ ] Re-run focused Vitest and `pnpm typecheck`.

### Task 3: Refresh retry reliability (TDD)

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2View.vue`
- Test: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`

- [ ] Add failing tests for a periodic GET rejection followed by a 5-second retry, and for no fallback event on refresh failure.
- [ ] Run the focused View spec to verify RED.
- [ ] Add a 5-second retry timer, preserve the previous snapshot on failure, and cancel retry on hidden/unmount/abort; successful responses restore configured interval.
- [ ] Run View/API/Timeline specs together and verify GREEN.

### Task 4: Verification and handoff

**Files:**
- Create: `docs/superpowers/reports/2026-08-20-t42-monitor-v2-freshness-reliability-verification.md`
- Create: `docs/handoffs/2026-08-20-t42-monitor-v2-freshness-reliability-handoff.md`

- [ ] Run direct Go tests, frontend Vitest, typecheck, production build, and `git diff --check`.
- [ ] Record migration/config/downtime status, final commit SHA, tests, rollback, and residual risks.
- [ ] Commit all task changes and report `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or edit global docs.
