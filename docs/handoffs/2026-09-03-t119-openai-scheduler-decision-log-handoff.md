# T119 OpenAI 调度决策日志交接

## 状态

`READY_FOR_ROOT_REVIEW`。候选分支：`codex/t119-openai-scheduler-decision-log`；候选 worktree：`.worktrees/t119-openai-scheduler-decision-log`；基线：`main@bced1a52b`。

## 已实现

- 新增 `openai_scheduler_logs` 表及时间、逻辑请求、账号、分组索引，默认 7 天清理，不改 `usage_logs`。
- 现有 OpenAI resilience selection/outcome/failover 事件接入防御性复制的有界异步 sink；请求线程不等待数据库，队列满/写入失败保留健康计数。
- 新增管理员只读 API：`GET /api/v1/admin/scheduler/logs` 和 `GET /api/v1/admin/scheduler/logs/:logical_request_id`，支持时间范围、游标、分组/账号/结果/机制/关键词过滤。
- 侧边栏与路由从 `/admin/scheduler-settings` 改为 `/admin/scheduler-logs`；删除旧 SchedulerSettingsView 及其前端可见开关/额外恢复次数控件。
- 调度日志页按需加载详情，展示实际算法版本、运行时重试预算、真实切号次数、候选/失败 attempt 时间线和日志缺口提示。

## 验证

- `go test ./internal/service -run 'TestOpenAISchedulerLog|TestRecordOpenAISchedulerSelection' -count=1`：通过。
- `go test ./internal/handler/admin -run TestSchedulerLogHandler -count=1`：通过。
- `go build ./cmd/server`：通过。
- 前端 `SchedulerLogsView`、路由、侧边栏聚焦测试：6/6 通过。
- 前端 `pnpm typecheck`：通过。
- `git diff --check`：通过。
- 全量 `go test ./internal/service ./internal/repository ./internal/handler/admin` 触发仓库既有基线失败（账号 Codex seed、channel monitor endpoint、CodexRadar 等），不属于 T119 变更；失败输出未作为本任务功能失败。

## 未验证与风险

- 未执行真实数据库集成测试、验收站/主站部署或线上功能验收。
- 当前算法版本常量为 `openai-multi-window-quality-v1`，由服务端拥有；根审查时应与当时生产 T114 版本确认一致。
- API 列表返回原始事件行摘要；若根要求严格按逻辑请求去重，可在整合阶段增加 SQL 聚合，但当前详情链已按 logical request ID 聚合。
- 本候选未修改根 `main`、项目进度总账、任务队列、生产配置或生产数据。

## 合并/发布

等待根总控授权 `AUTHORIZE_MERGE_TO_MAIN`。未经根授权不得合并、推送、部署或清理候选。

