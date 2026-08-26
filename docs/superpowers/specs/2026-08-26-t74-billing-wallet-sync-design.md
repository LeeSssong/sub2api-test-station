# T74 原生扣费与充值额度账本一致性修复 — 规格书

## 批准记录

- 任务包：T74；基线：`main@6d30ac9ef5526aa6860969baf6806ddd604bf8bc`。
- 用户在 2026-08-26 明确批准“按你方案进行修改”。此前已说明根因与推荐方案；本规格将该已批准范围固化。
- 发布总控复核：范围未引入新计费事实源、汇率、迁移或自动生产数据改写，符合原生优先和最小扩展约束；批准进入实施计划阶段。

## 证据与问题

Sub 原生运行时余额事实源为 `users.balance`。T55 的人工充值同时更新 `user_wallets`/`user_quota_ledger_entries` 与兼容字段 `users.balance`；T67 恢复原生推理扣费后，`usage_billing_repo.go` 只扣 `users.balance`，不再写钱包消费投影。故每次成功请求都会令管理员“充值”页读到的钱包余额高于实际网关可用余额。

2026-08-26 生产只读样本：

- `290435516@qq.com`：当时 `users.balance=18.84689040`，钱包额度总和 `20.57804890`，差额 `1.73115850`。
- `xuebii@qq.com`：当日充值 `¥110.00000000`；查询时 `users.balance=133.25238771`，钱包额度总和 `148.72738791`，差额 `15.47500020`。

另一个独立界面问题是 `UserBalanceHistoryModal` 在打开时直接显示传入用户列表快照的 `user.balance`，不刷新额度摘要；这解释了“历史弹窗 $0.33，点击充值后 $3.3”的视觉不一致。

## 目标与非目标

目标：

1. 每次原生推理余额扣费在同一个数据库事务内同步写入钱包和审计流水，使 `users.balance = paid_quota_balance_usd + gift_quota_balance_usd`。
2. 保持 Sub 原生扣费的一次技术透支、请求去重和网关返回语义不变。
3. 管理端两个余额弹窗在打开时显示服务端刷新的余额摘要，不再把旧列表快照作为“当前余额”。
4. 提供既有漂移用户的只读影响查询与可审计的后续对账方案。

非目标：

- 不改变 `¥1 = 1` 内部额度、充值/退款价格或 FX 语义。
- 不改订阅、API Key、账号额度、冻结额度与批量图片扣费路径。
- 不创建平行余额事实源，不更改历史流水，不自动批量修正生产数据。
- 不直接给任何用户写补款；补款须基于临近执行时的读数并使用既有管理员充值审计入口。

## 方案比较与选择

1. 仅前端改读 `users.balance`：快速但钱包审计账本仍错误，拒绝。
2. 推理扣费后另起钱包服务事务：会在原生扣费成功后留下失败窗口，且现有 `ConsumeUsage` 会拒绝原生允许的一次技术透支，拒绝。
3. 在原生 `usage_billing_dedup` 已成功声明的同一 `*sql.Tx` 中，按真实扣费结果同步钱包、写 `usage_consumption` 审计流水，并刷新 UI：选用。它复用原生事务、行锁、请求去重和运行时余额语义，新增仅为兼容投影。

## 数据与控制流契约

1. `usage_billing_dedup` 成功 claim 后，执行原有 `deductUsageBillingBalance`。
2. 在同一事务中锁定或初始化 `user_wallets`。若不存在，以扣费前余额初始化为 paid 额度；不得另开事务。
3. 钱包消费按 paid-first、gift-second 分摊。若原生扣费进入技术透支，剩余超额全部记为 paid 负数，gift 不得为负；更新后钱包总额必须等于 `deductUsageBillingBalance` 返回的新余额。
4. 更新钱包版本并插入 `user_quota_ledger_entries(record_type='usage_consumption')`；`reference_type='usage_billing'`、`reference_id=RequestID`，以既有 `usage_billing_dedup` 作为唯一幂等边界，不使用第二个独立事务。
5. 任何钱包投影失败都回滚该原生扣费及 dedup claim；重试只会成功创建一次消费流水。
6. `UserBalanceModal` 和 `UserBalanceHistoryModal` 打开时都调用 `GET /admin/users/:id/quota-summary`，余额标签只显示成功获取的摘要；摘要失败显示加载中/不可用，不用旧快照伪装为实时数值。

## 失败、安全与兼容性

- 用户不存在、锁定/SQL 错误：延续原生错误并回滚事务。
- 成本为零：不创建钱包消费流水。
- 相同请求 ID 重试：返回 `Applied=false`，不重复扣余额或写流水。
- 钱包缺失：事务内从扣费前的 `users.balance` 初始化，确保老用户第一次推理后仍满足恒等式。
- 已漂移的历史钱包不自动回填；新扣费仅保持后续变化一致。上线后先跑只读差异报告，再以独立批准的、带 `legacy_balance_adjustment` 的工具/管理操作逐用户或批量对账。

## 验收矩阵与测试

| 场景 | 预期 |
| --- | --- |
| 付费余额足够 | `users.balance` 与钱包总额同减，产生一条 paid 消费流水 |
| 付费不足、赠送足够 | paid 归零、gift 承担余量，两个总额一致 |
| 一次技术透支 | paid 可为负、gift 不为负、钱包总额等于新的负运行时余额 |
| 同请求重试 | 不重复写钱包或消费流水 |
| 零余额成本 | 不写钱包消费流水 |
| 两个余额弹窗 | 打开时读取刷新摘要，旧 `AdminUser.balance` 不作为当前余额回退 |

运行直接相关 Go repository tests、前端组件 tests、必要 Go build、前端 typecheck/build、gofmt 与 `git diff --check`。无迁移；发布前预检须返回 `downtime_required=false`，否则停止等待确认。回滚为回退 T74 代码发布；历史/用户数据不由本任务改写。

## 待决项

历史漂移总量与逐用户对账执行是独立、可影响业务账务的数据操作；本任务只产出只读报告和受控方案，待用户明确授权后执行。
