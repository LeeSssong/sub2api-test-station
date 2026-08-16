# T13 Task 1 Report

- Commit: `062532c856b75a7fe4fde086124b715eccf9cf49`
- Scope: NewAPI exact usage-row `other.group_ratio` parsing and Task 1 eligibility predicate.
- Files:
  - `upstream/sub2api/backend/internal/service/sub_upstream_cost.go`
  - `upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go`

## Tests

- RED observed before implementation: focused T13 tests failed to compile because the structured field and eligibility predicate were absent.
- GREEN focused tests:
  - `go test ./internal/service -run 'TestNewAPIUsageRecordParsesTopLevelGroupRatio|TestNewAPIUsageRecordEligibilityRequiresExactSuccessfulNewAPIUsage'`
- Existing NewAPI/evidence tests:
  - `go test ./internal/service -run 'TestSubUpstreamCostService|TestNewAPI|Test.*NewAPI'`
- Direct service package:
  - `go test ./internal/service`
- `gofmt` and `git diff --check` passed.

## Implementation

- Parses `other` as either the NewAPI JSON string form or a compatibility JSON object form.
- Keeps only typed `GroupRatio` and validation state; it does not retain the complete `other` payload.
- Accepts finite numeric values in `[0, 100]`, including `0`; rejects missing/null/string/boolean/negative/over-limit/non-finite values and nested non-top-level fields.
- Eligibility requires persisted usage ID, API-key account, NewAPI identity evidence, exact request/upstream ID match, valid group ratio, and a non-refund row.
- A successful native billing probe takes precedence and blocks NewAPI registration eligibility.

## Concerns

- Task 2 still needs to connect this predicate to the CAS claim/query/complete/release flow and reuse matched records without duplicate upstream requests.
- The existing NewAPI cost evidence path remains quota-based; this Task 1 change only exposes the typed group ratio for later registration and does not alter cost accounting.
- No production, deployment, migration, or global ledger files were modified.
