# OpenAI Scheduling Admission Resilience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent a slow OpenAI account from receiving a burst of HTTP streaming text requests before its first semantic output across all OpenAI scheduler groups and models, while retaining the existing scheduler and failing open if shared Redis state is unavailable.

**Architecture:** The existing `OpenAISharedHealthStore` gains Lua-atomic admission leases and a short slow-session guard keyed only by `account_id`. HTTP text handlers classify already-validated raw bodies, acquire a lease after selecting an account and obtaining its local slot, and clear it at first semantic output or terminal cleanup. T80 does not add shared-quality state: group-level quality attribution and explanations remain the T76 follow-up; existing group policies can rank only candidates that have already passed this non-bypassable safety layer.

**Tech Stack:** Go, Gin, Redis/go-redis v9 Lua scripts, miniredis, testify.

**Spec:** `docs/superpowers/specs/2026-08-27-t80-openai-scheduling-admission-resilience-design.md`

## Global Constraints

- Reuse native OpenAI scheduler, shared-health store, response stream detection, and bounded resilience ledger; do not add a second routing or quality source.
- Only HTTP streaming text requests with a recognized request shape participate; non-streaming, images, `/count_tokens`, WebSocket, and unknown shapes preserve current behavior.
- Raw body classification is `normal`/`long` at `65536` bytes by default; do not parse tokens or persist raw bodies.
- Redis admission and slow-session mutations are Lua atomic and keyed by `account_id` only; long max is 1, normal max is 2, a long blocks both shapes, stalled pre-output is 30 seconds, lease TTL is 90 seconds, renewal is 25 seconds.
- Delete the previously introduced `OpenAISharedRequestQualitySnapshot`, `GetRequestQuality`, `RecordRequestQuality`, quality maps, and account-model admission paths; T80 must not add a model dimension to safety, quality, or configuration contracts.
- All OpenAI scheduler groups and models use the same safety threshold. Existing group policies influence only post-filter candidate ordering, profit, and experience tradeoffs; they cannot bypass admission or slow-session guards.
- Shared-store failures fail open, log only structured/sanitized fields, and never expose credentials or request bodies.
- No database migration, API/UI contract change, GitHub Actions, root-ledger edits, push, merge to `main`, deployment, or production access from this worktree.
- Direct validation is scoped Go tests, `go build ./cmd/server`, `gofmt`, and `git diff --check`; no full-suite, load, or soak run.

---

## File Structure

- `upstream/sub2api/backend/internal/config/config.go`: admission defaults, environment mapping, and strict validation.
- `upstream/sub2api/backend/internal/config/config_test.go`: default/env/invalid admission configuration coverage.
- `upstream/sub2api/backend/internal/service/openai_shared_health.go`: account-only request-shape, lease, and slow-session guard contracts; independent mutation context; service-side acquire/release helpers.
- `upstream/sub2api/backend/internal/service/openai_shared_health.go`: keep account-only guard authoritative in Lua admission acquire after the existing scheduler selects an account and obtains its slot; group policy never bypasses this final safety gate.
- `upstream/sub2api/backend/internal/handler/openai_chat_completions.go`: acquire/retry admission after its final account selection/local slot and compose one idempotent release chain.
- `upstream/sub2api/backend/internal/service/openai_resilience_observability.go`: bounded decision/event fields for admission and quality outcomes.
- `upstream/sub2api/backend/internal/repository/openai_shared_health.go`: account-only Redis key derivation and Lua acquire/renew/release/slow-session scripts.
- `upstream/sub2api/backend/internal/repository/openai_shared_health_test.go`: miniredis account-only behavior, expiry, fencing, and negative interface assertions proving no shared-quality API remains.
- `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`: retain classified body shape in the request context after validation.
- `upstream/sub2api/backend/internal/handler/openai_chat_completions.go`: carry shape through scheduling and attach first-output release/TTFT recording to the existing HTTP streaming path.
- `upstream/sub2api/backend/internal/service/openai_account_scheduler_shared_health_test.go` and targeted gateway/handler tests: service integration, cancellation, release, fail-open, and endpoint exclusion cases.

### Task 1: Define Configuration and Shared Contracts

**Files:**
- Modify: `upstream/sub2api/backend/internal/config/config.go`
- Modify: `upstream/sub2api/backend/internal/config/config_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_shared_health.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_shared_health_test.go`

