# T140 实现报告

## 范围

本候选实现了 T140 规格：显式 `upstream_request_id_header` 优先；OpenAI API Key 账号使用合法非官方自定义 BaseURL 或已有可信 NewAPI 身份时，自动读取 `X-Oneapi-Request-Id`；NewAPI 倍率注册只有在 Sub 原生倍率探测明确为 `unsupported` 时才允许写入。

未修改数据库迁移、配置文件、生产账号、生产流水、凭据、部署链或 GitHub Actions。

## 基线与变更

- 基线：`14caa8b3c`（T140 规格与计划文档提交）
- 候选分支：`codex/t140-newapi-request-id-auto-detection`
- 变更文件：
  - `upstream/sub2api/backend/internal/service/upstream_request_id.go`
  - `upstream/sub2api/backend/internal/service/upstream_request_id_test.go`
  - `upstream/sub2api/backend/internal/service/newapi_rate_registration.go`
  - `upstream/sub2api/backend/internal/service/sub_upstream_cost.go`
  - `upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go`
  - `upstream/sub2api/backend/internal/service/usage_cost_evidence_test.go`

## 验证

- `go build ./internal/service`：通过。
- `go build ./cmd/server`：通过。
- `gofmt`：通过并已执行。
- `git diff --check`：通过。
- TDD RED：新增测试在实现前无法进入测试执行层；命中主线既有缺失符号 `resolveCompositeModelOwnership` 与 `decodeCodexManifestModels`。
- GREEN/聚焦测试：同一主线缺失符号仍阻断 `go test ./internal/service` 编译，未修改无关基线代码绕过。

## 发布与回滚

- 当前仅候选 worktree，未合入 `main`、未推送、未部署、未修改生产。
- 预期 `downtime_required=false`，但最终以根发布总控预检为准。
- 经明确授权发布后，必须从干净且与 `origin/main` 一致的根 `main` 构建，并用 SSH 核对 source commit/tree、image digest、健康状态、请求 ID 落库和倍率注册日志。
- 回滚使用上一已验证蓝绿槽或 Git 前向修复；保留已落库请求 ID 和历史倍率证据。

## 剩余风险

完整 service 测试仍受基线缺失符号阻断。自动识别不会回填历史用量；账号需产生新的自然请求后，才会开始记录自动识别的请求 ID。
