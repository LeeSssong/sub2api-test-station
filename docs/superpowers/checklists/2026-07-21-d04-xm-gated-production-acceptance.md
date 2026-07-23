# D04 and Generic Upstream Gated Production Acceptance

**Date:** 2026-07-21 (Asia/Shanghai)  
**Status:** STAGES 1 AND 2 AUTHORIZED; EXECUTION PENDING  
**Target topology:** `GPT-Plus -> XM Plus primary -> Wawazz backup`; `GPT-Pro -> XM Pro primary -> Wawazz backup`

## Capability And Run Identity

The benchmark capability is vendor-neutral. It always accepts a registry `channel_id`, protocol profile, Base URL and model-directory path, then follows the shared workflow `discover -> compatibility -> gateway -> billing -> capacity -> proposal`. XM is only the current pair of channel instances (`xm-plus` and `xm-pro`) and may appear in their registry rows and evidence report; it is not a protocol, implementation branch, directory layout or reusable capability name.

This checklist separates functional acceptance from non-functional qualification and keeps every production mutation behind a named approval. Completing an earlier stage does not authorize a later stage.

## Permanent Invariants

- `relay-ops` remains `RELAY_OPS_MODE=read_only` and `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`; never enter `enabled` in this workflow.
- Do not process Neko balance and do not use Neko health as an XM acceptance gate.
- Do not read or output `.env`, Admin API Keys, upstream Keys, Cookies, passwords, TOTP values, App secrets or user tokens.
- Do not put an upstream Key in chat, repository files, command arguments, reports, logs or the benchmark ledger.
- Do not directly write Sub2API PostgreSQL or D04 SQLite.
- Do not create a production candidate, enable paid probe, alter prices/multipliers, or change production routing before the exact proposal approval gate.
- Preserve the current route canonical snapshot and all existing container IDs before each production stage.

## Stage 0: Read-only Baseline

- [x] Record UTC/Shanghai timestamps and operator reference.
- [x] Record image, container ID, health, restart and OOM state for Sub2API, PostgreSQL, Redis, Caddy, relay-ops and D04.
- [x] Confirm `D04_MODE=read_only` and `D04_REGISTRATION_OPEN=false` using selected non-secret environment fields only.
- [x] Confirm relay-ops remains `read_only + dry_run`.
- [x] Regenerate a redacted canonical snapshot from the current Sub2API Admin API and compare it with the saved baseline; do not rely only on an older evidence file.
- [x] Verify `/healthz`, `/readyz`, `/pricing`, `/ops` and `/monitor` return HTTP 200.
- [x] Run the current D04, infrastructure and benchmark regression gates.

Stage 0 evidence at `2026-07-21T15:34:11Z` / `2026-07-21T23:34:11+0800`: all six containers were running; Sub2API, PostgreSQL, Redis, relay-ops and D04 were healthy, and Caddy had no configured Docker healthcheck. Restart/OOM values were `Sub2API=1/false`, all other containers `0/false`. The fresh allowlisted Admin API canonical hash and the normalized saved baseline were both `b2a2a6ce01bc6135e996eacba4e3739052bb2a70720439782e6d4b96bc3aaf82`; the Feishu routing-file hash remained `3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e`. No production write or model request occurred.

Stop if any service is unhealthy, the canonical route differs without explanation, a secret boundary is unavailable, or a rollback artifact cannot be created.

## Stage 1: D04 Single-user Write Acceptance

**Authorization required:** Explicit approval to create one real isolated Sub2API user and perform one USD 20 site-balance grant. This approval does not authorize any model request or XM action.

### Approved bounded configuration

```text
D04_MODE=write
D04_REGISTRATION_OPEN=true
D04_MAX_USERS=15
D04_DAILY_LOGIN_CREDIT_USD=20
D04_TOTAL_BUDGET_USD=2.00
D04_BUDGET_COST_BPS=1000
D04_COST_POLICY_QUALIFIED=true
D04_COST_POLICY_ID=d04-acceptance-conservative-1000bps-20260721
```

The USD 20 site balance occupies the full USD 2.00 acceptance cost ceiling at 1000 basis points. This is an acceptance budget, not the later 15-user launch budget.

