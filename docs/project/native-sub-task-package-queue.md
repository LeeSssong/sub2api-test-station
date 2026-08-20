# 原生 Sub 小步发布任务包队列

## 当前新增任务（2026-08-20，快速迭代-13）

- **T46 性能监测自定义页面挂载**：状态 `IMPLEMENTING`。从当前 `main@989c072a8` 创建独立候选，复用 Sub 原生 `MonitorV2RouteView`、Monitor V2 API/刷新/时间线数据与既有 T41/T42/T44 视觉稳定性；新增 `/custom/performance-monitor` 入口与“性能监测”用户菜单，隐藏原生 `/monitor` 固定入口，不保留旧路由兼容重定向；自定义页面路由直接挂载原生监控组件，保留 AppLayout、鉴权、主题和本地 optimistic 展示语义。预计无迁移、无生产数据写入、`downtime_required=false`；直接相关前端路由/侧边栏/页面挂载测试、typecheck/build、diff-check 后进入根合并车道。候选 worktree：待创建。

## 当前新增任务（2026-08-20，快速迭代-12）

- **T44 Monitor V2 时间线布局稳定性与卡片防抖优化**：状态 `DONE`。已合入、推送并从 `main@9b4d5b7f6` 发布；桌面端 24/28/30 桶按可用宽度均匀填充，小屏仅时间线内部横向滚动；卡片和柱体 hover 不再位移/缩放，固定边框与几何避免跨卡片抖动。宿主记录 `/var/lib/sub2api/release-records/20260820T100427Z-production-1108935.json`，`downtime_required=false`、`result=succeeded`、`state=promoted`；健康端点 200。
- **T45 CodexRadar 站长推荐与社区矩阵白天模式适配**：状态 `DONE`。已合入、推送并从 `main@9b4d5b7f6` 发布；站长推荐、社区矩阵及加载/失败/过期状态均补齐 light/dark 双主题，保留暗色视觉与分类强调色。宿主记录 `/var/lib/sub2api/release-records/20260820T100427Z-production-1108935.json`，`downtime_required=false`、`result=succeeded`、`state=promoted`；健康端点 200。

## 当前新增任务（2026-08-19）

- **T41 Monitor V2 时间线视觉与 Tooltip 交互优化（快速迭代-11）**：状态 `DONE`。已合入并从 `main@befce43e8` 发布；Tooltip 下置独立布局行，柱体放大为 6x20、间距 5px，`unavailable + null latency` 显示为灰色虚线无探测数据桶。固定 v7、24/28/30 桶和原生探测来源保持不变。宿主记录 `/var/lib/sub2api/release-records/20260820T080727Z-production-1014979.json`，`downtime_required=false`、`result=succeeded`、`state=promoted`、健康端点均 200。真机截图仍由用户验收。
- **T42 Monitor V2 时间新鲜度与刷新可靠性优化（快速迭代-11）**：状态 `DONE`。与 T41 合并从 `main@befce43e8` 发布；分组显示原生最新 `checked_at`，刷新 GET 失败保留旧快照并 5 秒重试，成功后恢复设置间隔；v7、24/28/30、no-store 和原生探测执行器保持不变。宿主记录 `/var/lib/sub2api/release-records/20260820T080727Z-production-1014979.json`，`downtime_required=false`、`result=succeeded`、`state=promoted`、健康端点均 200。真机截图仍由用户验收。

- **T39 Responses 流式 413 二次错误映射修复（快速迭代-10）**：状态 `BACKLOG`。修复应用内入站/上游 413 在 JSON 与 Responses SSE 路径被二次 `ProjectNativeUserError` 覆盖的问题，保持中文“请求内容过大”语义、错误类型、协议终止事件和脱敏；不处理 Cloudflare 已在边缘生成且未进入应用的 HTML 413。可与 T38 设计/实现并行准备，依赖：无；整合、部署、线上验证继续服从单车道。
- **T40 错误码/边缘错误中文映射补齐（快速迭代-10）**：状态 `BACKLOG`。复用 Sub 原生错误透传与 `NativeUserErrorProjection`，补齐应用侧 402、507、520、521、522、523、524、525，并保留 499 客户端断开分类；覆盖 JSON/SSE、关键词/正文匹配、管理员诊断及 Cloudflare HTML 413 不误判/不泄露边界。可与 T38 设计/实现并行准备，依赖：无；整合、部署、线上验证继续服从单车道。

- **T37 渠道状态用户可见专属分组裁剪**：状态 `DONE`。最终发布源 `main@daf965a0e1fbe421e002493b1d64a239de914f0a`、tested tree `0b6c34e9727102ca2a11bec3a95eb0cde6ae115e`；0600 证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-20-main-daf965a0-t37-feedback.json`，宿主记录 `/var/lib/sub2api/release-records/20260820T045028Z-production-862102.json`。蓝绿链 `downtime_required=false`、`result=succeeded`、`state=promoted`、`rolled_back=false`，活动槽 `blue`；Sub 原生配置分组语义、当前用户专属授权裁剪及 tooltip 下置已部署，健康端点均 200。无迁移、配置 schema 或生产数据写入；真机验收由用户自行完成。
- **T38 可调度账号最近原生探测评分保留**：状态 `DONE`。发布源 `main@b010e6b2df57efe453b8e8551a108164cfd06a93`、tested tree `5d6ce56585540900ecbc0b961e414e8ab541c63c`；0600 证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-20-main-b010e6b2-t38.json`，宿主记录 `/var/lib/sub2api/release-records/20260820T042911Z-production-841669.json`。蓝绿链 `downtime_required=false`、`result=succeeded`、`state=promoted`、活动槽 `green`；评分状态分离与原生探测评分保留已部署，健康端点均 200。无迁移、配置、前端、依赖或生产数据写入；真机验收由用户自行完成。
- **根总控当前车道（2026-08-20）**：T37、T38、T41、T42 已完成合并、推送、无停机蓝绿发布和线上健康验证；T39/T40 保持 BACKLOG；T44/T45 已完成发布，分别处理时间线/卡片稳定性和 CodexRadar 白天模式。用户真机验收作为后续反馈，不阻塞后续设计准备。

- **根总控最近车道（2026-08-20，T36）**：独立用户可见任务 `01a01b65-be8f-7f53-b169-d9ee55456c37` 已完成规格、计划、实现、根合并、推送、无停机蓝绿发布和线上验收，当前为 `DONE`。生产源 `main@12641c3281289ce66eed48f60e46b67f19d6d356`、tested tree `6375dc0a23bc1bf779114b895ea1b5caa60359fe`；合并提交 `808be1901fb4fcb65869336041b777c76d9ee5e8`。范围仅补充中英文界面文案、组件直接测试和发布验收；不改账务公式、金额、API、查询、数据或采购链路。真机验收仍由用户自行执行，不阻塞后续任务。
- **根总控当前车道（2026-08-20）**：T34、T35 均已完成无停机部署、线上健康验证并保持 `DONE`；T34 发布源为 `main@c1f102312cd35440a5a14c57ef8356b4cdcb5b7b`。真机验收按用户指令不阻塞后续；发布预检仅在 `downtime_required=true` 时暂停。

- **T32 账号评分回归修复**：状态 `DONE`。已合入并推送 `main@584b37bba6ed05d86a5a152160d37a9f92fefc9c`，完成 focused 测试、根发布预检、无停机蓝绿发布和线上专项验收。评分、当前状态与排名只使用 Sub 原生主动探测证据；暂停账号仍可参与评分和排名；只有主动探测返回 4xx/5xx 且调度关闭时停止探测并退出排名；调度关闭但主动探测成功时继续探测、评分和排名。生产记录 `/var/lib/sub2api/release-records/20260819T153809Z-production-245313.json`，0600 证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-19-main-584b37bba-t32.json`。
- **T33 经营页账号卡片与搜索**：状态 `DONE`。已合入并推送 `main@0839c7878d8d0c1f59fd11a3f0d3970de784ca1a`，tested tree `9d71116de98949f965c905d8a5fb4f66ce637ce5`；前端 33/33、typecheck、production build、diff-check 通过；发布链 `downtime_required=false`、`result=succeeded`、活动槽 `blue`，公网三项健康检查 200。USD/CNY 经营视图均为每账号独立卡片并支持搜索；页面明确解释本站 CNY 与 USD 额度按 1:1 理解，不改账务公式、采购保存或生产数据。用户于 2026-08-20 明确要求后续发布不再等待真机验收，真机发现问题时另行反馈，因此现有部署与线上验证证据完成收口并释放发布单车道。恢复 bundle `/Users/gongtengxinwen/Documents/sub2api-archives/t33-profitability-cards-90cee9bf4.bundle` 已通过 `git bundle verify`，SHA-256 `a86b71a39c38a6e0749331e7f22f1f87c4c1c801d94986f313abf3e604f05670`；候选 worktree、临时发布 worktree和本地分支已清理。
- **T34 渠道状态原生探测重构**：状态 `DONE`。独立用户可见任务 `01a01ad3-774a-7e80-a0b7-9bc9bcde54ed` 已合入并从 `main@c1f102312cd35440a5a14c57ef8356b4cdcb5b7b` 发布；证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-20-main-c1f10231-t34.json`，宿主记录 `/var/lib/sub2api/release-records/20260819T184343Z-production-394397.json`。Monitor V2 继续只读 `account_monitor_results` 原生主动探测；固定 24/28/30 桶、倍率紧邻名称、移除“旗舰”，线上三项健康端点均 200。无迁移、无配置、无生产数据写入；登录态视觉截图按用户指令留作未验证项，不阻塞任务完成。
- **T35 采购保存 PostgreSQL 参数类型热修**：状态 `DONE`。独立用户可见任务 `01a01b11-3613-7ce0-811e-93d986e5cf16` 已完成真实 PostgreSQL 录入、清空、结算验证和三条 `jsonb_build_object` 精确类型修复；版本台账、事务、幂等、审计、成本公式和前端合同不变。无迁移、无生产数据写入；发布源、蓝绿结果和线上采购页面验收见项目进度总账。

发布顺序：T32、T33 已完成；T34 与 T35 可并行刷新/设计实施，但合并、部署和线上验收仍严格单车道，一个任务包一次发布。根总控按候选就绪与故障优先级选择唯一发布候选，不再等待用户真机反馈；真机问题作为后续独立反馈处理。

