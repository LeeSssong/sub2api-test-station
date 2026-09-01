# T108 OpenAI 错误生命周期交接

- 任务包：T108
- 状态：`READY_FOR_ROOT_REVIEW`
- 基线 `main`：`a4580f476fbb61f89ddbcba3a8f5a5429313ec2f`
- 候选分支：`codex/t108-openai-error-lifecycle`
- 候选 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t108-openai-error-lifecycle`
- 候选提交：见本文件提交后 `git rev-parse HEAD`
- 迁移：无
- 配置：无
- 依赖：无
- 生产数据/凭据：未触碰
- `downtime_required`：待根 `main` 发布预检

## 变更

- 在 Responses、Chat Completions、Messages 和 Embeddings OpenAI 兼容入口的账号选号前，复用分组 `ModelsListConfig` 作为启用且非空时的请求准入清单；列表外模型返回 `404/model_not_found`，不进入账号选择、上游调用或重试预算。
- 新增 `GroupAllowsOpenAIModel`，对请求模型做空白裁剪和大小写不敏感比较；禁用或空清单保持原生账号能力语义。
- 新增 OpenAI 404 分类：模型不存在、端点/路由/路径不存在、无法判断的裸 404。非模型 404 保留请求级诊断分类，但实际 HTTP 响应已证明请求发出，在执行/计费结果未知时禁止重放；模型 404 继续走原生账号-模型冷却。
- OpenAI 账号错误处理对非模型 404 只返回请求级 failover 信号，不写账号临时不可调度或账号-模型持久化状态；Antigravity 既有裸 404 fallback 未改变。
- 补齐 Gateway Responses、Gateway Chat Completions、Images、Responses Input Tokens 和 Messages Count Tokens 五个 HTTP 入口的分组模型准入，均发生在账号选号、计费或上游转发前；Responses WebSocket 按规格非目标未改动。

## 验证

- 通过：`go test ./internal/service -run 'Test(GroupAllowsOpenAIModelUsesEnabledNonEmptyListAsAdmission|ClassifyOpenAINotFoundSeparatesModelAndRouteErrors|ClassifyOpenAIUpstreamFailure|IsUpstreamModelNotFoundError)'`
- 通过：`go build ./cmd/server`
- 通过：`git diff --check`
- 通过：T108 重放安全回归测试 `TestClassifyOpenAIUpstreamFailure_NonModel404BlocksReplayWhenRequestWasSent`、`TestOpenAIUnifiedFailureSafetyBlocksSentHTTP404WithUnknownExecution`
- 通过：七个 OpenAI HTTP 入口的源码准入 wiring 测试 `TestOpenAIHTTPHandlersApplyGroupModelAdmissionBeforeRouting`
- 未通过/未归因于 T108：完整 `go test ./internal/service`，候选基线中已有多项 Codex seed、Monitor 空桶、Channel Monitor、CodexRadar 等测试失败。
- 未通过/未归因于 T108：`go test ./internal/handler`，候选基线中 `ProvideHandlers` 参数数量不匹配，且 `openAIAccountScheduleModel` 符号缺失。

## 回滚与风险

- 回滚：根总控合并后如需撤回，在根 `main` 上形成明确 revert；候选 worktree 和分支在发布失败时保留。
- 风险：严格准入依赖 API key 快照中已加载分组模型清单；旧缓存快照缺少清单时由现有缓存版本门禁拒绝，不应静默放行。
- 风险：非模型 404 的诊断仍保留请求级 transient 属性，但已发送 HTTP 404 在执行/计费未知时不会重放；已产生语义输出、usage 或副作用时同样不会重放。
- 未执行：验收站、主站、线上专项验收、推送和部署。
