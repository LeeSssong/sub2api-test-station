# 商用 AI API 中转站项目 Handoff

**交接日期：** 2026-07-23
**用途：** 新 Codex 窗口的唯一恢复入口  
**当前政策（2026-07-26）：** D04 / internal-test 运行时、部署文件和受控开放流程已退役。注册与邀请码仅由 Sub2API 原生管理；Caddy 不再拦截认证或公开设置接口。内测人数上限、预算门禁、每日额度和独立注册代理均不得恢复。
**当前主线：** 账号质量巡检和 relay-ops 继续只读观测 `active + schedulable` 的动态账号集合，不控制注册、邀请码、路由、账号、额度或余额。后文所有 D04 段落均为历史证据，不能作为当前操作说明。

**最新部署证据：** `relay-ops` 已更新为 `ops-dual-domain-20260723-v1`，container `84ad983a6be4`，healthy/restart `0`，仍为 `read_only + dry_run`。collector SHA-256 为 `20452b9218af79fb6ba19895d7f250b6754011a828eaf3e3f77f69faf8e90ea9`；一次 systemd 验收成功生成 `ACCOUNT-QUALITY-20260723T152705Z`，结果 SHA-256 为 `4677925dbbcee85f4881c155c513bdb8f7fee2460bb851420b545fc3bd181536`。timer 为 `enabled/active`，五个公网入口为 200，匿名 ops-view 为 404，除 relay-ops 外的容器 ID 未变。旧 model-release timer 仍 `disabled/inactive`，其一次性 service 的既有 `exit-code` 历史状态不属于本次部署且未被重启。
**非功能子 Agent 收口：** 公共 v3 又补齐了 shared-pool 的 `topology_phase/wave_id/TTFT/总耗时` 证据、观察窗口的 sync/SSE 指标以及 topology evidence 的 `scenario_id/scenario_hash` 绑定；非功能测试 `32/160`、V2 `32/194`、protocol `10/44`、V1 `18/63` 全部通过。XM/Wawazz 目标角色仍没有 live 资格证据，目标状态为 `NOT_READY`，详见 `docs/superpowers/reports/2026-07-22-nonfunctional-baseline-subagent.md`。

**历史 D04 验收：** 单用户窗口、`403 D04_REGISTRATION_CLOSED` 和相关设置恢复只保留为历史审计证据；D04 文件与运行时现已退役。禁止再用空对象 PUT 探索设置接口。
**飞书边界：** 告警、恢复、日报、Interactive Card、命令卡、去重、App Bot 投递和确定性只读分析均已收口，并保留在当前 `ops-dual-domain-20260723-v1` 镜像中。账号质量巡检不逐次发送飞书消息；本轮没有发送合成消息或制造事件。告警/恢复卡真实视觉仅等待自然事件，属于非阻塞观察；不删除去重记录、不制造故障。

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
4. 全量 Ruby 回归当前为 `118 tests / 472 assertions`，`0 failures / 0 errors / 0 skips`；基础设施契约通过。
5. 当前比较报告：`docs/superpowers/reports/2026-07-18-upstream-channel-comparison.md`。Neko `live_direct`、`live_gateway` 与 `live_billing` 已通过；用户已批准 Neko 生产切换，Aliu 调度关闭并保留为暂停候选。容量报告：`docs/superpowers/reports/2026-07-19-neko-capacity-verification.md`。
6. 比较器现支持追加式 `supersedes` 纠正语义，即使旧记录时间戳异常靠后，也不会压过明确的纠正记录；旧证据仍保留审计历史。

2026-07-19 上游接入评测 V2 已实现：

1. `ops/upstream-benchmark-v2.rb` 与 `config/upstream-benchmarks/mvp-text-v2.yaml` 提供完整模型发现/分类、每个文本模型独立同步与 SSE、并发 `1/2/3/5/8/10`、RPM `6/12/20/30` 有界探测、错误停阶和排队识别。
2. V2 价格顾问要求逐模型输入/输出价格和可解释实际倍率；默认异常准备 `10%`、目标完全成本毛利 `50%`、内测倍率 `1.0`，并输出理论最低、支付调整、固定成本和建议商业倍率。Neko 示例在成本倍率 `0.07`、支付费率 `3%` 时建议商业倍率 `0.18`。
3. V2 proposal 包含 `requested` 计费、模型限制、模型映射、四类 Token 价格、账号成本倍率、并发/RPM 建议和 SHA-256 proposal hash；不包含 Key 或模型输出。
4. 个人 `$benchmark-upstream-channel` Skill 已升级为浏览器供应商 Key、价格/账单取证、用户明确回复“采纳”后快照/隔离应用/生产配置/回验/清理。用户未采纳时不改生产；浏览器无法安全桥接直接 Key 时，direct 证据保持 `unknown`。
5. 当前仅完成离线实现和测试，尚未用 V2 对新供应商执行实时网络评测，也未自动应用任何新 proposal。

