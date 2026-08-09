# Host Executor Round 2 Implementation Report

## Scope

This round closes the second production-release review findings for
`ops/deploy-sub2api-blue-green-host.sh`. It does not push, deploy, restart the
production application, touch the protected scheduling worktree, or add GitHub
Actions.

## TDD Evidence

The newly added deadline regression was confirmed RED against the first-round
implementation:

```text
ONLY_TEST=maintenance-deadline bash tests/operations/deploy_sub2api_blue_green_host_test.sh
FAIL: post-stop maintenance command exceeded its deadline: 5s
```

The first-round script had armed `maintenance_stopped` before the stop command
and had started checking public rollback acceptance, so the partial-stop and
truthful-readiness cases were retained and strengthened with complete image,
health, public-readiness, shared-service, and final-record assertions.

## Implementation

- Added a per-command watchdog for every operation after the maintenance stop
  begins. The watchdog uses the smaller of the end-to-end release deadline and
  the remaining maintenance window, runs the command in a dedicated process
  group, and terminates the full group on timeout.
- Added a monotonic elapsed-time bound in addition to the epoch deadline so a
  stalled or non-advancing wall clock cannot extend the unavailable window.
- Armed `maintenance_stopped=true` before the multi-service stop mutation so a
  partial stop failure always enters API and worker restoration.
- Restored the previous API and worker without recreating PostgreSQL, Redis, or
  Caddy, then proved the old API and worker image IDs, API and worker health,
  worker startup logs, live Caddy upstream, public API acceptance, and shared
  container identities before recording `rolled_back=true`.
- If any rollback proof fails, the final record remains
  `state=rollback_failed`, `rolled_back=false`, and the partial checkpoint is
  retained for recovery.

## Verification

The following commands completed successfully:

```text
ONLY_TEST=maintenance bash tests/operations/deploy_sub2api_blue_green_host_test.sh
bash tests/operations/deploy_sub2api_blue_green_host_test.sh
bash -n ops/deploy-sub2api-blue-green-host.sh tests/operations/deploy_sub2api_blue_green_host_test.sh
git diff --check
```

The complete host-executor matrix reported all suites passing, including
authorized maintenance, rollback and interruption recovery, immutable network
probe policy, worker health, lock safety, final-review regressions, and
immutable release-state recovery IDs.

## Remaining Work

- Independent scoped review is still required before push or production host
  installation.
- This implementation has not been pushed or installed on the production host.
- Existing untracked release evidence and the protected scheduling worktree are
  unchanged.
