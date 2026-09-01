# T87 逻辑请求错误生命周期投影设计

日期：2026-09-01
任务：T87 逻辑请求错误生命周期投影
基线：`main@a4580f476fbb61f89ddbcba3a8f5a5429313ec2f`
候选 worktree：`/Users/gongtengxinwen/Documents/sub2api-t87-logical-request-error-lifecycle`
候选分支：`codex/t87-logical-request-terminal-projection`
状态：规格已完成并获用户批准，进入实现候选收口；本文件不授权合并、推送或部署

## 1. 摘要

当前系统已经能够在部分安全场景下对同一用户请求执行同账号重试或跨账号切号，但生产错误记录多数仍按一次 attempt 或一次 `ops_error_logs` 行展示。2026 年 9 月 1 日主站只读排查还发现，用户频繁看到的断流错误与路由/账号错误混在同一监控分母中。这样会产生四类误判：

1. 中间上游 502/503 已被内部重试吸收，却仍被监控计为失败；
2. 多次 attempt 最终只向用户返回一个 503，却在运营视图里被放大为多条失败；
3. 流已输出、usage 已产生或状态已不明时仍被当作可静默重放，无法解释用户看到的 `stream disconnected before completion`、`malformed_stream` 或上游错误。

4. 账号余额耗尽、模型不支持和真实上游断流都可能最终表现为 502/503，但它们的责任归属、是否切号和用户可见性不同。

本任务只增加对既有请求事实的**逻辑请求生命周期读时投影**。监控的第一性原理是“尽可能接近用户体感”：用户最终收到完整成功响应才算成功；中间错误最终恢复成功时不算失败；用户最终看到错误或流终止时只算一次失败；账号级故障留在管理员账号视图，不把账号诊断直接暴露给用户。不得新增第二套错误事实源、账务事实源或运行时平行控制面。

运行时切号/重试由独立的“四次切号”任务承接：普通 OpenAI 请求恢复 Sub 原生逻辑，最大跨账号切换为 4 次；保留 `extra_retry_count` 兼容字段，但不再作为运行时重试控制器。T87 只消费并投影其最终结果，不重复实现这套控制逻辑。

## 2. 现状与证据

### 2.1 原生能力盘点

现有 Sub2API 原生能力可以直接复用：

- `usage_logs.logical_request_id` 关联一次用户逻辑请求；`attempt_id` 关联物理上游 attempt；缺失时可回退 `request_id`。
- `usage_logs.usage_completeness` 区分 `complete`、`partial`、`unknown`，并已有 `actual_cost`、`first_token_ms`、`duration_ms` 等字段。
- `ops_error_logs` 保存脱敏的 `request_id`、`client_request_id`、`status_code`、`error_phase`、`error_owner`、`error_source`、`upstream_errors`、`duration_ms`、`time_to_first_token_ms`、账号/分组等诊断字段。
- OpenAI 网关运行时已有 attempt metadata、`logical_request_id`、`attempt_id`、`unsafe_to_replay`、`switch_allowed`、`switch_reason` 和最终结果记录。
- `OpenAIResilienceEvent` ledger 可补充当前进程内的安全重放和切号判断，但它是 process-local，只能作为诊断辅助，不能作为持久最终事实的唯一来源。
- T66 已定义余额/配额隔离、502/503 短冷却，以及 Responses 只有纯 `response.failed` 且无副作用时允许安全切号。
- T93 已定义 Monitor V4 只按最终用户可见结果计数，自动恢复的中间 attempt 不进入失败分母。

### 2.2 已确认生产行为

2026-09-01 主站只读排查确认：

- 账号“合租巴士-特惠-0.08”存在多次上游 502 后最终恢复的记录，`ops_error_logs.status_code=200`，消息为 `Recovered upstream error 502`；同一账号也存在重试耗尽后最终 503 的记录。
- 账号“xian-plus”存在上游 403、余额不足/`insufficient_user_quota`，本站最终对客状态为 502；账号仍可能是 `active`，但已有 `temp_unschedulable_until`。余额硬错误不能被当作普通网络瞬时错误。
- 当日聚合同时存在 routing 503、upstream 502/503、429 和 API error；它们的故障相位不同，不能只按最终 HTTP 状态合并。
- 生产 `ops_error_logs` 中的 recovered telemetry 使用了 `status_code=200`，因此 `status_code=200` 不能单独证明完整协议成功，也不能把所有错误行直接算失败。

