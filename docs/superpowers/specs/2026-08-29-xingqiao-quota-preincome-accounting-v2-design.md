# 星桥 Q 额度、预收入与账务规则 v2 规格

**日期：** 2026-08-29  
**状态：** REVISION FOR USER CONFIRMATION
**任务类型：** Sub2API 原生账务模型收敛与只读经营归因

## 1. 目标

在现有 Sub2API 原生支付订单、用户 Q 余额、兑换码、返利、使用记录和经营总览基础上，统一定义：预收入、Q 额度发放、paid/gift 额度归因、实收入和管理员账务退款。

用户账户只维护一个可消费的星桥 Q 总余额。人民币不进入用户余额，不参与 API 消费校验；人民币只存在于真实支付订单、额度发放记录和管理员账务退款记录中。

## 2. 已确认决策

1. 保留“预收入”这个产品名词。
2. 系统统一记录额度单位 `Q`；页面统一显示 `$`，此 `$` 只是星桥额度 Q 的标记符号，不代表 USD，不得按美元或汇率解释。
3. `charged_quota_q` 是本次 API 总扣费，不等于实收入。
4. 系统只保留两类扣费字段：`paid_consumed_q` 和 `gift_consumed_q`；`实收入 = paid_consumed_q`。不再引入产品层 `cash_paid_consumed_q`。
5. 真实支付充值必须关联 `payment_orders`。
6. 只要产生 `paid_q` 或 `gift_q` 发放，就必须有统一的额度发放记录；非真实支付来源不创建真实支付订单。
7. 消费不维护“某次消费扣了哪一笔充值订单”的分摊关系。
8. v1 退款统一为“管理员账务退款”，不要求关联 `payment_orders`。
9. 管理员账务退款默认不自动清零赠送额度；赠送撤销必须是独立事件。
10. `usage_logs` 不可变，只允许新增只读额度归因投影。
11. 产品层不突出泛化“额度流水”，技术层仍保留追加式账务事件记录用于审计、幂等、并发和重建。

## 3. 统一口径

### 3.1 Q 与页面显示

Q 是星桥站内额度单位，不是美元，不代表汇率；页面显示 `$` 仅作为 Q 的符号。当前政策固定为：

```text
¥1 = 1Q
```

每条额度发放记录保存政策快照，避免未来政策变化影响历史解释。

### 3.2 预收入

预收入是用户已向本站支付并完成收款确认的人民币金额；选定时间内确认收款多少，就统计多少，不扣除同期 API 消耗，也不要求先转化为实收入。

预收入只来自真实支付订单的已确认收款，不包括手工赠送、兑换码、返利、历史迁移、未支付订单或支付失败订单。

### 3.3 实收入

假设本次请求：

```text
paid_consumed_q = 2Q
gift_consumed_q = 1Q
charged_quota_q = 3Q
```

则：

```text
实收入 = 2Q
```

`usage_logs.actual_cost` 仍代表 Sub 原生实际扣费总额，不能直接全部作为实收入；实收入只取其中的 `paid_consumed_q`。

### 3.4 内部额度来源

```text
paid_q              // 付费额度
gift_q              // 赠送/福利额度
total_q             // 对外可消费总额度
```

守恒关系：

```text
total_q = paid_q + gift_q
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
| 兑换码/返利 | `redeem_codes`、现有兑换与返利服务 | 返利接入统一发放事件；兑换码待分类决策后接入 |
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
paid_q
gift_q
version
updated_at
```

不得再新增或维护 `cash_backed_paid_q`、`non_cash_paid_q` 两个产品层余额池。是否把兑换码计入 `paid_q` 或 `gift_q`，在兑换码来源决策明确前保持未决，不得先行实现或进入预收入、实收入、退款计算。

### 5.2 额度发放记录 `quota_issuance_records`

每一笔付费或赠送额度发放都产生一条记录。

