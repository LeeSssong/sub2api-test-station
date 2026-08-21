# T47-R1 Operational Timeline Green Handoff

## Status

`READY_FOR_ROOT_REVIEW`

## Root cause and correction

T47 added a frontend-only override that rendered `operational` timeline buckets amber when `latency_ms >= 2000`, even though the native Monitor V2 status remained runnable. T47-R1 removes that derived degraded state so timeline colors and tooltip headings follow the native status contract:

- `operational`: green, `UP`
- `unavailable` with a probe result: red, `DOWN`
- `unavailable` without a probe result: gray no-data bucket, `NO DATA`

## Scope

- Changed `MonitorV2Timeline.vue` and its direct test only.
- No API, backend, data source, migration, configuration, or production-data changes.

## Verification

- TDD RED reproduced the amber high-latency operational bucket.
- Focused Vitest: 3 files, 20 tests passed.
- `pnpm typecheck`: passed.
- `pnpm build`: passed; 1058 modules transformed.
- `git diff --check`: passed.

## Release

- Expected `downtime_required=false`; root preflight is authoritative.
- Rollback uses the retained previous blue-green slot; no data rollback is required.
