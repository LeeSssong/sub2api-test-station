# T03-R1 Final Branch Review

日期：2026-08-13（Asia/Shanghai）

## Review scope

- Base: `19492c57da24270eb2b3e9b5d9727c2865aebb9e`.
- Reviewed Tasks 1–8 through the SDD ledger, task reports, Task 8 integration evidence, full branch diff, migration metadata, and focused build/test output.
- The review found one blocker: `UsageHandler.GetUpstreamCost` retained a nil-financial-service fallback to `SubUpstreamCostService.GetByUsageID`, allowing administrator read-time upstream HTTP.

## Blocker fix

- Removed the executable `GetByUsageID` fallback and the stored upstream-cost service dependency from the administrator usage handler.
- The compatibility endpoint now reads only `AccountFinancialService.GetUsageEvidence`; if the local service is absent it returns HTTP 500 fail-closed.
- RED: the two legacy synchronous-read tests failed with expected 500 instead of their old 200 expectations after the production change.
- GREEN: updated regressions require fail-closed behavior and prove a configured test upstream receives zero HTTP calls.

## Verification

- `go test ./internal/handler/admin -run 'TestAdminUsageGetUpstreamCost|TestAccountFinancial|TestAdminUsage.*(Evidence|UpstreamCost|Exception)' -count=1` — PASS.
- `go test ./internal/handler -run 'TestUsageRecord.*(Evidence|Fallback)|TestGateway.*Usage|TestUsage.*Detail' -count=1` — PASS.
- `go test ./internal/server/routes -run 'Test.*AccountFinancial|Test.*AdminUsage' -count=1` — PASS (package reported no matching tests).
- `go vet ./internal/service ./internal/repository ./internal/handler ./internal/handler/admin` — PASS.
- `make build` — PASS.
- Static `GetByUsageID(` scan over frontend and administrator handler — zero matches.
- Task 8 fresh matrix remains PASS: backend migration/repository/service/admin-handler/handler tests; frontend 6 files / 53 tests; typecheck; production build; diff-check.

## Contract checks

- Migration 222 SHA-256: `47f786d6b2b020d0211a17d4ccd2bc6bb3774a315f483fdc0ac45657c9ee738e`.
- `usage_log.go` SHA-256: `5adc345325675a4d439524f20603412f69971c3f024913240996df7eb7e461f9`.
- No new GitHub Actions, external-primary, xingqiao, relay-ops, T05, production, push, or deployment paths.
- Ordinary users receive no evidence, review, upstream-cost, or profit projection; administrator reads use local persisted evidence only.

## Verdict

- Spec Compliance: **APPROVE**
- Code Quality: **APPROVE**
- Open findings: **0**
- Status: **READY_FOR_ROOT_REVIEW**
- `downtime_required=unverified until root preflight`
