# T55 原生额度钱包与手动充值退款 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Sub2API 原生用户余额路径上增加可审计的钱包拆分、手动充值/退款、付费优先扣费和额度流水，同时保持 `users.balance` 兼容投影。

**Architecture:** 以 `user_wallets` 为拆分余额事实源，以 `user_quota_ledger_entries` 为来源审计流水，所有余额写入经单一 wallet coordinator 在同一数据库事务中更新钱包与 `users.balance`。旧管理员余额、兑换码、返利、支付入账和用量扣费都通过 coordinator 适配，不引入第二套计费事实源；`frozen_balance` 保持总额度冻结语义。

**Tech Stack:** Go 1.26.6、Ent 0.14.5、PostgreSQL migrations、Gin、shopspring/decimal、Vue 3、TypeScript、Vitest、Vue Test Utils、pnpm。

**Spec:** `docs/superpowers/specs/2026-08-23-t55-native-quota-wallet-recharge-refund-design.md`

## Global Constraints

- 付费额度优先消耗，赠送额度其次；历史 `users.balance` 迁移为付费额度，现金和赠送额度均为 0。
- `paid_quota_balance_usd + gift_quota_balance_usd = users.balance` 必须在每次写入后成立。
- 现金余额只由手动充值/退款和真实支付履约路径更新；历史迁移不产生可退款现金。
- 退款不能超过现金余额或付费额度，退款时赠送额度全部清零且不折现。
- 不删除或重命名 `users.balance`、`frozen_balance`、`payment_orders`、`redeem_codes`、`usage_logs`。
- 不使用 GitHub Actions；不修改 T54-R1 worktree；迁移/发布预检若返回 `downtime_required=true` 必须停在用户授权门禁。
- 每个任务先写失败测试，再写最小实现；每个任务结束运行直接相关测试并提交。

---

## Task 1: Ent wallet/ledger schema and expand-only migration

**Files:**
- Create: `upstream/sub2api/backend/ent/schema/user_wallet.go`
- Create: `upstream/sub2api/backend/ent/schema/user_quota_ledger_entry.go`
- Create: `upstream/sub2api/backend/ent/schema/quota_idempotency_record.go`
- Create: `upstream/sub2api/backend/migrations/225_user_quota_wallet_ledger.sql`
- Create: `upstream/sub2api/backend/migrations/user_quota_wallet_ledger_migration_test.go`
- Modify: `upstream/sub2api/backend/ent/schema/user.go` only if an explicit wallet edge is required by generated Ent relations
- Generate: `upstream/sub2api/backend/ent/*` using the repository's Ent generation command

**Interfaces:**
- Produces Ent entities `UserWallet`, `UserQuotaLedgerEntry`, and `QuotaIdempotencyRecord` with decimal numeric fields, user foreign keys, unique `(user_id, idempotency_key)` scope, and indexes for `(user_id, created_at DESC)` and `(reference_type, reference_id)`.
- Migration creates tables, nonnegative checks for cash/paid/gift balances, ledger snapshot columns, and an idempotent initialization statement that copies non-deleted `users.balance` into paid quota while setting cash/gift to zero.

- [ ] **Step 1: Write migration contract tests** asserting the SQL creates all three tables, preserves legacy columns, uses `ON CONFLICT (user_id) DO NOTHING`, sets `cash_balance_cny` and `gift_quota_balance_usd` to zero, and contains no `INSERT INTO user_quota_ledger_entries` history backfill.
- [ ] **Step 2: Run `go test ./migrations -run UserQuotaWalletLedger -v` and verify failure because the migration/schema do not exist.**
- [ ] **Step 3: Define Ent schemas** with `decimal(20,8)` fields represented by the existing project numeric conventions, enum/text record types (`recharge`, `refund`, `usage_consumption`, `legacy_balance_adjustment`, `payment_fulfillment`, `redeem_credit`, `affiliate_credit`, `migration_projection`), operator/note/reference fields, and timestamp/version fields.
- [ ] **Step 4: Add the expand-only SQL migration** with constraints and indexes; make initialization safe for repeated startup/migration execution and preserve soft-deleted users.
- [ ] **Step 5: Run the Ent generator from `upstream/sub2api/backend` and rerun `go test ./migrations -run UserQuotaWalletLedger -v`.**
- [ ] **Step 6: Run `go test ./ent/... ./migrations/...` and `git diff --check`; commit `feat: add native quota wallet schema`.**

