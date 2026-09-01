# T96 Unified Quality Scheduling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement deterministic account-level quality routing and safe cross-account recovery for ordinary OpenAI text requests while preserving Sub's native account slots, protocol bindings, accounting, health isolation, and image scheduling.

**Architecture:** Add a read-only seven-day quality repository and a 60-second process snapshot provider, then add a unified text candidate path inside the existing OpenAI scheduler. The selector performs existing eligibility and T82 health filtering, partitions candidates with the native profit gate, reads T95 effective cost live, and sorts by success rate, trimmed TTFT, U, and ID. The existing handlers retain ownership of native slot acquisition and forwarding, but use a group-level cross-account budget and never replay an ordinary text request on the same account.

**Tech Stack:** Go 1.27, PostgreSQL, existing Sub2API service/repository interfaces, Gin handlers, Vue 3, TypeScript, Vitest, sqlmock.

**Spec:** `docs/superpowers/specs/2026-08-31-t96-group-account-baseline-unified-quality-scheduling-design.md`

## Global Constraints

- T95 must be committed, reviewed, and merged into the latest clean `main` before the T96 worktree is created.
- Create T96 from that latest clean `main` in branch `codex/t96-unified-quality-scheduling` and worktree `.worktrees/t96-unified-quality-scheduling`.
- Do not read or copy implementation files from the currently dirty T95 worktree. Consume only the merged T95 contract.
- Do not restore T80 admission, slow-session guards, pre-output leases, or any custom account concurrency/queue fields.
- Reuse `accounts.concurrency`, native account waiting, waiting timeout, cancellation, and release behavior.
- Do not add a SQL migration, table, column, quality fact source, profit field, or automatic account-group mutation.
- Ordinary text order is exactly `success_rate DESC NULLS LAST`, `ttft_trimmed_mean_ms ASC NULLS LAST`, live `U ASC NULLS LAST`, `account_id ASC`.
- Total duration, priority, weights, Top-K, randomization, exploration, fairness, starvation, ordinary sticky, and queue/load scores must not change unified text order.
- Protocol-mandated bindings remain ahead of ordinary quality order when still safe and eligible.
- Image requests remain on the existing native image selector and native image retry behavior.
- A failed attempt may switch accounts only when no semantic output, no billable usage, no side effect, and `safe_to_replay=true` are all proven.
- `extra_retry_count` is `0..3`, defaults to `0`, and counts only different-account attempts that actually enter `Forward` after the first attempt.
- Preserve all existing recognized policy, preset, weight, fairness, quality-gate, and session-escape fields for runtime rollback; unified text mode ignores them.
- Do not modify production account pools, priorities, status, schedulable flags, concurrency, or profit settings in code or deployment scripts.
- Do not use GitHub Actions.

---

### Task 1: Establish the merged dependency and isolated implementation baseline

**Files:**
- Read: `docs/project/native-sub-incremental-delivery-constraints.md`
- Read: `docs/project/native-sub-task-package-queue.md`
- Read: `docs/project/project-progress.md`
- Read: `docs/project/acceptance-station-global-constraints.md` before any acceptance-station action
- Read: `docs/superpowers/specs/2026-08-31-t96-group-account-baseline-unified-quality-scheduling-design.md`
- Read: merged `upstream/sub2api/backend/internal/service/account_effective_cost.go`
- Create after Steps 1-2 pass: `.worktrees/t96-unified-quality-scheduling`

**Interfaces:**
- Consumes: merged T95 `EffectiveCostForAccount(account *Account) EffectiveCost` with `Status`, `U`, and `EffectiveCostStatusReady` semantics.
- Produces: isolated T96 branch/worktree whose base contains T95 and no uncommitted files.

- [ ] **Step 1: Confirm T95 is in `main` and the root checkout has no unrelated integration in progress**

Run:

```bash
git status --short
git worktree list --porcelain
git log --oneline --all -- upstream/sub2api/backend/internal/service/account_effective_cost.go | head
git show main:upstream/sub2api/backend/internal/service/account_effective_cost.go | sed -n '1,180p'
```

Expected: the T95 provider exists in `main`; no other task is in `INTEGRATING`, `DEPLOYING`, or `VERIFYING`; root changes are either clean or explicitly owned by the release controller. If the provider is absent, stop without creating T96.

- [ ] **Step 2: Audit all registered non-main worktrees before creation**

Run:

```bash
git worktree list --porcelain
for d in .worktrees/*; do test -e "$d/.git" || continue; git -C "$d" status --short; git -C "$d" log -1 --oneline; done
```

