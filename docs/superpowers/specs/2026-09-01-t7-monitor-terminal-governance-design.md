# Spec: T7 监控口径与逻辑请求终态治理

日期：2026-09-01（Asia/Shanghai）
状态：设计已获用户确认，待实施计划

## 1. 问题与证据

当前系统已经具备 `logical_request_id`、`attempt_id`、`usage_completeness`、`ops_error_logs`、主动探测结果和 Monitor V4 快照，但这些事实在不同页面和统计路径中仍可能以不同粒度解释。

主要问题：

- 同一用户逻辑请求的多个上游 attempt 可能放大失败数量。
- 中间 502/503、429 或网络错误已被切号/重试吸收时，仍可能污染监控失败率。
- `pool=25` 只表示原始账号池，但容易被解释为 25 个可用账号。
- 没有真实请求时，主动探测应提供一个桶级样本；探测终态缺失时，当前链路可能把完整性缺口误写成服务失败。
- `usage_logs`、`ops_error_logs`、探测终态和快照缺少统一的最终终态优先级。

## 2. 第一性原理与目标

监控只回答一个核心问题：

> 用户的请求是否最终成功完成？

因此必须分离：

```text
用户逻辑请求终态 -> 决定可用性和成功率
账号/上游 attempt -> 解释重试、切号和故障来源
主动探测终态     -> 在无真实请求时提供观测样本
```

目标：

1. 每个逻辑请求只生成一个最终监控结果。
2. 中间 attempt、最终用户可见结果和证据不足明确区分。
3. Monitor V4、管理员诊断和错误生命周期使用一致的终态语义。
4. 让账号池诊断能区分原始池、有效候选和实际请求尝试。
5. 让探测终态缺失成为独立的数据完整性告警，不伪装成服务失败。
6. 保持 T87 已确认的安全重放、流式协议和 Sub 原生账号槽位语义不变。

## 3. 非目标

- 不新增逻辑请求、错误或账务事实表。
- 不改变 Sub 原生重试、切号、扣费、账号状态机或并发槽位。
- 不恢复 admission、slow-session 或其他已废弃的自定义并发控制。
- 不把所有上游 4xx/5xx 统一自动切号。
- 不改变 T87 的安全重放边界：已输出语义内容、已产生 usage/副作用或状态不明时禁止静默重放。
- 不新增模型级额度概念，不引入其他上游平台扩展。
- 不回填或重写历史 usage/error 事实。

## 4. 方案比较

### 方案 A：只修改 Monitor V4 查询

改动最小，但管理员错误详情、正常流水和监控仍可能使用不同口径，无法解决逻辑请求被多次计数的问题。不采用。

### 方案 B：复用现有事实做统一逻辑请求终态读时投影

使用 `usage_logs`、`ops_error_logs`、`logical_request_id`、`attempt_id` 和探测终态，按严格证据优先级生成唯一逻辑请求结果，再供 Monitor V4 和管理员诊断使用。无需新事实表，兼容现有 Sub 原生链路。采用。

### 方案 C：新增逻辑请求终态事实表

虽然查询简单，但会引入新的写入事实源、双写一致性、迁移、失败补偿和历史回填问题。违反原生 Sub 优先和读时投影边界。不采用。

## 5. 逻辑请求终态

### 5.1 关联规则

1. 优先使用 `logical_request_id`。
2. 缺失时使用 `request_id`，并标记 `correlation_quality=legacy_request_id`。
3. `client_request_id` 只用于辅助检索，不能单独作为合并键。
4. attempt 优先用 `(api_key_id, attempt_id)` 去重，缺失时回退到请求 ID。
5. 无法精确关联时返回 `correlation_quality=unknown`，不得通过时间、账号名称或邮箱猜测合并。

### 5.2 证据优先级

```text
完整协议成功/最终用户可见错误
    > 流内终止或客户端可见错误
    > 已确认的重试/切号停止原因
    > 中间上游错误集合
    > incomplete_unknown
```

`status_code=200`、`resolved=true`、存在 usage 或存在 upstream error 均不能单独决定成功；必须综合同一逻辑请求的终态证据。

### 5.3 终态分类

```text
success
auto_retry_recovered
single_attempt_user_visible
retry_exhausted_user_visible
stopped_unsafe_to_replay
incomplete_unknown
```

- `success`：有可靠完整协议成功证据。
- `auto_retry_recovered`：最终完整成功，且存在中间可恢复错误；计成功，不计失败。
- `single_attempt_user_visible`：单次 attempt 最终向用户返回错误。
- `retry_exhausted_user_visible`：多次 attempt 后最终仍向用户返回错误。
- `stopped_unsafe_to_replay`：因已输出、usage/副作用、状态不明或安全策略禁止重放而停止；用户看到错误时计失败。
- `incomplete_unknown`：证据不足，不能证明完整成功或最终用户可见失败；不计成功，单独列为证据不足。

## 6. Monitor V4 统一计分

```text
successful_requests_total
  = terminal_kind ∈ {success, auto_retry_recovered}

user_visible_failures_total
  = user_visible = true

logical_requests_total
  = distinct logical request projection rows
```

