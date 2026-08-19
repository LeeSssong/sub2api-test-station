# T26-R1 CodexRadar 三标签社区测试矩阵实施计划

**目标：** 在现有 CodexRadar 四类站长推荐下方，使用两个固定公开只读接口补齐综合智能、软件工程能力和视觉空间推理矩阵。

**架构：** 新增与 recommendations 隔离的 `CodexRadarCommunityService` 及同域登录态 endpoint。服务端分别校验 software/visual DTO，按 CodexRadar 原站口径计算 comprehensive DTO；前端在原组件内添加独立社区矩阵子区域。

## Task 1：后端 service RED

**文件：**
- 新增 `upstream/sub2api/backend/internal/service/codexradar_community_test.go`

1. 写固定 GET 目标测试，同时要求 `/api/intelligence-efficiency-metrics` 与 `/api/visual-spatial-reasoning`。
2. 写精确组合口径测试：IQ 几何平均，费用/耗时按样本数加权，样本数求和，只保留交集档位。
3. 写 60 秒缓存、过期失败快照回退、无快照错误、无效 schema/字段/重复档位/超大响应拒绝测试。
4. 运行 `go test ./internal/service -run 'TestCodexRadarCommunity' -count=1`，确认 RED。

## Task 2：后端 service GREEN

**文件：**
- 新增 `upstream/sub2api/backend/internal/service/codexradar_community.go`
- 修改 `upstream/sub2api/backend/internal/service/wire.go`

1. 实现两个独立 wire DTO 和对外 community DTO。
2. 复用 3 秒 HTTP client 策略，每个响应使用 `LimitReader(512KiB+1)`。
3. 实现严格校验、稳定排序、综合口径、60 秒缓存与快照回退。
4. 运行 Task 1 命令确认 GREEN，执行 gofmt。

## Task 3：handler / DI / route RED-GREEN

**文件：**
- 新增 `upstream/sub2api/backend/internal/handler/codexradar_community_handler.go`
- 新增 `upstream/sub2api/backend/internal/handler/codexradar_community_handler_test.go`
- 修改 `upstream/sub2api/backend/internal/handler/handler.go`
- 修改 `upstream/sub2api/backend/internal/handler/wire.go`
- 修改 `upstream/sub2api/backend/internal/server/routes/user.go`
- 修改 `upstream/sub2api/backend/internal/server/routes/monitor_v2_routes_test.go`
- 修改 `upstream/sub2api/backend/cmd/server/wire_gen.go`

1. 先写 handler 成功/stale/503 与 route 登录态测试。
2. 运行聚焦 Go 测试确认 RED。
3. 接入 provider、Handlers 字段与 `GET /monitor-v2/codexradar-community`，复用 `panelRateLimiter.Heavy()`。
4. 运行 handler/routes 聚焦测试确认 GREEN。

## Task 4：前端 DTO RED-GREEN

**文件：**
- 新增 `upstream/sub2api/frontend/src/features/monitor-v2/codexRadarCommunity.ts`
- 新增 `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/codexRadarCommunity.spec.ts`

1. 先写三标签顺序、点数边界、数字/时间/组合字段校验和固定 endpoint 测试。
2. 运行单文件 Vitest 确认 RED。
3. 实现 TypeScript DTO/parser/API，不接受动态上游目标。
4. 重跑确认 GREEN。

## Task 5：三标签矩阵组件 RED-GREEN

**文件：**
- 新增 `upstream/sub2api/frontend/src/features/monitor-v2/CodexRadarCommunityMatrix.vue`
- 新增 `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/CodexRadarCommunityMatrix.spec.ts`
- 修改 `upstream/sub2api/frontend/src/features/monitor-v2/CodexRadarRecommendations.vue`
- 修改 `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/CodexRadarRecommendations.spec.ts`

1. 先写默认综合、三标签切换、全量卡、样本/IQ/费用/耗时、更新时间、众测说明、stale、失败隔离和窄屏容器合同测试。
2. 运行组件 Vitest 确认 RED。
3. 实现子组件，在既有推荐下方挂载；使用外层容器防溢出与内部受控横向滚动。
4. 重跑确认 GREEN。

## Task 6：直接相关验证、刷新主线与交接

1. 运行 CodexRadar service/handler/routes 聚焦 Go 测试；必要时运行受影响包编译与 server build。
2. 运行 CodexRadar DTO/组件 Vitest、`pnpm typecheck`、`pnpm build`。
3. 执行 gofmt 与 `git diff --check`，自审不包含迁移、生产、全局队列/总账或 GitHub Actions 变更。
4. 在候选提交后将最新 `main@80b4b57a13f29e5c25e33ce93daca78b44bcf4a2` 刷新进候选，仅解决本任务范围冲突，不编辑全局队列/进度文件。
5. 在刷新后重跑上述直接相关验证，提交并确认 worktree 干净。
6. 交接状态只报 `READY_FOR_ROOT_REVIEW`，包含 baseline、candidate SHA/tree、变更文件、测试、未验证项、迁移/配置、`downtime_required=false`、回滚与风险。
