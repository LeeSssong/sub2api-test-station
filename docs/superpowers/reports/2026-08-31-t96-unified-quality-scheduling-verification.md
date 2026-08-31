# T96 Unified Quality Scheduling Verification

## Status

`READY_FOR_ROOT_REVIEW`

This candidate is implemented and directly functionally verified. It has not been merged into the root `main`, pushed, deployed, or used to modify production data.

## Identity and refresh

- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t96-unified-quality-scheduling`
- Branch: `codex/t96-unified-quality-scheduling`
- Refresh base: clean root `main == origin/main == 7d6c4a178d394b756e8c6428a4508e7230b4a73c`
- Refresh base tree: `262776dba20cde897ca446ab30252b6577069684`
- Candidate HEAD: `509b631dc194108397c7ea393508738db888abe8`
- Candidate tree: `eda9659f93307b78819d656dc9a44ccbdd79c823`
- Candidate status: clean after committing this report; no staged or untracked files
- Root status: clean, `main == origin/main`

The candidate was refreshed inside its own worktree and retains all T96 commits plus the current root changes. The preserved historical evidence worktree `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t96-group-account-baseline` was not modified.

The final opt-in boundary correction constructs the Responses selection context through `openAIUnifiedQualityContextForResponses` exactly once. Ordinary HTTP text opts in; Responses image intent, Responses WebSocket, and alpha-search do not. This avoids accidentally carrying the marker into image scheduling.

## Delivered scope

- Added the seven-day `usage_logs` account-quality projection and a 60-second stale-tolerant snapshot provider.
- Added deterministic ordinary OpenAI text ordering: success rate descending, trimmed-mean TTFT ascending, live T95 effective cost `U` ascending, account ID ascending; nullable values are last.
- Kept model use at capability qualification only; image requests continue through the native image selector and retry path.
- Reused native Groups profit controls and T95 `EffectiveCostForAccount`; qualified candidates are preferred and the full eligible pool is used as availability-first fallback when none qualify.
- Added group-scoped `extra_retry_count` (`0..3`) and recovery accounting based only on different-account attempts that actually enter `Forward` after the first attempt.
- Disabled ordinary same-account replay only in unified text mode; replay remains restricted to explicitly safe, unbilled, no-output, no-side-effect failures with known billing state.
- Kept Sub native account slots, queue, timeout, cancellation, and release as the only account-concurrency mechanism. No admission, slow-session, or account-level custom limiter was added.
- Preserved T105 OAuth 429 native cooldown/stop semantics in the unified path and prevented the legacy one-shot OAuth group reset from bypassing the T96 budget.
- Added non-sensitive scheduler decision/event fields and reduced the scheduler settings page to the global switch plus `extra_retry_count`, while preserving legacy policy JSON on save.

## Direct functional verification

All commands were run after the latest-main refresh, from the candidate worktree, with `GOPROXY=https://goproxy.cn,direct` for Go commands where needed.

| Area | Command | Result |
| --- | --- | --- |
| Unified quality/profit/scheduler service | `go test -vet=off ./internal/service -run 'TestOpenAI(AccountQuality\|UnifiedQuality\|Profit\|AccountScheduler)\|TestRecordOpenAISchedulerSelectionProjectsUnifiedQualityDecision\|TestPersistOpenAIOAuth429Cooldown\|TestShouldStopOpenAIOAuth429Failover' -count=1` | PASS |
| Quality SQL/repository | `go test -vet=off ./internal/repository -run 'TestUsageLogRepository(ListOpenAIAccountQuality\|_ListOpenAIAccountQuality)\|TestOpenAIAccountQualityQuery' -count=1` | PASS |
| Unified retry/OAuth helper | `go test -vet=off ./openai_retry_budget.go ./openai_retry_budget_test.go -run 'TestOpenAIUnifiedOAuth429KeepsNativeCooldownAndStopSemantics\|TestOpenAIUnifiedModeSkipsLegacyOAuth429GroupReset\|TestOpenAIUnified\|TestOpenAIRetryBudget' -count=1` | PASS |
| Server compile | `go build ./cmd/server` | PASS |
| Admin scheduler UI | `vitest run src/views/admin/__tests__/SchedulerSettingsView.spec.ts --reporter=dot` | 4/4 PASS |
| Frontend types | `pnpm typecheck` | PASS |
| Native-only source guard | `ops/assert-native-openai-concurrency-only.sh --worktree /Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t96-unified-quality-scheduling` | PASS |
| Formatting and whitespace | `gofmt -d ...` and `git diff --check` | PASS |

The frontend typecheck rewrote dependency metadata in `pnpm-lock.yaml`; that unrelated drift was restored to the root-main version before final status verification. The final candidate worktree is clean.

## Known verification limits

- A full `internal/handler` package test cannot compile on this repository baseline because `handler_wiring_test.go` calls the older `ProvideHandlers` arity and `openai_gateway_handler_test.go` references the missing `openAIAccountScheduleModel`. The isolated T96 retry/OAuth helper tests pass; no new T96 production compile error was observed.
- The focused `internal/handler` package command remains blocked by pre-existing baseline test compile defects (`handler_wiring_test.go` has an older `ProvideHandlers` arity and `openai_gateway_handler_test.go` references missing `openAIAccountScheduleModel`). The new boundary test is present and statically checks the exact entrypoint composition; service/repository/helper coverage and production handler compilation/build passed.
- Acceptance-station behavioral checks and production deployment were not run in this task worktree. They require root release-control authorization and must be performed only after this candidate is merged to a clean, pushed root `main`.
- No account-pool, priority, status, schedulable, concurrency, profit configuration, or production-data write was performed here. The historical ranking report remains planning evidence, not a runtime mutation.

## Migration, configuration, and release attributes

- T96 introduces no SQL migration, table, column, or new quality fact source.
- `extra_retry_count` is stored in the existing scheduler settings JSON and defaults to `0` when absent.
- Existing native Groups `profit_min_margin` and `profit_safety_buffer` remain the administrator-managed source of profit thresholds; T96 does not duplicate them.
- Expected `downtime_required`: `false`, subject to the root release preflight. No release preflight was run from this candidate.
- No GitHub Actions workflow or release path was added.

## Rollback

- Runtime rollback: set `openai_advanced_scheduler_enabled=false` through the existing settings path; ordinary requests return to the prior scheduler path while legacy policy JSON and account relationships remain intact.
- Code rollback: restore the previous verified root commit/image through the existing root-only release chain.
- Do not change or delete account-group relationships as part of rollback.

## Root handoff

The root release controller may now review candidate `509b631dc194108397c7ea393508738db888abe8` against `main@7d6c4a178d394b756e8c6428a4508e7230b4a73c`. If approved, the next actions are root-only: merge to a clean root `main`, run the root direct gates, push `origin/main`, and follow the acceptance-station/main-site authorization rules. This report does not authorize merge, push, deployment, or production verification.
