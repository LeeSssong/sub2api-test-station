# 项目当前状态

**更新日期：** 2026-08-05
**权威计划：** `docs/superpowers/plans/2026-08-05-six-stage-production-closure-deployment-units.md`
**项目全局进度总账：** [docs/project/project-progress.md](project-progress.md)（以该总账的生产部署、验证和用户验收口径为准）

## 当前指针

> 2026-07-30 起，D04 内测服务及 relay-ops 自建运维控制面均已退役。账号监控唯一管理员入口是 `/admin/accounts/monitor`；旧 `/ops` 路径仍重定向到原生 `/admin/ops`。`relay-ops` 不暴露独立 UI，只保留内部定时采集、对账、日结、价格采集、受保护数据接口和飞书出站。

- 当前阶段：`L1-2` 至 `L1-9` 的离线准备、`M0` 主机基线和 `M1` 核心站点部署均已有历史成果；其中离线成果属于“准备完成”，只有部署并验证生效的站点能力计入生产完成。Sub2API、PostgreSQL、Redis、Caddy 和只读 relay-ops 是当前运行边界。总账将状态区分为“生产工程代码/配置已部署并验证、工程代码/配置差异待部署、持续实施、运维/研究跟进”；文档、研究、历史证据和外部验收不单独构成工程未部署。
- 历史准备完成：项目机制、MVP 边界、首版技术栈、D01、D13、最终采购建议、核心网关本地基线、UP01 填报、精确定价映射、人工充值/余额/用量对账、ACC01 候选评估、PAY01 支付模拟、ROUTE01 路由韧性，以及 OPS01 日常检查、止损和 BKP01 备份恢复离线基线。生产完成数量和当前进行中事项以项目全局进度总账为准。
- 当前操作：OPS01 固定为 `report_only`，BKP01 固定为 `dry_run_only`；PAY01 保持支付关闭。当前活动上游发现规则是 Sub2API 原生 Admin 账户列表中未删除且 `status=active && schedulable=true` 的成员；账号质量、评分、服务健康只由 `/admin/accounts/monitor` 承载，营收、账务与对账只进入后续独立 `/admin/revenue`。relay-ops 仅作为内部后台服务运行，不拥有浏览器控制面。
- 当前风险：上游余额、错误率、TTFT P95 和总耗时 P95 仍是生产风险，但不再作为注册门禁。付费 probe 和模型发布继续关闭。飞书只允许出站告警、持续提醒、恢复和日报；入站回调、命令控制与确认接手均已退役。
- 当前生产运行：账号监控 V3 的 Sub2API 来源提交为 `05985e62ec88b04d1e647a815eecdb1cf1155776`，源码树为 `c37b383bf54e485d7393ff0793e30dd03f5e2328`，活动槽为 green，运行镜像为 `sha256:0d10260b745e2086326977303b15f6eb78e8e03de7858fe356dec046bf0e10e8`；评分迁移、API/UI 和状态/单卡刷新均已验收。当前记录的 relay-ops 生产身份为 `release-48244833b` / `xingqiao-relay-ops@sha256:c88f58e4f9cbee2338dc6b607fa3e1f4f54fa8adbb32790f29411d3a5f224c66`；PostgreSQL、Redis、Caddy 在账号监控发布中未重建。
- 18:49 供应商页面复核覆盖上述早前样本：Wawazz 余额约 `$9.62`，累计 `5,996` 请求、`977.6M` Token、实际 `$51.3664`、平均响应 `14.56s`。GPT-Plus 状态页虽标“正常”，7 天可用性仅 `94.70%`，近 60 次包含多次约 `30s` 错误和降级；GPT-Pro 显示 `100.00%`。用户已确认高负载为预期业务，但余额、错误率、TTFT P95 和总耗时 P95 仍是生产风险。
- Git 基线：`codex/production-baseline-convergence` 仅收纳 `origin/codex/account-monitor-completion@bbfe4a36d` 与 `origin/main@138d26efa` 的版本历史；在协调代理推送、受控合并前，它不是新的生产运行时来源。
- 六阶段剩余顺序：账务运行时部署、真实账单授权/映射与非零闭环、独立 `/admin/revenue`、Monitor/飞书闭环、OpenAI 实际响应模型展示均为进行中。每个生产部署或生产配置激活后必须停在“等待用户验收”，只有用户明确确认后才能进入下一个部署单元。不恢复 relay-ops 控制面或飞书入站写能力。

L1-9 详细计划：`docs/superpowers/plans/2026-07-15-operations-and-stop-loss-offline-baseline-plan.md`。  
最新验证：`.superpowers/sdd/2026-08-04-account-monitor-card-production-implementation-plan/production-verification.md`、`docs/superpowers/reports/2026-08-01-main-blue-green-production-verification.md`、`docs/superpowers/reports/2026-07-31-command-driven-blue-green-local-verification.md` 和 `docs/superpowers/reports/2026-07-30-native-ops-reminder-only-production-verification.md`。蓝绿设计：`docs/superpowers/specs/2026-07-31-command-driven-30-minute-blue-green-deployment-design.md`；生产运行手册：`docs/runbooks/sub2api-blue-green-production-deployment.md`。旧 relay-ops 运维页、飞书命令控制、D04 v1/v2 和旧账号集合只保留历史证据。

## 产品

- 目标：先用现有上游 API 跑通可售卖闭环，再接入 K12、Plus、Pro 等订阅账号池和其他上游中转 API。
- 首条闭环：境外服务器 → Sub2API/PostgreSQL/Redis/Caddy → 现有上游 API → 测试 Key → 请求日志和扣费 → 人工充值。
- 用户注册：网站注册与邀请码均遵循 Sub2API 原生设置。没有独立的内测人数上限、预算门禁、每日额度或注册代理。
- 当前不做：多节点高可用、自动支付、昂贵优化线路和复杂营销功能。

## 已确认决策

| 编号 | 结论 | 状态 |
|---|---|---|
| D01 | USD 20/月是硬上限而非默认支出；首选腾讯云国际站东京 2C2G USD 10.08/年，按已定义顺序回退，2 GiB 达阈值后再扩容 | 已确认 |
| D13 | 所有外部支出只做选型和报告，不执行真实付款；后续按“未购买/假定配置”继续准备 | 已确认 |
| D03 | 生产公开分组和账号关系从 Sub2API 原生能力读取，不按供应商名称定义当前上游 | 当前活动集合为未删除且 `active + schedulable` 的 `10/11/12/13`；未授权改绑或隐藏现有分组 |
| D04 | 历史内测自动化已退役 | 注册和邀请码由 Sub2API 原生设置直接控制；没有人数、预算或每日额度门禁 |
| D02 | 首发阶段先用 `xingqiaolab.top`，API 子域为 `api.xingqiaolab.top`；规模化商业化前再评估 `.com` | 已注册、已解析并启用 HTTPS |

2026-07-16 新事实覆盖 D01 的旧服务器选型：用户已亲自购买腾讯云中国站 Lighthouse 首尔节点，2 vCPU / 4 GiB / 60 GB SSD / 30 Mbps / 1536 GB 月流量，Ubuntu Server 24.04 LTS，1 年 CNY 199，自动续费关闭。

详细依据见 `docs/superpowers/decisions/2026-07-15-d01-server-budget-and-selection.md` 和 `docs/superpowers/decisions/2026-07-15-d13-deferred-external-spending.md`。

## 工程状态

