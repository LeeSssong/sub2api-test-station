# Account monitor Task 6 local report

Date: 2026-08-02 (Asia/Shanghai)

Scope: frontend API contracts, dark account monitor card, group score dialog, daily ledger history drawer, and pnpm 11 workspace configuration. Backend/relay billing files were not changed.

## Verification

| Command | Result |
|---|---|
| `CI=true pnpm --dir upstream/sub2api/frontend install --frozen-lockfile --reporter append-only` | PASS; lockfile current; only approved `esbuild` and `vue-demi` build scripts ran |
| `pnpm --dir upstream/sub2api/frontend test --run src/api/__tests__/admin.accountMonitor.spec.ts src/api/__tests__/admin.reconciliation.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts src/components/admin/account-monitor/AccountMonitorLedgerHistoryDrawer.spec.ts` | PASS; 5 files, 15 tests |
| `pnpm --dir upstream/sub2api/frontend typecheck` | PASS |
| `pnpm --dir upstream/sub2api/frontend build` | PASS; Vite emitted existing chunk-size/dynamic-import warnings only |

## Implemented

- Added `operations` and daily `history` reconciliation contracts with scope parameters.
- Added group score weights, evidence, rank, eligibility, and score-weight API methods.
- Reworked `AccountMonitorCard` with dark monitoring treatment, quality evidence, global priority label, and read-only real-cost economics. Unknown coverage remains “待对账”. Closed groups are informational and do not use a red failure border.
- Added score-weight editor with a strict 100-point validation gate and reset event.
- Added a 30-day daily ledger history drawer that loads the active scope.
- Moved pnpm overrides/build approvals from deprecated `package.json.pnpm` to `pnpm-workspace.yaml`.

This is local verification only; push, deployment, and production acceptance remain outstanding.
