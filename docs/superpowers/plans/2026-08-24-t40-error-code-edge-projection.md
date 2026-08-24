# T40 错误码/边缘错误中文映射补齐实施计划

> Source: `docs/superpowers/specs/2026-08-24-t40-error-code-edge-projection-design.md`
> 状态：IMPLEMENTING；完成后仅进入 `READY_FOR_ROOT_REVIEW`。

## 任务

- [x] 1. 在 `upstream/sub2api/backend/internal/service/native_user_error_projection_test.go` 先加入 RED：402、507、520–525 的状态/关键词/正文矩阵、已选账号优先级、敏感正文不泄露，以及 499 用户语义。
- [x] 2. 在 `upstream/sub2api/backend/internal/service/native_user_error_projection.go` 实现最小状态与 marker 映射；保持 T39 的 413 优先、机器 type/code 保留和统一安全过滤。
- [x] 3. 在 `upstream/sub2api/backend/internal/service/native_error_diagnostics_test.go` 先加入 RED：499 `upload_interrupted`、402 本地额度/上游边界、507 与 520–525 的状态/正文分类和列表/详情一致性。
- [x] 4. 在 `upstream/sub2api/backend/internal/service/native_error_diagnostics.go` 扩展既有分类/解释函数，不新增诊断 class，不改变管理员脱敏边界。
- [x] 5. 在 `upstream/sub2api/backend/internal/handler` 相关直接测试加入 JSON、通用 SSE、Responses SSE 的映射合同；确认唯一 `response.failed`、机器 code 和用户消息，不回归 T39 413。
- [x] 6. 运行直接相关 Go 测试，修复失败并保持 TDD GREEN；运行 `go build ./cmd/server`、`gofmt`、`git diff --check`。
- [x] 7. 做 diff/范围自审：确认仅规格、计划、handoff、service/handler 测试与实现变更；无迁移、配置、前端、生产、GitHub Actions 或全局队列/总账变更。
- [x] 8. 写 handoff，绑定基线、最终 HEAD、变更文件、测试证据、未验证项、迁移/配置、`downtime_required` 预期、回滚和剩余风险；提交干净候选并将状态写为 `READY_FOR_ROOT_REVIEW`。

## 验证命令

在 `upstream/sub2api/backend` 执行：

```bash
go test ./internal/service -run 'Test(ProjectNativeUserError|NativeError)' -count=1
go test ./internal/handler -run 'Test(OpenAIBodyLimitFailoverExhausted|OpenAIHandleStreamingAwareError|GatewayHandleStreamingAwareError|MapResponsesErrorCode|NativeUserError)' -count=1
go build ./cmd/server
gofmt -w internal/service/native_user_error_projection.go internal/service/native_user_error_projection_test.go internal/service/native_error_diagnostics.go internal/service/native_error_diagnostics_test.go internal/handler/*error*test.go
git diff --check
```

## 验收

- [ ] 应用侧 402、507、520–525 的 JSON/SSE 用户消息稳定、中文、脱敏，并支持关键词/正文匹配。
- [ ] 499 保持客户端断开/上传中断分类；管理员诊断仅在既有管理员边界暴露脱敏状态和证据。
- [ ] Cloudflare 边缘 HTML 413 不进入测试或实现的误判路径；T39 413 行为保持。
- [ ] 直接相关测试、server build、gofmt、diff-check 全部通过；候选未合并/推送/部署。

## 风险

- 状态码和正文同时出现时的优先级必须由表驱动测试锁定，避免已选账号泛化覆盖确定性语义。
- 520–525 可能来自应用已接收的上游 HTML/文本或真正边缘响应；仅对已进入应用的输入分类，不能证明边缘未接管的响应。
- 预计无停机，但最终值留给根总控在合并后预检。