## Task 2: Wallet coordinator and ledger repository

**Files:**
- Create: `upstream/sub2api/backend/internal/service/quota_wallet.go`
- Create: `upstream/sub2api/backend/internal/service/quota_wallet_test.go`
- Create: `upstream/sub2api/backend/internal/repository/quota_wallet_repo.go`
- Create: `upstream/sub2api/backend/internal/repository/quota_wallet_repo_test.go`
- Modify: `upstream/sub2api/backend/internal/service/user_service.go` interfaces to expose coordinator operations without removing existing methods
- Modify: `upstream/sub2api/backend/internal/repository/user_repo.go` only to route legacy atomic balance methods through the coordinator

**Interfaces:**
- `QuotaWalletService.GetSummary(ctx, userID) (QuotaSummary, error)`
- `QuotaWalletService.Recharge(ctx, RechargeInput) (QuotaMutationResult, error)`
- `QuotaWalletService.Refund(ctx, RefundInput) (QuotaMutationResult, error)`
- `QuotaWalletService.ConsumeUsage(ctx, UsageConsumptionInput) (QuotaMutationResult, error)`
- `QuotaWalletService.LegacyAdjust(ctx, LegacyBalanceAdjustmentInput) (QuotaMutationResult, error)`
- `QuotaWalletRepository.WithLockedWallet(ctx, userID, fn)` and paginated `ListLedger(ctx, userID, page, pageSize, recordType)`

- [ ] **Step 1: Write table-driven unit tests** for paid-first consumption, gift fallback, insufficient total balance, recharge 1:1 cash/paid plus gift, refund cash/paid limits and gift clearing, zero/negative validation, legacy `set` target validation, and stable business errors.
- [ ] **Step 2: Run `go test ./internal/service -run QuotaWallet -v` and verify failure.**
- [ ] **Step 3: Implement decimal-based pure mutation calculations** returning before/after snapshots and source-specific deltas; reject float-derived input except at the HTTP boundary conversion.
- [ ] **Step 4: Implement repository transactions** that lock `user_wallets FOR UPDATE`, create a missing wallet exactly once from the current `users.balance`, update wallet and `users.balance` together, insert ledger and idempotency rows, and preserve `frozen_balance` untouched.
- [ ] **Step 5: Add repository tests** for rollback, duplicate idempotency key with equal/different request hash, concurrent refund serialization, and projection invariant.
- [ ] **Step 6: Run `go test ./internal/service ./internal/repository -run 'QuotaWallet|UserRepo' -v`; commit `feat: add quota wallet coordinator`.**

