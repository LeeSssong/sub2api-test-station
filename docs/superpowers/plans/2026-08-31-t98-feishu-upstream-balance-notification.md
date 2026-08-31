# T98 飞书上游余额通知重构实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Sub2API 原生 worker 中按规范化 BaseURL 聚合现有有效余额快照，发送 P1/P2 飞书余额通知，并永久禁用旧 relay-ops 通知业务路径，同时保留飞书传输凭据与卡片样式能力。

**Architecture:** 新增窄接口的余额通知域服务，读取 `AccountMonitorService` 已有账号/余额/排名投影；以 `ops_alert_events` 保存仅含非敏感状态的 BaseURL 活动事件，使用数据库唯一约束与 advisory lock 做 claim/CAS；飞书 sender 从受保护文件加载凭据和 BaseURL 登记簿，在发送前重新读取当前数据并内存渲染卡片。relay-ops 先移除旧通知启动和迁移重放，再加入显式 retire/清理门禁，旧表清理只在受控发布阶段执行。

**Tech Stack:** Go 1.27, PostgreSQL, `database/sql`, existing Sub2API worker lifecycle, Feishu tenant OpenAPI, Go `encoding/json`, existing Vue-free backend card JSON tests, sqlmock.

**Spec:** `docs/superpowers/specs/2026-08-31-t98-feishu-upstream-balance-notification-design.md`

## Global Constraints

- 只消费已有账号、分组、`scheduler_rank` 和 `account_monitor_balance`，不得新增余额 HTTP 请求、探测、事实源或业务写入。
- 仅未删除、`status=active`、`platform=openai`、`type=api_key` 且 BaseURL 可规范化的账号参与。
- 当前余额只能是同一规范化 BaseURL 下 `status=ok`、`version=1`、合法 `value_usd` 快照按 `observed_at` 最新值；无活跃账号、无有效值、读取失败或同时间冲突不改变状态。
- `0 < value_usd < 5` 为 P2 橙色、标题“上游账号余额不足”、30 分钟；`value_usd = 0` 为 P1 红色、标题“上游账号余额为 0”、5 分钟并 `@` 现有接收人但不加急。
- 卡片只在内存显示一次余额/BaseURL/登记簿账号/明文密码，并列出全部活跃账号及原生 `scheduler_rank`；事件表、日志、错误、trace、API 和测试固定值不得含密码。
- 新 sender 不经过 relay-ops；旧 writer、策略、jobs、表和 migration 重放必须移除。规格批准不构成主站部署授权。

---

### Task 1: 建立可测试的 BaseURL 与快照聚合域

**Files:**
- Create: `upstream/sub2api/backend/internal/service/upstream_balance_notification.go`
- Create: `upstream/sub2api/backend/internal/service/upstream_balance_notification_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_balance.go`

**Interfaces:**
- Consumes: `Account`, `AccountMonitorBalance`, `AccountMonitorAccountRepository.ListAllWithFilters`, `AccountMonitorService.resolveBalance` semantics.
- Produces: `NormalizeNotificationBaseURL(string) (string, error)`, `EvaluateUpstreamBaseURLBalance([]UpstreamBalanceAccount) ([]UpstreamBalanceEvaluation, error)` and internal `validNotificationSnapshot` used by repository and sender tasks.

- [ ] **Step 1: Write failing tests** for URL normalization (trim trailing slash, lowercase host, reject non-HTTP(S)/userinfo/query/fragment), eligibility filtering, latest `observed_at` selection, failed snapshot rejection, equal-time conflicting values, and low/zero/healthy boundaries.
- [ ] **Step 2: Run the focused service test**:
  `cd upstream/sub2api/backend && go test -run 'Test(NormalizeNotificationBaseURL|EvaluateUpstreamBaseURLBalance)' ./internal/service`
  Expected: FAIL because the new functions do not exist.
- [ ] **Step 3: Implement minimal pure functions** with hand-derived status values and no database or HTTP calls; preserve failed snapshots as ineligible even when they retain old values.
- [ ] **Step 4: Re-run the focused tests** and confirm all boundary cases pass.
- [ ] **Step 5: Commit** `feat: add baseurl balance notification evaluation`.

### Task 2: Load protected Feishu and login-registry secrets

**Files:**
- Create: `upstream/sub2api/backend/internal/notify/upstream_balance_secrets.go`
- Create: `upstream/sub2api/backend/internal/notify/upstream_balance_secrets_test.go`

**Interfaces:**
- Consumes: host paths for app ID, app secret, chat ID, recipient OpenIDs, and converted registry JSON.
- Produces: `LoadUpstreamBalanceSecrets(UpstreamBalanceSecretPaths) (UpstreamBalanceSecrets, error)` and `LoginRegistry.Lookup(normalizedBaseURL) (loginAccount, loginPassword string, ok bool)`.

