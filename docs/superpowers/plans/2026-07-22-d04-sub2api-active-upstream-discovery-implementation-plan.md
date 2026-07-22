# D04 Sub2API Active Upstream Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the manually selected D04 launch-gate upstream with the complete Sub2API-discovered set of schedulable active accounts, and expose each account's independent, read-only readiness result in `/ops`.

**Architecture:** A bounded Admin-GET account reader provides membership; a small snapshot command joins only account-ID-keyed, server-local, non-secret balance and natural-quality evidence. A versioned Ruby evaluator accepts only schema v3 snapshots and emits a generic account-scoped decision artifact. Relay-ops consumes that artifact as a read-only `/ops` projection. Neither collection, evaluation, nor rendering owns scheduling, routing, balances, keys, probes, D04 opening, or Feishu delivery.

**Tech Stack:** Go 1.24.13 (`net/http`, `encoding/json`); Ruby 2.6 (`YAML`, `JSON`, Minitest); existing relay-ops `/ops` HTML/JS; Docker Compose read-only bind mounts.

## Global Constraints

- Membership is exactly `status == "active" && schedulable == true` from Sub2API Admin GET account data.
- Do not branch on provider name, hostname, group name, or fixed account ID.
- Every discovered account must pass balance, freshness, runtime availability, and account-attributed natural-quality gates independently.
- Evidence files may only augment a discovered `account_id`; they cannot add, remove, or select accounts.
- Use Admin GET only. Do not invoke a model endpoint, paid probe, route/scheduling mutation, balance mutation, candidate intake, Feishu send, or D04 write path.
- Keep D04 `read_only` with registration closed and relay-ops `read_only + dry_run`.
- Do not process Neko balance. Historic v1/v2 files and reports remain untouched.
- Preserve unrelated dirty worktree changes. Only stage files explicitly changed for this v3 feature.

---

### Task 1: Bounded Sub2API account-list contract

**Files:**
- Modify: `relay-ops-service/internal/sub2api/types.go`
- Modify: `relay-ops-service/internal/sub2api/client.go`
- Modify: `relay-ops-service/internal/sub2api/client_test.go`

**Interfaces:**
- Add `ListAccounts(context.Context) ([]Account, error)` to the new narrow `sub2api.AccountReader` contract.
- Use `GET /api/v1/admin/accounts?page=<n>&page_size=100`, with at most 20 pages and 2,000 accounts.
- Decode only `data.items`, `data.total`, `data.page`, and `data.page_size`; reject absent or inconsistent pagination, duplicate IDs, and no-progress pages.

- [ ] **Step 1: Write failing pagination tests.**

Add `TestReaderListsAccountsAcrossPages` with two responses shaped as:

```go
fmt.Fprint(w, `{"data":{"items":[{"id":11,"status":"active","schedulable":true}],"total":2,"page":1,"page_size":100}}`)
fmt.Fprint(w, `{"data":{"items":[{"id":12,"status":"disabled","schedulable":false}],"total":2,"page":2,"page_size":100}}`)
```

Assert ordered IDs `11,12`, the admin header on every request, and GET-only methods. Add cases for absent `total`, a repeated ID, and an empty page before `total`; each must return `IsSchemaMismatch(err) == true`.

- [ ] **Step 2: Run the focused test and verify RED.**

```bash
cd relay-ops-service && go test ./internal/sub2api -run 'TestReaderListsAccounts' -count=1
```

Expected: compile failure because `ListAccounts` does not exist.

- [ ] **Step 3: Implement the minimal read-only method.**

```go
type accountPage struct {
    Items    *[]Account `json:"items"`
    Total    *int       `json:"total"`
    Page     *int       `json:"page"`
    PageSize *int       `json:"page_size"`
}

func (c *HTTPReader) ListAccounts(ctx context.Context) ([]Account, error) {
    // Validate each page before appending; return no partial result.
}
```

Keep `Account` limited to existing non-secret metadata. Do not log or decode credential values.

- [ ] **Step 4: Run the package suite and verify GREEN.**

```bash
cd relay-ops-service && go test ./internal/sub2api -count=1
```

Expected: exit `0`.

### Task 2: Discovery and secret-free v3 snapshot collector