Expected: any completed worktree that leads `main` is handled by the release controller first; protected or dirty worktrees are preserved.

- [ ] **Step 3: Create the isolated worktree from the verified `main`**

Run:

```bash
git worktree add .worktrees/t96-unified-quality-scheduling -b codex/t96-unified-quality-scheduling main
git -C .worktrees/t96-unified-quality-scheduling status --short
git -C .worktrees/t96-unified-quality-scheduling rev-parse HEAD
```

Expected: empty status and a base SHA equal to the verified `main` SHA.

- [ ] **Step 4: Record the exact merged T95 contract in the T96 handoff draft**

Create `docs/handoffs/2026-08-31-t96-unified-quality-scheduling-handoff.md` in the T96 worktree with the base SHA and the actual merged names of the effective-cost function, result type, ready status, and U field. Do not change T95 APIs in T96.

- [ ] **Step 5: Commit the task-local baseline artifacts**

```bash
git add docs/superpowers/specs/2026-08-31-t96-group-account-baseline-unified-quality-scheduling-design.md \
  docs/superpowers/plans/2026-08-31-t96-unified-quality-scheduling.md \
  docs/superpowers/reports/2026-08-31-t96-account-quality-ranking.md \
  docs/handoffs/2026-08-31-t96-unified-quality-scheduling-handoff.md
git commit -m "docs: start T96 unified quality scheduling"
```

### Task 2: Add the group recovery setting without losing recognized legacy policy fields

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/settings_view.go`
- Modify: `upstream/sub2api/backend/internal/service/setting_parse.go`
- Modify: `upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/setting_handler_auth_source_defaults_test.go`
- Modify: `upstream/sub2api/frontend/src/api/admin/settings.ts`

**Interfaces:**
- Consumes: existing `OpenAISchedulerGroupPolicy` JSON object and settings GET/PUT flow.
- Produces: `OpenAISchedulerGroupPolicy.ExtraRetryCount *int` serialized as `extra_retry_count`; runtime resolver returns `0` for a missing field and rejects values outside `0..3`.

- [ ] **Step 1: Write failing backend parse/round-trip tests**

Add cases that assert:

```go
raw := `{"11":{"extra_retry_count":3,"mode":"custom","priority":{"profit":1,"ttft":2,"latency":3}}}`
policies, err := parseOpenAISchedulerGroupPolicies(raw)
require.NoError(t, err)
require.NotNil(t, policies[11].ExtraRetryCount)
require.Equal(t, 3, *policies[11].ExtraRetryCount)

for _, invalid := range []string{
    `{"11":{"extra_retry_count":-1}}`,
    `{"11":{"extra_retry_count":4}}`,
    `{"11":{"extra_retry_count":1.5}}`,
} {
    _, err := parseOpenAISchedulerGroupPolicies(invalid)
    require.Error(t, err)
}
```

Also assert that a policy containing existing `priority`, `operations`, `compiled_snapshot`, `weight_overrides`, and `fairness` survives GET/PUT round-trip unchanged when only `extra_retry_count` changes.

- [ ] **Step 2: Run the focused tests and confirm RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/handler/admin -run 'Test.*OpenAIScheduler.*(GroupPolic|ExtraRetry|RoundTrip)' -count=1
```

Expected: failure because `ExtraRetryCount` is not defined or validated.

- [ ] **Step 3: Add the field and one validation function**

Implement:

```go
type OpenAISchedulerGroupPolicy struct {
    // existing fields remain unchanged
    ExtraRetryCount *int `json:"extra_retry_count,omitempty"`
}

func normalizeOpenAIExtraRetryCount(value *int) (*int, error) {
    if value == nil {
        return nil, nil
    }
    if *value < 0 || *value > 3 {
        return nil, infraerrors.BadRequest(
            "INVALID_OPENAI_SCHEDULER_GROUP_POLICY",
            "extra_retry_count must be between 0 and 3",
        )
    }
    normalized := *value
    return &normalized, nil
}

func resolveOpenAIExtraRetryCount(policy OpenAISchedulerGroupPolicy) int {
    if policy.ExtraRetryCount == nil {
        return 0
    }
    return *policy.ExtraRetryCount
}
```

Call the validator from both current normalized-policy paths so legacy and current policy modes behave identically.

- [ ] **Step 4: Add the TypeScript contract**

Add `extraRetryCount: { min: 0, max: 3, step: 1 }` to `OPENAI_SCHEDULER_LIMITS` and `extra_retry_count?: number` to `OpenAISchedulerGroupPolicy`.

