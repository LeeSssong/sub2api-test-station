# 星桥 Q 额度、预收入与账务规则 v2 规格

**日期：** 2026-08-29  
**状态：** DRAFT FOR USER REVIEW  
**任务类型：** Sub2API 原生账务模型收敛与只读经营归因

## 1. 目标

在现有 Sub2API 原生支付订单、用户 Q 余额、兑换码、返利、使用记录和经营总览基础上，统一定义：预收入、Q 额度发放、付费/非现金/赠送额度归因、实收入和管理员账务退款。

用户账户只维护一个可消费的星桥 Q 总余额。人民币不进入用户余额，不参与 API 消费校验；人民币只存在于真实支付订单、额度发放记录和管理员账务退款记录中。

## 2. 已确认决策

1. 保留“预收入”这个产品名词。
2. 页面统一显示额度单位 `Q`，不显示 `$` 或 USD。
3. `charged_quota_q` 是本次 API 总扣费，不等于实收入。
4. `实收入 = cash_paid_consumed_q`，只计算有现金付费依据的消耗。
5. 真实支付充值必须关联 `payment_orders`。
6. 只要产生 `paid_q` 或 `gift_q` 发放，就必须有统一的额度发放记录；非真实支付来源不创建真实支付订单。
7. 消费不维护“某次消费扣了哪一笔充值订单”的分摊关系。
8. v1 退款统一为“管理员账务退款”，不要求关联 `payment_orders`。
9. 管理员账务退款默认不自动清零赠送额度；赠送撤销必须是独立事件。
10. `usage_logs` 不可变，只允许新增只读额度归因投影。
11. 产品层不突出泛化“额度流水”，技术层仍保留追加式账务事件记录用于审计、幂等、并发和重建。

## 3. 统一口径

### 3.1 Q

Q 是星桥站内额度单位，不是美元，不代表汇率。当前政策固定为：

```text
¥1 = 1Q
```

每条有现金依据的发放记录保存政策快照，避免未来政策变化影响历史解释。

### 3.2 预收入

预收入是用户已向本站支付并完成收款确认、但尚未因 API 消耗转化为实收入的人民币金额。

预收入只来自真实支付订单的已确认收款，不包括手工赠送、兑换码、返利、历史迁移、未支付订单或支付失败订单。

### 3.3 实收入

假设本次请求：

```text
cash_paid_consumed_q = 2Q
non_cash_paid_consumed_q = 0Q
gift_consumed_q = 1Q
charged_quota_q = 3Q
```

则：

```text
实收入 = 2Q
```

`usage_logs.actual_cost` 仍代表 Sub 原生实际扣费总额，不能直接全部作为实收入。

### 3.4 内部额度来源

```text
cash_backed_paid_q  // 有现金依据，可形成预收入并进入可退款池
non_cash_paid_q     // 无现金依据但归类为 paid，例如兑换码
gift_q              // 赠送额度
total_q             // 对外可消费总额度
```

守恒关系：

```text
total_q = cash_backed_paid_q + non_cash_paid_q + gift_q
users.balance = total_q
```

用户界面只展示 `total_q`；来源拆分仅在管理员详情、经营分析和使用归因中展示。

## 4. 原生能力复用

| 能力 | 当前事实源 | 本规格处理 |
|---|---|---|
| 用户总 Q | `users.balance` | 保留为兼容投影和消费入口 |
| 付费/赠送状态 | `user_wallets` | 作为内部投影，不作为第二用户余额 |
| 真实支付 | `payment_orders`、支付履约、`payment_audit_logs` | 只承载真实支付生命周期 |
| API 使用 | `usage_logs`、`actual_cost`、`user_id`、`account_id` | 不改写，增加只读归因 |
| 兑换码/返利 | `redeem_codes`、现有兑换与返利服务 | 接入统一发放事件 |
| 经营总览 | `/admin/operations/business-overview` | 继续复用原生成本和总扣费，补实收入归因 |

当前已部署的 `cash_balance_cny` 第一阶段不删除；先停止页面展示和退款计算使用，后续由独立迁移任务处理废弃。

## 5. 数据模型

### 5.1 用户 Q 状态

对外只有一个余额：

```text
users.balance = total_q
```

内部状态需要能区分：

```text
cash_backed_paid_q
non_cash_paid_q
gift_q
version
updated_at
```

若短期不能增加 `non_cash_paid_q` 列，则必须在事件记录中保存 `cash_backed` 属性，并由读模型重建三类余额。不得把兑换码等无现金 Q 误算为可退款额度。

### 5.2 额度发放记录 `quota_issuance_records`

每一笔付费或赠送额度发放都产生一条记录。

