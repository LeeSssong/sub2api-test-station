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