真实请求和主动探测继续遵守现有桶源选择：

- 同桶有真实请求：只使用真实逻辑请求，排除探测。
- 同桶无真实请求：使用一个组级主动探测终态。
- 同轮跨账号探测先聚合，成功为 `1/1`，全部明确失败为 `0/1`。
- 中间切号 attempt 不扩大分母。

明确模型不支持、客户端责任错误和 `usage_completeness=unknown` 不计本站服务失败；最终用户可见的本站/上游错误计一次失败。

### 6.1 探测终态缺失

探测终态分为：

```text
success
failed
missing
```

- `success`：至少一个账号探测成功。
- `failed`：已有账号级探测结果，但没有成功结果。
- `missing`：需要探测兜底，但没有可归并的账号级结果。

`missing` 只增加 `missing_probe_terminal_count` 和管理员完整性告警，不进入服务成功率失败分母，也不得单独把分组判定为不可用。该规则是本规格的最新确认口径，覆盖 T104 中“缺失终态按 `0/1` 失败计分”的旧文字。

## 7. 账号池诊断契约

管理员诊断必须区分：

- `pool`：进入本次调度扫描的原始账号数。
- `eligible`：通过资格过滤的候选数。
- `attempted`：实际发起上游请求的账号数。
- `filtered`：按原因统计的排除数量。
- `upstream_failed`：实际请求上游后失败的数量。

语义约束：

- `eligible=0`：调度阶段没有候选。
- `eligible>0 && attempted=0`：候选存在，但可能被槽位、二次复核或预算拦截。
- `attempted>0 && upstream_failed>0`：已进入上游请求/切号链路。
- `pool` 不得被单独展示为可用账号数。

用户继续只收到脱敏 502/503；账号和过滤原因只在管理员诊断、错误详情和账号视图展示。

## 8. 端到端数据流

```text
usage_logs + ops_error_logs + probe terminals
                  ↓
       exact correlation and deduplication
                  ↓
          one logical request terminal
                  ↓
 admin diagnostics / Monitor V4 / operational aggregates
```

读时投影必须先限定时间窗，再按逻辑键聚合；分页和统计均在去重后进行。历史字段不足时保守返回 `unknown`，不虚构成功、失败或扣费。

## 9. 管理员与用户边界

- 用户只看到最终脱敏协议结果和必要的重试建议。
- 管理员可以看到账号名称、账号 ID、分组、上游状态、错误阶段、过滤原因和安全重放判断。
- 不返回 API Key、Authorization、私钥、完整请求体、完整上游响应或完整模型输出。
- 账号级问题在账号管理/管理员诊断页面展示；分组视图只展示逻辑请求最终结果和分组级指标。

## 10. 兼容性与冲突处理

- T87 的安全重放和流终态规则保持不变。
- T93 的逻辑请求成功率、真实请求优先和探测一次化保持不变。
- T97 的客户端责任错误/明确模型不支持排除规则保持不变。
- T104 的持久化快照、固定 `as_of` 和原子替换保持不变。
- 本规格明确修正 T104 与第 5 题之间的冲突：`missing` 为完整性告警，不进入失败分母。
- T106 用户用量汇总 SQL 修复不属于本任务范围；其生产部署由主线另行完成。

## 11. 验收矩阵

1. 一个逻辑请求包含多个失败 attempt、最终成功时，Monitor V4 只计一个成功。
2. 多次 attempt 最终失败时，只计一个失败。
3. `pool=25, eligible=0` 时，管理员能看到真实过滤原因，不能误判为 25 个可用账号。
4. `pool=25, eligible=3, attempted=0` 时，能区分候选存在但未发起上游请求。
5. 无真实请求、探测明确失败时，计一个失败探测样本。
6. 无真实请求、探测终态缺失时，产生 `missing` 告警但不计服务失败。
7. 同桶有真实请求时，缺失探测终态不影响真实请求计分。
8. 已输出内容或 usage 后停止重放时，最终失败计一次且不拼接第二段流。
9. 客户端责任错误和明确模型不支持不降低本站服务成功率。
10. 管理员诊断、Monitor V4 和错误流水对同一逻辑请求使用一致终态。

## 12. 测试策略与发布边界

测试覆盖逻辑键关联、attempt 去重、终态优先级、恢复成功、重试耗尽、不安全重放、账号池诊断、探测 `success/failed/missing`、真实请求覆盖探测和快照聚合。

本规格不直接修改代码、不新增迁移、不写生产数据、不改变账号/分组配置。实施必须在独立任务包中完成，根总控负责合并、推送、部署和线上验证；发布前必须从干净且与 `origin/main` 一致的根 `main` 执行。

## 13. 批准记录

- 用户已确认第 4 题 `pool` 诊断口径。
- 用户已确认第 5 题探测终态完整性口径。
- 用户已确认将第 4、5 题规格合并到本题规格。
- 用户已确认采用“统一逻辑请求终态读时投影”的推荐方案。
