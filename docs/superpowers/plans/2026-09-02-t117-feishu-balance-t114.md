# T117 Feishu Balance Card T114 Ranking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Feishu upstream-balance cards render the existing 1h/6h/24h silence actions when the protected callback capability is configured, and source account ranks from the T114 scheduler projection at send time.

**Architecture:** Keep the existing `ops_alert_events` silence ledger, callback route, token hashing, BaseURL aggregation, and Feishu sender. Add a narrow in-process ranking projection contract around the existing scheduler provider; `ReadUpstreamBalanceEvaluations` will use one snapshot/projection pass per evaluation, propagate snapshot freshness and rank metadata into the card input, and degrade ranking independently from balance delivery.

**Tech Stack:** Go, existing Sub2API service/repository/notify packages, sqlmock/testify tests, generated wire registration, no migration.

**Spec:** `docs/superpowers/specs/2026-09-02-t117-feishu-balance-card-t114-ranking-silence-design.md`

## Global Constraints

- Work only in `.worktrees/t117-feishu-balance-t114`; do not modify root `main`.
- Reuse T114 `1h + 24h + 7d` quality provider and existing scheduler projection; do not add a second ranking formula or external API.
- Keep balance alerts deliverable when ranking or silence actions are unavailable.
- Keep silence scoped to normalized BaseURL and use only 1h, 6h, and 24h durations.
- No database migration, production data mutation, real Feishu delivery, GitHub Actions, push, or deployment.

### Task 1: Define the card-ranking contract and write RED tests

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service_test.go`
- Modify: `upstream/sub2api/backend/internal/notify/upstream_balance_card.go`
- Modify: `upstream/sub2api/backend/internal/notify/upstream_balance_card_test.go`

**Interfaces:**
- Consume existing `OpenAIAccountSchedulerProjectionProvider.Project(context.Context, OpenAIAccountSchedulerProjectionRequest)`.
- Produce card rank metadata containing group name, rank, rank total, eligibility, T114 enabled state, and non-sensitive display state; also produce one snapshot timestamp and stale flag per evaluation.

- [ ] **Step 1: Add failing service tests** for a T114 projection-backed evaluation, including rank total, projection timestamp, stale propagation, ineligible account display state, and no `ListWindow("24h")` dependency in the new path.
- [ ] **Step 2: Run the focused service test**:

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/service -run 'Test(ReadUpstreamBalanceEvaluations|BuildUpstreamBalanceEvaluations).*T114' -count=1
  ```

  Expected: FAIL because the card evaluation has no T114 snapshot/rank metadata contract.

- [ ] **Step 3: Add failing notify tests** for normal rank text, `当前不可调度`, `未启用 T114 排名`, stale/no-snapshot labels, and rank totals.
- [ ] **Step 4: Run the focused notify test**:

  ```bash
  go test ./internal/notify -run 'TestRenderUpstreamBalanceCard.*T114' -count=1
  ```

  Expected: FAIL because the card input has no metadata fields or display behavior.

### Task 2: Implement T114 projection-backed balance evaluations

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go` only if a narrow helper is required to expose the already-injected projection
- Test: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service_test.go`

**Interfaces:**
- `AccountMonitorService.ReadUpstreamBalanceEvaluations` reads accounts and current balance snapshots, then uses the existing injected scheduler projection for each applicable group without acquiring slots or making upstream calls.
- The resulting `UpstreamBalanceEvaluation` carries `RankingSnapshotAt` and `RankingStale`; each `UpstreamBalanceAccountRank` carries rank total, eligibility, T114 enablement, and a safe display reason.

- [ ] **Step 1: Implement a single snapshot-at evaluation path** that reuses the current quality provider and scheduler projection, preserving normalized BaseURL grouping and balance freshness checks.
- [ ] **Step 2: Keep non-T114 groups explicit** and map unavailable/ineligible candidates to display states instead of stale numeric ranks.
- [ ] **Step 3: Run the service RED tests again** and confirm they pass:

  ```bash
  go test ./internal/service -run 'Test(ReadUpstreamBalanceEvaluations|BuildUpstreamBalanceEvaluations).*T114' -count=1
  ```

- [ ] **Step 4: Run the existing balance-notification service focused suite**:

  ```bash
  go test ./internal/service -run 'Test(NormalizeNotificationBaseURL|EvaluateUpstreamBaseURLBalance|ReadUpstreamBalanceEvaluations|UpstreamBalanceNotificationService)' -count=1
  ```

### Task 3: Render ranking metadata and restore/verify silence actions

**Files:**
- Modify: `upstream/sub2api/backend/internal/notify/upstream_balance_card.go`
- Modify: `upstream/sub2api/backend/internal/notify/upstream_balance_card_test.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go` only for non-sensitive disabled-action diagnostics if needed
- Test: `upstream/sub2api/backend/internal/handler/feishu_upstream_balance_callback_test.go`
- Test: `upstream/sub2api/backend/internal/repository/upstream_balance_event_repo_test.go`

**Interfaces:**
- `UpstreamBalanceCardInput` receives the evaluation snapshot metadata and existing opaque `SilenceToken`.
- Existing callback service/repository signatures remain unchanged.

- [ ] **Step 1: Implement card rendering** for T114 source/time, stale marker, rank totals, ineligible/non-T114/no-snapshot labels, while retaining the existing 30 KiB fail-closed limit and credential escaping.
- [ ] **Step 2: Preserve the existing conditional action rendering**: configured protected callback token yields exactly 1h/6h/24h actions; absent/unsafe token yields no actions but still sends the balance body.
- [ ] **Step 3: Add or tighten non-sensitive structured diagnostics** for disabled silence actions without logging callback/action tokens or credentials.
- [ ] **Step 4: Run notify, handler, and repository focused tests**:

  ```bash
  go test ./internal/notify -run 'TestRenderUpstreamBalanceCard|TestLoadUpstreamBalanceSecrets|TestFeishuSender' -count=1
  go test ./internal/handler -run 'TestFeishuUpstreamBalanceCallback' -count=1
  go test ./internal/repository -run 'TestUpstreamBalanceEventRepository' -count=1
  ```

### Task 4: Wiring, regression verification, and handoff

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/wire.go` only if required by tests
- Modify: `upstream/sub2api/backend/cmd/server/wire_gen.go` only if generated wiring changes
- Create: `docs/handoffs/2026-09-02-t117-feishu-balance-t114-handoff.md`

- [ ] **Step 1: Verify protected-file wiring** for configured, missing, unsafe-permission, and parent-permission cases using existing secret-loader tests; do not add secrets to the repository.
- [ ] **Step 2: Run the complete direct regression set**:

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/service ./internal/notify ./internal/handler ./internal/repository -run 'Test(UpstreamBalance|ReadUpstreamBalance|RenderUpstreamBalance|FeishuUpstreamBalanceCallback|LoadUpstreamBalanceSecrets)' -count=1
  go build ./cmd/server
  gofmt -w <only changed Go files>
  git diff --check
  ```

- [ ] **Step 3: Confirm no migration, configuration, production-data, or root-main changes.**
- [ ] **Step 4: Write the handoff with base SHA, candidate SHA, changed files, tests, residual risks, and release boundary; leave the candidate unmerged and unpushed.**

