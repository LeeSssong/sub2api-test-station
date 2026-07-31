# 全站总账与极简采购台账设计

日期：2026-07-31  
状态：已确认设计，待实现计划

## 1. 目标

为当前 Sub2API 中转站建立一套全站总账，回答两个问题：

1. 每日有多少真实客户营收；
2. 每日资源消耗、账号采购和上游充值后，经营结果与现金结果分别是多少。

设计目标：

- 尽可能复用 Sub2API 原生用量、账号和支付数据；
- 不区分分组，只看全站；
- 只在账号导入或发生采购/充值时人工登记；
- 自有账号采购不按有效期或天数摊销；
- 管理员/内部请求不算营收，但其账号资源消耗算成本；
- 正式启用前清理旧账务数据，从新的启用日重新累计。

## 2. 当前运行基线

只读盘点得到：

- 2026-07-11 至 2026-07-26，`usage_logs` 共 35,281 条；
- 同期 `actual_cost` 合计约 4,466.9162；
- 日志全部来自管理员用户；
- `payment_orders` 当前为 0 条；
- `billing_usage_entries` 当前为 0 条；
- 现有数据应作为运行基线，不得直接当作正式客户营收。

因此，正式账本启用后：

- 营收从 0 开始；
- 启用日后的管理员请求继续产生成本；
- 启用日前的用量、采购和日报不进入新账本；
- 账号、分组、路由和模型价格等运行配置保留。

## 3. 业务口径

### 3.1 单币种规则

站内商业规则固定为：

```text
用户充值 1 CNY -> 获得 1 USD 站内额度
消耗 1 USD 站内额度 -> 记 1 CNY 营收
```

因此不使用外汇换算。`usage_logs.actual_cost` 的数值直接按 1:1 映射为 CNY 营收。

### 3.2 外部客户营收

对自然日 `D`：

```text
external_revenue_cny(D)
  = SUM(usage_logs.actual_cost)
    WHERE created_at ∈ D
      AND usage is billable
      AND user is not admin/internal/test
```

具体规则：

- 时区使用 `Asia/Shanghai`；
- 以 `actual_cost` 作为用户扣费口径；
- 不以 `total_cost` 作为营收；
- 充值到账金额单独记录为现金收入，不重复计入营收；
- 管理员、内部测试用户和内部 API Key 默认排除；
- 内部流量识别规则应使用稳定的用户/API Key 范围配置，而不是依赖名称模糊匹配。

### 3.3 全站资源消耗成本

对自然日 `D`：

```text
resource_cost_cny(D)
  = SUM(account cost for all usage logs in D)
```

该成本包含：

- 外部客户请求；
- 管理员/内部测试请求；
- `owned_oauth` 自有账号；
- `upstream_apikey` 上游 API Key。

原生账号成本优先使用：

```text
COALESCE(account_stats_cost, total_cost)
  × COALESCE(account_rate_multiplier, 1)
```

若未来上游可提供真实账单或配额成本，优先写入/映射到 `account_stats_cost`；否则明确标注为原生标准价估算，不冒充现金付款。

管理员流量规则：

```text
管理员营收 = 0
管理员资源成本 > 0（按实际账号消耗统计）
```

### 3.4 经营毛利

```text
operating_gross_profit_cny(D)
  = external_revenue_cny(D)
  - resource_cost_cny(D)
```

该指标用于衡量流量消耗本身是否有毛利，管理员测试会降低该指标。

### 3.5 现金采购支出

账号采购和上游充值不做日摊销，只在真实付款日入账：

```text
cash_outflow_cny(D)
  = SUM(paid events on D)
```

事件类型：

- `account_purchase`
- `upstream_topup`
- `refund`
- `fee`

退款应以负向现金事件或明确的反向事件记录，不能修改历史事件。

### 3.6 现金净收益

```text
cash_net_result_cny(D)
  = external_revenue_cny(D)
  - cash_outflow_cny(D)
```

采购或充值发生的当天出现负数是允许且正确的现金流结果，不应被自动平滑或摊销。

## 4. 极简采购台账

正式账本只保留以下字段：

