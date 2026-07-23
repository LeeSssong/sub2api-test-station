# Upstream Benchmark Registry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a reusable OpenAI-compatible upstream benchmark, append-only evidence registry, comparison workflow, and Codex Skill, then run the same baseline for Neko and compare it with Aliu.

**Architecture:** A Ruby standard-library CLI owns validation, direct HTTP/SSE measurements, JSONL persistence, historical imports, comparisons, and decisions. Project YAML defines channels and benchmark profiles; a personal Codex Skill wraps the CLI with the browser-assisted Sub2API, billing, terms, and cleanup workflow. Secrets remain runtime-only environment variables.

**Tech Stack:** Ruby 2.6 standard library, Minitest, YAML, JSON/JSONL, Net::HTTP, Codex Skill Markdown.

## Global Constraints

- Do not store or print API keys, cookies, passwords, TOTP data, authorization headers, full prompts, or full model output.
- Do not recharge, pay, switch production traffic, or modify the production Aliu account.
- Treat historical imports, direct runs, gateway runs, billing evidence, network evidence, and terms evidence as distinct evidence levels.
- Keep image, audio, and realtime billing outside the `mvp-text-v1` profile.
- Use append-only ledger entries; corrections reference old IDs with `supersedes`.

---

### Task 1: Ledger contracts and validation

**Files:**
- Create: `tests/upstream_benchmarks/upstream_benchmark_test.rb`
- Create: `ops/upstream-benchmark.rb`
- Create: `config/upstream-benchmarks/channels.yaml`
- Create: `config/upstream-benchmarks/mvp-text-v1.yaml`
- Create: `config/upstream-benchmarks/ledger/runs.jsonl`
- Create: `config/upstream-benchmarks/ledger/decisions.jsonl`

**Interfaces:**
- Produces: `UpstreamBenchmark::Registry`, `Profile`, `Ledger`, and CLI `validate`.
- Ledger methods: `append_run(record)`, `append_decision(record)`, `runs`, `decisions`, `validate!`.

- [ ] **Step 1: Write failing contract tests**

Test valid channel/profile loading, unknown channel IDs, duplicate run IDs, invalid timestamps, secret-shaped values, append-only writes, and `supersedes` corrections.

- [ ] **Step 2: Verify RED**

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_test.rb`

Expected: failure because `ops/upstream-benchmark.rb` does not exist.

- [ ] **Step 3: Implement minimal registry, profile, ledger, redaction, and validate command**

Use `YAML.safe_load`, one JSON object per line, UTC ISO 8601 timestamps, UUID run IDs, recursive secret-key/value rejection, and atomic single-line append with file locking.

- [ ] **Step 4: Verify GREEN**

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_test.rb`

Expected: all Task 1 tests pass.

### Task 2: Direct benchmark runner

**Files:**
- Modify: `tests/upstream_benchmarks/upstream_benchmark_test.rb`
- Modify: `ops/upstream-benchmark.rb`

**Interfaces:**
- Produces: `UpstreamBenchmark::Runner#run`, `HttpClient`, `SseParser`, `Metrics`.
- CLI: `run --channel ID --profile PATH --key-env NAME [--dry-run]`.

- [ ] **Step 1: Write failing runner tests**

Start a local WEBrick fixture and test `/models`, non-streaming JSON, complete SSE, missing terminal SSE, HTTP 429/5xx classification, timeout classification, 2/3 concurrency wall time, P50/P95 calculation, request/output caps, and response-body redaction.

- [ ] **Step 2: Verify RED**

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_test.rb`

Expected: runner-related assertions fail because the interfaces are missing.

- [ ] **Step 3: Implement the bounded runner**

Use `Net::HTTP` with TLS verification, bearer auth supplied only at request time, monotonic clocks, Thread-based concurrency, structured error categories, and summary-only persisted responses.

- [ ] **Step 4: Verify GREEN and regression**

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_test.rb`

Run: `ruby -Itests -e 'Dir["tests/**/*_test.rb"].sort.each { |f| require File.expand_path(f) }'`

Expected: new and existing test suites pass.

### Task 3: Historical import, comparison, and decisions

**Files:**
- Modify: `tests/upstream_benchmarks/upstream_benchmark_test.rb`
- Modify: `ops/upstream-benchmark.rb`
- Create: `config/upstream-benchmarks/imports/aliu-20260717.yaml`
- Create: `config/upstream-benchmarks/imports/neko-20260718.yaml`

