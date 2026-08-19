# T26 用户错误中文投影与 CodexRadar 站长推荐实施计划

**目标：** 在 Sub 原生网关错误写入链统一输出脱敏中文，并在 Monitor V2 分组状态下方接入 CodexRadar 四类原始推荐。

**架构：** `service` 包提供唯一的用户错误投影；各协议 writer 在写出边界调用。CodexRadar 使用独立固定目标 service + handler + 用户路由，内存短缓存与最近成功回退；前端独立 DTO parser/API/component，不改变 Monitor snapshot 合同。

## 任务 1：统一错误投影 RED/GREEN

**文件：**
- 新增 `upstream/sub2api/backend/internal/service/native_user_error_projection_test.go`
- 新增 `upstream/sub2api/backend/internal/service/native_user_error_projection.go`
- 修改 `upstream/sub2api/backend/internal/service/native_error_diagnostics.go`

1. 先写表驱动测试，调用期望 API：

```go
got := ProjectNativeUserError(NativeUserErrorInput{
    Status: http.StatusForbidden, Type: "billing_error",
    Message: "Insufficient balance", Ownership: "client",
})
require.Equal(t, "余额不足，请充值后重试。", got.Message)
```

覆盖余额/额度订阅/认证/频率并发/权限/格式/内容过大/本地资源/已选账号余额/5xx，并断言敏感标记集合不在 `got.Message`。
2. 运行 `go test ./internal/service -run 'TestProjectNativeUserError' -count=1`，记录因符号缺失而 RED。
3. 实现 `NativeUserErrorInput/Projection` 与 `ProjectNativeUserError`；复用 `containsAnyNativeErrorMarker`、本地限制标记与阶段/归属语义，最后执行中文/敏感标记守卫。
4. 重跑 focused test 至 GREEN，gofmt。

## 任务 2：三协议 JSON/SSE 写入链 RED/GREEN

**文件：**
- 修改 `internal/handler/openai_gateway_handler_test.go`
- 修改 `internal/handler/stream_error_event_test.go`
- 修改/新增 `internal/handler/native_user_error_projection_test.go`
- 修改 `internal/service/openai_gateway_chat_completions_test.go`
- 修改 `internal/service/openai_gateway_messages_failed_response_test.go`
- 修改 `internal/handler/gateway_handler.go`
- 修改 `internal/handler/openai_gateway_handler.go`
- 修改 `internal/handler/stream_error_event.go`
- 修改 `internal/service/openai_gateway_chat_completions.go`
- 修改 `internal/service/openai_gateway_messages.go`

1. 先增加测试：Anthropic JSON、Chat JSON、Responses JSON；三者 stream-started error；Responses `response.failed`；Chat/Anthropic 服务层 SSE helper。输入包含英文、URL、Ray/request id 与内部余额，期望固定中文且原文缺失。
2. 运行对应 handler/service focused tests，确认当前英文响应导致 RED。
3. 增加 handler 级小 helper，把 status/type/code/message 与 ops 上下文映射到 `service.ProjectNativeUserError`；在 `errorResponse`、`anthropicErrorResponse`、`handleStreamingAwareErrorWithCode` 和 `writeResponsesFailedSSE` 写出前调用。
4. 让 `buildChatStreamErrorSSE`、`buildAnthropicStreamErrorSSE` 接收已投影值或内部调用同一 projector；保持 envelope 与机器码不变。
5. 重跑 focused tests 至 GREEN；验证管理员 `native_error_diagnostics_test.go` 仍通过。

## 任务 3：CodexRadar service RED/GREEN

**文件：**
- 新增 `internal/service/codexradar_insights.go`
- 新增 `internal/service/codexradar_insights_test.go`

1. 用注入的 `http.Client`/`RoundTripper` 写失败测试：成功解析四类 DTO；固定 `GET https://codexradar.com/api/radar-insights`；缓存 60 秒不重复请求；过期超时/非 2xx/非法 JSON/非法字段/超大响应回退最近快照；无快照返回 sentinel unavailable。
2. 运行 `go test ./internal/service -run 'TestCodexRadarInsights' -count=1`，记录 RED。
3. 实现常量目标、3 秒 client、512 KiB `LimitReader`、严格 DTO validator、mutex 缓存 `{value,fetchedAt}` 与 `stale` 返回。
4. 重跑测试至 GREEN并 gofmt。

## 任务 4：CodexRadar handler、DI 与固定用户路由 RED/GREEN

