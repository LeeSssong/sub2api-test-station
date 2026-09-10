# OpenAI Responses 流式可观测性与原生错误终止设计

日期：2026-09-10  
任务状态：DESIGNING

## 1. 背景与问题证据

本任务处理 OpenAI `/responses` 流式 passthrough 的两个相关问题：

1. Responses passthrough 主路径没有接入现有 `stream_observability_runtime`，导致生产日志中看不到完整的流生命周期、上游响应头、SSE 事件、终态和断流分类。现有 `BeginStreamObservation`、`ObserveUpstreamHeaders`、`ObserveSSEEvent`、`ObserveVisibleOutput`、`ObserveTerminal`、`ObserveReadFailure`、`ObserveClientWriteFailure`、`FinishStreamObservation` 主要接在 Chat/raw Chat SSE 路径。
2. 当上游返回 HTTP 200、随后发送裸 `event: error` 或网关内部写出统一 `event: error` 后结束时，`openAIForwardErrorAlreadyCommunicated` 仅按写出字节和错误前缀判断“错误已通知”。对于 Responses 协议，这不足以证明已经写出 `response.completed`、`response.failed`、`response.incomplete` 或 `response.cancelled` 终止事件，客户端因此看到 `stream closed before response.completed`。

截图关联日志记录为 2026-09-11 00:03:19 至 00:03:41（Asia/Shanghai；该日期晚于当前环境日期 2026-09-10，实施前需核对日志宿主时钟/时区）。三次请求共同表现为：外层 HTTP 200、上游/网关最终 502、已写出 `event: error`，但没有合法的 `event: response.failed`。这三次属于上游 502 被网关转换后的失败，不应被归类为 OpenAI overload。

## 2. 原生能力边界

账号编辑弹窗中的 `accounts.extra.openai_passthrough` 只控制 OpenAI 自动透传路径，语义是“仅替换认证并尽量保持请求/响应协议”。它不绕过 `sanitizeOpenAIResponseFailedEventForClient`，也不自动保证所有上游错误原样返回。

本任务保持该开关的既有职责，不把它扩展成无条件错误原样透传开关。错误响应仍须遵守来源、协议和敏感信息边界。

## 3. 目标

- 让 `/responses` passthrough 产生与 Chat SSE 一致的可关联生命周期日志。
- 将上游 EOF、连接重置、超时、SSE 解码失败、客户端断开和合法错误终态区分记录。
- 对已开始的 Responses SSE 流，保证异常结束前尽量写出一次合法 `response.failed`；若客户端已断开，不重复写入或伪造成功。
- 保留已经由上游发送的合法 `response.failed`，避免追加第二个失败终态。
- 对开启 OpenAI passthrough 的自购账号，在明确可安全透传的 OpenAI 原生错误范围内保留原生 `code/message`；网关内部错误、第三方错误和敏感字段继续清洗。
- 埋点不保存原始 SSE body、完整请求体、API Key、Bearer token、内部 URL、Cloudflare Ray ID 或其他凭据。

## 4. 非目标

- 不改变账号【透传】开关的存储字段、前端交互或默认值。
- 不把所有 OpenAI 错误都原样返回，不把第三方 502 伪装成 OpenAI overload。
- 不改变 failover、计费、并发、审计和 cyber policy 的既有业务判定。
- 不新增数据库表、迁移、配置项、外部日志系统或平行错误事实源。
- 不处理 WebSocket v2 的独立生命周期问题；仅在共享 helper 不改变其既有行为的前提下复用协议判断。
- 不在本任务中部署验收站或主站。

## 5. 方案比较与选择

### 方案 A：仅补日志

在 `handleStreamingResponsePassthrough` 入口和 scanner 循环中接入现有观察器，但不改变终止判断。

优点是改动小；缺点是只能观察断流，不能修复严格 Responses 客户端收到的协议错误。拒绝。

### 方案 B：只在 handler 末尾无条件追加 `response.failed`

只修改 `ensureForwardErrorResponse` 或 `openAIForwardErrorAlreadyCommunicated`，看到已写字节但无完成结果时追加失败事件。

