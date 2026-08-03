# 项目全局进度总账

**更新时间：** 2026-08-03

**当前生产交付序列：** 已按 [当前生产交付决策总表](active-delivery-contract.md) 登记四个顺序生产任务。账号评分与监控入口已发布过首版，但 2026-08-02 用户线上验收确认其全站账号范围、全站经营/服务分区、默认全站视图、分组账号分区和评分入口位置均未达到已确认规格，因此重新归为**进行中**；全站账务运行时已安全暂停且未部署新镜像，全站账单数据闭环、Monitor 与飞书联动继续等待任务 1 完整验收。30 分钟仅为目标节奏，不是停止修复、跳过门禁或宣布完成的硬上限。只有当前协调任务可以更新总账、合并发布基线或宣布完成，实施任务不得并行修改同一基线。

**本轮登记（2026-08-02）：** 账号监控真实上游成本与对账闭环恢复核验，状态：进行中（仅在 `codex/relay-ops-public-route` 分支执行；先核对生产 Caddy 热配置、健康/认证路由、发布锁和真实只读账单输入；未满足“已推送 + 已部署 + 真实数据验证”前不得标记完成）。

**本轮登记（2026-08-03）：** 账号监控产品验收收口，状态：进行中（账号范围、聚合与首轮卡片修复已推送并部署；生产复验新增确认：监控分组查询遗漏软删除过滤，错误展示 10 个已删除历史/测试分组；账号卡片须限制桌面每行最多 2 个、收起今日调用、评分显示整数并改用通俗证据文案、弱化评分与全局优先级、补齐渠道监控式柱状图和各指标样本数；“纸面利润”统一改为“账号利润”；渠道监控成功探测统一按绿色成功展示并恢复指标样本数；“历史按日”“异常明细”及账号监控其他交互须逐项验证修复。全部重新推送、部署并在线验证前不得标记完成）。

**唯一总账：** 本文件是项目事项、部署状态和验证证据的唯一全局入口。

## 统计口径

本次回填审阅了 `docs/superpowers/reports/` 下截至 2026-07-30 的 60 份报告，并结合项目当前状态和 Git 历史，将同一功能的阶段性报告合并为 49 个独立事项。只有同时满足“已推送到服务端 + 已部署 + 已验证生效”才计入“生产工程代码/配置已部署并验证”。只有仍存在运行时代码或生产配置差异，才计入“工程代码/配置差异待部署”；文档、研究、报告、离线证据、历史验证和外部验收本身不构成工程部署差异。事项按独立交付结果合并统计，不按报告文件数量统计；历史暂停或阻塞事项保留在进行中区，不制造完成结论。

## 统计摘要

| 分区 | 独立事项 | 判定 |
|---|---:|---|
| 生产工程代码/配置已部署并验证 | 18 | 已推送、已部署、已验证生效 |
| 工程代码/配置差异待部署 | 10 | 仍存在运行时代码或生产配置差异 |
| 持续实施 | 2 | 工程实现尚未完成，或仍在推进中 |
| 运维/研究跟进（非工程部署差异） | 23 | 文档、研究、历史证据或外部验收跟进 |
| **合并后的独立事项合计** | **53** | **当前总账快照** |

## 当前最重要进行中事项

0. **OpenAI 模型映射审计**：仅覆盖 OpenAI-compatible HTTP JSON、SSE 与 Responses WebSocket；新增 `usage_logs.actual_response_model`，记录上游原始响应模型并在管理员 usage log 展示。**状态：进行中（本地实现与验证阶段，尚未推送、部署或线上验证）**。
0.1. **账号监控全站经营、分组运营与账号质量**：已确认产品规格；首版已推送并部署，但线上产品验收发现默认进入首个分组、前后端过滤暂停/非调度账号、全站经营与账号服务数据混排、分组缺少明确的服务/经营分区、评分入口不显眼、账号未按可用/不可用/成本不合格/待确认/暂停分区。**状态：进行中（正在基于已部署提交 `51df96c3d` 修复；未重新推送、部署并验证前不得标记完成）**。

