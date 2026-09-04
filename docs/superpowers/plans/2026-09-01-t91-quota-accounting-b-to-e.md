# T91-B–E Quota Accounting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 完成最终开发基线中额度发放、API 扣费归因、退款 Saga、管理员扣减、历史切换与双写/cutover 的可验证实现。

**Architecture:** 以 `user_quota_grants` 作为正向额度事实源、`billing_usage_entries` 作为消费事实源、`user_quota_adjustments` 作为退款/管理员扣减事实源，`user_wallets` 作为余额投影；所有金额通过 Decimal/numeric(20,8) 边界，旧 `user_quota_ledger_entries` 仅保留历史只读证据。第三方支付调用在事务外，通过现有 outbox 驱动可恢复 Saga。

**Tech Stack:** Go、Ent、PostgreSQL 18、现有迁移 runner、现有 payment/usage service、sqlmock/SQLite/Colima PostgreSQL 集成测试。

**Spec:** `/Users/gongtengxinwen/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/wxid_9944479444511_c3ab/temp/drag/2026-08-31-quota-accounting-rules-development-plan(1).md`

## Global Constraints

- 当前所有工作只在 `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t91-quota-accounting` 进行。
- 不修改根目录 `main`，不合并、推送、构建发布或部署。
- 新账务金额使用 `decimal.Decimal`，数据库额度列使用 `numeric(20,8)`。
- paid 先于 gift，同桶 FIFO；`migration_opening` 不参与消费、退款或管理员扣减。
- `attempted_quota_usd` 是应扣额度；`delta_usd` 是实际扣费；未扣差额不形成欠款。
- 可重试退款保持 `pending`、`unknown` 或 `reconciling`；只有明确永久错误才进入 `failed`/dead-letter。
- 所有新行为先写失败测试，再写最小实现；每个任务完成后运行直接相关测试。
- 不执行 acceptance/生产迁移、历史正式回填、双写切换或真实 provider 退款。

### Task 1: T91-B 完整额度发放与标准订单履约

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/payment_order.go`
- Modify: `upstream/sub2api/backend/internal/service/payment_fulfillment.go`
- Modify: `upstream/sub2api/backend/internal/service/payment_service.go`
- Modify: `upstream/sub2api/backend/ent/schema/payment_order.go`
- Test: `upstream/sub2api/backend/internal/service/*quota*test.go`

**Interfaces:** `CreateQuotaGrant(ctx, QuotaGrantInput)`, payment-order fulfillment idempotency, and admin-recharge request/response contracts.

- [ ] Step 1: Add failing tests for admin-recharge validation, five payment types plus aliases, quota snapshot calculation, and repeated fulfillment.
- [ ] Step 2: Run the focused tests and confirm failures identify missing behavior.
- [ ] Step 3: Implement server-side snapshot calculation, standard `admin_recharge` order creation, operator audit fields, and grant fulfillment using `(payment_order_id)` idempotency.
- [ ] Step 4: Add redeem/promo/affiliate grant adapters that preserve legacy aliases and use the same grant service.
- [ ] Step 5: Run service/repository tests, SQLite tests, `go vet`, and `git diff --check`.

### Task 2: T91-C 完整 API 扣费归因与批次状态

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/usage_billing.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_billing_repo.go`
- Modify: `upstream/sub2api/backend/internal/service/gateway_usage_billing.go`
- Test: `upstream/sub2api/backend/internal/repository/usage_billing_repo_unit_test.go`
- Test: `upstream/sub2api/backend/internal/service/*usage*test.go`

**Interfaces:** usage billing command with `UsageLogID`, paid/gift allocation JSON, `attempted_quota_usd`, and exact/legacy attribution.

- [ ] Step 1: Add failing tests for paid-only, paid-then-gift, insufficient balance, zero/failed request, multi-grant FIFO allocation, and idempotent retry.
- [ ] Step 2: Run focused tests and verify red failures.
- [ ] Step 3: Implement one transaction with wallet lock → grant locks → usage entry → counters → legacy balance projection.
- [ ] Step 4: Ensure failed/zero-cost semantics match the existing path and never create new negative balances.
- [ ] Step 5: Run direct tests plus migration schema checks and `go vet`.

### Task 3: T91-D Refund reservation, outbox worker, and admin adjustments

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/payment_refund.go`
- Modify: `upstream/sub2api/backend/internal/service/scheduler_events.go`
- Modify: `upstream/sub2api/backend/internal/service/payment_service.go`
- Modify: `upstream/sub2api/backend/internal/quota/refund/saga.go`
- Modify: `upstream/sub2api/backend/internal/quota/refund/retry.go`
- Test: `upstream/sub2api/backend/internal/service/payment_refund_quota_test.go`
- Test: `upstream/sub2api/backend/internal/quota/refund/*_test.go`

**Interfaces:** reservation/finalization/reconciliation methods, `quota_refund.requested` outbox event, worker claim/retry/dead-letter, provider callback/query correlation, and gift FIFO admin deduction.

- [ ] Step 1: Add failing tests for reservation exclusion, crash recovery, provider timeout/connection retry, permanent dead-letter, callback correlation, and admin gift deduction.
- [ ] Step 2: Run focused tests and verify red failures.
- [ ] Step 3: Implement reservation and finalization in separate transactions; never call provider while holding DB locks.
- [ ] Step 4: Implement outbox claim/lease, retry schedule, dead-letter, and unknown/reconciling recovery.
- [ ] Step 5: Implement admin paid refund and gift deduction idempotency, allocation updates, wallet projection, and audit records.
- [ ] Step 6: Run service/quota/repository tests and `go vet`.

### Task 4: T91-E Historical opening, dry-run, dual-write, and cutover gates

**Files:**
- Create: `upstream/sub2api/backend/internal/quota/cutover/*.go`
- Create: `upstream/sub2api/backend/cmd/quota-cutover/`
- Modify: `upstream/sub2api/backend/internal/quota/reconciliation/*.go`
- Test: `upstream/sub2api/backend/internal/quota/cutover/*_test.go`

**Interfaces:** dry-run report, signed `migration_opening` grant generation, reconciliation gate, dual-write status machine, and cutover read-only transition.

- [ ] Step 1: Add failing tests for positive/negative opening, legacy unknown attribution, residual reporting, gate refusal, and idempotent rerun.
- [ ] Step 2: Run focused tests and verify red failures.
- [ ] Step 3: Implement read-only dry-run and report serialization; do not mutate production data.
- [ ] Step 4: Implement an explicit state machine requiring reconciliation pass before any cutover action.
- [ ] Step 5: Add command-level dry-run tests and rollback/recovery evidence checks.

### Task 5: End-to-end audit and handoff

- [ ] Step 1: Run all T91 direct tests, integration migration tests, `go vet`, and `git diff --check`.
- [ ] Step 2: Audit every final-baseline requirement against source and test evidence.
- [ ] Step 3: Update only task-local handoff/report documents; keep root release lane untouched.
- [ ] Step 4: Report remaining unimplemented or externally gated work explicitly.
