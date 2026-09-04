# T130 Stream Capacity Feedback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make OpenAI streaming capacity failures affect subsequent account selection while preserving no-replay billing safety, and feed long no-first-output client cancellations into the existing W1 quality score as right-censored evidence.

**Architecture:** Extend the existing upstream failure classifier and account-model transient state. The handler will independently record current-request replay decisions and future-request health feedback. The streaming service will emit one idempotent slow-output evidence event only for upstream-dispatched attempts that pass the 60-second threshold and end by client cancellation; the existing T114 in-process quality projection consumes that event without creating a new persistence source.

**Tech Stack:** Go 1.27, Gin, existing OpenAI gateway service, in-process transient state, existing Redis shared-health store, existing resilience event ledger and scheduler decision log.

**Spec:** `docs/superpowers/specs/2026-09-04-t130-stream-capacity-feedback-design.md`

## Global Constraints

- Do not modify root `main`, push, merge, deploy, restart services, or change production/test-station data.
- Preserve the existing output/usage/side-effect replay safety boundary.
- Do not add database migrations, new public APIs, new control planes, admission limits, or persistent quality facts.
- Capacity feedback is scoped to `(account_id, canonical_model)` and must remain bounded, idempotent, and compatible with T82/T114/T126.
- Client cancellation is not an upstream failure unless the documented 60-second post-dispatch slow-output conditions all hold.

### Task 1: Classify Capacity Failures and Preserve Failure Windows

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_upstream_errors.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_model_transient.go`
- Test: `upstream/sub2api/backend/internal/service/openai_gateway_upstream_errors_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_model_transient_test.go`

**Interfaces:**
- Produce `FailureClass` fields that identify `upstream_capacity_pressure` and a bounded subtype for handler consumption.
- Preserve `OpenAIAccountModelFailureEvent` and `OpenAIAccountModelRuntimeDecision` public-in-package shapes unless a backward-compatible field is required.

- [ ] **Step 1: Write failing classifier tests** for pending queue, account concurrency, rate limit, and temporary unavailable messages with `status_code=0` or 429/503; add negative tests for model-not-found, client cancellation, auth, and arbitrary user text.
- [ ] **Step 2: Run the focused classifier tests** with `go test ./internal/service -run 'Test(ClassifyOpenAI|OpenAI.*Capacity)' -count=1`; confirm the new capacity assertions fail because the classifier has no capacity subtype.
- [ ] **Step 3: Implement bounded capacity classification** using structured fields first and case-insensitive boundary-safe message matching only when structured fields are absent. Keep existing transient/error-owner classification intact.
- [ ] **Step 4: Write failing transient-state tests** proving a `status_code=0` capacity failure with `output_started=true` gets a 10-second future-request cooldown, and a normal success does not erase recent failure timestamps. Add natural-window expiry and half-open recovery assertions.
- [ ] **Step 5: Run the transient tests** and confirm the new assertions fail against the current `RecordOpenAIAccountModelFailure` and `RecordOpenAIAccountModelSuccess` behavior.
- [ ] **Step 6: Implement the minimal state change**: add a capacity-pressure/immediate-feedback signal to the failure event, apply the existing short cooldown independently of replay safety, retain recent window timestamps across ordinary successes, and keep half-open success hysteresis.
- [ ] **Step 7: Run focused service tests** and `gofmt` on changed Go files; confirm all classifier/transient tests pass.
- [ ] **Step 8: Commit** with `git add upstream/sub2api/backend/internal/service/... && git commit -m "fix: retain stream capacity feedback"`.

### Task 2: Feed Handler Decisions Into Future-Request Health

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_resilience_observability.go`
- Test: `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_resilience_observability_test.go`

**Interfaces:**
- Consume `OpenAIUpstreamFailureClass` capacity classification from Task 1.
- Produce resilience events with separate current/future actions, capacity subtype, and cooldown values while preserving existing event names and durable scheduler-log compatibility.

