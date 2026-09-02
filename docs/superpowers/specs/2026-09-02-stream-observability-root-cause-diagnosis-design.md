# 流式请求全链路可观测与根因诊断规格

**日期：** 2026-09-02  
**任务包：** T113  
**状态：** 设计已确认，待实施计划  
**范围：** Sub2API OpenAI 流式请求、主站 Caddy/蓝绿容器、验收站同构入口及管理员请求诊断

## 1. 背景与问题

当前用户可见错误：

```text
stream disconnected before completion: Transport error: network error: error decoding response body
```

该错误只能证明客户端在流式响应完成前失去了可解码的响应体，不能直接证明故障来自上游账号、OpenAI、Cloudflare、Caddy 或 Sub2API。

当前主要缺口：

- Caddy 有入口记录，但应用日志不一定保留同一请求的完整终态。
- 蓝绿切换后，旧容器日志可能无法回查。
- `request_id`、`X-Client-Request-Id`、`upstream_request_id`、`response_id` 和客户端窗口标识没有形成强制关联合同。
- SSE 解码异常缺少最后事件、读取字节数、输出状态、usage 状态和客户端断开状态。
- 底层错误被转换为泛化错误后，无法稳定区分 TCP、HTTP/2、压缩、分块、代理、DNS 和协议错误。
- Caddy 日志没有稳定记录实际活动槽、后端实例和上游连接结果。

## 2. 目标

- 建立客户端、Caddy、Sub2API、账号、上游 attempt 和最终响应的稳定关联链。
- 记录流开始、响应头、首事件、首输出、终态、解码失败、客户端断开和请求结束。
- 完整保留非敏感底层传输错误，不使用笼统的 `network error` 替代可判定证据。
- 区分客户端、Cloudflare/Caddy、Sub2API、代理/TCP/TLS、上游显式错误和上游无终态断流。
- 在蓝绿切换和容器替换后仍能按 commit、槽位和容器身份回查。
- 在管理员请求详情中展示完整诊断摘要。
- 复用现有请求日志、错误日志、usage 日志和账号状态，不建立第二套业务事实源。

## 3. 非目标

- 不改变调度、重试预算、计费、服务档位、账号状态或上游协议。
- 不因增加诊断日志而自动封禁账号或扩大切号范围。
- 不把客户端显示的错误文案直接认定为上游根因。
- 不新建外部 tracing 平台或平行运维控制面。
- 不记录 Authorization、API Key、Cookie、OAuth token、代理密码、请求正文、响应正文或完整 SSE data。

请求关联 ID、账号 ID/名称、模型、上游请求 ID、协议元数据和非敏感底层错误不做业务级模糊化。

## 4. 关联标识合同

每个流式请求应保留：

| 字段 | 语义 | 要求 |
|---|---|---|
| `request_id` | Sub2API 服务端请求主键 | 必须 |
| `client_request_id` | `X-Client-Request-Id` | 有则完整保留 |
| `thread_id` | 客户端线程标识 | 有则完整保留 |
| `window_id` | `X-Codex-Window-Id` | 有则完整保留 |
| `session_id` | 客户端会话标识 | 有则完整保留 |
| `logical_request_id` | 跨 attempt 的稳定请求键 | 有重试时必须 |
| `attempt_id` | 单次上游尝试键 | 上游请求必须 |
| `upstream_request_id` | 上游请求标识 | 尽可能保留 |
| `response_id` | OpenAI 响应标识 | 收到后必须 |
| `deployment_commit` | 发布源 commit | 必须 |
| `container_slot` | `blue`、`green` 或 `acceptance` | 必须 |
| `container_id` | 容器实例身份 | 必须 |
| `environment` | `production` 或 `acceptance` | 必须 |

所有事件必须携带 `request_id`。跨账号 attempt 必须同时携带 `logical_request_id` 和 `attempt_id`。字段缺失时记录 `correlation_degraded=true`，禁止按时间邻近猜测关联。

## 5. Caddy 入口日志合同

Caddy 对 `/responses` 等流式入口记录：

