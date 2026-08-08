# 项目全局进度总账

**本轮登记（2026-08-08）：** 非 main 工作区整合与发布流程收口，状态：进行中（已保护“新建运营界面”和“优化账号卡片”两个活动线程所在的当前 main 工作区；其余非 main worktree 正在建立可恢复快照、分类并整合到独立候选，待活动线程完成后再并回 main、推送、部署和线上验证）。

**更新时间：** 2026-08-08

**本轮登记（2026-08-08）：** 账号监控卡片统一探测口径及四项交互增强，状态：进行中（用户已确认账号卡片主指标、成功率、TTFT、总耗时、评分和证据全部以主动探测为唯一来源；同时加入成本来源警示、账号名称上游外链、评分构成悬浮展示。仅允许相关专项测试、构建、蓝绿无停机部署和线上页面/API 验证；尚未推送、部署或线上验证，不得标记完成）。

**本轮登记（2026-08-08）：** 账号运营损益页已完成本地实现并整理到提交，状态：进行中（已推送待部署；后端接口 `/api/v1/admin/operations/account-profitability` 与前端 `/admin/operations/account-profitability` 兼容 sub2api、newapi、自购账号。收入按 `actual_cost`，中继支出按账号真实成本，采购成本按 CNY 单独展示；未配置汇率时自购账号利润/利润率保持待换算，避免混币误算）。

**本轮继续登记（2026-08-08）：** 账号盈利页仍待生产环境部署；官方升级流程正在收口为“管理员触发候选准备 → readiness 可观测 → 管理员二次确认后蓝绿切换”。不得恢复 GitHub Actions 发布链，也不得在候选未 ready 时执行生产切换；当前状态：进行中。

**本轮实现登记（2026-08-08）：** 候选准备 POST、幂等状态机、失败/目标变化重试、readiness 阶段/失败原因和手动 UI 触发已完成本地实现；管理页刷新后会恢复 preparing/ready/failed 状态，updater 主程序支持注入受控的 root-owned host preparer 命令，未配置时明确返回“候选准备器未配置”，不再静默卡在准备中。后端 `go test ./...`、race、vet、前端 33 项 UI 测试与 updater 打包契约通过；生产 host preparer 实体、候选状态持久化评估、生产凭据、部署和线上验证仍未完成，状态保持进行中。

**本轮登记（2026-08-07）：** 升级弹窗在当前版本已等于目标版本时仍显示残留失败状态，状态：已完成（修复提交 `177bab334` 已推送至工作分支和 `main`，缓存版本 `20260807-2` 已部署；公网 HTML/JS、新文案、`current_version=latest_version=0.1.171`、`has_update=false`、`/health=200`、22 个模型及受保护容器身份/重启次数均已复验；管理员强制刷新后的生产截图显示 `v0.1.171`、绿色成功标记和“已是最新版本”，不再显示残留的“升级失败”状态）。

**本轮推进（2026-08-07）：** 管理页官方升级适配蓝绿生产拓扑已完成。修复提交 `ef6e12560`、`57793c0fd` 已推送；候选 `0.1.171`（提交 `20482a733af8caa40fde277c28c5df35c1ff08b4`，镜像 `sha256:bcb9a659…`）已通过完整资格矩阵、生产暂存和蓝绿切换；更新器已安装并校验，线上版本、健康接口、模型列表和共享容器身份均已复验。

**本轮登记（2026-08-07）：** 管理页官方升级适配蓝绿生产拓扑，状态：已完成（首次旧单容器执行器失败已修复；`0.1.171` 候选通过生产安装、预加载暂存和蓝绿切换；公网 `/health`、管理员 system/version、`/v1/models` 与 worker 健康均已验证，PostgreSQL、Redis、Caddy 容器 ID、启动时间和重启次数未变化）。

