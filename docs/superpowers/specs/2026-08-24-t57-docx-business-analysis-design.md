# T57 DOCX 经营分析总览规格

**状态：** 已完成设计，自审通过，根总控代审批准（2026-08-24）
**需求源：** `/Users/gongtengxinwen/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/wxid_9944479444511_c3ab/msg/file/2026-08/星桥AI-Link-经营分析开发需求.docx`
**基线：** `main@e1a0039b0`
**任务边界：** T57 只交付 DOCX 的经营总览查询和页面；T55 交付额度钱包、手动充值/退款和额度流水。两个任务独立 worktree 并行，整合、部署和线上验收单车道。

## 1. 问题证据与目标

现有 Sub 原生能力已经提供：

- `usage_logs.actual_cost`、请求时间、Token、不可变 `group_id`；
- 上游成本表达式 `COALESCE(account_cost, COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1))`；
- `/api/v1/admin/operations/*` 管理员鉴权和路由模式；
- `AccountFinancialService`、`AccountProfitabilityService`、`AccountProfitabilityView.vue` 的时间范围、刷新、错误态和聚合结构。

现有原生接口不能直接满足 DOCX，因为它固定 USD 语义，没有付费/赠送消费拆分、现金充值、期初/期末付费余额、净沉淀和充值/消耗趋势。把全部 `actual_cost` 当收入，或把 `user_unconsumed_balance_cny` 当现金余额，都会产生不可对账的经营数据。

T57 的目标是在原生管理员经营页面中交付一个可验证的“经营总览”：

1. CNY 站内收入、上游成本、毛利、毛利率；
2. 现金充值、期初/期末付费余额、付费实际消耗、净沉淀和余额核对；
3. 按北京时间自然日的充值/付费消耗/净充值趋势；
4. 按站内分组的实际收入、上游成本、毛利和毛利率；
5. 赠送额度消耗、缺失成本和无法拆分历史的明确状态。

## 2. 非目标

- 不实现 T55 的充值、退款、冲正、额度扣费协调器或流水写入；
- 不创建 `payment_orders` 的替代事实源，不把手动充值伪装成支付订单；
- 不回填或猜测历史现金、付费/赠送来源，不把未知值按 0 计算；
- 不展示上游名称、供应商明细、上游数量或排名；
- 不实现用户留存、充值用户数、充值后 7 天使用率等行为分析；
- 不把 T54 调度权重当作财务路由权重；没有明确上游声明倍率、模型内路由权重和多模型用量权重时，预设倍率列显示 `unavailable`，不倒推；
- 不新增外部控制面、平行账务库或独立管理入口。

## 3. 方案与选择

### 方案 A：复用现有 account-financial DTO

改动最少，但旧 DTO 固定 `currency=USD`，且字段无法表达现金流、余额核对、赠送拆分和 `pending_split`。修改旧字段会破坏既有 USD 页面与调用者，拒绝。

### 方案 B：原生 operations 路由上的 DOCX 专用只读总览（推荐）

新增版本化的 `/api/v1/admin/operations/business-overview`，内部复用现有 usage reader、成本表达式、管理员鉴权和页面组件；额度来源仅通过 T55 只读契约接入。接口缺少 T55 表、历史无法拆分或余额核对不完整时，返回结构化状态而不是伪造零值。该方案保持原生边界、可与 T55 并行开发，也能在 T55 未部署时安全返回待确认状态。

### 方案 C：新建独立经营/充值数据库

可一次性满足页面，但会产生第二套账务事实源、同步和回填风险，违反项目原生优先和单一流水约束，拒绝。

## 4. 数据和口径

### 4.1 经营结果

在请求区间 `[start, end)` 内：

```text
revenue_cny = paid_consumption_q * quota_to_cny_rate
upstream_cost_cny = SUM(
  COALESCE(account_cost,
    COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1))
)
gross_profit_cny = revenue_cny - upstream_cost_cny
gross_margin = gross_profit_cny / revenue_cny, revenue_cny = 0 时为 null
```

阶段一默认记录口径为 `1 Q = ¥1`，但该关系必须来自 T55 流水/批次的保存口径，不使用查询当天汇率。`gift_consumption_q` 不计收入，但对应调用成本仍计入成本；如不能按请求关联分拆，`revenue_status=pending_split`，`revenue_cny/gross_profit_cny/gross_margin` 返回 null，成本仍可返回。

内部运营消耗通过原生业务/内部标识过滤，不进入核心经营结果；若当前数据没有可靠标识，接口返回 `internal_usage_status=unknown` 并把经营结果标为 `pending`，不得静默混入。

