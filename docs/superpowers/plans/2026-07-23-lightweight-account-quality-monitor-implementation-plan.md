# Lightweight Account-Pool Quality Monitor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the model-release scheduler with a 15-minute, account-isolated Sub2API quality pulse that reports stability, TTFT, configured multiplier, and explicit balance exhaustion.

**Architecture:** A constrained systemd oneshot starts a short-lived Ruby worker on the existing internal Docker network. The worker discovers only `active + schedulable` accounts, invokes Sub2API's native account-test SSE endpoint once per account, stores only first-token timing and stable result codes, and never copies upstream credentials. Relay-ops reads the secret-free result into the existing hidden-admin `/ops` projection; existing quality reports, Feishu cards, routing, and native scheduling remain unchanged.

**Tech Stack:** Ruby standard library, POSIX shell, systemd, Docker Compose, Go 1.24, existing relay-ops templates and Sub2API Admin API.

## Global Constraints

- Discover accounts only from `GET /api/v1/admin/accounts`; include exactly `status=active && schedulable=true`.
- Reuse `POST /api/v1/admin/accounts/:id/test`; never read, copy, print, or persist an upstream Base URL, credential, request header, raw response, or model output.
- Test accounts sequentially in ascending ID order. Any per-account failure must not prevent later accounts from running.
- Emit `balance_exhausted` only for explicit insufficient-credit, balance, or quota errors. Unknown balance is not depleted balance.
- Stop at the first non-empty native `content` event, store TTFT only, and never retry within a run.
- Keep relay-ops `read_only`, Feishu `dry_run`, D04 `read_only`, and registration closed. Never mutate routing, groups, priorities, scheduling, multipliers, prices, balances, Keys, models, D04, or Feishu.
- Keep `/ops` read-only and hidden-admin protected. Add no form, button, mutation route, or new notification.
- Retain old model-release source/evidence as history, but disable its production timer after the replacement is live.

---

### Task 1: Account-Isolated Pulse Collector

**Files:**
- Create: `ops/collect-account-quality-pulse.rb`
- Create: `tests/operations/collect_account_quality_pulse_test.rb`

**Interfaces:**
- Consumes: `collect --base-url URL --admin-key-file PATH --output PATH`.
- Produces: mode-`0600` `account-quality-result.json` plus a private 24-hour history file.

- [ ] **Step 1: Write failing collector tests**

Use a local HTTP fixture with accounts `21/22` active+schedulable and account `23` disabled. Return native models and SSE events so account `21` emits content, account `22` emits `Insufficient balance`, and any request for `23` fails the test. Assert:

```ruby
assert_equal [21, 22], result.fetch("accounts").map { |item| item.fetch("account_id") }
assert_equal "passed", result.fetch("accounts")[0].fetch("last_result")
assert_operator result.fetch("accounts")[0].fetch("ttft_p95_ms"), :>=, 0
assert_equal "balance_exhausted", result.fetch("accounts")[1].fetch("last_result")
assert_nil result.fetch("accounts")[1].fetch("ttft_p95_ms")
refute_includes JSON.generate(result), "fixture response text"
```

Add cases for deterministic model selection, no text model, malformed SSE, timeout, generic error, forbidden output, and a failed account followed by a successful account.

- [ ] **Step 2: Run RED**

```bash
ruby tests/operations/collect_account_quality_pulse_test.rb
```

Expected: failure because the collector does not exist.

- [ ] **Step 3: Implement the bounded collector**

Implement `AccountQualityPulse::CLI.run(ARGV)`. Use these native paths:

```text
GET  /api/v1/admin/accounts?page=1&page_size=100
GET  /api/v1/admin/accounts/:id
GET  /api/v1/admin/accounts/:id/models
POST /api/v1/admin/accounts/:id/test
```

Require an absolute `0600`/`0640` Admin-Key file, cap responses at 2 MiB, use 5-second connect and 20-second read timeouts, and send the fixed prompt `Reply with OK only.`. Select the highest numeric `gpt-<major>.<minor>` family, then its lexicographically first model; otherwise reuse `UpstreamBenchmarkV2::ModelCatalog` and take the first text model. Never branch on provider or account name.

Publish exactly this public shape:

