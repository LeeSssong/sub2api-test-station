# Sub2API Native Rolling Model Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reuse Sub2API native model discovery and model-aware scheduling to qualify only approved rolling GPT candidates, show a read-only `可升级` decision in `/ops`, and provide a separately invoked, rollback-capable native promotion command.

**Architecture:** Ruby owns the secret-free policy, candidate benchmark, readiness proposal, and explicit promotion workflow because the existing benchmark/evidence toolchain already lives there. Relay-ops only reads a bounded signed-by-hash JSON result and projects it through the existing hidden-admin `/ops` page. Sub2API remains the only production authority for accounts, credentials, mappings, channels, pricing, and scheduling.

**Tech Stack:** Ruby 3 standard library and Minitest; Go 1.24 standard library and `net/http`; existing relay-ops HTML/CSS; Docker Compose deployment contracts; Sub2API `v0.1.161` Admin API.

## Global Constraints

- Bootstrap public catalog is exactly `gpt-5.5`, `gpt-5.6`, `gpt-5.6-luna`, `gpt-5.6-sol`, and `gpt-5.6-terra`; future policy derives families from discovery instead of a permanent exact-name list.
- Keep at most the latest two approved GPT minor families; discovery never publishes or retires a family automatically.
- Active upstreams are undeleted Sub2API accounts with `status == "active" && schedulable == true`; never select by provider name, hostname, account name, or fixed account ID.
- Candidate compatibility requires three synchronous and three terminal-complete SSE attempts for every candidate/account pair that was discovered on that account.
- Every candidate public model must be covered by the qualified account union in every public group.
- Balance must be at least USD `5.00`; discovery, financial, and quality evidence must be no older than `20 minutes`; quality requires at least `20` samples, success `>= 95%`, error `<= 5%`, TTFT P95 `<= 5s`, and total latency P95 `<= 45s`.
- `/ops` stays read-only, contains no form/input/select/button, and operational data returns HTTP `404` to non-admin callers.
- Proposal generation requires relay-ops `read_only`, Feishu commands `dry_run`, and D04 registration closed.
- Promotion is never callable from `/ops` or a scheduler. It updates only native account model mappings and restricted channel model catalog/pricing.
- Promotion uses optimistic concurrency and a local permission-restricted pre-change snapshot; no encrypted or offsite backup is required.
- Never change account status, schedulable state, groups, route roles, multipliers, balances, credentials, Keys, D04 mode, registration, probe mode, or Feishu mode.
- Preserve unrelated dirty-worktree changes. Stage and commit only files explicitly listed in each task.

---

### Task 1: Rolling GPT Family Policy

**Files:**
- Create: `ops/model-release-policy.rb`
- Create: `config/operations/model-release-policy-v1.yaml`
- Create: `tests/operations/model_release_policy_test.rb`

**Interfaces:**
- Produces: `ModelRelease::Policy.load(path) -> Policy`
- Produces: `Policy#classify(model_id) -> Classification`
- Produces: `Policy#candidate_set(discovered_models:, published_models:) -> CandidateDecision`
- `Classification` exposes `model_id`, `family`, `state`, and `reason_code`.
- `CandidateDecision` exposes `published_families`, `candidate_family`, `candidate_models`, `review_models`, `status`, and `reason_codes`.

- [ ] **Step 1: Write policy tests that define bootstrap, rolling, exclusion, and unknown-suffix behavior**

```ruby
def test_bootstrap_catalog_is_the_approved_55_and_56_text_set
  decision = policy.candidate_set(
    discovered_models: %w[gpt-5.5 gpt-5.6 gpt-5.6-luna gpt-5.6-sol gpt-5.6-terra gpt-5.6-codex],
    published_models: []
  )
  assert_equal %w[gpt-5.5 gpt-5.6 gpt-5.6-luna gpt-5.6-sol gpt-5.6-terra], decision.candidate_models
end

def test_new_minor_proposes_two_family_roll_without_publishing
  decision = policy.candidate_set(
    discovered_models: %w[gpt-5.6 gpt-5.6-sol gpt-5.7 gpt-5.7-terra],
    published_models: %w[gpt-5.5 gpt-5.6 gpt-5.6-sol]
  )
  assert_equal "5.7", decision.candidate_family
  assert_equal %w[gpt-5.7 gpt-5.7-terra], decision.candidate_models
  assert_equal "待测试", decision.status
end

def test_unknown_suffix_is_review_only
  classification = policy.classify("gpt-5.7-orbit")
  assert_equal "review", classification.state
  assert_equal "unknown_model_suffix", classification.reason_code
end
```