## Task 3: Route every existing balance writer through the coordinator

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/gateway_usage_billing.go`
- Modify: `upstream/sub2api/backend/internal/service/usage_service.go`
- Modify: `upstream/sub2api/backend/internal/service/redeem_service.go`
- Modify: `upstream/sub2api/backend/internal/service/promo_service.go`
- Modify: `upstream/sub2api/backend/internal/service/payment_refund.go` and payment fulfillment writer identified by repository search
- Modify: `upstream/sub2api/backend/internal/service/user_service.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go` and `upstream/sub2api/backend/cmd/server/wire.go`/generated wiring as required
- Tests: existing gateway, usage, redeem, promo, payment, and user service tests plus new `upstream/sub2api/backend/internal/service/quota_balance_writer_integration_test.go`

**Interfaces:**
- Usage billing calls `ConsumeUsage` and records paid/gift deltas on `usage_consumption`; `usage_logs.actual_cost` and request billing values remain unchanged.
- Redeem balance credits use `redeem_credit`; affiliate credits use `affiliate_credit`; payment fulfillment uses `payment_fulfillment`; old `UpdateBalance`/`DeductBalance` no longer write `users.balance` directly.

- [ ] **Step 1: Add failing integration tests** that execute each writer in an Ent transaction and assert wallet, `users.balance`, and ledger deltas are identical; assert `frozen_balance` is unchanged.
- [ ] **Step 2: Run the focused service tests and verify direct-writer assertions fail.**
- [ ] **Step 3: Inject one coordinator instance through Wire and replace direct repository calls with typed coordinator methods, preserving each source's existing transaction and status behavior.**
- [ ] **Step 4: Update old `set/add/subtract` compatibility mapping: `add/subtract` become paid-only `legacy_balance_adjustment`; `set` computes target total delta and rejects targets below current gift quota.**
- [ ] **Step 5: Update cache invalidation calls after successful commit; cache failure logs diagnostics without rolling back ledger state.**
- [ ] **Step 6: Run focused tests for gateway, usage, redeem, promo, payment, and user service; commit `refactor: route balance writers through quota wallet`.**

## Task 4: Admin quota APIs and DTOs

**Files:**
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/user_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/types.go` and `mappers.go`
- Modify: `upstream/sub2api/backend/internal/service/admin_service.go` or add `upstream/sub2api/backend/internal/service/admin_quota_service.go` for admin-facing orchestration
- Create: `upstream/sub2api/backend/internal/handler/admin/quota_wallet_handler_test.go`
- Modify: `upstream/sub2api/backend/internal/server/api_contract_test.go`

**Interfaces:**
- `GET /api/v1/admin/users/:id/quota-summary` returns `user_id`, cash, paid, gift, total, wallet version, and updated timestamp.
- `POST /api/v1/admin/users/:id/quota-ledger` accepts `record_type`, `amount_cny`, optional `gift_quota_usd`, note, and `Idempotency-Key`; returns the confirmed ledger ID plus latest summary.
- `GET /api/v1/admin/users/:id/quota-ledger?page&page_size&type` returns paginated sanitized ledger entries and snapshots.

- [ ] **Step 1: Write handler tests** for admin auth, missing/soft-deleted users, malformed decimals, zero amounts, refund over cash/paid, idempotency replay/conflict, pagination, and sanitized response fields.
- [ ] **Step 2: Run `go test ./internal/handler/admin ./internal/server -run 'Quota|Balance' -v` and verify failure.**
- [ ] **Step 3: Implement request DTO validation, stable error mapping, admin actor extraction, idempotency header enforcement, and the three routes.**
- [ ] **Step 4: Keep `/balance` registered and mark its response as compatibility behavior while routing writes through the coordinator.**
- [ ] **Step 5: Run handler/API contract tests and `go test ./internal/handler/admin ./internal/server -run 'Quota|Balance' -v`; commit `feat: expose admin quota wallet APIs`.**

## Task 5: Admin users UI and API client

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/users.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/types/index.ts` or the existing admin user type location
- Modify: `upstream/sub2api/frontend/src/views/admin/UsersView.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.vue`
- Create/modify tests: `upstream/sub2api/frontend/src/components/admin/user/__tests__/UserBalanceModal.spec.ts`, `upstream/sub2api/frontend/src/views/admin/__tests__/UsersView.spec.ts`
- Modify translations under `upstream/sub2api/frontend/src/i18n/locales/{zh-CN,en}/admin/` for cash/paid/gift/recharge/refund/ledger errors

**Interfaces:**
- API client exports `getUserQuotaSummary`, `createQuotaLedgerEntry`, and `getUserQuotaLedger` with typed request/response contracts and generated UUID idempotency keys per submit.
- Modal has only recharge/refund modes, no user selector; recharge shows cash/paid 1:1 and editable gift; refund shows cash limit, paid deduction, full-refund action, and gift auto-clear notice.

- [ ] **Step 1: Write Vitest tests** for mode fields, paid/gift preview math, full-refund fill, zero disable, over-limit message, backend error display, and idempotency header.
- [ ] **Step 2: Run `pnpm --dir upstream/sub2api/frontend test:run -- src/components/admin/user/__tests__/UserBalanceModal.spec.ts` and verify failure.**
- [ ] **Step 3: Add typed API methods and replace `updateBalance` calls in the new modal flow; keep old method only for compatibility consumers.**
- [ ] **Step 4: Implement responsive modal summary and update the user list row/detail display with server-returned summary values, including 390px overflow protection.**
- [ ] **Step 5: Run focused component/view tests, `pnpm --dir upstream/sub2api/frontend typecheck`, and `pnpm --dir upstream/sub2api/frontend build`; commit `feat: add quota recharge refund admin UI`.**

## Task 6: Quota ledger history UI

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/users.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/UsersView.vue` and `upstream/sub2api/frontend/src/views/admin/UsageView.vue` only for tab/modal integration
- Create: `upstream/sub2api/frontend/src/components/admin/user/__tests__/UserBalanceHistoryModal.quota.spec.ts`
- Modify translations under `upstream/sub2api/frontend/src/i18n/locales/{zh-CN,en}/admin/`

