# Task 5 scoped fix round 2: financial audit request correlation

- Scope: close the two scoped review findings only: normalize fallback IDs exactly like `RequestLogger`, and prove all five mutation handlers carry correlation through service into audit recording.
- Source precedence: `ctxkey.RequestID`, then `ctxkey.ClientRequestID`, then `X-Request-ID` for non-standard/test mounts.
- Normalization: exported `middleware.NormalizeCorrelationID` delegates to the unchanged middleware implementation: trim, remove invalid UTF-8, require non-empty, and reject values over 64 bytes. Invalid candidates fall through; no valid candidate returns empty.
- Mutations covered: single review, selected review, filtered review, OAuth daily cost, and today override.
- Handler-level tests construct the real `AccountFinancialService` and `AccountFinancialAudit`, invoke each Gin handler, and assert the recorder receives the correlation ID.
- No schema, route, upstream HTTP, main, production, push, or deployment changes.
- TDD evidence: the normalization regression initially failed with a valid-UTF-8 NUL fixture; the corrected invalid-byte fixture passes against the shared helper.
- Validation: focused handler/service/middleware tests with `GOCACHE=/tmp/sub2api-go-build`; `git diff --check`.
- Commit blocker: this session cannot create the shared Git metadata lock at `/Users/gongtengxinwen/Documents/sub2api搭建/.git/worktrees/sub2api搭建/index.lock` (`Operation not permitted`). Verified changes remain uncommitted in the named candidate worktree; no out-of-scope workaround was attempted.