- 计划技术栈：Ubuntu 24.04 LTS、Docker Compose、Sub2API、PostgreSQL、Redis、Caddy。
- 代码和基础设施配置：完整 Compose 基线、临时 Caddy-only bootstrap、环境变量生成器、契约测试和运行手册均已建立；生产主机已从 bootstrap 切换到完整四服务栈。
- Sub2API 候选准备：GitHub Actions 每 6 小时发现最新稳定官方 Release，在无秘密 Job 中三方合并、全量测试并构建 `linux/amd64` 合格镜像；受信任 Job 将不可变镜像发布到私有 GHCR、通过 forced SSH 在生产拉取和验证候选、以 compare-and-swap fast-forward 推进合格源码，并发送无固定下一步的飞书事实卡片。生产 staging 不调用更新 API、不修改 Compose、不操作数据库、不切换或重启运行容器；运行时升级仍只由管理员在现有后台确认。实现已完成本地契约验证，GitHub Environment 和生产 forced-command 安装将在首次真实 workflow 验收中激活。
- Sub2API 更新：官方 `v0.1.166`（commit `dc893dd0b8eab41df5be595ae9fcd1aa74a062b8`）已无文本冲突合入 Xingqiao 定制快照并通过后端、前端和部署契约验证；合格 `linux/amd64` 镜像 `xingqiao-sub2api:upstream-0.1.166` 已加载到生产，平台 manifest 为 `sha256:e146027d59ab96c40d9ef12eea3943446a22b32e7aba918b85313909efce4ccf`。生产仍运行合格 `0.1.165`，等待管理员手动点击更新；交付过程未调用更新 API、未修改生产 Compose、未重建容器。
- 数据存储：生产 PostgreSQL、Redis 和 Sub2API 命名卷已创建；管理员记录在受控应用重启后保持存在。2026-07-22 已对 `sub2api` 和 `relay_ops` 做服务器本地 `pg_dump -Fc` 并在隔离 PostgreSQL 18 中恢复，逐表行数哈希一致，临时资源已清理。D04 SQLite 备份仅保留为历史证据，不再属于当前备份或恢复范围。
- 生产主机：腾讯云首尔二区实例运行 Sub2API、PostgreSQL、Redis、Caddy 和 relay-ops；正式入口为 `https://api.xingqiaolab.top`。
- 外部服务配置事实：账号 `8` 对应上游的 Base URL 为 `https://wawazz.xyz`，绑定 `GPT-Plus`、成本倍率 `0.05x`、并发 `1`，账号级 `User-Agent` 固定为 `node`，原生监控和 Node/Python 同步/SSE 已通过。账号 `7` 对应上游的 OpenAI 兼容 Base URL 为 `https://api.999555999.com/v1`，绑定 `GPT-Pro`、成本倍率 `0.10x`、并发 `2`。隔离复制账号 `9` 保留但未绑定、不可调度；Aliu `2` 未绑定、不可调度、总并发 `1`，现作为 Pro/Plus 共享灾备。上述名称和地址只记录配置事实，不定义“当前活动上游”。
- 容量证据：Neko Pro 池短测同步并发 1–50 路全部 200，SSE 并发 3/5/10 全部完成，60/120/180 RPM 一分钟窗口全部 200；240 RPM 出现 1 次超时。生产主/备账号配置并发为 `2 + 1 = 3`，未超过已验证共享 Key 上限；详细报告见 `docs/superpowers/reports/2026-07-19-neko-capacity-verification.md`。
- 2026-07-19：直接核对 Sub2API `v0.1.161` 源码：原生支持注册、登录、2FA、用户中心、邀请码、管理员 API Key、用量/余额读取和强制幂等余额调整。当前只使用这些原生注册与邀请码能力。
- 2026-07-20：用户完成 `xingqiaolab.top` 注册；`api` A 记录已指向首尔实例公网地址，Caddy 已为 `api.xingqiaolab.top` 签发公开信任证书，HTTP 308、HTTPS `/health` 200、`/pricing` 200 和 `/ops` 200 均已复验。`.com` 仍保留为规模化商业化升级候选。
- 网关扩容研究：LiteLLM 和 New API 均采用同模型多 deployment/渠道、健康路由、失败重试和用户级限流；第二个 Neko Key 应作为第二个 Sub2API 账号对象加入同一逻辑组，不能假设容量线性翻倍。详见 `docs/superpowers/reports/2026-07-19-gateway-scaling-practices.md`。
- 生产凭据：未记录；密钥、Cookie、OAuth 凭据、2FA 恢复码和支付密钥禁止进入 Git 和普通文档。
- 首发用户自动化：`internal-test-service` 使用 Go 1.24、SQLite WAL 和独立 Admin API 客户端；只代理原生 register/login/login-2fa 和 public-settings，1 MiB/20 秒有界且不存储认证内容。有效注册开关是“Sub2API 原生开关 AND D04 模式/配置/15 人/预算门禁”；历史邀请/推荐表保留只读兼容，旧 join、邀请、推荐发奖和手动签到不再属于活动路径。容器契约为无宿主机端口、只读根文件系统、非 root、仅 `/var/lib/internal-test` 可写。当前生产运行 `sub2api-internal-test:d04-auth-client-identity-20260723-v2`，healthy/restart `0`，`D04_MODE=read_only` 且 `D04_REGISTRATION_OPEN=false`；历史低额验收已有 1 个隔离首发用户、1 条 `daily_login_credit` 成功 grant 和 1 条匹配的 `$20` provider balance history。详见 D04 验收报告。
- relay-ops：当前代码契约只保留 `/pricing`、健康检查、采集与出站通知；`/ops` 和 relay admin API 未挂载，Caddy 将所有旧 `/ops` 路径 `302` 到 Sub2API `/admin/ops`。日报与 15 分钟告警/持续提醒/恢复使用同一只读数据边界。飞书卡片只有指向 `/admin/ops` 的导航按钮，不能确认、接手或修改状态；App Bot 出站投递只需要 App ID、App Secret 和目标会话配置，不再需要 verification token、Encrypt Key、命令路由文件或入站 callback。**2026-07-30 原生运维重定向和现有飞书纯提醒边界已完成生产部署并验证生效**；独立的原生 P0/P1 告警事件桥接仍处于设计/实施阶段；历史 relay-ops 表、迁移与旧 model-release 证据保留，但不定义当前上游或活动监控任务。
- 2026-07-29：飞书通知收敛已完成本地实现，所有主动通知必须由服务端 JSON 策略显式启用；候选、质量报告、Usage Session 和 synthetic acceptance 不再接入生产 notifier。面向公开分组的用户影响合并为单一事故，提醒使用最新事实快照，定价变化和日报走独立 one-shot 生命周期，事故与 one-shot 统一由 retry worker 重试。该实现尚未部署，生产保持不变：生产镜像未更新、生产策略文件未安装，未制造飞书消息，也未写入路由、账号、价格、余额或 Key。
- 2026-07-22：GPT/Codex 缓存让利只读基线已实现。relay-ops 能区分缓存字段缺失与真实零值，验证公开 `gpt-*` 模型缓存读取价低于普通输入价，并在日报显示缓存读写、命中率和价格覆盖；Sub2API `v0.1.161` 继续负责四类 Token 计费、用户账单、`prompt_cache_key` 和粘性路由。本地全量测试、race、vet、Compose 和差异检查通过；未部署、未改价、未发请求，24 小时自然流量门禁仍待执行。证据见 `docs/superpowers/reports/2026-07-22-gpt-codex-cache-savings-verification.md`。
- 2026-07-20：生产 URL allowlist 仅追加 `wawazz.xyz`，只重建 Sub2API，其他容器未重建。GPT-Pro 隔离同步/SSE 均 HTTP 200、SSE 含 `[DONE]`；两条记录各 `10/5` Token，Sub2API 各扣 `$0.000200`，Neko 各实际 `$0.000020`，实测倍率 `0.10x`；测试用户/Key 已清理。Neko 原生监控样本为 `100%`、`1396 ms`。Wawazz 原有监控 Key 返回 `INVALID_API_KEY`，已删除并替换为新低额 Key；替换 Key 有效但上游账户余额不足，监控返回 `INSUFFICIENT_BALANCE`，当前 `/monitor` 为 `DEGRADED`。
- 2026-07-20：Wawazz 补余额后的原生监控连续恢复为 `operational`（`1754 ms`、`2293 ms`）。Node UA 下 GPT-Plus 同步/SSE 均 HTTP 200，SSE 含 `[DONE]`；两条 Sub2API 记录各扣 `$0.002410`，Wawazz 各实际 `$0.000121`，实测约 `0.0502x`。同请求改为 `Python-urllib` 或 `OpenAI/Python` UA 时上游返回 403 且零扣费，说明原生 Go 监控不能覆盖客户 UA 兼容性；临时用户/Key 已清理，路由和模式未修改。详见 `docs/superpowers/reports/2026-07-20-wawazz-balance-recovery-verification.md`。
- 2026-07-20：Wawazz Python 兼容修复完成。源码和同请求对照确认 Chat Completions 直转会透传客户 UA，Wawazz 拒绝 Python UA；账号 `8` 增加 `credentials.user_agent=node` 后，`Python-urllib` 同步 HTTP 200/`4869 ms`，`OpenAI/Python` SSE HTTP 200、TTFT `1942 ms`、总耗时 `1989 ms`、含 `[DONE]`。两条 Sub2API 各扣 `$0.002410`，Wawazz 各实际 `$0.000121`；所有临时对象删除，五个容器 ID 未变。一次中间请求因并行高负载持续 `69s`，测试客户端过早清理导致一条 `USER_NOT_FOUND` 用量写入错误；修正清理时序后的最终回归通过。
- 2026-07-20：Neko 原生监控最新 8 条均为 `INSUFFICIENT_BALANCE`，最近一条为 22:55、`570 ms`；该实时失败覆盖“余额已经恢复”的旧操作假设，但不否定此前 `0.10x` 同请求计费证据。未自动切换到 Aliu。
- 2026-07-20：飞书确定性分组控制已完成 `disabled -> dry_run` 生产验收。真实 Feishu challenge、事件收发和 `mentioned_type=bot` 解析通过；两条切换返回 dry-run `succeeded`，两条恢复返回 `no_op`，查询确认两组仍为 `primary`，未知命令固定拒绝。前后证据文件 SHA-256 均为 `3a3f2abd72e64fd088d31b20971794762152e4bff814ba23e08847975571f8ef`，规范化 canonical SHA-256 均为 `225777ef5a2f73b9bcbe276a43a52129a335c894c37dfb269d26c64fec5f18ff`；最终基础容器 ID 为 `sub2api=5fd8adccdb9e`、`postgres=2db52788ad73`、`redis=c45202c0d9e6`、`caddy=7c28088cd9fe`，relay-ops 健康且重启计数为 0。生产停在 `dry_run`，未进入 `enabled`；私聊权限未额外开启，自动化测试覆盖其拒绝边界。详见 `docs/superpowers/reports/2026-07-20-feishu-production-dry-run-verification.md`。
- 2026-07-20：Aliu `2` 已改为 Pro/Plus 共享灾备；配置只允许 backup 复用，主账号唯一且不能兼任灾备。worker 切换同时锁定分组、主账号和灾备账号，真实 PostgreSQL 并发测试通过。仅重建 relay-ops，四个基础容器 ID 未变；新容器 `1e7194b56bb8` 健康、重启计数 `0`。Sub2API 前后快照 SHA-256 均为 `bb12c7da55fbee4d05746bd2e8ed5d10e56c5b8b85e226e3579f7c25689e6275`。群邀请和上线后的群内 dry-run 命令按用户要求暂缓。详见 `docs/superpowers/reports/2026-07-20-feishu-shared-aliu-dry-run-verification.md`。
- 2026-07-21：飞书主动通知闭环完成。原生 Monitor P1 两窗口确认、重复/新证据抑制、恢复通知、所有客户公开分组的 24h 日报和只读 Agent 确定性回退已通过测试与真实群验收。最终 relay-ops 容器 `866dbae4eb75` 健康、重启计数 `0`；四个基础容器未变，前/后/最终路由快照 canonical SHA-256 均为 `0346b79d19cffdca58898e6db6490d62df89b1f0d889cc9fbaa22946b1163433`。数据库保留日报 `1` 条、合成异常/恢复 `2` 条成功投递，重复调用未增加投递。详见 `docs/superpowers/reports/2026-07-21-feishu-proactive-alert-production-verification.md`。
- 2026-07-21：Wawazz 高负载仍在持续。与前次 `2251` 请求、`370.53M` Token、实际 `$16.89` 相比，02:26 样本增长至 `3458`、`579.22M`、`$25.3222`，分别增加约 `53.6%`、`56.3%`、`49.9%`；`gpt-5.6-sol` 占 `3362/3458` 请求和 `569.73M/579.22M` Token。页面未提供足够错误聚合证据，不能把高吞吐直接等同于稳定容量。
- 2026-07-21：用户确认 Wawazz 上述高负载为预期业务状态，不再追查 `test` Key 用途；但高负载不等于 SLA 或容量已验证，余额、错误率、TTFT P95 和总耗时 P95 仍按生产风险持续监控。Neko 余额明确不处理。
- 2026-07-21：飞书专业卡片与运维消息主线完成关闭。日报卡片在群内显示结构化分区和“运维后台”按钮；02:32 发送唯一一条 `查询当前分组状态` 后，机器人返回 Interactive Card，结果为 `succeeded` 并明确 `dry-run，仅预测，未写入路由`。告警/恢复模板、30 KB 门禁、脱敏、官方稳定卡片元素和共用 App Bot `interactive` 传输已通过自动化验证；其下一次真实视觉证据等待自然事件，属于非阻塞观察项。收口复核时生产镜像 `candidate-admin-intake-20260721-v2` 健康、重启计数 `0`，五个公网入口均为 HTTP 200，模式仍为 `read_only + dry_run`，飞书路由哈希仍为 `3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e`，未重建服务或制造合成事件。详见 `docs/superpowers/reports/2026-07-21-feishu-professional-card-production-verification.md`。
- 2026-07-21：候选上游管理员录入曾完成部署与安全验收；该能力现已被 2026-07-22 的 Sub2API 原生能力复用方案覆盖。历史记录和表保留，但 `/ops` 不再展示或接受候选/Base URL/Key，相关浏览器写路由返回 404；新上游统一在 Sub2API 原生账户界面管理。
- 2026-07-21：D04 注册安全/恢复增量完成。邀请码由 AES-256-GCM 认证加密，`join_id` 作为附加认证数据；配置要求独立权限受限密钥文件，未提供明文兼容回退。新增错误 method 405、余额历史分页、pending grant provider 证据收敛。`go test ./... -count=1`、单独 `-race`、`go vet ./...`、D04 Compose/Caddy 契约和 `git diff --check` 均通过。生产独立 D04 镜像 `sub2api-internal-test:d04-read-only-20260721-v3`（AMD64 manifest `sha256:3b52f06d3ca6cd2d0cf256bbf1e21463a2f7516f3b97ee307d5aba1fc8395dbc`），模式仍 `read_only` 且成本策略未认证；调度健康、重启 `0`、`audit_events/credit_grants/internal_users/invitations/jobs/usage_cursors/usage_records` 全为 `0`。正式域名 Caddy 已重新挂载 D04 路由，GET `/internal-test/api/checkin` 返回 `405`，加入链接不存在返回 D04 `404`，注册 POST 在只读模式返回 `403`。详见 `docs/superpowers/reports/2026-07-21-d04-controlled-launch-v2-verification.md`。
- 2026-07-21：D04 非功能第一阶段基线完成但保留容量边界。TLS/HTTP、入口 GET 健康、登录/注册页面可达、资源和容器健康均通过；未发起模型请求、上游费用为 `0`。对先前缺完整分母的 TLS 信号补做 5 路并发 × 12 次复测，`60/60` 为 HTTP 200、其他状态 `0`、传输错误 `0`，P50 `1.779s`、P95 `4.779s`。该结果只代表单地点入口健康，不代表模型容量或用户 SLA；XM PLUS/PRO 付费兼容与并发阶梯仍等待凭据、模型映射、预算和清理批准。详见 `docs/superpowers/reports/2026-07-21-d04-nonfunctional-baseline.md`。
- 2026-07-21：D04 新业务口径已完成本地实现与验证。Caddy 契约接管原生 register/login/login-2fa/public-settings；有效注册开关为原生开关与 D04 模式、配置、15 人硬上限、预算门禁的合取。20 路并发注册只有 15 个成功，12 路同日登录只有 1 次余额效果；注册自动登录立即获得当天 `$20`，上海次日可再次获得。旧 join/邀请/手动签到入口 404，usage 不再触发推荐奖励，D04 日报不再显示签到或推荐。随后曾短暂应用隔离 write overlay，但未注册用户、未写入余额，已用 `compose.d04-read-only.yaml` 重建回滚；当前生产 `read_only/registration=false`。详见 `docs/superpowers/reports/2026-07-21-d04-public-registration-daily-login-verification.md`。
- 2026-07-21：D04 独立审查缺口已完成本地修复并复验：SQLite 持久化 registration slot 在原生注册转发前占位，两个 service/store 实例只允许一个剩余名额进入上游；uncertain daily grant 即使系统已进入只读也能用 provider history 收敛；认证代理不跟随重定向，POST 强制同源，余额工作最多等待 2 秒；`D04_MAX_USERS` 限定 `1..15`，write 模式必须明确总预算。生产未部署，模式仍保持旧 `read_only`。
- 2026-07-21：D04 新版已以 `sub2api-internal-test:d04-public-registration-20260721-v1` 部署到生产，保持 `D04_MODE=read_only` 与 `D04_REGISTRATION_OPEN=false`，健康且重启计数 `0`。正式域名下五个公网入口均为 HTTP 200，公开设置强制关闭注册/邀请/affiliate，旧 `/internal-test/*` 显式 404，无 Origin 认证请求 403，同源空注册请求 403 `D04_REGISTRATION_CLOSED`。Sub2API、PostgreSQL、Redis 和 relay-ops 未重建；Caddy 因文件级 bind mount 重新挂载一次，最终重启计数 `0`。未创建用户、未发放余额、未改路由。
- 2026-07-21：XM PLUS/PRO 完成供应商页面只读发现并加入 registry，状态均为 `discovered`。页面确认共用 Base URL `https://api3.xmhbao.cn`、标称倍率 `0.045x/0.07x`，客户端示例使用 `responses` 且默认 `gpt-5.5`；Pro 有 1 条既有同步历史样本。公共评测器已增加 Chat Completions/Responses 强类型适配器、可配置安全路径、usage 归一化、SSE 终止和脱敏错误分类；XM Plus/Pro 对同一 Responses profile 的 dry-run 均为 `requests_sent=0/network_sent=false`。完整目录、逐模型同步/SSE、价格、容量、条款和三方账单仍未知。未复制 Key、创建候选、发付费请求或生成 proposal。详见 `docs/superpowers/reports/2026-07-21-upstream-benchmark-protocol-adapters-verification.md`。
- 2026-07-21：公共协议适配器的独立审查缺口已本地修复：同步与 SSE 总响应上限为 1 MiB，CRLF/LF 事件均可解析，URL 解码后仍含 traversal 的路径被拒绝，上游错误 `type/code` 只映射到固定类别，不再进入报告；V2 `validate` 输出实际解析的 profile ID。Chat、Responses 和 V2 全套 Ruby 回归以及 XM Plus/Pro 零请求 dry-run 均通过。
- 2026-07-21 22:13：规格通过后的功能/非功能收口复核完成。D04 全量 Go race、`go vet`、Compose/Caddy 契约、基础设施契约、Ruby benchmark/registry 和 `git diff --check` fresh 通过；生产 D04 镜像仍为 `d04-public-registration-20260721-v1`，healthy、restart `0`、OOM false，Caddy restart `0`，五个公网入口 HTTP 200。新版入口 `GET /healthz` 的 5 路 x 12 次样本为 `60/60` HTTP 200、P50 `1.747s`、P95 `2.182s`，但响应属于总站/Caddy HTML，不能作为 D04 handler 或模型容量证据。XM live 评测改为先各 1 次 `/models` 目录发现，再按文本模型数 `M` 审批；当时记录的 V2 `2M+42/2M+41` 已由随后获批的 v3 独立 sync/SSE 预算 `2M+71+K / 2M+70+K` 取代，当前仍为零 live 请求。
- 2026-07-21 22:20：XM 已登录 Key 管理页只读确认支持按分组创建 Key，并配置美元额度、速率、有效期、IP 限制和启用/禁用状态。后续 Plus/Pro 评测应各用独立低额临时 Key，不复用现有 Key。现有两把 Key 只观察到活跃、永久有效的脱敏行；未读取或复制 Key 原文，未创建、编辑、禁用或提交任何表单。
- 2026-07-21：公共非功能 v3 规格获批并完成离线实现。新 profile 分离 sync/SSE 并发阶梯与 RPM，样本绑定渠道、角色、分组、账号证据引用、模型、profile 和测量位置；共享 backup 通过通用 `shared_capacity_pool` 表达，禁止主账号复用或用聚合指标掩盖成员缺证据。默认阶梯的精确短测上界为 HTTP `2M+71+K`、生成 `2M+70+K`；旧 V2 的 `2M+42/2M+41` 只描述旧同步容量流程。`capacity-dry-run` 实测 `M=3/K=4` 得到 `81/80` 且 `requests_sent=0/network_sent=false`。本轮没有 live 请求、Key、候选、probe、部署或生产写入；目标拓扑仍为 `NOT_READY / NONFUNCTIONAL_EVIDENCE_INCOMPLETE`。详见 `docs/superpowers/reports/2026-07-21-upstream-sse-capacity-and-topology-nonfunctional-verification.md`。
- 2026-07-21：上述离线实现收口时只读复核生产：relay-ops 与 D04 均 running/healthy、重启 `0`；模式分别为 `read_only + dry_run` 与 `read_only + registration false`；五个公网入口均为 HTTP 200。没有重建服务、发送合成事件、调用 Admin API 或修改生产状态。
- 2026-07-21 23:34：独立审查发现 v3 初版对共享池身份/三阶段并发、观察窗口健康度、drill 读后写与 sync/SSE 证据、执行预算和 RPM 启动节奏的 P0 门禁不足；已按 TDD 加固，v3 回归增至 `31 runs / 142 assertions`，V2/协议/V1 仍为 `32/194`、`10/44`、`18/63`。Stage 0 新鲜只读复核通过：五个公网入口 200，模式仍为 D04 `read_only/registration=false` 与 relay-ops `read_only/dry_run`；新鲜 Admin API allowlist canonical 与保存基线均为 `b2a2a6ce...aaf82`，飞书路由哈希仍为 `3262403a...f7435e`。Sub2API healthy、restart `1`、OOM false，其余容器 restart `0`、OOM false。未发送模型请求、未改路由或生产数据；下一门禁仍是两项分别授权的 D04 单用户 write 验收与 XM 两次目录发现。
- 2026-07-21：授权前门禁进一步固化。新增 secret-free `compose.d04-acceptance.yaml`，与只读生产基线叠加后固定 `write/registration=true/15 users/$20/$2/1000 BPS/qualified policy`，Compose 和部署契约通过，但未应用到生产。D04 公开设置 fresh 显示注册/邀请/affiliate 均关闭，同源空注册返回 `D04_REGISTRATION_CLOSED`，内部调度健康。XM Plus/Pro 两条目录 dry-run fresh 均为 1 次 `/models`、0 生成、0 网络；仍等待创建临时 Key 和真实目录 GET 的单独授权。
- 2026-07-22：用户明确目标拓扑为 `GPT-Plus -> XM API Plus primary -> Wawazz backup`、`GPT-Pro -> XM API Pro primary -> Wawazz backup`。该目标已进入功能性/非功能性评测主线，但尚未应用生产路由；必须先完成公共 `discover -> compatibility -> gateway -> billing -> capacity -> proposal` 证据和隔离回验。非功能子 Agent 已补齐 v3 的 shared-pool、观察窗口、drill scenario 绑定门禁；当前目标状态仍 `NOT_READY`。
- 2026-07-22：质量优先轻量评测已在本地 Sub2API 账号 `73/74/75` 完成。三者 pulse 均为 `6/6`；全目录分别为 `14/16`、`14/26`、`18/18`。`73` 的 `gpt-5.6` 与 `74` 的六个模型未通过同步/SSE 硬门禁；只有 `75` 进入容量阶梯并完成 `129/129`，观察下界为并发 `10`、RPM 目标 `30`，保守建议 `8/24`。三者隔离 gateway 同步/SSE 均 200，但价格、provider debit、余额与商业条款未知，且 Token 计数存在异常差异，因此没有任何账号可晋级或切换。临时 Key/绑定已清理，账号均恢复 group `5`、`schedulable=false`，最终状态哈希 `145ba7085e8da2d319a05fe293ef1b488a7a38295a96e92cfb06cf41547d0ef1`。
- 2026-07-22：质量报告闭环审查补齐三项缺口：fast run 入库后先持久化稳定 incident 状态，再通过现有外键去重投递器生成无切换按钮的 Interactive Card；去重证据排除 run ID/时间，只绑定归一化质量结论；真实 PostgreSQL 的 `failed` notification row 可原位重新预留，而 `delivered/reserved` 仍抑制重复。生产已单独重建 relay-ops 到 `quality-report-read-only-20260722-v1`，启动迁移只新增空 `quality_reports` 表；容器 healthy/重启 0，`read_only + dry_run`，候选/probe/report 数均为 0，通知仍为 3，不存在 `candidate-fast:*` job。未发送合成事件或飞书消息，基础容器、D04 容器和飞书路由哈希未变。
- 2026-07-22：D04 v1 首发开放准备曾完成可执行清单、离线 fail-closed 评估器、准备专用 launch overlay、只读回滚和生产备份/隔离恢复演练；05:38 的七项 `NO-GO` 结果保留为历史证据，不再作为当前开放任务单。
- 2026-07-22：生产 `sub2api` 和 `relay_ops` 逻辑备份已通过 SHA-256、`pg_restore --list`、隔离 PostgreSQL 18 恢复和逐表行数哈希对比，全流程 10 秒，临时容器/网络/卷余留数为 0。该历史恢复证据仍有效；当前 D04 轻量门禁只要求服务器本地完整账户备份，不要求异地副本、七天留存或周期恢复演练。
- 2026-07-22：D04 单用户低额生产验收通过。隔离用户 provider ID `17` 仅有 1 条 `daily_login_credit` 成功 grant（`20,000,000` micro-USD）、1 条 `$20` provider balance history 和当前 `$20` 余额；同日重新登录未产生第二条 effect，provider/D04 usage 均为 `0`，未请求模型。最终恢复 `D04_MODE=read_only`、`D04_REGISTRATION_OPEN=false`，同源注册返回 `403 D04_REGISTRATION_CLOSED`，验收前后窄路由哈希均为 `b6e6ee12f484a2a919d993da56fa293904672ff2c16b65afef5caa6398832ec4`。
- 2026-07-22：D04 验收期间的 Admin API 方法发现曾误发空 settings PUT，导致 21 项设置归零/默认；日志发现后立即停止，并依据 Sub2API `v0.1.161` 官方 DTO 和审计 `283` 通过官方 Admin API 恢复，未直接写 PostgreSQL。TOTP、OIDC 与站点设置均复原，其他 243 项保持 hash-identical，恢复审计为 `289`，最终设置哈希 `52eff24fce0338ee4f8f81ad12a5d1406c46b6de050c99587035cdfd1f71a28e`。后续禁止用空对象 PUT 探索设置接口。
- 2026-07-22：D04 轻量开放门禁 v2 已完成实现和生产只读准备。策略、快照、阻塞码与动作统一使用 `active_upstream`，不绑定具体供应商；服务器本地账户备份集 `20260722T015202Z` 以 `0700/0600` 保存完整 `sub2api.dump` 与一致性 `d04.sqlite`，两者 SHA-256、`pg_restore --list` 和 SQLite integrity 均通过。新 AMD64 镜像 `d04-lightweight-launch-20260722-v2` 已构建但未部署，运行 D04 未重建，仍为 `read_only/registration=false`。02:28 最终离线复评为 `NO-GO`，当前阻塞码是 `launch_not_approved`、`upstream_balance_below_minimum`、`upstream_financial_evidence_stale`、`upstream_quality_metrics_stale` 和 `upstream_samples_insufficient`；评估器未执行真实动作或联系外部系统。
- 2026-07-22：用户批准实际受控开放后，服务器本地账户备份刷新为 `20260722T033408Z`；生产只读复核仍为 D04/relay-ops `read_only`、注册关闭/飞书 `dry_run`，`healthz/readyz/ops` 均 HTTP 200。新鲜无秘密快照 `D04-LIGHTWEIGHT-LAUNCH-20260722T033646Z` 记录唯一批准、活动上游余额 `-$0.01` 和最近 15 分钟自然样本 `0`；v2 评估结果为 `NO-GO`，仅阻塞于 `upstream_balance_below_minimum`、`upstream_samples_insufficient`，`real_action_executed=false`、`external_system_contacted=false`。未应用 launch overlay、未开放注册、未制造模型流量或 Feishu 事件。评估器新增回归覆盖负余额证据，避免把真实欠余额误判为快照格式错误。
- 2026-07-22：在同一批准尝试中再次刷新生产证据。快照 `D04-LIGHTWEIGHT-LAUNCH-20260722T034531Z` 的财务证据年龄约 `0.69` 分钟、质量证据年龄约 `1.09` 分钟；运维面板近 1 小时请求/Token/错误均为 `0`，D04/relay-ops 模式和 `healthz/readyz/ops=200` 未变。v2 仍为 `NO-GO`，阻塞码仍只有 `upstream_balance_below_minimum`、`upstream_samples_insufficient`；未应用 overlay 或制造流量。
- 2026-07-22：继续刷新同一批准尝试。快照 `D04-LIGHTWEIGHT-LAUNCH-20260722T035301Z` 的财务/质量证据年龄约 `0.82`/`1.65` 分钟；运维面板刷新于 `03:52:11Z`，近 1 小时请求、Token、错误均为 `0`，D04/relay-ops 模式和 `healthz/readyz/ops=200` 未变。v2 仍为 `NO-GO`，阻塞码仍只有 `upstream_balance_below_minimum`、`upstream_samples_insufficient`；未应用 overlay 或制造流量。
- 2026-07-22 04:09Z：按用户批准再次做生产只读复核。D04 容器 `healthy/restarts=0/OOM=false`，镜像仍为 `sub2api-internal-test:d04-public-registration-20260721-v1`；relay-ops 同样 `healthy/restarts=0/OOM=false`，模式仍为 `read_only + dry_run`。D04 为 `read_only`、注册关闭，`healthz/readyz/ops=200`，同源空注册 `403`。对同一 Git 忽略快照运行 v2 评估，结果仍为 `NO-GO`，仅 `upstream_balance_below_minimum` 与 `upstream_samples_insufficient`；`real_action_executed=false`、`external_system_contacted=false`。未应用 launch overlay、未制造模型流量或 Feishu 事件，不修改路由、倍率、价格、余额、Key、候选、probe 或数据库。
- 2026-07-22 04:11Z：再次完成同一只读复核。D04/relay-ops 镜像、健康、重启 `0`、OOM `false` 和模式均未变化；`healthz/readyz/ops=200`，注册仍为 `403`。v2 评估仍为 `NO-GO`，阻塞码保持 `upstream_balance_below_minimum`、`upstream_samples_insufficient`；评估未执行真实动作或联系外部系统。随着自然时间流逝，质量证据已接近 20 分钟新鲜窗口上限；没有人为生成样本或改变上游状态。
- 2026-07-22 04:25Z：通过已登录只读页面刷新活动上游财务证据和本站自然质量窗口。活动上游余额仍为 `-$0.01`；本站运维面板刷新于 `12:22:32`（Asia/Shanghai），近 1 小时请求/Token/错误均为 `0`，TTFT P95 与总耗时 P95 无样本。新快照 `D04-LIGHTWEIGHT-LAUNCH-20260722T042507Z` 记录 `launch_approved=true`，评估时财务/质量证据年龄约 `1.43`/`3.40` 分钟，仍仅返回 `upstream_balance_below_minimum`、`upstream_samples_insufficient`，且 `real_action_executed=false`、`external_system_contacted=false`。这是连续第三个目标回合的相同外部阻塞；D04 首发开放改为等待活动上游余额与自然样本变化后再恢复，不继续轮询、不制造流量、不充值或改生产。

