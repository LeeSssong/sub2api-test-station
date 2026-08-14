# T06 原生管理员利润页移除外部控制面状态/调用规格书

## 1. 任务与基线

- 任务包：T06
- 任务名称：原生管理员利润页移除外部控制面状态/调用
- 设计状态：`DESIGNING`，等待用户书面批准规格书
- 设计基线：根总控最新干净 `main@032b3591e2df7408641b48ae584c10eee8e7a0be`
- 独立 worktree：当前 T06 顶层任务 worktree；不得从其他候选分支派生
- 允许修改：T06 规格、实施计划（规格批准后）、页级测试、任务报告和交接证据
- 明确禁止：利润页运行时代码改动、成本公式改动、账号监控、用量页、共享控制面文件、根 `main`、生产、GitHub Actions

## 2. 问题证据与当前行为

当前基线的 `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue` 由提交 `2aff29c6e` 的管理员财务首页重写，后续 `3d0d44630` 与 `94063b339` 仅补充原生财务行细节和今日覆盖。当前页面只导入 `adminAPI` 的原生 `accountFinancial` API，读取 `/api/v1/admin/operations/account-financial`，并通过既有原生管理员接口保存今日营收/成本与 OAuth 日成本。

当前页面和页级测试中，`git grep` 对以下外部控制面或状态符号均为零命中：`controlPlaneAPI`、`@/api/controlPlane`、`/xingqiao`、`ReadModelStatus`、`useReadModelFreshness`、`degraded`、`integrity`、`unknown`、`控制面`、`完整性`。当前页级测试只 mock `accountFinancial`，未调用控制面。

历史证据显示外部行为曾存在于 `e26649ab4`/`b69e26b55`：利润页曾导入 `controlPlaneAPI`，在非 `legacy_only` 模式请求 `/xingqiao/operations/profitability`，并渲染 `ReadModelStatus`、控制面来源、degraded/完整性状态。该行为已随 `2aff29c6e` 重写移除。本任务不重复删除已不存在的运行时代码，而是建立局部可执行的防回归门禁。

## 3. 目标

1. 保持利润页当前原生运行时行为不变。
2. 以页级守门测试证明首次加载、手动刷新、范围切换和自动刷新只使用原生 `accountFinancial` 读取。
3. 以页级静态源码合同阻止未来官方更新重新引入外部控制面调用、控制面状态组件或 unknown/degraded/完整性状态。
4. 保持原生利润数据、字段、成本/利润公式、今日覆盖、OAuth 成本写入、异常跳转和官方页面入口行为不变。

## 4. 非目标

- 不删除或重构共享 `src/api/controlPlane.ts`、`src/config/externalizationFlags.ts` 或其他页面的控制面能力。
- 不修改后端 handler、service、数据库 schema、迁移、成本公式、财务汇总和审计语义。
- 不修改账号监控、用量页、账务页、对账页或外部控制面服务。
- 不新增页面、路由、配置开关、环境变量、网络请求、缓存或轮询机制。
- 不在生产修改财务数据，不执行今日覆盖或 OAuth 成本写入探测。

## 5. 方案比较与选择

### 方案 A：仅运行时守门测试

在 `AccountProfitabilityView.spec.ts` mock 控制面客户端为失败型 spy，覆盖加载、刷新、切换范围并断言控制面调用为零；同时断言页面不渲染控制面状态。

优点是实现最小、贴近用户行为；缺点是未来可能重新加入静态依赖但暂时不发请求时，单靠运行时测试不一定拦截。

### 方案 B：运行时守门 + 利润页局部静态禁词断言（推荐）

保留方案 A，并在同一页级测试中用 `?raw` 读取 `AccountProfitabilityView.vue`，拒绝该页面重新引入控制面 API、`/xingqiao/` 路径、读模型状态组件和控制面状态变量。静态合同只覆盖利润页，不扫描、不修改共享控制面文件。

优点是同时防止“重新调用”和“重新显示状态”两类回归，改动仍限定于一个测试文件；缺点是测试对少量页面符号有轻微源码结构耦合。该取舍符合用户要求的最短路径和最小范围。

### 方案 C：仅保留现状证据，不新增测试

依赖当前源码、历史 diff 和人工审查声明行为已满足。

优点是零代码改动；缺点是没有可执行的未来回归门禁，无法满足“守门测试与证据”的长期要求。

选择方案 B。

## 6. 端到端数据/控制流

```text
进入利润页 / 手动刷新 / 60 秒刷新 / 切换范围
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
原生财务报告 -> summary/accounts -> 页面汇总卡片与账号行
```

今日覆盖和 OAuth 日成本仍沿用现有原生 PUT 接口，写入完成后重新读取上述原生报告。控制流中不存在 control-plane decision、外部 profitability read、`/xingqiao/**` 请求或外部状态渲染。

## 7. 接口与字段契约

本任务不改变接口，只固化现有契约：

- 原生读取：`GET /api/v1/admin/operations/account-financial?range=<today|24h|7d|31d>`。
- 原生报告：`generated_at`、`range`、`summary`、`accounts`、`exception_count`、`affected_revenue`、`user_unconsumed_balance_cny`。
- `summary`/账号 `amounts`：`revenue`、`cost`、`profit`、`margin`、`exception_count`、`affected_revenue`。
- 今日营收/成本覆盖：现有 `PUT /api/v1/admin/accounts/:id/financial/today-override`，带北京业务日期和显式金额字段。
- OAuth 日成本：现有 `PUT /api/v1/admin/accounts/:id/financial/oauth-cost`，带北京业务日期和 `cost_cny`。
- 控制面接口：本任务不得从利润页调用或新增任何 `/xingqiao/**` 接口；共享控制面 API 的其他页面契约保持不变。

## 8. 失败、安全与兼容语义

