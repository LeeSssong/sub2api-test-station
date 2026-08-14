# T05 用量页仅使用原生 Sub 数据设计

## 问题证据与当前行为

管理员用量页 `upstream/sub2api/frontend/src/views/admin/UsageView.vue` 当前同时读取原生 Sub 数据与外部控制面会计投影。页面初载、刷新、筛选或统计重载时，先通过原生接口读取用量列表、统计、图表、错误请求、详情和异常核对数据；在统计加载成功后，页面继续调用外部控制面的 accounting 页面决策接口，并在决策允许时请求外部会计 ledger。

现有页面因此具有以下可观察行为：

- `loadStats()` 成功读取原生 `/api/v1/admin/usage/stats` 后会继续执行 `loadControlPlaneLedger()`。
- `loadControlPlaneLedger()` 会调用 `/api/v1/xingqiao/externalization/pages/accounting`，并可能继续调用 `/api/v1/xingqiao/accounting/ledger`。
- 外部 ledger 被认为可信时，可以覆盖 `usageStats.total_requests`、`usageStats.total_cost` 和 `usageStats.total_actual_cost`。
- 页面顶部会展示 `ReadModelStatus`，包括“控制面暂时不可用”“完整性：unknown”等外部控制面状态。
- 外部控制面失败、超时、401/403 或字段不完整时，原生用量内容仍可显示，但用户会看到控制面降级或完整性状态。

组件测试 `upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts` 当前显式 mock `controlPlaneAPI.decision` 和 `controlPlaneAPI.ledger`，并覆盖 shadow、external-primary、控制面失败降级和完整性状态。这证明外部控制面行为是页面当前契约的一部分，而不是未使用代码。

生产只读检查也确认：当前管理员用量页会显示“控制面暂时不可用”和“完整性：unknown”，初载会请求 accounting decision 且该请求可能返回 401；同时原生用量列表、统计、错误请求、管理员详情和成本异常核对仍能通过原生/本地接口渲染。

## 目标

- 管理员用量页在初载、刷新、筛选、分页、统计重载、错误详情和用量详情路径中，对 `/api/v1/xingqiao/**` 以及其他外部控制面 API 实现零请求。
- `/api/v1/admin/usage` 和 `/api/v1/admin/usage/stats` 始终是用量列表与统计卡片的原生数据源。
- 删除用量页上的外部控制面状态条、完整性信息、外部 ledger 决策、外部 ledger 请求和外部覆盖统计逻辑。
- 不新增“原生数据源”“完整性”或其他替代常驻状态条；正常状态只显示既有用量页内容。
- 保持用量列表、统计卡片、图表、筛选、分页、导出、错误请求、错误详情、管理员用量详情和 T03-R1 成本异常核对入口语义不变。
- 保持 T03-R1 已上线的财务/异常入口语义：管理员仍可从用量页进入异常核对和用量详情，且详情内本地成本证据与上游诊断路径不因本任务改变。

## 非目标

- 不修改成本数值规则、成本/利润公式、利润页、账号监控页、后端账务模型、调度、external-primary、relay-ops 主路径或 GitHub Actions。
- 不修改后端接口、数据库、迁移、权限、路由、运行配置或生产数据。
- 不清理共享 `controlPlane.ts`、`ReadModelStatus.vue`、`useReadModelFreshness.ts`、外部化决策配置或共享测试；这些共享设施保留给 T06 等后续串行任务。
- 不改变 `UsageTable.vue`、`UsageDetailDialog.vue`、错误请求详情组件、成本异常表格或它们的 API 契约，除非测试改写需要更新本页组件 mock。
- 不修改 `docs/project/project-progress.md` 或 `docs/project/native-sub-task-package-queue.md`；功能任务只维护自己的规格、计划、实现、测试、复审报告和交接材料。
- 不执行合并 `main`、推送、部署、生产写操作或线上状态修改；本任务最终只报告 `READY_FOR_ROOT_REVIEW`。

## 影响用户与边界条件

直接影响对象是使用原生 Sub2API 管理后台用量页的管理员。正常情况下，管理员继续看到相同的用量统计、列表、图表、错误请求和详情交互；用户可见变化仅为不再出现外部控制面状态条和完整性状态。

需要保持的边界条件包括：