## 已核验证据

- 2026-07-15：腾讯云国际站 Lighthouse 产品页公开展示 2C2G USD 10.08/年、原价 USD 50.4/年。
- 2026-07-16：腾讯云中国站实例已真实购买并验收；首尔二区、2C4G/60GB/30Mbps/1536GB 月流量、Ubuntu 24.04 LTS、自动续费关闭、到期时间 2027-07-16 09:44:32。
- 2026-07-16：从当前管理端到节点 Ping 5/5 成功，平均约 78ms，SSH 22 可达且返回 Ubuntu OpenSSH 9.6；应用 HTTP/HTTPS 尚未提供有效响应，符合未部署状态。
- 2026-07-16：专用 Ed25519 Key 已绑定并通过全新 SSH 会话验证；`PermitRootLogin no`、`PasswordAuthentication no`、`MaxAuthTries 3` 已生效。
- 2026-07-16：云防火墙 TCP 443 规则已生效；主机尚无 Web 进程监听 80/443，符合应用未部署状态。
- 2026-07-16：Ubuntu 全量更新完成并重启到 6.8.0-134 内核；无待更新包或待重启标记；NTP 已同步，时区为 Asia/Shanghai，系统自带约 2 GiB swap。
- 2026-07-16：Docker Engine 29.6.1、Compose v5.3.1、overlayfs 和 cgroup v2 验收通过；临时 `hello-world` 容器实跑成功并清理；日志轮转和自动安全更新已启用。
- 2026-07-16：Caddy-only 临时 HTTPS bootstrap 已通过端到端验收：HTTP 自动跳转 HTTPS，`/health` 返回固定 JSON 和 200，证书为公开信任证书且 SAN 匹配运行时域名；主机仅 Caddy 发布 80/443，5432、6379、8080 均未监听。运行时域名和实例地址不写入项目文档。
- 2026-07-16：对 UP01 执行无鉴权只读探测；站点根路径返回 200，`/v1/models` 返回结构化 `API_KEY_REQUIRED` 401，确认 OpenAI 兼容 Base URL 为 `https://aliuapi.top/v1`。探测未携带或使用任何 API Key。
- 2026-07-16：服务器端生成五个互不相同的生产秘密，环境文件权限为 600；未复制或读取本地 `infra/.env`，未写入任何上游 API Key。
- 2026-07-16：完整 Compose 原子替换 bootstrap 成功；PostgreSQL、Redis、Sub2API 均 healthy，Caddy 运行正常，HTTP 308、HTTPS `/health` 200 和公开 TLS 证书通过验收。
- 2026-07-16：宿主机未监听 5432、6379、8080；数据库中仅有一个已确认管理员记录，单独重启 Sub2API 后管理员记录保持一个且 HTTPS 连续返回 200。
- 2026-07-16：生产站全局 2FA 开关已通过官方管理界面启用，管理员个人 2FA 已成功绑定；数据库复验 `totp_enabled=true` 且 TOTP 密钥已加密持久化。本机旧 `localhost:8080` 实例未修改。
- 2026-07-16：UP01 已通过官方管理界面创建；Base URL、并发 1、优先级 50、倍率 1、自动透传关闭和 WS 关闭均复核通过，模型目录同步接口返回 200 并保存 20 个模型。自动探测的 `gpt-5.4`、`codex-auto-review` 和手动最小测试的 `gpt-5.4-mini` 均由上游返回 503。上游控制台确认余额为 `$4.83`，当前 Key 绑定的 `SOL-通道1` 持续错误，而 `SOL-通道3` 正常，故障根因已定位为 Key 的故障通道绑定；尚未完成调用与计费闭环。
- 2026-07-16：用户将同一上游 Key 改到状态正常的稳定 Plus 分组后，使用相同 `gpt-5.4-mini` 和 `hi` 的管理端非流式复测成功，响应为 `Hi! How can I help?`。生产日志记录测试接口 200、约 1.4 秒；上游控制台新增 1 次请求、178 tokens、标准费用 `$0.0003`。该管理端账号测试不计入 Sub2API 用户扣费；真实下游闭环随后于 2026-07-17 完成。
- 2026-07-17：腾讯云主机安全报告一次高危可疑 SSH 登录；服务器日志确认告警时段的成功登录源与受控运维出口一致，且公钥指纹与本机专用 ED25519 Key 完全匹配。从开机至核验时所有成功 SSH 登录均使用该唯一指纹；其他来源均停在认证前或失败。`PermitRootLogin no`、`PasswordAuthentication no`、`MaxAuthTries 3` 仍生效，未发现新增授权公钥或成功入侵证据。
- 2026-07-17：创建低额度、短有效期的下游测试 Key；首次 403 定位为 UP01 未绑定测试 Key 所属 `openAI` 分组，第二次 403 定位为测试用户站内余额为零。两项均通过官方管理界面修正，没有直接修改数据库调度状态。
- 2026-07-17：真实下游 `gpt-5.4-mini` 非流式 HTTP 200，记录 555 输入、10 输出 Token，扣费 `$0.00074925`，耗时约 2.19 秒；流式 HTTP 200，收到完整 Responses SSE 生命周期，记录 554 输入、9 输出 Token，扣费 `$0.00074400`，耗时约 3.11 秒，首 Token 约 3.07 秒。
- 2026-07-17：两次真实请求合计扣费 `$0.00149325`；API Key 已用额度与测试用户余额减少额完全一致，数据库留下两条独立 usage 记录，无重复计费。上游同一时刻出现两条约 563/565 Token 的 `gpt-5.4-mini` 记录，标准费用各约 `$0.0007`，实际费用合计约 `$0.0001`。
- 2026-07-17：保存账号分组时 Sub2API 自动执行一次 `codex-auto-review` Responses 能力探测，服务日志标记为 `openai_probe`；上游实际费用约 `$0.0003`、标准费用约 `$0.0081`，未计入测试用户用量。验收后测试 Key 已禁用但保留审计记录。
- 2026-07-17：三笔 `gpt-5.4-mini` 样本确认标准价格为输入 `$0.75/M`、输出 `$4.50/M`、缓存读取 `$0.075/M`；稳定 Plus 实际扣费约为标准价的 4%，但 D03 不依赖该折扣。
- 2026-07-17：D03 邀请制暂定价为输入 CNY 8.70/M、输出 CNY 48.30/M、缓存读取 CNY 1.60/M，缓存写入不提供。30M Token/月基准下完全成本毛利 25.90%；使用该固定价格时，盈亏平衡约 8.55M、20% 毛利约 19.09M、25% 毛利约 27.60M Token/月。价格尚未写入生产，等待用户确认。
- 2026-07-18：上述固定 CNY 价格被 D03 方案一取代且从未写入生产。用户确认首版按模型标准基础价乘用户组 `0.15x`；Neko 已切为生产，账号倍率 `0.07`、并发 `3`，Aliu 调度关闭并保留候选。
- 2026-07-18：NekoAPI Pro 池独立短测完成。`/v1/models` 返回 200，`gpt-5.6-sol/terra/luna`、`gpt-5.5`、`gpt-5.4-mini` 均完成非流式请求；`gpt-5.6-sol` 流式 SSE 完整。11 次请求的标准费用 `$0.0026`、实际逐请求费用约 `$0.000182`，验证 `0.07x`；2 路和 3 路并发均在约 2.39 秒内同时完成，至少 3 路未串行。API usage Token 与站方计费明细存在差异；长期稳定性、国内链路和商业转售权未验证。测试 Key 限额 `$0.10`，测试后已停用，生产路由未修改。详见 `docs/superpowers/reports/2026-07-18-neko-upstream-short-verification.md`。
- 2026-07-18：建立统一上游验收工具 `ops/upstream-benchmark.rb`、版本化 `mvp-text-v1` profile、追加式运行/决策 JSONL 台账、Aliu/Neko 历史导入和比较报告；新增个人 Skill `$benchmark-upstream-channel`。Neko `live_direct`、隔离 `live_gateway` 和 `live_billing` 均通过：两次同步和一次 SSE 共 `225` Token，Sub2API 扣 `$0.005600`，Neko 实际扣 `$0.000393`，三方 Token 完全一致，显示精度下倍率约 `0.06895x`。测试 Key 已停用、管理员专属组授权已撤销、隔离账号和分组已删除；当日生产 Aliu 路由未修改，随后已由用户批准切换到 Neko。此前 blocked 记录由追加式 `supersedes` 证据纠正。
- 2026-07-18：建立统一上游验收工具 `ops/upstream-benchmark.rb`、版本化 `mvp-text-v1` profile、追加式运行/决策 JSONL 台账、Aliu/Neko 历史导入和比较报告；新增个人 Skill `$benchmark-upstream-channel`。Neko `live_direct`、隔离 `live_gateway` 和 `live_billing` 均通过：两次同步和一次 SSE 共 `225` Token，Sub2API 扣 `$0.005600`，Neko 实际扣 `$0.000393`，三方 Token 完全一致，显示精度下倍率约 `0.06895x`。随后用户明确批准切换，Neko 生产连接测试、同步请求和 SSE 请求均 HTTP 200 且 SSE 含 `[DONE]`；临时下游 Key 和用户已清理，Aliu 调度已关闭并保留候选。
- 2026-07-15：腾讯境外活动页显示 2C2G 一年卡可选东京、新加坡等地区，并要求产品新用户、实名主体限购。
- 2026-07-15：阿里云中国站宣传支持海外地域、低至 68 元/年，但公开页面未返回最低价对应的确切配置和地域，不能直接作为已验证购买项。
- 2026-07-15：主计划通过旧规则扫描和 Markdown 围栏平衡检查。
- 2026-07-15：初始基线锁定的 Sub2API 镜像运行时为 v0.1.155；2026-07-19 已升级并固定生产运行时为 v0.1.161，兼容性修复见同日记录。Compose 和 Caddy 配置通过解析。
- 2026-07-19：上游接入评测 V2 已实现：`ops/upstream-benchmark-v2.rb` 可发现并分类完整模型目录、逐一测试文本模型同步/SSE、执行有界并发/RPM 阶梯、计算 50% 目标毛利下的内测/商业倍率，并生成无凭据 Sub2API proposal。个人 `$benchmark-upstream-channel` Skill 已升级为浏览器取证、用户“采纳”审批后配置、快照/回验/清理闭环；V2 尚未对新上游执行实时评测。
- 2026-07-15：本地四服务健康，经 Caddy 的 `/health` 和首页返回 200；应用连接池、PostgreSQL/Redis 低内存参数和 16 MiB 请求体限制生效。
- 2026-07-15：重建应用/代理后管理员记录仍为 1，证明 PostgreSQL 卷持久化；测试容器和网络已清理，卷保留。
- 2026-07-15：UP01 YAML 校验器 13 个测试通过；安全示例通过普通校验，严格模式按预期仅因 `draft` 状态失败；`*.local.yaml` 已确认被 Git 忽略。
- 2026-07-15：API 售价试算器 8 个测试、28 个断言通过；虚构示例得到 CNY 1.54/5.74 每百万 Token、25.21% 完全成本毛利，并输出 Sub2API USD/Token 精确字段；这些不是实际报价。
- 2026-07-15：人工账本工具 8 个测试、33 个断言通过；四事件虚构哈希链验证成功，模拟收款到余额差异为 0，请求预览无认证头且未发送。
- 2026-07-15：订阅账号评估器 9 个测试、49 个断言通过；虚构独立 OpenAI Plus 为 `recommended`，托管 K12 和共享 Token-only Pro 为 `rejected`；未处理真实账号或凭据。
- 2026-07-15：加入 ACC01 后全量 Ruby 回归共 38 个测试、140 个断言全部通过；基础设施契约、候选 YAML、Markdown 围栏和目标文件秘密值扫描通过。
- 2026-07-15：支付控制模拟器 12 个测试、54 个断言通过；PAY01 是 `disabled_simulation_only`，合成支付/退款各只改变余额一次，未连接真实 Provider。
- 2026-07-15：加入 PAY01 后全量 Ruby 回归共 50 个测试、194 个断言全部通过；基础设施契约、所有示例 YAML、Markdown 围栏和 PAY01 受控文件秘密值扫描通过。
- 2026-07-15：ROUTE01 专项 15 个测试、62 个断言通过；虚构评分、安全重试、熔断、半开探测、人工禁用和容量门槛均未发送真实流量或执行采购。
- 2026-07-15：加入 ROUTE01 后全量 Ruby 回归共 65 个测试、256 个断言全部通过；基础设施契约、8 份示例 YAML、Markdown 围栏、路由本地文件忽略、秘密值和无网络客户端边界检查通过。
- 2026-07-15：OPS01 专项 14 个测试、72 个断言通过；健康快照为 0 个 Critical/High/Warning，事故演示正确识别 3 个 Critical，并明确没有执行动作或联系外部系统。
- 2026-07-15：最终全量 Ruby 回归共 79 个测试、328 个断言，0 failures / 0 errors / 0 skips；基础设施契约、10 份 YAML、Markdown 围栏、运营本地文件忽略、OPS01 秘密值和无网络/数据库/Docker/进程控制边界检查通过。

