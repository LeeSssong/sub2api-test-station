# T91 充值体系隔离工作进度

日期：2026-09-01

状态：IMPLEMENTING（仅隔离 worktree；根 main 与发布车道已释放）

## 工作区

- Worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t91-quota-accounting`
- 分支：`codex/t91-quota-accounting`
- HEAD：`a4580f476fbb61f89ddbcba3a8f5a5429313ec2f`
- HEAD tree：`db29b1e9cbcee31560f657783e7a43892f8e8342`

## 本次继续完成

- 直接充值订单写入 `confirmed` quota snapshot；CNY 与现有外币余额订单沿用既有 amount→paid 规则。
- 支付履约按 `payment_order:<id>` 幂等生成 `payment_order` grant，并同步 wallet/users.balance 旧投影。
- API billing 在实际成功扣费时按 paid→gift 记录 attempted/delta/attribution；不足时不新增负余额。
- 增加 gift FIFO 管理员扣减服务，排除 `migration_opening`，支持幂等。
- 增加 paid FIFO 退款额度、退款上限领域规则；`force_refund` 不绕过普通额度上限。
- 增加退款 Saga 状态转换规则，禁止可重试状态直接进入 `failed`，并保证 `completed/failed` 终态不可回退。
- provider 成功后的确认退款会创建幂等 `refund_recovery` adjustment，按 paid FIFO 更新 grant、订单累计回收额度与钱包投影；历史未知订单不伪造新事实。
- 修复 Decimal Ent 字段的 SQLite 方言映射，避免既有 SQLite 测试建表失败。

## 验证

通过：

```text
git diff --check
go vet ./internal/quota/accounting ./internal/service ./internal/repository
go test ./internal/quota/accounting ./internal/service ./internal/repository -run 'Test(Quota|UsageBilling|PaymentOrderQuotaSnapshot|RefundQuotaLimit|ValidateRefundAgainstQuota)' -count=1
go test -tags=unit ./internal/repository -run TestSplitQuotaDeductionPaidFirstThenGift -count=1
```

## 尚未完成

- reservation、provider outbox/worker/retry/dead-letter/callback，以及事务内最终化路径的统一 adjustment 写入。
- 管理员标准 `admin_recharge` 订单接口和 webhook/fulfillment 全面幂等验收。
- 全仓历史切换、双写、cutover 与 acceptance/生产发布。

未经发布总控明确授权，不得合并、推送、部署或触碰根 `main`。
