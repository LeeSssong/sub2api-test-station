# 账号监控产品补完 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `/admin/accounts/monitor` 默认展示全站唯一账号全集，并完整呈现全站/分组的经营、服务、评分权重与账号状态分区。

**Architecture:** 后端投影负责提供完整账号范围及互斥健康计数；前端用“全站”作为显式默认 Tab，并把经营数据、服务数据和账号分区拆为清晰的视图模型。沿用现有账务聚合接口、评分接口和账号卡片，不新增页面或第二套数据源。

**Tech Stack:** Go、Vue 3、TypeScript、Vitest、Tailwind CSS、现有 Sub2API 管理员 API。

## Global Constraints

- 唯一管理员页面是 `/admin/accounts/monitor`；relay-ops 不拥有 UI。
- 默认/恢复默认评分权重固定为 `15/45/20/20`，每组独立；评分只影响监控排名。
- `accounts.priority` 是唯一真实调度优先级。
- 全部唯一账号按 `account_id` 去重；暂停账号可见但不参与服务健康或评分。
- 页面、面向人的报告和验收文案全部使用中文。
- 只允许一个文件写入所有者和一个生产发布者；实现任务不得并行修改同一基线。
- 30 分钟仅是目标节奏；完成、部署和验证优先。

---

### Task 1: 完整账号投影与互斥状态口径

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- Consumes: `AccountRepository.ListAllWithFilters` 返回的完整账号集合。
- Produces: 顶层 `accounts` 包含暂停账号；顶层与分组账号均返回明确 `monitor_bucket`；全站与分组 `health` 的四类计数互斥；关闭分组保留账号投影。

- [ ] **Step 1: 写失败测试**：覆盖非 `active`、不可调度、启用可用、启用不可用、启用待确认和分组成本不合格账号；断言全部账号均投影、`monitor_bucket` 唯一且四类服务计数之和等于唯一账号数。
- [ ] **Step 2: 运行目标测试并确认因现有 active 过滤、关闭分组清空账号而失败。**
- [ ] **Step 3: 最小实现**：移除 active-only 投影过滤；按“暂停 > 待确认 > 成本不合格 > 可用/不可用”生成范围相关 `monitor_bucket`；只让启用且可调度账号进入服务聚合与评分；关闭分组保留账号但保持 `closed`。
- [ ] **Step 4: 运行 service 目标包测试、`go test ./internal/service/...` 与 `git diff --check`。**
- [ ] **Step 5: 提交独立代码与测试。**

### Task 2: 默认全站 Tab 与无隐式筛选

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.spec.ts`

**Interfaces:**
- Consumes: Task 1 的完整 `projection.accounts` 与分组账号投影。
- Produces: `activeGroupId === null` 表示全站；首个 Tab 固定为“全站”；切换分组后才加载分组账务和组内卡片。

- [ ] **Step 1: 写失败测试**：API 给出多分组、未分组和暂停账号时，首次加载显示全部唯一账号，且没有分组账务请求。
- [ ] **Step 2: 运行 Vitest 并确认现有默认首分组和前端 active+schedulable 过滤导致失败。**
- [ ] **Step 3: 最小实现**：保留完整账号数组；新增全站 Tab；删除默认选首分组逻辑；移除重复的分组下拉筛选，以 Tab 作为唯一范围选择。
- [ ] **Step 4: 覆盖全站与分组切换、搜索和服务状态筛选组合；运行两个目标测试文件。**
- [ ] **Step 5: 提交独立代码与测试。**

### Task 3: 双维度摘要、权重入口与账号分区

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
- Optionally create: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorScopeSummary.vue`
- Optionally create: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorAccountSection.vue`
- Test optional new components beside their source files.

**Interfaces:**
- Consumes: 全站/分组健康投影、现有 reconciliation operations 接口、分组倍率/评分权重及账号证据。
- Produces: 明确的全站经营数据、全站账号数据、分组经营数据、分组服务数据，以及五类互斥账号分区。

- [ ] **Step 1: 写失败测试**：断言全站经营与账号服务是独立区块；分组经营与服务均存在；权重摘要与账号状态摘要同行；五类账号进入正确分区。
- [ ] **Step 2: 运行 Vitest 并确认当前混排与单一卡片网格导致失败。**
- [ ] **Step 3: 建立按 `monitor_bucket` 分区的纯计算函数/计算属性**；全站不展示分组质量分，分组可用区按组内质量分降序。
- [ ] **Step 4: 重排模板**：经营/服务分区、权重显式入口、分区标题与双列卡片；空分组或搜索零结果仍保留分组摘要与评分入口；复用现有卡片和弹窗，不改评分/调度接口。
- [ ] **Step 5: 将历史状态、空态和状态标签统一为中文；运行 AccountMonitorView、卡片、评分弹窗测试和前端类型检查。**
- [ ] **Step 6: 提交独立代码与测试。**

### Task 4: 完整回归、视觉验收与生产发布

**Files:**
- Modify: `docs/superpowers/reports/2026-08-02-account-monitor-product-completion-production.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Consumes: Tasks 1-3 的提交和现有 immutable/host-executor 发布路径。
- Produces: 已推送提交、生产不可变镜像、线上 API/UI 证据和回滚点。

- [ ] **Step 1: 运行后端 service/handler/repository 目标包、`go vet`、`go build`、前端目标测试、`vue-tsc`、生产构建和 `git diff --check`。**
- [ ] **Step 2: 独立全分支复审；所有重要发现回到对应实现者修复并复审。**
- [ ] **Step 3: 推送 `codex/account-monitor-completion`，使用既有 SSH 别名 `sub2api-prod` 和不可变发布控制器只发布 Sub2API。**
- [ ] **Step 4: 线上 API 验证账号总数、去重、状态互斥和分组账号范围；确认 PostgreSQL、Redis、Caddy、relay-ops 未被重建。**
- [ ] **Step 5: 使用合法管理员 Chrome 验证全站默认视图、两个全站区块、分组 Tab、两个分组区块、权重入口、五类账号分区和桌面/移动布局，检查控制台无业务错误。**
- [ ] **Step 6: 写中文生产报告；只有推送、部署、线上生效同时成立才把总账标记为完成，再恢复账务任务。**