- [ ] **Step 1: Write failing handler tests** for an already-output `response.failed` capacity error: assert no current failover/replay, but assert `RecordOpenAIAccountModelFailure` receives future-feedback metadata and starts cooldown.
- [ ] **Step 2: Run the handler tests** with the focused `go test` command and verify they fail because status-0 capacity failures are not marked for immediate feedback.
- [ ] **Step 3: Implement handler wiring** so capacity classification sets the future-feedback flag without changing `unsafeToReplay`, retry budget, output handling, or current-request switch rules.
- [ ] **Step 4: Add resilience event assertions** for `failure_class`, `capacity_subtype`, `current_request_action`, `future_request_action`, and `shared_feedback_written` using the existing bounded event ledger.
- [ ] **Step 5: Run focused handler/service tests** and inspect event snapshots for absence of request bodies, credentials, or upstream response bodies.
- [ ] **Step 6: Commit** with `git add upstream/sub2api/backend/internal/handler upstream/sub2api/backend/internal/service && git commit -m "fix: wire capacity feedback into scheduling"`.

### Task 3: Record Slow First-Output Client Abandonment

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_response_handling.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_passthrough.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_resilience_observability.go`
- Test: `upstream/sub2api/backend/internal/service/openai_gateway_response_flush_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_compact_sse_keepalive_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_resilience_observability_test.go`

**Interfaces:**
- Consume existing stream lifecycle state: upstream dispatch, semantic output, terminal event, client disconnect, and elapsed time.
- Produce one `openai.first_output_slow`/`openai.client_abandoned_after_upstream_wait` event and an in-process right-censored W1 quality sample keyed by attempt ID.

- [ ] **Step 1: Write failing stream tests** for 60-second-plus no-semantic-output client cancellation, sub-60-second cancellation, real semantic output before cancellation, explicit upstream reset, and duplicate finalization.
- [ ] **Step 2: Run the focused stream tests** and confirm the long-wait case has no slow evidence today.
- [ ] **Step 3: Implement an attempt-scoped idempotent slow-evidence helper** that records only after upstream dispatch, the 60-second threshold, no semantic output, and client-cancellation classification; it must not cancel or replay the upstream context.
- [ ] **Step 4: Connect both normal Responses streaming and passthrough streaming finalization** to the helper, preserving existing usage recording, terminal-event handling, and client response errors.
- [ ] **Step 5: Add/adjust W1 quality projection tests** proving the right-censored sample lowers first-output score, is not counted as a failure, and is replaced by real TTFT when the same attempt later completes.
- [ ] **Step 6: Run focused stream/quality tests** and inspect resilience events for bounded IDs and no payload leakage.
- [ ] **Step 7: Commit** with `git add upstream/sub2api/backend/internal/service && git commit -m "fix: observe slow stream abandonment"`.

### Task 4: Shared Health, Scheduler Guard, and Regression Verification

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_model_transient.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_scheduler_log_sink.go` only if existing insert projection needs the new bounded fields
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_shared_health_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_adaptive_test.go`
- Test: `upstream/sub2api/backend/internal/repository/openai_shared_health_test.go`

**Interfaces:**
- Consume future-request feedback from Tasks 1-3.
- Produce consistent local/shared cooldown eligibility and ensure T126 cannot re-add blocked accounts.

- [ ] **Step 1: Write failing shared-health tests** for concurrent capacity failures, Redis write failure fallback, and T126 candidate filtering during cooldown.
- [ ] **Step 2: Run those focused tests** and confirm shared state or candidate filtering does not yet cover the new signal.
- [ ] **Step 3: Implement atomic shared feedback propagation** using existing Redis/event-ID primitives; keep local state active when Redis is unavailable and mark degraded observability.
- [ ] **Step 4: Ensure adaptive candidate-pool code only reads post-gate candidates** and never reintroduces a runtime-blocked account; do not change score weights or Top-K rules.
- [ ] **Step 5: Run the complete direct verification set:** focused service/handler/repository tests, `go build ./cmd/server`, `gofmt -w` followed by `git diff --check`.
- [ ] **Step 6: Inspect `git diff`, `git status`, and changed-file scope** for no migrations, config changes, secrets, production data, or GitHub Actions.
- [ ] **Step 7: Commit** with `git add upstream/sub2api/backend/internal && git commit -m "test: verify stream capacity feedback"`.

### Task 5: Handoff Without Main or Deployment Changes

**Files:**
- Modify: `docs/handoffs/2026-09-04-t130-stream-capacity-feedback-handoff.md`

- [ ] **Step 1: Record** baseline `main` SHA, candidate branch/worktree, commit list, changed files, tests, migration/config/data/secrets assessment, `downtime_required` expectation, rollback, and unverified items.
- [ ] **Step 2: Verify** root `main` remains untouched, candidate is not deployed, and no remote environment was accessed for writes.
- [ ] **Step 3: Commit** the handoff document on the T130 branch.

