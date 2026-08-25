
# T67 Native Billing Restoration Implementation Plan

> For agentic workers: implement this plan task-by-task with a fresh implementation pass. Steps use checkbox syntax.

Goal: Restore Sub native users.balance billing for inference and invalidate the native balance cache after manual quota mutations.

Architecture: Keep T55 wallet and ledger for administrator recharge/refund audit and users.balance compatibility projection. Remove wallet consumption from the inference billing repository and call the existing atomic native balance deduction helper. Reuse the existing BillingCache dependency in the admin user handler to invalidate billing:balance:<user_id> after a successful recharge or refund.

Tech stack: Go, PostgreSQL transactions, sqlmock, Ent repositories, Gin handler tests.

Spec: docs/superpowers/specs/2026-08-25-t67-native-billing-design.md

## Global Constraints

- Inference billing uses users.balance as its runtime fact source.
- T55 wallet, ledger, idempotency tables, and migrations remain intact.
- Manual recharge/refund success invalidates the user's balance cache; failed mutations do not.
- Historical actual_cost=0 rows are not backfilled, deleted, or recharged.
- No migration, configuration schema, API schema, payment channel, or GitHub Actions change.
- Do not modify main, push, deploy, or perform production writes from this candidate.
- T66-R1 has priority for the single integration/deployment lane.

---

### Task 1: Restore Native Inference Balance Deduction

Files:
- Modify: upstream/sub2api/backend/internal/repository/usage_billing_repo.go
- Modify: upstream/sub2api/backend/internal/repository/usage_billing_repo_unit_test.go
- Modify: upstream/sub2api/backend/internal/repository/usage_billing_repo_integration_test.go

Interfaces:
- Consumes: service.UsageBillingCommand, deductUsageBillingBalance, and service.UsageBillingApplyResult.
- Produces: usageBillingRepository.Apply that performs native balance deduction inside the existing transaction and returns NewBalance plus BalanceOverdrafted. Constructor signature remains compatible with NewUsageBillingRepository(client, sqlDB).

- [ ] Step 1: Add the RED assertion for native billing effects.

Extend TestApplyUsageBillingEffects_FlagsBalanceOverdraft so BalanceCost=10 expects the guarded native update to return no rows, the compatibility overdraft update to return -5, NewBalance=-5, and BalanceOverdrafted=true. Keep the existing SQL constants and use usageBillingRepository{} without a wallet dependency.

Run:
    cd upstream/sub2api/backend
    go test -tags unit ./internal/repository -run 'TestApplyUsageBillingEffects_FlagsBalanceOverdraft|TestDeductUsageBillingBalance_' -count=1

Expected: FAIL because the current T55 applyUsageBillingEffects no longer calls deductUsageBillingBalance.

- [ ] Step 2: Add the RED integration boundary for wallet separation.

Extend TestUsageBillingRepositoryApply_DeduplicatesBalanceBilling with a query for user_quota_ledger_entries filtered by the test user and record_type='usage_consumption'. Assert zero rows after repo.Apply while retaining the exact users.balance deduction and one usage_billing_dedup row assertions.

Run:
    cd upstream/sub2api/backend
    go test -tags integration ./internal/repository -run TestUsageBillingRepositoryApply_DeduplicatesBalanceBilling -count=1

Expected: FAIL on the new ledger assertion because the current repository delegates inference consumption to QuotaWalletService.

- [ ] Step 3: Restore the minimal native implementation.

In usage_billing_repo.go:
1. Remove the wallet service.QuotaWalletService field from usageBillingRepository.
2. Keep NewUsageBillingRepository(client, sqlDB) source-compatible for Wire generation, but do not construct a quota wallet in this repository.
3. Remove the shopspring/decimal import used only by wallet consumption.
4. In applyUsageBillingEffects, before the other balance-dependent effects, add:
       if cmd.BalanceCost > 0 {
           newBalance, sufficient, err := deductUsageBillingBalance(ctx, tx, cmd.UserID, cmd.BalanceCost)
           if err != nil {
               return err
           }
           result.NewBalance = &newBalance
           result.BalanceOverdrafted = !sufficient
       }
5. Preserve usage billing deduplication, subscription billing, API Key quota updates, rate-limit updates, account quota updates, commit behavior, and the native overdraft helper unchanged.

Run:
    cd upstream/sub2api/backend
    gofmt -w internal/repository/usage_billing_repo.go internal/repository/usage_billing_repo_unit_test.go internal/repository/usage_billing_repo_integration_test.go
    go test -tags unit ./internal/repository -run 'TestApplyUsageBillingEffects_FlagsBalanceOverdraft|TestDeductUsageBillingBalance_' -count=1

Expected: PASS with native SQL expectations and no wallet ledger call.

- [ ] Step 4: Run the balance billing integration regression.

Run:
    cd upstream/sub2api/backend
    go test -tags integration ./internal/repository -run 'TestUsageBillingRepositoryApply_DeduplicatesBalanceBilling|TestUsageBillingRepositoryApply_PartialReconciliationRetryChargesOnce' -count=1

Expected: PASS; users.balance is deducted once, inference creates no usage_consumption wallet ledger row, and retry deduplication remains unchanged.

