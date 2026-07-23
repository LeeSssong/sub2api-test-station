# D04 Launch Readiness And Quality Report Production Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the remaining launch-readiness work and deploy the verified quality-report monitoring/Feishu increment without opening registration, enabling probes, or changing production routing.

**Architecture:** Add a deterministic offline launch gate beside the existing OPS01 evaluator, prepare a launch-only Compose overlay with an unconditional read-only rollback, and verify production recoverability through an isolated PostgreSQL restore. Deploy relay-ops as a single-container image replacement and validate only read-only behavior because candidates and paid probe mode remain disabled.

**Tech Stack:** Ruby 2.6/Minitest/YAML/JSON; Docker Compose; PostgreSQL 18 `pg_dump`/`pg_restore`; Go 1.24; Feishu Interactive Cards.

## Global Constraints

- Use only the already completed local Sub2API account evidence for `73`, `74`, and `75`; do not generate another upstream request.
- Keep D04 `read_only` with registration closed and do not create another user or grant.
- Keep relay-ops `read_only`, Feishu commands `dry_run`, candidates `0`, and paid probes `0`.
- Do not change routes, multipliers, prices, balances, Keys, account bindings, or Feishu deduplication rows.
- Do not trigger a synthetic or manufactured production event.
- Do not process Neko balance.
- Do not print or persist credentials, prompts, model output, cookies, passwords, or tokens.

---

### Task 1: Deterministic D04 launch-readiness gate

**Files:**
- Create: `ops/evaluate-d04-launch-readiness.rb`
- Create: `tests/operations/evaluate_d04_launch_readiness_test.rb`
- Create: `config/operations/D04-launch-readiness-v1.yaml`
- Create: `config/operations/d04-launch-snapshot.example.yaml`

**Interfaces:**
- Consumes: a versioned threshold policy and secret-free live snapshot.
- Produces: JSON `decision`, stable blocking reason codes, required actions, and zero-action booleans.

- [x] **Step 1: Add RED tests for validation and fail-closed decisions.**

Cover a healthy `go` snapshot, insufficient provider balance, missing spend rate, stale metrics, low sample count, low success rate, high error/TTFT/total latency, stale backup/restore, wrong D04/relay-ops modes, missing operator ownership, and credential-shaped input rejection.

- [x] **Step 2: Run the focused test and confirm the expected missing-implementation failure.**

Run: `ruby -Itest tests/operations/evaluate_d04_launch_readiness_test.rb`

- [x] **Step 3: Implement strict validators, evaluator, and CLI.**

The CLI is `ruby ops/evaluate-d04-launch-readiness.rb evaluate POLICY SNAPSHOT`; it must never contact a network or execute an action.

- [x] **Step 4: Run the focused test and confirm GREEN.**

Run: `ruby -Itest tests/operations/evaluate_d04_launch_readiness_test.rb`

### Task 2: Launch overlay, rollback, and operator checklist

**Files:**
- Create: `infra/compose.d04-launch.yaml`
- Create: `docs/superpowers/checklists/2026-07-22-d04-controlled-launch-readiness.md`
- Modify: `docs/runbooks/operations-and-incident-response.md`
- Modify: `tests/internal_test/validate_internal_test_contract.sh`

**Interfaces:**
- Produces: a preparation-only launch overlay and a tested rollback path to the independent read-only Compose project.

- [x] **Step 1: Add failing contract checks.**

Require the launch overlay to set `write`, registration true, 15 users, USD 20 credit, USD 100 budget, 1000 bps, qualified policy, no port publication, and the same service/image boundary as read-only. Require runbook text to forbid applying it without a fresh `go` artifact and explicit approval.

- [x] **Step 2: Run the D04 contract and confirm RED.**

Run: `bash tests/internal_test/validate_internal_test_contract.sh`

- [x] **Step 3: Add the overlay, checklist, and current rollback commands.**

Replace obsolete invitation/referral/check-in operations text with the current public-registration and daily-login-credit policy.

- [x] **Step 4: Re-run the D04 and infrastructure contracts.**

Run:

```text
bash tests/internal_test/validate_internal_test_contract.sh
bash tests/infra/validate-baseline.sh
```

### Task 3: Fresh production backup and isolated restore

**Files:**
- Modify: `docs/superpowers/checklists/backup-and-restore-live-acceptance.md`
- Create: `docs/superpowers/reports/2026-07-22-production-backup-restore-verification.md`
- Server-only: `/opt/sub2api/production/backups/postgres/`
- Server-only: `/opt/sub2api/production/evidence/backup-restore-20260722/`

**Interfaces:**
- Produces: one restricted `pg_dump -Fc` archive, SHA-256, `pg_restore --list` evidence, isolated restore aggregates, and cleanup proof.

- [x] **Step 1: Capture non-sensitive pre-state and validate space/version.**

Record database size, PostgreSQL image/version, service health, disk free space, and aggregate schema/table counts without printing environment values.

- [x] **Step 2: Create and validate the custom-format backup.**

