# P1 余额耗尽账号隔离与恢复：恢复官方原生状态机实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 移除余额耗尽相关的定制状态抢占与账单探针越权恢复，恢复官方 Sub2API v0.1.183 原生账号状态机，并让账号监控只读展示 effective schedulability。

**Architecture:** 保留官方 `Account.IsSchedulableAt`、官方 402/403/404/429/5xx 处理、CN provider 专用余额/额度状态机和账单探针的观测能力。删除或断开 `deterministic_failure_isolation` 的运行时决策入口，移除 billing probe 对通用 `ClearTempUnschedulable` 的调用；监控 API 在同一账号快照时间调用原生资格判断，增加只读 effective 字段，不创建第二套调度事实源。

**Tech Stack:** Go、Gin、Ent/PostgreSQL repository、Vue 3、TypeScript、Vitest、现有 Sub2API admin API 与本地/宿主发布链。

**Spec:** `docs/superpowers/specs/2026-09-01-p1-restore-native-balance-state-machine-design.md`

## Global Constraints

- 官方基线：Sub2API `v0.1.183`，commit `e8cb019fabf8b55199436229044cbf9aa7a82564`。
- 只处理 P1 余额耗尽账号隔离/恢复异常；不修改 P2-P5、通知、排名、采购、经营页、账务或其他调度策略。
- 不修改 `docs/project/project-progress.md`、`docs/project/native-sub-task-package-queue.md`、根 `main`、生产数据、SSH、部署或推送。
- 不新增数据库迁移、隔离表、恢复表、余额事实源或平行 scheduler veto。
- 保留官方账单探针本身、官方 402/403/429/5xx 和 CN provider 专用余额检查/恢复语义。
- 账单探针成功不得清除任意 `temp_unschedulable`、`error`、429、overload、quota、模型限制或人工暂停。
- 历史十年 `temp_unschedulable` 只允许只读盘点和后续授权运营处置；本计划不执行生产数据修改。
- 直接相关功能测试通过、必要编译/构建、格式检查和 `git diff --check` 是本候选完成门槛；不运行全仓压力、mutation、soak 或人为消耗生产余额。

---

## File Map

### Runtime and configuration

- Modify: `upstream/sub2api/backend/internal/service/ratelimit_service.go`，移除 `HandleUpstreamError` 的定制确定性前置分支，恢复官方 402/403/404/429/5xx 控制流。
- Delete or stop compiling: `upstream/sub2api/backend/internal/service/deterministic_failure_isolation.go`，由实现阶段先确认引用，最终不得有运行时调用或配置依赖。
- Test/modify: `upstream/sub2api/backend/internal/service/deterministic_failure_isolation_test.go`，将测试改为证明定制路径不再参与运行时，或随实现文件删除；不得保留会要求旧分类行为的测试。
- Modify: `upstream/sub2api/backend/internal/service/upstream_billing_probe.go`，移除 recovery streak 字段、初始化、成功回调和通用清理副作用，保留探针读取、快照、失败、unsupported 与已有允许同步行为。
- Test: `upstream/sub2api/backend/internal/service/upstream_billing_probe_test.go`，覆盖一次/连续成功均不清除通用临时状态，并覆盖 CN provider 专用恢复仍不被该改动触碰。
- Inspect and modify only if directly required: `upstream/sub2api/backend/internal/config/config.go`，删除仅供定制余额隔离使用的配置字段/default；保留其他 rate-limit 配置不变。
- Test: `upstream/sub2api/backend/internal/config/config_test.go`，删除旧定制配置断言并增加配置范围无残留 guard（若当前测试已有相应覆盖则复用，不重复造测试）。

### Native state and admin contracts

- Inspect/modify only when compilation or contract requires: `upstream/sub2api/backend/internal/service/account.go`、`upstream/sub2api/backend/internal/repository/account_repo.go`、`upstream/sub2api/backend/internal/handler/admin/account_handler.go`。
- Test: `upstream/sub2api/backend/internal/service/ratelimit_service_*.go`、`upstream/sub2api/backend/internal/service/model_rate_limit_test.go`、`upstream/sub2api/backend/internal/repository/account_repo_temp_unsched_test.go` and directly affected tests.

### Account monitor effective projection

- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`，增加 `EffectiveSchedulable`、`EffectiveSchedulableAt` 和受限 `EffectiveUnschedulableReason` 字段，字段只属于监控投影。
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`，在 `List`、`ListWindow` 以及实际共用的账号行构造边界捕获一次 UTC snapshot time，并调用 `Account.IsSchedulableAt`；不得改变评分、排名、探测准入或写入状态。
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go` and `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`，覆盖人工暂停、临时隔离、429/过载、过期、quota、健康账号和快照时间边界。
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`，同步新增字段类型。
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue` and `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorAccountInfoDialog.vue`，分开展示人工 `schedulable` 与 effective 状态/原因，不增加写操作。
- Test: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`、`AccountMonitorAccountInfoDialog.spec.ts`、必要的 `AccountMonitorView.spec.ts`。

### Historical read-only operations evidence

- Create: `docs/superpowers/reports/2026-09-01-p1-native-balance-state-machine-verification.md`，记录测试、范围、自审、未验证项、历史数据只读处置门禁和发布边界。
- Create only if an existing repository-safe read-only command is needed: `docs/superpowers/checklists/2026-09-01-p1-historical-temp-unschedulable-read-only-audit.md`，只记录操作步骤和字段白名单，不包含凭据、生产值或可直接写数据的命令。
- Modify no global ledger/queue files from this task worktree.

---

## Task 1: Reconfirm the official baseline and freeze the implementation surface

**Files:**
- Read: `upstream/sub2api/XINGQIAO_UPSTREAM.md`
- Read: `docs/superpowers/specs/2026-09-01-p1-restore-native-balance-state-machine-design.md`
- Read: `docs/superpowers/specs/2026-08-17-s1-r2-native-deterministic-failure-isolation-design.md`
- Read: `docs/handoffs/2026-08-17-s1-r2-native-deterministic-failure-isolation-handoff.md`
- Read: `docs/superpowers/reports/2026-08-17-s1-r2-native-deterministic-failure-isolation-verification.md`
- Inspect: all current references to `deterministic_failure_isolation`, `recordSuccessfulProbeRecovery`, `BalanceExhaustedIsolationMinutes`, `effective_schedulable`, and `ClearTempUnschedulable`.

**Interfaces:**
- Produces: an exact reference inventory and a bounded changed-file list for Tasks 2-5.
- Consumes: the approved P1 specification and official commit record.

- [ ] **Step 1: Capture current repository identity and clean-state evidence.**

  Run:

  ```bash
  git rev-parse HEAD
  git status --short --branch
  git diff --check
  ```

  Expected: HEAD is the approved spec commit `ca20ba87cf77bbc6fbe96a5c5877a803b20ec8f5`; the only task-local change is the plan while it is being written; no implementation files are modified.

- [ ] **Step 2: Build the exact runtime reference inventory.**

  Run:

  ```bash
  rg -n -F 'deterministic_failure_isolation' upstream/sub2api/backend
  rg -n -F 'recordSuccessfulProbeRecovery' upstream/sub2api/backend
  rg -n -F 'BalanceExhaustedIsolationMinutes' upstream/sub2api/backend
  rg -n -F 'ClearTempUnschedulable' upstream/sub2api/backend/internal/service/upstream_billing_probe.go
  rg -n -F 'effective_schedulable' upstream/sub2api/backend upstream/sub2api/frontend
  ```

  Expected: every production reference is assigned to Task 2, 3, or 4; no unrelated subsystem is added to the plan.

- [ ] **Step 3: Record the baseline comparison requirement.**

  Use official source commit `e8cb019fabf8b55199436229044cbf9aa7a82564` as the behavior reference during implementation. Do not copy a source file wholesale; compare the affected functions and preserve Xingqiao-specific behavior explicitly listed as retained by the spec.

- [ ] **Step 4: Commit the plan only after the plan self-review is complete.**

  ```bash
  git add docs/superpowers/plans/2026-09-01-p1-restore-native-balance-state-machine.md
  git commit -m "docs: plan native balance state machine restoration"
  ```

