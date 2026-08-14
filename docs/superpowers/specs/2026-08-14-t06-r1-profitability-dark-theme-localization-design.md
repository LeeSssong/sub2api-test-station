# T06-R1 利润页深色主题与中文本地化修复规格书

## 1. 任务、状态与基线

- 任务包：T06-R1
- 任务名称：利润页深色主题与中文本地化修复
- 当前状态：`REVIEWING`，书面规格书与后续实施计划均已获用户批准；当前仅处理最终全分支审查反馈
- 设计基线：根 `main@651bc2fab27544a8cc131137ab351bf8f2f90f89`
- 独立 worktree：当前用户可见 T06-R1 顶层任务 worktree，启动时 `HEAD` 已核对为上述基线
- 生产事实：T06 已通过无停机蓝绿发布和健康检查，记录为 `/var/lib/sub2api/release-records/20260814T154749Z-production-3329818.json`；管理员登录态验收因利润页深色主题可读性和中文本地化问题失败
- 任务边界：本任务只维护自己的规格、计划、实现、测试、复审和交接；不得修改根 `main`、全局任务队列、项目进度总账、生产或发布证据

## 2. 问题证据与当前行为

目标页面为 `/admin/operations/account-profitability`，当前生产和基线源码证据一致：

1. 页面运行在 `html` 的 dark 主题下。
2. `AccountProfitabilityView.vue` 的六个汇总卡片使用固定 `bg-white`，账号表格外层也使用固定 `bg-white`。
3. 页面正文继承深色主题的浅色文字，生产中形成白底浅字，卡片和表格内容近乎不可读。
4. 页面范围集合为 `today | 24h | 7d | 31d`，但中文 `admin.accountProfitability.ranges` 只有 `today`、`7d`、`30d`、`month`，因此 `24h` 和 `31d` 直接显示翻译 key。
5. 表头 `Account`、`Revenue`、`Expense`、`Profit`、`Margin`、`Exceptions`、`Today override` 在 Vue 模板中硬编码英文；现有中英文 locale 已提供对应 `admin.accountProfitability.columns.*` 词条，但模板没有使用。
6. 全局 `style.css` 已提供原生管理员主题模式：`.card` 包含 light/dark 背景和边框，`.table-container` 包含 light/dark 边框，`.table` 的 `th`、`td` 和行 hover 包含 light/dark 文字、背景及分隔线。本页尚未复用这些能力。
7. T06 既有页级测试已覆盖原生报告加载、手动刷新、60 秒刷新、范围切换、今日覆盖、OAuth 日成本、异常跳转和控制面禁词，但 i18n mock 只返回 key，且未验证主题样式入口和本地化显示。

## 3. 目标

1. 让利润页汇总卡片和账号表格在深色主题下清晰可读，并保持浅色主题兼容。
2. 中文环境中将 `24h`、`31d` 分别显示为 `24 小时`、`31 天`，不再泄漏翻译 key。
3. 七列表头全部通过现有 i18n 命名空间渲染，中文环境显示为账号、收入、支出、盈利、利润率、异常、今日覆盖。
4. 用页级测试固定主题样式入口和本地化合同，同时保留 T06 的原生 API、刷新、范围切换、今日编辑及控制面禁用门禁。
5. 将候选交付至 `READY_FOR_ROOT_REVIEW`，等待根任务授权后再合并、发布和线上验收。

## 4. 非目标

- 不修改财务计算、金额格式、利润率算法、接口字段、请求参数或响应 DTO。
- 不修改后端、数据库 schema、迁移、配置、环境变量或依赖。
- 不修改账号监控、用量页、其他管理员页面、T07 或 GitHub Actions。
- 不删除、不改写全局 `xingqiao-update-ui.css`，也不修改全局 `style.css` 的主题类定义。
- 不新增外部控制面、完整性、degraded、unknown 状态或 `/api/v1/xingqiao/**` 请求。
- 不重构页面脚本、自动刷新、今日覆盖、OAuth 成本写入或异常跳转。
- 不在本任务中合并根 `main`、推送、部署、操作生产或生成发布证据。

## 5. 影响用户与边界条件

