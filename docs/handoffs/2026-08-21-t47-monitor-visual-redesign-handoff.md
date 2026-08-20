# T47 Monitor Visual Redesign Handoff

## Status

`READY_FOR_ROOT_REVIEW`

## Scope

Frontend-only visual redesign of `/custom/performance-monitor` using the existing native Monitor V2 data and refresh contract. Added a dedicated sidebar pulse-monitor icon, compact service-line cards, dark near-black-blue shell, dense vertical timeline bars, degraded high-latency coloring, and preserved keyboard/ARIA tooltip behavior. No API, database, migration, configuration, or production-data changes.

## Changes

- `upstream/sub2api/frontend/src/components/layout/AppSidebar.vue`: added `PerformanceMonitorIcon` and mapped it to the virtual menu item.
- `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2View.vue`: compressed title/status/window controls and added the dark compact shell.
- `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2GroupCard.vue`: added availability badge, compact metrics, tighter spacing, and responsive service-line grid.
- `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2Timeline.vue`: dense vertical bars, inner narrow-screen scrolling, no-data styling, and degraded amber buckets for operational latency >= 2000ms. Tooltip/focus semantics remain intact.
- Direct Vitest updates/additions under `src/**/__tests__`.

## Baseline and candidate

- Baseline: `main@13bae69bf`
- Candidate branch: `codex/t47-performance-monitor-visual`
- Candidate worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t47-performance-monitor-visual`

## Verification evidence

- Focused Vitest: 5 files, 24 tests passed.
- `pnpm typecheck`: passed.
- `pnpm build`: passed (`vite build`, 1058 modules transformed).
- `git diff --check`: passed.
- Dev server smoke: Vite served `http://127.0.0.1:5187/`; unauthenticated route redirected to Login.

## Release properties

- No migration, API, or data-source changes.
- `downtime_required=false` expected; root release preflight is authoritative.
- Rollback: revert candidate merge or promote the previous verified blue-green slot.

## Remaining root actions

1. Root review candidate diff and confirm no scope drift.
2. Refresh candidate if `main` advances before merge.
3. Run root minimal preflight, merge authorization, deploy, and online visual verification under the single release lane.
