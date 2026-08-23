# T54-R1 Task 2 Report

## Status

READY_FOR_ROOT_REVIEW. Implemented in the designated worktree and branch only; no merge, push, deploy, or root project ledger changes were made.

## Scope Delivered

- Extended the admin settings contract for `custom` and `preset` scheduler modes, preset IDs, policy value snapshots, available preset definitions, and administrator custom presets.
- Split group state so default subscriptions continue to use active `subscription_type=subscription` groups while scheduler policies use every active native OpenAI group from `adminAPI.groups.getAll("openai")`.
- Removed scheduler group auto-selection and added a visible scheduler-group load error state.
- Added per-group draft storage and switching behavior.
- Added legacy `weighted_override`/`fair` normalization to `custom`/`preset` and builtin preset IDs.
- Added server snapshot retention for preset policies; serialization uses the retained effective snapshot and ignores later draft mutations.
- Added custom preset add/rename/delete state helpers and serializes custom preset definitions with settings updates.

## TDD Evidence

RED was observed before implementation with the focused scheduler run. The new tests failed on the unparameterized groups API call, scheduler/subscription group coupling, auto-selection, and legacy mode handling.

GREEN evidence:

- `pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts -t 'scheduler'`: 5/5 passed.
- `pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts`: 41/41 passed.

## Additional Verification

- `pnpm typecheck`: passed.
- `pnpm build`: passed.
- `git diff --check`: passed.

Vitest continues to emit existing unresolved `router-link` warnings in the test mount and one existing jsdom XHR AggregateError warning; they do not fail assertions or alter the focused result.

## Files Changed

- `upstream/sub2api/frontend/src/api/admin/settings.ts`
- `upstream/sub2api/frontend/src/views/admin/SettingsView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/SettingsView.spec.ts`

## Deployment / Migration

No migration, configuration schema, dependency, production-data, deployment, or GitHub Actions changes. `downtime_required` is expected to remain `false`; final release preflight belongs to the root release controller.

## Remaining Concerns

- The visible custom-preset management controls remain owned by Task 3; this task exposes the state helpers and persistence contract without rebuilding that UI.
- Existing unrelated test warnings described above remain present.
