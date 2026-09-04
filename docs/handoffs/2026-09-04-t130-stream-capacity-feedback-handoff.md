# T130 Stream Capacity Feedback Handoff

## Source

- Task: T130 stream capacity feedback
- Worktree: `.worktrees/t130-stream-capacity-feedback`
- Branch: `codex/t130-stream-capacity-feedback`
- Baseline commit: `9e4c708842fbf4ea2900225bc7f330902aa0364d`
- Primary implementation commit: `3bfd7e2fe`; the branch tip also contains the shared-health follow-up and this handoff update.
- Root `main` at final check: `b44708a624163ab3b19ee39cdec49f2b23c70db0`
- Root `main` tree at final check: `fad4355ae888588ddede5dea4757319072a2f29a`
- `origin/main` at final check: `b44708a624163ab3b19ee39cdec49f2b23c70db0`
- Candidate tip before the final handoff-only commit: `97aaaec81918e3e43828e8fd8b942eb8b60148d0`
- Status: `REFRESH_REQUIRED`; not pushed, merged, or deployed

The user stated that `main` was deploying and instructed this task not to touch it. Root `main` advanced during implementation and was clean and synchronized with `origin/main` at the final read-only check. T130 did not modify, stage, clean, merge, or deploy from root `main`.

## Changes

- Classifies bounded upstream capacity signals: pending requests, account concurrency, upstream rate limit, and temporary unavailability.
- Applies a 10-second future-request account/model cooldown for capacity failures even when output already started, without allowing current-request replay.
- Retains bounded 60-second, 5-minute, and 15-minute failure-window history across ordinary successes; explicit restore remains a hard clear.
- Preserves active cooldown when a concurrent successful request completes.
- Preserves the same active cooldown in the existing Redis shared-health projection.
- Records bounded capacity failure metadata in the existing resilience ledger.
- Retains 60-second no-first-output client cancellation as T114 right-censored slow evidence and emits `openai.client_abandoned_after_upstream_wait`.
- Does not count client abandonment as upstream failure, start cooldown, create another attempt, or alter billing/replay safety.

## Verification

Passed:

```text
go test ./internal/service -run 'Test(ClassifyOpenAIUpstreamFailure|OpenAIModelTransient|RecordOpenAIAccountModelFailure_Capacity|RecordOpenAIAccountModelSuccessRetains|OpenAIFirstOutputSlow|OpenAIUnifiedQualityScoreUsesSlowEvidence|OpenAIAccountScheduler.*(Shared|Cooldown|Adaptive))' -count=1
go test ./internal/repository -run 'TestOpenAISharedHealth' -count=1
go build ./cmd/server
git diff --check
```

The broad `go test ./internal/service ./internal/handler` run was not green. It exposed pre-existing failures unrelated to T130, including Codex fingerprint seed lifecycle, account-monitor evidence projection, channel-monitor URL validation, localized handler error text assertions, and image permission/concurrency text assertions. T130-focused service, handler, shared-health, scheduler, quality, classifier, and build checks passed.

## Scope And Safety

- Migrations: none
- Configuration changes: none
- Public API changes: none
- Credentials or secrets: none
- Runtime or customer data: none
- GitHub Actions: none
- Production/test-station writes: none
- Expected deployment downtime: false; deployment preflight remains authoritative

## Refresh Requirement

Before integration, fetch and rebase/cherry-pick the candidate onto the then-current clean `origin/main`, resolve any conflict with the deployment changes, and rerun the focused tests and build. Integration and any deployment must occur only from clean root `main` after it exactly matches pushed `origin/main`, subject to the existing authorization and downtime gates.

## Rollback

Before deployment, discard the candidate branch. After integration, use explicit revert commits for the T130 changes on `main`, push them, then run the reviewed release chain from clean synchronized `main`.

## Unverified

- No live environment deployment or post-deployment verification was performed.
- No remote branch was pushed.
- Acceptance-station and production commit/tree values were not changed or re-read during implementation.
