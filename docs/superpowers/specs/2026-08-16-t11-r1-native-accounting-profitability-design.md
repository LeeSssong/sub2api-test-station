# T11-R1 Sub 原生计费聚合经营页纠偏设计

**文件名日期说明：** 当前系统日期为 2026-08-15；本文件按根总控移交明确指定的未来日期文件名 `2026-08-16` 保存。
**状态：** 正式规格已完成并自审，等待唯一发布总控书面审查与批准
**任务状态目标：** 本窗口最终只推进到 `READY_FOR_ROOT_REVIEW`
**基线：** `main@6289c22a31a9c6a53836e2086f2f356c13be1c1b`
**候选分支：** `codex/t11-r1-native-accounting-profitability`

## 1. 问题证据与当前行为

T11 已上线并完成生产验收的范围是经营页结构与状态基础：

- 全站固定摘要；
- 分组 Tab；
- 全站账号行和当前分组账号行；
- 今日、24 小时、7 天、31 天；
- 刷新以及错误重试；
- 用量异常页的 loading、data、empty、error/retry。

但当前 `GET /api/v1/admin/operations/account-financial` 并不直接聚合 Sub 原生
`usage_logs`。现有读取链先受 `account_financial_settings.enabled_at` 截断，再读取
`usage_upstream_cost_evidence`、`usage_cost_reviews`、OAuth 日成本和今日人工覆盖：

```text
AccountProfitabilityView
  -> /admin/operations/account-financial
  -> AccountFinancialService.GetReport
  -> AccountFinancialRepository.ReadSnapshot
  -> usage_logs + evidence + reviews + daily values + overrides
```

只有独立证据为 confirmed 或已人工复核的非 OAuth 流水才进入现有财务汇总。生产只读诊断曾在一个当日窗口看到 271 条官方流水、约 `¥7.825477` 被该模型排除，原因主要为
`endpoint_unsupported` 和 `evidence_not_registered`。这些流水已经存在于 Sub 原生
`usage_logs`，却没有进入经营页。

页面本身还保留以下不再适用于本次产品方向的内容：

- 白色今日营收、成本和 OAuth 成本输入；
- 摘要“异常流水”卡片；
- 账号行异常数量、操作列和成本异常页跳转；
- 人民币符号与收入/支出的旧账务文案。

用户未消费余额卡片保持现有只读语义。它是当前活动用户余额快照，不是所选时间范围的经营数值，不参与全站、分组、账号的用量守恒校验，本任务不改变其 CNY 字段和展示语义。

此外，当前经营页虽然维护 `loading` 状态，但没有渲染可见 loading；页面空结果也没有独立 empty 呈现。T11-R1 必须在更换数据源时保留并补实 loading、data、empty、error/retry 合同。

当前原生代码已有可复用的正确统计口径：

- `account_usage_service.go` 的 `WindowStats`、`GetTodayStats` 和批量今日统计；
- `usage_log_repo_stats.go` 的今日/窗口 SQL；
- 原生账号页面的“请求 / Token / 账号计费 / 用户扣费”展示；
- 旧 `AccountProfitabilityService.queryAggregates` 中已经验证过的账号成本表达式。

本任务复用这些字段与公式，不复用旧 `AccountProfitabilityService` 的来源判断、采购成本、人民币或成本待配置模型。

## 2. 目标与非目标

### 2.1 目标

1. 保留现有全站固定摘要、分组 Tab、全站账号行、当前分组账号行、四个时间范围、自动/手动刷新和页面状态。
2. 经营页六项经营指标只来自 Sub 原生 `usage_logs` 官方字段或其确定性派生结果；原有未消费余额卡片是明确排除在外的只读余额快照。
3. 以互斥的 `(group_id, account_id)` 聚合行作为基础，保证全站、分组、全站账号和分组账号守恒。
4. 六项经营指标的金额统一保持美元，不做人民币或汇率换算；未消费余额卡片保留原 CNY 语义。
5. 删除人工输入、“异常流水”摘要、账号异常数量/操作和成本异常页入口。
6. 历史 T03/T03-R1 表、记录、审计、异常页与写接口继续保留，不做破坏性清理。
7. 保留用户未消费余额卡片的现有只读 CNY 快照语义，并将其与本次六项 USD 经营指标明确分隔。

