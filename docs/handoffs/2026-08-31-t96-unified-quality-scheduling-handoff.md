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
