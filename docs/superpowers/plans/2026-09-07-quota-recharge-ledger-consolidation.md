# 充值—额度—退款体系收敛实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在保留 Sub2API 原生消费与支付生命周期的前提下，统一充值、赠送、额度投影和退款语义，并为后续删除现金字段建立无依赖基线。

**Architecture:** 第一阶段复用 `UserQuotaWallet`、`QuotaGrant`、`QuotaLedger`、`payment_orders` 和现有 provider Saga。所有余额变化经统一额度协调器在同一事务中更新 paid/gift 钱包与 `users.balance = paid + gift`；管理员账务退款只回收具体代充值订单的 paid quota，外部退款继续使用 provider 流程。第二阶段在完成全仓依赖清理和迁移合同后删除 `cash_balance_cny` 及现金快照字段。

**Tech Stack:** Go、Ent、PostgreSQL、Vue 3、TypeScript、Vitest、现有本地/宿主发布链。

---

## 阶段一：额度语义与入口收敛

### Task 1: 建立额度协调器行为合同

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/quota_accounting.go`
- Modify: `upstream/sub2api/backend/internal/repository/quota_wallet_repo.go`
- Test: `upstream/sub2api/backend/internal/service/*quota*test.go`
- Test: `upstream/sub2api/backend/internal/repository/quota_wallet_repo_test.go`

- [ ] 写失败测试：任何 paid/gift 变更都在同一事务更新钱包与 `users.balance`，并验证 `balance = paid + gift`。
- [ ] 写失败测试：管理员 gift add/subtract 不改变 paid、不创建退款资格；不足时拒绝扣减。
- [ ] 写失败测试：幂等键重复请求返回原结果且不重复变更版本或账本。
- [ ] 实现最小协调器入口，统一接受 paid delta、gift delta、来源、订单、操作人和幂等键。
- [ ] 将旧余额调整调用改为通过协调器，保留历史读取兼容。
- [ ] 运行定向 service/repository 测试并提交阶段性 commit。

### Task 2: 收敛真实充值、手工充值和管理员代充值

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/payment_fulfillment.go`
- Modify: `upstream/sub2api/backend/internal/service/admin_recharge.go`
- Modify: `upstream/sub2api/backend/internal/service/quota_accounting.go`
- Modify: related admin payment/user handlers and repositories identified by tests
- Test: related service/repository/handler tests

- [ ] 写失败测试：外部支付确认产生 paid grant；活动赠送产生独立 gift grant，并对重复回调幂等。
- [ ] 写失败测试：有人民币凭证的管理员手工充值使用 `manual_recharge` 来源并进入可退款池；无凭证只能是 `admin_gift`。
- [ ] 写失败测试：管理员代充值必须有交易单号，创建 `admin_recharge` 订单，付费额度与赠送额度分开入账。
- [ ] 实现上述入口，禁止通过伪造 `admin_balance` 兑换码表达新充值事实。
- [ ] 保持普通余额兑换码原流程和 `pay-` 自动核销；仅将注册优惠码、注册默认额度和首次绑定默认额度改为 gift grant。
- [ ] 运行定向测试并提交阶段性 commit。

### Task 3: 完成双退款流程的额度语义

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/payment_refund.go`
- Modify: `upstream/sub2api/backend/internal/service/quota_refund_reservation.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/payment_handler.go`
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.vue`
- Modify: related payment types, routes, order views and locales
- Test: related Go and Vitest specs

- [ ] 写失败测试：账务退款必须指定 `admin_recharge` 订单、唯一退款交易号和原因；外部订单调用账务接口被拒绝。
- [ ] 写失败测试：同一订单部分退款累计不超过 `paid_quota_usd - refunded_paid_quota_usd`，并发退款不会超退。
- [ ] 写失败测试：余额不足时普通模式拒绝；`force` 只回收实际 paid，不扣 gift，不突破订单上限。
- [ ] 写失败测试：外部退款成功、pending、失败回滚和重复回调保持原 provider 语义，失败回滚只恢复 paid。
- [ ] 实现统一 reservation/finalization 和订单累计退款，确保所有写入经过额度协调器。
- [ ] 前端区分“账务退款”和“支付渠道退款”，显示充值额度、赠送额度、可用额度和订单剩余可账务退款额。
- [ ] 运行定向后端、前端、typecheck 和 build 检查并提交阶段性 commit。

### Task 4: 阶段一范围审计与交接

- [ ] 扫描所有 `cash_balance_cny`、现金 delta、`admin_balance` 新写入和绕过协调器的余额写入。
- [ ] 确认 `usage_logs` 仍是模型消费事实，未新增重复消费流水。
- [ ] 运行 `gofmt`、Go 定向测试、前端 Vitest、`pnpm typecheck`、前端 production build 和 `git diff --check`。
- [ ] 记录迁移、配置、数据、凭据、`downtime_required`、回滚方式和未验证项。
- [ ] 形成阶段一交接，状态进入 `READY_FOR_ROOT_REVIEW`；不自行合并、推送或部署。

## 阶段二：现金字段删除

### Task 5: 清理现金字段运行时依赖

- [ ] 在阶段一验收通过后，删除钱包查询、初始化、更新、报表和前端摘要对 `cash_balance_cny` 的读写与展示。
- [ ] 将退款上限全部改为订单 paid quota 剩余额，不再使用现金与额度的 `min` 计算。
- [ ] 更新 Ent schema/生成物、查询合同和所有直接相关测试。
- [ ] 运行全量 `rg` 依赖审计，允许仅保留迁移历史、兼容说明和删除迁移测试中的预期文本。

### Task 6: 删除 migration 与回滚合同

**Files:**
- Create: next numbered migration under `upstream/sub2api/backend/migrations/`
- Modify: migration registry/checksum and migration tests as required by existing conventions

- [ ] 写失败迁移合同：钱包表不再含现金字段，现金快照流水字段同步删除。
- [ ] 实现 additive/可回滚 schema migration；回滚只恢复列定义，不伪造历史现金数据。
- [ ] 运行 migration checksum、schema contract、相关 repository/service 测试。
- [ ] 记录 migration hash、停机预检结果和回滚命令，交由根总控决定发布。

### Task 7: 最终整合验证

- [ ] 在干净根 `main` 上合入候选后确认 `HEAD == origin/main`，运行直接相关回归、构建、迁移检查和发布预检。
- [ ] 仅在明确主站授权且 `downtime_required=false` 时发布；若为 `true`，在停机/迁移前暂停等待授权。
- [ ] 测试站与主站分别记录 source commit/tree、image digest、健康检查和版本一致性。
- [ ] 线上验证充值、赠送、兑换码、管理员代充值、两类退款、pending/失败回滚、并发幂等及现金字段不再依赖。