## Task 2: Remove deterministic failure isolation from the runtime error path

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/ratelimit_service.go`
- Delete or modify: `upstream/sub2api/backend/internal/service/deterministic_failure_isolation.go`
- Delete or modify: `upstream/sub2api/backend/internal/service/deterministic_failure_isolation_test.go`
- Modify: `upstream/sub2api/backend/internal/config/config.go`
- Modify: `upstream/sub2api/backend/internal/config/config_test.go`
- Test: directly affected `ratelimit_service` 401/403/model-not-found/deterministic tests.

**Interfaces:**
- Consumes: existing official `HandleUpstreamError`, `handle403`, `handle429`, `HandleUpstreamModelNotFound`, `SetTempUnschedulable`, `SetError`, and model availability APIs.
- Produces: no `deterministic_failure_isolation` runtime decision; native error handling remains the only general path for 402/403/404/429/5xx.

- [ ] **Step 1: Write RED regression tests proving the custom branch is no longer authoritative.**

  Add focused cases to the existing service tests:

  ```go
  func TestRateLimitService_HandleUpstreamError_DoesNotUseDeterministicBalanceClassifier(t *testing.T) {
      // Arrange an OpenAI API-key account and a response containing
      // insufficient_user_quota / balance_exhausted text.
      // Assert the repository receives only the official branch's writes and
      // never a reason whose source is deterministic_failure_isolation.
  }
  ```

  Also cover generic 403, 404/model-not-found, 429 and 5xx so the test asserts delegation to existing native handlers rather than merely absence of one string.

- [ ] **Step 2: Run the RED tests.**

  ```bash
  go test ./internal/service -run 'TestRateLimitService_HandleUpstreamError_DoesNotUseDeterministic|TestRateLimitService_HandleUpstreamError_(403|ModelNotFound)|TestRateLimitService.*429' -count=1
  ```

  Expected: the new tests fail against the current pre-removal branch because `HandleUpstreamError` invokes `handleDeterministicUpstreamFailure` before native handling.

- [ ] **Step 3: Remove the deterministic preemption with the smallest control-flow change.**

  Delete the `handleDeterministicUpstreamFailure` invocation from `HandleUpstreamError`, then remove the now-unreferenced classifier and its dedicated configuration. If another current caller exists, remove that caller first and delete the file only after `rg` proves no production reference remains. Do not modify the native 402/403/404/429/5xx branches except where compilation requires it.

- [ ] **Step 4: Remove obsolete configuration without changing unrelated rate-limit defaults.**

  Delete `RateLimitConfig.BalanceExhaustedIsolationMinutes` and its Viper default only if no retained code uses it. Run a repository search for the exact field and key; the expected result is no runtime/config reference outside historical docs.

- [ ] **Step 5: Run the focused native error regression.**

  ```bash
  go test ./internal/service -run 'TestRateLimitService_HandleUpstreamError|TestHandleUpstreamModelNotFound|Test.*OpenAI403|Test.*OpenAI429|Test.*Deterministic' -count=1
  go test ./internal/config -count=1
  ```

  Expected: native error tests pass; old classifier tests are removed or rewritten to assert non-use; no test expects a generic 402/403/404 to create the custom reason.

- [ ] **Step 6: Commit the runtime restoration.**

  ```bash
  git add upstream/sub2api/backend/internal/service upstream/sub2api/backend/internal/config
  git commit -m "fix: restore native upstream account error handling"
  ```

## Task 3: Remove billing probe recovery authority while retaining billing observation

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/upstream_billing_probe.go`
- Test: `upstream/sub2api/backend/internal/service/upstream_billing_probe_test.go`
- Test: `upstream/sub2api/backend/internal/service/admin_account_upstream_billing_probe_test.go`
- Test: directly affected repository CAS/snapshot tests only if compilation requires it.

**Interfaces:**
- Consumes: existing billing probe settings, identity validation, HTTP request, snapshot persistence, failure scheduling and optional rate synchronization.
- Produces: a billing probe that never clears general account scheduling state; no recovery streak or cross-owner cleanup.

- [ ] **Step 1: Add RED lifecycle tests for one and two successful probes.**

  Extend the existing fake repository fixture with an account carrying a future `TempUnschedulableUntil` and a non-CN reason. Assert after each successful probe:

  ```go
  require.NotNil(t, repo.accounts[id].TempUnschedulableUntil)
  require.Empty(t, repo.clearTempUnschedulableCalls)
  ```

  Run the test once after adding it; it must fail because the current second success calls `ClearTempUnschedulable`.

- [ ] **Step 2: Add ownership cases.**

  Cover existing `error`, 429/rate-limit, overload, manual pause, quota and CN-provider-owned temporary states. For every non-billing state, a successful billing probe must leave the native state unchanged. Preserve existing snapshot `status`, timestamps, failure count, unsupported behavior and rate-sync assertions.

