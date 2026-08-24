# T39 Responses 413 二次错误投影修复交接

## 状态与提交

- 状态：`READY_FOR_ROOT_REVIEW`，尚未合并、推送、部署或线上验证。
- 功能基线：`main@5ded56aac949b6f1b8dced8a384b3761a54b39f5`。
- 工作区：`/Users/gongtengxinwen/.codex/worktrees/98b0/sub2api搭建`，保持 detached HEAD，未创建或修改功能分支。
- 最终 detached HEAD：以本文件随候选提交后运行的 `git rev-parse HEAD` 为权威值，由顶层任务交接消息精确报告。提交对象无法在自身内容中自引用其最终 SHA。
- 根总控状态：根 `main` 后续仅因 T39 全局登记文档推进到 `e1a41d4d3`；业务源码相对功能基线未变化。候选未修改 `docs/project`，根总控在合并前刷新并处理其文档差异。

## 变更摘要

- `ProjectNativeUserError` 将 HTTP 413/明确 oversized marker 放在通用已选账号/上游泛化之前，应用内上游 413 不再变成“服务暂时异常”。
- Responses `response.failed` writer 接收原始 status/type/code/message，并在序列化前只执行一次用户错误投影；不再以固定 502 二次投影。
- OpenAI 与 Anthropic-backed Responses 流都沿用同一单次投影；非 Responses SSE 仍在原位置投影，普通选中账号 5xx 继续为脱敏“服务暂时异常”。
- 上游 failover 耗尽 JSON 保持 HTTP 413 与 `invalid_request_error`；Responses SSE 保持唯一 `response.failed` 与 `invalid_request`，两者 message 均为“请求内容过大，请缩短内容后重试。”。
- 413 回归测试增加已选择账号上下文、唯一终止事件、机器分类与 `must-not-leak` 脱敏断言。

## 变更文件

- `docs/superpowers/specs/2026-08-24-t39-responses-413-projection-design.md`
- `docs/superpowers/plans/2026-08-24-t39-responses-413-projection.md`
- `docs/handoffs/2026-08-24-t39-responses-413-projection-handoff.md`
- `upstream/sub2api/backend/internal/service/native_user_error_projection.go`
- `upstream/sub2api/backend/internal/service/native_user_error_projection_test.go`
- `upstream/sub2api/backend/internal/handler/stream_error_event.go`
- `upstream/sub2api/backend/internal/handler/stream_error_event_test.go`
- `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- `upstream/sub2api/backend/internal/handler/gateway_handler.go`
- `upstream/sub2api/backend/internal/handler/openai_body_limit_failover_test.go`

## TDD 证据

RED：

- `go test ./internal/service -run 'TestProjectNativeUserError' -count=1`：失败；`selected_account_too_large` 期望“请求内容过大”，实际“服务暂时异常”。
- `go test ./internal/handler -run 'TestOpenAIBodyLimitFailoverExhausted' -count=1`：JSON 与 Responses SSE 均失败；实际为“服务暂时异常”。

GREEN：

- `go test ./internal/service -run 'TestProjectNativeUserError' -count=1`：通过。
- `go test ./internal/handler -run 'TestOpenAIBodyLimitFailoverExhausted' -count=1`：通过。
- `go test ./internal/handler -run 'Test(OpenAIHandleStreamingAwareError|GatewayHandleStreamingAwareError|OpenAIBodyLimitFailoverExhausted|MapResponsesErrorCode|OpenAIErrorResponse|NativeUserError)' -count=1`：通过。
- `go test ./internal/handler -run 'Test(OpenAIBodyLimitFailoverExhausted|OpenAIHandleStreamingAwareError|GatewayHandleStreamingAwareError|InboundIsResponses|SynthesizeResponseID|MapResponsesErrorCode)' -count=1`：通过。
- `go build ./cmd/server`：通过。
- `gofmt`：已执行。
- `git diff --check`：通过。

## 范围、迁移与配置

- `.github`、`ops`、`docs/project`、迁移目录均零差异。
- 无数据库迁移、历史回填、配置、依赖、前端或生产数据变化。
- 不含 T40 的其他状态码/正文映射；不处理未进入应用的 Cloudflare HTML 413。
- 未运行全仓测试、前端测试、浏览器或生产验收，符合直接相关最小验证政策。

## 发布属性与回滚

- 预期 `downtime_required=false`，最终以根总控在合并后 `main` 的发布预检为准。
- 只能由根总控刷新、授权合并并从已验证 `main` 走既有本地/宿主蓝绿发布链；禁止 GitHub Actions。
- 代码回滚：回退 T39 候选提交并重新走根发布链。
- 运行回滚：使用上一已验证蓝绿槽/镜像；没有数据或迁移回滚。

## 未验证项与剩余风险

- 未访问生产，未验证自然或受控生产 413；由根发布后最小线上专项验收闭环。
- Cloudflare HTML 413 明确不在应用接管范围，不能作为 T39 失败或通过证据。
- `ProjectNativeUserError` 的明确 oversized marker 也会优先于已选账号泛化，这是既有 413/context-window 语义的统一顺序调整；直接 service 测试保留普通已选账号 429/502 泛化，降低相邻回归风险。
- Responses writer 内部签名变化已由 handler 包编译和直接协议回归覆盖；未扩大到全仓测试。
