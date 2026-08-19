# T32 账号评分回归修复规格

## 状态与批准

- 任务包：T32 账号评分回归修复
- 基线：`main@dc51b37c9dbf73a87cccceab5815f129882812c5`
- 状态：DESIGNING -> IMPLEMENTING（规格自审完成）
- 批准记录：用户在 T32 任务指令中明确批准以下产品合同；该合同同时作为本规格的代审批准依据。不得缩小或改变。

## 问题证据与当前行为

原生事实源是 `account_monitor_results` 主动探测结果。`AccountMonitorService.ListWindow` 读取主动探测聚合、最近探测和时间线；`accountMonitorWindowEvidence` 只在有主动探测样本且未过期时产生评分证据；`CalculateAccountMonitorWindowQualityScore` 使用现有成本、成功率、TTFT 和延迟权重，不读取真实请求指标作为评分输入。

当前回归的投影根因有两处：

1. `accountMonitorAccountPaused` 将状态非 active、`schedulable=false`、临时不可调度、限流和过载统一标记为 paused；`accountMonitorAvailabilityStatus` 直接把 paused 变成 `disabled`，`projectAccountMonitorProbe` 清除 `QualityScore` 和 `GroupRank`，所以暂停账号即使有新鲜成功探测也没有分数。
2. `listPool` 只返回 `status=active && schedulable=true`。调度关闭但主动探测成功的账号不会继续探测；同时没有以“最近主动探测明确 4xx/5xx 且调度关闭”作为停止条件，导致探测和排名语义错误地绑定到了调度开关。

## 目标

- 评分、当前状态和排名只依赖 Sub 原生账号主动探测结果。
- 暂停账号仍可参与评分和排名，只要存在新鲜主动探测证据且未命中唯一停止门控。
- 仅当最近主动探测返回 HTTP 4xx 或 5xx 且账号调度关闭时，停止继续探测并退出排名。
- 调度关闭但主动探测成功的账号继续探测、显示评分并参与排名。
- 无主动探测证据、探测过期或探测失败按现有证据语义保持无分数/无排名；不填默认分数。

## 非目标

- 不改评分公式、权重、计费、成本、调度器决策、数据库 schema/事实源、API 路由或其他页面。
- 不把真实请求指标、usage logs 或错误日志转为评分证据。
- 不回填历史探测、不改变账号状态字段、不自动重新打开调度。

## 方案比较

| 方案 | 做法 | 取舍 |
| --- | --- | --- |
| A | 前端对空分数填默认值 | 掩盖无证据，破坏评分真实性；不采用 |
| B | 只移除 paused 投影的 disabled 分支 | 暂停可排名，但关闭调度账号仍不会探测；不完整 |
| C（选择） | runner 以最近主动探测 4xx/5xx + 调度关闭作为唯一停止门控；投影按主动探测证据评分 | 改动集中在原生 service，保留公式和事实源，完整覆盖合同 |

## 端到端数据与控制流

1. `RunAll`/`RunOne` 读取原生账号列表和最近探测快照。
2. 对每个账号，若调度关闭且最近探测的 `HTTPStatus` 在 400..599，则跳过物理探测；否则执行原生 `ProbeAccountConnection` 并持久化结果。
3. `List`/`ListWindow` 读取主动探测聚合、最近结果和时间线。暂停只影响管理/健康展示桶，不再直接清空评分资格。
4. 新鲜主动探测样本生成 `monitor_probe` 证据，继续调用既有评分公式。最近成功的关闭调度账号可得 normal/eligible；暂停账号也可得评分和排名。
5. 无样本或过期证据保持 `stale`、无 `QualityScore`/`GroupRank`。失败探测保持既有异常/封顶语义；只有“4xx/5xx + 调度关闭”才进入 unavailable/停止/无排名。
6. 排名仅对 `ScoreStatus` 为 eligible/capped 且成本资格满足的行生成，排序和 tie-break 规则不变。

## 字段与接口契约

- 不新增字段或迁移。
- `AvailabilityStatus` 仍为 `normal|abnormal|unavailable|disabled|stale`；`disabled` 只保留给没有评分证据的管理展示或既有兼容场景，不再由“暂停”单独抹掉有证据分数。
- `QualityScore`、`GroupRank` 为空表示没有可用主动探测评分证据或命中停止门控，不表示用默认值。
- runner 的内部停止判断使用现有 `AccountMonitorLatest.HTTPStatus` 与账号调度字段，不引入第二事实源。

## 失败与兼容语义

- 探测连接错误、超时、空流和非 HTTP 错误不会触发停止门控；按既有失败/过期证据语义投影。
- HTTP 4xx/5xx 对调度仍开启的账号不触发停止门控，继续探测；其评分状态遵循主动探测失败语义。
- HTTP 4xx/5xx 对调度关闭的账号停止物理探测并从排名中排除，但保留最近结果供管理员诊断。
- `ListLatest` 失败、探测聚合失败或仓储错误继续向调用方返回原生错误，不生成合成分数。

## 验收矩阵

| 场景 | 继续探测 | 评分 | 排名 |
| --- | --- | --- | --- |
| 暂停 + 新鲜成功探测 | 是 | 有 | 有 |
| `paused/schedulable=false` + 新鲜成功探测 | 是 | 当前状态正常，有 | 有 |
| `paused/schedulable=false` + 最近 4xx/5xx | 否 | 当前状态不可用/异常，无 | 无 |
| `paused/schedulable=false` + 无探测记录 | 是（待探测） | 无 | 无 |
| `schedulable=true` + 最近 4xx/5xx | 是 | 按既有原生错误/调度语义 | 不用真实请求替代探测 |
| 调度关闭 + 新鲜成功探测 | 是 | 有 | 有 |
| 调度关闭 + 最近 4xx/5xx | 否 | 无 | 无 |
| 无探测证据 | 是（未命中停止门控） | 无 | 无 |
| 探测过期 | 是（未命中停止门控） | 无 | 无 |
| 探测失败/超时且无 4xx/5xx | 是 | 既有异常/过期语义 | 不新增默认分数 |

## 测试策略

- 后端 service focused TDD：增加停止门控纯函数、runner 物理调用次数、List/ListWindow 投影和全局/分组排名回归。
- 覆盖暂停账号、关闭调度成功、关闭调度 4xx/5xx、无证据、过期/失败，以及排名 tie-break。
- 运行 `go test` 账号监控 service 包与直接相关 repository/handler 包；执行 `gofmt`、`go vet`（受影响包）和 `git diff --check`。
- 不运行全仓压力、浏览器、生产或部署验收；这些由根发布总控负责。

## 发布、回滚与未验证项

- 无迁移、无配置变化，预期 `downtime_required=false`；最终以根发布预检为准。
- 回滚为恢复候选提交前的 `main` SHA；不需要数据回滚。
- 本候选不合并、推送、部署或修改全局队列/总账/生产证据。
- 未验证项：生产登录态账号投影、真实宿主探测调用次数、根发布链和线上排名，均留给 READY_FOR_ROOT_REVIEW 后的根任务。

## 规格自审

- 范围仅涉及 `account_monitor_service.go`、其直接测试和必要 handoff/spec/plan 文档。
- 评分公式、权重、成本、调度器和数据库事实源未改变。
- 每个评分结论均要求主动探测证据；没有默认填充路径。
- 停止条件同时要求 HTTP 4xx/5xx 与调度关闭，且与“暂停可排名”相容。
