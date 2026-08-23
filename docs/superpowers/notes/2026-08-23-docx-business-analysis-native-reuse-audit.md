# DOCX 经营分析需求：Sub 原生复用与改造审计

**日期：** 2026-08-23

**需求源：** `/Users/gongtengxinwen/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/wxid_9944479444511_c3ab/msg/file/2026-08/星桥AI-Link-经营分析开发需求.docx`

**当前阶段：** 发现与规格准备；未修改业务代码、未创建经营分析实现分支、未部署。

## 1. 结论先行

DOCX 经营分析不能直接套用现有“账号财务”页面。现有 Sub 原生链路可以复用相当一部分查询、聚合和展示基础，但 DOCX 的核心收入口径依赖“付费额度/赠送额度来源拆分”，这在当前生产代码中还不存在，必须等待 T55 额度钱包与手动充值账本提供稳定的额度来源和流水接口。

因此建议按以下顺序推进：

1. 先完成并验证 ZIP 对应的 T55 钱包拆分与手动充值/退款能力；
2. 再单独创建 DOCX 经营分析任务，复用 `usage_logs` 原生聚合和 T55 钱包流水；
3. 对“分组预设上游平均倍率”单独做规格决策，不能把 T54 调度预设权重直接当成财务路由概率。

## 2. 当前可直接复用的 Sub 原生能力

| DOCX 需求 | 可复用能力 | 现有证据 | 结论 |
|---|---|---|---|
| 用户实际消耗/站内扣费 | `usage_logs.actual_cost`、请求时间、Token、`group_id` | `upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go`、`account_profitability.go` | 可复用为原始消耗和本站当前扣费基础，但尚不能区分付费/赠送来源 |
| 上游实际成本 | `account_cost`，回退 `account_stats_cost/total_cost * account_rate_multiplier` | `upstream/sub2api/backend/internal/service/account_profitability.go`、`account_cost_sql_test.go` | 可复用现有成本表达式；缺失成本必须保留 pending/unavailable，不能默认为 0 |
| 分组归属 | 不可变的 `usage_logs.group_id` 及原生分组表 | `usage_log_repo_stats.go`、管理员分组 API | 可复用；未归组单独投影 |
| 账号/分组/全站聚合 | `ReadAccountFinancialUsage` 和 `AccountFinancialService` 的层级聚合 | `upstream/sub2api/backend/internal/service/account_financial.go` | 可复用聚合结构，但需要改成 DOCX 的 CNY/Q 口径 |
| 管理员鉴权和路由 | 原生 `/api/v1/admin/operations/*` 路由和管理员中间件 | `upstream/sub2api/backend/internal/server/routes/admin.go` | 可复用，不新增平行控制面 |
| 经营页前端基础 | `AccountProfitabilityView.vue`、`accountFinancial.ts`、既有时间范围/刷新/错误态测试 | `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue` | 可复用组件结构和请求状态，不直接复用字段语义 |
| 分组站内倍率 | 分组倍率/用户分组倍率现有接口与 `rate_multiplier` 字段 | `upstream/sub2api/backend/internal/service/admin_user.go`、分组管理路由 | 可作为“当前站内倍率”来源；需要明确快照时间语义 |

## 3. 必须在原生链路上改造的部分

### 3.1 收入口径

DOCX 要求：

```text
站内收入 = 期间内付费额度实际消耗折算金额
赠送额度消耗 = 不计收入，但对应上游成本仍计入成本
```

当前原生 `usage_logs.actual_cost` 只有请求的总本站扣费，没有记录该次扣费来自付费额度还是赠送额度。因此必须由 T55 在消费事务中提供：

- `paid_consumption`；
- `gift_consumption`；
- 每笔消费对应的额度流水或可关联引用；
- 失败/无法拆分时的 `revenueStatus=pending_split` 语义。

没有这些字段，DOCX 的收入、毛利和毛利率不能安全实现。

### 3.2 现金充值、付费额度发放和净沉淀

现有 `payment_orders` 是真实支付订单事实，不等于 DOCX 阶段一要求的人工充值录入；现有 `users.balance` 也不能证明人民币现金余额。

需要复用/扩展 T55 提供的：

- `cash_balance_cny`；
- `paid_quota_balance_usd`；
- `gift_quota_balance_usd`；
- 充值流水中的现金、付费额度、赠送额度变化；
- 按北京时间统计的充值和额度发放时间。

DOCX 的净沉淀只能计算为：

```text
期间现金充值 - 期间付费额度实际消耗折算金额
```

不能直接拿 `users.balance` 的变化替代。

### 3.3 CNY / Q 展示

现有账号财务接口固定返回 `currency=USD`，前端 `accountFinancial.ts` 也按 USD 归一化。DOCX 要求：

- 经营结果统一 CNY；
- 站内额度显示 `$`，但明确 `$` 只是内部 Q 单位，不是美元；
- 历史换算使用记录发生时保存的口径，不使用查询当天汇率。

因此需要新 DTO 或明确版本化扩展，不能仅把现有 `currency` 字符串改成 `CNY`。

### 3.4 时间范围与趋势

现有 `account-financial` 仅支持 `today / 24h / 7d / 31d`，DOCX 还要求：

- 今天；
- 近 7 天；
- 近 30 天；
- 本月；
- 上月；
- 自定义开始/结束日期；
- Asia/Shanghai 自然日聚合。

