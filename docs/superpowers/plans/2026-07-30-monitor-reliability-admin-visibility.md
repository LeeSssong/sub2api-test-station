# Monitor Reliability And Admin Visibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recover multiplier measurements from delayed upstream counters,
show all active groups to administrators, and bind channel monitors to stable
group IDs.

**Architecture:** Keep the current services and routes. Add bounded polling
inside `AccountMultiplierService`, pass a trusted visibility scope from the
Monitor V2 handler, and extend the existing channel-monitor Ent entity with a
nullable group foreign key plus legacy name fallback.

**Tech Stack:** Go 1.24, Gin, Ent/PostgreSQL, Vue 3, TypeScript, Vitest, pnpm.

## Global Constraints

- Preserve ordinary-user public-only Monitor V2 behavior.
- Never expose secrets, raw quota values, or raw upstream errors.
- Keep failed multiplier refresh independent from connectivity status.
- Preserve legacy `group_name` compatibility.
- Do not stage or alter `监控日报-2026-07-28.md`.
- Deploy through the existing 30-minute rapid-release lane with rollback to
  the currently qualified image.

---

### Task 1: Recover delayed multiplier measurements

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_multiplier_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_multiplier.go`

**Interfaces:**
- Produces: bounded `waitForNewAPIQuotaUsage(...) (float64, error)`.
- Produces: separate retry TTL for failed measurement snapshots.

- [ ] Add a failing HTTP test where the first post-completion usage reads do
  not advance and a later read is positive.
- [ ] Add a failing refresh-policy test proving a recent failed snapshot is
  retried after the short failure interval.
- [ ] Run focused tests and confirm the expected failures.
- [ ] Implement bounded polling, test-injectable wait behavior, sanitized
  failure codes, and short failed-snapshot retry.
- [ ] Re-run the focused tests.

### Task 2: Add role-aware Monitor V2 group visibility

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2_test.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2.go`
- Modify: `upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/monitor_v2_handler.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/monitor_v2_routes_test.go`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`

**Interfaces:**
- Produces: `MonitorV2ScopePublic` and `MonitorV2ScopeAdmin`.
- Handler derives scope from `GetUserRoleFromContext`.

- [ ] Add failing service tests for public and admin selection.
- [ ] Add a failing handler test proving an admin request uses admin scope.
- [ ] Implement explicit scope selection without query-string override.
- [ ] Update privacy copy to describe ordinary-user and administrator views.
- [ ] Re-run focused backend and frontend tests.

### Task 3: Bind channel monitors to stable group IDs

**Files:**
- Create: `upstream/sub2api/backend/migrations/192_channel_monitor_group_id.sql`
- Modify/regenerate: `upstream/sub2api/backend/ent/schema/channel_monitor.go`
- Modify/regenerate: `upstream/sub2api/backend/ent/**`
- Modify: `upstream/sub2api/backend/internal/service/channel_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/channel_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/repository/channel_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/channel_monitor_handler.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2.go`
- Modify: focused channel-monitor and Monitor V2 tests
- Modify: `upstream/sub2api/frontend/src/api/admin/channelMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/monitor/MonitorFormDialog.vue`
- Modify: focused form tests

**Interfaces:**
- Produces: nullable `group_id` in channel-monitor CRUD and user projections.
- Monitor V2 uses ID association first and name fallback second.

- [ ] Add failing service/repository/handler tests for `group_id` persistence.
- [ ] Add failing Monitor V2 tests for ID-first matching and legacy fallback.
- [ ] Add the idempotent migration and Ent field; regenerate Ent code.
- [ ] Thread `group_id` through service, repository, handlers, duplication,
  user projections, and frontend types.
- [ ] Replace the free-form group field with an admin group selector while
  keeping `group_name` in the payload.
- [ ] Re-run focused backend and frontend tests.

### Task 4: Verify and release

- [ ] Run focused Go tests for service, repository, handlers, and routes.
- [ ] Run frontend focused tests, typecheck, lint, and build.
- [ ] Run `git diff --check` and inspect the complete diff.
- [ ] Commit only this repair, push the `codex/fix-monitor-reliability` branch,
  and merge through the existing release process.
- [ ] Build and qualify a new image, deploy production, and verify health.
- [ ] Idempotently set production channel monitor 13 `group_id=16`.
- [ ] Force one account-monitor refresh for account 23 after the new code is
  active and confirm the measurement no longer remains stuck on the old
  failure snapshot.
- [ ] Verify admin Monitor V2 returns six groups and the public scope returns
  one group.
