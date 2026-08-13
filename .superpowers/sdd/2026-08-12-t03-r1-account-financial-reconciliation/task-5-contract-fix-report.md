# T03-R1 Task 5 API Contract Fix Report

Status: local implementation complete and ready for independent scoped review. This change has not been merged, pushed, deployed, or production-verified.

## Scope

This narrow follow-up closes the Task 5 contract gap without changing schema, migrations, generated Ent code, routes, frontend, Task 7 stash, `main`, or any production resource.

- The administrator exception list accepts optional RFC3339 `start_time` and `end_time` query values.
- When present, the handler passes them as the existing half-open snapshot range (`created_at >= start_time`, `created_at < end_time`).
- Malformed timestamps and a range where `start_time >= end_time` return HTTP 400 before service access.
- Exception rows and the local evidence compatibility DTO now expose only cost-traceable fields: local request ID, account ID/name/type, source, upstream request ID, upstream billing time/model, and structured Sub/NewAPI cost values.
- No credential, API key, raw upstream response, or request body is added to either response.

## TDD evidence

RED was observed before production changes:

```text
go test ./internal/handler/admin -run 'TestAccountFinancial(ListExceptionsPassesRFC3339HalfOpenRange|ListExceptionsRejectsMalformedOrInvalidRange|ListExceptionsReturnsScopedCostTrace)' -count=1
FAIL: missing SnapshotEntry/Exception trace fields and date validation

go test ./internal/repository -run TestAccountFinancialRepositoryUsageEvidenceIncludesScopedTraceability -count=1
FAIL: missing UsageFinancialEvidence trace fields
```

GREEN verification from `upstream/sub2api/backend`:

```text
go test ./internal/handler/admin -run 'TestAccountFinancial(ListExceptionsPassesRFC3339HalfOpenRange|ListExceptionsRejectsMalformedOrInvalidRange|ListExceptionsReturnsScopedCostTrace)' -count=1
PASS

go test ./internal/repository -run TestAccountFinancialRepositoryUsageEvidenceIncludesScopedTraceability -count=1
PASS

go test ./internal/handler/admin -run TestAccountFinancial -count=1
PASS

go test ./internal/repository -run TestAccountFinancialRepository -count=1
PASS

go test ./internal/service -run TestAccountFinancial -count=1
PASS

go vet ./internal/handler/admin ./internal/repository ./internal/service
PASS

git diff --check
PASS
```

## Files

- `upstream/sub2api/backend/internal/handler/admin/account_financial_handler.go`
- `upstream/sub2api/backend/internal/handler/admin/account_financial_handler_test.go`
- `upstream/sub2api/backend/internal/service/account_financial.go`
- `upstream/sub2api/backend/internal/repository/account_financial_repo.go`
- `upstream/sub2api/backend/internal/repository/account_financial_repo_test.go`

## Remaining gate

An independent, scoped review must confirm the range validation, half-open propagation, local-only trace projection, and absence of sensitive raw fields before Task 7's preserved frontend work is restored.
