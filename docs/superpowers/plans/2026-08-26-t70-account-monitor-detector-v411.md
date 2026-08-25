# T70 账号检测分层监测与记录面板实施计划

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** 将 detector v4.1.1 的低/中/高证据语义接入原生账号监控，并把最近检测摘要升级为可分页、可筛选、可展开的响应式检测记录面板。

**Architecture:** Sub 原生 runner、队列、账号投影和 account_model_detection_runs 继续作为唯一事实源；私网 sidecar 只生成有界检测证据。中档周期任务、低档手动复核和高档异常升级都复用同一异步任务链，前端用一个响应式抽屉/全屏面板查看历史。

**Tech Stack:** Go、Gin、PostgreSQL migrations、Vue 3、TypeScript、Vitest、Tailwind、现有本地/宿主蓝绿发布链。

**Spec:** docs/superpowers/specs/2026-08-26-t70-account-monitor-detector-v411-design.md

## Global Constraints

- Sub 原生账号监控、调度资格、错误隔离、计费和用量仍是唯一事实源；detector 不得创建第二套控制面。
- 不复制或打包 chen-006/gpt56_api_detector 的 PolyForm Noncommercial 核心、可信基线或报告逻辑进入商业生产镜像。
- API Key、Base URL、Authorization、prompt、output、response、token、secret 和 credentials 不得落库、入日志或返回前端。
- 旧检测记录不回填；没有新鲜 detector 证据时只显示历史参考，不改变调度资格。
- 所有新增 sidecar 摘要沿用现有 body/summary/string 深度与敏感 key 限制。
- 不使用 GitHub Actions；部署必须从验证后的 main 通过既有本地/宿主蓝绿链执行。

---

### Task 1: 扩展检测契约、持久化与历史分页

**Files:**
- Create: upstream/sub2api/backend/migrations/228_account_model_detection_evidence.sql
- Modify: upstream/sub2api/backend/internal/service/account_model_detection_types.go
- Modify: upstream/sub2api/backend/internal/service/account_model_detection.go
- Modify: upstream/sub2api/backend/internal/repository/account_model_detection_repo.go
- Modify: upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go
- Test: upstream/sub2api/backend/internal/repository/account_model_detection_repo_test.go
- Test: upstream/sub2api/backend/internal/service/account_model_detection_test.go
- Test: upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go
- Test: upstream/sub2api/backend/migrations/account_model_detection_migration_test.go

**Interfaces:**
- Produces AccountModelDetectionRun fields Profile, Mode, TriggerReason, PlannedRequests, ValidSamples, EvidenceState, and FingerprintStatus.
- Produces AccountModelDetectionRequest fields Profile, Mode, and TriggerReason.
- Produces AccountModelDetectionResponse fields Profile, PlannedRequests, ValidSamples, EvidenceState, and FingerprintStatus.
- Changes repository history access to ListRecent(ctx, accountID, limit, cursor, status, profile, mode) with stable created_at/id descending cursor.
- Changes handler history response to {items,next_cursor} while keeping omitted query parameters compatible with the existing 25-row response shape.

- [ ] Step 1: Write failing type and repository tests. Assert that a queued high/escalation run preserves profile/mode/reason, a completed response persists all evidence fields, old rows scan as unknown/historical, and cursor filtering returns the next page without duplicates.
- [ ] Step 2: Run the focused tests and confirm RED.

    cd upstream/sub2api/backend
    go test ./internal/repository ./internal/service ./internal/handler/admin ./migrations -run 'ModelDetection|AccountMonitor' -count=1

Expected: compile/test failure because the new fields and history signature do not exist.
- [ ] Step 3: Add migration 228. Add nullable/default-compatible columns for profile, mode, trigger_reason, planned_requests, valid_samples, evidence_state, and fingerprint_status; add an index on account_id, created_at DESC, id DESC. Do not update existing rows.
- [ ] Step 4: Implement bounded domain fields and repository SQL. Include the new columns in enqueue, claim, completion and list scans. Validate profile/mode enums at enqueue and clamp history limit to 1..100. Encode cursor as base64url JSON containing the last row UTC timestamp and UUID.
- [ ] Step 5: Extend the handler query contract. Parse limit, cursor, status, profile, and mode; return a bounded next_cursor; retain the existing route and auth middleware.
- [ ] Step 6: Run focused tests and migration checks GREEN.

    cd upstream/sub2api/backend
    go test ./internal/repository ./internal/service ./internal/handler/admin ./migrations -run 'ModelDetection|AccountMonitor' -count=1