```json
{
  "schema_version": 1,
  "snapshot_id": "ACCOUNT-QUALITY-20260723T000000Z",
  "observed_at": "2026-07-23T00:00:00Z",
  "account_set_sha256": "64-lowercase-hex",
  "accounts": [{
    "account_id": 21,
    "model_id": "gpt-5.6-sol",
    "rate_multiplier": 0.05,
    "sample_count": 4,
    "success_count": 3,
    "success_rate": 0.75,
    "ttft_p50_ms": 150.0,
    "ttft_p95_ms": 210.0,
    "last_result": "passed",
    "last_error_code": "",
    "last_observed_at": "2026-07-23T00:00:00Z"
  }]
}
```

Keep raw samples only in sibling `account-quality-history.json`, prune after 24 hours, and calculate nearest-rank P50/P95. Parse SSE line-by-line; return on the first non-empty `content.text` and discard it. Only these patterns map to balance exhaustion:

```ruby
/\binsufficient (?:balance|credit|quota)\b/i
/\b(?:balance|credit) (?:is )?exhausted\b/i
/\bquota (?:has been )?exhausted\b/i
```

Map other outcomes to `account_test_error`, `http_error`, `timeout`, `malformed_stream`, or `model_unavailable`. Atomically publish with a same-directory `Tempfile`, chmod `0600`, fsync, and rename.

- [ ] **Step 4: Run GREEN**

```bash
ruby tests/operations/collect_account_quality_pulse_test.rb
```

Expected: all tests pass and neither fixture secret nor SSE content appears.

- [ ] **Step 5: Commit**

```bash
git add ops/collect-account-quality-pulse.rb tests/operations/collect_account_quality_pulse_test.rb
git commit -m "feat: collect account quality pulses"
```

### Task 2: Hardened 15-Minute Task

**Files:**
- Create: `ops/run-account-quality-monitor.sh`
- Create: `infra/systemd/sub2api-account-quality-monitor.service`
- Create: `infra/systemd/sub2api-account-quality-monitor.timer`
- Create: `infra/systemd/account-quality-monitor.env.example`
- Create: `tests/operations/account_quality_monitor_test.rb`

**Interfaces:**
- Consumes: `ACCOUNT_QUALITY_ROOT`, `ACCOUNT_QUALITY_ADMIN_KEY_FILE`, `ACCOUNT_QUALITY_EVIDENCE_DIR`, `ACCOUNT_QUALITY_RUNNER_IMAGE`, `ACCOUNT_QUALITY_DOCKER_NETWORK`.
- Produces: stable `account_quality_monitor status=started|succeeded|failed` logs and Task 1 evidence.

- [ ] **Step 1: Write failing wrapper/unit tests**

Require Docker arguments:

```text
--rm --network sub2api_default --user 10002:10002 --read-only
--cap-drop ALL --security-opt no-new-privileges --pids-limit 64
--memory 128m --cpus 0.25 --tmpfs /tmp:rw,nosuid,nodev,noexec,size=16m
```

Require only `collect-account-quality-pulse.rb collect`; reject `upstream-benchmark`, `promote-model-release`, `sync-upstream`, `capacity`, `probe`, `curl`, and `wget`. Assert systemd hardening, `OnUnitActiveSec=15m`, `RandomizedDelaySec=2m`, `Persistent=true`, and no literal credential.

- [ ] **Step 2: Run RED**

```bash
ruby tests/operations/account_quality_monitor_test.rb
```

Expected: missing wrapper/unit failure.

- [ ] **Step 3: Implement wrapper and units**

Run exactly one constrained container:

```sh
"$docker_bin" run --rm --network "$docker_network" \
  --user 10002:10002 --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 64 --memory 128m --cpus 0.25 \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=16m \
  -v "$root:/work:ro" \
  -v "$admin_key_file:/run/secrets/sub2api-admin-api-key:ro" \
  -v "$evidence_dir:/var/lib/account-quality:rw" \
  --entrypoint /bin/sh "$runner_image" -ec '
    ruby /work/collect-account-quality-pulse.rb collect \
      --base-url http://sub2api:8080 \
      --admin-key-file /run/secrets/sub2api-admin-api-key \
      --output /var/lib/account-quality/account-quality-result.json
  '
```