| 字段 | 说明 |
|---|---|
| `event_type` | `account_purchase` / `upstream_topup` / `refund` / `fee` |
| `paid_at` | 实际付款时间，使用 Asia/Shanghai |
| `amount_cny` | CNY 金额，退款使用负数或反向事件 |
| `source_kind` | `owned_oauth` / `upstream_apikey` |
| `account_id` | Sub2API 账号 ID，可为空 |
| `notes` | 可选备注，不得写入凭据 |

明确不保留以下字段：

- `batch_id`
- `supplier`
- `scope`
- `external_ref`
- 任何 API Key、Cookie、Token、密码或 OAuth 凭据

默认来源映射：

- `accounts.type = oauth` 默认 `owned_oauth`；
- `accounts.type = apikey` 默认 `upstream_apikey`；
- 导入时仅在默认判断不正确时修正 `source_kind`；
- 账号采购事件填写 `account_id`；
- 无法关联具体账号的上游充值允许 `account_id = NULL`，进入全站共享现金支出。

## 5. 原生数据来源

优先复用以下 Sub2API 原生表和字段：

- `usage_logs`
  - `created_at`
  - `user_id`
  - `account_id`
  - `group_id`（保留作为原始事实，但账本不按它分组）
  - `actual_cost`
  - `total_cost`
  - `account_stats_cost`
  - `account_rate_multiplier`
- `accounts`
  - `id`
  - `type`
  - `name`
  - `notes`
  - `status`
- `users`
  - `id`
  - `role`
  - 内部用户/测试范围配置
- `payment_orders`
  - 用于未来现金收入对账，不与消耗营收重复
- `usage_dashboard_daily` / `usage_dashboard_hourly`
  - 用于较长窗口的读取优化；结算日以可重算的原始用量为事实来源

采购事件不是 Sub2API 原生字段，应以独立的追加式台账保存，并通过 `account_id` 与原生账号关联。

## 6. 日报输出

每天 Asia/Shanghai 00:10 结算前一天，并保留最近 3 天自动重算窗口。

全站日报至少包含：

| 指标 | 说明 |
|---|---|
| 外部客户营收 | 非内部可计费 `actual_cost` |
| 外部请求数 | 外部用户请求数量 |
| 管理员请求数 | 内部请求数量 |
| 管理员资源成本 | 内部请求账号成本 |
| 客户资源成本 | 外部请求账号成本 |
| 全站资源成本 | 上述两者合计 |
| 经营毛利 | 外部营收减全站资源成本 |
| 当日采购/充值支出 | 台账付款事件 |
| 现金净收益 | 外部营收减当日现金支出 |
| 现金支出事件数 | 采购、充值、退款、手续费数量 |
| 未关联采购支出 | `account_id` 为空的现金事件 |
| 数据状态 | 完整、延迟重算、存在异常 |

同时按 `source_kind` 展示两行来源拆分：

- `owned_oauth`
- `upstream_apikey`

不输出分组利润，不做分组账本。

## 7. 数据一致性与纠错

- 账务事件追加写入，不直接修改历史事件；
- 每个结算日使用日期幂等键，重复运行只更新同一日快照；
- 最近 3 天自动重算，处理延迟写入和补录；
- 账号删除或停用不删除采购历史；
- 账号重新导入时视为新采购事件；
- `account_id` 无效时保留事件并在日报列出异常，不静默丢弃；
- 采购台账不得包含任何凭据；
- 正式启用前执行备份、核验、清理和启用后首日验收。

## 8. 启用流程

1. 确定 `ledger_start_date`；
2. 备份数据库和现有运维数据；
3. 清理旧用量聚合、旧采购台账和旧日报；
4. 保留运行配置、账号、分组、路由和模型价格；
5. 初始化空账本；
6. 新导入账号时登记采购事件；
7. 新上游充值时登记 `upstream_topup`；
8. 连续运行 3 天，核对原生用量、日报和台账；
9. 验收通过后作为正式财务口径。

## 9. 验收标准

- 管理员请求不产生营收，但会增加资源成本；
- 外部请求营收等于 `actual_cost` 之和，按 1 CNY = 1 USD 额度规则映射；
- 采购只在付款日进入现金支出，不发生摊销；
- OAuth 与 API Key 来源可在全站日报中分别汇总；
- 无 `account_id` 的支出不会丢失，会显示为共享支出；
- 重跑同一天不会重复计账；
- 正式启用前历史数据不会混入新账本；
- 日报不输出任何账号凭据或敏感字段。