1. **账号监控真实上游成本与对账闭环**：统一真实账单适配器、对账请求/异常/补登记流水、管理员刷新、日切调度、飞书日报及现有 `AccountMonitorView.vue` 可见页面已推送、部署并通过公网接口验收；本轮继续接通按显式账单账号映射和专用凭证主动采集最新逐笔账单与累计快照，日结在采集或对账未闭合时必须阻断。手工补登记重试幂等性、逐笔请求 ID 回退、迁移重复映射预检、root-only 账单 provisioning、relay-ops 不可变发布路径及蓝绿演练夹具已完成本地实现与回归。**状态：进行中（relay-ops `d3860531d`、生产 host executor、root-only provisioning wrapper 和 Caddy 公网 JSON/认证路由均已部署并验收；生产仍无全站账号的真实只读账单授权和映射，五类核心账务数据均为 0）**。

**本轮新增登记：** `relay-ops` 不可变镜像发布路径及 root-only 账单 provisioning wrapper（提交 `df4870470`）；**状态：进行中（immutable relay-ops、host executor/wrapper 与 Caddy 公网 JSON/受保护 API 路由均已部署并验证；生产的账单授权会话、上游成本快照、对账请求明细、对账执行记录和每日账务快照仍为 0，待全站所有可计费账号的合法只读账单授权和明确映射）**。

**本轮新增登记（2026-08-03）：** 账号监控入口会话隔离修复，状态：进行中（已定位为 `/relay-ops/api/reconciliation/*` 的 401 被共享 API 客户端误判为主站会话失效；正在补充请求级会话恢复隔离、回归测试、生产部署和公网登录复验，未完成部署验证前不得标记完成）。

