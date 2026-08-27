# OpenAI Scheduling Admission Resilience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent a slow OpenAI account/model from receiving a burst of HTTP streaming text requests before its first semantic output, while retaining the existing scheduler and failing open if shared Redis state is unavailable.

**Architecture:** The existing `OpenAISharedHealthStore` gains Lua-atomic admission leases and real-request TTFT buckets keyed only by account ID plus canonical-model hash. HTTP text handlers classify already-validated raw bodies, the scheduler acquires a lease after selecting an account and obtaining its local slot, and the normal selection release chain clears that lease at first semantic output or terminal cleanup. Shared writes use a fresh bounded background context so request cancellation cannot prevent health, quality, or lease cleanup mutations.

**Tech Stack:** Go, Gin, Redis/go-redis v9 Lua scripts, miniredis, testify.

**Spec:** `docs/superpowers/specs/2026-08-27-t80-openai-scheduling-admission-resilience-design.md`

## Global Constraints

- Reuse native OpenAI scheduler, shared-health store, response stream detection, and bounded resilience ledger; do not add a second routing or quality source.
- Only HTTP streaming text requests with a recognized request shape participate; non-streaming, images, `/count_tokens`, WebSocket, and unknown shapes preserve current behavior.
- Raw body classification is `normal`/`long` at `65536` bytes by default; do not parse tokens or persist raw bodies.
- Redis admission mutations are Lua atomic and keyed by account plus canonical model only; long max is 1, normal max is 2, a long blocks both shapes, stalled pre-output is 30 seconds, lease TTL is 90 seconds, renewal is 25 seconds.
- Shared-store failures fail open, log only structured/sanitized fields, and never expose credentials or request bodies.
- No database migration, API/UI contract change, GitHub Actions, root-ledger edits, push, merge to `main`, deployment, or production access from this worktree.
- Direct validation is scoped Go tests, `go build ./cmd/server`, `gofmt`, and `git diff --check`; no full-suite, load, or soak run.

---

## File Structure

- `upstream/sub2api/backend/internal/config/config.go`: admission defaults, environment mapping, and strict validation.
- `upstream/sub2api/backend/internal/config/config_test.go`: default/env/invalid admission configuration coverage.
- `upstream/sub2api/backend/internal/service/openai_shared_health.go`: request-shape and lease/quality contracts; independent mutation context; service-side acquire/release/quality helpers.
- `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`: apply shape-specific shared quality cooldown as an eligibility filter and report decision fields.
- `upstream/sub2api/backend/internal/service/openai_gateway_scheduling.go`: acquire/retry admission after final account selection/local slot and compose one idempotent release chain.
- `upstream/sub2api/backend/internal/service/openai_resilience_observability.go`: bounded decision/event fields for admission and quality outcomes.
- `upstream/sub2api/backend/internal/repository/openai_shared_health.go`: Redis key derivation and Lua acquire/renew/release/quality scripts.
- `upstream/sub2api/backend/internal/repository/openai_shared_health_test.go`: miniredis script behavior, expiry, fencing, and non-sensitive key assertions.
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
- Produces `OpenAIAdmissionRequestShape`, `OpenAISharedAdmissionRequest`, `OpenAISharedAdmissionDecision`, and `OpenAISharedRequestQualitySnapshot` exactly as specified.
- Produces config fields `AdmissionEnabled`, `LongRequestBodyThresholdBytes`, `MaxPreFirstOutputNormal`, `MaxPreFirstOutputLong`, `StalledBeforeFirstOutputSeconds`, `AdmissionLeaseTTLSeconds`, `AdmissionRenewSeconds`, `SlowTTFTMS`, and `SlowQualityCooldownSeconds`.

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