这些证据只用于设计和回归 fixture，不在本任务设计阶段改生产数据、配置、服务或凭据。

## 3. 目标与非目标

### 3.1 目标

1. 以一次逻辑请求为单位聚合所有物理 attempt，并输出唯一的最终生命周期结果。
2. 明确区分中间错误、自动恢复、最终用户可见失败、重试耗尽、不可安全重放停止和证据不足。
3. 让 502/503、403/余额、429、404、网络断开及流协议错误拥有可解释的一致行为矩阵。
4. 让管理员运营接口同时看到最终结果和必要诊断字段，但不把账号、上游细节暴露给普通用户。
5. 让 Monitor V4、错误密度和健康度使用逻辑请求计数，避免 attempt 数放大失败率。
6. 保持 Sub 原生重试、切号、扣费和流式协议语义；本任务只补齐投影和可观测性。
7. 分组视图只回答“用户请求是否成功”；账号视图保留物理 attempt、账号故障、余额/权限和管理员诊断。
8. 不以 `status_code=200`、usage 存在、recovered 文案或“已选账号”作为单独成功证据。

### 3.2 非目标

- 不重写 Sub 原生错误码、扣费、余额、价格、退款或 usage 写入算法。
- 不新增 `ops_error_events`、`request_lifecycle` 等平行错误事实表。
- 不把所有 4xx/5xx 统一自动切号，也不把所有错误都改成用户不可见。
- 不对已输出语义内容、已产生 usage/扣费或状态不明的请求静默重放。
- 不自动把 `gpt-5.6-luna` 降级为其他模型；模型准入属于 T86/T91 相关范围，本规格只定义其错误如何投影。
- 不回填或重写历史 usage/error 行，不通过模糊的 `client_request_id` 猜测跨请求关联。
- 不改变 T83 主动探测空桶准入和 T93 的 5 分钟桶源选择。
- 不把健康度扩展成账号故障、模型覆盖率、探测完整性等复合评分；分组可用性只由用户逻辑请求最终成功/失败决定，P95 首字和耗时作为性能指标单独展示。

### 3.3 任务包拆分与依赖

本规格关联多个现象，但不把它们合并成一个实现 worktree：

| 任务包 | 负责内容 | 不负责内容 |
|---|---|---|
| 四次切号候选 | 普通 OpenAI Responses、Chat Completions、Messages、Embeddings 恢复 Sub 原生重试/切号；最大跨账号切换 4 次；保留字段兼容但不让 `extra_retry_count` 控制运行时 | Monitor V4 查询、用户可见终态投影、余额/模型配置事实源 |
| T87 候选 | 复用 usage/ops 事实，按逻辑请求聚合最终终态；修正 Monitor V4 分组成功/失败分母；提供管理员诊断字段 | 运行时切号策略、余额算法、模型准入、生产账号状态写入 |
| 后续关联任务 | 余额耗尽临时不可调用、Luna 模型准入/配置一致性、终态事件写出缺陷 | 不得以监控投影掩盖运行时错误 |

四次切号候选和 T87 候选可以并行处于实现/复核阶段，但必须分别验收；只有发布总控可以串行合并、推送和部署。

## 4. 方案比较与选择

### 方案 A：新增独立逻辑请求错误事实表

每次请求结束时写入一行逻辑请求终态，另建 attempt 事件表保存中间错误。

- 优点：查询简单，关联稳定，历史生命周期完整。
- 缺点：新增事实源和迁移；必须处理运行时写入失败、双写一致性、回滚和历史回填；与 Sub 原生 usage/ops 语义重复。
- 结论：不采用，违反当前任务包“不新增错误事实源”的边界。

### 方案 B：只按 `ops_error_logs` 的单行状态统计

把 `status_code`、`resolved`、`retry_count` 等既有字段直接映射为成功/失败。

- 优点：改动最小。
- 缺点：无法可靠识别跨 attempt 的唯一逻辑请求；无法区分 recovered telemetry 与最终用户错误；无法处理流已输出、usage 已产生和状态不明；会继续放大失败率。
- 结论：不采用，无法满足用户已确认的最终终态口径。

