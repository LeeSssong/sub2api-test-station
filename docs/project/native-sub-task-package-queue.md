# 原生 Sub 小步发布任务包队列

## 当前状态

- 队列状态：P0“使用记录触发会话过期”热修独占当前实施与发布车道。T12、T13 及其他任务均已按用户指令暂停，不得继续实现、复审、合并或发布。当前根基线为 `main@c42b5b8cca4b22b3974cda5500e8bd851fabd7b1`。
- 唯一发布总控：根目录 `/Users/gongtengxinwen/Documents/sub2api搭建` 的 `main`。只有发布总控可以修改全局队列/总账、根 `main`、发布证据和生产状态记录。
- 当前发布状态：P0 热修已合并为 `main@91bce7fe4111cec65ee23b71f49c0550049d86cb`，正在进入推送和宿主蓝绿发布链。仅允许相关功能测试和发布链必要保护；不执行额外独立复审、scoped re-review 或 whole-branch review。
- 原生错误中文提示配置已独立完成：生产 `ErrorPassthroughRule` 是全局规则、没有 `group_id`，因此一套配置已覆盖所有分组；该工作只调用 Sub 原生管理能力，不修改工程代码、不创建功能 worktree，也不占用发布车道。下一实施任务为 T09。
- 2026-08-10—2026-08-14 周复盘已纳入后续排序：P0 先修账号质量监控器 `203/EXEC Permission denied` 的可执行链路并完成真实运行验收；P0 将终端完成率作为 Pro 调度/经营硬门槛，不能只看排除业务失败后的平台 SLO；P1 继续处理余额/资格失败的账号准入否决和特惠账号稳定性风险；P1 规划卡片双口径（终端完成率、平台 SLO、排除量）；P2 为延时排名补充窗口、样本、模型构成、用户集中度和缓存命中上下文。以上是任务边界和验收约束，不代表本次 T08 顺带改动。
- 冻结项：S1 旧候选 `codex/upstream-resilience-s1-native-isolation@69a93343c` 因落后主线、Task 5 复审未闭合及迁移编号 `220` 冲突而 `FROZEN_FOR_REBASE`；T05 旧 detached `a71c675b1` 只作启动审计，轮到时从届时最新干净 `main` 重建。
- 流程偏差：T01、T02 虽有独立 worktree、规格书、计划和复审证据，但未建立用户可见的独立顶层 Codex 任务；T03 是纠偏前已在途并由根任务内部代理完成的任务。三者均不得宣称符合新增顶层任务门禁，已验证技术成果继续保留。
- 执行方式：最多两个互不依赖的功能 worktree 可并行准备；合并、推送、部署和线上验收严格单车道串行。每个新任务包必须从当时最新干净 `main` 创建用户可见独立顶层任务和独立 worktree。
- 模型规则：所有用户可见顶层任务统一使用 `GPT-5.6 Sol / medium`；任务内部 implementer/reviewer 子代理继续使用既定设置，不随顶层模型统一调整。
- 根任务职责：排队、创建顶层任务、读取交接、授权合并、合并后快速门禁、推送、部署和线上验收；不得用根任务内部 `spawn_agent` 代替整个任务包。
- 顶层任务职责：完整 brainstorming、书面规格书及用户批准、实施计划、实施与验证、独立任务复审、最终全分支终审，并在 `READY_FOR_ROOT_REVIEW` 等待根任务授权合并 `main`。

## 队列

### P0 使用记录触发会话过期热修

- 当前状态：`DEPLOYING`。独立顶层任务 `01a00b57-1365-7712-8c31-58e97d5d0941`，GPT-5.6 Sol / medium，候选 `c25fb9ad1` 已合并为 `main@91bce7fe4111cec65ee23b71f49c0550049d86cb`。
- 已确认根因：“使用记录”首屏并发请求在 access token 过期时同时发起 refresh；一个请求成功轮换并删除旧 refresh token 后，另一个请求使用旧 token 触发 `Refresh token not found, possible reuse attack`，全局 401 处理清除会话并跳转登录页。
- 范围：仅修复同一会话的并发 refresh 竞态；保留真实撤销与恶意 reuse 的安全边界。禁止忽略所有 401、无条件接受旧 token、关闭 reuse 检查或直接修改生产 Redis/数据库掩盖问题。
- 最小验收：先有能复现并发轮换的失败测试，再完成最小修复；仅运行直接相关功能测试、必要的编译/类型检查和 `git diff --check`。候选从根 `main` 合并后使用既有本地/宿主蓝绿链发布，不使用 GitHub Actions。

### T01 大上下文入站上传稳定性

- 当前状态：技术交付已部署并完成线上验证；存在“未使用用户可见独立顶层任务”的流程偏差，不宣称顶层任务合规，不重做或回滚。

