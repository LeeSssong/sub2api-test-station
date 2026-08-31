# T96 Unified Quality Scheduling Handoff

## Recovery checkpoint (2026-08-31)

- State: `IMPLEMENTING`; dependency recovery and baseline refresh are complete, and no runtime implementation has started.
- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t96-unified-quality-scheduling`
- Branch: `codex/t96-unified-quality-scheduling`
- Current HEAD/tree after safe refresh: `5e6ccee143f07ee34017c25e75979b74b6bcfc77` / `42dda8e317725a710340b5624bbda887cd1f6a50`
- Refresh target: reached exactly; root `main == origin/main` at the refresh check.
- Worktree status after refresh: only the four task-local baseline documents are untracked; no tracked or staged runtime changes.
- Running release/deploy/migration command: none observed. No production or acceptance-station action was performed.

## Goal and approved contract

- Ordinary non-image OpenAI requests use account-level deterministic ordering: success rate descending, trimmed-mean TTFT ascending, live effective cost `U` ascending, account ID ascending.
- Group membership is a native, administrator-maintained read-only pool. No automatic group, priority, status, schedulable, concurrency, or production-data mutation.
- Pro/专属 Pro, Plus, and 特惠 permit 1/2/3 extra cross-account Forward attempts respectively. Ordinary text never retries the same account.
- Account switching is allowed only for an unbilled, no-output, no-side-effect, explicitly `safe_to_replay` attempt with billing state known.
- Native Sub account slots, queue, timeout, cancellation, and release remain the only concurrency controls. T103 is abandoned, but native-only is permanent; no admission/slow-session/custom account limiter may be reintroduced.
- Image requests stay on the native image selector and retry path.

## Dependency evidence

T95 is now merged in the required root baseline. The only consumed contract is:

```go
type EffectiveCost struct {
    Model  string
    Status string
    A      *float64
    R      *float64
    U      *float64
}

func EffectiveCostForAccount(account *Account) EffectiveCost

