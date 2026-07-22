# GPT/Codex Cache Savings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Verify and continuously enforce that GPT/Codex cache reads are charged at the lower public cache rate, while exposing cache-hit evidence in the existing read-only operations workflow.

**Architecture:** Sub2API `v0.1.161` remains the only request router, billing engine, user ledger, and user usage UI. The relay-ops service extends its read-only Sub2API contract with cache-token aggregates, evaluates public channel pricing with a focused policy package, and includes cache-hit and discount-coverage evidence in the existing daily report; it never reads prompt bodies or rewrites cache keys.

**Tech Stack:** Go 1.24, `net/http`, JSON, existing Sub2API Admin API, Go tests, Docker Compose.

## Global Constraints

- Initial scope is customer-visible OpenAI groups and public model IDs beginning with `gpt-`.
- Cache reads use `cache_read_price`; ordinary input, cache writes, and output retain their own configured prices.
- Group/user multipliers remain unchanged; upstream account cost multipliers remain operator margin.
- Sub2API remains authoritative for routing, sticky sessions, request forwarding, billing, balances, and user usage pages.
- Do not cache responses, prompts, files, tool definitions, or cross-user data in relay-ops.
- Do not generate, inspect, log, or rewrite `prompt_cache_key`.
- All new production behavior is read-only and must not mutate routes, accounts, prices, multipliers, balances, keys, users, or databases.
- Work with the existing dirty worktree and stage only files named by each task.

---

### Task 1: Decode Native Cache Usage Evidence

**Files:**
- Modify: `relay-ops-service/internal/sub2api/types.go`
- Modify: `relay-ops-service/internal/sub2api/client_test.go`

**Interfaces:**
- Produces: `UsageStats.TotalCacheTokens`, `TotalCacheCreationTokens`, `TotalCacheReadTokens`, `TotalTokens`, and `CacheMetricsPresent`.
- Consumes: Sub2API `GET /api/v1/admin/usage/stats` response fields from `v0.1.161`.

- [ ] **Step 1: Write a failing decoding test**

Extend `TestReaderDecodesOpsAndUsageWithoutUserDetails` with this payload and assertions:

```go
fmt.Fprint(w, `{"data":{"total_requests":100,"total_input_tokens":1000,"total_output_tokens":500,"total_cache_tokens":3500,"total_cache_creation_tokens":500,"total_cache_read_tokens":3000,"total_tokens":5000,"total_cost":1.5,"total_actual_cost":0.15,"total_account_cost":0.1,"average_duration_ms":1600}}`)

if !usage.CacheMetricsPresent || usage.TotalCacheCreationTokens != 500 ||
    usage.TotalCacheReadTokens != 3000 || usage.TotalTokens != 5000 {
    t.Fatalf("GetUsageStats cache metrics = %#v", usage)
}
```

Add `TestReaderMarksMissingCacheUsageFieldsUnconfirmed` using an otherwise valid stats response without the four cache fields, and require `CacheMetricsPresent == false`.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `cd relay-ops-service && go test ./internal/sub2api -run 'ReaderDecodesOpsAndUsage|ReaderMarksMissingCache' -count=1`

Expected: compilation fails because the cache aggregate fields do not exist.

- [ ] **Step 3: Implement presence-aware decoding**

Add the cache aggregate fields to `UsageStats`. Implement `UnmarshalJSON` with an alias plus pointer wire fields so zero-valued but present JSON remains confirmed. `CacheMetricsPresent` is true only when all four cache/total fields are present. A pre-`v0.1.161` response remains decodable but downstream policy reports its cache evidence as unconfirmed.

- [ ] **Step 4: Run focused and package tests**

Run: `cd relay-ops-service && go test ./internal/sub2api -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/sub2api/types.go relay-ops-service/internal/sub2api/client_test.go
git commit -m "feat: decode Sub2API cache usage evidence"
```

### Task 2: Enforce Cache Discount Pricing

**Files:**
- Create: `relay-ops-service/internal/cachepolicy/policy.go`
- Create: `relay-ops-service/internal/cachepolicy/policy_test.go`

**Interfaces:**
- Produces: `cachepolicy.Evaluate(groups []sub2api.Group, channels []sub2api.Channel) cachepolicy.Result`.
- Produces: `cachepolicy.Summarize(usage sub2api.UsageStats) cachepolicy.UsageSummary`.
- Consumes: existing Sub2API public group, active channel, model pricing, interval pricing, and Task 1 usage types.

- [ ] **Step 1: Write failing policy tests**

Use this passing fixture:

```go
input, read, write := 5e-6, 0.5e-6, 6.25e-6
result := Evaluate(
    []sub2api.Group{{ID: 2, Name: "GPT-Pro", Platform: "openai", Status: "active"}},
    []sub2api.Channel{{ID: 7, Status: "active", GroupIDs: []int64{2}, ModelPricing: []sub2api.ChannelModelPrice{{
        Models: []string{"gpt-5.6-sol"}, InputPrice: &input, CacheReadPrice: &read, CacheWritePrice: &write,
    }}}},
)
```

Require `Ready`, `EligibleModels == 1`, and `DiscountedModels == 1`. Also require blockers for missing cache-read price, `cache_read_price >= input_price`, missing GPT-5.6 cache-write price, incomplete interval prices, and duplicated contradictory prices. Private groups, inactive channels, and non-`gpt-` models do not enter the denominator.

Test `Summarize` with 1,000 ordinary input, 3,000 cache reads, and 500 cache writes; require a 75% hit rate over ordinary input plus cache reads. Require `Confirmed == false` when Task 1 marked fields absent, and no NaN/Inf when the denominator is zero.