- [ ] **Step 5: Run focused backend tests and TypeScript typecheck**

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/handler/admin -run 'Test.*OpenAIScheduler.*(GroupPolic|ExtraRetry|RoundTrip)' -count=1
cd ../frontend
pnpm typecheck
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add upstream/sub2api/backend/internal/service/settings_view.go \
  upstream/sub2api/backend/internal/service/setting_parse.go \
  upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go \
  upstream/sub2api/backend/internal/handler/admin/setting_handler_auth_source_defaults_test.go \
  upstream/sub2api/frontend/src/api/admin/settings.ts
git commit -m "feat: add group cross-account recovery setting"
```

### Task 3: Build the seven-day account quality snapshot provider

**Files:**
- Create: `upstream/sub2api/backend/internal/service/openai_account_quality.go`
- Create: `upstream/sub2api/backend/internal/service/openai_account_quality_test.go`
- Create: `upstream/sub2api/backend/internal/repository/usage_log_quality.go`
- Create: `upstream/sub2api/backend/internal/repository/usage_log_quality_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_service.go`

**Interfaces:**
- Consumes: `usage_logs.logical_request_id`, `attempt_id`, `usage_completeness`, `actual_cost`, `first_token_ms`, `duration_ms`, `account_id`, and `created_at`.
- Produces:

```go
type OpenAIAccountQuality struct {
    AccountID            int64
    AttemptCount         int64
    SuccessCount         int64
    SuccessRate          *float64
    TTFTSampleCount      int64
    TTFTTrimmedMeanMS    *float64
    LatencySampleCount   int64
    LatencyTrimmedMeanMS *float64
}

type OpenAIAccountQualityRepository interface {
    ListOpenAIAccountQuality(ctx context.Context, start, end time.Time) ([]OpenAIAccountQuality, error)
}

type OpenAIAccountQualitySnapshot struct {
    WindowStart time.Time
    WindowEnd   time.Time
    SnapshotAt  time.Time
    Stale       bool
    Accounts    map[int64]OpenAIAccountQuality
}

type OpenAIAccountQualitySnapshotProvider interface {
    Snapshot(ctx context.Context) OpenAIAccountQualitySnapshot
}
```

- [ ] **Step 1: Write sqlmock tests for physical-attempt aggregation**

Cover these rows and expected results:

```text
same logical_request_id + different attempt_id -> two attempts
same attempt-derived request_id duplicated for one api_key_id -> one attempt
complete + actual_cost > 0 -> success
partial / unsafe / upstream failed durable usage -> denominator only
usage_completeness=unknown -> excluded
image/video billing modes -> excluded
first_token_ms NULL -> success counted, TTFT sample not counted
five samples -> floor(5*5%)=0, retain all
twenty samples -> trim one fastest and one slowest
```

The SQL test must assert use of `usage_logs` only and must not expect a join to `ops_error_logs`.

- [ ] **Step 2: Run the repository test and confirm RED**

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestUsageLogRepository_ListOpenAIAccountQuality' -count=1
```

Expected: failure because the method does not exist.

- [ ] **Step 3: Implement the aggregate query**

Use CTEs with this invariant structure:

```sql
WITH physical_attempts AS (
  SELECT DISTINCT ON (api_key_id, request_id)
    account_id, request_id, attempt_id, usage_completeness,
    actual_cost, first_token_ms, duration_ms, created_at
  FROM usage_logs
  WHERE created_at >= $1 AND created_at < $2
    AND account_id IS NOT NULL
    AND usage_completeness <> 'unknown'
    AND COALESCE(billing_mode, 'token') NOT IN ('image', 'video')
  ORDER BY api_key_id, request_id, id DESC
), successful AS (
  SELECT *,
    row_number() OVER (PARTITION BY account_id ORDER BY first_token_ms) AS ttft_rn,
    count(first_token_ms) OVER (PARTITION BY account_id) AS ttft_n,
    row_number() OVER (PARTITION BY account_id ORDER BY duration_ms) AS latency_rn,
    count(duration_ms) OVER (PARTITION BY account_id) AS latency_n
  FROM physical_attempts
  WHERE usage_completeness = 'complete' AND actual_cost > 0
)
```

Aggregate success rate from `physical_attempts`. Compute each trimmed mean only from non-null successful values where row number is greater than `floor(n*0.05)` and at most `n-floor(n*0.05)`. Preserve `NULL`, never coerce it to zero.