- 目标：修复大请求或慢速网络下，请求体尚未上传完成就被 Caddy 固定 300 秒窗口终止并返回 502 的问题。
- 范围：推理入口的 Caddy 反代超时策略、既有请求体大小保护、配置合同测试和慢速/不完整上传验证。
- 不包含：错误中文转译、上游重试、账号调度或 CDN 改造。
- 验收：超过 300 秒但持续上传的受控大请求不再被代理误杀；真正中断的上传可释放资源并保留可诊断日志；普通请求与健康检查不回归。
- 预期部署属性：配置级更新；派生线程必须报告是否可安全 reload，以及 `downtime_required`。

### T02 原生错误转译与管理员诊断 MVP

- 当前状态：技术交付已部署并完成线上验证；存在“未使用用户可见独立顶层任务”的流程偏差，不宣称顶层任务合规，不重做或回滚。

- 目标：在 Sub 原生错误透传/改写机制之外，为已持久化错误记录补充可读、脱敏的中文运营诊断。
- 与原生能力的区别：Sub 原生 `ErrorPassthroughService` 在请求链路中按上游 HTTP 状态、平台和关键词匹配规则，可透传或改写客户端状态码/消息，并可能影响账号错误处理；T02 不替换、不重复该机制，只在读取错误记录时投影 `local_limit`、`upstream_overloaded`、`upstream_failed`、`upload_interrupted` 四类诊断。
- 范围：优先复用 Sub 原生错误透传结果；用户显示脱敏中文含义与建议，管理员在既有错误详情查看阶段、归属、已选账号/分组、上游状态和二次脱敏证据；HTTP/SSE 传输、路由、重试、调度和计费行为保持不变。
- 管理员入口：管理后台 -> 用量明细 -> 错误请求 -> 点击错误详情；运维总览详情复用同一字段。
- 不包含：全尝试链、追踪系统、新错误表、自动根因推断或新的管理页面。
- 验收：用户不再看到难懂原文；管理员能看到错误阶段、归属、是否选中账号、账号/分组及原始上游证据；请求上传失败明确显示“未选择上游”。

### T03 上游扣费与利润始终有值

- 当前状态：已完成。纠偏前已合并并推送 `main@0432b87491a313b006643212cccdcd8d49001ae4`，完成无停机生产部署和健康检查；明确收费自然流水均返回数值上游扣费与利润。生产没有出现 confirmed-zero 自然样本，故以同一发布树的空白值服务/Handler/前端合同测试和生产部署身份、收费路径、失败边界实证组合验收。`endpoint_unsupported`、`record_not_found`、`response_unavailable` 继续明确不可用，不伪造为 0。候选已制作并校验可恢复 bundle，worktree 和本地分支已安全删除。

- 目标：管理员流水中的“上游实际扣费”和“利润”不再显示空白。
- 范围：继续使用已部署的 Sub/New 原生精确请求 ID 查询；原生实际扣费空白按 `0`；利润按本站实际扣费减上游实际扣费计算并返回数值。
- 不包含：估算、对账状态、异常标记、模糊匹配、历史回填或 relay-ops。
- 验收：Sub/New 成功、上游明确收费、上游空白三类定向用例均返回数值；非管理员仍看不到上游成本与利润。

### T04 账号监控移除外部控制面状态

- 当前状态：已完成。候选 `63019434684a53b7b856a6acea5605e3e8b4aede` 经根任务授权合并为 `main@be9e124d65c7457477fbe6d3435a9468b1ec1f4c`，已推送并通过预加载蓝绿链无停机部署；生产活动槽为 `blue`。登录态浏览器验证初载、reload 和 7 天切换均只调用原生账号监控接口，`/xingqiao/**` 请求为零，页面无外部控制面 banner 或完整性状态条。

- 目标：从原生账号监控删除“控制面暂时不可用 / 完整性 unknown”及相关外部状态请求。
- 范围：仅账号监控页面、对应原生 API 调用和组件测试。
- 不包含：全局评分、推荐提示、用量页或利润页。
- 验收：账号监控只依赖并显示原生 Sub 数据，页面无外部控制面 banner、unknown 状态或失败请求。

### T03-R1 上游扣费缺失与异步持久化修复

