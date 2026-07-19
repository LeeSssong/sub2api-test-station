# 商用 AI API 中转站项目 Handoff

**交接日期：** 2026-07-19  
**用途：** 新 Codex 窗口的唯一恢复入口  
**当前主线：** D03 GPT-Pro/GPT-Plus 生产切换、relay-ops 上游录入与 D02 正式域名审核并行推进
**当前节点：** relay-ops 已以 `read_only` 部署，公开 `/pricing`、管理员 `/ops` 和 Sub2API 原生 `/monitor` 已验收；生产付费探测为 0。目标分组设计已确认为 `GPT-Pro` / `GPT-Plus`、站内 `1.0x`、Neko `0.10x`、wawazz `0.05x`，但生产实际仍是 `GPT 0.15x`、旧专属分组 `1.0x`、Neko `0.07x`，wawazz 尚未接入。飞书、Agent 和候选记录尚未配置；域名信息模板仍在审核，Aliu 调度关闭

## 1. 新窗口先做什么

先阅读：

1. 本文件。
2. `docs/project/current-state.md`。
3. `docs/project/final-recommendation-report.md`。
4. `docs/project/purchase-recommendations.md`。
5. 需要下钻时再读 `docs/superpowers/plans/2026-07-15-commercial-ai-api-relay-implementation-plan.md`。

注意：后三份文档包含尚未按 2026-07-16 新事实更新的旧结论。本文件中的“新事实与覆盖关系”优先级更高。`final-recommendation-report.md` 内含用户直接添加的回复和疑问，不得覆盖或删除。

## 2. 项目目标与当前路线

目标是低成本、尽快上线一个商用 AI API 中转站。用户只负责关键决策、账户操作和最终付款；Codex 负责方案、配置、部署、测试、故障排查和文档维护。

首版路线：

```text
境外服务器
-> Sub2API + PostgreSQL + Redis + Caddy
-> 已充值的第三方中转 API Key
-> OpenAI 兼容接口
-> GPT / Gemini / DeepSeek
-> 测试用户、调用日志、扣费和人工充值
```

跑通后再逐步增加卡网购买的 K12、Plus、Pro 等消费订阅账号通道，以及其他第三方中转 API 通道。首版采用“人工运维 + AI 辅助”；Agent 自动运维、自动调度和自动止损后置。

消费订阅账号和第三方 Key 的再分发可能违反供应方条款，并存在封号、找回、断供、限额和成本突变风险。当前先做技术与成本验证，不把可调用等同于已获商业转售授权。

## 3. 协作与防漂移机制

所有讨论按以下层级逐层下钻：

```text
大框架 -> 大节点 -> 子任务 -> 可执行操作
```

每次只讨论当前一个层级或一个决策，不一次输出整套长方案。发生发散时使用：

```text
主线：
当前节点：
发散话题：
处理结果：立即决定 / 记录后置 / 回归主线
```

发散信息足够后，Codex 必须主动拉回当前主线。技术细节由 Codex 默认选择；只有预算、供应商、商业规则、不可逆操作或显著风险才交给用户决策。

## 4. 不可违反的约束

- 不执行付款、购买、充值、收款、实名、商户申请或其他真实交易。
- 只在报告中给出选择；所有真实交易由用户审阅后亲自完成。
- 不使用接码平台、购买境外手机号、虚假地区或虚假身份注册国际站。
- 不提供绕过登录、验证码、地区限制或平台风控的方法。
- 不读取、显示或输出 `infra/.env`。
- 不触碰已有容器 `sub2api`、`sub2api-postgres`、`sub2api-redis`。
- 临时域名只用于内部技术验证，不提供给外部测试用户，也不写入项目文档。
- 所有密钥、Cookie、OAuth 凭据、支付密钥和 2FA 恢复码不得进入 Git、普通文档或聊天。
- 不重新讨论已经确认的大框架；从当前节点继续。

## 5. 已完成的离线准备

以下内容已经形成配置、工具、测试或验收清单：

- Docker Compose 部署基线：Sub2API、PostgreSQL、Redis、Caddy。
- 上游信息填报与校验。
- 定价试算与人工账本。
- 订阅账号候选评估。
- 支付禁用态模拟。
- 路由、重试和熔断。
- OPS01 人工运营辅助与止损报告。
- BKP01 备份与恢复方案。
- 最终推荐配置报告。

