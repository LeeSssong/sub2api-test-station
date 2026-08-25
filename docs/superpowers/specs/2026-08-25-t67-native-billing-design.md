# T67 完全恢复 Sub 原生用户扣费设计规格

**状态：** 已批准（2026-08-25）
**日期：** 2026-08-25
**基线：** main@e07b9cced6576f2206e5b3467112d3358bc96417
**候选 worktree：** .worktrees/t67-native-billing
**候选分支：** codex/t67-native-billing

## 1. 问题证据与当前行为

生产账号 `940310446@qq.com`（user_id=1）在 22:16:06 完成 ¥100 手动充值；随后两笔 `usage_consumption` 流水将余额扣至 `$99.96431384`，说明充值和钱包写入成功。22:16:23 起仍连续返回“余额不足”，因为管理员充值链只更新 `user_wallets` 与兼容投影 `users.balance`，没有让 `billing:balance:<user_id>` Redis 准入缓存失效。

更严重的现状是：余额接近 `$0.00006754` 时，准入检查只验证余额为正，后续请求可先到达上游；`QuotaWalletService.ConsumeUsage` 在实际成本大于额度时返回 `ErrQuotaInsufficient`，调用链随后把 `usageLog.ActualCost` 置为 0 并记录流水。这会造成“上游已执行、本站未扣费、请求仍可继续”的透支窗口。

当前原生准入入口为 `BillingCacheService.CheckBillingEligibility`，统一扣费入口为 `usageBillingRepository.Apply`。T55 改动在 `Apply` 中调用 `QuotaWalletService.ConsumeUsage`，替代了官方原生 `deductUsageBillingBalance`；官方旧路径在余额不足时仍执行一次 `users.balance = balance - cost`，返回 `BalanceOverdrafted=true`，随后缓存失效使下一次请求 fail-closed。

## 2. 目标与非目标

### 目标

1. 推理余额扣减完全恢复 Sub 原生语义：以 `users.balance` 为唯一运行时余额事实源，在同一 usage billing 事务内扣减，并保留官方既有的一次技术透支语义。
2. 成功扣费后继续使用既有原生缓存同步：余额耗尽或低于保留阈值时删除 Redis 余额缓存，否则按原生扣减路径更新缓存。
3. 手动充值/退款完成后失效对应 `billing:balance:<user_id>` 缓存，保证充值后下一次请求回源读取最新 `users.balance`。
4. 保留 T55 钱包表、流水和管理员额度页面作为充值/退款审计及兼容投影，不把它们作为推理扣费事实源。
5. 不为修复历史 `actual_cost=0` 流水做回填、重算、补扣或数据删除。

### 非目标

- 不调整模型价格、用户倍率、上游账号余额、调度、T66 故障隔离或错误中文文案。
- 不删除 `user_wallets`、`user_quota_ledger_entries`、`quota_idempotency_records` 或修改既有迁移。
- 不新增支付渠道、自动支付、第二账务事实源、预扣额度模型或历史账务回填。
- 不通过关闭准入检查、忽略 `ErrQuotaInsufficient` 或放宽余额阈值解决问题。

## 3. 方案比较与选择

### 方案 A：保留钱包推理扣费并做严格预扣

在请求发出前根据预估成本冻结钱包额度，再在成功后结算。优点是可严格禁止超额；缺点是会新增请求预估、冻结、释放和流式失败语义，偏离 Sub 原生扣费并扩大账务范围。

### 方案 B：保留钱包扣费但在余额不足时强制拒绝

继续由钱包消费服务作为推理事实源，只把 `ErrQuotaInsufficient` 映射为请求前拒绝。优点是改动小；缺点是仍保留第二套扣费语义，与 `users.balance`/缓存可能分裂，也不能称为完全恢复原生逻辑。

### 方案 C（推荐）：推理扣费回到 `users.balance` 原生事务路径

恢复 `usageBillingRepository.Apply` 中官方 `deductUsageBillingBalance` 分支，移除推理路径对 `QuotaWalletService.ConsumeUsage` 的调用；保留钱包服务给管理员充值/退款和其他既有审计投影使用。充值成功后通过既有 `BillingCacheService.InvalidateUserBalance` 删除缓存。该方案改动最小、直接复用原生扣费与缓存语义，并消除当前“上游执行但本站扣费失败”的分裂。

**选择：方案 C。**

## 4. 端到端数据流与接口契约

### 推理成功扣费

1. Handler 完成上游调用并生成完整 `UsageBillingCommand`。
2. `usageBillingRepository.Apply` 开启现有事务并执行 usage billing 幂等声明。
3. 余额模式调用原生 `deductUsageBillingBalance(ctx, tx, user_id, balance_cost)`：优先 `WHERE balance >= cost`；若无行，再校验用户存在并执行官方兼容更新，返回新的 `users.balance` 和 `BalanceOverdrafted=true`。
4. 同一事务继续更新 API Key、限流和账号配额；任一失败整体回滚。
5. `finalizePostUsageBilling` 根据返回的新余额执行既有缓存同步：低于准入阈值则失效缓存，否则按扣费额更新缓存。
6. `usage_logs.actual_cost` 保持原生成功请求实际扣费值；只有原生失败边界才保留既有错误处理。