**本轮登记（2026-08-07）：** Sub2API `0.1.171` 生产候选登记，状态：已完成（候选源码、镜像标签、二进制版本、`linux/amd64`、迁移哈希和生产不可变暂存均已核对；蓝槽与 worker 已切换并健康运行，线上版本为 `0.1.171`）。

**本轮登记（2026-08-07）：** 升级弹窗当前进度状态可见性修复，状态：已完成（提交 `0829cf0639af13cc6913f129fcfcefe9ade86b4d` 已推送并部署；生产管理页已加载缓存版本 `20260807-1`，公网 JS/CSS 哈希分别为 `b2fb7a62…`、`200be5f4…`，健康检查 3/3 通过；PostgreSQL、Redis、Caddy 容器 ID、启动时间与重启次数均未变化）。

**本轮登记（2026-08-07）：** 账号监控分组切换误退出与成本倒挂提醒可见性复验，状态：进行中（生产审计已确认 relay-ops 经内部 Caddy 校验管理员会话时覆盖原始客户端 IP，触发 `/api/v1/auth/me` 会话绑定不匹配并撤销会话；前端会话恢复隔离已部署，内部 Caddy 客户端身份透传修复已完成实现、回归与独立审查，待推送、部署并以成本接口 200 和生产视觉检查验收）。

**本轮登记（2026-08-08）：** 账号监控「成本折合本站倍率」简化，状态：已完成（提交 `ff5d183a6` 已推送至 `origin/main` 并完成蓝绿生产部署，切换至 green 槽位且 `downtime_required=false`；无数据库迁移。聚焦后端/前端测试、类型检查、生产构建和独立复审通过；线上 `/health` 正常，账号监控 API 返回最新探测模型与 `equivalent_site_multiplier`，旧成本字段未返回，PostgreSQL/Redis/Caddy/Worker 健康且身份未变化）。

**本轮登记（2026-08-08）：** 账号监控「成本折合本站倍率」简化，状态：进行中（用户已确认移除上游原生倍率、当前分组倍率、状态、模型、样本和账单对账展示；改为按最新探测模型价格、账号有效倍率与本站标准价格计算单一折合倍率。已完成本地实现、聚焦测试、类型检查、生产构建和独立复审；尚未推送、部署或生产验证，不得标记完成）。

**本轮登记（2026-08-06）：** 账号监控评分权重入口、账号成本/预计额度与上游余额刷新修复，状态：进行中（原 Tasks 1-4 已完成本地实现和逐任务独立审查；原整体验证与审查曾通过并推送。生产完整发布链随后已同步进候选；最新官方倍率能力复核发现自定义策略和 measurement value 会造成监控与真实计费分叉，现新增 Task 5 原生倍率收敛与数据迁移，Task 6 负责重新整体验证、推送和生产门禁。部署和线上验证前不得标记完成）。

**本轮继续（2026-08-06）：** 已确认根因是候选基于 `origin/main@69caeaf8`，而生产运行独立合格发布链 `9aab62c2`，该生产链有 8 个提交未进入候选。现按用户授权进入“完整同步生产发布链、解决真实冲突、重新审查迁移集合与整分支”的阶段；禁止仅手工补 SQL、跳过生产代码或在复审前部署。

**生产同步结果（2026-08-06）：** 完整生产发布链已通过合并提交 `f64f93e4c` 纳入候选，integration 接口兼容修复 `3016c6951` 已完成独立复审。合并后的迁移集合当时为“生产完整集合 + 197”，规范哈希 `9f341792…`；该哈希已因新增 Task 5 清理迁移而失效，必须在 Task 5 完成后重新计算，当前不得执行生产门禁。

**原生倍率收敛（2026-08-06）：** 用户确认采用官方 `accounts.rate_multiplier + upstream_billing_rate_sync_enabled` 单一语义，不兼容运行时旧策略；新增一次性数据迁移把旧 `manual_override/upstream_managed` 转成官方同步布尔值并删除 `upstream_billing_rate_multiplier_policy` 与 `account_monitor_multiplier_measurement`。独立审查已确认当前候选存在策略冲突和监控/真实计费倍率分叉，现登记为 Task 5 进行中；完成实施、独立复审、整分支复审和推送前不重新执行生产门禁。