const EffectiveCostStatusReady = "ready"
```

`U` must be read live during candidate construction and post-slot recheck; it must not be stored in the quality snapshot. T95 models are direct API-key multiplier, ratio-based API-key upstream, and self-owned `oauth`/`setup-token` without an upstream return multiplier.

## Completed / not completed

Confirmed complete:

- Full recovery read of `AGENTS.md`, the native incremental-delivery constraints, task queue, acceptance-station constraints, T96 spec/plan/ranking report, T95 spec/plan/report/handoff, and recent root-thread metadata.
- Root and candidate worktree/status audit; old evidence worktree remains preserved and untouched.
- T95 contract symbols verified in the candidate's merged source tree.
- Candidate safely fast-forwarded from `d8f75dbfc` to `main@5e6ccee14` while preserving task-local documents.

Not complete:

- Commit of the T96 baseline artifacts in the refreshed candidate.
- All Tasks 2-8 implementation, direct tests, verification report, and final handoff.
- Acceptance-station behavior checks, deployment, merge, push, and production verification (root-only actions).

## Risks / stop conditions

- If root `main` moves again or the target tree differs, stop and recalculate the refresh base; do not guess or cherry-pick stale code.
- Preserve all local changes during refresh; never reset, clean, overwrite, merge, push, deploy, migrate, restart, or touch production from this worktree.
- T96 has no SQL migration and must not introduce admission, slow-session, or account-level extra concurrency fields.
- Historical account ranking is planning evidence only; no production account pool or priority is changed by this task.

## Next executable action

Commit the four T96 baseline artifacts, then begin Task 2 with RED tests for `extra_retry_count`.

## Recovery checkpoint (post-interruption, 2026-08-31)

- The previous settings inspection was interrupted before any source edit. No command is running and no release/deploy/migration action was started.
- Candidate remains clean except for the committed baseline; `HEAD=3b6a657d8b4485a9f17921ff8a23a14847527f1a`, tree `303c71fcd957ad1d7faf395c6e5d083718cef86a`, branch `codex/t96-unified-quality-scheduling`.
- Root `main` and `origin/main` still both resolve to `5e6ccee143f07ee34017c25e75979b74b6bcfc77`, tree `42dda8e317725a710340b5624bbda887cd1f6a50`; candidate is one local baseline commit ahead and has no divergence from the root base.
- The current queue records T105 as an independent `IMPLEMENTING` candidate and T103 as `ABANDONED`; neither permits changes to this worktree's scope. Native-only account-slot semantics remain mandatory.
- T95 dependency is confirmed in the candidate source (`EffectiveCostForAccount`, `EffectiveCost`, and `EffectiveCostStatusReady`). No T96 runtime tests or implementation files exist yet.
- Next action: add only the planned Task 2 RED tests, run the focused test command to capture the expected failure, then implement the minimal settings field/validation. Stop and update this handoff before any later task or interruption.

## Recovery checkpoint (post-test interruption, 2026-08-31)

- The two first RED test sessions were interrupted while the Go toolchain was running; both session IDs are now gone and `ps` shows no matching `go test` process. Their output was not retained, so no RED result is claimed.
- Current candidate remains on branch `codex/t96-unified-quality-scheduling`, `HEAD=3b6a657d8b4485a9f17921ff8a23a14847527f1a`, tree `303c71fcd957ad1d7faf395c6e5d083718cef86a`, with only the intended RED-test edits plus this handoff modification.
- Root `main`/`origin/main` remain `5e6ccee143f07ee34017c25e75979b74b6bcfc77` / `42dda8e317725a710340b5624bbda887cd1f6a50`; no root or release state was changed.
- Task 2 tests now cover parser/JSON round-trip, legacy policy field preservation, and invalid `-1/4/1.5` values. Production settings code has not been changed.
- Next action: rerun the two focused RED commands one at a time with a bounded wait, capture the actual expected compile/test failure, then make the minimal implementation. No later task or publish action is authorized from this worktree.

## Task 2 RED evidence (2026-08-31)

- `go test ./internal/service -run 'TestOpenAISchedulerGroupPolicyExtraRetryCountRoundTripAndValidation' -count=1` failed at compile time because `OpenAISchedulerGroupPolicy.ExtraRetryCount` is not defined.
- `go test ./internal/handler/admin -run 'TestSettingHandlerSchedulerExtraRetryCountRoundTripsWithLegacyPolicyFields' -count=1` failed because the stored policy omitted `extra_retry_count` (`actual <nil>`), confirming the handler/parser path does not yet persist the field.
- No production settings code has been edited yet. Next action is the minimal struct/parser/normalization/frontend contract implementation for Task 2.

## Task 2 completion checkpoint (2026-08-31)

- Implemented `OpenAISchedulerGroupPolicy.ExtraRetryCount` as an optional JSON `extra_retry_count` integer.
- Added strict `0..3` validation for parser and both normalized write paths, legacy JSON parsing support, a read-side bound normalizer, and `resolveOpenAIExtraRetryCount` with a missing-value default of `0`.
- Preserved existing priority, operations, compiled snapshot, weights, fairness, presets, and unrelated policy keys through the native custom JSON round-trip.
- Added frontend `OPENAI_SCHEDULER_LIMITS.extraRetryCount` (`0..3`, step `1`) and API type coverage.
- RED/GREEN evidence: service Task 2 tests pass; handler round-trip/invalid-input tests pass; existing scheduler normalization focused tests pass; frontend `pnpm typecheck` passes. `git diff --check` passes. The first typecheck generated a lockfile-only dependency rewrite, which was removed after verification; no lockfile change remains.
- Task 2 changes are uncommitted at this checkpoint. Next action is commit the Task 2 files, then start Task 3 with the quality SQL RED test.

## Task 2 commit and Task 3 RED checkpoint (2026-08-31)

- Task 2 is committed as `be31cd869` on the isolated branch; its focused service/handler tests and frontend typecheck passed.
- Task 3 RED test file `internal/service/openai_account_quality_test.go` is present. `go test ./internal/service -run 'TestOpenAIAccountQualitySnapshotProvider' -count=1` fails as expected because the quality types/provider do not yet exist.
- Repository RED tests are present in `internal/repository/usage_log_quality_test.go`. The focused repository command fails as expected because `ListOpenAIAccountQuality`, `OpenAIAccountQuality`, and `openAIAccountQualityQuery` do not yet exist.
- No Task 3 production code, schema migration, or runtime configuration has been written. Next action is the minimal read-only repository query and 60-second stale snapshot provider, followed by constructor wiring.

## Task 3 implementation checkpoint (2026-08-31)

- Added `OpenAIAccountQuality`, the narrow read-only repository interface, and the 60-second mutex/singleflight snapshot provider. Refresh failures return the last successful snapshot with `Stale=true`; cold-start failures return an empty stale snapshot and never block routing. U is intentionally not present in the snapshot.
- Added `usageLogRepository.ListOpenAIAccountQuality` with a usage-ledger-only CTE: physical-attempt deduplication, complete/positive-cost success definition, image/video exclusion, independent five-percent trimmed means, and nullable metrics.
- Wired the provider into `OpenAIGatewayService` only when the existing usage repository implements the narrow interface; absent repositories remain non-blocking.
- RED/GREEN evidence: repository focused tests pass with `-vet=off`; service provider tests pass with `-vet=off`. The normal repository command is currently blocked by a pre-existing `fmt.Sprintf` vet diagnostic in `usage_log_repo_stats.go:1004`, unrelated to this change. `git diff --check` passes.
- Root `main` advanced from the T96 base to `43ffa2353` (T105 OAuth 429 account cooldown) while this task was in progress; candidate must be refreshed to that latest main before READY_FOR_ROOT_REVIEW. T105 handler changes must be preserved.
- Next action: commit Task 3, fast-forward/merge the latest root main into this candidate without discarding local commits, resolve only real T96/T105 overlap, then continue Task 4.

## Post-refresh checkpoint (2026-08-31)

- Task 3 is committed as `6182893a8`; the candidate was safely merged with the latest root `main@43ffa2353` using a local merge commit `52dc7a0e6`. No conflicts occurred and all T105 changes were retained.
- Candidate HEAD/tree: `52dc7a0e69a007bb957c46c3980a23716d89adaa` / `663446a656cad0468818dce26afce29aa68fd62a`; branch `codex/t96-unified-quality-scheduling`; worktree is clean.
- Root `main`/`origin/main` at refresh: `43ffa2353ed96da668d0846753f472fea922d07d`, clean and equal. Candidate is ahead only by its T96 commits/merge and is not pushed.
- T105 is an independent root task now in the candidate history; T96 must preserve its native rate-limit block behavior while changing only approved unified text ordering/recovery later.
- Task 3 focused tests pass with `-vet=off`; the repository package's ordinary vet command remains blocked by the pre-existing `usage_log_repo_stats.go:1004` format diagnostic.
- Next action: begin Task 4 RED tests for the pure deterministic comparator and image-path bypass, then implement only after observing failure.

## Task 4 RED checkpoint (2026-08-31)

- Pure comparator tests initially failed because the unified candidate type/sort function did not exist; after the minimal comparator was added, all comparator tests pass with `-vet=off`.
- Selector/image routing tests now fail at compile time because the unified selection layer constant is not yet defined; this is the expected pre-integration RED state.
- No existing scheduler behavior has been changed yet. Next action is to implement the unified ordinary-text selector using native account listing, eligibility, live T95 U, and native slot acquisition, then wire it after protocol bindings and before ordinary sticky.

## Context recovery checkpoint (2026-08-31, resumed)

- Authoritative worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t96-unified-quality-scheduling`.
- Branch/HEAD/tree: `codex/t96-unified-quality-scheduling` / `52dc7a0e69a007bb957c46c3980a23716d89adaa` / `663446a656cad0468818dce26afce29aa68fd62a`.
- Root refs observed: local `main=b13980c980354739bf2a5feaecfd2664f6bf88a4`, `origin/main=1827057cb9dc1f9266da9b4f659af6045fa88e2e`; root has one local T98 integration commit and is not to be modified by this task. The candidate is intentionally based on an earlier main and must be refreshed only in this worktree before READY_FOR_ROOT_REVIEW, preserving all local changes.
- Worktree state: modified tracked files are this handoff plus the Task 4-6 handler/service files; untracked files are `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go` and its test. No staged files, no resets/cleanups, and no files outside this worktree were changed by this checkpoint.
- Runtime/implementation truth: Tasks 2 and 3 are committed (`be31cd869`, `6182893a8`); Task 4-5 selector/profit changes and Task 6 retry-budget/handler changes are present but uncommitted. Task 7 UI/events and Task 8 report/final handoff remain unfinished.
- Direct evidence already available: comparator/selector/image, profit partition/fallback, live-U slot recheck, and retry-budget focused tests passed in prior local runs; the normal handler package still has unrelated baseline compile failures, so only narrow feature coverage will be rerun. No claim is made for a full package or full-repository pass.
- Dependency: merged T95 `EffectiveCostForAccount`, `EffectiveCost`, and `EffectiveCostStatusReady` are present in the candidate history. T103 is abandoned; native-only Sub account-slot semantics remain permanent.
- Concurrent command check: one independent `tests/operations/deploy_sub2api_blue_green_host_test.sh` fixture process was observed from the root thread; no T96 release/deploy/migration command is running. This worktree will not stop or alter that process.
- Next executable action: inspect the uncommitted Task 4-6 diff against the approved spec, add only the missing direct functional coverage (especially handler safety/slot semantics), then run bounded focused tests and finish Task 7/8. Stop before any merge, push, deployment, production, queue, or project-ledger change.