## 9. 2026-07-20 双上游切换结果

- Sub2API 生产 URL allowlist 已追加 `wawazz.xyz`，只重建 `sub2api`，PostgreSQL、Redis、Caddy 未重建。
- 分组已正式命名并保存：`GPT-Pro` 公开、站内 `1.0x`，绑定 Neko；`GPT-Plus` 公开、站内 `1.0x`，绑定 Wawazz。临时 `wawazz-test` 分组已删除。
- 账号已正式命名：`neko-production-primary` 绑定 `GPT-Pro`、账号成本倍率 `0.10x`、并发 `2`；`wawazz-production-primary` 绑定 `GPT-Plus`、账号成本倍率 `0.05x`、并发 `1`。Neko 另保留并发 `1` 的隔离复制账号，生产主/备容量合计 `3`。
- Wawazz 余额已恢复，原生监控为 `operational`。账号 `8` 的 `credentials.user_agent` 已从空值改为 `node`；`Python-urllib` 同步和 `OpenAI/Python` SSE 均 HTTP 200，SSE 收到 `[DONE]`，两条实测成本约 `0.0502x`。
- Neko 正式 Pro 同步/SSE 均 HTTP 200；两条记录均为 `10/5` Token，Sub2API 各扣 `$0.000200`，Neko 各实际 `$0.000020`，实测 `0.10x`，SSE 收到 `[DONE]`；原生监控为 `100%`、`1396 ms`。
- 本轮创建的 3 个用户 API Key、临时用户和临时分组均已删除；未保留任何 Key 原文。

## 10. relay-ops 生产来源录入（2026-07-20）

- 生产来源录入阶段曾部署镜像 `sub2api-relay-ops:ops-pricing-parser-20260720-v7`（AMD64 image ID `sha256:d6a42dcd9076acaeb1facf8a352bf349da3f695d4c8196179f5e7ff6de9ea6ae`）；该镜像已由下文飞书命令禁用态版本取代，来源录入和解析能力保留。
- 管理员 `/ops` 已新增生产来源录入表单；只填写来源名称、HTTPS Base URL、公开页面和客户可见分组名称，不保存第二份生产 API Key。
- 已录入：Neko `https://api.999555999.com` -> `GPT-Pro`，价格 `/pricing`、用量 `/usage`、性能 `/monitor`；Wawazz `https://wawazz.xyz` -> `GPT-Plus`，公开页面 `/home`、用量 `/usage`、性能 `/monitor`。Wawazz 没有公开 `/pricing`，因此其价格不能标记为已验证。
- 生产验证：`go test ./...`、`go vet ./...`、公网 `/healthz`、`/readyz`、`/pricing`、`/ops` 和 `/relay-ops/static/ops-admin.js` 均通过；管理员动态视图与引导请求已禁用缓存，移动端页面宽度 `468/468`、宽表容器内部滚动，桌面无整体溢出，临时移动视口已清除。
- 会话验证：账单读取支持服务器秘密文件、Bearer/Cookie、登录页识别、401 重试一次、24 小时告警去重和恢复事件；生产 `/run/secrets/upstream-sessions` 当前为空，因此真实倍率/账单辅助证据尚未启用。
- 解析器验证：旧 HTML 全文正则曾把 Neko 页面终端装饰 `bash - 80x24` 解析为 `80x`，并把 Wawazz 编码内容解析为 `0x`。v7 改为只采信带“倍率/multiplier/rate”语义标签、显式 data 属性或结构化 JSON 字段的倍率，并通过 `pricing-evidence-v2` 在页面 hash 不变时安全追加重解析。自然 5 分钟周期已把两个来源更新为 `unparseable`；这表示公开页面未声明倍率，不表示上游故障，也不覆盖 Sub2API 中 `0.10x/0.05x` 的配置基线。
- 详细证据：`docs/superpowers/reports/2026-07-20-relay-ops-production-source-monitoring-verification.md`。