- [ ] **Step 2: Run the focused tests and confirm RED**

Run: `ruby -Itest tests/operations/model_release_policy_test.rb`

Expected: FAIL because `ops/model-release-policy.rb` and `ModelRelease::Policy` do not exist.

- [ ] **Step 3: Implement strict policy loading and classification**

The YAML contains schema version `1`, bootstrap exact models, allowed ordinary suffixes (`luna`, `sol`, `terra`), exclusion markers (`codex`, `mini`, `nano`, `image`, `audio`, `realtime`, `preview`), and `published_family_limit: 2`. Reject unknown keys, malformed model IDs, duplicate entries, and a bootstrap set spanning anything other than `5.5` and `5.6`.

Implement `classify` with an anchored GPT family parser and return immutable value objects. Dated aliases and exclusion markers return `excluded`; an unfamiliar suffix returns `review`; base IDs and configured ordinary suffixes return `candidate`.

- [ ] **Step 4: Run the policy tests and confirm GREEN**

Run: `ruby -Itest tests/operations/model_release_policy_test.rb`

Expected: PASS, including deterministic ordering and duplicate normalization cases.

- [ ] **Step 5: Commit only Task 1 files**

```bash
git add ops/model-release-policy.rb config/operations/model-release-policy-v1.yaml tests/operations/model_release_policy_test.rb
git commit -m "feat: add rolling GPT family policy"
```

### Task 2: Candidate-Only Repeated Compatibility Benchmark

**Files:**
- Modify: `ops/upstream-benchmark-v2.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

**Interfaces:**
- Changes: `FastRunner.new(client:, profile:, job_kind:, candidate_models: nil, attempts_per_mode: 1, clock: nil, sleeper: nil)`
- CLI adds: `fast --models PATH --attempts-per-mode N`
- Candidate file schema: `{ "schema_version": 1, "models": ["gpt-5.7", ...] }`
- `catalog_quick` result adds `candidate_models`, `unrelated_models_skipped`, and per-model arrays summarized from repeated attempts.

- [ ] **Step 1: Replace the all-model test with explicit candidate-scope and repetition tests**

```ruby
def test_catalog_quick_only_requests_explicit_discovered_candidates_three_times_per_mode
  runner = UpstreamBenchmarkV2::FastRunner.new(
    client: client_with_models(%w[gpt-5.7 gpt-5.7-sol gpt-4o]),
    profile: profile,
    job_kind: "catalog_quick",
    candidate_models: %w[gpt-5.7 gpt-5.7-sol],
    attempts_per_mode: 3
  )
  result = runner.run(channel_id: "account-10")
  assert_equal 12, generation_requests.length
  refute generation_requests.any? { |request| request.fetch("model") == "gpt-4o" }
  assert_equal %w[gpt-5.7 gpt-5.7-sol], result.dig("metrics", "candidate_models")
end

def test_catalog_quick_fails_before_generation_when_candidate_was_not_discovered
  result = runner_for(discovered: %w[gpt-5.7], candidates: %w[gpt-5.7 gpt-5.7-sol]).run(channel_id: "account-10")
  assert_equal "failed", result.fetch("status")
  assert_equal 0, generation_requests.length
  assert_includes result.fetch("errors").map { |item| item.fetch("category") }, "candidate_not_discovered"