截至 2026-07-15 的全量离线验证结果为 `79 tests / 328 assertions`，`0 failures / 0 errors`。这只证明离线配置和模拟工具，不代表生产部署或真实供应链已验收。

## 6. 新事实与覆盖关系

以下事实覆盖旧报告中的相应结论：

1. 用户是中国大陆用户，已有阿里云中国站账户，但没有阿里云国际站或腾讯云国际站账户。
2. 腾讯云国际站和阿里云国际站普通注册页均没有中国大陆 `+86` 选项，因此不应把国际站促销视为用户当前可购买方案。
3. 腾讯云国际站东京 2C2G、USD 10.08/年的方案只保留为公开价格参考，旧 D01 的“确定首选”状态失效，必须重新决策。
4. 当前没有正式的“上游下游合作”。用户只是在第三方中转站充值并获得普通用户 Key。该 Key 可以用于技术试跑，但再分发权限、成本持续性和封控风险均未确认。
5. 首版需要 OpenAI 兼容接口，并计划先提供 GPT、Gemini、DeepSeek；具体模型版本和适合作为后续运维 Agent 大脑的模型，留到“供应端与模型目录”节点讨论。

## 7. 阿里云中国站截图中已确认的信息

用户当前查看的是阿里云中国站轻量应用服务器，日本东京地域，未领取优惠券、未付款。

| 套餐 | 配置 | 磁盘 | 线路 | 月价 |
|---|---|---|---|---:|
| 通用型 | 2C2G | 40 GB | BGP | CNY 56 |
| 通用型 | 2C4G | 50 GB | BGP | CNY 112 |
| 国际型 | 2C2G | 40 GB | BGP_NCO | CNY 39 |
| 国际型 | 2C4G | 50 GB | BGP_NCO | CNY 78 |
| 容量型 | 2C2G | 60 GB | BGP | CNY 66 |

共同显示：峰值带宽 200 Mbps、固定公网 IPv4 一个；支持 Ubuntu 24.04 系统镜像；可选 1/3/6 个月和 1/2/3 年；截图中自动续费未勾选。

页面说明：`BGP_NCO` 是非中国优化 BGP，国际型只适用于不需要向中国大陆用户提供服务的场景。大陆访问海外地域可能有较高延迟和丢包，峰值带宽不是业务承诺。

截图尚未确认：年付总价、月流量额度、续费价格或规则、退款规则、新加坡同规格价格。

## 8. 当前待决策与下一步

### 2026-07-16 已购买服务器（覆盖本节旧的购买前流程）

用户已亲自购买腾讯云中国站 Lighthouse 首尔节点，活动实付 CNY 199。控制台只读验收结果：

- 首尔二区，实例运行中。
- 2 vCPU / 4 GiB / 60 GB SSD / 30 Mbps / 1536 GB 月流量。
- Ubuntu Server 24.04 LTS 64bit 系统镜像。
- 到期时间 2027-07-16 09:44:32，自动续费关闭。
- 基础 DDoS 防护和主机安全状态正常；自动化助手在线。
- 默认云防火墙已放通 TCP 22、TCP 80 和 ICMP；尚未配置 TCP 443。
- 未绑定 SSH Key，域名未设置，应用未部署。
- 从当前管理端 Ping 5/5 成功，平均约 78ms；SSH 22 可达并返回 Ubuntu OpenSSH 9.6。

主机安全初始化已完成：

1. 已在当前 Mac 生成专用 Ed25519 SSH Key，没有复用 GitHub Key；文档不记录私钥或公钥内容。
2. 公钥已在腾讯云首尔地域创建并在线绑定到 `ubuntu` 用户；全新 SSH 会话验证成功。
3. 已添加全部 IPv4 来源的 TCP 443 允许规则；22 仍开放，但主机已禁止密码登录和 root 登录，`MaxAuthTries` 为 3。
4. Ubuntu 已完成全量更新并重启到 6.8.0-134 内核；无待更新包和待重启标记。
5. Docker Engine 29.6.1 和 Compose v5.3.1 已安装；日志轮转、`live-restore`、自动安全更新已启用；临时容器实跑成功。
6. Asia/Shanghai 时区、NTP 同步和系统自带约 2 GiB swap 均正常。

