# Three Mainlines Closure Verification

**Date:** 2026-07-22 (Asia/Shanghai)  
**Result:** `PASS` for the requested three work lines; D04 opening remains correctly gated

## Closure Matrix

| Mainline | Completed evidence | Remaining work | Status |
|---|---|---|---|
| Quality-first upstream evaluation | bounded runner, hard gates, scheduler cadence, Feishu quality card, `/ops` dry-run preview, live `73/74/75` evidence and cleanup | pricing/billing/terms evidence for `75`; no account is switch-eligible | closed for this evaluation cycle |
| D04 launch preparation | public-registration policy, 15-user atomic cap tests, one real user, one USD 20 grant, same-day idempotency, three-way reconciliation, provider-neutral v2 evaluator/overlay/checklist, server-local account backup, read-only rollback | satisfy the current active-upstream minimum-balance and fresh natural-quality gates in the same approved snapshot | preparation complete; opening blocked |
| Feishu operations | alert/recovery semantics, daily report, Interactive Cards, command cards, deduplication, App Bot delivery, deterministic read-only analysis, and deployed quality-report notification path | natural alert/recovery visual observation only | production mainline closed |

## Upstream Decision

- Account `73`: blocked by `gpt-5.6` sync/SSE failure and incomplete SSE.
- Account `74`: blocked by six models failing sync/SSE.
- Account `75`: `18/18` catalog and `129/129` capacity requests passed; observed lower bounds are concurrency `10` and RPM target `30`, with conservative recommendation `8/24`.
- All three gateway smoke requests passed, but account `75` still lacks verified pricing, billing, balance, and commercial terms.
- No candidate may be switched or exposed to users. Production routing remains unchanged.

See `2026-07-22-local-sub-account-quality-first-verification.md` for run IDs, model failures, billing anomalies, and cleanup hash.

## D04 Decision

The one-user production acceptance succeeded with one `daily_login_credit` grant of USD 20, one matching provider balance-history entry, a USD 20 current balance, and no provider or D04 usage. A repeated same-day login created no second effect. D04 was restored to `read_only` with registration closed.

The settings reset incident encountered during Admin API discovery was fully recovered through the official API. The final settings hash is `52eff24fce0338ee4f8f81ad12a5d1406c46b6de050c99587035cdfd1f71a28e`. See `2026-07-22-d04-single-user-low-budget-acceptance.md` for the incident and reconciliation details.

The lightweight readiness evaluator, launch-only overlay, operator checklist, read-only rollback, and server-local account backup are complete. The user's single opening approval is recorded in the current snapshot, but the latest decision remains `NO-GO` for these exact provider-neutral reasons:

```text
upstream_balance_below_minimum
upstream_samples_insufficient
```

The latest fresh snapshot records an active-upstream balance of `-$0.01` and zero natural samples in the recent quality window. This is a correct gated result: D04 remains `read_only`, registration remains closed, and the launch overlay has not been applied. The lightweight gate does not calculate balance runway or require off-site backup.

## Feishu Decision

The Feishu mainline remains functionally closed. Alert/recovery templates and transport are automated and share the verified Interactive Card delivery path. The next real alert/recovery card screenshot is a non-blocking natural-event observation. No deduplication row should be deleted and no synthetic incident should be created for visual evidence.

Code review found that the newly added quality-report card template was initially not connected to fast-run execution, that no incident row existed for the delivery foreign key, and that real PostgreSQL would not retry a `failed` notification even though the unit fake did. All three gaps were fixed with RED/GREEN tests: fast results now persist, create/update a stable incident state, and send through the existing deduplicating transport; equivalent runs share semantic notification evidence; `failed` rows may be re-reserved in place; and `delivered/reserved` rows remain suppressed.

After launch-readiness preparation, the increment was deployed as `sub2api-relay-ops:quality-report-read-only-20260722-v1` (`sha256:b7977f9cb850d020dba66443a920c186772649edecd12d80023825552dd84b8e`). Startup added only the empty `relay_ops.quality_reports` table. Production acceptance found `0` candidates, `0` probe runs, `0` quality reports, the same `3` notification rows, and only the existing `daily-report` and `production-collection` scheduler keys. No message or synthetic event was generated.

## Preserved Production Boundaries

```text
RELAY_OPS_MODE=read_only
RELAY_OPS_FEISHU_COMMAND_MODE=dry_run
D04_MODE=read_only
D04_REGISTRATION_OPEN=false
```

No production route, multiplier, price, balance outside the one approved D04 grant, Key, candidate, paid probe, enabled mode, or Feishu deduplication state was changed. Neko balance was not processed.

## Fresh Closure Verification

The final read-only production recheck confirmed:

- D04 and relay-ops were healthy with restart count `0` and OOM false;
- Sub2API, PostgreSQL, and Redis were healthy; Caddy served all five public endpoints;
- `D04_MODE=read_only`, `D04_REGISTRATION_OPEN=false`, `RELAY_OPS_MODE=read_only`, and `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`;
- `/healthz`, `/readyz`, `/pricing`, `/ops`, and `/monitor` returned HTTP `200`;
- public registration, invitation, and affiliate flags were false, and same-origin registration returned `403 D04_REGISTRATION_CLOSED`;
- D04 still had one roster row, one successful USD 20 grant, one distinct idempotency key, and zero usage records;
- provider user `17` still had USD 20 current balance, one matching USD 20 idempotent balance-history entry, and zero usage;
- the 264-setting canonical hash remained `52eff24fce0338ee4f8f81ad12a5d1406c46b6de050c99587035cdfd1f71a28e`.
- production relay-ops still had `0` candidates, `0` probe runs, and `3` historical notification rows; the Feishu routing file SHA-256 remained `3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e`.
- the deployed quality-report image was healthy with restart count `0`, OOM false, `quality_reports=0`, and no `candidate-fast:*` scheduler job; all protected container IDs matched pre-state.
- the server-local backup directory and its four archive/checksum files remained `0700` and `0600` respectively.

Fresh local verification passed:

```text
D04 readiness evaluator: 11 runs / 91 assertions
upstream V2: 42 runs / 250 assertions
protocols: 10 runs / 44 assertions
nonfunctional: 32 runs / 160 assertions
upstream registry/profile validation
relay-ops go test ./... -p 1 -race -count=1 with a fresh temporary PostgreSQL
relay-ops go vet ./...
internal-test-service go test ./... -p 1 -race -count=1
internal-test-service go vet ./...
ops.js and ops-admin.js syntax checks
relay-ops, D04, and infrastructure deployment contracts
git diff --check
```

The temporary final-verification PostgreSQL container/network and all temporary Go cache volumes were removed. The unrelated existing `relay-ops-task3-postgres` object was not touched.

## Next Mainline

The requested order is complete: upstream quality evaluation, D04 opening preparation, then monitoring robot/Feishu quality-report deployment. The next mainline is therefore not more Feishu development. It is to refresh the current active-upstream balance and natural quality window, then rerun the same v2 evaluator. Require a hash-bound `decision=go` before applying the launch overlay; do not calculate balance runway, create candidates, enable probes, or change production routing as part of this gate.

The one-user `$20` acceptance must not be repeated merely to reconfirm the write path. Account `75` may separately receive a commercial-evidence review only after price, provider debit, balance, and resale terms are available; no production candidate, probe, or route change follows automatically.