2. **2026-08-01 main 全局低延迟配置蓝绿发布与待部署事项联合评估**：双槽 API、唯一 worker、host executor、Caddy 路由和停机 bootstrap 已完成生产部署；本轮对账应用已追加发布并验证。**状态：已完成**。
3. **2026-07-31 生产站点中断恢复**：11:56 首次故障后于 13:28 回滚恢复；14:10–14:11，“调度优化”任务误将事故回滚判断为外部覆盖，再次覆盖 Compose 并强制重建 Sub2API/relay-ops/Caddy，造成第二次中断。14:15 已再次恢复至 11:53 发布前快照；公网健康连续 5 次返回 200，首页、价格和文档入口正常，五个容器运行正常且重启计数为 0。**状态：恢复已验证；永久兼容性修复仍未推送、部署和验证**。
4. **Sub2API 低延迟/会话隔离/Monitor 自动刷新工程差异**：生产仍缺全局 OpenAI 低延迟配置、WebSocket `store=false` 会话隔离修复和 Monitor 自动刷新后端/前端/设置实现。**状态：工程代码/配置差异待部署**。
5. **relay-ops 原生 P0/P1 Bridge**：Bridge 实现仍缺通知策略兼容性和必需的只读数据库接线；relay-ops 与 Sub2API 的受控发布机制已经存在，不再作为本项缺口。**状态：持续实施**。
6. **Caddy/homepage 运行时差异与全站账务总账**：Caddy/homepage 运行时改动须独立发布；全站账务总账已进入 `origin/main`，但尚未部署、激活或生产验证。**状态：工程差异待部署**。
7. **Neko/Wawazz 门禁、D04 readiness、备份留存和后续运营**：均保留最新证据和外部验收跟进，不因文档或历史报告单独记为工程未部署。
8. **账号上游倍率自动同步**：托管/手工覆盖策略、审计、缓存同步和账号生命周期/定时触发已合并到 `main`；最终修复完成显式策略意图、提交顺序安全的 Redis 版本 fencing、按模式隔离 singleflight 和 `UpdateLastUsed` 持久化版本单调性。后端、前端及真实 PostgreSQL/Redis 集成回归通过，最终独立复审批准合并；尚未推送、部署或线上验证。**状态：工程差异待部署**。
9. **Monitor V2 卡片信息精简**：已合并移除各指标样本数量、模型折叠区及接口 `models` 字段，保留性能指标、有效调用、P95 与趋势；前后端测试已通过，但尚未推送、部署或生产验证。**状态：工程差异待部署**。
10. **Monitor 当前状态与飞书历史错误码修复**：已按“最新一次渠道探测”口径完成实现、回归并进入 `origin/main`，修正最新成功被历史失败压过，以及历史余额耗尽错误码残留导致账号正在运行却被告警为无可用账号。**状态：工程差异待部署（已推送，待生产部署和告警/恢复线上验证）**。
11. **`api.xingqiaolab.top` TLS 兼容性修复**：Caddy RSA-2048/TLS 1.2/1.3 策略已部署，但 CC Switch 刷新仍失败。2026-08-01 完成 nginx TLS 前置层受控生产切换，Compose、Caddy、nginx 及未受影响服务门禁通过；但受影响 Mac 上 TLS 1.2 仍在 ClientHello 阶段被重置，CC Switch 3.18.0 实际刷新仍显示“查询失败”，因而已回滚到 Caddy 直接接管 80/443。同一公网 IP 上仅 SNI `api.xingqiaolab.top` 的 TLS 1.2 被重置，替换 SNI、无 SNI 及服务器本机访问均成功，根因已收敛为客户端到源站之间的域名特异网络干预。**状态：进行中（nginx 代码已推送、生产试部署已回滚；待使用兼容域名或新公网/CDN 入口完成客户端验收）**。
12. **账号监控分组经营与评分规则**：已接通真实账单分组归属、全站/分组/账号聚合与按日历史、跨分组唯一对账请求去重、分组关闭语义、可配置评分权重、账号卡片经营数据、分组 Tab 与历史入口，并修复 RuntimeService 对经营汇总/历史 API 的转发回归；本轮补齐首页“今日 + 历史累计”账务展示、全站/分组服务健康（可用/不可用/待确认/暂停、成功率、首包时间 P50、总耗时 P95）及卡片优先级的单事件保存。账号监控目标套件、后端 `vet/build`、前端类型检查/构建、relay-ops 全量 Go 测试、生产同构 pnpm 9 frozen install 和 Linux/amd64 镜像构建通过；发布修复提交 `a0f343588` 已推送到 `origin/main`，合格候选为 `xingqiao-sub2api@sha256:dc5344a63881fbff40a360f613165f694b2f2c652d3c4ac0ef7ad015455699fa`。2026-08-02 正式 host executor 预检返回 `downtime_required=true`、`reason_code=migration_set_changed`、预计不可用 300 秒，并在启动候选、切流量和执行迁移前停止；生产仍运行提交 `638ce8f01a9c1879f76c0415fe90ef78c782d089`，评分权重接口仍为 `404`。**状态：进行中（已推送、候选已构建并通过预检；用户已确认接受本轮受控维护停机，待执行维护发布与线上验证）**。

## 生产工程代码/配置已部署并验证（18 项）

1. **首版生产 Compose/Caddy/Sub2API/PostgreSQL/Redis**
   - **解决的问题：** 建立可运行的首条生产链路和基础服务边界。
   - **做了什么优化：** 从主机基线切换到完整 Compose 四服务栈，并由 Caddy 提供入口。
   - **影响范围：** 生产站点、网关、数据库、缓存和反向代理。
   - **状态：** 已完成；生产部署并通过健康与链路验证。
   - **证据：** [当前状态](current-state.md)、[relay-ops 只读生产验收](../superpowers/reports/2026-07-19-relay-ops-read-only-verification.md)

2. **relay-ops 只读部署**
   - **解决的问题：** 让运维、价格和采集信息可观察，同时避免控制面写入生产。
   - **做了什么优化：** 复用管理员会话，提供只读页面、健康检查和公开价格展示。
   - **影响范围：** relay-ops 运行容器、生产运维入口和价格页。
   - **状态：** 已完成；只读部署已验收。
   - **证据：** [relay-ops 只读生产验收](../superpowers/reports/2026-07-19-relay-ops-read-only-verification.md)