### Task 2: Implement Atomic Redis Admission and Shape-Scoped Quality

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/openai_shared_health.go`
- Modify: `upstream/sub2api/backend/internal/repository/openai_shared_health_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_shared_health.go`

**Interfaces:**
- Extends `OpenAISharedHealthStore` with `AcquireAdmission`, `RenewAdmission`, `ReleaseAdmission`, `GetRequestQuality`, and `RecordRequestQuality`.
- `AcquireAdmission(ctx, request)` returns `OpenAISharedAdmissionDecision`; it removes expired entries, detects stalled entries, enforces long/normal limits, and never stores raw lease/correlation/request values.

- [ ] **Step 1: Write failing miniredis tests**

```go
func TestOpenAISharedHealthAdmissionLongBlocksNormal(t *testing.T) {
    long := service.OpenAISharedAdmissionRequest{Key: key, LeaseID: "lease-a", Shape: service.OpenAIAdmissionShapeLong, ObservedAt: now}
    require.True(t, store.AcquireAdmission(ctx, long).Allowed)
    normal := service.OpenAISharedAdmissionRequest{Key: key, LeaseID: "lease-b", Shape: service.OpenAIAdmissionShapeNormal, ObservedAt: now}
    got, err := store.AcquireAdmission(ctx, normal)
    require.NoError(t, err)
    require.False(t, got.Allowed)
    require.Equal(t, "long_pre_first_output", got.Reason)
}
```

Also cover concurrent long acquisition (one winner), normal capacity two, stalled rejection, lease-owner-only release, renewal, expiry cleanup, long-only quality cooldown, and key/script arguments excluding raw request values.

- [ ] **Step 2: Run repository tests and verify they fail**

Run: `go test ./internal/repository -run 'TestOpenAISharedHealthAdmission|TestOpenAISharedHealthRequestQuality' -count=1`

Expected: FAIL because the new store methods and Lua scripts do not exist.

- [ ] **Step 3: Add Redis Lua scripts and decoding helpers**

Use a ZSET/hash admission key namespace under the existing versioned prefix. In one Lua call, remove expired leases, count shape entries, reject a stale entry older than configured duration, reject any normal/long request while a long entry exists, enforce shape capacity, and add only the hash-safe lease identifier with `PEXPIRE` TTL. Implement renew/release as conditional scripts matching the lease ID. Persist real TTFT quality per shape separately, update EWMA only for positive TTFT, and set only that shape's cooldown when TTFT meets `SlowTTFTMS`.

- [ ] **Step 4: Run repository tests and format**

Run: `gofmt -w internal/repository/openai_shared_health.go internal/repository/openai_shared_health_test.go internal/service/openai_shared_health.go && go test ./internal/repository -run 'TestOpenAISharedHealthAdmission|TestOpenAISharedHealthRequestQuality' -count=1`

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

Use a store stub to assert `RecordAttempt` and admission release receive a live bounded context. Assert a canceled read context still prevents `GetAccountModel` remote selection. Assert emitted degraded events include `operation` and approved `error_kind` values but not error text, model text, credentials, or request content.

- [ ] **Step 2: Run service tests and verify they fail**

Run: `go test ./internal/service -run 'TestOpenAISharedHealthWriteContext|TestOpenAISharedHealth.*Degraded' -count=1`

Expected: FAIL because mutation contexts inherit cancellation or events lack required fields.

- [ ] **Step 3: Route every mutation through the write context**

Use the helper for record attempts, half-open completion, admission acquire/renew/release, and request-quality writes. Classify errors into `context_canceled`, `deadline_exceeded`, `redis_unavailable`, or `script_error`; attach only operation, classification, account/model hash, and result fields to the existing ledger/logger.

- [ ] **Step 4: Run service tests and format**

Run: `gofmt -w internal/service/openai_shared_health.go internal/service/openai_resilience_observability.go internal/service/openai_account_scheduler_shared_health_test.go && go test ./internal/service -run 'TestOpenAISharedHealthWriteContext|TestOpenAISharedHealth.*Degraded' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit write reliability changes**

```bash
git add upstream/sub2api/backend/internal/service/openai_shared_health.go upstream/sub2api/backend/internal/service/openai_resilience_observability.go upstream/sub2api/backend/internal/service/openai_account_scheduler_shared_health_test.go
git commit -m "fix: isolate OpenAI shared-health write contexts"
```

### Task 4: Integrate Admission Into Scheduler Selection and Release Ownership

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_scheduling.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_resilience_observability.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler_shared_health_test.go`

**Interfaces:**
- Scheduling context carries `OpenAIAdmissionRequestShape` only for eligible HTTP streams.
- A successful selected result owns a single idempotent `ReleaseFunc` that releases the local slot, half-open state, and admission lease exactly once.

- [ ] **Step 1: Write failing scheduler tests**

```go
func TestSchedulerReselectsAfterAdmissionReject(t *testing.T) {
    store.rejectAccount[286] = "long_pre_first_output"
    result, decision, err := svc.SelectAccountWithSchedulerForCapability(ctx, groupID, "", "session", "gpt-5.6", nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions)
    require.NoError(t, err)
    require.Equal(t, int64(287), result.Account.ID)
    require.Equal(t, 1, decision.AdmissionRejectedCount)
}
```

Cover: releasing the just-acquired local slot on rejection, excluding the rejected account before reselect, all candidates rejected using the existing no-account behavior, Redis error selecting normally with `store_degraded`, cooldown filtering only same shape, and multiple calls to final `ReleaseFunc` issuing one admission release.

- [ ] **Step 2: Run focused scheduler tests and verify they fail**

Run: `go test ./internal/service -run 'TestSchedulerReselectsAfterAdmissionReject|TestOpenAIAdmission.*' -count=1`

Expected: FAIL because selection has no admission retry/release composition.

- [ ] **Step 3: Apply admission only after the existing final selection and slot acquisition**

Construct a hashed lease ID from the existing service owner plus a monotonic sequence. On rejection call the slot/result release immediately, append the account ID to the existing exclusion set, add bounded decision fields, and restart current selection logic. On store error record degradation and retain the selected account. Start a 25-second renewal only while the pre-output lease remains active; stop it and release idempotently through the composed result release function.

- [ ] **Step 4: Run scheduler tests and format**

Run: `gofmt -w internal/service/openai_account_scheduler.go internal/service/openai_gateway_scheduling.go internal/service/openai_resilience_observability.go internal/service/openai_account_scheduler_shared_health_test.go && go test ./internal/service -run 'TestSchedulerReselectsAfterAdmissionReject|TestOpenAIAdmission.*' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit scheduler integration**

