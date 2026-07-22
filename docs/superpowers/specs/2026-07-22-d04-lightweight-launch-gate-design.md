# D04 Lightweight Launch Gate Design

**Date:** 2026-07-22 (Asia/Shanghai)
**Status:** Approved design, pending written-spec review
**Policy ID:** `D04-LIGHTWEIGHT-LAUNCH-v2`

## Problem

The v1 D04 readiness policy proved that launch checks can be evaluated offline, but it is heavier and more provider-specific than this relay station needs. It combines a named upstream, balance-runway estimates, separate budget and opening-window approvals, encrypted off-site backup, restore-drill freshness, and local launch readiness in one decision.

The launch gate should remain meaningful without becoming a disaster-recovery project. Its job is to answer one narrow question: is the current D04 deployment ready for a controlled opening of no more than 15 users, based on a fresh local account backup, healthy services, sufficient active-upstream balance, recent quality evidence, a tested rollback, and one explicit launch approval?

## Goals

1. Replace the v1 opening decision with a provider-neutral, report-only v2 evaluator.
2. Protect the authoritative user-account data with a small server-local backup set.
3. Require recent natural-traffic quality evidence for whichever upstream is active at evaluation time.
4. Preserve D04 launch limits and rollback safety without adding off-site storage or recurring restore drills.
5. Keep registration closed until the v2 evaluator returns `go`; the evaluator never opens registration itself.

## Non-goals

- Do not require or implement encrypted off-site backup.
- Do not require seven-day retention or a periodic restore drill as an opening gate.
- Do not calculate a minimum number of balance-runway days or require a spend-rate estimate.
- Do not encode a provider name in policy fields, snapshot fields, reason codes, actions, alerts, or evaluator logic.
- Do not manufacture model requests to obtain quality samples.
- Do not change routes, multipliers, prices, balances, Keys, account bindings, candidates, probes, or Feishu command mode.
- Do not automatically apply a launch overlay or enable registration.

## Versioning And Authority

The existing v1 policy, evaluator inputs, reports, and `no_go` output remain historical evidence. They are not rewritten or reinterpreted. V2 uses new policy, example-snapshot, evaluator, and test files so the old provider-specific result remains reproducible and the new contract cannot silently change its meaning.

V2 becomes the only readiness policy used for future D04 opening decisions after its implementation and verification. Project status and handoff documents must label v1 results as historical and point current launch work to v2.

## Lightweight Policy

The tracked policy remains secret-free and uses these initial thresholds:

```yaml
schema_version: 2
policy_id: D04-LIGHTWEIGHT-LAUNCH-v2
status: preparation_policy
action_execution_mode: report_only

launch:
  max_users: 15
  daily_login_credit_usd: 20.0
  total_budget_usd: 100.0
  budget_cost_bps: 1000
  active_upstream_balance_min_usd: 10.0
  financial_evidence_max_age_minutes: 20
  quality_window_minutes: 15
  quality_evidence_max_age_minutes: 20
  samples_min: 20
  success_rate_min: 0.95
  error_rate_max: 0.05
  ttft_p95_ms_max: 5000
  total_latency_p95_ms_max: 45000
  account_backup_age_hours_max: 24
  disk_used_ratio_max: 0.80

required_modes:
  d04_mode: read_only
  registration_open: false
  relay_ops_mode: read_only
  feishu_command_mode: dry_run
```

The USD 10 minimum is configuration, not a provider-specific assumption. It can be changed in a reviewed policy revision without changing evaluator code. The USD 100 budget, USD 20 daily credit, 1000-bps cost factor, and 15-user maximum remain launch-configuration invariants; they are not separate approval gates.

## Secret-free Snapshot Contract

The runtime snapshot contains only aggregate, non-sensitive evidence:

