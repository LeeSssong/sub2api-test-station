# T76 Task 4 Implementation Report

## Changed files

- `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`
  - Strengthened the endpoint contract for both concrete group rows, including
    quality/scheduler ranks and totals, eligibility, policy facts, effective
    weights, candidate scope, snapshot time, primary reason code/label, and the
    success envelope.
  - Added full-site assertions that scheduler rank fields and the unavailable
    flag are omitted.
  - Added error, nil-projection, and nil-load-snapshot assertions that preserve
    the success envelope and quality fields, omit scheduler ranks/explanation,
    and emit `scheduler_unavailable: true` without invoking the projection
    provider when the load snapshot cannot be obtained.
  - Extended the test repository fixture for group-scoped window aggregates.
- `upstream/sub2api/backend/internal/service/account_monitor_service.go`
  - Marks supported group rows explicitly unavailable when scheduler projection,
    scheduler load retrieval, or the configured concurrency cache is unavailable.
  - Keeps unsupported/non-applicable groups omitted and preserves scheduler
    projection behavior when no concurrency service is configured.
- `upstream/sub2api/backend/internal/service/account_monitor_types.go`
  - Added optional `scheduler_unavailable` to the account monitor row contract.
- `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
  - Added optional `scheduler_unavailable` and `candidate_total` fields matching
    the backend JSON contract.

`upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go`
was unchanged; existing response wrapping preserves the endpoint,
authentication, envelope, and legacy-field contract.

## Commit

- Fix commit: `c47257a63` (`fix: make scheduler projection unavailability explicit`)
- Fix round 2 commit: `917a39faf` (`test: assert scheduler projection transport contract`)
- Round 2 changed only `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`.
  The handler contract now asserts `RequiredTransport == OpenAIUpstreamTransportAny`
  and `RequestedModel == ""`, preserving the approved transport contract without
  introducing a model dimension.

## Tests and results

- `cd upstream/sub2api/backend && go test ./internal/handler/admin -run 'TestAccountMonitorHandler.*(Rank|Projection|Group)' -count=1`
  - PASS
- `cd upstream/sub2api/backend && go test ./internal/handler/admin -run 'TestAccountMonitorHandler.*(Rank|Projection|Group|LoadSnapshot)' -count=1`
  - RED before the production guard: `TestAccountMonitorHandlerNilLoadSnapshotExposesSchedulerUnavailable` failed because `scheduler_unavailable` was absent; PASS after the guard.
- `cd upstream/sub2api/backend && go test ./internal/handler/admin -run 'AccountMonitor' -count=1`
  - PASS
- `gofmt -w internal/handler/admin/account_monitor_handler_test.go`
  - PASS
- `git diff --check`
  - PASS
- `cd upstream/sub2api/frontend && pnpm typecheck`
  - PASS (`vue-tsc --noEmit`)
- `gofmt -d` on all touched Go files
  - PASS (no output)
- `git diff --check`
  - PASS

## Unresolved issue

None in the Task 4 implementation scope. No deployment, production
verification, migration, or runtime business-data write was performed in this
worktree; root integration and release verification remain pending.
