# SDD ledger — plan: docs/superpowers/plans/2026-08-07-monitor-probe-and-admin-cost-details-implementation-plan.md

Task 1 base: 82cd7cb13
Task 1: complete
Task 1 commit: b32c1fa0d feat: make account monitor probe-only
Task 1 verification: go test ./internal/service -count=1; 55/55 targeted frontend tests; npm run typecheck; git diff --check
Task 1 review: independent reviewer dispatches failed because the selected model was at capacity; final whole-branch review must explicitly cover Task 1.

Task 2 base: b32c1fa0d
Task 2: in progress (partial TDD implementation present and uncommitted; resuming at HTTP contract tests and compile fixes)
Task 2 implementation commit: 7098cb918 feat: expose native upstream request cost details
Task 2 review round 0: spec ❌ / quality needs fixes
Task 2 fix round 1: in progress — persist upstream request ID through the import path; add authoritative price-table estimated evidence or preserve pending when unavailable; net charge/refund rows; add ambiguous fallback regression; enforce consistent query identifiers.
Task 2 fix round 1 commit: 7b627e5ad fix: complete native request cost evidence
Task 2 fix round 1 verification: go test ./... -count=1; go vet ./...; git diff --check (PostgreSQL-backed store regressions skipped because RELAY_OPS_TEST_DATABASE_URL is unset)
Task 2 fix round 1 controller verification: all prior findings have direct code/test coverage in 7b627e5ad; scoped independent re-review dispatch was unavailable, so final whole-branch review must re-check Task 2.
Task 2: complete locally (database-backed integration cases still require RELAY_OPS_TEST_DATABASE_URL during final verification)

Task 3 base: 7b627e5ad
Task 3 implementation commit: cd13ba553 feat: persist upstream request ids separately
Task 3 verification: repository/DTO unit tests passed; migrations and Ent schema tests passed; git diff --check passed.
Task 3 test infrastructure caveat: `go test -tags=unit ./internal/service` is blocked by a pre-existing duplicate `timePtr` helper in ops_health_score_test.go and account_monitor_service_test.go; final verification must resolve or explicitly adjudicate it.
Task 3 review: controller verification complete; independent task review dispatch was unavailable, so final whole-branch review must explicitly cover Task 3.
Task 3: complete locally

Task 4 base: cd13ba553
Task 4: complete
Task 4 implementation commit: 4b145ba6e feat: clarify administrator usage cost details
Task 4 verification: focused Vitest 33/33; frontend lint/typecheck/build; git diff --check
Task 4 review: independent dispatch attempted twice but both failed with upstream 503; final whole-branch review must explicitly re-check Task 4.
