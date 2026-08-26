# T76 Task 4 Implementation Report

## Changed files

- `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`
  - Added endpoint contract coverage for concrete group quality/scheduler ranks,
    explanation facts, candidate totals, policy labels, and bounded reason codes.
  - Added full-site assertions that scheduler rank fields are omitted.
  - Added projection-failure assertions that preserve the success envelope and
    quality fields while leaving scheduler fields unavailable/omitted.
  - Extended the test repository fixture for group-scoped window aggregates.
- `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
  - Added optional `candidate_total` to `AccountMonitorSchedulerExplanation`,
    matching the backend JSON contract.
- `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go`
  - No change required; existing response success/error wrapping preserves the
    endpoint, authentication, envelope, and legacy-field contract.

## Commit

- Implementation commit: `22abbfe6c` (`test: lock account monitor ranking response contract`)

## Tests and results

- `cd upstream/sub2api/backend && go test ./internal/handler/admin -run 'TestAccountMonitorHandler.*(Rank|Projection|Group)' -count=1`
  - PASS
- `cd upstream/sub2api/backend && go test ./internal/handler/admin -run 'AccountMonitor' -count=1`
  - PASS
- `cd upstream/sub2api/frontend && pnpm typecheck`
  - PASS (`vue-tsc --noEmit`)
- `git diff --check`
  - PASS

## Unresolved issue

None for Task 4. No deployment, production verification, migration, or runtime
business-data write was performed in this worktree.