```yaml
schema_version: 2
snapshot_id: D04-LIGHTWEIGHT-LAUNCH-EXAMPLE
status: fictional
captured_at: "2026-07-22T10:00:00+08:00"

approvals:
  launch_approved: false

modes:
  d04_mode: read_only
  registration_open: false
  relay_ops_mode: read_only
  feishu_command_mode: dry_run

services:
  sub2api: true
  postgres: true
  redis: true
  caddy: true
  d04: true
  relay_ops: true
  unexplained_restart_count: 0
  oom_killed: false
  disk_used_ratio: 0.30

d04:
  configured_max_users: 15
  configured_daily_login_credit_usd: 20.0
  configured_total_budget_usd: 100.0
  configured_budget_cost_bps: 1000
  registered_users: 1
  balance_drift_usd: 0.0
  read_only_reason: ""

active_upstream:
  role: active_upstream
  balance_usd: 20.0
  financial_recorded_at: "2026-07-22T09:50:00+08:00"
  quality_source: natural_production_traffic
  quality_recorded_at: "2026-07-22T09:50:00+08:00"
  sample_count: 30
  success_rate: 0.99
  error_rate: 0.01
  ttft_p95_ms: 2000
  total_latency_p95_ms: 8000

account_backup:
  archive_created_at: "2026-07-22T09:30:00+08:00"
  sha256_verified: true
  includes_sub2api_postgres: true
  includes_d04_sqlite: true

operations:
  primary_owner: site-owner
  support_channel: feishu-operations-group
  rollback_validated: true
```

`active_upstream.role` and `active_upstream.quality_source` are fixed semantic values, not free-form provider labels. A concrete provider may appear in a dated production evidence report, but never in the v2 schema, reason codes, actions, or evaluator branches.

The snapshot contains exactly one approval boolean. Setting `launch_approved: true` records the one explicit opening approval; there is no second budget or window approval. A `go` result is still only an authorization artifact for an operator-controlled deployment step. It cannot mutate production.

## Account-data Backup

The lightweight backup set is stored only on the production server under a restricted directory. It contains:

- a PostgreSQL custom-format dump of the complete Sub2API database, because the database is small and a full logical dump is simpler and safer than maintaining a fragile table allowlist;
- a transactionally consistent snapshot of the complete D04 SQLite database, including the local roster, grants, reconciliation state, and idempotency records;
- a SHA-256 manifest and a small metadata file containing timestamps, file sizes, and format versions, but no row content or credentials.

The D04 snapshot must use SQLite's online backup mechanism or `VACUUM INTO`; copying only the live database file is forbidden because WAL state could be omitted. The backup command writes to a temporary directory, verifies both files against the manifest, and only then atomically promotes the set as complete. Directories are `0700`; archives and manifests are `0600`. A failed or partial run never replaces the last verified set.

The operational cadence is daily and the local cleanup keeps the latest three verified sets. The evaluator gates only on a verified set no older than 24 hours and complete coverage of both data stores. Retention count is operational housekeeping, not an opening blocker. There is no off-site-copy field, encryption field, restore timestamp, isolated-restore field, or restore-drill age in v2.

The existing isolated restore report remains useful historical evidence, but future D04 openings do not wait for another restore exercise. Restoration stays a manual incident procedure.

## Evaluation Rules

The evaluator validates both YAML documents before evaluation, rejects credential-shaped keys or values, performs no network access, and emits deterministic JSON.

Provider-neutral blocking reasons are:

```text
launch_not_approved
upstream_balance_unknown
upstream_balance_below_minimum
upstream_financial_evidence_stale
upstream_quality_source_invalid
upstream_quality_metrics_stale
upstream_samples_insufficient
upstream_success_rate_low
upstream_error_rate_high
upstream_ttft_p95_high
upstream_total_latency_p95_high
account_backup_stale
account_backup_hash_unverified
account_backup_scope_incomplete
```

Existing neutral operational reasons remain where applicable:

```text
d04_not_read_only
registration_not_closed
relay_ops_not_read_only
feishu_not_dry_run
service_unhealthy
container_restarted
container_oom
disk_pressure
d04_configuration_mismatch
d04_user_limit_exceeded
d04_balance_drift
d04_read_only_reason_present
primary_owner_missing
support_channel_missing
rollback_unverified
```

Quality thresholds are evaluated only when `sample_count` meets the minimum. An insufficient sample count produces `upstream_samples_insufficient`; it does not produce misleading percentile pass/fail results. Samples must come from the declared natural-production-traffic window. Missing, malformed, future-dated, stale, or credential-bearing evidence fails closed.

The four configured D04 launch values must exactly match the tracked policy, and `registered_users` must not exceed `configured_max_users`. These checks keep the simplified approval from authorizing a different user cap, credit, risk budget, or cost factor.

The JSON result includes:

```text
decision: go | no_go
blocking_reasons: stable reason codes
required_actions: provider-neutral operator actions
policy: launch invariants and threshold summary
derived: finance, quality, and backup evidence ages
real_action_executed: false
external_system_contacted: false
```

Required actions use neutral names such as `refresh_upstream_financial_evidence`, `restore_upstream_quality`, and `create_verified_local_account_backup`. They never contain a provider name.

