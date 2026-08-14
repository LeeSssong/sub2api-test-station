# 账号监控卡片原生账号复用实施计划

## 目标

在不改变账号管理页、监控列表精简投影或卡片既有字段的前提下，为每张监控卡片增加账号信息、编辑、删除、更多入口，并按账号 ID 按需加载完整原生账号后复用既有账号管理组件和逻辑。

## 任务 1：卡片入口与按需加载事件契约

文件：

- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

步骤：

1. 先补红测，锁定卡片现有关键字段仍在，并验证 `account-info`、`account-edit`、`account-delete`、`account-more` 四个入口事件携带当前账号 ID/投影对象。
2. 在 header 右上角加入低强调“账号信息”入口，在卡片操作区加入 `编辑`、`删除`、`更多`；不删除现有刷新、优先级、成本、指标、探测和调用折叠结构。
3. 保持组件只依赖 `AccountMonitorAccount`，不引入完整 `Account` 到监控 DTO。
4. 运行组件聚焦 Vitest 与 `git diff --check`，提交 Task 1。

独立复审：检查入口可访问性、既有字段无删减、事件 payload、响应式/移动端布局和变更范围。

## 任务 2：监控视图协调、原生组件复用与只读信息壳

文件：

- `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
- 新增 `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorAccountInfoDialog.vue`
- 对应组件测试

步骤：

1. 先补红测：按账号 ID 调用 `adminAPI.accounts.getById`；同账号并发入口去重；加载失败不改变卡片；成功后分别打开信息壳、`EditAccountModal`、原生删除确认、`AccountActionMenu`。
2. 在监控视图维护完整账号和 pending 请求状态，统一处理加载、错误、关闭和操作成功后的 monitor projection 刷新。
3. 直接复用 `EditAccountModal`、`AccountActionMenu`、`ConfirmDialog` 及 `AccountsView` 已有 API 处理语义；只抽取必要的共享协调函数，不改 `AccountsView.vue`。
4. 新增只读 `AccountMonitorAccountInfoDialog`，仅展示完整原生账号中卡片缺失且安全脱敏的账号级字段；禁止渲染凭据原文，不新增写接口。
5. 运行 AccountMonitorView、信息壳、原生账号组件相关 Vitest，修复类型与事件契约。
6. 提交 Task 2。

独立复审：验证详情请求按 ID、请求去重、原生操作复用、账号类型门控、删除确认、敏感字段不泄漏、账号管理页未改动。

## 最终验证与交付

在两个任务均复审通过后：

- 运行监控卡片/视图/信息壳聚焦 Vitest。
- 运行 `pnpm typecheck`、`pnpm build`、`git diff --check`。
- 检查 `AccountMonitorAccount` projection 无非必要字段扩张。
- 检查 `AccountsView.vue` 无改动。
- 做一次浏览器或组件级交互验证：详情、编辑、删除、更多四个入口及失败态。
- 做全分支终审并写入 `docs/superpowers/reports/2026-08-14-account-monitor-card-ready.md`。
- 更新 `docs/project/project-progress.md` 为 `READY_FOR_ROOT_REVIEW`，报告基线、候选 SHA、变更文件、测试、无迁移/无配置、`downtime_required=false`、回滚和风险。

本计划不授权合并、推送、部署或生产操作；交付节点只到 `READY_FOR_ROOT_REVIEW`。