- Host、路由、HTTP 协议和 TLS 协议。
- `X-Client-Request-Id`、`X-Codex-Window-Id`、Thread ID 和 Session ID。
- 实际 `active_upstream`、活动槽和后端服务名。
- 后端响应的 `X-Request-Id`。
- HTTP 状态、请求字节数、响应字节数和总耗时。
- Content-Type、Content-Encoding 和 Transfer-Encoding 枚举。
- 上游连接、响应头和响应体转发错误。

Caddy 不记录 Authorization、attestation、Cookie 和认证凭据原文。除这些敏感字段外，关联 ID和非敏感传输错误必须完整保留。

## 6. 应用流生命周期

统一事件名：

```text
openai.stream.lifecycle
```

允许的 `stage`：

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

公共字段：

```json
{
  "event": "openai.stream.lifecycle",
  "stage": "decoder_error",
  "request_id": "...",
  "logical_request_id": "...",
  "attempt_id": "...",
  "client_request_id": "...",
  "thread_id": "...",
  "window_id": "...",
  "session_id": "...",
  "account_id": 0,
  "account_name": "...",
  "platform": "openai",
  "model": "gpt-5.6-sol",
  "mapped_model": "gpt-5.6-sol",
  "upstream_request_id": "...",
  "response_id": "...",
  "environment": "production",
  "deployment_commit": "...",
  "container_slot": "blue",
  "container_id": "...",
  "elapsed_ms": 0
}
```

### 6.1 响应头事件

`response_headers_received` 记录：

- `http_status`
- `content_type`
- `content_encoding`
- `transfer_encoding`
- `protocol`
- `upstream_endpoint_class`

### 6.2 首事件与首输出

记录：

- `event_type`
- `event_index`
- `first_token_ms`
- `semantic_output_seen`
- `usage_known`
- `bytes_read`

只记录事件类型，不记录事件正文。

### 6.3 终态事件

记录：

- `terminal_event_type`
- `terminal_event_valid`
- `usage_known`
- input、output、cache read 和 cache creation Token 数量
- `client_output_started`
- `semantic_output_seen`
- `bytes_read`
- `response_bytes_forwarded`

### 6.4 解码或传输失败

`decoder_error` 或 `failed` 必须记录：

- `failure_stage`
- `error_class`
- `error_type`
- 完整非敏感 `error_chain`
- `last_event_type`
- `event_index`
- `saw_terminal_event`
- `saw_failed_event`
- `client_output_started`
- `semantic_output_seen`
- `usage_known`
- `bytes_read`
- `response_bytes_forwarded`
- `content_encoding`
- `protocol`
- `client_disconnected`

错误中若包含凭据、认证头、Cookie、token、正文或敏感 URL 参数，必须在落盘前移除。其余底层错误允许完整投影到管理员诊断。

## 7. 错误分类合同

`error_class` 允许值：

| 类别 | 含义 |
|---|---|
| `client_disconnected` | 客户端取消或应用向客户端写入失败 |
| `upstream_explicit_error` | 上游完整返回失败事件或 HTTP 错误 |
| `upstream_eof` | 上游在终态前 EOF/UnexpectedEOF |
| `upstream_connection_reset` | TCP、HTTP/2 reset、aborted 或 broken pipe |
| `upstream_timeout` | 上游连接或读取超时 |
| `upstream_sse_malformed` | SSE/JSON 内容不合法或单行超限 |
| `proxy_or_dns_failure` | 代理认证、DNS、路由或连接拒绝 |
| `edge_response_interrupted` | 应用已完成，但边缘到客户端响应链中断 |
| `unknown_transport` | 证据不足的传输错误 |

`failure_stage` 允许值：

```text
client_request_read
upstream_connect
upstream_headers
upstream_body_read
sse_decode
client_write
post_stream_usage
unknown
```

映射规则：

