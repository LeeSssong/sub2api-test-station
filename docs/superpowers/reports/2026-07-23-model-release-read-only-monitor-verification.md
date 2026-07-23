# Scheduled Model Release Monitor Verification

**Date:** 2026-07-23 (Asia/Shanghai)
**Result:** PASS
**Production modes:** relay-ops read_only, Feishu commands dry_run, D04 read_only, registration closed

## Scope

This increment installs a server-local systemd monitor that performs native
model-directory discovery and readiness evaluation every 15 minutes without an
LLM. It does not create a model-generation request, run compatibility or SSE
qualification, run a capacity test, promote a model, change a route, alter
prices, multipliers, balances, credentials, account scheduling, D04 state, or
Feishu state.

The service runs as the existing Docker-capable ubuntu user. Each run starts a
short-lived relay-ops Ruby image on sub2api_default as UID/GID 10002:10002,
with read-only root filesystem, no capabilities, no-new-privileges, 16 MiB
noexec temporary storage, 64 PID limit, 128 MiB memory limit, and 0.25 CPU.

## Automated Verification

The following local checks passed:

    ruby tests/operations/model_release_policy_test.rb
    ruby tests/operations/collect_model_release_snapshot_test.rb
    ruby tests/operations/evaluate_model_release_readiness_test.rb
    ruby tests/operations/model_release_monitor_test.rb
    bash tests/relay_ops/validate_relay_ops_contract.sh
    docker run ... go test ./... && go vet ./...
    git diff --check

The monitor-specific suite passed 4 runs and 103 assertions. It verifies the
fixed Docker command sequence, rejected relative paths, restricted runner
arguments, failure status handling, secret-free output, service hardening,
15-minute cadence, randomized delay, persistent timer behavior, and
secret-free environment template.

The complete relay-ops Go suite and `go vet ./...` also passed in the pinned
Go image. Module and build caches were mounted only under `/tmp` to make the
verification reproducible without changing the repository or production.

## Production Installation

Installed server-local artifacts:

    /opt/sub2api/production/ops/model-release/run-model-release-monitor.sh
    /etc/systemd/system/sub2api-model-release-monitor.service
    /etc/systemd/system/sub2api-model-release-monitor.timer
    /etc/sub2api/model-release-monitor.env

The environment file is mode 0600 and owned by ubuntu. It contains only local
file paths, the existing relay-ops image reference, and the Docker network
name. The Admin-Key value was neither read nor copied.

The service was manually executed successfully. A later timer activation also
executed successfully. The timer is enabled and active; after final acceptance
its next observed activation is 2026-07-23 02:22:35 CST.

## Atomic Visibility Correction

The first acceptance detected that the evaluator atomically replaced the host
result file but relay-ops retained the old result. Hashes differed because the
original Docker bind mount targeted one file inode.

The fix changes the relay-ops bind mount to the existing evidence directory,
read-only, with the application reading the result inside that directory. This
required one relay-ops recreation only. No scheduled run recreates a
long-running container.

Final post-fix manual run proved atomic visibility:

    host result SHA-256:      e68a167faa483b97e260cebbfcf62afa2a9ef2415d4f49f0fdaafec22aaae550
    relay-ops mounted result: e68a167faa483b97e260cebbfcf62afa2a9ef2415d4f49f0fdaafec22aaae550

The relay-ops mount source is the evidence directory with read-write false.

## Current Read-Only Result

    status: 待测试
    proposal_id: df247edf1a01b4940a568b76d0d9d2f51df1cdb43d5863d0bf49ca85f20f2ce8
    account_set_sha256: cf28d87d0070ac5eca5847714ad4512b01b8e1cc098bf47691924cbf484aef3c
    base_config_sha256: 1261a40c660b6b6d6a4e47c3e6ce63825e36302b7c01832ef5ed676c71690f68

The unchanged account and base-configuration hashes show the monitor did not
alter current account membership, model mappings, public group model lists,
routing, price, multiplier, or account schedule.

## Production Recheck

- relay-ops is healthy with restart count 0 after its required recreation.
- Sub2API, PostgreSQL, Redis, Caddy, and D04 container identities are unchanged.
- relay-ops remains read_only and Feishu commands remain dry_run.
- D04 remains read_only with registration closed.
- /healthz, /readyz, /ops, /monitor, and /pricing each returned HTTP 200.
- The monitor journal's final accepted runs contain only status, snapshot ID,
  account-set hash, readiness status, and proposal ID. No warning, credential,
  Base URL, raw response, or model output was recorded.

## Remaining Separate Gates

The automatic monitor is complete. It cannot convert a candidate into a
customer-facing model. The following remain explicitly approved, separate
activities:

1. Bounded paid compatibility and terminal SSE qualification for each
   candidate/account pair.
2. Trustworthy minimum-balance and fresh natural account-quality evidence.
3. A resulting 可升级 proposal and separate controlled-promotion approval.
4. A fresh D04 v3 GO decision before opening the controlled first-user cohort.
