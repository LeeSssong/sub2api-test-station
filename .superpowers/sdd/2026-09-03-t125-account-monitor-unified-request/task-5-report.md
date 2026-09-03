# T125 Task 5 Report

## Summary

Updated the existing Account Monitor card to consume the unified request
contract without changing its layout or native action/event paths. The public
timeline type no longer exposes probe/source fields. Request counts and chart
colors use only the selected unified request fields, so a probe-only selected
bucket renders as one ordinary successful request. The card now uses neutral
"窗口请求" and "近期请求" wording, and the native "编辑账号" action is
visible again after removing only its legacy hiding selector.

The existing refresh event remains functional, but its visible label is now
"刷新账号状态" and does not describe the underlying observation source.
Lifetime accounting remains represented by the existing cumulative business
request field; this frontend change does not add probe accounting data.

## Changed Files

- `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

## TDD Verification

The initial focused card test run passed the inherited 14 tests. After adding
the minimum source-agnostic and probe-only assertions, the focused test failed
as expected on the old "近期真实请求" and probe-specific refresh wording.
After the component changes, the final focused run passed:

```text
pnpm exec vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts --pool=forks --poolOptions.forks.singleFork=true
2 test files passed
50 tests passed (14 card, 36 view)
```

The view test also covers the existing native edit modal path through the
`accountEdit` event and `EditAccountModal` integration. No layout structure,
modal loading, or account action routing was changed.

## Additional Checks

- Targeted ESLint: passed with one pre-existing warning for unused
  `latestModalInput` in `AccountMonitorView.spec.ts`; no errors.
- `git diff --check`: passed before commit.
- No Go files were changed in Task 5, so `gofmt` was not applicable.
- `upstream/sub2api/frontend/pnpm-lock.yaml` was restored and is excluded.
- No deployment, push, merge, production access, or global ledger/queue
  changes were made.

## Worktree Boundaries

The worktree also contains pre-existing uncommitted changes in
`upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go`
and its test. They were inspected only and are intentionally not included in
this Task 5 commit.

## Commit

This report and the four Task 5 frontend files are included in the single
commit created for this task; the exact commit SHA is reported alongside the
final handoff after commit creation.

## Release State

Candidate remains local and unpushed in
`codex/t125-account-monitor-unified-request`. It is ready for root review only;
no deployment or production verification was performed.