### 4.2 T55 只读契约

T57 读取以下稳定字段，不调用 T55 写接口：

```text
user_wallets:
  user_id
  cash_balance_cny
  paid_quota_balance_usd
  gift_quota_balance_usd

user_quota_ledger_entries:
  user_id
  record_type              # recharge / usage_consumption / refund / adjustment
  cash_delta_cny
  paid_quota_delta_usd
  gift_quota_delta_usd
  paid_before_usd
  paid_after_usd
  gift_before_usd
  gift_after_usd
  reference_type
  reference_id              # usage_id/request_id 可关联时使用
  status
  created_at
```

对于 `usage_consumption`：

```text
paid_consumption_q = -paid_quota_delta_usd
gift_consumption_q = -gift_quota_delta_usd
```

请求无法通过 `reference_id` 或稳定关联键与流水对应时，不把整笔请求归为 paid 或 gift；这笔请求计入成本并进入 `pending_split_count`。

### 4.3 资金状态

```text
cash_recharge_cny = SUM(recharge.cash_delta_cny)
opening_paid_balance_cny = paid balance at start boundary
paid_quota_issued_cny = SUM(recharge.paid_quota_delta_usd * recorded_rate)
paid_consumption_cny = SUM(paid_consumption_q * recorded_rate)
closing_paid_balance_cny = paid balance at end boundary
net_settlement_cny = cash_recharge_cny - paid_consumption_cny
```

余额核对返回独立的 `balance_reconciliation`：`opening + issued + adjustments - consumption - refunds = closing`，并包含 `status=balanced|unbalanced|pending`、差额和调整项。退款/冲正字段预留，但 T57 不产生它们。

### 4.4 分组毛利

按 `usage_logs.group_id` 聚合，未归组使用 `unassigned=true` 的独立行：

```text
confirmed_revenue_cny = paid consumption associated with group
upstream_cost_cny = native usage cost expression associated with group
gross_profit_cny = confirmed_revenue_cny - upstream_cost_cny
gross_margin = gross_profit_cny / confirmed_revenue_cny, zero => null
```

字段 `configured_multiplier` 只显示当前站内倍率；`preset_upstream_multiplier` 和 `preset_margin` 只有在明确财务预设输入存在时才计算，否则为 null 并返回 `preset_status=unavailable`。实际有效倍率按实际基础用量/成本加权，不按上游数量简单平均。

## 5. API 契约

```http
GET /api/v1/admin/operations/business-overview
  ?range=today|7d|30d|month|previous_month|custom
  &start_date=YYYY-MM-DD
  &end_date=YYYY-MM-DD
  &timezone=Asia/Shanghai
  &group_id=<optional>
```

自定义日期为闭区间输入，服务端转换为指定时区的半开区间。默认时区固定为 `Asia/Shanghai`；所有趋势桶按北京时间自然日。

响应核心结构：

```json
{
  "data": {
    "generated_at": "...",
    "timezone": "Asia/Shanghai",
    "start_date": "2026-08-01",
    "end_date": "2026-08-24",
    "currency": "CNY",
    "quota_unit": "Q",
    "quota_unit_label": "内部记账额度，不是美元",
    "revenue_status": "confirmed|pending_split|pending|unavailable",
    "summary": {
      "revenue_cny": 0,
      "upstream_cost_cny": 0,
      "gross_profit_cny": null,
      "gross_margin": null,
      "paid_consumption_q": null,
      "gift_consumption_q": null,
      "gift_upstream_cost_cny": null,
      "pending_split_count": 0
    },
    "cash_and_balance": {
      "cash_recharge_cny": null,
      "opening_paid_balance_cny": null,
      "paid_quota_issued_cny": null,
      "paid_consumption_cny": null,
      "closing_paid_balance_cny": null,
      "opening_gift_balance_q": null,
      "closing_gift_balance_q": null,
      "net_settlement_cny": null,
      "balance_reconciliation": {"status":"pending","difference_cny":null,"adjustments":[]}
    },
    "trend": [{"date":"2026-08-24","cash_recharge_cny":0,"paid_consumption_cny":0,"net_settlement_cny":0}],
    "groups": [{
      "group_id": null,
      "group_name": "未归组",
      "unassigned": true,
      "model_count": 0,
      "request_count": 0,
      "configured_multiplier": null,
      "preset_upstream_multiplier": null,
      "preset_margin": null,
      "preset_status": "unavailable",
      "effective_upstream_multiplier": null,
      "revenue_cny": null,
      "upstream_cost_cny": 0,
      "gross_profit_cny": null,
      "gross_margin": null,
      "revenue_status": "pending_split"
    }]
  }
}
```

