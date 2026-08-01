# Final fix round 3 report

Base commit: `6f3c1d619`.

Status: scoped independent re-review approved; remains `进行中` pending
push, deployment, and production verification.

## Remaining durable account-row version finding

- `accountRepository.UpdateLastUsed` now writes `last_used_at` through
  `execAccountMonotonicUpdate`, so the account row version is updated in the
  same statement with
  `GREATEST(clock_timestamp(), updated_at + interval '1 microsecond')`.
- The existing account-level `SchedulerOutboxEventAccountLastUsed` publication
  remains unchanged: its payload is `{"last_used":{"<account-id>":<unix>}}`.

## TDD evidence

1. Added
   `TestUpdateLastUsedAdvancesDurableAccountVersionAndPublishesLastUsedEvent`
   before changing production code.
2. RED observed: the pre-fix Ent path issued
   `UPDATE "accounts" SET "updated_at" = $1, "last_used_at" = $2 ...`, which
   did not match the expected database-monotonic update.
3. Replaced only that mutation with the existing monotonic helper and observed
   GREEN.

## Validation

- `go test ./internal/repository -run '^Test(UpdateLastUsedAdvancesDurableAccountVersionAndPublishesLastUsedEvent|ExecAccountMonotonicUpdateUsesLockedDatabaseVersionExpression)$' -count=1` - passed.
- `go test ./internal/repository -run 'Test(UpdateLastUsed|ExecAccountMonotonicUpdate|SchedulerCache|UpdateUpstreamBillingProbeSnapshot)' -count=1` - passed.
- `go test ./internal/repository -count=1` - passed.
- `DOCKER_HOST=unix:///Users/gongtengxinwen/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true SUB2API_TEST_POSTGRES_IMAGE=postgres:18-alpine go test -tags=integration ./internal/repository -run '^(TestManagedBillingMultiplierPersistsQuantizedValueIdempotentlyAndRefreshesCache|TestSchedulerCacheNewerManualSnapshotWinsAgainstStaleManagedWriteIntegration|TestProbeTransactionStartedBeforeNormalEditPublishesStrictlyNewerCacheVersion)$' -count=1` - passed (`ok .../internal/repository 3.433s`).
- `git diff --check` - passed before commit.

No push, deployment, production access, or production verification was
performed. The temporary local `redis:8.4-alpine` compatibility tag used to
avoid a Docker Hub metadata fetch was removed after validation.

## Independent review

The requested range `341a6c902..8d6a1f8d8` and the excluded range-start fix
commit `341a6c902` were reviewed together against the round-2 finding, this
report, the SDD ledger, and the project progress ledger. No Critical or
Important finding remains in the scoped fix. `UpdateLastUsed` retains the
existing event type and payload while advancing the durable account-row
version in the same PostgreSQL statement as `last_used_at`.

Verdict: **APPROVED / merge-ready after this review commit**. This verdict is
limited to branch integration readiness; the project item stays `进行中` until
server push, deployment, and production verification are all complete.
