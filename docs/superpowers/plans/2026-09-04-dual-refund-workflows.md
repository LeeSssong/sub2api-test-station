# 双退款流程与额度语义实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将管理员代充值退款与外部支付渠道退款彻底分流，并让管理员退款只扣充值额度、保留赠送额度。

**Architecture:** 复用现有 `payment_orders`、`user_quota_grants`、`user_quota_adjustments` 和 provider Saga。管理员账务退款走原生 quota accounting reservation/finalization，不调用外部 provider；外部支付退款保留现有 provider 流程。前端按订单支付类型和用户记录标签页选择对应 API。

**Tech Stack:** Go 1.27、Ent、PostgreSQL、Vue 3、TypeScript、Vitest、pnpm、现有测试站发布链。

---

### Task 1: 后端账务退款领域服务

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/quota_refund_reservation.go`
- Modify: `upstream/sub2api/backend/internal/service/quota_accounting.go`
- Modify: `upstream/sub2api/backend/internal/service/payment_refund.go`
- Test: `upstream/sub2api/backend/internal/service/*refund*_test.go`

- [ ] 写失败测试：`admin_recharge` 订单可创建账务退款 adjustment，外部订单被拒绝，gift 余额保持不变。
- [ ] 写失败测试：同一订单多次部分退款的剩余 paid 上限、重复 refund_trade_no 冲突和并发锁。
- [ ] 实现管理员账务退款 reservation/finalization，填充 `refund_method=admin_accounting`、`refund_trade_no`、`payment_order_id`、`operator_user_id`。
- [ ] 将成功退款累计写入 `payment_orders.refunded_paid_quota_usd`，仅扣 `user_wallets.paid_quota_balance_usd`，不修改 gift。
- [ ] 将外部 provider 分支与管理员账务分支隔离，管理员订单不再查 provider。
- [ ] 运行定向 Go 测试并提交。

### Task 2: 管理员退款 API 与订单筛选

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/admin/payment_handler.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go`
- Modify: `upstream/sub2api/frontend/src/api/admin/payment.ts`
- Modify: `upstream/sub2api/frontend/src/types/payment.ts`
- Test: `upstream/sub2api/backend/internal/handler/admin/payment_handler_test.go`

- [ ] 写失败 API 测试：管理员账务退款要求订单类型、金额、退款交易号和原因。
- [ ] 写失败 API 测试：外部支付退款仍要求 provider 参数，不能走账务退款接口。
- [ ] 实现独立账务退款 endpoint；后端再次校验 `admin_recharge` 和交易号格式/唯一性。
- [ ] 增加查询管理员订单剩余可账务退款额度的响应字段。
- [ ] 运行定向 handler/service 测试并提交。

### Task 3: 用户记录双退款标签页

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/admin/user_handler.go`
- Modify: `upstream/sub2api/frontend/src/api/admin/users.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.vue`
- Create: `upstream/sub2api/frontend/src/components/admin/user/AdminAccountingRefundModal.vue`
- Create: `upstream/sub2api/frontend/src/components/admin/user/PaymentChannelRefundModal.vue`
- Test: related Go/Vitest specs

- [ ] 写失败组件测试：标签页切换只显示对应订单集合。
- [ ] 实现管理员账务退款订单列表，只返回该用户 `admin_recharge` 且仍有剩余 paid 的订单。
- [ ] 实现支付渠道退款订单列表，只返回 provider 可退款的外部订单。
- [ ] 账务退款弹窗要求唯一退款交易号，支付渠道退款弹窗沿用 provider 退款字段。
- [ ] 运行定向前后端测试并提交。

### Task 4: 文案与金额语义

**Files:**
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/overview.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/overview.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/resources.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/resources.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceModal.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/user/UserBalanceHistoryModal.vue`

- [ ] 将“付费额度”统一改成“充值额度”，将“当前可消费额度”改成“可用额度”。
- [ ] 将“可退款现金余额”改成“可账务退款金额”。
- [ ] 删除或改写“退款会清空当前剩余赠送额度”的旧提示。
- [ ] 增加管理员账务退款只扣充值额度的提示。
- [ ] 运行文案和组件测试并提交。

### Task 5: 订单管理操作与详情

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/admin/payment/AdminOrderTable.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/payment/AdminOrderDetail.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/orders/AdminOrdersView.vue`
- Test: `upstream/sub2api/frontend/src/components/admin/payment/__tests__/*.spec.ts`

- [ ] 写失败测试：管理员代充值显示“账务退款”，外部订单显示“支付渠道退款”。
- [ ] 实现两类按钮分别调用独立事件/API；管理员订单不进入 provider 退款弹窗。
- [ ] 详情显示剩余可账务退款额度、退款交易号和累计已退充值额度。
- [ ] 更新筛选和空状态文案。
- [ ] 运行定向前端测试并提交。

### Task 6: 汇总验证与测试站发布

- [ ] 在 `main` 合入候选前运行 Go 定向测试、前端 Vitest、typecheck、build、diff-check。
- [ ] 核对无新增不必要 migration；若修改现有 migration，记录 hash 与回滚方式。
- [ ] 推送候选分支，生成交接报告：原始测试站 commit/tree/image digest、候选 commit、变更文件、测试、数据/凭据边界和回滚。
- [ ] 根 `main` 合并并推送后，从干净 `main` 发布独立测试站。
- [ ] 线上验证：管理员订单两类退款入口、部分退款上限、gift 保留、重复退款交易号拒绝、外部 provider 退款路径和健康端点。
