# Final fix round 3 report

Base commit: `6f3c1d619`.

Status: local implementation complete; remains `进行中` pending scoped
independent re-review, push, deployment, and production verification.

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
- `git diff --check` - passed before commit.

No push, deployment, production access, or production verification was
performed. The remaining required step is an independent scoped review.