## 11. 新窗口可直接使用的 Loop Brief

```text
Goal: 使用新鲜轻量账号池质量和最低余额证据完成 D04 首发用户受控开放准备。
Context: 双区 `/ops`、日报和告警集成已部署为 `ops-dual-domain-20260723-v1`；`sub2api-account-quality-monitor.timer` 每 15 分钟动态发现 `active + schedulable` 集合并顺序调用 Sub2API 原生账号测试。当前集合为 `[10,11,12,13]`，集合哈希为 `f6b733f89e799048c92d90dc0d404ce1f96300bf1f2964184cc681bdcc2457e7`。最新 45 样本中成功数为 `27/12/0/30`，TTFT P95 约 `12.27s/11.92s/无成功样本/14.28s`，最新一轮均为 `account_test_error`。D04 为 `read_only/registration=false`，relay-ops 为 `read_only + dry_run`。
Constraints: 不读取或输出生产 `.env`、App Secret、verification token、Encrypt Key、Admin API Key、上游 Key、Cookie 或密码；不直接写 Sub2API PostgreSQL；不按供应商名称或固定账号 ID 设置例外，不改任何上游余额、路由、倍率、价格、Key、模型或调度。飞书不进入 `enabled`，不恢复复杂模型资格、容量、排序或发布任务。
Plan: 让 systemd 巡检自然累积质量窗口，复用 Sub2API 原生账户和余额能力补齐 D04 门禁证据；先确认账号 `12` 的持续零成功及其余账号高 TTFT/错误，再判断是否达到轻量门禁。门禁通过后再申请新的 D04 行动批准。
Implement: 只更新无秘密账号质量、最低余额与 D04 准入证据；不由 LLM 手工逐账号测试、不重复 D04 grant、不扩展飞书、不创建候选或启用 probe。
Validate: 核对 timer、结果新鲜度、集合 hash、各账号最新结果和最低余额；复核 D04 403/只读、账户备份、relay-ops `read_only + dry_run`、零候选/probe、路由哈希和注册关闭。任何历史值都不得替代 SSH 新鲜证据。
Review: 重点审查账户集合漂移、单账号失败隔离、余额分类是否明确、样本新鲜度、首发总预算和注册关闭回滚。
Done when: 动态活动账号满足轻量最低余额与新鲜质量条件，D04 准入结果允许开放，并在新的明确行动批准下打开注册；此前继续关闭注册。
```
## D04 首发自动化历史基线（2026-07-19，已被 2026-07-21 新口径覆盖）

本地实现已完成，代码入口为 `internal-test-service/`，Compose/Caddy 集成为 `infra/compose.yaml`、`infra/Caddyfile` 和 `infra/Dockerfile.internal-test`。

- 历史规则曾包含邀请码、签到和推荐奖励；它们已被“可配置公开注册、15 人硬上限、注册/每日首次登录 `$20`、首发期间无推荐”覆盖。本节只保留迁移背景，不得用于后续实现或验收。
- 服务：Go 1.24 + SQLite WAL；JWT 通过 Sub2API `/api/v1/auth/me` 验证；Admin API 只用只读挂载的 `x-api-key` 文件；所有余额写入固定幂等键，超时后先查余额历史；余额漂移自动进入只读并告警。
- 新入口契约：Caddy 将原生 register/login/login-2fa 和 public-settings 交给独立服务；旧 `/internal-test/*` 不再路由，OAuth 建号路径仍返回 403。服务没有宿主机端口，根文件系统只读，默认 `D04_MODE=read_only`。
- 验证：`go test ./... -race`、`go vet ./...`、Docker 镜像构建、Compose/Caddy D04 契约、基础设施基线和既有 Ruby 回归均通过。真实 Admin API Key、飞书 Webhook、JWT、用户 Key 和生产余额均未触碰。

上述“尚未启用生产写入”是 2026-07-19 的历史门禁，已被 2026-07-22 单用户验收覆盖：一个隔离用户的一次 `$20`、同日幂等和三方对账已经通过，最终恢复只读和注册关闭。下一门禁是首发开放准备与单独批准；不签发邀请码、不启用推荐奖励，也不重复发放验收 grant。