- 首次进入用量页时统计、列表、图表和详情 tab 懒加载按现有路径工作。
- 用户刷新、修改日期范围、修改筛选条件、分页、排序或切换 tab 时不触发外部控制面请求。
- 原生统计请求失败时，页面保留现有错误提示语义，不回退到外部 ledger，也不显示外部控制面降级。
- 原生列表请求失败时，`UsageTable` 的加载和空态/错误语义保持现有行为。
- 错误请求 tab 继续使用 `/api/v1/admin/ops/errors`；错误详情继续使用本地 ops 详情和上游错误证据接口。
- 用量详情弹窗继续使用 `/api/v1/admin/usage/:id` 和 `/api/v1/admin/usage/:id/upstream-cost`。
- 成本异常 tab 继续使用 `/api/v1/admin/usage/cost-exceptions` 及本地 review 接口。
- 外部控制面成功、失败、超时、401/403 或响应字段变化均不影响本页面，因为本页面不再发出相关请求。

## 方案比较与选择

### 方案 1：删除 UsageView 页面级外部依赖与覆盖逻辑（采用）

从 `UsageView.vue` 删除 `ReadModelStatus` 状态条、控制面导入、`useReadModelFreshness`、`resolveTrustedPageDecision`、`controlPlaneResponse`、`controlPlaneDegraded`、`renderSource`、`loadControlPlaneLedger()` 和 `accountingLedger()`。`loadStats()` 收敛为只提交原生 stats 响应，不再追加 accounting decision/ledger 调用。同步改写 `UsageView.spec.ts`，证明本页不再 mock 或调用控制面 API，且页面文本不包含控制面/完整性状态。

优点是数据源单一、零请求契约直接、变更范围最小且与 T04 页面级边界一致。代价是用量页不再保留 accounting external-primary 覆盖统计的页面能力；这是本任务的明确目标。

### 方案 2：保留代码但用固定页面开关短路外部逻辑（不采用）

在 `UsageView.vue` 保留控制面导入和状态，只通过固定条件阻止请求。短期 diff 较小，但留下不可达代码、状态和测试歧义，未来配置漂移或分支改动可能重新产生外部请求，不适合作为验收需要的零 `/xingqiao/**` 契约。

### 方案 3：修改共享控制面客户端或外部化配置以短路 accounting（不采用）

在 `controlPlane.ts` 或外部化配置层让 accounting 页面不访问网络。该方式会把用量页职责隐藏到共享层，并影响 T06 等后续页面的清理边界，扩大跨任务耦合，不符合用户已批准的 T05 页面级范围。

选择方案 1，因为它以最小、显式且可测试的页面级删除实现用户已经确认的边界：只从 `UsageView.vue` 删除状态条、accounting decision/ledger 调用及覆盖逻辑，并仅改写对应测试。

## 架构与范围边界

### 从用量页删除

- `controlPlaneAPI` 与 `ControlPlaneResponse` 导入。
- `ReadModelStatus` 组件导入及模板中的控制面状态条。
- `useReadModelFreshness` 与 `resolveTrustedPageDecision` 导入。
- `controlPlaneResponse`、`controlPlaneDegraded`、`renderSource` 和 `readModel` 页面状态。
- `loadStats()` 中对 `loadControlPlaneLedger()` 的后续调用。
- `loadControlPlaneLedger()` 的 decision、ledger 请求、degraded 状态处理和 external-primary 覆盖分支。
- `accountingLedger()` 及仅服务于外部 ledger 解析的帮助逻辑。
- `UsageView.spec.ts` 中针对控制面 mock、shadow mode、external-primary 覆盖、ledger freshness、控制面失败降级和完整性状态的测试契约。

### 在用量页保留

- 用量列表：`/api/v1/admin/usage`。
- 用量统计：`/api/v1/admin/usage/stats`。
- 图表和分布统计相关原生 admin usage/model stats 路径。
- 筛选、日期范围、granularity、分页、排序、列显示、导出、清理弹窗和用户余额历史入口。
- 错误请求 tab、错误详情弹窗和上游错误证据路径。
- 管理员用量详情弹窗、上游成本证据、T03-R1 财务/异常核对入口和本地 review 语义。
- `UsageTable.vue`、`UsageDetailDialog.vue`、`CostExceptionTable.vue`、`OpsErrorLogTable.vue`、`OpsErrorDetailModal.vue` 的既有契约。

### 明确保留的共享边界

以下共享文件及其共享测试保持原样，不在 T05 做跨页面清理：

- `upstream/sub2api/frontend/src/api/controlPlane.ts`
- `upstream/sub2api/frontend/src/components/admin/ReadModelStatus.vue`
- `upstream/sub2api/frontend/src/composables/useReadModelFreshness.ts`
- `upstream/sub2api/frontend/src/config/externalizationFlags.ts`
- `controlPlaneApi.spec.ts`、共享 API client 测试和外部化配置测试

## 端到端数据与控制流

### 统计读取流程

