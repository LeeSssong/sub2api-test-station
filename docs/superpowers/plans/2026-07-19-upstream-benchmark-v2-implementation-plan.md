# Upstream Benchmark V2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade the project with a deterministic V2 evaluator that discovers and tests every text model, probes bounded concurrency/RPM, computes model pricing and 50% margin recommendations, emits an auditable Sub2API proposal, and lets the existing Skill apply only after explicit user approval.

**Architecture:** Keep V1 in `ops/upstream-benchmark.rb` unchanged and add a focused V2 library/CLI that reuses its `HttpClient`, `SseParser`, `Redactor`, `Registry`, and `ValidationError`. The Skill remains responsible for browser-only credentials, supplier pricing/billing evidence, isolated Sub2API work, approval prompts, production application, rollback, and cleanup; the CLI handles deterministic JSON/YAML calculations and report generation.

**Tech Stack:** Ruby 2.6 standard library (`json`, `yaml`, `optparse`, `time`, `securerandom`, `bigdecimal`), Minitest, YAML/JSON artifacts, Codex Skill Markdown.

## Global Constraints

- Never accept or print an upstream Key in chat, files, command arguments, reports, logs, or the ledger; use the signed-in supplier/Sub2API page path.
- V2 must classify all discovered models but automatically test only `text` models; image, audio, realtime, and unknown models remain explicitly unverified.
- Every text model must receive one synchronous and one SSE request before it can be `verified`.
- Concurrency ladder is `1,2,3,5,8,10`; RPM ladder is `6,12,20,30`; stop at the first 429, 5xx, timeout, protocol error, or clear queueing.
- Default pricing assumptions are 10% failure preparation, 50% fully-loaded margin, 0% payment fee unless supplied, internal multiplier `1.0`, and recommendation buffer `0.01`.
- Missing or unexplained prices/charges remain `unknown` and exclude a model from the openable proposal.
- V2 generates proposals only; production writes and upstream switches require an explicit user approval bound to the proposal hash.
- Preserve V1 commands, tests, historical records, and production routing.

---

### Task 1: V2 profile, model catalog, and evidence contracts

**Files:**
- Create: `config/upstream-benchmarks/mvp-text-v2.yaml`
- Create: `config/upstream-benchmarks/pricing-evidence.example.yaml`
- Create: `config/upstream-benchmarks/v2-scenario.example.yaml`
- Create: `ops/upstream-benchmark-v2.rb`
- Create: `tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

**Interfaces:**
- `UpstreamBenchmarkV2::Profile.new(document)` validates bounded prompt, token, timeout, concurrency, RPM, and 10-second RPM window settings.
- `UpstreamBenchmarkV2::ModelCatalog.classify(model_id)` returns one of `text`, `image`, `audio`, `realtime`, `unknown`.
- `UpstreamBenchmarkV2::ModelCatalog.discover(models)` returns non-sensitive model records with `id`, `kind`, and `testable`.
- `UpstreamBenchmarkV2::PricingEvidence.validate!(document)` rejects credentials, invalid prices, invalid currencies, and incomplete model IDs.

- [ ] **Step 1: Write failing tests**

Add tests for the exact contract:

```ruby
def test_profile_accepts_v2_bounds
  profile = UpstreamBenchmarkV2::Profile.new(profile_document)
  assert_equal [1, 2, 3, 5, 8, 10], profile.concurrency_levels
  assert_equal [6, 12, 20, 30], profile.rpm_levels
end

def test_catalog_classifies_non_text_before_text_fallback
  assert_equal "image", UpstreamBenchmarkV2::ModelCatalog.classify("dall-e-3")
  assert_equal "audio", UpstreamBenchmarkV2::ModelCatalog.classify("whisper-1")
  assert_equal "realtime", UpstreamBenchmarkV2::ModelCatalog.classify("gpt-4o-realtime-preview")
  assert_equal "text", UpstreamBenchmarkV2::ModelCatalog.classify("gpt-5.6-sol")
end

def test_pricing_evidence_rejects_secret_shaped_fields
  assert_raises(UpstreamBenchmark::ValidationError) do
    UpstreamBenchmarkV2::PricingEvidence.validate!("api_key" => "secret")
  end
