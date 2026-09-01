# T91-A Acceptance 只读基线记录

**状态：** 设计/核验阶段，未连接生产写入，未执行 acceptance migration。

## 已确认的现网代码基线

1. 余额不足时，`usage_billing_repo.go` 的第二次扣减允许 `users.balance` 变为负数；现有 unit test 将该行为视为预期。
2. 当前扣费投影会把原生余额结果映射到 `user_wallets`，并写入历史 `user_quota_ledger_entries`。
3. 现有 Ent payment order 金额字段生成 `float64`；服务层 quota wallet 已有 `shopspring/decimal` 模型。
4. 通用 externalization outbox 具备 claim/retry/dead 基础，但没有退款消费者。
5. 当前退款 pending 依靠管理员查询接口，不存在自动退款 worker 或退款 webhook 路由。
6. migration runner 使用嵌入 SQL、checksum 和 advisory lock，并已有重复执行与 schema 检查范式。

## 需要在验收站建立、但本轮尚未执行的基线场景

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

这些场景涉及 acceptance 运行态和独立数据，必须在 Task 0 独立顶层任务中按验收站全局约束执行；当前没有读取或记录任何敏感凭据，也没有写入验收站或生产数据。
