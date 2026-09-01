# T91-A Acceptance 只读基线记录

**状态：** `READY_FOR_ROOT_REVIEW`（Task 0 源码核验与验收站只读基线完成）；未连接生产写入，未执行 acceptance migration。

**核验时间：** 2026-09-01（验收站入口和数据库只读检查）。

## 验收站固定基线

- `/admin/lab/health` HTTP 200，返回 `{"status":"ok"}`；登录页 HTTP 200，HTML 资源路径为 `/admin/lab/assets/`。
- `sub2api-acceptance` 六项服务均 healthy，数据库为独立 `sub2api_acceptance`。
- 脱敏计数：users 2、payment_orders 13、payment_audit_logs 28、usage_logs 0、billing_usage_entries 0、user_wallets 2、旧 quota ledger 9、redeem_codes 8、externalization_outbox 1029。
- 当前无负 `users.balance`、无负 wallet paid/gift；订单状态 `CANCELLED=4`、`EXPIRED=2`、`FAILED=4`、`PARTIALLY_REFUNDED=1`、`REFUNDED=2`。
- 无重复 payment trade scope、订单审计 `(order_id, action)` 或 redeem code；兑换码 `used=5`、`unused=3`，无 `legacy_auto/auto` 类型；outbox 仅有 `account.health_changed/pending`。

## 已确认的现网代码基线

1. 余额不足时，`usage_billing_repo.go` 的第二次扣减允许 `users.balance` 变为负数；现有 unit test 将该行为视为预期。
2. 当前扣费投影会把原生余额结果映射到 `user_wallets`，并写入历史 `user_quota_ledger_entries`。
3. 现有 Ent payment order 金额字段生成 `float64`；服务层 quota wallet 已有 `shopspring/decimal` 模型。
4. 通用 externalization outbox 具备 claim/retry/dead 基础，但没有退款消费者。
5. 当前退款 pending 依靠管理员查询接口，不存在自动退款 worker 或退款 webhook 路由。
6. migration runner 使用嵌入 SQL、checksum 和 advisory lock，并已有重复执行与 schema 检查范式。

7. 当前源码没有 `attempted_quota_usd`；`billing_usage_entries.delta_usd` 仍为 `DECIMAL(20,10)`，新额度字段的 `numeric(20,8)` 不能直接复用旧列语义。
8. 旧 `usage_billing_repo` 在余额不足时仍允许负余额；新 `QuotaWalletService` 已 paid-first 且返回 `ErrQuotaInsufficient`，两条路径并存。
9. 退款 provider pending 依靠管理员查询最终确认；没有退款 outbox 事件、退款 webhook 或自动 worker。通用 outbox 不能直接视为退款能力。
10. 旧 Ent/schema 与历史迁移仍存在 `float64` 金额边界及 `ON DELETE CASCADE`，与新账务十进制和 RESTRICT 合同有待设计适配。

## 未执行的场景化验收

- paid 足够；
- paid 不足但 gift 足够；
- paid 与 gift 都不足；
- 历史负 opening；
- 失败且未扣费请求；
- 零费用请求；
- 重复支付回调；
- 重复 API 请求；
- 重复兑换；
- 旧自动核销码。

验收站无 usage/billing 样本，本轮没有通过写入造数补齐这些场景；因此 paid/gift 不足矩阵、失败请求和零费用的端到端行为仍是未验证项。重复支付回调、重复 API 请求、重复兑换和旧自动码已完成源码核对及验收站现存数据的只读核对，但没有发起真实回放或写入。

## 命令结果

- repository usage-billing 定向 unit tests：通过。
- service quota/refund/redeem 定向 tests：被现有测试文件缺少 `context` import 阻塞编译，未修改该无关文件。
- `git diff --check`：通过。
- 所有验收站命令仅为 health/login、Compose 状态和 PostgreSQL `SELECT`；未执行迁移、支付、退款、双写、cutover 或生产操作。
