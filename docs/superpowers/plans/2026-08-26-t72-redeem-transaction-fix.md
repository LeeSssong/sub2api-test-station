# T72 兑换码充值事务边界修复实施计划

> **For agentic workers:** Execute this plan task-by-task with TDD and verification evidence.

**Goal:** 让兑换码正向充值在已有兑换事务中原子完成钱包、余额投影和账本写入。

**Architecture:** 钱包仓库识别并复用 Ent ambient transaction；自有调用继续自行开启并提交事务。兑换服务、钱包服务和账本模型保持不变。

**Tech Stack:** Go 1.26、Ent、PostgreSQL、sqlmock、testify。

**Spec:** `docs/superpowers/specs/2026-08-26-t72-redeem-transaction-fix-design.md`

## Global Constraints

- 只改兑换事务边界和直接相关测试；不新增迁移、配置、依赖或账务事实源。
- 不修改生产数据，发布只能从验证后的 `main` 通过既有本地/宿主蓝绿链执行。
- T71 仍占用 VERIFYING；T72 在其完成前不得抢占合并、部署或线上验收车道。

### Task 1: Lock transaction reuse regression (RED/GREEN) - completed

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/quota_wallet_repo.go`
- Test: `upstream/sub2api/backend/internal/repository/quota_wallet_repo_test.go`

- [x] Add a sqlmock test that places an Ent transaction in context and expects user lock, wallet initialization/read, callback execution, and no nested `BEGIN`.
- [x] Run the single test against the baseline and record the expected nested-transaction failure.
- [x] Implement the smallest branch in `WithLockedWallet` that reuses `dbent.TxFromContext(ctx)` and leaves ownership/commit behavior unchanged for self-created transactions.
- [x] Run the single regression test and the existing quota repository tests.

### Task 2: Direct redemption and rollback coverage - completed

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/redeem_service_redeem_test.go`
- Test/inspect: `upstream/sub2api/backend/internal/repository/user_repo_redeem_adjustment_test.go`, `upstream/sub2api/backend/internal/service/payment_fulfillment_test.go`

- [x] Add focused service/repository contracts for positive redeem credit, negative floor semantics, and callback failure rollback where a real database harness is available; keep doubles at the external repository boundary.
- [x] Run the focused redeem, quota, payment fulfillment, affiliate, and usage billing tests; capture any environment-blocked integration test separately.
- [x] Audit all `LegacyAdjust`, `Recharge`, `Refund`, and `ConsumeUsage` call sites for ambient transaction propagation and document findings in the handoff.

### Task 3: Candidate verification and handoff - completed

**Files:**
- Create: `docs/handoffs/2026-08-26-t72-redeem-transaction-fix-handoff.md`

- [x] Run `go test ./internal/service -run 'Redeem|QuotaWallet|PaymentFulfillment' -count=1`.
- [x] Run `go test ./internal/repository -run 'Redeem|QuotaWallet|PaymentFulfillment' -count=1` and note external prerequisite failures.
- [x] Run `go build ./cmd/server` and `git diff --check`.
- [x] Review diff for scope, record baseline/candidate SHA, changed files, migration/config status, downtime expectation, rollback, and residual risks.
- [x] Commit candidate and report `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or mutate production from the candidate.