- [ ] **Step 2: Run tests and verify RED**

Run: `cd relay-ops-service && go test ./internal/cachepolicy -count=1`

Expected: package does not exist.

- [ ] **Step 3: Implement the minimal policy package**

Use these public types:

```go
type Result struct {
    Ready            bool
    EligibleModels   int
    DiscountedModels int
    Blockers         []string
}

type UsageSummary struct {
    Confirmed           bool
    CacheReadTokens     int64
    CacheCreationTokens int64
    HitRatePercent      float64
}
```

Canonicalize price entries by group, model, and interval bounds. For every eligible entry require `input_price > 0`, `0 <= cache_read_price < input_price`, and an explicit non-negative cache-write price for `gpt-5.6-*`. Validate interval overrides independently. Sort and deduplicate blocker codes for stable reports.

- [ ] **Step 4: Run focused tests and vet**

Run: `cd relay-ops-service && go test ./internal/cachepolicy -count=1 && go vet ./internal/cachepolicy`

Expected: PASS with no vet findings.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/cachepolicy
git commit -m "feat: enforce GPT cache discount policy"
```

### Task 3: Add Cache Evidence to the Daily Operations Report

**Files:**
- Modify: `relay-ops-service/internal/dailyreport/service.go`
- Modify: `relay-ops-service/internal/dailyreport/service_test.go`

**Interfaces:**
- Consumes: `cachepolicy.Evaluate`, `cachepolicy.Summarize`, `Reader.ListChannels`, `Reader.ListGroups`, and `Reader.GetUsageStats`.
- Produces: one redacted 24-hour cache line per public OpenAI group and cache pricing blockers in the read-only analysis contract.

- [ ] **Step 1: Write failing report tests**

Extend the public-group fixture with cache fields and channel pricing. Require report text containing:

```text
缓存读取 3.00M，写入 500.00K，命中率 75.00%，缓存优惠 1/1 模型
```

Add a missing-field fixture requiring `缓存统计不可确认`, and a bad-price fixture requiring `缓存优惠未就绪` plus a stable blocker code. Assert the message still excludes private group names, secrets, prompt keys, and URLs.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `cd relay-ops-service && go test ./internal/dailyreport -count=1`

Expected: FAIL because reports do not include cache evidence.

- [ ] **Step 3: Integrate the policy and summary**

Read channels once before the group loop. Evaluate pricing once against all groups/channels. For each customer-visible OpenAI group, append cache read/write counts and hit rate when confirmed; otherwise append the unconfirmed label. Append global discount coverage or blockers without account IDs, raw API keys, cache keys, or prompt content.

Keep the service read-only. An unready cache policy is evidence in the report, not an automatic production mutation.

- [ ] **Step 4: Run daily report and neighboring tests**

Run: `cd relay-ops-service && go test ./internal/dailyreport ./internal/app ./internal/http -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/dailyreport/service.go relay-ops-service/internal/dailyreport/service_test.go
git commit -m "feat: report cache savings evidence"
```

### Task 4: Verify the Read-only Rollout Baseline

**Files:**
- Create: `docs/superpowers/reports/2026-07-22-gpt-codex-cache-savings-verification.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`

**Interfaces:**
- Consumes: Tasks 1-3 and pinned Sub2API `v0.1.161` source/API behavior.
- Produces: reproducible local evidence and explicit live follow-up gates; it performs no production write.

- [ ] **Step 1: Run focused and full verification**

```bash
cd relay-ops-service
go test ./internal/sub2api ./internal/cachepolicy ./internal/dailyreport -count=1
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
```

From the repository root:

```bash
git diff --check
docker compose -f infra/compose.yaml config --quiet
```

Expected: every command exits 0.

- [ ] **Step 2: Record authoritative upstream behavior**

Document the verified `v0.1.161` source facts: OpenAI usage splits ordinary input, cache creation, cache read, and output into mutually exclusive buckets; billing uses `cache_read_price` and `cache_write_price`; user usage records show both cache token classes and costs; `prompt_cache_key` participates in session hashing; native sticky routing remains authoritative.

- [ ] **Step 3: Record remaining live gates**

State that production activation still requires a 24-hour read-only window with at least 20 successful requests per public group, per-request user/Sub2API/upstream billing reconciliation, and same-model/same-channel success-rate and TTFT comparison. Do not claim those live gates passed from fixture tests.

- [ ] **Step 4: Commit documentation**

```bash
git add docs/superpowers/reports/2026-07-22-gpt-codex-cache-savings-verification.md docs/project/current-state.md docs/project/llm-handoff.md
git commit -m "docs: verify cache savings baseline"
```

## Plan Self-review

- Spec coverage: native routing and billing stay authoritative; cache usage presence, hit-rate evidence, explicit cache discount pricing, privacy boundaries, read-only rollout, and live acceptance gates each have an implementation task.
- Scope boundary: Sub2API's native user usage surface already exposes per-request cache tokens and cache costs and remains responsible for user billing detail; relay-ops adds no second user center and does not fork the gateway. The implementation reports hit-rate and discount-price evidence rather than inventing a savings amount that cannot be reconstructed exactly from the aggregate Admin API. A dedicated hypothetical-uncached savings label would require a separately versioned native Sub2API change.
- Placeholder scan: no `TBD`, `TODO`, deferred implementation instruction, or undefined type remains.
- Type consistency: Task 1 produces the cache fields consumed by Task 2; Task 2 produces `Evaluate` and `Summarize`, both consumed by Task 3; Task 4 verifies all three.