1. 页面挂载、日期范围变化或用户刷新时，`loadStats()` 按现有方式设置统计加载状态。
2. 页面只调用原生 `adminUsageAPI.getStats()`，对应 `/api/v1/admin/usage/stats`。
3. 请求成功后直接提交 `usageStats`。
4. 请求失败时保持现有错误处理与用户提示语义。
5. `finally` 清理本轮统计加载状态。

此流程没有外部 decision、外部 ledger 获取、外部完整性解析、外部数据源标签或统计覆盖阶段。

### 列表、筛选与刷新流程

用量列表继续通过现有 `loadUsageData()` 或同等加载函数读取 `/api/v1/admin/usage`。筛选、分页、排序、日期范围和刷新只影响原生请求参数，不触发 accounting decision/ledger，也不触发任何 `/api/v1/xingqiao/**`。

### 错误请求与详情流程

错误请求 tab 继续懒加载本地 ops 错误列表。打开错误详情时继续读取本地 request-error 详情和上游错误证据。打开普通用量详情时继续读取管理员 usage detail 和 upstream-cost。T05 不改变这些路径的参数、弹窗状态、诊断展示或错误处理。

### 成本异常核对流程

成本异常 tab 继续由 `CostExceptionTable` 管理本地异常列表与 review 操作。用户从异常行进入详情时，仍复用管理员用量详情弹窗。T05 不改变异常筛选、review 后刷新或 T03-R1 异常入口语义。

## 接口与字段契约

- `/api/v1/admin/usage` 请求参数、分页、排序和响应字段保持不变。
- `/api/v1/admin/usage/stats` 响应字段保持不变，页面继续使用原生 `total_requests`、`total_cost`、`total_actual_cost` 等统计值。
- 页面不再消费外部 accounting decision、ledger items、freshness、completeness、degraded、calculation version 或 source 字段。
- 不新增 API、DTO、特性开关、环境变量或配置项。
- 不修改成本数值规则；如果原生 stats 与旧外部 ledger 曾存在差异，T05 以后用量页以原生 stats 为唯一页面统计来源。

## 失败与安全语义

- 原生统计成功：显示原生统计卡片，不显示任何控制面或完整性状态。
- 原生统计失败：按现有本地错误语义提示，不请求外部 ledger，不显示控制面降级。
- 原生列表成功或失败：保持 `UsageTable` 现有加载、空态、错误和分页语义。
- 日期范围或筛选快速变化：继续依赖现有列表/图表加载机制；本任务不新增外部请求或新的取消模型。
- 错误详情、用量详情和异常核对失败：保持各自组件现有错误处理；不得因本任务新增外部回退。
- 外部控制面的网络、鉴权、完整性或字段状态不再具有页面失败含义，因为用量页不会请求或解析外部 accounting 数据。
- 不在前端伪造统计成功数据，不把外部响应作为容灾备份，也不改变权限或会话处理。

## 兼容性与迁移

- 这是前端页面级删除，不需要数据库迁移、数据回填、后端兼容层或配置迁移。
- 原生 API、用量表格、详情弹窗、错误请求和异常核对组件契约不变，因此无需双读、灰度字段或消费者迁移。
- 共享控制面设施不删除，T06 和其他后续任务可以继续独立处理其页面边界。
- 已保存的浏览器状态不需要迁移；页面重新加载后自然使用原生单读流程。
- 回滚时可以反向恢复本任务的页面和测试提交，不涉及数据恢复。

## 场景化验收矩阵

| 场景 | 页面数据源与请求 | 用户可见结果 |
| --- | --- | --- |
| 首次加载成功 | 仅原生 usage/stats/图表接口；`/api/v1/xingqiao/**` 为 0 | 显示原有统计、图表和列表，无控制面或完整性状态 |
| 用户刷新 | 仅重新请求原生用量与统计 | 页面刷新结果正常，无外部状态条 |
| 日期范围变化 | 新日期范围只影响原生 stats/list/chart 参数 | 统计、图表、列表按现有语义更新 |
| 筛选、分页、排序 | 仅请求原生 list 或相关图表统计 | 表格行为不变，无外部请求 |
| 原生 stats 失败 | 不请求外部 ledger | 保持现有错误提示，不显示控制面降级 |
| 错误请求 tab | 使用 `/api/v1/admin/ops/errors` | 错误列表正常显示和分页 |
| 错误详情 | 使用本地 request-error 详情与 upstream-errors | 管理员诊断、阶段、归属和上游证据保持正常 |
| 用量详情 | 使用 `/api/v1/admin/usage/:id` 与 upstream-cost | 详情弹窗和本地成本证据保持正常 |
| 成本异常 tab | 使用本地 cost-exceptions 与 review 接口 | T03-R1 异常核对入口语义保持正常 |
| 外部控制面 401/403/超时/字段变化 | 页面不发请求 | 页面不受影响，也不显示完整性 unknown |

