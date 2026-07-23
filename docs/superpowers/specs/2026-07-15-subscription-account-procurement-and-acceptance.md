# L1-6 订阅账号采购比较与单账号验收规格

**状态：** 已确认实施  
**日期：** 2026-07-15  
**来源：** `docs/superpowers/plans/2026-07-15-commercial-ai-api-relay-implementation-plan.md` 第 12 节、D06、D07、D13

## 1. 目标

建立一套不接触真实凭据、不执行购买或登录的账号候选登记、硬淘汰、评分、Sub2API 映射和单账号验收机制。当前只推荐首个样本的品类与配置；卖家和商品必须在最终购买前用真实候选资料复核。

## 2. 当前决策

- 首个样本推荐：独立可控的 OpenAI ChatGPT Plus 成品账号或等价交付，预算不超过 CNY 300。
- 首选原因：采购损失低于 Pro，Sub2API v0.1.155 可映射为 `platform: openai`、`account_type: oauth`，适合先验证完整授权、调用、日志、禁用和成本闭环。
- 当前状态：推荐配置，未选择卖家，未购买，未登录，未授权。
- `K12` 只按卖家标签登记；必须先还原为平台、官方产品、订阅档位和控制权。学校或组织托管、管理员保留控制权的候选直接淘汰。
- Pro 不作为首个样本；只有 Plus 样本证明单位成本和售后可接受后才比较 Pro。

## 3. 方案选择

采用“结构化登记 -> 硬淘汰 -> 合格候选评分 -> 单账号验收”的方案。

没有采用只比较标题和价格的方式，因为 `K12`、`Plus`、`Pro` 可能跨越 OpenAI、Anthropic、Google 等不同平台，标题不能证明权益和授权入口。也不采用先购买再试错的方式，因为 D13 要求所有真实支出集中延后。

## 4. 候选数据边界

候选文件只保存非敏感元数据：卖家公开名称、商品快照引用、价格、平台、档位、控制权、售后、有效期和授权映射。禁止保存密码、Cookie、Access Token、Refresh Token、Session Key、API Key、2FA 恢复码或支付信息。

真实候选使用被 Git 忽略的 `config/subscription-accounts/*.local.yaml`。仓库中的 `*.example.yaml` 只允许虚构数据。

## 5. 平台映射

| 商品还原结果 | Sub2API 平台 | 账号类型 | 正常入口 | 首轮判断 |
|---|---|---|---|---|
| OpenAI ChatGPT Plus/Pro | `openai` | `oauth` | 管理后台 OpenAI OAuth | Plus 首选；Pro 后置 |
| Anthropic Claude Pro/Max | `anthropic` | `oauth` 或 `setup-token` | 管理后台生成的对应授权流程 | 第二品类候选 |
| Google AI Pro/Ultra | `gemini` | `oauth` | `google_one` OAuth，并记录对应 tier | 后续候选 |
| Antigravity 账号 | `antigravity` | `oauth` | 管理后台 Antigravity OAuth | 与 Gemini/Anthropic 分组隔离 |
| 上游中转 API | 对应平台 | `upstream` | Base URL + API Key | 由 UP01 流程管理，不属于 ACC01 |

映射依据是 Sub2API v0.1.155 代码中的平台和账号类型常量及创建账号界面。OpenAI 官方文档同时说明，ChatGPT 登录使用订阅权益，API Key 使用 OpenAI Platform 按量计费，两者不能混为同一成本来源。

## 6. 硬淘汰规则

以下任一项成立即为 `rejected`，不再参与评分：

1. 无法确定真实平台、官方产品或订阅档位。
2. 共享账号、多人共用、组织/学校托管，或管理员/卖家仍保留账号控制权。
3. 买方不能独立控制主邮箱、恢复方式、密码和 2FA，或卖家保留找回权。
4. 只交付 Token、Cookie、Session、浏览器配置或来历不明凭据。
5. 不能使用 Sub2API v0.1.155 对应平台的正常授权入口。
6. 要求提取凭据、绕过登录/验证码/地区限制/风控，或需要未经授权访问。
7. 单个样本价格超过 CNY 300，或商品要求首轮批量购买。
8. 候选文件含疑似真实凭据或支付信息。

## 7. 评分规则

只有未命中硬淘汰的候选才评分，总分 100：

| 维度 | 分值 | 判定重点 |
|---|---:|---|
| 账号控制权 | 30 | 主邮箱、恢复方式、密码、2FA 均归买方；卖家不保留恢复权 |
| 正常授权兼容性 | 25 | 平台、账号类型和授权入口与 Sub2API v0.1.155 对齐 |
| 售后与可追溯性 | 20 | 商品快照、订单引用、换号/退款规则和至少 7 天售后 |
| 样本经济性 | 15 | 单号、CNY 300 内、价格和剩余有效期明确 |
| 运营隔离能力 | 10 | 可单独分组、禁用、记录成本和到期日 |

得分 `>= 85` 为 `recommended`，`70-84` 为 `conditional`，低于 70 为 `not_recommended`。硬淘汰优先于总分。

## 8. 单账号真实验收

真实资产到位后，按清单执行但不在普通文档记录凭据：

1. 核对交付与商品快照一致，买方完成全部控制权接管。
2. 通过 Sub2API 管理后台的正常授权入口新增账号。
3. 账号先放入独立测试分组，默认并发 1、自动过期暂停开启，不与 UP01 混池。
4. 完成非流式、流式、长上下文和工具调用测试，保存请求 ID、模型、Token、错误和耗时。
5. 验证 429/配额耗尽、手工禁用和恢复探测，不在已开始流式输出后跨账号重试。
6. 至少观察 24 小时并计算有效单位成本；未通过前不扩池、不出售该分组。

## 9. 验收标准

- 示例 YAML 能表达 K12、Plus、Pro 候选的真实权益、控制权、售后和 Sub2API 映射。
- 校验器拒绝共享/托管、Token/Cookie-only、卖家保留恢复权、绕过要求、超预算和含疑似凭据的候选。
- 评分器只对未命中硬淘汰的候选评分，并输出推荐等级和分项得分。
- 虚构比较明确推荐独立 OpenAI Plus，淘汰托管 K12 和共享 Pro。
- 采购指南、映射表和单账号真实验收清单均明确“未购买/假定配置”。

## 10. 依据与限制

- [Sub2API v0.1.155 平台及账号类型常量](https://github.com/Wei-Shaw/sub2api/blob/41cec0db059ffb82d0efdcfcf07a24ab51fbfe97/backend/internal/domain/constants.go)
- [Sub2API v0.1.155 创建账号界面](https://github.com/Wei-Shaw/sub2api/blob/41cec0db059ffb82d0efdcfcf07a24ab51fbfe97/frontend/src/components/account/CreateAccountModal.vue)
- [OpenAI 官方认证文档](https://learn.chatgpt.com/docs/auth#openai-authentication)

Sub2API 支持某种授权只证明技术入口存在，不证明消费订阅允许转售或公开提供服务。收费前仍需执行主计划的 D12，并重新核对各平台当时条款。
