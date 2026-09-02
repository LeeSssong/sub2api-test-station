# T91-A Task 0 源码映射与准入报告

**核验日期：** 2026-09-01

**核验基线：** 根目录 `/Users/gongtengxinwen/Documents/sub2api搭建`，分支 `main`，HEAD `e8046cde284ca8d0ca45361b674705e3ae5029ff`，工作区干净；`origin/main` 为 `01b58202e...`，当前根 `main` 领先远端 9 个提交。该基线只允许设计和只读核验，不得用于部署。

**依据：**

- `AGENTS.md`
- `docs/project/native-sub-incremental-delivery-constraints.md`
- `docs/project/native-sub-task-package-queue.md`
- `docs/project/acceptance-station-global-constraints.md`
- `docs/superpowers/plans/2026-09-01-t91-a-quota-accounting-foundation.md`
- 2026-09-01 最新额度账务开发基线附件

## 1. 实际代码映射

| 核验项 | 实际位置 | 当前事实 |
|---|---|---|
| 订单 Ent schema | `upstream/sub2api/backend/ent/schema/payment_order.go:45-125` | `amount`、`pay_amount`、`fee_rate`、`refund_amount` 使用 `field.Float`，数据库类型为 decimal；订单已有 provider instance/key/snapshot 与退款字段。 |
| 订单迁移 | `upstream/sub2api/backend/migrations/092_payment_orders.sql:1-47` | `payment_orders` 已存在；现有支付交易号、退款字段和订单状态索引均为历史 schema。 |
| 支付订单创建/履约 | `upstream/sub2api/backend/internal/service/payment_order.go`、`internal/service/payment_refund.go`、`internal/handler/payment_webhook_handler.go` | 支付回调通过 `HandlePaymentNotification` 履约；回调处理已有重复通知兼容，但尚无 quota grant 事实表。 |
| 支付类型/provider | `upstream/sub2api/backend/internal/payment/types.go`、`internal/payment/provider/*.go` | 已有支付宝、微信、Stripe、Airwallex、EasyPay 等 provider；provider refund/query 接口存在，金额边界仍有 float64 DTO。 |
| 钱包 schema | `upstream/sub2api/backend/ent/schema/user_wallet.go`、`migrations/227_user_quota_wallet_ledger.sql` | `user_wallets` 已有 paid/gift 钱包字段；历史 ledger 与 `users.balance` 兼容投影并存。 |
| 钱包服务 | `upstream/sub2api/backend/internal/service/quota_wallet.go` | 服务层已使用 `shopspring/decimal`；`ConsumeUsage` paid-first 并在余额不足时返回 `ErrQuotaInsufficient`。 |
| 现行 API 扣费 | `upstream/sub2api/backend/internal/repository/usage_billing_repo.go:179-279,334-364` | 计费路径先扣 `users.balance`；余额不足时第二条 SQL 无条件扣减，允许负余额；随后把结果投影到 `user_wallets`/旧 ledger。 |
| 现行余额不足测试 | `upstream/sub2api/backend/internal/repository/usage_billing_repo_unit_test.go:52-75` | 明确把余额从 5 扣 10 得到 -5 作为现有预期，并设置 `sufficient=false`。 |
| billing 表 | `upstream/sub2api/backend/migrations/027_usage_billing_consistency.sql:38-56` | `billing_usage_entries` 已有 `usage_log_id` 唯一索引；新 paid/gift 拆分字段尚不存在。 |
| 兑换码 | `upstream/sub2api/backend/ent/schema/redeem_code.go`、`internal/service/redeem_service.go`、`internal/handler/admin/redeem_handler.go` | 现有 redeem 类型和核销入口存在；本期应仅做兼容核验，不自动把旧类型推断为 grant。 |
| 优惠码 | `upstream/sub2api/backend/ent/schema/promo_code.go`、`internal/service/promo_service.go`、`migrations/033_add_promo_codes.sql` | 现有优惠码/使用记录存在，历史 `bonus_amount` 语义需在迁移报告中保持 unknown。 |
| 退款 service/handler | `upstream/sub2api/backend/internal/service/payment_refund.go:210-470`、`internal/handler/admin/payment_handler.go:220-273` | 当前管理员退款同步修改订单/余额并调用 provider；pending 主要靠管理员查询接口最终确认。 |
| 退款 webhook | `upstream/sub2api/backend/internal/handler/payment_webhook_handler.go:40-130` | 当前入口处理支付入账通知，没有退款回调路由。 |
| 通用 outbox | `upstream/sub2api/backend/internal/events/outbox.go:43-168`、`migrations/200_externalization_outbox.sql` | 已有 append/claim/publish/retry/dead 状态基础；代码内没有退款事件类型或本机退款 worker 调用者。 |
| 迁移注册/执行 | `upstream/sub2api/backend/migrations/migrations.go`、`internal/repository/migrations_runner.go:48-365` | SQL 嵌入、文件排序、checksum、PostgreSQL advisory lock、事务执行与幂等重跑机制已存在；已应用迁移不可修改。 |
| 迁移测试范式 | `upstream/sub2api/backend/internal/repository/migrations_schema_integration_test.go`、`migrations/*_migration_test.go` | 已有 schema/index/constraint、重复执行和历史 checksum 测试模式。 |

## 2. 关键基线与差异

### 2.1 余额不足

现行行为是：先尝试 `balance >= amount` 的原子扣减；无行后再无条件扣减，并返回 `sufficient=false`。新基线要求 `attempted_quota_usd` 表示应扣额度、`delta_usd` 表示实际扣费、未扣差额不进入余额公式且不形成欠款。两者不能直接共存，T91-C 必须在独立任务中重构计费路径；T91-A 只保留基线证据，不改实现。

### 2.2 金额类型

服务层部分模型已使用 `decimal.Decimal`，但 Ent payment order 和支付 DTO 仍使用 `float64`。T91-A 必须先锁定统一十进制适配器和新字段的 Ent/Scanner/Valuer 方案；否则不能开始新增金额字段的 schema 生成。

### 2.3 退款异步能力

通用 outbox 具备基础设施，但当前没有 `quota_refund.requested` 事件、消费者、退款回调路由或自动查询 worker。T91-A 只核验可复用边界；退款 Saga、worker、dead-letter 与 `reconciling` 处理属于 T91-D。

### 2.4 现有 worktree 与发布边界

当前存在多个历史非 `main` worktree，部分为设计、验证或保护状态；根 `main` 领先 `origin/main` 9 个提交。T91-A 不得从候选 worktree 或 detached HEAD 构建/部署，也不得修改其他任务的 worktree、队列或生产记录。

## 3. T91-A 准入判断

**源码核验：通过。** 已找到订单、钱包、计费、退款、provider、outbox、迁移和测试的真实文件与入口。

**schema/Ent 写入：暂缓。** 在以下两项被根总控明确接受前不写入：

1. 新版附件的 `attempted_quota_usd`/实际扣费语义与 T91-C 接口边界；
2. 新账务 `decimal.Decimal` 与旧 Ent `float64` 的统一适配器及字段持久化方案。

**acceptance migration：暂缓。** 当前根 `main` 未与 `origin/main` 同步，且 Task 0 报告尚未被根总控接受；不得执行 acceptance schema migration。

**生产/退款/cutover：禁止。** 本报告未执行生产查询、真实支付/退款调用、历史回填、双写或 cutover。

## 4. 后续交接

- T91-A 下一步可在独立顶层任务中继续完成 schema/Ent 设计和 migration 测试计划。
- T91-B～E 保持未启动。
- 本报告不构成 schema/迁移写入授权，也不构成任何主站发布授权。