- [ ] **Step 3: Remove recovery streak state and callback.**

  Delete `recoveryMu`, `recoveryStreak`, their constructor initialization, `recordSuccessfulProbeRecovery`, and the call after snapshot persistence. Do not remove the probe snapshot writer or any balance/declaration read path.

- [ ] **Step 4: Run billing probe and CN-focused tests.**

  ```bash
  go test ./internal/service -run 'Test.*UpstreamBillingProbe|Test.*CNProvider|Test.*Balance' -count=1
  go test ./internal/repository -run 'Test.*UpstreamBillingProbe|Test.*TempUnschedulable' -count=1
  ```

  Expected: one and two successful probes do not clear account state; CN provider tests continue to pass through their own service; no probe test expects generic recovery.

- [ ] **Step 5: Commit the probe ownership fix.**

  ```bash
  git add upstream/sub2api/backend/internal/service/upstream_billing_probe.go upstream/sub2api/backend/internal/service/upstream_billing_probe_test.go upstream/sub2api/backend/internal/service/admin_account_upstream_billing_probe_test.go
  git commit -m "fix: keep billing probes observational"
  ```

## Task 4: Add read-only effective schedulability to account monitoring

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorAccountInfoDialog.vue`
- Test: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Test: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorAccountInfoDialog.spec.ts`
- Test if needed: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

**Interfaces:**
- Consumes: `Account.IsSchedulableAt(time.Time)` and the existing account monitor account snapshot.
- Produces: `effective_schedulable: boolean`, `effective_schedulable_at: time.Time`, and `effective_unschedulable_reason: string` in the monitor projection only.

- [ ] **Step 1: Add RED backend projection tests.**

  Add table-driven cases using a fixed UTC time:

  ```go
  tests := []struct {
      name   string
      mutate func(*Account)
      want   bool
      reason string
  }{
      {name: "active and manually enabled", want: true},
      {name: "manual pause", mutate: func(a *Account) { a.Schedulable = false }, reason: "manual_disabled"},
      {name: "temporary native pause", mutate: func(a *Account) { a.TempUnschedulableUntil = timePtr(fixedNow.Add(time.Hour)) }, reason: "temp_unschedulable"},
      {name: "rate limit", mutate: func(a *Account) { a.RateLimitResetAt = timePtr(fixedNow.Add(time.Hour)) }, reason: "rate_limited"},
      {name: "overload", mutate: func(a *Account) { a.OverloadUntil = timePtr(fixedNow.Add(time.Hour)) }, reason: "overload"},
      {name: "expired", mutate: func(a *Account) { a.AutoPauseOnExpired = true; a.ExpiresAt = timePtr(fixedNow) }, reason: "expired"},
  }
  ```

  Assert raw `Schedulable` remains unchanged, `EffectiveSchedulable` equals `IsSchedulableAt(fixedNow)`, timestamp equals the response snapshot, and the reason is a display mapping only.

- [ ] **Step 2: Run the RED monitor tests.**

  ```bash
  go test ./internal/service ./internal/handler/admin -run 'Test.*AccountMonitor.*Schedul|Test.*AccountMonitor.*Effective' -count=1
  ```

  Expected: failure because the projection has no effective fields and currently copies only raw `Schedulable`.

- [ ] **Step 3: Implement one shared native projection helper.**

  Add a small pure helper in the account monitor service package:

  ```go
  func projectEffectiveSchedulability(account *Account, snapshotAt time.Time) (bool, string)
  ```

  It must call `account.IsSchedulableAt(snapshotAt)` and choose the first matching display reason in the same order as the native gates: inactive, manual disabled, expired, overload, rate limited, temp unschedulable, quota exceeded, otherwise empty. It must not inspect billing probe status and must not mutate `account`.

- [ ] **Step 4: Populate both full-site and window monitor rows.**

  Capture one UTC `snapshotAt` per response before row construction. Populate the three fields in every `AccountMonitorAccount` row from the helper. Do not change monitor score, rank, sample, probe admission, scheduler projection, or account repository writes.