**Files:**
- Create: `relay-ops-service/internal/d04readiness/collector.go`
- Create: `relay-ops-service/internal/d04readiness/collector_test.go`
- Create: `relay-ops-service/cmd/d04-readiness-snapshot/main.go`
- Create: `config/operations/d04-upstream-balance-evidence-v3.example.json`
- Create: `config/operations/d04-upstream-quality-evidence-v3.example.json`
- Create: `config/operations/d04-lightweight-launch-base-v3.example.json`
- Create: `config/operations/d04-lightweight-launch-snapshot-v3.example.json`

**Interfaces:**
- `Collector{Accounts AccountLister, Clock func() time.Time}.Collect(context.Context, Inputs) (Snapshot, error)`.
- `Snapshot.ActiveUpstreams []ActiveUpstream` is sorted by `AccountID`.
- `Snapshot.UpstreamDiscovery.AccountSetSHA256` hashes canonical JSON of ID, status, schedulable, and sorted group IDs.
- Evidence inputs are strict JSON arrays keyed by `account_id`; they never define membership.

- [ ] **Step 1: Write failing discovery tests.**

Use accounts with states:

```go
[]sub2api.Account{
    {ID: 42, Status: "active", Schedulable: true, GroupIDs: []int64{9}},
    {ID: 7, Status: "active", Schedulable: true},
    {ID: 99, Status: "active", Schedulable: false},
    {ID: 100, Status: "disabled", Schedulable: true},
}
```

Assert only `7,42` are included in sorted order. A sidecar evidence record for `99` must be ignored, not added. Add cases for duplicate account IDs, zero matching accounts, duplicate/future evidence, empty groups, expired accounts, and temporary scheduling blocks. Assert exactly one account-list call and no controller call.

- [ ] **Step 2: Run the focused test and verify RED.**

```bash
cd relay-ops-service && go test ./internal/d04readiness -run 'TestCollector' -count=1
```

Expected: package/type failure because the collector does not exist.

- [ ] **Step 3: Implement discovery, evidence joins, and atomic output.**

```go
type BalanceEvidence struct {
    AccountID   int64     `json:"account_id"`
    BalanceUSD  *float64  `json:"balance_usd"`
    RecordedAt  time.Time `json:"recorded_at"`
    EvidenceRef string    `json:"evidence_ref,omitempty"`
}

type QualityEvidence struct {
    AccountID         int64     `json:"account_id"`
    Source            string    `json:"source"`
    RecordedAt        time.Time `json:"recorded_at"`
    SampleCount       int64     `json:"sample_count"`
    SuccessRate       float64   `json:"success_rate"`
    ErrorRate         float64   `json:"error_rate"`
    TTFTP95MS         float64   `json:"ttft_p95_ms"`
    TotalLatencyP95MS float64   `json:"total_latency_p95_ms"`
}
```

Reject unknown keys, non-positive IDs, duplicate evidence IDs, future timestamps, unrecognized quality sources, and credential-shaped content. Missing evidence stays missing on a discovered account so the evaluator can fail closed. Write JSON to `path.tmp`, `fsync`, rename, and `fsync` the parent. The CLI prints only snapshot ID and canonical hash.

- [ ] **Step 4: Run collector and CLI tests and verify GREEN.**

```bash
cd relay-ops-service
go test ./internal/d04readiness -count=1
go run ./cmd/d04-readiness-snapshot -h
```

Expected: exit `0`, with no secret values in output.

### Task 3: Immutable v3 offline policy and per-account evaluator

**Files:**
- Create: `config/operations/D04-lightweight-launch-readiness-v3.yaml`
- Create: `ops/evaluate-d04-lightweight-launch-readiness-v3.rb`
- Create: `tests/operations/evaluate_d04_lightweight_launch_readiness_v3_test.rb`

**Interfaces:**
- CLI: `ruby ops/evaluate-d04-lightweight-launch-readiness-v3.rb evaluate POLICY SNAPSHOT [--now ISO8601]`.
- Result fields: `policy_id`, `snapshot_id`, `account_set_sha256`, `evaluated_at`, `decision`, `blocking_reasons`, `upstreams`, `real_action_executed:false`, and `external_system_contacted:false`.
- Each upstream result contains only `account_id`, `decision`, and sorted generic blockers.

