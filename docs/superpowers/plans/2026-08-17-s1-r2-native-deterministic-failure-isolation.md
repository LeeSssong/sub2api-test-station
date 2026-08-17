# S1-R2 Native Deterministic Failure Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Map deterministic upstream failures and incomplete OpenAI SSE streams into Sub2API's existing account, account-model, and transient scheduling state without adding a second scheduler veto.

**Architecture:** A pure classifier returns a bounded deterministic decision. `RateLimitService` projects account-scoped decisions into existing `temp_unschedulable` or `SetError`, projects model-scoped decisions into `model_rate_limits`, and the existing SSE response paths record missing successful terminal events in the existing account-model transient state. Ownership/audit metadata remains inside native reason payloads and never participates independently in scheduling.

**Tech Stack:** Go 1.26, Gin service layer, PostgreSQL-backed account repository, JSONB account `extra`, existing scheduler outbox/snapshot, Go `testing` + testify.

## Global Constraints

- Baseline is `main@a00fdb186b9598c0ab0ca747d9dff1a5cea04ae2`.
- Do not modify `docs/project/project-progress.md` or `docs/project/native-sub-task-package-queue.md`.
- Do not merge `main`, push, run release preflight, deploy, access production, or start S2/S3.
- Balance isolation defaults to 90 minutes and accepts only 60–120 minutes.
- Generic 403, network failures, and empty/truncated/incomplete model catalogs never create deterministic hard isolation.
- Preserve transient cooldown, half-open, sticky, scheduler outbox, proxy stream circuit, stream billing drain, and billing idempotency.
- No migration; do not use migration 225 or 226.
- Run only directly related tests, necessary compile/build checks, and `git diff --check`.

---

### Task 1: Deterministic classifier and bounded reason payload

**Files:**
- Create: `upstream/sub2api/backend/internal/service/deterministic_failure_isolation.go`
- Create: `upstream/sub2api/backend/internal/service/deterministic_failure_isolation_test.go`
- Modify: `upstream/sub2api/backend/internal/config/config.go`
- Test: `upstream/sub2api/backend/internal/config/config_test.go`

**Interfaces:**
- Produces: `DeterministicFailureDecision`, `classifyDeterministicUpstreamFailure(account *Account, statusCode int, responseBody []byte, requestedModel string) DeterministicFailureDecision`, `buildDeterministicFailureReason(decision, message, now) string`, and `deterministicBalanceIsolationDuration(cfg *config.Config) time.Duration`.
- Classification enums: `balance_exhausted`, `credential_invalid`, `model_unsupported`; scopes `account`, `account_model`; policies `expires`, `probe_required`.

- [x] Write table-driven failing classifier tests proving explicit balance codes/messages classify account-wide, explicit model-not-found classifies canonical mapped model, API-key 401 classifies credential invalid, and generic 403/network-like/empty catalog evidence stays unclassified.
- [x] Run `go test ./internal/service -run 'TestClassifyDeterministicUpstreamFailure|TestBuildDeterministicFailureReason' -count=1` and confirm failures are caused by missing production symbols.
- [x] Implement the pure classifier and bounded sanitized JSON reason. Use exact machine-code paths first and a small case-insensitive allowlist; never classify solely from status 403.
- [x] Add `RateLimitConfig.BalanceExhaustedIsolationMinutes` with Viper default `90` and duration validation returning 90 for values outside 60–120.
- [x] Add failing then passing config tests for 60, 90, 120, 59, and 121.
- [x] Run focused service/config tests and commit with `feat: classify deterministic upstream failures`.

### Task 2: Native model `probe_required` scheduling semantics

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/model_rate_limit.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_repo.go`
- Modify: `upstream/sub2api/backend/internal/service/account_service.go`
- Test: `upstream/sub2api/backend/internal/service/model_rate_limit_test.go`
- Test: `upstream/sub2api/backend/internal/repository/account_repo_model_availability_test.go`

**Interfaces:**
- Produces: `SetModelRateLimitWithPolicy(ctx, id, scope, probeAfter, reason, recoveryPolicy)` on the account repository interface and implementation.
- Existing `SetModelRateLimit` remains unchanged for ordinary expiring limits.

- [x] Write a failing real `Account` test where a past `rate_limit_reset_at` plus `recovery_policy=probe_required` still makes `IsSchedulableForModelWithContext` false, while an ordinary past limit expires.
- [x] Run the focused model-rate-limit test and confirm the new probe-required case fails because current parsing only compares reset time.
- [x] Implement policy-aware parsing: `probe_required` remains active until the entry is removed; ordinary payloads retain time-based expiry.
- [x] Reuse the existing repository reason writer so `recovery_policy` persists without a new interface or migration; verify focused model-rate-limit and existing repository-backed model-not-found contracts.
- [x] Implement policy-aware parsing while preserving scheduler outbox and snapshot sync through the existing writer.
- [x] Run focused service/repository tests and commit with `feat: preserve probe-required model isolation`.

### Task 3: Project deterministic failures into native state

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/ratelimit_service.go`
- Test: `upstream/sub2api/backend/internal/service/ratelimit_service_deterministic_isolation_test.go`
- Modify: existing test repository stubs only where compilation requires the new interface method.

