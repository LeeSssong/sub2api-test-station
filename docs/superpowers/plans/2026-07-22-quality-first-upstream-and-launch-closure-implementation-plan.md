# Quality-first Upstream and Launch Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the quality-first lightweight upstream loop, validate it with local Sub2API accounts `73/74/75`, complete D04 single-user low-budget production acceptance, and close the Feishu observation boundary.

**Architecture:** Extend the existing vendor-neutral V2 runner and relay-ops comparison/reporting boundaries instead of adding a second benchmark service. Keep live local account credentials in process memory, reuse relay-ops notification and authenticated `/ops` surfaces, and retain proposal-first/manual-only production routing. Execute D04 through its existing bounded acceptance overlay and restore read-only state in an unconditional cleanup phase.

**Tech Stack:** Ruby 2.6 benchmark runner and Minitest; Go 1.24 relay-ops and internal-test-service; PostgreSQL; Docker Compose; Feishu Interactive Cards; Sub2API Admin API.

## Global Constraints

- Quality-first means absolute hard gates before relative scoring; price never overrides a failed quality gate.
- Live upstream testing is limited to local Sub2API accounts `73`, `74`, and `75`.
- Never print or persist account Keys, authorization headers, cookies, passwords, tokens, prompts, or model output.
- No production candidate creation, automatic switch, Feishu switch action, or production route/price/multiplier mutation.
- Keep production relay-ops `read_only` and Feishu commands `dry_run`; do not enter `enabled`.
- D04 may enter the existing bounded `write` overlay only for one isolated user and one USD 20 grant; always restore `read_only` and registration closed.
- Do not handle Neko balance or delete Feishu deduplication records.

---

### Task 1: Fast-run contract and quality gate evaluator

**Files:**
- Modify: `ops/upstream-benchmark-v2.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`
- Create: `config/upstream-benchmarks/quality-first-fast-v1.yaml`
- Create: `config/upstream-benchmarks/local-sub-accounts-20260722.yaml`

**Interfaces:**
- Produces `fast` run records with `job_kind`, direct/gateway views, per-model outcomes, representative roles, percentile metrics, capacity lower bounds, hard gates, weighted score, and recommendation status.
- Credentials are consumed only from the named runtime environment variable.

- [ ] **Step 1: Write failing tests for the new profile and fast record.**

Add tests proving the profile accepts three configured representative roles, `health_pulse`, `catalog_quick`, and `capacity_check`; the runner emits no response content; and dry-run reports exact request bounds.

- [ ] **Step 2: Run the focused Ruby test and confirm RED.**

Run: `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

Expected: failures because `fast`, the quality policy, and the new record fields do not exist.

- [ ] **Step 3: Implement the minimal vendor-neutral fast runner and profiles.**

Keep protocol request construction in existing adapters. Add deterministic nearest-rank percentiles, fixed error categories, job-specific model selection, and exact request estimation. The local account registry contains IDs, display names, base URLs, protocol, and no secret references.

- [ ] **Step 4: Run the focused tests and confirm GREEN.**

Run: `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

Expected: all cases pass with zero network access.

### Task 2: Absolute gates, relative score, and report artifacts

**Files:**
- Modify: `ops/upstream-benchmark-v2.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`
- Modify: `relay-ops-service/internal/compare/service.go`
- Modify: `relay-ops-service/internal/compare/service_test.go`
- Modify: `relay-ops-service/internal/notify/feishu.go`
- Modify: `relay-ops-service/internal/notify/feishu_test.go`

**Interfaces:**
- Produces `blocked`, `needs_evidence`, `not_better`, `review_recommended`, or `eligible_for_manual_switch`.
- Produces a secret-free Markdown/JSON report and an Interactive Card containing the same report ID/hash and `/ops` link.

- [ ] **Step 1: Write failing gate and score tests.**

Cover hard-gate override, unknown price, missing representative role, incomplete SSE, 5% relative regression, gateway overhead thresholds, minimum quality score `80`, and evidence freshness.

- [ ] **Step 2: Run Ruby and Go focused tests and confirm RED.**

Run:

```text
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb
go test ./internal/compare ./internal/notify -count=1
```

- [ ] **Step 3: Implement the evaluator and card/report projection.**

Use weights `40/25/10/15/10`; preserve component evidence and hard-gate reasons. Unknown values score zero and remain explicit. The card contains no switch action.

- [ ] **Step 4: Re-run focused tests and confirm GREEN.**

Expected: all focused tests pass and secret-shaped fixtures remain redacted.

### Task 3: Cadence and management-console dry-run workflow

**Files:**
- Modify: `relay-ops-service/internal/scheduler/scheduler.go`
- Modify: `relay-ops-service/internal/scheduler/scheduler_test.go`
- Modify: `relay-ops-service/internal/http/server.go`
- Modify: `relay-ops-service/internal/http/server_test.go`
- Modify: `relay-ops-service/internal/http/templates/ops.html`
- Modify: `relay-ops-service/internal/http/static/ops-admin.js`
- Modify: `relay-ops-service/internal/http/static/app.css`

**Interfaces:**
- Scheduler claims `health-pulse` every 15 minutes, `catalog-quick` every 6 hours, and `capacity-check` every 24 hours or configuration revision.
- `/ops` displays gate evidence and offers an authenticated dry-run preview only while mode is `read_only`.