Use the database container environment internally, redirect archive bytes directly to a `0600` host file, record SHA-256/size/duration, and require `pg_restore --list` success.

- [x] **Step 3: Restore to an isolated PostgreSQL 18 container.**

Use a new internal network and volume, no published port, different database name, and `pg_restore --exit-on-error`. Compare aggregate tables, migrations, admin users, settings, and relay-ops schema evidence.

- [x] **Step 4: Remove the temporary container/network/volume and write the report.**

Retain only the restricted archive and non-sensitive evidence. Record encrypted off-site backup as not yet configured.

### Task 4: Current production readiness decision

**Files:**
- Create: `docs/superpowers/reports/2026-07-22-d04-launch-readiness-verification.md`
- Git-ignored/runtime-only: `config/operations/d04-launch-snapshot.local.yaml`

**Interfaces:**
- Produces: a timestamped `go` or `no_go` artifact using current Wawazz balance/quality, backup, modes, and ownership evidence.

- [x] **Step 1: Capture a secret-free live snapshot.**

Use current Wawazz aggregate balance/spend/quality facts, service health, D04 reconciliation, backup age, restore age, registration state, and operator role labels.

- [x] **Step 2: Evaluate the snapshot.**

Run: `ruby ops/evaluate-d04-launch-readiness.rb evaluate config/operations/D04-launch-readiness-v1.yaml config/operations/d04-launch-snapshot.local.yaml`

- [x] **Step 3: Verify registration remains closed.**

Require D04 `read_only`, `D04_REGISTRATION_OPEN=false`, healthy/restart `0`, and same-origin registration `403 D04_REGISTRATION_CLOSED`.

- [x] **Step 4: Record the exact blocking reasons or opening prerequisites.**

Do not weaken thresholds to obtain `go`; if current balance or evidence fails, close the preparation stage with a documented `no_go` and a separately approvable remedy.

### Task 5: Quality-report relay-ops production deployment

**Files:**
- Modify: `docs/runbooks/relay-ops-monitoring.md`
- Create: `docs/superpowers/reports/2026-07-22-quality-report-feishu-production-verification.md`
- Server-only: `/opt/sub2api/production/relay-ops-service/`
- Server-only: `/opt/sub2api/production/compose.yaml`

**Interfaces:**
- Produces: a pinned AMD64 relay-ops image containing the quality-report execution wiring, with production still read-only and no generated event.

- [x] **Step 1: Run full local verification and capture production pre-state.**

Require Go race/vet, JavaScript syntax, deployment contracts, current image/modes, row counts, routing hashes, and base container IDs.

- [x] **Step 2: Build a fixed AMD64 image on the production host.**

Synchronize only non-secret relay-ops source, benchmark runner/config needed by the image, and Dockerfile into the existing restricted build directory. Tag the image `sub2api-relay-ops:quality-report-read-only-20260722-v1`.

- [x] **Step 3: Atomically update Compose and recreate only relay-ops.**

Validate Compose before `up -d --no-deps --force-recreate relay-ops`; retain the old image and Compose backup for rollback.

- [x] **Step 4: Accept read-only behavior without an event.**

Require healthy/restart `0`, `/healthz` and `/readyz` success, `read_only + dry_run`, candidates/probe runs unchanged at zero, notification rows unchanged, and no scheduled fast-run claim in read-only mode. Compare routing hashes and base container IDs.

### Task 6: Final verification and durable mainline

**Files:**
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`
- Modify: `docs/superpowers/reports/2026-07-22-three-mainlines-closure-verification.md`

**Interfaces:**
- Produces: a requirement-by-requirement completion audit for the original three work lines.

- [x] **Step 1: Run all relevant local tests.**

```text
ruby -Itest tests/operations/evaluate_d04_launch_readiness_test.rb
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb
ruby ops/upstream-benchmark-v2.rb validate
cd relay-ops-service && go test ./... -p 1 -race -count=1 && go vet ./...
cd internal-test-service && go test ./... -p 1 -race -count=1 && go vet ./...
node --check relay-ops-service/internal/http/static/ops.js
node --check relay-ops-service/internal/http/static/ops-admin.js
bash tests/relay_ops/validate_relay_ops_contract.sh
bash tests/internal_test/validate_internal_test_contract.sh
bash tests/infra/validate-baseline.sh
git diff --check
```

- [x] **Step 2: Recheck final production safety state.**

Confirm D04 read-only/closed registration, relay-ops read-only/dry-run, service health, route hashes, zero candidate/probe changes, D04 one-user/one-grant reconciliation, and backup artifact permissions.

- [x] **Step 3: Update authoritative state and reports.**

State that upstream evaluation is closed, launch preparation is complete with the current `go`/`no_go` verdict, and quality-report monitoring is deployed without paid execution.

- [x] **Step 4: Perform the completion audit.**

Map all three original work lines to live evidence. Do not mark the persistent goal complete if any required implementation, rollback, verification, or documentation item remains unproven.