### 管理员手动充值/退款

1. `CreateQuotaLedgerEntry` 继续调用 T55 钱包协调器，原子更新 `user_wallets`、兼容投影 `users.balance` 和账本。
2. 成功提交后立即调用余额缓存失效；后续准入检查回源读取 `users.balance`。
3. 充值幂等重放仍返回原账本结果，并再次失效该用户缓存；退款失败不改变缓存。

### 运行时事实源边界

- 推理准入/扣费：`users.balance`，由 Sub 原生代码读取和更新。
- 管理员充值/退款审计：T55 钱包与账本，`users.balance` 是其兼容投影。
- Redis `billing:balance:<id>`：仅为准入缓存，不是事实源；任何余额写入成功后必须失效或按原生路径同步。

## 5. 失败与安全语义

- 余额为 0：准入检查返回原生 `ErrInsufficientBalance`，不上游。
- 余额为正但不足本次实际费用：恢复官方兼容语义，允许该已发起请求扣成负数一次；扣费后删除余额缓存，下一次请求被拒绝。
- 余额充值后立即请求：缓存已删除，回源取得充值后余额，不误报余额不足。
- Redis 缓存删除失败：记录告警，保持既有原生错误处理，不伪造缓存已刷新。
- 钱包账本写入失败：事务回滚，充值前后 `users.balance` 不变；前端继续显示后端错误。
- 推理账务事务失败：保留现有错误返回与审计行为，不引入钱包补扣。
- 幂等重试：usage billing dedup 与钱包手动操作幂等键继续分别生效，不复用或交叉污染。

## 6. 兼容性、迁移与配置

- 数据库迁移：无。
- 配置变化：无。
- API schema：无新增字段；管理员充值接口保持现有请求/响应。
- 历史数据：不回填、不修正历史 `actual_cost=0`，保留 T55 钱包/账本和现有 usage 日志。
- 回滚：恢复 `usage_billing_repo.go` 到当前 T55 钱包扣费版本，并恢复充值缓存行为；数据库无需回滚。

## 7. 场景化验收矩阵

| 场景 | 预期 | 证据 |
|---|---|---|
| 余额为 0 的标准请求 | 准入拒绝，不调用上游 | Billing eligibility test |
| 余额足够的成功请求 | 原生 `users.balance` 减少，usage billing 成功 | repository test |
| 余额不足但请求已进入扣费 | `users.balance` 可按原生语义变负，返回 `BalanceOverdrafted`，缓存失效 | repository/service test |
| 余额不足后的下一请求 | 原生准入拒绝 | cache eligibility test |
| 手动充值后立即请求 | `users.balance` 与钱包同步，缓存失效，下一次准入读取新余额 | wallet/handler/repository test |
| 手动充值重复提交 | 只产生一条账本，重放结果，缓存仍失效 | idempotency test |
| 充值失败 | 不改余额、不失效错误用户缓存，前端显示后端错误 | handler/service test |
| 既有 T66 错误与切号 | 行为不变 | targeted regression |

## 8. 测试策略

- 先写 RED：锁定 `usageBillingRepository.Apply` 不再调用钱包消费、原生 `users.balance` 扣减、余额不足后的负余额与缓存失效，以及充值成功后的缓存失效。
- GREEN：只修改直接相关 repository/service/handler 文件及测试。
- 保留门禁：后端直接相关 Go 测试、`go build ./cmd/server`、前端直接相关 Vitest、`pnpm typecheck`、`pnpm build`、`gofmt`、`git diff --check`。
- 不运行无关全仓回归，不执行真实生产充值、退款或模型请求。

## 9. 发布、线上验证与停止条件

- 候选完成后只能进入 `READY_FOR_ROOT_REVIEW`，不得自行合并、推送、部署。
- 根合并后在最新 `main` 运行直接相关测试、构建与发布预检。
- 若发布预检 `downtime_required=false`，按既有本地/宿主蓝绿链发布并线上验证；若为 `true`，在任何停服/迁移/重启前暂停等待用户授权。
- 线上专项验证只读：健康端点、生产源身份、管理员额度摘要读取；不制造生产扣费或充值写入。
- 若部署或验证失败，保留候选、证据与回滚依据，在同一候选继续修复。

## 10. 待决事项与批准记录

- 用户已在 2026-08-25 明确批准产品方向：“完全恢复官方原生扣费”。
- 用户已在 2026-08-25 明确批准本书面规格，允许生成实施计划并进入候选 worktree 实施。
- T66-R1 已占用当前发布车道，本任务不进入合并/部署车道，直至根总控释放。