- `context.Canceled` 且客户端连接关闭：`client_disconnected`。
- `context.DeadlineExceeded`：根据阶段区分客户端取消或 `upstream_timeout`。
- `io.EOF`、`io.ErrUnexpectedEOF` 且没有终态：`upstream_eof`。
- `ECONNRESET`、`ECONNABORTED`、`EPIPE`：根据读写方向区分上游 reset 或客户端断开。
- `bufio.ErrTooLong`、SSE/JSON 语法错误：`upstream_sse_malformed`。
- 代理认证失败、连接拒绝、无路由、DNS 不存在：`proxy_or_dns_failure`。
- 应用收到有效终态但客户端仍报解码失败：检查 Caddy/Cloudflare，标记 `edge_response_interrupted` 候选。

## 8. 根因判定规则

只有证据充分时才输出 `root_cause`；否则输出 `insufficient_evidence`。

| 证据 | 结论 |
|---|---|
| Caddy 有入口、应用无 `accepted` | 应用入口、容器路由或应用日志缺失 |
| 应用有 `accepted`、无响应头 | 上游连接、代理、DNS、TLS 或请求发起失败 |
| 有响应头、无终态且有 EOF/reset/timeout | 上游或上游代理在响应体阶段断开 |
| 响应体可读取但 SSE/JSON 解析失败 | 上游返回非法协议内容 |
| 应用收到 `response.completed`，Caddy 响应不完整 | Caddy、Cloudflare 或客户端响应链故障 |
| `client_disconnected=true` | 客户端主动断开，不归因为账号故障 |
| 单账号和模型重复出现代理/DNS/连接拒绝 | 该账号代理或通道故障候选 |
| 多账号、多模型同时出现相同错误 | 共享出口、边缘或宿主网络故障候选 |
| 单账号单模型明确 `model_not_found` | 账号模型能力问题 |

不得仅凭 HTTP 200、请求耗时、用户看到 502 或 `network error` 文案生成根因结论。

## 9. 蓝绿日志保留

每条入口和应用日志必须包含：

```text
environment
deployment_commit
container_slot
container_id
service_name
active_upstream
```

旧槽容器停止接收流量后，其日志仍需保留。初始保留窗口定为至少 24 小时，并覆盖最长流式请求时长、发布切换窗口和人工回查时间。

若本地 Docker 日志容量不足，应将脱敏后的结构化日志压缩归档；不能在仍可追查的窗口内因删除容器而丢失证据。

生产和验收站日志必须以 `environment` 分离，禁止跨环境近似关联。

## 10. 管理员诊断投影

请求详情新增“流式链路诊断”，展示：

- 环境、域名、协议、活动槽、commit 和容器 ID。
- 客户端请求 ID、线程 ID、窗口 ID和会话 ID。
- 服务端请求 ID、逻辑请求 ID、attempt ID。
- 账号 ID/名称、模型、映射模型、上游请求 ID和响应 ID。
- 生命周期节点、各阶段耗时和传输字节数。
- 最后事件、是否产生语义输出、是否收到终态、usage 是否已知。
- 完整非敏感底层错误链、错误分类和失败阶段。
- 是否客户端主动断开、是否发生切号及最终结果。
- `root_cause` 或 `insufficient_evidence` 及其支撑证据。

管理员页面仍不得展示认证凭据、Token、Cookie、代理密码、请求正文、响应正文或完整 SSE data。

## 11. 诊断查询接口

管理员接口支持按 `request_id` 或 `logical_request_id` 查询：

```json
{
  "request_id": "...",
  "logical_request_id": "...",
  "environment": "production",
  "entry": {
    "host": "api.xingqiaolab.top",
    "route": "/responses",
    "active_slot": "blue",
    "deployment_commit": "...",
    "container_id": "..."
  },
  "attempts": [],
  "final": {
    "status": "failed",
    "terminal_event": "none",
    "error_class": "upstream_eof",
    "failure_stage": "upstream_body_read",
    "root_cause": "insufficient_evidence"
  },
  "evidence_missing": []
}
```

接口只读，不触发重试、切号、账号状态变化或计费变更。

## 12. 安全与容量

