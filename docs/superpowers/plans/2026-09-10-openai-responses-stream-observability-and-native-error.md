# OpenAI Responses Stream Observability and Native Error Handling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make OpenAI Responses passthrough streams observable and protocol-correct on failure, while preserving only safe, explicitly recognized OpenAI-native errors for passthrough accounts.

**Architecture:** Keep SSE protocol state in `handleStreamingResponsePassthrough`, where event boundaries and client-write state are available. Keep HTTP fallback decisions in `OpenAIGatewayHandler`, but make them depend on a recorded legal Responses terminal event rather than on “some bytes were written”. Reuse existing observation and sanitization structures.

**Tech Stack:** Go, Gin, `net/http`, SSE scanner, existing `StreamObservation`, `testify/require`, `go test`, `go build`.

**Spec:** `docs/superpowers/specs/2026-09-10-openai-responses-stream-observability-and-native-error-design.md`

## Global Constraints

- Keep `accounts.extra.openai_passthrough` semantics unchanged; it selects the passthrough path and is not an unconditional raw-error switch.
- Responses legal terminal events are `response.completed`, `response.failed`, `response.incomplete`, `response.cancelled`, plus the existing `[DONE]` compatibility marker.
- A bare `event: error` never by itself proves that a Responses stream has ended legally.
- Never expose credentials, complete request bodies, internal URLs, request IDs, Ray IDs, upstream HTML, or raw provider error chains.
- Do not change failover, billing, concurrency, audit, or cyber-policy decisions except to avoid duplicate terminal events.
- Do not add migrations, runtime configuration, external logging systems, or GitHub Actions workflows.
- Work from an isolated `codex/` worktree created from the latest clean `origin/main`; do not deploy from the candidate.
- Preserve the unrelated `docs/project/project-progress.md` modification.

## File Map

- Modify `upstream/sub2api/backend/internal/service/openai_gateway_passthrough.go`: wire the observer into Responses passthrough; record event/visible-output/terminal/client-write state; synthesize one sanitized `response.failed` when needed.
- Modify `upstream/sub2api/backend/internal/service/stream_observability_runtime.go` only if the existing lifecycle API cannot represent the required terminal/incomplete distinction.
- Modify `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`: make `openAIForwardErrorAlreadyCommunicated` recognize a legal Responses terminal or complete non-stream response.
- Modify `upstream/sub2api/backend/internal/service/openai_gateway_response_handling.go`: add the narrow structured safe-native-error decision while retaining sanitization.
- Extend focused service, handler, sanitization, and lifecycle tests in existing `*_test.go` files.

## Task 1: Isolated Workspace and Baseline

**Files:** Create `.worktrees/openai-responses-stream-observability` and branch `codex/openai-responses-stream-observability`; read `AGENTS.md`, the three project constraint documents, the approved spec, and the task queue.

**Interfaces:** Consumes current `origin/main`; produces a clean candidate worktree and baseline evidence.

- [ ] **Step 1: Verify repository identity and protected worktrees.** Run `git fetch origin`, `git status --short --branch`, `git worktree list --porcelain`, `git rev-parse HEAD`, and `git rev-parse origin/main`. Root `main` must match `origin/main`; preserve the existing progress-ledger modification.
- [ ] **Step 2: Create the candidate from `origin/main`.** Verify `.worktrees` is ignored, then run `git worktree add .worktrees/openai-responses-stream-observability -b codex/openai-responses-stream-observability origin/main`.
- [ ] **Step 3: Run the focused baseline.** From `upstream/sub2api/backend`, run `go test ./internal/service ./internal/handler` and `go build ./cmd/server`; record pre-existing failures without changing unrelated code.

## Task 2: Red Tests for Terminal Semantics

**Files:** Test the closest existing passthrough file under `upstream/sub2api/backend/internal/service`, `internal/handler/openai_gateway_handler_test.go`, and `internal/service/stream_observability_test.go`.

**Interfaces:** Consumes current passthrough, handler fallback, and `StreamObservation` behavior; produces failing tests defining the regression contract.