- 主要影响用户：使用管理员利润页查看账号收入、支出、盈利和异常的已登录管理员。
- 深色主题：卡片背景、边框、正文、表头、单元格和 hover 状态必须使用既有主题兼容样式，不能出现白底浅字。
- 浅色主题：继续保持白色卡片、浅色表头和深色正文的可读性。
- 中文 locale：四个实际范围均显示中文；七列表头不得出现硬编码英文或翻译 key。
- 英文 locale：补齐实际使用的 `24h`、`31d` 范围词条，七列表头继续显示既有英文翻译。
- 空账号列表、零金额、空利润率、存在异常、OAuth 与非 OAuth 账号的现有渲染和交互不变。
- `today` 仍显示编辑输入；`24h`、`7d`、`31d` 仍为只读范围。
- 原生接口失败继续沿用当前页面行为；本任务不新增错误 UI 或 fallback。

## 6. 方案比较与选择

### 方案 A：复用全局原生管理员主题类（选择）

汇总卡片使用既有 `.card`，表格外层和表格分别使用 `.table-container`、`.table`，仅保留页面自身必要的布局和 padding 类。

优点是直接复用仓库已经维护的 light/dark 背景、边框、文字、分隔线和 hover 语义，改动小且与其他原生管理员界面一致。代价是页面视觉会受既有全局组件类合同约束，但这正是本页应遵循的设计系统边界。

### 方案 B：在页面每个元素追加 `dark:*` 工具类

为卡片、表头、单元格、边框和 hover 分别添加 dark 工具类。

优点是局部控制精细；缺点是复制全局主题规则、容易遗漏单元格或交互状态，后续主题调整也会产生漂移。

### 方案 C：新增利润页专属 CSS

建立页级 class 并重复定义 light/dark 颜色。

优点是完全隔离；缺点是为一个已有全局模式的问题增加新样式层，维护成本最高，也没有证据支持修改或绕开现有设计系统。

选择方案 A。全局样式定义保持只读，本页只消费其公开 class。

## 7. 组件改动与端到端数据/控制流

### 7.1 页面结构

- 六个汇总 `article` 从固定 `rounded-xl border bg-white p-4` 改为复用 `.card` 并保留 `p-4`。
- 表格外层从固定 `overflow-x-auto rounded-xl border bg-white` 改为 `.table-container`。
- 表格从局部 `min-w-full text-sm` 改为复用 `.table`；如横向内容需要最小宽度，仅保留不与 `.table` 冲突的布局类。
- 七个 `th` 的文字改为 `t('admin.accountProfitability.columns.<key>')`，列顺序、字段映射和行结构不变。

### 7.2 本地化数据

- 中文 `ranges` 增加 `'24h': '24 小时'`、`'31d': '31 天'`。
- 英文 `ranges` 增加 `'24h': '24 hours'`、`'31d': '31 days'`，避免切换英文时泄漏 key。
- 表头复用已有 columns key：`account`、`revenue`、`expense`、`profit`、`margin`、`exceptions`、`actions`。
- 不新增命名空间，不重命名或删除现有词条；历史 `30d`、`month` 词条保持不变。

### 7.3 运行时控制流

```text
进入页面 / 手动刷新 / 60 秒刷新 / 选择范围
        |
        v
AccountProfitabilityView.load()
        |
        v
adminAPI.accountFinancial.getReport({ range })
        |
        v
GET /api/v1/admin/operations/account-financial
        |
        v
原生报告 -> 既有汇总卡片和账号表格
                 |
                 +-> 既有全局主题类决定 light/dark 显示
                 +-> i18n key 决定范围按钮和表头文案
```

今日覆盖、OAuth 日成本和异常跳转继续使用原 T06 控制流；本次样式与文案修改不参与数据请求或财务计算。

## 8. 接口与字段契约

本任务不改变任何接口或字段，只保持并回归以下既有合同：

- 原生读取：`GET /api/v1/admin/operations/account-financial?range=<today|24h|7d|31d>`。
- 范围类型：`FinancialRange = 'today' | '24h' | '7d' | '31d'`。
- 报告继续读取 `generated_at`、`range`、`summary`、`accounts`、`exception_count`、`affected_revenue`、`user_unconsumed_balance_cny`。
- 金额继续读取 `revenue`、`cost`、`profit`、`margin`、`exception_count`、`affected_revenue`。
- 今日覆盖继续使用既有 `PUT /api/v1/admin/accounts/:id/financial/today-override`。
- OAuth 日成本继续使用既有 `PUT /api/v1/admin/accounts/:id/financial/oauth-cost`。
- 利润页不得调用或新增 `/api/v1/xingqiao/**`、控制面或完整性接口。