- 仅凭据、认证头、Cookie、token、正文和敏感 URL 参数必须在落盘前移除。
- 请求关联 ID、账号/模型、上游 ID、错误类别和非敏感底层错误不得被无意义截断或泛化。
- 不允许每个 token 或每个 delta 写一条日志，只记录生命周期关键节点。
- 日志写入失败不得阻断用户响应、usage 或 failover；应记录受限的 `diagnostic_write_failed` 指标。
- 允许按环境、槽位、模型、平台、错误类别和失败阶段聚合指标。

## 13. 验收矩阵

| 场景 | 必须证据 | 预期结论 |
|---|---|---|
| 正常完整流 | headers、首事件、首输出、终态、completed | 成功 |
| 上游 EOF 无终态 | last event、bytes、typed error | upstream EOF |
| 上游 reset | typed error、account、model、attempt | upstream connection reset |
| 代理认证失败 | proxy marker、account、attempt | proxy or DNS failure |
| SSE 非法 JSON | headers、bytes、decode error | upstream SSE malformed |
| 客户端主动取消 | client disconnected、context canceled | client disconnected |
| 应用完成后边缘截断 | 应用终态、Caddy 响应不完整 | edge response interrupted |
| 蓝绿切换期间请求 | slot、commit、container ID | 定位到具体容器 |
| 应用日志缺失 | Caddy 入口存在 | evidence missing，不归因上游 |
| 验收站请求 | environment=acceptance | 与生产隔离 |

## 14. 测试策略

- 生命周期事件字段和 stage 校验。
- request、logical request、attempt、upstream 和 response ID关联测试。
- EOF、UnexpectedEOF、reset、timeout、EPIPE、DNS、代理认证和客户端取消分类测试。
- `response.completed` 后客户端断开与上游未终态的区别测试。
- 非法 SSE、超长行和空流测试。
- 敏感字段不落盘测试；非敏感传输错误完整保留测试。
- 日志写入失败不影响用户响应、usage 和 failover 测试。
- 管理员查询在证据缺失时返回 `insufficient_evidence`。
- Caddy 公网请求与应用 `X-Request-Id` 关联测试。
- 蓝绿切换后旧槽日志保留测试。

只运行直接相关测试、必要构建、格式化和 `git diff --check`；不产生真实上游流量，不修改生产业务数据。

## 15. 兼容、迁移与发布

- 用户错误响应保持兼容，仅增加内部结构化诊断。
- 优先使用现有日志和管理员只读投影，不新增数据库迁移。
- 老日志缺字段时返回部分结果和 `correlation_degraded`，不回填、不猜测。
- 生产与验收站使用同一字段合同，但日志和数据完全隔离。
- 实施必须在独立 worktree 完成；功能实现和直接相关测试通过后才能进入根审查。
- 所有部署只能从已推送、干净且与 `origin/main` commit/tree一致的根 `main`发起。
- 本规格不授权验收站或主站部署。

## 16. 方案选择

采用“生命周期节点 + 稳定关联 ID + 结构化错误分类”。

只增加最终错误日志无法区分故障阶段；记录每个 SSE 事件又会造成日志量和正文泄露风险。关键生命周期节点可以用可控容量回答请求到了哪里、断在哪里、是否已输出、是否已终态以及能否归因。

## 17. 决策记录

- 2026-09-02：用户再次点名本规格并明确“开始实施”，批准以本文件作为 T113 正式实施依据。
- 2026-09-02：确认先证明实际入口和活动容器，再判断上游根因。
- 2026-09-02：确认 `stream disconnected before completion` 只是表现，不是根因类别。
- 2026-09-02：确认主站 `/responses` 与验收站 `/admin/lab/*` 必须隔离追踪。
- 2026-09-02：确认请求关联 ID、账号/模型、上游请求 ID和非敏感底层错误完整保留。
- 2026-09-02：确认认证凭据、Token、Cookie、代理密码和正文不得写入普通日志。
- 2026-09-02：确认证据不足时返回 `insufficient_evidence`，禁止按时间或错误文案猜测。
- 2026-09-02：确认本规格不改变调度、重试、计费、账号状态或部署授权。