**Task 5 实施结果（2026-08-06）：** 原生倍率收敛提交 `ded650e06` 已完成；迁移 198 保留 `accounts.rate_multiplier`、转换官方 sync 开关并直接删除两个旧 JSON 键。生产只读盘点为 74 个有效账号、38 个旧策略（全部 `upstream_managed`）、16 个 measurement、29 个 OpenAI API Key 自动探测账号。后端测试/vet、前端聚焦测试/lint/typecheck/build、diff 与旧符号搜索均通过；当前保持进行中，待推送和生产门禁。

**本轮推进（2026-08-06）：** Task 3 已完成本地实现与独立复审，Task 4 成本弹窗与余额卡片进入实施；本轮仍保持进行中，尚未推送、部署或线上验证。

**本轮登记（2026-08-05）：** 六阶段生产收口独立部署执行，状态：待办（用户暂停）。Task 1 基线收口已完成；Task 2 已部署但尚待用户验收；Task 3 至 Task 9 均未启动。恢复前不得派发实施代理、生成下一任务 brief、修改生产配置或执行部署。执行入口为 [独立部署单元实施计划](../superpowers/plans/2026-08-05-six-stage-production-closure-deployment-units.md)，恢复时所有代理仍须先读取 [代理上下文合同](six-stage-production-closure-agent-context.md) 并报告固定 `CONTEXT_ACK`。

**Task 2 部署登记（2026-08-05）：** 状态：待办（已部署、待用户验收、当前暂停）。提交 `8fef0e03c80a55ec1a1cceedabd1949bf12bfe8b` 已推送到 canonical 分支和 `origin/main`，relay-ops 不可变镜像已部署，生产 accounting 唯一显式保持 `false`；镜像/源码/迁移身份、`/healthz`、`/readyz`、disabled accounting 404、未认证 reconciliation 401 及 PostgreSQL/Redis/Caddy/Sub2API 容器身份不变均已验证。恢复时第一步只能验收 Task 2；验收前不得标记完成，不得启动 Task 3。

**六阶段剩余任务待办登记（2026-08-05）：**

| 任务 | 待办状态 |
|---|---|
| Task 2 relay-ops 账务代码部署（accounting disabled） | 待办：已部署，待用户验收，暂停 |
| Task 3 激活 accounting、账本基线与日调度 | 待办：未启动 |
| Task 4 全站账单能力只读盘点 | 待办：未启动 |
| Task 5 必要平台账单适配器子计划 | 待办：条件任务，未启动 |
| Task 6 真实账单授权、映射与首个非零闭环 | 待办：未启动 |
| Task 7 独立营收页面 | 待办：未启动 |
| Task 8 Monitor 与飞书告警/恢复 | 待办：未启动 |
| Task 9 OpenAI 实际响应模型审计 | 待办：未启动 |

**当前生产交付序列：** 用户已暂停六阶段剩余任务，当前没有活动实施或部署单元。账号监控 V3 已完成；Task 2 保留已部署、accounting disabled、待验收事实并归入待办；Task 3 至 Task 9 全部归入待办。只有用户明确恢复后，协调任务才能从 Task 2 验收门继续。

**本轮登记（2026-08-02）：** 账号监控真实上游成本与对账闭环恢复核验，状态：进行中（仅在 `codex/relay-ops-public-route` 分支执行；先核对生产 Caddy 热配置、健康/认证路由、发布锁和真实只读账单输入；未满足“已推送 + 已部署 + 真实数据验证”前不得标记完成）。

