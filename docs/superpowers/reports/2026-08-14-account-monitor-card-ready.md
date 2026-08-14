# 账号监控卡片原生账号复用 READY_FOR_ROOT_REVIEW

日期：2026-08-14

## 状态

`READY_FOR_ROOT_REVIEW`。候选尚未合并、推送、部署或触碰生产。

## 基线与候选

- 基线：`main@263a2de748269b3c96057f500eda5426fe1c013e`
- 报告写入前候选：`5cac7893b`
- 分支：`codex/account-monitor-card`
- 工作区：`/Users/gongtengxinwen/.codex/worktrees/account-monitor-card`

## 已交付

- 监控列表继续使用原有精简 `AccountMonitorAccount` projection，没有增加完整账号字段，卡片现有字段、指标和事件均保留。
- 每张卡片增加低强调 `账号信息` 入口，以及 `编辑`、`删除`、`更多` 三个入口。
- 点击入口后通过 `adminAPI.accounts.getById(account_id)` 按需获取完整原生账号；相同账号并发请求去重，不同账号乱序成功/失败不会覆盖当前操作目标或 loading/error 状态。
- `编辑` 复用原生 `EditAccountModal`；`删除` 复用 `ConfirmDialog` 和原生删除 API；`更多` 复用 `AccountActionMenu`、账号类型门控、测试/统计/定时任务/重授权弹窗和原生操作结果语义。
- `更多` 菜单在异步请求前捕获触发按钮矩形，并使用项目 `getFloatingPanelPosition` 规则锚定定位。
- 新增只读账号信息对话框，只展示安全的账号级管理字段；不渲染凭据、token 或原始 `error_message`。
- `AccountsView.vue` 未修改；列设置、批量操作、导入导出、自动刷新等全局能力未搬迁。

## 变更文件

- `docs/superpowers/specs/2026-08-14-account-monitor-card-native-reuse.md`
- `docs/superpowers/plans/2026-08-14-account-monitor-card-native-reuse.md`
- `docs/superpowers/reports/2026-08-14-account-monitor-card-task1.md`
- `docs/superpowers/reports/2026-08-14-account-monitor-card-task2-fix1.md`
- `docs/superpowers/reports/2026-08-14-account-monitor-card-task2-fix2.md`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorAccountInfoDialog.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorAccountInfoDialog.spec.ts`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

## 验证

- 最终聚焦 Vitest：5 files，90 tests passed。
- 其中账号监控卡片/视图/信息壳：71 tests passed。
- 原生 `AccountActionMenu` 与 `AccountsView` Spark 回归：19 tests passed。
- `pnpm typecheck`：PASS。
- `pnpm build`：PASS；仅仓库既有 Vite 动态/静态 import 与 Browserslist 数据提示。
- `git diff --check`：PASS。
- scoped re-review：`APPROVE`，前两轮竞态、原生语义、菜单定位和敏感字段问题均关闭。
- 最终全分支审查：`APPROVE`，完整 diff 未包含 `AccountsView.vue`、监控 DTO、后端、迁移、配置、项目总账或任务队列变化。

## 迁移、配置与停机

- 数据库迁移：无。
- 后端/API 变更：无。
- 配置变更：无。
- GitHub Actions：无。
- `downtime_required=false`；该候选仅包含前端代码与任务文档。

## 回滚

根总控可在合并前拒绝候选；合并后若需回滚，可 revert 本候选的功能提交并重新构建前端，不涉及数据回滚、收缩迁移或配置恢复。

## 剩余风险

- 原生账号管理处理器仍分别存在于两个视图中；本候选已用同一组件、API、i18n 和测试锁定语义，但未来若 `AccountsView` 新增“更多”动作，需要同步接入监控入口。
- 最终验证为组件交互、类型检查和 production build；生产页面视觉验收应在根总控合并后的发布验证阶段执行。
