# T105 Handoff

- Task: T105 OpenAI OAuth 429 account-level native cooldown
- Workspace: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t105-openai-429-account-cooldown`
- Branch: `codex/t105-openai-429-account-cooldown`
- Baseline: `main@5e6ccee143f07ee34017c25e75979b74b6bcfc77`
- Implementation commit: `7afeccae221e47bc9e6ebc537e02ba7cffd5136b`
- Implementation tree: `1ff0f56bf78fdf68e6477a8b287b2f0fa0bf0321`
- Status: `READY_FOR_ROOT_REVIEW`
- Scope: account-level OpenAI OAuth/SetupToken only; no model-level cooldown.

## Changes

- Added `PersistOpenAIOAuth429Cooldown`, using reliable upstream reset when present and a fixed five-minute fallback otherwise.
- Uses optional `SetRateLimitedIfLater` when available, preserving a later existing reset; synchronizes the runtime blocker and clears the request retry marker after persistence.
- Calls the helper at same-account retry exhaustion and account-switch transitions across Responses, Messages, Chat Completions, WebSocket, Images, Alpha Search, and Grok media handlers.
- Adds one bounded group recovery pass after transient-429 candidate exhaustion. It refreshes the authoritative group account list, clears only excluded short T105 cooldowns (including the current account) and request-local exclusions, while retaining native 7d quota, disabled accounts, credential failures, and other durable state.
- Kept capacity-shed, image-specific rate limit, model-not-found, non-OAuth 429, Spark shadow, and request-scoped `failedAccountIDs` behavior unchanged.

## Verification

- `gofmt` passed on all changed Go files.
- `git diff --check` passed.
- `go build ./cmd/server` passed from `upstream/sub2api/backend`.
- Isolated T105 service tests passed with production source files plus the existing Gemini repository test helper: `TestPersistOpenAIOAuth429Cooldown*` and `TestRefreshOpenAIOAuth429Group*`.
- Full `go test -tags=unit ./internal/service ./internal/handler` is blocked by pre-existing test compilation errors unrelated to T105: missing `context` imports in `gateway_forward_as_chat_completions_test.go`, stale `ProvideHandlers` arguments in `handler_wiring_test.go`, and missing `openAIAccountScheduleModel` symbols in `openai_gateway_handler_test.go`.

## Release gate

- No migration or configuration changes.
- No production data or credentials touched.
- Must refresh this candidate to the latest `main` before root review if `main` advances.
- Root merge, push, deployment, and online verification remain pending root-controller authorization.
- Candidate working tree is clean after the implementation commit; this handoff update is the only subsequent documentation change.
- T98 currently occupies the sole `DEPLOYING` lane and is stopped at `downtime_required=true` / `migration_set_changed`; T105 cannot deploy until that lane is resolved and any required downtime authorization is obtained.
- Rollback: use the release chain's previous validated slot after a root `main` revert or forward-fix commit.
