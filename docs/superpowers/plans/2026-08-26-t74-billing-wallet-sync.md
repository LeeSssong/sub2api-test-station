# T74 实施计划：原生扣费与充值额度账本一致性

基线：`main@6d30ac9ef5526aa6860969baf6806ddd604bf8bc`。规格自审结论：目标、数据契约、失败语义、验证、发布与回滚条件闭环；生产历史对账明确排除在本包之外。

1. 在 `usage_billing_repo_unit_test.go` 先添加失败用例：余额扣费后钱包更新与消费流水、赠送额度回退、一次技术透支、重复请求不重复投影、零成本不投影。
2. 在 `usage_billing_repo.go` 新增只接受现有 `*sql.Tx` 的钱包投影辅助函数；从原生扣费返回的实际新余额驱动分摊与不变量校验，保持原 `deductUsageBillingBalance`、dedup claim 顺序和返回语义不变。
3. 为 `UserBalanceHistoryModal` 添加单元测试，先证明旧用户快照不能作为当前余额；再让历史与充值弹窗均在开启时获取 `quota-summary`，仅显示刷新摘要。
4. 运行定向 Go/Vitest 测试，修正实现到绿色；接着运行 Go build、前端 typecheck/build、gofmt、diff-check。
5. 记录只读生产漂移影响查询和逐用户可审计对账操作设计，写 handoff；候选只进入 `READY_FOR_ROOT_REVIEW`，不触碰主线、生产、推送或部署。
