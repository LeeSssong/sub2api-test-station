# T71 调度设置独立页面交接

## 状态

`READY_FOR_ROOT_REVIEW`。候选实现和直接相关验证已完成，等待根发布总控审查、合并、发布与线上验收。

## 基线与范围

- 基线：`main@a98e932581eaa643c8ec2edbfefbdcdd75e60353`
- 候选分支：`codex/t71-scheduler-settings-page`
- 候选工作区：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t71-scheduler-settings-page`
- 目标：修复调度选中态和账号均衡预览反馈；将配置移至管理员 `/admin/scheduler-settings`；保留原生 settings API、字段和服务不中断护栏。

## 交付内容

- 新增 `SchedulerSettingsView.vue`：单卡工作台、分组选择、1/2/3 优先级、账号均衡/高峰保护/会话连续性三段控件、平峰/高峰/会话场景预览、保存成功/失败与加载状态、浅深色和窄屏布局。
- 新增 `schedulerPolicy.ts`：推荐优先级回退、优先级合法性、摘要和场景预览纯函数。
- 新增管理员路由和侧栏入口，复用 `requiresAuth`/`requiresAdmin`。
- 新增中英文 `schedulerSettings` 文案和 `nav.schedulerSettings`。
- SettingsView 的旧调度工作面通过 `v-if="false"` 隐藏，避免同一字段存在两套可见编辑入口。
- 保存只写既有 `openai_advanced_scheduler_enabled` 与 `openai_advanced_scheduler_group_policies`，展开合并保留旧模式、权重、公平字段和服务端快照。

## 直接相关验证

- `pnpm vitest run src/views/admin/scheduler/__tests__/schedulerPolicy.spec.ts src/views/admin/__tests__/SchedulerSettingsView.spec.ts`：7/7 通过。
- `pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts src/views/admin/scheduler/__tests__/schedulerPolicy.spec.ts src/views/admin/__tests__/SchedulerSettingsView.spec.ts src/router/__tests__/performance-monitor-route.spec.ts src/components/layout/__tests__/AppSidebar.performance-monitor.spec.ts src/i18n/__tests__/localesNoKeyCollision.spec.ts`：52 通过，11 个旧 SettingsView 调度测试因旧可见入口被替换而跳过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过，生成独立 `SchedulerSettingsView` chunk。
- `git diff --check`：通过。

## 迁移、配置与发布

- 无数据库迁移、后端 API、配置 schema、依赖或生产业务数据写入。
- 预期 `downtime_required=false`，以根发布预检实际输出为准。
- 必须从合并并验证后的根 `main` 使用 `ops/release-sub2api-blue-green.sh` 发布；候选未推送、未合并、未部署。
- 回滚：恢复上一已验证 `main` 提交并通过同一蓝绿链重新发布。

## 未验证项

- 当前自动化浏览器无管理员登录态，访问本地页面时已正确重定向登录；根总控需要在登录态下验收桌面/窄屏、深色模式、键盘焦点、选中态和预览联动。
- SettingsView 测试仍有既有 `router-link` 与 jsdom 导航警告，不影响断言结果。
