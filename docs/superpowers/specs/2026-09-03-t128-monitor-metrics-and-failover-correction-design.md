# T128 性能指标与切号恢复纠偏规格

## 1. 文档状态

- 日期：2026-09-03
- 状态：DESIGNING，待用户审阅批准
- 任务性质：生产问题纠偏，拆分为三个串行可验收任务包：快照刷新根因修复、Monitor V4 指标口径修正、OpenAI 切号故障域优化
- 依赖：T120 账号监控加载性能与错误恢复、T125 账号监控统一请求口径、T126 自适应候选池/Top-K/公平性
- 当前基线：生产主站运行 `main@443a3ce8`；本地工作区已有未提交的 Monitor V4 三列 SELECT 修复，尚未授权合并、推送或部署

## 2. 问题证据与当前行为

### 2.1 快照刷新失败

生产日志持续出现：

```text
account_monitor_v4_snapshot: refresh failed
sql: expected 16 destination arguments in Scan, not 19
```

当前 `ProjectMonitorV4GroupsForGroups` 的 Go `rows.Scan` 接收 19 列，但同一生产提交的最终 SELECT 只返回 16 列；缺失字段为 `cache_read_tokens`、`cache_creation_tokens`、`cache_hit_denominator`。因此快照刷新失败，页面继续读取旧快照。

### 2.1.1 实施前根因核查结论（2026-09-04）

该问题已在实施前完成生产只读核查，结论不是“刷新调度未触发”或“页面缓存未失效”：

- 生产 worker 于 `2026-09-03 21:33:48 +08:00` 启动，健康检查正常，`OOMKilled=false`，没有重启循环。
- worker 从 `21:33:49` 起每 5 分钟准时触发刷新，直至 `23:53:49`；每次均在第一个 `1h` 投影阶段失败，`24h` 与 `7d` 未进入执行。
- 每次失败均为确定性的 `sql: expected 16 destination arguments in Scan, not 19`，不是数据库锁等待、连接池耗尽、SQL 超时或事务提交失败。
- `account_monitor_v4_snapshots` 中 `1h/24h/7d` 的最新 `generated_at` 均停在 `2026-09-03 21:13:00 +08:00`，与失败开始时间一致；页面读取的是该持久化旧快照。
- 代码历史显示提交 `2c88165e5` 增加了三个缓存聚合字段及 19 参数 `rows.Scan`，但遗漏最终 `SELECT` 的三个投影列；生产运行镜像包含该不完整变更。

因此已确认根因链为：**worker 正常触发 -> `1h` SQL 正常返回 16 列 -> Go 按 19 列扫描失败 -> 三窗口刷新提前返回 -> 原子快照替换未执行 -> API 继续返回 `21:13` 的旧快照。**

同一核查还发现独立的 Channel Monitor 列契约错误：`channel_monitor: batch load latest failed`，原因是 `Scan` 接收 7 列而查询返回 6 列。它不是 V4 快照停刷的根因，但必须作为同类风险单独修复和验收。

### 2.2 两套性能口径

Sub 原生 Monitor V2 复用 `account_monitor_results` 主动检测：5 分钟桶内有一次成功即视为该桶 operational，速度使用成功探测 TTFT P50 和平均 latency。它不统计真实业务请求，也不统计缓存命中率。

Monitor V4 复用 `usage_logs`、`ops_error_logs` 和主动探测：每个分组/5 分钟桶有真实逻辑请求时只选择真实请求；无真实请求时最多使用一次有效主动探测；两者都无则该桶不进入请求分母。真实请求按逻辑终态去重，并排除客户端错误、`model_not_found`、unknown usage 和不应归属账号质量的请求。

当前 V4 字段 `latency_p95_ms` 实际是去除两端 5% 后的截尾平均值，不是 P95；该命名误导管理员。

### 2.3 缓存展示

V4 分组缓存率使用：

```text
cache_read / (cache_read + cache_creation)
```

