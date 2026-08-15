# T10 Account Quality Monitor Executable Chain Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the native account-quality timer's executable chain, preserve the protected production tree and UID `10002` collector boundary, and preserve the redacted failure integration with the existing native alert/Feishu path. A6 delivery is explicitly user-waived for this release and remains unverified.

**Architecture:** A root-owned host wrapper performs fixed path, mode, credential, Docker, and real bind-mount preflight, then launches the existing restricted collector. A systemd failure hook invokes one deterministic redacted signal helper even when `ExecStart` fails before execution. Existing `relay-ops` consumes the native `/api/v1/admin/ops/alert-events` projection and sends the signal through its existing Feishu path; no new receiver or control plane is introduced.

**Tech Stack:** POSIX shell, systemd unit/timer, Docker, Ruby collector, Ruby/Minitest operation contracts, relay-ops Go tests, existing host release and evidence scripts.

## Global Constraints

- Keep the production tree at `0700 root:root`; do not add ACLs, symlinks, path migrations, or secret copies.
- Keep the collector at UID/GID `10002:10002`, read-only rootfs, `cap-drop=ALL`, `no-new-privileges`, noexec tmpfs, 64 PID limit, 128 MiB memory limit, and 0.25 CPU limit.
- Reuse the existing healthy `relay-ops` plus native `/api/v1/admin/ops/alert-events` and Feishu outbound chain; do not create a receiver, API, table, or parallel control plane.
- Preserve the existing timer name, cadence, evidence filenames, JSON shape, and read-only business boundary.
- A6 is explicitly waived by the user for this release. Do not run or invent
  the controlled delivery receipt; record it as unverified residual risk while
  preserving the existing receiver integration and redaction contract. This
  residual risk does not block implementation or deployment in this release.
- No implementation task may modify `main`, project queue/progress ledgers, production state, or deploy from a candidate worktree.
- No GitHub Actions; release and deployment remain in the reviewed local/host chain.

---

### Task 1: Deterministic failure signal helper and host wrapper stages

**Files:**
- Create: `ops/account-quality-failure-signal.sh`
- Modify: `ops/run-account-quality-monitor.sh`
- Test: `tests/operations/account_quality_monitor_test.rb`
- Test: `tests/operations/account_quality_failure_signal_test.rb`

**Interfaces:**
- Consumes: `T10_FAILURE_PHASE`, `T10_REASON_CODE`, systemd result/status metadata, and a unit name supplied by the failure hook.
- Produces: one newline-delimited `t10.failure.v1` journal payload with allowlisted phase/reason/status fields and a stable dedupe hash; wrapper exits with stage-specific allowlisted status.

- [ ] **Step 1: Add failing contract tests** for every allowlisted phase (`systemd`, `preflight`, `evidence`, `credentials`, `runtime`, `collector`, `resource`, `publish`), unknown-value redaction, stable dedupe output, and exactly one signal per invocation. Assert the payload contains no raw paths, stderr, account/model identifiers, or Admin-Key.
- [ ] **Step 2: Run the focused tests and verify they fail** with missing helper/stage behavior: `ruby -Itests tests/operations/account_quality_failure_signal_test.rb` and `ruby -Itests tests/operations/account_quality_monitor_test.rb`.
- [ ] **Step 3: Implement the signal helper** with a fixed schema, allowlists, `unknown` fallback, SHA-256 dedupe over non-secret normalized fields, and a single `logger`/journald emission. Make delivery/write failure a non-success result without claiming `failure_signal_delivery` as a successful journal event.
- [ ] **Step 4: Refactor the wrapper into explicit preflight, UID-10002 evidence, credentials, Docker, collector, and publish stages**. Preserve the existing command and resource/security flags, map each failure to one helper invocation, suppress secret-bearing command output, and retain the last valid evidence on failure.
- [ ] **Step 5: Run the focused tests and shell checks**: `ruby -Itests tests/operations/account_quality_failure_signal_test.rb`, `ruby -Itests tests/operations/account_quality_monitor_test.rb`, `sh -n ops/account-quality-failure-signal.sh ops/run-account-quality-monitor.sh`; expected result is PASS.
- [ ] **Step 6: Commit** `feat: harden account quality monitor failure stages`.