## relay-ops 监控与预警 Agent 设计（2026-07-19）

设计文件：`docs/superpowers/specs/2026-07-19-relay-ops-monitoring-and-alert-agent-design.md`。

- 架构已确认：独立 `relay-ops` 服务与 Sub2API 并行，Caddy 提供 `/pricing`、`/performance`、`/ops` 路径；不 fork Sub2API，不直接写其 PostgreSQL。
- 监控范围：所有已启用且对客户公开的 Sub2API 分组；候选站由管理员录入名称、Base URL、低额度 Key、定价/倍率页和用量/账单页。
- 采集策略：生产质量由 Sub2API 原生监控负责，生产价格/倍率每 5 分钟，候选来源每 6 小时；只有明确启用 `probe` 模式的候选才发同步/SSE，生产来源不发探测请求。
- 告警机制：原始探测全量入库；事件状态机确认后才发飞书，采用状态变化、持续恶化、恢复、价格/倍率变化、候选综合变优和每日摘要，避免重复消息。
- Agent：只读、事件触发、输入为脱敏结构化 JSON，只能查询质量窗口/探测/价格/候选证据；不可读取秘密或执行路由、价格、余额、Key、数据库写操作。
- 真实账单：默认相信上游标注倍率和定价；能读取则作为辅助证据，不能读取时保留质量/价格监控并明确显示“真实倍率未验证”，不反复要求管理员授权。
- 页面：`/pricing` 无需登录；`/performance` 仅登录用户可见；`/ops` 为数据为空的 bootstrap 加 Sub2API 管理员令牌验证后的只读投影。缺失、无效、非管理员或停用管理员访问数据接口均按 404 隐藏。
- 历史状态：原生 Monitor 异常/恢复、日报、重复抑制、App Bot 投递、Agent 确定性回退和质量报告闭环已保留。候选录入是历史已验收能力；后续 `/ops` 历史版本已移除候选/Base URL/Key/账单会话/验收写界面与浏览器路由，新上游统一在 Sub2API 原生管理端维护。该节验收镜像为 `sub2api-native-ops-20260722-v1`，当时 probe 和质量报告均为 0，飞书 `enabled` 未批准；这些值不得当作当前生产状态。

详细运维与回滚见 `docs/runbooks/relay-ops-monitoring.md`；验证证据见 `docs/superpowers/reports/2026-07-20-relay-ops-production-source-monitoring-verification.md`。

## 11. 原生监控复核（2026-07-20）

- `relay-ops` 复用 Sub2API 原生 Channel Monitor、Accounts、Groups、Ops 和 Usage 数据；`/ops` 每 30 秒重新读取管理员投影。候选后端历史代码/记录保留，但浏览器不再提供候选录入或付费 probe 控件。
- Neko 已创建独立低额监控 Key（总上限 `$0.02`，仅在浏览器内转移），原生 `/admin/channels/monitor` 的 `GPT-Pro / Neko` 主模型为 `gpt-5.6-sol`、间隔 `3600` 秒、抖动 `120` 秒，手动样本正常、7 天 `100%`、`1396 ms`。
- Wawazz 原 `relay-monitor` Key 已替换；补余额后监控恢复 `operational`。账号级 `User-Agent: node` 已解决 Python 客户端 403，最终 Python 同步/SSE 与 `0.0502x` 计费通过。Neko 监控当前因 `INSUFFICIENT_BALANCE` 为 `error`。
- relay-ops `/ops` 已显示 `GPT-Plus`、`GPT-Pro`；monitor 历史仍由 Sub2API 原生保存，relay-ops 只记录引用和窗口摘要，不复制 Key 或原始历史。
- 本次真实生产复验使用 `https://api.43-133-75-82.sslip.io`（`v0.1.161`）；本机 `localhost:8080` 为旧 `v0.1.153` 开发实例，误建的隔离用户已删除，不纳入生产证据。
- 本轮验证：Wawazz 手动监控、UA 前后对照、Python 同步/SSE、`[DONE]`、同请求计费、临时对象清理、生产账号不变量和容器 ID 均通过；追加式台账已记录。未持久化任何 Key 原文。Neko 余额不足仅作为历史/配置事实保留，用户已明确不处理，不再是当前阻塞。

## 12. 飞书确定性分组控制（2026-07-20）