3. **飞书命令 disabled/dry_run 生产链路**
   - **解决的问题：** 在不改动路由和基础服务的前提下验证飞书回调边界。
   - **做了什么优化：** 固定命令解析、禁用态审计和 dry-run 零写路径。
   - **影响范围：** relay-ops 飞书回调、生产审计和通知链路。
   - **状态：** 已完成；disabled 和 dry_run 生产链路均已验证。
   - **证据：** [disabled 生产验证](../superpowers/reports/2026-07-20-feishu-command-disabled-production-verification.md)、[dry-run 生产验证](../superpowers/reports/2026-07-20-feishu-production-dry-run-verification.md)

4. **生产来源、价格、用量监控**
   - **解决的问题：** 缺少来源登记、公开价格采集和用量会话的统一观察。
   - **做了什么优化：** 增加来源登记、价格 hash 去重、阶梯价格展示和只读用量会话支持。
   - **影响范围：** relay-ops 采集、管理员观察和价格页面。
   - **状态：** 已完成；生产接口、来源和采集行为已验收。
   - **证据：** [生产来源与会话监控](../superpowers/reports/2026-07-20-relay-ops-production-source-monitoring-verification.md)

5. **D04 公共注册和每日登录**
   - **解决的问题：** 将公开注册和每日登录规则落到可验证的生产链路。
   - **做了什么优化：** 完成公开注册、邀请码和每日首次登录行为的生产验证。
   - **影响范围：** D04 用户入口和注册相关行为。
   - **状态：** 已完成；该历史生产事项已验证，后续口径由 Sub2API 原生设置承接。
   - **证据：** [D04 公共注册与每日登录](../superpowers/reports/2026-07-21-d04-public-registration-daily-login-verification.md)

6. **飞书主动告警**
   - **解决的问题：** 生产异常缺少主动通知、恢复和重复抑制。
   - **做了什么优化：** 接通主动告警、恢复通知、去重和每日运营摘要。
   - **影响范围：** 原生监控、relay-ops 出站通知和运营值守。
   - **状态：** 已完成；真实群投递和零写边界已验证。
   - **证据：** [飞书主动告警生产闭环](../superpowers/reports/2026-07-21-feishu-proactive-alert-production-verification.md)

7. **飞书专业卡片**
   - **解决的问题：** 告警信息缺少结构化、可扫描的展示形式。
   - **做了什么优化：** 统一专业卡片字段、状态和恢复展示。
   - **影响范围：** 飞书告警阅读体验和运营摘要。
   - **状态：** 已完成；功能主链路生产验证通过。
   - **证据：** [飞书专业卡片生产验证](../superpowers/reports/2026-07-21-feishu-professional-card-production-verification.md)

8. **Sub2API 原生运维页只读投影**
   - **解决的问题：** 自建运维控制面与 Sub2API 原生能力重复且边界复杂。
   - **做了什么优化：** 复用原生 `/admin/ops`，将自建页面收敛为只读投影。
   - **影响范围：** 管理员运维入口、认证和监控展示。
   - **状态：** 已完成；生产页面和认证已验收，受控测试仍按门禁保留。
   - **证据：** [Sub2API 原生运维简化](../superpowers/reports/2026-07-22-sub2api-native-ops-simplification-verification.md)、[原生运维重定向生产验证](../superpowers/reports/2026-07-30-native-ops-reminder-only-production-verification.md)

9. **质量报告与飞书通知**
   - **解决的问题：** 账号和请求质量结果缺少定期汇总和通知出口。
   - **做了什么优化：** 生成只读质量报告并接入生产飞书通知。
   - **影响范围：** 质量巡检、运维摘要和通知。
   - **状态：** 已完成；生产构建、部署和只读验收通过。
   - **证据：** [质量报告飞书生产验证](../superpowers/reports/2026-07-22-quality-report-feishu-production-verification.md)

10. **账号池质量监控及 systemd**
    - **解决的问题：** 账号池健康、错误率和延迟缺少周期性生产采集。
    - **做了什么优化：** 安装周期巡检、结果落盘、单账号隔离和 systemd 调度。
    - **影响范围：** 上游账号池、容量和路由决策。
    - **状态：** 已完成；生产安装和首份结果已复核。
    - **证据：** [账号池质量监控](../superpowers/reports/2026-07-23-account-quality-monitor-verification.md)