优点是能覆盖部分截图场景；缺点是 handler 不掌握上游事件边界，容易在合法 `response.failed`、客户端已断开或其他 SSE 协议中重复写入。拒绝作为唯一方案。

### 方案 C：服务层记录协议终态，handler 按终态补偿，配合白名单原生错误透传（推荐）

在 Responses passthrough 服务层记录事件、可见输出、合法终态和客户端写失败；异常返回前由服务层补发一次 `response.failed`，handler 的“已通信”判断只认可合法 Responses 终态或明确的非流式错误响应。错误内容通过现有清洗函数和新的有限原生错误白名单决定。

该方案让协议知识留在 SSE 服务层，让 HTTP fallback 留在 handler，能够同时解决截图中的断流和可观测性缺失，且不破坏现有 failover/计费路径。

## 6. 端到端数据流

1. `/responses` 请求选定 OpenAI 账号并进入 `handleStreamingResponsePassthrough`。
2. 入口确认或创建 `StreamObservation`，记录请求、账号、模型、环境、部署 commit、槽位和关联 ID；收到上游 `http.Response` 后记录状态、Content-Type、编码、协议和传输编码。
3. SSE scanner 每解析一个事件，记录事件类型、递增索引和累计字节数，不记录事件 body。首个可见输出记录 `semantic_output_seen` 和转发字节数。
4. 收到 `response.completed`、`response.failed`、`response.incomplete`、`response.cancelled` 或既有协议允许的 `[DONE]` 时记录合法终态、response ID、usage 摘要和终态事件名。
5. 收到裸 `error` 时：
   - 如果上游随后发送 `response.failed`，保留权威失败终态，并避免把裸 error 当成最终终态；
   - 如果流结束或读取失败前没有 `response.failed`，由服务层补发一次脱敏后的 `response.failed`；
   - 如果客户端已经写失败，只记录客户端断开，不再尝试写入。
6. scanner 返回 EOF、unexpected EOF、连接重置、超时、SSE 行过长或其他读取错误时，记录对应失败阶段和分类；未收到合法终态的流必须标记 `openai.stream_incomplete`，而不是成功或 overload。
7. `FinishStreamObservation` 只在合法终态完成且没有失败，或明确记录客户端断开/上游失败后结束；不会把“写出过 event:error”单独视为 Responses 已完成。
8. handler 收到服务错误时，只有在确认合法终态已写出或已写出完整非流式响应时，才跳过 fallback；否则调用既有 `writeResponsesFailedSSE` 生成协议终止事件。

## 7. 错误和安全契约

### 7.1 合法终止事件

Responses 流的合法终止集合为：

- `response.completed`
- `response.failed`
- `response.incomplete`
- `response.cancelled`
- 现有实现明确支持的 `[DONE]` 兼容终止标记

裸 `event: error` 只能作为错误信号，不能单独证明 Responses 流已正常终止。

### 7.2 原生错误透传

开启 `IsOpenAIPassthroughEnabled()` 且来源被确认是 OpenAI passthrough 的账号，只允许在安全白名单内保留上游错误 `code` 和用户可公开的 `message`。白名单至少覆盖：

- OpenAI 容量/过载类错误（如明确的 overload / capacity shed）；
- OpenAI 模型不可用或模型选择类错误（如 select model / model unavailable）；
- OpenAI 明确的限流错误。

白名单必须由结构化 code、HTTP 状态和已知事件形态共同确认，不能仅凭 message 关键词判断。未命中白名单时继续使用现有脱敏和统一错误映射。

以下内容不得透传：内部代理地址、第三方上游地址、request ID/Ray ID、认证信息、完整内部错误链、上游 HTML、未知供应商私有字段和完整请求内容。

### 7.3 截图类 502

第三方或代理层 502 不属于 OpenAI 原生错误。客户端错误应保持 `upstream_error`/`upstream_unavailable` 等网关安全语义；同时通过管理员诊断和 stream lifecycle 日志保留脱敏的上游状态、失败阶段和错误分类。

## 8. 预期代码边界

实现阶段优先修改以下既有模块和测试，不做无关重构：