### Execution

The approved values are encoded in the secret-free `infra/compose.d04-acceptance.yaml` overlay. It must be combined with `infra/compose.d04-read-only.yaml`; never edit the production environment ad hoc. Merely rendering this overlay does not authorize applying it.

- [ ] Save the active D04 Compose override, selected non-secret environment values, D04 container ID and D04 data-volume backup/rollback reference.
- [ ] Recreate only `internal-test-service`; do not recreate Sub2API, PostgreSQL, Redis, Caddy or relay-ops.
- [ ] Confirm D04 is healthy and public settings show registration enabled only after all native/D04/cap/budget gates agree.
- [ ] The user registers one operator-controlled isolated account through the native browser page. Password, Cookie and token stay in the browser and never enter evidence.
- [ ] Confirm registration returns the native authenticated response and one USD 20 balance effect is visible through supported application/Admin APIs.
- [ ] Confirm D04 records one `daily_login` grant with a Shanghai-date idempotency key and no invitation/referral/check-in grant.
- [ ] Log out and log in once on the same Shanghai day; confirm the provider balance, provider history and D04 ledger do not receive a second grant.
- [ ] Confirm effective public registration becomes closed after the acceptance budget is fully occupied, even though the configured switch remains open and the user count is below 15.
- [ ] Reconcile three independent views: D04 grant ledger, Sub2API balance history and current user balance. Record only IDs/hashes, amounts, timestamps and statuses.
- [ ] Confirm no model request was sent and the route canonical snapshot did not change.

### Mandatory restore

- [ ] Restore `D04_MODE=read_only` and `D04_REGISTRATION_OPEN=false` immediately.
- [ ] Recreate only `internal-test-service`, confirm healthy/restart/OOM state and confirm public registration is closed.
- [ ] Preserve the isolated user only when needed for the next Shanghai-day proof. Any deletion must use a separately verified application/Admin API and must not directly edit PostgreSQL or SQLite.
- [ ] Update D04 verification, `current-state.md` and `llm-handoff.md` with the actual result and residual user/ledger state.

Stop and restore immediately on an unexpected second balance effect, an uncertain provider write, reconciliation drift, mode/config mismatch, service health failure, or route hash change.

## Stage 2: Generic Upstream Directory-only Discovery (Current Instances: XM Plus/Pro)

**Authorization:** Approved for the current `xm-plus` and `xm-pro` registry instances: create two temporary supplier Keys, install them through an isolated secret boundary, send exactly one authenticated `GET /models` per instance, and disable/delete both Keys after the agreed workflow. This approval does not authorize generation and does not add XM-specific behavior to the shared benchmark.

### Temporary Key controls

- [x] Separate Key for Plus and Pro; never reuse the existing supplier Keys.
- [x] Keys were temporary and isolated to their respective supplier groups.
- [x] Key values were not written to chat, repository files, command arguments, reports or ledger records.
- [x] Both temporary Keys were deleted after directory discovery; final supplier page shows `0` Keys.

### Dry-run and live directory calls

- [x] Run `ruby ops/upstream-benchmark.rb validate` and V2 validation before any authenticated call.
- [x] Run `discover --dry-run` for `xm-plus` and `xm-pro`; each must show one planned directory request, zero generation, `requests_sent=0` and `network_sent=false`.
- [x] Run `discover` once for Plus and once for Pro from the approved network boundary.
- [x] Confirm each ledger record is `partial / live_direct / discovered_not_qualified`, has `request_count=1`, `generation_request_count=0`, a sorted classified directory and no errors.
- [x] Confirm no generation request was sent; directory discovery produced no generation usage in the benchmark evidence.
- [x] Confirm the benchmark ledger and reports contain no Key, authorization header, Cookie or provider response body.
- [x] Record `M_plus=15` and `M_pro=10`; the groups expose different directories.

Stop after the two directory calls. Do not run `run`, `watch`, sync, SSE or capacity tests under this authorization.

Pre-authorization dry-run evidence: both channel plans used `mvp-text-responses-v2`, `/models`, one directory request, zero generation, `requests_sent=0`, and `network_sent=false`. No supplier credential was created or read.

