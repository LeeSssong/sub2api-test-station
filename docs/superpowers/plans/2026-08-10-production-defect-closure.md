# Production Defect Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Make upstream per-request costs visible for Sub/New API accounts, make account-monitor score/cost help reliable after scrolling, and make official candidate preparation durable and actionable for `0.1.173`.

**Architecture:** Keep the existing admin usage detail and account monitor contracts, adding only the missing New API lookup and explicit admin grouping. Fix the shared fixed-position tooltip at its coordinate boundary and use its existing click mode for dense card metrics. Persist candidate preparation beside the existing updater operation state, reload it on restart, and report host preparation failures instead of leaving an indeterminate UI.

**Tech Stack:** Go services and tests, Vue 3/TypeScript frontend, shell host release chain, Docker blue-green deployment.

## Global Constraints

- Production release uses the reviewed local/host script chain only; do not add GitHub Actions.
- Administrator-only upstream cost/profit data remains inaccessible to non-admin users.
- Candidate preparation never promotes a release; promotion remains a separate explicit update action.
- Preserve the two user-protected worktrees and do not delete them.

### Task 1: New API Upstream Cost And Admin Group

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/sub_upstream_cost.go`
- Modify: `upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go`
- Modify: `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue`
- Add or modify focused frontend test covering administrator grouping.

**Interfaces:** `GetByUsageID` continues returning `SubUpstreamCostDetail`; Sub paths keep `/v1/usage/records`; New API paths use `/api/log/token` and `/api/status`, matching by local/upstream request ID and converting `quota / quota_per_unit` to the exact decimal cost.

- [ ] Write a failing test for a New API account returning a matching token log and `quota_per_unit`, asserting `Status=confirmed`, upstream cost, and profit.
- [ ] Run the focused Go test and observe the failure caused by the current Sub-only endpoint.
- [ ] Implement endpoint-family detection and New API token-log parsing with exact ID matching and bounded HTTP behavior; retain unavailable reason codes for unsupported/auth/error responses.
- [ ] Move the three admin-only cost/profit rows under a labeled “管理员信息” section in the usage detail template without changing the non-admin guard.
- [ ] Run focused backend and frontend tests plus typecheck.
- [ ] Commit the task.

### Task 2: Scroll-Safe Account Monitor Affordances

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/common/HelpTooltip.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Add or modify focused component tests.

**Interfaces:** `HelpTooltip` keeps hover/focus behavior and its existing `trigger="click"` API. Fixed coordinates are viewport-relative (`getBoundingClientRect()` only); score and cost metric triggers use click mode with accessible button semantics and retain tooltip content.

- [ ] Write a failing test that opens a metric tooltip after a non-zero scroll offset and asserts it remains inside the viewport; cover the third card metric trigger.
- [ ] Run the focused component test and observe the failure caused by adding `window.scrollY/scrollX` to fixed coordinates.
- [ ] Correct positioning and convert score/cost triggers to explicit click affordances; preserve outside-click and Escape close behavior.
- [ ] Run focused component tests and frontend typecheck.
- [ ] Commit the task.

### Task 3: Durable Candidate Preparation

**Files:**
- Modify: `sub2api-updater/internal/updater/store.go`
- Modify: `sub2api-updater/internal/updater/service.go`
- Modify: `sub2api-updater/internal/updater/service_test.go`
- Modify: `infra/systemd/sub2api-updater.service` or environment example only if the sidecar state path needs explicit configuration.

**Interfaces:** Add durable candidate state load/save using a `0600` sidecar next to updater state; `NewService` restores the latest candidate and `PrepareCandidate` persists preparing/ready/failed/target_changed transitions. A bounded preparation context marks hung commands failed after the configured timeout.

- [ ] Write failing tests for candidate state surviving service restart and for a preparation error being visible after reload.
- [ ] Run focused updater tests and observe the failure because `s.candidate` is memory-only.
- [ ] Implement atomic sidecar persistence, startup restore, and timeout-wrapped preparation while preserving operation-state compatibility.
- [ ] Run all updater tests, `go vet`, and shell syntax tests.
- [ ] Commit the task.

### Task 4: Integration, Release, And Verification

**Files:** `docs/project/project-progress.md` and release evidence files only.

- [ ] Merge the three task commits into the isolated candidate, resolve conflicts, and run focused regressions, updater tests, frontend typecheck/build, and migration/release preflight.
- [ ] Fast-forward/merge the verified candidate into `main`, push `origin/main`, and publish through the reviewed host blue-green chain with the user-authorized downtime.
- [ ] Prepare and qualify `0.1.173` on the production host, then promote only after readiness is true.
- [ ] Verify public health/readiness, admin usage detail data, account-monitor third-card click tooltip, and updater version/candidate state.
- [ ] Record deployment evidence and mark the ledger complete only after push, deployment, and online verification.
