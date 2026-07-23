# L1-6 订阅账号采购比较与验收设计验证

**日期：** 2026-07-15  
**范围：** ACC01 候选结构、硬淘汰、评分、虚构比较、采购配置和真实验收清单  
**资产状态：** 全部未购买/假定配置

## 结果

- 首个样本推荐已固定为独立可控 OpenAI ChatGPT Plus，1 个，硬预算 CNY 300。
- Sub2API 映射为 `openai + oauth`，独立测试分组，并发 1，到期自动暂停。
- 虚构独立 Plus 得分 100/100，结论 `recommended`。
- 虚构托管 K12 和共享 Token-only Pro 均命中硬淘汰，结论 `rejected`。
- 没有选择或背书具体卖家，没有购买、登录、授权或处理真实凭据。

## TDD 证据

1. 创建测试后首次运行失败，原因为 `ops/evaluate-subscription-account.rb` 尚不存在。
2. 实现后专项测试暴露合法 `authorization` 分区误报和 Psych 兼容问题。
3. 修正后专项测试通过：9 tests / 49 assertions / 0 failures / 0 errors。

## 验证命令

```bash
ruby -w tests/subscription_accounts/evaluate_subscription_account_test.rb
ruby ops/evaluate-subscription-account.rb compare config/subscription-accounts/*.example.yaml
```

全量回归结果：38 tests / 140 assertions / 0 failures / 0 errors。  
基础设施契约：`PASS: infrastructure baseline contracts`。  
YAML 解析、Markdown 围栏和目标文件秘密值扫描：PASS。

## 比较摘要

```text
recommended: 1
conditional: 0
not_recommended: 0
rejected: 2
invalid: 0
```

## 依据

- Sub2API v0.1.155 commit `41cec0db059ffb82d0efdcfcf07a24ab51fbfe97` 的平台、账号类型和创建账号实现。
- OpenAI 官方认证文档：ChatGPT 登录用于订阅权益，API Key 用于 OpenAI Platform 按量计费。
- 主计划 D06、D07、D13 和 R01。

## 未验证

- 真实卡网、卖家、库存、商品、价格和售后条款。
- 真实账号控制权、OAuth、模型、配额、并发、失败率和单位成本。
- 各平台对预期经营方式的最新条款判断。

这些项目只能在用户审阅最终报告并自行购买后按真实验收清单完成，当前不构成离线工作的阻塞。
