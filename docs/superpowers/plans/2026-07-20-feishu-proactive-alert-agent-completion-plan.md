# Feishu Proactive Alert Agent Completion Plan

> Execute test-first and keep production `read_only + dry_run` throughout.

## Source

- Spec: `docs/superpowers/specs/2026-07-19-relay-ops-monitoring-and-alert-agent-design.md`
- Existing implementation plan: `docs/superpowers/plans/2026-07-19-relay-ops-monitoring-and-alert-agent-implementation-plan.md`

## Goal

Complete the remaining production notification path: native Sub2API Channel Monitor failures and recoveries, one daily operations report, and redacted read-only Agent analysis delivered through the existing Feishu App Bot.

## Constraints

- Do not call an upstream during acceptance and do not create probe cost.
- Do not change Sub2API routes, groups, pricing, balances, keys, or PostgreSQL directly.
- Keep `RELAY_OPS_MODE=read_only` and Feishu command mode `dry_run`.
- Reuse the existing Feishu App credentials and alert chat; do not install OpenClaw or another Agent framework.
- Production deployment may rebuild only `relay-ops`.

## Tasks

- [x] 1. Add native monitor alert tests for two-window P1 confirmation, duplicate suppression, new evidence, and one recovery notification.
- [x] 2. Implement native monitor event processing and wire it into the existing Sub2API synchronization cycle.
- [x] 3. Add daily report tests covering every public group, 24-hour SLA/errors/TTFT P95, usage cost, candidates, active incidents, and deterministic fallback.
- [x] 4. Implement the daily report service, scheduler assembly, and a fixed zero-cost report acceptance action.
- [x] 5. Run focused tests, `go test ./... -race -count=1`, `go vet ./...`, frontend syntax, Compose/Caddy contracts, and secret/diff checks.
- [x] 6. Build on AMD64, recreate only `relay-ops`, and verify App Bot alert, duplicate suppression, recovery, report, Agent or explicit fallback, zero upstream access, and unchanged route snapshot.
- [x] 7. Update the runbook, current state, handoff, and a production verification report.

## Acceptance

- [x] A native monitor error does not notify on the first P1 window, notifies once on confirmation, suppresses identical repeats, and notifies once on recovery.
- [x] The daily report is scheduled once per Shanghai day and cannot be duplicated for the same date.
- [x] Agent input is versioned, structured, and redacted; failure or missing configuration uses deterministic fallback without blocking Feishu.
- [x] Synthetic acceptance proves alert, suppression, recovery, and report without calling an upstream.
- [x] Production evidence proves only `relay-ops` changed and route/configuration snapshots stayed unchanged.