- [ ] **Step 4: Write provider cache tests and confirm RED**

Use a fake clock and repository to prove:

```go
first := provider.Snapshot(ctx) // repository called once
second := provider.Snapshot(ctx) // same snapshot inside 60 seconds
clock.Advance(61 * time.Second)
third := provider.Snapshot(ctx) // refreshes
repo.Fail(errors.New("db unavailable"))
stale := provider.Snapshot(ctx) // last success, Stale=true
```

Also prove cold-start failure returns an empty snapshot and does not return an error that can block routing.

- [ ] **Step 5: Implement the provider and wire it into `OpenAIGatewayService`**

The provider owns a mutex-protected last-success snapshot, a 60-second expiry, and a `singleflight.Group`. It stores no U. `NewOpenAIGatewayService` type-asserts the existing repository:

```go
qualityRepo, _ := usageLogRepo.(OpenAIAccountQualityRepository)
svc.openaiQuality = NewOpenAIAccountQualitySnapshotProvider(qualityRepo, 60*time.Second, time.Now)
```

When `qualityRepo` is absent in a unit-test stub, the provider returns the empty cold-start snapshot.

- [ ] **Step 6: Run focused tests**

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestUsageLogRepository_ListOpenAIAccountQuality' -count=1
go test ./internal/service -run 'TestOpenAIAccountQualitySnapshotProvider' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add upstream/sub2api/backend/internal/service/openai_account_quality.go \
  upstream/sub2api/backend/internal/service/openai_account_quality_test.go \
  upstream/sub2api/backend/internal/service/openai_gateway_service.go \
  upstream/sub2api/backend/internal/repository/usage_log_quality.go \
  upstream/sub2api/backend/internal/repository/usage_log_quality_test.go
git commit -m "feat: add OpenAI account quality snapshots"
```

### Task 4: Replace ordinary text ranking with the unified deterministic selector

**Files:**
- Create: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go`
- Create: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_images_test.go`

**Interfaces:**
- Consumes: fixed group candidates, existing capability/transport/privacy/T82 health checks, quality snapshot, merged T95 `EffectiveCostForAccount`.
- Produces: `selectByUnifiedQuality(ctx context.Context, req OpenAIAccountScheduleRequest) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error)`; the existing decision type carries the stable ordered candidate list and unified decision fields.

- [ ] **Step 1: Write comparator property tests**

Define test candidates with nullable values and assert:

```go
want := []int64{10, 20, 30, 40}
// 10 beats 20 by success rate even when slower and more expensive.
// 20 beats 30 by TTFT when success rate matches.
// 30 beats 40 by U when the first two fields match.
// ID breaks a complete tie.
```

Shuffle input 100 times and require the same full ID sequence. Add explicit tests that success/TTFT/U nulls are last at their own comparison position and are not zero.

- [ ] **Step 2: Write routing tests for forbidden ordering inputs and image bypass**

Prove that changing account priority, relation priority, load, queue depth, Top-K, weight overrides, fairness, exploration, ordinary sticky account, or total latency leaves the unified text sequence unchanged. Prove `requiredImageCapability != ""` never calls the quality provider and retains current native image selection/fallback.

- [ ] **Step 3: Run focused tests and confirm RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestOpenAIUnifiedQuality|Test.*Image.*UnifiedQuality' -count=1
```

Expected: failure because the unified selector does not exist.

- [ ] **Step 4: Implement the pure candidate comparator**

Use this key object:

```go
type openAIUnifiedQualityCandidate struct {
    account     *Account
    successRate *float64
    ttftMS      *float64
    effectiveU  *float64
}
```

Implement explicit nullable comparators; do not negate values or substitute sentinels. Always finish with `account.ID` ascending.

- [ ] **Step 5: Implement the unified text selection entry**

In `Select`:

```text
1. forced protocol binding / previous_response_id / guardian parent
2. image capability -> existing path unchanged
3. advanced scheduler enabled ordinary text -> selectByUnifiedQuality
4. advanced scheduler disabled -> existing base path unchanged
```

Ordinary session sticky must not run before `selectByUnifiedQuality`. The selector reuses existing list and eligibility helpers, applies `ExcludedIDs`, T82 blocked/open/half-open semantics, model/capability/transport checks, and native slot acquisition order. It reads live U by calling the merged T95 provider for each eligible account. Missing quality or U remains nullable.

- [ ] **Step 6: Keep protocol bindings narrow**

