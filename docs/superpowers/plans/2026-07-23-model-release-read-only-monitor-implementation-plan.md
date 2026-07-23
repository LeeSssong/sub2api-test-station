# Model Release Read-Only Monitor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run the existing Sub2API model discovery and readiness evaluator every 15 minutes without an LLM, paid requests, or production configuration writes.

**Architecture:** A systemd timer calls a shell wrapper as the existing `ubuntu` Docker user. The wrapper starts one transient restricted container from the already deployed relay-ops Ruby image on `sub2api_default`, mounts only the existing read-only tools/Admin-Key path and the existing writable secret-free evidence directory, then runs collector followed by evaluator. Relay-ops consumes the result through a read-only evidence-directory bind mount, so atomic replacement stays visible; one migration recreation is allowed, but no long-running service is rebuilt on each scheduled run.

**Tech Stack:** POSIX shell, Ruby 3.3 from the existing relay-ops image, Docker, systemd, Minitest.

## Global Constraints

- Sub2API remains the only authority for priority, concurrency, scheduling, groups, model mappings, and pricing.
- The only permitted native discovery work is for accounts that are `active` and `schedulable`; no provider or account ID is special-cased.
- The wrapper may invoke only `collect-model-release-snapshot.rb collect` then `evaluate-model-release-readiness.rb evaluate`.
- Never invoke a benchmark, model generation, synchronous/SSE qualification, capacity test, promoter, route write, price/multiplier/balance/Key operation, candidate creation, D04 overlay, or Feishu action.
- The transient container must run as `10002:10002` with `--read-only`, `--cap-drop ALL`, `no-new-privileges`, a 16 MiB noexec tmpfs, 64 PID cap, 128 MiB memory cap, and 0.25 CPU cap.
- Do not expose a port, install Ruby on the host, copy a Key, or rebuild a long-lived container.
- Logs may contain status, UTC time, snapshot ID, proposal ID, and hash-shaped fields. Never log a Base URL, credential, header, raw response, or model output.
- A failure returns non-zero and preserves the last valid `model-release-result.json`; `/ops` remains fail-closed when that evidence is stale.
- Preserve `RELAY_OPS_MODE=read_only`, `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`, `D04_MODE=read_only`, and closed registration.

---

### Task 1: Transient Runner Wrapper

**Files:**
- Create: `ops/run-model-release-monitor.sh`
- Create: `tests/operations/model_release_monitor_test.rb`

**Interfaces:**
- Consumes five non-secret variables: `MODEL_RELEASE_ROOT`, `MODEL_RELEASE_ADMIN_KEY_FILE`, `MODEL_RELEASE_EVIDENCE_DIR`, `MODEL_RELEASE_RUNNER_IMAGE`, and `MODEL_RELEASE_DOCKER_NETWORK`.
- Produces: exit `0` only after collection and evaluation both complete; the existing evaluator atomically replaces the current result file.

- [ ] **Step 1: Write the failing tests**

Create a Minitest fixture that supplies temporary directories and a fake executable through `MODEL_RELEASE_DOCKER_BIN`. The fake records argv and exits with a test-selected status. Assert the wrapper passes these arguments and only these two Ruby commands:

```ruby
assert_includes argv, "--read-only"
assert_includes argv, "--cap-drop"
assert_includes argv, "ALL"
assert_includes argv, "--security-opt"
assert_includes argv, "no-new-privileges"
assert_includes argv, "--network"
assert_includes argv, "sub2api_default"
assert_match(/collect-model-release-snapshot\.rb collect/, argv.join(" "))
assert_match(/evaluate-model-release-readiness\.rb evaluate/, argv.join(" "))
refute_match(/upstream-benchmark|promote-model-release|capacity|probe/, argv.join(" "))
```

Add failure tests for a non-zero Docker result, missing or relative root/key/evidence paths, and output containing the key-path fixture value. These tests must fail because the wrapper does not yet exist.

- [ ] **Step 2: Run RED**

Run: `ruby tests/operations/model_release_monitor_test.rb`

Expected: failure because `ops/run-model-release-monitor.sh` is missing.

- [ ] **Step 3: Implement the wrapper**

Create a strict `set -eu` shell script with `umask 077`. Validate all five variables before starting Docker: root/key/evidence must be absolute existing paths; root must contain the collector, evaluator, policy helper, and policy YAML; the configured Docker executable must be executable. Keep `MODEL_RELEASE_DOCKER_BIN` as a test-only override and default it to `/usr/bin/docker`.