11. **模型目录只读监控**
    - **解决的问题：** 模型发布资格和目录变化缺少自动发现与只读可见性。
    - **做了什么优化：** 增加定时发现、原子结果可见性和只读生产监控。
    - **影响范围：** 模型目录、管理员观察和发布前检查。
    - **状态：** 已完成；监控已部署；付费资格发布仍是独立门禁。
    - **证据：** [模型发布只读监控](../superpowers/reports/2026-07-23-model-release-read-only-monitor-verification.md)

12. **运维监控与飞书双域名**
    - **解决的问题：** 监控结果和飞书通知缺少稳定的双域名生产入口。
    - **做了什么优化：** 部署只读监控、双域名访问和成功的 systemd runner。
    - **影响范围：** 监控页、公开入口和飞书通知链路。
    - **状态：** 已完成；生产部署和只读验收通过。
    - **证据：** [运维监控与飞书双域名](../superpowers/reports/2026-07-23-ops-monitoring-and-feishu-dual-domain-verification.md)

13. **Xingqiao 初学者指南**
    - **解决的问题：** 新用户缺少从入口到首次使用的生产说明。
    - **做了什么优化：** 发布初学者指南、同源链接和公开路由。
    - **影响范围：** 公开文档入口和新用户上手流程。
    - **状态：** 已完成；生产文档路由和链接已验收。
    - **证据：** [初学者指南生产验证](../superpowers/reports/2026-07-25-xingqiao-beginner-guide-production-verification.md)

14. **详细指南**
    - **解决的问题：** 进阶用户缺少完整配置、排查和安装说明。
    - **做了什么优化：** 发布详细指南并验证生产路由与安装链接。
    - **影响范围：** 公开文档和用户支持成本。
    - **状态：** 已完成；生产页面和容器保留已验证。
    - **证据：** [详细指南生产验证](../superpowers/reports/2026-07-25-xingqiao-detailed-guide-production-verification.md)

15. **Feishu 用户影响告警**
    - **解决的问题：** 用户可感知的 P0/P1 影响缺少可区分的通知信号。
    - **做了什么优化：** 增加用户影响判断、合成 P0 验收和生产告警行为。
    - **影响范围：** 用户请求质量、管理员告警和响应优先级。
    - **状态：** 已完成；生产行为、健康和合成告警已验证。
    - **证据：** [用户影响告警生产验证](../superpowers/reports/2026-07-28-feishu-user-impact-alerting-production-verification.md)

16. **Monitor 可靠性与管理员可见分组**
    - **解决的问题：** 监控刷新可靠性和管理员视图分组不足。
    - **做了什么优化：** 完成合格镜像、生产提升和管理员可见分组；后续自动刷新实现另列为工程差异。
    - **影响范围：** 原生监控页面、管理员观察和运维判断。
    - **状态：** 已完成；生产 UI、健康和部署提升已验收。
    - **证据：** [Monitor 可靠性与管理员可见性](../superpowers/reports/2026-07-30-monitor-reliability-admin-visibility-production-verification.md)

17. **Monitor V2 7/30 天窗口**
    - **解决的问题：** 监控窗口过短，无法比较短期和长期趋势。
    - **做了什么优化：** 增加 7 天、30 天窗口和桶化结果，并完成生产提升。
    - **影响范围：** 监控统计、管理员诊断和容量判断。
    - **状态：** 已完成；生产提升和结果验证通过。
    - **证据：** [Monitor V2 长窗口](../superpowers/reports/2026-07-30-monitor-v2-long-window-buckets-production-verification.md)

18. **原生运维重定向与飞书纯提醒**
    - **解决的问题：** 旧运维入口和飞书入站控制能力造成边界混淆。
    - **做了什么优化：** 旧 `/ops` 路由重定向到原生 `/admin/ops`，飞书收敛为出站提醒、恢复和日报。
    - **影响范围：** 公网入口、管理员运维和飞书权限边界。
    - **状态：** 已完成；部署、公网审计和生产健康均已通过。
    - **证据：** [原生运维重定向与纯提醒生产验证](../superpowers/reports/2026-07-30-native-ops-reminder-only-production-verification.md)

## 运维/研究跟进（23 项）