## 9. 失败与安全语义

- 原生报告加载失败时保持现有 `try/finally` 行为；不新增外部 fallback、重试、伪造数据或 unknown 状态。
- 翻译 key 缺失或模板重新硬编码七个英文表头时，页级本地化测试必须失败。
- 页面重新出现固定 `bg-white` 卡片/表格容器，或不再消费约定主题 class 时，页级主题合同测试必须失败。
- 页面源码出现既有控制面禁词或 `/api/v1/xingqiao/**` 时，原 T06 静态守门测试继续失败。
- 不改变管理员认证、授权、会话、输入写入边界或错误脱敏，不进行生产财务写入探测。

## 10. 兼容性、迁移与配置

- 前端 API 调用、路由、类型、后端、数据库 schema 和迁移集合均不变。
- 配置、环境变量、依赖和构建链均不变。
- 中文与英文 locale 仅做向后兼容的增量词条补齐；已有 key 不删除、不改名。
- 既有全局主题样式不修改，其他页面不受本候选影响。
- 预期 `downtime_required=false`；最终值由根发布总控在授权合并后的发布预检确认。
- 无数据迁移和配置迁移；回滚不需要数据库或配置恢复。

## 11. 场景化验收矩阵

| 场景 | 验收条件 |
| --- | --- |
| 深色主题汇总卡片 | 六个卡片使用既有 `.card` 主题入口；背景、边框、标签和值清晰可读，无固定白底浅字 |
| 深色主题账号表格 | 外层使用 `.table-container`、表格使用 `.table`；表头、单元格、分隔线和 hover 清晰可读 |
| 浅色主题 | 卡片与表格保持既有浅色管理员样式，可读性不回归 |
| 中文范围 | `today`、`24h`、`7d`、`31d` 分别显示今日、24 小时、7 天、31 天，不显示翻译 key |
| 中文表头 | 七列依次显示账号、收入、支出、盈利、利润率、异常、今日覆盖 |
| 英文 locale | `24h`、`31d` 和七列表头均由英文 locale 渲染，不显示翻译 key |
| 首次加载 | 继续调用 `getReport({ range: 'today' })` 并显示六项汇总和账号行 |
| 手动与自动刷新 | 继续只调用原生 `getReport`；60 秒 timer 行为不变 |
| 范围切换 | `24h`、`7d`、`31d` 使用对应原生 range；非 today 不显示今日编辑 |
| 今日编辑与异常跳转 | 今日营收/成本、OAuth 日成本写入及异常页跳转保持既有合同 |
| 控制面边界 | 页面无控制面 banner、完整性、degraded、unknown 状态，且无 `/api/v1/xingqiao/**` 请求 |
| 原生请求失败 | 保持原页面错误/加载语义，不引入 fallback 或伪造状态 |
| 线上专项验收 | 根总控使用管理员登录态验证深色可读性、中文范围和表头、刷新与范围切换；浏览器网络中 `/api/v1/xingqiao/**` 为 0 |

## 12. 测试与验证策略

实施阶段遵循 TDD：先在 `AccountProfitabilityView.spec.ts` 写出失败断言，再修改页面和 locale 使其通过。

页级测试至少覆盖：

1. i18n mock 对目标 key 返回明确测试文案，使范围按钮和七列表头的翻译调用可被观察。
2. `24h`、`31d` 呈现中文目标文案；渲染结果不含对应翻译 key。
3. 七列表头呈现中文目标文案；页面模板不再包含七个硬编码英文表头。
4. 汇总卡片含 `.card`，表格外层含 `.table-container`，表格含 `.table`；目标容器不再含固定 `bg-white`。
5. 保留并复跑现有首次加载、手动刷新、60 秒 timer、范围切换、今日编辑、OAuth 日成本、异常跳转和控制面静态禁词测试。
6. 复跑相关 i18n 或 locale 类型/构建检查，确保中英文消息结构可被构建接受。

候选验证包括定向 Vitest、前端 typecheck、production build、变更范围检查和定向 `rg`。样式单元测试只验证页面消费正确的主题 class，不重测 Tailwind 生成的颜色；构建后由本地页面级视觉检查确认深色/浅色布局无重叠、截断或不可读问题，生产最终验收由根总控执行。

