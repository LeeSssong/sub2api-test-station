# Account Monitor Card Task 1

## Scope

- Added low-emphasis `账号信息` entry and native-style `编辑`、`删除`、`更多` entry points to `AccountMonitorCard.vue`.
- Each entry emits the current compact `AccountMonitorAccount` projection (`accountInfo`、`accountEdit`、`accountDelete`、`accountMore`).
- Existing card fields, layout, `data-test` selectors, priority/cost/refresh events, and monitor projection shape were preserved.
- No changes to `AccountMonitorView.vue`、`AccountsView.vue`、API DTOs、backend、migrations、deployment configuration, or production.

## Validation

- RED: `pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts --reporter=dot` failed as expected before implementation: 1 new entry-point test failed because `account-info` did not exist (35 existing tests passed).
- GREEN: `pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts --reporter=dot` passed: 36 tests.
- Typecheck: `pnpm vue-tsc --noEmit` passed.
- Formatting: `git diff --check` passed.

## Changed files

- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

`downtime_required`: not applicable; this task changes frontend component code only.