### 2.2 非目标

- 不新增计费模型、第二事实源、平行经营 API 或外部控制面。
- 不使用 `usage_upstream_cost_evidence`、reviews、OAuth 日成本或人工 override 作为经营页输入。
- 不修改计费写入、余额、扣费、账号配额、调度、路由或账号状态。
- 不估算、不补查、不重试上游、不回填历史、不修改生产业务数据。
- 不做采购成本分摊、人民币经营口径或汇率换算。
- 不删除历史异常页面、证据表、复核表、每日值表或审计记录。
- 不修改普通用户入口或 DTO。
- 不改变 `user_unconsumed_balance_cny` 的口径、币种、数据来源或只读展示；它不属于本次 `usage_logs` 经营指标改造。
- 不新增数据库迁移、依赖、环境变量、运行配置或 GitHub Actions。
- 不顺带改造旧 `/api/v1/admin/operations/account-profitability` 服务。

## 3. 影响用户与边界条件

- 影响对象仅为已登录管理员。
- 全站总计必须包含范围内全部官方 `usage_logs`，不能因为账号或分组已软删除、独立证据缺失或旧财务功能未启用而排除。
- 当前活动账号继续出现在全站账号视图；范围内有流水的历史/软删除账号也必须以稳定账号 ID 和可用历史名称出现，避免总计与账号行不守恒。
- 当前活动分组 Tab 继续保留，即使所选范围为零流水；历史/软删除分组仅在范围内存在引用流水时出现。
- `usage_logs.group_id IS NULL` 的流水进入明确的“未归属”分组；不根据账号当前绑定关系猜测历史归属。
- 分组账号行严格以 `(group_id, account_id)` 为键；同一账号跨分组的流水分别进入对应行。
- 全站账号行以 `account_id` 折叠同一账号的全部分组行；全站总计直接由互斥基础行求和，每条流水只计一次。
- 账号或分组名称不可读时使用稳定 ID 回退，例如 `账号 #123`、`分组 #45`，金额仍必须保留。
- `user_cost = 0` 时，`profit` 仍为 `0 - cost`，`margin` 返回 `null`，前端显示“—”。

## 4. 方案比较与批准结论

### 4.1 方案 A：原生 usage 聚合读取并复用现有经营 API（已批准）

在 `usage_log_repo_stats.go` 增加窄的只读经营聚合能力，按
`(usage_logs.group_id, usage_logs.account_id)` 返回互斥统计行。现有
`AccountFinancialService` 注入该只读能力并折叠成当前报告结构；历史财务 repository 继续服务异常、复核和人工写接口，但不再参与 `GetReport`。

优点：

- 直接复用原生 SQL 字段与公式；
- 三层聚合天然守恒；
- 查询返回聚合行而不是 31 天逐条明细；
- 现有路由、页面和发布链变化最小；
- 历史证据能力可非破坏性保留。

代价：需要新增一个小型只读接口，并调整 service 构造、wire 和测试装配。

### 4.2 方案 B：继续读取逐条财务快照并在 Go 中改算

为 `AccountFinancialSnapshotEntry` 补官方 Token/成本字段，在报告模式跳过 evidence/review/daily values，再由 Go 遍历聚合。

优点是表面代码改动较少；缺点是 31 天窗口会搬运大量明细，报告与异常快照继续共享膨胀接口，未来容易重新耦合。未选择。

### 4.3 方案 C：迁移到旧 account-profitability 服务

扩展旧 `/operations/account-profitability` 的 SQL 和 DTO，再让当前页面切换接口。该服务已混入来源、采购成本、人民币和成本待配置逻辑，剥离成本高且会制造两个经营 API 的迁移问题。未选择。

### 4.4 批准记录

唯一发布总控依据项目在 2026-08-15 获得的用户离席代审授权，已批准方案 A，并批准
`user_cost=0 -> margin=null -> 前端显示“—”`。

## 5. 架构与端到端数据流

### 5.1 组件职责

#### 原生 usage 聚合 reader

位置优先为 `upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go`。

它只负责：