| 字段 | 语义 |
|---|---|
| `id` | 发放记录 ID |
| `user_id` | 用户 ID |
| `source_type` | `payment`、`manual_recharge`、`redeem_code`、`affiliate`、`admin_gift` |
| `status` | `pending`、`confirmed`、`failed`、`cancelled` |
| `amount_cny` | 有现金依据时的金额；非现金来源为 0/NULL |
| `paid_q` / `gift_q` | 本次发放额度 |
| `cash_backed_q` | 有现金依据的 paid Q |
| `payment_order_id` | 真实支付时关联 `payment_orders.id`，其他来源为空 |
| `source_reference_type/id` | 兑换码、返利、人工凭证等来源 |
| `operator_id` | 手工操作人 |
| `external_reference` | 线下凭证或人工参考号 |
| `policy_snapshot` | `¥1=1Q` 等规则快照 |
| `note` | 备注 |
| `created_at` / `confirmed_at` | 创建/确认时间 |

约束：`paid_q >= 0`、`gift_q >= 0`、`0 <= cash_backed_q <= paid_q`；真实支付必须有 `payment_order_id`；非真实支付不得填写渠道交易字段；同一来源不得重复确认。

### 5.3 `payment_orders`

只表示真实支付订单，兼容现有生命周期：

```text
PENDING -> PAID -> RECHARGING -> COMPLETED
                         └-> FAILED
COMPLETED -> REFUND_REQUESTED -> REFUNDING -> REFUNDED / REFUND_FAILED
```

`out_trade_no`、`payment_trade_no`、`payment_type`、provider 快照、`pay_amount`、`paid_at` 等字段只在 `payment_orders` 中维护。支付确认和 Q 发放必须在同一事务内完成。

### 5.4 管理员账务退款 `admin_refund_records`

v1 只实现一种产品退款：管理员账务退款。

字段：

```text
id
user_id
refund_amount_cny
refund_q
status: pending|confirmed|rejected|cancelled
reason
operator_id
external_reference
idempotency_key
payment_order_id nullable（仅说明用途，不参与上限计算）
created_at
confirmed_at
```

### 5.5 内部事件记录

继续保留现有 `user_quota_ledger_entries`，但重新定义为内部追加式账务事件记录，不作为用户余额事实源，也不必完整暴露为“额度流水”页面。

事件至少覆盖：发放、消费、退款、迁移、兼容调账，并记录来源、操作人、幂等键、备注、前后 Q 快照和确认状态。

## 6. 来源规则

### 6.1 真实支付

```text
source_type = payment
amount_cny = 实际收款金额
paid_q = amount_cny
cash_backed_q = paid_q
gift_q = 活动赠送额度（如有）
payment_order_id = 必填
```

预收入取已确认真实收款金额；实收入在消费发生时按 `cash_paid_consumed_q` 确认。

### 6.2 手工充值

手工充值是有人民币依据、但没有支付渠道回调的人工确认充值：

```text
source_type = manual_recharge
amount_cny = 线下确认金额
paid_q = amount_cny
cash_backed_q = paid_q
gift_q = 0
payment_order_id = NULL
operator_id = 必填
external_reference = 建议必填
```

不得填写 `out_trade_no`、`payment_trade_no` 或支付渠道字段。当前手工充值入口是管理员用户列表/详情中的 `UserBalanceModal`；记录当前显示在 `UserBalanceHistoryModal` 的额度页签中。

### 6.3 管理员赠送

```text
source_type = admin_gift
amount_cny = 0
paid_q = 0
gift_q = 赠送数量
cash_backed_q = 0
payment_order_id = NULL
operator_id = 必填
```

界面显示“管理员赠送”，不得显示为“充值 ¥0”。

### 6.4 兑换码

按当前决策，兑换码归类为无现金依据的 paid Q：

```text
source_type = redeem_code
paid_q = 兑换码额度
cash_backed_q = 0
gift_q = 0
source_reference_id = redeem_codes.id
```

可消费，但不进入预收入或可退款池。

### 6.5 返利

返利默认归类为 `gift_q`：

```text
source_type = affiliate
paid_q = 0
gift_q = 返利额度
cash_backed_q = 0
```

如业务以后强制归类为 paid Q，仍必须保持 `cash_backed_q=0`，不得计入实收入或可退款池。

## 7. 消费与只读归因

### 7.1 消费顺序

不绑定具体充值订单，只按来源池扣减：

```text
cash_backed_paid_q -> non_cash_paid_q -> gift_q
```

### 7.2 `usage_quota_attributions`

不得修改 `usage_logs`。新增只读投影，至少包含：

```text
usage_log_id
request_id
charged_quota_q
cash_paid_consumed_q
non_cash_paid_consumed_q
gift_consumed_q
source_status: attributed|legacy_unattributed|unavailable
event_reference_id
```

通过用户 ID + 逻辑请求 ID 关联已确认消费事件，并校验三类消耗之和等于 `charged_quota_q`。无法关联的历史记录标记 `legacy_unattributed`，不得默认归为现金付费消耗。

