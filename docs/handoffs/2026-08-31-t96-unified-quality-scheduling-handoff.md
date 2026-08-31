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
