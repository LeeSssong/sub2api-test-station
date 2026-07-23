# L1-7 支付控制离线模拟验证

**日期：** 2026-07-15  
**范围：** PAY01 禁用态配置、Provider 条件式选择、配置校验、订单/退款幂等状态机  
**外部状态：** 未申请商户、未实名、未配置真实密钥、未付款、未收款

## 结果

- 当前模式固定为人工账本模拟，支付总开关关闭，可见方式为空。
- 大陆合格主体首选支付宝直连，微信直连第二；海外合格主体首选 Stripe；EasyPay 延后尽调。
- PAY01 示例通过校验，激活状态为 `disabled_simulation_only`。
- 合成 CNY 72 订单得到模拟 USD 10：支付成功只加款 1 次，重复成功不再加款；退款只冲减 1 次，重复退款不再冲减。
- 未连接任何 Provider，`real_payment_sent: false`。

## TDD 证据

1. 测试首次运行因 `ops/payment-control-simulator.rb` 不存在而失败。
2. 实现配置校验器和状态机后，专项测试通过：12 tests / 54 assertions / 0 failures / 0 errors。

## 验证命令

```bash
ruby -w tests/payments/payment_control_simulator_test.rb
ruby ops/payment-control-simulator.rb validate config/payments/PAY01.example.yaml
ruby ops/payment-control-simulator.rb demo config/payments/PAY01.example.yaml
for f in tests/**/*_test.rb; do ruby -w "$f" || exit 1; done
bash tests/infra/validate-baseline.sh
ruby -c ops/payment-control-simulator.rb
```

## 全量验证

- 支付专项：12 tests / 54 assertions / 0 failures / 0 errors。
- 全量 Ruby 回归：50 tests / 194 assertions / 0 failures / 0 errors。
- 基础设施契约：通过；只解析 Compose、检查固定镜像/配置、Git 忽略和临时密钥生成，不改动现有容器。
- 所有示例 YAML 均可解析；项目 Markdown 围栏平衡。
- PAY01 受控配置和文档未发现真实秘密值模式或已填充支付凭据字段。

## 合成事件序列

```text
payment_completed
duplicate_event_noop
refund_requested
refund_completed
duplicate_event_noop
```

最终状态：`REFUNDED`；`credit_count = 1`；`reversal_count = 1`；加款与冲减均为模拟 USD 10。

## 未验证

- 经营主体、商户资格、商品审核和真实费率。
- 支付宝 RSA2、微信 APIv3、Stripe 或 EasyPay 的真实 Provider 验签。
- 真实回调可达性、主动查单、退款、拒付、冻结和对账文件。
- 真实客户付款和公开注册。

这些项目保持在 `docs/superpowers/checklists/payment-live-acceptance.md`，用户审阅最终报告并自行开通商户后再执行。