**本轮登记（2026-08-03）：** 账号监控产品验收收口，状态：进行中（账号范围、聚合与首轮卡片修复已推送并部署；生产复验新增确认：监控分组查询遗漏软删除过滤，错误展示 10 个已删除历史/测试分组；账号卡片须限制桌面每行最多 2 个、收起今日调用、评分显示整数并改用通俗证据文案、弱化评分与全局优先级、补齐渠道监控式柱状图和各指标样本数；“纸面利润”统一改为“账号利润”；渠道监控成功探测统一按绿色成功展示并恢复指标样本数；“历史按日”“异常明细”及账号监控其他交互须逐项验证修复。全部重新推送、部署并在线验证前不得标记完成）。

**本轮登记（2026-08-04）：** 账号监控卡片收敛与营收页面职责拆分，状态：已完成（最终提交 `05985e62ec88b04d1e647a815eecdb1cf1155776` 已推送并部署至生产 green；账号监控继续只承接服务质量、评分、排名、成本录入、倍率与并发，营收、利润、账务和对账保持独立页面/后续事项。#118/#119 均已在线确认 `service_state=available`、可评分排名、24 小时失败数为 0，单卡刷新后检查时间同步推进）。

**本轮生产实施登记（2026-08-04）：** 账号监控卡片数据完善 V3，状态：已完成（生产 `release-state` 已更新为 `source_commit=05985e62ec88b04d1e647a815eecdb1cf1155776`、`source_tree=c37b383bf54e485d7393ff0793e30dd03f5e2328`、`migrations_hash=337212b4af85839c9497d0fef3153e5c858bd976fed268086459c21a12abcc76`、`active_slot=green`，运行镜像 ID 为 `sha256:0d10260b745e2086326977303b15f6eb78e8e03de7858fe356dec046bf0e10e8`。PostgreSQL `2db52788…`、Redis `c45202c0…`、Caddy `ace4a23b…` 身份未变化；历史三个时间窗、7 个分组、66 个账号、迁移与并发验证事实继续有效，最终状态与刷新缺陷已完成线上复验）。

**本轮视觉纠偏登记（2026-08-04）：** 账号监控 V3 设计稿 1:1 还原，状态：已完成（不可漂移合同、独立审查、桌面/移动同视口对比和既有生产卡片验证结论全部保留；本轮状态与单卡刷新缺陷已随最终提交完成生产收口，账号监控 V3 整体交付恢复为已完成）。

**唯一总账：** 本文件是项目事项、部署状态和验证证据的唯一全局入口。

## 统计口径

本次回填审阅了 `docs/superpowers/reports/` 下截至 2026-07-30 的 60 份报告，并结合项目当前状态和 Git 历史，将同一功能的阶段性报告合并为 49 个独立事项。只有同时满足“已推送到服务端 + 已部署 + 已验证生效”才计入“生产工程代码/配置已部署并验证”。只有仍存在运行时代码或生产配置差异，才计入“工程代码/配置差异待部署”；文档、研究、报告、离线证据、历史验证和外部验收本身不构成工程部署差异。事项按独立交付结果合并统计，不按报告文件数量统计；历史暂停或阻塞事项保留在进行中区，不制造完成结论。

## 统计摘要

| 分区 | 独立事项 | 判定 |
|---|---:|---|
| 生产工程代码/配置已部署并验证 | 20 | 已推送、已部署、已验证生效 |
| 工程代码/配置差异待部署 | 10 | 仍存在运行时代码或生产配置差异 |
| 持续实施 | 2 | 工程实现尚未完成，或仍在推进中 |
| 运维/研究跟进（非工程部署差异） | 23 | 文档、研究、历史证据或外部验收跟进 |
| **合并后的独立事项合计** | **55** | **当前总账快照** |

## 当前最重要进行中事项与本轮已完成交付

