# Upstream SSE Capacity and Topology Nonfunctional Design

**Date:** 2026-07-21  
**Status:** `APPROVED FOR OFFLINE IMPLEMENTATION`  
**Scope:** Extend the public, protocol-driven upstream benchmark with bounded SSE capacity evidence and target-topology nonfunctional acceptance, without vendor-specific execution branches.

## 1. Goal

Define a reusable design that can answer two questions independently:

1. What bounded synchronous, streaming, and RPM lower bounds has one channel demonstrated for one explicit route role?
2. Does a proposed primary/backup topology meet its approved error, latency, shared-capacity, sustained-observation, failover, and failback requirements?

The design extends the existing V2 benchmark and protocol adapters. It does not qualify a channel, choose a supplier, approve a new SLO, create credentials, send a live request, or change production routing by itself.

The current target topology may be represented as scenario data:

```text
GPT-Plus -> XM API Plus primary -> Wawazz backup
GPT-Pro  -> XM API Pro primary  -> Wawazz backup
```

These names are an instance of the generic contracts below. No channel ID, hostname, vendor, model, or group name may select an implementation branch.

## 2. Explicit Non-Goals

- No live benchmark, paid probe, production write, route switch, account creation, Key creation, balance change, or configuration application is authorized by this design.
- No automatic retry of an interrupted or billing-ambiguous request on another upstream.
- No claim that a short test proves an SLA, supplier maximum, TPM ceiling, queue depth, or 24-to-72-hour durability.
- No inference that synchronous capacity proves SSE capacity.
- No inference that direct-channel evidence proves gateway, primary-role, backup-role, or shared-pool behavior.
- No new numeric SLO becomes effective until the user approves it in a versioned acceptance profile or proposal.
- No change to valid V1, V2 Chat Completions, or V2 Responses behavior.

## 3. Why Synchronous Capacity Does Not Prove SSE Capacity

A synchronous request occupies a client connection until one complete response arrives. An SSE request may produce an early first event and then retain a connection, parser, proxy slot, upstream lease, account concurrency slot, and downstream writer for much longer. Consequently, equal request counts can impose materially different pressure.

SSE introduces failure modes that a synchronous success cannot expose:

- first-event delay despite an accepted HTTP response;
- clean HTTP 200 followed by a missing terminal event;
- a stalled stream after partial content;
- proxy, socket, idle-timeout, or connection-pool exhaustion;
- incomplete UTF-8 or SSE framing across chunks;
- client cancellation and upstream work continuing after disconnect;
- one long stream starving another group in a shared account pool;
- usage or billing emitted only at stream completion;
- a route change affecting new requests while existing streams remain pinned to the old upstream.

Therefore synchronous and SSE capacity have separate ladders, samples, stable lower bounds, stop reasons, and recommendations. Neither result may populate the other when evidence is absent.

## 4. Architecture

```text
Channel registry + benchmark profile
  -> profile validator and normalizer
  -> protocol adapter registry
  -> common sample executor
       -> synchronous sample
       -> SSE sample
  -> capacity coordinator
       -> sync concurrency ladder
       -> SSE concurrency ladder
       -> RPM ladder
  -> normalized per-channel/per-role evidence

Topology scenario
  -> role-isolation validator
  -> shared-capacity-pool coordinator
  -> sustained-observation collector
  -> failover/failback evidence collector
  -> topology acceptance report
```

Protocol adapters continue to own request construction, usage normalization, SSE parsing, and terminal-event recognition. The common runner owns barriers, pacing, clocks, timeouts, percentiles, error categories, queue signals, request budgets, redaction, and reports. The topology layer consumes normalized samples; it does not construct protocol-specific requests.

## 5. Generic Profile Contract

The extension should be versioned so existing V2 profiles remain valid. A normalized capacity profile contains the following public fields:

```yaml
schema_version: 3
id: bounded-text-capacity-v3

protocol: responses
models_path: /models
generate_path: /responses
terminal_events:
  - response.completed
  - "[DONE]"

prompt: Reply with OK only.
max_output_tokens: 8
request_timeout_seconds: 45

capacity:
  sync_concurrency_levels: [1, 2, 3, 5, 8, 10]
  sse_concurrency_levels: [1, 2, 3, 5, 8, 10]
  rpm_levels: [6, 12, 20, 30]
  rpm_window_seconds: 10
  waves_per_level: 1

metrics:
  percentile_method: nearest_rank
  percentiles: [50, 95]
  record_queue_signal: true
```

