# Task 5 Report: Same-Session Admin Dual Reads

## Status

DONE. The project-progress ledger remains `进行中`: this task did not push,
deploy, or perform online verification.

## RED Evidence

The focused API test was expanded before implementation and failed as expected:

```text
pnpm vitest run src/__tests__/controlPlaneApi.spec.ts
```

The failure showed that `apiClient.get` and `apiClient.post` did not receive
`skipSessionRecovery: true`, and that `resolveControlPlaneReadMode` was absent.

Page-level contract tests were then added before their integrations:

- `AccountMonitorView.spec.ts` failed because shadow mode did not call the
  control-plane monitor route and rendered no local degraded status.
- `AccountProfitabilityView.spec.ts` failed because shadow mode did not read
  the control-plane profitability projection.
- `UsageView.spec.ts` failed because shadow mode did not read ledger freshness.

## GREEN Evidence

```text
cd upstream/sub2api/frontend
pnpm vitest run src/views/admin/__tests__/UsageView.spec.ts \
  src/views/admin/__tests__/AccountProfitabilityView.spec.ts \
  src/views/admin/__tests__/AccountMonitorView.spec.ts \
  src/__tests__/controlPlaneApi.spec.ts
```

Passed: 4 files, 50 tests.

```text
pnpm vitest run
pnpm lint
pnpm typecheck
pnpm build
```

Passed: 231 test files and 1646 tests; lint, typecheck, and production build
all exited 0. Existing suite output retains its unrelated Vue/JSDOM warnings.
`git diff --check` exited 0.

## Changed Behavior

- Added safe parsing for `legacy_only`, `shadow`, and `external_primary`, with
  a global `VITE_CONTROL_PLANE_READ_MODE` and per-page overrides:
  `VITE_ACCOUNT_MONITOR_READ_MODE`,
  `VITE_ACCOUNT_PROFITABILITY_READ_MODE`, and `VITE_USAGE_READ_MODE`.
  Invalid or omitted values fail closed to `legacy_only`.
- All control-plane requests remain same-origin `/api/v1/xingqiao/*` calls and
  use `skipSessionRecovery`, so an isolated control-plane 401 cannot clear or
  redirect the active Sub2API administrator session.
- Account Monitor and Account Profitability retain legacy output in
  `legacy_only` and `shadow`. In `external_primary`, they use a control-plane
  response only if it satisfies the existing full page response shape;
  otherwise legacy output remains visible with a local degraded status.
- Usage retains its established table, filters, sorting, pagination, details,
  and export path. It dual-reads control-plane ledger freshness without trying
  to substitute the narrower ledger projection for usage detail rows.
- The existing status component now appears only for enabled dual-read modes
  and exposes source, update time, completeness, calculation version, local
  degraded state, and retry.

## Compatibility Checks

- Existing routes and navigation remain untouched: `/admin/accounts/monitor`,
  `/admin/operations/account-profitability`, and `/admin/usage` are unchanged.
- Existing account-monitor ranges, profitability filters and CSV behavior, and
  usage filtering/sorting/pagination/detail/export contracts remain covered by
  the pre-existing page tests plus the new shadow-read tests.
- Source and generated production bundle scans found no internal hostname,
  relay service hostname, or added login/second-auth route in Task 5 code.

## Self-Review

- Verified control-plane calls only use relative same-origin paths.
- Verified failures are caught at the page-local boundary and never call the
  primary page error/session path.
- Inspected the final task diff; no router, menu, production, deployment, or
  workflow file changed.

## Residual Concerns

The Task 4 account/profitability/ledger projections are intentionally narrower
than the current full administrator page contracts. `external_primary`
therefore remains guarded by a runtime shape check and falls back locally when
the projection is not yet compatible. Task 9 must add the cross-source
comparison gates and evidence needed before a production cutover.

## Commits

`e26649ab4b88e7b57852c855373f2cd71cedd8e6`
`feat: dual-read admin views without changing routes`

## Fix Round 1

### Status

DONE. The project-progress ledger remains `进行中`; this correction did not
push, deploy, or perform online verification.

### RED Evidence

The new `external_primary` page tests first failed in four cases. A
complete-looking control-plane monitor/profitability response was labelled
`来源：控制面`, while a shallow incomplete response replaced the legacy cards or
profitability rows. Those failures demonstrated that the old shape guards were
not sufficient to preserve the legacy business contract.

The added 401/403 page tests cover monitor, profitability, and usage local
degradation. The new integration-level client test invokes
`controlPlaneAPI.monitor` through the real Axios client and response
interceptor, rather than testing a mocked request option.

### GREEN Evidence

```text
cd upstream/sub2api/frontend
pnpm vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts \
  src/views/admin/__tests__/AccountProfitabilityView.spec.ts \
  src/views/admin/__tests__/UsageView.spec.ts \
  src/api/__tests__/client.spec.ts \
  src/__tests__/controlPlaneApi.spec.ts
```

Passed: 5 files, 83 tests.

```text
pnpm vitest run
pnpm lint
pnpm typecheck
pnpm build
git diff --check
```

Passed: 231 test files and 1657 tests; lint, typecheck, build, and diff check
all exited 0. Existing Vue/JSDOM warnings remain unrelated to this fix.

### Fixes

- Account Monitor and Account Profitability now always render their legacy
  response in `external_primary`, even if the control-plane response appears
  complete. The control-plane result remains observable only as freshness
  metadata and a local degraded state until Task 9 proves the full mapping and
  cross-source comparison gate.
- Degraded status now names the actual active source (`现有系统`) as well as the
  control-plane failure, so the display cannot imply a promoted external data
  source.
- Page regressions cover complete-looking and incomplete external responses,
  plus 401 and 403 local degradation for monitor, profitability, and usage.
- The real client/interceptor regression confirms a control-plane 401 carries
  `skipSessionRecovery`, preserves access and refresh tokens, and leaves the
  administrator route unchanged without a login redirect.

### Self-Review And Residual Concern

The correction is limited to the two external-primary selection paths, shared
read-model status display, and focused page/client tests. No routes, menus,
auth implementation, deployment, or workflow files changed. The deliberate
residual limitation is that `external_primary` is not a production promotion
mechanism until Task 9 supplies complete field mapping and comparison evidence.

### Fix Commit

Pending final Fix Round 1 commit.