当前不再讨论服务器选型或主机初始化。域名策略已暂定：内部技术闭环先使用零成本临时域名；内测候选为腾讯云 `xingqiaolab.top`，API 子域规划为 `api.xingqiaolab.top`；商业化前再评估 `.com`，并由用户亲自购买。

M1 核心站点已完成：

1. 生产主机已从 Caddy bootstrap 切换到 Sub2API、PostgreSQL、Redis、Caddy 完整 Compose；三项健康检查通过，Caddy 正常运行。
2. HTTP 308、HTTPS `/health` 200、公开 TLS 证书和证书 SAN 已验收；切换后一次即时探测处于启动窗口，随后连续探测、组件边界和重启后探测均为 200。
3. 只有 Caddy 发布 80/443；5432、6379、8080 未监听。
4. 五个生产秘密在服务器端生成，环境文件权限为 600；未使用任何上游 Key，也未读取或上传本地 `infra/.env`。
5. 数据库中仅有一个已确认管理员记录；单独重启 Sub2API 后记录仍存在。
6. 临时运行时域名、实例地址、管理员邮箱和密码均不进入项目文档；本机受代理 Fake-IP 影响时使用 `curl --resolve` 验收。

管理员与 UP01 已完成：

1. 管理员已登录生产站并修改密码；全局 2FA 已开放，管理员个人 2FA 已成功绑定。数据库确认 `totp_enabled=true` 且 TOTP 密钥已加密持久化。
2. UP01 Base URL 为 `https://aliuapi.top/v1`；旧 Key 已由用户确认撤销，新 Key 已由用户本人在管理界面录入。
3. UP01 已创建并保存 20 个通过上游目录同步返回的模型；并发 1、优先级 50、倍率 1.0，自动透传、WS 和 pool mode 保持关闭。
4. 初次 503 已定位为上游 Key 绑定故障分组。用户将同一 Key 切到状态正常的稳定 Plus 后，`gpt-5.4-mini` 管理端非流式复测成功；生产日志为 200、约 1.4 秒，上游新增 1 次请求、178 tokens、标准费用 `$0.0003`。
5. 临时域名不得交给外部用户；对外测试前切换到用户购买的正式域名。

真实下游闭环于 2026-07-17 完成：

1. 创建了 `openAI` 分组和低额度、短有效期的下游测试 Key；Key 本身不进入文档或聊天。
2. 首次真实请求返回 403，确认为 UP01 账号尚未绑定 `openAI` 分组；通过官方管理界面绑定后，账号与 Key 进入同一调度域。
3. 第二次真实请求仍返回 403，确认为测试用户余额为零；通过官方用户管理增加 `$0.01` 站内测试余额后恢复。该操作不是外部付款，余额流水备注为 E2E 验证。
4. `gpt-5.4-mini` 非流式请求 HTTP 200，状态 `completed`，精确返回测试目标；记录 555 输入、10 输出 Token，用户扣费 `$0.00074925`，耗时约 2.19 秒。
5. 同模型流式请求 HTTP 200，收到完整 Responses SSE 生命周期并精确返回测试目标；记录 554 输入、9 输出 Token，用户扣费 `$0.00074400`，耗时约 3.11 秒，首 Token 约 3.07 秒。
6. 两次请求合计扣费 `$0.00149325`；API Key `quota_used` 与测试用户余额减少额完全一致，数据库留下两条独立 usage 记录，未出现重复计费。
7. 上游控制台出现同一时刻的两条 `gpt-5.4-mini` 记录，每条标准费用显示约 `$0.0007`；两条实际费用合计因页面精度仅显示约 `$0.0001`，与稳定 Plus 折扣口径一致。
8. 保存账号分组配置后，Sub2API 自动执行了一次 `codex-auto-review` Responses 能力探测；服务日志明确标记为 `openai_probe`，上游实际费用约 `$0.0003`、标准费用约 `$0.0081`。该探测未计入测试用户用量，但属于运维成本。
9. 验收完成后测试 Key 已禁用但未删除，便于保留审计证据；UP01 仍为正常且可单独停用。