## Stage 3: Generic Upstream Qualification Budget (Current Candidate Topology: XM + Wawazz)

**Authorization required:** A new approval bound to the actual directory records and exact HTTP/generation/Token/currency ceilings.

For each channel with text-model count `M`:

```text
maximum HTTP requests = 2M + 71 + K
maximum generation requests = 2M + 70 + K
per-model generation = 1 sync + 1 SSE
sync capacity generation = 1+2+3+5+8+10
SSE capacity generation = 1+2+3+5+8+10
RPM capacity generation = 1+2+4+5
K = separately approved topology-verification requests for that channel/role
maximum output = 8 Token per request
```

These are v3 upper bounds for one channel/role. Calculate each XM primary role and each Wawazz backup role independently, then add separately approved shared-pool and drill workloads. Never treat `2M+71+K` as the total budget for the complete topology.

- [x] Calculate Plus and Pro ceilings independently: `K=0` gives Plus `101/100`, Pro `91/90`; example `K=4` gives Plus `105/104`, Pro `95/94` HTTP/generation.
- [ ] Add supplier pricing/terms evidence and a currency ceiling before requesting approval.
- [ ] Run direct sync/SSE and bounded capacity; stop on 429, 5xx, timeout, protocol error, incomplete SSE, cost threshold or unexplained billing.
- [ ] Run isolated Sub2API gateway sync/SSE with no production users bound.
- [ ] Reconcile API usage, Sub2API debit and supplier standard/actual charge.
- [ ] Collect production-host latency, TTFT and total-duration evidence; keep short tests separate from sustained observation.
- [ ] Record resale, refund, price-change, capacity and support terms as verified or unknown.
- [ ] Disable/delete temporary downstream and supplier credentials and verify cleanup.

Synchronous capacity does not prove SSE concurrency capacity. Any missing mandatory gate remains `unknown` or blocks production qualification.

## Stage 4: Proposal and Topology Change

**Authorization required:** The user must explicitly reply `采纳` with the exact proposal ID/hash. Benchmark success alone is not authorization.

- [ ] Generate a secret-free proposal containing verified model mappings/prices, requested billing source, account cost multipliers, concurrency/RPM recommendations and rollback mapping.
- [ ] Proposal explicitly sets XM Plus as `GPT-Plus` primary and Wawazz as backup.
- [ ] Proposal explicitly sets XM Pro as `GPT-Pro` primary and Wawazz as backup.
- [ ] Proposal states the resulting Aliu and Neko roles; neither may remain silently schedulable through an unintended path.
- [ ] Save a fresh production snapshot and canonical route hash immediately before apply.
- [ ] Apply the proposal first to isolated Sub2API objects and verify sync/SSE/billing.
- [ ] After exact ID/hash adoption, apply only the approved production fields.
- [ ] Verify model restrictions, requested billing, user debit, account cost, health, fallback and recovery independently for Plus and Pro.
- [ ] Confirm Wawazz backup behavior without manufacturing an uncontrolled production failure.
- [ ] Confirm relay-ops remains `read_only + dry_run` and D04 remains in its approved mode.
- [ ] Save post-apply canonical snapshot and compare all unrelated route/account/model fields.

On failed apply, restore the pre-apply snapshot when safe. Stop and alert if restoration fails or the route enters `mixed`, `none` or `partial` state.

## Completion Evidence

The overall objective is complete only when:

- [ ] D04 production write acceptance proves registration, immediate first-day USD 20 credit, same-day idempotency, budget fail-closed behavior and three-way reconciliation.
- [ ] XM Plus and XM Pro satisfy direct, gateway, billing, network and terms gates or every accepted unknown is explicitly documented.
- [ ] A named proposal is explicitly adopted and the production topology matches the target.
- [ ] Wawazz is verified as backup for both groups, and unintended legacy routes are not schedulable.
- [ ] Functional and non-functional reports, `current-state.md` and `llm-handoff.md` match production reality.
- [ ] Temporary credentials and isolated qualification objects are cleaned up or explicitly retained with owner and expiry.
