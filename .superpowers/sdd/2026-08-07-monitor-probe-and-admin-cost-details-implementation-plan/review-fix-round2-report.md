# Whole-branch review fix round 2 report

Date: 2026-08-07
Branch: `codex/fix-relay-admin-auth`
Base: `4223fc86d`
Implementation commit: `a1d527d67` (`fix: close monitor and import review gaps`)

## Status

Both scoped Important findings are fixed and verified locally. The project remains **in progress** because independent scoped re-review, push, deployment, and production verification are still pending.

## Finding 1 — real Antigravity Chinese HTTP errors were not fatal on the first probe

### Root cause

Antigravity account testing emits `API 返回 <status>: ...`, while the monitor status extractor and HTTP classifier recognized only English `returned/status/http/failed` contexts. Chinese 401/402/403 therefore had no HTTP status and stayed `abnormal`; a Chinese 500 body could also fall into a non-HTTP classifier branch.

### RED

Command:

```text
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitor(ProbeResultClassifiesFatalErrorsWithHTTPStatus|ClassifierFatalErrorsAreUnavailableImmediately)' -count=1
```

Result: failed as expected. Chinese 401/402/403 produced `account_test_error` with nil HTTP status and `abnormal`; Chinese 500 did not classify as `http_error`.

### GREEN

The status-context regex now recognizes the real `返回` token, and the classifier recognizes `API 返回` as an HTTP error context. The same focused command passed. Tests assert:

- Chinese 401/402/403 extract their exact status and are immediately `unavailable` on the first failed probe.
- Chinese 500 extracts status 500 but remains non-fatal `abnormal` until the normal consecutive-failure threshold.

## Finding 2 — periodic usage re-import could not enrich an empty upstream request ID

### Root cause

`RecordUpstreamCostAttempt` used a no-op conflict update, returned the existing attempt with an empty `UpstreamRequestID`, and then `sameAttempt` rejected the newly enriched input. `UsageImporter` stopped that source immediately, so later logs in the same page were not imported.

### RED

Using an isolated PostgreSQL 18 test database:

```text
cd relay-ops-service
RELAY_OPS_TEST_DATABASE_URL='postgres://.../relay_ops_test?sslmode=disable' \
  go test ./internal/store -run TestUsageImporterBackfillsMissingUpstreamRequestIDAndContinuesSource -count=1
```

Result: failed as expected with `ErrConflict`, `Observed=1`, `Inserted=0`; the later usage log was never reached.

### GREEN

The store now validates that every immutable field matches with the upstream ID normalized to the existing empty value, then reuses the existing conditional `BindUpstreamRequestID` operation for the sole allowed empty-to-non-empty enrichment. The returned attempt is updated in memory before the final full identity check.

The PostgreSQL-backed regression now proves:

- Empty-to-non-empty `upstream_request_id` enrichment succeeds.
- The importer continues to the following log in the same source.
- Repeating the same non-empty ID is idempotent.
- A different non-empty upstream ID remains `ErrConflict`.
- Changed local request ID or model remains `ErrConflict` and cannot partially bind the provider ID while the stored value is empty.

A deliberate broken-order mutation (binding before immutable-field validation) made the new test fail by exposing the partial write; restoring the guard returned the test to green.

## Verification

All commands completed with exit code 0 on the final tree:

```text
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitor(ProbeResultClassifiesFatalErrorsWithHTTPStatus|ClassifierFatalErrorsAreUnavailableImmediately)' -count=1
go test ./... -count=1
go vet ./...

cd relay-ops-service
RELAY_OPS_TEST_DATABASE_URL='postgres://.../relay_ops_test?sslmode=disable' \
  go test ./internal/store ./internal/reconciliation -run 'Test(UsageImporterBackfillsMissingUpstreamRequestIDAndContinuesSource|UsageImporterCreatesIdempotentAttemptsAndContinuesAfterSourceFailure|RecordUpstreamCostAttemptPreservesGroupScopeAndRejectsConflict)' -count=1
RELAY_OPS_TEST_DATABASE_URL='postgres://.../relay_ops_test?sslmode=disable' go test ./... -count=1
go vet ./...

git diff --check
```

Unlike round 1, relay-ops PostgreSQL-backed store tests were executed against a dedicated temporary database rather than skipped.

## Residual risks

1. HTTP status extraction still consumes formatted account-test error strings. The regression now covers the current English and Antigravity Chinese formats, but a future structured status-bearing error would be more robust.
2. Upstream ID enrichment deliberately uses a conditional second store statement after the identity read. The bind is race-safe (`NULL` or identical value only); a concurrent different bind fails closed with `ErrConflict`.
3. No push, deployment, independent scoped re-review, or production verification was performed in this round.
