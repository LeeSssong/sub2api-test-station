# 上游故障的缓存感知调度与流式恢复实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 OpenAI Responses/Chat Completions 的上游临时故障统一接入“缓存优先、故障递进”的账号-模型状态机，使流式断流不再重复命中坏账号，同时保证安全重试、客户继续、半开恢复和账务幂等。

**Architecture:** 保留现有调度器、`failedAccountIDs` 请求级排除和 `openAIAccountModelTransientState` 内存状态，在其上增加统一的故障事件/决策接口。请求先保持 sticky；首个首字节前且可安全重放的临时错误只允许一次同账号短重试，后续才排除并记录 cooldown；首字节后永不静默重放，客户端继续由调度器避开失败账号-模型。所有尝试共享一个 logical request ID，usage 通过现有 `usage_billing_dedup` 等价机制幂等入账；当普通候选全被 cooldown 过滤时只发放一个 half-open probe。

**Tech Stack:** Go 1.x、Gin、现有 `OpenAIGatewayService`/账号调度器、SSE 转发器、PostgreSQL usage billing repository、Redis/运行时内存状态、现有本地/主机蓝绿发布脚本。

## Global Constraints

- transient 状态键必须是 `(account_id, canonical_scheduling_model)`；canonical model 只能在账号模型映射后计算一次。
- 首次安全重放的同账号短重试等待 300–1000ms，并带少量抖动；同一账号在当前 logical request 内最多一次。
- 首字节后发生错误时不得把完整请求静默重放到另一个账号；必须保留已输出内容并发送可恢复错误事件。
- 第二次连续临时失败进入 10 秒 cooldown，第三次及以后进入 45 秒 cooldown；失败窗口为 1 分钟。
- 401、402、403、明确模型不存在、明确余额/权限错误继续沿用持久化 hard-disable/不可调度逻辑，不走 transient retry。
- all-cooldown 场景不得清空全部状态；每个账号-模型同一时间只允许一个 half-open probe。
- 自动重试不得重复客户扣费、工具调用或其他外部副作用；未知副作用请求只允许客户显式继续。
- 生产发布不得使用 GitHub Actions，继续使用现有 `ops/publish-sub2api-candidate.sh`、`ops/deploy-sub2api-blue-green-host.sh`、`ops/smoke-sub2api-release.sh` 等受控链。
- 实施开始前必须在 `docs/project/project-progress.md` 登记本事项为“进行中”；只有“已推送到服务端 + 已部署 + 已验证生效”才能标记“已完成”。

---

## 文件与边界地图

- Modify `upstream/sub2api/backend/internal/service/openai_account_model_transient.go`: 扩展现有账号-模型 transient entry、冷却决策、half-open lease 和可观测快照。
- Modify `upstream/sub2api/backend/internal/service/openai_account_runtime_block_fastpath.go`: 暴露统一故障分类/记录入口，并保持 hard-disable 与 transient cooldown 的边界。
- Modify `upstream/sub2api/backend/internal/service/openai_gateway_upstream_errors.go`: 统一“首字节前/后、可重放/不可重放、是否含副作用”的故障分类输入。
- Modify `upstream/sub2api/backend/internal/service/gateway_forward.go` and the Responses/Chat forward helpers: 在流式读取失败时返回完整的 `UpstreamFailoverError` 元数据，包括 `OutputStarted`、部分 usage、response ID 和 retryability。
- Modify `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`: 将 Responses 与 Messages 两条循环接入统一 retry/failover policy、当前请求账号排除和恢复 SSE。
- Modify `upstream/sub2api/backend/internal/service/gateway_usage_billing.go`, `upstream/sub2api/backend/internal/service/openai_gateway_record_usage.go`, and `upstream/sub2api/backend/internal/repository/usage_billing_repo.go`: 传递 logical request/attempt 元数据，确保 usage/billing 幂等与未知 usage reconciliation 标记。
- Modify `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`, `upstream/sub2api/backend/internal/service/openai_account_model_transient_test.go`, `openai_account_runtime_transient_test.go`, `gateway_forward_partial_usage_test.go`, `openai_gateway_record_usage_test.go`, and add focused integration tests beside them.
- Modify the existing admin account-monitoring service/handler DTO files found by `rg -n "AccountMonitor|account-monitor" upstream/sub2api/backend/internal/{handler,service}` to expose account-model runtime state and manual recover/probe actions; add frontend files only where the current account monitor already renders runtime scheduling state.
- Modify `docs/project/project-progress.md`, add a rollout runbook under `docs/runbooks/`, and use existing `ops/` scripts for staging/production verification.

