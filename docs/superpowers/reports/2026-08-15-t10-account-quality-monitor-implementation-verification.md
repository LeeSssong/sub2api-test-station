# T10 Account Quality Monitor Implementation Verification

Date: 2026-08-15
Candidate worktree: `/Users/gongtengxinwen/.codex/worktrees/e0ba/sub2api搭建`
Baseline: `main@8f79f1330fa007761b2a82af9a845529fbc5b31d`

## Scope

This candidate restores the host executable chain for the native account
quality timer. The wrapper remains read-only against business data, launches
the existing UID/GID `10002:10002` collector with the approved container
hardening, and publishes the existing JSON evidence files atomically.

## Verification

- `ruby -Itests tests/operations/account_quality_failure_signal_test.rb`: PASS (2 tests)
- `ruby -Itests tests/operations/account_quality_monitor_test.rb`: PASS (4 tests)
- `ruby -Itests tests/operations/collect_account_quality_pulse_test.rb`: PASS (12 tests)
- `sh -n ops/account-quality-failure-signal.sh ops/run-account-quality-monitor.sh`: PASS
- `ruby -c ops/collect-account-quality-pulse.rb`: PASS
- `sh tests/operations/account_quality_monitor_systemd_contract_test.sh`: PASS
- `bash tests/operations/account_quality_monitor_alert_delivery_test.sh`: PASS with A6 explicitly waived/unverified
- `bash tests/relay_ops/validate_relay_ops_contract.sh`: PASS
- `git diff --check`: PASS
- Local Docker image build from `infra/Dockerfile.relay-ops`: PASS
- Real UID/GID `10002:10002` Docker volume preflight using the built image:
  write, fsync, same-directory rename, readback, delete, directory fsync, and
  zero remaining files: PASS

The candidate preserves the intended native receiver boundary and adds no
receiver, API, table, or parallel control plane. This worktree does not contain
a verified `t10.failure.v1` projection consumer, so receiver integration is
recorded as an actual evidence gap rather than claimed as implemented. The
controlled delivery receipt is separately user-waived.

## Acceptance status

- A1-A5: implementation/static evidence is present; host execution evidence remains for root review. The wrapper now enforces an evidence directory contract of `10002:10002` and `0700`, then performs the real UID 10002 bind-mount write/fsync/same-directory rename/readback/cleanup preflight.
- A6: explicitly waived by the user. The controlled `203/EXEC` delivery drill was not run and no receipt is claimed. This is an unverified residual risk, not a release blocker for this candidate.
- A7-A9: implementation/static contracts are present; runtime and read-only database evidence remain for root review.
- A10: explicitly waived by the user. No 24-hour or other time-based waiting is
  required; immediate production execution and evidence checks are the release
  acceptance. This time-based item is not claimed as PASS.

No deployment, production mutation, global ledger edit, or merge was performed.