end
```

- [ ] **Step 2: Run the focused test to verify RED**

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

Expected: FAIL because `UpstreamBenchmarkV2` does not exist.

- [ ] **Step 3: Implement the contracts**

Implement the V2 module with strict integer bounds, default `rpm_window_seconds: 10`, and non-text-first classification. Reuse `UpstreamBenchmark::SecretGuard` and `ValidationError`; never persist response content.

- [ ] **Step 4: Run focused tests to verify GREEN**

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

Expected: all Task 1 tests pass.

- [ ] **Step 5: Commit**

```bash
git add ops/upstream-benchmark-v2.rb config/upstream-benchmarks/mvp-text-v2.yaml config/upstream-benchmarks/pricing-evidence.example.yaml config/upstream-benchmarks/v2-scenario.example.yaml tests/upstream_benchmarks/upstream_benchmark_v2_test.rb
git commit -m "feat: add upstream benchmark v2 contracts"
```

### Task 2: Per-model text tests, concurrency, and RPM probing

**Files:**
- Modify: `ops/upstream-benchmark-v2.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

**Interfaces:**
- `UpstreamBenchmarkV2::Runner.new(client:, profile:, clock:, sleeper:).run(channel_id:)` returns one redacted run record.
- `UpstreamBenchmarkV2::CapacityProbe.new(invoke:, profile:, clock:, sleeper:).run` returns `concurrency`, `rpm`, `last_stable`, and `stop_reason` summaries.

- [ ] **Step 1: Write failing tests**

Use a scripted client returning distinct model results and failures:

```ruby
def test_runner_tests_each_text_model_sync_and_stream
  record = v2_runner(models: %w[gpt-a gpt-b dall-e-3]).run(channel_id: "neko")
  assert_equal %w[gpt-a gpt-b], record.dig("metrics", "text_models")
  assert_equal 2, record.dig("metrics", "per_model", "gpt-a", "sync", "success_count")
  assert_equal true, record.dig("metrics", "per_model", "gpt-b", "stream", "complete")
  assert_equal "image", record.dig("metrics", "catalog", "dall-e-3", "kind")
end

def test_capacity_stops_after_rate_limit_and_recommends_previous_stable_level
  probe = UpstreamBenchmarkV2::CapacityProbe.new(
    invoke: -> { @capacity_calls += 1; { "status" => @capacity_calls > 3 ? 429 : 200, "duration_ms" => 1 } },
    profile: capacity_profile,
    clock: monotonic_test_clock,
    sleeper: ->(_seconds) {}
  )
  result = probe.run
  assert_equal 3, result.dig("concurrency", "last_stable")
  assert_equal "rate_limited", result.dig("concurrency", "stop_reason")
end
```

- [ ] **Step 2: Run focused tests to verify RED**

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

Expected: FAIL on missing `Runner`/`CapacityProbe` behavior.

- [ ] **Step 3: Implement minimal runner and probes**

Call `client.models` once, classify every returned model, invoke sync and SSE once for each text model, and summarize only statuses, durations, first-event timing, completion, and usage. Capacity probes must stop on the first failure/queueing signal, pace RPM over `rpm_window_seconds`, and report “at least” when the highest level passes.

- [ ] **Step 4: Run focused and V1 tests**

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_test.rb`

Expected: both commands pass with 0 failures.

- [ ] **Step 5: Commit**

```bash
git add ops/upstream-benchmark-v2.rb tests/upstream_benchmarks/upstream_benchmark_v2_test.rb
git commit -m "feat: benchmark every text model and probe capacity"
```

### Task 3: Pricing advisor and Sub2API proposal builder

**Files:**
- Modify: `ops/upstream-benchmark-v2.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

**Interfaces:**
- `UpstreamBenchmarkV2::PricingAdvisor.new(evidence:, scenario:).calculate` returns model eligibility, per-model multipliers, internal multiplier, commercial floors, and recommendation.
- `UpstreamBenchmarkV2::ProposalBuilder.build(run:, pricing:, proposal_id:, generated_at:)` returns a secret-free proposal hash.
- `UpstreamBenchmarkV2::ProposalBuilder.markdown(report)` renders a user-facing summary without response bodies.

- [ ] **Step 1: Write failing tests**

Cover the Neko calculation and unknown-price blocking:

```ruby
def test_pricing_advisor_recommends_neko_point_eighteen
  result = UpstreamBenchmarkV2::PricingAdvisor.new(
    evidence: neko_pricing_evidence,
    scenario: { "failure_reserve_rate" => 0.10, "target_margin_rate" => 0.50, "payment_fee_rate" => 0.03, "recommendation_increment" => 0.01, "recommendation_buffer" => 0.01, "monthly_fixed_cost_usd" => 0, "monthly_standard_usage_usd" => 1 }
  ).calculate
  assert_in_delta 0.154, result.fetch("commercial").fetch("variable_floor"), 0.0001
  assert_in_delta 0.18, result.fetch("commercial").fetch("recommended_multiplier"), 0.0001
  assert_equal 1.0, result.fetch("internal").fetch("group_multiplier")
end

def test_unknown_input_price_blocks_model_from_proposal
  result = advisor_with_unknown_price.calculate
  refute_includes result.fetch("openable_models"), "gpt-unknown"
  assert_equal "unknown", result.dig("models", "gpt-unknown", "status")
end
```

- [ ] **Step 2: Run focused tests to verify RED**

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

Expected: FAIL because `PricingAdvisor` and `ProposalBuilder` are missing.

- [ ] **Step 3: Implement formulas and proposal rendering**

Use exact decimal arithmetic where practical. Require input/output prices for openable models; allow cache fields to be null only when no cache usage is forecast. Compute:

```ruby
risk_adjusted = cost_multiplier * (1 + reserve)
variable_floor = risk_adjusted / (1 - payment_fee - target_margin)
fixed_per_standard_dollar = monthly_fixed_cost / monthly_standard_usage
full_cost_floor = (risk_adjusted + fixed_per_standard_dollar) / (1 - payment_fee - target_margin)
recommended = ceil(full_cost_floor + recommendation_buffer, increment)
```

Emit `billing_model_source: requested`, `restrict_models: true`, model mapping, model prices, account multiplier, concurrency/RPM recommendations, evidence references, proposal ID, and a SHA-256 proposal hash. Never include credentials or model output.

- [ ] **Step 4: Run focused tests and inspect generated JSON**

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

Run: `ruby -Itests -e 'Dir["tests/**/*_test.rb"].sort.each { |f| require File.expand_path(f) }'`

Expected: all tests pass and generated proposal JSON contains no key-shaped values.

- [ ] **Step 5: Commit**

```bash
git add ops/upstream-benchmark-v2.rb tests/upstream_benchmarks/upstream_benchmark_v2_test.rb
git commit -m "feat: calculate upstream pricing and proposals"
```

### Task 4: V2 CLI, sample artifacts, and ledger integration

**Files:**
- Modify: `ops/upstream-benchmark-v2.rb`
- Create: `config/upstream-benchmarks/v2-scenario-neko.example.yaml`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

**Interfaces:**
- CLI commands:
  - `validate --profile PATH --pricing PATH --scenario PATH`
  - `run --channel ID --key-env NAME [--dry-run]`
  - `advise --pricing PATH --scenario PATH --run PATH --format json|markdown`
  - `proposal --pricing PATH --scenario PATH --run PATH --output PATH`
- `--dry-run` prints model count, text-model count, request estimate, capacity estimate, and no network activity.
- Live runs append a `live_direct` V2 record to the existing runs ledger only after all secret redaction.

- [ ] **Step 1: Write failing CLI tests**

Test dry-run output, unknown price blocking, JSON/Markdown output, proposal hash determinism, and append-only V2 ledger records.

- [ ] **Step 2: Run CLI tests to verify RED**