- [ ] Step 7: Commit the contract slice.

    git add upstream/sub2api/backend/migrations/228_account_model_detection_evidence.sql upstream/sub2api/backend/internal/service/account_model_detection_types.go upstream/sub2api/backend/internal/service/account_model_detection.go upstream/sub2api/backend/internal/repository/account_model_detection_repo.go upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go upstream/sub2api/backend/internal/repository/account_model_detection_repo_test.go upstream/sub2api/backend/internal/service/account_model_detection_test.go upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go upstream/sub2api/backend/migrations/account_model_detection_migration_test.go
    git commit -m "feat: add structured detector evidence history"

### Task 2: Add tiered scheduling, escalation and sidecar v4.1.1 fields

**Files:**
- Modify: upstream/sub2api/backend/internal/service/account_model_detection.go
- Modify: upstream/sub2api/backend/internal/service/account_monitor_runner.go only for existing detection loop wiring
- Modify: upstream/sub2api/backend/internal/service/account_model_detection_sidecar.go
- Modify: upstream/sub2api/backend/internal/service/account_model_detection_types.go
- Test: upstream/sub2api/backend/internal/service/account_model_detection_test.go
- Test: upstream/sub2api/backend/internal/service/account_model_detection_sidecar_test.go
- Test: upstream/sub2api/backend/internal/service/account_monitor_runner_test.go
- Test: upstream/sub2api/backend/cmd/model-detector/main_test.go only for contract fixtures; do not copy upstream detector core

**Interfaces:**
- Produces EnqueueScheduledMedium, EnqueueManualLow, and EnqueueEscalationHigh behavior through the existing service queue.
- Produces shouldEscalateDetection(recent []AccountModelDetectionRun) (bool, string) with reasons first_run, consecutive_abnormal, insufficient, or model_conflict.
- Sidecar requests include profile, mode, and trigger_reason; sidecar response parsing accepts v4.1.1 fields and rejects unbounded/invalid payloads.

- [ ] Step 1: Write failing policy tests. Cover medium scheduled runs, low manual runs, high first-run escalation, two consecutive abnormal runs, two consecutive insufficient runs, immediate model conflict escalation, active-task dedupe, and high-tier cooldown.
- [ ] Step 2: Run the policy tests and confirm RED.

    cd upstream/sub2api/backend
    go test ./internal/service -run 'Detection|Runner' -count=1

- [ ] Step 3: Implement policy helpers and run creation. Keep trigger_kind manual/scheduled compatibility, store profile/mode/reason separately, use existing slot keys, and avoid enqueueing high-tier work when queued/running or inside the cooldown window.
- [ ] Step 4: Implement sidecar request/response bounds. Parse profile/mode/reason, status, evidence state, sample counts, Juice state, fingerprint state/candidate/similarity, detector version and errors. Preserve T48 evidence envelope behavior and never use response model as a fingerprint candidate.
- [ ] Step 5: Add escalation scheduling after completion. A completion with hard conflict schedules one high run; insufficient schedules high only after two consecutive insufficient final results; scheduled medium remains the only routine automatic profile.
- [ ] Step 6: Run policy, sidecar, runner and server build checks GREEN.

    cd upstream/sub2api/backend
    go test ./internal/service ./internal/handler/admin ./cmd/model-detector -run 'Detection|Runner|Detector' -count=1
    go build ./cmd/server

- [ ] Step 7: Commit the scheduling slice.

    git add upstream/sub2api/backend/internal/service/account_model_detection.go upstream/sub2api/backend/internal/service/account_monitor_runner.go upstream/sub2api/backend/internal/service/account_model_detection_sidecar.go upstream/sub2api/backend/internal/service/account_model_detection_types.go upstream/sub2api/backend/internal/service/account_model_detection_test.go upstream/sub2api/backend/internal/service/account_model_detection_sidecar_test.go upstream/sub2api/backend/internal/service/account_monitor_runner_test.go upstream/sub2api/backend/cmd/model-detector/main_test.go
    git commit -m "feat: schedule tiered account detector checks"

### Task 3: Build the responsive detection history panel

**Files:**
- Create: upstream/sub2api/frontend/src/components/admin/account-monitor/AccountModelDetectionHistoryPanel.vue
- Modify: upstream/sub2api/frontend/src/api/admin/accountMonitor.ts
- Modify: upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue
- Modify: upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue
- Modify: upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts
- Modify: upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts
- Test: upstream/sub2api/frontend/src/components/admin/account-monitor/AccountModelDetectionHistoryPanel.spec.ts
- Test: upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts
- Test: upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts

**Interfaces:**
- modelDetectionHistory(accountID, params) consumes limit, cursor, status, profile, and mode and returns items plus next_cursor.
- AccountModelDetectionHistoryPanel consumes show and account, emits close, and owns selected row, filters, cursor loading, and structured detail expansion.
- AccountMonitorCard emits openModelDetectionHistory(accountID) and keeps the existing manual detection action separate.

- [ ] Step 1: Write failing component/API tests. Assert table columns on desktop, timeline rows on narrow viewport, filter changes, load-more cursor calls, structured Juice/fingerprint detail, historical fallback labeling, detector unavailable state, and no raw sensitive fields in rendered text.
- [ ] Step 2: Run focused Vitest and confirm RED.

    cd upstream/sub2api/frontend
    pnpm vitest run src/components/admin/account-monitor/AccountModelDetectionHistoryPanel.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts

- [ ] Step 3: Extend TypeScript API types and client. Add the new run fields and history query/response types; keep old calls valid by defaulting to limit 25.
- [ ] Step 4: Implement the panel. Use existing dialog/overlay conventions; desktop uses a 640px right drawer with table rows and inline dual-evidence expansion, narrow screens use a full-screen timeline. Show profile/mode/reason/sample counts and the not-a-routing-probability explanation only in the details area.
- [ ] Step 5: Replace the card recent-detail click target. Keep one-line current status on the card and route the click to the history panel; preserve existing model selection and manual run controls.
- [ ] Step 6: Add bilingual locale keys and integrate panel state into AccountMonitorView. Load records lazily when opened, reset cursor/filter on account change, and retain the latest projection reload behavior after a manual run.
- [ ] Step 7: Run focused Vitest, typecheck and production build GREEN.

    cd upstream/sub2api/frontend
    pnpm vitest run src/components/admin/account-monitor/AccountModelDetectionHistoryPanel.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
    pnpm typecheck
    pnpm build

- [ ] Step 8: Commit the panel slice.

    git add upstream/sub2api/frontend/src/components/admin/account-monitor/AccountModelDetectionHistoryPanel.vue upstream/sub2api/frontend/src/api/admin/accountMonitor.ts upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts upstream/sub2api/frontend/src/components/admin/account-monitor/AccountModelDetectionHistoryPanel.spec.ts upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts
    git commit -m "feat: add account detector history panel"

### Task 4: Candidate verification, integration and release

**Files:**
- Modify: docs/project/project-progress.md only from the root release controller
- Modify: docs/project/native-sub-task-package-queue.md only from the root release controller
- Create: docs/handoffs/2026-08-26-t70-account-monitor-detector-v411-handoff.md

- [ ] Step 1: Run candidate-wide direct checks from the T70 worktree.

    cd upstream/sub2api/backend
    go test ./internal/service ./internal/repository ./internal/handler/admin ./cmd/model-detector ./migrations -run 'Detection|AccountMonitor|ModelDetection' -count=1
    go build ./cmd/server
    cd ../../frontend
    pnpm vitest run src/components/admin/account-monitor/AccountModelDetectionHistoryPanel.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
    pnpm typecheck
    pnpm build
    cd ../..
    gofmt -w upstream/sub2api/backend/internal/service/account_model_detection*.go upstream/sub2api/backend/internal/repository/account_model_detection_repo.go upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go upstream/sub2api/backend/cmd/model-detector/*.go
    git diff --check

- [ ] Step 2: Review the candidate diff and write the handoff. Include baseline main SHA, commits, files, tests, migration hash/change, configuration change (none expected), downtime_required expectation, rollback and unverified items.
- [ ] Step 3: Refresh candidate against latest main before merge. Resolve only T70 conflicts, rerun direct checks, and confirm root worktree is clean.
- [ ] Step 4: Merge the candidate into root main only after the root release controller authorizes it. Run post-merge direct checks, migration validation and the existing local/host release precheck.
- [ ] Step 5: Push and deploy from verified main. If precheck returns downtime_required=false, continue the existing blue-green chain; if true, pause before stop/migration/restart and request authorization.
- [ ] Step 6: Verify production. Confirm public /healthz, /readyz, /health; authenticated admin monitor API exposes new history fields; detector unconfigured/unavailable semantics remain truthful; the history panel loads records and narrow layout has no horizontal overflow.
- [ ] Step 7: Update the root ledger to DONE only after push, deployment and online verification. Retain release evidence and clean the T70 worktree only after confirming no uncommitted or unarchived content remains.