- `backend/internal/service/openai_gateway_passthrough.go`：Responses passthrough 埋点接线、事件终态状态机和失败补偿。
- `backend/internal/service/stream_observability_runtime.go` 及其测试：必要时补充终态/不完整流契约。
- `backend/internal/handler/openai_gateway_handler.go`：收紧 `openAIForwardErrorAlreadyCommunicated`，只认可合法 Responses 终态或完整响应。
- `backend/internal/service/openai_gateway_response_handling.go`：扩展有限的原生错误保留判定，继续复用脱敏逻辑。
- 相关 `*_test.go`：服务层 passthrough、handler fallback、错误清洗和生命周期观察测试。

实际文件以实现阶段代码核对结果为准；不修改账号字段和前端开关。

## 9. 场景化验收矩阵

| 场景 | 客户端响应 | lifecycle 结论 | 备注 |
|---|---|---|---|
| 正常 `response.completed` | 原事件保留 | completed + terminal | usage/response ID 可关联 |
| 上游合法 `response.failed` | 保留一次脱敏后的 failed | failed + terminal | 不追加第二个 failed |
| 仅 `event:error` 后 EOF | 补一次 `response.failed` | incomplete/failed，非 overload | 修复截图同类协议断流 |
| 上游 HTTP 502、尚未输出 | 正常 JSON/fallback 错误 | upstream HTTP failure | 不伪装 OpenAI 原生错误 |
| 输出后上游 unexpected EOF | 补 `response.failed`，除非客户端已断开 | `openai.stream_incomplete` + upstream_eof | 不记为 completed |
| 上游连接 reset | 补 failed 或记录无法写入 | upstream_connection_reset | 不记为 overload |
| 上游超时 | 补 failed 或记录无法写入 | upstream_timeout | 保留失败阶段 |
| 客户端 broken pipe/cancel | 不重复写入 | client_disconnected | 不把客户端断开归因于上游 |
| passthrough 账号命中安全原生错误白名单 | 保留允许的 code/message | failed + terminal | 不透传敏感字段 |
| passthrough 账号未命中白名单 | 现有脱敏统一错误 | failed + terminal | 开关不改变安全边界 |

## 10. 测试策略

采用 TDD：每个行为先添加能在当前实现失败的定向测试，再实现最小修复。

- 服务层：正常 completed、原生 failed、仅裸 error、EOF/reset/timeout、客户端写失败、重复终态和失败补偿。
- handler：`openAIForwardErrorAlreadyCommunicated` 对合法终态、裸 error、仅心跳、完整 JSON 响应的区别。
- 错误清洗：原生白名单命中、未知错误统一映射、request ID/URL/token 脱敏。
- 生命周期：首个事件、可见输出、终态、失败阶段、client disconnected 和 incomplete 的字段契约。
- 最小验证命令：相关 Go package 定向测试、`go build ./cmd/server`、`gofmt`、`git diff --check`。不执行全仓无关测试，不做部署。

## 11. 发布、验证与回滚

- 本任务只产生候选分支和交接证据，不自动合并、推送根 `main` 或部署。
- 合并前必须确认候选从最新 `origin/main` 派生，且只包含本规格范围内的文件。
- 未来部署必须遵守验收站/主站统一约束：只能从干净且与 `origin/main` 一致的根 `main` 发起，并需要用户明确主站授权。
- 回滚方式为恢复上一已验证的 Sub2API 镜像/commit；本任务无数据库迁移、无配置迁移、无业务数据写入。
- 线上专项验证应至少检查：Responses 正常流、上游 502、仅 error 后 EOF、原生 failed、客户端断开，以及 `openai.stream.lifecycle` / `openai.stream_incomplete` 事件。

## 12. 待决事项与批准记录

待决事项：实现阶段需根据现有结构化错误字段确认 OpenAI 原生错误白名单的精确 code 集合；若某一错误只能通过不可靠文本关键词识别，则默认不原样透传。

用户已于 2026-09-10 明确确认开始编写本规格书，并确认继续处理流式埋点/断流修复；本书面规格书仍需用户审阅批准后，才能进入实施计划和代码修改阶段。
