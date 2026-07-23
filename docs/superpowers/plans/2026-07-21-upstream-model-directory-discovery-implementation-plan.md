# Generic Upstream Model Directory Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a generic V2 `discover` command that records one model-directory request and zero generation requests before any paid qualification run.

**Architecture:** Add a small `DiscoveryRunner` beside the existing full and candidate-watch runners. Reuse `Profile`, `ModelCatalog`, protocol adapters, `HttpClient`, `Redactor`, and `Ledger`; keep credentials at the existing environment-only boundary and keep XM out of execution logic.

**Tech Stack:** Ruby standard library, Minitest, local TCP fake server, existing YAML registry/profile and JSONL ledger.

## Global Constraints

- Make exactly one model-directory request and zero generation requests in live mode.
- Make zero network requests in dry-run mode.
- Never add XM/vendor/hostname/channel-specific execution behavior.
- Never output or persist Keys, environment values, raw upstream errors, Cookies, Tokens, passwords, or secrets.
- Record successful discovery only as `partial / live_direct / discovered_not_qualified`.
- Do not create Keys, candidates, proposals, routes, or production changes in this implementation loop.

---

### Task 1: Discovery Runner Contract

**Files:**
- Modify: `tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`
- Modify: `ops/upstream-benchmark-v2.rb`

**Interfaces:**
- Consumes: `client.models -> {"status", "models", "duration_ms", optional "error"}`.
- Produces: `UpstreamBenchmarkV2::DiscoveryRunner#run(channel_id:) -> Hash`.

- [x] **Step 1: Write failing runner tests**

Add a counting client and assert that success calls `models` once, never calls `generate`, produces sorted/deduplicated model and classification arrays, and remains `discovered_not_qualified`. Add a failure case asserting the output contains a fixed category but no raw secret-shaped error.

- [x] **Step 2: Run RED**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb --name '/discovery_runner/'
```

Expected: `NameError` because `UpstreamBenchmarkV2::DiscoveryRunner` does not exist.

- [x] **Step 3: Implement the minimal runner**

Add `DiscoveryRunner` with exactly one `@client.models` call. Normalize through `ModelCatalog`, sort IDs, set fixed request counts and qualification status, and map failures to the existing fixed categories without serializing response bodies.

- [x] **Step 4: Run GREEN**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb --name '/discovery_runner/'
```

Expected: all selected tests pass.

### Task 2: CLI Dry-run and Live Fake-server Flow

**Files:**
- Modify: `tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`
- Modify: `ops/upstream-benchmark-v2.rb`

**Interfaces:**
- Adds command: `discover`.
- Uses existing options: `--channels`, `--profile`, `--channel`, `--key-env`, `--runs`, `--decisions`, `--dry-run`.

- [x] **Step 1: Write failing CLI tests**

Add a dry-run assertion for one planned directory request, zero generation requests, and zero network requests. Add a local fake-server integration test that responds to the configured model path, records paths, and asserts exactly one request, no generation path, secret-free stdout, and one partial run ledger row.

- [x] **Step 2: Run RED**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb --name '/cli_discover/'
```

Expected: command validation rejects `discover` or produces no result.

- [x] **Step 3: Implement CLI orchestration**

Add `discover` to the fixed command list and dispatch. Build the same protocol adapter and HTTP client as `run`, require a non-empty environment Key only for live mode, invoke `DiscoveryRunner`, append the record through `Ledger`, and print `Redactor.clean(record)`.

- [x] **Step 4: Run GREEN**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb --name '/cli_discover/'
```

Expected: all selected tests pass and the fake server records one model request.

### Task 3: Regression, Documentation, and Handoff

**Files:**
- Modify: `docs/superpowers/reports/2026-07-21-upstream-benchmark-protocol-adapters-verification.md`
- Modify: `docs/superpowers/reports/2026-07-21-xm-upstream-discovery.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`

**Interfaces:**
- Documents the generic command as locally verified but not yet authorized against XM.

- [x] **Step 1: Run focused and full verification**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_test.rb
ruby -Itest tests/upstreams/validate_upstream_test.rb
ruby ops/upstream-benchmark.rb validate
ruby ops/upstream-benchmark-v2.rb validate
git diff --check
```

- [x] **Step 2: Run secret/vendor scans**

```bash
rg -n 'xm|xmhbao' ops/upstream-benchmark-v2.rb ops/upstream-benchmark-protocols.rb
rg -n 'api[_-]?key|authorization|bearer|cookie|password|secret' output config/upstream-benchmarks/ledger
```

Expected: no XM/vendor execution branch and no secret-bearing generated artifact. Existing schema field names in source/tests are reviewed rather than treated as a credential leak.

- [x] **Step 3: Update durable state**

Record that the command is implemented and locally verified, while live XM Plus/Pro directory discovery still requires separate authorization and temporary Key installation. Do not claim XM qualified.

- [x] **Step 4: Review the final diff**

Confirm the diff touches only the generic benchmark command/tests and the relevant docs, preserves existing uncommitted work, and contains no production or supplier mutation.
