# Task 3 Fix Round 4 Report

Status: implemented and locally verified; pending independent review.

Round 4 replaces the broad half-open context bypass with an account-and-canonical-model lease. Only the matching expired model-runtime block is ignored for the leased probe. Account-level runtime blocks and the normal privacy, schedulability, platform, group, quarantine, quota, parent-health, model, channel, capability, transport, load-balance, slot, freshness, DB recheck, compact, and profit checks remain active.

The lease is acquired immediately before normal slot acquisition. All pre-`Forward` rejection and abandonment paths release it through the selection's idempotent `ReleaseFunc`; half-open selection cannot return a wait plan. Responses and Messages complete the lease from the actual `Forward` result before deferred selection release, so failures do not receive a duplicate half-open state transition.

Standalone tests cover eligibility rejection without lease acquisition, slot and DB-recheck release, profit-gate release, one-probe ownership, success completion, and failed-forward streak accounting. Logical test timestamps are no later than 2026-08-08.

Verification passed:

```text
go test ./internal/service -run 'Test(OpenAIHalfOpenScheduler|OpenAIHalfOpenHandler|OpenAIModelTransient|OpenAI.*Scheduler)' -count=1
go test ./internal/handler -run 'Test(OpenAI.*(Retry|Failover|Sticky|Scheduler|Messages)|.*Cache.*Preservation|TestHandleOpenAITransientError_AttemptMetadataDefersFailureToHandler)' -count=1
go test ./internal/service -run 'Test(OpenAI.*(Retry|Failover|Sticky|Scheduler|Messages)|.*Cache.*Preservation|TestHandleOpenAITransientError_AttemptMetadataDefersFailureToHandler)' -count=1
go test -race ./internal/handler -run 'Test(OpenAI.*(Retry|Failover|Sticky|Scheduler|Messages)|.*Cache.*Preservation|TestHandleOpenAITransientError_AttemptMetadataDefersFailureToHandler|TestOpenAIHalfOpenHandler_)' -count=1
go test -race ./internal/service -run 'Test(OpenAI.*(Retry|Failover|Sticky|Scheduler|Messages)|.*Cache.*Preservation|TestHandleOpenAITransientError_AttemptMetadataDefersFailureToHandler|TestOpenAIHalfOpenHandler_)' -count=1
go vet ./internal/handler ./internal/service
git diff --check
```

No push or deployment was performed.
