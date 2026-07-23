# D04 Controlled Launch Readiness Verification

**Date:** 2026-07-22 (Asia/Shanghai)  
**Preparation result:** `COMPLETE`  
**Opening decision:** `NO-GO`  
**Final runtime:** `D04_MODE=read_only`, `D04_REGISTRATION_OPEN=false`

## Prepared Contract

The launch policy and preparation-only Compose overlay are fixed at:

```text
maximum users: 15
daily login credit: USD 20 per Shanghai day
provisional total cost-risk budget: USD 100
conservative Wawazz cost factor: 1000 bps
```

The USD 100 limit is not approved spending. Applying the launch overlay still requires a fresh `go` artifact and explicit approval bound to the policy/snapshot hashes.

The automated evaluator is offline and report-only. Its fresh test result was `11 runs / 91 assertions`, with zero failures. It rejects missing or credential-shaped input and fails closed on stale/unknown finance, insufficient quality samples, budget/health/backup/mode/ownership failures, or missing approvals.

## Current Decision

The live non-sensitive snapshot at `2026-07-22T05:03:00+08:00` produced:

```json
{
  "decision": "no_go",
  "blocking_reasons": [
    "budget_not_approved",
    "opening_window_not_approved",
    "wawazz_balance_below_reserve",
    "wawazz_balance_days_low",
    "wawazz_financial_evidence_stale",
    "wawazz_samples_insufficient"
  ],
  "provider_balance_days": 0.1873,
  "real_action_executed": false,
  "external_system_contacted": false
}
```

The latest supplier-page evidence was approximately USD 9.62 balance and USD 51.3664 observed spend, recorded about 614 minutes before the snapshot. The balance was below the USD 10 floor and covered only about 0.1873 days at that observed rate. Sub2API had zero `GPT-Plus` success or error rows in the fresh 15-minute window, so no customer-path success/error/TTFT/total-latency assertion is possible. Zero samples are reported as insufficient evidence, not as a false 0% quality result.

A final offline reevaluation at `2026-07-22T05:38:00+08:00` remained `no_go`. Because the same secret-free snapshot had aged, `wawazz_metrics_stale` was added to the six reasons above; financial evidence age was about `648.77` minutes and quality-metric age was about `37.66` minutes. The evaluator still reported `real_action_executed=false` and `external_system_contacted=false`. The authoritative current blocking set is therefore seven reasons; thresholds were not weakened and no traffic was generated to refresh the snapshot.

## Passed Readiness Gates

- D04 remains healthy with restart count `0`, OOM false, `read_only`, and registration closed.
- Same-origin registration returns `403 D04_REGISTRATION_CLOSED`.
- D04 retains exactly one internal user, one successful credit grant, and zero usage rows.
- relay-ops remains `read_only + dry_run`; PostgreSQL and Redis are healthy.
- Disk use is 30%.
- A fresh two-database backup set passed SHA-256, `pg_restore --list`, isolated PostgreSQL 18 restore, exact per-table row hashes, and temporary-resource cleanup.
- The read-only rollback is encoded as the independent Compose project and requires no ad hoc environment edit.
- The primary operator role and Feishu operations support channel are documented; the actual 24-hour window cannot be scheduled until an opening time is approved.

## Required Before Opening

1. Approve the exact USD 100 cost-risk ceiling or replace it with another explicit policy and rerun all gates.
2. Replenish Wawazz and verify at least USD 10 plus three days of observed-spend coverage.
3. Refresh provider balance/spend evidence within 20 minutes of opening.
4. Refresh quality metrics within the policy freshness window, collect at least 20 real customer-path samples in a fresh 15-minute window, and satisfy success/error/TTFT/total-latency gates.
5. Approve an exact opening window and schedule the T+0/15m/1h/4h/8h/24h checkpoints.
6. Rerun the evaluator and require `decision=go` before applying the launch overlay.

The preparation stage is complete even though opening is correctly blocked. No user, grant, model request, route, balance, Key, candidate, probe, or Feishu event was created during readiness work.