- [ ] Step 5: Commit the native billing restoration.

    git add upstream/sub2api/backend/internal/repository/usage_billing_repo.go upstream/sub2api/backend/internal/repository/usage_billing_repo_unit_test.go upstream/sub2api/backend/internal/repository/usage_billing_repo_integration_test.go
    git commit -m "fix: restore native balance billing"

---

### Task 2: Invalidate Balance Cache After Manual Quota Mutation

Files:
- Modify: upstream/sub2api/backend/internal/handler/admin/user_handler.go
- Modify: upstream/sub2api/backend/internal/handler/admin/quota_wallet_handler_test.go

Interfaces:
- Consumes: service.QuotaWalletService.Recharge, service.QuotaWalletService.Refund, and the existing service.BillingCache field injected by NewUserHandler.
- Produces: CreateQuotaLedgerEntry that invalidates the target user's balance cache after a successful recharge/refund, including idempotent replay, while preserving the existing response body and error mapping.

- [ ] Step 1: Add a RED handler test.

Add a BillingCache test double to quota_wallet_handler_test.go with a recorded InvalidateUserBalance call and no-op implementations for unrelated cache methods. Mount NewUserHandler with the test cache, inject quotaWalletHandlerFake, and submit a recharge with Idempotency-Key admin-test-1. Assert HTTP 200, target user ID 7, and exactly one invalidation. Add the same assertion path for refund to prove both mutation types invalidate the target user.

Run:
    cd upstream/sub2api/backend
    go test ./internal/handler/admin -run 'TestQuotaWalletHandler(CreateRecharge|InvalidatesBalanceCache)' -count=1

Expected: FAIL because CreateQuotaLedgerEntry currently never invokes InvalidateUserBalance.

- [ ] Step 2: Implement cache invalidation without changing the ledger contract.

After Recharge or Refund returns successfully and before response.Success, call:
    if h.billingCache != nil {
        if cacheErr := h.billingCache.InvalidateUserBalance(c.Request.Context(), userID); cacheErr != nil {
            slog.Error("invalidate user balance cache failed after quota ledger mutation", "user_id", userID, "error", cacheErr)
        }
    }

Do not turn a committed ledger mutation into an HTTP failure when Redis invalidation fails. Do not invalidate on validation or service errors.

Run:
    cd upstream/sub2api/backend
    gofmt -w internal/handler/admin/user_handler.go internal/handler/admin/quota_wallet_handler_test.go
    go test ./internal/handler/admin -run 'TestQuotaWalletHandler(CreateRecharge|InvalidatesBalanceCache)' -count=1

Expected: PASS; the response remains HTTP 200 and the target user ID is invalidated once per successful mutation.

- [ ] Step 3: Run existing quota handler and service regressions.

Run:
    cd upstream/sub2api/backend
    go test ./internal/handler/admin -run TestQuotaWalletHandler -count=1
    go test ./internal/service -run TestQuotaWallet -count=1

Expected: PASS with existing idempotency-key, precision, validation, and error-contract behavior unchanged.

- [ ] Step 4: Commit the cache invalidation fix.

    git add upstream/sub2api/backend/internal/handler/admin/user_handler.go upstream/sub2api/backend/internal/handler/admin/quota_wallet_handler_test.go
    git commit -m "fix: invalidate balance cache after quota mutation"

---

### Task 3: Candidate Verification and Handoff

Files:
- Verify: all T67 implementation and test files above.
- Create: docs/handoffs/2026-08-25-t67-native-billing-handoff.md

Interfaces:
- Consumes: Task 1 native billing and Task 2 cache invalidation.
- Produces: clean candidate with direct test evidence and explicit root-review handoff; no root main, queue, progress, release evidence, or production changes.

- [ ] Step 1: Run direct backend verification.

    cd upstream/sub2api/backend
    go test -tags unit ./internal/repository -run 'Test(DeductUsageBillingBalance|ApplyUsageBillingEffects)' -count=1
    go test ./internal/handler/admin -run TestQuotaWalletHandler -count=1
    go test ./internal/service -run TestQuotaWallet -count=1
    go build ./cmd/server

Expected: all commands exit 0.

- [ ] Step 2: Run formatting and scope checks.

    cd upstream/sub2api/backend
    gofmt -w internal/repository/usage_billing_repo.go internal/repository/usage_billing_repo_unit_test.go internal/repository/usage_billing_repo_integration_test.go internal/handler/admin/user_handler.go internal/handler/admin/quota_wallet_handler_test.go
    cd ../..
    git diff --check
    git status --short
    git diff --name-only main...HEAD

Expected: only repository, handler, test, spec, plan, and handoff files are changed; no migration or configuration file changes.

- [ ] Step 3: Write the handoff with exact evidence.

Record candidate baseline, implementation commits, changed files, commands and outcomes, migration/configuration status, downtime_required=unverified until root preflight, rollback to the prior blue-green source, and the remaining requirement that root refresh the candidate onto the then-current main before merge.

- [ ] Step 4: Commit the handoff.

    git add docs/handoffs/2026-08-25-t67-native-billing-handoff.md
    git commit -m "docs: hand off T67 native billing candidate"
