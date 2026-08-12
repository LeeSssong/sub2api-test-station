# Task 2 Fix Round 2 Report

Date: 2026-08-12

## Scope

- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/official-v0175-fast-merge`
- Branch: `codex/official-v0175-fast-merge`
- Starting candidate: `6690b282878e026ad45b34c74d057e36045f53a9`
- No `main` merge, push, deployment, production access, secrets, T03-R1, or
  other-worktree changes.

The scoped second review identified that `openAIRequestHasSideEffects` detected
Responses `input[].type=function_call_output` but did not detect an Anthropic
Messages continuation containing `messages[].content[].type=tool_result`.
Consequently, a pool-configured 401/403 could be replayed once on the same
account and then on a fallback account even though the request contained a
prior tool result.

## TDD Evidence

### RED

Before the helper fix, the new real handler regression was run against the
candidate behavior:

```text
GOFLAGS=-mod=mod go test ./internal/handler \
  -run '^TestOpenAIMessages_APIKeyPassthroughPoolAuthFailureWithToolResultNeverReplays$' \
  -count=1 -v
```

For both 401 and 403, it failed with actual upstream calls
`[9910, 9910, 9911]` instead of the required `[9910]`.

### GREEN

Implementation is split into two audit-friendly commits:

- `e4cb5363ed7d84451729e3909027c89520dac701` — Responses function-output and
  Messages tool-result regressions.
- `ada3a69dfb22bcdff3922549042afffbed6fba1a` — minimal side-effect recognition
  fix.

The helper now scans only array-valued `messages[].content[]` blocks for
`type=tool_result`; plain string Messages content remains safe and retains the
existing configured pool retry/fallback behavior. Responses function-output
recognition remains unchanged.

Focused verification on the implementation candidate passed:

```text
GOFLAGS=-mod=mod go test ./internal/handler \
  -run 'TestOpenAI(Responses_APIKeyPassthroughPoolAuthFailureWith(FunctionCallOutput|Tools)NeverReplays|Messages_APIKeyPassthroughPoolAuthFailureWith(ToolResult|Tools)NeverReplays|Responses_APIKeyPassthroughPoolAuthFailureRetriesThenSwitchesToHealthyAccount|Messages_APIKeyPassthroughPoolAuthFailureRetriesThenSwitchesToHealthyAccount|Responses_PostOutputFailureNeverReplays)$' \
  -count=1 -v
```

It proves both 401 and 403 cases: unsafe Responses function output and
Messages tool result call only `[9910]`; configured safe requests retain
`[9910, 9910, 9911]`; the existing post-output no-replay test also passes.

## Full Verification

All commands completed with exit code 0 after `ada3a69df`:

- `GOFLAGS=-mod=mod go test ./internal/handler -count=1`
- `GOFLAGS=-mod=mod go test ./internal/service -count=1`
- `GOFLAGS=-mod=mod go vet ./internal/handler ./internal/service`
- `pnpm vitest run src/components/admin/usage/__tests__/UsageTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts` — 2 files, 39 tests passed.
- `pnpm typecheck`
- `gofmt -d internal/handler/openai_gateway_handler.go internal/handler/openai_gateway_handler_test.go`
- `git diff --check`
- conflict-path and literal-marker checks.

The frontend commands emitted only pre-existing toolchain warnings about the
legacy `pnpm` package field, Node localStorage, and stale Browserslist data.

## Handoff

Task 2 remains **进行中** and pending independent scoped re-review. It is not
authorized for root merge, push, deployment, or online verification. No
migration or configuration change was introduced. `downtime_required` remains
for the root merged-main release preflight.
