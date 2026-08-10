# Task 9 Implementation Report

## Status

`DONE_WITH_CONCERNS`

The exact comparison, durable local report store, five-state page gate, and
fail-closed page integrations are implemented and locally verified. The real
Compose rehearsal and rollback could not run without an authorized rehearsal
environment and its required files. No push, deployment, production access,
traffic switch, or online verification occurred. Project status remains
`进行中`.

## RED Evidence

Tests were added before implementation.

```text
cd relay-ops-service && go test ./internal/compare -v
```

Failed at compile time on the absent Task 9 contracts, beginning with
`undefined: Page`, `undefined: WindowKind`, and `undefined: SourceSnapshot`.

```text
cd upstream/sub2api/frontend && pnpm vitest run \
  src/config/__tests__/externalizationFlags.spec.ts \
  src/views/admin/__tests__/AccountMonitorView.spec.ts \
  src/views/admin/__tests__/AccountProfitabilityView.spec.ts \
  src/views/admin/__tests__/UsageView.spec.ts \
  src/__tests__/controlPlaneApi.spec.ts
```

Failed 7 of 66 tests: the five-state normalizer and page decision function did
not exist, the Task 5 `shadow` alias was not reconciled, and monitor and
profitability continued to render legacy data despite complete passed gate
fixtures.

A second RED cycle added durable JSONL and balance-audit requirements. It
failed on the absent repository constructor and absent persisted balance
observation/source fields.

## GREEN Evidence

```text
cd relay-ops-service && go test ./internal/compare -v
```

PASS: 10 tests, including minimum/default/maximum windows, exact identifiers,
decimal precision, timestamped balance gaps, invalid decimal rejection, failed
report persistence, JSONL reload, freshness, rollback, and retirement gates.

```text
cd upstream/sub2api/frontend && pnpm vitest run \
  src/config/__tests__/externalizationFlags.spec.ts \
  src/views/admin/__tests__/AccountMonitorView.spec.ts \
  src/views/admin/__tests__/AccountProfitabilityView.spec.ts \
  src/views/admin/__tests__/UsageView.spec.ts \
  src/__tests__/controlPlaneApi.spec.ts
```

PASS: 5 files, 66 tests.

```text
cd relay-ops-service && go test ./...
cd upstream/sub2api/frontend && pnpm vitest run
cd upstream/sub2api/frontend && pnpm lint
cd upstream/sub2api/frontend && pnpm typecheck
cd upstream/sub2api/frontend && pnpm build
```

PASS: all relay-ops packages; 232 frontend test files and 1663 tests; lint,
typecheck, and production build exited 0. The frontend suite/build retained
pre-existing JSDOM, i18n, chunk-size, and dynamic/static import warnings.

```text
bash tests/operations/smoke_sub2api_release_test.sh
bash tests/operations/deploy_sub2api_release_test.sh
```

PASS: release smoke contracts and recreate-only release orchestration.

`git diff --check` passed before the implementation commit.

## Comparison And Persistence Semantics

Each report is keyed by page, exact window kind, exact start/end times, and a
comparison timestamp. A report contains both legacy and external values for:

- account/request/bill/token/rank/reconciliation-exception counts;
- account/request/bill/reconciliation-exception identifiers;
- raw cost, revenue, procurement cost, profit, margin, balance, multiplier,
  and score;
- rate and calculation versions;
- freshness, completeness, watermark, permission, export, degradation,
  rollback, full-contract, operator, comparison, and persistence evidence.

Identifiers are compared as exact sorted multisets. Counts and versions are
exact. Decimal strings use `shopspring/decimal`; numeric equality is exact and
has no float tolerance. A balance mismatch is passable only when both values
are valid decimals and both sides persist distinct observation timestamps and
non-empty source references. Invalid or source-less balance evidence fails.

Both passed and failed reports are appended to a regular `0600` JSONL file,
flushed, and reloadable by page. Repository errors fail the operation rather
than returning an unpersisted report as evidence.

## Page Cutover And Rollback

The sole frontend mode source normalizes every page to:
`legacy_only`, `shadow_building`, `dual_read_comparing`, `external_primary`,
or `legacy_retired`. Task 5's `shadow` is a compatibility alias; unknown or
missing values become `legacy_only`. Old Task 5 per-page variables remain
compatible but feed this same decision map.

Shadow and comparison states render legacy. External primary requires a
page-matched, passed, fresh, complete, three-window envelope with permission,
export, rollback, non-degradation, complete-contract, and evidence-reference
checks. Monitor and profitability also validate their mapped runtime response
before replacement. Any failure leaves legacy visible and marks local
degradation. Usage remains legacy because the accounting projection does not
cover its full table/filter/sort/pagination/detail/export contract.