## 当前状态

- 队列状态：S1-R2、S2、S3、T15、T16、T17、T18、T19、T20、T21、T22、T23、T24、T25、T26、T26-R1、T27、T28、T29、T30、T31、T32、T33、T34、T35、T36、T37 与 T38 均为 `DONE`；T39/T40 为 `BACKLOG`；T39/T40 为 `BACKLOG`。所有发布继续禁止使用 GitHub Actions。
- 当前实施：T37、T38 已完成唯一发布车道的合并、推送、无停机蓝绿部署和线上健康验证；T39/T40 保持 BACKLOG，仅登记不提前实现；真机视觉验收按用户指令作为后续反馈，不占用发布车道。
- 唯一发布总控：根目录 `/Users/gongtengxinwen/Documents/sub2api搭建` 的 `main`。只有发布总控可以修改全局队列/总账、根 `main`、发布证据和生产状态记录。
- 当前发布状态：生产源 `main@12641c3281289ce66eed48f60e46b67f19d6d356`、tree `6375dc0a23bc1bf779114b895ea1b5caa60359fe`、迁移哈希 `18c4ac1fc83294634c42c6d08c6511c01515406f296d40b54840f3dae726949f`；T36 蓝绿链返回 `downtime_required=false`、`result=succeeded`、`state=promoted`、`rolled_back=false`，活动槽 `green`，API、worker 与 model-detector 使用同一不可变镜像 `sha256:57c4f8f1118887a77ede635debe96878f3758cc4e520acb0015f7d34ae2f6f35` 且健康。宿主记录为 `/var/lib/sub2api/release-records/20260819T194230Z-production-443041.json`；公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200；本地 0600 证据为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-20-main-12641c328-t36.json`。
- 非 `main` worktree 清理：T28/T29 两个功能 worktree、两个临时发布 worktree和两条已合并本地分支均已在生产验收后移除；恢复 bundle `/Users/gongtengxinwen/Documents/sub2api-archives/t28-t29-final-e0b2d99b/t28-t29-refs.bundle`，SHA-256 `a7815ce5a9111b07aea9026c6456f2d830019baacc142f46a5660451f086e741`，`git bundle verify` 通过。更早任务的清理证据沿用既有归档记录；当前仅保留用户指定保护的 `/private/tmp/sub2api-monitor-v3-preview` dirty detached 视觉证据。
- 全局审计（2026-08-19）：T28/T29 均已完成根复核、合并、推送、发布与生产验证并转为 `DONE`。T28 的生产专项保存保持只读，真实账号保存闭环、全量 OAuth 自购表和 CNY 行内录入入口登记为 T30；T30 顶层任务已创建并处于 `DESIGNING`，尚未占用整合/部署/验收车道。恢复 bundle `/Users/gongtengxinwen/Documents/sub2api-archives/t28-t29-final-e0b2d99b/t28-t29-refs.bundle` 已通过校验；用户指定保护的 detached 视觉预览 `/private/tmp/sub2api-monitor-v3-preview` 与根目录既有未跟踪资料继续保留。
- 最终归档：全量可恢复 bundle `/Users/gongtengxinwen/Documents/sub2api-archives/native-subtasks-final-44aaf3b70.bundle`，`git bundle verify` 通过，SHA-256 `88abe0117a85738311bf584c4d98b3fcdb4a178e821e0764571af7ef8fa381d6`。T15/T18/T19/T16 功能 worktree、分支及四个临时发布 worktree均在推送、部署、线上验收成功后安全移除；T19 根未跟踪规格/计划原件保留于 `/private/tmp/t19-root-untracked-backup.PuJrml/`，保护/历史 worktree 和根目录既有未跟踪资料未动。
- 原生错误中文提示配置已独立完成：生产 `ErrorPassthroughRule` 是全局规则、没有 `group_id`，因此一套配置已覆盖所有分组；该工作只调用 Sub 原生管理能力，不修改工程代码、不创建功能 worktree，也不占用发布车道。下一实施任务为 T09。
- 2026-08-10—2026-08-14 周复盘已纳入后续排序：P0 先修账号质量监控器 `203/EXEC Permission denied` 的可执行链路并完成真实运行验收；P0 将终端完成率作为 Pro 调度/经营硬门槛，不能只看排除业务失败后的平台 SLO；P1 继续处理余额/资格失败的账号准入否决和特惠账号稳定性风险；P1 规划卡片双口径（终端完成率、平台 SLO、排除量）；P2 为延时排名补充窗口、样本、模型构成、用户集中度和缓存命中上下文。以上是任务边界和验收约束，不代表本次 T08 顺带改动。
- 冻结项：S1 旧候选 `codex/upstream-resilience-s1-native-isolation@69a93343c` 因落后主线、Task 5 复审未闭合及迁移编号 `220` 冲突而 `FROZEN_FOR_REBASE`；T05 旧 detached `a71c675b1` 只作启动审计，轮到时从届时最新干净 `main` 重建。
- 流程偏差：T01、T02 虽有独立 worktree、规格书、计划和复审证据，但未建立用户可见的独立顶层 Codex 任务；T03 是纠偏前已在途并由根任务内部代理完成的任务。三者均不得宣称符合新增顶层任务门禁，已验证技术成果继续保留。
- 执行方式：最多两个互不依赖的功能 worktree 可并行准备；合并、推送、部署和线上验收严格单车道串行。每个新任务包必须从当时最新干净 `main` 创建用户可见独立顶层任务和独立 worktree。
- 模型规则：所有用户可见顶层任务统一使用 `GPT-5.6 Sol / medium`；任务内部 implementer/reviewer 子代理继续使用既定设置，不随顶层模型统一调整。
- 根任务职责：排队、创建顶层任务、读取交接、授权合并、合并后快速门禁、推送、部署和线上验收；不得用根任务内部 `spawn_agent` 代替整个任务包。
- 顶层任务职责：完整 brainstorming、书面规格书及用户批准、实施计划、实施与直接相关验证，并在 `READY_FOR_ROOT_REVIEW` 等待根任务授权合并 `main`；自 2026-08-16 起不再为形式增加额外复审或全分支终审。

## 队列

### T36 经营页 CNY/USD 额度关系明示文案

- 当前状态：`DONE`。独立任务 `01a01b65-be8f-7f53-b169-d9ee55456c37` 已从 `main@b02d538a0ac8d90b01ef92c45e55e72e11e9ee6b` 完成规格/计划/实现，刷新后合入根 `main`；实现合并提交为 `808be1901fb4fcb65869336041b777c76d9ee5e8`，生产源为 `main@12641c3281289ce66eed48f60e46b67f19d6d356`、tested tree `6375dc0a23bc1bf779114b895ea1b5caa60359fe`。
- 交付：在 USD/CNY 切换控件旁显示现有 i18n 驱动的中英文额度关系说明，并由直接页面合同测试锁定 `1 USD = 1 CNY` 的额度理解和“不是汇率换算”语义；不改账务、API、采购保存或数据。
- 合并后与发布门禁：页面+locale 32/32、`pnpm typecheck`、`pnpm build`、范围扫描和 `git diff --check` 通过；蓝绿链 `downtime_required=false`、`result=succeeded`、活动槽 `green`，公网三健康端点 200，未认证管理接口 401。无迁移、配置或生产数据变化。0600 证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-20-main-12641c328-t36.json`，宿主记录 `/var/lib/sub2api/release-records/20260819T194230Z-production-443041.json`；恢复 bundle `/Users/gongtengxinwen/Documents/sub2api-archives/t36-profitability-quota-parity-12641c328.bundle`，SHA-256 `f3f9a44db9c164a02aea88b24b55f96d62eb47146b57861fe412ccf88a6cc417`。真机视觉验收由用户自行完成，不阻塞后续。

### T31 Monitor V2 视觉放大、时间线交互与 CodexRadar 列对齐

- 当前状态：`DONE`。候选 `a742997a284719897eebcb6f6c5c82a2c60d2ed0`（功能提交 `8f846c962602af4238c71c817f9c701e798e9937`，刷新基线 `main@3097968e5`）已根审查、合并并推送为 `main@3a02d78833f245576478516ca9d395817f4d93c2`、tree `90111984d134101b76ac0b2e20827ee4e8dfd584`；T31 仅修改 Monitor V2 前端/i18n/直接测试，不含 T30 或后端改动。
- 交付：时间线固定 5px×16px 柱体与 4px 间距；悬停/键盘聚焦显示精确秒级时间、UP/DOWN、中文状态和延迟；滚动后 tooltip 夹在可视区域；整组 hover/focus 绿色底色、动画和强化倍率；可用率严格由真实 timeline 点计算，空样本显示无数据；页面最大宽度 1500px；站长推荐/社区矩阵放大；Radar 按 `ultra → max → xhigh → high → medium → low` 排列，同 effort 跨模型同列，缺档不渲染 placeholder，390px 仅组件内滚动。
- 验证：合并后 `pnpm vitest run src/features/monitor-v2/__tests__` 为 8 files/35 tests、`pnpm typecheck`、`pnpm build`、`git diff --check` 全部通过。0600 evidence `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-19-main-3a02d78833-t31.json`；宿主记录 `/var/lib/sub2api/release-records/20260819T133045Z-production-147673.json` 为 `succeeded/promoted`、`rolled_back=false`、`downtime_required=false`、活动槽 `blue`；API/worker/model-detector 同镜像且 healthy，共享 PostgreSQL/Redis/Caddy 身份未变，公网 `/healthz`、`/readyz`、`/health` 均 200。
- 线上验收：登录态 `/monitor` 中文页面显示真实可用率（如 95%）、3 组时间线共 180 个探测点、每点带秒级 `aria-label`，Radar 社区卡 19 个且 placeholder 为 0；hover tooltip 显示 UP/DOWN、时间、状态和延迟；390px 页面 `scrollWidth=clientWidth`，时间线/社区矩阵横向滚动仅发生在自身容器。候选与发布 worktree 已生成恢复 bundle `/Users/gongtengxinwen/Documents/sub2api-archives/t31-final-3a02d7883/t31-refs.bundle`，SHA-256 `174b762c003a5a964603c568745ba3caf81bc8d0e81d303d6d1c2cd1989124ab` 并通过 `git bundle verify`；用户指定的 `/private/tmp/sub2api-monitor-v3-preview` 继续只读保护。