D03 于 2026-07-17 形成 `Proposed`：

1. 三笔 `gpt-5.4-mini` 样本确认标准价格为输入 `$0.75/M`、输出 `$4.50/M`、缓存读取 `$0.075/M`；稳定 Plus 实际扣费约为标准价 4%，但售价不依赖该折扣。
2. 基准假设为 30M Token/月、20% 输入、5% 输出、75% 缓存读取，服务器固定成本 CNY 199/12，支付费率 0%，异常准备 10%，目标完全成本毛利 25%。
3. 邀请制暂定价为输入 CNY 8.70/M、输出 CNY 48.30/M、缓存读取 CNY 1.60/M；缓存写入不提供。
4. 基准预测月收入 CNY 160.65、完全成本利润 CNY 41.6017、毛利 25.90%。保持该价格时，盈亏平衡约 8.55M、20% 毛利约 19.09M、25% 毛利约 27.60M Token/月。
5. 价格没有写入生产。上游容量、退款、缓存写入价格和再分发条款仍未知，不能公开售卖。
6. 决策记录：`docs/superpowers/decisions/2026-07-17-d03-provisional-gpt54mini-pricing.md`。

该版绝对 CNY 单价随后被竞品的“CNY 充值额按 USD 内部额度 1:1 入账，再乘模型倍率”口径覆盖，且从未配置生产。2026-07-18 用户确认 D03 方案一：模型使用明确的官方/上游标准基础价，GPT 用户组倍率 `0.15x`；Neko 已切为生产，账号倍率 `0.07`、并发 `3`，Aliu 调度关闭并保留候选。自动支付仍关闭。完整决策见 `docs/superpowers/decisions/2026-07-18-d03-mvp-plan-one-pricing-and-upstream.md`。

RPM 是 requests per minute，即每分钟请求数；它不等于 TPM（每分钟 Token 数）或并发数（同时进行中的请求数）。方案一暂不对外承诺未经供应商确认的 RPM 数字，当前硬容量边界是 UP01 有效并发 `1`。

候选低价上游首轮验证于 2026-07-17 完成：

1. 候选站点的模型目录接口可用并包含 `gpt-5.6-sol`；因余额不足，`gpt-5.6-sol` 在预扣阶段被拒绝且未产生费用，未再次尝试。
2. `gpt-5.4-mini` 非流式和 Responses SSE 流式请求均成功；两路同时请求虽均返回 200，但完成时间明显串行，暂按有效并发 1 评估。
3. 生产创建临时账号 `shuai-test-20260717`（账号 #3），只配置 `gpt-5.6-sol` 和 `gpt-5.4-mini`、并发 1、优先级 99、倍率 1，且从未绑定任何分组。
4. 首次管理测试被生产 URL 白名单拒绝；根因是 `SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS` 只有原上游域名。已以逗号分隔追加候选上游域名，并只重建 Sub2API 容器；PostgreSQL、Redis、Caddy 容器保持不变。
5. 修复后候选账号的 `gpt-5.4-mini` 管理测试成功；原 UP01 同模型回归也成功。候选账号随后关闭调度并显示暂停，仍无分组，不能接收用户流量。
6. 没有充值、购买或把候选 Key 写入聊天/文档；测试结束后已清空浏览器剪贴板、临时变量和显示完整 Key 的临时标签页。

NekoAPI Pro 池候选短测于 2026-07-18 完成：

1. 使用注册送余额创建独立 `$0.10` 限额测试 Key；未充值、未绑定生产流量，测试后已停用并删除，生产 UP01 和路由未修改。
2. `https://api.999555999.com/v1/models` 返回 200；`gpt-5.6-sol/terra/luna`、`gpt-5.5` 和 `gpt-5.4-mini` 均完成非流式请求，`gpt-5.6-sol` 流式 SSE 完整。
3. 11 次请求合计 204 个站方计费 Token；标准费用 `$0.0026`，实际逐请求费用约 `$0.000182`，验证 Pro 池实际倍率约为 `0.07x`。
4. `gpt-5.6-sol` 2 路和 3 路并发的总墙钟时间均约 2.39 秒，所有请求返回 200，短测至少支持 3 路真实并发。
5. API usage Token 与站方计费 Token 存在差异，后续接入 Sub2API 时必须做用户扣费、上游 Token 和实际金额三方核对。
6. 当前只判定为“短测通过的候选”。长期稳定性、国内三网/首尔节点时延、长流错误率和商业再分发条款仍未验证；官方 UI 无法为临时用户创建/分配 API Key，故 `live_gateway` 与 `live_billing` blocked，不替换生产 UP01。详细记录：`docs/superpowers/reports/2026-07-18-neko-upstream-short-verification.md` 与比较报告。