- 当前状态：已完成。最终 `main@210d0397e647b91be080f0c7252da39a6e61d71d` 已推送并从受审维护链部署；生产记录 `20260814T051143Z-production-2876774.json` 为 `succeeded/promoted`，活动槽 `green`，源 tree `4e2b7be29191894a8e7fac7e7af21cb0cf4adb21`，迁移哈希 `6a0e141eb4788460a99fc3e108ce5b46c866fd2c45b9a7265ea66b0ef8faaf71`。用户已授权停机，公网 `/healthz`、`/readyz`、`/health` 均通过，生产服务健康。功能启用后首个稳定窗口内 15 笔自然流水中，3 笔明确 NewAPI 身份流水已自动登记 `confirmed`，12 笔无明确 Sub/New 身份流水按合同进入 `evidence_not_registered`；管理员财务、异常、本地详情与未认证隔离均已在线验收。维护任务 `019ffe60-c370-7290-a310-0f811e8d09ae` 因根 `main` 漂移并包含同范围旧候选而停止在 `BLOCKED_FOR_ROOT_RECONCILIATION`，未宣称为流程合规交付；不影响已审主线技术结果。
- 历史边界：以下独立证据、人工复核、OAuth 日成本和覆盖语义作为已部署历史能力保留，但其作为经营/盈利页面取数权威的产品方向已被 T11-R1 取代；T11-R1 不破坏性删除历史数据，只停止页面依赖。
- 目标：保持官方 `usage_logs` 不变，以独立一对一证据登记功能启用后的 Sub/New 原生逐笔成本；升级现有账号盈利页为管理员财务首页，提供全站/账号人民币营收、本站支出、利润、利润率、异常数量、用户未消费余额，以及使用记录中的异常核对 Tab。
- 事实语义：精确命中有效非零成本为 `confirmed` 并立即纳入；精确命中数值 0 或 blank/null/empty 为 `confirmed_zero`；无精确证据、端点/鉴权/网络/解析或登记失败为 `unavailable`。后两类待管理员核对前完全不进入财务汇总；不补查、不重试、不估算。
- 人工与 OAuth：异常可逐笔、选中项批量或当前筛选范围批量确认；未输入成本时按 0 纳入并保留原始证据状态。字面 `oauth` 类型的自购账号不查询上游、不产生成本异常，由管理员按北京自然日填写人民币成本；未填写的账号日不进入全站四项财务汇总。
- 范围：独立证据/复核/账号日值表，查询时实时汇总，今日营收/成本直接覆盖与截止点，60 秒刷新，管理员详情本地读取，现有审计日志留痕，以及必要管理员 API/UI/测试和 expand-only 迁移。
- 不包含：历史回填、直接扩展 `usage_logs`、定时汇总表、延迟补查、汇率/采购成本分摊、普通用户入口、审计专用 UI、外部账务源、T05 或 GitHub Actions。
- 验收：功能启用后的官方流水具备本地证据或明确缺证据投影；管理员无需打开详情即可看到一致事实；异常核对后按批准规则纳入；每日覆盖不吞掉后续流水；OAuth 待填写/填写语义正确；全站余额快照包含未删除用户且包含 disabled、排除 deleted；非管理员完全不可见。

### T05 用量页移除外部控制面状态

- 当前状态：`DONE`。已完成规格批准、计划、实现、两轮刷新、专项验证、任务复审、最终全分支复审、根授权合并、推送、蓝绿发布和登录态线上验收；发布无迁移/配置变化，`downtime_required=false`，页面已移除外部控制面状态与调用。
- 目标：从原生用量页删除外部控制面状态和调用，保留原生流水及管理员详情。
- 范围：仅管理员用量页及其测试。
- 不包含：成本数值规则、利润页或账号监控。
- 验收：用量、错误请求、详情弹窗正常；无外部控制面 banner、unknown 状态或网络调用。

### T06 利润页移除外部控制面状态

- 当前状态：`DONE`。原始 T06 生产发布记录 `/var/lib/sub2api/release-records/20260814T154749Z-production-3329818.json` 保留为历史验收失败证据；缺陷已由 T06-R1 修复并在新的生产记录 `/var/lib/sub2api/release-records/20260814T181009Z-production-3436954.json` 完成闭环。

- 目标：从原生利润页删除外部控制面状态和调用，保留原生利润数据。
- 范围：仅利润页面、原生 API 使用和测试。
- 不包含：成本公式修改或其他页面。
- 验收：利润页正常加载原生数据；无外部控制面 banner、unknown 状态或网络调用。

### T06-R1 利润页深色主题与中文本地化修复

- 当前状态：`DONE`。用户可见独立顶层任务 `01a00117-6fae-75f0-bbc1-6f340342acdc` 已完成 brainstorming、书面规格批准、计划、TDD、fresh implementer/独立 reviewer、最终全分支终审、刷新复核、根授权合并、推送、无停机蓝绿发布和管理员登录态线上验收。候选 `d50c47d744b405f54b8bf420de68a59ed70b9e0c` 已合入 `main@459a020fd99b605c3da50ead2cbc10121e57cbcd`；8/8 页级测试、typecheck、build、diff-check、范围检查通过；无迁移/配置/依赖/GitHub Actions 变化；生产记录 `/var/lib/sub2api/release-records/20260814T181009Z-production-3436954.json` 成功、活动槽 `green`、`downtime_required=false`；线上页面显示中文范围和表头、深色主题可读、原生 API 正常、外部控制面请求为 0。T07 未启动。
- 目标：修复利润页在深色主题下白底浅字导致内容近乎不可见的问题，并补齐中文范围名称和中文表头。
- 范围：仅利润页主题样式、`24h`/`31d` 中文词条、表头本地化及相应页级测试。
- 不包含：财务计算、接口字段、迁移、外部控制面、其他页面视觉重构或 T07。
- 验收：深色主题卡片和表格内容清晰可读；范围按钮为中文；表头为中文且无硬编码英文；原 T06 的刷新、范围切换、原生 API 和无外部控制面验收继续通过。