### Task 2: Atomic evidence preflight and publication contract

**Files:**
- Modify: `ops/run-account-quality-monitor.sh`
- Modify: `ops/collect-account-quality-pulse.rb`
- Modify: `tests/operations/account_quality_monitor_test.rb`
- Modify: `tests/operations/collect_account_quality_pulse_test.rb`
- Create: `tests/operations/account_quality_monitor_runtime_test.sh`

**Interfaces:**
- Consumes: evidence directory and collector output path from the existing environment contract.
- Produces: same-directory temporary JSON files that are fsynced, atomically renamed, mode `0600`, read back after publication, and cleaned up on failure.

- [ ] **Step 1: Add failing tests** for a real UID/GID `10002:10002` bind-mount write/fsync/rename/readback/cleanup, mode `0600` final files, preservation of the prior valid result, and cleanup of partial temporary files.
- [ ] **Step 2: Run `ruby -Itests tests/operations/collect_account_quality_pulse_test.rb` and the runtime fixture** to capture the expected failures.
- [ ] **Step 3: Implement the focused container preflight** before collection and make collector publication use same-directory temporary files, `fsync`, atomic rename, restrictive mode, readback, and cleanup. Keep history's existing 24-hour semantics and do not add a data table or a new evidence file contract.
- [ ] **Step 4: Run the focused Ruby and runtime tests** and verify the wrapper still sends the collector only the existing read-only Admin-Key and native API request.
- [ ] **Step 5: Commit** `fix: make account quality evidence publication atomic`.

### Task 3: Systemd root orchestration and unexecuted-start failure hook

**Files:**
- Modify: `infra/systemd/sub2api-account-quality-monitor.service`
- Modify: `infra/systemd/sub2api-account-quality-monitor.timer`
- Modify: `infra/systemd/account-quality-monitor.env.example`
- Modify: `tests/operations/account_quality_monitor_test.rb`
- Create: `tests/operations/account_quality_monitor_systemd_contract_test.sh`

**Interfaces:**
- Consumes: the Task 1 wrapper and failure helper at the fixed production paths.
- Produces: a `Type=oneshot` root host orchestration unit with `ExecStopPost` or `OnFailure` coverage for `203/EXEC`, while retaining the timer relationship and schedule.

- [ ] **Step 1: Add failing static tests** asserting `User=root`, matching group, fixed wrapper path, failure hook path, unchanged timer unit/cadence, restrictive `UMask`, and no permission-widening directives.
- [ ] **Step 2: Run `ruby -Itests tests/operations/account_quality_monitor_test.rb` and `sh tests/operations/account_quality_monitor_systemd_contract_test.sh`** to verify the current `User=ubuntu` contract fails.
- [ ] **Step 3: Update the service unit** to root orchestration with the existing host hardening, add a failure hook that receives systemd result/status variables even if `ExecStart` cannot execute, and preserve `ConditionPathExists`, environment file, timeout, timer binding, and runtime directory.
- [ ] **Step 4: Add a reversible runtime-drop-in test harness** that points `ExecStart` at a mode-0644 harmless target, observes `203/EXEC`, captures exactly one signal, restores the original unit/timer state, and fails closed if restoration is incomplete.
- [ ] **Step 5: Run `systemd-analyze verify` against the rendered unit/timer where systemd is available, plus all static tests; record the host limitation otherwise without claiming A4.**
- [ ] **Step 6: Commit** `fix: run account quality monitor through root host orchestration`.

### Task 4: Existing receiver integration and waiver evidence