1. 接收半开区间 `[from, to)`；
2. 按 `(group_id, account_id)` 聚合官方字段；
3. 返回账号/分组身份元数据或稳定 ID；
4. 返回当前活动账号和分组元数据，以维持现有零流水账号/分组视图；
5. 在同一只读快照中保留当前活动用户 `balance` 之和，仅供原有未消费余额卡片；
6. 不读取任何独立证据、人工复核、OAuth 日值或 override。

建议定义一个窄接口供财务 service 注入，而不是把新方法扩进庞大的通用
`UsageLogRepository` 接口。名称可以按实现风格确定，但职责必须等价于：

```go
type AccountFinancialUsageReader interface {
    ReadAccountFinancialUsage(ctx context.Context, from, to time.Time) (*AccountFinancialUsageSnapshot, error)
}
```

#### AccountFinancialService

`GetReport` 只负责：

1. 根据 `today|24h|7d|31d` 计算 `[from,to)`；
2. 调用原生 usage 聚合 reader；
3. 从互斥 pair 行折叠全站、分组、全站账号和分组账号；
4. 计算 `profit` 与 `margin`；
5. 透传未消费余额的现有只读 CNY 快照，不将它折入任何经营聚合；
6. 生成同一 `generated_at` 和六项经营指标 `currency=USD` 的响应。

`ListExceptions`、Review、OAuth 日成本、TodayOverride 和证据详情继续调用原
`AccountFinancialRepository`，不在本任务删除。

#### AccountProfitabilityView

页面继续只调用：

```text
GET /api/v1/admin/operations/account-financial?range=<today|24h|7d|31d>
```

页面只负责显示后端提供的维度和字段，不在浏览器重算分组金额或从当前账号绑定推断归属。

### 5.2 数据流

```text
管理员进入页面 / 切换范围 / 自动刷新 / 手动刷新
        |
        v
GET /api/v1/admin/operations/account-financial?range=...
        |
        v
AccountFinancialService 计算 [from,to)
        |
        v
usage_logs 原生只读聚合
GROUP BY (group_id, account_id)
        |
        +--> requests / tokens / cost / user_cost
        |
        v
service 折叠
  - 全站 summary
  - group_id 分组
  - account_id 全站账号
  - (group_id, account_id) 分组账号
        |
        v
前端呈现 loading / data / empty / error+retry
```

### 5.3 时间范围

所有查询使用半开区间 `[from,to)`，`to` 为统一生成时刻：

| 范围 | `from` | `to` |
| --- | --- | --- |
| 今日 | Asia/Shanghai 当日 00:00 | 生成时刻 |
| 24h | 生成时刻减 24 小时 | 生成时刻 |
| 7d | Asia/Shanghai 当日往前 6 天 00:00 | 生成时刻 |
| 31d | Asia/Shanghai 当日往前 30 天 00:00 | 生成时刻 |

查询不使用 `account_financial_settings.enabled_at`，因为产品目标是所选范围内全部官方流水。

### 5.4 分段内部审查结论

PASS：六项经营指标的数据源唯一、基础聚合行互斥、时间边界明确；可证明全站、分组和账号投影守恒。余额卡仅是独立只读快照，两者都不需要新表或新写链。

## 6. 统计公式与接口字段合同

### 6.1 官方聚合公式

每个互斥 pair 行固定使用：

```sql
requests = COUNT(*)

tokens = SUM(
  input_tokens
  + output_tokens
  + cache_creation_tokens
  + cache_read_tokens
)

cost = SUM(
  COALESCE(
    account_cost,
    COALESCE(account_stats_cost, total_cost)
      * COALESCE(account_rate_multiplier, 1)
  )
)

user_cost = SUM(actual_cost)
```

所有 SUM 使用 `COALESCE(..., 0)` 保证合法零值。金额单位为 USD。

派生值：

```text
profit = user_cost - cost
margin = user_cost == 0 ? null : profit / user_cost
```

不使用 `standard_cost` 作为经营页可见值。

### 6.2 规范响应

路径与范围参数保持不变：

```text
GET /api/v1/admin/operations/account-financial?range=today|24h|7d|31d
```

规范响应结构：