end
```

- [ ] **Step 2: Run focused benchmark tests and confirm RED**

Run: `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb -n '/catalog_quick|candidate_models|attempts_per_mode/'`

Expected: FAIL because `FastRunner` lacks candidate arguments and still selects all discovered text models.

- [ ] **Step 3: Implement fail-closed candidate selection and repeated attempts**

Require an explicit non-empty candidate set for `catalog_quick`; normalize and deduplicate it; reject candidates absent from the live directory before any generation request. Invoke every candidate exactly `attempts_per_mode` times for sync and SSE. A model/account pair passes only when all attempts succeed and every stream is complete.

Add strict CLI parsing for a maximum 256-model, 64 KiB candidate file and `attempts_per_mode` in `1..5`. Dry-run must report exactly `candidate_count * attempts_per_mode * 2` maximum generation requests and one directory request.

- [ ] **Step 4: Run focused tests and the entire benchmark suite**

Run: `ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb`

Expected: PASS; tests prove unrelated discovered models are never requested and dry-run sends zero network requests.

- [ ] **Step 5: Commit only benchmark files**

```bash
git add ops/upstream-benchmark-v2.rb tests/upstream_benchmarks/upstream_benchmark_v2_test.rb
git commit -m "feat: qualify only selected model candidates"
```

### Task 3: Native Discovery Client and Secret-Free Readiness Evaluator

**Files:**
- Create: `ops/evaluate-model-release-readiness.rb`
- Create: `tests/operations/evaluate_model_release_readiness_test.rb`
- Create: `config/operations/model-release-snapshot.example.yaml`
- Modify: `relay-ops-service/internal/sub2api/types.go`
- Modify: `relay-ops-service/internal/sub2api/client.go`
- Modify: `relay-ops-service/internal/sub2api/client_test.go`

**Interfaces:**
- Adds Go: `func (c *HTTPReader) SyncUpstreamModels(ctx context.Context, accountID int64) ([]Model, error)`
- Adds Ruby: `ModelRelease::Evaluator.new(policy:, now:).evaluate(snapshot) -> Hash`
- Result schema includes `schema_version`, `proposal_id`, `evaluated_at`, `status`, `account_set_sha256`, `base_config_sha256`, `published`, `candidate`, `groups`, `accounts`, and `blockers`.

- [ ] **Step 1: Write the native sync wire-contract test**

```go
func TestHTTPReaderSyncUpstreamModelsUsesNativeAdminEndpoint(t *testing.T) {
    // Assert POST /api/v1/admin/accounts/17/models/sync-upstream,
    // x-api-key authentication, bounded body parsing, ID normalization,
    // and no follow-up mutation request.
}
```

- [ ] **Step 2: Write evaluator boundary and coverage tests**

Cover all of these exact outcomes:

```ruby
assert_equal "可升级", evaluate(complete_snapshot).fetch("status")
assert_includes evaluate(missing_group_coverage).fetch("blockers"), "group_model_coverage_incomplete"
assert_includes evaluate(stale_quality).fetch("blockers"), "quality_evidence_stale"
assert_includes evaluate(low_balance).fetch("blockers"), "balance_below_minimum"
assert_includes evaluate(missing_price).fetch("blockers"), "model_pricing_incomplete"
assert_includes evaluate(wrong_modes).fetch("blockers"), "unsafe_operating_mode"
```

Also verify canonical hashes are independent of input ordering, future timestamps fail, secrets/model output are rejected recursively, and each account may qualify only a subset while each group must cover the union.

- [ ] **Step 3: Run focused Go and Ruby tests and confirm RED**

Run: `cd relay-ops-service && go test ./internal/sub2api -run SyncUpstreamModels -count=1`

Run: `ruby -Itest tests/operations/evaluate_model_release_readiness_test.rb`

Expected: FAIL because the client method and evaluator are absent.

- [ ] **Step 4: Implement the exact native sync DTO and evaluator**

The Go client accepts only a successful JSON envelope containing model IDs, caps the response at 2 MiB, rejects duplicates/malformed IDs, and never sends credentials other than the existing server-side Admin key header.

The Ruby evaluator validates a strict secret-free schema; recomputes account-set and base-configuration SHA-256 values from canonical JSON; applies the policy, six-attempt qualification, per-group union coverage, pricing, balance, freshness, quality, and mode gates; and emits stable blocker codes in deterministic order.

- [ ] **Step 5: Run all focused tests and confirm GREEN**

Run: `cd relay-ops-service && go test ./internal/sub2api -count=1`

Run: `ruby -Itest tests/operations/evaluate_model_release_readiness_test.rb`

Expected: PASS.

- [ ] **Step 6: Commit only Task 3 files**

```bash
git add ops/evaluate-model-release-readiness.rb tests/operations/evaluate_model_release_readiness_test.rb config/operations/model-release-snapshot.example.yaml relay-ops-service/internal/sub2api/types.go relay-ops-service/internal/sub2api/client.go relay-ops-service/internal/sub2api/client_test.go
git commit -m "feat: evaluate native model release readiness"
```

### Task 4: Bounded Go Evidence Reader

**Files:**
- Create: `relay-ops-service/internal/modelrelease/result.go`
- Create: `relay-ops-service/internal/modelrelease/result_test.go`
- Modify: `relay-ops-service/internal/config/config.go`
- Modify: `relay-ops-service/internal/config/config_test.go`

**Interfaces:**
- Produces: `modelrelease.Load(path string, now time.Time) (Result, error)`
- Produces: `Result.View(now time.Time) View`
- Adds config: `ModelReleaseResultFile string` from `RELAY_OPS_MODEL_RELEASE_RESULT_FILE`.

- [ ] **Step 1: Write strict loader tests**

```go
func TestLoadAcceptsSecretFreeReadyResult(t *testing.T) {}
func TestLoadRejectsUnknownFieldsSecretsAndOversizeInput(t *testing.T) {}
func TestLoadRejectsHashMismatchDuplicateModelsAndFutureTime(t *testing.T) {}
func TestViewMarksResultStaleAfterTwentyMinutes(t *testing.T) {}
```

Use a 2 MiB maximum, `json.Decoder.DisallowUnknownFields`, anchored lowercase SHA-256 validation, bounded list sizes, unique normalized model IDs, and recursive secret-key rejection.

- [ ] **Step 2: Run the package/config tests and confirm RED**

Run: `cd relay-ops-service && go test ./internal/modelrelease ./internal/config -count=1`

Expected: FAIL because the package and environment field do not exist.

- [ ] **Step 3: Implement loader, presentation view, and optional read-only config**

Missing result file is allowed and renders `未发现更新`; a configured unreadable or invalid file is an explicit unavailable state. The loader never opens a write handle and never invokes Sub2API.

- [ ] **Step 4: Run focused tests and confirm GREEN**

Run: `cd relay-ops-service && go test ./internal/modelrelease ./internal/config -count=1`

Expected: PASS, including permission and freshness cases.

- [ ] **Step 5: Commit only Task 4 files**

```bash
git add relay-ops-service/internal/modelrelease relay-ops-service/internal/config/config.go relay-ops-service/internal/config/config_test.go
git commit -m "feat: load model release evidence"
```

### Task 5: Read-Only `/ops` Model Version Projection

**Files:**
- Modify: `relay-ops-service/internal/http/server.go`
- Modify: `relay-ops-service/internal/http/sources.go`
- Modify: `relay-ops-service/internal/http/server_test.go`
- Modify: `relay-ops-service/internal/http/sources_test.go`
- Modify: `relay-ops-service/internal/http/templates/ops.html`
- Modify: `relay-ops-service/internal/http/static/app.css`
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/app/e2e_test.go`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`

**Interfaces:**
- Adds `ModelRelease modelrelease.View` to `httpserver.OpsView`.
- Adds a read-only `modelrelease.Source` dependency to the existing ops source composition.
- Keeps the sole endpoint `GET /relay-ops/api/ops-view` under `RequireHiddenAdmin`.

- [ ] **Step 1: Write HTTP and source tests for all display states**

```go
func TestOpsShowsReadOnlyModelVersionStatus(t *testing.T) {
    // Require 模型版本, published families, candidate family, group coverage,
    // account subsets, 可升级, freshness, and stable blocker details.
}