经营总览同时展示：原生总扣费、现金付费实收入、赠送消耗、历史未归因消耗。上游成本和原生毛利继续沿用 `usage_logs` 既有公式。

## 8. 管理员账务退款

### 8.1 可退款额度

不按订单分摊，使用聚合池：

```text
refundable_q = cash_backed_issued_q
             - cash_paid_consumed_q
             - confirmed_admin_refund_q
```

固定 `¥1=1Q` 时：

```text
refundable_cny = refundable_q
```

兑换码、返利、管理员赠送等无现金 Q 不进入可退款池。

### 8.2 退款事务

1. 锁定用户 Q 状态和退款聚合数据；
2. 重新计算 `refundable_q`；
3. 校验 `0 < refund_amount_cny <= refundable_cny`；
4. 扣减等额 `cash_backed_paid_q`；
5. 创建 `admin_refund_records` 和内部退款事件；
6. 更新 `users.balance`；
7. 提交后失效余额/鉴权缓存。

不要求 `payment_order_id`，不需要知道哪笔充值订单被哪次消费使用。默认不扣减赠送额度；赠送撤销使用独立 `gift_reversal` 事件。

### 8.3 并发和幂等

相同幂等键同请求重放第一次结果；同 key 不同请求返回冲突；并发退款最多一个消耗同一可退款边界；退款计算、Q 扣减、退款记录和事件记录必须同事务完成。

## 9. API 与页面

### 9.1 Q 摘要

保留 `/admin/users/:id/quota-summary`，新语义建议为：

```json
{
  "user_id": 37,
  "total_quota_q": "1000.00000000",
  "cash_backed_paid_q": "900.00000000",
  "non_cash_paid_q": "50.00000000",
  "gift_q": "50.00000000",
  "refundable_q": "800.00000000",
  "refundable_cny": "800.00000000"
}
```

旧 `*_usd` 字段可在兼容期保留，但新页面不得按 USD 展示。

### 9.2 发放接口

```http
POST /api/v1/admin/users/:id/quota-issuances
Idempotency-Key: <key>
```

手工充值提交 `source_type=manual_recharge` 和 `amount_cny`；管理员赠送提交 `source_type=admin_gift` 和 `gift_q`。来源、操作人、确认状态、cash-backed 属性全部由服务端生成。

### 9.3 退款接口

```http
POST /api/v1/admin/users/:id/admin-refunds
Idempotency-Key: <key>
```

请求包含 `refund_amount_cny`、`reason` 和可选 `external_reference`。`payment_order_id` 可选但不参与资格判断。

### 9.4 页面

- 用户列表/详情只显示“当前可消费额度 XXXX Q”；
- 管理员详情可显示现金付费 Q、非现金付费 Q、赠送 Q、可退款额度 ¥；
- 手工充值、管理员赠送、管理员账务退款使用明确分开的操作模式；
- 真实支付、额度发放、退款、使用归因在管理员财务页面分组展示；
- 使用详情增加只读“额度来源”区块；
- 390px 无整页横向溢出。

## 10. 迁移与兼容

历史 `users.balance` 不得推导为可退款现金：

```text
cash_backed_paid_q = 0
non_cash_paid_q = 迁移前 users.balance
gift_q = 0
```

不生成伪造支付订单、预收入或退款额度。旧 `/balance` 保留兼容，但写入必须经过事件协调器；新页面不调用它。

现有 `cash_balance_cny` 先停止展示和退款计算使用，待所有旁路写入确认收敛后再由独立任务删除或迁移。

## 11. 安全、测试与发布

必须覆盖：真实支付→发放记录事务、手工充值、管理员赠送、兑换码、返利、三类 Q 消费、实收入公式、历史未归因、聚合退款、并发退款、幂等冲突、事务回滚、旧 `/balance` 兼容、管理员鉴权、脱敏和 390px 页面。

最小验证：

```bash
go test ./internal/service/... ./internal/repository/... ./internal/handler/...
go build ./cmd/server
pnpm typecheck
pnpm build
git diff --check
```

本规格不授权直接改代码、迁移或生产数据。实施必须拆成独立任务包，依次完成：发放记录与支付关联 → 三类 Q 归因 → 管理员账务退款 → 页面/经营总览 → 旧现金字段弃用。涉及迁移、账务写入或停机时，遵守项目全局授权门禁。

## 12. 完成条件

完成条件是：真实支付、手工充值、兑换码、返利、管理员赠送均能追溯来源；用户只维护一个 Q 总余额；实收入只统计现金付费消耗；使用记录通过只读投影完成归因；管理员退款不按订单分摊且聚合校验正确；直接相关测试、合并、推送、部署和线上专项验证全部通过。