### 方案 C：复用既有 `usage_logs` 与 `ops_error_logs`，做读时逻辑请求投影

以 `usage_logs.logical_request_id` 优先、缺失时 `request_id` 聚合 attempt；用 `usage_logs` 作为 attempt/usage 的主证据，用 `ops_error_logs` 作为错误可见性和诊断证据；运行时已有 metadata 仅在可精确关联时补充安全重放判断。

- 优点：不引入第二事实源；与 T66/T93、T49 和现有管理员 ops 入口兼容；可以对历史记录保守返回 `unknown`，不伪造成功。
- 缺点：旧记录字段缺失时无法恢复完整生命周期；需要明确关联优先级和不确定性处理；查询和分页需要避免重复扫描。
- 结论：采用。通过严格证据优先级解决“数据不全时宁可未知，不猜成功”的问题。

## 5. 核心定义

### 5.1 逻辑请求与物理 attempt

- **逻辑请求**：一次客户端意图，从入口接收开始，到向客户端写出完整协议终态、用户可见错误、客户端断开或服务端无法继续为止。
- **物理 attempt**：一次实际账号/上游调用。一次逻辑请求可以包含同账号重试、跨账号切号或不同上游尝试。
- `logical_request_id` 是首选聚合键。
- 当 `logical_request_id` 缺失时，使用 `request_id` 作为兼容聚合键，并在结果中标记 `correlation_quality=legacy_request_id`。
- `client_request_id` 只作为辅助检索和展示字段，不能单独把两条记录合并为同一逻辑请求；多个请求可能复用相同客户端 ID。
- attempt 去重优先使用 `attempt_id`；由于 `attempt_id` 不承诺数据库全局唯一，实际去重键为同一 API key/请求上下文内的 `attempt_id`，缺失时回退到 `request_id`。

### 5.2 事实源优先级

1. 最终协议和错误事实：运行时已持久化的最终 `ops_error_logs`/usage 结果及其状态字段。
2. attempt、usage 和计费事实：`usage_logs`。
3. 中间上游错误明细：`ops_error_logs.upstream_errors` 及脱敏错误字段。
4. 安全重放诊断：已持久化的 attempt metadata；只有在与同一逻辑请求/attempt 精确关联时才使用。
5. 进程内 `OpenAIResilienceEvent`：仅作为实时诊断补充，进程重启或实例切换后缺失时不得阻止保守投影。

`status_code=200`、`resolved=true`、存在 `upstream_errors` 或存在 usage 任意一个字段都不能单独决定最终生命周期；投影必须综合同一逻辑请求的所有证据。

### 5.3 终态分类

`terminal_kind` 使用以下枚举：

```text
success
auto_retry_recovered
single_attempt_user_visible
retry_exhausted_user_visible
stopped_unsafe_to_replay
incomplete_unknown
```

其中：

- `success`：无可见错误，且存在完整协议终态的可靠证据。历史读时投影至少要求最终状态为 2xx、没有最终错误帧/错误行，并且 usage 为 `complete` 或存在同等可靠的完整终态证据。
- `auto_retry_recovered`：最终满足 `success`，且同一逻辑请求存在一个或多个中间可恢复错误 attempt；这是成功请求的细分标签，不是失败。
- `single_attempt_user_visible`：只有一个可确认 attempt，最终向用户返回 HTTP 错误或 200 流内错误终止。
- `retry_exhausted_user_visible`：存在两个或以上可确认 attempt，最终仍向用户返回错误或流内失败终止，且停止原因为重试/切号预算耗尽、候选耗尽或临时不可调用账号耗尽。
- `stopped_unsafe_to_replay`：系统判断不能安全重放，或已产生语义输出、usage/扣费、副作用、客户端状态不明等导致切号/重试被禁止；若用户收到终止错误，`user_visible=true` 且计失败。
- `incomplete_unknown`：持久证据不足以证明完整成功或最终用户可见失败，例如历史记录只剩中间错误、客户端先断开且没有服务端终态。不得计成功；在健康度报表中进入未决/证据不足单独分栏，不伪装成普通成功或失败。

### 5.4 最终结果优先级

为了防止一条中间错误覆盖最终结果，投影按以下顺序寻找最终事实：

