# T40 错误码/边缘错误中文映射补齐交接

## 状态与基线

- 任务包：T40
- 状态：`READY_FOR_ROOT_REVIEW`
- 基线：`main@c1ac170c2dcbed9887904a0c48fef61e726401ab`
- 候选 worktree：`/Users/gongtengxinwen/.codex/worktrees/c92f/sub2api搭建`
- 候选分支：detached HEAD（未创建或修改根 `main`）
- 最终 HEAD：以本文件提交后的 `git rev-parse HEAD` 为准
- Brainstorming/规格：`docs/superpowers/specs/2026-08-24-t40-error-code-edge-projection-design.md`，已完成自审并记录 `APPROVED_BY_ROOT_RELEASE_CONTROLLER_PROXY`
- 计划：`docs/superpowers/plans/2026-08-24-t40-error-code-edge-projection.md`

## 实现摘要

- `ProjectNativeUserError` 新增应用侧 402、507、520–525 的状态与英文 marker/正文映射；消息固定中文，状态分支位于已选账号上游泛化之前。
- 保留 413 既有优先级与 T39 Responses 单次投影；499/client closed/upload interrupted 保持上传中断用户语义。
- `ProjectNativeErrorDiagnosis` 在管理员既有列表/详情诊断边界补充 499 上传中断和 402 额度限制的优先级；507/520–525 沿用 `upstream_failed`，不新增 class。
- 既有管理员证据脱敏逻辑不变；用户 JSON/SSE 不回显 URL、Cloudflare/Ray/request ID、密钥或原始正文。
- Cloudflare 在边缘生成且未进入应用的 HTML 413 明确不进入本实现或验收，不误判、不泄露。

## 变更文件

- `docs/superpowers/specs/2026-08-24-t40-error-code-edge-projection-design.md`
- `docs/superpowers/plans/2026-08-24-t40-error-code-edge-projection.md`
- `docs/handoffs/2026-08-24-t40-error-code-edge-projection-handoff.md`
- `upstream/sub2api/backend/internal/service/native_user_error_projection.go`
- `upstream/sub2api/backend/internal/service/native_user_error_projection_test.go`
- `upstream/sub2api/backend/internal/service/native_error_diagnostics.go`
- `upstream/sub2api/backend/internal/service/native_error_diagnostics_test.go`
- `upstream/sub2api/backend/internal/handler/native_user_error_writer_test.go`

## TDD 与验证证据

RED 已观察：新增 service 状态/marker 与诊断测试在旧实现上分别落入“服务暂时异常”、旧通用分类，499 未进入 `upload_interrupted`。

GREEN/验证（工作目录 `upstream/sub2api/backend`）：

- `go test ./internal/service -run 'Test(ProjectNativeUserError|ProjectNativeErrorDiagnosis|AttachNativeErrorDiagnosis)' -count=1`：通过。
- `go test ./internal/handler -run 'Test(OpenAIHandleStreamingAwareError|GatewayHandleStreamingAwareError|OpenAIBodyLimitFailoverExhausted|NativeUserError|ResponsesStreamingEmitsResponseFailed|MapResponsesErrorCode)' -count=1`：通过。
- `go build ./cmd/server`：通过。
- `gofmt`：已执行。
- `git diff --check`：通过。

## 范围与发布属性

- 无数据库迁移、配置、依赖、前端、生产数据、GitHub Actions 或全局队列/项目总账改动。
- 未合并 `main`、未推送、未部署、未访问生产；未运行全仓测试、前端测试、浏览器或线上专项验收。
- `downtime_required` 预期：`false`；最终值由根总控在合并后的 `main` 发布预检决定。
- 根总控应在唯一单车道中授权合并，运行既有本地/宿主蓝绿发布链，并验证应用可控 JSON/SSE 402/507/520–525、499 与健康端点；不得将边缘 HTML 413作为验收样本。

## 回滚与剩余风险

- 代码回滚：回退 T40 候选提交并重新走根发布链；运行回滚使用上一已验证蓝绿槽/镜像；无数据回滚。
- 520–525 只有在响应已进入应用时才可投影；边缘接管的 HTML 不可由应用证明或改写。
- 状态与正文 marker 同时出现时由状态/确定性分支优先；新增矩阵已锁定该顺序。最终线上真实上游样本仍待根总控验收。
