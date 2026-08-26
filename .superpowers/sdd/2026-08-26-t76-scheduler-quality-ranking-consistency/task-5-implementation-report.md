# T76 Task 5 Implementation Report

## Summary

Rebuilt the admin account monitor as dense, full-width explainable account rows. The implementation keeps quality ranking separate from the current scheduler projection and does not calculate ranking order in the frontend.

## Changed Files

- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
  - Replaced nested card-wall layout with a stable desktop grid row and stacked mobile layout.
  - Added quality score/rank, scheduler rank/priority, server-provided reason, and accessible expandable evidence.
  - Preserved account info/edit/delete/more, cost, model detection, refresh, concurrency, trend, calls, and priority events.
- `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
  - Changed the account list to one full-width column.
  - Uses scheduler rank only for selected concrete groups; full-site sorting uses quality rank with legacy `group_rank` fallback.
  - Keeps ranked rows first and stable account-ID order for unranked rows.
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
  - Added explainable ranking assertions and updated visual-contract assertions for the full-width row.
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
  - Added differing quality/scheduler order coverage and updated one-column/quality-view expectations.

## Commit

- Commit SHA: 76c916dc2cdca877965373a58db2483a8668025d
- Commit message: `feat: redesign account monitor as explainable full-width rows`

## Verification

- Focused Vitest: PASS, 2 files / 105 tests.
- `pnpm typecheck`: PASS.
- `pnpm build`: PASS on the completed production build run; a final rebuild after the last presentation-only cleanup reached module transformation before the command time cap and was not repeated.
- `git diff --check`: PASS.

## Unresolved Issues

- Browser-based desktop/mobile visual inspection was not run. Rendered markup and responsive class contracts are covered by the focused component/view tests.
- Existing Browserslist and Node deprecation warnings remain unrelated to this task.

## Release Scope

- No backend, migration, configuration, ledger, queue, deployment record, or unrelated worktree changes.
- No production deployment or data changes were performed.