Invoke Docker with the fixed shape:

```sh
"$docker_bin" run --rm --network "$MODEL_RELEASE_DOCKER_NETWORK" \
  --user 10002:10002 --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 64 --memory 128m --cpus 0.25 \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=16m \
  -v "$MODEL_RELEASE_ROOT:/work:ro" \
  -v "$MODEL_RELEASE_ADMIN_KEY_FILE:/run/secrets/sub2api-admin-api-key:ro" \
  -v "$MODEL_RELEASE_EVIDENCE_DIR:/var/lib/model-release:rw" \
  --entrypoint /bin/sh "$MODEL_RELEASE_RUNNER_IMAGE" -ec '
    ruby /work/collect-model-release-snapshot.rb collect --policy /work/model-release-policy-v1.yaml --base-url http://sub2api:8080 --admin-key-file /run/secrets/sub2api-admin-api-key --output /var/lib/model-release/model-release-snapshot.json
    ruby /work/evaluate-model-release-readiness.rb evaluate --policy /work/model-release-policy-v1.yaml --snapshot /var/lib/model-release/model-release-snapshot.json --output /var/lib/model-release/model-release-result.json
  '
```

Print exactly `model_release_monitor status=started`, `model_release_monitor status=succeeded`, or `model_release_monitor status=failed`; do not echo a command or environment value.

- [ ] **Step 4: Run GREEN**

Run: `bash -n ops/run-model-release-monitor.sh && ruby tests/operations/model_release_monitor_test.rb`

Expected: pass. The fake Docker capture contains no forbidden tool name and test output contains no fixture secret value.

- [ ] **Step 5: Commit Task 1**

```bash
git add ops/run-model-release-monitor.sh tests/operations/model_release_monitor_test.rb
git commit -m "feat: add read-only model release monitor runner"
```

### Task 2: Service, Timer, and Environment Contract

**Files:**
- Create: `infra/systemd/sub2api-model-release-monitor.service`
- Create: `infra/systemd/sub2api-model-release-monitor.timer`
- Create: `infra/systemd/model-release-monitor.env.example`
- Modify: `tests/operations/model_release_monitor_test.rb`

**Interfaces:**
- Consumes the Task 1 wrapper and existing `ubuntu` Docker-group membership.
- Produces a `sub2api-model-release-monitor.service` oneshot and timer that run no more often than every 15 minutes.

- [ ] **Step 1: Extend the test with failing systemd assertions**

Require the service to contain:

```ruby
assert_includes service, "User=ubuntu"
assert_includes service, "EnvironmentFile=/etc/sub2api/model-release-monitor.env"
assert_includes service, "ExecStart=/opt/sub2api/production/ops/model-release/run-model-release-monitor.sh"
%w[NoNewPrivileges=true PrivateTmp=true ProtectHome=true ProtectSystem=full ProtectKernelTunables=true ProtectControlGroups=true RestrictSUIDSGID=true].each do |setting|
  assert_includes service, setting
end
assert_includes timer, "OnUnitActiveSec=15m"
assert_includes timer, "RandomizedDelaySec=2m"
assert_includes timer, "Persistent=true"
refute_match(/api[_-]?key\s*=|token\s*=|secret\s*=|password\s*=/i, environment)
```

Require the example environment to contain only these non-secret values:

```ini
MODEL_RELEASE_ROOT=/opt/sub2api/production/ops/model-release
MODEL_RELEASE_ADMIN_KEY_FILE=/opt/sub2api/production/secrets/sub2api-admin-api-key
MODEL_RELEASE_EVIDENCE_DIR=/opt/sub2api/production/evidence/model-release-20260722
MODEL_RELEASE_RUNNER_IMAGE=sub2api-relay-ops:model-release-read-only-20260722-v1
MODEL_RELEASE_DOCKER_NETWORK=sub2api_default
```

- [ ] **Step 2: Run RED**

Run: `ruby tests/operations/model_release_monitor_test.rb`

Expected: failure because the three systemd templates do not exist.

- [ ] **Step 3: Add the minimal templates**