该公式与 Sub 原生缓存命中率一致。当前页面对分组百分比做简单平均，导致低请求量分组显著拉低页面观感。此前线上 24h 快照为 Group 2 `23.20%`、Group 6 `82.26%`、Group 20 `85.95%`，简单平均约 `63.8%`，按请求量加权约 `84.3%`。

### 2.4 切号失败

当前观察窗口内，17 个逻辑请求发生真实 `post_failure_selected` 切号，其中 12 个最终成功、5 个最终失败。失败链主要是连续上游 `502/503/524`；候选池通常仍有 9-19 个候选，但多个候选落在同一故障域。部分决策记录显示 `quality_snapshot_stale=true`，说明切号排序可能使用过期质量证据。

## 3. 目标

1. 在任何实现改动前，完成“快照超过 10 分钟未刷新”的根因核查，形成可复核的日志、代码、运行状态和数据库证据；未完成或证据不足时不得进入实现。
2. 在根因确认后修复 Monitor V4 快照刷新问题，使 1h/24h/7d 快照连续刷新且字段契约一致。
3. 将 V4 的 TTFT/完整耗时指标命名和计算改为真实标准 P95；保留 Sub 原生 Monitor V2 口径不变。
4. 保持真实请求优先、空桶探测补足、客户端/Luna/model_not_found 排除等 T125 语义。
5. 保持 V4 分组缓存率公式与 Sub 原生一致；只修正 V4 自身展示所需的分组聚合，不触碰 Sub 原生控制面板或原生全站缓存汇总。
6. 在质量快照恢复新鲜后，优先跨 channel/BaseURL/上游故障域切号，减少同一故障域连续失败。
7. 提升切号证据可解释性，能够明确区分：无候选、同故障域候选失败、重放不安全、预算耗尽和上游持续故障。

## 4. 非目标与硬边界

- 不修改 Sub 原生控制面板、原生 Monitor V2、原生全站缓存汇总或原生账务统计。
- 不修改 `usage_logs` 的账务含义，不回填历史流水，不新增第二套账务或质量事实源。
- 不把客户端主动拒绝、Luna `model_not_found` 或其他客户端请求错误计入 V4 业务失败分母。
- 不直接通过“超过 10 分钟显示降级”掩盖快照刷新根因；过期提示只有在刷新根因修复并经验证后另行决定。
- 不提高 `unsafe_to_replay`、已输出、已产生 usage、工具副作用或客户端断开请求的切号权限。
- 不修改生图、WebSocket、非 OpenAI 平台或协议强制绑定调度。
- 不使用 GitHub Actions，不改变发布链，不从候选 worktree 构建或部署。

## 5. 方案比较与选择

### 方案 A：只修快照列数

优点是范围最小；缺点是 P95 命名错误、缓存展示失真和切号故障域问题仍存在。仅作为任务 A，不作为完整方案。

### 方案 B：直接恢复 Sub 原生监控口径

优点是实现稳定、容易与原生核对；缺点是页面失去真实业务请求反馈，不能解释用户实际体验。不采用。

### 方案 C：修复 V4 数据链并保留原生诊断基线（采用）

先完成 10 分钟未刷新根因调查并固化证据，再修复已确认的 SQL/Scan 契约，使快照恢复；之后修正 V4 指标计算和缓存展示；最后复用新鲜质量快照做故障域感知切号。Sub 原生监控只作为独立诊断基线，不被 V4 覆盖。该方案与 T125 的真实请求优先语义、T126 的候选池机制兼容，且不建立平行事实源。

## 5.1 已确认根因对应的修复方案

### 第一阶段：修复 V4 列契约