1. 完整协议成功终态或最终用户可见错误终态；
2. 已写入的流内终止事件或已记录的客户端可见错误；
3. 已确认的重试/切号停止原因；
4. 中间错误集合；
5. 仍无法确认时返回 `incomplete_unknown`。

分类优先级为：

```text
stopped_unsafe_to_replay
  > auto_retry_recovered
  > retry_exhausted_user_visible
  > single_attempt_user_visible
  > incomplete_unknown
```

`stopped_unsafe_to_replay` 只有在确有 unsafe/禁止切换证据时优先于最终成功；不能仅因为“发生过一次错误”就归入该类。`auto_retry_recovered` 只有在最终成功证据完整时成立，不能仅凭 `Recovered upstream error 502` 文案成立。

## 6. 端到端数据流

```text
usage_logs                         ops_error_logs
  logical_request_id                 request_id/client_request_id
  attempt_id                         final status/error phase
  usage_completeness                 upstream error events
  actual_cost/duration               user/account/group diagnostics
          \                         /
           \                       /
            exact correlation + evidence ranking
                              ↓
                 logical request lifecycle projection
                              ↓
      admin ops API / Monitor V4 aggregation / daily diagnostics
```

### 6.1 读时聚合步骤

1. 按时间窗先筛选候选 `usage_logs` 和 `ops_error_logs`，避免对全表做无界关联。
2. 对每条记录计算 `correlation_key`：优先 `logical_request_id`，否则 `request_id`。
3. 在同一逻辑键内建立 attempt 集合，按 `(api_key_id, attempt_id)` 去重，缺失时按 request ID 去重。
4. 将 `ops_error_logs` 通过精确 `request_id` 关联到 attempt；`client_request_id` 仅用于辅助过滤，不能作为唯一 join key。
5. 识别中间错误、最终结果、usage 完整性、输出是否开始、unsafe/switch 判断和客户端可见性。
6. 依据第 5 节规则生成一行逻辑请求结果。
7. 在分页前按逻辑请求排序和去重；同一逻辑请求不得因多条 attempt 在管理员列表中出现多次。

### 6.2 证据不足处理

- 没有 `logical_request_id` 时使用 `request_id`，但必须返回兼容标记。
- `ops_error_logs` 有错误而找不到对应 usage 时，不创建虚拟 usage，也不推断已扣费。
- 只有中间 recovered 行、没有最终成功/完整 usage 证据时返回 `incomplete_unknown`，不返回 `auto_retry_recovered`。
- `usage_completeness=unknown` 不代表成功；它可以证明存在一次不完整 attempt，但不能单独证明用户是否看到终态。
- 老记录不回填新字段，不通过时间邻近、账号名称或邮箱把不同请求合并。

## 7. 行为与用户可见性矩阵

| 错误/终止类型 | 默认重试/切号 | 前提 | 中间错误是否对用户隐藏 | 最终失败是否可见/计失败 |
|---|---|---|---|---|
| 上游 502/503 | 可以有限重试或切换账号 | 未开始语义输出、无 usage/副作用、预算未耗尽 | 最终恢复时隐藏 | 耗尽时可见并计失败 |
| 网络断开、连接失败 | 可以有限重试或切换账号 | 同上；必须可安全重放 | 最终恢复时隐藏 | 否则可见并计失败 |
| `malformed_stream` | 仅在流未产生语义输出时重试 | 无 usage/副作用且安全重放 | 最终恢复时隐藏 | 流已开始或重试失败时可见并计失败 |
| 缺少 `response.completed` | 仅在纯失败终态、无输出/usage 时按 Responses 合同处理 | 只能是合法 `response.failed` 安全重放路径 | 最终恢复时隐藏 | 否则发送合法失败终态并计失败 |
| `error decoding response body` | 按网络/上游瞬时错误处理 | 同上 | 最终恢复时隐藏 | 最终失败可见并计失败 |
| `upstream stalled` / recycled connection | 可以有限重试 | 未输出语义内容且安全重放 | 最终恢复时隐藏 | 已输出或耗尽时可见并计失败 |
| 上游 401/403 余额/配额不足 | 不按普通网络错误重试；隔离账号并尝试其他合资格账号仅在整体请求仍安全时 | 明确 `insufficient_user_quota`/余额硬错误 | 中间账号错误可隐藏 | 无安全恢复时可见并计失败 |
| 上游 401/403 凭据/权限失效 | 账号隔离；不对同一坏账号重复尝试 | 仅按账号故障路径切换 | 最终恢复时隐藏 | 最终失败可见并计失败 |
| HTTP 429 | 可按 T105 同账号有限重试；跨账号必须安全重放 | 遵守 Retry-After、预算和安全边界 | 最终恢复时隐藏 | 最终失败可见并计失败 |
| HTTP 404 模型/请求确定性错误 | 不切号 | 入口/请求配置错误 | 不隐藏确定性错误 | 立即可见并计失败 |
| 本站 routing 503 | 不把它伪装成上游 503；仅在确有可选候选和安全重放时继续调度 | 由原生调度/准入状态决定 | 中间调度失败可不展示 | 无候选或策略停止时可见并计失败 |
| 已输出后上游失败 | 禁止拼接第二段流；禁止静默切号 | 已有语义输出或副作用 | 不隐藏终止错误 | 流内失败可见并计失败 |
| 客户端已断开且服务端无终态 | 不重放 | 状态不明 | 不向已断开客户端发送 | `incomplete_unknown`，不计成功，单列证据不足 |

