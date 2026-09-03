# OpenAI 调度决策日志 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 OpenAI 调度的真实决策与 attempt 因果链以不阻塞请求的异步日志形式持久化，并提供管理员调度日志页面。

**Architecture:** 复用现有 `OpenAIResilienceEvent` 和请求 attempt context，在选择/失败/最终结果发出紧凑事件；有界队列异步批量写入新的调度日志表。管理员 API 按游标返回摘要，详情按逻辑请求聚合 attempt 时间线；前端替换原调度设置入口并按需展开详情。

**Tech Stack:** Go、Gin、Ent/PostgreSQL migration、Vue 3、TypeScript、Vitest。

**Spec:** `docs/superpowers/specs/2026-09-03-openai-scheduler-decision-log-design.md`

## Global Constraints

- 仅覆盖 OpenAI/Codex 普通 HTTP 文本调度；生图、Responses WebSocket、alpha-search 和其他平台行为不变。
- 日志写入尽力异步，不得阻塞请求、选择、重试、账务或响应；队列满允许丢失但必须可观测。
- 不落完整请求体、Authorization、API key、凭据、提示词或原始敏感响应。
- 不修改 `usage_logs`，不回填历史事件；默认保留 7 天。
- 不修改根 `main`、全局队列、项目进度总账或生产环境。

### Task 1: 事件持久化与异步队列

**Files:**
- Create: `upstream/sub2api/backend/ent/schema/openai_scheduler_log.go`
- Create: `upstream/sub2api/backend/migrations/*openai_scheduler_logs.sql`
- Modify: `upstream/sub2api/backend/internal/service/openai_resilience_observability.go`
- Create: `upstream/sub2api/backend/internal/service/openai_scheduler_log_sink.go`
- Test: `upstream/sub2api/backend/internal/service/openai_scheduler_log_sink_test.go`

**Interfaces:**
- Consumes: `OpenAIResilienceEvent`, `RecordOpenAIResilienceOutcomeWithContext`。
- Produces: `OpenAISchedulerLogSink.Enqueue(OpenAIResilienceEvent)`, `ListOpenAISchedulerLogs` persistence contract and cleanup worker。

- [ ] Write tests for bounded non-blocking enqueue, event field copying, batch write failure/drop counters, and 7-day cleanup predicate.
- [ ] Run the focused service tests and confirm they fail before implementation.
- [ ] Add the Ent schema/migration with indexed timestamp, logical request ID, attempt ID, group/account, outcome; store controlled JSON snapshots and algorithm version.
- [ ] Implement a singleton sink with bounded channel, background batch flush, explicit shutdown for tests, drop/error counters, and no request-path database wait.
- [ ] Wire existing resilience event recording to enqueue only OpenAI scheduler selection/outcome/failover events and preserve process-local ledger behavior.
- [ ] Run focused tests, schema generation/checks, gofmt, and diff-check.
- [ ] Commit `feat: persist openai scheduler decision events`.

### Task 2: Administrator scheduler-log API

**Files:**
- Create: `upstream/sub2api/backend/internal/handler/admin/scheduler_log_handler.go`
- Modify: `upstream/sub2api/backend/internal/server/routes.go` (or current admin route registration file)
- Modify: `upstream/sub2api/backend/internal/handler/dto/types.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/scheduler_log_handler_test.go`

**Interfaces:**
- Consumes: persisted scheduler log repository/sink query methods.
- Produces: `GET /admin/scheduler/logs` cursor list and `GET /admin/scheduler/logs/:logical_request_id` timeline detail.

- [ ] Add handler tests for time range/default 1h, cursor/limit bounds, filters, redaction, successful and failed requests, and incomplete-log indicator.
- [ ] Run focused handler tests to establish RED.
- [ ] Implement DTOs exposing algorithm version, selection mechanism, candidate/exclusion summaries, selected account/rank/score deltas, runtime retry budget, actual switch count, attempt status, final outcome, and completeness.
- [ ] Register admin-authenticated read-only routes and repository queries with stable ordering by event time plus ID.
- [ ] Run focused handler tests and server build.
- [ ] Commit `feat: expose scheduler decision log api`.

### Task 3: Frontend navigation and decision-log view

**Files:**
- Rename/replace: `upstream/sub2api/frontend/src/views/admin/SchedulerSettingsView.vue` with scheduler log view or create `SchedulerLogsView.vue`
- Modify: `upstream/sub2api/frontend/src/router/index.ts`
- Modify: `upstream/sub2api/frontend/src/components/layout/AppSidebar.vue`
- Modify: `upstream/sub2api/frontend/src/api/admin/index.ts` (or matching API module)
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`, `en/admin/index.ts`, `zh/common.ts`, `en/common.ts`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/SchedulerLogsView.spec.ts`

**Interfaces:**
- Consumes: scheduler-log list/detail API DTOs.
- Produces: `/admin/scheduler-logs` route and sidebar label `调度日志`/`Scheduler Logs`.

- [ ] Add component tests asserting old toggle/retry controls are absent, list renders actual algorithm/version/budget/switch count, detail timeline includes failed attempts, and incomplete state is visible.
- [ ] Run focused Vitest to establish RED.
- [ ] Implement cursor-paginated list with 1h/24h/7d ranges and filters; load detail only on selected row.
- [ ] Render compact dark-workbench layout with candidate explanation, attempt timeline, mobile stacking, loading/error/empty/incomplete states; remove save/toggle behavior.
- [ ] Update route metadata and all navigation/localized labels from scheduler settings to scheduler logs.
- [ ] Run focused Vitest, frontend typecheck, and production build.
- [ ] Commit `feat: replace scheduler settings with decision logs`.

### Task 4: Integration verification and handoff

**Files:**
- Modify: `docs/handoffs/2026-09-03-t119-openai-scheduler-decision-log-handoff.md`

- [ ] Run direct Go service/handler tests, server build, frontend focused tests, typecheck, and diff-check from the candidate worktree.
- [ ] Inspect the diff for legacy setting controls, sensitive-field leakage, blocking writes, and non-OpenAI behavior changes.
- [ ] Record migration/config/data-write status and known verification gaps in the handoff.
- [ ] Commit the handoff and report `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or modify root ledgers.