**Interfaces:**
- Existing redeem/affiliate/admin/concurrency/subscription history remains unchanged; new “额度流水” tab renders sanitized quota entries with record type, deltas, operator, note, ID, timestamps, and before/after snapshots.

- [ ] **Step 1: Write tests** for quota tab selection, pagination/filter, empty/error states, paid/gift usage source display, and coexistence with legacy history.
- [ ] **Step 2: Run the focused Vitest file and verify failure.**
- [ ] **Step 3: Implement the tab using the new API client without merging record semantics in the old balance-history mapper.**
- [ ] **Step 4: Run focused tests, frontend typecheck, and build; commit `feat: add quota ledger history view`.**

## Task 7: Migration/release gates and handoff evidence

**Files:**
- Create: `upstream/sub2api/backend/migrations/user_quota_wallet_release_contract_test.go` if existing migration contract conventions require a separate release test
- Modify: `docs/handoffs/2026-08-23-t55-native-quota-wallet-handoff.md`
- Modify only task-local implementation notes; do not modify root `docs/project/project-progress.md` or `docs/project/native-sub-task-package-queue.md`

**Interfaces:**
- Handoff records baseline SHA, implementation commit(s), migration name, test commands/results, `downtime_required`, rollback image/slot, and unresolved risks.

- [ ] **Step 1: Add a migration contract test** proving rerun safety and no destructive DDL.
- [ ] **Step 2: Run the minimal backend/frontend commands from the spec with actual repository paths: `go test ./internal/service/... ./internal/repository/... ./internal/handler/...`, `go build ./cmd/server`, `pnpm --dir upstream/sub2api/frontend test:run`, `pnpm --dir upstream/sub2api/frontend typecheck`, `pnpm --dir upstream/sub2api/frontend build`, and `git diff --check`.
- [ ] **Step 3: Record migration lock/rollback analysis and set `downtime_required` only from the reviewed release precheck output.
- [ ] **Step 4: Commit handoff evidence as `docs: add T55 quota wallet handoff`; report `READY_FOR_ROOT_REVIEW` without merging, pushing, deploying, or deleting the worktree.

---

## Plan self-review

- Spec coverage: schema/migration (Task 1), wallet algorithms/transactions/idempotency (Task 2), every existing balance writer and cache invalidation (Task 3), admin API/auth/error semantics (Task 4), recharge/refund UI and responsive behavior (Task 5), ledger audit display (Task 6), release/rollback evidence (Task 7).
- Placeholder scan: no `TBD`, `TODO`, “implement later”, or undefined neighboring method names are used; actual repository paths and commands are listed.
- Type consistency: all API names use `QuotaSummary`, `QuotaMutationResult`, `getUserQuotaSummary`, `createQuotaLedgerEntry`, and `getUserQuotaLedger`; backend coordinator methods use the same operation vocabulary as the spec.
- Scope guard: payment, redeem, affiliate, usage, admin compatibility, migration, and UI are coupled by the single `users.balance` invariant, so they remain one task package rather than separate drifting plans.