“错误用户不可见”只适用于中间 attempt 最终恢复成功的情况。任何最终错误，包括最终 HTTP 502/503、流内 error 帧、合法 `response.failed` 终止或重试耗尽，都不能通过监控投影改写为成功。

## 8. 流式和 Responses 协议语义

### 8.1 流未开始

- 尚未向客户端写出协议头或语义事件时，可以返回既有 HTTP 错误响应。
- 若满足 T66 的安全重放条件，可以在同一逻辑请求内继续同账号重试或切号。
- 中间 attempt 不进入用户可见错误计数；最终失败必须生成一条逻辑请求失败结果。

### 8.2 流已开始但尚未产生语义输出

- 已写出协议头不等于已产生语义输出；是否允许切号以 `OutputStarted`、output item、usage、副作用和 `unsafe_to_replay` 为准。
- Responses 只有收到纯 `response.failed` 且无 usage/扣费/语义输出时，才允许安全切号。
- 不得因缺少 `response.completed` 而直接拼接第二段流；必须先证明客户端尚未收到不可重放内容。

### 8.3 已产生语义输出或 usage

- 禁止切号、禁止从第二个账号拼接内容、禁止把 HTTP 200 改写为新的 HTTP 400/503。
- 必须使用该协议允许的单个流内失败/恢复终止事件；Responses 使用合法的 `response.failed` 等终止语义，避免客户端看到 `stream closed before response.completed` 但服务端没有终态记录。
- 投影将其归为 `stopped_unsafe_to_replay` 或最终用户可见失败；Monitor V4 计一个失败逻辑请求，不计多个 attempt。

### 8.4 流错误示例归类

以下错误均不是“成功请求”的证据：

- `stream disconnected before completion: The upstream connection failed. Please retry later.`
- `stream disconnected before completion: stream closed before response.completed`
- `stream disconnected before completion Transport error network error error decoding response body`
- `stream disconnected before completion: codex upstream stalled: no real data for SmOs. connection recvcled`

如果它们发生在首个语义输出前且未产生 usage，系统可以安全重试；如果最终仍出现、发生在已输出后或状态不明，则必须落为用户可见失败或 `incomplete_unknown`，不能被 `status_code=200` 单独覆盖。

## 9. 监控与管理员接口契约

### 9.1 逻辑请求结果字段

现有管理员 ops 入口新增读模型字段，字段名最终遵循代码现有 DTO 命名，但语义必须保持如下：

```text
logical_request_id
correlation_quality
attempt_count
failover_count
upstream_error_count
final_status
final_protocol
terminal_kind
terminal_reason
user_visible
auto_retry_recovered
retry_exhausted
stopped_unsafe_to_replay
unsafe_to_replay
switch_allowed
switch_reason
error_phase
error_owner
error_source
final_error_code
final_error_message_sanitized
first_attempt_at
completed_at
duration_ms
time_to_first_token_ms
usage_completeness
usage_present
```

约束：

