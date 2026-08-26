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

## Fix Round 1

- Commit SHA: f962fcdd6
- Commit message: `fix: harden account monitor row responsiveness and compatibility`
- Moved the wide desktop grid, column spans, action labels, and action stacking from `lg` to `xl`, so the row remains safely stacked around the 1024px breakpoint without clipping controls or introducing page-level horizontal overflow.
- Changed the card quality-rank display to `quality_rank ?? group_rank` in both global and selected-group scopes. Scheduler display and ordering remain based only on `scheduler_rank`.
- Added focused regression coverage for the pre-wide breakpoint contract and selected-group legacy `group_rank` fallback while keeping scheduler rank independent.

## Verification

- Focused Vitest: PASS, 2 files / 107 tests (`59` component tests, `48` view tests).
- TDD fix cycle: PASS. Both new regression tests were run red before the production changes, then green after the fixes.
- `pnpm typecheck`: PASS.
- `pnpm build`: PASS, completed in 12.66s after transforming 1,076 modules.
- `git diff --check`: PASS.

## Unresolved Issues

- Browser-based desktop/mobile visual inspection was not run; the requested time-bounded fallback was used. Rendered markup and responsive class contracts are covered by the focused component/view tests.
- Existing Browserslist and Node deprecation warnings remain unrelated to this task.

## Release Scope

- No backend, migration, configuration, ledger, queue, deployment record, or unrelated worktree changes.
- No production deployment or data changes were performed.