以下事项是文档、研究、离线证据、历史验证或外部验收跟进，不因缺少一次工程部署而分类为“工程代码/配置差异待部署”。

1. **离线部署基线**：解决基础设施契约和本地启动验证问题；优化 Compose/Caddy 基线；影响基础设施准备；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-l1-2-offline-baseline-verification.md)。
2. **人工账本模拟**：解决充值、用量和对账规则缺少可重复样例；优化链式事件和模拟校验；影响账务设计；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-manual-ledger-verification.md)。
3. **运营与止损基线**：解决日常检查和止损动作缺少固定节奏；优化 report-only 评估器；影响运营流程；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-operations-baseline-verification.md)。
4. **支付控制模拟**：解决支付状态机和退款幂等规则缺少离线验证；优化禁用态 provider 选择和状态机；影响支付边界；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-payment-control-simulation-verification.md)。
5. **MVP 定价模拟**：解决首版价格、倍率和毛利规则缺少可回归样例；优化定价计算器和配置校验；影响价格策略；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-pricing-simulation-verification.md)。
6. **路由韧性离线基线**：解决重试、熔断和上游评分规则缺少安全基线；优化失败分类和容量建议；影响路由策略；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-routing-resilience-verification.md)。
7. **订阅账号采购比较**：解决账号来源、筛选和验收缺少非敏感比较材料；优化硬淘汰与评分清单；影响采购决策；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-subscription-account-procurement-verification.md)。
8. **上游接入资料包**：解决上游接入所需资料和验收项不完整；优化候选模板、接入表和安全边界；影响上游接入准备；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-15-upstream-intake-verification.md)。
9. **D03 GPT-5.4 Mini 定价**：解决指定模型的价格映射缺少明确基线；优化价格区间和计费映射；影响模型定价；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-17-d03-pricing-verification.md)。
10. **Neko 候选短测**：解决候选上游性能和价格缺少短样本比较；优化短测指标与保守结论；影响上游筛选；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-18-neko-upstream-short-verification.md)。
11. **上游渠道比较/扩容研究**：解决渠道、容量和扩容策略缺少统一研究；优化候选比较与容量建议；影响上游规划；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-18-upstream-channel-comparison.md)、[扩容实践](../superpowers/reports/2026-07-19-gateway-scaling-practices.md)。
12. **Benchmark V2/协议适配器**：解决协议适配和基准测试缺少可复用实现；优化 benchmark v2 与适配器契约；影响上游测试工具；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-21-upstream-benchmark-protocol-adapters-verification.md)。
13. **XM 目录发现与非功能基线**：解决 XM 目录发现和 SSE/拓扑非功能信息不足；优化只读发现与非功能记录；影响目录研究；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-21-xm-upstream-discovery.md)、[非功能基线](../superpowers/reports/2026-07-21-upstream-sse-capacity-and-topology-nonfunctional-verification.md)。
14. **GPT Codex 缓存节省本地实现**：解决缓存节省和计价收益缺少本地计算基线；优化缓存字段、节省估算和验证样例；影响成本分析；**状态：准备完成，不能标记已完成**；[证据](../superpowers/reports/2026-07-22-gpt-codex-cache-savings-verification.md)。