- `attempt_count`、`failover_count` 和 `upstream_error_count` 是诊断字段，不能直接作为用户失败数。
- `user_visible=true` 只表示最终用户确实收到错误，或在已开始流中收到错误终止；中间恢复行不能设置该值。
- `auto_retry_recovered=true` 必须同时满足最终完整成功证据和中间错误证据。
- `stopped_unsafe_to_replay=true` 必须有 `unsafe_to_replay=true`、`switch_allowed=false`、已输出/usage/副作用或等价停止证据之一；不能因普通 404 自动设置。
- 敏感上游响应、Authorization、API key、完整请求体和完整模型输出不进入普通用户接口或规格化响应。

### 9.2 普通用户与管理员的边界

- 普通用户继续收到既有脱敏错误协议和必要的重试提示，不返回账号名称、账号 ID、分组内部状态或上游完整错误。
- 管理员 ops 视图可以看到账号名称、账号 ID、分组、上游状态、错误相位、安全重放判断和 attempt 数，但仍只显示已脱敏字段。
- 用户错误接口不读取新的诊断投影来“美化”为成功；投影只服务监控和管理员诊断。

### 9.3 计数规则

对每个时间窗：

```text
logical_requests_total       = distinct logical request projection rows
user_visible_failures_total  = rows where user_visible = true
successful_requests_total    = rows with terminal_kind in (success, auto_retry_recovered)
auto_recovered_total         = rows where auto_retry_recovered = true
retry_exhausted_total        = rows where terminal_kind = retry_exhausted_user_visible
unsafe_stop_total            = rows where terminal_kind = stopped_unsafe_to_replay
incomplete_unknown_total     = rows where terminal_kind = incomplete_unknown
```

Monitor V4 继续遵守 T93 的成功样本资格、`usage_completeness`、真实请求优先和主动探测空桶规则；T87 不改变 T93 的桶源，也不把 `incomplete_unknown` 伪造成成功。

## 10. 兼容性与迁移

- 首选方案不新增数据库表和迁移；复用现有 `usage_logs`、`ops_error_logs`、原生 ops repository/service/handler/页面。
- 允许在现有查询、服务 DTO 和日志投影中增加计算字段，但不得建立第二套持久错误账本。
- 旧 `ops_error_logs` 没有独立 `logical_request_id`/`attempt_id` 时，通过现有 `request_id` 与 `usage_logs` 精确关联；无法精确关联则返回 `correlation_quality=unknown`。
- 历史 `resolved=true` 或 `status_code<400` 只作为辅助证据。没有匹配最终完整成功 usage/终态时，不得自动标记 `auto_retry_recovered`。
- 不删除、不更新、不重写历史 usage/error 数据，不执行历史回填。
- 新字段缺失时使用空值、`false` 或 `unknown` 的保守表达，不能用 0 代替未知耗时，也不能把未知错误归为永久账号故障。

## 11. 测试与验收矩阵

### 11.1 聚合与终态

| 场景 | 输入 | 预期 |
|---|---|---|
| 单次完整成功 | 一个 attempt，最终 2xx，usage complete | `success`，一次成功 |
| 502 后恢复 | attempt A 502，attempt B 完整成功 | `auto_retry_recovered`，用户失败 0，逻辑成功 1 |
| 502/503 耗尽 | 多个失败 attempt，最终 503 | `retry_exhausted_user_visible`，用户失败 1 |
| 单次 404 | 一个确定性 404 | `single_attempt_user_visible`，不切号 |
| unsafe stop | 已输出或 usage 后 upstream 失败 | `stopped_unsafe_to_replay`，用户失败 1 |
| unknown usage | 只有 partial/unknown，未找到最终终态 | 不得成功；按证据返回失败或 `incomplete_unknown` |
| 重复记录 | 同一 API key/attempt 重复两条 usage | 只计一个物理 attempt |
| 缺失逻辑 ID | 多条记录只有 request_id | 使用 request_id，并标记 legacy correlation |
| 相同 client ID | 不同 request_id 复用 client_request_id | 不得合并 |

### 11.2 错误矩阵

至少覆盖：403 余额不足、403 权限、429 同号重试、429 跨号安全重放、502、503、网络断开、`malformed_stream`、缺失 `response.completed`、error decoding body、upstream stalled/recycled、routing 503、404 模型错误。