```bash
git add upstream/sub2api/backend/internal/service/openai_account_scheduler.go upstream/sub2api/backend/internal/service/openai_gateway_scheduling.go upstream/sub2api/backend/internal/service/openai_resilience_observability.go upstream/sub2api/backend/internal/service/openai_account_scheduler_shared_health_test.go
git commit -m "feat: enforce OpenAI pre-output admission"
```

### Task 5: Propagate HTTP Request Shape and Release at Semantic First Output

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_chat_completions.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_response_handling.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_chat_completions.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_chat_completions_raw.go`
- Test: `upstream/sub2api/backend/internal/handler/openai_chat_completions_test.go`

**Interfaces:**
- `WithOpenAIAdmissionRequestShape(ctx, shape)` and `OpenAIAdmissionRequestShapeFromContext(ctx)` do not expose body data.
- Existing first-semantic-output callback receives an idempotent release closure and TTFT recorder.

- [ ] **Step 1: Write failing handler/stream tests**

```go
func TestOpenAIHTTPStreamReleasesAdmissionAtFirstSemanticOutput(t *testing.T) {
    release := newCountingRelease()
    forward := forwardStreamWithFirstOutputHook(release)
    forward.write([]byte("data: {\"type\":\"response.output_text.delta\"}\n\n"))
    require.Equal(t, 1, release.Calls())
}
```

Cover raw-byte boundary classification after body validation, no lease context for images/count-tokens/non-streaming endpoints, failure/cancel cleanup before first output, and completed real streaming TTFT writing only the matching shape. Ensure monitor probes do not use this pathway.

- [ ] **Step 2: Run focused handler/service tests and verify they fail**

Run: `go test ./internal/handler ./internal/service -run 'TestOpenAIHTTPStreamReleasesAdmissionAtFirstSemanticOutput|TestOpenAIAdmissionRequestShape|TestOpenAI.*RequestQuality' -count=1`

Expected: FAIL because request shape and first-output hook are not wired.

- [ ] **Step 3: Wire raw body size, stream callbacks, and quality writes**

Attach shape only when the accepted OpenAI text request is streaming. Leave endpoint/transport behavior untouched otherwise. When the existing stream parser identifies its first semantic output, invoke the selected result's release closure. At successful streaming completion, record positive TTFT through the shared write path only for a real request and its classified shape; terminal defer still invokes the same closure for every error/cancel/panic path.

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
go test ./internal/service -run 'OpenAI.*(Admission|SharedHealth|FirstOutput|RequestQuality)' -count=1
go test ./internal/handler -run 'OpenAI.*(Admission|FirstOutput|ChatCompletions)' -count=1
go test ./internal/service -run '^$' -count=1
go build ./cmd/server
```

Expected: all commands pass.

- [ ] **Step 2: Run repository hygiene checks**

Run: `gofmt -w $(git diff --name-only -- '*.go') && git diff --check && git status --short`

Expected: no whitespace errors and only intended T80 files changed.

- [ ] **Step 3: Write a root handoff**

Include base `main` SHA, candidate SHA, changed files, commands/results, no migration/config production edit, expected `downtime_required=false` pending root precheck, rollback `admission_enabled=false` then previous verified blue-green image, and residual risks: capacity reduction from thresholds, Redis fail-open, HTTP-stream-only coverage.

- [ ] **Step 4: Commit plan checklist and handoff**

```bash
git add docs/superpowers/plans/2026-08-27-t80-openai-scheduling-admission-resilience.md docs/superpowers/handoffs/2026-08-27-t80-openai-scheduling-admission-resilience.md
git commit -m "docs: hand off T80 scheduler admission resilience"
```

## Self-Review

1. **Spec coverage:** Tasks 1-2 cover the complete config, contracts, atomic Redis mutation, TTL, renewal, stalled, capacity, and bucket-quality requirements. Task 3 covers canceled-write root cause and sanitized degradation. Task 4 covers post-selection admission, exclusion/reselect, release ownership, quality filtering, and decision auditing. Task 5 covers raw byte classification, first semantic output, terminal cleanup, and endpoint exclusions. Task 6 provides the exact scoped validation and root-only release handoff.
2. **Placeholder scan:** This plan contains no deferred implementation placeholders. The concrete Lua behavior, method names, test assertions, commands, and release rules are stated in their owning tasks.
3. **Type consistency:** All later tasks use the Task 1 request-shape types and Task 2 store method names; all release work flows through the existing `AccountSelectionResult.ReleaseFunc` composed in Task 4 and consumed by Task 5.
