# T98-R3 Feishu Notification Snooze Implementation Plan
> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add secure Feishu card actions that silence one normalized upstream BaseURL balance alert for 1 hour, 6 hours, or 24 hours.

**Architecture:** Reuse the existing `ops_alert_events` balance notification ledger and extend it with a nullable `silenced_until` timestamp plus a hashed card scope token. Render three Feishu card buttons carrying an opaque action value; a public callback handler validates the configured callback token, recipient identity, action token, and duration before atomically updating the event. Balance notification claiming skips firing events while the silence window is active.

**Tech Stack:** Go, Gin, PostgreSQL migrations, `database/sql`, Feishu interactive cards, existing Wire provider graph, Go tests.

**Spec:** `docs/superpowers/specs/2026-09-01-t98-r3-feishu-notification-snooze-design.md`

## Global Constraints

- Scope is the normalized BaseURL of the existing upstream balance event.
- Persist only non-sensitive state; never store API keys, passwords, or full card payloads.
- Read callback secrets only from local mode `0600` protected files.
- Do not send real Feishu messages or call production callbacks during development.
- Do not merge, push, deploy, modify root `main`, or modify global task ledgers from this worktree.

### Task 1: Extend the balance event contract and migration

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/ops_alert_models.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_port.go`
- Modify: `upstream/sub2api/backend/internal/repository/upstream_balance_event_repo.go`
- Create: `upstream/sub2api/backend/migrations/233_upstream_balance_notification_silence.sql`
- Test: `upstream/sub2api/backend/internal/repository/upstream_balance_event_repo_test.go`
- Test: `upstream/sub2api/backend/migrations/upstream_balance_notification_silence_migration_test.go`

**Interfaces:**
- Produce `UpstreamBalanceEvent.SilencedUntil *time.Time`.
- Produce `UpstreamBalanceNotificationSilenceInput{RuleID, ScopeKey, ActionTokenHash, Until, Now}`.
- Produce repository method `Silence(ctx, input) (bool, error)`.
- Produce repository method `IsSilenced(ctx, ruleID, scopeKey, now) (bool, error)`.

- [ ] **Step 1: Add failing repository and migration tests** asserting the new column, no sensitive columns, atomic silence update, and expiry behavior.
- [ ] **Step 2: Run the focused tests and confirm they fail** because the column, methods, and migration do not exist.
- [ ] **Step 3: Add the additive migration and service/repository contract fields.**
- [ ] **Step 4: Update event SELECT/scan and Claim logic so active claims return false during `silenced_until > Now`.**
- [ ] **Step 5: Implement atomic `Silence` with scope lock, token-hash comparison, and lease cleanup.**
- [ ] **Step 6: Run repository/migration focused tests and confirm they pass.**
- [ ] **Step 7: Commit `feat: persist upstream balance notification silence`.**

### Task 2: Add opaque card actions and callback secret loading

**Files:**
- Modify: `upstream/sub2api/backend/internal/notify/upstream_balance_card.go`
- Modify: `upstream/sub2api/backend/internal/notify/upstream_balance_secrets.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service.go`
- Test: `upstream/sub2api/backend/internal/notify/upstream_balance_card_test.go`
- Test: `upstream/sub2api/backend/internal/notify/upstream_balance_secrets_test.go`
- Test: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service_test.go`

**Interfaces:**
- Produce card action values containing only an opaque token and one of `1h`, `6h`, or `24h`.
- Produce `UpstreamBalanceCardInput.SilenceToken string`.
- Produce `UpstreamBalanceSecrets.CallbackToken string` loaded from `SUB2API_UPSTREAM_BALANCE_FEISHU_CALLBACK_TOKEN_FILE`.
- Produce token helpers that return a cryptographically random raw token and a SHA-256 hash for persistence.

- [ ] **Step 1: Add failing card tests** asserting three buttons, exact duration values, opaque action data, and no BaseURL/password leakage in action values.
- [ ] **Step 2: Run the card tests and confirm they fail** because the card has no action elements or token field.
- [ ] **Step 3: Add minimal card action rendering and wire the token into notification card input.**
- [ ] **Step 4: Add failing secret-loader tests** for protected callback-token files and missing/unsafe files.
- [ ] **Step 5: Implement protected callback-token loading and service provider wiring.**
- [ ] **Step 6: Run notify/service focused tests and confirm they pass.**
- [ ] **Step 7: Commit `feat: add feishu balance silence actions`.**

### Task 3: Implement Feishu callback handling and routing

**Files:**
- Create: `upstream/sub2api/backend/internal/handler/feishu_balance_callback_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/wire.go`
- Create: `upstream/sub2api/backend/internal/server/routes/feishu.go`
- Modify: `upstream/sub2api/backend/internal/server/router.go`
- Test: `upstream/sub2api/backend/internal/handler/feishu_balance_callback_handler_test.go`
- Test: `upstream/sub2api/backend/internal/server/routes/feishu_test.go`

**Interfaces:**
- Produce `POST /api/v1/notifications/feishu/upstream-balance/callback`.
- Accept Feishu URL-verification challenges and card action callbacks.
- Validate callback token, `open_id` against configured recipients, action token hash, normalized duration, and BaseURL event scope.
- Return a bounded JSON success message; never echo secrets or raw action payloads.

- [ ] **Step 1: Add failing handler tests** for challenge response, valid silence, invalid token, unauthorized recipient, invalid duration, replay/idempotency, and unknown action token.
- [ ] **Step 2: Run handler/route tests and confirm they fail** because the handler and route do not exist.
- [ ] **Step 3: Implement strict request decoding and callback validation.**
- [ ] **Step 4: Register the route without JWT/admin middleware, relying on callback-token and recipient checks.**
- [ ] **Step 5: Wire the handler through existing Wire providers without changing unrelated handlers.**
- [ ] **Step 6: Run focused handler/route tests and confirm they pass.**
- [ ] **Step 7: Commit `feat: handle feishu balance silence callbacks`.**

### Task 4: Integrate suppression into notification delivery

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/backend/internal/repository/wire.go`
- Test: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service_test.go`

- [ ] **Step 1: Add failing service tests** proving a valid silence suppresses due notification claims, expiry permits claims, and healthy resolution remains unchanged.
- [ ] **Step 2: Run the focused service tests and confirm they fail.**
- [ ] **Step 3: Add the repository-backed silence check to the claim path and preserve existing lease/CAS behavior.**
- [ ] **Step 4: Run all direct service/repository/notify/handler tests.**
- [ ] **Step 5: Commit `fix: suppress silenced feishu balance alerts`.**

### Task 5: Final verification and handoff

**Files:**
- Modify: `docs/handoffs/2026-09-01-t98-r3-feishu-notification-snooze-handoff.md`

- [ ] **Step 1: Run focused tests, `go build ./cmd/server`, `gofmt -l`, and `git diff --check`.**
- [ ] **Step 2: Review migration/config/sensitive-value scope and confirm no real Feishu calls.**
- [ ] **Step 3: Write handoff with baseline, commits, tests, migration/config changes, release boundary, rollback, and residual risks.**
- [ ] **Step 4: Leave the candidate at `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or delete the worktree.**
