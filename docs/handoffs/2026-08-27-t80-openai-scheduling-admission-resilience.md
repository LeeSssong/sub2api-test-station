# T80 OpenAI Scheduling Admission Resilience Handoff

## Status

`READY_FOR_ROOT_REVIEW`. Candidate `b82277680` is refreshed from root `main@5cceb1643436bcd3a4a751b932376805c49df44a` with no T80 conflicts. Direct validation after refresh passed. Root review must use this candidate tree; do not merge, push, deploy, or modify root project records from this worktree.

## Scope Delivered

- Account-only Redis admission lease and `slow_session_guard`, visible across all OpenAI scheduler groups and models. No T80 account-model quality or configuration contract exists.
- Chat Completions, Responses, and Messages acquire admission after scheduler selection and local-slot acquisition. Rejection releases the acquired slot, excludes the account, and reselects in the same handler loop.
- Admission release is composed with the existing release closure and is triggered after the first semantic output is actually emitted. Chat, Responses, Messages, raw Chat Completions, Anthropic-to-Chat forwarding, and both Chat fallback paths propagate the callback.
- The CC fallback scanner regression test proves its callback runs after `emit`, not before it.
- Guarded streaming now releases only after the complete semantic SSE event flush succeeds; a flush failure leaves the admission held for terminal cleanup.
- Slow-session guard writes require a trusted completed real stream with first output and threshold-exceeding TTFT; probes, failures, unknown/non-stream requests, and no-first-output cancellation do not create it.

## Changed Areas

- `internal/config`: global admission defaults and validation.
- `internal/repository/openai_shared_health.go`: account-only Redis/Lua admission and guard state, plus miniredis coverage.
- `internal/service/openai_shared_health.go`: acquire, renewal, idempotent release, fail-open only for store errors, and trusted slow-session guard recording.
- `internal/handler/openai_chat_completions.go` and `openai_gateway_handler.go`: account-only admission/reselect/release wiring for Chat, Responses, and Messages.
- OpenAI stream forwarders and CC fallback pipeline: first-semantic-output callback propagation and release timing.
- T80 spec, plan, and targeted wiring tests.

## Direct Validation

The following commands passed on August 27, 2026 in `upstream/sub2api/backend` on refreshed candidate `b82277680` after the final flush-failure fix and the policy-independence regression test:

```bash
go test ./internal/config -run 'OpenAISharedHealth|OpenAIAdmission' -count=1
go test ./internal/repository -run 'OpenAISharedHealth' -count=1
go test ./internal/service -run 'OpenAI.*(Admission|SharedHealth|FirstOutput|SlowSession)|TestHandleChatStreamingResponse|TestHandleCCStreamingFromAnthropic|TestForwardAsRawChatCompletions' -count=1
go test ./internal/handler -run 'TestOpenAIStreamingHandlersWireAccountOnlyAdmissionBeforeFirstSemanticOutput|TestOpenAIForwardSucceededForScheduling' -count=1
go test ./internal/service -run '^$' -count=1
go build ./cmd/server
git diff --check
```

Additional direct checks passed:

```bash
go test ./internal/service -run 'TestOpenAIAdmissionSafetyIsIndependentOfGroupPolicy|TestOpenAIAdmissionRejectDoesNotFailOpen|TestOpenAISlowSessionGuardRequiresTrustedCompletedStream|TestOpenAIAdmissionFirstSemanticOutputReleasesIdempotently|TestHandleStreamingResponseGuardDoesNotNotifyWhenSemanticFlushFails' -count=1
go build ./cmd/server
gofmt -w internal/service/openai_account_scheduler_shared_health_test.go internal/service/openai_gateway_response_handling.go internal/service/openai_admission_first_output_wiring_test.go
git diff --check
```

The policy regression uses two policy fixtures and verifies that the same guarded account is rejected identically; the admission API receives only `account_id` and request shape. No production safety branch reads a group name, Pro label, or model.

The broader handler regex `OpenAI.*(Admission|FirstOutput|ChatCompletions|Responses|Messages)` has pre-existing failures on both this branch and root `main`: legacy assertions expect English messages while the current shared error mapper returns Chinese messages. Image-permission tests also reach an existing mock scheduling dependency gap. Those tests are outside T80's targeted admission paths and remain unchanged here.

## Release Notes

- Migration: none.
- Production configuration edit: none. This adds global optional admission configuration with defaults; no group-specific, model-specific, or Pro-specific knob is introduced.
- Expected `downtime_required=false`, pending the root merged-main release precheck.
- Rollback: set `gateway.openai_shared_health.admission_enabled=false`; if necessary, restore the previous verified blue-green image. Redis safety keys use TTL and require no data rollback.

## Residual Risk

- Global pre-output limits can reduce short-term throughput if thresholds are too restrictive.
- Redis errors intentionally fail open, so safety protection degrades while the shared store is unavailable.
- T80 covers eligible HTTP text streams only; WebSocket and non-streaming paths retain current behavior.
- Group-level quality attribution and the explanation that remaining ranking differences come from group policy stay deferred to frozen T76 after T80 is stable.
- Candidate status is ready for root review. No merge to root `main`, push, deployment, or production verification has occurred from this worktree.