## Task 1: 统一账号-模型 transient 状态与 half-open API

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_model_transient.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_runtime_block_fastpath.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_model_transient_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_runtime_transient_test.go`

**Interfaces:**
- Produces `type OpenAIAccountModelFailureEvent struct { AccountID int64; CanonicalModel string; StatusCode int; ErrorType string; OutputStarted bool; SafeToReplay bool; HasSideEffect bool; UsageKnown bool; Now time.Time }`.
- Produces `type OpenAIAccountModelRuntimeDecision struct { FailureStreak int; Cooldown time.Duration; BlockUntil time.Time; CurrentRequestRetry bool; ExcludeFromRequest bool; HalfOpenProbe bool; RetryAfterSeconds int }`.
- Produces `func (s *OpenAIGatewayService) RecordOpenAIAccountModelFailure(ctx context.Context, event OpenAIAccountModelFailureEvent) OpenAIAccountModelRuntimeDecision`, `func (s *OpenAIGatewayService) AcquireOpenAIAccountModelHalfOpenProbe(accountID int64, canonicalModel string, now time.Time) bool`, `func (s *OpenAIGatewayService) ReleaseOpenAIAccountModelHalfOpenProbe(accountID int64, canonicalModel string, success bool, now time.Time)`, and `func (s *OpenAIGatewayService) SnapshotOpenAIAccountModelRuntime(now time.Time) []OpenAIAccountModelRuntimeSnapshot`.
- Consumes the existing `recordFailure`, `recordSuccess`, `isBlocked`, `canonicalOpenAIAccountSchedulingModel`, and `ReportOpenAIAccountScheduleResult` behavior.

- [ ] **Step 1: Write failing state-machine tests.** Add table-driven tests for first failure (no future block, current request exclusion), second failure (10s block), third failure (45s block), one-minute window reset, account/model isolation, success reset, half-open single lease, half-open success clear, and half-open failure extension. Assert the exact `OpenAIAccountModelRuntimeDecision` fields.

- [ ] **Step 2: Run the focused tests and verify failure.**

  Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'TestOpenAIAccountModelTransient|TestOpenAIAccountRuntimeTransient' -count=1`

  Expected: FAIL because the new event/decision/half-open APIs do not exist.

- [ ] **Step 3: Implement the minimal state extension.** Keep the existing 1-minute/10-second/45-second constants. Add a per-entry `halfOpenInFlight bool` and expose only the methods listed above. `RecordOpenAIAccountModelFailure` must always set `ExcludeFromRequest=true`, set `CurrentRequestRetry=true` only when `!OutputStarted && SafeToReplay && !HasSideEffect && FailureStreak==1`, and set `Cooldown` only for streak 2+; hard errors must return `ExcludeFromRequest=true` without mutating transient state. `Acquire...` must atomically return false when `blockUntil` is in the future or another probe is active, and true only for the first probe after expiry/all-cooldown selection.

- [ ] **Step 4: Run focused tests and race the state.**

  Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'TestOpenAIAccountModelTransient|TestOpenAIAccountRuntimeTransient' -race -count=1`

  Expected: PASS with no race report.

- [ ] **Step 5: Commit.**

  ```bash
  git add upstream/sub2api/backend/internal/service/openai_account_model_transient.go upstream/sub2api/backend/internal/service/openai_account_runtime_block_fastpath.go upstream/sub2api/backend/internal/service/openai_account_model_transient_test.go upstream/sub2api/backend/internal/service/openai_account_runtime_transient_test.go
  git commit -m "feat: add cache-aware account-model runtime decisions"
  ```

## Task 2: 统一上游错误分类并接入流式断流

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_upstream_errors.go`
- Modify: `upstream/sub2api/backend/internal/service/gateway_forward.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go` (the file containing `handleFailoverExhausted` and `ensureForwardErrorResponse`)
- Modify: `upstream/sub2api/backend/internal/handler/stream_error_event.go` (the shared SSE error-event writer)
- Test: `upstream/sub2api/backend/internal/service/gateway_forward_partial_usage_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_upstream_transport_error_test.go`
- Test: `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`