### T26-R1 CodexRadar 三标签社区测试矩阵补齐

- 当前状态：`DONE`。独立用户可见顶层任务 `01a01859-d512-7cd1-88c8-0c1dc18ab023` 候选 `da80eb11aeda7bc5bc4981d862623f49172a9591` 已根审查、合并并推送为 `main@92610b809588939b0c27f3fa831e9b24ef086de4`、tree `1a11bc8532935207b20430f4df6ad48986880ae9`；新增登录态 GET-only `/api/v1/monitor-v2/codexradar-community`，补齐综合智能、软件工程能力、视觉空间推理三标签、原站综合口径、全量模型档位和社区指标。
- 根合并后直接相关 service/handler/routes 测试、8 项前端 Vitest、typecheck、production build、Go build、gofmt 与 diff-check 均通过；无迁移、配置、生产数据写入或 GitHub Actions。0600 证据为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-19-main-92610b809-t26-r1-community-matrix-v1.json`。
- 既有蓝绿链返回 `succeeded/promoted`、`rolled_back=false`、`downtime_required=false`，活动槽 `green`，宿主记录 `/var/lib/sub2api/release-records/20260819T052951Z-production-3967901.json`；线上登录态 `/monitor` 已确认三标签、社区众测说明、样本/IQ/平均费用/分钟字段和模型卡存在，390px `scrollWidth=clientWidth=480` 无整页横向溢出，三标签切换生效；公网健康三项均 200。
- 候选 worktree `/Users/gongtengxinwen/.codex/worktrees/5c03/sub2api搭建` 与分支 `codex/t26-r1-codexradar-community-matrix` 已在发布成功后归档删除；`/private/tmp/sub2api-monitor-v3-preview` 继续作为用户指定的 dirty detached 视觉证据只读保护。

### T27 自购账号保存、口径与双视图经营页重设计

- 当前状态：`DONE`。独立用户可见实现任务 `01a0187a-5c80-74f1-ad03-d658f01b9a52` 交付候选 `1114336908305a0ed0ea4211cdc5e2ac9aaefb7f`，根合并并推送为 `main@3bc16ee2682e6e978f73a71099c010a8353f2064`。合并后 Go service/handler、前端 23/23、typecheck、production build、Go build、gofmt 与 diff-check 通过；0600 证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-19-main-3bc16ee26-t27-oauth-dual-view-v1.json`；宿主记录 `/var/lib/sub2api/release-records/20260819T071706Z-production-4050553.json` 为 `succeeded/promoted`、`rolled_back=false`、`downtime_required=false`、活动槽 `blue`。线上默认 USD、CNY 自购专题 7 项摘要与长表、OAuth 过滤和 480px 无整页横向溢出均已确认；无迁移、无生产数据写入。设计任务 `01a01875-7991-72a3-9bd6-5a0af430b61e` 只交付信息架构和统计合同。
- 根因证据：`AccountProfitabilityService.UpdateProcurementConfig` 读取 `cost_pending` 活动版本时，把允许为 `NULL` 的成本/额度扫描到非空 `float64`，数据库驱动错误被前端归一化成 `internal error`；`GetSelfPurchasedReport` 当前只按采购投影/台账识别账号，未限制 `accounts.type='oauth'`；`AccountProfitabilityView.vue` 当前把自购面板渲染在财务摘要卡片之前。
- 目标：1) `cost_pending -> active` 重新录入采购成本时事务成功、幂等与审计保持不变；2) 自购报告 SQL、历史投影兼容分支和结算入口只纳入 `oauth` 且已有采购台账/投影的账号；3) 页面新增一级“经营结果 · USD / 自购专题 · CNY”切换，USD 视图保持 T16 原生五项摘要、分组和账号表，CNY 视图独立显示七项自购摘要与完整长表，两种币种不相加；4) self-purchased endpoint 支持 `today|24h|7d|31d`，与 USD 使用同一北京时间窗口，保留现有显式日期参数兼容；5) 两视图按需加载、刷新与错误态隔离，390px 无整页横向溢出。
- 非目标：不修改用户扣费、渠道 USD 经营口径、采购成本公式、账号调度、账号类型数据、历史 usage_logs 或生产数据；不新增迁移，不使用 GitHub Actions。
- 最小验证：后端 service/sqlmock 覆盖 `cost_pending` 重录、非 oauth 排除、结算过滤与四档北京时间范围；handler/API 合同；前端 self-purchased API、AccountMonitorView、AccountProfitabilityView 的双视图/按需加载/刷新/错误隔离/390px 回归；必要 typecheck、production build、Go build、gofmt 与 diff-check。无迁移、无生产数据写入，预期 `downtime_required=false`。
- 范围收敛：用户明确不为“切到 CNY、回 USD、改范围、再回 CNY”增加额外复杂竞态状态机；当前实现的按需加载与 `loadedRange` 基本保护满足本轮合同，不继续扩大设计。
- 发布顺序：T27 新候选达到 `READY_FOR_ROOT_REVIEW` 后先刷新最新 main，再由根总控审查、合并、最小门禁、推送、0600 证据、预检、蓝绿发布和线上专项验收；成功后归档 T27 和设计任务，不另立并行实现包。

### T28 评分方向、采购成本事实源与保存链路修复

- 当前状态：`DONE`。独立用户可见顶层任务 `01a0191a-9b73-7f72-9adb-13aa48e863e6` 已在原 worktree闭合根复核三项问题，最终候选合入并推送为 `main@5be1681c58ae9e66001193e400eac25d47fb24f4`、tree `40e33c4cc043a271b0d85e1ac7964769967c74f4`。账号卡片按 `group_rank` 升序进入最终 DOM；采购-only PUT 在台账事务后直接读取账号，不再进入通用更新；幂等键按账号、弹窗会话与 payload 隔离，未知结果重试复用，关闭重开、payload/保存模式变化和确认成功均轮换。
- 根合并后 Go 专项测试、server build、前端 101/101、typecheck、production build、gofmt、diff-check 与范围检查通过；无迁移、配置或 GitHub Actions。0600 evidence 为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-19-main-5be1681c5-t28.json`。
- 既有蓝绿链返回 `downtime_required=false`、`result=succeeded`，活动槽 `green`；release-state、API/worker/model-detector 镜像及宿主记录均绑定该 source/tree，不可变镜像 ID `sha256:359c1018f9bc4cf841d5659c68c5d34728526c8a5965a2642e52fd6454e11ad0`，相关容器 healthy 且重启计数 0，公网三项健康端点均 200。生产验收保持只读，未为验证幂等或采购事务制造账务写入；保存、清空、未知结果重试、reload 持久化与 DOM 顺序由同一已发布 tree 的直接测试覆盖。
- 范围：只修复目标评分组件最终 DOM 的强到弱顺序（左到右、上到下），保留评分算法与数值语义；采购成本继续复用 `accounts.procurement_cost_cny`、`accounts.estimated_usable_quota_usd` 与 `account_procurement_cost_versions`，不新增事实源或第二入口；查明并修复采购成本 PUT 保存 `internal error`，覆盖幂等键、handler/service 事务、NULL `cost_pending`、错误映射、成功 PUT+reload 反馈和重复提交幂等。
- 验收：桌面/390px 顺序稳定且无整页横溢出；账号监控与自购 CNY 页读取同一采购字段；新录入、修改、清空、重复提交、旧 NULL 版本、服务错误和 reload 保持均有直接测试；Go/前端聚焦测试、typecheck/build、gofmt、diff-check。无迁移、无生产写入、无 GitHub Actions，预期 `downtime_required=false`。
- 非目标：不调整评分权重或算法，不改账号监控其他卡片样式，不改变盈利口径，不扩展到其他页面或发布链。

### T29 Monitor V2 二态健康展示与统一指标口径

- 当前状态：`DONE`。用户可见顶层任务 `01a0191f-0386-7053-9cc1-9d01857dc92d` 的刷新候选已合入并推送为 `main@e0b2d99b91dcbaa20b1cb4d859cd58182795c60f`、tree `34ace5c193dd1c647215ed6894c7ec1945dd69b4`。合并后专项门禁通过；0600 evidence 为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-19-main-e0b2d99b9-t29.json`。蓝绿链返回 `downtime_required=false`、`succeeded/promoted`，活动槽 `blue`；宿主记录 `/var/lib/sub2api/release-records/20260819T102718Z-production-3917.json`，公网健康三项均 200。登录态 `/monitor` 已验证 v6、零旧百分比字段、严格二态、Pro 第一/旗舰、统一样本口径与 1432px/390px 无横向溢出。生产报告见 `docs/superpowers/reports/2026-08-19-t29-monitor-v2-health-semantics-production.md`。
- 展示合同：页面删除全部百分比型值，包括服务可用率、真实请求成功率、有效调用占比和缓存命中率；真实请求成功率也不进入明细、悬浮提示或无障碍文案。用户可见状态仅为“运行中 / 服务不可用”；运行中时间线统一绿色，服务不可用时间线使用故障色，卡片状态、整体状态和时间线使用同一二态投影。
- 指标合同：毫秒、TPS、倍率等非百分比性能事实继续来自真实数据；Monitor V2 的所有性能查询统一时间窗、`group_id`、有效计费文本请求资格及可比主模型范围，避免 TTFT、总延迟、TPS、缓存因分母和模型构成不同而不可比。Pro 固定置顶并标记“旗舰”，不复制 Plus 数值、不人工覆盖统计结果。
- 验收：TDD 覆盖百分号/成功率文案彻底消失、二态状态、时间线配色、Pro 置顶/旗舰和统一查询谓词；运行直接相关 Go service/repository tests、Monitor V2 Vitest、typecheck、frontend build、必要 Go build、gofmt 与 diff-check，并做桌面/390px 视觉核对。无迁移、无生产数据写入、无 GitHub Actions，预期 `downtime_required=false`。
- 非目标：不改变主动探测重试、计费价格、调度策略、分组成员、用户请求错误处理、CodexRadar 或其他管理页面。

### T30 真实采购保存与全量 OAuth 自购账号