NekoAPI 隔离网关与计费闭环随后于 2026-07-18 完成，覆盖上述 blocked 结论：

1. 历史记录：管理员新建低额度、7 天有效的 `test-neko` Key；测试分组设为专属，隔离账号并发 `1`、优先级 `99`，只绑定测试分组。当时测试不触碰生产 Aliu 路由；该事实已被后续用户批准的 Neko 生产切换覆盖。
2. 两次非流式请求和一次 SSE 请求均通过公开 Sub2API 入口返回 200；SSE 收到 25 个事件和一个 `[DONE]`，生命周期完整。
3. 三笔 API usage 合计 `46` 输入、`179` 输出、`225` Token；Sub2API 分别扣 `$0.001965`、`$0.001965`、`$0.001670`，合计 `$0.005600`。
4. Neko 同时刻三笔记录的 Token 与 API/Sub2API 完全一致，实际费用分别为 `$0.000138`、`$0.000138`、`$0.000117`，合计 `$0.000393`；上游标准费用页面合计约 `$0.0057`，显示精度下倍率约 `0.06895x`，与 Pro 池声明 `0.07x` 一致。
5. 测试后 Sub2API Key 已停用、管理员专属组授权已撤销、隔离账号和分组已删除、Neko 供应方测试 Key 已停用，临时剪贴板凭据已清空；生产 Neko 保持调度，Aliu 账号暂停且保留回滚。
6. Neko 的 `/keys` 直接导航会命中公开站点，而从 `/dashboard` 侧栏进入可正常显示管理页；这是服务端直达路由与 SPA 路由冲突，不是实际退出登录。后续操作必须先进入 `/dashboard` 再点侧栏。

2026-07-19 生产定价与升级记录：

1. Sub2API 应用内升级到 `v0.1.161` 后，因默认 `gateway.text_max_body_size=32 MiB` 超过部署的 `gateway.max_body_size=16 MiB` 短暂返回 502；已固定 `GATEWAY_TEXT_MAX_BODY_SIZE=16777216`，仅重建 Sub2API，HTTPS `/health` 恢复 200。
2. 生产渠道 `GPT` 已保存六条请求模型定价：`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.5`、`gpt-5.4`、`gpt-5.4-mini`；限制模型开启、计费基准为 requested、用户组 `0.15x`，图片桥接关闭。
3. 受控验证中六次同步和一次 `gpt-5.6-sol` SSE 均 HTTP 200，SSE 收到 `[DONE]`；未知模型返回 404。7 条使用记录的 Token 均为输入 8、输出 5，用户扣费合计约 `$0.000125`，与标准价乘 `0.15x` 一致；测试用户和 Key 已删除。

2026-07-18 收尾清理：Sub2API 临时用户 ID `3`、`neko-benchmark-20260718` 分组和 Neko 隔离账号 #5 均已删除；Neko 供应方测试 Key 已停用。随后生产账号 `neko-production-primary` 已启用，Aliu #2 调度关闭并保留在 GPT 分组作为候选。

统一上游验收机制于 2026-07-18 建立：

1. `ops/upstream-benchmark.rb` 提供 `validate`、`run`、`import`、`compare` 和 `decide`，Key 仅从运行时环境变量读取。
2. `config/upstream-benchmarks/` 保存 Aliu/Neko 非敏感档案、`mvp-text-v1` 固定基准、追加式运行/决策台账和带证据级别的历史导入。
3. 个人 Skill `$benchmark-upstream-channel` 固定 direct、gateway、billing、network、terms、decision、cleanup 七阶段流程。
4. 全量 Ruby 回归当前为 `97 tests / 391 assertions`，`0 failures / 0 errors / 0 skips`；基础设施契约通过。
5. 当前比较报告：`docs/superpowers/reports/2026-07-18-upstream-channel-comparison.md`。Neko `live_direct`、`live_gateway` 与 `live_billing` 已通过；用户已批准 Neko 生产切换，Aliu 调度关闭并保留为暂停候选。容量报告：`docs/superpowers/reports/2026-07-19-neko-capacity-verification.md`。
6. 比较器现支持追加式 `supersedes` 纠正语义，即使旧记录时间戳异常靠后，也不会压过明确的纠正记录；旧证据仍保留审计历史。

