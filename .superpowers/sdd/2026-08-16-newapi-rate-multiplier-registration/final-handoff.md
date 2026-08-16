# T13 Final Handoff

## Candidate

- Branch: `codex/newapi-rate-multiplier-registration`.
- Approved refreshed baseline: `45de05dffa560f8d2f92695258d4928e6d18ac34`.
- Final refreshed main baseline:
  `0fc6bb9c7f6535796a11eb10759bf53945c5ff89`.
- Pre-Task-2/3 tip: `0730603c806779ebdedff9ed18ecac6be4134f4a`.
- Implementation commit:
  `83e84a780452f337b075e378348d50a7f2cd86b9`.
- Implementation tree: `faa8c12b595b85d6d2afaf961b60694740c833e4`.
- Conflict-free refresh merge:
  `7282056938458c52f7b638edbf8764670b6176b1`.
- Refresh merge tree: `84de9b7f3cfc9f2d8da0048903e996fe4e1002ef`.
- Final handoff commit: the docs-only branch tip containing this update; its
  exact SHA is reported to the root controller after commit because a commit
  cannot embed its own SHA.
- State: `READY_FOR_ROOT_REVIEW` once the docs-only handoff commit and clean
  worktree are confirmed.

## Scope Delivered

- NewAPI-only exact-log `other.group_ratio` parsing and eligibility.
- Beijing-day Claim/Complete/Release CAS with five-minute lease and scheduler
  outbox update.
- Post-usage best-effort registration with one shared NewAPI log lookup and
  accurate failure-reason propagation.
- Generic usage-cost evidence excludes unsupported-only identity; the separate
  registration proof accepts explicit NewAPI balance identity or an unsupported
  native probe, while exact valid log matching remains mandatory.
- Administrator registered state, current multiplier, safe tooltip timestamps,
  and manual-edit overwrite warning.

## Direct Verification

- PASS: focused service tests:
  `TestUsageCostEvidenceRegistrarRequiresKnownNativeLedger`,
  `TestNewAPIUsageRecordEligibilityRequiresExactSuccessfulNewAPIUsage`, and
  `TestUsageCostEvidenceRegistrarReusesNewAPILogForRateRegistration`.
- PASS: focused repository tests:
  `TestNewAPIRateRefreshRepositoryContract` and
  `TestCompleteNewAPIRateRefreshRemovesNestedClaimFields`.
- PASS: compile-only checks for service, repository, and server packages.
- PASS: `gofmt -d` and `git diff --check`.
- The same focused service, repository, and compile-only commands were rerun
  after merging `main@0fc6bb9c7`; all exited 0.
- PASS: `gofmt -d` produced no output after refresh.
- PASS: `git diff --check 0730603c8..HEAD` and refresh-only
  `git diff --check c8ec34498..HEAD` both exited 0.
- Frontend Vitest not run: the binary is missing and the prior install attempt
  failed with `ENOTFOUND registry.npmjs.org`. No frontend PASS is claimed.

## Integration Boundary

- Root `main`/`origin/main@0fc6bb9c7` was merged with `git merge --no-edit main`.
  The merge completed without conflicts and retained all mainline P0/T14 and
  official-update behavior.
- No migration, configuration, dependency, lockfile, workflow, or production
  data changes are included. Expected `downtime_required=false`, subject to the
  root release preflight.
- This task did not merge into root main, push, deploy, or access production.
  Mainline `docs/project/*` updates were incorporated unchanged by the authorized
  refresh merge; this task did not edit them.
- Rollback is to revert the T13 candidate commits and redeploy the prior verified
  main; do not perform destructive account-data rollback without separate
  approval.