- 允许命令固定为五条：切换/恢复 `GPT-Pro`、切换/恢复 `GPT-Plus`、查询当前分组状态。只做逐字匹配，不使用 LLM、模糊意图、动态参数、shell 或通用 Admin API 代理。
- 回调只公开精确 `POST /relay-ops/api/feishu/events`；私聊、bot/app/system、非文本和未知协议事件不能执行。事件以 `event_id` 去重，worker 使用租约和按序获取的 PostgreSQL 分组/账号 advisory lock。
- Sub2API 控制固定到 `v0.1.161` 原生接口：预检分组/账号/模型，先加入并复读目标，再移除并复读源；不发送 `confirm_mixed_channel_risk`，不自动回滚未知写入，源删除不确定时返回 `partial`。
- 审计只允许五条命令、两种动作、两个分组、小写稳定错误码和脱敏路由状态；短 ID 使用进程内随机密钥 HMAC。飞书 tenant token 缓存、401 单次刷新、1 MiB 响应上限和最多三次回复均有测试。
- 本地验证通过：Go 全量 `-race`、`go vet`、隔离 PostgreSQL 全包、relay-ops 镜像构建、Compose 配置、Caddy 解析和部署契约。运维入口为 `docs/runbooks/feishu-command-control.md`。
- 生产门禁：当前镜像以 `dry_run` 运行；五个飞书文件只读挂载，Caddy 仅公开精确 `POST /relay-ops/api/feishu/events`。真实群两条切换命令为零写入 `succeeded` 预测，两条恢复为 `no_op`，查询确认两组仍为 `primary`；未知命令固定拒绝，私聊权限未额外开启且自动化测试覆盖拒绝边界。Sub2API 前后证据文件 SHA-256 均为 `3a3f2abd72e64fd088d31b20971794762152e4bff814ba23e08847975571f8ef`，规范化 canonical SHA-256 均为 `225777ef5a2f73b9bcbe276a43a52129a335c894c37dfb269d26c64fec5f18ff`，relay-ops 健康且重启计数为 0。`enabled` 仍需新的、指定目标分组的单独批准。证据见 `docs/superpowers/reports/2026-07-20-feishu-production-dry-run-verification.md`。
- 共享 Aliu 更新：Pro `7 -> 2`、Plus `8 -> 2` 已写入服务器 `0640` 路由文件，新镜像 `feishu-shared-aliu-20260720-v1` 健康且重启计数为 0；账号 `9` 保留、未绑定、不可调度。前后生产快照 SHA-256 均为 `bb12c7da55fbee4d05746bd2e8ed5d10e56c5b8b85e226e3579f7c25689e6275`。截至该次验收，群内机器人尚未正式邀请，因此当时没有发送上线后的群命令；该状态已被 7 月 21 日真实群主动告警验收覆盖。证据见 `docs/superpowers/reports/2026-07-20-feishu-shared-aliu-dry-run-verification.md`。

## 13. 飞书主动告警闭环（2026-07-21）

- 现有飞书 App Bot 已接通 Sub2API 原生 Monitor 事件、P1 两窗口确认、重复/新证据抑制、一次恢复通知和上海日运营摘要；不新增 Webhook 机器人。
- 日报动态覆盖全部客户公开分组，包含 24h SLA、错误、TTFT P95、总延迟 P95、本站费用、上游成本、候选数和事件数；稳定键为 `daily-report:YYYY-MM-DD`。
- 只读 Agent 只接收 `relay-ops-incident-v1` 脱敏结构化输入，无工具和生产写权限。生产未安装真实模型凭据，因此使用已验收的确定性回退，飞书投递不受影响。
- 真实群验收得到一次异常、一次恢复和一次日报；重复调用未增加投递。最终镜像 `feishu-proactive-alert-20260721-v3` 健康、重启计数 `0`，四个基础容器未变，路由 canonical SHA-256 始终为 `0346b79d19cffdca58898e6db6490d62df89b1f0d889cc9fbaa22946b1163433`。
- 证据见 `docs/superpowers/reports/2026-07-21-feishu-proactive-alert-production-verification.md`。用户已确认 Wawazz 高负载为预期状态，Neko 余额不处理；两者均不影响本闭环的完成判定。

## 14. 飞书专业卡片生产跟进（2026-07-21）

