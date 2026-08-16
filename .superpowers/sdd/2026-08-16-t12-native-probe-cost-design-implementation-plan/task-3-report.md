# Task 3 implementation report

## Outcome

Added isolated probe aggregation to the existing administrator `account-financial` read model. Native `usage_logs` remains the only source for requests, tokens, cost, user cost, profit, margin, aliases, and `UserBalanceCNY`; probe values are read separately from `account_probe_cost_logs` in the same repeatable-read transaction and never enter native financial formulas.

## Changes

- Extended `AccountFinancialUsageSnapshot` with exact-decimal probe rows and the stable probe-only error channel.
- Extended `FinancialAmounts` with nullable `probe_requests`, `probe_tokens`, `probe_cost`, and `probe_cost_status` fields.
- Extended `AccountFinancialReport` with `probe_data_error` and `probe_error_code`.
- Added same-transaction probe aggregation by immutable `(group_id, account_id)`, including unassigned and historical dimensions.
- Preserved exact decimal sums through `decimal.Decimal` until JSON presentation.
- Implemented `confirmed`, `incomplete`, and successful-no-row `unavailable` semantics.
- On probe query, scan, row iteration, close, or decimal decode failure, rolls back the failed read transaction and returns the already-read native snapshot with `probe_data_error=true`, `probe_error_code="probe_aggregate_unavailable"`, and all probe fields null.
- Added repository, service, handler, and repository integration coverage. No migration, configuration, generated Wire graph, UI, deployment, or production changes were made.

## TDD evidence

RED repository command:

```text
go test ./internal/repository -run 'Test(ReadAccountFinancialUsage|AccountFinancialProbe)' -count=1
```

Failed to compile because `AccountFinancialUsageSnapshot` did not yet contain `ProbeRows`, `ProbeDataError`, or `ProbeErrorCode`.

RED service/handler command:

```text
go test ./internal/service ./internal/handler/admin -run 'TestAccountFinancial.*Probe|TestAccountFinancialReport' -count=1
```

Failed to compile because the snapshot, report, and `FinancialAmounts` probe fields were absent.

## Final verification

```text
go test ./internal/repository -run 'Test(ReadAccountFinancialUsage|AccountFinancialProbe)' -count=1
ok github.com/Wei-Shaw/sub2api/internal/repository

go test ./internal/service ./internal/handler/admin -run 'TestAccountFinancial.*Probe|TestAccountFinancialReport' -count=1
ok github.com/Wei-Shaw/sub2api/internal/service
ok github.com/Wei-Shaw/sub2api/internal/handler/admin

go vet ./internal/repository ./internal/service ./internal/handler/admin
exit 0

git diff --check
exit 0
```

## Remaining concerns

- Verification is intentionally limited to the Task 3 focused tests and required vet/diff checks; no full backend suite, frontend Task 4 work, migration precheck, deployment, or production validation was run.
- `probe_cost` retains exact decimal precision in the backend DTO and is serialized using the existing `shopspring/decimal` JSON behavior; Task 4 owns frontend normalization and two-decimal display.
