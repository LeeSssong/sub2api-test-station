# T12 Task 2 Report — explicit native probe cost capture

## Scope delivered

- Added the narrow `AccountProbeCostRecorder` boundary and its native-pricing
  implementation. It calls `BillingService.CalculateCostUnified` only for
  complete observed usage, then appends an immutable `account_probe_cost_logs`
  row through the Task 1 repository.
- Added explicit `manual`, `monitor`, and `scheduled` wrappers. The legacy
  `TestAccountConnection` and `RunTestBackground` paths remain unclassified,
  so recovery-only callers do not create probe ledger rows.
- Attached a request-local observer to the native Claude, Gemini, OpenAI Chat
  Completions, and OpenAI Responses stream parsers. It observes provider usage
  without changing the SSE event contract. Missing usage is `unknown` with a
  null cost; pricing failure is `probe_pricing_unavailable` with a null cost.
- Recorder/append failures are logged as `probe_cost_append_failed` and do not
  change the original probe result.

## Explicit non-effects

- No `usage_logs`, user balance, API-key, account-cost, routing, scheduler
  recovery, migration, frontend, `docs/project`, main, push, deployment, or
  production changes.
- The ledger input contains no user or API-key identity.

## Focused verification (fresh)

```text
go test ./internal/service -run 'TestAccountProbe|TestAccountMonitorProbe' -count=1
PASS

go test ./internal/handler/admin -run 'Test.*Account.*Test' -count=1
PASS (no matching focused handler tests in this package)

go vet ./internal/service ./internal/handler/admin
PASS

go test ./cmd/server -run '^$'
PASS

git diff --check
PASS
```

## Review handoff

Candidate is ready for an independent Task 2 review only. Reviewer should
verify the three source labels, the unclassified recovery path, failure-open
append behavior, no user-billing side effects, and the generated Wire graph.

## P1 resolved-model fix — 2026-08-16

```text
RED
cd upstream/sub2api/backend && go test ./internal/service -run 'TestAccountProbeRecordsResolved(DefaultUpstreamModel|MappedUpstreamModel)$' -count=1
FAIL as expected: TestAccountProbeRecordsResolvedDefaultUpstreamModel showed
the upstream request model was gpt-5.4 while ProbeRecordInput.Model was empty.

GREEN
cd upstream/sub2api/backend && go test ./internal/service -run 'TestAccountProbeRecordsResolved(DefaultUpstreamModel|MappedUpstreamModel)$' -count=1
PASS

Focused service coverage
cd upstream/sub2api/backend && go test ./internal/service -run 'TestAccountProbe' -count=1
PASS

Formatting / diff
cd upstream/sub2api/backend && gofmt -d internal/service/account_probe_cost_service.go internal/service/account_test_service.go internal/service/account_probe_cost_test.go
PASS (no output)
cd upstream/sub2api/backend && git diff --check
PASS
```

Implementation commit: `55ccfdeef98f1f48a2d63c7e4106f79fa5df8f3a`.
