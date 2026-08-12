# Task 2 Fix Round 3 Report

Actual work date: 2026-08-12. The existing directory name is retained as a
stable task-package path and is not an assertion about the actual date.

Status: the third independent scoped review was `REJECT`. Local implementation
and all required verification gates passed. The candidate remains `进行中`
pending a new scoped independent review. No merge to `main`, push, deployment,
production access, or online verification was performed.

## Scope and Identity

- Branch: `codex/official-v0175-fast-merge`
- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/official-v0175-fast-merge`
- Exact starting HEAD: `ce4e2d320701dfa9fde58b024a005b208d22a766`
- Production fix and symmetric regressions:
  `d3fc44fa88359ab110897908e7a987e608df54c0`
- Explicit protocol-selection test cleanup:
  `731dedc641c6c2100848813212cb1d760994ed2b`
- Pre-existing draft backup retained unchanged at
  `.superpowers/sdd/2026-08-13-official-v0175-fast-merge/root-review/fix-round-3-draft.patch`.
- Changed runtime surface: Responses and Messages failover loops only.
- Changed test surface: handler regressions and their local account-repository
  fixture only.
- Migration change: none.
- Configuration change: none.

## Finding and Root Cause

Both endpoint loops stopped at
`retryDecision.NoRetry && !poolReplaySafe` before evaluating
`ShouldRetryNextAccount()`. `poolReplaySafe` included
`RetryableOnSameAccount`, so the code incorrectly treated absence of a
configured same-account pool retry as semantic replay unsafety. Safe non-pool
401/403 requests, and pool accounts whose status override excluded 401/403,
therefore stopped after the primary account instead of reaching the healthy
fallback.

The correction separates the two decisions:

- `semanticReplaySafe` is true only before output and without known tool or
  other side effects.
- `poolRetryEligible` additionally requires the configured same-account pool
  retry status.
- A hard `NoRetry` response terminates only when semantic replay is unsafe.
  Otherwise the existing `ShouldRetryNextAccount()` contract decides whether
  to switch accounts.

The same change was made symmetrically in Responses and Messages. Configured
same-account retry counts and status-code limits were not changed.

## TDD RED

Before the production change, the new real-handler regressions were run on the
exact starting candidate:

```text
GOFLAGS=-mod=mod go test ./internal/handler \
  -run 'TestOpenAI(Responses|Messages)_SafeAuthFailureSwitchesWithoutSameAccountPoolRetry$' \
  -count=1
```

Result: expected FAIL. All eight cases — Responses/Messages × non-pool or
pool-status-override × 401/403 — expected `[9910, 9911]` but actually called
only `[9910]`. This was a compiled behavioral failure, not a build or fixture
syntax failure.

## Focused GREEN Matrix

The final explicit matrix command was:

```text
GOFLAGS=-mod=mod go test ./internal/handler \
  -run 'TestOpenAI(Responses_SafeAuthFailureSwitchesWithoutSameAccountPoolRetry|Messages_SafeAuthFailureSwitchesWithoutSameAccountPoolRetry|Responses_APIKeyPassthroughPoolAuthFailureRetriesThenSwitchesToHealthyAccount|Messages_APIKeyPassthroughPoolAuthFailureRetriesThenSwitchesToHealthyAccount|Responses_APIKeyPassthroughPoolAuthFailureWith(FunctionCallOutput|Tools)NeverReplays|Messages_APIKeyPassthroughPoolAuthFailureWith(ToolResult|Tools)NeverReplays|Responses_PostOutputFailureNeverReplays|Messages_TransientFailureRetriesOnceThenFailsOver)$' \
  -count=1
```

Result: PASS (`ok github.com/Wei-Shaw/sub2api/internal/handler 3.945s`). The
observable call sequences were:

- safe non-pool 401/403: `[9910, 9911]`
- pool status override excluding 401/403: `[9910, 9911]`
- configured pool retry including 401/403: `[9910, 9910, 9911]`
- Responses tools/function-call-output and Messages tools/tool-result:
  `[9910]`
- post-output failure: `[9930]`
- existing Messages 502/transient retries and failover remain green

Both safe fallback endpoint responses contained semantic text `ok`.

The non-pool fixtures exercise the real `RateLimitService` account-state path.
The handler account-repository test double therefore implements no-op
`SetError` and `SetTempUnschedulable` methods, allowing the real auth side
effect to complete without mutating the in-memory backup account.

## Required Verification

- `GOFLAGS=-mod=mod go test ./internal/handler -count=1`
  - PASS on the final source/test tree:
    `ok github.com/Wei-Shaw/sub2api/internal/handler 39.944s`
- `GOFLAGS=-mod=mod go vet ./internal/handler`
  - PASS; no output.
- `gofmt -d internal/handler/openai_gateway_handler.go internal/handler/openai_gateway_handler_test.go`
  - PASS; no output.
- `git diff --check`
  - PASS; no output.

## Release Boundary

- No `main` change, merge, push, deployment, production or secret access.
- No T03-R1 or other worktree change.
- No GitHub Actions change or use.
- No migration or configuration change.
- `downtime_required` remains deferred to the root task's reviewed merged-main
  release preflight.
- Remaining gate: a new scoped independent review of the final candidate,
  followed by the root whole-branch review and merged-main release gates.
