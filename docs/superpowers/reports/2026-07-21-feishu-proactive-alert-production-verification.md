# Verification: Feishu Proactive Alert Production Loop

**Date:** 2026-07-21 (Asia/Shanghai)  
**Result:** `PASS`  
**Production modes:** `RELAY_OPS_MODE=read_only`, `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`

## Scope

This verification covers the remaining proactive notification path through the existing Feishu App Bot:

- Sub2API native Channel Monitor failure and recovery events.
- P1 two-window confirmation and duplicate suppression.
- One daily operations report for every customer-visible group.
- Versioned, redacted, read-only Agent analysis with deterministic fallback.
- Real Feishu group delivery for alert, recovery and daily report.

It does not enable route writes, paid candidate probes, balance changes, pricing changes or production account changes.

## Implementation Evidence

- `internal/nativealerts` converts the latest native monitor sample into a stable incident, confirms P1 failures across two windows, suppresses identical repeats and emits one recovery notification.
- `internal/dailyreport` aggregates 24-hour SLA, errors, TTFT P95, total-latency P95, user charge, upstream cost, candidate count and incident count for all customer-visible groups.
- Agent input uses `relay-ops-incident-v1`, excludes secrets and user content, and allows only `observe` or `request_human_review`. Missing or failed model configuration uses deterministic fallback without blocking Feishu.
- Daily report notification first registers `daily-report:YYYY-MM-DD` in the incident state machine. This fixes the production acceptance failure `incident not found for notification` and preserves one delivery per Shanghai date even when report content changes.

## Local Verification

Fresh verification used the Go 1.24.13 containerized toolchain:

```text
go test ./... -race -count=1                  PASS
go vet ./...                                  PASS
node --check ops.js                           PASS
node --check ops-admin.js                     PASS
bash tests/relay_ops/validate_relay_ops_contract.sh  PASS
git diff --check                              PASS
```

The regression test first reproduced the missing daily-report incident record, then passed after the report service registered the stable daily incident before delivery. It also verifies same-date deduplication when report content changes.

## Production Deployment

- Image: `sub2api-relay-ops:feishu-proactive-alert-20260721-v3`
- AMD64 image ID: `sha256:48474bc66f93af15a92d54d4ceec984677f071274b01b8ca43d4f70eb8dfaa4d`
- Final relay-ops container: `866dbae4eb752d27c72d27cb6d96d2198fb40365d7990f0cb7509a3b4576c75c`
- Health: `healthy`; restart count: `0`
- Unchanged base containers:
  - Sub2API: `5fd8adccdb9e`
  - PostgreSQL: `2db52788ad73`
  - Redis: `c45202c0d9e6`
  - Caddy: `7c28088cd9fe`

Only `relay-ops` was rebuilt and recreated. Public `/healthz`, `/readyz`, `/pricing`, `/ops` and `/monitor` returned HTTP `200`; the unauthenticated daily-report acceptance endpoint returned `401` as required.

## Real Feishu Acceptance

The authenticated `/ops` fixed acceptance controls were used without reading browser cookies, Local Storage, tokens or application secrets.

Alert cycle result:

```text
Upstream: not accessed
Agent: deterministic fallback
Feishu: acceptance complete
Alert: delivered
Duplicate: suppressed
Recovery: delivered
```

Daily report result:

```text
2026-07-21
3 customer-visible groups
Agent: fallback
Feishu: delivered
```

The group count is discovered from Sub2API customer-visible groups and is not hard-coded to GPT-Pro/GPT-Plus.

Database evidence after repeated acceptance:

```text
daily-report:2026-07-21                 1 delivery
synthetic:relay-ops:acceptance:v1       2 deliveries
production-collection                   passed
```

The two synthetic deliveries are one alert and one recovery. Repeating either acceptance action did not create additional delivery rows. The scheduler callback is wired to the same daily-report service; no scheduled daily row was expected before the Shanghai 09:00 threshold.

## Zero-Write Evidence

The normalized route/account/model snapshot hash was identical before deployment, after deployment and after final acceptance:

```text
0346b79d19cffdca58898e6db6490d62df89b1f0d889cc9fbaa22946b1163433
```

The Feishu routing file hash also remained unchanged:

```text
3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e
```

Restricted production evidence remains at `/opt/sub2api/production/evidence/feishu-proactive-alert-20260721/`.

## Remaining Boundaries

- Production stays `read_only + dry_run`; `enabled` still requires separate approval naming the target group.
- No real Agent model credential is installed. Deterministic read-only fallback is the accepted behavior and does not block alerts or reports.
- Neko's latest balance/monitor state and the high-load Wawazz `test` Key remain separate upstream-readiness issues, not failures of this notification loop.

