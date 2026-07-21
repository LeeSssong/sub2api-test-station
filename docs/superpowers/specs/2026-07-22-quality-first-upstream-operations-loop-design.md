# Quality-first Upstream Operations Loop Design

**Date:** 2026-07-22 (Asia/Shanghai)  
**Status:** Approved  
**Scope:** Replace the heavyweight one-time qualification mainline with a bounded, reusable operations loop while preserving optional deep qualification tools.

## Problem

The repository can discover catalogs and run comprehensive direct benchmarks, but the current mainline still assumes a roughly 190-generation-request qualification before useful comparison. That workflow is too expensive and too slow for routine upstream operations. Administrators need frequent, comparable evidence, a concise Feishu report, and a guarded management-console switch without turning Feishu into a route controller.

## Goal

Build this reusable workflow:

```text
Scheduled bounded tests
  -> absolute hard gates
  -> quality-first relative scoring
  -> quality / price / capacity report
  -> Feishu notification with /ops link
  -> administrator review
  -> manual snapshot / apply / verify / rollback
```

The first live acceptance uses the existing local Sub2API accounts `73`, `74`, and `75`. Their credentials remain in the local Sub2API database and are passed to the runner only in process memory. They are never copied into Git, reports, command arguments, or production relay-ops.

## Non-goals

- No account pool, fairness scheduler, 24-72 hour mandatory qualification, automatic failover drill, or repeated full billing audit in the first version.
- No vendor-, hostname-, account-name-, or model-name-specific execution branch.
- No automatic production route change and no Feishu switch button or route-changing Feishu command.
- No production candidate creation for local accounts `73/74/75`.
- No customer exposure when the base price or billing rule is unknown.
- No Neko balance work.

## Test Cadence

| Job | Cadence | Coverage | Purpose |
|---|---|---|---|
| `health_pulse` | every 15 minutes | common, expensive, and newly added representative models; one sync and one SSE each | detect availability and latency regressions quickly |
| `catalog_quick` | every 6 hours | one sync and one SSE for every discovered text model | maintain the openable model catalog |
| `capacity_check` | daily and after configuration changes | representative models at concurrency `1/2/3/5/8/10` and RPM `6/12/20/30` | establish a bounded lower limit and detect queueing |
| `incident_recheck` | immediately after production-upstream failure | all configured candidates using the bounded quick profile | produce a fresh recommendation, never an automatic switch |

Every run records direct upstream measurements. When an isolated Sub2API path is available, it also records gateway measurements and the direct-versus-gateway delta. A missing view remains `unknown`; it is never inferred from another view.

## Measurements

Each model sample records only normalized, content-free fields:

- HTTP and protocol outcome, error class, timeout, and retry-free request count;
- TTFT and total latency with nearest-rank P50/P95;
- output-token generation speed when usage is available;
- SSE terminal-event completion and disconnect state;
- normalized input, output, and total usage;
- direct or gateway measurement location;
- concurrency/RPM level, queueing signal, profile hash, run ID, and timestamp.

Prompts and generated content are not persisted. Authorization values, cookies, passwords, upstream response bodies, and credentials are forbidden in output and ledgers.

## Absolute Hard Gates

Hard gates run before scoring. A failed or unknown mandatory gate makes the result ineligible for switching regardless of price.

1. HTTPS, authentication, `/models`, required protocol, and bounded response parsing must succeed safely.
2. Every model proposed for customer exposure must pass one synchronous request and one complete SSE request. A failed model is excluded from the openable set.
3. The three representative roles must be present. A role may be mapped by configuration, never by vendor-specific code.
4. A pulse or quick run permits zero `401`, `403`, `429`, `5xx`, timeout, transport, malformed-usage, wrong-model, or incomplete-SSE outcomes.
5. A capacity level passes only with 100% success, 100% SSE completion where streaming is used, and no request exceeding three times the single-request baseline total latency. Testing stops at the first failed level.
6. Gateway validation fails when gateway overhead is both material and excessive: TTFT delta greater than `max(500 ms, 20%)` or total-latency delta greater than `max(2000 ms, 20%)`.
7. Billing must be explainable before commercial exposure. `explicit_model_price` requires verified per-model prices. `multiplier_only` requires trusted base prices and a verified upstream multiplier. Unknown base price or billing semantics permits technical testing only and blocks switching.
8. Cleanup, snapshot creation, route pre-read, or post-apply verification uncertainty blocks switching.

