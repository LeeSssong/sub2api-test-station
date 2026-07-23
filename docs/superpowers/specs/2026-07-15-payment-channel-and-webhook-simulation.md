# L1-7 支付渠道选型与 Webhook 离线模拟规格

**状态：** 已确认实施  
**日期：** 2026-07-15  
**来源：** 主计划第 13 节、D05、D08、D09、D13

## 1. 目标

在不申请商户、不实名、不配置真实密钥、不发起支付和不收款的前提下，固定正式支付的条件式渠道选择、Sub2API 禁用态参数、订单状态和回调幂等验收方法。

## 2. 当前选择

- 当前 MVP：继续使用人工账本模拟，`payment_enabled: false`，不向用户展示支付入口。
- 中国大陆合格经营主体 + CNY 客户：首个正式渠道选择支付宝直连；微信支付直连作为第二渠道。
- 海外合格经营主体 + 国际客户：选择 Stripe，替代而不是叠加大陆首发路径。
- EasyPay：仅在官方直连和 Stripe 都不适用、且完成资金流向/冻结/退款/对账/连续性尽调后作为回退；当前不选。
- Airwallex：Sub2API v0.1.155 代码存在 Provider，但同版本 `docs/PAYMENT.md` 未提供完整配置说明；首版不选。
- 公开注册：继续邀请制；真实支付连续运行至少 7 天且没有重复入账、越权或重大对账事故后才进入 D09。

## 3. 选择理由

支付宝直连只依赖官方商户与应用配置，Sub2API 内置下单、签名验证、主动查单、退款和关单，且首轮只开一个支付方式，配置面小于同时上线支付宝和微信。微信 APIv3 所需 AppID、MchID、私钥、APIv3 Key、公钥和证书信息更多，放在第二步。Stripe 具备国际卡、多币种、Webhook 和退款能力，但只有海外主体与目标客户匹配时采用。

聚合商的接入门槛较低不代表资金和合规风险较低，因此不把“免审核”作为首选理由，也不把项目文档中的推广描述当作尽调结论。

## 4. 禁用态默认参数

在真实商户通过验收以前，配置必须保持：

| 字段 | 假定值 |
|---|---:|
| `payment_enabled` | `false` |
| `enabled_payment_types` | `[]` |
| `payment_min_amount` | CNY 20 |
| `payment_max_amount` | CNY 200 |
| `payment_daily_limit` | CNY 200/用户 |
| `payment_order_timeout_minutes` | 15 |
| `payment_max_pending_orders` | 2 |
| `payment_balance_recharge_multiplier` | `1 / 7.2 = 0.1388888888888889` USD/CNY |
| `payment_recharge_fee_rate` | `0`，模拟阶段不收费 |
| `payment_load_balance_strategy` | `round_robin`；首版实际只有一个 Provider |
| 取消限流 | 滚动 1 小时最多 3 次 |

汇率只用于离线示例，真实启用时必须与 D03 售价、账本和当时汇率同时复核。

## 5. 支付配置数据边界

`config/payments/PAY01.example.yaml` 只保存渠道选择、非敏感参数、回调路径和符号型凭据位置。禁止保存 App 私钥、APIv3 Key、商户 Key、Stripe Secret/Webhook Secret、证书私钥或真实商户 ID。

真实非敏感配置使用被 Git 忽略的 `config/payments/*.local.yaml`；真实密钥只进入 Sub2API 管理后台或受控密钥存储。

## 6. 配置校验

校验器必须拒绝：

1. 未有 `live_accepted` Provider 时打开支付或暴露支付方式。
2. 最小金额、最大金额、单用户日限额和待支付订单数关系错误。
3. CNY/USD 汇率和余额充值倍率不互为倒数。
4. 回调不是 HTTPS、路径与 Provider 不匹配或包含查询/片段。
5. Provider 声称可上线但缺少验签、主动查单、退款或对账能力。
6. EasyPay 未完成尽调却被列为推荐或启用。
7. 文件含真实密钥、私钥、疑似 Token 或支付卡资料。

## 7. 合成订单状态机

离线模拟只验证 Sub2API 外围控制要求，不替代 Provider SDK 的真实验签：

```text
PENDING -> COMPLETED -> REFUND_REQUESTED -> REFUNDED
    |           |
    +-> FAILED  +-> duplicate/out-of-order event = no-op
    +-> EXPIRED
```

Provider 事件必须包含：事件 ID、订单号、金额、币种、发生时间、原始请求体 SHA-256 和“已由 Provider 适配器验签”的边界标记。模拟器拒绝伪造标记、金额/币种/订单不一致、事件 ID 复用但请求体摘要变化；同一支付成功或退款成功无论回调多少次都只允许一次余额变更。

## 8. 验收标准

- PAY01 YAML 能表达当前禁用态、条件式渠道优先级和符号型凭据位置。
- 校验器拒绝不安全启用、金额关系错误、倍率错误、回调错误、能力缺失和秘密值。
- 合成状态机覆盖成功、重复、第二个成功事件、伪造、金额不符、乱序、退款和事件 ID 冲突。
- 采购/开通报告明确支付宝直连、微信直连、Stripe 和 EasyPay 的使用条件。
- 真实商户申请、实名、密钥、付款和收款保持未执行。

## 9. 依据与限制

- [Sub2API v0.1.155 支付配置文档](https://github.com/Wei-Shaw/sub2api/blob/41cec0db059ffb82d0efdcfcf07a24ab51fbfe97/docs/PAYMENT.md)
- [Sub2API v0.1.155 支付类型](https://github.com/Wei-Shaw/sub2api/blob/41cec0db059ffb82d0efdcfcf07a24ab51fbfe97/backend/internal/payment/types.go)

本规格不能确认任何主体能通过商户审核，也不能证明某聚合商或资金路径适合实际经营。真实启用必须重新核对服务商当时文档、结账条款、商品描述和经营主体。
