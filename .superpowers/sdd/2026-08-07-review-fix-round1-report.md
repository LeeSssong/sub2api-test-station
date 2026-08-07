# Whole-branch review fix round 1 report

Date: 2026-08-07
Branch: `codex/fix-relay-admin-auth`
Base: `a93ecedbd7316a962e4fee1518b1a33c3f14c1ef`
Implementation commit: `07f47bd61` (`fix: address monitor and cost evidence review`)

## Status

All four Important findings from the whole-branch review are fixed and verified locally. The project remains **in progress** because this branch has not been pushed, deployed, or verified in production.

## Finding 1 — failed probes polluted latency and TTFT aggregates

### Root cause

`ListAggregates` and `LoadAggregate` counted every non-null `ttft_ms` and `latency_ms`, and their percentile filters also accepted failed rows. `SuccessSampleCount` was populated with `COUNT(*)`. Failed probes intentionally retain total latency for diagnostics, so aggregation had to exclude those values at the SQL boundary.

### RED

Command:

```text
cd upstream/sub2api/backend
go test ./internal/repository -run TestAccountMonitorRepositoryProbeLatencyAggregatesUseOnlySuccessfulProbes -count=1
```

Result: failed as expected. sqlmock reported that the actual query used `COUNT(*)`, `COUNT(ttft_ms)`, `COUNT(latency_ms)`, and percentile filters without `status = 'success'`.

### GREEN

Command:

```text
go test ./internal/repository -run 'TestAccountMonitorRepository(ProbeLatencyAggregatesUseOnlySuccessfulProbes|ReadsAggregatesAndDeletesExpiredHistory)' -count=1
```

Result:

```text
ok github.com/Wei-Shaw/sub2api/internal/repository
```

### Fix

Both probe aggregate queries now derive `SuccessSampleCount`, TTFT/latency sample counts, and all four percentile values exclusively from `status = 'success'`. Raw failed latency remains persisted for diagnostics.

## Finding 2 — classifier-derived fatal probe errors were not immediately unavailable

### Root cause

The account-test path returns messages such as `API returned 401`, `No API key available`, and balance/quota errors. The classifier either mislabeled these as `malformed_stream`/`account_test_error` or returned `balance_exhausted`, while fatal availability checked only a limited error-code substring list and ignored HTTP status.

### RED

Command:

```text
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitor(ProbeResultClassifiesFatalErrorsWithHTTPStatus|ClassifierFatalErrorsAreUnavailableImmediately)' -count=1
```

Result: failed as expected. Actual 401/500 messages were classified as `malformed_stream` with no HTTP status; missing API key/authentication messages were `account_test_error`; `balance_exhausted` remained `abnormal` instead of `unavailable`.

### GREEN

Command:

```text
go test ./internal/service -run 'TestAccountMonitor(ProbeResultClassifiesFatalErrorsWithHTTPStatus|ClassifierFatalErrorsAreUnavailableImmediately|ProbeProjectionUsesOnlyFreshProbeEvidence)' -count=1
```

Result:

```text
ok github.com/Wei-Shaw/sub2api/internal/service
```

### Fix

Probe results now extract HTTP status only from HTTP/status-oriented error contexts, classify no-status credential failures as `invalid_auth`, retain generic HTTP failures as `http_error`, and immediately mark balance exhaustion, explicit authentication failures, and HTTP 401/402/403 unavailable. HTTP 500 remains abnormal until the consecutive-failure threshold is reached.

## Finding 3 — relay-ops usage importer dropped `upstream_request_id`

### Root cause

The relay-ops `sub2api.UsageLog` DTO did not expose `upstream_request_id`, and `UsageImporter` populated only `LocalRequestID`, even though `AttemptInput` and persistence already supported a separate upstream ID.

### RED

Command:

```text
cd relay-ops-service
go test ./internal/sub2api ./internal/reconciliation -run 'Test(ReaderListsUsageLogsAcrossExactWindowPages|UsageImporterCreatesIdempotentAttemptsAndContinuesAfterSourceFailure)' -count=1
```

Result: failed as expected at compile time: `UsageLog` had no `UpstreamRequestID` field and decoded logs exposed no such member.

### GREEN

Command:

```text
go test ./internal/sub2api ./internal/reconciliation -run 'Test(ReaderListsUsageLogsAcrossExactWindowPages|UsageImporterCreatesIdempotentAttemptsAndContinuesAfterSourceFailure|UsageImporterPreservesNilGroupScope)' -count=1
```

Result:

```text
ok example.invalid/relay-ops-service/internal/sub2api
ok example.invalid/relay-ops-service/internal/reconciliation
```

### Fix

The DTO now decodes `upstream_request_id`; the importer trims and copies it into `AttemptInput.UpstreamRequestID` while preserving `request_id` as the local request ID.

## Finding 4 — owned-account manual allocation displayed zero or price-table cost

### Root cause

`ReadRequestCostDetail` put a manual allocation into `UpstreamActualCost`, then could independently populate `UpstreamStandardCost` from a pricing snapshot. The frontend correctly reads estimated evidence from `upstream_standard_cost`, so owned-account allocation rendered as zero or an unrelated price-table estimate.

### RED

Command:

```text
cd relay-ops-service
go test ./internal/store -run 'TestApplyRequestCostLedgerEvidence' -count=1
```

Result: failed as expected because the tested ledger-evidence projector did not exist; the existing store path had no isolated contract that could project manual allocation into the estimated field.

### GREEN

Commands:

```text
go test ./internal/store -run 'TestApplyRequestCostLedgerEvidence|TestReadRequestCostDetail(NetsNativeChargeAndRefundRows|UsesStoredUpstreamPriceTableEvidence|KeepsAmbiguousUpstreamIDPending)' -count=1

cd upstream/sub2api/frontend
pnpm exec vitest run src/components/usage/__tests__/usageDetail.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts
```

Results:

```text
ok example.invalid/relay-ops-service/internal/store
2 frontend test files passed; 33 tests passed
```

### Fix

Ledger evidence now has explicit precedence and field semantics:

- Native ledger rows set `upstream_actual_cost`, `confirmed`.
- Manual owned-account allocation sets `upstream_standard_cost`, `estimated`, and returns before price-table lookup.
- Price-table estimates are used only when no native or manual ledger evidence exists.
- Missing evidence leaves both amounts zero with `pending`.

Frontend regressions confirm that an owned allocation payload renders the allocated amount, estimated margin, owned-allocation source, and estimated badge.

## Full verification

All commands below completed with exit code 0:

```text
cd upstream/sub2api/backend && go test ./... -count=1
cd upstream/sub2api/backend && go vet ./...

cd relay-ops-service && go test ./... -count=1
cd relay-ops-service && go vet ./...

cd upstream/sub2api/frontend
pnpm exec vitest run
pnpm run lint:check
pnpm run typecheck
pnpm run build

git diff --check
```

Frontend full-suite result: 229 test files passed, 1635 tests passed. The production build completed successfully with existing non-fatal chunk-size/dynamic-import warnings.

## Unresolved risks

1. `RELAY_OPS_TEST_DATABASE_URL` was not set, so PostgreSQL-backed store integration tests were skipped by the existing test harness. The always-running pure projection tests, relay-ops full unit suite, and existing persistence/query tests cover the field contract locally, but a real PostgreSQL run remains required before deployment.
2. HTTP status extraction still depends on stable account-test error-message context (`returned`, `status`, `HTTP`, or `failed`). A future structured error type would be more robust than string parsing.
3. No production push, deployment, or online verification was performed in this round. The project progress ledger therefore remains `进行中`.
