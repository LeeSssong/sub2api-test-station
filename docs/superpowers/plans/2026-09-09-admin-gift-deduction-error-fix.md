# Admin Gift Deduction Error Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make administrator gift-quota deduction reliable and understandable while preserving the rule that paid quota is never deducted.

**Architecture:** Keep the existing `/admin/users/:id/balance` compatibility route and its idempotency coordinator. The frontend always supplies an idempotency key, loads the native quota summary, validates against gift quota, and maps structured backend errors. The backend continues routing `subtract` exclusively to `DeductGift`, which atomically fails when gift quota is insufficient.

**Tech Stack:** Go service/handler tests, Vue 3 + TypeScript, Vitest, pnpm typecheck/build.

---

### Task 1: Lock down the frontend request and error contract

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/users.ts:168-188`
- Test: `upstream/sub2api/frontend/src/api/__tests__/admin.users.spec.ts:150-190`
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.vue:76-101`
- Test: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.spec.ts:1-120`

- [ ] **Step 1: Add a failing API test for an automatically generated idempotency key**

  Assert that `updateBalance(7, 2, 'subtract', 'reason')` calls `POST /admin/users/7/balance` with a non-empty `Idempotency-Key` header and does not rely on the caller to provide one.

- [ ] **Step 2: Run the focused API test and verify the failure**

  Run from `upstream/sub2api/frontend`:

  ```bash
  pnpm vitest run src/api/__tests__/admin.users.spec.ts
  ```

  Expected result: the new assertion fails if the current branch does not already satisfy it; if it passes because the fix is already present, retain the test as regression coverage and continue to the modal error test.

- [ ] **Step 3: Add a failing modal test for explicit insufficient gift quota feedback**

  Mount the subtract modal with a summary such as `paid_quota_balance_usd: '20'` and `gift_quota_balance_usd: '1'`, enter `2`, submit, and assert that the UI calls `showError` with the translated insufficient-gift-quota key and never calls `updateBalance`.

- [ ] **Step 4: Run the focused modal test and verify the failure**

  ```bash
  pnpm vitest run src/components/admin/user/UserBalanceModal.spec.ts
  ```

  Expected result: the new assertion fails for the current behavior or exposes the exact mismatch in the existing test harness.

- [ ] **Step 5: Implement the minimal frontend contract**

  Keep `createIdempotencyKey()` as the single key generator and pass its result through `updateBalance`. In `UserBalanceModal.vue`, keep the `gift_quota_balance_usd` max constraint and pre-submit guard; map the backend insufficient-gift error to the existing `admin.users.insufficientBalance` translation while preserving other backend messages through `extractApiErrorMessage`.

- [ ] **Step 6: Run the focused frontend tests**

  ```bash
  pnpm vitest run src/api/__tests__/admin.users.spec.ts src/components/admin/user/UserBalanceModal.spec.ts
  ```

  Expected result: all tests pass.

- [ ] **Step 7: Commit the frontend regression coverage and fix**

  ```bash
  git add upstream/sub2api/frontend/src/api/admin/users.ts \
    upstream/sub2api/frontend/src/api/__tests__/admin.users.spec.ts \
    upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.vue \
    upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.spec.ts
  git commit -m "fix(admin): make gift deduction errors actionable"
  ```

### Task 2: Verify backend gift-only deduction and error propagation

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/admin_user.go:510-553` only if the focused test identifies an error contract gap
- Test: `upstream/sub2api/backend/internal/service/admin_service_update_balance_test.go:104-140`
- Test: `upstream/sub2api/backend/internal/handler/admin/quota_wallet_handler_test.go` or the existing admin user handler test if a route-level header assertion is needed

- [ ] **Step 1: Add a failing service test for the missing idempotency-key contract**

  Call `UpdateUserBalance` with an empty key and assert an error containing `idempotency key`; assert the gift adjuster receives no call.

- [ ] **Step 2: Run the focused Go test and verify the failure or existing coverage**

  Run from `upstream/sub2api/backend`:

  ```bash
  go test -tags unit ./internal/service -run 'TestAdminService_UpdateUserBalance' -count=1
  ```

- [ ] **Step 3: Implement only the backend contract gap revealed by the test**

  Preserve the existing validation, `add -> GrantGift`, `subtract -> DeductGift`, `ErrBalanceNegative -> gift quota is insufficient` mapping, and cache invalidation. Do not route subtraction through refund or paid-quota mutation.

- [ ] **Step 4: Run the focused backend tests**

  ```bash
  go test -tags unit ./internal/service ./internal/handler/admin -run 'Test(AdminService_UpdateUserBalance|QuotaWallet)' -count=1
  ```

  Expected result: pass, with no paid-quota mutation assertions failing.

- [ ] **Step 5: Commit backend coverage or the minimal backend adjustment**

  ```bash
  git add upstream/sub2api/backend/internal/service/admin_service_update_balance_test.go \
    upstream/sub2api/backend/internal/service/admin_user.go \
    upstream/sub2api/backend/internal/handler/admin
  git commit -m "test(admin): enforce gift-only deduction contract"
  ```

### Task 3: Run direct verification and prepare handoff

**Files:**
- Modify: `docs/handoffs/2026-09-09-admin-gift-deduction-error-fix-handoff.md`

- [ ] **Step 1: Run the complete direct frontend verification**

  ```bash
  cd upstream/sub2api/frontend
  pnpm vitest run src/api/__tests__/admin.users.spec.ts src/components/admin/user/UserBalanceModal.spec.ts src/components/admin/user/UserBalanceHistoryModal.spec.ts
  pnpm typecheck
  pnpm build
  ```

- [ ] **Step 2: Run the complete direct backend verification**

  ```bash
  cd upstream/sub2api/backend
  go test -tags unit ./internal/service ./internal/handler/admin -run 'Test(AdminService_UpdateUserBalance|QuotaWallet)' -count=1
  go build ./cmd/server
  ```

- [ ] **Step 3: Check the diff and sensitive-file boundary**

  ```bash
  git diff --check
  git status --short
  git diff --name-status origin/main...HEAD
  ```

  Expected result: only the approved frontend/backend tests, minimal implementation files, and handoff/spec/plan documents are changed; no credentials, runtime data, or production files are included.

- [ ] **Step 4: Write the handoff report**

  Record baseline commit, final commit, changed files, tests, migration/config/data/credential status, deployment authorization status, rollback approach, and remaining real-user verification gap.

- [ ] **Step 5: Commit the handoff**

  ```bash
  git add docs/handoffs/2026-09-09-admin-gift-deduction-error-fix-handoff.md
  git commit -m "docs: hand off admin gift deduction error fix"
  ```