- 当前状态：`DONE`。独立顶层任务 `01a019e7-34cb-7002-a91f-0a3211bdde7b` 候选 `5758e91adcd1deb30ad8b0d5c7f63f4e2c29c2e0`、tree `3d59e838fc3b458f0087f83bbfd5a59aa100a5b8` 已完成根审查、合并、推送、直接相关门禁、0600 evidence、无停机蓝绿发布和登录态专项验收。T30 未修改渠道监控、Monitor V2 或 CodexRadar；相关视觉优化继续由 T31 独立处理。
- 保存链：真实失败根因是超长幂等键写入 `audit_logs.request_id VARCHAR(64)` 使采购事务失败；审计写入现已按 schema 边界截断。输入错误、账号不存在和幂等冲突继续保留 4xx/409；真正内部错误返回中文 `message/reason/request_id`。台账提交后账号回读失败使用可识别的 HTTP 202 partial-success 契约，前端能展示“采购成本已保存，但账号刷新失败”，重复同键只重放、不重复创建版本。
- 自购报告与入口：`GetSelfPurchasedReport` 以全部未删除且 `type='oauth'` 的 Sub 原生账号为候选；无采购版本账号生成 `cost_pending` 投影，0 流水仍显示。CNY 自购表逐行提供“录入成本/编辑成本”，复用 `accounts.procurement_cost_cny`、`accounts.estimated_usable_quota_usd`、`account_procurement_cost_versions`、既有保存 API 和共享表单，默认预计额度 60 USD。
- 验证与发布：合并前后 Go focused、Go build、前端 37/37、typecheck、production build、gofmt 和 diff-check 通过；无迁移/配置变化。0600 evidence `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-19-main-5758e91ad-t30.json`。宿主记录 `/var/lib/sub2api/release-records/20260819T130153Z-production-122858.json` 为 `succeeded/promoted`、`rolled_back=false`、`downtime_required=false`，活动槽 `green`；API、worker、model-detector 使用同一 T30 镜像且 healthy，共享 PostgreSQL/Redis/Caddy 身份保持不变，公网 `/healthz`、`/readyz`、`/health` 均 200。
- 线上验收：生产数据库只读统计全部未删除 OAuth 账号为 17；登录态 CNY 自购表同样显示 17 行，其中 14 行“成本待录入”、3 行“编辑成本”，每行均有成本入口，页面未出现 `internal error`。窄视口下页面 `scrollWidth=clientWidth`，未出现整页横向溢出。用户最终收敛范围不再要求额外制造真实生产采购写入，因此本次验收保持只读。

### T26 用户错误中文投影与 CodexRadar 原生站长推荐接入

- 当前状态：`DONE`。用户可见顶层任务 `01a017cb-f2a7-7563-86fc-eb9afe141fed` 的中文错误投影与 CodexRadar 推荐候选已合入并完成首轮无停机发布；390px 线上验收发现时间线 64 个柱体横向溢出后，在独立修复候选 `c6aea3bdee57812cdadb424a4932bea5a7b0f4f5` 将柱体改为可收缩并约束容器溢出，增加窄屏回归测试，最终合入、推送并发布为 `main@9de147ad673ab23f92a59a36e9f075d8bbeb8897`、tree `be3d53d052dfb4fcb35f9c9e6e8661b1825be38c`。Monitor V2 18/18、typecheck、build 与 diff-check 通过；最终宿主记录 `/var/lib/sub2api/release-records/20260819T040727Z-production-3903052.json` 为 `succeeded/promoted`、`rolled_back=false`、`downtime_required=false`，活动槽 `blue`。线上登录态 `/monitor` 已确认中文界面、四类站长推荐及模型档位/IQ/耗时/费用/更新时间均存在；移动检查为 `scrollWidth=clientWidth=480`、无横向溢出；错误请求抽样的 404/403/503 响应内容均为脱敏中文。公网三项健康均 200，无迁移或生产数据修改。0600 证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-19-main-9de147ad6-t26-mobile-overflow-v2.json`；T26 worktree/分支已归档清理，恢复 bundle 与校验值见顶部清理记录。
- 错误语义：本站余额不足必须显示“余额不足，请充值后重试。”；本站额度、订阅、频率/并发、模型/分组权限、请求格式和服务资源类错误分别提供可操作中文提示。内部服务账号余额或外部服务异常对用户统一为透明的“服务暂时异常/繁忙”语义，不出现“上游”；管理员诊断继续保留阶段、归属、状态和经脱敏的原始证据。覆盖 Responses、Chat Completions、Anthropic，以及 JSON/SSE 终结路径，优先复用现有 `native_error_diagnostics` 与原生错误响应写入链。
- 推荐语义：服务端只读获取 CodexRadar `radar-insights`，做严格字段校验、短超时、短时缓存与最近成功快照回退；前端只渲染其四类推荐、模型/档位、IQ、耗时、费用和来源更新时间，视觉按用户截图复刻，保留 CodexRadar 原有分类配色。不得读取本站监控、计费或模型事实来替换推荐数据，不持久化或伪造外部结果，不提交评分或触发外部写操作。
- 验收边界：先写失败测试；只运行直接相关 service/handler、JSON/SSE 错误投影、推荐代理与 Monitor V2 组件测试，外加后端必要编译、前端 typecheck/build、桌面与 390px 页面专项验收和 diff-check。无迁移、无生产数据修改、无 GitHub Actions；预期 `downtime_required=false`，最终以根合并后的发布预检为准。
- 工作区边界：根发布总控只登记、审查、合并、推送、发布和线上验收；实现由新建用户可见独立顶层任务及独立 worktree 承担。`/private/tmp/sub2api-monitor-v3-preview` 为 T25 detached、dirty 的只读视觉证据，HEAD 已被 `main` 包含，继续保护，不清理、不合并、不作为 T26 基线。

### T25 自建渠道监控最终视觉与主动探测重试收口

- 当前状态：`DONE`。候选 `codex/t25-channel-monitor-final@3e49fb8e7` 已完成规格、计划、TDD、直接相关测试、构建和视觉核对，并无冲突合入根 `main@20c563345fe802b9662faf9189ca8cc7ecb3d3aa`；最终源已推送并通过无停机蓝绿发布。宿主记录 `/var/lib/sub2api/release-records/20260818T181312Z-production-3451615.json`，结果 `succeeded/promoted`、`rolled_back=false`、`downtime_required=false`，活动槽 `blue`；源 tree `e62afb7f22a9519ec985416a4994fb2fa216d4da`，迁移哈希保持 `18c4ac1fc83294634c42c6d08c6511c01515406f296d40b54840f3dae726949f`。线上通过原生管理员设置将 `channel_monitor_enabled=true`、`channel_monitor_mode=v1` 生效；`/monitor` 已渲染自建 Monitor V2，中文卡片保留 P50/TPS/缓存率/倍率，P95 不展示，有效调用文案为“基于 N 次真实请求。”。公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200。0600 证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-18-main-20c563345-t25-channel-monitor-v1.json`。临时历史预览 `/private/tmp/sub2api-monitor-v3-preview` 为 detached 只读视觉证据，不进入候选。
- 原生盘点：复用现有 `MonitorV2View`、`MonitorV2GroupCard`、`MonitorV2Timeline`、Monitor V2 API/统计合同和 `channel_monitor_service` 主动探测链；T19 已实现的 `actual_cost > 0` 有效服务响应统计继续作为真实请求口径，不新增监控事实源或平行探测器。
- 页面范围：保留旧版卡片结构、TTFT/总延迟 P50、TPS、缓存率及各指标样本数，不展示 P95；删除模型数量/展开行、模型列表、P95 解释和底部两条说明；将有效调用文案统一为“基于 N 次真实请求。”；倍率显著强化；趋势柱体统一青绿色、固定高度与宽度，耗时和探测结果不改变颜色或高低。中文预览通过既有 `sub2api_locale=zh` 验证，不修改全站语言默认或旧版布局。
- 探测范围：每轮首次主动探测失败后再重试 5 次，任一次成功即记录本轮成功，只有总计 6 次均失败才记录本轮失败；成功不继续重试。保持既有调度、账号隔离、计费和错误分类语义不变。
- 验收与发布：TDD 覆盖文案/删项/倍率/统一柱体，以及第 1 至第 6 次成功、六次全失败和成功后停止重试；前端 Monitor V2 27/27、typecheck/build、后端直接相关 service tests、gofmt 与 diff-check 已通过；全量 service 基线存在无关 GatewayServiceRecordUsage 请求 ID 断言失败，已记录在 handoff。无迁移、无生产数据写入，预期 `downtime_required=false`。发布脚本要求明确“部署生产”或等价授权；授权后才按既有本地/宿主蓝绿链发布并完成登录态页面、主动探测与公网健康专项验收。

### P0 Cloudflare 边缘 IP 误触发会话绑定事故

- 当前状态：`DONE`。生产先恢复 Sub 原生默认 `session_binding_enabled=false`，随后撤回旧 P0 自定义 refresh replay 状态机，使 auth/cache 核心恢复官方 v0.1.177；最终候选 `codex/p0-native-session-stability@f381c8802` 增加默认关闭的宿主级 `security.session_binding_allowed` 与显式 trusted-proxy 双重门禁，管理 API 无法单独重新启用绑定。
- 根 `main@e554b7d2ec02714ac2930eb54e3fd2ede460e3ca` 已推送并通过既有本地/宿主蓝绿链发布，`downtime_required=false`，活动槽 `green`；发布记录为 `/var/lib/sub2api/release-records/20260816T185827Z-production-1362380.json`，公网三个健康端点均为 HTTP 200。
- 管理员真实登录态已在“使用记录”加载后切换至“管理控制台”，刷新后仍保持登录并加载数据；“安全与认证”页面显示“会话 IP/UA 绑定”关闭。自发布时刻 `2026-08-16T18:58:27Z` 起，活动 API 与 worker 的 `auth.session_binding.mismatch` 均为 0。
- 范围保持原生：不接受任意 Cloudflare 转发头作为自动启用信号，不恢复密文 replay marker，不修改生产账号/token 数据，不使用 GitHub Actions；已在事故中撤销的 token family 不可恢复，受影响用户只需重新登录一次。

### P0 使用记录触发会话过期热修