## Implementation checkpoint (2026-08-31, focused validation)

- Task 4-6 implementation is now present in the candidate: deterministic account-level text selector, native-profit availability fallback, live-U post-slot check, group extra-retry budget, native-slot retry-next handling, and no ordinary same-account replay in unified text paths (Responses, Messages, Chat Completions, Embeddings).
- Task 7 direct surface is present: scheduler settings page exposes only `extra_retry_count` 0–3, preserves legacy policy JSON on save, and the existing bounded resilience event ledger carries unified selection/profit/quality/recovery fields without credentials.
- Additional correctness fix: quality SQL physical-attempt deduplication now includes `account_id` before `api_key_id` and the attempt identity, preventing cross-account request IDs from collapsing.
- Fresh direct evidence: service T96 focused tests passed; repository quality SQL/scan tests passed; all handler production Go files compile with `go test <production files> -run '^$'`; frontend `SchedulerSettingsView.spec.ts` passed 4/4; `vue-tsc --noEmit` passed; `git diff --check` passed. The isolated handler budget test command is currently environment-blocked by a missing `go.opentelemetry.io/otel/metric@v1.43.0` download timeout, not a reported code failure.
- Root release state has since settled at clean `main == origin/main == fde3ece1b6e20a9e0b6a7ff47bf1e0be03213178` (tree `3c7a8c6d85d18d9c3ecb1a40dd3efaeab95315ad`). Candidate remains intentionally unrefreshed until its implementation commit is recorded; no root files, queue, project ledger, deployment, or production state were changed here.
- Next executable action: commit the candidate implementation/UI/tests, then run the smallest remaining direct handler budget check and prepare the T96 verification report. Before `READY_FOR_ROOT_REVIEW`, refresh this worktree to the then-current clean root `main` while preserving all commits and rerun only direct coverage.

