# Upstream SSE Capacity and Topology Nonfunctional Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement vendor-neutral sync/SSE/RPM capacity evidence and offline shared-pool/topology acceptance without sending live requests or changing production.

**Architecture:** Keep protocol wire behavior in the existing adapters and add a focused nonfunctional module for v3 profile validation, normalized samples, independent capacity ladders, shared-pool evaluation, observation/drill evidence, and dry-run budgets. The existing V2 runner remains backward compatible; new CLI entry points expose only deterministic planning and offline evidence evaluation unless a later authorization explicitly enables live execution.

**Tech Stack:** Ruby standard library, Minitest, YAML/JSON, existing upstream benchmark protocol adapters.

## Global Constraints

- No vendor, hostname, channel ID, model, public group, or price-specific implementation branch.
- No live request, Key handling, candidate creation, paid probe, route write, balance change, or production deployment in this plan.
- Preserve all valid V1/V2 Chat Completions and Responses behavior.
- Sync and SSE capacity results remain independent; missing evidence is `unknown`.
- Shared accounts remain one `shared_capacity_pool` unless independent capacity is proved.
- Dry-run formulas must exactly match scheduled request counts and remain secret-free.
- Sustained observation and failover/failback evidence are offline evaluations; executing those phases requires separate production authorization.

---

### Task 1: V3 Profile and Exact Request Budget

**Files:**
- Create: `ops/upstream-benchmark-nonfunctional.rb`
- Create: `tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb`
- Create: `config/upstream-benchmarks/bounded-text-capacity-v3.example.yaml`

**Interfaces:**
- Produces: `UpstreamBenchmarkNonfunctional::Profile.new(document)`.
- Produces: `UpstreamBenchmarkNonfunctional::RequestBudget.new(profile:).calculate(model_count:, include_discovery:, topology_verification_requests:)`.

- [x] **Step 1: Write failing tests** for v3 bounds, independent sync/SSE ladders, unsafe or duplicate values, and the exact formulas `D + 2M + W*Csync + W*Csse + R + K` (HTTP) and `2M + W*Csync + W*Csse + R + K` (generation).
- [x] **Step 2: Run RED:** `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb --name '/profile|request_budget/'`. Expected: the module does not exist.
- [x] **Step 3: Implement the minimum validator and calculator** with bounded integers, strict increasing ladders, nearest-rank percentiles, fixed protocol/path delegation, and no secret-shaped fields.
- [x] **Step 4: Run GREEN** with the Step 2 command and validate the example profile.

### Task 2: Normalized Samples and Independent Capacity Ladders

**Files:**
- Modify: `ops/upstream-benchmark-nonfunctional.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb`

**Interfaces:**
- Produces: `Sample.normalize(raw, request_kind:, identity:)`.
- Produces: `CapacityProbe.new(invoke:, profile:, request_kind:, clock:, sleeper:).run`.
- Result keys include `request_kind`, `levels`, `last_stable`, `limit`, `stop_reason`, `recommendation`, and normalized aggregate metrics.

- [x] **Step 1: Write failing tests** proving sync and SSE invokes are distinct, SSE requires a terminal event, overlap is measured, one qualifying failure stops escalation, P50/P95 include `n`, and an unmeasured queue remains `unknown`.
- [x] **Step 2: Run RED:** `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb --name '/sample|capacity/'`.
- [x] **Step 3: Implement normalized samples, nearest-rank metrics, barriers, fail-fast ladders, and conservative `floor(0.8 * last_stable)` recommendations.**
- [x] **Step 4: Run GREEN** with the Step 2 command and then the complete new test file.

### Task 3: Shared Capacity Pool and Topology Evidence

**Files:**
- Modify: `ops/upstream-benchmark-nonfunctional.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb`
- Create: `config/upstream-benchmarks/topology-scenario-v3.example.yaml`

**Interfaces:**
- Produces: `TopologyScenario.new(document)` with role/account/pool isolation validation and stable SHA-256.
- Produces: `SharedCapacityPoolEvaluator.new(scenario:, thresholds:).evaluate(samples:)`.
- Produces: `ObservationEvaluator#evaluate(windows:)` and `DrillEvaluator#evaluate(timeline:)`.

- [x] **Step 1: Write failing tests** for duplicate primary identities, a backup shared across two groups, aggregate/per-member fairness, missing member evidence, pending unapproved thresholds, 24-hour window completeness, and exact failover/failback timeline calculations.
- [x] **Step 2: Run RED:** `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb --name '/topology|shared_pool|observation|drill/'`.
- [x] **Step 3: Implement strict scenario validation and pure evidence evaluators.** Never construct HTTP requests or route writes in these types.
- [x] **Step 4: Run GREEN** and verify the example topology contains no vendor-specific execution fields.