- 专业卡片代码已包含在生产镜像 `feishu-proactive-alert-20260721-v3` 中；生产模式仍为 `read_only + dry_run`。
- 真实飞书群内的每日运营摘要已确认为 Interactive Card，包含结构化字段和“运维后台”按钮。
- 2026-07-21 02:32 发送一条只读命令 `查询当前分组状态`，机器人以 Interactive Card 返回 `succeeded`，并明确显示 `dry-run，仅预测，未写入路由`。
- 已有合成告警/恢复消息发送于旧文本版本。当前告警/恢复模板、30 KB 门禁和 App Bot `interactive` wire payload 已通过自动化测试；真实视觉验收等待下一次自然事件，不通过清理去重数据或制造生产故障强制触发。
- 收口审计已将该视觉证据明确降为非阻塞观察项：原生告警、恢复、日报、命令卡、去重、App Bot 传输和确定性只读分析均有自动化与生产证据，飞书功能主线已完成关闭。最新生产镜像 `candidate-admin-intake-20260721-v2` 健康、重启计数 `0`，五个公网入口均为 HTTP 200，模式仍为 `read_only + dry_run`；本次未重建服务、发送合成事件或修改生产状态。
- 证据见 `docs/superpowers/reports/2026-07-21-feishu-professional-card-production-verification.md`。

## 15. 候选上游管理员录入（2026-07-21）

> 历史能力，已被 2026-07-22 的 Sub2API 原生账户管理方案覆盖。以下只保留安全验收事实；当前 `/ops` 不再展示或接受候选、Base URL 或 Key，相关浏览器写路由返回 404。

- `/ops` 已提供候选名称、Base URL、定价页、用量页、可选性能页和独立低额度 Key 的六字段表单，并提供停用控制。
- Key 通过 `FileSecretStore` 安装到 `/var/lib/relay-ops/candidate-keys`，文件名由候选名称 SHA-256 派生，目录/文件权限分别为 `0700/0600`；数据库不保存 Key 原文或托管路径之外的秘密。
- 生产镜像 `candidate-admin-intake-20260721-v2` 健康、重启计数 `0`；只有托管候选 Key 目录是新可写挂载，既有秘密和根文件系统仍只读。
- 安全复审后的 `v2` 增加 `candidate_probe_key` 指纹部分唯一索引，并将数据库内部失败映射为稳定 500、清理失败包装改为不泄露原错误、变更接口 Content-Type 限定为精确 `application/json`。启动迁移索引存在，候选/秘密引用/创建审计计数仍为 `0 0 0`。
- 认证浏览器使用无效私网 URL 与非秘密 Key 验证失败回滚：Key 输入框清空，候选列表、托管目录、候选记录、秘密引用和创建审计均为零。
- 前后 Sub2API 路由 canonical 哈希和飞书路由文件哈希一致，四个基础容器 ID 未变。真实候选尚未录入，付费 probe 仍需单独批准。
- 证据见 `docs/superpowers/reports/2026-07-21-candidate-upstream-admin-intake-production-verification.md`。

## 16. 三条主线收口（2026-07-22）

