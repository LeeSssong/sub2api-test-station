# T66 Responses 安全切号与账号故障隔离设计

## 状态

原范围已获用户确认；2026-08-25 用户又明确确认 Luna 本地不可用指引扩展，重新进入实现。

## 目标

只加强用户指定的四类调度行为：余额不足账号立即隔离；502/503 账号短时冷却并从当前请求排除；Responses 仅在上游只发出 `response.failed`、没有 usage/扣费/语义输出时允许同请求切号；一旦产生 usage、扣费或 `unsafe_to_replay`，禁止切号。

同时，当用户请求 `gpt-5.6-luna` 并进入本站无可用账号路径时，返回明确、稳定的模型不可用错误码和替代指引；不主动把请求降级为其他模型。

## 原生现状与证据

- OpenAI 网关已有 `UpstreamFailoverError`、attempt metadata、`UsageProduced`、`OutputStarted`、`unsafe_to_replay` 和账号运行时冷却记录。
- `openai_gateway_handler.go` 已在多个分支判断 `openAIForwardMayFailover`、`ShouldRetryNextAccount` 和 `SafeToFailoverAfterWrite`，但 Responses 的 `response.failed` 终止事件与“仅失败、无副作用”条件尚未形成单一准入函数。
- 账号运行时已有 `RecordOpenAIAccountModelFailure`、失败 streak、冷却截止时间和半开探测 API，可直接复用。

## 设计

### 余额不足

- 识别上游 401/403 中明确的 insufficient balance/quota 语义。
- 在当前尝试结束时立即把该账号-模型标记为不可调度/冷却，避免继续被同一请求或后续请求选中。
- 不对余额不足做同请求重试；返回原生映射错误。

### 502/503

- 对上游 HTTP 502/503 或明确的 transient upstream/network 失败调用已有运行时 failure recorder。
- 使用有界短冷却（复用现有默认值，不新增永久禁用），并把失败账号加入当前请求的 `excludedAccountIDs`。
- 同一请求后续候选不得再次选中该账号；冷却到期后由现有半开探测恢复。

### Responses 安全切号

增加唯一的安全判定：

允许切号仅当同时满足：

1. 上游终止事件是 `response.failed`；
2. 没有任何语义输出或 output item；
3. 没有 usage 事件、usage 记录或扣费；
4. attempt metadata 的 `unsafe_to_replay=false`；
5. 响应尚未向客户端写入不可重放内容。

以下任一条件成立则禁止切号：

- 已产生 usage 或扣费；
- `unsafe_to_replay=true`；
- 已开始语义输出/output item；
- 终止原因不是纯 `response.failed`；
- 客户端已断开或响应已不可安全重放。

### 可观测性

- 在切号、禁止切号、余额隔离和短时冷却日志中记录 request/attempt/account、判定原因、usage/output/unsafe 标志；不记录完整 Authorization、请求体或上游密钥。
- 保留现有 ops error 与 usage 账务语义，不新增账务记录。

### Luna 本地不可用指引

- 复用原生 `classifyNoAccountError` 路径，因为它同时持有用户原始模型名、分组约束和本站“无可用账号”的事实；不接入全局 `ErrorPassthroughRule`。
- 仅精确匹配 `gpt-5.6-luna`。在该模型无法获得本站账号时，保留 HTTP 503、`local_capacity_exhausted` 重试协议，仅将文案固定为“本站暂不支持gpt-5.6-luna，请切换模型重试”。
- 不暴露分组名称、账号数量、账号状态或上游信息；不改动其他模型的 404/503 语义，不覆盖已经开始上游响应后的错误。

## 不在范围

- 不修改粘性权重、Top-K、公平性、并发上限或模型映射。
- 不对普通 4xx、策略拒绝或已扣费响应自动切号。
- 不改动 Chat、Anthropic、Gemini 的非 Responses 语义，除非它们复用同一安全判定函数且测试证明无行为漂移。
- 不改变全局错误透传规则，也不将任何全局 502/503 改写为 Luna 提示。

## 验收标准

- 余额不足账号被立即隔离，后续同请求不再被选中。
- 502/503 账号进入短时冷却，并被当前请求排除；冷却后可半开恢复。
- 只有纯 `response.failed`、无 usage/扣费/输出、可安全重放时才切号。
- 已 usage、扣费、语义输出或 `unsafe_to_replay` 时不切号。
- Luna 无可用账号时，Responses/OpenAI-compatible JSON 仍返回 HTTP 503 与稳定 `local_capacity_exhausted`，仅替换提示文案；非 Luna 维持原有协议。
- 现有 failover、账务、Responses SSE/JSON 测试与新增定向测试通过。
