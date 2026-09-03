# OpenAI 流请求诊断、模型能力预检查与缓存指标统一规格

**日期：** 2026-09-03  
**状态：** 设计已确认，待实施计划  
**范围：** OpenAI `/responses` 流请求、账号-模型能力判定、生产蓝绿版本身份、尽力异步请求生命周期日志、性能监控缓存命中率

## 1. 背景

生产日志已确认以下现象：

- 用户提供的 `cf-ray=a3539a0f3a7e1912-LAS` 对应请求最终为网关返回的 `502`，直接原因是生产 `green` 槽选中的上游账号返回 `503`，并非 Cloudflare 主动生成错误。
- 部分账号对目标模型返回明确的 `404 model_not_found`，说明账号模型能力配置与调度请求不匹配。
- 部分流请求在首输出后出现 `stream_read_error` 或 `Upstream response stream was interrupted`。首输出后可能已经产生上游计费或副作用，不能自动重放或盲目切换账号。
- 生产 blue/green 当前镜像和发布 commit 不一致，导致同一问题可能在不同槽表现不同，也降低日志回查准确性。
- Monitor V4 的缓存命中率使用普通输入 token 作为分母，而控制面板的 Sub 原生指标使用 `cache_creation_tokens + cache_read_tokens` 作为分母，造成页面数值明显不一致。

## 2. 用户确认的决策

以下决策属于本规格的强约束：

1. **不修改首输出前 `502/503` 的跨账号切换策略。** 即使首输出尚未到达，也可能已经产生上游扣费或不可见副作用；本规格不得扩大重试、切号或重放范围。
2. **实施账号-模型能力预检查和 404 cooldown。** 在调度前尽量排除已知不支持目标模型的账号；收到明确 `404 model_not_found` 后，将账号-模型组合进入冷却期。
3. **统一 blue/green 镜像与 commit 标识，以最新版本为准。** 活动槽和候选槽必须使用同一不可变镜像及同一源码 commit；日志中的发布身份必须与实际容器一致。
4. **保留并上线 `openai.stream_incomplete` 埋点。** 该事件用于识别无终态流、客户端断开和上游读取异常，不改变业务重试语义。
5. **补充请求生命周期字段，但采用尽力异步登记。** 日志登记不得阻塞请求、不得阻塞流转发、不得影响计费和响应；写入失败允许丢弃，并记录有限的丢弃/失败计数。不把该登记作为请求成功条件。
6. **性能监控缓存命中率改为 Sub 原生口径。** Monitor V4 与控制面板使用同一公式和字段定义。

## 3. 非目标

- 不自动把首输出前的 `502/503` 改为跨账号 failover。
- 不对已产生输出、usage、工具调用或其他副作用的请求进行自动重放。
- 不取消正在等待的上游请求，不因 60 秒慢首输出创建第二个 attempt。
- 不修改用户价格、倍率、计费扣费顺序、分组关系、账号 `schedulable` 状态或利润保护规则。
- 不把诊断日志作为第二套账务或质量事实源。
- 不记录 Authorization、API Key、Cookie、OAuth token、代理密码、完整请求体、完整响应体或 SSE data 正文。
- 不在本规格内接入外部 tracing 平台或新增独立运维系统。

## 4. 账号-模型能力预检查与 404 冷却

### 4.1 能力状态

账号-模型能力状态至少包括：

```text
unknown
supported
unsupported
cooldown
```

能力判定必须按规范化的 `account_id + canonical_scheduling_model` 维度保存或复用现有状态，不得只按账号或原始请求模型保存。

### 4.2 调度前过滤

调度器在既有资格检查之后、选择上游账号之前，过滤已知 `unsupported` 或仍在 `cooldown` 的账号。`unknown` 账号仍可参与正常调度，不得因为缺少探测证据直接禁用。

预检查不得发起额外的真实业务请求。若复用主动探测，必须沿用现有探测隔离和成本规则，且不得将探测结果伪装成业务账务流水。

### 4.3 404 处理

当上游明确返回 HTTP `404` 且错误语义为 `model_not_found`、`model_not_supported` 或等价模型能力错误时：

