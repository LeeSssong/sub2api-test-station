# D04 Sub2API-Sourced Active Upstream Discovery Design

**Date:** 2026-07-22 (Asia/Shanghai)
**Status:** Approved for implementation
**Target policy:** `D04-LIGHTWEIGHT-LAUNCH-v3`

## Problem

The v2 launch gate accepts one manually populated `active_upstream` object. That is the wrong source of truth: it can omit a schedulable account, keep evaluating an account after scheduling is disabled, and encourages provider-specific assumptions outside Sub2API.

Sub2API already owns the production scheduling state. An account explicitly configured as both `status=active` and `schedulable=true` can receive traffic and must therefore be treated as a current active upstream. D04 readiness must discover that complete set from Sub2API and require every member to pass independently.

## Goals

1. Derive the complete active-upstream set from Sub2API account state with no manual provider selection.
2. Evaluate balance and natural quality evidence separately for every discovered account.
3. Fail closed when discovery, per-account attribution, freshness, or evidence is incomplete.
4. Show the discovered accounts and their individual gate results in the existing `/ops` interface.
5. Preserve the lightweight operating model: read-only discovery, server-local evidence, no paid probe, no route mutation, and no automatic opening.

## Non-goals

- Do not encode a provider name, hostname, account name, or fixed account ID in policy or evaluator logic.
- Do not add a second routing or upstream-management system beside Sub2API.
- Do not let `/ops`, relay-ops, or the evaluator change `status`, `schedulable`, groups, priorities, routes, prices, balances, Keys, or credentials.
- Do not aggregate several accounts into one balance or quality result.
- Do not substitute group-level metrics when they cannot prove which account served the request.
- Do not manufacture model traffic, enable paid probes, or generate synthetic Feishu incidents to satisfy the gate.
- Do not require encrypted off-site backup, balance-runway calculations, or recurring restore drills.

## Source Of Truth

An active upstream is one Sub2API account satisfying both conditions:

```text
status == "active"
schedulable == true
```

No provider-name or public-group allowlist participates in membership. Accounts with scheduling disabled are excluded. A schedulable account remains in the set while temporarily rate-limited, overloaded, expired, or temporarily unschedulable because it may automatically resume without a configuration change; that runtime condition is recorded and blocks opening until healthy.

The discovery result is sorted by `account_id` and includes the account's current `group_ids` for explanation and quality attribution. An empty group list does not make a schedulable account disappear; it remains visible and fails closed because no natural routed quality can be attributed to it.

If Sub2API returns zero active accounts, duplicate account IDs, incomplete pagination, malformed state, or an account set that changes during capture, the collector returns no valid launch snapshot.

## Architecture

### 1. Sub2API read-only account discovery

Extend the existing relay-ops Sub2API client with a paginated `ListAccounts` method. It calls the official Admin GET endpoint, follows the response pagination contract, enforces a bounded maximum, and reads only non-secret account metadata:

```text
id
name
platform
status
schedulable
group_ids
expires_at
auto_pause_on_expired
rate_limit_reset_at
overload_until
temp_unschedulable_until
temp_unschedulable_reason
```

Credential values are never requested, stored, logged, returned to `/ops`, or written to the launch snapshot. The collector records a canonical SHA-256 of sorted account IDs and scheduling fields so the decision is bound to the discovered set.

### 2. Per-account evidence collection

Membership comes only from Sub2API. Evidence is then joined by `account_id`:

- **Balance:** use the freshest available read-only balance evidence for that account. Sub2API currently does not expose a trusted upstream-balance field in the existing account DTO, so the first implementation accepts a secret-free local balance-evidence record keyed by `account_id`. Missing evidence is an explicit blocker; it never removes the account.
- **Quality:** use natural Sub2API request records attributable to the serving `account_id` within the configured 15-minute window. Compute request count, success rate, error rate, TTFT P95, and total-latency P95 per account. If the production API cannot provide account-attributed duration and TTFT fields, return `upstream_quality_attribution_missing`; do not reuse a mixed group aggregate.
- **Runtime availability:** evaluate temporary scheduling blocks and expiry from the Sub2API account metadata. A configured active account that is not currently runtime-eligible remains listed and returns `upstream_temporarily_unavailable`.