## Recovery checkpoint (2026-08-31, narrowed verification)

- Authoritative worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t96-unified-quality-scheduling`.
- Branch/HEAD/tree: `codex/t96-unified-quality-scheduling` / `c4c81662f900207b668ffe318f58cdcc72135517` / `dcb884a9ec088548b76952e54958ef3472931369` before committing the pending compatibility patch.
- Root refs: `main == origin/main == fde3ece1b6e20a9e0b6a7ff47bf1e0be03213178`, tree `3c7a8c6d85d18d9c3ecb1a40dd3efaeab95315ad`; root worktree is clean. No release, deploy, migration, or production command is running.
- Worktree changes: only five handler files contain the pending T105 OAuth 429 compatibility patch; the transient frontend lockfile rewrite was restored and is no longer modified. No staged files or untracked files remain.
- Verified complete: T96 implementation/UI commits are present; the OAuth helper has focused coverage; service and repository focused tests, `go build ./cmd/server`, scheduler settings Vitest (4/4), `pnpm typecheck`, gofmt, and diff-check passed.
- Known verification limit: complete `internal/handler` package tests cannot compile because existing `handler_wiring_test.go` calls the old `ProvideHandlers` arity and `openai_gateway_handler_test.go` references missing `openAIAccountScheduleModel`. This is recorded as an environment/baseline gap, not a T96 test failure.
- Dependency: T95 effective-cost contract is present in this candidate history. T103 is abandoned; native Sub account-slot-only semantics remain mandatory.
- Pending: commit the five-file OAuth compatibility patch, safely merge the current root `main` into this candidate, rerun the narrowed functional coverage, create the verification report, and mark this handoff `READY_FOR_ROOT_REVIEW`. Do not modify root `main`, global queue, project ledger, deploy, push, or production.
- Stop/rollback: if refresh conflicts or root moves, stop and record the new SHA; preserve all candidate commits and local changes. Code rollback remains the existing scheduler feature switch/previous verified commit.
