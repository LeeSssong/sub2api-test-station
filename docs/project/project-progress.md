# 项目全局进度总账

**更新时间：** 2026-07-31

**唯一总账：** 本文件是项目事项、部署状态和验证证据的唯一全局入口。

## 统计口径

本次回填审阅了 `docs/superpowers/reports/` 下截至 2026-07-30 的 60 份报告，并结合项目当前状态和 Git 历史，将同一功能的阶段性报告合并为 49 个独立事项。只有同时满足“已推送到服务端 + 已部署 + 已验证生效”才计入“生产工程代码/配置已部署并验证”。只有仍存在运行时代码或生产配置差异，才计入“工程代码/配置差异待部署”；文档、研究、报告、离线证据、历史验证和外部验收本身不构成工程部署差异。事项按独立交付结果合并统计，不按报告文件数量统计；历史暂停或阻塞事项保留在进行中区，不制造完成结论。

## 统计摘要

| 分区 | 独立事项 | 判定 |
|---|---:|---|
| 生产工程代码/配置已部署并验证 | 18 | 已推送、已部署、已验证生效 |
| 工程代码/配置差异待部署 | 4 | 仍存在运行时代码或生产配置差异 |
| 持续实施 | 5 | 工程实现尚未完成，或仍在推进中 |
| 运维/研究跟进（非工程部署差异） | 23 | 文档、研究、历史证据或外部验收跟进 |
| **合并后的独立事项合计** | **50** | **当前总账快照** |

## 当前最重要进行中事项

1. **2026-07-31 main 全局低延迟配置蓝绿发布与待部署事项联合评估**：`main@c49c0a6ec` 已推送并构建镜像，但其 relay-ops 与生产遗留通知策略不兼容，两次原地重建均导致 Caddy 被依赖链阻塞、生产公网中断。事故恢复已将生产恢复到旧版本；先完成蓝绿发布设计、兼容性门禁和逐项工程差异审计，设计获确认前不修改生产。**状态：进行中（设计与审计）**；`accounting-ledger` 继续排除。
2. **2026-07-31 生产站点中断恢复**：11:56 首次故障后于 13:28 回滚恢复；14:10–14:11，“调度优化”任务误将事故回滚判断为外部覆盖，再次覆盖 Compose 并强制重建 Sub2API/relay-ops/Caddy，造成第二次中断。14:15 已再次恢复至 11:53 发布前快照；公网健康连续 5 次返回 200，首页、价格和文档入口正常，五个容器运行正常且重启计数为 0。**状态：恢复已验证；永久兼容性修复仍未推送、部署和验证**。
3. **Sub2API 低延迟/会话隔离/Monitor 自动刷新工程差异**：生产仍缺全局 OpenAI 低延迟配置、WebSocket `store=false` 会话隔离修复和 Monitor 自动刷新后端/前端/设置实现。**状态：工程代码/配置差异待部署**。
4. **relay-ops 原生 P0/P1 Bridge 与蓝绿发布机制**：Bridge 实现仍缺通知策略兼容性和必需的只读数据库接线；可复用蓝绿机制尚不存在。**状态：持续实施**。
5. **Caddy/homepage 运行时差异与全站账务总账**：Caddy/homepage 运行时改动须独立发布；`accounting-ledger` 仍在 `codex/accounting-ledger`，未合并。**状态：工程差异待部署/持续实施**。
6. **Neko/Wawazz 门禁、D04 readiness、备份留存和后续运营**：均保留最新证据和外部验收跟进，不因文档或历史报告单独记为工程未部署。
7. **账号上游倍率自动同步**：上游 billing probe 已解析账号有效倍率，但当前只写入观测快照，未回写 `accounts.rate_multiplier`；本次将增加托管/手工覆盖策略、审计、缓存同步和账号生命周期/定时触发。**状态：进行中；待代码实现、推送、部署和线上验证**。

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

## 工程代码/配置差异待部署（4 项）

