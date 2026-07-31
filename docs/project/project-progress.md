# 项目全局进度总账

**更新时间：** 2026-07-31

**唯一总账：** 本文件是项目事项、部署状态和验证证据的唯一全局入口。

## 统计口径

本次回填审阅了 `docs/superpowers/reports/` 下截至 2026-07-30 的 60 份报告，并结合项目当前状态和 Git 历史，将同一功能的阶段性报告合并为 45 个独立事项。只有同时满足“已推送到服务端 + 已部署 + 已验证生效”才计入“生产已部署并验证”。本地测试、离线准备、代码合并、镜像生成、候选资格或报告完成，均不能标记为已完成。事项按独立交付结果合并统计，不按报告文件数量统计；历史暂停或阻塞事项保留在进行中区，不制造完成结论。

## 统计摘要

| 分区 | 独立事项 | 判定 |
|---|---:|---|
| 生产已部署并验证 | 18 | 已推送、已部署、已验证生效 |
| 仅本地/离线准备 | 14 | 有本地或离线证据，不能标记已完成 |
| 待部署/待验证/阻塞 | 13 | 进行中；必要时注明外部阻塞/历史保留 |
| **合并后的独立事项合计** | **45** | **当前总账快照** |

## 当前最重要进行中事项

1. **Neko/Wawazz 生产分流门禁**：生产配置和质量证据仍需同快照复核，当前不能以历史样本替代新鲜 `GO`。
2. **D04 受控开放与 v3 readiness**：开放注册由 Sub2API 原生能力控制，v3 活动账号和质量门禁尚未形成可放行结论。
3. **生产备份异地副本与 7 天留存**：方案和验收清单已有，真实连续备份、异地副本及独立恢复演练仍待完成。
4. **无人值守发布、Feishu 合并部署和原生 P0/P1 Bridge**：代码或本地验证已具备，但生产激活/部署/公网验证尚未全部闭环。
5. **全站账务总账与后续运营功能**：用量、支持、上游状态、首页转化和品牌更新仍属于后续运营交付，不计入已完成。

## 生产已部署并验证（18 项）

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
    - **做了什么优化：** 完成合格镜像、生产提升、可靠刷新和管理员可见分组。
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

## 仅本地/离线准备（14 项）

以下事项有代码、模拟、离线报告或候选研究证据，但没有同时满足生产推送、部署和生效验证，**不能标记为已完成**。

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

## 待部署/待验证/阻塞（13 项）

