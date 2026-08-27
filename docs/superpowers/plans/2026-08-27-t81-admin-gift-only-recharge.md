# T81 管理员仅赠送额度充值 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow a manual recharge with ¥0 cash and a positive gift quota, while retaining all zero-value and refund safeguards.

**Architecture:** Reuse the existing native quota-ledger request. The modal has one derived validity rule: recharge accepts either positive field; refund accepts only positive cash amount.

**Tech Stack:** Vue 3, TypeScript, Vitest, Vue Test Utils.

**Spec:** `docs/superpowers/specs/2026-08-27-t81-admin-gift-only-recharge-design.md`

## Global Constraints

- Do not change the native quota-wallet service, API contract, schema, migration, payment flow, or deployment configuration.
- Do not push, deploy, or write production data.
- Retain server-side rejection of negative and double-zero mutations.

---

### Task 1: Permit gift-only recharge in the existing modal

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.vue`

**Interfaces:**
- Consumes: `form.amount`, `form.giftQuota`, and `props.operation`.
- Produces: an existing `createQuotaLedgerEntry` request with `{ record_type: 'recharge', amount_cny: 0, gift_quota_usd: positive }`.

- [ ] **Step 1: Write the failing component test**

```ts
await wrapper.findAll('input[type="number"]')[0].setValue('0')
await wrapper.findAll('input[type="number"]')[1].setValue('5')
await wrapper.get('#balance-form').trigger('submit')
expect(createQuotaLedgerEntry).toHaveBeenCalledWith(1, {
  record_type: 'recharge', amount_cny: 0, gift_quota_usd: 5, note: ''
})
```

- [ ] **Step 2: Run the single test and verify it fails because cash amount is currently required**

Run: `pnpm vitest run src/components/admin/user/UserBalanceModal.spec.ts`

- [ ] **Step 3: Implement the minimal local validity predicate**

```ts
const hasValidMutation = () => props.operation === 'add'
  ? form.amount > 0 || form.giftQuota > 0
  : form.amount > 0
```

Use it for the confirm button and submit guard; remove the cash field `required` attribute only in recharge mode.

- [ ] **Step 4: Add regression checks for double-zero recharge and zero refund**

```ts
expect(createQuotaLedgerEntry).not.toHaveBeenCalled()
expect(showError).toHaveBeenCalledWith('admin.users.amountRequired')
```

- [ ] **Step 5: Run the focused test file, inspect the diff, and commit locally**

Run: `pnpm vitest run src/components/admin/user/UserBalanceModal.spec.ts && git diff --check`

