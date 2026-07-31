# Task 2 report — repository update, audit, and cache coherence

STATUS: READY FOR INDEPENDENT REVIEW

## Scope completed

`UpdateUpstreamBillingProbeSnapshot` now calls the Task 1 pure decision API
and always persists the sanitized probe snapshot under its existing identity
CAS and durable scheduler-outbox transaction. For an `upstream_managed`
account with a valid changed `effective_rate_multiplier`, the same transaction
also writes `accounts.rate_multiplier`, guarded by the loaded multiplier and
the current persisted policy. A concurrent/manual rate or policy change leaves
the new snapshot intact and suppresses the multiplier write and audit event.

After a successful commit, the repository refreshes the existing scheduler
account snapshot. A changed multiplier records a non-blocking, post-commit
audit event through the existing `AuditLogService`; audit fields are account
id, old/new multiplier, `native_billing`, probe timestamp, trigger, policy,
and system actor. The trigger defaults to `scheduled`; Task 3 can use
`service.WithUpstreamBillingRateMultiplierSyncTrigger(..., "lifecycle")` for
lifecycle probes.

Manual-override, unchanged, and invalid probe decisions retain the probe
snapshot but do not write a multiplier or misleading audit event. Commit
failure produces neither a scheduler refresh nor an audit event.

No production configuration, data, lifecycle trigger, or request-forwarding
path was changed. The project ledger remains **进行中** because this work has
not been pushed, deployed, or verified in production.

## Commits

- `30d29a8f612bdf0f24e31a34e382d997b8c5f2b7` — `feat: sync managed billing rate multiplier`

## Verification evidence

RED (before implementation):

```text
go test ./internal/repository -run 'TestUpdateUpstreamBillingProbeSnapshot(SynchronizesManagedMultiplierAuditsAndRefreshesScheduler|CommitFailureDoesNotRefreshSchedulerOrAudit|PreservesSnapshotWithoutMultiplierChange)$' -count=1
...
undefined: newAccountRepositoryWithSQLAndAudit
FAIL github.com/Wei-Shaw/sub2api/internal/repository [build failed]
FAIL
```

RED (lifecycle audit trigger contract before its context helper existed):

```text
go test ./internal/repository -run TestUpstreamBillingMultiplierAuditUsesLifecycleTriggerFromContext -count=1
...
undefined: service.WithUpstreamBillingRateMultiplierSyncTrigger
undefined: service.UpstreamBillingRateMultiplierSyncTriggerLifecycle
FAIL github.com/Wei-Shaw/sub2api/internal/repository [build failed]
FAIL
```

GREEN (targeted Task 2 tests):

```text
go test ./internal/repository -run 'TestUpdateUpstreamBillingProbeSnapshot(SynchronizesManagedMultiplierAuditsAndRefreshesScheduler|CommitFailureDoesNotRefreshSchedulerOrAudit|PreservesSnapshotWithoutMultiplierChange)$|TestUpstreamBillingMultiplierAuditUsesLifecycleTriggerFromContext' -count=1
ok github.com/Wei-Shaw/sub2api/internal/repository 2.166s
```

GREEN (required package regression set, exit status 0):

```text
go test ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes -count=1
PASS (exit 0)
```

GREEN (Wire-generated server composition compiles):

```text
go test ./cmd/server -run '^$' -count=1
ok github.com/Wei-Shaw/sub2api/cmd/server 1.984s [no tests to run]
```

`git diff --check` passed before commit.

## Self-review

- Reuses `DecideUpstreamBillingRateMultiplierSync`; policy parsing and numeric
  validation are not duplicated in the repository.
- Keeps the original snapshot identity CAS, transaction, durable outbox, and
  post-commit scheduler snapshot synchronization.
- Separates snapshot persistence from the guarded multiplier update so a
  concurrent/manual rate change cannot discard valid probe observability.
- Calls `AuditLogService.Record` only after commit, matching the existing
  non-blocking audit convention; a writer failure therefore cannot roll back
  a valid probe update.
- Tests cover managed change plus audit/cache refresh, commit failure, manual
  override, unchanged value, invalid value, and lifecycle trigger propagation.

## Concerns / follow-up

- `AuditLogService` is intentionally asynchronous and currently logs/counts
  batch-write failures rather than synchronously retrying them. This preserves
  the project contract that audit failure cannot lose a committed probe update;
  any stronger durable-audit retry requirement needs a separately designed
  outbox flow.
- Calls made inside a caller-owned Ent transaction cannot safely run a
  post-commit audit/cache callback because the repository does not own that
  commit. The production probe entry point owns its transaction and is covered
  here; callers that introduce an outer transaction should add an explicit
  post-commit hook.
- Task 3 must mark lifecycle probes with the new context helper and must not
  add billing probes to ordinary request forwarding.

## Fix round 1/5

Independent review findings were reproduced and fixed:

- Probe multipliers are quantized with positive half-up semantics to
  PostgreSQL `decimal(10,4)` before validation, comparison, CAS, persistence,
  and audit construction. `0.249975` therefore becomes `0.2500`, repeated
  probes are no-ops, and values that round to zero or overflow the decimal
  maximum are rejected.
- Scheduler full and metadata account payloads now use one Redis Lua write with
  a fixed-width UTC `UpdatedAt` fence. Stale post-commit `SetAccount` and bulk
  snapshot rebuild writes are skipped while snapshot membership remains intact;
  `last_used` stays a separate monotonic side key. Delete and unencodable
  payload paths use tombstone/live version markers to prevent resurrection.
- Added real PostgreSQL/Redis integration coverage through
  `integration_harness_test.go` for decimal idempotency, newer-manual cache
  wins, and managed update/cache refresh.

RED/GREEN evidence:

```text
go test ./internal/service -run TestDecideUpstreamBillingRateMultiplierSync -count=1
RED: high-precision normalization and rounding-boundary cases failed.
GREEN: ok github.com/Wei-Shaw/sub2api/internal/service

go test -tags=unit ./internal/repository -run 'TestSchedulerCache' -count=1
GREEN: ok github.com/Wei-Shaw/sub2api/internal/repository

go test -tags=unit ./internal/repository -count=1
GREEN: ok github.com/Wei-Shaw/sub2api/internal/repository

go test ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes -count=1
go test ./cmd/server -run '^$' -count=1
GREEN: all packages exited 0.

go test ./... -count=1
GREEN: all packages exited 0.

go test -tags=integration ./internal/repository -run 'Test(ManagedBillingMultiplierPersistsQuantizedValueIdempotentlyAndRefreshesCache|SchedulerCacheNewerManualSnapshotWinsAgainstStaleManagedWriteIntegration)$' -count=1 -v
BLOCKED: harness panicked while starting PostgreSQL with exact error `rootless Docker not found`; Docker is unavailable in this environment.
```

The project progress ledger remains **进行中**: this fix is committed locally
but has not been pushed, deployed, or verified in production.
