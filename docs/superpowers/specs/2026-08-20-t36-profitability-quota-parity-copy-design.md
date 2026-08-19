# T36 经营页 CNY/USD 额度关系文案设计

## 问题证据与当前行为

`upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue` 已通过路由懒加载，提供“经营结果 · USD”和“自购专题 · CNY”两个视图，并复用同一页面的 USD 原生经营报表与 CNY 自购报表。页面目前只在 CNY 面板写有“独立于渠道 USD 汇总；按标准额度消耗确认采购成本”，没有在 USD/CNY 切换附近明确说明本站两种额度的数值关系，也没有明确声明该关系不是汇率换算。

生产页面因此不易让运营人员在切换币种时直观看懂：本站的 `1 USD` 额度与 `1 CNY` 额度按数值 1:1 理解。该缺口是展示合同缺失，不是账务公式或数据错误；当前 route 仍为 `() => import('@/views/admin/AccountProfitabilityView.vue')`，无需改变懒加载边界。

## 目标

1. 在 USD/CNY segmented view 附近提供一条始终可见的、短的额度关系说明。
2. 中文和英文均由现有 `admin.accountProfitability` i18n 命名空间提供，跟随页面当前语言切换。
3. 固定中文文案为：`额度口径：1 USD 额度 = 1 CNY 额度（仅用于额度关系理解，不是汇率换算）`。
4. 固定英文文案为：`Quota basis: 1 USD quota = 1 CNY quota (for understanding the quota relationship only; not an exchange-rate conversion)`。
5. 以稳定 `data-test="quota-parity-note"` 暴露组件合同；直接测试同时验证源码引用、zh/en locale 值和挂载后的可见文本。

## 非目标与边界

- 不修改任何账务公式、金额、API、SQL、查询、采购保存/结算、账号数据、数据库、迁移、配置或生产状态。
- 不增加汇率服务、汇率字段、换算逻辑、第二账务源或新的页面/组件。
- 不改变现有 USD/CNY 切换、按需请求、搜索、卡片、刷新、错误或采购操作行为。
- 不改变路由懒加载或手工分包策略；locale 只复用现有全局动态语言包。
- 不要求浏览器登录态验收作为候选阻塞项；根总控负责合并后的发布链验证。

## 方案比较与选择

### 方案 A（采用）：切换控件下的 i18n 说明

在 segmented view 之后插入一个 `<p data-test="quota-parity-note">`，内容来自 `t('admin.accountProfitability.quotaParityNote')`；在 `src/i18n/locales/zh/admin/index.ts` 与 `src/i18n/locales/en/admin/index.ts` 添加同名键。优点是用户在切换前后都能看到，改动集中且保留现有懒加载页面结构；直接测试可覆盖语言值与 DOM。缺点是文案在两个视图共享，但这是本次合同所需的共同口径。

### 方案 B：分别放入 USD 与 CNY 面板标题

在两个面板各放一条文案。虽然每个面板上下文更近，但会重复渲染和维护相同文案，切换加载状态时可能短暂不可见，增加文案漂移风险。

### 方案 C：把关系放入页面 description 或 tooltip

修改页头描述或仅通过 tooltip 提示。页头描述不靠近币种控件，tooltip 也不满足“直观看懂”的默认可见合同，排除。

采用方案 A，因为它在现有页面层级中最小、默认可见、双语一致且不触碰数据流。

## 端到端控制流

1. 路由仍懒加载 `AccountProfitabilityView.vue`。
2. 页面使用既有 `useI18n()`；语言包由现有 `loadLocaleMessages()` 动态加载。
3. segmented view 渲染后紧邻渲染 `quota-parity-note`，调用 `t('admin.accountProfitability.quotaParityNote')`。
4. 切换 USD/CNY、loading、刷新、搜索和 API 请求都不影响该文案的可见性。
5. 仅 locale 文本随语言切换变化；不进行任何数值计算或 API 调用。

## 接口与字段契约

- 新增 i18n 字段：`admin.accountProfitability.quotaParityNote: string`。
- zh 值：`额度口径：1 USD 额度 = 1 CNY 额度（仅用于额度关系理解，不是汇率换算）`。
- en 值：`Quota basis: 1 USD quota = 1 CNY quota (for understanding the quota relationship only; not an exchange-rate conversion)`。
- 页面节点：`p[data-test="quota-parity-note"]`，文本严格等于当前语言对应值。
- 不新增 API 请求、响应字段、数据库列或持久化状态。

## 失败与兼容语义

- 语言包编译失败由既有 locale compile 测试捕获；不存在 fallback 到硬编码金额关系的逻辑。
- 该文案是静态说明，不能因 API 失败、空数据或切换失败而伪造数据状态。
- 若页面运行在英文环境，必须显示英文值；中文环境必须显示中文值；locale fallback 继续沿用现有 en 规则。
- 既有页面测试中对 `t()` 的最小 mock 增加该键，不改变其它测试语义。

## 场景化验收矩阵

| 场景 | 验收 |
|---|---|
| USD 默认视图 | 切换控件旁存在 `quota-parity-note`，显示完整中文/英文当前语言文案 |
| CNY 视图 | 切换后同一节点仍存在，文本不依赖 CNY API 成功或 rows 数量 |
| 语言包 | zh/en 均存在唯一同名键，locale compile 通过 |
| 生产懒加载入口 | route 仍使用 `AccountProfitabilityView.vue` 动态 import；未新增平行入口或外部包 |
| 账务边界 | 直接 diff 只涉及页面、zh/en locale、页面直接测试和 T36 文档；无 API/SQL/迁移/配置变更 |

## 测试策略

- 先在 `AccountProfitabilityView.spec.ts` 增加一个失败的组件合同测试：断言 `pageSource` 引用 i18n key，zh/en locale 值精确匹配，挂载后节点可见；在 `messages` mock 中先不加入新 key，确认 RED 原因是缺失文案。
- 实现后运行该文件的 Vitest；再运行 locale compile 与页面直接相关测试（如需要）。
- 执行 `pnpm typecheck`、`pnpm build`、`git diff --check`；构建只验证现有懒加载入口和产物可生成，不改变手工分包策略。
- 不执行后端测试、迁移测试、生产写入或线上操作。

## 发布、回滚与停机条件

- 候选仅停在本 worktree 的 `READY_FOR_ROOT_REVIEW`，不得自行合并、推送或部署。
- 预期无迁移、无配置变化，`downtime_required=false`；最终结果以根总控合并后发布预检为准。
- 回滚为恢复上一已验证 `main` 提交/镜像；该回滚不涉及数据恢复。
- 根总控合并后按既有本地/宿主蓝绿发布链完成线上健康验证；真机视觉验收按当前全局指令不阻塞收口。

## 仍待决事项与批准记录

- 产品方向已由根总控代审授权固定；无需新增用户问题。
- 规格批准记录待根总控回复 `APPROVE_SPEC_T36` 后补入本文件。
