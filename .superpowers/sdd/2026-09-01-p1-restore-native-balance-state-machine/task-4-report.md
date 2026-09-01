# Task 4 Report: Read-Only Effective Schedulability Contract

## Scope

Task 4 only. The account-monitor projection now exposes native effective
schedulability for full-site and window responses. The implementation does not
change scheduler selection, score, rank, probe admission, or persisted account
state. Billing snapshots remain independent from effective schedulability.

## Implementation

- Added `effective_schedulable`, `effective_schedulable_at`, and
  `effective_unschedulable_reason` to `AccountMonitorAccount` and the frontend
  API contract.
- Added `projectEffectiveSchedulability(*Account, time.Time)`, which calls
  `Account.IsSchedulableAt(snapshotAt)` and maps the first native gate in this
  order: `inactive`, `manual_disabled`, `expired`, `overload`,
  `rate_limited`, `temp_unschedulable`, `quota_exceeded`.
- `List` and `ListWindow` each capture one UTC `observedAt` response snapshot;
  every row uses that same timestamp for the effective projection.
- The raw `schedulable` field remains unchanged. The projection does not read
  billing probe status and does not mutate the account.
- The card and account-info dialog display the manual switch separately from
  effective schedulability and its native reason. The view passes monitor
  projection fields into the already-loaded native account details without
  writing them back.

## RED/GREEN Evidence

- RED: before implementation,
  `TestProjectEffectiveSchedulabilityUsesNativeGateOrder` failed to compile
  because the projection helper did not exist.
- GREEN: the helper test passes and covers all native gate reasons, gate order,
  equality with `Account.IsSchedulableAt(fixedNow)`, and non-mutation of the raw
  schedulable flag.

## Direct Verification

Passed:

```text
go test ./internal/service ./internal/handler/admin -run 'Test.*AccountMonitor.*(Schedul|Effective|Handler)|TestProjectEffectiveSchedulabilityUsesNativeGateOrder' -count=1
go test ./internal/service -run 'TestAccountMonitorList(Window)?|TestProjectEffectiveSchedulabilityUsesNativeGateOrder' -count=1
vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorAccountInfoDialog.spec.ts
vue-tsc --noEmit
git diff --check
```

Frontend component result: 2 test files, 11 tests passed.

## Known Test Gap

`src/views/admin/__tests__/AccountMonitorView.spec.ts` ran with 48 tests: 47
passed and 1 failed in the existing cost-dialog test. The assertion expects
only `rate_multiplier` and `upstream_billing_rate_sync_enabled`, while the
implementation sends the already-existing `effective_cost_model`,
`upstream_actual_cost`, and `upstream_obtained_quota` fields as well. The same
payload is present in baseline commit `8a655cffe`; this unrelated test was not
changed.

An initial frontend dependency invocation also failed because this worktree had
no complete `node_modules` links. Existing local dependency artifacts were used
only to run the direct frontend tests; no dependency files are tracked or
included in the commit.

## Scope Review

- No production data, SSH, deployment, push, migration, workflow, scheduler,
  probe recovery, or account-state mutation was performed.
- No global progress or task queue file was modified.
- The historical temporary-isolation data remains untouched.

## Commit

This report and the Task 4 implementation are committed together after the
verification above.
