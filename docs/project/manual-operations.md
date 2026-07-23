# 人工充值账本与每日对账

**当前状态：** 仅完成本地模拟；未收款、未调整用户余额、未调用 Admin API。  
**适用阶段：** 自动支付之前的 3–10 名邀请内测用户。

## 1. 三个事实必须分开

1. `payment_received`：外部资金已经确认到账，不等于站内余额已增加。
2. `balance_adjustment`：Sub2API 已执行或模拟执行余额 `add/subtract`，不等于外部收款。
3. `usage_snapshot`：站内用户消耗和上游成本的每日快照，不等于现金利润。

账本使用 JSONL，每行一个事件，后一个事件保存前一个事件的 SHA-256。修改或删除旧行会破坏哈希链。哈希只能帮助发现改动，不能证明银行流水真实，也不能替代备份。

## 2. 当前可执行的模拟命令

验证虚构账本：

```bash
ruby ops/manual-ledger.rb verify \
  --ledger config/ledger/manual-ledger.example.jsonl
```

生成对账摘要：

```bash
ruby ops/manual-ledger.rb summary \
  --ledger config/ledger/manual-ledger.example.jsonl \
  --format markdown
```

生成不发送的 Sub2API 请求预览：

```bash
ruby ops/manual-ledger.rb request-preview \
  --event config/ledger/balance-adjustment.example.json
```

预览只包含请求方法、路径、`Idempotency-Key`、`Content-Type` 和请求体；不会包含 Admin API Key，也不会发 HTTP 请求。

## 3. 未来真实账本初始化（当前不执行）

用户审阅完整报告、确认 D05/D12 并允许真实运营后：

```bash
ruby ops/manual-ledger.rb init \
  --ledger config/ledger/manual-ledger.local.jsonl \
  --ledger-id MVP-LEDGER-REAL \
  --mode real \
  --operator-ref operator-01
```

`init` 使用 0600 创建文件并拒绝覆盖。真实账本路径已被 Git 忽略；每天加密备份到主机外位置，并限制只有实际操作人可读。

## 4. 每笔人工充值顺序（延后执行）

1. 建立唯一订单号，例如 `ORD-20260715-0001`，只向已核对的站内用户收款，不赊账。
2. 在支付渠道确认“已结算/可用”，不能只凭客户截图；账本不保存卡号、银行账号或付款人敏感信息。
3. 追加 `payment_received`，记录 CNY 金额、当时冻结的 `usd_per_cny`、预期 USD 余额和非敏感外部参考。
4. 准备尚未追加的 `balance_adjustment` 草稿，操作固定使用正数 `add`，禁止日常充值使用 `set`。
5. 使用 `request-preview --event` 检查路径、用户 ID、金额和幂等键。由用户或第二复核人再次核对订单、站内用户、CNY 金额、汇率和 USD 入账额。
6. 用户在受控终端执行一次真实 Admin API 请求；AI 不代为持有或输出 Admin API Key。相同业务重试必须遵守 Sub2API 幂等语义。
7. 结果明确后，将草稿状态改成 `succeeded` 或 `failed`，填入非敏感响应参考，再用 `append` 追加。不要先把 `succeeded` 写入账本。
8. 重新查询用户余额和 `/balance-history`，确认调整记录、金额、备注和用户一致。

追加命令只接受 `*.local.jsonl`：

```bash
ruby ops/manual-ledger.rb append \
  --ledger config/ledger/manual-ledger.local.jsonl \
  --event config/ledger/balance-adjustment.local.json
```

如果请求结果不确定，先使用同一个幂等键查询/重试并确认结果，不得用新键盲目再次入账。明确失败且需要重新发起时，保留失败事件并使用新的事件 ID 和幂等键。

## 5. 错误、扣减与退款

- 入账过多：新增 `balance_adjustment`，使用正数 `subtract`；先确认扣减不会让余额为负。
- 入账过少：新增 `add`，引用原始付款事件，使用新的幂等键。
- 客户退款：先登记 `refund_recorded`，再按确认的未消费余额执行独立 `subtract`；不能删除原充值。
- 已产生的正常 API 用量默认不退款；计费错误、重复扣费和法律要求另行处理并登记事件。
- 不允许用户之间转余额、提现或负余额赊账。
- 任何 Key 泄漏、超预算调用或账务争议同时追加 `incident_recorded`，只记录事件 ID 和非敏感事实。

## 6. 每日对账

每天固定时间追加 `usage_snapshot`，然后运行：

```bash
ruby ops/manual-ledger.rb summary \
  --ledger config/ledger/manual-ledger.local.jsonl \
  --format json
```

必须核对：

- `payment_received_cny` 与外部已结算收款总额。
- `expected_balance_usd` 与按冻结汇率应入账总额。
- `balance_added_usd` 与 Sub2API 余额历史的 `admin_balance` 增加额。
- `payment_credit_variance_usd` 必须为 0，`unreconciled_order_ids` 必须为空。
- `site_usage_usd` 与 Sub2API 用量/余额扣减。
- `upstream_cost_cny` 与上游请求或余额变化。
- `pending_adjustment_count` 和 `failed_adjustment_count` 必须逐笔处理。

真实余额的期末勾稽还需满足：

```text
期初余额 + 成功增加 - 成功扣减 - 站内使用 = 期末余额
```

本地账本只保存运营侧事件，最终以 Sub2API 数据库/余额历史、上游账单和外部结算记录三方核对为准。

## 7. D05 默认建议（待用户确认）

- 只做小额预付，不赊账、不自动续费、不接受余额提现或转让。
- 邀请内测建议单笔最低 CNY 20、单笔和单用户单日最高 CNY 200；一周无余额事故后再调整。
- 使用定价场景冻结的 USD/CNY 运营汇率，不临时口头换算；变更只影响新订单。
- 不做首充赠送和复杂套餐，避免对账多一层倍率。
- 未消费余额退款边界、手续费承担和到账周期在真实收款前单独确认并公开。

这些是 AI 的默认运营建议，不代表 D05 已确认，也不代表已经建立收款渠道。