0.0. **账号监控成本、余额与评分权重修复**：恢复分组评分权重编辑入口；OpenAI API Key 账号统一使用官方 `accounts.rate_multiplier`，OpenAI 非 API Key 账号使用采购成本和预计可用额度计算等效倍率；余额跟随账号健康探测，单卡刷新同时执行官方 billing probe。**状态：已完成（最终提交 `c12f930b1` 已推送、维护迁移已部署、健康/监控 API/权重/倍率/余额已线上验证）**。
0. **OpenAI 模型映射审计**：仅覆盖 OpenAI-compatible HTTP JSON、SSE 与 Responses WebSocket；新增 `usage_logs.actual_response_model`，记录上游原始响应模型并在管理员 usage log 展示。**状态：进行中（本地实现与验证阶段，尚未推送、部署或线上验证）**。
0.1. **账号监控 V3：账号质量、评分、排名与调度信息**：2026-08-04 最终确认已覆盖旧的“账号监控同时承接经营/账务”方案；账号监控生产页只承接原生分组 Tab、七项分组汇总、真实请求时间窗、完整六段式账号卡片、卡内全局优先级、一次性采购成本、倍率与原生并发，营收、利润、账务和对账仍是独立后续事项。最终提交 `05985e62ec88b04d1e647a815eecdb1cf1155776` 已推送并部署至绿色槽位；#118/#119 均以真实请求窗口投影为可用、保留评分排名，单卡刷新请求成功且检查时间推进。**状态：已完成（已推送、已部署、已验证生效）**。

0.4. **账号卡片成本倒挂预警**：在管理员账号监控卡片接入 Sub2API/New API 通用上游原生倍率、本站标准费用折合成本倍率、逐笔账单证据和当前分组倒挂判定；至少 6 笔有效逐笔实际扣费样本才标记“确认亏损”，仅定价推算/额度测得标记“可能亏损”。本次不创建站点经营看板，不新增或迁移现有金额型投入产出区。**状态：进行中（已完成本地实现并合并远端最新代码，待推送、生产部署和线上验证）**。

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
12. **历史账号监控分组经营与评分方案（已被 V3 边界覆盖）**：该旧方案曾把账号经营数据、账务历史和对账入口放入账号监控页，并形成候选提交与发布预检证据。2026-08-04 最新确认已废止其页面职责：旧后端账务能力保留为未来“营收”页面输入，但不得恢复到 `/admin/accounts/monitor`；当前账号监控只实施账号质量、评分、排名和调度信息。**状态：历史方案停止继续实施；当前交付状态统一以上方 0.1 与 V3 不可漂移合同为准，不得把旧候选当作当前可部署结果**。
## 生产工程代码/配置已部署并验证（20 项）

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

19. **账号监控 V3 生产闭环**
    - **解决的问题：** 账号监控页未按最终职责边界完整呈现真实分组、评分、排名、成本与并发，且采购成本迁移需要受控维护发布。
    - **做了什么优化：** 部署最终 V3 卡片、三个真实请求窗口、原生分组汇总、采购成本/倍率模式、稳定排名和实时并发；以绿色槽位完成采购成本附加迁移。
    - **影响范围：** `/admin/accounts/monitor`、账号监控 API、账户采购成本迁移与蓝绿 API/Worker 切换。
    - **状态：** 已完成；最终提交 `05985e62ec88b04d1e647a815eecdb1cf1155776` 已推送、生产部署且状态/单卡刷新缺陷线上验证生效。
    - **证据：** [V3 生产闭环证据](../../.superpowers/sdd/2026-08-04-account-monitor-card-production-implementation-plan/production-verification.md)。

