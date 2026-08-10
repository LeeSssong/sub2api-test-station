# Externalization Dual-Read Report

Status: local gate implementation complete; real Compose rehearsal not run. No
production traffic was switched, and no page is authorized as production
`external_primary` by this report.

## Comparison Contract

Each persisted report is scoped to one page and one exact administrator time
window. Promotion requires one passed, fresh report for every window kind:
`minimum`, `default`, and `maximum`.

The comparator records both source values and an exact result for:

- account, request, bill, token, rank, and reconciliation-exception counts;
- account, request, bill, and reconciliation-exception identifiers;
- raw cost, revenue, procurement cost, profit, profit margin, balance,
  multiplier, and score;
- rate and calculation versions;
- generated time, source watermark, completeness, and freshness deadline;
- permission, export, degradation, rollback, contract-completeness, operator,
  compared-at, and persisted-at evidence.

Decimal strings are parsed with `shopspring/decimal` and compared exactly.
There is no floating-point tolerance. Numerically equal representations such
as `1.2300` and `1.23000` match; `1.2300000001` does not. A balance difference
is allowed only when both sides contain valid decimals, distinct observation
timestamps, and non-empty source evidence. Both timestamps and source
references remain in the persisted report. Invalid balance decimals never
qualify as an explained observation gap.

Passed and failed reports are appended as JSON Lines to a regular `0600` file.
The repository flushes every append and can reload reports by page. A failed
comparison remains auditable instead of disappearing.

## Page Gates

There is one five-state decision implementation for all pages:

```text
legacy_only
  -> shadow_building
  -> dual_read_comparing
  -> external_primary
  -> legacy_retired
```

The Task 5 value `shadow` is accepted only as a compatibility alias for
`shadow_building`; unknown values resolve to `legacy_only`. Page overrides and
the previous Task 5 environment variables feed the same normalized flag map.

`shadow_building` and `dual_read_comparing` always render legacy data. An
`external_primary` response is rendered only when its evidence is page-matched,
passed, fresh, complete for all three windows, permission-compatible,
export-compatible, non-degraded, rollback-capable, and backed by a non-empty
evidence reference. The monitor and profitability pages additionally validate
the mapped response contract locally. Missing, stale, failed, or malformed
evidence keeps legacy visible and marks only the local read model degraded.

The accounting projection still does not implement the full usage-detail,
filter, sorting, pagination, detail, and export contract, so Usage stays on its
legacy source even if an external envelope claims promotion. Reconciliation
has the same centralized flag slot but no Task 5 page replacement path to
promote in this task. `legacy_retired` requires separate passed retirement
evidence with an evidence reference, operator, and timestamp; no legacy query
code was deleted.

The rollback flag is `legacy_only` per page. It immediately selects the legacy
path and does not touch the real-time request path or administrator session.

## Local Evidence

- `go test ./internal/compare -v`: PASS, 10 tests. Covers all three windows,
  exact values/identifiers, decimal precision, balance evidence, failed-report
  persistence, JSONL reload, cutover freshness, and retirement proof.
- `go test ./...`: PASS for every relay-ops package.
- Focused frontend suite: PASS, 5 files and 66 tests.
- `pnpm vitest run`: PASS, 232 files and 1663 tests.
- `pnpm lint`, `pnpm typecheck`, and `pnpm build`: PASS. Build retains the
  repository's existing chunk-size and dynamic/static import warnings.
- `bash tests/operations/smoke_sub2api_release_test.sh`: PASS.
- `bash tests/operations/deploy_sub2api_release_test.sh`: PASS.

The brief's literal command
`ops/smoke-sub2api-release.sh --rehearsal --rollback` exited 1 with
`release smoke failed`. The script does not parse those flags and requires a
real rehearsal environment through `BASE_URL`, `EXPECTED_VERSION`, absolute
Compose/deployment paths, secret and release env files, administrator and
gateway key files, and a baseline-count file. Those materials were not
available or authorized for this local task. No rehearsal success is claimed;
the two passing operations suites are dry-run contract evidence only.

## Release Status

This is a local implementation and contract report. Project status remains
`进行中`. Production promotion still requires real page/window comparison
records, an authorized rehearsal, push, deployment, and online verification.

## Fix Round 1 Evidence

The original literal command failure was the acceptance gap that this round
closed with a non-production fixture. The command now passes:

```text
ops/smoke-sub2api-release.sh --rehearsal --rollback
externalization_rehearsal=passed output=evidence/sub2api-rehearsal/task-9-local
```

The fixture writes regular `0600` JSONL artifacts: four immutable report sets
(monitor, profitability, accounting, reconciliation), three explicit windows
per set (minimum 15 minutes, default 24 hours, maximum 30 days), one shared
run/operator/lineage per set, and eight authenticated idempotent audit records
(four ordered promotions followed by four rollbacks). The summary records all
twelve report IDs, comparison timestamps, promotion results, rollback results,
and the final `legacy_only` mode for every page. This evidence is isolated
local data; it is not production authorization or deployment evidence.

The promotion authority is now server-side and authenticated. It loads the
durable report-set repository, rejects future/ancient or mixed-run evidence,
enforces the recursive monitor → profitability → accounting → reconciliation
predecessor chain, and persists runtime mode changes and rollback results. The
frontend only consumes the separate trusted decision endpoint; data responses
cannot supply cutover evidence, and Vite build-time modes are no longer an
authority. Accounting uses validated relay ledger totals for the summary while
retaining the legacy detail/filter/pagination/export contract; reconciliation
remains a separately authenticated control-plane read surface.