- [ ] **Step 1: Add the bare-error EOF regression test.** Use an `httptest.ResponseRecorder` and HTTP 200 `text/event-stream` body containing `event: error` without `response.failed`; parse SSE events and assert the target behavior: exactly one `response.failed`, no visible duplicate `event:error`, and an upstream failure returned.
- [ ] **Step 2: Add legal-terminal tests.** Cover upstream `response.failed` and `response.completed`; assert each is emitted/preserved once and no synthetic terminal is appended.
- [ ] **Step 3: Add the handler fallback test.** Extend `openAIForwardErrorAlreadyCommunicated` tests with a Responses writer containing only `event: error`; target result is `false`.
- [ ] **Step 4: Add lifecycle tests.** Cover read failure without legal terminal as `openai.stream_incomplete`, `response.failed` as terminal, and no persisted event body.
- [ ] **Step 5: Verify red.** Run `go test ./internal/service -run 'TestHandleStreamingResponsePassthrough|TestStreamObservation' -count=1` and `go test ./internal/handler -run 'TestOpenAIForwardErrorAlreadyCommunicated' -count=1`; failures must be behavioral, not fixture or compile errors.
- [ ] **Step 6: Commit red tests.** Run `git add upstream/sub2api/backend/internal/service upstream/sub2api/backend/internal/handler && git commit -m "test: cover Responses passthrough terminal failures"`.

## Task 3: Wire Responses Passthrough into Stream Observability

**Files:** Modify `internal/service/openai_gateway_passthrough.go`; modify `stream_observability_runtime.go` only if required; extend `stream_observability_test.go`.

**Interfaces:** Consumes existing `BeginStreamObservation`, `ObserveUpstreamHeaders`, `ObserveSSEEvent`, `ObserveVisibleOutput`, `ObserveTerminal`, `ObserveReadFailure`, `ObserveClientWriteFailure`, and `FinishStreamObservation`; produces one observation per Responses passthrough request with event index, bytes, semantic output, terminal event, failure class, and client-disconnect state.

- [ ] **Step 1: Start or reuse the observer at passthrough entry.** Reuse a context observer or call `BeginStreamObservation` with original model, mapped model, platform, and account; call `ObserveUpstreamHeaders` after the upstream response is available.
- [ ] **Step 2: Record parsed SSE events without bodies.** After deriving the effective event type, call `ObserveSSEEvent` with monotonic index and cumulative bytes; never add payload bytes to `StreamObservation` or logs.
- [ ] **Step 3: Record visible output and legal terminals.** On first visible output call `ObserveVisibleOutput`; for each legal terminal call `ObserveTerminal` with event type, response ID, parsed usage, and forwarded bytes.
- [ ] **Step 4: Record write/read failures.** Route pending-line, ordinary event, and synthetic-terminal write errors through `ObserveClientWriteFailure`; route scanner errors through `ObserveReadFailure`; never classify client write failure as upstream EOF.
- [ ] **Step 5: Finish every return path correctly.** A valid terminal is successful only without failure; a stream without one remains incomplete/failed; a disconnected client must not trigger another write.
- [ ] **Step 6: Verify and commit.** Run `gofmt -w internal/service/openai_gateway_passthrough.go internal/service/stream_observability_runtime.go internal/service/stream_observability_test.go && go test ./internal/service -run 'TestStreamObservation|TestHandleStreamingResponsePassthrough' -count=1`; then commit `fix: observe OpenAI Responses passthrough streams`.

## Task 4: Enforce a Legal Responses Terminal on Failure

**Files:** Modify `internal/service/openai_gateway_passthrough.go` and `internal/handler/openai_gateway_handler.go`; extend focused passthrough and handler tests.

**Interfaces:** Consumes passthrough event state and `writeResponsesFailedSSE`/`buildOpenAIResponseFailedSSE`; produces exactly one Responses terminal on recoverable failure, or no additional write after client disconnect.