- 当前状态：`DONE`。独立顶层任务 `01a00b57-1365-7712-8c31-58e97d5d0941`，候选 `c25fb9ad1` 已合并并随 `main@527f2195cbec517a72fbc05ee898b6999324aced` 推送、发布和线上验收。
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
- T15 已完成刷新并回到 `READY_FOR_ROOT_REVIEW`，保持独立 worktree/分支并受保护；S1-R2 已完成根 `main@2271b818` 的推送、预加载蓝绿发布与线上健康验收，进入 `DONE`。
- S1-R2 生产发布结果为 `succeeded/promoted`、活动槽 `green`、`downtime_required=false`，不可变镜像绑定 `main@2271b818`；本地 0600 证据为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-17-main-2271b818-s1-r2-maintenance-ready-v1.json`，公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200。未触发人为上游失败或修改生产账号；旧 S1 候选保持冻结。T15 已保留迁移编号 225，S1-R2 未使用 225/226。S1-R2 生产收口后，才允许按队列启动 S2，再完成 S2 生产验收后才允许启动 S3；三者严格串行，不得被 T16 或其他独立任务插队。
- “正在重新连接 1/5”与 `stream disconnected before completion` 已确认属于上游 SSE 在 `response.completed` 前断开的 S1-R2 冷却/故障转移范围。S1-R2 与 S2 均已进入 `DONE`；S3 的前置依赖已满足，但仍须按独立任务/worktree 门禁从最新干净 `main` 启动。

### S2 共享健康、故障域与抗故障重试

- 当前状态：`DONE`。独立候选 `codex/s2-shared-health-failure-domain@33d9fdb6a` 已整合根 `main@566fc52ba`，无冲突合入 `main@d1f9bc06c`，最终发布源为已推送的根 `main@aab79007f`。合并树及最终干净发布 worktree 的 repository/service/handler/config 聚焦测试、server compile-only/build、gofmt、diff-check、零迁移和零 GitHub Actions 范围检查均通过；真实 Redis integration-tag 用例仍被既有无关 `stringPtr` 重名编译冲突阻断。既有预加载蓝绿链返回 `succeeded`、`downtime_required=false`、活动槽 `blue`；API/worker 同镜像且 healthy/restart 0，PostgreSQL/Redis/Caddy 身份未变。公网三项健康端点均 200；自然流量已生成 8 个 healthy account-model 投影、2 个故障域投影和 13 个幂等事件标记，API/worker 15 分钟内 shared-health 告警、panic、fatal 均为 0。未修改生产账号或人为制造上游故障。恢复 bundle 已验证后，S2 功能 worktree、本地分支和临时发布 worktree均已安全删除；T15/T16/历史保护 worktree 未动。交接与验证见 `docs/handoffs/2026-08-17-s2-shared-health-failure-domain-handoff.md`、`docs/superpowers/reports/2026-08-17-s2-shared-health-failure-domain-verification.md`。
- 目标：以 Redis 承载可重建的跨实例账号模型 transient/EWMA/half-open 与故障域运行时投影；保持 S1 数据库确定性隔离为唯一权威；为单一逻辑请求统一最大尝试数、账号切换数、故障域数和总重试预算；429 尊重 `Retry-After`，5xx/连接错误受有界指数退避；Redis 故障按本地 fail-safe 降级，不放行 S1 veto、不造成全站失败。
- 独立边界：不改变 S1 分类/原生状态、不改变 Top-K/粘性体验、不迁移管理员审计到 Redis、不改变价格、倍率、账务或外部控制面；不开启默认 TTFT 并行竞速；目标发布属性 `downtime_required=false`。
- 依赖与门禁：S1、S2 均已完成生产验收；S3 的依赖已满足，可按队列从最新干净 `main` 创建独立任务/worktree。T15/T16 与历史冻结候选的保护状态不变。

### S3 自适应选择、粘性逃逸与调度体验观测

- 当前状态：`DONE`。独立候选 `codex/s3-adaptive-scheduling-experience@026b7b26d` 已无冲突合入并推送根 `main@0720b8bf0b5e23486904e571f12b483e7329a9c0`；合并树的 config/service/handler/admin/routes focused tests、server compile/build、前端 3 files / 9 tests、typecheck/build、diff-check、零迁移和零 GitHub Actions 范围检查均通过。既有本地/宿主蓝绿链返回 `result=succeeded`、`state=promoted`、`rolled_back=false`、`downtime_required=false`，活动槽 `green`；宿主记录为 `/var/lib/sub2api/release-records/20260817T093040Z-production-1990545.json`，API/worker 使用同一不可变镜像，迁移哈希保持 `aaebed88f7fb712e1f518e73cc89bd44eb214f365f3b49f003598c93883a4604`。公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200。登录态 Ops Dashboard 已显示自然流量样本（22）、平均尝试 1.00/P95 1、sticky 保留 20/20、sticky 逃逸 0/20、Top-K 过滤 21/26（80.8%）、TTFT report-only 合格 2/22（9.1%）；自动恢复、重复坏账号、预算耗尽均为自然 `no_data 0/0`，未制造失败或修改生产账号。0600 发布证据为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-17-main-0720b8bf0-s3-adaptive-scheduling-v1.json`；恢复 bundle `/Users/gongtengxinwen/Documents/sub2api-archives/s3-adaptive-scheduling-experience-026b7b26.bundle` 已验证，SHA-256 `b8f64d71e4659dab7bd01b499b3015c1c209d24b04de7b40cbd7b77c339823c4`，随后已清理 S3 worktree、分支和临时发布 worktree。
- 目标：消费 S1/S2 的健康与预算决定，先健康门槛、再动态 Top-K/最低质量阈值、再可解释 sticky escape；仅对安全重放且尚未输出的请求做 TTFT report-only/受控预热；在现有原生监控/运维入口呈现自动恢复率、平均尝试数、坏账号重复命中率、缓存代价和预算耗尽率。
- 独立边界：不重新定义错误状态、S2 重试上限、价格、账务或控制面；默认不启用并行竞速；S1/S2 veto 永远优先于分数和 sticky；目标发布属性 `downtime_required=false`。

### T12 经营页本站探测花费与排序/美元字段优化

- 当前状态：`DONE`。原候选 `c7587599a` 因 P0 插队被根主线撤回；P0 收口后，T12 worktree 快进 `main@b16d45203`，仅反向撤销移除 T12 运行时/任务文档的 `9d9e2b758`，生成运行时候选 `35baf14ae` 和最终 handoff 候选 `4029240f4`。该候选已无冲突快进合入；首次根发布树 `59316b4a9` 的预检暴露 T12 新迁移尚未加入宿主精确维护白名单，根总控随后以 `04d171e35` 仅补齐 `ef121384… -> aaebed88…` 的 maintenance-10 哈希对、fail-closed 测试和当前 runbook，并推送至 `origin/main`。
- 验证与生产收口：干净发布树的四组 focused Go、前端 19/19、typecheck/build、维护白名单精确允许/错误 old/new/未授权拒绝、bash 语法和 diff-check 均通过；0600 证据为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-17-main-04d171e35-t12-probe-cost-maintenance-ready-v1.json`。用户明确授权本次维护发布后，链路返回 `succeeded`；宿主记录 `/var/lib/sub2api/release-records/20260817T005154Z-production-1618298.json` 为 `promoted`、`rolled_back=false`，活动 `blue` 与 worker 使用同一 `04d171e35` 镜像。公网健康均 200；管理员页在线显示全站/7 分组/93 账号卡、六项排序、USD 两位、自然探测成本及不完整/暂无记录状态，390px 单列且页面无横向滚动。候选 bundle `/Users/gongtengxinwen/Documents/sub2api-archives/t12-native-probe-cost-4029240f4.bundle` 已验证，活动 worktree/分支和 T12 临时发布 worktree已清理。
- 实施边界：沿用批准规格与计划，不新增产品范围，不恢复旧 Task 4 RED，不运行全仓测试或额外 reviewer。T12 是当前唯一允许进入 `INTEGRATING`、`DEPLOYING` 或 `VERIFYING` 的候选，worktree/分支保留到生产验证成功。
- 恢复设计结论：独立 docs-only 分支 `codex/account-probe-cost-design@50567e862` 把页面合同修订为“全站 -> 分组 -> 账号”三层、账号层独立卡片、桌面最多两列/390px 单列/无横向滚动，并统一外部金额为 USD 两位与利润率 0.00%；这些合同及 Task 1-3 的隔离账本、原生定价、fail-open、probe 聚合均已落实到最终候选 `c7587599a`。旧 Task 4 RED 已废弃且未带入；本轮不重做功能，只将该已验证候选刷新到最新主线并复跑直接相关门禁。
- 目标：保持未消费金额为 USD；补充六项排序（请求、Token、账号计费、用户扣费、利润、利润率）；新增独立“本站探测花费”字段、卡片和账号列。
- 范围：探测记录与用户消费隔离；探测花费不影响账号成本、用户成本、利润或利润率；外部金额两位小数、内部原始精度保留；不做历史迁移/回填，启用后重新记录。
- 非目标：不改变用户消费、账号计费、利润/利润率、余额事实源、调度/路由、普通用户入口，不建设第二账务源或外部控制面。
- 设计前置：必须先复用现有 Sub 原生 `usage_logs`、账号探测/测试链路和经营页；只有正式规格证明原生能力不足后才可做最小扩展。规格批准前不得写计划或代码。
- 旧设计证据：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/account-probe-cost-design`、分支 `codex/account-probe-cost-design@893933924` 仅作冻结的 docs-only 设计证据，不作为 T12 顶层任务、不继续写入、不合并或部署。

### T13 NewAPI 上游倍率自动登记

- 当前状态：`DONE`。刷新候选 `codex/newapi-rate-multiplier-registration@8faf65547` 已合入并随根 `main@3673d5a9a` 推送；既有预加载蓝绿链返回 `succeeded`、活动槽 `blue`、`downtime_required=false`，宿主记录为 `/var/lib/sub2api/release-records/20260816T171553Z-production-1285992.json`。公网三项健康检查均 200；管理员登录态账号页正常加载 92 个账号、显示“上游声明倍率”列，发布资源 `AccountsView-rMmhHhPy.js` 包含 `rate_registration/registered` 合同。当前无自然“已登记”样本，未为验收制造请求或修改生产账号；功能将在后续首笔合格真实请求后按合同登记。
- 权威输入：仅接受 NewAPI 精确匹配日志中的 `other.group_ratio`；仅适用于 NewAPI API-key 且没有原生 Sub 倍率声明的账号。
- 写入语义：首次真实成功请求后登记 `accounts.rate_multiplier`，并在 `accounts.extra` 标记来源/登记状态；已登记账号按北京时间自然日仅首笔合格请求刷新一次。
- 并发与失败：使用 CAS 防止并发覆盖；失败不得覆盖既有倍率或登记标记；管理员可见“已登记”。
- 非目标：不做数据库迁移、历史回填、生产数据修改，不扩展到 OAuth、非 NewAPI、已有原生倍率声明账号或其他上游日志字段。