2026-07-19 上游接入评测 V2 已实现：

1. `ops/upstream-benchmark-v2.rb` 与 `config/upstream-benchmarks/mvp-text-v2.yaml` 提供完整模型发现/分类、每个文本模型独立同步与 SSE、并发 `1/2/3/5/8/10`、RPM `6/12/20/30` 有界探测、错误停阶和排队识别。
2. V2 价格顾问要求逐模型输入/输出价格和可解释实际倍率；默认异常准备 `10%`、目标完全成本毛利 `50%`、内测倍率 `1.0`，并输出理论最低、支付调整、固定成本和建议商业倍率。Neko 示例在成本倍率 `0.07`、支付费率 `3%` 时建议商业倍率 `0.18`。
3. V2 proposal 包含 `requested` 计费、模型限制、模型映射、四类 Token 价格、账号成本倍率、并发/RPM 建议和 SHA-256 proposal hash；不包含 Key 或模型输出。
4. 个人 `$benchmark-upstream-channel` Skill 已升级为浏览器供应商 Key、价格/账单取证、用户明确回复“采纳”后快照/隔离应用/生产配置/回验/清理。用户未采纳时不改生产；浏览器无法安全桥接直接 Key 时，direct 证据保持 `unknown`。
5. 当前仅完成离线实现和测试，尚未用 V2 对新供应商执行实时网络评测，也未自动应用任何新 proposal。

## 9. 新窗口可直接使用的 Loop Brief

```text
Goal: 完成 GPT-Pro/GPT-Plus 生产配置和 relay-ops 的 Neko/wawazz 只读录入，再准备一次明确报价的低成本候选 probe。
Context: relay-ops 镜像 `665fa0a` 已在生产 `read_only` 运行，`/pricing`、`/ops`、`/monitor` 已验收，`probe_runs=0`。目标是 GPT-Pro/GPT-Plus 站内 `1.0x`、Neko `0.10x`、wawazz `0.05x`；当前实际仍是 GPT `0.15x`、旧专属分组 `1.0x`、Neko `0.07x`，wawazz 未接入。域名审核并行进行。
Constraints: 项目是副业，日常流程必须无人值守；不读取或输出 `infra/.env`；任何 JWT、Admin API Key、用户 Key、密码或 2FA 信息不得进入 Git、普通文档、日志或聊天；不直接写 Sub2API 数据库，不维护私有 Sub2API 分支，域名和自动化验收前不邀请外部用户。
Plan: 先对现有 Sub2API 分组/账号做快照；创建或重命名分组并设置目标倍率，wawazz 用独立账号/Key 接入；逐分组完成模型、同步、SSE、计费和原生监控验收。随后把公开价格/用量页和服务器秘密引用录入 relay-ops，仍保持 `read_only`。
Implement: 仅通过 Sub2API 官方管理/API 和 relay-ops 管理 API 操作，不直接写数据库；飞书/Agent 使用独立秘密文件。首次 probe 前单独报告每候选 2 请求、最多 `$0.002` 的费用并等待明确批准。
Validate: 核对公开组名/倍率/绑定账号、Neko/wawazz 成本倍率、模型价格、同步/SSE、TTFT、用户扣费与上游成本；确认 `/pricing` 只显示公开组，`/monitor` 有有效样本，`probe_runs` 在批准前仍为 0。
Review: Admin API Key、候选监测 Key 和上游会话必须分离；账单真实倍率只是辅助证据，无法读取时不得伪装成已验证；任何 Agent 输出都不能直接修改路由、价格、余额、Key 或数据库。
Done when: GPT-Pro/GPT-Plus 在生产按目标配置通过隔离验收，Neko/wawazz 已进入 relay-ops 只读监控，飞书/Agent 假事件通过；真实 probe 只在用户明确批准后执行，域名审核不被阻塞。
```
## D04 首发自动化历史基线（2026-07-19）

