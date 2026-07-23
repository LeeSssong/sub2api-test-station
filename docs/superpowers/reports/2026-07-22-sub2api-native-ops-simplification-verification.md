# Sub2API Native Ops Simplification Verification

**Date:** 2026-07-22 (Asia/Shanghai)  
**Result:** `PRODUCTION PASS / CONTROLLED TESTING NO-GO`  
**Production modes:** relay-ops `read_only + dry_run`; D04 `read_only`, registration closed

## Decision

The operations UI and administrator-access increment is complete. `/ops` is now a read-only projection over Sub2API's native account, group, scheduling, and administrator identity capabilities. It no longer maintains a second browser control plane for upstream credentials or candidates.

This does not authorize controlled user testing. The current Sub2API account set has not produced one fresh readiness snapshot in which every active account passes the generic minimum-balance and quality gates, so D04 remains closed.

## Sub2API Reuse

| Capability | Decision |
|---|---|
| Accounts, Base URL, Key, scheduling, groups | Directly reuse Sub2API native administration |
| Current upstream membership | Adapt the native Admin account list; include only undeleted `status=active && schedulable=true` accounts |
| Administrator identity | Directly reuse the existing Sub2API browser token and `/api/v1/auth/me` verification |
| Registration | Keep Sub2API native registration plus the existing D04 server-side safety arm |
| Readiness display | New read-only projection because Sub2API has no product-specific controlled-launch decision |

Candidate intake, production-source intake, billing-session/Cookie/Bearer configuration, synthetic acceptance controls, quality-preview mutation, and Base URL/Key entry were removed from `/ops`. Historical relay-ops records remain intact but do not define current upstream membership.

## Production UI And Authentication

An authenticated Sub2API administrator saw exactly the current accounts:

```text
10 | XM PLUS | GPT-Plus
11 | XM PRO  | GPT-Pro
12 | shen    | GPT-Plus
```

The canonical account-set SHA-256 is:

```text
907ccb7121a3362f294855a1d70336cac869cfb4000f59b966b68cfda4284dd4
```

The page displayed `内测开放状态`, `暂不可开放`, `当前活动上游`, and the collapsed technical details. It contained zero forms, inputs, selects, textareas, or buttons. Desktop `scrollWidth` equaled `clientWidth`; the narrow viewport also had no document-level horizontal overflow or section overlap. The authenticated refresh timestamp advanced from `14:24 UTC` to `14:25 UTC` without changing the account set.

The public `/ops` response was a 477-byte data-free bootstrap and contained no account names, readiness hash, or operational status. Anonymous and invalid-token calls to `/relay-ops/api/ops-view` returned HTTP 404. Retired candidate, upstream, acceptance, billing-session, and quality-preview POST routes also returned 404.

## Production State

Only relay-ops was recreated. The running image is:

```text
sub2api-relay-ops:sub2api-native-ops-20260722-v1
manifest: sha256:0faf838a414dd53df65f6f985579828a290a0c5977c1c47c4af42c6b1dd3c687
container: afa5aa814ddb...
health: healthy
restart count: 0
```

Sub2API, PostgreSQL, Redis, Caddy, and D04 were not recreated. D04 remained `read_only` with `D04_REGISTRATION_OPEN=false`. All public checks returned HTTP 200:

```text
/healthz /readyz /ops /monitor /pricing
```

Fresh read-only counts remained:

```text
Sub2API accounts / usage_logs / users: 12 / 27 / 17
relay-ops historical upstreams / probes / deliveries / quality reports: 2 / 0 / 4 / 0
```

The apparent additional active rows `1/4/5/6/7/8` are soft-deleted history. Sub2API's native Admin API correctly excludes them; direct database status alone must not be used to infer current upstreams.

Configuration evidence remained unchanged:

```text
Caddy SHA-256: 668b274207f7265affa03f4ecc22725db34b30e9d9ae0cc1b7d39b483250b292
Feishu routing SHA-256: 3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e
```

No route, account, scheduling state, price, multiplier, balance, Key, candidate, probe, user, database row, Feishu event, or deduplication record was changed during post-deployment acceptance.

## Launch Readiness

The current set is `NO-GO`:

| Account | Existing qualification evidence | Blocking conclusion |
|---|---|---|
| `10` | Direct catalog `26/30` | Intermittent failures; no fresh trusted balance/readiness evidence |
| `11` | Direct catalog `13/30` | Persistent HTTP 502 across eight models; no fresh trusted balance/readiness evidence |
| `12` | No fresh qualification result | Balance and quality evidence unavailable |

The fresh Sub2API native `usage_logs` query returned no rows for accounts `10/11/12`, so there are currently zero natural account-attributed production samples for the v3 quality window. The generic minimum provider balance is USD 5.00. Unknown balance cannot pass. The UI correctly renders `等待新门禁检查` and does not reuse the old `7/8` readiness result because its account-set hash differs.

Controlled testing may open only after a fresh v3 snapshot for the same live account set returns `GO`, followed by the existing D04 server-side arm and Sub2API native registration setting. This increment intentionally adds no second web toggle.

## Verification

Fresh implementation verification passed before deployment:

```text
go test ./... -p 1 -count=1
go test ./... -p 1 -race -count=1
go vet ./...
ruby v3/v2 readiness and benchmark regression suites
node --check internal/http/static/ops.js
node --check internal/http/static/ops-admin.js
tests/relay_ops/validate_relay_ops_contract.sh
tests/internal_test/validate_internal_test_contract.sh
tests/infra/validate-baseline.sh
git diff --check
```

The next mainline is to obtain fresh provider-neutral balance and natural quality evidence for all current Sub2API-scheduled accounts, then perform D04 controlled-launch acceptance. It is not to rebuild upstream management or extend Feishu functionality.