### T14 用量详情上游扣费/利润字段兼容热修

- 当前状态：`DONE`。用户可见 GPT-5.6 Sol/medium 顶层任务 `01a00a15-a76c-7be1-b66f-7a34ddb2b749` 的候选已随 `main@200d4b1c9e4745a6a54e467630c68aba14fb4028` 推送并通过本地/宿主蓝绿链切换，脚本结果为 `succeeded`、活动槽 `blue`、`downtime_required=false`。`/healthz`、`/readyz`、`/health` 均为 HTTP 200；刷新到已发布前端包后，管理员详情对自然确认样本 `usage_log_id=120896` 正确显示上游实际扣费 `$0.001010` 和利润 `$0.000505`。无迁移、配置、依赖或生产数据改动；回滚依据为宿主上一 `green` 槽/镜像和 release record。
- 已确认根因：`/admin/usage/:id/upstream-cost` 返回 PascalCase 字段，例如 `NormalizedCostCNY`、`EvidenceStatus`；详情弹窗仅读取 snake_case 字段，例如 `normalized_cost_cny`、`evidence_status`，因此“上游实际扣费 / 利润”错误显示为 `-`，不是生产数据缺失。
- 范围：仅对该详情弹窗/API 响应做向后兼容字段归一化，并保留 PascalCase 与 snake_case 两种响应兼容；只做直接相关页级/API 合同验证、必要类型检查/构建、diff/范围检查和发布后定向验收。
- 非目标：不得并入 T12，不改变账号成本、用户扣费、利润/利润率口径或聚合，不做数据库迁移、历史回填、生产数据修改、账务重算、相邻页面重构或外部控制面。

### T15 账号监控原生探测模型与异步模型检测

- 当前状态：`DONE`。T15 已随根 `main@3e5f9393d948603019fdde212957efdbbad0d715`、tree `deadf8ec212b05c4555a108ba0b627bb12030112` 推送并通过授权维护链发布。新增 migration 225 使迁移哈希从 `aaebed88…` 推进到 `bb6ebff3…`；精确 `MAINTENANCE_11` allowlist、错误 old/new/未知/未授权拒绝、控制器合同、T15 后端与前端专项门禁均通过。宿主记录 `/var/lib/sub2api/release-records/20260817T174502Z-production-2353131.json` 为 `succeeded/promoted`、`rolled_back=false`，活动槽 `green`，API/worker 同镜像且 healthy/restart 0；PostgreSQL、Redis、Caddy 保持原容器身份。公网健康均 200；两张检测表存在；新增管理员路由保持 401 认证隔离；登录态账号监控显示 87 个账号、模型检测状态行与弹窗。生产 detector URL/token 保持未配置，页面按合同显示“不支持”。完整报告见 `docs/superpowers/reports/2026-08-17-t15-native-probe-model-detection-production.md`。
- 原生连接测试：继续复用 `AccountTestService.ProbeAccountConnection`；每账号持久化独立 `connection_probe_model`，默认优先 `gpt-5.6-sol`，不支持时回退 Sub 原生登记的首个文本模型，页面不显示“自动选择”。近期探测标题旁提供修改连接测试模型入口。
- 异步检测模型：新增独立 `model_detection_model`；可选模型是 Sub 原生账号模型登记（`GET /admin/accounts/:id/models` 及 sync-upstream 结果）与检测器运行时基线目录的交集。原生模型可见但无基线时置灰并显示“检测器暂不支持”。
- 执行架构：检测器为仅执行的独立 sidecar，Sub worker 负责调度、持久化和唯一事实。北京时间 `00:00/10:00/12:00/15:00/18:00/21:00` 检测所有未删除、API Key 且检测器支持的账号，不受可调度状态影响；OAuth 不执行，每个任务只跑一轮探针。单账号可立即异步检测，同账号已排队/运行则复用；固定时隙持久化去重，错过仅在 30 分钟内补触发。
- 页面合同：每个 `AccountMonitorCard.vue` 卡片增加默认收缩的一行检测状态；点击弹窗查看最近结果、申报模型、Juice 摘要、行为指纹候选/相似度、检测器版本、时间/错误，并可修改检测模型或立即检测。无全局摘要，不改变 `AccountMonitorView.vue` 现有卡片主样式。
- 状态与隔离：状态仅为未检测、排队中、检测中、正常、异常、证据不足、检测失败、不支持；检测结果不参与质量评分、调度权重、可调度状态或分组建议。异常只能表述“检测器观察到异常”，不得表述“上游确认替换”。
- 安全与失败：不保存或记录 API Key、完整提示词、完整输出或上游地址；凭据只在私网内存中传给 sidecar。sidecar 故障仅记录检测失败，不影响原生连接测试或账号状态；账号删除、变 OAuth 或模型失效时，执行前跳过或回退。
- 验证：migration focused、service/repository/routes focused、backend compile-only、前端 2 files / 93 tests、`npm run typecheck`、`npm run build` 与 `git diff --check` 均通过；未运行全仓测试、额外 reviewer、压力/mutation/浏览器矩阵。
- 许可证与配置门禁：参考检测器目录 `tools/gpt56_api_detector-git` 为 PolyForm Noncommercial 1.0.0；T15 未复制其核心实现或基线。未取得商业书面授权或合法独立实现前，生产不得配置 `SUB2API_MODEL_DETECTOR_URL` 或 `SUB2API_MODEL_DETECTOR_TOKEN`；未配置时页面显示“不支持”，不影响原生监控。
- 发布边界：候选阶段尚未运行发布预检、未合并根 `main`、未推送、未部署或线上验收；继续使用本地/宿主发布链，不增加 GitHub Actions。发布预检若返回 `downtime_required=false`，按全局约束无需再次询问即可继续；若返回 `true`，必须停在用户授权门禁。原生证据入口为 `backend/internal/service/account_monitor_probe.go`、`account_monitor_service.go`、前端 `AccountMonitorCard.vue` 与 `AccountMonitorView.vue`。

### S1-R2 确定性故障原生隔离编排

- 当前状态：`DONE`。用户可见顶层任务 `01a00da8-ed25-7b72-b9d9-cdcee5fa75c1` 已合入并推送根 `main@2271b81874d9dfc5eb0894bd02e0f30c2a1f085b`；合并后直接相关 service/unit/config/compile-only/build/gofmt/diff-check 全部通过。生产发布结果为 `succeeded/promoted`、活动槽 `green`、`downtime_required=false`，不可变镜像为 `ghcr.io/leesssong/xingqiao-sub2api:release-2271b81874d9dfc5eb0894bd02e0f30c2a1f085b-93a0a891bcaf6acc2457fa37329cb86229199c6545bf67259e96b8cae5ca01ba`；本地 0600 证据为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-17-main-2271b818-s1-r2-maintenance-ready-v1.json`。公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200，未触发人为上游失败或修改生产账号；交接见 `docs/handoffs/2026-08-17-s1-r2-native-deterministic-failure-isolation-handoff.md`。
- 目标：把明确且可确定归因的上游账号/模型故障映射到 Sub 原生隔离、冷却与恢复机制，包括页面“正在重新连接 1/5”、`stream disconnected before completion` 以及上游 SSE 在 `response.completed` 前断开的账号模型冷却/故障转移边界。
- 已有合同：余额不足复用原生 `temp_unschedulable`（默认 90 分钟，允许范围 60–120 分钟）；确认凭据失效使用原生 `status=error/schedulable=false` 并要求受控探测或管理员恢复；明确模型不支持使用原生 `model_rate_limits`，作用域为账号 + canonical model；episode 仅作审计解释，不形成第二套 scheduler veto。
- 实现与验证：余额不足统一落原生 90 分钟账号冷却（配置允许 60–120，越界回退 90）；明确模型不支持落账号 + canonical model 的原生 `model_rate_limits/probe_required`；API Key 明确凭据失效继续原生 error/不可调度；未收到成功终态的 SSE 进入既有账号模型 transient，同时保留输出后禁止重放。直接相关 service、unit 回归、config、受影响包 compile-only、server build、gofmt 与全候选 diff-check 通过；无迁移、未使用 225/226、无 GitHub Actions 变化。
- 安全边界：泛化 403、网络失败、空/截断/不完整模型清单不得硬隔离；继续复用现有 transient cooldown、half-open、sticky、scheduler outbox、计费幂等和流式恢复。`downtime_required=unverified`；用户解除部署冻结前不得合并、push、预检、部署或触碰生产，S2/S3 不得启动。

### T16 经营页真实结果与视觉层级重设计

- 当前状态：`DONE`。T16 刷新候选 `84b08ac9cbef6abeb4cb16b3cb2f36863f8f2164` 已合入并随根 `main@ad49f9004418d779dfb0d7967d3fc3681486fbbe` 推送和无停机发布；宿主记录 `/var/lib/sub2api/release-records/20260817T184654Z-production-2406458.json` 为 `succeeded/promoted`、`rolled_back=false`，活动槽 `blue`。登录态经营页与原生财务 API 均已验收。报告：`docs/superpowers/reports/2026-08-17-t16-profitability-visual-hierarchy-production.md`。
- 默认视图与字段：默认打开“全部真实结果”；账号明细只显示运营消耗、业务消耗、业务营收、总消耗、净利润五项。摘要强调业务营收、总消耗、净利润和对外毛利率，并单独显示“内部运营消耗”且说明已包含在总消耗中。
- 原生事实源与公式：继续复用同一 Sub 原生 `usage_logs`，不建立第二套账务源，不改变 `cost`/`user_cost` 基础公式。运营消耗为管理员/内部使用的上游 `cost`；业务消耗为对外业务上游 `cost`；业务营收为对外用户 `user_cost`；总消耗为运营消耗加业务消耗；净利润为业务营收减总消耗。管理员免费使用仍保留真实上游成本，归为内部运营消耗，不能从总成本删除。
- 身份边界：禁止用 `user_cost=0` 猜管理员身份。正式规格必须先核查当前 `usage_logs` 与用户角色事实，说明用 `user_id/role` 查询时的历史角色变化风险；若需要不可变 actor type，必须作为最小数据契约变化单独论证，不得无声回填或猜测历史。
- 视觉合同：业务营收使用蓝色语义，真实上游消耗使用琥珀色，内部运营使用紫色，净利润使用绿色，真实亏损/内部补贴成本使用红色或警示语义；账号明细为紧凑表格。桌面层级清晰，390px 摘要两列且整页无横向溢出；若明细采用受控横向滚动，必须限制在表格容器内。
- 发布边界：本项保持冻结，不执行原生盘点、brainstorming、planning、实现或测试。只有 S1-R2 完成生产验收、S2 完成生产验收且总控重新 GO 后，才可解冻并从届时最新干净 `main` 重新核对基线。

### T17 用量详情“上游扣费/利润”统一 Sub 原生有效账号成本口径热修

- 当前状态：`DONE`。T17 已无冲突合入并推送根 `main@892db8cefb37bcab14b0aded8082811ac3935f48`；前端 38/38 focused tests、typecheck/build、后端管理员/DTO focused tests、server compile-only/build 和范围检查均通过。普通预加载蓝绿链完成 `succeeded/promoted`，活动槽 `blue`，迁移哈希未变，API/worker 同镜像且 healthy/restart 0，公网三项健康端点均 200。登录态页面确认 evidence unavailable 时详情仍显示有效账号成本和利润并与列表一致；控制器在宿主成功 final record 后遇到 SSH 关闭产生本地假阴性，已通过 release-state、final record、容器和标签只读核对完成收口。宿主记录 `/var/lib/sub2api/release-records/20260817T102828Z-production-2034943.json`，0600 证据 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-17-main-892db8cef-t17-effective-account-cost-v1.json`。恢复 bundle `/Users/gongtengxinwen/Documents/sub2api-archives/t17-effective-account-cost-hotfix-9ffbdbc2.bundle` 已验证，SHA-256 `c8aa71b345f74486e97cafdd2a6078afe22b8fa6da62c7c35386646c767c3879`；功能 worktree/分支和临时 release worktree 已清理，另一个既有用户可见 T17 worktree 未动。不得并入 S3。
- 已确认问题：使用记录列表与账号利润/经营页均使用 Sub 原生有效账号成本 `COALESCE(account_cost, COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1))`；用量详情弹窗却以 `usage_upstream_cost_evidence.normalized_cost_cny` 决定主金额，严格 evidence 为 `unavailable` 时显示 `-`，造成同一流水口径不一致；T14 只修复了 PascalCase/snake_case 兼容，未修改事实源。
- 生产证据：`usage_log_id=125444/125509/125512` 的 `account_cost` 分别为 `0.0033144600/0.0058255200/0.0060059400`，对应利润为 `0.0022096400/0.0038836800/0.0040039600`，详情当前均显示 `-`。账号 214 当日 518 笔均有 `account_cost`，账号成本合计与利润页成本均为 `4.9629669888`，用户扣费 `8.2716116480`，利润 `3.3086446592`，但 518 笔严格 evidence 均为 `unavailable`。两个详情 API 均 HTTP 200，证明为选错主事实源而非接口失败。
- 目标：详情“上游扣费”读取 effective account cost，利润统一为 `actual_cost - effective_account_cost`；历史 `account_cost` 为空时使用上述 fallback。`usage_upstream_cost_evidence` 只作严格账单核验状态/原因，不得决定主金额是否显示或成为利润主事实源。同一流水在列表、详情和账号利润/经营页的成本数学值必须一致，允许展示精度不同。
- 最小验收：增加直接相关前端/API/公式回归，覆盖 `account_cost`、历史 fallback 和 `evidence_status=unavailable` 三类；不改价格、倍率、`actual_cost/account_cost` 写入逻辑或经营页聚合公式。
- 边界：无数据库迁移、历史回填、生产数据修改、账务重算或历史 evidence 表删除；不并入 S3，不使用 GitHub Actions。预检 `downtime_required=false` 时按全局规则直接发布，为 `true` 时暂停请求授权。

