# T10 Account Quality Monitor Root Handoff

Date: 2026-08-16
Status: `READY_FOR_ROOT_REVIEW`

## Identity

- Candidate branch: `codex/t10-account-quality-monitor`
- Candidate worktree: `/Users/gongtengxinwen/.codex/worktrees/e0ba/sub2api搭建`
- Refreshed baseline: `main@f6ec86733d26219830646fd88859aa39613797a3`
- Candidate commit: `0294cd97fb18d20ebe704406b68f23d2e991dc32`
- Tested tree: `7d7ec2f5ac2a67155dda140d2197b0838a17e4bf`

## Delivery

The service now uses root only for host orchestration while the collector and
real evidence preflight remain UID/GID `10002:10002` with read-only rootfs,
capability drop, no-new-privileges, noexec tmpfs, and existing resource limits.
`OnFailure` covers unexecuted starts, emits one allowlisted redacted
`t10.failure.v1` journal signal, and maps wrapper exit stages deterministically.
Evidence publication uses staged files, fsync, atomic rename, readback, and
rollback to the last valid pair.

## Tests And Risk

All focused tests, syntax checks, static contracts, Docker image build, and
real UID 10002 Docker evidence preflight pass. No migration, dependency, UI,
API, business-write, GitHub Actions, or parallel-control-plane change exists.

A6 actual alert delivery and A10 time-based observation are explicitly waived
by the user and remain unverified; neither is claimed PASS. The root must still
run release preflight, stop if `downtime_required=true`, install the reviewed
host unit/scripts, start the service once, and immediately verify successful
systemd execution plus valid mode-0600 evidence and health endpoints.

## Rollback

Restore the previous systemd service/timer snapshot, remove the failure unit
only if rolling back the feature, reload systemd, and preserve the last valid
evidence. Do not change application data or delete failure transcripts.

## Authorized Next Action

The root release controller may authorize merge to the exact current main,
run the focused merged-tree gates, push, execute no-downtime deployment, and
perform immediate functional verification. The candidate must not deploy
directly.