1. **Neko/Wawazz 生产分流门禁**：解决候选上游分流缺少同快照质量门禁；优化账号发现、余额新鲜度和自然质量判定；影响生产路由；**状态：进行中，外部质量证据和配置复核未完成**；[证据](../superpowers/reports/2026-07-20-neko-wawazz-production-split-verification.md)、[v3 readiness](../superpowers/reports/2026-07-22-d04-v3-active-upstream-readiness-verification.md)。
2. **D04 受控开放/轻量门禁/v3 readiness**：解决开放注册与活动上游准入条件不清；优化由原生账号列表动态发现并保留只读门禁；**状态：进行中，外部阻塞/历史保留；没有新鲜 GO 不得放行**；[证据](../superpowers/reports/2026-07-21-d04-controlled-launch-v2-verification.md)、[轻量门禁](../superpowers/reports/2026-07-22-d04-lightweight-launch-gate-verification.md)、[v3 readiness](../superpowers/reports/2026-07-22-d04-v3-active-upstream-readiness-verification.md)。
3. **生产备份异地副本与 7 天留存**：解决备份异地性、持续性和恢复演练缺口；优化 `pg_dump`、restic 和 R2 验收方案；影响数据库恢复和灾备；**状态：进行中，外部阻塞；真实连续 7 天备份与独立恢复待完成**；[证据](../superpowers/reports/2026-07-22-production-backup-restore-verification.md)。
4. **GPT Codex 缓存定价 24 小时门禁**：解决缓存收益实现缺少生产价格和持续观察门槛；优化 24 小时观察与价格变更门禁；影响成本和定价；**状态：进行中，待部署/待验证**；[证据](../superpowers/reports/2026-07-22-gpt-codex-cache-savings-verification.md)。
5. **模型发布资格监控**：解决模型目录发现与真正发布资格混淆；优化只读发现和资格前置检查；影响模型发布；**状态：进行中，历史阻塞保留；资格测试、余额和覆盖证据不足**；[证据](../superpowers/reports/2026-07-23-model-release-read-only-monitor-verification.md)、[滚动模型策略](../superpowers/reports/2026-07-22-sub2api-native-rolling-model-policy-verification.md)。
6. **账号倍率监控 SSH 复核**：解决生产账号倍率与监控结果可能漂移；优化只读倍率检查和 SSH 复核清单；影响路由与价格；**状态：进行中，待新鲜生产复核**；[证据](../superpowers/reports/2026-07-26-account-monitor-multiplier-verification.md)。
7. **Sub2API 0.1.166 管理员切换**：解决合格镜像已交付但运行版本仍需管理员确认切换；优化官方版本合并、合格镜像和回滚边界；影响网关运行时；**状态：进行中，外部阻塞/待管理员切换**；[证据](../superpowers/reports/2026-07-27-sub2api-0.1.166-qualified-image-verification.md)。
8. **无人值守 Sub2API 发布生产激活**：解决自动发现、构建和候选交付尚未进入真实生产激活；优化无秘密构建、强制 SSH 和 compare-and-swap 发布准备；影响发布运维；**状态：进行中，待真实 workflow 和生产 forced-command 验收**；[证据](../superpowers/reports/2026-07-28-unattended-sub2api-release-preparation-verification.md)。
9. **Feishu 通知合并生产部署**：解决多通知路径重复和边界不统一；优化通知合并、纯出站契约和本地回归；**状态：进行中，待生产部署和公网验证**；[证据](../superpowers/reports/2026-07-29-feishu-notification-consolidation-local-verification.md)。
10. **原生 P0/P1 Feishu Bridge**：解决原生运维 P0/P1 事件尚缺生产出站桥接；优化只读事件、持续提醒、恢复和去重设计；**状态：进行中，待部署/待验证**；[证据](../superpowers/reports/2026-07-30-native-ops-redirect-and-reminder-only-feishu-verification.md)。
11. **全站账务总账**：解决用量、充值、余额、成本和对账分散；优化统一账务数据模型和收口入口；影响全站财务与运营；**状态：进行中，待实现、部署和三方验收**；[证据](../superpowers/reports/2026-07-15-manual-ledger-verification.md)、[当前状态](current-state.md)。
12. **首页 CTA/主题/品牌/更新准备**：解决首页转化、品牌一致性和更新提示尚未收口；优化 CTA、主题、品牌资产和版本更新准备；影响公开首页和用户转化；**状态：进行中，待生产发布与验收**；[证据](current-state.md)、[0.1.166 合格镜像](../superpowers/reports/2026-07-27-sub2api-0.1.166-qualified-image-verification.md)。
13. **用量明细/支持卡片/上游账号状态等后续运营功能**：解决用户和管理员缺少细粒度用量、支持入口和上游状态视图；优化后续运营功能清单与原生能力复用方向；影响用户支持和运营效率；**状态：进行中，待设计、实现、部署和验证**；[证据](current-state.md)、[Sub2API 原生运维简化](../superpowers/reports/2026-07-22-sub2api-native-ops-simplification-verification.md)。

## 总账维护规则

以后每次任务都必须在实施前先登记到本文件，并将初始状态写为“进行中”。任务收尾时，只有完成服务端推送、生产部署和生效验证，才可转入“生产已部署并验证”；否则保留在“仅本地/离线准备”或“待部署/待验证/阻塞”，并补充最新证据。每次收尾同时更新统计摘要和 current-state 入口。