1. **Sub2API 全局 OpenAI 低延迟配置**：生产尚未应用 `cded84a7e` 对应的全局 relay latency 配置；**状态：工程代码/配置差异待部署**。
2. **Sub2API WebSocket `store=false` 会话隔离修复**：生产尚未应用 `c49c0a6ec`；**状态：工程代码/配置差异待部署**。
3. **Monitor 页面自动刷新后端/前端/设置**：生产尚未应用自动刷新实现及其设置暴露；既有 Monitor 可靠性和长窗口能力不等于该实现已部署；**状态：工程代码/配置差异待部署**；[证据](../superpowers/reports/2026-07-30-monitor-reliability-admin-visibility-production-verification.md)、[长窗口](../superpowers/reports/2026-07-30-monitor-v2-long-window-buckets-production-verification.md)。
4. **Caddy/homepage 运行时改动**：生产 Caddy 镜像仍为 `79b0f0a724bb412cfe94d0e2ffb35a4796e4ba7e`，首页运行时改动须独立于 Sub2API 发布；**状态：工程代码/配置差异待部署**；[当前状态](current-state.md)、[0.1.166 合格镜像证据](../superpowers/reports/2026-07-27-sub2api-0.1.166-qualified-image-verification.md)。

## 持续实施（4 项）

1. **可复用 Sub2API 蓝绿发布机制**：当前不存在可复用的蓝绿机制；需先完成候选、兼容性门禁、回滚和切换设计与实现；**状态：持续实施**。
2. **原生 P0/P1 Feishu Bridge 独立扩展**：`ops_alert_events` 出站桥接实现尚缺通知策略兼容性和必需的只读数据库接线，当前实现不安全；**状态：持续实施**；[设计](../superpowers/specs/2026-07-30-native-p0-p1-feishu-bridge-design.md)、[计划](../superpowers/plans/2026-07-30-native-p0-p1-feishu-bridge-implementation-plan.md)。
3. **全站账务总账**：`codex/accounting-ledger` 仍在进行中，未合并到 `main`；Task 1–3 已实现并通过独立复审，Task 4 受保护的极简现金台账与日报页面/API 正在实施，仍待后续实现、推送、部署和三方验收；**状态：持续实施**；[设计](../superpowers/specs/2026-07-31-whole-site-accounting-ledger-design.md)、[计划](../superpowers/plans/2026-07-31-whole-site-accounting-ledger-implementation-plan.md)、[历史证据](../superpowers/reports/2026-07-15-manual-ledger-verification.md)、[当前状态](current-state.md)。
4. **用量明细/支持卡片/上游账号状态等后续运营功能**：仍待设计、实现、部署和验证；**状态：持续实施**；[证据](current-state.md)、[Sub2API 原生运维简化](../superpowers/reports/2026-07-22-sub2api-native-ops-simplification-verification.md)。
5. **账号上游倍率自动同步**：解决 billing probe 结果与实际计费倍率漂移；将同步 `effective_rate_multiplier` 到托管账号的 `accounts.rate_multiplier`，保留手工覆盖并刷新缓存/调度器快照。**状态：进行中；尚待代码实现、服务端推送、生产部署和生效验证**；[设计](../superpowers/specs/2026-07-31-account-rate-multiplier-sync-design.md)、[计划](../superpowers/plans/2026-07-31-account-rate-multiplier-sync-implementation-plan.md)。

## 总账维护规则

以后每次任务都必须在实施前先登记到本文件，并将初始状态写为“进行中”。任务收尾时，只有完成服务端推送、生产部署和生效验证，才可转入“生产工程代码/配置已部署并验证”；仍存在运行时代码或生产配置差异的事项进入“工程代码/配置差异待部署”，尚未完成实现的进入“持续实施”，文档、研究、离线证据和外部验收进入“运维/研究跟进”，不得仅因这些材料缺失而声称工程未部署。每次收尾同时更新统计摘要和 current-state 入口。
