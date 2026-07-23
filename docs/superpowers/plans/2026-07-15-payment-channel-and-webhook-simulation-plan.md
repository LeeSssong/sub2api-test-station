# L1-7 支付渠道选型与 Webhook 离线模拟实施计划

> **执行约束：** 当前主助手在本任务内执行；不使用子代理，不提交 Git，不申请商户、不实名、不配置真实密钥、不付款、不收款。

**Goal:** 交付可校验的 PAY01 禁用态配置、正式渠道选择和可重复运行的订单回调状态机模拟。

**Architecture:** YAML 保存非敏感选择和符号型凭据引用；Ruby 标准库校验配置并处理合成订单事件。Provider 真实验签留在 Sub2API 内置适配器和未来沙箱/生产验收，本地状态机专注金额、币种、事件幂等、状态顺序和余额变更次数。

**Tech Stack:** Ruby 标准库、YAML、JSON、Minitest、Markdown。

## Global Constraints

- `payment_enabled` 保持 `false`，真实支付方式列表为空。
- 只允许符号型凭据引用，不保存商户或支付秘密。
- 支付金额 CNY 20-200，单用户每日 CNY 200，待支付订单最多 2。
- 余额倍率使用离线示例 `1 / 7.2`，真实启用前同步复核。
- 所有外部开通和资金动作根据 D13 延后。

## 文件职责

- `config/payments/PAY01.example.yaml`：当前禁用态和条件式 Provider 选择。
- `ops/payment-control-simulator.rb`：配置校验、合成订单事件状态机和 CLI。
- `tests/payments/payment_control_simulator_test.rb`：配置和状态机红绿测试。
- `docs/project/payment-channel-selection.md`：渠道推荐、参数、商户前置条件和开通顺序。
- `docs/superpowers/checklists/payment-live-acceptance.md`：真实商户、沙箱、Webhook、退款和对账验收。
- `docs/superpowers/reports/2026-07-15-payment-control-simulation-verification.md`：验证证据和未验证项。

## 任务

- [x] 1. 写 PAY01 完整配置、禁用态和秘密扫描的失败测试。
- [x] 2. 写订单成功、重复、伪造、金额错误、乱序和退款的失败测试。
- [x] 3. 运行专项测试并确认实现缺失导致红灯。
- [x] 4. 实现最小配置校验器和状态机，使测试通过。
- [x] 5. 创建 PAY01 虚构 YAML 并运行 CLI 验证。
- [x] 6. 编写渠道选择、真实开通和验收文档。
- [x] 7. 更新采购汇总、资产台账、主计划和项目状态。
- [x] 8. 运行专项、全量回归、基础设施契约和秘密值扫描。

## 验证命令

- `ruby -w tests/payments/payment_control_simulator_test.rb`
- `ruby ops/payment-control-simulator.rb validate config/payments/PAY01.example.yaml`
- `ruby ops/payment-control-simulator.rb demo config/payments/PAY01.example.yaml`
- `for f in tests/**/*_test.rb; do ruby -w "$f" || exit 1; done`
- `bash tests/infra/validate-baseline.sh`

## Acceptance

- [x] 规格第 8 节全部满足。
- [x] 合成支付和退款各只产生一次余额变更。
- [x] 所有测试、结构和秘密值检查通过。
- [x] 支付入口、商户和真实资金均未启用。

## 风险

- 合成验签边界不能替代支付宝、微信、Stripe 或 EasyPay 的真实 SDK/沙箱验签。
- 经营主体未知，因此正式渠道是按主体条件选择，不是已获商户资格。
- 汇率、费率、结算和冻结规则会变化，启用前必须重新核对。
