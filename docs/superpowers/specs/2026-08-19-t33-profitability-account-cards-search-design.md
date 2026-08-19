# T33 经营页账号卡片与搜索设计

## 问题证据与当前行为

`upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue` 已复用原生 `accountFinancial` USD 报表、`selfPurchasedProfitability` CNY 报表、范围/分组/排序、加载/错误/刷新状态和 `AccountMonitorCostDialog` 采购入口，但两个视图仍分别渲染一张宽表。桌面账号信息不易逐卡扫描，移动端需要组件内横向滚动；没有按账号元数据的本地搜索。

原生接口已经返回本任务所需事实：USD 账号的 `id/name/platform/type/historical/amounts`，CNY 自购行的 `account_id/name/platform/account_type/status/cost_status` 与全部采购派生字段。无需增加 API、数据库字段、汇率服务或账务事实源。

## 目标

1. USD 与 CNY 视图均以独立账号卡片展示，每行桌面最多两列，390px/窄屏单列。
2. 卡片指标使用固定 CSS 网格，使标签和值在同一视图内对齐；长账号名可换行且不撑宽页面。
3. USD 卡片保留内部运营消耗、业务消耗、业务营收、总消耗、净利润、利润率/原生结果字段；CNY 卡片保留采购成本、预计额度、标准消耗、利用率、确认成本、待摊、采购损失、营收、净利润、利润率、成本状态和操作入口。
4. 两个视图共享一个本地关键字输入，按账号名、ID、平台、类型及可用的状态/成本状态大小写不敏感匹配；搜索不触发网络请求。
5. 保留时间范围、USD 分组范围、排序以及各自 loading/error/refresh/retry 状态；搜索结果为空时仍显示明确空态。
6. 经营展示继续遵循既有口径：USD 原生成本/营收字段保持 USD；CNY 采购经营字段使用 CNY，预计额度/标准消耗继续明确标 USD，底层 1 USD = 1 CNY 的产品关系不新增汇率字段或服务。

## 非目标

- 不修改后端服务、SQL、账务公式、采购保存/结算 API、调度、账号数据或生产数据。
- 不新建平行经营 API、汇率服务、第二账务源或新的采购表单。
- 不改变摘要公式、分组统计、排序语义或刷新时序。

## 方案比较

### 方案 A（采用）：组件内卡片渲染 + 计算属性过滤

在现有单文件组件中将两个表格替换为共享卡片模板，分别提供 USD/CNY metric 定义；增加 `searchQuery` 和 `filtered*Accounts` 计算属性。优点是改动边界最小、继续复用当前 API/弹窗/状态机，测试可以直接覆盖 DOM 与计算行为。缺点是组件模板变长，但没有引入新的抽象层。

### 方案 B：新建 `AccountProfitabilityCard` 子组件

把卡片抽成子组件并通过 discriminated props 传 USD/CNY 数据。可复用性较好，但需要额外 props/类型和测试边界，当前只有一个消费者，增加了交互回归面。

### 方案 C：继续保留表格并仅在移动端包装卡片

能减少桌面改动，但不满足“每个账号一张独立卡片”和固定字段网格的合同，排除。

选择方案 A，原因是它最大化复用原生页面生命周期和现有直接测试，同时把变更限定在一个用户可见垂直功能。

## 端到端数据与控制流

1. 页面 mounted/刷新/范围切换仍只调用当前视图对应的既有 API。
2. USD 响应进入 `report`，按 `activeScope` 得到账号集合；CNY 响应进入 `selfPurchased`，使用 rows。
3. `searchQuery.trim().toLocaleLowerCase()` 与每条账号的 name/id/platform/type/status/cost_status 拼接值比较；空查询保留全部账号。
4. 搜索后的账号集合再按现有排序（USD）或稳定原始顺序（CNY）渲染卡片。
5. 操作按钮继续调用共享采购弹窗、保存/清空/结算方法；成功后只刷新既有 CNY API。

## 字段契约与展示

USD 卡片：账号名、平台/类型/ID、状态（`historical` 映射为 `historical`，否则 `active`）、内部运营消耗、业务消耗、业务营收、总消耗、净利润、利润率。金额使用既有 `usd()`，利润/利润率继续沿用 tone 规则。

CNY 卡片：账号名、平台/类型/ID、运行状态、成本状态、采购成本（CNY）、预计额度（USD）、标准消耗（USD）、利用率、确认成本（CNY）、待摊（CNY）、采购损失（CNY）、营收（CNY）、净利润（CNY）、利润率；预计额度与标准消耗明确保留底层 USD 单位，其余经营金额按既有 1 USD = 1 CNY 产品关系显示 CNY，不引入汇率换算。缺失数值沿用 `成本待录入`/`—`。保留“录入成本/编辑成本”和符合既有条件的“确认失效”入口。

固定网格使用 `grid-cols-2`，每个 metric 使用 `min-w-0` 与 `break-words`；卡片外层 `grid-cols-1 gap-4 lg:grid-cols-2`，主容器继续 `overflow-x-hidden`。

## 失败与兼容语义

- API loading、refreshing、error/retry 和 stale-response sequence 逻辑不变。
- 搜索只作用于当前已加载数据；网络失败时不伪造卡片。
- USD 原始响应缺状态时使用 `historical ? 'historical' : 'active'`，避免后端接口扩展。
- CNY `net_profit_cny`/`margin` 为 null 时显示 `—`，不把零或待摊成本替换为采购损失。

## 场景化验收矩阵

| 场景 | 验收 |
|---|---|
| USD 默认加载 | 摘要、范围、分组和每账号独立卡片出现，保留 5 个原生经营字段和利润率 |
| CNY 切换 | 自购摘要与每个 OAuth 行转换为独立卡片，11 个成本/经营字段和操作入口存在 |
| 搜索 | 输入名称、ID、平台、类型、active/historical、CNY status/cost_status 只留下匹配卡片；无匹配显示空态；不请求 API，清空恢复 |
| 范围/排序/刷新 | 既有范围、分组、排序、loading/error/retry/refresh 仍可操作 |
| 响应式 | 桌面最多两列；390px 与窄屏一列、主页面 `scrollWidth === clientWidth`；长名换行 |
| 采购操作 | 共享弹窗、保存/清空、失效结算入口与既有 API 参数不变 |

## 测试策略

- 先在 `AccountProfitabilityView.spec.ts` 增加搜索/卡片/响应式断言并观察失败，再实现；覆盖长账号名、成本待录入、零流水和无匹配空态。
- 保留并改写原有字段、层级、排序、状态测试的选择器以对应卡片 DOM。
- 运行直接 Vitest 文件；随后 `pnpm typecheck`、`pnpm build` 和 `git diff --check`。
- 不执行后端/迁移/生产写入；无迁移和配置变化，预期 `downtime_required=false`（最终发布预检由根总控决定）。

## 发布与回滚

候选只在本 worktree 保持 `READY_FOR_ROOT_REVIEW`，不合并、推送、部署。根总控合并后按既有本地/宿主蓝绿链预检；回滚为恢复上一已验证 main 提交/镜像。剩余风险是后端未提供 USD 实时状态，只能以 `historical` 推导本地状态；真实生产视觉验收留给根总控。

## 用户批准记录

2026-08-19：用户在 T33 独立用户可见工作线程中明确批准 USD/CNY 账号卡片、CNY/USD 口径、原生字段保留、本地搜索、状态保留、无后端/生产变更及 390px/长名验收合同；该合同作为本规格书的批准输入。