```json
{
  "generated_at": "2026-08-15T12:00:00+08:00",
  "range": "today",
  "currency": "USD",
  "user_unconsumed_balance_cny": 120.5,
  "summary": {
    "requests": 100,
    "tokens": 250000,
    "cost": 4.25,
    "user_cost": 7.5,
    "profit": 3.25,
    "margin": 0.4333333333
  },
  "accounts": [{
    "id": 11,
    "name": "Sub relay",
    "type": "api_key",
    "platform": "openai",
    "historical": false,
    "amounts": {}
  }],
  "groups": [{
    "id": 6,
    "name": "GPT-Plus",
    "unassigned": false,
    "historical": false,
    "amounts": {},
    "accounts": []
  }]
}
```

`amounts` 在 summary、group 和 account 层使用相同六字段合同。

`user_unconsumed_balance_cny` 保留旧合同：它是当前活动用户余额之和的 CNY 只读快照，与 `range`
无关，不属于 `summary.amounts`，不参与 profit、margin 或任何守恒式。`currency=USD` 只限定六项原生经营指标。

全站账号 `accounts[]` 以 `account_id` 折叠全部 pair 行；`groups[].accounts[]` 保持
`(group_id, account_id)` 粒度。当前活动但零流水的账号必须保留为零值行；历史账号只有在范围内有流水时出现。

### 6.3 兼容合同

- API 路径、范围参数、顶层 `summary/accounts/groups` 结构不变。
- 新前端 normalization 必须兼容旧后端的 `revenue/RevenueCNY` 与 `cost/CostCNY`，用于受控回滚或短暂版本交错；旧收入映射为新 `user_cost`。
- 新后端的规范字段为 `user_cost` 和 `cost`，并在本任务的兼容窗口继续返回弃用的 `revenue`、`expense` 只读别名；两者必须分别等于同一官方 `user_cost`、`cost`，且不得带 CNY 语义。
- 新经营报告不再返回 `exception_count`、`affected_revenue`、`complete` 和 `has_unallocated_adjustments`。历史异常与人工写接口自身的响应合同不变。
- `user_unconsumed_balance_cny` 继续返回且语义不变；新旧前后端交错时均不得将它解释为 USD 经营指标。

### 6.4 页面可见合同

全站与分组的经营指标区统一显示六项：

1. 请求数；
2. Token；
3. 账号计费；
4. 用户扣费；
5. 利润；
6. 利润率。

全站摘要另保留一张“用户未消费余额”卡片，因此全站共七张卡：六张 USD 经营指标卡加一张现有 CNY 余额卡。分组摘要只显示六项 USD 经营指标，不分摊或复制全站用户余额。

账号表显示：账号、请求数、Token、账号计费、用户扣费、利润、利润率。

删除：

- 人工营收输入；
- 人工成本输入；
- OAuth 日成本输入；
- “今日覆盖”操作列；
- 异常数量列和按钮；
- `/admin/usage?tab=cost-exceptions...` 跳转；
- 摘要“异常流水”卡片；
- 分组“不完整/未分摊调整”提示。

六项经营指标的金额使用美元符号 `$`，并复用原生账号统计的动态精度：较小金额不得被固定两位格式化为假零。请求和 Token 使用现有原生数字/紧凑格式。未消费余额卡片单独沿用 CNY 格式，不得改标为 USD。

### 6.5 分段内部审查结论

PASS：六项经营指标均可追溯到 `usage_logs` 官方字段或确定性派生公式；未消费余额的原有 CNY 只读例外已明确隔离。单位、零分母和兼容边界明确，不引入人工经营值或汇率语义。

## 7. 页面状态、失败与安全语义

### 7.1 状态合同

- `loading`：首次加载且尚无成功数据时显示可见骨架或加载状态。
- `data`：有成功响应时显示摘要、Tabs 和对应账号行。
- `empty`：成功响应但当前范围无流水时显示明确空态，保留范围、刷新和分组导航。
- `error`：读取失败时显示本地化错误和 retry；不得把失败呈现为零数据。
- `retry`：重新请求当前范围。
- `refreshing`：已有成功数据时刷新不清空旧内容，只显示轻量刷新状态。

快速切换范围或连续刷新时必须使用请求序号、取消机制或等价发布守卫，确保旧响应不能覆盖最新范围。

### 7.2 失败语义

- repository 失败直接返回错误；不得回退旧 evidence、人工值、估算、缓存财务快照或另一经营 API。
- 身份元数据缺失不应丢弃官方金额；使用稳定 ID 回退。
- 非法范围继续返回 HTTP 400。
- service 或 reader 未装配继续 fail-closed 返回 HTTP 500，不返回伪造零报表。