### T18 渠道状态官方聚合/自建监控可切换

- 当前状态：`DONE`。T18 刷新候选 `99f7d5f7cde8926d57bd095c331483afb8040f8a` 已合入并随根 `main@80e5fe2a66a5eef11ad220ff280c7e3796dbb2d7` 推送和无停机发布；宿主记录 `/var/lib/sub2api/release-records/20260817T180044Z-production-2367549.json` 为 `succeeded/promoted`、`rolled_back=false`，活动槽 `blue`。生产公开设置为 `channel_monitor_enabled=true`、`channel_monitor_mode=v2`；登录态 `/monitor` 显示官方渠道监控聚合页，资源记录中 `/api/v1/monitor-v2` 请求为 0。公网三项健康均 200，API/worker 同镜像，共享服务身份不变。报告：`docs/superpowers/reports/2026-08-17-t18-channel-status-official-toggle-production.md`。
- 范围：仅改 `MonitorV2RouteView` 入口与专项测试；复用已有 `channel_monitor_mode=v1|v2`。`v2` 直接渲染官方 `ChannelStatusView` 并跳过 `/api/v1/monitor-v2`，`v1` 保留自建页及失败回退；无后端、迁移、配置 schema 或 GitHub Actions 变化。
- 验证：`MonitorV2RouteView` 1 文件 3 tests、`pnpm typecheck`、`pnpm build`、`git diff --check` 均通过；预期 `downtime_required=false`，最终以根合并后的发布预检为准。上线参数为 `channel_monitor_enabled=true`、`channel_monitor_mode=v2`；回滚为 `channel_monitor_mode=v1`。
- 车道约束：T15 当前仍停在 `downtime_required=true` 的停机授权门禁；T18 不自行插队、合并、推送、发布或改生产配置，待 T15 生产收口或明确冻结后再由根总控单独授权。

### T19 Monitor V2 缓存命中率有效样本口径修正

- 当前状态：`DONE`。T19 刷新候选 `b2945a5d35fe05381f5095ae354144284a5a01a7` 已合入并随根 `main@949f200f3ad6fc0455cef7788abdc941a756c65f` 推送和无停机发布；宿主记录 `/var/lib/sub2api/release-records/20260817T181347Z-production-2379500.json` 为 `succeeded/promoted`、`rolled_back=false`，活动槽 `green`。24h/7d API 与固定 `generated_at` SQL 交叉核对中，三组有效样本和命中数全部一致。报告：`docs/superpowers/reports/2026-08-17-t19-monitor-v2-cache-eligibility-production.md`。
- 候选：worktree `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t19-monitor-v2-cache-eligibility`，分支 `codex/t19-monitor-v2-cache-eligibility`，基线 `main@8729884a113cf844a2850ba87463c2f7f711577c`，候选 tip `1b8832461`，tree `1484c609a9a4f4281eae6dcf0ce71b1f16d0c6a6`，刷新合并提交 `c4fc01c53802300bec61c5c8e5d55c58cff82a2a`；功能提交 `0f9ef38f2a0621d9afe5b5c965da025161dba399`；交接 `docs/handoffs/2026-08-17-t19-monitor-v2-cache-eligibility-handoff.md`。
- 规格与计划：`docs/superpowers/specs/2026-08-17-monitor-v2-cache-hit-rate-eligibility-design.md`；`docs/superpowers/plans/2026-08-17-monitor-v2-cache-hit-rate-eligibility.md`。候选已携带正式规格、计划和交接文件；待发布前须刷新到届时最新干净 `main` 并重跑直接相关门禁。
- 范围：仅修正 `upstream/sub2api/backend/internal/repository/monitor_v2_repo.go` 的 Monitor V2 缓存统计 SQL 及直接相关 sqlmock 测试。分子/分母统一限定为 `actual_cost > 0`、成功流水且具备文本 Token Prompt Cache 语义：`billing_mode='token'`，或历史 `billing_mode` 为空且图片/视频字段全零；排除 `billing_mode=image|video|per_request` 及 `actual_cost=0` 的失败占位。保持 API 响应、前端、账务/价格/倍率、缓存策略不变；无迁移、无生产数据写入，预期 `downtime_required=false`。
- 验证与发布：TDD RED/GREEN、仓储/服务聚焦测试、后端 compile-only/build、gofmt、diff-check 已通过；发布后仍需进行 24 小时/7 天只读交叉验收。预检若返回 `downtime_required=false`，按全局约束直接继续蓝绿发布与线上验证；若返回 `true`，停在用户授权门禁。当前按用户指令暂停所有发布动作，不得使用 GitHub Actions。

### T20 用量详情过时提示清理与盈利页零流水账号补齐

- 当前状态：`DONE`。候选 `3b120046e328535ce587db60a5ef750586d652d0` 已合入并推送根 `main@c2a1429623b22d0e5c3d4746a508d0f34e0a93e9`，tree `e41c1460b05ad1b040c2700fb54227b2dad0f947`。
- 候选：隔离 checkout `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t20-usage-detail-zero-flow`，分支 `codex/t20-usage-detail-zero-flow`，基线 `main@d579e6f99f4f281227578676dff060df92e3f870`，提交 `3b120046e328535ce587db60a5ef750586d652d0`，bundle `/private/tmp/t20-usage-detail-zero-flow-ready.bundle`（SHA-256 `09f352f1c19c14336c280f24342d4366735933377578a7a2678214a4c0800c82`），交接 `docs/handoffs/2026-08-18-t20-usage-detail-zero-flow-handoff.md`。
- 范围：删除 `UsageDetailDialog` 中过时的严格上游账单提示及对应前端断言；保留后端 evidence 接口、`evidence_status` 与 `reason_code`，不改变 T17 已上线的有效账号成本主口径。`account-financial` 分组读模型先从 `account_groups` 加载全部有效账号并初始化零值，再叠加 `usage_logs` 与探测成本聚合，使时间窗内无流水的有效账号仍显示且金额均为零。
- 原生事实与边界：复用 Sub 原生有效账号、`usage_logs`、现有财务字段和聚合公式；保持 API 响应、字段结构、成本/收入/利润数学、探测成本语义及账号绑定状态不变。不做迁移、回填、生产数据写入、账务重算、evidence 表删除、S3/T22 顺带实现或 GitHub Actions。
- 验收与验证：详情过时提示消失；分组账号数与当前有效绑定一致；零流水账号金额为零；有流水账号与现有结果一致。新增直接相关前端/API/读模型回归，执行后端仓储/服务聚焦、受影响包 compile-only/build、前端聚焦测试、typecheck/build、gofmt 与 diff-check。候选已完成根合并、推送、发布和线上验收；`downtime_required=false` 按全局规约直接蓝绿发布，`true` 停在停机授权门禁。
- 生产与验收：既有本地/宿主链返回 `succeeded/promoted`、`rolled_back=false`、`downtime_required=false`，活动槽 `blue`，宿主记录 `/var/lib/sub2api/release-records/20260818T051214Z-production-2858199.json`。三项公网健康均 200；生产用量样本 125444/125509/125512 的 `actual_cost` 与 `account_cost` 继续按 T17 口径返回，evidence 仍为 `unavailable` 但不再决定主金额；24h 财务读模型中零流水有效绑定账号仍出现且金额全为零。回滚使用宿主保留槽与上一已验证镜像，不改生产数据。