- 记录账号、规范化模型、上游状态和脱敏后的错误分类。
- 将对应账号-模型状态标记为 `unsupported` 或进入带过期时间的 `cooldown`。
- 当前逻辑请求继续遵守既有 retry/failover 安全策略；本规格不扩大切号权限。
- 后续请求在冷却期内不再优先选择该账号-模型组合。
- 冷却到期后允许重新验证，成功时恢复 `supported`，再次明确 404 时重新冷却。

### 4.4 验收要求

- 账号 299 请求 `gpt-5.6-terra` 的 404 能被归类为模型能力问题，而不是网络故障。
- 已知不支持账号不会继续出现在该模型的正常候选排序中。
- 冷却状态按模型隔离，不能错误影响同一账号支持的其他模型。
- 不改变首输出前的现有扣费安全和重试边界。

## 5. blue/green 镜像与发布身份一致性

### 5.1 一致性合同

发布完成后，blue、green、worker、model-detector（若参与同一版本发布）必须满足：

```text
source_commit == image_source_commit == deployment_commit
image_digest 相同
```

活动槽由 Caddy 的实际 upstream 决定，不能仅根据容器名称推断。

### 5.2 日志字段

每条 Caddy 入口日志和 Sub2API 请求终态日志必须包含：

```text
environment
deployment_commit
container_slot
container_id
image_digest
active_upstream
```

字段缺失或互相不一致时，发布预检必须失败关闭；不得继续声称双槽版本一致。

### 5.3 版本选择

统一使用当前已验证的最新版本作为 blue/green 的唯一候选来源。不得让旧 green 继续承载与新 blue 不同源码行为的生产流量。

本规格只定义一致性要求，不授权生产部署。部署仍须遵守项目全局发布门禁，并从已推送且干净的 `main` 发起。

## 6. `openai.stream_incomplete` 埋点

### 6.1 触发条件

当 OpenAI 流在观察到终态事件前结束，或读取/解码过程出现异常时，记录一次：

```text
openai.stream_incomplete
```

客户端主动断开也可记录该事件，但必须通过 `client_disconnected=true` 与上游读取异常区分。

### 6.2 必填字段

```json
{
  "event": "openai.stream_incomplete",
  "request_id": "...",
  "logical_request_id": "...",
  "attempt_id": "...",
  "client_request_id": "...",
  "thread_id": "...",
  "window_id": "...",
  "session_id": "...",
  "account_id": 0,
  "model": "...",
  "mapped_model": "...",
  "upstream_request_id": "...",
  "response_id": "...",
  "output_started": false,
  "client_disconnected": false,
  "terminal_event_seen": false,
  "failed_event_seen": false,
  "usage_present": false,
  "error_class": "upstream_eof",
  "error_message": "脱敏后的非敏感错误",
  "bytes_read": 0,
  "response_bytes_forwarded": 0,
  "account_switch_count": 0,
  "ttft_ms": null,
  "elapsed_ms": 0,
  "environment": "production",
  "deployment_commit": "...",
  "container_slot": "blue",
  "container_id": "..."
}
```

错误消息必须移除认证信息、token、Cookie、请求正文、响应正文和敏感 URL 参数；保留可用于判断 TCP、HTTP/2、TLS、EOF、超时、代理和 SSE 解码问题的非敏感错误链。

### 6.3 与业务语义的关系

- 埋点只记录事实，不触发重试、切号、扣费、封禁或状态升级。
- `output_started=true` 时必须保留 `unsafe_to_replay` 语义。
- `client_disconnected=true` 不得被归类为账号故障。
- 已收到 `response.completed` 后的客户端边缘错误，不得记录为“上游无终态”；应单独保留终态和边缘候选信息。

## 7. 请求生命周期异步登记

### 7.1 事件阶段

尽力记录以下阶段：

```text
accepted
upstream_request_started
response_headers_received
first_event_received
first_visible_output
terminal_event_received
decoder_error
client_disconnected
completed
failed
```

### 7.2 观测字段

在不记录正文的前提下，尽力补充：

- 首字节时间、首事件时间、首个可见输出时间。
- 最后事件类型、终止事件类型、是否收到 `response.completed`/`response.failed`。
- 上游 HTTP 状态、Content-Type、Content-Encoding、协议类型。
- 读取错误分类和脱敏错误链。
- 客户端是否断开、是否已经向客户端写出内容。
- 账号切换次数、上游 attempt 次数。
- 上游读取字节数、向客户端转发字节数。
- TTFT、总耗时、usage 是否存在。