Use production paths under `/opt/sub2api/production/ops/account-quality`, `/opt/sub2api/production/evidence/account-quality`, and `/etc/sub2api/account-quality-monitor.env`. The env file contains paths/image/network only.

- [ ] **Step 4: Run GREEN**

```bash
ruby tests/operations/account_quality_monitor_test.rb
```

Expected: all tests pass and logs contain stable status only.

- [ ] **Step 5: Commit**

```bash
git add ops/run-account-quality-monitor.sh infra/systemd/sub2api-account-quality-monitor.* \
  infra/systemd/account-quality-monitor.env.example tests/operations/account_quality_monitor_test.rb
git commit -m "feat: schedule account quality monitoring"
```

### Task 3: Strict Relay-Ops Loader And `/ops` Projection

**Files:**
- Create: `relay-ops-service/internal/accountquality/result.go`
- Create: `relay-ops-service/internal/accountquality/result_test.go`
- Modify: `relay-ops-service/internal/config/config.go`
- Modify: `relay-ops-service/internal/config/config_test.go`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/http/server.go`
- Modify: `relay-ops-service/internal/http/sources.go`
- Modify: `relay-ops-service/internal/http/templates/ops.html`
- Modify: `relay-ops-service/internal/http/model_release_test.go`
- Modify: `relay-ops-service/internal/http/sources_test.go`

**Interfaces:**
- Consumes: absolute `RELAY_OPS_ACCOUNT_QUALITY_RESULT_FILE`.
- Produces: `accountquality.FileSource.Read(time.Time) (accountquality.Result, error)` and `accountquality.View`.
- Replaces: deployed model-release projection only; keeps existing `QualityReports` unchanged.

- [ ] **Step 1: Write failing loader/render tests**

Assert the Task 1 shape loads and renders ordered accounts with `成功 3/4`, `TTFT P95 210ms`, `倍率 0.05x`, and `余额不足`. Reject future/stale timestamps, malformed hashes, duplicate IDs, invalid metrics, relative path, and forbidden keys such as `response_text`. Require `/ops` to contain `账号池质量`, `稳定性`, `TTFT P95`, and `倍率`; forbid model-release copy and all write controls.

- [ ] **Step 2: Run RED**

```bash
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24 \
  sh -c 'go test ./internal/accountquality ./internal/config ./internal/http -count=1'
```

Expected: compile failure for missing `accountquality` package.

- [ ] **Step 3: Implement strict loader and view**

Define Task 1's fields in Go with `time.Time` timestamps and nullable TTFT pointers. Limit input to 2 MiB; disallow unknown and secret/response keys; validate lowercase SHA-256, unique positive IDs, finite non-negative multiplier/metrics, `success_count <= sample_count`, `success_rate` in `[0,1]`, approved result codes, and model IDs. Mark evidence stale after 20 minutes.

Add `AccountQualityResultFile string` to config, validate absolute paths, install `accountquality.FileSource` in `app.New`, and add its view to `OpsView`. Replace only the `模型版本` section with a read-only `账号池质量` table ordered by account ID. Keep `内测开放状态`, `当前活动上游`, `质量报告`, events, Agent analysis, admin authentication, and refresh behavior unchanged.

- [ ] **Step 4: Run GREEN**

```bash
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24 \
  sh -c 'go test ./internal/accountquality ./internal/config ./internal/http -count=1'
```

Expected: all focused packages pass.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/accountquality relay-ops-service/internal/config \
  relay-ops-service/internal/app/app.go relay-ops-service/internal/http
git commit -m "feat: show account quality pulses in ops"
```

### Task 4: Compose Contract And Documentation

