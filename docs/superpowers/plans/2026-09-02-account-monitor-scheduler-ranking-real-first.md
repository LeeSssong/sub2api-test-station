# 账号监控调度排名与真实请求优先性能监控 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 统一账号监控与原生调度排名，恢复真实请求优先的性能投影，并修复首页 internal error 错误态。

**Architecture:** 保留单一账号监控聚合接口，后端从原生分组调度投影生成分组排名和全站最佳分组排名。性能 SQL 在五分钟桶内执行真实请求优先选择；前端保留最近成功快照并区分加载失败与真实空池。

**Tech Stack:** Go、PostgreSQL、Vue 3、TypeScript、Vitest。

**Spec:** `docs/superpowers/specs/2026-09-02-account-monitor-scheduler-ranking-real-first-design.md`

## Global Constraints

- 不改写原生调度算法，只消费其排名投影。
- 主动探测仅补足没有真实请求的五分钟桶。
- 首次失败不得显示“全站 0”或英文 `internal error`。
- 无迁移、无生产数据写入、无推送或部署。

---

### Task 1: 修复账号监控仓储投影

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`

- [x] 增加失败测试，要求累计真实请求接口存在、时间线按真实请求优先补探测、分组 SQL 的 request bridge 直接读取 `usage_logs`。
- [x] 运行 repository 定向测试确认失败。
- [x] 补回累计计数和真实请求优先时间线实现，修正分组 SQL 未定义字段。
- [x] 运行 repository 定向测试确认通过。

### Task 2: 固化调度排名合同

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`

- [x] 更新旧的“全站不返回调度排名”测试为“全站返回最佳分组调度排名”。
- [x] 验证分组排名、全站最佳分组排名、未排名置后和 ID tie-break。
- [x] 保持旧质量证据字段兼容但不参与主排序。

### Task 3: 修复首页错误态

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

- [x] 增加失败测试：首次 `internal error` 映射成中文，且不显示“全站 0”。
- [x] 实现未知计数 `--`、中文错误归一化与旧快照保留。
- [x] 验证刷新失败仍保留已渲染卡片。

### Task 4: 完整验证

- [x] 运行 AccountMonitor/MonitorV4 后端定向测试。
- [x] 运行账号监控和 Monitor V4 前端定向测试。
- [x] 运行 `go build ./cmd/server`、`pnpm typecheck`、`pnpm build` 和 `git diff --check`。
- [x] 检查工作区只包含规格、计划、实现和直接测试，停在可发布状态。
