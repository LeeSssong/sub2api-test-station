# Task 4 Review Fix Report

## Finding

`schedulableLabel` previously returned `暂停` whenever `account.status` was not
`active`. Because the card labels this value as `人工开关`, an account with
`status=disabled` and `schedulable=true` was incorrectly shown as manually
paused.

## Fix

- `schedulableLabel` now depends only on the raw `account.schedulable` field.
- The account status remains independently rendered through `statusLabel`.
- The effective schedulability and native reason remain independently rendered
  through `effective_schedulable` and `effective_unschedulable_reason`.
- Added a regression test for `status=disabled`, `schedulable=true`, and
  `effective_unschedulable_reason=inactive`; it asserts `人工开关：可调度`,
  independent status `暂停`, and effective `不可调度（账号未激活）`.

## Verification

Passed:

```text
vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts
11 tests passed
vue-tsc --noEmit
git diff --check
```

No Task 5 documents, global progress or queue files, production data,
deployment, SSH access, or push operations were touched.

## Files

Only these three files are included in the fix commit:

- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- `.superpowers/sdd/2026-09-01-p1-restore-native-balance-state-machine/task-4-review-fix-report.md`

No baseline test gap was encountered in the direct verification.