| 字段 | 语义 |
|---|---|
| `id` | 发放记录 ID |
| `user_id` | 用户 ID |
| `source_type` | `payment`、`manual_recharge`、`redeem_code`、`affiliate`、`admin_gift` |
| `status` | `pending`、`confirmed`、`failed`、`cancelled` |
| `amount_cny` | 仅真实收款或人工确认收款时记录人民币金额；赠送类来源为 0/NULL |
| `paid_q` / `gift_q` | 本次发放额度 |
| `payment_order_id` | 真实支付时关联 `payment_orders.id`，其他来源为空 |
| `source_reference_type/id` | 兑换码、返利、人工凭证等来源 |
| `operator_id` | 手工操作人 |
| `external_reference` | 线下凭证或人工参考号 |
| `policy_snapshot` | `¥1=1Q` 等规则快照 |
| `note` | 备注 |
| `created_at` / `confirmed_at` | 创建/确认时间 |

约束：`paid_q >= 0`、`gift_q >= 0`；真实支付必须有 `payment_order_id`；非真实支付不得填写渠道交易字段；同一来源不得重复确认。不得新增 `cash_backed_q`、`non_cash_paid_q` 等第三类额度字段。

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
gift_q = 活动赠送额度（如有）
payment_order_id = 必填
```

预收入取选定期间内真实支付订单的已确认收款金额；手工充值的人民币凭证不计入预收入。实收入在消费发生时按 `paid_consumed_q` 确认，两者是独立指标。

### 6.2 手工充值

手工充值是有人民币依据、但没有支付渠道回调的人工确认充值：

```text
source_type = manual_recharge
amount_cny = 线下确认金额
paid_q = amount_cny
gift_q = 0
payment_order_id = NULL
operator_id = 必填
external_reference = 建议必填
```

不得填写 `out_trade_no`、`payment_trade_no` 或支付渠道字段。当前手工充值入口是管理员用户列表/详情中的 `UserBalanceModal`；记录当前显示在 `UserBalanceHistoryModal` 的额度页签中。手工充值的 `amount_cny` 只作为人工财务凭证，不计入“预收入”指标。

### 6.3 管理员赠送

```text
source_type = admin_gift
amount_cny = 0
paid_q = 0
gift_q = 赠送数量
payment_order_id = NULL
operator_id = 必填
```

界面显示“管理员赠送”，不得显示为“充值 ¥0”。

### 6.4 兑换码（待决策）

当前不确定兑换码应归入 `paid_q` 还是 `gift_q`，因此本任务不得先行固化其产品分类。无论最终选择哪一类，兑换码都必须通过 `source_type=redeem_code` 和 `source_reference_id=redeem_codes.id` 记录来源，且不得伪造 `payment_orders` 或支付渠道字段；在决策完成前，不得把兑换码计入预收入、实收入或可退款池。

### 6.5 返利

返利默认归类为 `gift_q`：

```text
source_type = affiliate
paid_q = 0
gift_q = 返利额度
```

不得归类为 `paid_q`，除非后续另立规格明确真实收款依据和退款口径。

## 7. 消费与只读归因

### 7.1 消费顺序

不绑定具体充值订单，只按来源池扣减：

```text
paid_q -> gift_q
```

### 7.2 `usage_quota_attributions`

不得修改 `usage_logs`。新增只读投影，至少包含：

```text
usage_log_id
request_id
charged_quota_q
paid_consumed_q
gift_consumed_q
source_status: attributed|legacy_unattributed|unavailable
event_reference_id
```

通过用户 ID + 逻辑请求 ID 关联已确认消费事件，并校验两类消耗之和等于 `charged_quota_q`。无法关联的历史记录标记 `legacy_unattributed`，不得默认归为 `paid_consumed_q`。

经营总览同时展示：原生总扣费、`paid_consumed_q` 实收入、`gift_consumed_q`、历史未归因消耗；预收入单独统计选定期间真实支付订单的已确认收款人民币金额。上游成本和原生毛利继续沿用 `usage_logs` 既有公式。

## 8. 管理员账务退款

### 8.1 可退款额度

不按订单分摊，使用聚合池：

```text
refundable_q = paid_issued_q
             - paid_consumed_q
             - confirmed_admin_refund_q
