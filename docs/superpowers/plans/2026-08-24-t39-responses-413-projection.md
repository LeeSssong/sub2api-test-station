# T39 Responses 413 二次错误投影修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: execute inline with systematic debugging and test-driven development. Leave the detached candidate at `READY_FOR_ROOT_REVIEW`; root controls refresh, merge, push, deploy, and production verification.

**Goal:** Preserve application-handled 413 as a redacted Chinese oversized-request error in JSON and Responses SSE without a second native user-error projection.

**Architecture:** Keep the existing `ProjectNativeUserError` as the single user-message authority. Give deterministic 413 semantics priority over generic selected-account masking, and pass original status/type/code/message into the Responses terminal-event writer so it projects exactly once before serialization.

**Tech Stack:** Go, Gin, `net/http/httptest`, Testify.

**Spec:** `docs/superpowers/specs/2026-08-24-t39-responses-413-projection-design.md`

## Global Constraints

- Only application-handled inbound/upstream 413 in JSON and Responses SSE is in scope.
- Preserve `invalid_request_error`, Responses `invalid_request`, the `response.failed` terminal event, and fixed Chinese `请求内容过大，请缩短内容后重试。` semantics.
- Never expose upstream body, URL, request/Ray ID, credentials, or account/provider identity.
- Do not implement Cloudflare HTML 413 handling or any T40 status mapping.
- No migration, configuration, production data, GitHub Actions, main merge, push, deployment, or production access.

---

### Task 1: Lock deterministic 413 projection priority

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/native_user_error_projection_test.go`
- Modify: `upstream/sub2api/backend/internal/service/native_user_error_projection.go`

**Interfaces:**
- Consumes: `NativeUserErrorInput{Status, Type, Code, Message, Stage, Ownership, AccountSelected}`.
- Produces: unchanged `NativeUserErrorProjection{Type, Code, Message}` with 413 message priority.

- [x] **Step 1: Write the failing test**

Add a case equivalent to:

```go
got := ProjectNativeUserError(NativeUserErrorInput{
    Status: 413, Type: "invalid_request_error", Message: "proxy limit secret=must-not-leak",
    Stage: "upstream", Ownership: "provider", AccountSelected: true,
})
require.Equal(t, "invalid_request_error", got.Type)
require.Equal(t, "请求内容过大，请缩短内容后重试。", got.Message)
```

Retain or add a selected-account 502 assertion for `服务暂时异常，请稍后重试。`.

- [x] **Step 2: Run RED**

Run: `go test ./internal/service -run 'TestProjectNativeUserError' -count=1`

Expected: FAIL because the selected-account branch currently wins and returns `服务暂时异常，请稍后重试。`.

- [x] **Step 3: Implement the minimal priority fix**

Move the 413/oversized-request case before generic upstream masking. Keep all other cases and the final safe-message check unchanged.

- [x] **Step 4: Run GREEN**

Run the same focused service command. Expected: PASS.

### Task 2: Make Responses terminal serialization project once

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/stream_error_event.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_body_limit_failover_test.go`
- Modify if direct contract needs it: `upstream/sub2api/backend/internal/handler/stream_error_event_test.go`

**Interfaces:**
- Consumes: raw `status`, `errType`, optional `code`, and `message` at each Responses error call site.
- Produces: `writeResponsesFailedSSE(c, status, errType, code, message) bool`; writer applies `projectNativeUserErrorForContext` once, preserves non-empty projected code, otherwise uses `mapResponsesErrorCode(projected.Type)`.

- [x] **Step 1: Write the failing handler tests**

Update the body-limit failover tests to set an account ID in context and assert:

```go
require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
require.Equal(t, "invalid_request_error", errBody["type"])
require.Equal(t, "请求内容过大，请缩短内容后重试。", errBody["message"])
require.NotContains(t, rec.Body.String(), "must-not-leak")
```

For streaming, parse the sole `response.failed` data payload and assert `code=invalid_request`, the same Chinese message, exactly one terminal event, and no sensitive body evidence.

- [x] **Step 2: Run RED**

Run: `go test ./internal/handler -run 'TestOpenAIBodyLimitFailoverExhausted' -count=1`

Expected: FAIL because selected-account projection returns the generic service-error message; SSE also reprojects with fixed 502.

- [x] **Step 3: Implement the smallest single-projection data flow**

Change `writeResponsesFailedSSE` to receive status/type/code/message and perform the only Responses projection. In `OpenAIGatewayHandler` and `GatewayHandler`, route Responses into this writer before the generic stream projection; retain the generic projection for non-Responses SSE. Update compact-heartbeat and cyber-session call sites with their real status and code.

- [x] **Step 4: Run GREEN**

Run the handler command from Step 2. Expected: PASS.

- [x] **Step 5: Run directly related protocol regressions**

Run:

```bash
go test ./internal/handler -run 'Test(OpenAIBodyLimitFailoverExhausted|WriteResponsesFailedSSE|OpenAIStreamingError|GatewayStreamingError)' -count=1
```

If exact test names differ, select only the existing `stream_error_event`, native writer, and error-fallback contracts that compile all modified call sites.

### Task 3: Validate, review, and hand off

**Files:**
- Create: `docs/handoffs/2026-08-24-t39-responses-413-projection-handoff.md`

**Interfaces:**
- Consumes: clean detached candidate and verification output.
- Produces: root-ready handoff with baseline, final detached SHA, files, tests, migration/config, downtime expectation, rollback, and risk.

- [x] **Step 1: Format and run focused verification**

Run:

```bash
gofmt -w internal/service/native_user_error_projection.go internal/service/native_user_error_projection_test.go internal/handler/stream_error_event.go internal/handler/stream_error_event_test.go internal/handler/openai_gateway_handler.go internal/handler/gateway_handler.go internal/handler/openai_body_limit_failover_test.go
cd upstream/sub2api/backend
go test ./internal/service -run 'TestProjectNativeUserError' -count=1
go test ./internal/handler -run 'Test(OpenAIBodyLimitFailoverExhausted|OpenAIHandleStreamingAwareError|GatewayHandleStreamingAwareError|InboundIsResponses|SynthesizeResponseID|MapResponsesErrorCode)' -count=1
go build ./cmd/server
```

- [x] **Step 2: Run scope and diff checks**

Run:

```bash
git diff --check
git diff --name-only 5ded56aac949b6f1b8dced8a384b3761a54b39f5...HEAD
git diff -- .github ops upstream/sub2api/backend/migrations docs/project
```

Expected: no `.github`, `ops`, migration, queue, or project-progress changes.

- [x] **Step 3: Review against the spec**

Confirm every acceptance-matrix row covered by code/test evidence; confirm ordinary selected-account 5xx remains generic; confirm no T40 statuses or Cloudflare HTML detection entered the diff.

- [x] **Step 4: Write handoff and commit the clean detached candidate**

Record RED/GREEN commands and outputs, fresh GREEN/build/diff evidence, unverified production items, no migration/config, expected `downtime_required=false`, root-only rollback, and residual risk. Commit all T39 files and report the detached HEAD SHA without creating or modifying main.

## Plan Self-Review And Approval

- Spec coverage: all goals, non-goals, contracts, security, compatibility, tests, release and rollback map to Tasks 1–3.
- Placeholder scan: no implementation placeholder or unresolved decision remains; verification commands name the concrete files and tests produced by Tasks 1–2.
- Type consistency: the writer signature and projected fields are identical across Tasks 2–3.
- Approval: `APPROVED_BY_ROOT_RELEASE_CONTROLLER_PROXY` under the existing queue authorization; no product, safety, data, cost, migration, or downtime decision is pending.