### 7.3 安全语义

- 路由继续位于既有管理员认证组。
- 不扩展普通用户 DTO 或入口。
- 响应只包含聚合数字和非敏感账号/分组身份，不返回 credentials、完整 API Key、上游响应或请求正文。
- 报告请求全程只读，不触发上游网络、补查、重试、回填、审计写入、人工复核或计费写入。
- 页面不得引入 `/xingqiao/**`、external-primary 或控制面状态。

### 7.4 历史能力保留

以下能力继续存在，但经营页不调用、不展示：

- 成本异常 Tab 和详情；
- `usage_upstream_cost_evidence`；
- `usage_cost_reviews`；
- OAuth 日成本；
- 今日营收/成本 override；
- 对应审计记录与管理员写接口。

未来如需删除，必须另立任务并单独评估数据保留与回滚。

### 7.5 分段内部审查结论

PASS：失败不会被伪装成经营结果；历史数据与审计得到非破坏性保留；认证、脱敏和只读边界不变。

## 8. 兼容性、迁移与配置

- 数据库 schema：无变化。
- 数据库迁移：无新增、无修改。
- 历史数据：不回填、不更新、不删除。
- 计费写入：无变化。
- 运行配置与环境变量：无变化。
- 依赖与锁文件：无变化。
- GitHub Actions：无变化且不得新增发布 workflow。
- 路由：经营 GET 路径不变；历史异常/人工写路由保留。
- 旧 `/operations/account-profitability`：不修改、不迁移当前页面过去。
- 预期发布属性：应用代码只读查询与 UI 更新，预计 `downtime_required=false`；以根 `main` 的发布预检为最终事实。

### 8.1 分段内部审查结论

PASS：本任务没有数据转换、不可逆操作、停机前提或配置漂移，范围保持单一垂直纠偏。

## 9. 场景化验收矩阵

| 场景 | 验收条件 |
| --- | --- |
| 今日 | 北京当日零点至生成时刻的官方流水全部进入统计 |
| 24 小时 | 精确滚动 24 小时，不扩成自然日 |
| 7 天 | 北京当日加前 6 个自然日，至同一生成时刻 |
| 31 天 | 北京当日加前 30 个自然日，至同一生成时刻 |
| Token | 输入、输出、缓存创建、缓存读取之和 |
| 账号计费 | 严格使用批准的 `COALESCE(account_cost, ...)` 公式 |
| 用户扣费 | 严格使用 `SUM(actual_cost)` |
| 利润 | 每层都等于 `user_cost - cost` |
| 零用户扣费 | `margin=null`，前端显示“—”；profit 仍可为负 |
| 跨分组账号 | 各分组账号行按 pair 分开；全站账号折叠；全站流水只计一次 |
| 未归属 | `group_id NULL` 进入未归属，且参与全站守恒 |
| 历史账号/分组 | 身份缺失或软删除不丢金额，使用历史名称或 ID 回退 |
| 零流水活动分组 | Tab 保留，摘要为零，账号区显示 empty |
| 首次加载 | 可见 loading，不先显示伪零数据 |
| 刷新 | 手动和 60 秒刷新保留；已有数据不在刷新时清空 |
| 快速范围切换 | 旧响应不能覆盖最新选择 |
| 空结果 | 明确 empty，可继续切换范围或刷新 |
| 接口失败 | 明确 error + retry，不回退旧证据或估算 |
| UI 删除项 | 无白色输入、“异常流水”卡、异常列/操作/跳转 |
| 余额卡保留 | 全站摘要仍有用户未消费余额，保持原 `user_unconsumed_balance_cny` 只读 CNY 语义，不随分组聚合 |
| 单位 | 六项经营指标显示 USD `$`，无汇率；仅保留的未消费余额卡继续使用 CNY |
| 权限 | 未认证/非管理员继续按现有中间件拒绝 |
| 移动端 | 390×844 无页面级横向溢出；表格可受控横向滚动 |

## 10. 测试策略

### 10.1 Backend repository

新增定向单元/集成测试，覆盖：