## Quality-first Relative Scoring

Only hard-gate-passing evidence receives a score. Unknown inputs receive zero points and stay visible.

| Dimension | Weight | Inputs |
|---|---:|---|
| Reliability | 40 | success, error, timeout, SSE completion |
| Latency | 25 | TTFT and total-latency P50/P95 |
| Generation quality proxy | 10 | valid usage, output speed, protocol completion |
| Capacity | 15 | last stable concurrency/RPM and queueing |
| Price | 10 | verified model price or multiplier evidence |

A candidate may be recommended for review only when:

- all mandatory hard gates pass;
- quality score is at least `80/100` before price points are added;
- reliability is not worse than the current production baseline;
- neither TTFT P95 nor total-latency P95 regresses by more than 5%;
- at least one of reliability, latency, capacity, or verified price is materially better;
- evidence is fresh for its job cadence.

The output statuses are `blocked`, `needs_evidence`, `not_better`, `review_recommended`, and `eligible_for_manual_switch`. A score never grants switch authority.

## Model, Price, and Promotion Rules

- Every discovered text model is catalogued. Only individually passing models enter the openable set.
- `explicit_model_price` displays per-model prices on the pricing page.
- `multiplier_only` hides per-model price claims and displays only the verified multiplier plus its base-price source.
- Unknown base prices or billing rules block customer exposure and switching.
- Temporary promotional groups are separate offers with an explicit expiry. Expiry cannot silently mutate the permanent group.
- A switch proposal contains the exact model allowlist, model mapping, price mode, price payload or multiplier, account cost multiplier, group multiplier, concurrency, RPM, and expiry metadata.

## Feishu Contract

Feishu is notification-only. The card contains:

- gate result and quality-first score breakdown;
- direct, gateway, and overhead summaries;
- openable and blocked model counts;
- verified price mode, balance status, and capacity lower bound;
- unknowns and stop reasons;
- report ID/hash and an `运维后台` link.

No card has switch, confirm, retry, or route-changing buttons. Existing incident deduplication applies to repeated equivalent reports.

## Management-console Switch State Machine

```text
observed -> eligible -> reviewed -> approved -> applying -> verifying -> active
                                 \-> blocked
applying/verifying failure -> rollback_required -> rolled_back | manual_intervention
```

The administrator must review the report, upstream status, balance confirmation, model/pricing payload, and exact report/proposal hash in `/ops`. The apply operation:

1. acquires group and account locks;
2. saves a redacted, restorable pre-switch snapshot;
3. rechecks report freshness and all hard gates;
4. applies model list, mapping, price mode, price or multiplier, group binding, concurrency, and RPM as one audited operation;
5. performs one bounded sync and one bounded SSE gateway check;
6. commits `active` only after read-after-write verification;
7. restores the snapshot on a verified-safe failure and enters `manual_intervention` if restoration cannot be proven.

Production application remains disabled while relay-ops is `read_only`. The console may render evidence and a dry-run preview, but it cannot write until a separately approved production mode and exact proposal are present.

## Three-stage Project Mainline

1. Complete this lightweight upstream test, report, notification, and guarded console workflow; use local Sub2API accounts `73/74/75` for live acceptance without creating production candidates.
2. Complete D04 single-user low-budget production acceptance: registration, automatic login, one Shanghai-day USD 20 grant, same-day idempotency, three-way reconciliation, then restore `D04_MODE=read_only` and close registration.
3. Keep the existing Feishu/monitoring mainline closed. Wait for a natural alert/recovery card visual as a non-blocking observation; do not manufacture incidents or duplicate implementation.

## Verification and Acceptance

- Unit and contract tests cover gate ordering, score calculation, unknown evidence, cadence, report/card rendering, snapshot/apply/verify/rollback, and secret redaction.
- Live local acceptance uses only accounts `73/74/75`, saves pre/post non-sensitive snapshots, and restores their schedulable/binding state.
- D04 production acceptance creates at most one isolated user and one USD 20 site-balance grant, sends no model request, and restores read-only/registration-closed state even on failure.
- relay-ops remains `read_only + dry_run`; production routing, prices, multipliers, balances, Keys, candidate rows, and deduplication rows remain unchanged.
- `current-state.md`, `llm-handoff.md`, and dedicated verification reports record facts, unknowns, hashes, cleanup, and the next mainline.