**Files:**
- Modify: `infra/compose.yaml`
- Modify: `infra/Dockerfile.relay-ops`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`
- Create: `docs/runbooks/account-quality-monitor.md`
- Modify: `docs/runbooks/model-release-read-only-monitor.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`
- Create: `docs/superpowers/reports/2026-07-23-account-quality-monitor-verification.md`

**Interfaces:**
- Consumes: `${RELAY_OPS_ACCOUNT_QUALITY_RESULT_HOST_DIR}`.
- Produces: read-only `/run/relay-ops/account-quality/account-quality-result.json`.

- [ ] **Step 1: Write failing deployment contract**

Require:

```text
RELAY_OPS_ACCOUNT_QUALITY_RESULT_FILE: /run/relay-ops/account-quality/account-quality-result.json
${RELAY_OPS_ACCOUNT_QUALITY_RESULT_HOST_DIR:-/dev/null}:/run/relay-ops/account-quality:ro
```

Forbid old model-release environment/mounts, require the collector in the image, require `账号池质量`, and forbid `模型版本` in the active template.

- [ ] **Step 2: Run RED**

```bash
bash tests/relay_ops/validate_relay_ops_contract.sh
```

Expected: failure on old wiring.

- [ ] **Step 3: Implement wiring and docs**

Mount the entire evidence directory read-only so atomic rename stays visible. Change no other Compose mount, route, mode, or dependency. The runbook permits timer/status/hash inspection and disabling the timer; it forbids manual account tests, credential inspection, route changes, or evidence deletion. Mark the old model-release timer as superseded, not deleted. Update current-state and handoff to keep D04 opening separate.

- [ ] **Step 4: Run GREEN**

```bash
bash tests/relay_ops/validate_relay_ops_contract.sh
```

Expected: `PASS: relay-ops container and routing contracts`.

- [ ] **Step 5: Commit**

```bash
git add infra/compose.yaml infra/Dockerfile.relay-ops tests/relay_ops/validate_relay_ops_contract.sh \
  docs/runbooks/account-quality-monitor.md docs/runbooks/model-release-read-only-monitor.md \
  docs/project/current-state.md docs/project/llm-handoff.md \
  docs/superpowers/reports/2026-07-23-account-quality-monitor-verification.md
git commit -m "feat: wire scheduled account quality evidence"
```

### Task 5: Full Verification And Controlled Installation

**Files:**
- Modify: `docs/superpowers/reports/2026-07-23-account-quality-monitor-verification.md`

**Interfaces:**
- Consumes: tested Tasks 1-4 artifacts and the existing restricted Admin-Key file.
- Produces: enabled account-quality timer, disabled old model-release timer, fresh redacted evidence, and unchanged routing/modes.

- [ ] **Step 1: Run all local gates**

```bash
ruby tests/operations/collect_account_quality_pulse_test.rb
ruby tests/operations/account_quality_monitor_test.rb
bash tests/relay_ops/validate_relay_ops_contract.sh
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang:1.24 \
  sh -c 'GOMAXPROCS=2 go test ./... && go vet ./...'
git diff --check
```

Expected: every command exits zero before any server change.

- [ ] **Step 2: Capture a read-only production baseline**

Record container IDs, relay-ops/D04/Feishu modes, public endpoint status codes, Caddy/route hashes, and timer states. Read no secret values or upstream responses.

- [ ] **Step 3: Install only the new task and result projection**

Install wrapper, collector, unit, timer, `0600` env file, and `0700` evidence directory. Build the tested AMD64 relay-ops image and recreate only relay-ops with `--no-deps --no-build --force-recreate`. Enable/start the account-quality timer. Disable/stop only the old model-release timer; preserve its unit and evidence. The installed service, not an LLM-issued account request, generates the first pulse.

- [ ] **Step 4: Verify one natural task completion**

Prove: new timer enabled/active; old timer disabled/inactive; service success; host/container hashes equal; no response text/Base URL/Key/raw response in evidence; every discovered account has a record; relay-ops healthy/restart 0; other container IDs unchanged; five public endpoints return 200; modes remain `read_only + dry_run`; registration remains closed; route/Caddy hashes unchanged. Do not manufacture balance exhaustion or another fault.

- [ ] **Step 5: Finish report and commit**

```bash
git diff --check
git add docs/superpowers/reports/2026-07-23-account-quality-monitor-verification.md
git commit -m "docs: verify scheduled account quality monitor"
```

## Plan Self-Review

- Tasks 1-2 cover account isolation, native SSE TTFT, explicit balance classification, fixed cadence, and no LLM-generated account requests.
- Task 3 uses the Task 1 schema exactly and preserves existing report ordering rather than creating another score.
- Task 4 replaces only obsolete model-release wiring; Task 5 proves unchanged routes, modes, services, and credentials.
- No provider, account name, account ID, secret, or unfinished placeholder is part of the implementation contract.

## Execution

Execute inline in this session using `superpowers:executing-plans`. No subagents are used because the shared working tree contains unrelated user changes.