- 精确 SQL 成本表达式；
- `requests` 与四类 Token 求和；
- `actual_cost` 用户扣费；
- 当前活动用户余额之和保留在 `user_unconsumed_balance_cny`，不受时间范围和财务启用时间截断；
- `[from,to)` 半开范围；
- `(group_id, account_id)` 聚合；
- 同账号跨分组；
- `group_id IS NULL`；
- 软删除/历史账号与分组身份；
- 零值与数据库错误；
- 查询不引用 evidence/review/daily-value 表。

### 10.2 Backend service

覆盖：

- 全站 = 所有分组（含未归属）之和；
- 分组 = 其 pair 行之和；
- 全站账号 = 同 `account_id` 的 pair 行之和；
- 四个时间范围；
- 利润与利润率；
- `user_cost=0 -> margin=nil`；
- 活动零流水账号/分组；
- 历史身份回退；
- reader 失败 fail-closed；
- `GetReport` 不调用旧 `ReadSnapshot`。

### 10.3 Handler/API

覆盖：

- 四个合法范围；
- 非法范围 400；
- reader/service 不可用 500；
- `currency=USD`；
- summary/group/account 六字段；
- 管理员路由隔离。

### 10.4 Frontend

覆盖：

- 新字段 normalization 与旧响应兼容；
- 全站七张摘要卡（六张 USD 经营指标加一张 CNY 余额卡）、分组六张经营指标卡和账号表七列；
- 美元动态精度；
- 全站/活动分组/历史分组/未归属切换；
- loading、data、empty、error/retry；
- 刷新保留旧数据；
- 并发请求最新响应胜出；
- 今日、24h、7d、31d；
- 已批准删除项不存在，未消费余额卡仍存在且保持原语义；
- 中英文生产 locale 不泄漏 key；
- 390×844 防页面级溢出。

### 10.5 负向范围门禁

经营报告和页面差异不得引入或继续消费：

- `usage_upstream_cost_evidence`；
- `usage_cost_reviews`；
- OAuth 日成本；
- today override；
- `tab=cost-exceptions` 或经营页到 `/admin/usage` 的异常跳转；
- 六项经营指标中的 `¥` / CNY / 汇率（保留的 `user_unconsumed_balance_cny` 余额卡为唯一 CNY 例外）；
- `/xingqiao/**`、control plane、external-primary；
- `.github/workflows` 变化。

### 10.6 计划验证命令

实施计划阶段应精确化命令，至少包含：

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'Test.*AccountFinancial.*Usage|Test.*NativeAccounting' -count=1
go test ./internal/service -run 'TestAccountFinancial' -count=1
go test ./internal/handler/admin -run 'TestAccountFinancial' -count=1

cd upstream/sub2api/frontend
pnpm exec vitest run \
  src/api/__tests__/admin.accountFinancial.spec.ts \
  src/views/admin/__tests__/AccountProfitabilityView.spec.ts
pnpm typecheck
pnpm build

