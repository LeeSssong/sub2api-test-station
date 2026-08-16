# T13 Final Handoff

## Candidate

- Branch: `codex/newapi-rate-multiplier-registration`.
- Approved refreshed baseline: `45de05dffa560f8d2f92695258d4928e6d18ac34`.
- Pre-Task-2/3 tip: `0730603c806779ebdedff9ed18ecac6be4134f4a`.
- Implementation commit: recorded after the first candidate commit.
- Final handoff commit: the branch tip containing this handoff update.
- State: `READY_FOR_ROOT_REVIEW` after both candidate commits and a clean
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
- Frontend Vitest not run: the binary is missing and the prior install attempt
  failed with `ENOTFOUND registry.npmjs.org`. No frontend PASS is claimed.

## Integration Boundary

- Root `main`/`origin/main` was `92db09644` when work resumed. It was not merged
  into this dirty candidate. Root review must refresh the candidate deliberately
  before authorizing integration.
- No migration, configuration, dependency, lockfile, workflow, or production
  data changes are included. Expected `downtime_required=false`, subject to the
  root release preflight.
- This task did not merge main, push, deploy, access production, or modify
  `docs/project/*`.
- Rollback is to revert the T13 candidate commits and redeploy the prior verified
  main; do not perform destructive account-data rollback without separate
  approval.