The prompt and `max_output_tokens: 8` keep the workload deterministic and bounded. The existing V2 `45` second request timeout remains the default test safety cap. These are workload controls, not customer-facing latency SLOs. Any different Token cap, prompt, timeout, ladder, wave count, or RPM window must appear in dry-run output and in the approval record.

The validator rejects unbounded or missing output caps, zero/negative timeouts, unsorted or duplicate ladder values, unknown percentile methods, arbitrary terminal events outside the protocol profile, and any request path that violates the existing adapter path policy.

## 6. Runner Interface and Result Contract

The public runner should support explicit modes rather than inferring behavior from a channel:

```text
discover
compatibility
sync-capacity
sse-capacity
rpm-capacity
topology-dry-run
topology-observe
topology-drill
```

Every invocation binds evidence to an explicit identity tuple:

```text
channel_id
role: direct | gateway_primary | gateway_backup
public_group_id or non-secret group alias
account_evidence_ref
model_id
profile_id and profile_hash
measurement_location
run_id and recorded_at
```

`account_evidence_ref` is a non-secret immutable reference or redacted fingerprint. It must not contain a Key, Cookie, authorization header, credential file path, or account login.

Each sample returns only normalized evidence:

```text
request_kind: sync | sse
scheduled_at, started_at, first_event_at, completed_at
ttft_ms
total_duration_ms
queue_wait_ms or queue_signal=unknown
http_status_class
error_category
stream_started
stream_completed
terminal_event_class
input_tokens, output_tokens, total_tokens
client_timeout, client_cancelled
```

Response content and provider error text are never stored. Queue wait is reported only when directly measured or when a configured, deterministic queue heuristic fires. It must otherwise remain `unknown`, not zero.

## 7. Channel and Role Isolation

Evidence is qualified per channel and role. The runner must not reuse a pass across these boundaries:

- direct channel versus Sub2API gateway;
- gateway primary versus gateway backup;
- one public group versus another public group;
- one account or credential pool versus another;
- one model versus another, except that a capacity ladder may use the first already-compatible stable text model as defined by V2;
- isolated single-role load versus combined shared-pool load.

A channel used as backup for two groups therefore needs three distinct bodies of evidence:

1. backup behavior for group A;
2. backup behavior for group B;
3. aggregate and per-group behavior while both roles consume the shared pool concurrently.

Direct supplier traffic or unrelated production traffic may be recorded as contextual evidence, but it cannot satisfy a gateway-role gate because routing, headers, account limits, transformations, billing, and queueing differ.

## 8. Bounded Capacity Procedure

For each channel/role pair:

1. Require an already successful compatibility result for the selected model: one sync request and one complete SSE request.
2. Run sync concurrency independently at `1, 2, 3, 5, 8, 10`.
3. Return to a quiet interval and run SSE concurrency independently at `1, 2, 3, 5, 8, 10`.
4. Run the existing bounded RPM ladder independently at `6, 12, 20, 30` over the configured short window.
5. Stop escalation at the first failed level and retain the prior level as the last stable lower bound.
6. Report the highest level as "at least this level," never as the supplier maximum.

All requests in a concurrency level start from a barrier. A level records both planned concurrency and achieved overlap. If the runner fails to demonstrate overlap, the level is invalid rather than passed.

For every level and for the run as a whole, report:

- request count and achieved overlap;
- success and error counts and error rate;
- 429, 5xx, authentication, insufficient-balance, timeout, cancellation, protocol, and unknown categories;
- TTFT P50/P95;
- total-duration P50/P95;
- complete-stream count and stream-interruption ratio;
- measured queue-wait P50/P95 or a clearly named queue signal;
- first and last sample timestamps;
- Token totals and estimated/actual test cost when available.

Small-sample P95 uses nearest-rank and must be labeled with `n`; it is descriptive, not a durability claim.

## 9. Stop Conditions

The capacity coordinator stops the current ladder immediately on any of the following:

- any 429, 5xx, authentication failure, insufficient balance, or model-unavailable response;
- TLS, connection, timeout, protocol, invalid framing, missing terminal event, or response-size-limit error;
- billing that is duplicated, unknown, unexplained, or beyond the approved cost boundary;
- the approved total request, Token, wall-clock, or currency budget is reached;
- achieved overlap is lower than the requested level;
- queueing or total duration crosses an approved stop threshold;
- cleanup, route isolation, or production invariants can no longer be proved;
- an in-flight request has an unknown completion or billing state.

Stopping one channel/role does not silently continue against another channel. Cross-upstream retries require a separately designed request policy and are not part of capacity measurement.

## 10. Shared Capacity Pool and Fairness

The topology schema models a backup shared by multiple public groups with a generic `shared_capacity_pool`:

```yaml
shared_capacity_pools:
  - id: shared-backup-pool-1
    members:
      - group: group-a
        role: gateway_backup
        channel: backup-channel
        requested_concurrency: 1
      - group: group-b
        role: gateway_backup
        channel: backup-channel
        requested_concurrency: 1
    aggregate_concurrency_limit: 2
    allocation_policy: user_approved_policy
```

The schema expresses configuration and evidence identity only. It does not assume that two Sub2API account objects double supplier capacity. Accounts sharing a Key, subscription, upstream worker pool, quota, or provider account are one pool unless independent capacity is proved.

Shared-pool acceptance has three phases:

1. Test each member alone at its intended configured load.
2. Test all members together at the intended aggregate load with equal demand.
3. Test the user-approved traffic mix, including asymmetric demand when one group is expected to be busier.

The report provides aggregate and per-member success, TTFT, total duration, completion, errors, achieved concurrency, queue signals, and completed-request share. It must expose starvation and head-of-line blocking rather than averaging them away.

The pool cannot pass when aggregate metrics look healthy but one member violates an inherited gate, receives no service, or lacks a complete sample set. A production aggregate limit must not exceed the last stable combined level, the supplier/account limit, or the sum of approved per-role limits.

## 11. Short Test and Sustained Observation Are Separate

### 11.1 Bounded short test

The short test discovers compatibility and stable lower bounds with fixed requests and cost. It is designed to fail fast and normally completes in minutes. It does not produce an SLA or long-duration reliability claim.

### 11.2 Sustained observation

The 24-to-72-hour phase is a separate run and approval record. It uses the already-approved conservative concurrency/RPM settings; it does not repeat escalation. It combines low-frequency synthetic probes with clearly labeled real gateway traffic where authorized.

The observation report separates:

- measurement location and network segment;
- channel, role, group, account evidence reference, and model;
- synthetic versus real traffic;
- sync versus SSE;
- hourly sample counts and missing windows;
- errors, TTFT P50/P95, total-duration P50/P95, stream completion, queue signals, 429/5xx, resource use, and cost;
- supplier self-reported status from first-party gateway evidence.

Supplier dashboards and high aggregate request counts are contextual evidence. They do not replace site-side, per-role samples with a known denominator.

## 12. Failover and Failback Evidence

Topology acceptance requires separately authorized drills for each primary/backup pair. When several groups share one backup pool, it also requires a simultaneous-failure drill at the approved aggregate load.

The evidence timeline records monotonic and wall-clock timestamps for:

```text
t_fault_observed
t_detection_confirmed
t_change_requested
t_change_accepted
t_route_converged
t_first_backup_success
t_primary_recovery_confirmed
t_failback_requested
t_failback_converged
t_first_primary_success
```

Reports calculate, without hiding components:

```text
service_failover_rto = t_first_backup_success - t_fault_observed
control_failover_time = t_route_converged - t_change_requested
service_failback_rto = t_first_primary_success - t_failback_requested
```

If `t_fault_observed` is unavailable, service failover RTO is `unknown`; the report may still provide detection-to-success and control convergence. Dry-run command success proves only validation and predicted state, not RTO.

The drill proves route state by read-after-write, then sends only the approved bounded verification requests. Existing SSE connections are not claimed to migrate or resume. A stream interrupted by the fault remains interrupted; only new requests may use the backup. Billing-ambiguous requests are reconciled before any retry.

Failback requires primary health evidence across the user-approved recovery window, explicit authorization, route convergence, a first successful primary sync/SSE check, and confirmation that the backup returned to its intended role. Partial, mixed, none, or unprovable route state fails the drill and stops further writes.

## 13. Threshold Ownership

### 13.1 Inherited OPS01 gates