func TestOpsModelVersionHasNoMutationControls(t *testing.T) {
    // Reject form/input/select/textarea/button and any model write endpoint.
}
```

Retain and extend existing tests proving empty, invalid, expired, and non-admin tokens receive `404`, while the page refreshes every 30 seconds using `cache: 'no-store'`.

- [ ] **Step 2: Run focused HTTP/app tests and confirm RED**

Run: `cd relay-ops-service && go test ./internal/http ./internal/app -run 'Ops|ModelRelease' -count=1`

Expected: FAIL because `OpsView` and the template lack model-release state.

- [ ] **Step 3: Implement the unframed read-only section**

Render `模型版本` with current published families/models, last sync, candidate family/models, group coverage, per-account qualified subsets, pricing/balance/quality state, and exactly one Chinese status: `未发现更新`, `待确认`, `待测试`, `测试未通过`, `可升级`, or `已发布`. Use tables and compact status labels matching the existing operational UI; do not add controls.

- [ ] **Step 4: Extend static deployment contracts**

Require the result-file read-only mount and environment variable, require the `模型版本` heading and 30-second refresh, and forbid model editor/release controls and any POST/PUT/DELETE model-release route.

- [ ] **Step 5: Run focused and contract tests and confirm GREEN**

Run: `cd relay-ops-service && go test ./internal/http ./internal/app -count=1`

Run: `bash tests/relay_ops/validate_relay_ops_contract.sh`

Expected: PASS.

- [ ] **Step 6: Commit only Task 5 files**

```bash
git add relay-ops-service/internal/http relay-ops-service/internal/app/app.go relay-ops-service/internal/app/e2e_test.go relay-ops-service/internal/http/templates/ops.html relay-ops-service/internal/http/static/app.css tests/relay_ops/validate_relay_ops_contract.sh
git commit -m "feat: show model upgrade readiness in ops"
```

### Task 6: Explicit Native Promoter With Rollback

**Files:**
- Create: `ops/promote-model-release.rb`
- Create: `tests/operations/promote_model_release_test.rb`
- Modify: `relay-ops-service/internal/sub2api/types.go`
- Modify: `relay-ops-service/internal/sub2api/client.go`
- Modify: `relay-ops-service/internal/sub2api/client_test.go`

**Interfaces:**
- Adds Go DTO helpers for exact native Admin API requests used by the standalone command contract tests.
- Ruby CLI: `ruby ops/promote-model-release.rb apply --proposal PATH --snapshot-dir DIR --base-url URL --admin-key-file PATH`
- Optional safe mode: `preflight` performs all re-reads and zero writes.

- [ ] **Step 1: Write preflight, scope, concurrency, rollback, and secret tests**

Tests must prove:

```text
- non-ready/stale/hash-mismatched proposal => zero writes
- preflight => zero writes
- snapshot file is mode 0600 and contains only affected native objects
- apply calls only account bulk-update and affected channel PUT endpoints
- partial native write => restore every changed object and re-read old hash
- successful apply => re-read exact proposal hash
- request/log/output contains no Admin key, upstream key, Base URL credential, or model output
- account status, schedulable, groups, routes, multiplier, balance, credentials, D04, probes, and Feishu modes are absent from write payloads
```

- [ ] **Step 2: Run focused tests and confirm RED**

Run: `ruby -Itest tests/operations/promote_model_release_test.rb`

Run: `cd relay-ops-service && go test ./internal/sub2api -run 'BulkUpdate|UpdateChannel' -count=1`

Expected: FAIL because controlled promotion APIs do not exist.

- [ ] **Step 3: Implement exact native write methods and the standalone promoter**

Use `POST /api/v1/admin/accounts/bulk-update` with credentials merge limited to `model_mapping`, and `PUT /api/v1/admin/channels/:id` limited to the current channel object plus `restrict_models`, `model_mapping`, `model_pricing`, and `billing_model_source`. Before writing, re-read current accounts/groups/channels/mappings/pricing and recompute both hashes. Create the local snapshot with `O_CREAT|O_EXCL`, mode `0600`, and `fsync` before the first native mutation.

On any write or verification failure, restore changed objects in reverse order, re-read the old canonical hash, and exit non-zero. Never mark evidence published unless the final re-read exactly matches the proposal.

- [ ] **Step 4: Run focused tests and confirm GREEN**

Run: `ruby -Itest tests/operations/promote_model_release_test.rb`

Run: `cd relay-ops-service && go test ./internal/sub2api -count=1`

Expected: PASS.

- [ ] **Step 5: Commit only Task 6 files**

```bash
git add ops/promote-model-release.rb tests/operations/promote_model_release_test.rb relay-ops-service/internal/sub2api/types.go relay-ops-service/internal/sub2api/client.go relay-ops-service/internal/sub2api/client_test.go
git commit -m "feat: add controlled native model promoter"
```

### Task 7: Full Verification and Read-Only Production Acceptance

**Files:**
- Modify: `infra/compose.yaml`
- Modify: `infra/Dockerfile.relay-ops`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`
- Create: `docs/superpowers/reports/2026-07-22-sub2api-native-rolling-model-policy-verification.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`