## 未完成项

- [x] A01：用户已亲自核对并购买服务器，Codex 已完成只读验收。
- [ ] D02 用户购买 `xingqiaolab.top` 后确认归属和首版 API 子域 `api.xingqiaolab.top`。
- [x] 使用零成本临时域名完成 Caddy HTTPS 与公开证书技术验证；仅限内部验证，不提供给外部测试用户。
- [x] 建立现有上游 Base URL、鉴权、模型、价格、余额、限额和条款的安全模板、校验器和验收清单。
- [x] Neko Base URL、生产账号、六模型定价、限制模型、按请求模型计费、同步/SSE 和受控扣费验收已完成；测试 Key 和用户已清理。长期稳定性、价格变化和商业条款仍待观察。
- [x] 离线准备并验证 Compose、Caddy、环境变量生成器、契约测试和运行手册。
- [x] 服务器到位后完成 SSH、防火墙、Docker、swap 和真实网络基线。
- [x] 部署并验证 Sub2API、PostgreSQL、Redis、Caddy 核心站点，完成 HTTPS、管理员初始化和重启持久性验收。
- [x] 部署并验证首条真实请求、日志和计费闭环；测试 Key 已在验收后禁用。
- [x] 建立不依赖真实价格的 CNY 成本、USD 内部余额、支付费率、异常准备和固定成本试算机制。
- [x] D03 双上游方案已由用户确认并切换；生产 `GPT-Pro` / `GPT-Plus` 站内倍率均 `1.0x`，Neko 主账号倍率 `0.10`、并发 `2`（隔离复制账号并发 `1`），Wawazz 账号倍率 `0.05`、并发 `1`；Aliu 调度关闭并保留灾备。
- [x] 完成 NekoAPI Pro 池统一验收及生产切换冒烟：目标模型、非流式、流式、`0.07x` 实际扣费、至少 3 路并发、生产网关同步/SSE 均通过；临时测试账号和 Key 已清理。长期稳定性、网络和商业授权仍待观察。
- [x] 完成 Neko 容量短测：同步并发至少 50、SSE 并发至少 10、稳定 RPM 至少 180；240 RPM 出现一次超时。临时容量测试 Key 已删除，生产路由未修改。
- [x] D04 内测自动化已退役：独立注册服务、人数上限、预算门禁、每日额度和受控开放流程均不再属于活动路径；注册和邀请码仅由 Sub2API 原生设置管理。
- [x] 2026-07-19：生产升级到 `v0.1.161` 后因 `gateway.text_max_body_size` 默认 32 MiB 超过 16 MiB 上限短暂返回 502；固定 `GATEWAY_TEXT_MAX_BODY_SIZE=16777216` 后仅重建 Sub2API，健康检查恢复 200，其他三项服务未重建。
- [x] 2026-07-19：Neko 六模型受控验证：6 次同步、1 次 `gpt-5.6-sol` SSE 均 200，SSE 含 `[DONE]`；未知模型 404；7 条记录 Token 均 `8/5`，用户扣费约 `$0.000125`，符合标准价乘 `0.15x`。
- [x] 2026-07-19：relay-ops 只读生产部署完成；Go race/vet、PostgreSQL E2E、Ruby `118/472`、三组 Compose/Caddy 契约、真实 `/ops` 登录复用和 `/pricing` 单位/阶梯价验收通过；生产 `probe_runs=0`。
- [x] 执行已确认的分组切换：公开 `GPT-Pro` / `GPT-Plus`、站内 `1.0x`，账号 `7/8` 分别为 `0.10x/0.05x`；两池同步/SSE/计费均有有效历史样本。该项只记录历史配置，当前活动集合改由 Sub2API 调度状态动态发现。
- [x] 为 Neko 创建独立低额监控 Key 并完成 GPT-Pro 原生样本；relay-ops 保持 `read_only`，候选真实 probe 仍需单独批准（每候选最多 2 请求、费用上限 `$0.002`）。
- [x] 2026-07-20：relay-ops 完成生产来源录入与 UI 部署；Neko 绑定 `GPT-Pro`，价格页 `/pricing`、用量页 `/usage`、性能页 `/monitor`；Wawazz 绑定 `GPT-Plus`，公开页面 `/home`、用量页 `/usage`、性能页 `/monitor`。Wawazz 当前没有公开价格页，relay-ops 不把主页当作已验证模型价格，仅保留页面证据并等待后续补充。
- [x] 2026-07-20：账单会话能力接入调度器；管理员可登记 `/run/secrets/upstream-sessions/` 下的 Cookie/Bearer 秘密文件，系统每小时读取用量页并记录辅助成本证据，登录页/401 会生成去重会话失效事件和登录链接。未安装真实会话时，质量与公开价格监控继续运行。
- [x] 2026-07-20：飞书确定性命令新镜像已在生产以 `read_only + disabled` 部署；五个飞书文件只读挂载、Caddy 精确 POST 路由、事件订阅、challenge、真实群解析/回复和 PostgreSQL `command_disabled` 审计均通过；四个基础容器未重建。
- [x] 2026-07-20：Aliu `2` 已配置为 GPT-Pro/GPT-Plus 共享灾备，总并发保持 `1`，覆盖六个必需模型；Neko 隔离复制账号 `9` 保留但未绑定、不可调度。共享账号级锁、全量 race/vet/契约、生产健康和零写快照均通过。
- [x] 2026-07-21：现有飞书 App Bot 已接通原生监控主动告警、重复抑制、恢复通知、每日运营摘要和只读 Agent 确定性回退；真实群、数据库投递计数、调度、零上游访问和零路由写入均通过。
- [x] 建立人工充值、余额调整、退款/反向流水和每日用量成本的追加式模拟账本与对账摘要。
- [ ] 用户确认 D05 后，在真实小额订单上验证外部收款、Sub2API 余额历史和账本三方一致。
- [x] 建立订阅账号非敏感候选模板、硬淘汰、评分、首样推荐、Sub2API 映射和单账号验收清单。
- [ ] 最终报告后录入真实卡网候选并由用户购买 1 个通过评分的样本，再执行 ACC01 联网验收。
- [x] 建立 PAY01 条件式渠道选择、禁用态配置校验、合成回调/退款幂等模拟和真实验收清单。
- [ ] 最终报告确认经营主体后由用户申请对应商户，再执行沙箱/最小额支付、退款和对账验收。
- [x] 建立 ROUTE01 非敏感候选、资格/评分、安全重试、熔断、三网测量模板和扩容经济阈值。
- [ ] 真实资产到位后采集三网和上游样本；只有 D10/D11 触发并确认后购买优化线路或第二节点。
- [x] 建立 OPS01 日/周/月节奏、告警分级、可重复止损动作和只报告的离线评估器。
- [x] 确定 BKP01 为 `pg_dump -Fc`、restic 和 Cloudflare R2 Standard，并建立真实备份/恢复验收清单。
- [ ] 服务器和数据库到位后，由用户开通备份存储并完成连续 7 天备份及一次独立恢复演练。
- [ ] 用户审阅最终推荐报告并确认实际购买顺序；当前不执行付款、购买、充值、实名、商户申请或真实备份。