- [ ] **Step 1: Write failing tests** for exact `0600` file/`0700` parent checks, symlink rejection, 1 MiB registry limit, `DisallowUnknownFields`, trailing JSON rejection, duplicate normalized BaseURL rejection, missing whole-file fail-closed, and per-BaseURL “未登记”. Tests use only values such as `fake-app-id` and `registry-user.invalid`.
- [ ] **Step 2: Run** `go test -run 'TestLoadUpstreamBalanceSecrets|TestLoginRegistry' ./internal/notify` and verify expected FAIL.
- [ ] **Step 3: Implement secure file checks, bounded decoding, normalization and zeroing of temporary byte buffers; do not log file content.
- [ ] **Step 4: Re-run tests** and verify malformed files fail without preventing unrelated service construction.
- [ ] **Step 5: Commit** `feat: add protected upstream balance secret loader`.

### Task 3: Render and send P1/P2 cards with existing Feishu transport contracts

**Files:**
- Create: `upstream/sub2api/backend/internal/notify/upstream_balance_card.go`
- Create: `upstream/sub2api/backend/internal/notify/upstream_balance_card_test.go`
- Create: `upstream/sub2api/backend/internal/notify/feishu_client.go`
- Create: `upstream/sub2api/backend/internal/notify/feishu_client_test.go`

**Interfaces:**
- Consumes: `UpstreamBalanceEvaluation`, `LoginRegistry`, existing wide-screen card conventions, `http.Client`.
- Produces: `RenderUpstreamBalanceCard(UpstreamBalanceCardInput) ([]byte, error)`, `FeishuSender.Send(context.Context, UpstreamBalanceCardInput) (messageID string, error)`.

- [ ] **Step 1: Write failing tests** asserting P2 orange/no mention/title, P1 red/recipient mention/no urgent call, one balance and credential block, all active accounts/ranks in stable order, “未登记”/“未排名”, exact 30 KiB rejection, and no API key or raw snapshot fields.
- [ ] **Step 2: Run** `go test -run 'Test(RenderUpstreamBalanceCard|FeishuSender)' ./internal/notify` and verify expected FAIL.
- [ ] **Step 3: Implement the card model and sender using 10-second timeout, tenant-token refresh-once behavior, HTTP/business-code validation, and no response-body propagation.
- [ ] **Step 4: Re-run tests**, including a fake HTTP transport that confirms zero calls to `urgent_app` for P1.
- [ ] **Step 5: Commit** `feat: add upstream balance feishu cards`.

### Task 4: Extend native event ledger for BaseURL-scoped claims

**Files:**
- Create: `upstream/sub2api/backend/migrations/231_upstream_baseurl_balance_notifications.sql`
- Create: `upstream/sub2api/backend/migrations/upstream_baseurl_balance_notifications_migration_test.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_alert_models.go`
- Modify: `upstream/sub2api/backend/internal/repository/ops_repo_alerts.go`
- Create: `upstream/sub2api/backend/internal/repository/upstream_balance_event_repo.go`
- Create: `upstream/sub2api/backend/internal/repository/upstream_balance_event_repo_test.go`

**Interfaces:**
- Consumes: `ops_alert_events`, `ops_alert_rules`, existing `hashAdvisoryLockID`/advisory-lock helpers.
- Produces: `UpstreamBalanceEventRepository.Claim`, `ConfirmDelivery`, `RecordFailure`, `Resolve`, with event fields limited to scope, state, timestamps, generation, lease token and non-sensitive error code.

- [ ] **Step 1: Write failing migration/repository tests** for columns, partial unique index on `(rule_id, scope_type, scope_key)` while firing, and CAS rejection when generation/token changed.
- [ ] **Step 2: Run** `go test -run 'Test(UpstreamBalanceEventMigration|UpstreamBalanceEventRepository)' ./migrations ./internal/repository` and verify expected FAIL.
- [ ] **Step 3: Add the migration and repository methods**; use one transaction for row lock/claim, `pg_try_advisory_xact_lock` for scope serialization, and never accept card JSON or credentials as repository inputs.
- [ ] **Step 4: Re-run tests** and verify two competing claims produce one active lease while different BaseURLs remain independent.
- [ ] **Step 5: Commit** `feat: add baseurl scoped native alert ledger`.

### Task 5: Implement native evaluator and due-delivery worker