1. 在 `ProjectMonitorV4GroupsForGroups` 的最终 `SELECT` 中补齐并固定三个字段，顺序与 `rows.Scan`、内部投影和单元测试完全一致：`cache_read_tokens`、`cache_creation_tokens`、`cache_hit_denominator`。
2. 增加 SQL 列合同测试，显式断言最终 SELECT 的列名、数量和顺序与 Scan 目标一致；测试不得只断言查询成功。
3. 增加真实 PostgreSQL 定向测试，验证查询可返回 19 列，并覆盖成功真实请求、探测回退和无缓存数据场景。
4. 保持现有刷新事务边界：`1h/24h/7d` 全部投影成功后才替换快照，任一窗口失败继续保留上一个完整快照。

### 第二阶段：修复并隔离同类列漂移

1. 定位 Channel Monitor “Scan 6/7”对应的 repository 查询，按查询实际列顺序修正 `SELECT` 或 Scan，不能通过忽略字段掩盖数据契约变化。
2. 为该查询补充列数量/顺序合同测试，并验证 `/admin/channel-monitors` 与 history API 不再产生该错误。
3. Channel Monitor 修复不得改写 Sub 原生 Monitor V2 的指标、控制面板或全站缓存汇总。

### 第三阶段：验证刷新恢复

1. 仅在前两阶段的定向测试通过后，发布 V4 修复候选。
2. 线上验证 worker 每 5 分钟触发且连续多个周期成功；`1h/24h/7d` 的 `generated_at` 同步前进，日志不再出现 `Scan 16 vs 19`。
3. 确认 API 返回的 `generated_at` 与数据库最新快照一致，排除页面/API 缓存造成的假恢复。
4. 连续刷新验证通过后，才进入 P95、缓存展示和故障域切号的后续任务；不把“显示超过 10 分钟”作为根因修复替代品。

该方案不需要数据库迁移、历史数据回填、快照表重建或修改 Sub 原生全站汇总缓存。

## 6. 端到端数据与控制流

### 6.1 Monitor V4 刷新

调度刷新器 -> `ProjectMonitorV4GroupsForGroups` -> 真实请求/错误逻辑终态去重 -> 5 分钟桶选择真实请求或单次探测补足 -> 计算成功率、TTFT P95、完整耗时 P95、缓存 numerator/denominator -> 原子替换 1h/24h/7d 快照 -> V4 API 读取最新一致快照。

快照替换必须保持现有事务边界：所有窗口生成和写入成功后再提交；任一窗口失败不得发布半套快照。

### 6.2 真实请求过滤

- 使用 `usage_logs` 成功/完整请求作为真实成功候选；使用 `ops_error_logs` 补充可归属账号的上游/服务失败。
- 使用 request_id、logical_request_id、attempt_id 按既有 T125 规则去重。
- 排除 `usage_completeness=unknown`、客户端错误、`error_owner=client`、request/client_request 阶段错误、Luna/model_not_found 和不支持模型错误。
- 真实请求优先占用桶；同桶不再叠加探测。
- 无真实请求时只选择一个探测终态，空桶不进入分母。

### 6.3 调度切号

原生资格/能力/利润/并发/安全门禁 -> 统一质量快照 -> channel/BaseURL/上游故障域过滤与排序 -> Top-K/公平探索 -> 原生并发槽 -> 上游请求 -> 失败分类 -> 冷却和事件记录。

故障域规则只收窄或重排合格候选，不得把硬排除账号重新加入候选。502/503/524/网络超时必须携带故障域信息；在保留安全重放约束下，下一候选优先选择不同故障域。

## 7. 接口与字段契约

### 7.1 V4 快照内部投影

每个分组投影必须包含并保持顺序一致：

```text
group_id
success_rate
request_count
success_count
real_request_count
real_success_count
probe_fallback_bucket_count
probe_fallback_request_count
missing_probe_terminal_count
ttft_p95_ms
ttft_sample_count
latency_p95_ms
latency_sample_count
cache_read_tokens
cache_creation_tokens
cache_hit_denominator
cache_hit_rate
source_updated_at
current_operational
```

Go `rows.Scan`、SQL SELECT、存储投影和 handler JSON 必须由同一测试合同锁定。`cache_hit_denominator` 定义为成功真实请求的 `cache_read + cache_creation` 总和。

