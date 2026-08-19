# T26 用户错误中文投影与 CodexRadar 站长推荐 handoff

- 状态：`READY_FOR_ROOT_REVIEW`
- 基线：`origin/main@ea49b1ee0032794ce966489df92de7c0445dd522`
- 实现提交：`93dcccb5c`（rebase 后） (rebased implementation)
- 实现 tree：`<superseded; see refreshed candidate below>`
- 分支：`codex/t26-localized-errors-codexradar`

## 精确范围

1. 新增 Sub 原生用户错误中文投影，区分本站余额/额度订阅与内部账号、外部服务异常；接入 Anthropic、Chat Completions、Responses 的 JSON/SSE writer，并保留管理员原始诊断链。
2. 新增固定 `GET https://codexradar.com/api/radar-insights` 的只读服务：3 秒超时、512 KiB 上限、schema/四分类/字段校验、60 秒缓存和最近成功快照回退。
3. 新增登录态 `GET /api/v1/monitor-v2/codexradar-insights`，前端在 Monitor V2 分组状态下方渲染四类深色推荐卡、真实模型/档位、IQ、耗时、费用、规则、更新时间及 loading/error/stale 状态。

## TDD 与验证

- RED：`native_user_error_projection` 初始因缺少类型/函数编译失败；协议 writer 初始返回英文；CodexRadar service/handler/前端 DTO 与组件初始因实现缺失失败。
- GREEN：
  - `go test ./internal/service -run 'Test(ProjectNativeUserError|CodexRadarInsights|ProjectNativeErrorDiagnosis)' -count=1`
  - `go test ./internal/handler -run 'Test(GatewayErrorResponseProjects|OpenAIErrorResponseHides|ResponsesStreamErrorProjects|AnthropicStreamErrorProjects|CodexRadarInsightsHandler|OpenAIHandleStreamingAwareError|OpenAIEnsureForwardErrorResponse)' -count=1`
  - `go test ./internal/server/routes -run 'TestMonitorV2' -count=1`
  - `go build ./cmd/server`
  - 前端 focused Vitest：3 files、15 tests 通过。
  - `pnpm typecheck` 通过。
  - `pnpm build` 通过（仅既有 Browserslist/chunk 警告）。
  - `git diff --check` 通过。

## 未执行项

- 未运行全仓测试、压力、mutation 或无关浏览器矩阵。
- 未执行 Playwright 1440/390 专项；根整合后可按发布门禁补做 Monitor 页面两视口核对。
- 未合并、推送、部署或访问生产。

## 发布属性

- 数据库迁移：无。
- 配置变化：无。
- 生产数据写入：无。
- GitHub Actions：无。
- 预期：`downtime_required=false`，最终以根 `main` 发布预检为准。
- 回滚：撤销实现提交，或由根发布总控切回上一蓝绿槽/镜像。

根发布总控可从上述实现提交审查范围；刷新后候选 HEAD：`75b78d3a9`；候选 tree：`<see git rev-parse HEAD^{tree}>`。

本任务停在 `READY_FOR_ROOT_REVIEW`。