**Files:**
- Create: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service.go`
- Create: `upstream/sub2api/backend/internal/service/upstream_balance_notification_service_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_runner.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/backend/cmd/server/wire_gen.go`

**Interfaces:**
- Consumes: Task 1 evaluator, Task 2 loader, Task 3 sender, Task 4 repository, existing account-monitor runner.
- Produces: `UpstreamBalanceNotificationService.Evaluate(ctx)`, `RunDue(ctx)`, `Stop()`; runner hook invoked after an existing balance refresh and a due-only worker tick that never refreshes balances.

- [ ] **Step 1: Write failing service tests** for healthy/low/zero transitions, 30/5-minute cadence from successful delivery time, retry backoff `1,2,5,10m`, no state mutation on no active account/invalid snapshot/read error, current-data re-render before send, and stale-generation cancellation.
- [ ] **Step 2: Run** `go test -run 'TestUpstreamBalanceNotificationService' ./internal/service` and verify expected FAIL.
- [ ] **Step 3: Implement evaluator/worker with at-least-once semantics: claim non-sensitive lease, reacquire current data, hold scoped lock through final review/send/CAS, clear or advance generation on state change, and isolate all errors from account refresh and gateway paths.
- [ ] **Step 4: Re-run service tests** and add a concurrency test with two goroutines sharing one repository stub.
- [ ] **Step 5: Wire construction into native worker lifecycle** with sender disabled by default unless explicit runtime enablement is present; ensure cleanup stops it.
- [ ] **Step 6: Commit** `feat: wire native upstream balance notification worker`.

### Task 6: Convert protected workbook and add deployment-only secret contract

**Files:**
- Create: `tools/convert_upstream_login_registry.go`
- Create: `tools/convert_upstream_login_registry_test.go`
- Modify: `infra/compose/*.yml` (only the Sub2API worker secret mount files actually present)
- Modify: relevant `.env.example` or deployment contract files

**Interfaces:**
- Consumes: protected workbook `outputs/feishu-balance-account-map-20260830/sub2api-account-login-map-20260830.xlsx` in local operator environment.
- Produces: deterministic normalized JSON registry at a host-provided path, never committed or printed.

- [ ] **Step 1: Write failing converter tests** with an in-memory XLSX fixture for C-column BaseURL and final two credential columns, duplicate BaseURL conflict and blank credential behavior.
- [ ] **Step 2: Run converter tests** and verify expected FAIL.
- [ ] **Step 3: Implement conversion using the repository’s spreadsheet runtime/parser, exact output schema and `0600` output permission; refuse non-regular or world-readable output targets.
- [ ] **Step 4: Run converter tests** and a local conversion only to a `0700` temporary directory; verify output path and file content are not emitted to logs.
- [ ] **Step 5: Commit** `feat: add protected upstream login registry conversion` without adding the workbook or generated JSON.

### Task 7: Retire relay-ops notification runtime and migration replay

**Files:**
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/store/postgres.go`
- Modify: `relay-ops-service/internal/store/notification_consolidation.go`
- Modify: `relay-ops-service/internal/notificationpolicy/policy.go`
- Modify: `relay-ops-service/internal/notify/*.go`
- Create: `relay-ops-service/internal/store/migrations/015_retire_legacy_notifications.sql`
- Create: `relay-ops-service/internal/store/legacy_notification_retirement_test.go`
- Modify: relay-ops startup/provision wiring files found by `rg 'SupersedeLegacy|notification.*Start|go:embed migrations'`

**Interfaces:**
- Consumes: existing relay-ops app/store/notify wiring.
- Produces: a compatibility build that has no old notification writers, schedulers, policy families, retry payload writes, migration embedding or startup queries for retired tables; explicit cleanup command remains disabled unless release gate invokes it.

- [ ] **Step 1: Write failing contract tests** that start the store against a schema without retired tables and assert startup/provision performs no query/create against them; assert old notifier constructors and policy families are unreachable.
- [ ] **Step 2: Run** `go test ./internal/store ./internal/app ./internal/notify ./internal/notificationpolicy` and verify expected FAIL.
- [ ] **Step 3: Remove old notification runtime wiring and embedded migrations; add explicit ordered retire SQL for the nine authorized tables and a dry-run/count-only guard that never logs BaseURL or credentials.
- [ ] **Step 4: Re-run relay-ops tests** and assert billing/externalization/non-notification services remain constructible.
- [ ] **Step 5: Commit** `refactor: retire legacy feishu notification runtime`.

### Task 8: Cross-boundary verification and handoff

**Files:**
- Create: `docs/superpowers/reports/2026-08-31-t98-feishu-upstream-balance-notification-verification.md`
- Create: `docs/handoffs/2026-08-31-t98-feishu-upstream-balance-notification-handoff.md`

- [ ] **Step 1:** Run focused Go tests for service, notify, repository and migrations; run `go build ./cmd/server` and relay-ops build/tests.
- [ ] **Step 2:** Run `gofmt -w` on changed Go files and `git diff --check`.
- [ ] **Step 3:** Run repository-wide secret scans that exclude the protected local `outputs` directory and prove no card payload/password/API key enters code, tests, logs, event schema or API types.
- [ ] **Step 4:** Run disabled/fake-transport smoke tests proving zero real Feishu egress and zero new upstream balance HTTP requests.
- [ ] **Step 5:** Record exact commands, pass/fail counts, known baseline failures and release boundary; mark handoff `READY_FOR_ROOT_REVIEW`, with no push, production clear, deployment or real message.
- [ ] **Step 6: Commit** `docs: record t98 implementation verification and handoff`.

## Execution Notes

- The production deletion sequence is intentionally excluded from local implementation. Only the controlled release process may execute the authorized no-backup cleanup after the compatibility mirror and rollback target are fixed.
- The local candidate must not read the real workbook into logs or tests; use the protected file only for the operator conversion smoke test and keep generated JSON outside Git.
- Before claiming completion, re-read the spec and this plan, run fresh verification, and report the exact remaining gate: root review, test-station acceptance, and explicit main-site deployment authorization.