These existing OPS01 thresholds are not relaxed by this design. A breach blocks qualification and produces report-only action; it does not authorize an automatic production change:

| Metric | Existing OPS01 gate |
|---|---:|
| Upstream success rate | at least `95%` |
| 429 ratio | at most `15%` |
| 5xx ratio | at most `5%` |
| TTFT P95 | at most `5000 ms` |
| Stream interruption ratio | at most `1%` |
| Upstream balance runway | at least `3 days` |
| Request ID coverage | `100%` |
| Daily total cost | at most `USD 20` |
| Per-user daily cost | at most `USD 5` |
| Unexplained balance difference | at most `USD 0.01` absolute |

For a short ladder, the fail-fast stop rules are intentionally stricter than the aggregate OPS01 percentages: one qualifying error stops escalation. Passing the ladder still requires the aggregate inherited gates to pass.

### 13.2 Proposed values requiring user approval

The following are design suggestions, not approved SLOs or active gates. Until the user approves concrete values in a versioned acceptance profile, the runner reports measurements and marks the corresponding topology decision `pending_threshold_approval`:

| Proposed field | Suggested initial value | Purpose |
|---|---:|---|
| `total_duration_p95_degradation_max` | `30%` above the isolated role baseline | Prevent a shared pool from hiding severe tail regression |
| `queue_wait_p95_ms_max` | `1000 ms` | Bound observable scheduler/upstream queue delay |
| `shared_pool_share_deviation_max` | `20%` from the requested traffic share | Detect unfair service when both roles are backlogged |
| `short_test_capacity_headroom` | configure at `70%-80%` of last stable level, rounded down | Preserve the existing V2 conservative recommendation |
| `sustained_observation_hours_min` | `24 hours`; extend toward `72` after errors or degradation | Separate durability evidence from short capacity |
| `failover_rto_seconds_max` | `60 seconds` | Bound user-visible primary-to-backup recovery |
| `failback_rto_seconds_max` | `120 seconds` | Bound controlled restoration to primary |

The user may approve different values or decline an absolute threshold. Every approval binds the profile ID/hash, topology scenario ID/hash, measurement locations, workload, and validity window. Reports always show the measured values even when no pass/fail threshold is approved.

## 14. Secret, Budget, and Cleanup Boundary

- A dry-run is mandatory before any network request. It reports model count assumptions, requests by phase, Token cap, timeout, duration bound, and currency boundary.
- Provider and downstream Keys are temporary, low-quota, short-lived, and installed only through the approved secret boundary. Their values never enter chat, files, command arguments, Git, reports, ledger entries, fixtures, or logs.
- Cookie, password, TOTP data, authorization headers, prompts beyond the fixed public probe, response content, and provider error text are never persisted.
- Live discovery, compatibility/capacity, sustained observation, and topology drill are separate authorization gates. Approval of one does not authorize the next.
- Every live phase has approved maximum requests, output Tokens, wall-clock time, and monetary cost, plus an owner for cleanup.
- Cleanup disables/deletes temporary provider Keys and downstream Keys and removes or disables approved isolated users, groups, and accounts.
- Production route, group, pricing, model, multiplier, concurrency, and mode invariants are re-read after cleanup.
- Incomplete cleanup, unexplained billing, or an unprovable invariant makes the run `partial` or `blocked`, never passed.

## 15. Dry-Run Request Bounds

Let:

```text
M = discovered text-model count for one channel
D = 1 when model discovery is included, otherwise 0
Csync = sum(sync concurrency levels)
Csse = sum(SSE concurrency levels)
R = sum(ceil(rpm_level * rpm_window_seconds / 60))
W = waves per concurrency level
K = explicitly configured topology verification requests
```

The per-channel qualification bounds are:

```text
compatibility generation requests = 2M
sync capacity requests = W * Csync
SSE capacity requests = W * Csse
RPM requests = R

qualification HTTP requests = D + 2M + W*Csync + W*Csse + R
qualification generation requests = 2M + W*Csync + W*Csse + R
```

With the proposed default ladders, `W=1`, and a 10-second RPM window:

```text
Csync = 29
Csse = 29
R = 1 + 2 + 4 + 5 = 12

HTTP requests with discovery = 2M + 71
generation requests = 2M + 70
```

For multiple channels, sum the formula using each channel's own `M`; do not substitute one channel's directory size for another. Shared-pool, failover, and failback requests are not hidden inside qualification. Their exact configured count is `K` and the total dry-run bound is:

```text
total HTTP requests = sum(channel qualification HTTP requests) + K
```

Sustained observation uses a separate formula based on approved probe cadence, duration, roles, and request kinds. It must not be represented as part of a short-run bound.

## 16. Test Strategy

### 16.1 Unit tests

- Validate versioned profile fields, bounded Tokens/timeouts, sorted ladders, percentile method, and backward compatibility.
- Verify exact dry-run counts for arbitrary model counts, ladders, windows, waves, channels, and topology request counts.
- Verify nearest-rank P50/P95, error ratios, stream-completion ratio, and unknown queue behavior.
- Verify evidence identity prevents reuse across channel, role, group, account reference, model, profile, or location.
- Verify OPS01 inherited gates and user-approved proposed gates remain separately labeled.

### 16.2 Protocol and fixture tests

- Complete SSE with LF and CRLF framing and the configured terminal event.
- Delayed first event, delayed later event, missing terminal event, partial UTF-8, oversized stream, client cancellation, and overall timeout.
- 429, 5xx, authentication, insufficient balance, model unavailable, TLS/connection, protocol, and unknown fixed error classes.
- Usage before completion, usage at completion, and missing usage without storing response content.

### 16.3 Capacity tests

- Barrier-started sync and SSE levels demonstrate actual overlap.
- The runner stops after the first failing level and never schedules a higher one.
- Sync and SSE last-stable results cannot overwrite one another.
- Queue signals and percentile samples stay scoped to the correct level.
- Token, request, duration, and cost caps stop scheduling deterministically.

### 16.4 Shared-pool and topology tests

- Two arbitrary groups share a generic pool without vendor-specific code.
- Aggregate success cannot hide per-member failure or starvation.
- Equal and asymmetric requested mixes produce per-member and total metrics.
- Dry-run route commands cannot generate RTO evidence.
- Fixture timelines calculate service and control intervals correctly and preserve `unknown` when a timestamp is absent.
- Mixed, partial, none, read-after-write mismatch, and recovery-window failure stop the drill.

### 16.5 Security and compatibility tests

- No secret or response content reaches stdout, stderr, JSON, Markdown, YAML, ledger, fixture failure, or command arguments.
- Existing V1 and V2 profiles and Chat Completions/Responses adapters remain unchanged.
- Source scans and arbitrary channel fixtures prove no vendor, hostname, model, or channel branch.

## 17. Acceptance Conditions

### 17.1 Design implementation acceptance

The future implementation is accepted only when:

1. Existing profiles remain compatible and the new profile fails closed on unsafe bounds.
2. Sync and SSE capacity run independently and produce separate stable lower bounds.
3. Every result is bound to channel, role, group, account evidence reference, model, profile, and location.
4. All required metrics, sample counts, completion semantics, error classes, and queue unknowns are reproducible.
5. Shared-pool reports expose aggregate and per-member capacity and fairness.
6. Dry-run request formulas exactly match scheduled requests, including topology verification.
7. Short tests, sustained observation, and topology drills are separate authorization and evidence records.
8. Failover/failback reports preserve the full timeline and never infer RTO from dry-run.
9. OPS01 gates and user-approved proposed thresholds cannot be confused in config or reports.
10. Automated tests prove redaction, fixed bounds, fail-fast behavior, backward compatibility, and absence of vendor branches.

### 17.2 Channel-role qualification acceptance

A channel/role may be called short-test qualified only when compatibility, its requested capacity modes, cleanup, billing explanation, and inherited OPS01 gates pass. Untested modes remain `unknown`. A passed short test does not imply sustained qualification.

### 17.3 Topology nonfunctional acceptance

A topology may be recommended for a separately approved production change only when:

- every primary and backup role is independently qualified for its required models and request kinds;
- every shared pool passes isolated and combined-load evidence;
- the user has approved all new thresholds needed for a pass/fail decision;
- the 24-to-72-hour observation required by that approval is complete;
- each failover and failback drill has complete, bounded, reproducible RTO and route-state evidence;
- budget, billing, cleanup, production invariants, and secret boundaries pass;
- the secret-free proposal binds all evidence and the user explicitly approves its ID/hash.

Until all conditions are met, the report must state the exact missing evidence and keep the target topology proposed rather than qualified.