数值字段在口径未知时使用 `null`，不使用 `0`；成本字段可在收入待确认时仍返回真实聚合值。所有响应都经过管理员鉴权，不返回上游名称或凭据。

## 6. 页面行为

在原生管理员 operations 导航下增加“经营总览”，复用 `AppLayout`、刷新/加载/错误态、日期输入和图表依赖。固定顺序：

1. 经营结果：站内收入、上游成本、毛利、毛利率；
2. 充值与余额：现金充值、期初付费余额、实际付费消耗、期末付费余额；展示“资金状态·不计入毛利”和余额核对状态；
3. 充值与消耗趋势：现金充值、付费实际消耗、净充值；
4. 分组毛利表：仅站内分组、模型数、调用次数、站内倍率、预设倍率状态、实际收入、成本、毛利、毛利率和倍率偏差。

所有金额为 CNY/¥，Q 仅用于赠送/付费额度辅助信息并明确不是美元。收入待确认时用“口径待确认”状态，不展示虚假的 0.00；收入为 0 时毛利率显示 `—`。负毛利和负毛利率使用现有错误色；正常毛利用现有成功色。页面不出现上游供应商名称、排名或明细。

## 7. 失败、安全和兼容性

- T55 表或字段尚未部署：返回 HTTP 200 的结构化 `revenue_status=pending`，成本和 usage 计数仍可用，余额/充值字段为 null；页面显示待账本上线提示，不阻断旧 USD 页面。
- 付费/赠送无法拆分：`pending_split`，不把总扣费当收入；保留 `pending_split_count` 和关联失败原因的管理员级简短状态。
- 成本缺失：成本为 null 或 `cost_status=pending`，毛利和毛利率为 null；不能按零成本计算。
- 无效日期、非法时区或越界 `group_id`：400；数据库/依赖失败：500 且不泄露 SQL 或凭据。
- 旧 `/account-financial` 和 `/account-profitability` API/页面字段保持不变；T57 使用新 DTO 和新 API，不修改 USD 归一化。
- 查询只读，不写账本、余额、支付订单或生产数据；管理员鉴权沿用原生 middleware。

## 8. 测试策略与验收矩阵

后端直接相关测试：

- 日期范围：today/7d/30d/month/previous_month/custom 的北京时间半开区间；
- paid-first 请求：全付费、全赠送、一次请求跨两种额度；
- 无关联流水、旧历史和 T55 表不可用时为 `pending_split/pending`；
- 上游成本表达式、缺失成本、零收入毛利率 null；
- 充值/消耗趋势按自然日、净沉淀公式和余额核对差额；
- 分组、未归组和按 group_id 筛选；禁止上游名称出现在 DTO；
- 管理员鉴权、错误响应、旧 API 回归。

前端直接相关测试：

- 四张经营卡和 CNY 文案顺序；
- 待确认/空收入/负毛利状态；
- 日期切换同时刷新四个区块；
- 趋势图和分组表字段，不渲染上游名称；
- 加载、刷新、错误、空状态和 390px/桌面布局不溢出。

验收必须证明：真实 T55 已部署时能读到充值、钱包和消费拆分；T55 未部署或历史不可拆分时页面明确待确认且没有把数据归零；`/healthz`、`/readyz`、`/health` 通过；管理员线上页面和 API 与发布源一致。

## 9. 发布、回滚和依赖

- T57 预计无迁移、无写入；最终 `downtime_required` 以发布预检为准。
- T55 契约稳定后，T57 必须刷新候选到包含 T55 的最新 `main`，重新运行直接相关测试；旧候选不得直接合并。
- 发布顺序：T55 先完成合并、推送、部署和线上验收；之后 T57 才能进入整合车道。两者不能合并成一次大发布。
- 发布失败保留 T57 worktree、分支、测试和失败证据；回滚使用上一已验证 `main`/蓝绿槽，不删除数据或候选。
- 只有根总控可以授权合并、推送、部署和线上验收；T57 顶层任务不得自行执行这些动作。

## 10. 待决事项

没有阻塞 T57 第一版实现的产品待决事项。上游预设倍率属于明确的后续数据契约；在其稳定前按 `unavailable` 展示。T55 负责的手动充值/退款页面和账本写路径必须先按 T55 规格上线，T57 只消费其读模型。

## 11. 代审批准记录

2026-08-24 根总控根据用户“按方案实施直至部署生产生效”的授权完成规格自审并批准方案 B。批准不扩大 T57 范围，不解除 T55 依赖，不授权 T57 修改 T55 worktree、合并 `main` 或直接部署。
