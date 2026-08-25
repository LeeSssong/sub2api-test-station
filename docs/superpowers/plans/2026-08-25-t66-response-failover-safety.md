# T66 Responses 安全切号与账号故障隔离实施计划

> For agentic workers: use superpowers:executing-plans to implement this plan task-by-task.

Goal: 让余额不足、502/503 和纯 Responses response.failed 失败按安全边界处理，避免重复打坏账号或对已产生副作用的请求错误切号。

Architecture: 复用现有 OpenAI 网关 attempt metadata、UpstreamFailoverError、账号模型运行时 cooldown 和请求级 excluded account map；新增纯判定 helper 作为 Responses failover 入口的唯一门禁，并把余额与 502/503 接入已有故障记录器。

Tech Stack: Go、Gin、OpenAI gateway handler/service、runtime account-model resilience、Go tests。

Spec: docs/superpowers/specs/2026-08-25-t66-response-failover-safety-design.md

Global Constraints:
- 余额不足立即隔离，不同账号重试。
- 502/503 账号短时冷却并加入当前请求排除集合。
- 仅纯 response.failed、无 usage/扣费/语义输出、unsafe_to_replay=false 才允许切号。
- 已产生 usage、扣费、语义输出或 unsafe_to_replay=true 时禁止切号。
- 不修改粘性权重、Top-K、公平性、并发和模型映射。
- 所有实现先 RED 测试，再 GREEN 实现。

### Task 1: 集中 Responses 重放安全判定

Files:
- Modify: upstream/sub2api/backend/internal/handler/openai_gateway_handler.go
- Test: upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go
- Test: upstream/sub2api/backend/internal/service/openai_gateway_response_handling_type_test.go

Steps:
- [x] 写 table-driven 测试：纯 response.failed 允许；usage、输出、unsafe、非 failed 拒绝。
- [x] 运行聚焦 handler 测试确认 helper 尚不存在或行为错误。
- [x] 增加 openAIResponsesFailoverAllowed 纯 helper，并在 Responses failure 分支切号前调用。
- [x] 重跑聚焦测试确认只有安全场景允许切号。
- [x] 提交 feat: gate responses failover by replay safety。

### Task 2: 余额隔离与 502/503 短时冷却

Files:
- Modify: upstream/sub2api/backend/internal/handler/openai_gateway_handler.go
- Modify: upstream/sub2api/backend/internal/service/openai_account_model_runtime.go
- Test: upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go
- Test: upstream/sub2api/backend/internal/service/openai_account_model_runtime_test.go

Steps:
- [x] 写余额不足 403 停止重试、502/503 记录冷却并加入请求排除的失败测试。
- [x] 运行聚焦测试确认当前实现缺少目标状态。
- [x] 在现有 failure branch 接入稳定的余额分类、runtime recorder 和 failedAccountIDs；复用有界默认冷却。
- [x] 验证普通 4xx 和策略拒绝不会新增冷却。
- [x] 提交 feat: isolate depleted and transient upstream accounts。

### Task 3: 安全切号可观测性

Files:
- Modify: upstream/sub2api/backend/internal/handler/openai_gateway_handler.go
- Test: upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go
- Test: upstream/sub2api/backend/internal/service/openai_resilience_observability_test.go

Steps:
- [x] 写安全切号与禁止切号原因字段的失败断言。
- [x] 运行聚焦测试确认字段缺失。
- [x] 记录 response_failed_only、usage_produced、output_started、unsafe_to_replay、switch_allowed、switch_reason。
- [x] 验证日志不包含 Authorization、请求体或完整密钥。
- [x] 提交 chore: record safe failover decisions。

### Task 4: 集成验证

Steps:
- [x] gofmt changed Go files。
- [x] go test ./internal/handler ./internal/service（直接相关聚焦测试通过；全包存在既有基线失败，见交接说明）。
- [x] go build ./cmd/server、git diff --check。
- [x] 运行 Responses SSE/JSON、billing 和 failover 回归测试。
