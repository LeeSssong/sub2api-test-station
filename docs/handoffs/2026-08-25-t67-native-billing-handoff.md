# T67 Native Billing Restoration Handoff

**Task:** T67 完全恢复 Sub 原生用户扣费
**Status:** READY_FOR_ROOT_REVIEW (refreshed onto main@8e7b79a4b932e63523ebd45c1ae1c73f45f0bb9b)
**Date:** 2026-08-25
**Baseline:** main@8e7b79a4b932e63523ebd45c1ae1c73f45f0bb9b
**Candidate:** codex/t67-native-billing
**Implementation commit:** 81b7b78e4bcf32eb9e4fa461b53117b6d9f3ca2e; refresh merge: a66f79ce3
**Worktree:** /Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t67-native-billing

## Delivered

- Inference billing no longer calls QuotaWalletService.ConsumeUsage.
- Inference billing uses the existing native deductUsageBillingBalance transaction against users.balance, including the guarded deduction and native overdraft result.
- Successful administrator recharge and refund invalidate the target user's balance cache after the wallet transaction succeeds, including idempotent replay responses.
- T55 wallet, ledger, idempotency tables, migrations, API response shape, pricing, routing, and historical usage rows are unchanged.
- Direct regression coverage verifies native overdraft projection, absence of inference usage_consumption ledger rows, and recharge/refund cache invalidation.

## Changed Files

- upstream/sub2api/backend/internal/repository/usage_billing_repo.go
- upstream/sub2api/backend/internal/repository/usage_billing_repo_integration_test.go
- upstream/sub2api/backend/internal/handler/admin/user_handler.go
- upstream/sub2api/backend/internal/handler/admin/quota_wallet_handler_test.go
- docs/superpowers/specs/2026-08-25-t67-native-billing-design.md
- docs/superpowers/plans/2026-08-25-t67-native-billing.md
- docs/handoffs/2026-08-25-t67-native-billing-handoff.md

## Verification

After refresh onto `main@8e7b79a4b932e63523ebd45c1ae1c73f45f0bb9b`, the direct gates were rerun:

- `go test -tags unit ./internal/repository -run 'Test(DeductUsageBillingBalance|ApplyUsageBillingEffects)' -count=1` — passed.
- `go test ./internal/handler/admin -run TestQuotaWalletHandler -count=1` — passed.
- `go test ./internal/service -run TestQuotaWallet -count=1` — passed.
- `go build ./cmd/server` — passed.
- `gofmt` and `git diff --check` — passed.
- Repository integration test — blocked before test execution by the environment error `panic: rootless Docker not found`.

Passed after the implementation commit:

- go test -tags unit ./internal/repository -run Test(DeductUsageBillingBalance|ApplyUsageBillingEffects) -count=1
- go test ./internal/handler/admin -run TestQuotaWalletHandler -count=1
- go test ./internal/service -run TestQuotaWallet -count=1
- go build ./cmd/server
- gofmt on changed Go files
- git diff --check

RED/GREEN evidence:

- Before implementation, TestApplyUsageBillingEffects_FlagsBalanceOverdraft failed because NewBalance was nil.
- Before cache invalidation, recharge and refund handler tests failed because no invalidation call occurred.
- Both regression groups passed after the minimal fixes.

Not verified:

- The repository integration test could not start because the local testcontainers harness reported: panic: rootless Docker not found.
- downtime_required remains unverified until root preflight.
- No production writes, real recharge/refund, model request, push, merge, or deployment were performed.

## Migration and Configuration

- Database migrations: none.
- Configuration changes: none.
- GitHub Actions: none.

## Root Review and Release Requirements

The root release controller must refresh this candidate onto the then-current main, rerun the direct billing, handler, and build gates, inspect the migration hash and release preflight, and issue merge authorization before integration. The candidate must not self-merge, push, deploy, or perform production writes.

Expected release property is downtime_required=false, but only root preflight is authoritative. If preflight returns true, stop before maintenance or cutover and obtain explicit authorization.

Rollback is the existing blue-green rollback to the prior verified production source; no database rollback is required.

## Residual Risk

Historical actual_cost=0 rows remain unchanged by design. The unverified integration test should be rerun in an environment with the repository PostgreSQL and testcontainers prerequisites before root merge.