**Interfaces:**
- Mounts a secret-free model-release result as `/run/relay-ops/model-release-result.json:ro`.
- Production acceptance in this task is discovery/proposal/display only; promotion remains a separately approved operation if and only if status is `可升级`.

- [ ] **Step 1: Add deployment-contract tests before changing Compose**

Require `RELAY_OPS_MODEL_RELEASE_RESULT_FILE`, a read-only result mount, inclusion of policy/evaluator assets in the image, no writable model directory, and no browser mutation route.

- [ ] **Step 2: Run the contract and confirm RED**

Run: `bash tests/relay_ops/validate_relay_ops_contract.sh`

Expected: FAIL on the missing model-release mount/assets.

- [ ] **Step 3: Add the minimal read-only deployment wiring**

Copy only policy/evaluator/runtime assets into the image and mount only the secret-free result. Do not mount Sub2API Admin credentials into a new process or add a scheduler that can promote.

- [ ] **Step 4: Run complete local verification**

Run all commands independently and require exit `0`:

```bash
ruby -Itest tests/operations/model_release_policy_test.rb
ruby -Itest tests/operations/evaluate_model_release_readiness_test.rb
ruby -Itest tests/operations/promote_model_release_test.rb
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_v2_test.rb
cd relay-ops-service && GOMAXPROCS=2 go test -p 1 ./... -count=1
cd relay-ops-service && GOMAXPROCS=1 go test -p 1 ./... -race -count=1
cd relay-ops-service && GOMAXPROCS=2 go vet -p 1 ./...
node --check relay-ops-service/internal/http/static/ops-admin.js
bash tests/relay_ops/validate_relay_ops_contract.sh
git diff --check
```