**Interfaces:**
- Produces `func ClassifyOpenAIUpstreamFailure(statusCode int, upstreamMessage string, responseBody []byte, outputStarted bool, requestHasSideEffects bool) OpenAIUpstreamFailureClass`.
- Produces `func NewRetryableOpenAIStreamError(retryAfter time.Duration, responseID string, usageKnown bool) error` and a serializable recovery payload with `type`, `message`, `retryable`, `resume_supported`, and `retry_after_seconds`.
- Extends `UpstreamFailoverError` with `OutputStarted`, `SafeToFailoverAfterWrite`, `ResponseID`, `UsageKnown`, `LogicalRequestID`, and `AttemptID` while preserving existing callers.

- [ ] **Step 1: Write failing forwarder/handler tests.** Cover (a) pre-output 502 classified safe-to-replay, (b) pre-output 401/403 classified hard and not retryable, (c) post-output reader EOF/connection reset carrying `OutputStarted=true`, response ID and partial usage, (d) post-output failure never returning a failover permission, and (e) serialized SSE error event matching the exact JSON contract from the spec.

- [ ] **Step 2: Run the focused tests and verify failure.**

  Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/handler -run 'Test.*(Upstream|Stream|Forward|Failover)' -count=1`

  Expected: FAIL on missing metadata/classifier and the old generic `Upstream request failed` payload.

- [ ] **Step 3: Implement classification and propagation.** Reuse `isOpenAITransientProcessingError` and `shouldCooldownOpenAITransientUpstreamError`; classify 502/503/504/520–524/connect/timeout as transient only when output is not committed, and classify reader errors after a valid SSE event as post-output. Preserve partial usage and response ID parsed by the forwarder. Do not change hard-disable decisions for 401/402/403/model-not-found/permission errors.

- [ ] **Step 4: Implement recovery SSE writing.** Add one helper that writes exactly one JSON error event after the already-forwarded stream, sets `retryable=true`, `resume_supported` only when a usable response ID exists, and uses 10 seconds as the default `retry_after_seconds` for account-model cooldown. Ensure the helper is a no-op for client disconnects and never writes a second fallback response.

- [ ] **Step 5: Run focused tests and commit.**

  Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/handler -run 'Test.*(Upstream|Stream|Forward|Failover)' -count=1`

  ```bash
  git add upstream/sub2api/backend/internal/service/openai_gateway_upstream_errors.go upstream/sub2api/backend/internal/service/gateway_forward.go upstream/sub2api/backend/internal/handler/openai_gateway_handler.go upstream/sub2api/backend/internal/handler/stream_error_event.go upstream/sub2api/backend/internal/service/gateway_forward_partial_usage_test.go upstream/sub2api/backend/internal/service/openai_upstream_transport_error_test.go upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go
  git commit -m "feat: propagate recoverable stream upstream failures"
  ```

## Task 3: 在 Responses 与 Messages 调度循环中落实缓存感知重试/排除

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_upstream_errors.go`
- Test: `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go`

**Interfaces:**
- Consumes `OpenAIAccountModelFailureEvent`, `OpenAIAccountModelRuntimeDecision`, `ClassifyOpenAIUpstreamFailure`, and the extended `UpstreamFailoverError` from Tasks 1–2.
- Produces `type OpenAIRequestAttemptMetadata struct { LogicalRequestID string; AttemptID string; AttemptNumber int; AccountID int64; CanonicalModel string; CachePreservationMode string; OutputStarted bool; UsageProduced bool }` and context helpers `WithOpenAIRequestAttemptMetadata` / `OpenAIRequestAttemptMetadataFromContext`.
- Produces `func (h *GatewayHandler) decideOpenAIRetry(...) OpenAIRetryDecision` used by both Responses and Messages loops; `OpenAIRetryDecision` must distinguish same-account retry, failover, terminal recovery error, and no retry.

- [ ] **Step 1: Write failing handler/scheduler tests.** Add tests for sticky healthy selection, first safe failure causing one 300–1000ms same-account retry, second failure excluding the account and selecting another, post-output failure never replaying, unsafe tool request returning retryable error for explicit continue, and `cache_preservation_mode` values `sticky`, `same_account_retry`, `failover_after_failure`, `half_open_probe`.

- [ ] **Step 2: Run focused tests and verify failure.**

  Run: `cd upstream/sub2api/backend && go test ./internal/handler ./internal/service -run 'Test(OpenAI.*(Retry|Failover|Sticky|Scheduler)|.*Cache.*Preservation)' -count=1`

  Expected: FAIL because current non-`UpstreamFailoverError` stream path only calls `ReportOpenAIAccountScheduleResult(false)` and does not add the failed account to `failedAccountIDs`.

- [ ] **Step 3: Add stable logical/attempt metadata.** Generate one logical request ID at handler entry from the client request ID when valid, otherwise a cryptographically random ID; increment attempt IDs for every upstream call. Store metadata in request context and copy the immutable values into each `OpenAIForwardResult`/error.

- [ ] **Step 4: Replace duplicated retry branches with the shared policy.** For both Responses and Messages: (1) call `RecordOpenAIAccountModelFailure` on every classified failure, including post-output stream failures; (2) always add the failed account ID to `failedAccountIDs` before any `continue`; (3) allow only one same-account retry when the decision says so and no output/side effect exists; (4) otherwise fail over only before output; (5) after output write the recovery SSE and return; (6) keep sticky session binding on a single soft failure and clear/mark it only after retry failure or cooldown.

- [ ] **Step 5: Add cooldown filtering and decision logging.** Pass the existing `failedAccountIDs` map unchanged to `SelectAccountWithSchedulerForCapability`; make the scheduler log/filter `runtime_blocked` with canonical model and emit `openai.account_model_cooldown_skipped_for_cache` when a sticky candidate is skipped. Include `attempt_number`, `account_id`, `canonical_scheduling_model`, `cache_preservation_mode`, `output_started`, and `usage_produced` in request logs.

- [ ] **Step 6: Run focused tests, then commit.**

  Run: `cd upstream/sub2api/backend && go test ./internal/handler ./internal/service -run 'Test(OpenAI.*(Retry|Failover|Sticky|Scheduler)|.*Cache.*Preservation)' -count=1`

  ```bash
  git add upstream/sub2api/backend/internal/handler/openai_gateway_handler.go upstream/sub2api/backend/internal/service/openai_account_scheduler.go upstream/sub2api/backend/internal/service/openai_gateway_upstream_errors.go upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go
  git commit -m "feat: apply cache-aware retry and failover policy"
  ```

## Task 4: 防止自动重试造成重复扣费或外部副作用

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/gateway_usage_billing.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_usage.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_billing_repo.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_billing_repo_unit_test.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_billing_repo_integration_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_gateway_record_usage_test.go`