## 恢复工作规则

1. 先读本文件，再读权威计划的第 19 节。
2. 不重复询问已确认的 D01。
3. 遵守 D13：在用户审阅完整报告前，不执行任何真实付款、充值、购买、收款或商户开通。
4. 所有后续工作可按推荐资产继续离线准备，但必须标记为假定配置；真实验收保持未完成。
5. 收到实际实例、域名或其他资产的非敏感信息后，再更新资产台账并执行真实环境复验。

## D04 历史本地基线（2026-07-19，已被 2026-07-21 新口径覆盖）

- Go 1.24 服务最初建立了 SQLite WAL 账本、Admin API 幂等客户端、Fake Provider、邀请/签到/推荐基线、总预算、用量游标、余额漂移只读降级、调度日报和飞书告警。
- 该节的邀请、签到和推荐规则已由 2026-07-21 的公开注册/每日首次登录规则覆盖，只保留为历史实现事实，不得作为下一任务的业务口径。
- 默认部署模式是 `D04_MODE=read_only`。服务未读取生产 `infra/.env`，未安装真实 Admin API Key、Webhook、JWT 或用户 Key，也未改变生产余额。
- 验证通过：`go test ./... -race`、`go vet ./...`、本地镜像构建、只读非 root 容器 `/healthz` 冒烟、Compose/Caddy D04 契约、基础设施基线和既有 Ruby 回归；当前本地镜像 ID 为 `sha256:b5945deb71fdf3d03da878730eb84774007c529f0fa7e55f0263658cf31f0a07`，约 5.6 MB。