### T21 生产模型检测 sidecar 接入与离线状态纠正

- 当前状态：`DONE`。sidecar 收口候选合入后，首次发布在生产变更前由镜像构建门禁发现 detector Go module cache 未指定 target；根以 TDD 增加 Dockerfile 合同并修复，最终 `main@aee203ac4` 已推送、无停机蓝绿发布和线上验收。
- 候选：`codex/t21-model-detector-sidecar@7120593f2db99757b4cf0d7de664d40e18391320`，基线 `main@74aa0d0126e7097cecb4d6d6df33b767da65a494`，worktree `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t21-model-detector-sidecar`，交接 `docs/handoffs/2026-08-18-t21-model-detector-sidecar-handoff.md`。
- 范围：后端显式区分 `ready`、`unconfigured`、`unavailable` detector 状态并通过现有 admin API/projection 暴露；前端显示“检测服务未接入/暂不可用”，仅在 `ready` catalog 中对未收录模型显示“检测器暂不支持”；原生连接测试和账号卡片原生探测保持不变；Compose 向 blue、green、worker 透传既有 URL/token 配置。
- 验证：后端检测器 focused tests、前端账号监控 `51/51`、typecheck/build、Compose 合同、compile-only、gofmt 与 diff-check 均通过；无迁移、无业务数据写入，预期 `downtime_required=false`，以根预检为准。
- 生产验收边界：宿主当前未配置 `SUB2API_MODEL_DETECTOR_URL/TOKEN`，也没有合规 sidecar 制品；本次发布可验收离线语义（显示未接入、不误报模型不支持、连接测试正常），至少一个模型真实检测需在提供符合 T15 许可/合同门禁的 sidecar 后补验收。禁止复制 `tools/gpt56_api_detector-git` 核心/基线/报告，禁止使用 GitHub Actions。
- 生产结果：根 `main@65d70601a024e4f9b8c4c23e4756b6ae67ec8df8`、tree `61670b6394de7b58c6aeb79eb94f861875c767a4` 已推送；宿主记录 `/var/lib/sub2api/release-records/20260818T054645Z-production-2885531.json` 为 `succeeded/promoted`、`rolled_back=false`、`downtime_required=false`，活动槽 `green`，API/worker 同一不可变镜像 `...ffc6c25deac5b2327e1f769c256046140c7d7d01c4ab615d077c548c604b2369`。公网三项健康均 200；登录态 admin API 显示 90 个账号，API Key 账号 `detector_state=unconfigured`、状态 `service_unconfigured`、原因 `detector_unconfigured`，连接测试模型仍存在；未配置 sidecar，T22 继续等待 T21 完整验收。
- 最终验收：宿主记录 `/var/lib/sub2api/release-records/20260818T110720Z-production-3118657.json` 为 `succeeded/promoted`，活动槽 `blue`；detector/API/worker 同镜像且 healthy。catalog 在线返回 `gpt-5.6-terra`、`gpt-5.6-sol`、`gpt-5.4`；账号 `#23` 真实检测运行 `9dc7b02a-f5f2-4ea1-a25f-2f2f6ab2c6dd` 进入 `normal`，模型 `gpt-5.6-terra`、detector `native-1`。最近日志精确扫描未出现 API Key 值、base URL、Authorization/Bearer 或 detector token。

### T22 官方 Channel Monitor V2 简洁运营视图

- 当前状态：`DONE`。候选已在根 `main@cbfe9ab7d10b071373afecb9b427a103f1df72cc` 合入并推送；宿主预加载蓝绿链返回 `succeeded/promoted`、`rolled_back=false`、`downtime_required=false`，活动槽 `green`。发布证据：`/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-18-main-cbfe9ab7d-t22-channel-monitor-v1.json`；线上专项验收由登录态页面完成，默认 24h、90m/7d/30d 四窗口、详细分析按需加载、自然零流量状态和真实 critical/warning 评分均确认，公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200，1440/390 视口无整页横向溢出。截图证据目录：`/Users/gongtengxinwen/.codex/visualizations/2026/08/18/01a0149d-b946-78e1-b0f3-200fed647f00/`。
- 范围：默认时间窗改为 24h，保留 90m/7d/30d；首屏保留分组状态、成功率、首 Token、缓存率和最近趋势，模型明细/错误分类/用户排行移入“详细分析”。低流量/样本不足显示“已就绪·暂无流量”或“待观察”，不计入整体异常和健康评分且不伪造健康；真实错误、低成功率、高延迟仍黄/红显示。
- 口径与验收：复用 T19 有效样本分母，排除本地拒绝、禁用模型、参数校验失败等未获上游响应请求；确认不重复实现已完成能力。桌面/移动端无溢出，v1 可回滚，预期无迁移且 `downtime_required=false`。

### T23 自购账号独立采购成本与人民币利润模型

- 当前状态：`DONE`。根 `main@d295e73050750c58edd040b6c6d517aad31358db`、tree `33b0053bc3dd3b743d74e7f64e71f59bb9cfe12f` 已推送 `origin/main`，包含 T23 合并提交 `95ef3c713` 与 migration 226 宿主 allowlist 提交 `d295e7305`。发布证据为 `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-18-main-d295e7305-t23-procurement-v2.json`（0600）；宿主记录 `/var/lib/sub2api/release-records/20260818T140601Z-production-3259304.json` 返回 `succeeded/promoted`、`rolled_back=false`、活动槽 `blue`，`release-state` 绑定 source/tree 与迁移哈希 `18c4ac1fc83294634c42c6d08c6511c01515406f296d40b54840f3dae726949f`。首次缺少网络探针 allowlist 的发布尝试在宿主变更前 fail-closed，补齐精确 allowlist 后同一车道重试成功。
- 依赖与窗口：T21/T22/T23 均已完成 `DONE`；T16 等冻结/保护 worktree 未解冻，未启动后续任务。
- 目标：为明确归属的自购账号建立独立、可审计的人民币采购成本与利润模型，不与渠道账号 USD 上游成本混加，不修改用户扣费规则、原始 `usage_logs` 成本事实或渠道经营口径。采购成本、预计可用标准 Token 额度、版本化台账、历史生效、剩余成本/额度、采购损失、失效结算、审计、幂等与并发保护均按已批准业务规则实施。
- 核心公式：采购成本倍率 = 采购成本 CNY / 预计可用额度 USD；已确认采购成本基于倍率前的标准 Token 消耗额度并以采购周期真实采购价封顶；人民币营收按已实际消耗的站内 USD 额度 1:1 计入；净利润 = CNY 营收 - 已确认采购成本 - 采购损失，内部运营消耗单列进入总成本。未录入成本显示“成本待录入”，不按零成本计算。
- 数据边界：保留 `accounts.procurement_cost_cny`、`estimated_usable_quota_usd`、`procurement_cost_effective_at` 作为当前投影；新增 expand-only、版本化、可审计采购成本台账，记录账号、成本、额度、生效/结束/结算、损失、状态、操作者和时间戳。不得迁移删除、历史回填、生产数据修改或重算账务。
- 页面与验收：经营页新增独立“自购账号”人民币视图，展示采购成本、预计额度、标准额度消耗、利用率、已确认成本、待摊成本、采购损失、人民币营收、净利润、利润率和成本状态；提供明确的“确认失效并结算”二次确认；桌面及 390px 移动端无横向溢出。覆盖首次录入历史生效、后续版本生效、额度变更剩余摊销、失效结算、超额封顶、未配置和渠道 USD 汇总隔离。
- 发布属性：宿主最终记录为 `downtime_required=false`；迁移 226 按用户已给出的停机授权走受控维护切换，PostgreSQL/Redis/Caddy 身份保持不变。不得使用 GitHub Actions。
- 线上专项验收：登录态经营页确认独立“自购账号 · 人民币”视图与渠道 USD 汇总分离，成本待录入、采购成本/预计额度、利用率、确认/待摊/损失/营收/净利润字段均按规则展示；账号监控真实入口显示“采购成本（CNY）”“预计可用额度（USD）”及派生倍率。公网 `/healthz`、`/readyz`、`/health` 均 HTTP 200。宿主数据库确认 `226_account_procurement_cost_versions.sql` 已应用，版本台账当前 0 行，未发生历史回填；成本封顶、失效结算采购损失、幂等与 actor 审计由 T23 直接相关测试覆盖，本次未为制造样本修改生产数据。

### T24 特惠分组本地调度耗尽 503 纠正

- 当前状态：`DONE`。独立候选 `codex/t24-local-scheduling-exhaustion@6871ddaac` 经根审查合入，最终发布源为 `main@a76dff256d53b7e3b9f0d3df8aa8d1699edcd39b`。根合并后直接门禁、推送、无停机蓝绿发布和线上验收均完成。线上历史真实本地 503 样本 `19877` 已投影为 `LOCAL_CAPACITY_EXHAUSTED / routing / platform / 未选择账号`，上游样本 `20190` 仍归属 `upstream_failed / provider / 已选择账号`；公网健康均 200，未制造新失败流量或修改生产数据。
- 依赖与窗口：T23/T24 均已完成 `DONE`；当前没有功能任务占用整合/发布/验收车道；T16 等冻结/保护 worktree 不解冻。
- 验收边界：覆盖特惠分组仅允许自购账号且全部不可调度时的 `/responses`、Chat Completions 与流式/非流式本地拒绝；保留上游真实 503 透传；用户侧中文泛化提示与管理员诊断阶段/归属可区分；无迁移、无生产数据修改，预期 `downtime_required=false`。