- [ ] **Step 5: Add frontend contract and display tests.**

  Extend the TypeScript interface and fixtures with the new fields. In the card and info dialog, render raw schedulable and effective schedulability separately. Assert these cases:

  - raw true/effective false displays the native blocking reason;
  - raw false displays manual pause and does not label it automatic balance isolation;
  - billing `status=ok` does not override effective false;
  - existing normal/abnormal/score/timeline content remains rendered.

- [ ] **Step 6: Run backend and frontend focused tests.**

  ```bash
  go test ./internal/service ./internal/handler/admin -run 'Test.*AccountMonitor.*(Schedul|Effective|Handler)' -count=1
  ./node_modules/.bin/vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorAccountInfoDialog.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
  ./node_modules/.bin/vue-tsc --noEmit
  ```

  Expected: all direct monitor tests pass and TypeScript has no contract errors.

- [ ] **Step 7: Commit the read-only monitor contract.**

  ```bash
  git add upstream/sub2api/backend/internal/service/account_monitor_types.go upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/service/account_monitor_service_test.go upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go upstream/sub2api/frontend/src/api/admin/accountMonitor.ts upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorAccountInfoDialog.vue upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorAccountInfoDialog.spec.ts upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts
  git commit -m "feat: expose native effective account schedulability"
  ```

## Task 5: Define historical ten-year isolation audit and operational authorization gates

**Files:**
- Create: `docs/superpowers/reports/2026-09-01-p1-native-balance-state-machine-verification.md`
- Create if needed: `docs/superpowers/checklists/2026-09-01-p1-historical-temp-unschedulable-read-only-audit.md`
- Do not modify: `docs/project/project-progress.md`, `docs/project/native-sub-task-package-queue.md`

**Interfaces:**
- Consumes: native account read APIs, existing admin recovery semantics, and the approved P1 spec.
- Produces: a secret-free, read-only audit contract and evidence location; no production write command.

- [ ] **Step 1: Define the read-only field allowlist in the report.**

  Include only account ID, redacted name/reference, platform/type, status, raw schedulable flag, temp-until timestamp, bounded reason classification, updated-at, latest native error/probe timestamps, and evidence timestamps. Explicitly exclude credentials, API keys, full upstream bodies, passwords, tokens, cookies and webhook secrets.

- [ ] **Step 2: Define ownership buckets and authorization gates.**

  The report must distinguish manual pause, CN-owned pause, official 401/429/overload/quota/model state, legacy deterministic reason, and unknown reason. Require a written authorization naming account IDs, batch size, recovery endpoint, pre/post snapshots, stop conditions and rollback evidence before any recovery action.

- [ ] **Step 3: Add a no-write operational checklist.**

  The checklist must say: inspect only; do not call `POST /schedulable`, `DELETE /temp-unschedulable`, `recover-state`, or direct SQL; do not infer health from billing `ok`; do not open manual scheduling; do not export secrets. It must direct any future authorized action to the official admin endpoint and root-controller release gates.

- [ ] **Step 4: Record that no production audit was executed in this worktree.**

  The report must state `production_data_changed=false`, `ssh_accessed=false`, `deployment_executed=false`, and `downtime_required=unverified` for this candidate.

- [ ] **Step 5: Commit the evidence contract.**

  ```bash
  git add docs/superpowers/reports/2026-09-01-p1-native-balance-state-machine-verification.md docs/superpowers/checklists/2026-09-01-p1-historical-temp-unschedulable-read-only-audit.md
  git commit -m "docs: define native balance recovery audit gates"
  ```

## Task 6: Direct verification, scope review, and handoff

**Files:**
- Modify: `docs/superpowers/reports/2026-09-01-p1-native-balance-state-machine-verification.md`
- Create: `docs/handoffs/2026-09-01-p1-native-balance-state-machine-handoff.md`
- Do not modify: global queue, progress ledger, root `main`, production state records.

**Interfaces:**
- Consumes: completed runtime, probe, monitor and audit-gate changes from Tasks 2-5.
- Produces: `READY_FOR_ROOT_REVIEW` handoff with exact baseline/candidate SHA, changed files, direct tests, migration/config status, release boundary, rollback and residual risk.