**Interfaces:**
- Produces `OpenAIAdmissionRequestShape`, account-only `OpenAISharedAdmissionRequest`, `OpenAISharedAdmissionDecision`, and account-only slow-session guard methods exactly as specified.
- Produces global config fields `AdmissionEnabled`, `LongRequestBodyThresholdBytes`, `MaxPreFirstOutputNormal`, `MaxPreFirstOutputLong`, `StalledBeforeFirstOutputSeconds`, `AdmissionLeaseTTLSeconds`, `AdmissionRenewSeconds`, `SlowTTFTMS`, and `SlowSessionGuardSeconds`.

- [ ] **Step 1: Write failing config and request-shape tests**

```go
func TestLoadDefaultOpenAISharedHealthAdmissionConfig(t *testing.T) {
    cfg, err := Load()
    require.NoError(t, err)
    require.True(t, cfg.Gateway.OpenAISharedHealth.AdmissionEnabled)
    require.Equal(t, 65536, cfg.Gateway.OpenAISharedHealth.LongRequestBodyThresholdBytes)
    require.Equal(t, 1, cfg.Gateway.OpenAISharedHealth.MaxPreFirstOutputLong)
}

func TestClassifyOpenAIAdmissionRequestShape(t *testing.T) {
    require.Equal(t, OpenAIAdmissionShapeNormal, ClassifyOpenAIAdmissionRequestShape(65536, 65536))
    require.Equal(t, OpenAIAdmissionShapeLong, ClassifyOpenAIAdmissionRequestShape(65537, 65536))
}
```

- [ ] **Step 2: Run the focused tests and verify they fail**

Run: `go test ./internal/config ./internal/service -run 'TestLoadDefaultOpenAISharedHealthAdmissionConfig|TestClassifyOpenAIAdmissionRequestShape' -count=1`

Expected: FAIL because admission fields and classifier do not exist.

- [ ] **Step 3: Add the strict defaults, validation, and contracts**

```go
type OpenAIAdmissionRequestShape string

const (
    OpenAIAdmissionShapeUnknown OpenAIAdmissionRequestShape = "unknown"
    OpenAIAdmissionShapeNormal  OpenAIAdmissionRequestShape = "normal"
    OpenAIAdmissionShapeLong    OpenAIAdmissionRequestShape = "long"
)

func ClassifyOpenAIAdmissionRequestShape(bodyBytes, threshold int) OpenAIAdmissionRequestShape {
    if bodyBytes < 0 || threshold <= 0 { return OpenAIAdmissionShapeUnknown }
    if bodyBytes > threshold { return OpenAIAdmissionShapeLong }
    return OpenAIAdmissionShapeNormal
}
```

Validate ranges from the specification and require renewal to be at least 5 seconds and below lease TTL.

- [ ] **Step 4: Run focused tests and format**

Run: `gofmt -w internal/config/config.go internal/config/config_test.go internal/service/openai_shared_health.go internal/service/openai_account_scheduler_shared_health_test.go && go test ./internal/config ./internal/service -run 'TestLoadDefaultOpenAISharedHealthAdmissionConfig|TestClassifyOpenAIAdmissionRequestShape' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the contracts**

```bash
git add upstream/sub2api/backend/internal/config/config.go upstream/sub2api/backend/internal/config/config_test.go upstream/sub2api/backend/internal/service/openai_shared_health.go upstream/sub2api/backend/internal/service/openai_account_scheduler_shared_health_test.go
git commit -m "feat: define OpenAI admission resilience contracts"
```

### Task 2: Replace Account-Model Contracts With Account-Only Redis Admission and Guard

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/openai_shared_health.go`
- Modify: `upstream/sub2api/backend/internal/repository/openai_shared_health_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_shared_health.go`

**Interfaces:**
- Extends `OpenAISharedHealthStore` with account-only `AcquireAdmission`, `RenewAdmission`, `ReleaseAdmission`, `RecordSlowSessionGuard`, and `HasSlowSessionGuard`.
- `OpenAISharedAdmissionRequest` contains only `AccountID`, `LeaseID`, `Shape`, and `ObservedAt`; it has no `OpenAISharedHealthKey`, canonical model, group, or request-body field.

- [ ] **Step 1: Write failing miniredis tests**

```go
func TestOpenAISharedHealthAdmissionLongBlocksAcrossModelsAndGroups(t *testing.T) {
    long := service.OpenAISharedAdmissionRequest{AccountID: 153, LeaseID: "lease-a", Shape: service.OpenAIAdmissionShapeLong, ObservedAt: now}
    require.True(t, store.AcquireAdmission(ctx, long).Allowed)
    normal := service.OpenAISharedAdmissionRequest{AccountID: 153, LeaseID: "lease-b", Shape: service.OpenAIAdmissionShapeNormal, ObservedAt: now}
    got, err := store.AcquireAdmission(ctx, normal)
    require.NoError(t, err)
    require.False(t, got.Allowed)
    require.Equal(t, "long_pre_first_output", got.Reason)
}
```