```

固定 `¥1=1Q` 时：

```text
refundable_cny = refundable_q
```

返利、管理员赠送等 `gift_q` 不进入可退款池；兑换码在分类决策完成前不进入退款计算。

### 8.2 退款事务

1. 锁定用户 Q 状态和退款聚合数据；
2. 重新计算 `refundable_q`；
3. 校验 `0 < refund_amount_cny <= refundable_cny`；
4. 扣减等额 `paid_q`；
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
  "paid_q": "900.00000000",
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

手工充值提交 `source_type=manual_recharge` 和 `amount_cny`；管理员赠送提交 `source_type=admin_gift` 和 `gift_q`。来源、操作人和确认状态全部由服务端生成。

### 9.3 退款接口

```http
POST /api/v1/admin/users/:id/admin-refunds
Idempotency-Key: <key>
```

请求包含 `refund_amount_cny`、`reason` 和可选 `external_reference`。`payment_order_id` 可选但不参与资格判断。

### 9.4 页面

- 用户列表/详情只显示“当前可消费额度 $XXXX”；其中 `$` 是 Q 的页面标记，不是 USD；
- 管理员详情可显示付费额度 `$XXXX`、赠送额度 `$XXXX`、可退款额度 `$XXXX`；人民币金额只在预收入、支付订单和退款财务记录中显示；
- 手工充值、管理员赠送、管理员账务退款使用明确分开的操作模式；
- 真实支付、额度发放、退款、使用归因在管理员财务页面分组展示；
- 使用详情增加只读“额度来源”区块；
- 390px 无整页横向溢出。

## 10. 迁移与兼容

历史 `users.balance` 没有可核验的来源凭证，不得推导为 `paid_q`、预收入或可退款额度。T91 不执行历史 paid/gift 分类迁移；在后续受控对账任务完成前，仅保留 `users.balance` 作为兼容的总 Q 投影，不将其计入预收入、实收入或退款池。该兼容总额不新增第三类用户余额字段。

不生成伪造支付订单、预收入或退款额度。旧 `/balance` 保留兼容，但写入必须经过事件协调器；新页面不调用它。

现有 `cash_balance_cny` 先停止展示和退款计算使用，待所有旁路写入确认收敛后再由独立任务删除或迁移。

## 11. 安全、测试与发布

必须覆盖：真实支付→发放记录事务、手工充值、管理员赠送、返利、两类 Q 消费、实收入公式、预收入独立统计、历史未归因、聚合退款、并发退款、幂等冲突、事务回滚、旧 `/balance` 兼容、管理员鉴权、脱敏和 390px 页面。兑换码测试与写入须等待其 paid/gift 分类决策，不得以未决策口径实现。

最小验证：

```bash
go test ./internal/service/... ./internal/repository/... ./internal/handler/...
go build ./cmd/server
pnpm typecheck
pnpm build
git diff --check
```

本规格不授权直接改代码、迁移或生产数据。实施必须拆成独立任务包，依次完成：发放记录与支付关联 → 两类 Q 归因 → 管理员账务退款 → 页面/经营总览 → 旧现金字段弃用。涉及迁移、账务写入或停机时，遵守项目全局授权门禁。

## 12. 完成条件

完成条件是：真实支付、手工充值、返利、管理员赠送均能追溯来源；兑换码在分类决策后再纳入实现；用户只维护一个 Q 总余额；实收入只统计 `paid_consumed_q`；预收入只统计真实支付订单已确认收款；使用记录通过只读投影完成归因；管理员退款不按订单分摊且聚合校验正确；直接相关测试、合并、推送、部署和线上专项验证全部通过。
