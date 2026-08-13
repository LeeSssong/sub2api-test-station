# Monitor P95 Hotfix Design

## Problem and evidence

The channel monitor cards request `OpsQueryModeAuto`. For multi-hour windows, that path merges hourly rollups by taking `MAX` of hourly P95/P99 values. Production read-only SQL proved the displayed 7-day values exactly equal the maximum hourly P95, while exact window-level `percentile_cont(0.95)` values are materially lower.

## Approved scope

- Keep T03-R1 active in its isolated worktree.
- Change only channel monitor card metric reads from automatic pre-aggregation to the existing raw query mode.
- Preserve the shared Ops dashboard and its pre-aggregation behavior.
- Add one focused service contract regression proving the card requests raw metrics.
- No schema migration, data backfill, configuration change, or GitHub Actions workflow.

## Data flow

`MonitorV2Service` constructs the group-scoped `OpsDashboardFilter` with `OpsQueryModeRaw`. The existing Ops repository then calculates P50/P95 over all raw `usage_logs` samples in the selected window. The response and frontend millisecond-to-second formatting remain unchanged.

## Failure and performance semantics

Existing monitor behavior remains: if the Ops read fails, the card keeps its unavailable/insufficient-data metric state. Production measurements for the three visible groups were approximately 70 ms, 38 ms, and 2 ms for exact 7-day percentile queries, so no cache or new persistence is introduced.

## Validation and release

- RED/GREEN focused unit test for the query mode contract.
- Focused backend package test and `git diff --check` only.
- Independent task review and final branch review.
- Merge to current `main`, push, and use the reviewed blue-green production chain.
- `downtime_required=false`; rollback is the previous application image.
- Online acceptance compares the monitor API P95 values with production raw SQL and verifies health endpoints.

## Approval

The user approved this exact minimal hotfix and immediate production deployment on 2026-08-13, explicitly requesting removal of unrelated validation scope.
