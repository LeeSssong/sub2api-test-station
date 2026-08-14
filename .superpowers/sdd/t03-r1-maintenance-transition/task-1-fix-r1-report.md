# Task 1 Fix R1 Report — maintenance outage-window wording

## Status

`READY_FOR_RE-REVIEW`. This fixes only P1 from the scoped review; no executor,
test, migration, application, production, remote, or deployment change was
made.

## Verified finding

The executor uses `MAINTENANCE_UNAVAILABLE_SECONDS=${...:-300}` and rejects a
value above `300`. The prior runbook phrase, “default 300-second, maximum
600-second,” was therefore inaccurate. The `+600 seconds` value shown in the
manual recovery command is a separate `--deadline-epoch` recovery-control
deadline, not authorization to extend the API/worker outage.

## Fix

- Changed the maintenance release contract to state that the unavailable window
  is 300 seconds by default and 300 seconds maximum.
- Explicitly labeled that as the only authorized API/worker outage window.
- Explicitly distinguished the manual recovery `--deadline-epoch` from the
  outage authorization.

## Changed files

- `docs/runbooks/sub2api-blue-green-production-deployment.md`
- `.superpowers/sdd/t03-r1-maintenance-transition/task-1-fix-r1-report.md`

## Verification

All commands exited `0`:

```bash
ONLY_TEST=maintenance-t03-r1-transition bash tests/operations/deploy_sub2api_blue_green_host_test.sh
bash -n ops/deploy-sub2api-blue-green-host.sh
git diff --check
```

## Commit SHA

Fix commit: `c01b5674ce7df3a93f572822c7256aa16efd3f6d`.

## Concerns

The branch still requires independent re-review, whole-branch review, root
merge authorization, push, maintenance deployment, and online acceptance.
