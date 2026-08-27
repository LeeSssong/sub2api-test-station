# T82 调度器健康隔离与故障切换优化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变账号持久化 `active` 状态的前提下，补齐质量门禁、分级隔离、半开恢复、安全重试切换和调度可观测性。

**Architecture:** 复用现有 `openAIAccountRuntimeStats`、`openAIAccountModelTransientState`、`RateLimitService`、上游 billing probe 和 handler failover。通过默认质量策略、账号+模型运行时状态机、请求级排除集以及结构化事件把低质量账号从当前请求和后续窗口中安全移开。

**Tech Stack:** Go、现有 Sub2API service/handler、Go test、gofmt。

**Spec:** `docs/superpowers/specs/2026-08-27-t82-scheduler-health-failover-design.md`

## Global Constraints

- 不新增数据库迁移、平行控制面、计费事实源或 GitHub Actions。
- 账号持久化 `status` 保持 `active`；临时质量/余额/鉴权问题只进入运行时隔离或 `temp_unschedulable`。
- 只有安全重放才允许自动重试或跨账号切换；已输出、usage/副作用或幂等性不明时必须阻止切换。
- 本轮只做本地直接相关测试和构建；不改主站配置、不部署主站。

### Task 1: 默认启用质量门禁并固定 sticky 逃逸排除

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler_quality_gate.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler_projection.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_quality_gate_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go`

**Interfaces:**
- `qualityGatePolicyForGroup(ctx, groupID)` returns normalized default policy when no explicit override exists, while honoring explicit `Enabled:false`.
- Scheduler selection receives a request-local excluded account set after sticky escape and never immediately reselects the escaped account.

- [ ] **Step 1: Write failing tests** for an unconfigured group using default thresholds and for sticky escape excluding the original account from the same logical request.
- [ ] **Step 2: Run focused tests** with `go test ./internal/service -run 'TestOpenAI.*QualityGate|TestOpenAI.*Sticky'` and confirm failure.
- [ ] **Step 3: Implement default policy resolution and request-local exclusion** without changing persistent account status.
- [ ] **Step 4: Re-run focused tests** and confirm pass.
- [ ] **Step 5: Run scheduler package tests** and `gofmt` on modified files.

### Task 2: 分级 502/503 cooldown 与半开滞回

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_model_transient.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_runtime_block_fastpath.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_model_transient_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_half_open_round5_test.go`

**Interfaces:**
- `RecordOpenAIAccountModelFailure` computes windowed failure counts and cooldowns: 2/60s→60s, 3/5m→5m, 5/15m→30m.
- `AcquireOpenAIAccountModelHalfOpenProbe` grants one lease; `Release...` requires two successful observations before clearing the breaker.

- [ ] **Step 1: Add failing tests** for each escalation window, single half-open lease, and two-success recovery.
- [ ] **Step 2: Run focused transient tests** and confirm failure.
- [ ] **Step 3: Implement bounded failure timestamps/counters and hysteresis recovery.** Preserve existing safe replay guard.
- [ ] **Step 4: Re-run transient and half-open tests** and confirm pass.
- [ ] **Step 5: Run `go test ./internal/service -run 'TestOpenAI.*Transient|TestOpenAI.*HalfOpen'` and format files.