Run: `ruby tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

Expected: CLI assertions fail because V2 command dispatch and proposal output do not exist.

- [ ] **Step 3: Implement CLI and examples**

Load YAML with safe parsing, reject non-HTTPS channels through the existing registry, reuse runtime environment variables only for direct runs, append V2 records with `profile_id: mvp-text-v2`, and write proposal artifacts only to explicit output paths.

- [ ] **Step 4: Run CLI validation and all tests**

Run: `ruby ops/upstream-benchmark-v2.rb validate --profile config/upstream-benchmarks/mvp-text-v2.yaml --pricing config/upstream-benchmarks/pricing-evidence.example.yaml --scenario config/upstream-benchmarks/v2-scenario-neko.example.yaml`

Run: `ruby ops/upstream-benchmark-v2.rb run --channel neko --key-env UNUSED --dry-run`

Run: `ruby -Itests -e 'Dir["tests/**/*_test.rb"].sort.each { |f| require File.expand_path(f) }'`

Expected: validation succeeds, dry-run says `network_sent: false`, and all tests pass.

- [ ] **Step 5: Commit**

```bash
git add ops/upstream-benchmark-v2.rb config/upstream-benchmarks/v2-scenario-neko.example.yaml tests/upstream_benchmarks/upstream_benchmark_v2_test.rb
git commit -m "feat: expose upstream benchmark v2 cli"
```

### Task 5: Upgrade the benchmark Skill with approval-gated browser workflow

**Files:**
- Modify: `/Users/gongtengxinwen/.codex/skills/benchmark-upstream-channel/SKILL.md`
- Create: `/Users/gongtengxinwen/.codex/skills/benchmark-upstream-channel/references/v2-workflow.md`

**Interfaces:**
- The Skill invokes `ops/upstream-benchmark-v2.rb`, asks the user to create a temporary Key in the signed-in supplier page, and never requests a Key in chat.
- The Skill reports verified facts, recommendations, unknowns, exact proposal ID/hash, and a proposed change list before asking `是否采纳这份 proposal？`.
- Only an explicit `采纳` response authorizes snapshot → isolated apply → production apply → verification → cleanup.

- [ ] **Step 1: Write the workflow reference**

Document the browser credential handoff, per-text-model sync/SSE checklist, price evidence capture, capacity evidence, proposal review, approval binding, Sub2API snapshot/rollback, and cleanup checklist.

- [ ] **Step 2: Update the Skill entrypoint**

Add V2 as the default workflow for requests to add, compare, recheck, or switch an upstream. Keep V1 historical evidence rules and acceptance gates. Explicitly keep direct evidence `unknown` if the browser path cannot safely bridge a provider Key to the direct runner.

- [ ] **Step 3: Validate the Skill**

Run: `python3 /Users/gongtengxinwen/.codex/skills/.system/skill-creator/scripts/quick_validate.py /Users/gongtengxinwen/.codex/skills/benchmark-upstream-channel`

Expected: valid frontmatter and no malformed Skill metadata.

- [ ] **Step 4: Commit the project-side Skill reference and separately report the personal Skill change**

Commit only the project-side Skill reference file created in this task. The personal Skill path is outside the repository and must be verified by file inspection, not staged with project files.

### Task 6: Durable project state and final verification

**Files:**
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`
- Create: `docs/superpowers/reports/2026-07-19-upstream-benchmark-v2-implementation.md`

- [ ] **Step 1: Add the implementation record**

Record the V2 CLI, profile, proposal schema, Skill approval boundary, and remaining runtime unknowns. Do not claim a new upstream passed; this is an implementation report.

- [ ] **Step 2: Run complete verification**

Run all of:

```bash
ruby ops/upstream-benchmark.rb validate
ruby ops/upstream-benchmark-v2.rb validate --profile config/upstream-benchmarks/mvp-text-v2.yaml --pricing config/upstream-benchmarks/pricing-evidence.example.yaml --scenario config/upstream-benchmarks/v2-scenario-neko.example.yaml
ruby -Itests -e 'Dir["tests/**/*_test.rb"].sort.each { |f| require File.expand_path(f) }'
python3 /Users/gongtengxinwen/.codex/skills/.system/skill-creator/scripts/quick_validate.py /Users/gongtengxinwen/.codex/skills/benchmark-upstream-channel
git diff --check
```

Run a targeted secret scan limited to V2 files and inspect the final diff. Expected: all commands exit 0, no secret-shaped values are present, and no production endpoint was contacted.

- [ ] **Step 3: Commit the implementation**

```bash
git add ops/upstream-benchmark-v2.rb config/upstream-benchmarks tests/upstream_benchmarks docs/project/current-state.md docs/project/llm-handoff.md docs/superpowers/reports docs/superpowers/plans/2026-07-19-upstream-benchmark-v2-implementation-plan.md
git commit -m "feat: implement upstream benchmark v2"
```
