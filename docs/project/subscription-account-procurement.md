# 订阅账号采购比较与首样配置

**更新日期：** 2026-07-15  
**资产状态：** 推荐已定，未选择卖家，未购买，未登录，未授权  
**适用节点：** ACC01 / L1-6

## 当前选择

首个样本选择“独立可控的 OpenAI ChatGPT Plus 账号”，只买 1 个，实付总额不超过 CNY 300。优先选择价格不超过 CNY 200、剩余有效期至少 30 天、售后至少 14 天且支持退款或换号的商品。

首轮不选择：

- 学校或组织托管的 `K12` 账号。
- 共享 OpenAI Pro 或多人共用账号。
- 只交付 Token、Cookie、Session、浏览器环境或恢复码的商品。
- 卖家保留主邮箱、恢复方式、2FA 或找回权的商品。
- 要求绕过登录、验证码、地区限制或风控的商品。
- 强制批量购买或单样本超过 CNY 300 的商品。

当前没有指定卡网或卖家。Sub2API README 中的赞助商描述只能证明公开营销存在，不能证明商品库存、控制权、售后或账号来源已经验收。最终报告审阅后，应把当时仍在售的真实商品分别录入本地候选文件，再由同一规则比较。

## 推荐配置

| 项目 | 选择 |
|---|---|
| 平台 | OpenAI |
| 官方产品 | ChatGPT |
| 订阅档位 | Plus |
| 数量 | 1 |
| 理想价格 | 不超过 CNY 200 |
| 硬预算 | 不超过 CNY 300 |
| 剩余有效期 | 至少 30 天优先；低于 14 天不推荐 |
| 交付 | 买方获得独立账号控制权 |
| 买方控制 | 主邮箱、恢复方式、密码、2FA 全部可独立控制 |
| 卖家控制 | 不保留恢复权或管理员控制权 |
| 售后 | 至少 14 天优先，退款或换号规则有公开快照 |
| Sub2API 授权 | 管理后台正常 OpenAI OAuth |
| 首测并发 | 1 |
| 扩池条件 | 24 小时验收完成且有效单位成本低于拟定售价 |

## Sub2API 映射

真实账号到位后，先使用独立测试分组，不与 UP01 混池：

| Sub2API 字段 | 建议值 |
|---|---|
| `platform` | `openai` |
| `type` | `oauth` |
| 账号名称 | `ACC01-openai-plus-sample-01` |
| 分组名称 | `acc01-openai-plus-test` |
| `concurrency` | `1` |
| `priority` | `50`；仅在独立分组内比较 |
| `rate_multiplier` | `1.0`；价格由独立分组设置 |
| `expires_at` | 按真实到期日填写 |
| `auto_pause_on_expired` | `true` |
| `schedulable` | 授权后仅在测试分组开启；未通过验收前不进入售卖分组 |

OpenAI Plus/Pro 使用 ChatGPT 订阅登录权益；OpenAI API Key 是 OpenAI Platform 按量计费。两者成本、配额和条款必须分别登记。

## 候选比较

仓库内三个候选均为虚构数据：

| 候选 | 结果 | 原因 |
|---|---|---|
| 独立 OpenAI Plus | `recommended`，100/100 | 独立控制、正常 OAuth、售后和证据完整、单号预算内 |
| 托管 K12 | `rejected` | 组织托管、买方控制不完整、卖家保留恢复权 |
| 共享 OpenAI Pro | `rejected` | 共享、Token-only、需要凭据提取、不能正常授权 |

运行比较：

```bash
ruby ops/evaluate-subscription-account.rb compare config/subscription-accounts/*.example.yaml
```

## 真实候选登记

最终购买前，每个商品单独创建 `config/subscription-accounts/<ID>.local.yaml`。该路径已被 Git 忽略，只填公开和非敏感资料；不得填入密码、Token、Cookie、Session、恢复码或支付信息。

比较完成后只选择：未命中硬淘汰、评分至少 85、且价格和条款仍符合本页推荐配置的一个候选。真实候选不足时保持未购买，不用低质量候选补位。

## 最终购买前核对

- [ ] 商品实际平台、官方产品和订阅档位已还原，不只看 `K12/Plus/Pro` 标题。
- [ ] 账号独享，非学校/组织托管，非邀请子账号，非共享池。
- [ ] 主邮箱、恢复方式、密码和 2FA 均可由买方独立控制。
- [ ] 卖家不保留管理员、恢复或找回权。
- [ ] 交付可走 Sub2API v0.1.155 正常 OpenAI OAuth。
- [ ] 数量为 1，实付不超过 CNY 300，不绑定批量或自动续费。
- [ ] 到期日、售后起止日、退款/换号条件有快照。
- [ ] 当前适用的平台条款和 D12 已复核。
- [ ] 用户自行完成支付；AI 不登录、不购买、不付款。

真实资产到位后的技术验收见 `docs/superpowers/checklists/subscription-account-live-acceptance.md`。

## 公开依据

- [Sub2API v0.1.155 平台与账号类型](https://github.com/Wei-Shaw/sub2api/blob/41cec0db059ffb82d0efdcfcf07a24ab51fbfe97/backend/internal/domain/constants.go)
- [Sub2API v0.1.155 创建账号流程](https://github.com/Wei-Shaw/sub2api/blob/41cec0db059ffb82d0efdcfcf07a24ab51fbfe97/frontend/src/components/account/CreateAccountModal.vue)
- [OpenAI 官方认证说明](https://learn.chatgpt.com/docs/auth#openai-authentication)
