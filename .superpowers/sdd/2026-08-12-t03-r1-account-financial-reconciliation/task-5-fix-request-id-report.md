# Task 5 scoped fix round 2: financial audit request correlation

- Scope: close the two scoped review findings only: normalize fallback IDs exactly like `RequestLogger`, and prove all five mutation handlers carry correlation through service into audit recording.
- Source precedence: `ctxkey.RequestID`, then `ctxkey.ClientRequestID`, then `X-Request-ID` for non-standard/test mounts.
- Normalization: exported `middleware.NormalizeCorrelationID` delegates to the unchanged middleware implementation: trim, remove invalid UTF-8, require non-empty, and reject values over 64 bytes. Invalid candidates fall through; no valid candidate returns empty.
- Mutations covered: single review, selected review, filtered review, OAuth daily cost, and today override.
- Handler-level tests construct the real `AccountFinancialService` and `AccountFinancialAudit`, invoke each Gin handler, and assert the recorder receives the correlation ID.
- No schema, route, upstream HTTP, main, production, push, or deployment changes.
- TDD evidence: the normalization regression initially failed with a valid-UTF-8 NUL fixture; the corrected invalid-byte fixture passes against the shared helper.
- Validation: focused handler/service/middleware tests with `GOCACHE=/tmp/sub2api-go-build`; `git diff --check`.

## Fix round 3: route-aware handler coverage

- Review finding: the five-handler correlation test registered parameterized handlers under concrete paths (`/one/1/review`, `/oauth/4`, `/override/5`). Gin therefore left `c.Param(...)` empty and three cases returned HTTP 400 before reaching the service/audit recorder.
- Fix: split each case into a Gin route template and request path, using `:usageLogID`/`:id` templates for the parameterized handlers.
- RED evidence: `go test ./internal/handler/admin -run '^TestFinancialMutationHandlersPersistCorrelationThroughService$' -count=1` failed with HTTP 400 for `one`, `oauth`, and `override` before the fix.
- Validation after fix: `go test ./internal/handler/admin -count=1`, `go test ./internal/service -run '^TestAccountFinancial(Service|Audit)' -count=1`, `go test ./internal/server/middleware -run '^TestRequestLogger' -count=1`, and `git diff --check` all pass.