- [ ] **Step 1: Write failing v3 evaluator tests.**

Build a valid two-account `GO` fixture. Then lower only account `42`'s balance and require its independent blocker and overall `NO-GO`:

```ruby
snapshot["active_upstreams"][1]["balance_usd"] = 0.0
result = evaluator.evaluate(snapshot)
row = result.fetch("upstreams").find { |item| item.fetch("account_id") == 42 }
assert_includes row.fetch("blocking_reasons"), "upstream_balance_below_minimum"
assert_equal "no_go", result.fetch("decision")
```

Cover empty discovery, stale discovery, hash mismatch, temporary runtime block, missing/stale balance, missing/wrong attribution, stale quality, insufficient samples, all quality thresholds, and account-set drift. Scan the v3 policy/evaluator for provider names and fixed production IDs. Keep the v2 tests unchanged.

- [ ] **Step 2: Run v3 tests and verify RED.**

```bash
ruby -Itest tests/operations/evaluate_d04_lightweight_launch_readiness_v3_test.rb
```

Expected: load failure because v3 files do not exist.

- [ ] **Step 3: Implement strict schema-v3 validation and evaluation.**

Require `schema_version: 3`, policy ID `D04-LIGHTWEIGHT-LAUNCH-v3`, a valid `upstream_discovery`, and a nonempty `active_upstreams` array. Reuse only generic validation ideas from v2; do not edit v2. Per-account reasons are:

```text
upstream_balance_unknown
upstream_balance_below_minimum
upstream_financial_evidence_stale
upstream_quality_attribution_missing
upstream_quality_source_invalid
upstream_quality_metrics_stale
upstream_samples_insufficient
upstream_success_rate_low
upstream_error_rate_high
upstream_ttft_p95_high
upstream_total_latency_p95_high
upstream_temporarily_unavailable
```

Keep common approval, mode, service, D04 config, account backup, ownership, and rollback checks. The evaluator reads local files only and never runs a shell command or contacts a network.

- [ ] **Step 4: Run v3 and historical v2 suites and verify GREEN.**

```bash
ruby -Itest tests/operations/evaluate_d04_lightweight_launch_readiness_v3_test.rb
ruby -Itest tests/operations/evaluate_d04_lightweight_launch_readiness_test.rb
```

Expected: both exit `0`.

### Task 4: Authenticated read-only /ops readiness projection

**Files:**
- Modify: `relay-ops-service/internal/config/config.go`
- Modify: `relay-ops-service/internal/config/config_test.go`
- Create: `relay-ops-service/internal/d04readiness/result.go`
- Create: `relay-ops-service/internal/d04readiness/result_test.go`
- Modify: `relay-ops-service/internal/http/sources.go`
- Modify: `relay-ops-service/internal/http/sources_test.go`
- Modify: `relay-ops-service/internal/http/templates/ops.html`
- Modify: `relay-ops-service/internal/http/static/ops-admin.js`
- Modify: `relay-ops-service/internal/http/server_test.go`
- Modify: `relay-ops-service/internal/app/app.go`

**Interfaces:**
- Optional `RELAY_OPS_D04_READINESS_RESULT_FILE`; missing means unavailable/`NO-GO`.
- `d04readiness.FileSource{Path string}.Read() (Result, error)` validates evaluator JSON and exposes neither paths nor evidence references.
- `OpsView.D04LaunchReadiness` contains decision, snapshot/hash/timestamps, stale state, generic blockers, and sanitized account rows.

- [ ] **Step 1: Write failing file-source and HTTP tests.**

Use a temporary two-account result. Assert the authenticated ops API includes IDs `7,42`, overall `NO-GO`, and the failing account reason. Anonymous bootstrap/API responses must contain neither account data nor hash. Require labels `D04 首发门禁`, `Sub2API 调度状态`, `账号`, `余额`, `自然质量`, and `NO-GO`. Assert this section has no form, button, POST endpoint, or mutation handler.

- [ ] **Step 2: Run focused tests and verify RED.**

```bash
cd relay-ops-service && go test ./internal/d04readiness ./internal/http ./internal/app -run 'D04|Ops' -count=1
```

Expected: failure because result source/view do not exist.

- [ ] **Step 3: Implement the projection without controls.**

