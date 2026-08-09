# Host Executor Round 3 Implementation Report

## Scope

Round 3 fixes the maintenance deadline accounting in
`ops/deploy-sub2api-blue-green-host.sh`. It does not deploy, push, restart
production services, or touch the protected scheduling worktree.

## Root-Cause Investigation

The maintenance deadline regression was reproduced before the fix:

```text
ONLY_TEST=maintenance-deadline bash tests/operations/deploy_sub2api_blue_green_host_test.sh
FAIL: hung rollback operation did not write a final record
```

The preserved fixture showed the rollback API hang marker, no final record,
and a `Terminated: 15` host executor process. The test shell's `SECONDS` did
advance during the real hang (45 to 58 seconds in an xtrace run), while the
executor's absolute fake `date` clock remained constant. The failure was the
combination of shell-relative `SECONDS` accounting and a parent watchdog that
was re-armed only to the rollback phase deadline, leaving no reliable time for
finalization after a timed-out rollback command. `BASH_ENV`'s `kill` hook was
also verified in isolation; it does not make the test shell's `SECONDS`
advance reliably and must not be used as a timing source.

## Implementation

- Added a Perl `Time::HiRes::CLOCK_MONOTONIC` millisecond helper.
- Replaced maintenance phase elapsed calculations and the end-to-end budget
  calculation with the monotonic timestamp, independent of fake wall-clock
  output and `BASH_ENV` child-shell state.
- Re-armed the EXIT recovery watchdog to the overall maintenance hard deadline
  (while each rollback operation remains bounded by its rollback phase budget),
  preserving the finalization reserve without extending the maintenance
  window.
- Changed the deadline regression measurements to the same monotonic clock and
  explicitly assert that a real hung child advances elapsed time.
- Removed the temporary `KEEP_FIXTURE` debug switch.

## Verification

```text
ONLY_TEST=maintenance-deadline bash tests/operations/deploy_sub2api_blue_green_host_test.sh   # pass
ONLY_TEST=maintenance-bounded-ops bash tests/operations/deploy_sub2api_blue_green_host_test.sh # pass
ONLY_TEST=maintenance-partial-stop bash tests/operations/deploy_sub2api_blue_green_host_test.sh # pass
ONLY_TEST=maintenance-rollback-proofs bash tests/operations/deploy_sub2api_blue_green_host_test.sh # pass
```

The complete matrix, Bash syntax check, and `git diff --check` are run before
commit. No production deployment or push is part of this round.

## Remaining Risk

The change is locally verified only. Independent scoped review, integration on
the updated `main`, production host installation, deployment, and online
acceptance remain outstanding; the project ledger therefore remains
`进行中`.