**Interfaces:**
- Consumes `OpenAIRequestAttemptMetadata` from Task 3.
- Produces billing inputs with `LogicalRequestID`, `AttemptID`, `UsageCompleteness` (`complete`, `partial`, `unknown`), and `ReconciliationRequired bool`.
- Preserves the existing uniqueness contract in `usage_billing_dedup`; if schema changes are necessary, add a forward-only migration and extend `migrations_schema_integration_test.go`.

- [ ] **Step 1: Write failing billing tests.** Assert that two attempts with the same logical request and same usage fingerprint apply one customer charge; a 502/connect failure with no usage applies zero extra charge; partial usage writes one usage row marked partial and creates reconciliation evidence; and two retries of the same reconciliation job remain idempotent.

- [ ] **Step 2: Run focused tests and verify failure.**

  Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/repository -run 'Test(OpenAIGatewayServiceRecordUsage|UsageBilling).*Dedup|.*Reconciliation' -count=1`

  Expected: FAIL because billing inputs do not yet carry logical/attempt IDs and usage completeness.

- [ ] **Step 3: Thread metadata through usage recording.** Extend `RecordUsageInput` and `recordUsageCoreInput`; derive a stable fallback logical ID only once at handler entry; make `buildRecordUsageLog` and `applyUsageBilling` use `logical_request_id + request_fingerprint` as the idempotency boundary while retaining attempt-level audit fields.

- [ ] **Step 4: Add partial/unknown usage handling.** When `ForwardResult` contains partial usage, persist the observed token counts and `reconciliation_required=true`; when no usage was observed for a transient connect/502 failure, skip customer billing and write only the failure audit event. Never replay tools or external side effects automatically; mark such attempts unsafe in the audit record.

- [ ] **Step 5: Run unit and repository integration tests, then commit.**

  Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/repository -run 'Test(OpenAIGatewayServiceRecordUsage|UsageBilling).*Dedup|.*Reconciliation' -count=1`

  ```bash
  git add upstream/sub2api/backend/internal/service/gateway_usage_billing.go upstream/sub2api/backend/internal/service/openai_gateway_usage.go upstream/sub2api/backend/internal/repository/usage_billing_repo.go upstream/sub2api/backend/internal/repository/usage_billing_repo_unit_test.go upstream/sub2api/backend/internal/repository/usage_billing_repo_integration_test.go upstream/sub2api/backend/internal/repository/migrations_schema_integration_test.go upstream/sub2api/backend/internal/service/openai_gateway_record_usage_test.go
  git commit -m "feat: make upstream retry billing idempotent"
  ```