## 测试策略（最小 MVP）

仅调整用量页相关测试，并保留与本次范围直接相关的既有回归。最小验证覆盖：

1. `UsageView.spec.ts` 不再 mock 或断言 `controlPlaneAPI.decision` / `controlPlaneAPI.ledger`。
2. 首次加载、刷新或日期范围变化只依赖原生 stats/list 路径，页面文本不存在“控制面暂时不可用”“完整性”或其他替代常驻状态。
3. 原生用量列表、统计卡片、筛选、分页和表格基本交互保持既有测试覆盖。
4. 错误请求 tab、错误详情、用量详情和成本异常入口的既有专项测试继续通过。
5. T03-R1 成本异常和管理员详情相关测试继续通过，证明本任务没有破坏财务/异常入口语义。

实施验证至少运行：

- `UsageView.spec.ts` 专项组件测试。
- 用量页相邻组件测试：`UsageTable.spec.ts`、`UsageDetailDialog.spec.ts`、`usageDetail.spec.ts`、`CostExceptionTable.spec.ts`、`OpsErrorLogTable.spec.ts`、`OpsErrorDetailModal.spec.ts`、`errorDetailResponse.spec.ts`、`admin.usage.spec.ts`。
- `git diff --check` 和变更范围检查。

是否执行更宽的前端构建、类型检查或全量测试，由后续实施计划在读取 package scripts 后写出准确命令；若专项测试暴露共享风险，实施计划应扩大验证范围。

## 发布、线上验证与回滚条件

### 发布属性

- 预计为纯前端页面改动。
- 无数据库迁移、配置变化或停机要求，`downtime_required=false`；最终以合并后发布预检为准。
- 不使用 GitHub Actions，继续使用项目既有、已审查的本地/宿主发布链。
- 本功能任务不自行合并 `main`、推送、部署或修改生产。

### 门禁与发布顺序

1. T05 独立候选完成规格、获批计划、实现、逐任务复审和整分支终审后，报告 `READY_FOR_ROOT_REVIEW`。
2. 未收到根发布总控包含目标 `main` SHA 的 `AUTHORIZE_MERGE_TO_MAIN` 前，本任务不得合并、推送或部署。
3. 根发布总控负责授权合并、合并后回归、发布链预检、推送、部署和线上验收。
4. 只有推送、部署和线上验证全部成功，根发布总控才能按项目总账规则标记完成并清理候选。

### 线上最小 MVP 验证

由根发布总控在授权发布后执行：

- 使用登录态打开管理员用量页，确认原有统计卡片、图表和用量列表正常显示。
- 初载、刷新、日期范围切换、筛选和错误详情路径中，浏览器网络记录确认 `/api/v1/xingqiao/**` 为零请求。
- 页面不存在“控制面暂时不可用”“完整性 unknown”或其他外部控制面状态。
- 错误请求列表、错误详情、管理员用量详情和成本异常入口保持正常。
- 公网及发布链要求的基础健康检查通过。

### 失败和回滚条件

- 专项测试、最低构建检查、最终整分支复审、根 review、合并后检查、部署或线上 MVP 验证任一失败，均不得标记完成。
- 若失败发生在 T05 候选内，保留候选 worktree、提交和证据，在同一候选上修复并重新审查。
- 若线上出现 `/api/v1/xingqiao/**` 请求、控制面状态残留、用量/错误/详情/异常入口回归或基础健康异常，则由根发布总控回滚本任务候选提交，通过同一受控发布链重新发布并验证。
- 回滚不涉及数据库、配置或生产数据恢复；恢复目标是合并前已验证的页面版本。

## 待决事项

产品与范围决策均已确认，目前没有阻塞规格的待决事项。实施计划需在规格获批后读取前端 package scripts，并确定准确的专项测试、构建或类型检查命令。发布阶段的目标 `main` SHA、实际候选提交、发布身份和线上证据必须由根发布总控在当时填写，不能在设计阶段预设。

## 用户批准记录

- 用户批准 T05 沿用 T04 的页面级边界：只从 `UsageView.vue` 删除状态条、accounting decision/ledger 调用及覆盖逻辑，并改写对应测试。
- 用户确认共享 `controlPlane.ts`、`ReadModelStatus.vue`、外部化配置与共享测试全部保留给 T06 等后续任务。
- 用户要求批准书面规格前禁止 `writing-plans`、实现代码、派 implementer/reviewer、合并、推送或部署。
- 用户要求本任务及后续所有用户可见顶层任务使用 GPT-5.5 Sol、medium reasoning，不使用 xhigh。