Create a `Type=oneshot` service with `User=ubuntu`, `Group=ubuntu`, `UMask=0077`, `TimeoutStartSec=4min`, `After=docker.service network-online.target`, the required environment file, and all tested hardening settings. Do not grant capabilities, use `sudo`, open a port, or include a secret value.

Create a timer with:

```ini
[Timer]
OnBootSec=5m
OnUnitActiveSec=15m
RandomizedDelaySec=2m
Persistent=true
Unit=sub2api-model-release-monitor.service
```

- [ ] **Step 4: Run GREEN**

Run: `ruby tests/operations/model_release_monitor_test.rb && git diff --check`

Expected: pass. Production acceptance additionally runs `systemd-analyze verify` against installed units.

- [ ] **Step 5: Commit Task 2**

```bash
git add infra/systemd/sub2api-model-release-monitor.service infra/systemd/sub2api-model-release-monitor.timer infra/systemd/model-release-monitor.env.example tests/operations/model_release_monitor_test.rb
git commit -m "feat: schedule read-only model release monitoring"
```

### Task 3: Runbook, Full Test Suite, and Production Acceptance

**Files:**
- Create: `docs/runbooks/model-release-read-only-monitor.md`
- Create: `docs/superpowers/reports/2026-07-23-model-release-read-only-monitor-verification.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`

**Interfaces:**
- Consumes validated Tasks 1-2 and the existing `/opt/sub2api/production/evidence/model-release-20260722` read-only relay-ops directory mount.
- Produces a documented installation/disable procedure and evidence that no Sub2API configuration or paid work occurred.

- [ ] **Step 1: Write the runbook and install procedure**

Document copying the reviewed four model-release Ruby/YAML files and wrapper to `/opt/sub2api/production/ops/model-release/`, installing the environment file with mode `0600`, installing units with mode `0644`, calling `systemctl daemon-reload`, manually starting the service once, then enabling the timer. Require `systemctl status`, `journalctl -u`, `list-timers`, and `disable --now` instructions. Explicitly prohibit benchmark, promoter, model request, route write, D04 overlay, and Feishu mode changes.

- [ ] **Step 2: Run local verification**

Run:

```bash
ruby tests/operations/model_release_policy_test.rb
ruby tests/operations/collect_model_release_snapshot_test.rb
ruby tests/operations/evaluate_model_release_readiness_test.rb
ruby tests/operations/model_release_monitor_test.rb
bash tests/relay_ops/validate_relay_ops_contract.sh
git diff --check
```

Expected: all pass.

- [ ] **Step 3: Perform constrained production acceptance**

Before installation, record only container IDs, health/restart counts, selected read-only modes, result hash, result mount source, and redacted canonical account/group/mapping hash. Do not print secrets.

Install files, run `sudo systemd-analyze verify`, execute exactly one manual service run, confirm a fresh secret-free snapshot/result and the normal `/ops` refresh, then enable the timer. Inspect timer next activation and journal status. Do not wait by generating traffic or force a paid qualification.

Afterward re-read the same non-sensitive facts. Require unchanged mappings, group model lists, routes, prices, multipliers, balances, Keys, database rows, D04 state, Feishu mode, and persistent-container identities, except for the one required relay-ops recreation that migrates the result mount from a file to its evidence directory. The only permitted content changes are systemd state and model-release snapshot/result artifacts.

- [ ] **Step 4: Update evidence and commit**

Write unit hashes, runner image digest, result hashes, timer state, zero-write proof, and remaining separate gates into the report. Update both project handoff files to state that discovery/readiness is automatic but qualification, financial evidence, natural quality evidence, promotion, and D04 opening remain separately gated.

```bash
git add docs/runbooks/model-release-read-only-monitor.md docs/superpowers/reports/2026-07-23-model-release-read-only-monitor-verification.md docs/project/current-state.md docs/project/llm-handoff.md
git commit -m "docs: verify scheduled model release monitoring"
```

## Plan Self-Review

- Spec coverage: Task 1 implements fixed command ordering, secret-free logs, and result retention. Task 2 implements cadence, jitter, reboot persistence, and service hardening. Task 3 covers installation, production evidence, and remaining manual gates.
- Placeholder scan: every task gives exact paths, interfaces, tests, command shapes, and expected outcomes; no deferred implementation placeholder remains.
- Contract consistency: the wrapper reads the same five variables defined by the environment example; the service points to that wrapper; the timer references that service; the evaluator writes the exact file already mounted read-only by relay-ops.