**文件：**
- 新增 `internal/handler/codexradar_insights_handler.go`
- 新增 `internal/handler/codexradar_insights_handler_test.go`
- 修改 `internal/handler/handler.go`
- 修改 `internal/handler/wire.go`
- 修改 `internal/service/wire.go`
- 修改 `internal/server/routes/user.go`
- 修改/新增 `internal/server/routes/codexradar_insights_routes_test.go`
- 按项目生成方式更新 Wire 生成文件（若仓库要求）。

1. 先写 handler/route 测试：成功返回 `stale=false`、快照回退 `stale=true`、无快照 503；POST 与带任意 query 不会改变目标且 POST 为 404/405；路由位于登录态 Monitor V2 下。
2. 运行 handler/routes focused test，确认 RED。
3. 接入 `CodexRadarInsightsService`、handler 字段与 `GET /monitor-v2/codexradar-insights`；复用现有 authenticated 与 `panelRateLimiter.Heavy()`。
4. 重跑至 GREEN并编译受影响包。

## 任务 5：前端 DTO/API RED/GREEN

**文件：**
- 新增 `frontend/src/features/monitor-v2/codexRadar.ts`
- 新增 `frontend/src/features/monitor-v2/__tests__/codexRadar.spec.ts`

1. 先测试真实四分类 fixture 的解析和 API 请求；断言 key 顺序、model/effort、IQ、分钟、美元、时间、rule 与 stale；非法分类/字段 fail closed。
2. 运行 focused Vitest，记录缺少实现的 RED。
3. 实现 TypeScript DTO、严格 parser 与 `getCodexRadarInsights(signal?)`，请求固定 `/monitor-v2/codexradar-insights`。
4. 重跑至 GREEN。

## 任务 6：推荐卡组件与 Monitor 插入 RED/GREEN

**文件：**
- 新增 `frontend/src/features/monitor-v2/CodexRadarRecommendations.vue`
- 新增 `frontend/src/features/monitor-v2/__tests__/CodexRadarRecommendations.spec.ts`
- 修改 `frontend/src/features/monitor-v2/MonitorV2View.vue`
- 修改 `frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`
- 修改 `frontend/src/i18n/locales/zh/dashboard.ts` 与 `en/dashboard.ts`（或实际 Monitor V2 文案文件）。

1. 先挂载真实 API DTO，断言四类标题/配色标记、8 个模型档位、IQ、耗时、金额、更新时间、规则；分别测试 loading、unavailable、stale；Monitor 测试断言组件位于分组区域之后。
2. 运行 focused Vitest，确认 RED。
3. 实现深色四卡、截图配色、受控响应式布局、loading/error/stale；组件自身请求失败不发 `fatal`。
4. 重跑至 GREEN。

## 任务 7：直接相关验证与视觉修正

1. 后端：运行 projector、handler streaming、Chat/Anthropic service、CodexRadar service/handler/route focused tests；运行受影响 Go 包 compile 与 server build；gofmt。
2. 前端：运行新增/受影响 Vitest、`pnpm typecheck`、`pnpm build`。
3. 启动本地候选，以 deterministic CodexRadar fixture 截取 Monitor 页面 1440 与 390；检查推荐位于分组后、四类配色、长规则换行、无整页横向溢出。
4. 运行 `git diff --check`、迁移/GitHub Actions 范围扫描。
5. 若视觉或功能失败，先补失败测试再修正并重跑直接相关门禁。

## 任务 8：候选提交与 handoff

1. 确认未修改 `docs/project/project-progress.md`、`docs/project/native-sub-task-package-queue.md`、根 main、发布证据和生产记录。
2. 提交规格、计划、代码、测试与视觉证据索引；工作区保持干净。
3. 写 `docs/handoffs/2026-08-19-t26-localized-errors-codexradar-handoff.md`，列出 baseline、candidate HEAD/tree、精确范围、RED/GREEN、测试、未执行项、无迁移/配置、`downtime_required=false` 预期、回滚和剩余风险。
4. 状态仅声明 `READY_FOR_ROOT_REVIEW`，等待根发布总控。

## 计划自审

- 规格覆盖：错误分类、敏感隐藏、三协议 JSON/SSE、管理员证据、代理固定目标/校验/缓存回退、四卡真实 DTO、响应式与失败态均有任务。
- 占位扫描：不存在 TBD/TODO/“类似处理”；每个生产步骤都有具体文件、API、命令与失败条件。
- 类型一致性：后端统一 `NativeUserErrorInput/Projection`；代理统一 `CodexRadarInsights`；前端 parser 与 endpoint DTO 一致；Monitor snapshot 合同保持独立。