### 7.2 V4 API

现有字段保持兼容：`success_rate`、`request_count`、`success_count`、`ttft_p95_ms`、`latency_p95_ms`、`cache_hit_rate`。本任务不新增原生控制面板字段；如需标识刷新状态，只能在 V4 自有 API 内增加可选的 `source_updated_at`/新鲜度信息，且不能伪造数据为实时。

### 7.3 调度事件

复用 `openai_scheduler_logs` 现有 `decision` JSON，补充或规范化非敏感字段：

```text
failure_domain
previous_failure_domain
quality_snapshot_age_seconds
quality_snapshot_stale
switch_allowed
switch_reason
switch_block_reason
candidate_count
eligible_count
runtime_retry_budget
retry_budget_exhausted
unsafe_to_replay
output_started
usage_produced
```

不得记录 API Key、完整请求体、凭据或完整上游响应。

## 8. 指标定义

### 8.1 V4 P95

对每个分组窗口中被选中的成功终态样本分别计算：

```sql
PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY first_token_ms)
PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms)
```

样本为空时返回 null，样本数为 0；不能使用 0、旧值或截尾均值伪装 P95。Sub 原生 Monitor V2 的 P50/平均延迟保持原样。

### 8.2 V4 缓存率

分组缓存率保持：

```text
SUM(cache_read_tokens) / SUM(cache_read_tokens + cache_creation_tokens)
```

仅统计成功真实请求；探测不进入缓存分母。V4 页面现有分组卡片继续展示分组率；如存在 V4 自有跨组摘要，必须使用 numerator/denominator 合并后计算，不得简单平均。该条不改变 Sub 原生控制面板任何汇总。

## 9. 失败、安全与兼容语义

- SQL/Scan 不一致时刷新失败并保留旧快照，但必须记录结构化错误；不得写入半套窗口。
- 快照刷新恢复前不引入新的过期降级行为。
- 质量快照不可用时，调度回退现有静态/中性策略，不因诊断失败阻断正常请求；但决策日志必须标记 stale/unknown。
- 账号能力、利润、冷却、并发、故障域硬门禁优先于质量分和公平探索。
- 已开始语义输出、已知 usage、不可安全重放或具有工具副作用的请求禁止切号。
- 502/503/524 和网络超时可以触发冷却/切号；客户端 404/model_not_found 不进入业务质量失败分母，也不触发账号故障冷却。
- 旧 V4 快照格式读取保持兼容；缺少新字段时不得把未知转换为 0，必要时返回 null 或要求快照重建。
- 无数据库迁移；如只修改已有快照生成逻辑，不进行历史快照回填。

## 10. 场景化验收矩阵

| 场景 | 预期 |
| --- | --- |
| SQL 返回 19 列 | Go Scan 成功，1h/24h/7d 均可写入快照 |
| 刷新连续运行 | 连续多个刷新周期 generated_at/source_updated_at 前进，无 `Scan 16 vs 19` |
| 真实请求桶 | 只统计真实逻辑终态，不叠加探测 |
| 空真实请求桶 | 最多使用一个有效探测；没有探测则不进分母 |
| Luna/model_not_found | 不进入 V4 失败分母，不触发账号故障冷却 |
| P95 | API 数值与直接 `PERCENTILE_CONT(0.95)` 一致；无样本返回 null |
| 缓存 | 分组公式与 Sub 原生一致；V4 跨组摘要按 token 合并，不简单平均 |
| Sub 原生控制面板 | 页面、SQL、数据和汇总完全不变 |
| 单一故障域失败 | 下一候选优先跨故障域，保留硬门禁 |
| 多故障域同时失败 | 记录真实失败链，预算耗尽或无安全重放时明确终止 |
| unsafe_to_replay | 不切号、不增加重试权限 |
| 质量快照 stale | 请求仍按兼容回退策略运行，日志标记 stale，不把陈旧证据伪装新鲜 |
| 单账号/单查询失败 | 不阻断其他分组或整页；错误可定位 |