## Task 5: 暴露客户恢复、管理员处置与观测事件

**Files:**
- Modify: the existing admin account-monitoring handler/service/DTO files located with `rg -l "AccountMonitor|account-monitor" upstream/sub2api/backend/internal/{handler,service}`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_alert_evaluator_service.go` and the existing ops event/log helper files
- Test: matching admin handler/service tests, `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`, and ops alert tests
- Add: `docs/runbooks/openai-upstream-failure-recovery.md`

**Interfaces:**
- Produces admin read DTO fields: `account_id`, `canonical_scheduling_model`, `state`, `failure_streak`, `last_failure_at`, `cooldown_until`, `half_open_in_flight`, `last_status_code`, `last_error_type`, `output_started`, and `sticky_reference_count`.
- Produces admin actions with explicit service methods `ImmediatelyCooldownAccountModel`, `RestoreAccountModelScheduling`, and `ProbeAccountModelOnce`; all actions require admin authorization and write an audit event.
- Produces event names exactly: `openai.stream_upstream_failure`, `openai.account_model_soft_failure`, `openai.account_model_cooldown_started`, `openai.account_model_cooldown_skipped_for_cache`, `openai.failover_after_stream_failure`, `openai.account_model_half_open_probe`, `openai.retry_billing_reconciled`.

- [ ] **Step 1: Write failing contract tests.** Verify the SSE JSON shape, `Retry-After`/`retry_after_seconds`, `resume_supported` behavior, admin list/action authorization, and presence of all event names/required fields.

- [ ] **Step 2: Run focused tests and verify failure.**

  Run: `cd upstream/sub2api/backend && go test ./internal/handler ./internal/service -run 'Test(OpenAI.*(Recovery|Error|Admin)|.*AccountModel.*(Admin|Event)|.*Ops.*Alert)' -count=1`

  Expected: FAIL because runtime state is not exposed and stream errors still use the generic message.

- [ ] **Step 3: Implement customer recovery contract.** Return the exact retryable error object from the spec; preserve already-emitted content; include `response_id` only when safe to continue; set `resume_supported=false` when no usable ID exists. A client “continue” request is treated as a new logical request that inherits the prior failed account-model exclusion for the response/session.

- [ ] **Step 4: Implement admin visibility and actions.** Add read-only runtime snapshot endpoint data and three guarded actions. `RestoreAccountModelScheduling` clears only the selected account-model transient state; it must not clear persistent hard-disable without using the existing account restore workflow. `ProbeAccountModelOnce` acquires the half-open lease and records success/failure.

- [ ] **Step 5: Add structured events and alert inputs.** Emit the event names with account/model, attempt, status, output/usage flags, cache mode, cooldown and retry-after fields. Add alert counters for repeated account-model failures, cooldown saturation, stream failure/failover degradation, repeated post-failure selection, and cache-hit decline correlated with failover.

- [ ] **Step 6: Write and validate the recovery runbook, then commit.** Document customer recovery (“wait for retry-after or click continue; no client restart/API key rebuild”), admin recovery actions, safety limits, and rollback. Run the focused tests and commit.

  ```bash
  git add upstream/sub2api/backend/internal/handler upstream/sub2api/backend/internal/service docs/runbooks/openai-upstream-failure-recovery.md
  git commit -m "feat: expose upstream failure recovery and observability"
  ```

## Task 6: 集成矩阵、report-only 灰度开关与回归验证

**Files:**
- Modify: existing gateway/scheduler runtime settings DTO/config files found by `rg -l "AdvancedScheduler|scheduler.*settings|SettingKey.*OpenAI" upstream/sub2api/backend/internal`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`
- Add: `upstream/sub2api/backend/internal/handler/openai_upstream_resilience_integration_test.go`
- Add: `upstream/sub2api/backend/internal/service/openai_upstream_resilience_integration_test.go`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Produces runtime settings with default `report_only=true`, `exclude_failed_account=true` after the second rollout gate, `enable_same_account_retry=true` only for safe requests, and `enable_half_open_probe=false` until the final gate.
- Consumes every API from Tasks 1–5 and does not alter model prices, group priority, or upstream channel choice.

- [ ] **Step 1: Register the project ledger entry before code/config changes.** Add a dated entry in `docs/project/project-progress.md` stating this item is “进行中”, naming the approved spec/plan and the deployment gates. Do not mark it complete during local implementation.

