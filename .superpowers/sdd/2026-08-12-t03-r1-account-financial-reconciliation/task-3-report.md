# T03-R1 Task 3 Report

Status: READY_FOR_INDEPENDENT_REVIEW. Local implementation and verification are complete. This task has not been merged to main, pushed, deployed, or validated online.

Commit: `b8c5a7f8c0342a7176a5ad232170480c3af6c77b` (`feat: register one-shot upstream cost evidence`)

## Changes

- Added a one-shot `UsageCostEvidenceRegistrar.RegisterOnce(ctx, usageLogID)` and a PostgreSQL `ON CONFLICT (usage_log_id) DO NOTHING` evidence repository.
- Registers only after an official `usage_logs` insert returns `inserted=true` and a generated usage-log ID. Registration errors are logged and do not alter the official usage success result.
- Excludes literal OAuth accounts before any upstream HTTP or evidence row creation.
- Records exact Sub/New API evidence as `confirmed` for finite nonzero cost, `confirmed_zero` for exact zero or blank/null/empty normalized values, and terminal `unavailable` for no request ID, no exact record, upstream endpoint/auth/network/parse failure, or invalid New API unit.
- Keeps evidence matching to the official row's upstream request ID. A local request ID cannot become cost evidence.
- Wires the registrar into both native gateway usage paths. No `usage_logs`, Ent schema/generated files, or migrations were changed.

## RED / GREEN Evidence

- RED: temporarily removed the strict evidence-match guard and ran `go test ./internal/service -run TestUsageCostEvidenceRegistrarRejectsLocalRequestIDFallback -count=1`. It failed as expected because the local-ID-only row was incorrectly recorded as `confirmed` instead of `unavailable`.
- GREEN: restored the guard and the full required targeted commands passed.

## Tests

```text
go test ./internal/service -run 'Test(Registrar|UsageCostEvidence|SubUpstreamCost)' -count=1  PASS
go test ./internal/repository -run TestUsageCostEvidenceRepository -count=1                 PASS
go test ./internal/handler -run 'TestUsageRecord.*(Evidence|Fallback)|TestGateway.*Usage' -count=1 PASS
go test ./cmd/server -run '^$' -count=1                                                     PASS
git diff --check                                                                            PASS
```

## Risk / Follow-up

- The implementation deliberately uses the existing usage-record async task and its stopped-pool synchronous fallback. It does not queue a separate repair job or retry an evidence lookup.
- Exact upstream rows may be unavailable at insertion time; that result is terminal by design and is expected to be projected as an administrator review exception by later tasks.
- Independent task review must verify the insert-before-lookup ordering, a maximum single terminal registration invocation, OAuth exclusion, and absence of read-time lookup/retry or `usage_logs` writes.

## Review Round 1 Fix

Status: READY_FOR_INDEPENDENT_REVIEW. Fix work is local only and remains unmerged, unpushed, undeployed, and not online-validated.

### Addressed Findings

- Restored official usage persistence semantics. `CreateBestEffortWithResult` is an observation extension of the existing best-effort writer: it keeps its 32768-entry batching queue, idempotency behavior, and fresh-context synchronous fallback. The response-after service helper only registers evidence when that writer (or its existing fallback) reports an inserted row and generated ID.
- Added a focused persisted `account_financial_settings.enabled_at` reader. Missing setting, nullable `enabled_at`, and pre-enable records return before upstream HTTP or evidence insert; exact-at and post-enable records are eligible.
- Closed persisted reason codes: `request_id_missing`, `record_not_found`, `endpoint_unsupported`, `credentials_unavailable`, `authentication_rejected`, `request_unavailable`, and `response_unavailable`. Parser-only endpoint/pagination labels normalize before persistence.
- Added stream/nonstream response-after task contract tests; restricted registration to literal `apikey` native ledger accounts. OAuth, Vertex service accounts, Bedrock, and empty/other types make zero upstream/evidence calls. Batch-image Vertex and OpenAI Live remain structurally outside this Sub/New contract.
- Added bounded same-registration request-count tests: two requests for a two-page exact Sub match and three for Sub 404 fallback to New ledger plus unit lookup.

### Round-1 Verification

```text
go test ./internal/service -run 'Test(Registrar|UsageCostEvidence|SubUpstreamCost)' -count=1  PASS
go test ./internal/repository -run TestUsageCostEvidenceRepository -count=1                 PASS
go test ./internal/handler -run 'TestUsageRecord.*(Evidence|Fallback)|TestGateway.*Usage' -count=1 PASS
go test ./cmd/server -run '^$' -count=1                                                     PASS
git diff --check                                                                            PASS
```

The direct fresh-context fallback test is in the unit-tagged record-usage test file and passes when run alone. Running the broader legacy `-tags unit` record-usage suite also reports three existing request-id expectation failures unrelated to this round; the original required non-tag GREEN matrix above is clean.

### Fix Round 2

Status: READY_FOR_INDEPENDENT_REVIEW. Fix work remains local only and is unmerged, unpushed, undeployed, and not online-validated.

- Evidence eligibility now requires positive persisted native ledger identity: a successful Sub billing probe or Sub balance source, or a New API balance source. `apikey` type alone and an `unsupported` probe no longer trigger Sub/New HTTP or evidence rows; direct official-provider API-key accounts are excluded.
- Added real handler-level success-path tests. `ChatCompletions` drives the non-stream Gateway response branch; `Responses` drives a healthy OpenAI SSE stream. Both use a real one-worker usage task pool, capture the official writer result, and assert the registrar receives the generated usage ID after the writer.
- Focused handler tests, service/repository matrices, tagged best-effort fallback, server compile, and `git diff --check` all pass.

### Final Round-1 Check

- Fix commit: `f41f4682c` (`fix: preserve usage evidence registration boundaries`).
- The best-effort batch now treats both `QueryContext` and `rows.Scan`/`rows.Err` failures as batch failures and uses the pre-existing single-write fallback. It never reports a scan failure as a successful non-insert.
- Handler coverage exercises the two response-after submission contracts directly: regular Gateway non-stream task submission and OpenAI stream completion through `submitOpenAIUsageRecordTask`. Those tests prove submission reaches the official task; registrar ordering remains covered at the service write helper.
- Fresh final verification on 2026-08-13 passed the full required matrix, `go test -tags unit ./internal/service -run TestWriteUsageLogBestEffortWithRegistrar_UsesOfficialSyncFallbackBeforeRegistration -count=1`, and the best-effort repository query-shape tests. No schema, Ent-generated, migration, `usage_logs`, Task 4+, production, main, push, or deployment changes were made.