## 11. 测试策略

### 任务 A：快照刷新根因

- **实施前置门禁已满足：** 已复核刷新调度、SQL 执行、数据库运行状态、Scan 错误、事务未提交、持久化快照和 API 读取链路，并排除“只是页面缓存或读取延迟”。
- 根因证据链和时间窗口见第 2.1.1 节；实现范围仅修复已证实的 V4 最终 SELECT/Scan 列契约，并增加合同测试。
- 同类 Channel Monitor `Scan 6/7` 错误作为独立定向修复纳入本任务的监控完整性验收，不得与 V4 根因混淆。
- 根因修复后必须验证三窗口连续刷新；在验证完成前不得增加“超过 10 分钟降级”逻辑。
- repository SQL contract：SELECT 列清单与 Scan 参数数量/顺序一致。
- Channel Monitor repository SQL contract：查询列清单与 Scan 参数数量/顺序一致。
- 真实 PostgreSQL 集成测试：三窗口刷新、事务原子替换、缓存字段读取。
- service 测试：刷新失败不发布半套快照，刷新成功更新时间前进。
- 运行定向 Go 测试、`go build ./cmd/server`、gofmt 和 `git diff --check`。

### 任务 B：V4 指标口径

- 成功/失败/unknown/client/model_not_found 过滤测试。
- logical request/attempt 去重测试。
- P95 与空样本 null 测试。
- 缓存 numerator/denominator 计算及 V4 跨组加权测试。
- 前端 API contract、卡片显示和刷新错误状态测试；运行定向 Vitest、`pnpm typecheck`、必要生产构建。

### 任务 C：切号故障域

- 502/503/524/网络超时故障域提取和冷却测试。
- 同故障域候选避让、跨故障域优先和候选耗尽测试。
- `unsafe_to_replay`、output_started、usage_produced 和客户端断开安全边界测试。
- 质量快照新鲜/过期/缺失回退测试。
- 调度事件字段、最终成功/失败链和日志脱敏测试。

不运行全仓压力、长时间 soak、无关模块回归或真实上游额度消耗；已有可靠测试证据可引用。

## 12. 发布、线上验证与回滚

每个任务包独立进入发布车道，不合并为一次大发布。候选必须从最新干净 `main` 创建并完成直接相关测试；合并后从干净且与 `origin/main` 一致的根 `main` 推送和发布。主站发布需用户明确授权匹配验收站全局约束，验收站同步按授权执行。

上线专项验证：

1. 健康探针、API/worker/model-detector 健康和运行 source/tree 一致。
2. Monitor V4 三窗口连续刷新至少多个周期，日志无 Scan 错误。
3. 管理接口读取的 P95、缓存和请求数与只读 SQL 同口径。
4. 选取脱敏切号日志核对原账号、目标账号、故障域、最终终态和安全门禁。
5. Sub 原生控制面板做只读不变性核对。

回滚为恢复上一已验证蓝绿镜像/提交；无数据库迁移、无历史回填、无账务回滚。若任务 C 出现异常，先关闭新增故障域自适应策略或回滚任务 C，不回滚已经稳定的快照修复，除非快照修复本身导致服务故障。

## 13. 待决事项

- V4 页面是否在刷新根因修复后仍需要展示显式“数据更新时间”；本规格暂不强制新增过期降级文案。
- 故障域字段如何从不同 channel/BaseURL 规范化，实施前必须基于现有 channel 配置和错误日志确定稳定、非敏感的规范化键。
- 切号恢复率目标暂定为至少一周生产样本中的 `>=80%`，不作为单日发布验收的硬承诺。

## 14. 用户确认记录

2026-09-03：用户确认：先找出并修复快照不刷新的原因；不修改 Sub 原生控制面板及其全站缓存汇总；其余性能口径、V4 修正、故障域切号和日志增强方案可按前述方案推进。