Also cover concurrent long acquisition (one winner), normal capacity two, stalled rejection, lease-owner-only release, renewal, expiry cleanup, account-only slow-session guard, and an interface assertion that the store does not implement `GetRequestQuality` or `RecordRequestQuality`. Use separate handler requests with different requested models and group IDs to prove the same account lease blocks both.

- [ ] **Step 2: Run repository tests and verify they fail**

Run: `go test ./internal/repository -run 'TestOpenAISharedHealthAdmission|TestOpenAISharedHealthSlowSession|TestOpenAISharedHealthStoreHasNoQualityAPI' -count=1`

Expected: FAIL because the new store methods and Lua scripts do not exist.

- [ ] **Step 3: Add Redis Lua scripts and decoding helpers**

Use a hash admission key namespace under the existing versioned prefix whose name contains account ID only. In one Lua call, remove expired leases, count shape entries, reject a stale entry older than configured duration, reject any normal/long request while a long entry exists, enforce shape capacity, and add only the hash-safe lease identifier with `PEXPIRE` TTL. Implement renew/release as conditional scripts matching the lease ID. Add a separate account-only slow-session guard key with TTL. Remove all quality types, quality methods, quality scripts, quality maps, and account-model admission key derivation.

- [ ] **Step 4: Run repository tests and format**

Run: `gofmt -w internal/repository/openai_shared_health.go internal/repository/openai_shared_health_test.go internal/service/openai_shared_health.go && go test ./internal/repository -run 'TestOpenAISharedHealthAdmission|TestOpenAISharedHealthSlowSession|TestOpenAISharedHealthStoreHasNoQualityAPI' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the Redis store**

```bash
git add upstream/sub2api/backend/internal/repository/openai_shared_health.go upstream/sub2api/backend/internal/repository/openai_shared_health_test.go upstream/sub2api/backend/internal/service/openai_shared_health.go
git commit -m "feat: add Redis-backed OpenAI admission leases"
```

### Task 3: Make Shared Mutations Cancellation-Independent and Observable

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_shared_health.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_resilience_observability.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler_shared_health_test.go`

**Interfaces:**
- Produces `openAISharedHealthWriteContext()`, which always starts from `context.Background()` and applies the configured Redis timeout.
- Existing reads continue to use `openAISharedHealthSelectionContext(parent)` and therefore retain caller cancellation.

- [ ] **Step 1: Write failing cancellation and event tests**

```go
func TestOpenAISharedHealthWriteContextIgnoresCanceledRequest(t *testing.T) {
    parent, cancel := context.WithCancel(context.Background())
    cancel()
    writeCtx, writeCancel := svc.openAISharedHealthWriteContext()
    defer writeCancel()
    require.NoError(t, writeCtx.Err())
    require.ErrorIs(t, parent.Err(), context.Canceled)
}
```

Use a store stub to assert `RecordAttempt`, admission release, and slow-session guard writes receive a live bounded context. Assert a canceled read context still prevents `GetAccountModel` remote selection. Assert emitted degraded events include `operation` and approved `error_kind` values but not error text, model text, credentials, or request content.

- [ ] **Step 2: Run service tests and verify they fail**

Run: `go test ./internal/service -run 'TestOpenAISharedHealthWriteContext|TestOpenAISharedHealth.*Degraded' -count=1`

Expected: FAIL because mutation contexts inherit cancellation or events lack required fields.

- [ ] **Step 3: Route every mutation through the write context**

Use the helper for record attempts, half-open completion, admission acquire/renew/release, and slow-session guard writes. Classify errors into `context_canceled`, `deadline_exceeded`, `redis_unavailable`, or `script_error`; attach only operation, classification, account hash, and result fields to the existing ledger/logger.

- [ ] **Step 4: Run service tests and format**

Run: `gofmt -w internal/service/openai_shared_health.go internal/service/openai_resilience_observability.go internal/service/openai_account_scheduler_shared_health_test.go && go test ./internal/service -run 'TestOpenAISharedHealthWriteContext|TestOpenAISharedHealth.*Degraded' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit write reliability changes**

```bash
git add upstream/sub2api/backend/internal/service/openai_shared_health.go upstream/sub2api/backend/internal/service/openai_resilience_observability.go upstream/sub2api/backend/internal/service/openai_account_scheduler_shared_health_test.go
git commit -m "fix: isolate OpenAI shared-health write contexts"
```

### Task 4: Integrate Admission Into the HTTP Scheduler Loop and Release Ownership

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/openai_chat_completions.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_resilience_observability.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`