### 7.3 异步与降级

- 事件写入使用有界内存队列或等价的进程内异步通道。
- 队列满、上下文取消或写入失败时，立即丢弃该事件，不阻塞主请求。
- 以聚合计数器记录 `dropped_count`、`enqueue_failed_count`，不得记录请求正文。
- 请求结束时不得等待异步登记完全落盘。
- 该功能关闭、降级或丢失时，业务响应、账务和调度行为必须保持不变。

### 7.4 验收要求

- 正常完成、上游 EOF、SSE 解码错误、客户端取消各至少能产生可区分的事件。
- 异步队列故障不会增加请求延迟或改变 HTTP 状态。
- 通过 `request_id`、`logical_request_id`、`attempt_id` 可以关联 Caddy、应用和上游日志。

## 8. 性能监控缓存命中率统一

### 8.1 采用的 Sub 原生公式

性能监控页面的缓存命中率改为与控制面板一致：

```text
cache_hit_rate =
  cache_read_tokens /
  (cache_creation_tokens + cache_read_tokens)
```

当分母为 0 时返回 `null`，页面显示 `--`，不得显示 0% 误导用户。

### 8.2 数据范围

- 继续沿用 Sub 原生成功请求、真实业务请求和既有时间窗口过滤规则。
- `2026-08-31 00:00 +08:00` 至 `2026-09-02 00:00 +08:00` 的排除窗口必须保留，不得移除或回填进统计。
- 失败请求、主动探测和不具备完整 usage 的样本继续遵守现有 Monitor V4 过滤规则；本规格只改变缓存命中率公式，不改变其他指标口径。

### 8.3 接口字段

后端应同时返回以下原始值，便于页面和控制面板核对：

```text
cache_read_tokens
cache_creation_tokens
cache_hit_denominator
cache_hit_rate
```

`cache_hit_denominator` 必须等于 `cache_creation_tokens + cache_read_tokens`。前端不得重新使用普通 `input_tokens` 计算该指标。

### 8.4 验收要求

- 同一时间窗口、同一分组、同一数据源下，性能监控和控制面板缓存命中率一致。
- 普通输入 token 增加但缓存 token 不变时，命中率不因普通输入 token 被动降低。
- 0 分母显示 `--`，不产生 NaN、Infinity 或 0% 假值。
- 日期排除窗口前后，其他指标的既有过滤和口径不变。

## 9. 测试与发布门禁

### 9.1 直接相关测试

- 账号-模型能力预检查：支持、未知、不支持、冷却到期恢复、模型维度隔离。
- 404 `model_not_found` 分类和 cooldown 持久化/读取。
- `openai.stream_incomplete`：EOF、连接重置、SSE 解码错误、客户端取消、收到终态后的边缘异常。
- 异步登记队列满和写入失败不阻塞主请求。
- blue/green 版本身份一致性预检和不一致时 fail-closed。
- Monitor V4 缓存命中率公式、0 分母、日期排除窗口和控制面板口径一致性。

### 9.2 发布约束

- 本规格本身不授权部署。
- 任何验收站或主站发布都必须从干净、已推送且与 `origin/main` 一致的根目录 `main` 发起。
- 未获得用户明确主站部署授权前，只能完成本地实现和直接相关测试。
- blue/green 统一必须以最新已验证版本为唯一发布输入；不得以旧 green 作为回退式新发布来源。

## 10. 完成定义

本规格对应实现只有同时满足以下条件才可标记完成：

1. 账号-模型预检查和 404 cooldown 已实现并通过直接相关测试。
2. `openai.stream_incomplete` 已进入实际生产镜像，并能按关联 ID 回查。
3. 生命周期日志为有界尽力异步，不阻塞请求且有丢弃计数。
4. blue/green 镜像、源码 commit、deployment commit 和 digest 一致。
5. 性能监控缓存命中率与 Sub 原生控制面板口径一致，日期排除窗口仍存在。
6. 按项目发布门禁完成必要的推送、部署和线上验证后，才可在项目总账标记“已完成”。
