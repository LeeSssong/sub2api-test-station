# 账号监控加载性能与错误恢复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 降低账号监控接口尾延迟，解除模型检测 N+1 阻塞，并让前端错误和性能图表可恢复、可解释。

**Architecture:** 服务端保留原生聚合口径，对逐账号模型检测投影使用 8 worker 限流和单账号降级，并移除重复设置查询；其他大型 SQL 保持顺序以控制数据库压力。前端按错误分类显示中文恢复状态。

**Tech Stack:** Go、PostgreSQL、Gin、Vue 3、TypeScript、Vitest。

**Spec:** `docs/superpowers/specs/2026-09-03-account-monitor-performance-design.md`

## Global Constraints

- TTFT P95 ≤ 10000 ms 为绿色，> 10000 ms 为黄色；失败红色；无数据灰色短柱。
- 不修改账务、调度数学、探测间隔、模型检测算法或历史数据。
- 模型检测并行度固定为 8；不创建无限 goroutine。
- 不新增迁移或生产配置；不得发布。

### Task 1: 后端模型检测并行与单账号降级

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

- [ ] 增加失败测试：多个账号的模型检测投影并行执行；单账号错误不使 `ListWindow` 失败；上下文取消能停止 worker。
- [ ] 运行定向 Go 测试确认失败。
- [ ] 实现固定 8 worker 的任务队列，按账号写入独立结果，保留当前字段映射和状态语义。
- [ ] 将 `ListWindow` 中串行投影替换为 worker 结果；对失败账号记录结构化 warning，不返回整页错误。
- [ ] 运行服务定向测试、gofmt 和 `go test`。
- [ ] 提交 `perf: parallelize account monitor detection projection`。

### Task 2: 前端错误恢复与按需评分

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

- [ ] 增加失败测试：超时、取消、5xx、401/403 和其他网络错误分别显示中文；已有 projection 不被清空；重试重新发起请求。
- [ ] 将错误分类封装为纯函数，保留现有错误对象信息。
- [ ] 加入首屏核心数据显示和检测状态补充的显式 loading/error 状态，不阻断卡片渲染。
- [ ] 运行定向 Vitest。
- [ ] 提交 `fix: harden account monitor loading errors`。

### Task 3: 性能柱状图阈值与验证

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Test: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

- [ ] 增加失败测试覆盖 10000ms 边界、失败桶、空桶和自定义 hover 提示。
- [ ] 调整颜色判断为 `<= 10000` 绿色、`> 10000` 黄色，失败优先红色，无数据灰色短柱。
- [ ] 运行定向 Vitest、`pnpm typecheck` 和前端构建。
- [ ] 提交 `fix: align account monitor latency color thresholds`。

### Task 4: 收口与发布前停机

**Files:**
- Create: `docs/handoffs/2026-09-03-t120-account-monitor-performance-handoff.md`

- [ ] 运行 Go 定向测试、服务构建、前端定向测试、typecheck、构建和 diff-check。
- [ ] 检查无迁移、无配置、无数据写入；确认未触碰发布脚本。
- [ ] 记录基线 SHA、提交 SHA、测试结果、未验证项、性能目标和回滚方式。
- [ ] 将任务标记为 `READY_FOR_ROOT_REVIEW`，停在发布前，不合并、推送或部署。
