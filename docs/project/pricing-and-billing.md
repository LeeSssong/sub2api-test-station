# API 成本、售价与 Sub2API 计费配置

**当前状态：** 生产公开分为 `GPT-Pro`（Neko，账号成本倍率 `0.10x`）和 `GPT-Plus`（Wawazz，账号成本倍率 `0.05x`）；两组用户倍率均为 `1.0x`，六个文本模型已完成同步、SSE 和受控计费验证。  
**当前禁止：** 仍保持邀请制、支付关闭；不把供应商倍率当作长期或全模型成本保证，不把未知商业再分发授权当作已获授权。

## 1. 计价口径

本项目同时保留两套口径：

- 内部成本核算：按实际 CNY 支出、服务器成本和真实上游扣费计算。
- 用户市场口径：CNY 充值额按数值相同的 USD 内部额度入账，再按模型标准基础价和用户组倍率扣费。
- Sub2API 渠道价格：`USD / Token`，与当前生产 `v0.1.161` 的 `input_price`、`output_price`、`cache_read_price` 和 `cache_write_price` 一致。

方案一的用户余额口径是：

```text
1 CNY payment = 1 unit of USD-denominated internal quota
customer debit = explicit model base price x 1.0
```

自动支付仍关闭，因此当前只记录规则，不启用支付换算配置。真实人民币支出仍按实际金额进入内部成本账本，不能把站内 1:1 展示额度当成真实美元汇率。

## 2. 售价公式

```text
每百万 Token 固定成本分摊 = 月固定成本 / 月预测 Token × 1,000,000
含风险的单项成本 = 上游单项成本 × (1 + 异常补偿率) + 固定成本分摊
最低售价 = 含风险的单项成本 / (1 - 支付费率 - 目标完全成本毛利率)
推荐售价 = 按设定步长向上取整的最低售价
```

这里的“完全成本毛利率”已经扣除：上游成本、异常补偿准备、支付手续费和分摊固定成本。支付手续费按收入比例计算，所以必须放在分母中，不能简单加到成本上。

缓存读取和缓存写入分别计算。上游没有给出某项价格时，只有预测用量为 0 才允许继续；一旦预计会产生该类 Token，试算器会拒绝生成价格。

## 3. 示例为什么是 1.54 / 5.74

`MVP.example.yaml` 使用完全虚构的数据：

- 输入成本：CNY 1.00 / 百万 Token。
- 输出成本：CNY 4.00 / 百万 Token。
- 月预测：输入 8000 万、输出 2000 万 Token。
- 月固定成本：CNY 10.00。
- 异常补偿率：5%。
- 支付费率：0%（因为 MVP 仍只模拟人工账本）。
- 目标完全成本毛利率：25%。

固定成本分摊是 CNY 0.10 / 百万 Token，向上按 CNY 0.01 取整后：

```text
输入：(1.00 × 1.05 + 0.10) / 0.75 → 1.54
输出：(4.00 × 1.05 + 0.10) / 0.75 → 5.74
```

这两个数字只证明公式，不代表任何真实模型或上游报价。

## 4. 运行方法

Markdown 决策摘要：

```bash
ruby ops/calculate-pricing.rb \
  --upstream config/upstreams/UP01.example.yaml \
  --scenario config/pricing/MVP.example.yaml \
  --format markdown
```

机器可读的 Sub2API 字段：

```bash
ruby ops/calculate-pricing.rb \
  --upstream config/upstreams/UP01.example.yaml \
  --scenario config/pricing/MVP.example.yaml \
  --format json
```

真实非敏感资料到位后：

1. 填好并校验 `config/upstreams/UP01.local.yaml`。
2. 复制示例为 `config/pricing/MVP.local.yaml`；该路径已被 Git 忽略。
3. 把月固定成本拆成服务器实付价/月份、域名实付价/12、监控和其他确定支出；未发生支出填 0，不填猜测值。
4. 填入当日采用的保守 CNY/USD 运营汇率、预估模型用量和支付费率。
5. 运行两种输出并保存不含密钥的输入版本、结果和时间，提交 D03。

## 5. Sub2API 推荐映射

试算器输出采用以下配置原则：

| 字段 | 推荐 | 原因 |
|---|---|---|
| 渠道 `billing_model_source` | `requested` | 按本站公开模型名计费，上游别名变化不改变用户价格 |
| 渠道 `restrict_models` | `true` | 只允许已定价模型，防止未知模型走默认价 |
| 渠道 `model_mapping` | `public_name → upstream_name` | 对外名称与供应商名称解耦 |
| 渠道模型价格 | 明确登记的官方/上游标准 USD/Token 基础价 | 不依赖运行时未知默认价 |
| 分组 `rate_multiplier` | `1.0` | 当前 GPT-Pro/GPT-Plus 用户售价倍率 |
| 账号 `rate_multiplier` | `0.10` / `0.05` | Neko Pro/Wawazz Plus 的生产成本统计倍率，不把它当用户售价 |
| 支付/人工充值口径 | CNY 1 对应内部额度 1 | 与首版竞品比较口径一致；自动支付仍关闭 |

`requested`、`channel_mapped` 和 `upstream` 的计费模型选择不同。首版明确使用 `requested`，并在真实请求日志中核对 `billing_model_source`、原始模型、映射模型和上游模型，防止价目匹配错位。

## 6. D03 实施条件

以下信息用于复核已确认的方案一，并决定模型是否可以从受控测试进入邀请制：

- UP01 真实输入、输出、缓存价格和币种。
- 至少三笔真实小额请求的 Token 与上游扣费核对。
- 服务器/域名真实实付或明确为 0 的复用成本。
- 首批用户的保守月用量假设。
- 人工账本阶段费率 0%，或正式支付渠道的真实费率。
- 汇率、异常补偿率和价格生效时间。

截至 2026-07-20，`GPT-Pro` 和 `GPT-Plus` 均限制为 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.5`、`gpt-5.4` 和 `gpt-5.4-mini`；渠道使用 `billing_model_source=requested` 且 `restrict_models=true`。两组同步、SSE 和同请求扣费均已通过，用户扣费符合标准价乘 `1.0x`，上游实际成本分别约为 `0.10x` 和 `0.05x`。Wawazz 曾拒绝透传的 Python User-Agent；生产账号现以账号级 `User-Agent: node` 统一出站，`Python-urllib` 同步和 `OpenAI/Python` SSE 已复验通过，两条实际成本均约为标准价 `0.0502x`。

本次生产网关冒烟前曾因应用内升级到 `v0.1.161` 触发兼容性错误：默认 `gateway.text_max_body_size` 为 32 MiB，而部署上限为 16 MiB，导致 502。已固定 `GATEWAY_TEXT_MAX_BODY_SIZE=16777216` 并仅重建 Sub2API；PostgreSQL、Redis、Caddy 未重建，健康检查恢复 200。

现行决策见 `docs/superpowers/decisions/2026-07-18-d03-mvp-plan-one-pricing-and-upstream.md`。旧的固定 CNY 价格决策已被替代且从未写入生产。生产配置完成后仍必须核对逐模型实际扣费。

## 7. 价格快照与复核

- 每次改价生成新的场景 ID 和时间，不覆盖旧结果。
- 保留对外模型名、上游模型名、四类 Token 单价、汇率、费率和目标毛利。
- 用量日志必须能关联当时的渠道、模型映射和价格口径。
- 上游涨价、汇率偏离、异常补偿超过准备金或实际毛利低于 20% 时重新计算。
- 在真实收费前逐笔核对本站扣费与上游成本；未通过时保持邀请制和模拟账本。