每个场景验证：

- 是否允许同账号重试；
- 是否允许跨账号切号；
- 是否把中间错误隐藏；
- 最终用户是否收到错误；
- 是否计一个逻辑请求失败；
- 是否记录账号隔离/短冷却和停止原因；
- 是否避免重复 usage/扣费。

### 11.3 API 与监控

- 管理员 ops API 对同一逻辑请求只返回一行。
- recovered `status_code=200` 行显示为中间恢复事件，不进入用户失败计数。
- 最终 503 和流内失败显示为用户可见失败，即使同一逻辑请求之前已经有 502/503 attempt。
- `attempt_count` 增加不会线性增加失败分母。
- T93 Monitor V4 的真实请求优先、空桶主动探测和 success/TTFT/P95 规则不回归。
- 普通用户接口不包含诊断字段和敏感上游信息。

## 12. 实施边界与候选文件

候选实现只限于以下原生边界，最终文件名按代码实际结构调整：

- `upstream/sub2api/backend/internal/service/ops_errors.go` 及其测试；
- ops-error repository、查询和测试；
- `ops_error_logger.go` 或现有错误投影 helper；
- 管理员 ops API DTO、handler、页面和对应前端测试；
- 仅为直接相关的 usage/ops correlation 测试 fixture。

不得顺带修改账号调度策略、模型目录、余额算法、支付、主站配置或其他任务包文件。T89 的 routing 503 原因细分和 T86 的模型准入改动分别保持在自己的任务边界内；T87 只消费它们最终可用的错误字段。

## 13. 发布、验证与回滚条件

本规格不授权生产变更。进入实现后：

1. 先在候选 worktree 写失败测试，再实现读时投影。
2. 运行直接相关 Go 测试、管理员 API 测试、必要的服务构建和 `git diff --check`。
3. 功能实现和直接相关测试通过后，只能汇报 `READY_FOR_ROOT_REVIEW`，不得自行合并、推送或部署。
4. 根总控决定是否合并；所有部署必须从干净、已推送且与 `origin/main` 一致的根 `main` 发起。
5. 主站只有在用户明确说“测试站验收通过，部署主站”或“快速部署到主站”时才能发布；若部署成功，必须用同一 commit 同步/核对验收站。
6. 回滚使用上一已验证版本，不删除或回写历史 usage/error 记录；读时投影关闭或恢复旧版本后，原生请求和账务语义应保持不变。

## 14. 自审结论与批准记录

规格自审清单：

- 未使用 `TBD`、`TODO` 或未决实现占位。
- 已明确区分中间恢复事件与最终用户可见错误。
- 已覆盖 usage、语义输出、流协议、状态不明和客户端断开边界。
- 已明确 403/余额、429、502/503、404、网络、流格式错误的行为。
- 已与 T66 的安全切号边界、T93 的最终用户成功率和 T49 的 unknown usage 排除规则保持一致。
- 未新增错误事实源，未扩大到 T86/T89 的实现范围。

批准记录：用户于 2026-09-01 明确确认“完成规格书”。本书面规格现视为已批准，可进入既有候选的实施收口和直接相关验证；该确认不等同于合并、推送、验收站发布或主站部署授权。

## 15. 本轮结论

1. 用户频繁看到的四类 `stream disconnected before completion` 错误，不能按成功请求处理；它们必须由运行时记录最终协议终态，并由 T87 按逻辑请求只计一次。
2. 余额不足/403、模型不支持/Luna 路由拒绝、真实上游 502/503、429 和网络断流必须分开保留责任相位；T87 不通过统一文案或监控投影掩盖根因。
3. 普通 OpenAI 请求的四次切号由独立候选恢复 Sub 原生预算；T87 只投影最终用户结果。`extra_retry_count` 继续兼容保留，但不控制运行时。
4. 分组健康度以用户最终请求成功为唯一可用性口径；账号故障、余额、权限、attempt 数和切号细节只在账号/管理员视图展示。P95 首字和耗时是独立性能指标，不构成“降级成功”。
5. 未确认的客户端断开或缺少完整终态，必须落为 `incomplete_unknown`，不得计成功；是否进入失败分母遵循本规格的证据不足单列规则。