- 质量优先上游循环已经在本地账号 `73/74/75` 完成验收：`73/74` 因全目录同步/SSE 失败被硬门禁阻断；`75` 全目录 `18/18`、容量 `129/129`，但价格、provider billing、余额和商业条款未知，只能 `needs_evidence`。所有临时 Key/绑定已清理，没有创建生产候选或改路由。
- 质量报告闭环已部署到生产 relay-ops：fast result 入库后生成通知-only 飞书卡片，等价报告按语义摘要去重；真实 PostgreSQL 允许 `failed` 投递原位重试但继续抑制 `delivered/reserved`。生产验收期间不发消息、不创建事件、不启用 probe。
- D04 单用户低额验收已经通过：provider user `17` 只有 1 条成功的 `$20` daily-login grant 和 1 条匹配 balance history，同日再次登录没有第二次 effect，usage 为零；最终 `read_only/registration=false`。
- 用户批准后的最新只读复核（2026-07-22 04:09Z）确认 D04 与 relay-ops 均 `healthy/restarts=0/OOM=false`，D04 镜像仍为 `d04-public-registration-20260721-v1`，模式为 `read_only + registration=false`，relay-ops 为 `read_only + dry_run`；`healthz/readyz/ops=200`，同源空注册 `403`。同一快照的 v2 评估仍为 `NO-GO`，仅阻塞于 `upstream_balance_below_minimum` 和 `upstream_samples_insufficient`，未应用 overlay 或制造流量。下一主线仍是自然证据达标后重跑门禁，不是重复部署或扩展飞书功能。
- 2026-07-22 04:11Z 复核仍未改变门禁结果：同一快照 `NO-GO`，阻塞仍为活动上游余额不足和自然样本不足；质量证据接近新鲜窗口上限。继续保持 D04 `read_only/registration=false` 与 relay-ops `read_only/dry_run`，等待真实外部状态变化后再生成新快照。
- 2026-07-22 04:25Z 已用只读页面重新取得新鲜财务/自然质量证据：活动上游余额 `-$0.01`，本站近 1 小时自然请求 `0`，TTFT P95/总耗时 P95 无样本。新快照 `D04-LIGHTWEIGHT-LAUNCH-20260722T042507Z` 的财务/质量证据年龄约 `1.43`/`3.40` 分钟，仍只返回 `upstream_balance_below_minimum` 与 `upstream_samples_insufficient`。这是连续第三个目标回合的同一外部阻塞；当前停止重复轮询，待余额和自然流量真实变化后恢复，不制造样本、不充值、不改路由或部署。
- D04 验收期间的空 settings PUT 事故已通过官方 Admin API 完整恢复，最终设置哈希为 `52eff24fce0338ee4f8f81ad12a5d1406c46b6de050c99587035cdfd1f71a28e`。后续方法发现必须先读官方 handler/DTO，不得用空对象写请求探测。
- 飞书 Agent/告警/恢复/日报/Interactive Card/命令卡/去重/确定性只读分析保持主线完成态；只等待自然事件的告警/恢复卡视觉证据，不触发合成事件。
- 权威收口报告：`docs/superpowers/reports/2026-07-22-three-mainlines-closure-verification.md`；质量报告生产证据见 `docs/superpowers/reports/2026-07-22-quality-report-feishu-production-verification.md`。三条历史工作线已按顺序收口；D04 当前只按 provider-neutral v3 动态账号集合门禁推进，不再扩展飞书功能，不自动切换上游。

## 17. D04 轻量开放门禁 v2（历史，已被 v3 覆盖）

- `D04-LIGHTWEIGHT-LAUNCH-v2` 曾替代 v1；它的单一手工 `active_upstream` 已被 v3 动态账号集合覆盖，只保留历史证据。
- 服务器已安装 `/opt/sub2api/production/ops/backup-d04-account-data.sh`，备份集 `20260722T015202Z` 完整包含 PostgreSQL 自定义格式归档和 D04 SQLite 一致性快照；目录/文件 `0700/0600`，SHA-256、`pg_restore --list` 和 SQLite integrity 均通过。
- 新 AMD64 镜像 `sub2api-internal-test:d04-lightweight-launch-20260722-v2` 已构建，image ID 为 `sha256:89bd28421f8002091f7d5411ae6da92d058f767db625e77bc65ce958c759a290`，但没有部署或重建运行 D04。生产继续 `read_only/registration=false`，relay-ops 继续 `read_only/dry_run`。
- 02:28 离线复评为 `NO-GO`：`launch_not_approved`、`upstream_balance_below_minimum`、`upstream_financial_evidence_stale`、`upstream_quality_metrics_stale`、`upstream_samples_insufficient`。质量证据过期是时间自然流逝导致；15 分钟窗口没有自然样本，不人为制造流量。
- 用户随后批准实际受控开放；同一尝试已在服务器本地刷新账户备份至 `20260722T033408Z`。最新无秘密快照 `D04-LIGHTWEIGHT-LAUNCH-20260722T035301Z` 记录 `launch_approved: true`、活动上游余额 `-$0.01`、自然样本 `0`，财务/质量证据年龄约 `0.82`/`1.65` 分钟，评估结果仍为 `NO-GO`，仅阻塞于 `upstream_balance_below_minimum`、`upstream_samples_insufficient`。生产运维面板于 `03:52:11Z` 刷新后近 1 小时请求、Token、错误均为 `0`；负余额已纳入评估器回归测试，评估未执行真实动作或联系外部系统。
- 历史入口：`docs/superpowers/reports/2026-07-22-d04-lightweight-launch-gate-verification.md`。不得再用 v2 结果授权开放。