git diff --check
```

若 PostgreSQL 集成环境不可用，必须明确记录未验证，不得用 SQLite 或 mock 冒充已完成真实 SQL 验证。

### 10.7 分段内部审查结论

PASS：测试覆盖字段口径、三层守恒、时间边界、状态完整性、权限与明确禁区。

## 11. 发布、即时线上验收与回滚

### 11.1 顶层任务边界

本候选完成实现、定向测试、typecheck/build、diff 自查、逐任务独立 reviewer 和最终全分支 reviewer 后，只能报告 `READY_FOR_ROOT_REVIEW`。

本窗口不得：

- 修改根 `main`；
- 修改 `docs/project/project-progress.md`；
- 修改 `docs/project/native-sub-task-package-queue.md`；
- 推送、部署或改生产；
- 自行发出合并授权。

### 11.2 根总控发布门禁

根总控授权并合入当前最新 `main` 后，必须重新执行：

- 后端定向测试；
- 前端定向测试；
- typecheck 和 production build；
- migration/config/dependency/GitHub Actions diff；
- 禁区扫描；
- 发布资格与预检。

只有预检明确 `downtime_required=false` 才能进入受审本地/宿主蓝绿链。若为
`downtime_required=true`，必须在任何停服、迁移、重启、候选启动或切流前停止并等待用户明确确认。

### 11.3 即时线上验收

不等待小时或 24 小时窗口。发布后立即验证：

1. 管理员登录态四个范围均返回并显示数据；
2. 全站、分组、全站账号、分组账号抽样与官方字段一致；
3. 全站/分组/pair 守恒；
4. 六项经营指标的美元、利润和零 `user_cost` 利润率合同，以及未消费余额卡的独立 CNY 只读合同；
5. 手动刷新和状态切换；
6. loading、empty、error/retry；
7. 390×844；
8. 无“异常流水”卡、人工输入、异常列/操作/跳转；未消费余额卡仍存在；
9. 页面只调用原生管理员 API，无旧证据/控制面网络请求；
10. 未认证接口按预期拒绝；
11. `/healthz`、`/readyz`、`/health` 与 API/worker 健康。

不得修改生产流水或制造付费请求来填充验收数据；自然数据不足的边界由同发布树专项测试证明。

### 11.4 回滚

回滚为蓝绿切回上一活动应用镜像。由于本任务无迁移、配置或数据写入：

- 不需要数据库回滚；
- 不删除新数据；
- 历史 T03/T03-R1 表、记录和审计保持原样；
- 失败候选、测试证据和回滚 SHA 必须保留。

### 11.5 分段内部审查结论

PASS：预计为可逆应用级变更，无停机或数据恢复前提；生产权力严格保留给唯一发布总控。

## 12. 预期变更范围

实施计划可在以下最小文件集合内细化：

### Backend

- `internal/repository/usage_log_repo_stats.go`
- 对应 repository unit/integration tests
- `internal/service/account_financial.go`
- `internal/service/account_financial_test.go`
- service/wire 装配文件
- `internal/handler/admin/account_financial_handler_test.go`（仅字段/范围合同需要时）

### Frontend

- `src/api/admin/accountFinancial.ts`
- `src/api/__tests__/admin.accountFinancial.spec.ts`
- `src/views/admin/AccountProfitabilityView.vue`
- `src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
- 中英文 admin locale

若实现需要修改本清单外的运行时代码，必须先判断是否为本规格直接依赖；涉及 schema、迁移、计费写入、生产配置、历史数据清理或范围扩大时立即停止并报告。

## 13. 完成定义与剩余风险

### 13.1 顶层候选完成定义

只有以下全部满足，顶层任务才可报告 `READY_FOR_ROOT_REVIEW`：

- 规格和实施计划均获批准；
- 每个计划任务由 fresh implementer 实施；
- 每个任务有独立只读 reviewer；
- 最终全分支 reviewer 完成；
- 后端/前端专项、typecheck、build、diff 和禁区检查通过；
- 无未解释迁移、配置、依赖或 GitHub Actions 变化；
- handoff 包含基线 SHA、提交 SHA、变更文件、测试、未验证项、迁移/配置、`downtime_required`、回滚和剩余风险。

这不代表已合并、已推送、已部署或已完成生产验收。

### 13.2 剩余风险

- 31 天官方流水量可能使新聚合 SQL 成为热点；实现必须用数据库聚合而不是逐条加载，并检查现有 `created_at/group_id/account_id` 索引可用性。不得为性能猜测新增迁移；如确需索引，立即停止并报告范围变化和潜在停机属性。
- 旧前端缓存与新后端短暂交错可能遇到字段差异；由 normalization 兼容和稳定路径降低风险。余额卡保留 CNY，前端必须与 USD 经营金额使用不同格式化函数，避免误标币种。
- 历史软删除身份可能缺失名称；稳定 ID 回退保证金额不丢失。
- 浮点 JSON 展示可能有尾数；汇总守恒测试使用适当误差，页面格式化不得改变原始 API 值。

### 13.3 未决产品问题

无。方案 A、零分母利润率、已批准 UI 删除项、未消费余额卡保留、经营指标 USD 单位、历史数据保留和发布边界均已确认。

## 14. 书面批准门禁

本规格提交后必须停下，等待唯一发布总控基于 2026-08-15 离席代审授权形成书面审查与批准记录。

在书面规格批准前：

- 不调用 `superpowers:writing-plans`；
- 不写实施计划；
- 不写实现代码；
- 不派生 implementer/reviewer；
- 不修改根总账、队列、生产或发布状态。
