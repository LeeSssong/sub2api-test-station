# T123 Responses 流式语义逃逸与错误脱敏规格

**状态：** 修复方案已获用户确认，书面规格等待用户审阅批准  
**日期：** 2026-09-03  
**范围：** OpenAI Responses 流式失败判定、可重放切号、客户端终态和管理员诊断

## 1. 问题证据与当前行为

生产日志出现 `stream disconnected before completion`、`Service temporarily unavailable`、`openai_error` 及 request id 直接暴露给用户。对应请求记录显示 `output_started=true`、`usage_produced=false`、`unsafe_to_replay=true`、`switch_allowed=false`，且 `fallback_error_response_written=false`、`upstream_error_response_already_written=true`。

当前 `OutputStarted` 在部分 Responses 路径上接近“已写出任意流字节”，会把 `response.created`、`response.in_progress` 等协议前导误当作用户语义输出。失败后 handler 因此禁止重放切号；同时 forward 已将上游错误帧写给客户端，统一脱敏/替换未覆盖所有 `error.message` 形态，导致英文原文和 request id 泄露。

## 2. 目标与非目标

目标：区分协议前导、语义输出、usage、业务副作用和终态失败；在语义输出前且无 usage/副作用时安全切号；在不可重放时输出合法且脱敏的 `response.failed`；管理员保留完整非敏感诊断链。

非目标：不扩大 retry budget，不改变 Chat/Messages/WebSocket、计费、余额、调度、数据库 schema 或上游协议；不把原始错误正文、Authorization、API key、URL 参数或 request id 放入用户文案。

## 3. 方案比较与推荐

1. **局部补丁 OutputStarted（不采用）**：改少量条件，但继续混淆协议字节与语义输出，无法证明可重放安全。
2. **全链路语义状态机（采用）**：在现有 parser/handler 状态上增加明确状态，统一驱动切号、终态和脱敏，兼容当前错误账本。
3. **先缓冲完整流再决定（不采用）**：可降低误判但增加延迟、内存和断流风险，且不能解决已写错误帧泄露。

## 4. 状态与控制流

每次 attempt 必须维护：`ProtocolOutputStarted`、`SemanticOutputStarted`、`UsageProduced`、`SideEffectStarted`、`TerminalFailureSeen`、`ClientDisconnected`。仅以下条件同时满足才允许切号：

```text
terminal_failure_seen
&& !semantic_output_started
&& !usage_produced
&& !side_effect_started
&& request_context_valid
&& retry_budget_allows
```

语义输出前失败：丢弃暂存协议前导，标记当前账号失败并切换下一账号；不得向客户端追加上游错误帧。语义输出后失败：禁止切号，发送唯一合法 `response.failed` 终态，并结束 SSE。客户端已断开时停止写入，但仍记录终态和诊断。

## 5. 接口与字段契约

`OpenAIStreamRecoveryDetails`、`UpstreamFailoverError` 和 attempt metadata 增加/映射上述状态；旧 `OutputStarted` 仅作为兼容派生字段，不能单独决定 `UnsafeToReplay`。日志字段至少包括 `semantic_output_started`、`protocol_output_started`、`usage_produced`、`side_effect_started`、`terminal_failure_seen`、`switch_allowed`、`switch_block_reason` 和 `client_error_sanitized`。

用户可见错误允许值为稳定本地化文案和有限错误码，如 `upstream_unavailable`、`upstream_timeout`、`upstream_protocol_error`；禁止原样透传 request id、Ray ID、URL、代理认证、英文上游正文。管理员日志保留经敏感字段过滤后的完整 error chain，并与 `request_id`、`logical_request_id`、`attempt_id` 关联。

## 6. 失败、安全与兼容性

解析到 `response.failed` 或 bare `error` 时先提取 usage，再判断语义输出和副作用；同一 attempt 只允许一个终态。脱敏函数必须覆盖 SSE `error`、`response.failed.error.message`、HTTP 错误和 fallback 文案，采用白名单模板而非黑名单替换。脱敏失败时使用固定 `Upstream response failed`，绝不回退原文。

历史客户端仍收到合法 Responses SSE；非 Responses 路径保持旧行为。无需迁移。回滚恢复上一个已验证镜像即可。

## 7. 可观测性

新增计数：`responses_failover_allowed_total`、`responses_failover_blocked_total{reason}`、`responses_error_sanitized_total`、`responses_terminal_duplicate_total`。日志必须能回答：是否收到终态、是否产生语义输出/usage、副作用、为何切号被阻止、是否已写客户端错误。禁止记录凭据和原始响应正文。

## 8. 验收矩阵

| 场景 | 预期 |
| --- | --- |
| 仅收到 created/in_progress 后断流 | 允许切号，客户端看不到上游英文错误 |
| 收到首个文本/工具语义事件后断流 | 禁止切号，输出一次脱敏 response.failed |
| usage 已产生但无语义输出 | 禁止切号，账务保持一次 |
| bare error 后 response.failed | 合并为一个终态，不重复写帧 |
| 错误含 request id/URL/Ray ID | 用户文案无敏感值，管理员诊断可关联 |
| 客户端主动断开 | 不追加响应，记录 client_disconnected |
| Chat/Messages/非 Responses | 现有测试和行为不变 |

## 9. 测试与发布

先添加 parser、handler 和 failover 的失败回归测试，再实现状态机；覆盖前导误判、语义输出边界、usage、副作用、重复终态、脱敏字段和客户端断开。运行直接相关 Go 测试、`go build ./cmd/server`、`gofmt`、`git diff --check`。本规格不授权合并、推送、部署或真实上游流量；后续发布须从干净且已推送的 `main`，按验收站全局约束执行并保留回滚证据。

## 10. 未决事项与批准记录

- 语义输出事件白名单（文本 delta、工具调用 delta、音频/图像输出）在实施计划中逐项固定；未知事件默认不算语义输出但必须告警。
- 用户已确认修复方向；当前仅等待书面规格批准，未实现、未测试、未提交、未推送、未部署。