本地实现已完成，代码入口为 `internal-test-service/`，Compose/Caddy 集成为 `infra/compose.yaml`、`infra/Caddyfile` 和 `infra/Dockerfile.internal-test`。

- 规则：最多 15 名注册用户；旧专用分组站内倍率 `1.0x`；每个上海日 `$20` 签到；每个被推荐用户首次成功计费请求奖励 `$5`；签到和推荐进入同一累计余额，余额不跨日清空；多个未使用邀请码可并存。`internal-test-service` 和 `D04_*` 仅是既有内部代码标识，产品页面和后台不展示“内测”命名。
- 服务：Go 1.24 + SQLite WAL；JWT 通过 Sub2API `/api/v1/auth/me` 验证；Admin API 只用只读挂载的 `x-api-key` 文件；所有余额写入固定幂等键，超时后先查余额历史；余额漂移自动进入只读并告警。
- 入口：Caddy 将 `POST /api/v1/auth/register` 和 `/internal-test/*` 交给独立服务，OAuth 建号路径返回 403；服务没有宿主机端口，根文件系统只读，默认 `D04_MODE=read_only`。
- 验证：`go test ./... -race`、`go vet ./...`、Docker 镜像构建、Compose/Caddy D04 契约、基础设施基线和既有 Ruby 回归均通过。真实 Admin API Key、飞书 Webhook、JWT、用户 Key 和生产余额均未触碰。

该基线尚未启用生产写入。启用首发用户自动化前仍需：完成正式域名和 GPT-Pro/GPT-Plus 切换，以只读模式运行一个调度周期，安装权限受限的真实 Admin API Key/Webhook，再使用隔离低额用户验证签到、重复签到、首次推荐奖励和余额三方对账。只有这些通过且用户明确批准，才把 `D04_MODE` 改为 `write` 并签发首个外部邀请码；这些门禁不阻塞当前 relay-ops 和上游分组工作。

## relay-ops 监控与预警 Agent 设计（2026-07-19）

设计文件：`docs/superpowers/specs/2026-07-19-relay-ops-monitoring-and-alert-agent-design.md`。

- 架构已确认：独立 `relay-ops` 服务与 Sub2API 并行，Caddy 提供 `/pricing`、`/performance`、`/ops` 路径；不 fork Sub2API，不直接写其 PostgreSQL。
- 监控范围：所有已启用且对客户公开的 Sub2API 分组；候选站由管理员录入名称、Base URL、低额度 Key、定价/倍率页和用量/账单页。
- 采集策略：生产质量每分钟、生产价格/倍率每 5 分钟、候选页面每 15 分钟、候选同步/SSE 每 6 小时；全局探测预算一次配置，所有上游继承。
- 告警机制：原始探测全量入库；事件状态机确认后才发飞书，采用状态变化、持续恶化、恢复、价格/倍率变化、候选综合变优和每日摘要，避免重复消息。
- Agent：只读、事件触发、输入为脱敏结构化 JSON，只能查询质量窗口/探测/价格/候选证据；不可读取秘密或执行路由、价格、余额、Key、数据库写操作。
- 真实账单：默认相信上游标注倍率和定价；能读取则作为辅助证据，不能读取时保留质量/价格监控并明确显示“真实倍率未验证”，不反复要求管理员授权。
- 页面：`/pricing` 无需登录；`/performance` 仅登录用户可见；`/ops` 仅管理员可见。生产和候选上游名称、采购倍率和内部告警不向客户暴露。
- 当前状态：实现、本地 Fake Provider 验证和生产 `read_only` 部署已完成。生产镜像为 `665fa0a`；真实 `/ops` 管理员登录、`/pricing` USD/1M 与阶梯价、原生 `/monitor` 均通过。飞书/Agent 真实凭据、Neko/wawazz 记录和首次真实 probe 尚未配置。

详细运维与回滚见 `docs/runbooks/relay-ops-monitoring.md`；验证证据见 `docs/superpowers/reports/2026-07-19-relay-ops-read-only-verification.md`。