### Task 3: 余额不足改为 probe_required 并自动恢复

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/ratelimit_service.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_billing_probe.go`
- Test: `upstream/sub2api/backend/internal/service/ratelimit_service_deterministic_isolation_test.go`
- Test: `upstream/sub2api/backend/internal/service/upstream_billing_probe_test.go`

**Interfaces:**
- Deterministic balance failure persists `recovery_policy:"probe_required"` and keeps account `active`.
- Successful billing probe invokes existing recovery path; two consecutive successful probes clear temp isolation/runtime block.

- [ ] **Step 1: Add failing tests** asserting no fixed 90-minute expiry and automatic recovery after two successful probes.
- [ ] **Step 2: Run the two focused test files** and confirm failure.
- [ ] **Step 3: Implement probe-required persistence and recovery callback using existing repository/cache APIs.**
- [ ] **Step 4: Re-run focused tests** and confirm pass.
- [ ] **Step 5: Run `go test ./internal/service -run 'Test.*(DeterministicIsolation|UpstreamBillingProbe)'`.

### Task 4: 401 OAuth refresh 与 API Key 探活恢复

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/ratelimit_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/token_refresh_service.go` (only integration hook if required)
- Test: `upstream/sub2api/backend/internal/service/ratelimit_service_test.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- OAuth 401 invalidates cache, attempts refresh, then enters half-open on success.
- API Key 401 remains runtime-isolated and is probed every 5 minutes; three consecutive probe failures increase interval.

- [ ] **Step 1: Add failing tests** for OAuth refresh success/failure and API Key probe cadence/backoff.
- [ ] **Step 2: Run focused tests** and confirm failure.
- [ ] **Step 3: Integrate existing token refresh/account monitor probe results with runtime recovery state.** Do not call `SetError` for recoverable 401.
- [ ] **Step 4: Re-run focused tests** and confirm pass.
- [ ] **Step 5: Run relevant service tests and format files.

### Task 5: 安全重试与最多两次跨账号切换

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/failover_loop.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_retry_budget.go`
- Test: `upstream/sub2api/backend/internal/handler/failover_loop_test.go`
- Test: `upstream/sub2api/backend/internal/handler/openai_retry_budget_test.go`
- Test: `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`

**Interfaces:**
- Request-local retry budget allows same-account retry once and cross-account switch at most twice for safe pre-output 502/503.
- Unsafe replay logs a stable `switch_block_reason`; output/usage/side-effect paths never switch.

- [ ] **Step 1: Add failing tests** for safe retry budget, two-account cap, and unsafe output/side-effect blocking.
- [ ] **Step 2: Run focused handler tests** and confirm failure.
- [ ] **Step 3: Implement explicit budget counters and preserve existing stream safety checks.**
- [ ] **Step 4: Re-run focused tests** and confirm pass.
- [ ] **Step 5: Run failover and retry package tests; format files.

### Task 6: 调度候选/排除/切换结构化可观测性

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler_projection.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_quality_gate_test.go`
- Test: `upstream/sub2api/backend/internal/handler/openai_gateway_compact_log_test.go`

**Interfaces:**
- Scheduler/handler events include `candidate_account_ids`, `excluded_account_ids`, `exclude_reasons`, `final_account_id`, `health_state`, `switch_allowed`, `switch_block_reason`, and `cooldown_until`.

- [ ] **Step 1: Add failing log assertions** for candidate/exclusion/final-account fields and cooldown deadline.
- [ ] **Step 2: Run focused logging tests** and confirm failure.
- [ ] **Step 3: Add fields at existing decision log sites without logging secrets or request bodies.**
- [ ] **Step 4: Re-run logging tests** and confirm pass.
- [ ] **Step 5: Run all directly related service/handler tests and format files.

### Task 7: 集成验证与交接

**Files:**
- Modify: `docs/project/project-progress.md` (root-only status update after implementation handoff)
- Create: `docs/superpowers/handoffs/2026-08-27-t82-scheduler-health-failover.md`

- [ ] **Step 1: Run direct Go tests** covering quality gate, transient breaker, probe recovery, 401 recovery, failover, and logs.
- [ ] **Step 2: Run `go build ./cmd/server` from `upstream/sub2api/backend`.
- [ ] **Step 3: Run `gofmt -w` on changed Go files and `git diff --check`.
- [ ] **Step 4: Review diff against the spec and record migration/config/deployment status.
- [ ] **Step 5: Commit the candidate branch and report `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or edit root progress ledger from the feature worktree.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-27-t82-scheduler-health-failover.md`. Execute inline in this task worktree using the same TDD checkpoints; no main merge or deployment is authorized by this plan.