## D04 Sub2API 活动上游门禁 v3（2026-07-22）

- v2 的手工 `active_upstream` 已被识别为语义不足，不能再作为下一次真实开放的准入依据。v3 的成员集合只能从 Sub2API Admin GET 账户列表发现：每个 `status=active && schedulable=true` 的账号都是当前活动上游；不写死供应商、名称、分组或账号 ID。
- 每个发现账号必须独立通过最低余额、余额新鲜度、运行时可用性、15 分钟内账户归属的自然质量（样本、成功率、错误率、TTFT P95、总耗时 P95）。任一账号缺证据、过期、暂不可用或失败均为整体 `NO-GO`，不得用分组汇总或另一个通过账号掩盖。
- 工程规则固定为：做任何功能前先核对 Sub2API 官方 handler/DTO、现有管理界面和生产能力，并明确记录为“直接复用、改造复用或确需新增”；用户、认证、调度、用量和请求质量优先复用 Sub2API，禁止平行重建。
- v3 的旧 `7/8` 与 `10/11/12` 只读验收已成为历史证据。本轮通过 SSH 重新发现的实时集合为 `10/11/12/13`，规范哈希为 `f6b733f89e799048c92d90dc0d404ce1f96300bf1f2964184cc681bdcc2457e7`；后续判定仍必须每次动态读取，不把该集合写死。
- 本轮验证显示 D04 为 `read_only/registration=false`、relay-ops 为 `read_only + dry_run`，其他五个容器 ID 未变化，也没有制造飞书事件或修改路由/账号/倍率/价格/余额/Key/D04/业务数据库。当前质量证据尚未达到 v3 `GO`；没有新鲜同快照 `GO` 和新的行动批准，不得应用 launch overlay。