- [ ] **Step 1: Run direct backend verification.**

  ```bash
  go test ./internal/service -run 'TestRateLimitService_HandleUpstreamError|Test.*UpstreamBillingProbe|Test.*CNProvider|Test.*AccountMonitor.*(Schedul|Effective)' -count=1
  go test ./internal/repository -run 'Test.*UpstreamBillingProbe|Test.*TempUnschedulable' -count=1
  go test ./internal/config -count=1
  go test ./internal/service ./internal/handler/admin ./internal/repository -run '^$' -count=1
  go build -o /tmp/sub2api-p1-native-balance-server ./cmd/server
  ```

  Expected: direct tests, compile-only checks and server build pass. Do not run the full repository suite.

- [ ] **Step 2: Run direct frontend verification.**

  ```bash
  ./node_modules/.bin/vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorAccountInfoDialog.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
  ./node_modules/.bin/vue-tsc --noEmit
  ./node_modules/.bin/vite build
  ```

  Expected: listed monitor tests pass, typecheck passes and production build succeeds.

- [ ] **Step 3: Run scope and safety gates.**

  ```bash
  git diff --check
  git diff --name-only ca20ba87cf77bbc6fbe96a5c5877a803b20ec8f5...HEAD -- upstream/sub2api/backend/migrations
  git diff --name-only ca20ba87cf77bbc6fbe96a5c5877a803b20ec8f5...HEAD -- .github/workflows
  rg -n -F 'deterministic_failure_isolation' upstream/sub2api/backend
  rg -n -F 'recordSuccessfulProbeRecovery' upstream/sub2api/backend
  rg -n -F 'BalanceExhaustedIsolationMinutes' upstream/sub2api/backend
  ```

  Expected: no migration/workflow changes; no production runtime reference to the removed deterministic classifier or billing recovery callback; no secrets or unrelated task files in the diff. If the candidate is refreshed before this step, replace the left-hand comparison SHA with the actual candidate starting SHA in the report.

- [ ] **Step 4: Complete plan/spec self-review.**

  Check each approved spec section: evidence, native baseline, goals/non-goals, affected accounts, three options, control flow, API/UI contract, failure/recovery, compatibility, historical data gates, acceptance matrix, tests, release/rollback, unresolved decisions and approval record. Search the plan and implementation diff for unfinished-task markers and ambiguous “later” instructions; fix documentation gaps before handoff.

- [ ] **Step 5: Write the handoff.**

  Include:

  - task identifier and approved spec path;
  - baseline `main` SHA and candidate SHA;
  - changed files and whether runtime/config/frontend/docs changed;
  - direct test commands and results;
  - migration/workflow/config status;
  - `production_data_changed=false`, `deployment_executed=false`, `downtime_required=unverified` until root preflight;
  - rollback as root-main revert plus existing blue-green rollback, with no direct database overwrite;
  - historical ten-year isolation remains untouched unless separately authorized;
  - residual risks, especially lack of production replay and any official-source delta requiring root review.

- [ ] **Step 6: Commit the handoff and stop at `READY_FOR_ROOT_REVIEW`.**

  ```bash
  git add docs/superpowers/reports/2026-09-01-p1-native-balance-state-machine-verification.md docs/handoffs/2026-09-01-p1-native-balance-state-machine-handoff.md
  git commit -m "docs: hand off native balance state machine restoration"
  git status --short --branch
  ```

  Expected: clean task worktree, no merge/push/deploy, and handoff explicitly says `READY_FOR_ROOT_REVIEW` rather than `DONE`.

## Plan Self-Review

- **Spec coverage:** Tasks 2-3 restore native error and probe ownership; Task 4 implements the effective monitor projection; Task 5 covers historical data gates; Task 6 covers direct tests, scope, release boundary and rollback. All spec sections map to at least one task.
- **Concrete steps:** All code-facing steps name files, symbols, commands, expected outcomes and commit boundaries. Comparison commands use the current approved spec commit as their concrete baseline; a refreshed candidate must record its actual starting SHA before execution.
- **Type consistency:** `projectEffectiveSchedulability(*Account, time.Time) (bool, string)` is defined in Task 4 and its return values populate the three monitor fields defined in the same task. The raw `schedulable` field remains unchanged.
- **Ownership consistency:** Billing probe success updates billing evidence only. CN provider recovery remains in its existing owner service. Generic account recovery remains native/admin-owned.
- **Migration consistency:** No task creates or deletes a migration; no task modifies production data or global release ledgers.
- **Execution gate:** This plan is written only after the user approved the formal specification. Implementation still requires the project’s task/worktree and root release gates; the plan itself does not authorize merge, push or deployment.
