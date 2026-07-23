# L1-6 订阅账号采购比较与单账号验收实施计划

> **执行约束：** 由当前主助手在本任务内逐项实施；不使用子代理，不提交 Git，不执行任何购买、登录、授权或真实凭据处理。

**Goal:** 建立可自动校验和评分的订阅账号候选包，并形成首个样本推荐、Sub2API 映射和真实验收清单。

**Architecture:** 使用 YAML 表达候选公开元数据，由单个 Ruby CLI 完成结构校验、硬淘汰和确定性评分。虚构比较和文档共享同一规则；真实候选只允许进入被 Git 忽略的本地文件。

**Tech Stack:** Ruby 标准库、YAML、Minitest、Markdown。

## Global Constraints

- 所有外部资产保持“未购买/假定配置”。
- 禁止保存或处理账号凭据、Cookie、Token、2FA 恢复码和支付信息。
- 只允许 Sub2API v0.1.155 支持的正常授权入口。
- 首个样本预算不超过 CNY 300，首轮只买 1 个账号。
- `K12` 必须先还原真实平台和权益；组织/学校托管直接淘汰。

## 文件职责

- `config/subscription-accounts/*.example.yaml`：三个虚构候选和字段示例。
- `ops/evaluate-subscription-account.rb`：YAML 加载、结构校验、凭据扫描、硬淘汰、评分和 JSON/文本输出。
- `tests/subscription_accounts/evaluate_subscription_account_test.rb`：规则的红绿测试。
- `docs/project/subscription-account-procurement.md`：首样推荐、字段说明、候选比较和最终购买前核对。
- `docs/superpowers/checklists/subscription-account-live-acceptance.md`：真实单账号授权、调用、故障和成本验收。
- `docs/superpowers/reports/2026-07-15-subscription-account-procurement-verification.md`：命令、结果、证据和未验证项。

## 任务

- [x] 1. 先写加载、必填字段、枚举、秘密扫描和硬淘汰的失败测试。
- [x] 2. 运行专项测试，确认因实现缺失而失败。
- [x] 3. 实现最小校验器，使结构和硬淘汰测试通过。
- [x] 4. 先写五维评分、等级和 JSON 输出的失败测试。
- [x] 5. 实现确定性评分，并再次运行专项测试。
- [x] 6. 增加三个虚构候选：独立 OpenAI Plus、托管 K12、共享 Pro。
- [x] 7. 验证独立 Plus 被推荐，另外两个被硬淘汰。
- [x] 8. 编写采购指南、Sub2API 映射和单账号真实验收清单。
- [x] 9. 运行所有 Ruby 测试和基础设施契约测试，检查输出无凭据。
- [x] 10. 更新采购汇总、资产台账、主计划指针和项目当前状态。

## 验证命令

- `ruby -w tests/subscription_accounts/evaluate_subscription_account_test.rb`
- `for f in tests/**/*_test.rb; do ruby -w "$f" || exit 1; done`
- `bash tests/infra/validate-baseline.sh`
- `ruby ops/evaluate-subscription-account.rb compare config/subscription-accounts/*.example.yaml`
- `rg -n -i '(access[_-]?token|refresh[_-]?token|session[_-]?key|api[_-]?key|cookie|2fa.*code)' config/subscription-accounts docs/project/subscription-account-procurement.md docs/superpowers/checklists/subscription-account-live-acceptance.md`

## Acceptance

- [x] 规格第 9 节的五项标准全部满足。
- [x] 专项测试和现有回归测试全部通过。
- [x] 比较结果只推荐虚构独立 OpenAI Plus，并明确所有项目未购买。
- [x] 真实联网、购买、登录和授权均保持未执行。

## 风险

- 卖家描述可能失实，离线评分不能替代交付验收。
- 消费订阅的权益、限额和平台条款会变化，购买前必须重查。
- 技术接入可用不等于允许转售；收费前仍需 D12。
