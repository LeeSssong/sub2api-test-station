# T98-R2 Feishu Balance Freshness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development task-by-task.

**Goal:** Prevent old-Key and stale-zero balance snapshots from producing Feishu alerts, and actively recover zero-balance accounts after recharge.

**Architecture:** Bind successful balance evidence to a one-way API Key fingerprint, filter notification evidence by identity and a 10-minute freshness window, and let active zero events request a scoped balance refresh before due processing. Existing account balance persistence remains responsible for scheduler outbox/cache synchronization.

**Tech Stack:** Go 1.27, Sub2API account monitor, PostgreSQL-backed account repository, Go testing/testify.

**Spec:** `docs/superpowers/specs/2026-09-01-t98-r2-feishu-balance-freshness-design.md`

## Global Constraints

- No plaintext API Key in snapshots, logs, errors, tests, or evidence.
- No migration or new configuration.
- Candidate worktree only; do not merge, push, deploy, or send real Feishu messages.

### Task 1: Credential-bound fresh balance evidence

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_balance.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_balance_test.go`
- Test: `upstream/sub2api/backend/internal/service/upstream_balance_notification_test.go`

- [x] Write failing tests for fingerprint persistence, old-Key rejection, and snapshots older than 10 minutes.
- [x] Run focused tests and verify the expected failures.
- [x] Add SHA-256 credential fingerprint generation and notification freshness/identity filtering.
- [x] Run focused tests and verify they pass.

### Task 2: Active zero-event refresh and recovery

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Test: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service_test.go`

- [x] Write failing tests proving an unschedulable active account is refreshed for a zero-event BaseURL and `RunDue` refreshes before reading the projection.
- [x] Run focused tests and verify the expected failures.
- [x] Add the scoped refresher interface and account-monitor implementation.
- [x] Run focused tests and verify recovery resolves without sending another zero alert.

### Task 3: Verification and candidate handoff

**Files:**
- Create: `docs/handoffs/2026-09-01-t98-r2-feishu-balance-freshness-handoff.md`

- [x] Run directly related service/repository tests and `go build ./cmd/server`.
- [x] Run `gofmt -l`, `git diff --check`, and a sensitive-field diff review.
- [x] Commit the candidate and stop at `READY_FOR_ROOT_REVIEW`.
