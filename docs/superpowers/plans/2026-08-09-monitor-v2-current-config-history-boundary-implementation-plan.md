# Monitor V2 当前监控集合与历史边界 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `/monitor` 只展示当前启用且绑定有效活动分组的监控，并让监控身份配置变化后的状态、可用率和时间线从新的统计起点重新累计。

**Architecture:** 在 `channel_monitors` 增加持久化的 `history_started_at`，保留原始历史但在当前状态聚合 SQL 中过滤统计起点之前的数据。Monitor V2 继续使用 enabled monitor user views 作为探测事实源，并把活动分组集合过滤为能与这些 views 匹配的分组。

**Tech Stack:** Go 1.25、Ent、PostgreSQL 18、Gin、Vue 3、TypeScript、Vitest、pnpm。

## Global Constraints

- 不物理删除 `channel_monitor_histories`，管理员原始历史接口继续可审计旧记录。
- 迁移对所有存量 `channel_monitors` 把 `history_started_at` 设置为迁移执行时刻，并设为非空、默认当前时间。
- provider、API mode、规范化 endpoint、主模型、非空新 API Key 或 `group_id` 变化时推进统计起点；名称、间隔、抖动、启用开关、模板、附加模型变化不推进。
- `/monitor` 只返回 enabled monitor 通过 `group_id` 或 legacy `group_name` 匹配到的活动分组；public 仍排除专属分组，admin 可包含已监控专属分组。
- 不修改分组倍率、账号、路由、调度、客户计费或用户用量。
- 不引入 GitHub Actions；生产发布使用现有本地/宿主蓝绿脚本链。

---

### Task 1: 建立监控历史统计起点

**Files:**
- Create: `upstream/sub2api/backend/migrations/201_channel_monitor_history_started_at.sql`
- Modify/regenerate: `upstream/sub2api/backend/ent/schema/channel_monitor.go`
- Modify/regenerate: `upstream/sub2api/backend/ent/**`
- Modify: `upstream/sub2api/backend/internal/service/channel_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/channel_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/repository/channel_monitor_repo.go`
- Test: `upstream/sub2api/backend/internal/service/channel_monitor_history_boundary_test.go`
- Test: `upstream/sub2api/backend/internal/repository/channel_monitor_history_boundary_test.go`
- Test: `upstream/sub2api/backend/migrations/channel_monitor_history_started_at_migration_test.go`

**Interfaces:**
- Produces: `ChannelMonitor.HistoryStartedAt time.Time`.
- Produces: `channelMonitorIdentityChanged(before, after *ChannelMonitor, apiKeyUpdated bool) bool`.
- Consumes: existing `ChannelMonitorService.Update`, `ListLatestForMonitorIDs`, `ComputeAvailabilityForMonitors`, `ListRecentHistoryForMonitors`, and their single-monitor counterparts.

- [ ] **Step 1: 写失败的服务测试**

新增表驱动测试，构造固定 `before.HistoryStartedAt`，分别更新 provider、API mode、endpoint、primary model、API Key 和 group ID，断言 repository 收到的 `HistoryStartedAt` 晚于旧值；更新 name、interval、jitter、enabled、template、extra models 时断言时间不变。测试必须通过注入时钟或比较明确的前后范围避免睡眠。

- [ ] **Step 2: 运行服务测试确认 RED**

Run: `go test ./internal/service -run 'TestChannelMonitorUpdateHistoryBoundary' -count=1`

Expected: FAIL，因为 `ChannelMonitor` 尚无 `HistoryStartedAt` 或更新不会推进它。

- [ ] **Step 3: 写失败的 repository 与迁移测试**

Repository 测试必须断言 current-state SQL 含以下语义，而原始 `ListHistory` 不受影响：

```sql
JOIN channel_monitors cm ON cm.id = h.monitor_id
AND h.checked_at >= cm.history_started_at
```

迁移测试断言：列为 `timestamptz`、`NOT NULL`、默认 `NOW()`，且迁移包含存量回填。

- [ ] **Step 4: 运行 repository/迁移测试确认 RED**

Run: `go test ./internal/repository ./migrations -run 'HistoryStartedAt|HistoryBoundary' -count=1`

Expected: FAIL，因为 schema、迁移和 SQL 过滤尚不存在。

- [ ] **Step 5: 实现最小持久化与身份判定**

在 schema/service model 增加：

```go
HistoryStartedAt time.Time
```