### T07 全局评分设置

- 当前状态：`DONE`。用户可见顶层任务 `01a0018d-65fa-7dd2-9393-31d9e1643adc` 已完成完整规格/计划、TDD、逐任务复审、全分支审查、发布链补丁的独立 scoped/whole-branch 复审，并完成根授权合并、推送、维护部署和线上验收。最终 `main@44ec9ed2797e86ae6ad140dd85b9efa91d29756d`、tree `cb6c9fea78b406741fa5709b389ea5f45b57bc24`；发布证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-15-main-44ec9ed-t07-global-score-weights-v2.json`。新增 MAINTENANCE_8 仅放行 `6a0e141… -> d3fe99…`，未授权、错误 old/new hash 均 fail-closed；宿主/控制器合同、bash 语法、diff-check 通过。生产记录 `/var/lib/sub2api/release-records/20260815T004424Z-production-3723827.json` 为 `succeeded/promoted`、`rolled_back=false`，活动槽 `blue`，健康检查通过，管理员全局权重 API 返回 `15/45/20/20`，账号监控接口返回 7 组/78 个账号。由于候选 worktree 的 index.lock 被工具层拒绝，本次由根总控使用 Git plumbing 辅助落候选提交 `01705a694fbe91913359afe14defd2df9d9cfc88`，未改权限、未绕过复审；该流程例外已记录。T08 随后按独立顶层任务完成并已在上方条目闭环。
- 目标：在未进入具体分组时提供全局评分权重设置。
- 范围：全局权重持久化/API、账号监控全局设置按钮、复用分组评分弹窗；默认权重保持成本 15、成功率 45、首字延迟 20、总延迟 20。
- 不包含：分组权重迁移、评分指标增加或调度算法修改。
- 验收：全局与分组权重互不覆盖；全局账号排序即时反映新权重；刷新后仍保留。

### T08 “暂不建议入组”轻提示

- 当前状态：`DONE`。用户可见顶层任务 `01a00306-a473-73f3-9240-addaf11b119d` 已刷新、实现、独立复审、全分支终审并经根授权合并；最终文档提交 `main@1bebe479257e39c9433782836788238399e76b0e`，tested tree `6b9eb0a7f79d65f47e82e944f5d467d1f83323b9`。生产记录 `/var/lib/sub2api/release-records/20260815T085054Z-production-4053846.json` 为 `succeeded/promoted`、`rolled_back=false`、`downtime_required=false`，活动槽 green；64/64 定向测试、typecheck、build、diff-check 和线上管理员验收通过。真实生产无 `not_recommended` 自然样本，未修改生产数据；线上页面的中文、资源身份、390x844 无横向溢出、账号操作和健康检查均通过。详细证据见 `docs/superpowers/reports/2026-08-15-t08-do-not-recommend-light-hint-production.md`。候选归档 `/Users/gongtengxinwen/Documents/sub2api-archives/t08-do-not-recommend-light-hint-772c89d4.bundle` 已验证，worktree/本地分支已删除。

- 目标：保留紧凑标签，把原因收进按需提示而不是常驻文本。
- 范围：账号卡片标签、桌面悬浮/点击、移动端点击、可访问性和防溢出测试。
- 不包含：推荐算法、分组迁移或卡片其他布局重构。
- 验收：默认只占一行标签空间；原因最多一到两行；桌面和移动端均可查看且不遮挡其他内容。

### T10 账号质量监控器可执行链路

- 当前状态：`DONE`。用户可见顶层任务 `01a004ce-6aee-76c1-8efb-7b915f43d290` 的候选已完成实现、第一轮独立复审修复、根全差异复核、定向验证和 handoff；根修复生产蓝绿上游寻址后，最终 `main@b1b92cf30a791d0573c212e865d3a52c43564d95` 已推送。实现包含 root host orchestration、UID/GID 10002 真实证据预检、正确 `[Unit] OnFailure`、脱敏稳定 `t10.failure.v1`、分阶段退出码、双文件原子发布/恢复和活动蓝绿上游严格白名单。最新 2/75、5/119、13/53 测试及 systemd/alert/relay-ops/语法/diff 合同通过；宿主安装校验、真实 service、timer、证据文件和公网健康即时验收通过。fresh reviewer 调度失败作为流程例外，不倒称 PASS；用户豁免 A6 实际送达和 A10 按时间等待，两项保留未验证。候选已归档为已验证的 0600 完整历史 bundle `/Users/gongtengxinwen/Documents/sub2api-archives/t10-account-quality-monitor-d075da534.bundle`（SHA-256 `f02cc7907bd749c8f289716c2cb4b65a8d2554a4441d8b64d2823d45e478b471`），worktree 和本地分支已删除。生产报告见 `docs/superpowers/reports/2026-08-16-t10-account-quality-monitor-production.md`。
- 定义：T10 不是新的账号质量算法、评分系统或用户页面；它修复的是既有宿主只读采集任务因 systemd `203/EXEC` 无法执行的问题。
- 目标：让 systemd timer 定期调用采集器，读取 Sub 原生账号监控/账号测试 API，并把结果写成两份受保护 JSON 快照；失败时可产生脱敏诊断信号。
- 范围：ExecStart 可执行路径/目录权限或等价的受控安装布局、systemd unit 合同、只读证据输出、失败告警和真实运行验收；不写账号、路由、余额、计费、评分或调度状态。
- 不包含：调度器权重、账号准入算法、余额/利润、监控卡片双口径、用户页面、外部控制面、官方更新冲突处理。
- 验收：连续受控触发不再出现 `203/EXEC`；timer/service 真实运行并写入既有证据位置；采集失败仍可诊断且不会写路由、账号、余额或计费；部署属性和停机需求由顶层任务报告。

### T11 经营页三层视图与异常空态修复

- 当前状态：`DONE`，但只对“全站/分组/账号三层结构、时间范围和页面状态”这一历史范围成立。独立候选 `codex/account-financial-dimensions@d5df834e3` 已合入并推送为根 `main@d17968ab95cb5f9db2a7374c59222b8b01c0e46f`，生产记录 `/var/lib/sub2api/release-records/20260815T135236Z-production-75345.json` 为 `succeeded/promoted`。该任务沿用了 T03-R1 独立财务证据口径，因此不能视为经营/盈利页面的最终产品修复；数据口径由 T11-R1 纠偏。详见 `docs/superpowers/reports/2026-08-15-account-financial-dimensions-production.md`。
- 目标：经营页提供全站固定摘要、分组 Tab 和当前分组账号列表；异常跳转后始终显示 loading、data、empty 或 error/retry，而不是空白内容区。
- 范围：向后兼容扩展现有原生 `account-financial` 报告，按 `usage_logs.group_id` 聚合分组及 `(group_id, account_id)` 行；前端展示三层结构；异常跳转保留账号、范围和 `review=pending`。
- 不包含：新账务源、平行经营 API、外部控制面、历史回填、延迟补查、上游重试、金额猜测分摊、数据库迁移、调度/计费写入、普通用户入口或 GitHub Actions。
- 验收：全站流水不重复；跨分组和未归属口径正确；账号级今日覆盖/OAuth 日成本不猜测分摊；桌面/移动端三层视图可用；异常加载、空结果、失败重试均有可见状态；只调用原生管理员 API。

### T11-R1 Sub 原生计费聚合经营页纠偏

- 当前状态：`DONE`。最终候选 `6e38e2f9d607361145dd183384824c32cc8c3a9c` 已合入并随 `main@7a7c9abd70fb108af6a06b93ef67eea3c4b34dab` 推送、无停机部署和线上验收；生产活动槽 `green`，source tree `916aaf16ad6ac354b8981755b8072dedab4f6cf7`，迁移哈希保持 `d3fe99bba69b0cf0cca8a7f5ec45499921f3496f58dd74c3a671d90a653589b5`。前端 19/19、必要 lint/typecheck/build、后端 focused、范围/发布门禁通过；31 天 API HTTP 200/0.153 秒，精确 390×844 页面无横向溢出，金额卡片不重叠。完整证据见 `docs/superpowers/reports/2026-08-16-t11-r1-native-accounting-profitability-production.md`。
- 目标：保留 T11 已上线的全站固定摘要、分组 Tab、账号行、今日/24 小时/7 天/31 天、刷新以及 loading/empty/error/retry 体验，但所有经营数值完全改用 Sub 原生 `usage_logs` 计费统计。
- 官方字段：请求数 `requests`；Token 数 `tokens`；账号计费 `cost = SUM(COALESCE(account_cost, COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)))`；用户扣费 `user_cost = SUM(actual_cost)`；利润可展示为 `user_cost - cost`，利润率由该利润除以 `user_cost` 派生。
- 聚合维度：全站、`usage_logs.group_id` 分组、`(group_id, account_id)` 账号行；所有维度和时间范围必须来自同一官方流水与美元单位。
- 范围：优先复用 `AccountUsageService.WindowStats`、今日/窗口批量统计、`usage_log_repo_stats.go` 的官方 SQL 口径和现有原生管理员 API；允许为全站/分组聚合做最小兼容扩展，但不得建立第二套计费模型。
- 不包含：汇率换算、人民币经营口径、独立上游成本证据、人工成本/OAuth 日成本覆盖、估算、补查、重试、历史回填、计费写入、调度修改、生产数据修改、GitHub Actions，或破坏性删除历史 T03-R1 表与证据。
- 页面纠偏：移除与官方唯一口径冲突的白色人工覆盖输入；删除摘要“异常流水”卡片、账号行异常数量/操作和跳转成本异常页的入口，不改造成失败请求数。历史 T03/T03-R1 异常流水页面和证据表继续保留，不作破坏性删除。
- 验收：同一时间范围内，经营页全站/分组/账号的请求、Token、账号计费、用户扣费与 Sub 原生账号统计口径一致；聚合守恒且无重复；利润和利润率仅为透明派生；旧证据缺失不再导致官方流水被排除；桌面和移动端现有页面状态不回归。

### OAuth 图片编辑上传 MIME 兼容热修

- 当前状态：`DONE`。用户可见顶层任务 `01a00892-2294-7083-aa01-7aa6f94d1dc4` 的刷新候选 `de462d34837a8d6ac4605e65f2f6193e1bfa867a` 已合入并随 `main@3d4580c55f106193617865c59c42dbc603fee435`、tree `5e5e3cecdcdaa4a36573c423c2f29b003260f0c8` 推送和无停机发布；生产记录 `/var/lib/sub2api/release-records/20260816T042531Z-production-727142.json` 为 `succeeded/promoted`、`rolled_back=false`，活动槽 `blue`。公网健康均 200；事故 API key `50` 的 `gpt-image-2` octet-stream 编辑返回 200/1 张图片。OAuth 精确线上重放因既有规避移出唯一生图组而安全不可执行，未恢复分组或改生产数据；发布 tree 的 focused OAuth 测试保留为该子项证据。详见 `docs/superpowers/reports/2026-08-16-oauth-images-edit-mime-compat-production.md`。
- 任务目标：修复 OAuth 账号 `/v1/images/edits` multipart 图片被转换为 Data URL 后仍保留 `application/octet-stream`，导致上游返回 400 `unsupported MIME type` 的兼容问题。
- 已知事故：`user_id=34`、API key `50`、生图 `group_id=19` 的请求命中；API-key 账号链路正常。生产临时规避已于 2026-08-16 10:02 将 OAuth 账号 `222/223` 移出 `group_id=19`。用户邮箱仅保留在授权交接上下文，不写入仓库总账。
- 最小范围：仅修改 `upstream/sub2api/backend/internal/service/openai_images_responses.go` 约 320 行附近；当 MIME 为空或为 `application/octet-stream` 时用文件字节识别真实 MIME，仅接受 `image/*`，无法识别则拒绝。
- 明确非目标：不改错误码、错误文案/中文提示、`ErrorPassthroughRule`、客户端表现，不扩大到其他上传链路，不改生产数据或账号分组策略。
- 最小验证：相关 service 单测、后端必要构建/类型检查、diff/范围检查、发布预检；上线后仅做 OAuth/API-key `/images/edits` 定向验收和健康检查。不得使用 GitHub Actions 或从旧 `origin/main`/独立产物直接覆盖生产。
- 发布边界：必须从 T11-R1 已部署后的最新 `main` 创建用户可见 GPT-5.6 Sol/medium 顶层任务和独立 worktree；候选部署必须包含 T11-R1 与本热修，避免回滚任一前序功能。

### T09 官方 v0.1.177 更新执行

- 当前状态：`DONE`（官方更新例外）。用户指定可见任务 `01a008e1-55d1-74f1-a659-fac363dcfd28` 保留官方更新执行证据，并由可见任务 `01a006a8-ef15-7960-b72a-de2fddab0339` 统筹；独立 T09-R1 顶层任务 `01a009d9-dbf9-7183-bf69-021b2d1fa7d9` 的精确维护放行补丁已合入根 `main@e91504e51` 并推送。宿主维护链已成功切换 `green`，公网 `/healthz`、`/readyz`、`/health` 均 200；发布证据为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-16-main-e91504e51-official-v0177-maintenance.json`。
- 目标：把官方最新稳定版 `v0.1.177` 合入当前定制树，人工解决全部冲突，并交由总控通过既有本地/宿主发布链直接生效。
- 范围：官方 release 发现、定制树合并、冲突解决、候选交接；根 `main` 合并、推送和发布仍由唯一总控执行。
- 不包含：继续实现“冲突即停止”产品方向、额外功能测试/回归/类型检查/独立构建验证/上线专项验收、关闭定制模块、GitHub Actions 或 external-primary。
- 完成条件：已取得用户停机授权并按既有发布链完成切换；不另做功能验收。迁移哈希 `d3fe99… -> ef1213…` 已由精确 allowlist 放行，发布结果为 `succeeded/promoted`，回滚依据为上一活动槽及宿主 release-state/release-record。

## 串行推进门禁

- 用户已于 2026-08-15 授权唯一发布总控在其离席期间代审并批准既定队列任务的规格书和实施计划；该授权不包含范围扩大、不可逆数据操作、外部付费、安全例外或 `downtime_required=true` 的生产变更。

每个任务包依次经过：

`根任务创建用户可见顶层任务 -> 最新 main 独立 worktree -> 完整 brainstorming -> 2–3 方案比较与分段设计批准 -> 正式规格书 -> 规格书自审 -> 用户明确批准或根总控依据离席代审授权批准书面规格书 -> writing-plans -> 计划获批 -> fresh implementer subagent -> 直接相关功能测试 -> READY_FOR_ROOT_REVIEW -> 根任务 AUTHORIZE_MERGE_TO_MAIN -> 顶层任务合并 main -> 根任务快速门禁 -> 无停机部署或停机暂停 -> 线上专项验证 -> 清理 -> 下一任务包`。自 2026-08-16 用户最新指令起，额外 task review、scoped re-review 与 whole-branch review 不再是强制门槛。

未经用户明确批准书面规格书，不得调用 writing-plans 或开始实施。任何一步出现范围漂移、冲突、`main` 漂移、验证失败、线上验收未闭环或 `downtime_required=true`，队列立即暂停，不启动下一任务包。

任务状态统一为：`BACKLOG -> DESIGNING -> IMPLEMENTING -> REVIEWING -> READY_FOR_ROOT_REVIEW -> REFRESH_REQUIRED -> INTEGRATING -> DEPLOYING -> VERIFYING -> DONE`，或 `FROZEN/BLOCKED`。同时只能有一个任务处于 `INTEGRATING`、`DEPLOYING` 或 `VERIFYING`。

## 当前推进门禁

- T04 已完成合并、推送、无停机部署、登录态线上验收、可恢复 bundle 归档及 worktree/分支清理。
- T03-R1 已完成推送、停机维护发布和线上验收；生产活动槽为 `green`，不得重复发布同一 SHA。
- 账号监控卡片、T05、T06/T06-R1、T07、T08 均已完成生产验收；当前没有待处理的迁移 223 停机门禁。
- T07、T08、T09、T10、T11、T11-R1 与 OAuth MIME 热修已完成生产收口；官方 `v0.1.177` 发布车道已释放。
- T09 已完成官方候选刷新、根合并、push、精确维护放行、宿主切换和即时健康验证；T14 排在其后进入下一独立任务，S1-R2 继续原队列位置，S1 旧候选继续冻结。
- S1 旧候选保持冻结；后续 S1-R2 必须从届时最新 `main` 重建并重新分配迁移编号，S2/S3 继续分别等待前一包生产验收。
- “正在重新连接 1/5”与 `stream disconnected before completion` 已确认属于上游 SSE 在 `response.completed` 前断开的 S1-R2 冷却/故障转移范围；当前不插队、不另开紧急实现任务、不解冻旧 S1 候选。

### T12 经营页本站探测花费与排序/美元字段优化

- 当前状态：`FROZEN`。用户已要求立即停止以让位 P0 会话热修。已保全 `HEAD a54222c5352889c0b48bff2a5824c8b6f214c657`，以及唯一未提交文件 `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`；无生产代码改动、无新提交、无合并或发布。未获根总控明确解冻前不得继续。
- 冻结前进度：Task 1、Task 2 已完成，Task 3 候选为 `a54222c5352889c0b48bff2a5824c8b6f214c657`；Task 4 仅开始编写 RED 测试且未运行。
- 当前状态：`IMPLEMENTING`（Task 1、Task 2 均已通过独立 scoped re-review，Task 3 已获授权；禁止自行合并、推送、部署或线上验收）。用户可见 GPT-5.6 Sol/medium 顶层任务 `01a00aa3-a274-7270-a970-ec23472627dd` 使用独立 worktree `/Users/gongtengxinwen/.codex/worktrees/1475/sub2api搭建` 和分支 `codex/t12-native-probe-cost-design-recovery`；批准规格为 `3cb9817f3be2581ff1dc1e0dcd025680d275b205`，批准计划为 `786d809cf0c366c03e7e75d3607c0b95c0c90553`，基线 `main@c42b5b8cca4b22b3974cda5500e8bd851fabd7b1`。Task 1 提交 `07ee44cd6` 与 `7ce562fb7` 已保留 add-only ledger、`ON DELETE RESTRICT`、DECIMAL 原始精度与 UTC 微秒幂等；Task 2 候选 `aff652b83` 经 P1 修复提交 `55ccfdeef` 与证据提交 `0f9451934`，已确认 monitor/scheduled/manual 显式来源、实际发送模型计价、recovery 不计量及 fail-open/用户账务隔离，最终 scoped re-review 为 PASS/PASS。Task 3 仅可读取独立 probe ledger 并向 account-financial 增加可空聚合与顶层错误合同；probe 查询失败必须由 `probe_data_error/probe_error_code` 显式表达并使所有 probe 聚合字段为 `null`。经营页所有外部金额必须 USD 两位展示，内部精度保持原样。T13 完成并发布前，T12 不得进入 `INTEGRATING`、`DEPLOYING` 或 `VERIFYING`。
- 目标：保持未消费金额为 USD；补充六项排序（请求、Token、账号计费、用户扣费、利润、利润率）；新增独立“本站探测花费”字段、卡片和账号列。
- 范围：探测记录与用户消费隔离；探测花费不影响账号成本、用户成本、利润或利润率；外部金额两位小数、内部原始精度保留；不做历史迁移/回填，启用后重新记录。
- 非目标：不改变用户消费、账号计费、利润/利润率、余额事实源、调度/路由、普通用户入口，不建设第二账务源或外部控制面。
- 设计前置：必须先复用现有 Sub 原生 `usage_logs`、账号探测/测试链路和经营页；只有正式规格证明原生能力不足后才可做最小扩展。规格批准前不得写计划或代码。
- 旧设计证据：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/account-probe-cost-design`、分支 `codex/account-probe-cost-design@893933924` 仅作冻结的 docs-only 设计证据，不作为 T12 顶层任务、不继续写入、不合并或部署。

### T13 NewAPI 上游倍率自动登记

- 当前状态：`FROZEN`。用户已要求立即停止以让位 P0 会话热修；保留现有候选分支/worktree 和未提交内容，不得继续实现、复审、合并、推送或部署。
- 冻结前进度：规格与计划已批准，Task 1 已完成，Task 2 候选保留在 `codex/newapi-rate-multiplier-registration`。
- 当前状态：`IMPLEMENTING`（规格与计划已批准，继续排队等待 T14 发布车道）。用户可见 GPT-5.6 Sol/medium 顶层任务 `01a00969-b2ea-7ff0-9f49-d7af64438e00` 负责规格/计划/实现/复审/handoff；Task 1 已通过 scoped re-review，Task 2 正在执行 CAS 与 post-usage 接线。候选分支为 `codex/newapi-rate-multiplier-registration`，当前未进入合并、推送、部署或线上验收车道。
- 权威输入：仅接受 NewAPI 精确匹配日志中的 `other.group_ratio`；仅适用于 NewAPI API-key 且没有原生 Sub 倍率声明的账号。
- 写入语义：首次真实成功请求后登记 `accounts.rate_multiplier`，并在 `accounts.extra` 标记来源/登记状态；已登记账号按北京时间自然日仅首笔合格请求刷新一次。
- 并发与失败：使用 CAS 防止并发覆盖；失败不得覆盖既有倍率或登记标记；管理员可见“已登记”。
- 非目标：不做数据库迁移、历史回填、生产数据修改，不扩展到 OAuth、非 NewAPI、已有原生倍率声明账号或其他上游日志字段。

### T14 用量详情上游扣费/利润字段兼容热修

- 当前状态：`DONE`。用户可见 GPT-5.6 Sol/medium 顶层任务 `01a00a15-a76c-7be1-b66f-7a34ddb2b749` 的候选已随 `main@200d4b1c9e4745a6a54e467630c68aba14fb4028` 推送并通过本地/宿主蓝绿链切换，脚本结果为 `succeeded`、活动槽 `blue`、`downtime_required=false`。`/healthz`、`/readyz`、`/health` 均为 HTTP 200；刷新到已发布前端包后，管理员详情对自然确认样本 `usage_log_id=120896` 正确显示上游实际扣费 `$0.001010` 和利润 `$0.000505`。无迁移、配置、依赖或生产数据改动；回滚依据为宿主上一 `green` 槽/镜像和 release record。
- 已确认根因：`/admin/usage/:id/upstream-cost` 返回 PascalCase 字段，例如 `NormalizedCostCNY`、`EvidenceStatus`；详情弹窗仅读取 snake_case 字段，例如 `normalized_cost_cny`、`evidence_status`，因此“上游实际扣费 / 利润”错误显示为 `-`，不是生产数据缺失。
- 范围：仅对该详情弹窗/API 响应做向后兼容字段归一化，并保留 PascalCase 与 snake_case 两种响应兼容；只做直接相关页级/API 合同验证、必要类型检查/构建、diff/范围检查和发布后定向验收。
- 非目标：不得并入 T12，不改变账号成本、用户扣费、利润/利润率口径或聚合，不做数据库迁移、历史回填、生产数据修改、账务重算、相邻页面重构或外部控制面。
