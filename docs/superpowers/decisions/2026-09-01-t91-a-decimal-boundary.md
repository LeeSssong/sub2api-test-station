# T91-A 十进制定点边界合同

**日期：** 2026-09-01

## 目的

新额度账务域必须避免把 Ent 生成的 `float64` 金额直接用于账务计算。旧支付/钱包模型继续兼容现有接口，但进入新 grant、billing、adjustment 事实前必须经过统一适配器。

## 边界

- 新账务服务内部金额类型：`shopspring/decimal.Decimal`。
- 新额度持久化列：PostgreSQL `numeric(20,8)`，服务端写入前统一量化到 8 位。
- HTTP 输入/输出：十进制字符串；禁止 JSON number 作为新账务金额契约。
- 旧 Ent `field.Float`、支付 provider `float64` 和历史 `users.balance`：仅作为 legacy 输入，先转换为十进制字符串或显式 decimal，再进入新域。
- `delta_usd` 表示实际落账变动；`attempted_quota_usd`（由 T91-C 增加）表示应扣额度，二者不可互换。

## 规范化规则

1. 空字符串、NaN、Inf、负数（在要求非负的字段）拒绝，不静默归零。
2. 解析使用 `decimal.NewFromString`；float64 适配必须使用 `decimal.NewFromFloat`，不得先格式化为二进制近似字符串。
3. 量化使用 8 位 `HALF_UP`；既有支付规则若另有舍入，以订单 `quota_rule_snapshot` 显式记录的规则优先。
4. 币种换算先在订单币种内计算，再使用订单规则快照转换为 `_usd`；无法换算时返回明确错误。
5. 持久化前后金额应以规范化十进制字符串比较；读取数据库 numeric 时不得转回 float64 参与计算。

## 推荐适配器接口

后续 T91-B～D 复用以下语义（具体包路径以实现时的模块布局为准）：

```go
type QuotaAmountAdapter interface {
    Parse(string) (decimal.Decimal, error)
    FromLegacyFloat(float64) (decimal.Decimal, error)
    Normalize(decimal.Decimal) (decimal.Decimal, error)
    Format(decimal.Decimal) string
}
```

本任务只锁定合同，不实现业务服务；任何新增账务字段必须使用该合同或提供等价的、可测试的实现。

## 禁止事项

- 不把 `pay_amount` 当作 `paid_quota_usd` 的计算基数。
- 不用 `float32/float64` 累加余额、grant、allocation、退款或扣费。
- 不把历史未知金额填成 0 后标记 `exact`。
- 不修改已应用迁移文件；新增迁移必须 additive 且可重复执行。