**Interfaces:**
- Consumes Task 1 classifier/reason and Task 2 policy-aware writer.
- Produces: `handleDeterministicUpstreamFailure(ctx, account, statusCode, responseBody, requestedModel) (handled bool, shouldDisable bool)` called before generic 401/402/403 handling.

- [x] Write failing behavior tests proving explicit balance evidence writes account `temp_unschedulable` for configured 90 minutes, explicit model unsupported writes only canonical model `probe_required`, API-key 401 writes native error, and generic 403 produces none of those writes.
- [x] Run `go test -tags unit ./internal/service -run 'TestRateLimitService_Deterministic' -count=1` and confirm the failures describe current 402/error, 403, or 30-minute behavior.
- [x] Implement the minimal early deterministic branch. Keep OAuth refresh-capable first 401 on the existing refresh-window path; only confirmed credential-invalid cases reach `SetError`.
- [x] Preserve current-request failover even when persistence fails; never widen a failed model write into account-wide isolation.
- [x] Run the focused deterministic tests plus existing `ratelimit_service_401`, `model_not_found`, and clear/recovery tests.
- [x] Commit with `feat: project deterministic failures into native scheduling state`.

### Task 4: Route incomplete HTTP SSE into account-model transient cooldown

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_response_handling.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_passthrough.go`
- Test: `upstream/sub2api/backend/internal/service/openai_gateway_service_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_gateway_passthrough_flush_test.go`

**Interfaces:**
- Produces: `recordOpenAIIncompleteStreamFailure(ctx, account, canonicalModel, outputStarted, safeToReplay, hasSideEffect, usageKnown) OpenAIAccountModelRuntimeDecision`.
- Consumes existing `RecordOpenAIAccountModelFailure`; error type is `transient_stream_disconnected_before_completion`, status code `0`.

- [x] Add failing tests for missing terminal before output and after output. Assert the real runtime snapshot failure streak/cooldown changes; assert only the pre-output safe request returns the existing failover error, while post-output never becomes replayable.
- [x] Run the two focused SSE tests and confirm current code records only proxy circuit/failover and does not update account-model transient state.
- [x] Implement one helper and call it from both raw passthrough and transformed response missing-terminal/read-error paths, excluding client cancellation/deadline and successful terminal responses.
- [x] Keep proxy-ID circuit calls and billing drain unchanged.
- [x] Run focused SSE, transient, failover, billing/usage tests and commit with `feat: cool account models on incomplete SSE streams`.

### Task 5: Direct verification, self-review, and handoff

**Files:**
- Create: `docs/superpowers/reports/2026-08-17-s1-r2-native-deterministic-failure-isolation-verification.md`
- Create: `docs/handoffs/2026-08-17-s1-r2-native-deterministic-failure-isolation-handoff.md`
- Modify: this plan to check completed steps.

**Interfaces:**
- Handoff reports baseline SHA, candidate SHA, changed files, test evidence, unverified items, migration/config changes, `downtime_required=unverified`, rollback, and remaining risk.

- [x] Run only the directly related service/repository/config tests named in Tasks 1–4.
- [x] Run compile-only checks for affected backend packages and the server build; do not run the full repository suite.
- [x] Confirm migration file set is unchanged from `a00fdb186` and `.github/workflows` has no delta.
- [x] Run `gofmt` on changed Go files, `git diff --check`, inspect `git diff --stat` and `git diff a00fdb186...HEAD` for scope creep.
- [x] Write the verification report and handoff with exact commands/results and remaining risks.
- [x] Read and apply `verification-before-completion`, commit the final docs, verify a clean worktree, and stop at `READY_FOR_ROOT_REVIEW`.

## Plan Self-Review

- Spec coverage: classifier, 90-minute balance, confirmed credentials, canonical model probe-required, incomplete SSE transient/failover, native-only eligibility, recovery compatibility, and minimal verification each map to a task.
- No migration is introduced; 225/226 remain untouched.
- Type consistency: Task 3 consumes the exact classifier and repository policy method produced by Tasks 1–2; Task 4 only consumes the existing account-model transient API.
- No unresolved product decision remains. The root-controller delegated contract approves inline execution in this task; stop only for scope/security/data/production changes.