15. **Neko/Wawazz 生产分流门禁**：解决候选上游分流缺少同快照质量门禁；优化账号发现、余额新鲜度和自然质量判定；影响生产路由；**状态：运维跟进，外部质量证据和配置复核未完成**；[证据](../superpowers/reports/2026-07-20-neko-wawazz-production-split-verification.md)、[v3 readiness](../superpowers/reports/2026-07-22-d04-v3-active-upstream-readiness-verification.md)。
16. **D04 受控开放/轻量门禁/v3 readiness**：解决开放注册与活动上游准入条件不清；优化由原生账号列表动态发现并保留只读门禁；**状态：运维跟进，外部阻塞/历史保留；没有新鲜 GO 不得放行**；[证据](../superpowers/reports/2026-07-21-d04-controlled-launch-v2-verification.md)、[轻量门禁](../superpowers/reports/2026-07-22-d04-lightweight-launch-gate-verification.md)、[v3 readiness](../superpowers/reports/2026-07-22-d04-v3-active-upstream-readiness-verification.md)。
17. **生产备份异地副本与 7 天留存**：解决备份异地性、持续性和恢复演练缺口；优化 `pg_dump`、restic 和 R2 验收方案；影响数据库恢复和灾备；**状态：运维跟进，外部阻塞；真实连续 7 天备份与独立恢复待完成**；[证据](../superpowers/reports/2026-07-22-production-backup-restore-verification.md)。
18. **GPT Codex 缓存定价 24 小时门禁**：解决缓存收益实现缺少生产价格和持续观察门槛；优化 24 小时观察与价格变更门禁；影响成本和定价；**状态：运维跟进，待价格观察与验证**；[证据](../superpowers/reports/2026-07-22-gpt-codex-cache-savings-verification.md)。
19. **模型发布资格监控**：解决模型目录发现与真正发布资格混淆；优化只读发现和资格前置检查；影响模型发布；**状态：运维跟进，历史阻塞保留；资格测试、余额和覆盖证据不足**；[证据](../superpowers/reports/2026-07-23-model-release-read-only-monitor-verification.md)、[滚动模型策略](../superpowers/reports/2026-07-22-sub2api-native-rolling-model-policy-verification.md)。
20. **账号倍率监控 SSH 复核**：解决生产账号倍率与监控结果可能漂移；优化只读倍率检查和 SSH 复核清单；影响路由与价格；**状态：运维跟进，待新鲜生产复核**；[证据](../superpowers/reports/2026-07-26-account-monitor-multiplier-verification.md)。
21. **Sub2API 0.1.166 管理员切换**：解决合格镜像已交付但运行版本仍需管理员确认切换；优化官方版本合并、合格镜像和回滚边界；影响网关运行时；**状态：运维跟进，外部阻塞/待管理员切换；不单独计为工程部署差异**；[证据](../superpowers/reports/2026-07-27-sub2api-0.1.166-qualified-image-verification.md)。
22. **无人值守 Sub2API 发布生产激活**：解决自动发现、构建和候选交付尚未进入真实生产激活；优化无秘密构建、强制 SSH 和 compare-and-swap 发布准备；影响发布运维；**状态：运维跟进，待真实 workflow 和生产 forced-command 验收；不单独计为工程部署差异**；[证据](../superpowers/reports/2026-07-28-unattended-sub2api-release-preparation-verification.md)。
23. **Feishu 通知合并生产部署**：解决多通知路径重复和边界不统一；优化通知合并、纯出站契约和本地回归；**状态：运维跟进，待 48 小时生产观察和公网验证；运行时实现已在生产基础上，不能仅因观察未完成称为工程未部署**；[证据](../superpowers/reports/2026-07-29-feishu-notification-consolidation-local-verification.md)。

## 工程代码/配置差异待部署（10 项）