The balance evidence file is server-local, secret-free, and contains only account IDs, numeric balances, source timestamps, and optional non-sensitive evidence references. It is not a route-selection file and cannot add or remove active accounts.

### 3. Snapshot collector

A report-only collector reads Sub2API, joins evidence, validates timestamps, and writes the Git-ignored snapshot atomically. It performs Admin GET requests only and emits no credentials or raw request content.

The v3 snapshot replaces the singular v2 object with an array:

```yaml
schema_version: 3
snapshot_id: D04-LIGHTWEIGHT-LAUNCH-V3-EXAMPLE
captured_at: "2026-07-22T10:00:00+08:00"

approvals:
  launch_approved: false

upstream_discovery:
  source: sub2api_admin_accounts
  recorded_at: "2026-07-22T09:59:50+08:00"
  account_set_sha256: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef

active_upstreams:
  - account_id: 7
    display_name: Primary account
    platform: openai
    status: active
    schedulable: true
    group_ids: [2]
    runtime_available: true
    balance_usd: 20.0
    financial_recorded_at: "2026-07-22T09:55:00+08:00"
    quality_source: sub2api_account_attributed_natural_traffic
    quality_recorded_at: "2026-07-22T09:59:00+08:00"
    sample_count: 30
    success_rate: 0.99
    error_rate: 0.01
    ttft_p95_ms: 2000
    total_latency_p95_ms: 8000
```

The existing modes, service health, D04 configuration, account-backup, operations, and approval sections remain. `display_name` is output data for operators, not a policy selector.

### 4. V3 evaluator

V3 is a breaking schema change and receives a new policy ID and evaluator path. V2 files and dated results remain immutable historical evidence.

The evaluator requires at least one discovered account and evaluates every array member independently. The top-level result contains unique generic reason codes plus a per-account result:

```json
{
  "decision": "no_go",
  "blocking_reasons": ["upstream_balance_below_minimum"],
  "upstreams": [
    {
      "account_id": 7,
      "decision": "no_go",
      "blocking_reasons": ["upstream_balance_below_minimum"]
    }
  ],
  "real_action_executed": false,
  "external_system_contacted": false
}
```

Top-level `decision=go` is possible only when discovery, every active account, backup, modes, service health, D04 configuration, operations ownership, rollback, and the single launch approval all pass in the same snapshot.

New generic blockers are:

```text
active_upstreams_empty
upstream_discovery_failed
upstream_discovery_stale
upstream_account_set_changed
upstream_temporarily_unavailable
upstream_quality_attribution_missing
```

Existing provider-neutral v2 balance, freshness, sample, quality, backup, mode, health, configuration, ownership, and rollback blockers continue to apply per account where relevant.

### 5. `/ops` read-only status

Add one unframed D04 launch-readiness section to the existing operations page. It reads the latest validated snapshot and evaluator artifact and shows:

- source: `Sub2API scheduling state`;
- discovery and decision timestamps;
- overall `GO` or `NO-GO`;
- one row per active account: account ID, display name, groups, scheduling/runtime state, balance freshness, sample count, success/error rate, TTFT P95, total-latency P95, and blockers;
- a clear empty/error state when discovery has no valid evidence.

The section has no upstream picker, scheduling toggle, route switch, launch button, probe button, balance input, or credential field. It cannot trigger collection or mutate state. Existing `/ops` authentication remains unchanged.

## Capture And Opening Flow

1. Keep D04 `read_only/registration=false` and relay-ops `read_only/dry_run`.
2. Read all Sub2API accounts and derive the active set from `active + schedulable` only.
3. Join balance and natural account-attributed quality evidence for every discovered account.
4. Create the verified server-local account backup.
5. Atomically write the v3 secret-free snapshot and evaluate it offline.
6. Show the same discovered set and decision in `/ops`.
7. On `NO-GO`, leave production unchanged and report exact account-scoped blockers.
8. Only a same-snapshot `GO` with the existing single approval authorizes the operator-controlled launch overlay.

