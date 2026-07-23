# 支付渠道选择与禁用态配置

**更新日期：** 2026-07-15  
**资产状态：** 条件式推荐已定；经营主体未确认，商户未申请，支付未启用，未收款  
**适用节点：** PAY01 / L1-7

## 当前选择

当前继续使用 `manual_ledger_simulation`，Sub2API 的支付开关保持关闭，可见支付方式为空。真实商户未验收以前，不展示充值入口，也不接收客户款项。

正式渠道按经营主体二选一：

| 条件 | 首选 | 第二步 | 当前状态 |
|---|---|---|---|
| 中国大陆合格经营主体，主要收 CNY | 支付宝直连 | 微信支付直连 | 条件式推荐，未申请 |
| 海外合格经营主体，主要收国际付款 | Stripe | 按目标国家再评估其他官方渠道 | 条件式推荐，未申请 |
| 上述官方渠道均不适用 | EasyPay 协议聚合商 | 逐家尽调后再选 | 延后，不推荐当前启用 |

支付宝直连作为大陆首发，是因为首轮只开一个渠道时所需配置少于微信 APIv3，且 Sub2API v0.1.155 已内置下单、验签、主动查单、退款和关单。微信直连在支付宝真实闭环稳定后增加。Stripe 只在海外主体路径采用。

## 当前 Sub2API 参数

| 设置 | 假定值 |
|---|---:|
| 支付开关 | 关闭 |
| 可见方式 | 空列表 |
| 单笔最低 | CNY 20 |
| 单笔最高 | CNY 200 |
| 单用户每日充值上限 | CNY 200 |
| 订单超时 | 15 分钟 |
| 单用户待支付订单上限 | 2 |
| CNY/USD 示例汇率 | 7.2 |
| 余额倍率 | 0.1388888888888889 USD/CNY |
| 模拟充值费率 | 0 |
| Provider 负载策略 | `round_robin`；首版只开一个实例 |
| 取消限流 | 滚动 1 小时最多 3 次 |
| 注册 | 邀请制 |

这些值已写入 `config/payments/PAY01.example.yaml`。汇率和费率是离线示例，真实启用时必须与 D03、账本和 Provider 实际费率同步。

## Provider 配置边界

Sub2API v0.1.155 的回调路径：

| Provider | 回调路径 | 真实启用前所需材料 |
|---|---|---|
| 支付宝直连 | `/api/v1/payment/webhook/alipay` | AppID、应用私钥、支付宝公钥 |
| 微信支付直连 | `/api/v1/payment/webhook/wxpay` | AppID、MchID、商户私钥、APIv3 Key、微信支付公钥/ID、证书序列号 |
| Stripe | `/api/v1/payment/webhook/stripe` | Secret Key、Publishable Key、Webhook Secret |
| EasyPay | `/api/v1/payment/webhook/easypay` | PID、PKey、API Base URL 和可选渠道 ID |

上表只列材料名称。真实值不得写入 YAML、Git、文档或聊天，只能进入 Sub2API 管理后台或受控密钥存储。

## EasyPay 回退门槛

任何具体 EasyPay 服务商在以下五项全部核实前保持 `deferred_due_diligence`：

- 资金最终进入谁的账户，服务商是否经手或沉淀资金。
- 结算币种、周期、手续费、最低提现和汇兑成本。
- 冻结、拒付、退款、争议和账户关闭后的资金处理。
- 主动查单、退款和可下载对账文件是否真实可用。
- 服务中断、域名变化、客服和数据导出的连续性方案。

“个人可开”“免审核”“低费率”或项目推广链接不能代替这些检查。

## 上线顺序

1. 维持人工账本模拟和邀请制。
2. 最终报告审阅时确认经营主体、目标客户地区和收款币种。
3. 只申请符合主体路径的首选 Provider，不同时开多个渠道。
4. 先完成 Provider 沙箱或最小额真实自测、回调、查单、退款和日对账。
5. 所有验收通过后，将该 Provider 标记为 `live_accepted`，再打开一个可见支付方式。
6. 连续运行 7 天无重复入账、越权或重大对账事故后，再决定第二渠道和公开注册。

## 用户付款前必须公开

- 模型、价格/倍率、余额币种、有效期和价格生效时间。
- 最低/最高充值、到账规则、退款条件和不可退款情形。
- Key 泄漏、上游故障、订阅账号池失效和服务中断的处理规则。
- 用户数据、日志保留、客服和争议处理入口。

首版文案必须与商户申请中的商品描述一致，不能用含糊商品名规避审核。

## 离线验证

```bash
ruby ops/payment-control-simulator.rb validate config/payments/PAY01.example.yaml
ruby ops/payment-control-simulator.rb demo config/payments/PAY01.example.yaml
```

演示只处理合成事件，不连接支付宝、微信、Stripe 或 EasyPay。真实验收见 `docs/superpowers/checklists/payment-live-acceptance.md`。

## 公开依据

- [Sub2API v0.1.155 支付配置](https://github.com/Wei-Shaw/sub2api/blob/41cec0db059ffb82d0efdcfcf07a24ab51fbfe97/docs/PAYMENT.md)
- [Sub2API v0.1.155 支付状态和类型](https://github.com/Wei-Shaw/sub2api/blob/41cec0db059ffb82d0efdcfcf07a24ab51fbfe97/backend/internal/payment/types.go)