**Interfaces:**
- Scheduling context carries `OpenAIAdmissionRequestShape` only for eligible HTTP streams.
- The handler's existing account-slot release closure is composed once with the selection release and admission lease release; every terminal path invokes that composed closure at most once.

- [ ] **Step 1: Write failing scheduler tests**

```go
func TestHTTPFailoverReselectsAfterAdmissionReject(t *testing.T) {
    store.rejectAccount[286] = "long_pre_first_output"
    response := performStreamingChatCompletionsRequest(t, handler, admissionShapeContext(ctx, OpenAIAdmissionShapeLong))
    require.Equal(t, http.StatusOK, response.Code)
    require.Equal(t, []int64{286, 287}, selectedAccounts())
    require.Equal(t, 1, store.AdmissionRejectedCount())
}
```

Cover: releasing the just-acquired local slot on rejection, excluding the rejected account before reselect, all candidates rejected using the existing no-account behavior, Redis error selecting normally with `store_degraded`, a lease acquired in one group/model rejecting the same account in another group/model, a slow-session guard rejecting all groups/models, and existing profit-first/group policy code being unable to bypass the safety filter.

- [ ] **Step 2: Run focused scheduler tests and verify they fail**

Run: `go test ./internal/handler -run 'TestHTTPFailoverReselectsAfterAdmissionReject|TestOpenAIAdmission.*' -count=1`

Expected: FAIL because the handler's existing select/slot loop has no admission retry/release composition.

- [ ] **Step 3: Apply admission only after the existing final selection and slot acquisition**

Construct a hashed lease ID from the existing service owner plus a monotonic sequence. In the handler, after `acquireResponsesAccountSlot` succeeds, acquire admission. On rejection call that newly acquired slot/result release immediately, append the account ID to the existing exclusion set, add bounded decision fields, and restart the current handler failover loop. On store error record degradation and retain the selected account. Start a 25-second renewal only while the pre-output lease remains active; stop it and release idempotently through the composed account release closure.

- [ ] **Step 4: Run scheduler tests and format**

Run: `gofmt -w internal/handler/openai_chat_completions.go internal/handler/openai_gateway_handler.go internal/service/openai_resilience_observability.go internal/handler/openai_gateway_handler_test.go && go test ./internal/handler -run 'TestHTTPFailoverReselectsAfterAdmissionReject|TestOpenAIAdmission.*' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit scheduler integration**

```bash
git add upstream/sub2api/backend/internal/handler/openai_chat_completions.go upstream/sub2api/backend/internal/handler/openai_gateway_handler.go upstream/sub2api/backend/internal/service/openai_resilience_observability.go upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go
git commit -m "feat: enforce OpenAI pre-output admission"
```

### Task 5: Propagate HTTP Request Shape, Release at Semantic First Output, and Guard Trusted Slow Sessions

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_chat_completions.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_response_handling.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_chat_completions.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_chat_completions_raw.go`
- Test: `upstream/sub2api/backend/internal/handler/openai_chat_completions_test.go`

**Interfaces:**
- `WithOpenAIAdmissionRequestShape(ctx, shape)` and `OpenAIAdmissionRequestShapeFromContext(ctx)` do not expose body data.
- Existing first-semantic-output callback receives an idempotent release closure. Only a trusted real completed stream with first output and TTFT at or above the global threshold may write the account-only slow-session guard.

- [ ] **Step 1: Write failing handler/stream tests**

```go
func TestOpenAIHTTPStreamReleasesAdmissionAtFirstSemanticOutput(t *testing.T) {
    release := newCountingRelease()
    forward := forwardStreamWithFirstOutputHook(release)
    forward.write([]byte("data: {\"type\":\"response.output_text.delta\"}\n\n"))
    require.Equal(t, 1, release.Calls())
}
```

Cover raw-byte boundary classification after body validation, no lease context for images/count-tokens/non-streaming endpoints, failure/cancel cleanup before first output, and only trusted real completed streams with first output triggering the account-only slow-session guard. Probes, failures, unknown shapes, and no-first-output cancellations must not trigger it.

- [ ] **Step 2: Run focused handler/service tests and verify they fail**

Run: `go test ./internal/handler ./internal/service -run 'TestOpenAIHTTPStreamReleasesAdmissionAtFirstSemanticOutput|TestOpenAIAdmissionRequestShape|TestOpenAI.*RequestQuality' -count=1`

