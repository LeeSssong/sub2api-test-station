# T72 兑换码充值事务边界修复设计

## 状态与范围

- 状态：已获用户确认，候选实现完成，等待根审。
- 基线：`main@801f5a915`（包含 T72 登记文档）。
- 目标：修复兑换码正向余额兑换在外层兑换事务中调用钱包协调器时二次开启事务，导致兑换回滚。
- 非目标：不改变余额/额度计费口径，不回填历史账本，不修改兑换码状态模型，不新增迁移、配置或第二账务事实源。

## 证据与当前行为

生产只读核验显示兑换码 `eb1bc00840de1b7ff6d3c66d7ea1f648` 为 `id=44`、`type=balance`、`value=20`、`status=unused`，说明失败请求没有提交兑换码使用标记或余额变更。代码路径 `RedeemService.Redeem` 已创建 Ent 事务并把 `txCtx` 传给 `QuotaWalletService.LegacyAdjust`；`quotaWalletRepository.WithLockedWallet` 无条件调用 `r.client.Tx(ctx)`，从事务 client 再次开启事务，返回 `ent: cannot start a transaction within a transaction`，外层回滚。

## 设计

`WithLockedWallet` 必须先检查 `dbent.TxFromContext(ctx)`：存在时直接使用该事务 client，锁用户和钱包并在调用回调后返回，不提交/回滚外部事务；不存在时保持现有自有事务生命周期。兑换流程继续在同一外层事务中完成兑换码 `Use`、钱包更新、兼容 `users.balance` 投影和账本写入。负数兑换继续沿用历史“余额下限为 0 且不消费赠送额度”的语义；幂等重试继续按原请求指纹返回历史 mutation。

## 审计与测试矩阵

1. RED/GREEN：已有事务上下文复用测试必须在修复前失败、修复后通过。
2. 兑换码：正向余额兑换原子成功；负向余额兑换仍保护赠送额度边界；失败回调回滚钱包/投影/账本/兑换码。
3. 钱包：充值/退款幂等指纹和重复请求语义不变。
4. 其他余额写入口：支付履约、返利、用量扣费继续使用其既有事务/幂等路径，跑直接相关测试和静态调用审计，不在本任务扩展业务行为。
5. 并发：兑换码数据库状态条件更新和钱包行锁继续保持。

## 发布与验收

无数据库迁移、配置和依赖变化，预期 `downtime_required=false`，以发布预检为准。候选合并后运行直接相关 Go 测试、`go build ./cmd/server`、`git diff --check`；生产发布只从已验证 `main` 走既有蓝绿链。线上先只读确认码仍为 unused，再以受控登录请求兑换该码，核验 HTTP 成功、码变为 used、用户 `users.balance` 与 `user_wallets.paid_quota_balance_usd + gift_quota_balance_usd` 一致、账本新增单条 `redeem_credit`，并确认重复兑换返回已使用且无第二条账本。失败按蓝绿既有回滚，不回滚数据库（本任务无迁移）。

## 用户批准记录

2026-08-26：用户明确回复“确认修复”，批准上述最小修复、直接相关审计、测试、发布和线上专项验证范围。
