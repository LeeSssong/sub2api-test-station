# T66 Responses 安全切号与账号故障隔离交接

- 状态：`READY_FOR_ROOT_REVIEW`（已补充用户于 2026-08-25 明确授权的 Luna 本地不可用指引）
- 原始基线：`main@7fb71683b`
- 候选提交：`36949c8d0`
- 候选 worktree：`.worktrees/t66-response-failover-safety`
- 候选分支：`codex/t66-response-failover-safety`
- 未合并、未推送、未部署，等待根总控发送 `AUTHORIZE_MERGE_TO_MAIN`
- `downtime_required`：待根发布预检确认；当前候选未执行停机或生产动作

## 范围与实现

本任务只覆盖已批准的三类调度行为，并复用 Sub 原生调度、短冷却、临时不可调度和账务事实源：

- 余额不足继续进入原生确定性失败隔离链路（`HandleUpstreamError` / `deterministic_failure_isolation`），不新增平行余额事实源。
- Responses 与 Anthropic Messages 的 502/503 失败账号立即进入原生 10 秒短冷却，并加入当前请求的 `failedAccountIDs`，跳过同账号重试；冷却后仍可按原生半开逻辑恢复。
- Responses 只有在终止事件为 `response.failed`、没有 usage/扣费、没有语义输出且没有 `unsafe_to_replay` 时，才允许同请求切换到其他账号。
- 已产生 usage、扣费、语义输出、请求副作用或 `unsafe_to_replay` 时，标记为不可安全重放并禁止切号/重复重放。
- 当 `gpt-5.6-luna` 进入本站无可用账号路径时，返回 HTTP 404、`model_not_found`，并提示改用 `gpt-5.6-sol` / `gpt-5.6-terra`，或切换到支持 Luna 的分组；不自动降级、不暴露内部账号或分组名称。
- Luna 提示复用本地 `classifyNoAccountError`，不复用或修改全局 `ErrorPassthroughRule`，因此不会改变其他模型或上游 5xx 的故障转移语义。

新增的 `UpstreamFailoverError`、请求尝试元数据和 resilience 事件字段包含 `ResponseFailedOnly`、`UnsafeToReplay`、`SwitchAllowed`、`SwitchReason` 等安全判定信息；日志只记录脱敏判定字段，不记录 Authorization、请求体或完整密钥。

## 主要变更文件

- `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`
- `upstream/sub2api/backend/internal/service/gateway_service.go`
- `upstream/sub2api/backend/internal/service/openai_account_model_transient.go`
- `upstream/sub2api/backend/internal/service/openai_account_model_transient_test.go`
- `upstream/sub2api/backend/internal/service/openai_gateway_passthrough.go`
- `upstream/sub2api/backend/internal/service/openai_gateway_response_handling_type_test.go`
- `upstream/sub2api/backend/internal/service/openai_gateway_upstream_errors.go`
- `upstream/sub2api/backend/internal/service/openai_resilience_observability.go`
- `upstream/sub2api/backend/internal/service/openai_resilience_observability_test.go`
- `docs/superpowers/specs/2026-08-25-t66-response-failover-safety-design.md`
- `docs/superpowers/plans/2026-08-25-t66-response-failover-safety.md`

## 验证证据

直接相关门禁已通过：

```bash
go test ./internal/handler \
  -run 'TestOpenAIResponsesFailoverAllowed_RequiresPureResponseFailed|TestOpenAIRequestAttemptMetadata_ContextRoundTrip|TestOpenAIDecideRetry_CachePreservationModes' \
  -count=1

go test ./internal/service \
  -run 'TestNewOpenAIStreamFailoverErrorMarksResponseFailedAndUsage|TestNewOpenAIStreamFailoverErrorPreservesUnsafeReplayMarker|TestRecordOpenAIResponsesFailoverDecisionPreservesSafetyFields|TestRecordOpenAIAccountModelFailure_502ImmediatelyStartsShortCooldown|TestOpenAIModelTransient_RuntimeDecisionAndHalfOpen|TestRateLimitService_DeterministicBalanceUsesNativeTempUnschedulable' \
  -count=1

go build ./cmd/server
git diff --check
```

新增/聚焦测试覆盖纯 `response.failed` 门禁、usage/输出/unsafe replay 禁止切号、502/503 立即短冷却、请求级账号排除、余额不足原生隔离和可观测性安全字段。整包 `go test ./internal/handler ./internal/service` 仍有既有基线失败（中文错误文案、scheduler/sticky 随机选择、WebSocket 长测，以及根 `main` 已存在的 Responses pool SSE rate-limit 文案断言）；完整输出保留在 `/tmp/t66-openai-focused.txt` 及任务执行日志中，聚焦 OpenAI response.failed/transient 测试通过。

Luna 扩展新增 RED→GREEN 证据：`TestClassifyNoAccountError_LunaUnavailable_ReturnsAlternativeGuidance` 初始失败于旧通用 “Model ... is not supported” 文案；实现后 Luna 无支持账号、空账号池、Responses JSON 契约和非 Luna 既有 404/503 分支均通过。`go build ./cmd/server`、`gofmt` 与 `git diff --check` 通过。完整 handler/service 包中的既有流式文案基线失败未修改。

## 发布与回滚边界

- 无数据库迁移、无配置变更、无生产数据写入。
- 根总控需先盘点 T65/T66 候选及最新 `main`，按队列串行合并；合并后的 `main` 再执行直接相关门禁、发布预检、推送、蓝绿部署和线上 Responses SSE/JSON 专项验证。
- 真实生产请求的 SSE/JSON response.failed、usage、扣费和 `unsafe_to_replay` 组合尚未在线验证，属于根发布后的专项验收风险。
- 若合并后验证或发布失败，保留本候选 worktree、分支和证据，在原任务包继续修复；回滚时恢复到合并前 `main` 或候选提交 `36949c8d0` 对应的上一稳定实现。