Expected: FAIL because request shape and first-output hook are not wired.

- [ ] **Step 3: Wire raw body size, stream callbacks, and slow-session guard writes**

Attach shape only when the accepted OpenAI text request is streaming. Leave endpoint/transport behavior untouched otherwise. When the existing stream parser identifies its first semantic output, invoke the selected result's release closure. At successful real streaming completion with first output and TTFT at/above `SlowTTFTMS`, write the account-only slow-session guard through the shared write path; terminal defer still invokes the same closure for every error/cancel/panic path. Do not consult model or group policy during this safety decision.

- [ ] **Step 4: Run focused tests and format**

Run: `gofmt -w internal/handler/openai_gateway_handler.go internal/handler/openai_chat_completions.go internal/service/openai_gateway_response_handling.go internal/service/openai_gateway_chat_completions.go internal/service/openai_gateway_chat_completions_raw.go internal/handler/openai_chat_completions_test.go && go test ./internal/handler ./internal/service -run 'TestOpenAIHTTPStreamReleasesAdmissionAtFirstSemanticOutput|TestOpenAIAdmissionRequestShape|TestOpenAI.*RequestQuality' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the HTTP stream integration**

```bash
git add upstream/sub2api/backend/internal/handler/openai_gateway_handler.go upstream/sub2api/backend/internal/handler/openai_chat_completions.go upstream/sub2api/backend/internal/service/openai_gateway_response_handling.go upstream/sub2api/backend/internal/service/openai_gateway_chat_completions.go upstream/sub2api/backend/internal/service/openai_gateway_chat_completions_raw.go upstream/sub2api/backend/internal/handler/openai_chat_completions_test.go
git commit -m "feat: release OpenAI admission at first output"
```

### Task 6: Validate and Prepare the Root Handoff

**Files:**
- Modify: `docs/superpowers/plans/2026-08-27-t80-openai-scheduling-admission-resilience.md` (checklist completion only)
- Create: `docs/superpowers/handoffs/2026-08-27-t80-openai-scheduling-admission-resilience.md`

- [ ] **Step 1: Run direct test suites**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/config -run 'OpenAISharedHealth|OpenAIAdmission' -count=1
go test ./internal/repository -run 'OpenAISharedHealth' -count=1
go test ./internal/service -run 'OpenAI.*(Admission|SharedHealth|FirstOutput|SlowSession)' -count=1
go test ./internal/handler -run 'OpenAI.*(Admission|FirstOutput|ChatCompletions)' -count=1
go test ./internal/service -run '^$' -count=1
go build ./cmd/server
```

Expected: all commands pass.

- [ ] **Step 2: Run repository hygiene checks**

Run: `gofmt -w $(git diff --name-only -- '*.go') && git diff --check && git status --short`

Expected: no whitespace errors and only intended T80 files changed.

- [ ] **Step 3: Write a root handoff**

Include base `main` SHA, candidate SHA, changed files, commands/results, no migration/config production edit, expected `downtime_required=false` pending root precheck, rollback `admission_enabled=false` then previous verified blue-green image, and residual risks: capacity reduction from global thresholds, Redis fail-open, HTTP-stream-only coverage, and deferred group-level quality explanation refresh in T76.

- [ ] **Step 4: Commit plan checklist and handoff**

```bash
git add docs/superpowers/plans/2026-08-27-t80-openai-scheduling-admission-resilience.md docs/superpowers/handoffs/2026-08-27-t80-openai-scheduling-admission-resilience.md
git commit -m "docs: hand off T80 scheduler admission resilience"
```

## Self-Review

1. **Spec coverage:** Tasks 1-2 remove the invalid account-model/shared-quality contract and cover global account-only config, atomic lease/guard mutation, TTL, renewal, stalled, capacity, and negative API assertions. Task 3 covers canceled-write root cause and sanitized degradation. Task 4 covers post-selection admission, cross-model/cross-group exclusion/reselect, release ownership, non-bypassable group-policy behavior, and decision auditing. Task 5 covers raw byte classification, first semantic output, trusted slow-session guard writes, terminal cleanup, and endpoint exclusions. Task 6 provides the exact scoped validation and root-only release handoff.
2. **Placeholder scan:** This plan contains no deferred implementation placeholders. The concrete Lua behavior, method names, test assertions, commands, and release rules are stated in their owning tasks.
3. **Type consistency:** All later tasks use the Task 1 request-shape types and Task 2 store method names; all release work flows through the existing `AccountSelectionResult.ReleaseFunc` composed in Task 4 and consumed by Task 5.