## D04 v3 恢复入口（2026-07-22）

**Goal:** 等待 Sub2API 动态发现的全部活动账号具备可信最低余额和足够新鲜自然质量样本，再生成 v3 同快照 `GO` 并准备受控开放。

**Context:** 当前 Sub2API 动态账户投影为 `10/11/12/13`，账号质量集合哈希为 `f6b733f89e799048c92d90dc0d404ce1f96300bf1f2964184cc681bdcc2457e7`；旧 `7/8` 与固定集合结果不再加入。生产 `/ops` 显示只读双区监测且无表单、输入、按钮或秘密字段，relay-ops 镜像 `sub2api-relay-ops:ops-dual-domain-20260723-v1` 健康、重启 `0`。最新质量结果已新鲜发布，但当前成功率和 TTFT 不满足开放门禁。

**Constraints:** 当前活动上游只等于 Sub2API `status=active && schedulable=true` 账号集合。不得硬编码供应商或账号 ID；每个发现账号独立通过余额和账户归属自然质量门禁。不得按供应商名称设例外，不得创建候选、启用 probe、制造流量/飞书事件、改路由/倍率/价格/余额/Key/数据库、进入 `enabled` 或打开 D04 注册。

**Plan:** 不再开发第二套上游管理。复用已部署的 systemd timer、Sub2API 原生账户和余额能力，为真实动态集合补齐轻量 D04 证据。所有后续功能先检查 Sub2API 并明确“直接复用、改造复用或新增”。

**Done when:** 同一新鲜 v3 快照中所有动态活动账号均通过最低余额、样本、成功率、错误率、TTFT P95 和总耗时 P95 门槛并返回 `GO`；只有此时才进入 D04 受控开放。飞书功能继续关闭主线。

## 18. Sub2API 原生 `/ops` 收口（2026-07-22，历史基线）

- `/ops` 已从重复的上游/候选管理页简化为只读内测状态页。Base URL、Key、Cookie/Bearer、候选、生产来源、验收、合成事件和质量预览写控件均已移除。
- 活动上游定义只取 Sub2API 未删除且 `status=active && schedulable=true` 的账户。该节验收时为 `10/11/12`；本轮 SSH 重新发现的实时集合为 `10/11/12/13`，软删除历史账号不参与集合，后续仍须动态读取而不是写死。
- 有效管理员看到当前账号、公开分组、只读报告、事件和 Agent 摘要；匿名或无效令牌访问投影返回 404，公开 `/ops` 只返回 477 字节无业务数据 bootstrap。
- 该节生产镜像曾为 `sub2api-relay-ops:sub2api-native-ops-20260722-v1`，随后由 `account-quality-monitor-v1` 替换；当前镜像为 `ops-dual-domain-20260723-v1`，模式为 `read_only + dry_run`，D04 为 `read_only/registration=false`。
- 权威证据：`docs/superpowers/reports/2026-07-22-sub2api-native-ops-simplification-verification.md`。

## 19. GPT/Codex 缓存让利只读基线（2026-07-22）

- 商业口径：缓存相对普通输入的价格差额全部让利，用户组倍率保持不变，上游账号采购折扣仍作为站点经营空间。
- Sub2API `v0.1.161` 继续是请求转发、四类 Token 计费、余额、用户用量页、`prompt_cache_key` 和粘性会话的唯一权威；不维护私有网关分支或第二套用户账单。
- relay-ops 新增只读缓存字段存在性、命中率和公开价格门禁；日报只展示聚合证据和脱敏门禁代码，不读取提示词、响应或原始缓存键，也不自动改价。
- 本地聚焦测试、全量测试、全量 race、vet、Compose 和差异检查已通过。没有部署、模型请求、价格/路由/倍率/余额/Key/用户/数据库写入。
- 恢复入口：等待 `GPT-Pro`、`GPT-Plus` 各至少 24 小时且 20 个成功自然请求，随后逐笔核对用户扣费、Sub2API Token 分类和上游账单；成功率下降须不超过 0.5 个百分点，TTFT P95 上升须不超过 10%。此前不得宣称生产让利验收完成。
- 权威证据：`docs/superpowers/reports/2026-07-22-gpt-codex-cache-savings-verification.md`。