Immediately before applying the overlay, rerun discovery and require the canonical account-set hash to match the evaluated snapshot. Any scheduling change invalidates the artifact and requires a new capture and decision.

## Error Handling

- Sub2API authentication, pagination, schema, timeout, or response-size errors fail discovery and produce no valid snapshot.
- Missing or stale balance evidence blocks only the affected account but makes the overall decision `NO-GO`.
- Missing, mixed, or unattributable quality evidence blocks the affected account; group averages never fill the gap.
- Future timestamps, duplicate account IDs, unknown fields, credential-shaped keys/values, or hash mismatches are validation errors.
- A zero-account result is `NO-GO`, never an empty success.
- The last valid `/ops` artifact may remain visible with a prominent stale marker, but it cannot authorize opening after expiry or account-set drift.
- Collector or evaluator failures do not modify the previous snapshot, account scheduling, routes, or D04 runtime.

## Test Strategy

Automated coverage must include:

- paginated Sub2API account discovery and exact `active + schedulable` membership;
- exclusion of scheduling-disabled accounts regardless of provider name;
- inclusion of schedulable accounts with empty groups or temporary runtime blocks;
- deterministic sorting and account-set hashing;
- zero accounts, duplicate IDs, incomplete pagination, response-size, schema, and authentication failures;
- one-account and multi-account `GO` fixtures;
- each per-account balance, freshness, attribution, sample, quality, and runtime blocker;
- proof that one passing account cannot mask one failing account;
- proof that a sidecar balance record cannot add an account absent from Sub2API discovery;
- account-set drift between capture and launch preflight;
- provider-name-free policy, reason code, action, and evaluator scans;
- `/ops` authenticated rendering for multi-account, empty, stale, and error states;
- no controls or endpoints capable of changing scheduling, routes, balances, Keys, probes, or launch mode;
- current v1/v2 historical tests remaining green;
- relay-ops race/vet, frontend syntax, deployment contracts, D04 race/vet, and `git diff --check`.

Production acceptance uses Admin GETs only, confirms the discovered account IDs against the Sub2API account page, confirms `/ops` renders the same canonical hash and per-account decisions, and proves no route, scheduling, balance, Key, candidate, probe, Feishu notification, or database business state changed.

## Acceptance Criteria

- [ ] Active-upstream membership is derived only from Sub2API accounts with `status=active` and `schedulable=true`.
- [ ] Every discovered account is present exactly once and evaluated independently.
- [ ] Every account must independently pass minimum balance and fresh natural account-attributed quality thresholds.
- [ ] One account's evidence cannot satisfy or mask another account's blocker.
- [ ] Provider names and fixed account IDs do not appear in policy or evaluator branches.
- [ ] Account-set changes invalidate the evaluated artifact before opening.
- [ ] `/ops` shows the discovered set, per-account evidence, blockers, and overall decision without write controls.
- [ ] Discovery, collection, evaluation, and UI rendering do not mutate production or contact an upstream model API.
- [ ] D04 remains `read_only/registration=false` and relay-ops remains `read_only/dry_run` until a v3 same-snapshot `GO`.
- [ ] V1 and v2 remain reproducible historical evidence and are not silently reinterpreted.

## Risks And Trade-offs

- Sub2API scheduling state identifies who may receive traffic, but its current account DTO does not provide trusted upstream balance. A separate read-only per-account balance evidence record remains necessary until Sub2API exposes such data.
- Per-account natural quality may be unavailable when production telemetry only exposes group aggregates. Failing closed can delay D04 opening, but aggregation would contradict the requirement that every schedulable account pass independently.
- A schedulable account with no groups may not currently receive traffic, yet it remains included because the user's declared source of truth is the scheduling switch. This favors visibility and safety over inference from route topology.
- The new policy is v3 rather than an in-place v2 edit so prior decisions remain auditable and no old snapshot is accepted under new multi-account semantics.