Read and validate the result for each authenticated ops-view request. Missing, malformed, stale, or hash-invalid evidence renders `证据不可用` or `已过期` and remains `NO-GO`. Pass the view through `DatabaseOpsSource.Snapshot` and app composition. Render one unframed table with escaped Go-template text. JavaScript may render fetched data only; it must not send a readiness mutation.

- [ ] **Step 4: Run HTTP/app tests and syntax checks.**

```bash
cd relay-ops-service
go test ./internal/d04readiness ./internal/http ./internal/app -count=1
node --check internal/http/static/ops-admin.js
```

Expected: exit `0`.

### Task 5: Deployment contract and report-only production acceptance

**Files:**
- Modify: `infra/Dockerfile.relay-ops`
- Modify: `infra/compose.yaml`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`
- Create: `docs/superpowers/checklists/2026-07-22-d04-v3-active-upstream-read-only-acceptance.md`
- Create: `docs/superpowers/reports/2026-07-22-d04-v3-active-upstream-readiness-verification.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`

**Interfaces:**
- Relay-ops image includes `d04-readiness-snapshot`.
- Compose mounts only a secret-free evaluator result as read-only and points `RELAY_OPS_D04_READINESS_RESULT_FILE` to it.
- Server-local snapshot and result files live in a `0700` directory and are replaced atomically.

- [ ] **Step 1: Write failing deployment-contract checks.**

Require the snapshot binary, a `:ro` result mount, and absence of `D04_MODE=write`, open registration, probe mode, enabled Feishu mode, or a readiness POST route. Assert v1/v2 files remain present.

- [ ] **Step 2: Run the contract and verify RED.**

```bash
bash tests/relay_ops/validate_relay_ops_contract.sh
```

Expected: failure because v3 wiring is absent.

- [ ] **Step 3: Add minimal image/mount wiring and acceptance checklist.**

The checklist compares before/after service IDs, routing canonical hash, Sub2API-discovered IDs, snapshot hash, database business-table counts, production modes, candidates/probes, and Feishu deliveries. It explicitly forbids model requests, paid probes, scheduling toggles, launch overlay, balance updates, candidate creation, and synthetic events.

- [ ] **Step 4: Run complete local verification.**

```bash
cd relay-ops-service
go test ./... -p 1 -race -count=1
go vet ./...
cd ..
ruby -Itest tests/operations/evaluate_d04_lightweight_launch_readiness_v3_test.rb
ruby -Itest tests/operations/evaluate_d04_lightweight_launch_readiness_test.rb
node --check relay-ops-service/internal/http/static/ops.js
node --check relay-ops-service/internal/http/static/ops-admin.js
bash tests/relay_ops/validate_relay_ops_contract.sh
bash tests/internal_test/validate_internal_test_contract.sh
bash tests/infra/validate-baseline.sh
git diff --check
```

Expected: every command exits `0`.

- [ ] **Step 5: Perform production report-only acceptance.**

Use the existing server-local Admin credential for GET-only account discovery. Confirm the official pagination/DTO shape without printing secrets. Build a pinned AMD64 image and recreate only relay-ops if required for the new projection. Collect/evaluate v3 without model traffic, then prove `/ops` shows the same account-set hash and account decisions while D04 stays `read_only/closed` and relay-ops stays `read_only/dry_run`. Record `NO-GO` when evidence is missing or failing; do not weaken a threshold.

- [ ] **Step 6: Update durable project truth.**

Record the discovered-set definition, same-snapshot requirement, current decision, account-scoped generic blockers, test evidence, production modes, and zero-write proof in the report, `current-state.md`, and `llm-handoff.md`. The next mainline is D04 opening only after a fresh same-snapshot v3 `GO`; Feishu remains closed.

## Plan Self-review

- Spec coverage: Tasks 1-2 implement complete discovery and evidence joining; Task 3 enforces per-account fail-closed evaluation and drift; Task 4 provides the read-only `/ops` view; Task 5 covers deployment safety, production acceptance, and handoff.
- Scope: no routing, scheduling, candidate, probe, Feishu, upstream model, or D04 write behavior is added.
- Contract consistency: `account_id`, `active_upstreams`, and `account_set_sha256` are the collector-to-evaluator-to-UI identity contract.
- No evidence file can define membership or let one account mask another.