## Sub2API 原生滚动模型策略（2026-07-22/23，历史任务已停用）

- 首版候选固定为已批准的 `gpt-5.5`、`gpt-5.6`、`gpt-5.6-luna`、`gpt-5.6-sol`、`gpt-5.6-terra`；后续从原生发现结果滚动保留最新两个已批准 GPT 次版本，不按供应商或固定账号选择。
- 该次历史发现账号为 `10/11/12`，没有发现高于 GPT-5.6 的新次版本；五个候选的 Sub2API 原生价格完整。模型发布账号集合哈希为 `cf28d87d0070ac5eca5847714ad4512b01b8e1cc098bf47691924cbf484aef3c`，基础模型配置哈希为 `1261a40c660b6b6d6a4e47c3e6ce63825e36302b7c01832ef5ed676c71690f68`。
- `/ops` 显示 `待测试`，提案 `eda38d86e130d156d2eb1c267cca8289771278e4da5ce9ddb8651f390bf3d09b`。阻塞为首版资格未完成、余额证据缺失、公开分组覆盖不完整、自然质量证据缺失。
- 两个公开组的 `models_list_config.enabled` 仍为 `false`；未发布首版目录。发现阶段只执行每个账号一次原生模型同步，模型生成请求为 `0`，未运行付费资格测试。
- 该任务的生产镜像曾为 `model-release-read-only-20260722-v1`；2026-07-23 已由账号池质量镜像替换，旧 timer 停用，unit、脚本和证据仅作历史保留。
- 账号集合与基础配置在部署后重新计算仍与提案完全一致；Sub2API 日志无账号 bulk-update 或 group PUT，D04 仍只读且注册关闭。权威证据见 `docs/superpowers/reports/2026-07-22-sub2api-native-rolling-model-policy-verification.md`。
