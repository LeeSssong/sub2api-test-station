# T98-R2 飞书余额新鲜度与排名投影修复实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复飞书上游余额通知的余额快照新鲜度和原生分组调度排名展示。

**Architecture:** 保留 Sub2API 原生账号监控、余额快照和 `scheduler_rank`。将辅助余额刷新从主动探测成功分支中解耦，并让通知读取 24 小时窗口投影以获得原生调度排名。

**Tech Stack:** Go 1.27、Sub2API 原生 account monitor service、Go testing/testify。

**Spec:** `docs/superpowers/specs/2026-09-01-t98-r2-feishu-balance-rank-design.md`

## Global Constraints

- 余额口径固定为同一规范化 BaseURL 下 `observed_at` 最新的有效 API Key 可用余额。
- 不新增迁移、配置、事实源、生产写入或真实飞书发送。
- 只在独立 worktree 实现和验证，不合并、推送或部署。

### Task 1: 余额刷新与探测解耦

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

- [x] **Step 1: Write the failing regression test** for a real-traffic account that skips the active probe but still refreshes auxiliary balance/declaration state.
- [x] **Step 2: Run the focused test and confirm it fails** because the current early return skips refresh.
- [x] **Step 3: Move auxiliary refresh before the active-probe skip return**, preserving the existing probe skip semantics and error handling.
- [x] **Step 4: Run the focused test and confirm it passes.**

### Task 2: Notification ranking projection

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service.go`
- Test: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service_test.go`

- [x] **Step 1: Write the failing regression test** asserting the balance reader uses the 24-hour window projection and the card input carries all group scheduler ranks.
- [x] **Step 2: Run the focused test and confirm it fails** because `ReadUpstreamBalanceEvaluations` currently calls `List()`.
- [x] **Step 3: Change the reader call to `ListWindow(ctx, string(AccountMonitorRange24Hours))`**, leaving evaluator and card conversion unchanged.
- [x] **Step 4: Run service and notify focused tests.**

### Task 3: Final verification and handoff

**Files:**
- Modify: `docs/handoffs/2026-09-01-t98-r2-feishu-balance-rank-handoff.md`

- [x] **Step 1: Run T98 service/notify focused tests, `go build ./cmd/server`, `gofmt -l`, and `git diff --check`.**
- [x] **Step 2: Review the diff for scope, sensitive values, migration/config changes, and real-send paths.**
- [x] **Step 3: Write the handoff with baseline, commit, changed files, test evidence, release boundary, rollback and remaining risks.**
- [x] **Step 4: Mark the candidate `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or delete the worktree.**
