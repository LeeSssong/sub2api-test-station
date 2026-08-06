# 账号监控验收收口实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 `/admin/accounts/monitor` 的布局、评分口径、渠道探测证据、历史/异常交互，并修复蓝绿发布控制器传递 host executor 环境变量，使同一候选可以安全部署和线上验收。

**Architecture:** 继续复用 Sub2API 原生账号监控页和 `/admin/groups` 返回的真实分组，不创建 relay-ops 第二套监控数据源。前端卡片只负责中文展示和交互；评分整数化、全站/分组投影和探测状态由既有 API 契约提供；发布控制器显式传递非敏感绝对路径变量，凭证仍由生产文件读取。

**Tech Stack:** Vue 3、TypeScript、Vitest、Go、Bash、Docker/SSH 蓝绿发布。

## Global Constraints

- 所有面向用户的标签、状态、错误和证据说明使用中文。
- 桌面账号卡片每行最多两张；移动端保持单列。
- 评分展示为整数；“账号利润”替代“纸面利润”；“全局样本回退”改为通俗中文。
- 渠道探测不可用但探测已完成时按绿色成功展示，柱状图高度较低，并显示调用样本数。
- 账单范围是全站所有账号，不得按 Wawazz 等单一账号限定。
- 只有推送、生产部署和线上验证全部完成后，才能更新项目总账为已完成。

### Task 1: 账号卡片信息层级与证据展示

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

- [ ] 添加失败断言：评分无小数、回退说明为中文、卡片包含样本数、今日调用默认收起、桌面网格为两列。
- [ ] 最小实现：弱化评分/调度优先级视觉层级；保留近期探测柱状图；为成功/不可用探测显示样本数；统一中文状态和“账号利润”。
- [ ] 运行账号监控定向 Vitest，确认旧英文/专业术语和浮点评分不再出现。

### Task 2: 全站与分组数据及按钮回归

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorLedgerHistoryDrawer.vue`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

- [ ] 添加失败用例：默认全站展示全部账号，分组仅来自 `/admin/groups` 投影；历史按日和异常明细按钮打开并显示加载/错误/空态。
- [ ] 修复重复导入、事件绑定和抽屉请求状态，确保每个按钮可点击、失败有中文反馈。
- [ ] 运行前端类型检查和定向/全量测试。

### Task 3: 评分计算口径与后端投影验证

**Files:**
- Inspect/modify as needed: `upstream/sub2api/backend/internal/service/account_monitor_service.go`, `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`, related DTO/tests.
- Test: existing account-monitor backend tests plus new regression if needed.

- [ ] 复现浮点 `quality_score` 来源，确认服务端计算和 JSON 投影的显示口径。
- [ ] 将展示契约收敛为整数，保留内部精度只用于排序；验证全站账号数、分组账号数、软删除分组过滤和全站默认视图。
- [ ] 运行 Go 定向测试、`go vet` 和构建。

### Task 4: 发布控制器环境传递与候选部署

**Files:**
- Modify: `ops/release-sub2api-blue-green.sh`
- Test: `tests/operations/release_sub2api_blue_green_test.sh`
- Update: `docs/project/project-progress.md`

- [ ] 添加失败夹具：`sudo env_reset` 下 host executor 收不到 `DEPLOY_ROOT` 时必须被测试捕获。
- [ ] 显式传递非敏感生产路径变量，保持 root-only 凭证读取和路径校验。
- [ ] 重跑 Bash 语法、发布控制器、host executor 和 diff 检查，再推送候选。

### Task 5: 生产发布与线上验收

- [ ] 使用已确认 SSH 凭证执行一次受控维护发布，不改变 PostgreSQL/Redis/Caddy 身份。
- [ ] 验收 `/admin/accounts/monitor`、`/admin/groups`、`/monitor`：全站账号范围、两列布局、整数评分、中文文案、绿色不可用探测柱、样本数、历史按日、异常明细。
- [ ] 记录镜像摘要、release record、活动槽、容器 ID 和失败/回滚证据。
- [ ] 只有线上验证通过后，将总账对应事项标记为已完成。