### Task 4: Safe CLI Integration

**Files:**
- Modify: `ops/upstream-benchmark-v2.rb`
- Modify: `ops/upstream-benchmark-nonfunctional.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb`

**Interfaces:**
- Adds: `capacity-dry-run --profile PATH --model-count N [--include-discovery] [--topology-verification-requests N]`.
- Adds: `topology-dry-run --scenario PATH --evidence PATH` for offline evaluation only.

- [x] **Step 1: Write failing CLI tests** proving both commands send zero network requests, reject unknown/missing bounds, redact secrets, and produce stable profile/scenario hashes.
- [x] **Step 2: Run RED:** `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb --name '/cli/'`.
- [x] **Step 3: Wire deterministic commands into the existing CLI.** Do not add a live capacity or route mutation command in this authorization scope.
- [x] **Step 4: Run GREEN** with the Step 2 command.

### Task 5: Regression, Documentation, and Closure

**Files:**
- Modify: `docs/superpowers/reports/2026-07-21-d04-nonfunctional-baseline.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`
- Create: `docs/superpowers/reports/2026-07-21-upstream-sse-capacity-and-topology-nonfunctional-verification.md`

- [x] **Step 1: Run full Ruby regression:** `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`, `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb`, `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_test.rb`, and the new suite.
- [x] **Step 2: Run static and compatibility gates:** `ruby -c ops/upstream-benchmark-nonfunctional.rb`, `ruby -c ops/upstream-benchmark-v2.rb`, `ruby ops/upstream-benchmark-v2.rb validate`, source scans for vendor branches and secrets, and `git diff --check`.
- [x] **Step 3: Re-run D04 Go verification** with the repository's Go 1.24 container and record the actual result; do not infer success from prior sessions.
- [x] **Step 4: Update the report and handoff** to state that tooling is implemented offline while XM/Wawazz topology remains `NOT_READY` until separately approved live discovery, qualification, shared-pool observation, and drills complete.

### Task 6: P0 Evidence Integrity Hardening

**Reason for reopening:** Independent read-only review found that the original happy-path tests did not prove the stronger evidence-integrity claims in this specification. The offline evaluator could accept samples with incomplete role identity, sequential shared-pool traffic, all-failed observation windows, or route-state strings without read-after-write and sync/SSE proof. Live capacity or topology qualification remains blocked until this task is complete.

**Files:**
- Modify: `ops/upstream-benchmark-nonfunctional.rb`
- Modify: `ops/upstream-benchmark-v2.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb`
- Modify: `config/upstream-benchmarks/topology-scenario-v3.example.yaml`
- Modify: `docs/superpowers/checklists/2026-07-21-d04-xm-gated-production-acceptance.md`

- [x] **Step 1: Write failing evidence-integrity tests** for full role identity, required sync/SSE kinds, isolated/equal/approved-mix shared-pool phases, demonstrated aggregate overlap, all-failed or threshold-breaching observation windows, route read-after-write evidence, backup/primary sync+SSE proof, and the primary recovery window.
- [x] **Step 2: Run RED:** `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_nonfunctional_test.rb --name '/identity|shared_pool|observation|drill/'` and confirm each new test fails for the intended missing gate.
- [x] **Step 3: Implement fail-closed evaluators** without adding any live request or route-write command. Evidence must bind to the scenario role's channel, account reference, model, profile/hash, location, request kinds and non-secret run metadata.
- [x] **Step 4: Write RED tests for execution safety** covering request/Token/currency/wall-clock budgets, latency/queue stops, target RPM start timing, v3 command defaults, and complete summary metadata.
- [x] **Step 5: Implement the minimum execution-safety contracts** and rerun the focused suite. No credential handling or network transport is added.
- [x] **Step 6: Run the complete V1/V2/protocol/v3 regression, static validation, supplier-branch scan and `git diff --check`; update the verification report and authority docs with the reopened-and-fixed result.**

## Acceptance

- [x] Existing V1/V2 suites remain green.
- [x] V3 dry-run budgets are exact and make zero network requests.
- [x] Sync, SSE, and RPM evidence have separate stable bounds and failure semantics.
- [x] Shared-pool evaluation binds complete role identity, three required phases and demonstrated aggregate overlap before reporting pass.
- [x] Observation/drill evaluators fail closed on unhealthy windows, missing route proofs, missing sync/SSE evidence or an unproved recovery window.
- [x] Capacity/RPM execution contracts enforce approved request, Token, currency and wall-clock ceilings and expose target-rate/latency stop reasons.
- [x] No supplier-specific branch, production write, deployment, Key, balance change, or paid request occurs.