## 13. 实施文件边界

预期实现只修改：

- `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
- `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`

本规格文件、后续实施计划和任务复审/交接文件属于任务证据。若实现需要修改上述范围之外的运行时代码，必须停止并回到规格修订与用户批准，不能自行扩围。

## 14. 发布、线上验证与回滚条件

- 顶层任务完成实现、定向测试、typecheck/build、diff 自查、每任务独立复审和最终全分支终审后，只报告 `READY_FOR_ROOT_REVIEW`。
- 报告必须包含基线 SHA、候选 SHA、变更文件、测试/typecheck/build 结果、未验证项、迁移变化、配置变化、`downtime_required`、回滚方式和剩余风险。
- 未收到根任务 `AUTHORIZE_MERGE_TO_MAIN` 前，不合并根 `main`、不推送、不部署、不操作生产。
- 根总控授权合并后，须在合并后的 `main` 完成冲突检查、专项回归、构建/类型检查、迁移检查和发布预检。
- 发布预检必须明确输出 `downtime_required=true|false`；本规格预期为 `false`。若实际为 `true`，必须在任何停服、迁移、重启或切换前暂停并取得用户明确授权。
- 线上验收失败时保留候选、worktree 和失败证据，在同一任务包修复；T06/T06-R1 不得标记完成，也不得启动 T07。
- 代码回滚由根总控撤销 T06-R1 候选提交并按受审发布链重新发布上一可用代码；蓝绿发布异常时按根总控流程切回上一活动槽。因无迁移和配置变化，无数据库或配置回滚步骤。

## 15. 风险与缓解

- 风险：只改卡片背景而遗漏表格文字、边框或 hover。缓解：成套复用 `.table-container` 与 `.table`，并以深色页面级检查覆盖完整表格。
- 风险：测试使用 identity i18n mock，翻译 key 泄漏仍通过。缓解：为目标 key 建立显式映射并断言最终显示文案。
- 风险：为修复本页而改动全局 CSS，影响其他页面。缓解：全局样式保持只读，仅修改本页 class 消费方式。
- 风险：静态 class 测试过度绑定 DOM。缓解：只固定三项设计系统入口和禁止固定白底的故障合同，不锁定无关布局类。
- 风险：本地单测无法证明生产主题链。缓解：候选构建后进行本地视觉检查，并由根总控执行管理员登录态生产专项验收。

## 16. 仍待决事项

- 无产品或实现待决事项；当前候选仍须完成最终全分支审查反馈、根任务审查、授权合并、发布和线上验收。
- 本任务未取得根任务合并授权、生产发布授权或线上验收批准。
- 若用户要求修改规格，先修订并重新执行规格自审，再请求书面批准。

## 17. 设计批准记录

- 澄清结论：用户确认优先复用仓库既有 `.card`、`.table-container`、`.table` 原生管理员主题模式。
- 方案结论：采用方案 A，页面消费既有主题类；不追加整套页级 `dark:*`，不新增利润页专属 CSS。
- 设计第 1 段（视觉与可读性）：用户已批准。
- 设计第 2 段（中文本地化）：用户已批准。
- 设计第 3 段（页级测试与回归边界）：用户已批准。
- 设计第 4 段（数据流、异常处理与交付边界）：用户已批准。
- 完整口头设计：用户已批准。
- 本书面规格书：用户已明确批准。
- 后续实施计划：用户已明确批准；批准后按计划完成 Task 1、Task 2 及其独立审查。

## 18. 规格书自审结果

- 占位符检查：无 `TBD`、`TODO`、`FIXME` 或未填写章节。
- 一致性检查：问题证据、方案选择、组件改动、验收矩阵和测试策略均指向复用既有主题类与补齐 i18n，不改变运行时数据流。
- 范围检查：运行时代码限制在利润页及中英文 locale，测试限制在页级合同；未扩展到后端、全局 CSS、其他页面或 T07。
- 歧义检查：明确了实际范围词条、七个表头 key、三项主题 class、允许文件和回滚责任，不保留双重解释。
- 接口检查：原生读取、今日覆盖、OAuth 日成本、字段与范围类型全部保持既有合同；控制面路径明确禁止。
- 发布检查：迁移和配置均无变化，`downtime_required=false` 仅为预期，最终发布预检和线上验收仍归根总控。