- [ ] **Step 2: Write failing integration tests.** Simulate: pre-output 502; post-output disconnect; same account `terra` success with `sol` failure; cache token accounting; unsafe tool call; continue selecting a healthy account; all candidates cooldown with exactly one probe; and duplicate usage reconciliation.

- [ ] **Step 3: Run the integration tests and verify failure.**

  Run: `cd upstream/sub2api/backend && go test ./internal/handler ./internal/service -run 'TestOpenAIUpstreamResilience' -count=1`

  Expected: FAIL until all preceding task contracts are wired together.

- [ ] **Step 4: Implement settings and rollout gates.** Make report-only the safe default. Gate actual request exclusion, same-account retry, cooldown filtering, and half-open probing independently so operations can enable them in the documented order without rebuilding the binary.

- [ ] **Step 5: Run the complete backend validation set.**

  ```bash
  cd upstream/sub2api/backend
  go test ./...
  go test -race ./internal/service ./internal/handler ./internal/repository
  go vet ./...
  ```

  Expected: all commands exit 0; no unrelated package is skipped without a recorded reason.

- [ ] **Step 6: Commit the integration/rollout work.**

  ```bash
  git add upstream/sub2api/backend docs/project/project-progress.md
  git commit -m "test: cover upstream resilience rollout gates"
  ```

## Task 7: 本地发布链、蓝绿部署与线上验收

**Files:**
- Modify only when required by reviewed implementation: `ops/publish-sub2api-candidate.sh`, `ops/deploy-sub2api-blue-green-host.sh`, `ops/smoke-sub2api-release.sh`, and their tests under `tests/operations/`.
- Add: `docs/superpowers/reports/YYYY-MM-DD-openai-upstream-resilience-production-verification.md`
- Modify: `docs/project/project-progress.md` only with evidence-backed status updates.

**Interfaces:**
- Consumes the final implementation commits and rollout settings from Task 6.
- Produces deployment evidence for source commit, image digest, migrations hash, active slot, container identities/restart counts, `/healthz`, `/readyz`, SSE recovery event, client continue failover, cache-token metrics, usage dedup/reconciliation, and rollback readiness.

- [ ] **Step 1: Run local release qualification.** Execute the existing tests for candidate publication, blue-green topology, deployment and smoke checks:

  ```bash
  tests/operations/publish_sub2api_candidate_test.rb
  tests/operations/deploy_sub2api_blue_green_host_test.sh
  tests/operations/release_sub2api_blue_green_test.sh
  tests/operations/smoke-sub2api-release_test.sh
  ```

- [ ] **Step 2: Publish and stage a candidate through the reviewed local/host chain.** Record source commit, immutable image digest and migration hash. Do not add or invoke GitHub Actions.

- [ ] **Step 3: Deploy report-only to one blue/green slot.** Verify health/readiness, worker health, container identities and restart counts; run synthetic pre-output and post-output failures and confirm only structured report events are emitted.

- [ ] **Step 4: Enable rollout gates in order.** Enable current-request exclusion, then safe same-account retry, then actual account-model cooldown and half-open probes. After each gate, verify cache hit/read/create tokens, average input cost, first-token latency, stream failure rate, failover success rate, and repeated-selection ratio.

- [ ] **Step 5: Verify customer/admin recovery and billing.** Use a real or isolated test API key to trigger stream failure, click/issue continue, assert a different healthy account-model is selected without client restart, and reconcile the resulting usage exactly once. Verify admin list, cooldown, restore and one-shot probe actions.

- [ ] **Step 6: Record evidence, rollback if any acceptance criterion fails, and update the ledger.** Only after push + deployment + online verification may the ledger state become “已完成”; otherwise keep it “进行中” and list the blocking evidence.

## Self-review checklist

- Spec coverage: Tasks 1–2 cover the failure state machine and stream path; Task 3 covers cache-aware scheduling and client continue; Task 4 covers logical/attempt billing and partial usage; Task 5 covers customer/admin experience and events; Tasks 6–7 cover integration, staged rollout, production verification and rollback.
- Placeholder scan: the plan contains no `TODO`, `TBD`, “implement later”, or unspecified “appropriate handling” steps; every task names files, symbols, tests, commands and commit intent.
- Type consistency: Task 1 defines `OpenAIAccountModelFailureEvent` and `OpenAIAccountModelRuntimeDecision`; Task 2 extends `UpstreamFailoverError`; Task 3 consumes both and defines `OpenAIRequestAttemptMetadata`; Task 4 consumes that metadata; Tasks 5–7 consume the resulting recovery/admin/event contracts.
