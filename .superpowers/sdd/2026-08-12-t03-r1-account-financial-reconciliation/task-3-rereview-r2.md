# T03-R1 Task 3 Scoped Re-review Round 2

Review inputs:

- Task brief: `.superpowers/sdd/2026-08-12-t03-r1-account-financial-reconciliation/task-3-brief.md`
- Implementer report: `.superpowers/sdd/2026-08-12-t03-r1-account-financial-reconciliation/task-3-report.md`
- Previous scoped review: `.superpowers/sdd/2026-08-12-t03-r1-account-financial-reconciliation/task-3-rereview-r1.md`
- Complete fix package: `.superpowers/sdd/2026-08-12-t03-r1-account-financial-reconciliation/review-f41f4682c..70a6d8970.diff`
- Reviewed HEAD: `70a6d8970`

This review is limited to the two open Important findings from round 1 and
Critical/Important regressions introduced by the round-2 fix. No implementation
file was modified.

## Verdicts

- Spec Compliance: ✅
- Code Quality: ✅
- Open finding count: 0

Both round-1 Important findings are addressed. No new Critical or Important
regression was identified in the scoped fix.

## Finding 1: Real Handler Success Paths and Writer-to-Registrar Ordering

**ADDRESSED**

`upstream/sub2api/backend/internal/handler/usage_record_submit_task_test.go:267`

`TestUsageRecordNonStreamSuccessfulChatCompletionsHandlerCallsGatewayRecordUsage`
constructs a concrete `GatewayService` and `GatewayHandler`, invokes
`GatewayHandler.ChatCompletions`, drives a successful non-stream response, and
asserts the parsed response plus the official usage writer input. The captured
usage row proves the real `GatewayService.RecordUsage` path received the
non-stream result, tokens, account, and inbound endpoint.

`upstream/sub2api/backend/internal/handler/usage_record_submit_task_test.go:329`

`TestUsageRecordStreamSuccessfulResponsesHandlerCallsOpenAIRecordUsage`
constructs a concrete `OpenAIGatewayService` and `OpenAIGatewayHandler`, invokes
`OpenAIGatewayHandler.Responses`, drives a healthy SSE stream through
`response.completed`, and asserts the streamed response plus the official usage
writer input. The captured row proves the real `OpenAIGatewayService.RecordUsage`
path received the streaming result, tokens, account, and normalized inbound
endpoint.

Both tests use a real one-worker `UsageRecordWorkerPool`, so they also cover the
actual response-after submission boundary rather than calling
`submitUsageRecordTask` or `submitOpenAIUsageRecordTask` directly. Their concrete
services are configured with a repository implementing
`CreateBestEffortWithResult`; that writer returns generated usage ID `701`, and
the registrar must receive exactly `701`. This is a causal result dependency:
the registrar ID comes from the writer result, not from the pre-insert log. It
therefore covers
`handler success -> RecordUsage -> CreateBestEffortWithResult -> RegisterOnce`
through both service implementations.

The production ordering remains explicit at
`upstream/sub2api/backend/internal/service/gateway_usage_billing.go:643`: the
writer completes first, then lines 664-666 call `RegisterOnce` only for
`Inserted=true` and a positive generated ID. The focused fresh-context fallback
test separately proves the official synchronous fallback supplies its generated
ID before registration.

## Finding 2: Positive Native Ledger Eligibility and Consistent Source

**ADDRESSED**

`upstream/sub2api/backend/internal/service/sub_upstream_cost.go:70`

`usageCostLedgerForAccount` is now the single evidence-persistence identity
decision. It first requires literal `apikey`, then accepts only persisted positive
native observations:

- persisted balance source `sub2api` -> Sub ledger;
- persisted balance source `newapi` -> New API ledger;
- successful persisted billing probe -> Sub ledger;
- otherwise -> unknown/ineligible.

An `unsupported` probe no longer implies New API for evidence persistence, and
the former Sub-404-to-New fallback was removed from `lookupEvidence`. The older
bounded discovery behavior is deliberately isolated to administrator detail
lookup in `isNewAPIUsageLedgerForDetail`, so it cannot create asynchronous
evidence.

`upstream/sub2api/backend/internal/service/usage_cost_evidence.go:123`

Eligibility and source selection both derive from the same ledger identity.
`RegisterOnce` returns before activation lookup, upstream HTTP, or evidence
creation when the identity is unknown. For eligible records,
`lookupEvidence` and `evidenceSourceForAccount` make the same Sub/New decision.

`upstream/sub2api/backend/internal/service/usage_cost_evidence_test.go:301`

The registrar test explicitly proves zero billing HTTP and zero evidence rows
for direct official OpenAI, Anthropic, Gemini, and Grok API-key accounts,
unsupported-only metadata, and unknown metadata. The positive identity table at
line 331 covers Sub probe, Sub balance, New balance, precedence when both signals
exist, eligibility, and evidence source consistency. Existing non-API-key tests
continue to cover OAuth, service account/Vertex, Bedrock, and empty account type.

## Fix-introduced Regressions

No new Critical or Important regression was found.

The round-2 production change is narrowly limited to evidence ledger identity
and the evidence lookup branch. The existing administrator detail lookup keeps
its prior bounded discovery behavior under a separate helper. No schema,
migration, Ent-generated, `usage_logs`, Task 4+, release, or deployment behavior
was introduced by the scoped code fix.

## Residual Warnings

- The PostgreSQL `ON CONFLICT (usage_log_id) DO NOTHING` behavior remains covered
  with repository mocks and the Task 2 unique constraint, but there is still no
  fresh concurrent PostgreSQL integration test proving simultaneous `CreateOnce`
  calls collapse to one row. This is the same non-blocking residual test gap from
  round 1.
- The handler tests use a repository test double at the
  `CreateBestEffortWithResult` boundary rather than a live PostgreSQL repository.
  This is appropriate for handler-path coverage because repository batching,
  query shape, fallback, and conflict behavior are covered separately; it does
  not leave the original helper-only gap open.

## Fresh Verification

```text
go test ./internal/handler -run 'TestUsageRecord(NonStreamSuccessfulChatCompletionsHandlerCallsGatewayRecordUsage|StreamSuccessfulResponsesHandlerCallsOpenAIRecordUsage)$' -count=1
PASS

go test ./internal/service -run 'TestUsageCost(EvidenceRegistrarRequiresKnownNativeLedger|LedgerForAccountUsesPositiveNativeEvidence)$' -count=1
PASS

go test ./internal/service -run 'Test(Registrar|UsageCostEvidence|SubUpstreamCost)' -count=1
PASS

go test ./internal/repository -run 'TestUsageCostEvidenceRepository|TestAccountFinancialActivationRepository|TestUsageLogRepositoryCreateBestEffort' -count=1
PASS

go test ./internal/handler -run 'TestUsageRecord.*(Evidence|Fallback)|TestGateway.*Usage' -count=1
PASS

go test -tags unit ./internal/service -run TestWriteUsageLogBestEffortWithRegistrar_UsesOfficialSyncFallbackBeforeRegistration -count=1
PASS

go test ./cmd/server -run '^$' -count=1
PASS

git diff --check f41f4682c575231f72a82ec3c98fa44a7a12b661..70a6d8970
PASS
```