更新路径在 `applyMonitorUpdate` 前复制身份字段，API Key 成功加密后把 `apiKeyUpdated` 传入判定；判定为 true 时设置注入时钟或 `time.Now().UTC()`。创建、复制监控都从当前时间开始，repository Create/Update/映射完整读写该字段。

- [ ] **Step 6: 实现 current-state SQL 过滤**

批量和单项 latest、availability、recent timeline 查询都 join `channel_monitors` 并增加 `checked_at >= history_started_at`；管理员显式 `ListHistory` 保持仅按 monitor ID/model 查询。

- [ ] **Step 7: 生成 Ent 并运行 GREEN**

Run: `go generate ./ent`

Run: `go test ./internal/service ./internal/repository ./migrations -run 'ChannelMonitor|HistoryStartedAt|HistoryBoundary' -count=1`

Expected: PASS。

- [ ] **Step 8: 运行任务级验证并提交**

Run: `go test ./internal/service ./internal/repository ./migrations -count=1`

Run: `go vet ./internal/service ./internal/repository`

Run: `git diff --check`

Commit: `fix: reset channel monitor statistics on identity changes`

---

### Task 2: 让 Monitor V2 只返回当前已监控分组

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2_test.go`
- Verify: `upstream/sub2api/frontend/src/features/monitor-v2/**`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Consumes: Task 1 已按统计起点过滤的 `ListUserView`。
- Produces: `monitorV2VisibleMonitoredGroups(groups []Group, views []*UserMonitorView, scope MonitorV2Scope) []Group`。

- [ ] **Step 1: 写失败的 Monitor V2 测试**

覆盖以下真实行为：

```go
groups := []Group{
    {ID: 2, Name: "GPT-Pro", Status: StatusActive},
    {ID: 6, Name: "GPT-Plus", Status: StatusActive},
    {ID: 20, Name: "GPT-特惠分组", Status: StatusActive},
    {ID: 21, Name: "GPT-测试分组", Status: StatusActive},
}
views := []*UserMonitorView{
    {GroupID: int64Ptr(2)},
    {GroupID: int64Ptr(6)},
    {GroupName: "GPT-特惠分组"},
}
```

断言 snapshot 只返回 `2,6,20`；无 views 时返回空集合；hidden stable group ID 不回退到同名可见分组；public/admin 专属可见性只作用于已监控分组。

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./internal/service -run 'TestMonitorV2Snapshot.*Monitored|TestMonitorV2SnapshotScopeControlsExclusiveGroups' -count=1`

Expected: FAIL，因为当前实现返回全部活动分组。

- [ ] **Step 3: 实现最小分组过滤**

在 `Snapshot` 中先读取 probes，依据 trusted scope 过滤活动/专属分组，再仅保留 `monitorV2ProbeGroupID(...) != 0` 能匹配的分组。后续 cache stats、ops metrics 和卡片构建只针对该集合执行；不创建 `unconfigured` 占位卡。

- [ ] **Step 4: 运行 GREEN 与回归**

Run: `go test ./internal/service -run 'TestMonitorV2' -count=1`

Run: `pnpm vitest run src/features/monitor-v2 --run`

Expected: PASS。

- [ ] **Step 5: 运行任务级验证、更新总账并提交**

Run: `go test ./internal/service ./internal/handler ./internal/repository -count=1`

Run: `pnpm typecheck`

Run: `pnpm build`

Run: `git diff --check`

总账记录两个任务的实现提交、测试和待生产验证状态。

Commit: `fix: show only configured monitor groups`

---

## 分支级验证与发布门禁

- `cd upstream/sub2api/backend && go test ./... -count=1`
- `cd upstream/sub2api/backend && go vet ./...`
- `cd upstream/sub2api/frontend && pnpm vitest run --run`
- `cd upstream/sub2api/frontend && pnpm typecheck && pnpm build`
- 运行迁移/发布契约测试、`git diff --check` 和独立全分支审查。
- 合并候选到 `main` 后在合并结果重复专项测试、后端全量测试/静态检查、前端测试/构建和发布预检。
- 推送 `main`，使用现有蓝绿脚本构建候选、切换非活动槽并健康验证，不停止 PostgreSQL、Redis、Caddy 或当前活动 API。
- 迁移后立即对三个监控各执行一次主动探测；验证 `/monitor` 仅三张卡，7 天统计不包含旧 190 条记录，原始历史仍存在。

## Acceptance

- [ ] `/monitor` 仅显示三个当前配置监控分组。
- [ ] 新配置统计不再混入旧历史。
- [ ] 原始历史仍可审计。
- [ ] 合并后的 `main` 已推送、蓝绿部署成功且线上验证生效。
