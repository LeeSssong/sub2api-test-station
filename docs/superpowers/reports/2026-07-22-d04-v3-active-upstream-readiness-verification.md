# D04 v3 Active Upstream Discovery Verification

**Date:** 2026-07-22 (Asia/Shanghai)  
**Result:** `PRODUCTION PASS / LAUNCH NO-GO`  
**Modes required:** D04 `read_only` with registration closed; relay-ops `read_only + dry_run`

## Decision

D04 launch readiness must no longer use a manually populated single upstream. The active set is now defined only by the Sub2API Admin account listing:

```text
status == "active" && schedulable == true
```

Every discovered account has its own balance, freshness, runtime, and natural account-attributed quality result. A passing account cannot satisfy a failing account's gate. No provider name, hostname, public-group name, or fixed account ID participates in evaluator membership.

## Local Evidence

| Check | Result |
|---|---|
| Paginated Sub2API Admin GET account reader | PASS |
| Duplicate/missing/incomplete pagination rejection | PASS |
| Active membership sorting and canonical account-set hash | PASS |
| Sidecar evidence cannot add an undiscovered account | PASS |
| Per-account low-balance, missing evidence, runtime, empty-set, and hash-drift blocks | PASS |
| V2 historical evaluator regression | PASS |
| Authenticated read-only `/ops` status section | PASS |
| Anonymous bootstrap isolation and no readiness control | PASS |
| Relay-ops full Go race suite | PASS |
| Go vet, Ruby v2/v3 tests, JS syntax, Compose/infra contracts | PASS |
| AMD64 relay-ops image build | PASS |

The local evaluator smoke artifact deliberately returned `no_go` because it used a fictional unapproved example with a placeholder account-set hash. It did not contact an external system or execute an action.

## Safety Boundary

- The collector calls only `GET /api/v1/admin/accounts`.
- Result files are strict JSON, maximum 2 MiB, secret-shaped fields are rejected, and snapshots use `0600` atomic replacement.
- Relay-ops receives only the evaluator result as a read-only mount. Missing, invalid, or stale results render as `NO-GO`.
- This increment does not mutate routes, account scheduling, balances, Keys, candidates, probes, Feishu state, D04 mode, registration, or databases.
- No provider has a special policy branch and no provider-specific balance mutation was added. Any Sub2API account configured active and schedulable is included; missing evidence blocks that account.

## Sub2API Reuse Audit

- **Direct reuse:** `GET /api/v1/admin/accounts` owns active membership; `GET /api/v1/admin/usage?account_id=...` and `GET /api/v1/admin/ops/requests?account_id=...` own passive account-attributed request quality.
- **Adapted reuse:** relay-ops joins those existing read-only fields with server-local, account-ID-keyed balance evidence and presents one D04 readiness projection.
- **New only where absent:** the versioned D04 launch policy/evaluator and read-only result artifact are local because Sub2API does not implement this product-specific opening decision.

No user, authentication, scheduling, usage, request-history, or route-management subsystem was duplicated.

## Production Status

The production collector discovered exactly two Sub2API accounts with `status=active && schedulable=true`: account IDs `7` and `8`. The canonical account-set SHA-256 is:

```text
073db0a178c2846e4b7613d0abe687d7af90c59d8c5e4595fac4bd337471338b
```

The secret-free snapshot `D04-LIGHTWEIGHT-LAUNCH-v3-20260722T064415Z` evaluated to `no_go`. Both accounts independently returned:

```text
upstream_balance_unknown
upstream_samples_insufficient
```

Neither account had a trusted passive balance record, and both had zero account-attributed natural requests in the quality window. The collection did not manufacture model traffic. The result records `real_action_executed=false` and `external_system_contacted=false`.

The production host built AMD64 image `sub2api-relay-ops:d04-active-upstream-v3-ui2-20260722` with manifest `sha256:74da15d949cf43d320acec4d0868a14dfcd28b600a11b8b4a39c0fe137e8ec14`. Only relay-ops was recreated. It is healthy with restart count `0`; D04 remains `read_only/registration=false`, and relay-ops remains `read_only + dry_run`.

Authenticated `/ops` showed the same snapshot, account IDs, set hash, and per-account decision. The D04 section contains zero buttons/forms/inputs. Its desktop table measured `clientWidth=1190` and `scrollWidth=1190`, so the previous blocker-column overflow is resolved. Operators see Chinese summaries (`余额证据缺失；自然样本不足`) while the exact reason codes remain in the title attribute. After the 20-minute window elapsed, the page correctly added `门禁结果已过期` and remained `NO-GO`.

Pre/post invariants remained unchanged:

```text
Feishu routing SHA-256: 3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e
Sub2API accounts / usage_logs / users: 9 / 27 / 17
relay-ops upstreams / probes / deliveries / quality reports: 2 / 0 / 4 / 0
other five container IDs: unchanged
/healthz /readyz /ops /monitor /pricing: HTTP 200
```

No route, scheduling state, multiplier, price, balance, Key, candidate, probe, Feishu row, D04 data, or Sub2API business row was changed. A missing balance or natural sample remains a real blocker; it is not replaced with group aggregates or a provider-specific exception.

## Next Mainline

The v3 read-only acceptance is complete. D04 remains closed until every dynamically discovered active account has trusted fresh minimum-balance evidence and enough fresh natural account-attributed samples in one snapshot, producing `GO`. The Feishu Agent/card mainline remains closed except its existing non-blocking natural-event visual observation.