- [ ] **Step 1: Centralize legal-terminal recognition.** Use one predicate/state check for `response.completed`, `response.failed`, `response.incomplete`, `response.cancelled`, and `[DONE]`; the handler must not independently parse raw body text.
- [ ] **Step 2: Synthesize failed only when needed.** If bare error is observed and no authoritative `response.failed` arrives before EOF/read failure, write one sanitized `response.failed`; mark it terminal and delivered so cleanup cannot duplicate it.
- [ ] **Step 3: Preserve one upstream failed event.** Keep current usage parsing, account-side-effect ordering, and response-field sanitization; do not append a second failure after the service emitted the upstream terminal.
- [ ] **Step 4: Tighten `openAIForwardErrorAlreadyCommunicated`.** For Responses return true only for a recorded legal terminal or complete non-stream response; written bytes plus error prefix or bare `event:error` returns false; preserve non-Responses behavior.
- [ ] **Step 5: Verify and commit.** Run `go test ./internal/service -run 'TestHandleStreamingResponsePassthrough|TestOpenAI.*ResponseFailed' -count=1` and `go test ./internal/handler -run 'TestOpenAIForwardErrorAlreadyCommunicated|Test.*Responses.*Failed|Test.*Stream.*Error' -count=1`; commit `fix: terminate Responses passthrough failures correctly`.

## Task 5: Add Safe OpenAI-Native Error Preservation

**Files:** Modify `internal/service/openai_gateway_response_handling.go`; extend `internal/service/openai_responses_error_sanitization_test.go`.

**Interfaces:** Consumes `Account.IsOpenAIPassthroughEnabled`, parsed error payload, upstream status/event type, and current sanitization helpers; produces a structured allow/deny decision with no raw-provider payload bypass.

- [ ] **Step 1: Add failing allowlist tests.** Cover structured capacity/overload, model-selection/model-unavailable, and explicit rate-limit codes for a passthrough OpenAI account; also cover unknown provider messages and request IDs/URLs. Do not rely only on free-text keywords.
- [ ] **Step 2: Verify red.** Run `go test ./internal/service -run 'TestSanitizeOpenAIResponseFailedEvent' -count=1`; at least one new preservation assertion must fail while existing redaction assertions remain meaningful.
- [ ] **Step 3: Implement the narrow structured decision.** Require OpenAI platform/passthrough account, Responses error event, recognized structured code/status, and sanitized message; preserve only safe `code/message`; keep current fallback for unknown/third-party/sensitive errors; do not modify `IsOpenAIPassthroughEnabled`.
- [ ] **Step 4: Verify and commit.** Run `gofmt -w internal/service/openai_gateway_response_handling.go internal/service/openai_responses_error_sanitization_test.go && go test ./internal/service -run 'TestSanitizeOpenAIResponseFailedEvent' -count=1`; commit `fix: preserve safe native OpenAI passthrough errors`.

## Task 6: Focused Verification and Candidate Handoff

**Files:** No planned modifications; read the approved spec, constraints, diff, test output, and branch history.

**Interfaces:** Consumes all candidate commits; produces `READY_FOR_ROOT_REVIEW` evidence with no merge or deployment claim.

- [ ] **Step 1: Run directly related tests.** From `upstream/sub2api/backend`, run `go test ./internal/service ./internal/handler -count=1`.
- [ ] **Step 2: Run formatting, build, and diff checks.** Run `gofmt` on changed Go files, `go test ./internal/service ./internal/handler -count=1`, `go build ./cmd/server`, and `git diff --check`.
- [ ] **Step 3: Review scope and sensitive-data boundaries.** Run `git diff origin/main...HEAD --stat`, `git diff origin/main...HEAD --name-status`, and `git status --short`; confirm no migrations, configuration, credentials, raw event bodies in logs, unrelated frontend changes, or account-toggle changes.
- [ ] **Step 4: Record handoff evidence.** Record branch, baseline, candidate HEAD, commit summary, changed files, tests, build, migration/config status, rollback commit/image, and unverified items; set status `READY_FOR_ROOT_REVIEW`; do not merge, push root `main`, deploy, or claim production behavior.