`legacy_retired` additionally requires explicit passed retirement evidence,
operator, timestamp, and reference. No legacy path was deleted. Setting a page
to `legacy_only` is the immediate rollback flag.

## Rehearsal Evidence

The required literal command was run:

```text
ops/smoke-sub2api-release.sh --rehearsal --rollback
release smoke failed
```

It exited 1. The script does not parse these flags and requires a real
rehearsal via `BASE_URL`, `EXPECTED_VERSION`, absolute deployment/Compose
paths, secret/release environment files, administrator and gateway key files,
and baseline record counts. Those inputs were not present and production or
remote access was prohibited. The passing operations suites above are the
strongest safe dry-run evidence; they are not represented as a real rollback.

## Compatibility Checks

- Administrator routes, menus, login, 2FA, fields, filters, sorting,
  pagination, refresh, details, CSV/export code, and same-origin URLs were not
  changed.
- Control-plane calls remain relative `/xingqiao/*` calls and retain
  `skipSessionRecovery: true`; a local 401/403 cannot clear the main session.
- No internal hostname, second login, direct core-table write, GitHub Actions
  workflow, production file, or remote branch was added or changed.
- Task 5 containment remains active for incomplete monitor, profitability,
  accounting, and reconciliation projections.

## Self-Review

- Inspected the complete Task 9 diff and verified all changed paths belong to
  comparator/report persistence, centralized flags, the three Task 5 pages,
  focused tests, or required reports/ledger.
- Checked realistic mutations: missing window, stale gate, wrong page,
  incomplete contract, permission/export/rollback failure, degradation,
  decimal drift, identifier drift, missing balance evidence, and missing
  retirement evidence are all rejected by tests.
- Confirmed generated frontend build files did not enter the worktree.

## Residual Concerns

- No real minimum/default/maximum production comparison records exist yet.
- The comparator/report library and evidence envelope are not production
  authorization: Task 10 must wire reviewed evidence into an authorized
  rehearsal/deployment chain.
- Accounting and reconciliation remain legacy-primary because their complete
  page contracts are not mapped in current control-plane projections.
- Real rollback timing, operator identity, permission parity, and export parity
  still require the authorized rehearsal and later online verification.

## Commit SHA(s)

- `2fee595965fb5395235e7df060bc6b157c608a0c` - implementation, tests,
  dual-read report, and project ledger update.
- This report is committed separately so it can cite the immutable
  implementation SHA above.

## Fix Round 1 Implementation Report

Status: `DONE_WITH_CONCERNS` pending independent review and the required
push/deploy/online-verification lifecycle. No production access or core-table
writes occurred.

RED→GREEN evidence added in this round:

- `go test ./internal/compare -run 'TestCompareAndPersistSet|TestEvaluateLatestValidSet|TestBalanceVarianceRequires'`: RED on absent coherent-set/snapshot-bound contracts, then PASS.
- `go test ./internal/compare -run TestComparisonRequiresCurrencyRanks`: RED on absent currency/rank/reconciliation/version dimensions, then PASS.
- `go test ./internal/compare -run TestRuntimeCutoverAuthority`: RED on absent runtime authority, then PASS, including recursive downstream fail-closed after an earlier-page rollback.
- `go test ./internal/controlplane -run TestRuntimeCutoverRoutes`: RED on absent trusted decision/mode routes, then PASS with actor derived from verified context.
- `pnpm vitest run src/config/__tests__/externalizationFlags.spec.ts src/__tests__/controlPlaneApi.spec.ts`: RED on browser-owned evidence/API, then PASS after separate server decision and idempotent runtime mode APIs.
- Focused monitor, profitability, Usage view suites: RED on missing trusted decision integration, then PASS. Usage now applies only validated relay accounting summary totals while preserving legacy detail/filter/export behavior.
- `bash tests/operations/smoke_sub2api_release_test.sh`: PASS after the new literal fixture case. `ops/smoke-sub2api-release.sh --rehearsal --rollback` produced actual local artifacts under `evidence/sub2api-rehearsal/task-9-local/` (4 report sets, 12 windows, 8 audit records, all final modes `legacy_only`).

Independent implementation review completed locally after the broad suite. It
confirmed authenticated server-side decision authority, recursive predecessor
fail-closed behavior, idempotent durable cutover records, and frontend fallback
when decision or payload contracts are invalid. The review also tightened
report-set eligibility so child reports must retain the parent set/run/lineage
and persistence identity before a set can authorize cutover.

Residual lifecycle concern: all evidence is local/non-production. The project
must remain `进行中` until the candidate is reviewed, pushed, deployed, and
verified online under the repository worktree rules.