Tests must show `previous_response_id`, guardian parent, and other existing non-movable protocol bindings remain bound when eligible. A normal session hash must not change quality order. The obsolete same-account `ForcedAccountID` path must not be reachable from ordinary T96 text recovery.

- [ ] **Step 7: Run focused scheduler and image tests**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestOpenAIUnifiedQuality|TestOpenAIAccountScheduler|TestOpenAIImages' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go \
  upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go \
  upstream/sub2api/backend/internal/service/openai_account_scheduler.go \
  upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go \
  upstream/sub2api/backend/internal/service/openai_images_test.go
git commit -m "feat: route text requests by unified account quality"
```

### Task 5: Extend the native profit gate with availability-first partitioning

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_profit_control.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_profit_control_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_profit_control_pricing_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_profit_slot_recheck_test.go`

**Interfaces:**
- Consumes: native group `profit_control_enabled`, `profit_min_margin`, `profit_safety_buffer`, frozen request D, and live T95 U.
- Produces: profit-qualified first partition, full-pool fallback with `profit_bypass`, and post-slot decision refresh.

- [ ] **Step 1: Write failing two-stage partition tests**

Cover:

```text
profit disabled -> all eligible candidates
one or more qualified -> only qualified candidates
none qualified -> full eligible pool, profit_bypass=margin_below
all U unknown -> full eligible pool, reason=unknown_u
mixed invalid/unknown/over-threshold -> deterministic reason counts
group read failure -> full pool, reason=config_read_failed
```

Assert the threshold formula uses `U <= D*(1-minimum_margin-safety_buffer)` and that U comes from T95, never `accounts.rate_multiplier` directly.

- [ ] **Step 2: Run focused tests and confirm RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestOpenAIProfit.*(Partition|Bypass|EffectiveCost|Latest)' -count=1
```

- [ ] **Step 3: Add a read-only partition result**

Implement:

```go
type openAIProfitPartition struct {
    candidates   []openAIUnifiedQualityCandidate
    bypass       bool
    bypassReason string
}
```

Reuse the native gate's request-frozen D and margin fields. Do not add settings. Unknown/invalid U cannot enter the preferred subpool when a known qualified candidate exists.

- [ ] **Step 4: Make post-slot validation re-evaluate order, not just veto**

After a slot is acquired, refresh the account and live U. If the current account is no longer in the preferred partition or a U change changes the ordered winner, release the slot and return a retry-next result without writing a response. The next scheduler call must use a fresh unified decision; it must not repeatedly select the same stale candidate.

- [ ] **Step 5: Remove profit-only hard exhaustion for unified text mode**

The existing `maxProfitVetoAttempts` remains for legacy/image/disabled paths. Unified text uses its finite candidate exclusion set; crossing the threshold cannot produce a profit-specific 503 because the full eligible pool is the fallback.

- [ ] **Step 6: Run focused tests**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestOpenAIProfit|TestOpenAIUnifiedQuality' -count=1
go test ./internal/handler -run 'Test.*Profit.*Slot' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add upstream/sub2api/backend/internal/service/openai_profit_control.go \
  upstream/sub2api/backend/internal/service/openai_profit_control_test.go \
  upstream/sub2api/backend/internal/service/openai_profit_control_pricing_test.go \
  upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go \
  upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go \
  upstream/sub2api/backend/internal/handler/openai_profit_slot_recheck_test.go
git commit -m "feat: keep OpenAI profit routing availability first"
```