- [ ] **Step 1: Write failing cadence, authorization, and preview tests.**

Prove exact intervals, duplicate suppression through scheduler claims, admin-only access, stale report rejection, exact proposal hash binding, and zero controller writes in read-only mode.

- [ ] **Step 2: Run scheduler and HTTP tests and confirm RED.**

Run: `go test ./internal/scheduler ./internal/http -count=1`

- [ ] **Step 3: Implement the minimal cadence and evidence UI.**

Render score components, hard gates, direct/gateway deltas, balance confirmation state, model counts, report/proposal hashes, and a dry-run preview command. Do not add a Feishu switch button.

- [ ] **Step 4: Run focused tests and browser/static checks.**

Run:

```text
go test ./internal/scheduler ./internal/http -count=1
node --check relay-ops-service/internal/http/static/ops-admin.js
```

### Task 4: Live local acceptance with accounts 73/74/75

**Files:**
- Create: `docs/superpowers/reports/2026-07-22-local-sub-account-quality-first-verification.md`
- Append: `config/upstream-benchmarks/ledger/runs.jsonl`

**Interfaces:**
- Consumes local Sub2API account credentials through in-memory environment variables and the secret-free local registry.
- Produces bounded run IDs, gate results, score evidence, and pre/post state hashes.

- [ ] **Step 1: Capture a non-sensitive pre-state.**

Record account IDs, names, status, schedulable flag, group IDs, concurrency, multiplier, sorted model mappings, and a canonical SHA-256. Confirm all three accounts are isolated from customer traffic before testing.

- [ ] **Step 2: Run dry-run for each job and account.**

Confirm exact maximum model, generation, capacity, Token, wall-clock, and error-stop bounds before live execution.

- [ ] **Step 3: Execute direct and isolated local gateway tests.**

Read each account Key from local Sub2API into a process environment variable without printing it. Run the all-model quick profile and representative capacity profile. For gateway measurements, make only one account schedulable at a time through the local Admin API, verify the target account, run the bounded group request, and restore all three to their captured state in an unconditional cleanup.

- [ ] **Step 4: Verify cleanup and write the report.**

Require the post-state canonical hash to equal the pre-state hash. Record quality results, failed/blocked models, unknown pricing or billing, and whether switching remains prohibited.

### Task 5: D04 single-user production acceptance

**Files:**
- Modify: `docs/superpowers/reports/2026-07-21-d04-public-registration-daily-login-verification.md`
- Create: `docs/superpowers/reports/2026-07-22-d04-single-user-low-budget-acceptance.md`

**Interfaces:**
- Consumes `infra/compose.d04-acceptance.yaml` and the existing production-only secret mounts.
- Produces one isolated user, one USD 20 grant, idempotency and three-way reconciliation evidence, followed by restored read-only state.

- [ ] **Step 1: Capture production pre-state and rollback command.**

Record container IDs, D04 mode, registration state, scheduler health, business-table counts, redacted route hash, and current user/balance summary. Validate both Compose files before changing state.

- [ ] **Step 2: Open the bounded write window and register one generated isolated user.**

Use the existing USD 2.00 conservative cost-policy ceiling. Keep the credential only in memory and never print it. Do not send a model request.

- [ ] **Step 3: Verify one grant and same-day idempotency.**

Verify native registration and automatic login, exactly one USD 20 balance adjustment with the stable D04 idempotency key, repeated same-day login with no second adjustment, D04 ledger/provider balance-history/current-balance reconciliation, and no route change.

- [ ] **Step 4: Restore read-only state unconditionally.**

Recreate D04 using only `compose.d04-read-only.yaml`, require `D04_MODE=read_only`, `D04_REGISTRATION_OPEN=false`, healthy/restart `0`, and same-origin empty registration `403 D04_REGISTRATION_CLOSED`.

### Task 6: Feishu closure and durable mainline

**Files:**
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`
- Modify: `docs/superpowers/reports/2026-07-21-feishu-professional-card-production-verification.md`
- Create: `docs/superpowers/reports/2026-07-22-three-mainlines-closure-verification.md`

**Interfaces:**
- Produces the authoritative three-stage mainline and requirement-by-requirement closure matrix.

- [ ] **Step 1: Recheck production Feishu state without sending an event.**

Confirm relay-ops image, health, restart count, `read_only + dry_run`, public endpoint status, route hash, and retained natural-event visual observation.

- [ ] **Step 2: Update project truth.**

Replace the obsolete 190-request mainline with the lightweight quality-first loop; record local account evidence, D04 acceptance outcome, and Feishu as functionally closed with a non-blocking natural-event observation.

- [ ] **Step 3: Run complete verification.**

Run:

```text
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb
ruby ops/upstream-benchmark-v2.rb validate
go test ./... -p 1 -race -count=1
go vet ./...
node --check internal/http/static/ops.js
node --check internal/http/static/ops-admin.js
bash tests/relay_ops/validate_relay_ops_contract.sh
bash tests/internal_test/validate_internal_test_contract.sh
bash tests/infra/validate-baseline.sh
git diff --check
```

- [ ] **Step 4: Perform the completion audit.**

Map every design acceptance criterion to fresh code, test, live-run, production-mode, cleanup, and documentation evidence. Do not close the goal while any mandatory evidence is missing or indirect.
