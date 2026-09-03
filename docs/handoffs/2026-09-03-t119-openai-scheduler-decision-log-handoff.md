# T119 OpenAI 调度决策日志交接

## 状态

`READY_FOR_ROOT_REVIEW`。候选分支：`codex/t119-openai-scheduler-decision-log`；候选 worktree：`.worktrees/t119-openai-scheduler-decision-log`；原基线：`main@bced1a52b`；已刷新至 `main@634b262a5`。

## 已实现

- 新增 `openai_scheduler_logs` 表及时间、逻辑请求、账号、分组索引，默认 7 天清理，不改 `usage_logs`。
- 现有 OpenAI resilience selection/outcome/failover 事件接入防御性复制的有界异步 sink；请求线程不等待数据库，队列满/写入失败保留健康计数。
- 新增管理员只读 API：`GET /api/v1/admin/scheduler/logs` 和 `GET /api/v1/admin/scheduler/logs/:logical_request_id`，列表按逻辑请求聚合并分页，详情返回完整 attempt 事件链；支持 1h/24h/7d、游标、分组/账号/结果/机制/关键词过滤。
- 侧边栏与路由从 `/admin/scheduler-settings` 改为 `/admin/scheduler-logs`；删除旧 SchedulerSettingsView 及其前端可见开关/额外恢复次数控件。
- 调度日志页按需加载详情，展示服务端实际算法版本、原生运行时重试预算、真实切号次数、候选/排除/评分分项/冷却/失败 attempt 时间线和日志缺口提示；不再回退到已失效 `extra_retry_count`。

## 验证

- `go test ./internal/service -run 'TestOpenAISchedulerLog|TestRecordOpenAISchedulerSelection' -count=1`：通过。
- `go test ./internal/handler/admin -run TestSchedulerLogHandler -count=1`：通过。
- `go test ./internal/handler -run TestAnnotateOpenAIUnifiedDecisionReportsNativeRuntimeBudget -count=1`：通过。
- `go test ./internal/repository -run TestOpenAISchedulerLogRepository -count=1`：通过。
- `go build ./cmd/server`：通过。
- 前端 `SchedulerLogsView`、路由、侧边栏聚焦测试：6/6 通过。
- 前端 `pnpm typecheck`：通过。
- `git diff --check`：通过。
- 全量 `go test ./internal/service ./internal/repository ./internal/handler/admin` 触发仓库既有基线失败（账号 Codex seed、channel monitor endpoint、CodexRadar 等），不属于 T119 变更；失败输出未作为本任务功能失败。

## 未验证与风险

- 未执行真实数据库集成测试、验收站/主站部署或线上功能验收。
- 当前算法版本常量为 `openai-multi-window-quality-v1`，与当前 T114 `1h + 24h + 7d` 运行语义一致，由服务端拥有。
- 新增 expand-only migration `234_openai_scheduler_logs.sql`；不回填历史、不改配置或现有业务数据。是否需要停机必须以根 `main` 生产发布预检结果为准。

## 合并/发布

等待根总控授权 `AUTHORIZE_MERGE_TO_MAIN`。未经根授权不得合并、推送、部署或清理候选。
