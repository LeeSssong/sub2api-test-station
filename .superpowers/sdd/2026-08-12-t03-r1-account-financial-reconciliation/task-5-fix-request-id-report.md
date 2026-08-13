# Task 5 scoped fix: financial audit request correlation

- Scope: pass the server request correlation ID from all administrator financial mutation handlers into their service inputs.
- Source: `ctxkey.RequestID`, with `ctxkey.ClientRequestID` and `X-Request-ID` fallback for non-standard/test mounts.
- Mutations covered: single review, selected review, filtered review, OAuth daily cost, and today override.
- No schema, route, upstream HTTP, main, production, push, or deployment changes.
- Tests: handler correlation precedence/fallback tests; existing admin handler and account financial audit service tests.
- Validation: `go test ./internal/handler/admin -run 'TestAccountFinancial|TestFinancialRequestID' -count=1`; `go test ./internal/service -run 'TestAccountFinancialServiceAudit|TestAccountFinancialAudit' -count=1`; `git diff --check`.