### Task 6: Implement group-level cross-account recovery and native-slot skipping

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/openai_retry_budget.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_retry_budget_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler_failover_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_chat_completions.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_embeddings.go`

**Interfaces:**
- Consumes: group `extra_retry_count`, existing failure classifier, `unsafe_to_replay`, output/usage/side-effect facts, native account slot result.
- Produces: maximum `1+extra_retry_count` different-account Forward attempts and no ordinary same-account replay.

- [ ] **Step 1: Write budget state-machine tests**

Use table tests for `extra_retry_count=0,1,2,3` and assert maximum Forward attempts `1,2,3,4`. Add methods with exact semantics:

```go
func (b *openAIRetryBudget) CanStartForward(accountID int64) bool
func (b *openAIRetryBudget) RecordForwardStarted(accountID int64) bool
func (b *openAIRetryBudget) RecordObservedDomains(domains []service.OpenAIFailureDomain)
```

`RecordObservedDomains` never reduces the attempt budget. Candidate selection, capability skip, profit refresh, slot queue full, slot wait timeout, and cancellation before Forward leave attempt/switch counters unchanged.

- [ ] **Step 2: Write handler tests for no same-account replay and unsafe terminal behavior**

Cover Responses streaming/non-streaming, Chat Completions, Messages compatibility, and Embeddings where applicable:

```text
safe failure on A -> next Forward uses B
pool_mode_retry_count=3 on A -> A is still attempted once
partial output -> no B
usage produced / upstream charged -> no B and usage is billed
side effect -> no B
unknown billing -> no B
client cancellation before B Forward -> no extra count
```

- [ ] **Step 3: Write native-slot skip tests**

Add `openAISlotAcquireRetryNext` to the handler-only state machine. In unified text mode, quick-acquire error, queue full, and wait timeout release/decrement native resources, write no response, exclude that account for this logical request, and continue to the next ranked account without consuming the recovery count. In legacy-disabled and image modes, retain the current immediate native error response.

- [ ] **Step 4: Run focused tests and confirm RED**

```bash
cd upstream/sub2api/backend
go test ./internal/handler -run 'TestOpenAIRetryBudget|Test.*Unified.*(Failover|Slot|Replay)' -count=1
```

- [ ] **Step 5: Implement a T96 budget constructor**

Implement a constructor that derives `MaxAttempts=1+extra_retry_count` and `MaxAccountSwitches=extra_retry_count`, has no independent five-second wall clock, and treats failure domains as health/diagnostic evidence only. Client/request context deadlines still stop work. Keep the old constructor for legacy and non-T96 paths.

- [ ] **Step 6: Move switch accounting to the Forward boundary**

Call `RecordForwardStarted(account.ID)` after native slot success and post-slot profit/order validation, immediately before `gatewayService.Forward`. Remove pre-increment at the safe-failure decision point. `attemptSequence.next` may retain sequence gaps, but `extra_used` and `switch_count` must match actual Forward calls.

- [ ] **Step 7: Disable ordinary same-account retry only in unified text mode**

Bypass both `retryDecision.RetrySameAccount` and `poolRetryAllowed` for T96 ordinary text requests. Preserve OAuth token refresh and protocol correctness actions that do not create a second billable ordinary Forward. Preserve current behavior for image and advanced-scheduler-disabled requests.

- [ ] **Step 8: Run handler regression tests**

```bash
cd upstream/sub2api/backend
go test ./internal/handler -run 'TestOpenAIRetryBudget|Test.*(Failover|Retry|Slot|Profit|Responses|ChatCompletions|Embeddings)' -count=1
```

Expected: PASS, including existing unsafe replay and accounting tests.

- [ ] **Step 9: Commit**

```bash
git add upstream/sub2api/backend/internal/handler/openai_retry_budget.go \
  upstream/sub2api/backend/internal/handler/openai_retry_budget_test.go \
  upstream/sub2api/backend/internal/handler/openai_gateway_handler.go \
  upstream/sub2api/backend/internal/handler/openai_gateway_handler_failover_test.go \
  upstream/sub2api/backend/internal/handler/openai_chat_completions.go \
  upstream/sub2api/backend/internal/handler/openai_embeddings.go
git commit -m "feat: recover OpenAI text requests across ranked accounts"
```

### Task 7: Add explainable scheduling events and simplify the admin scheduler UI

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_openai_scheduler_experience.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_openai_scheduler_experience_test.go`
- Modify: `upstream/sub2api/frontend/src/views/admin/SchedulerSettingsView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/SchedulerSettingsView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`

**Interfaces:**
- Consumes: unified selection decision and group policy.
- Produces: non-sensitive structured decision fields and an admin control limited to extra recovery count in unified mode.

- [ ] **Step 1: Write scheduling-event tests**

Assert events contain:

```text
logical_request_id, attempt_id, attempt_index
group_id, requested_model, image_intent
ordered candidate IDs, excluded IDs/reasons, selected ID/rank
quality_window_end, quality_snapshot_stale
profit mode/bypass/reason
extra_retry_count, extra_used, switch_count
safe_to_replay, switch_allowed/block reason, stop reason
native_slot_wait_ms, routing_ms, upstream_ttft_ms, total_ms
```

Assert no credential, API key, OAuth token, request body, or upstream content field is emitted.

- [ ] **Step 2: Write UI tests and confirm RED**

Replace current priority/operations expectations with:

```text
selected group displays one 0..3 extra-recovery control
missing value displays 0
changing 0 to 3 saves extra_retry_count=3
legacy priority/operations/weights remain in payload unchanged
no visible controls imply that weights, Top-K, fairness, or ordinary sticky affect unified routing
disabled global switch remains available for rollback
```