## Launch And Rollback Flow

1. Keep D04 in `read_only` with registration closed while collecting evidence.
2. Create and verify a fresh local account-data backup.
3. Populate the secret-free snapshot from current service health and natural production traffic.
4. Record the single explicit launch approval in that snapshot.
5. Run the offline v2 evaluator.
6. If the decision is `no_go`, leave production unchanged and resolve only the reported blockers.
7. If the decision is `go`, an operator may apply the reviewed D04 launch overlay without another approval gate. The evaluator itself performs no action.
8. If health, balance, quality, budget reconciliation, or D04 state degrades, recreate D04 from the read-only overlay and prove registration again returns `403 D04_REGISTRATION_CLOSED`.

Relay-ops remains `read_only` and Feishu commands remain `dry_run` throughout this launch flow. Registration control does not imply route control.

## Error Handling

- Invalid schema, missing required fields, invalid timestamps, forbidden credential material, or future evidence timestamps produce validation failure and no decision artifact.
- Valid but insufficient or stale evidence produces `decision=no_go` with stable blocking reasons.
- Backup creation uses a lock so concurrent runs cannot mix files. Any command, checksum, or atomic promotion failure leaves the previous verified backup intact.
- Cleanup removes only verified backup-set directories beyond the newest three and never targets the backup root, database volume, or current temporary set.
- The evaluator never catches validation errors by substituting zero or an empty success value.

## Testing

Automated tests must cover:

- a complete provider-neutral `go` fixture;
- every blocking reason and its provider-neutral action;
- nil balance, stale finance, stale quality, insufficient samples, and each quality threshold;
- a strict schema allowlist proving any provider-specific top-level section is rejected as unknown and only `active_upstream` can carry upstream evidence;
- exact D04 launch-configuration matching;
- registered-user count not exceeding the configured maximum;
- account-backup freshness, checksum status, and both required data stores;
- unknown fields or credential-shaped input rejection;
- future timestamp rejection;
- CLI output stability and explicit `real_action_executed=false` / `external_system_contacted=false`;
- backup-script success, partial failure, lock behavior, atomic promotion, permissions, checksum verification, and keep-latest-three cleanup using temporary fixtures only;
- existing v1 tests remaining unchanged and passing as historical regression coverage;
- existing D04 race/vet and Compose/Caddy rollback contracts.

Production acceptance remains read-only until the single explicit approval is recorded and the evaluator returns `go`. That approval authorizes the operator-controlled opening step; there is no second approval gate. Acceptance verifies current service modes, a fresh local backup set, the v2 snapshot and evaluator result, no route/configuration drift, and no manufactured model or Feishu event.

## Acceptance Criteria

- [ ] V1 policy and reports remain unchanged and clearly labeled historical.
- [ ] V2 policy, snapshot, evaluator, reason codes, actions, tests, and runtime output contain no provider-specific names.
- [ ] A current active upstream must have at least the configured minimum balance and fresh natural-traffic quality evidence.
- [ ] A verified server-local backup no older than 24 hours includes the complete Sub2API PostgreSQL database and a consistent D04 SQLite snapshot.
- [ ] No off-site backup, retention-days, restore-drill, spend-rate, or balance-runway requirement exists in v2.
- [ ] One explicit `launch_approved` value replaces separate budget and opening-window approvals.
- [ ] D04 configuration matches the 15-user, USD 20 daily credit, USD 100 budget, and 1000-bps policy values.
- [ ] D04 is healthy, `read_only`, and registration is closed during evaluation; rollback remains validated.
- [ ] Relay-ops remains `read_only`, Feishu commands remain `dry_run`, and the evaluator performs no external action.
- [ ] Current-state, handoff, runbook, and verification report identify v2 as the active launch gate and the next mainline as the controlled D04 opening.

## Residual Risks

- Server-local backup does not protect against complete host loss. This is an accepted trade-off for the current lightweight relay-station scope, not an unfinished v2 gate.
- A daily backup can lose changes made after the latest successful set. The 24-hour maximum age bounds this exposure; operators may create an additional fresh set immediately before opening.
- Natural production traffic may not provide 20 recent samples. The correct result is `no_go`; the system must wait for organic evidence rather than generating paid traffic.
- A minimum balance is only a short stop-loss threshold, not a capacity or runway guarantee. Ongoing balance alerts and D04 budget reconciliation remain necessary after opening.
