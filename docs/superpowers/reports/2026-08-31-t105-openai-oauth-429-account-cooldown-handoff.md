# T105 Handoff

- Task: T105 OpenAI OAuth 429 account-level native cooldown
- Workspace: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t105-openai-429-account-cooldown`
- Branch: `codex/t105-openai-429-account-cooldown`
- Baseline: `main@5e6ccee143f07ee34017c25e75979b74b6bcfc77`
- Prior implementation commit: `d267d298abd2dbdb5ba50f48948dc9d6a58dbcd6`
- Review-fix candidate: committed after official-five-second fallback correction and review fixes
- Status: `READY_FOR_ROOT_REVIEW`
- Scope: account-level OpenAI OAuth/SetupToken only; no model-level cooldown.

## Changes

- Updated `PersistOpenAIOAuth429Cooldown` to use the official five-second fallback and require an explicit HTTP 429 status.
- Uses optional `SetRateLimitedIfLater` when available, preserving a later existing reset; synchronizes the runtime blocker and clears the request retry marker after persistence.
- Calls the helper at same-account retry exhaustion and account-switch transitions across Responses, Messages, Chat Completions, WebSocket, Images, Alpha Search, Grok media, Embeddings, and Codex Models manifest handlers.
- Adds one bounded group recovery pass after transient-429 candidate exhaustion. It refreshes the authoritative group account list, clears only excluded short T105 cooldowns (including the current account) and request-local exclusions, while retaining native 7d quota, disabled accounts, credential failures, and other durable state.
- Replaces duration-based cooldown detection with an in-process T105 fallback observation plus an atomic repository clear-by-reset operation; a reliable `Retry-After: 5` is not marked as T105 fallback.
- Restores only the cleared account IDs from continuation exclusions, preserving unrelated model/session exclusions; handles both selection errors and nil selections.
- Kept capacity-shed, image-specific rate limit, model-not-found, non-OAuth 429, Spark shadow, and request-scoped `failedAccountIDs` behavior unchanged.

## Verification

- `gofmt` passed on all changed Go files.
- `git diff --check` passed.
- `go build ./cmd/server` passed from `upstream/sub2api/backend`.
- Isolated T105 service tests passed with production source files plus the existing Gemini repository test helper: `TestPersistOpenAIOAuth429Cooldown*` and `TestRefreshOpenAIOAuth429Group*`.
- Verified a reliable `Retry-After: 5` reset is not misclassified as the T105 fallback.
- Isolated T105 service tests and server build pass after the review fixes.
- Full `go test -tags=unit ./internal/service ./internal/handler` is blocked by pre-existing test compilation errors unrelated to T105: missing `context` imports in `gateway_forward_as_chat_completions_test.go`, stale `ProvideHandlers` arguments in `handler_wiring_test.go`, and missing `openAIAccountScheduleModel` symbols in `openai_gateway_handler_test.go`.

## Release gate

- No migration or configuration changes.
- No production data or credentials touched.
- Must refresh this candidate to the latest `main` before root review if `main` advances.
- Root merge, push, deployment, and online verification remain pending root-controller authorization.
- Candidate working tree is clean after the review-fix commit; no production deployment has been performed for this review-fix candidate.
- Previous T98 deployment-lane notes are historical; this candidate must still be re-reviewed against the current root `main` before any release.
- Rollback: use the release chain's previous validated slot after a root `main` revert or forward-fix commit.