**Files:**
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`
- Create: `tests/operations/account_quality_monitor_alert_delivery_test.sh`
- Create: `docs/superpowers/reports/2026-08-15-t10-account-quality-monitor-implementation-verification.md`
- Modify: `docs/runbooks/account-quality-monitor.md`

**Interfaces:**
- Consumes: the Task 1 `t10.failure.v1` journal signal and the existing native alert-event projection.
- Produces: read-only receiver contract evidence and an explicit record that the controlled `203/EXEC` delivery drill is user-waived/unverified.

- [ ] **Step 1: Add a receiver contract fixture** that rejects any new receiver/API path and asserts the existing relay-ops client reads `/api/v1/admin/ops/alert-events`, preserves the redacted fields, and deduplicates the same signal without claiming external receipt.
- [ ] **Step 2: Run the relay-ops contract tests** with `bash tests/relay_ops/validate_relay_ops_contract.sh`; expected result is failure until the fixture and documented event contract are wired.
- [ ] **Step 3: Implement only the existing event projection/consumer wiring needed to recognize `t10.failure.v1`**, without adding a table or parallel endpoint. Keep the alert payload free of credentials, paths, commands, account identifiers, models, and raw stderr.
- [ ] **Step 4: Record the user's A6 waiver in the implementation verification report**, inspect the existing native alert-event and relay-ops contracts read-only, and confirm no new receiver/API/table/control plane is introduced.
- [ ] **Step 5: Record A1-A5 and A7-A9 evidence plus A6 as explicitly unverified under user waiver; update the runbook with reversible install, optional future drill, inspection, and rollback commands.**
- [ ] **Step 6: Commit** `test: record account quality monitor alert contract`.

### Task 5: Full candidate validation and root handoff

**Files:**
- Modify: `docs/superpowers/reports/2026-08-15-t10-account-quality-monitor-implementation-verification.md`
- Create: `docs/superpowers/reports/2026-08-15-t10-account-quality-monitor-final-review.md`
- Create: `docs/superpowers/reports/2026-08-15-t10-account-quality-monitor-handoff.md`

**Interfaces:**
- Consumes: Tasks 1-4 commits, test transcripts, host evidence, receiver receipt, and release-controller preflight output.
- Produces: `READY_FOR_ROOT_REVIEW` handoff with exact baseline/source/tree identities, migration/config diffs, `downtime_required`, rollback, and residual risk.

- [ ] **Step 1: Run all focused tests**: `ruby -Itests tests/operations/account_quality_failure_signal_test.rb`, `ruby -Itests tests/operations/account_quality_monitor_test.rb`, `ruby -Itests tests/operations/collect_account_quality_pulse_test.rb`, `bash tests/relay_ops/validate_relay_ops_contract.sh`, and every runtime/systemd fixture.
- [ ] **Step 2: Run repository guards**: `git diff --check`, shell syntax checks, relevant Go tests under `relay-ops-service`, migration/dependency/GitHub Actions guards, and the approved release qualification/preflight scripts. Do not deploy from this worktree.
- [ ] **Step 3: Perform an independent task review** against the spec and acceptance matrix, explicitly checking no business writes, unchanged production-tree mode, redaction, timer restoration, and the recorded A6 waiver.
- [ ] **Step 4: Perform the final whole-branch review** and fix all findings before handoff. The waived A6 item is reported as unverified residual risk; only failures in non-waived gates or the 24-hour window block handoff.
- [ ] **Step 5: Write the handoff** with candidate branch/worktree, source and tested tree hashes, changed files, test results, unverified items, migrations/configuration, `downtime_required`, rollback, and the explicit root-only next action.
- [ ] **Step 6: Stop at `READY_FOR_ROOT_REVIEW`** and wait for root's `AUTHORIZE_MERGE_TO_MAIN`; do not edit global ledgers, merge, push, deploy, or clean the worktree.

## Self-review checklist

- Spec coverage: Tasks 1-3 cover all executable, atomic-write, security, and systemd contracts; Task 4 covers the existing receiver and records the user-waived A6 risk; Task 5 covers natural-window, review, release, rollback, and handoff evidence.
- Placeholder scan: no `TBD`, `TODO`, or unspecified implementation steps appear; each task names concrete files, commands, and expected gates.
- Type/contract consistency: the `t10.failure.v1` fields and allowlisted phases originate in Task 1, are consumed by the Task 3 hook, and are asserted by Task 4; the existing evidence paths remain unchanged across Tasks 1-2.
- Scope check: no account admission, scheduling, billing, UI, database migration, external-primary, or new receiver work is included.