充值趋势、付费消耗趋势和净沉淀趋势还需要读取 T55 流水，不是现有 usage-only 聚合可以单独完成的。

### 3.5 分组毛利

实际分组毛利可以在现有原生数据上扩展：

```text
实际收入 = 付费额度消耗
上游成本 = 该组 usage_logs 对应的实际上游成本
实际毛利 = 实际收入 - 上游成本
```

但“预设上游平均倍率”和“预设毛利率”不是现有字段的直接投影。DOCX 要求使用：

- 每个上游声明倍率；
- 同一模型内归一化路由权重；
- 多模型之间的预设模型用量权重。

当前代码中可见的 `rate_multiplier`、账号倍率和 T54 调度预设，不能直接证明这种财务预设权重。必须另立字段/配置契约，或者在第一版 DOCX 页面中将这两列标记为“暂无法确认”。

## 4. 不能直接复用或容易误用的内容

### 4.1 不能把现有 `account-financial` 当作 DOCX 最终接口

现有接口：

```http
GET /api/v1/admin/operations/account-financial?range=today
```

它的当前语义是 USD、账号/分组经营聚合和历史财务辅助字段；它没有 DOCX 所需的现金充值、付费/赠送消耗、期初/期末付费余额和净沉淀完整契约。

可以复用内部 reader/repository，但建议新增 DOCX 专用的版本化 overview DTO，避免前端继续猜字段含义。

### 4.2 不能把 `actual_cost` 全部当收入

在 T55 额度来源落地前，`actual_cost` 只是总扣费；将其全部标为收入会把赠送额度消耗错误计入收入。

### 4.3 不能把 `user_unconsumed_balance_cny` 当真实现金余额

现有字段在账号财务响应中存在，但它是历史兼容/汇总投影，不能替代 T55 的 `cash_balance_cny`。DOCX 要求的“未消耗余额现金价值”必须来自钱包和付费额度，而不是简单读取用户总余额。

### 4.4 不能把 T54 调度权重当财务路由权重

T54 的权重/预设用于调度选择，不等于真实或预设的调用概率。直接用于毛利预估会产生错误的“预设上游平均倍率”。

### 4.5 不能把真实支付订单当人工充值记录

DOCX 阶段一需要简单人工录入；ZIP/T55 需要独立的充值/退款额度流水。手动充值不应伪装成 `payment_orders`，否则会污染真实支付、订单和退款统计。

## 5. 当前框架下的主要风险

| 风险 | 触发原因 | 后果 | 建议 |
|---|---|---|---|
| 收入误计 | 没有付费/赠送消费拆分 | 毛利和毛利率虚高 | DOCX 依赖 T55 消费流水，无法拆分时返回 pending |
| 历史数据误判 | 历史余额没有现金来源、没有赠送来源 | 虚构可退款现金或历史收入 | 历史数据标记不可确认，不自动回填 |
| 口径混币 | 现有接口 USD，DOCX 要求 CNY/Q | 页面金额无法对账 | 新增明确 CNY/Q DTO，保留原生 USD 接口兼容 |
| 时间边界错误 | 现有 24h 滚动窗口与 DOCX 自然日并存 | 趋势和充值统计错一天 | 统一显式 `timezone=Asia/Shanghai` 和半开区间 |
| 成本缺失被当 0 | 现有成本有 fallback/unavailable | 毛利被虚高 | 缺成本时成本可显示，但收入/毛利状态必须保留不确定性 |
| 预设倍率无事实源 | 没有路由权重/模型用量权重契约 | 预设毛利率不可解释 | 第一版显示待确认，另立配置任务 |
| 旁路余额写入 | T55 迁移期间仍有旧余额写路径 | 钱包与 `users.balance` 不一致 | T55 必须统一协调器并覆盖支付、兑换、返利、消费路径 |

## 6. 推荐的 DOCX 独立任务边界

### 第一阶段：经营总览只读查询

依赖 T55 已上线并稳定提供钱包/额度流水后实现：

- `/admin/operations/business-overview` 或等价原生管理员路由；
- CNY 经营结果；
- 现金充值、期初/期末付费余额、付费消耗和净沉淀；
- 充值/消耗日趋势；
- 按分组实际毛利；
- 赠送消耗和无法拆分状态的辅助信息；
- 自定义日期和北京时间边界。

### 第二阶段：预设倍率补齐

只有明确保存：

- 上游声明倍率；
- 模型内路由权重；
- 多模型预设用量权重；

才能实现 DOCX 的“预设上游平均倍率/预设毛利率”。否则先显示缺少口径，不做猜测。

### 明确不放入 DOCX 任务的内容

- 手动充值/退款实现本身（归 T55/ZIP）；
- 自动支付和真实支付渠道；
- 生产历史现金回填；
- 上游名称、上游排名或供应商明细；
- 新的外部控制面或第二套账务事实源。

## 7. 当前状态与下一步

- 本审计已完成源码与需求对照。
- 当前未修改业务代码。
- 当前未创建 DOCX 经营分析实现 worktree。
- T55 仍在 `codex/t55-native-quota-ledger` 独立 worktree 中实施，不能从本任务直接接管或修改。

下一步应由根总控在 T55 完成并线上验证后，创建独立的 DOCX 经营分析顶层任务；该任务先完成正式规格和用户批准，再进入实现。
