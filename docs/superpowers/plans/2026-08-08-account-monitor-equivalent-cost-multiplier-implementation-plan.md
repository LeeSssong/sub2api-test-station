# Account Monitor Equivalent Cost Multiplier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace account-card reconciled billing evidence with one model-price-adjusted equivalent site multiplier.

**Architecture:** Sub2API computes the value in the existing account-monitor projection from the monitor model, account model mapping, model pricing, and the account effective multiplier. The frontend renders the single projected value and stops loading relay-ops cost guards for account cards.

**Tech Stack:** Go, Vue 3, TypeScript, Vitest

## Global Constraints

- No database migration.
- Do not change routing, billing, reconciliation, or scheduler behavior.
- Only focused tests and the existing blue-green production release path are required.

---

### Task 1: Project equivalent cost multiplier

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

- [x] Write failing tests for unchanged-model, mapped-model, zero fallback, and missing-price behavior.
- [x] Run the focused Go tests and confirm the new assertions fail.
- [x] Inject the existing billing price resolver and project `equivalent_site_multiplier`.
- [x] Run the focused Go tests and confirm they pass.

### Task 2: Simplify the account card

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Test: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

- [x] Write failing component/view tests asserting one cost field and no relay cost-guard request.
- [x] Run focused Vitest tests and confirm they fail.
- [x] Remove cost-guard loading and render only `equivalent_site_multiplier`.
- [x] Run focused Vitest tests and confirm they pass.

### Task 3: Verify and release

- [x] Run focused backend tests, focused frontend tests, frontend typecheck, and frontend build.
- [x] Review the scoped diff and update the project ledger without marking complete.
- [ ] Commit and push the scoped change.
- [ ] Deploy through the existing Sub2API blue-green release path without a database migration.
- [ ] Verify the production account-monitor API and page, then mark the ledger complete only after push, deployment, and online verification.