1. **Sub2API 全局 OpenAI 低延迟配置**：生产尚未应用 `cded84a7e` 对应的全局 relay latency 配置；**状态：工程代码/配置差异待部署**。
2. **Sub2API WebSocket `store=false` 会话隔离修复**：生产尚未应用 `c49c0a6ec`；**状态：工程代码/配置差异待部署**。
3. **Monitor 页面自动刷新后端/前端/设置**：生产尚未应用自动刷新实现及其设置暴露；既有 Monitor 可靠性和长窗口能力不等于该实现已部署；**状态：工程代码/配置差异待部署**；[证据](../superpowers/reports/2026-07-30-monitor-reliability-admin-visibility-production-verification.md)、[长窗口](../superpowers/reports/2026-07-30-monitor-v2-long-window-buckets-production-verification.md)。
4. **Caddy/homepage 运行时改动**：生产 Caddy 镜像仍为 `79b0f0a724bb412cfe94d0e2ffb35a4796e4ba7e`，首页运行时改动须独立于 Sub2API 发布；**状态：工程代码/配置差异待部署**；[当前状态](current-state.md)、[0.1.166 合格镜像证据](../superpowers/reports/2026-07-27-sub2api-0.1.166-qualified-image-verification.md)。
5. **全站账务总账**：代码、迁移、受保护账务页面/API、00:10 日调度、清账脚本、本地端到端和隔离 PostgreSQL 验证已进入 `origin/main`；尚未部署、清账激活或生产验证。**状态：工程差异待部署**；[设计](../superpowers/specs/2026-07-31-whole-site-accounting-design.md)、[计划](../superpowers/plans/2026-07-31-whole-site-accounting-ledger-implementation-plan.md)、[本地验证](../superpowers/reports/2026-07-31-whole-site-accounting-ledger-task7-local-verification.md)。
6. **账号上游倍率自动同步**：上游倍率解析后的托管账号回写、审计和缓存同步已合并到 `main`，本地测试已通过；尚未推送、部署或线上验证。**状态：工程差异待部署**；[设计](../superpowers/specs/2026-07-31-account-rate-multiplier-sync-design.md)、[计划](../superpowers/plans/2026-07-31-account-rate-multiplier-sync-implementation-plan.md)。
7. **Monitor V2 卡片信息精简**：已合并删除样本数量、模型列表和接口 `models` 字段，保留核心性能指标；本地前后端测试已通过，尚未推送、部署或生产验证。**状态：工程差异待部署**；[设计](../superpowers/specs/2026-07-31-monitor-v2-card-simplification-design.md)、[计划](../superpowers/plans/2026-07-31-monitor-v2-card-simplification-implementation-plan.md)。
8. **Monitor 当前状态与飞书历史错误码修复**：最新有效渠道探测成功时 Monitor 显示正常，最新失败但近期有成功时显示降级；飞书容量改用账号最新探测状态，最新成功不再继承历史 `balance_exhausted`。实现与回归已进入 `origin/main`，尚未部署或线上验证。**状态：工程差异待部署**；[设计](../superpowers/specs/2026-07-31-monitor-current-status-and-feishu-stale-error-design.md)、[计划](../superpowers/plans/2026-07-31-monitor-status-feishu-alert-fix-implementation-plan.md)。
9. **30 分钟指令驱动 Sub2API 蓝绿发布**：API/worker 角色、双槽 Compose、restart-stable Caddy 路由、停机门禁、恢复/回滚、候选等待、运行态证明和 host deadline 已完成本地实现与 focused 验证；2026-08-01 已补足 controller 向 host executor 传递绝对 deadline 的契约。生产预检确认无 host executor / `release-state`、Caddy 仍是遗留单实例上游，首次切换双槽需停机 bootstrap。**状态：工程差异待部署**；[运行手册](../runbooks/sub2api-blue-green-production-deployment.md)。
10. **`api.xingqiaolab.top` TLS 兼容性修复**：nginx TLS 前置实现已以 `95a81dc37` 推送到 `main`；2026-08-01 生产切换后服务端 TLS 1.2/1.3 均正常，但受影响客户端和 CC Switch 验收仍失败，已完成受控回滚。后续需改用不被干预的兼容域名或更换公网/CDN 入口，nginx 配置当前不在生产生效。**状态：工程差异待部署（前置层方案未通过客户端门禁）**。

## 持续实施（2 项）

1. **原生 P0/P1 Feishu Bridge 独立扩展**：`ops_alert_events` 出站桥接实现尚缺通知策略兼容性和必需的只读数据库接线；**状态：持续实施**；[设计](../superpowers/specs/2026-07-30-native-p0-p1-feishu-bridge-design.md)。
2. **用量明细/支持卡片/上游账号状态等后续运营功能**：仍待设计与实现；**状态：持续实施**；[证据](current-state.md)。

## 总账维护规则

以后每次任务都必须在实施前先登记到本文件，并将初始状态写为“进行中”。任务收尾时，只有完成服务端推送、生产部署和生效验证，才可转入“生产工程代码/配置已部署并验证”；仍存在运行时代码或生产配置差异的事项进入“工程代码/配置差异待部署”，尚未完成实现的进入“持续实施”，文档、研究、离线证据和外部验收进入“运维/研究跟进”，不得仅因这些材料缺失而声称工程未部署。每次收尾同时更新统计摘要和 current-state 入口。