- [ ] **Step 5: Perform bounded read-only production acceptance**

Before discovery, record redacted canonical hashes for active/schedulable accounts, account mappings, public channel mappings/pricing, routes, relay-ops/Feishu/D04 modes, and container IDs. Use native sync for current accounts, evaluate only the policy-selected candidates, and publish only the secret-free result to relay-ops. Do not promote in this step.

Afterward, require the same configuration hashes, routes, balances, Keys, modes, registration state, and unaffected container IDs. Verify `/ops` renders the exact evaluator state and unauthorized access remains `404`. If the state is not `可升级`, record blockers and stop without production writes.

- [ ] **Step 6: Update authoritative documentation**

The report must list discovered/current/candidate families, exact tested candidate set, request count, per-group coverage, freshness, balance, quality, pricing, hashes, mode evidence, and zero-write proof. Update `current-state.md` and `llm-handoff.md` with the status and the next concrete action. Do not name one provider as policy authority.

- [ ] **Step 7: Commit deployment and documentation files**

```bash
git add infra/compose.yaml infra/Dockerfile.relay-ops tests/relay_ops/validate_relay_ops_contract.sh docs/superpowers/reports/2026-07-22-sub2api-native-rolling-model-policy-verification.md docs/project/current-state.md docs/project/llm-handoff.md
git commit -m "docs: verify rolling model release readiness"
```

### Task 8: Separately Approved Promotion Acceptance

**Files:**
- Modify: `docs/superpowers/reports/2026-07-22-sub2api-native-rolling-model-policy-verification.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`

**Interfaces:**
- Consumes a `可升级` proposal whose ID, account-set hash, base configuration hash, candidate set, and freshness still match production.
- Produces a verified published snapshot or a verified rollback to the old snapshot.

- [ ] **Step 1: Stop unless promotion is separately approved and current evidence is still `可升级`**

Run `preflight` first. Any expired evidence, changed account set, changed base hash, missing pricing, mode drift, or D04 opening results in zero writes and a fresh proposal requirement.

- [ ] **Step 2: Apply the bounded native promotion**

Create the local `0600` snapshot and invoke the standalone command once. Do not use `/ops`, Feishu commands, a scheduler, or direct PostgreSQL writes.

- [ ] **Step 3: Verify native and gateway behavior**

Re-read account model subsets and channel restriction/mapping/pricing; verify the new canonical hash, public model list, and one minimal sync plus one terminal-complete SSE gateway request for each newly published public model. Confirm displaced oldest family is absent only after the new family is present and verified.

- [ ] **Step 4: Prove all unrelated state stayed unchanged**

Compare account status/schedulable/groups, routes, multipliers, balances, credential fingerprints, modes, registration, probes, Feishu mode, and unrelated container IDs with preflight evidence. Any mismatch triggers rollback and a failed acceptance result.

- [ ] **Step 5: Record final evidence and commit documentation only**

```bash
git add docs/superpowers/reports/2026-07-22-sub2api-native-rolling-model-policy-verification.md docs/project/current-state.md docs/project/llm-handoff.md
git commit -m "docs: record controlled model promotion"
```
