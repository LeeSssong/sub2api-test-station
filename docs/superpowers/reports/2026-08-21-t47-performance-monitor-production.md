# T47 Performance Monitor Visual Redesign — Production Report

## Release identity

- Root source: `main@0aabd76f93a1c334f05a4a447a49a7ff039968ff`
- Tested tree: `b0bfefbdc10fe4cd675739dc40aa704a31d346d0`
- Candidate merge: `b91a199aa`
- Evidence: `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-21-main-0aabd76f9-t47-performance-monitor.json`
- Host record: `/var/lib/sub2api/release-records/20260821T011717Z-production-1834048.json`
- Result: `succeeded`, `promoted`, `rolled_back=false`
- `downtime_required=false`

## Scope delivered

The production image contains the frontend-only T47 visual redesign: dedicated performance-monitor sidebar icon, compact service-line cards, near-black-blue shell, dense vertical timeline bars, degraded high-latency coloring, and preserved tooltip/focus/ARIA behavior. Native Monitor V2 data, refresh, windows, and optimistic snapshot behavior were unchanged. No API, database, migration, configuration, or production-data changes were made.

## Verification

- Focused Vitest: 5 files, 24 tests passed.
- `pnpm typecheck`: passed.
- `pnpm build`: passed; 1058 modules transformed.
- `git diff --check`: passed.
- Production `/healthz`: HTTP 200, `{"status":"alive"}`.
- Production `/readyz`: HTTP 200, `{"status":"ready"}`.
- Production `/health`: HTTP 200, `{"status":"ok"}`.
- Production `/custom/performance-monitor`: HTTP 200.
- Production `/monitor`: HTTP 200.

## Rollback

Use the retained previous blue-green slot and the host release rollback procedure. No migration rollback is required.