- [ ] **Step 3: Implement event projection and counters**

Extend the existing decision/event type rather than creating a parallel logger. Record timing segments separately: pool load, eligibility, profit partition, quality sort, native slot wait, and failover interval. Reuse existing metrics infrastructure; do not add a new database fact table.

- [ ] **Step 4: Replace the scheduler page's editable policy surface**

Keep the group selector and global toggle. For each text group, render a compact numeric stepper/select for `extra_retry_count` with valid values 0, 1, 2, 3. Do not duplicate native Groups profit settings. Preserve old policy objects through object spread on save.

- [ ] **Step 5: Add Chinese and English labels**

Use explicit labels for “额外恢复次数 / Extra recovery attempts” and state that it counts attempts after the first request attempt. Do not add instructional feature marketing copy to the page.

- [ ] **Step 6: Run focused tests, typecheck, and build**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'Test.*Scheduler.*(Decision|Experience|Unified)' -count=1
cd ../frontend
pnpm test:run -- src/views/admin/__tests__/SchedulerSettingsView.spec.ts
pnpm typecheck
pnpm build
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add upstream/sub2api/backend/internal/service/openai_account_scheduler.go \
  upstream/sub2api/backend/internal/service/ops_openai_scheduler_experience.go \
  upstream/sub2api/backend/internal/service/ops_openai_scheduler_experience_test.go \
  upstream/sub2api/frontend/src/views/admin/SchedulerSettingsView.vue \
  upstream/sub2api/frontend/src/views/admin/__tests__/SchedulerSettingsView.spec.ts \
  upstream/sub2api/frontend/src/api/admin/settings.ts \
  upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts \
  upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts
git commit -m "feat: expose unified scheduler recovery policy"
```

### Task 8: Verify the complete contract and prepare acceptance handoff

**Files:**
- Modify: `docs/handoffs/2026-08-31-t96-unified-quality-scheduling-handoff.md`
- Create: `docs/superpowers/reports/2026-08-31-t96-unified-quality-scheduling-verification.md`
- Read only: `docs/superpowers/reports/2026-08-31-t96-account-quality-ranking.md`

**Interfaces:**
- Consumes: all implementation tasks.
- Produces: directly related test evidence, account-pool read-only validator output, rollback evidence, and `READY_FOR_ROOT_REVIEW` handoff.

- [ ] **Step 1: Run formatting and focused backend tests**

```bash
cd upstream/sub2api/backend
gofmt -w internal/service/openai_account_quality.go \
  internal/service/openai_account_quality_test.go \
  internal/service/openai_unified_quality_scheduler.go \
  internal/service/openai_unified_quality_scheduler_test.go \
  internal/repository/usage_log_quality.go \
  internal/repository/usage_log_quality_test.go \
  internal/handler/openai_retry_budget.go \
  internal/handler/openai_retry_budget_test.go
go test ./internal/repository -run 'TestUsageLogRepository_ListOpenAIAccountQuality' -count=1
go test ./internal/service -run 'TestOpenAI(AccountQuality|UnifiedQuality|Profit|AccountScheduler|Images)|Test.*Scheduler.*Experience' -count=1
go test ./internal/handler -run 'TestOpenAI|Test.*(Responses|ChatCompletions|Embeddings|Profit.*Slot)' -count=1
go build ./cmd/server
```

Expected: all PASS.

- [ ] **Step 2: Run focused frontend tests and build**

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/views/admin/__tests__/SchedulerSettingsView.spec.ts src/views/admin/scheduler/__tests__/schedulerPolicy.spec.ts
pnpm typecheck
pnpm build
```

Expected: all PASS.

- [ ] **Step 3: Run source-contract guards**

```bash
! rg -n 'AcquireOpenAIAdmission|slow.session|slow_session|admission_wait_ms' upstream/sub2api/backend/internal/handler/openai_* upstream/sub2api/backend/internal/service/openai_*
! rg -n 'ai8|/10' upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go upstream/sub2api/backend/internal/service/openai_profit_control.go
git diff --check
```

Expected: all commands exit 0. Historical tests/docs outside the request chain may still mention removed T80 behavior; production request code must not.

- [ ] **Step 4: Validate the configured account-set contract read-only on the acceptance station**

After reading `docs/project/acceptance-station-global-constraints.md`, use the protected acceptance credentials and existing admin/read-only query path to prove:

```text
Pro = 专属 Pro = 19 IDs from the approved spec
Plus = 20 approved IDs
特惠 = 27 approved IDs
text ownership union = 66 unique IDs
image pool = 7 approved IDs
text/image intersection = empty
deleted 286 absent
all account and account-group priorities = 50
```

The application and deployment scripts must not write these relationships. An administrator performs any configuration through native pages.

- [ ] **Step 5: Run acceptance-station behavioral checks after authorized deployment**

Use controlled non-billable or explicitly budgeted fixtures to prove:

```text
same eligible set -> same complete order
safe unbilled failure -> next ranked account
Pro/Plus/特惠 max Forward attempts -> 2/3/4
slot queue full/timeout -> next candidate without consuming extra count
partial/billed/unknown/side-effect attempt -> no replay
profit-qualified subset preferred
all over threshold -> service continues with profit_bypass
image request -> native image path, no quality snapshot lookup
account monitor -> every physical attempt visible
group monitor -> one final logical result
```

Do not manufacture a billable production failure. Main-site deployment still requires one of the two exact authorizations in the acceptance-station global constraints.

- [ ] **Step 6: Verify rollback before requesting root review**

Set `openai_advanced_scheduler_enabled=false` on the acceptance station and prove existing base routing still serves requests and ignores `extra_retry_count`. Re-enable only after the smoke test. Record both setting values and request IDs without credentials.

- [ ] **Step 7: Write the verification report and handoff**

Record base SHA, candidate SHA, changed files, exact commands/results, migration/config status, `downtime_required` expectation, rollback switch, unverified production items, and remaining risks. Status is `READY_FOR_ROOT_REVIEW`, not DONE.

- [ ] **Step 8: Final commit**

```bash
git add docs/handoffs/2026-08-31-t96-unified-quality-scheduling-handoff.md \
  docs/superpowers/reports/2026-08-31-t96-unified-quality-scheduling-verification.md
git commit -m "docs: record T96 verification handoff"
```

## Acceptance Criteria

- [ ] The selected group is resolved before candidate loading, and runtime never writes group membership.
- [ ] Every ordinary text candidate sequence obeys the four-key deterministic order.
- [ ] Live T95 U is not stored in or read from the 60-second quality snapshot.
- [ ] Missing/small-sample quality remains eligible with independent NULLS LAST behavior.
- [ ] Protocol binding remains correct; ordinary sticky cannot reorder candidates.
- [ ] Native profit settings are the only margin controls, with availability-first fallback.
- [ ] Pro/专属 Pro, Plus, and 特惠 permit exactly 1, 2, and 3 extra different-account Forward attempts.
- [ ] Ordinary text never performs a same-account upstream retry.
- [ ] Billable, partial, side-effecting, output-started, or unknown attempts are never replayed.
- [ ] Native slot queue failure skips a candidate only in unified text mode and never consumes extra recovery count.
- [ ] T82 health/cooldown/half-open remains active; failure-domain count does not truncate the group budget.
- [ ] Image scheduling and retry behavior remain native and bypass text quality/profit fallback.
- [ ] Each physical attempt remains in account monitoring; logical outcome remains one group-monitor result.
- [ ] No SQL migration, production data rewrite, automatic group mutation, or custom admission control exists.
- [ ] Closing the global scheduler switch restores the prior base path.

## Risks And Review Focus

- The highest-risk boundary is deciding safe replay after network ambiguity; default to terminal when billing is unknown.
- The second risk is writing an error during a native slot failure before trying the next candidate; unified retry-next paths must leave the response untouched.
- The third risk is stale U after slot acquisition; release the slot and rebuild the decision when live U changes ordering or partition membership.
- Deterministic ranking concentrates traffic. Native concurrency and T82 health are the only capacity/health controls; do not add another admission mechanism.
- T95 may change its final symbol names before merge. If its merged public contract differs from Task 1, revise this plan and its interface blocks before implementation rather than adapting silently in code.
- Existing WebSocket and protocol-bound flows require focused regression because a movable ordinary sticky session and a non-movable protocol binding are deliberately treated differently.

## Release And Rollback

- Expected migration: none.
- Expected production data write during deployment: none.
- Expected `downtime_required`: `false`, subject to the release preflight result.
- Runtime rollback: set `openai_advanced_scheduler_enabled=false`.
- Code rollback: promote the previous verified image/commit through the existing local/host blue-green chain.
- Account relationships and legacy policy JSON are preserved during rollback.
- A `downtime_required=true` preflight stops before maintenance, migration, restart, or switch and requires explicit user authorization.
