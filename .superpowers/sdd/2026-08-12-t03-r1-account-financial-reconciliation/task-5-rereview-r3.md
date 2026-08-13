# T03-R1 Task 5 Scoped Re-review Round 3

## Review scope

- Reviewed diff: `89597a5c455e808825613964f45ff101d9381610..24e00a90486e915feefd0d0eb71acf51fe56351b`
- Scoped finding: the five financial mutation handler tests previously mounted parameterized handlers on concrete paths, causing `one`, `oauth`, and `override` to return HTTP 400 before parameter parsing could reach the service and audit recorder.
- Regression scope: preserve the fix-round-2 request-correlation contract for context request ID, client request ID, header fallback, valid UTF-8/64-byte normalization, and all five mutations.
- No implementation file was modified by this review.

## Verdicts

- **Spec Compliance: APPROVE**
- **Code Quality: APPROVE**
- **Open finding count: 0**

The round-3 fix closes the only scoped finding. No new Critical or Important regression was identified.

## Finding verification

`TestFinancialMutationHandlersPersistCorrelationThroughService` now separates each Gin route template from its concrete request path:

- single review: `/one/:usageLogID/review` with `/one/1/review`;
- selected review: `/selected` with `/selected`;
- filtered review: `/filtered` with `/filtered`;
- OAuth cost: `/oauth/:id` with `/oauth/4`;
- today override: `/override/:id` with `/override/5`.

This makes Gin populate `c.Param("usageLogID")` and `c.Param("id")` on the three parameterized routes. Each case invokes the real `AccountFinancialHandler`, which parses the route/body and calls a real `AccountFinancialService` configured with `AccountFinancialAudit`. The repository double returns a successful mutation result, and the test requires both HTTP 200 and exactly one newly appended recorder entry carrying the expected request ID. Therefore the assertions cannot pass through the former early-400 path or without traversing the service and audit recorder.

## Request-correlation regression check

The fix-round-2 behavior remains intact:

- `financialRequestID` checks `ctxkey.RequestID`, then `ctxkey.ClientRequestID`, then `X-Request-ID`.
- Every candidate passes through exported `middleware.NormalizeCorrelationID`, which delegates to the same `normalizeCorrelationID` used by `RequestLogger`.
- Normalization trims whitespace, removes invalid UTF-8 bytes, rejects empty values, and rejects values longer than 64 bytes; an invalid higher-priority candidate falls through to the next source.
- Single review, selected review, filtered review, OAuth daily cost, and today override all place the resulting request ID into their service input.
- The five route-aware test cases prove that each successful mutation reaches `AccountFinancialAudit.Record` and persists that request ID in the emitted audit log.

## Fresh verification

From `upstream/sub2api/backend`:

```text
GOCACHE=/tmp/sub2api-go-build go test ./internal/handler/admin -run '^TestFinancialMutationHandlersPersistCorrelationThroughService$' -count=1 -v
PASS (all five subtests)

GOCACHE=/tmp/sub2api-go-build go test ./internal/handler/admin -count=1
PASS

GOCACHE=/tmp/sub2api-go-build go test ./internal/service -run '^TestAccountFinancial(Service|Audit)' -count=1
PASS

GOCACHE=/tmp/sub2api-go-build go test ./internal/server/middleware -run '^TestRequestLogger' -count=1
PASS

GOCACHE=/tmp/sub2api-go-build go vet ./internal/handler/admin ./internal/service ./internal/server/middleware
PASS

git diff --check 89597a5c455e808825613964f45ff101d9381610..24e00a90486e915feefd0d0eb71acf51fe56351b
PASS
```

## Open findings

**0 Critical, 0 Important, 0 Minor.**