**Interfaces:**
- CLI: `import --channel ID --file PATH`, `compare [--as-of TIME] [--format json|markdown]`, `decide --file PATH`.
- Comparison selects the latest record at or before `as-of` per channel and never converts unknown evidence into a pass.

- [ ] **Step 1: Write failing import/comparison tests**

Test evidence-source preservation, historical confidence, latest-run selection, current failure visibility, unknown-field display, deterministic ordering, decision references, and Markdown escaping.

- [ ] **Step 2: Verify RED**

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_test.rb`

Expected: import/compare/decide assertions fail.

- [ ] **Step 3: Implement commands and non-sensitive fixtures**

Encode only facts already present in project reports. Mark unavailable Aliu metrics and unverified Neko facts as `unknown`; do not invent samples.

- [ ] **Step 4: Verify GREEN and generate initial comparison**

Run imports into a temporary ledger first, validate, then import into the project ledger once. Run `compare --format markdown` and save the derived report under `docs/superpowers/reports/`.

### Task 4: Reusable Codex Skill

**Files:**
- Create: `/Users/gongtengxinwen/.codex/skills/benchmark-upstream-channel/SKILL.md`
- Create: `/Users/gongtengxinwen/.codex/skills/benchmark-upstream-channel/agents/openai.yaml`
- Create: `/Users/gongtengxinwen/.codex/skills/benchmark-upstream-channel/references/acceptance-gates.md`

**Interfaces:**
- Trigger on requests to test, qualify, compare, select, recheck, or switch an upstream relay/channel.
- Skill calls the project CLI, uses Chrome only for signed-in dashboards, and enforces isolated Sub2API objects and cleanup.

- [ ] **Step 1: Initialize using `init_skill.py`**

Create the skill with `references` resources and generated UI metadata.

- [ ] **Step 2: Write workflow and acceptance gates**

Require direct, gateway, billing, network, terms, decision, and cleanup stages; explicitly separate verified, partial, failed, and unknown evidence.

- [ ] **Step 3: Validate the Skill**

Run `quick_validate.py` and inspect the generated metadata. Execute one dry-run invocation against the project profile without a Key.

### Task 5: Neko equal-baseline verification

**Files:**
- Append: `config/upstream-benchmarks/ledger/runs.jsonl`
- Append: `config/upstream-benchmarks/ledger/decisions.jsonl`
- Create or update: `docs/superpowers/reports/2026-07-18-upstream-channel-comparison.md`
- Update: `docs/superpowers/reports/2026-07-18-neko-upstream-short-verification.md`

**Interfaces:**
- Consumes a fresh low-limit Neko Key via an environment variable or a browser-entered isolated Sub2API account.
- Produces direct and gateway run records with explicit missing evidence.

- [ ] **Step 1: Execute direct benchmark**

Run the fixed `mvp-text-v1` profile for Neko with bounded cost and no secret output.

- [ ] **Step 2: Execute isolated gateway validation**

Create dated Neko test account/group/user/Key, run synchronous and streaming calls, and record Sub2API usage. Do not bind production users.

- [ ] **Step 3: Reconcile billing and network evidence**

Capture Neko dashboard actual charge and Token figures if the signed-in console exposes them; record unavailable facts as unknown. Run from the production host only if the existing safe SSH path is available.

- [ ] **Step 4: Clean up and compare**

Delete/disable temporary downstream Key, user, group, and Sub2API account; preserve Aliu. Generate the Aliu/Neko comparison and append a decision record without switching production traffic.

### Task 6: Project handoff and final verification

**Files:**
- Update: `docs/project/current-state.md`
- Update: `docs/project/llm-handoff.md`

- [ ] **Step 1: Run complete verification**

Run all Ruby tests, CLI validation, Skill validation, Markdown fence checks, `git diff --check`, and a secret-pattern scan limited to created/modified benchmark artifacts.

- [ ] **Step 2: Review acceptance criteria and production state**

Confirm the production Aliu account and `openAI` group remain enabled and unchanged except for the already-approved `0.15x` group multiplier. Confirm test resources are absent.

- [ ] **Step 3: Update durable state**

Record exact evidence, unknowns, current channel choice, and the next 24-to-72-hour observation loop.