- 原生报告失败继续使用当前页面既有失败/加载语义；本任务不引入外部 fallback、重试或状态替代。
- 控制面 spy 被调用、页面源码命中禁词或页面出现控制面/完整性/unknown 状态时，页级测试失败，候选不得进入发布审查。
- 不改变管理员认证、会话恢复、权限边界或错误脱敏；不把控制面错误转换为原生财务错误。
- 不改变原生财务数据、成本公式、账号类型或 OAuth/非 OAuth 语义；不写生产数据。

## 9. 兼容性与迁移

- 运行时代码、API、DTO、路由、数据库 schema、迁移、配置和 i18n 均无变化。
- 预计迁移集合无变化，预期 `downtime_required=false`；由根总控在合并后发布预检最终确认。
- 测试只依赖现有 Vitest/Vue Test Utils 和 Vite `?raw` 模块能力，不新增依赖。
- 官方后续更新若改变利润页实现，必须先通过运行时守门和静态页级禁词合同；共享控制面能力仍可供其他页面使用。

## 10. 场景化验收矩阵

| 场景 | 验收条件 |
| --- | --- |
| 首次进入利润页 | 原生 `getReport({ range: 'today' })` 被调用并渲染既有六项汇总与账号行；控制面调用为 0 |
| 手动刷新 | 仅追加一次原生 `getReport`；无 `/xingqiao/**` |
| 范围切换 | `24h`、`7d`、`31d` 只以对应 range 调用原生读取；今日编辑在非 today 不出现 |
| 自动刷新 | 继续使用既有 60 秒 timer；不新增控制面轮询 |
| 今日覆盖/OAuth 成本 | 既有原生写接口和北京日期字段不变；成功后重新读取原生报告 |
| 原生接口失败 | 保持当前页面错误语义；不显示外部、unknown、degraded 或完整性状态 |
| 静态回归 | 利润页源码不含约定禁词/依赖符号；共享控制面文件不在变更集合中 |
| 线上验收 | 管理员登录后初载、reload、手动刷新、范围切换正常；浏览器 `/api/v1/xingqiao/**` 请求为 0；健康端点通过 |

## 11. 测试策略

仅在 `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts` 增加测试：

1. 运行时失败型控制面 spy：`profitability`/`decision` 一旦调用立即使测试失败，并在首次加载、刷新、范围切换后断言调用次数为 0。
2. 原生读取断言：验证 `accountFinancial.getReport` 的 range 参数和既有展示/编辑语义。
3. 页面状态断言：渲染文本不含控制面、完整性和 unknown 状态。
4. 静态源码合同：通过 `?raw` 检查利润页不含 `@/api/controlPlane`、`controlPlaneAPI`、`getControlPlaneReadMode`、`/xingqiao/`、`ReadModelStatus`、`useReadModelFreshness`、`resolveTrustedPageDecision`、`controlPlaneDegraded`、`renderSource` 等页面专属符号。
5. 复跑既有 `admin.accountFinancial.spec.ts`，并执行前端 typecheck、production build、diff check 与定向 `rg`。

当前 worktree 无 `node_modules`；依赖准备和命令执行属于规格批准后的计划/实施阶段，本规格不宣称测试已通过。

## 12. 发布、线上验证与回滚条件

- 顶层任务完成实现、测试、独立任务复审和全分支终审后只能报告 `READY_FOR_ROOT_REVIEW`。
- 只有根总控授权后才能合并到 `main`、推送、部署和线上验收。
- 合并前/发布前必须确认候选仍基于届时最新 `main`，工作树干净，变更集合仅含允许的测试/规格/报告文件，且 `downtime_required=false`。
- 线上验证只读检查利润页功能和网络请求，不写入财务覆盖；如发现 `/xingqiao/**`、外部状态或原生财务异常，保留候选与证据并停止推进。
- 若部署失败，按根总控蓝绿链切回上一活动槽；因无迁移/配置变化，不需要数据库回滚。
- 若仅测试合同失败，在同一候选修复；不得删掉证据、覆盖其他 worktree 或扩大到共享控制面文件。

## 13. 规格书自审结果

- 占位符扫描：未发现 `TBD`、`TODO`、`FIXME` 或空白要求。
- 一致性检查：方案 B、端到端控制流、验收矩阵和测试策略均限定为页级测试/证据；未引入运行时代码、共享控制面、数据库或生产变化。
- 范围检查：变更可收敛为一个页级测试文件，规格、复审和报告仅作交接证据；未拆出相邻任务。
- 契约检查：原生读取/写入接口、字段、北京业务日期、管理员权限和异常跳转均明确为既有契约；控制面接口明确为禁止从利润页调用。
- 失败语义检查：测试失败、真实残留、发布/线上验证失败均 fail-closed，保留候选和证据，不自动扩大范围。
- 发布检查：无迁移/配置变化，`downtime_required` 仅作为预期值，最终由根总控发布预检确认；规格未宣称测试或生产已通过。

## 14. 仍待决事项

- 用户是否批准本书面规格书；在批准前不得调用 `writing-plans`、写实现代码、派生实现代理或进入实施。
- 规格批准后由根总控确认依赖安装方式、定向测试命令和最终发布预检；这些不改变本规格范围。
- 若后续取证发现当前页面存在未覆盖的真实外部分支，必须停下并回到规格修订，不得自行扩围。

## 15. 设计批准记录

- 方案选择：根总控确认采用方案 B（运行时守门 + 页级静态禁词断言）。
- 设计第 1 段（端到端边界与控制流）：根总控已批准。
- 设计第 2 段（验收矩阵与失败/兼容语义）：根总控已批准。
- 设计第 3 段（测试、发布、线上验证与回滚边界）：根总控已批准。
- 书面规格书批准：待用户明确批准。