20. **账号监控成本折合本站倍率简化**
    - **解决的问题：** 成本区域展示了重复的上游倍率、分组倍率、样本和对账字段，且折合成本未严格使用最新探测模型。
    - **做了什么优化：** 仅保留成本折合本站倍率；按最新探测模型价格、账号有效倍率和本站模型价格计算，缺少有效价格显示 `--`。
    - **影响范围：** `/admin/accounts/monitor` 账号卡片成本区域及账号监控 API。
    - **状态：** 已完成；`ff5d183a6` 已推送、蓝绿部署并通过线上健康、接口和容器身份验收，无数据库迁移、无停机。
    - **证据：** [本轮发布证据](/Users/gongtengxinwen/Documents/sub2api搭建/release-evidence/sub2api-account-monitor-equivalent-cost-ff5d183a.json)。

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
22. **Sub2API 受控本地发布流程**：GitHub Actions 发布 workflow 已按管理员要求退役；受控 worktree 的本地候选链已完成 `discover -> merge -> qualify -> stage -> advance`，并通过生产蓝绿切换和线上验证。**状态：已完成；已推送、已部署、已验证生效**。
23. **Feishu 通知合并生产部署**：解决多通知路径重复和边界不统一；优化通知合并、纯出站契约和本地回归；**状态：运维跟进，待 48 小时生产观察和公网验证；运行时实现已在生产基础上，不能仅因观察未完成称为工程未部署**；[证据](../superpowers/reports/2026-07-29-feishu-notification-consolidation-local-verification.md)。

## 工程代码/配置差异待部署（11 项）

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
11. **账号监控成本、余额、评分权重与官方倍率收敛**：已恢复分组评分权重入口，OpenAI API Key 统一使用官方 `accounts.rate_multiplier` 与 `extra.upstream_billing_rate_sync_enabled`，非 API Key 使用人民币采购成本和预计可用 USD 额度；支持上游余额、单卡强制刷新，并通过迁移 198 删除旧策略/measurement 数据。自动倍率展示现已读取官方 probe snapshot 的 `status`、`received_at`、`fresh_until`，过期/失败会明确标记。最终提交 `c12f930b1` 已推送并部署；生产迁移哈希已为 `0204f39423f3218ffa0c8d4e3d665f7113c4990610e0dd22e9f5910c4d578c6d`，健康接口、监控 API、分组权重、native 倍率和余额快照均已线上验证。**状态：已完成；已推送、已部署、已验证生效**；[设计](../superpowers/specs/2026-08-06-account-monitor-cost-balance-and-score-weights-design.md)、[计划](../superpowers/plans/2026-08-06-account-monitor-cost-balance-and-score-weights-implementation-plan.md)。

## 持续实施（2 项）

1. **原生 P0/P1 Feishu Bridge 独立扩展**：`ops_alert_events` 出站桥接实现尚缺通知策略兼容性和必需的只读数据库接线；**状态：持续实施**；[设计](../superpowers/specs/2026-07-30-native-p0-p1-feishu-bridge-design.md)。
2. **用量明细/支持卡片/上游账号状态等后续运营功能**：仍待设计与实现；**状态：持续实施**；[证据](current-state.md)。

## 总账维护规则

以后每次任务都必须在实施前先登记到本文件，并将初始状态写为“进行中”。任务收尾时，只有完成服务端推送、生产部署和生效验证，才可转入“生产工程代码/配置已部署并验证”；仍存在运行时代码或生产配置差异的事项进入“工程代码/配置差异待部署”，尚未完成实现的进入“持续实施”，文档、研究、离线证据和外部验收进入“运维/研究跟进”，不得仅因这些材料缺失而声称工程未部署。每次收尾同时更新统计摘要和 current-state 入口。

**本轮维护门禁登记（2026-08-04）：** 采购成本附加迁移维护门禁，状态：已完成（最终提交 `82095b80770236eac24adb0bdb1b80cd639675cb` 已推送并完成维护发布；生产迁移哈希已为 `337212b4af85839c9497d0fef3153e5c858bd976fed268086459c21a12abcc76`。PostgreSQL `2db52788…`、Redis `c45202c0…`、Caddy `ace4a23b…` 与 `release-state` 一致且未重建；维护路径仅切换 API/Worker，生产验证通过）。
