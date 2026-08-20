# T46 性能监测自定义页面挂载交接

- 任务包：T46
- 基线：`main@989c072a87f40abcaa3b6c5c60d0eeb6941c2761`
- 候选分支：`codex/t46-performance-monitor`
- 候选 worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t46-performance-monitor`
- 提交：`d7a8c2539`（规格/计划）、`7d4d2b9a4`（实现与测试）、`336e90472`（交接）
- 状态：`READY_FOR_ROOT_REVIEW`

## 变更文件

- `upstream/sub2api/frontend/src/router/index.ts`：新增 `/custom/performance-monitor`、`PerformanceMonitor` 路由，登录鉴权和 `nav.performanceMonitor` 标题。
- `upstream/sub2api/frontend/src/views/user/PerformanceMonitorView.vue`：直接挂载原生 `MonitorV2RouteView`。
- `upstream/sub2api/frontend/src/components/layout/AppSidebar.vue`：移除固定 `/monitor` 导航；注入带 feature flag 的虚拟“性能监测”自定义菜单项，普通用户和管理员个人区共用。
- `upstream/sub2api/frontend/src/i18n/locales/zh/common.ts`、`en/common.ts`：新增菜单文案。
- 直接相关测试：路由合同、侧边栏合同、页面标题合同。

## 验证

- RED：新增路由/导航合同测试在实现前按预期失败（未注册路由、旧导航仍存在）。
- GREEN：`pnpm vitest run src/router/__tests__/title.spec.ts src/router/__tests__/performance-monitor-route.spec.ts src/components/layout/__tests__/AppSidebar.performance-monitor.spec.ts src/features/monitor-v2/__tests__`：11 files / 49 tests passed。
- `pnpm typecheck`：passed。
- `pnpm build`：passed（Vite 5.4.21，生成 `PerformanceMonitorView` chunk）。
- `git diff --check`：passed。
- 构建仅有既有动态/静态导入和 Browserslist 过期提示，无新增错误。

## 发布与回滚

- 无迁移、无配置 schema、无生产数据写入、无依赖变化，预期 `downtime_required=false`，以根合并后的发布预检为准。
- 回滚：恢复上一已验证蓝绿槽或回退 T46 合并提交；无数据库回滚。
- 根发布后需登录态确认：新菜单标签/URL、原生 Monitor V2 卡片和时间线、1440px 与 390px 无整页横向溢出；公网 `/healthz`、`/readyz`、`/health` 均 200。
- 未验证项：候选 worktree 未进行生产发布和真机登录态截图；不影响候选本地门禁。

## 范围说明

- 未修改 `/monitor` 路由本身，不新增重定向；本次仅隐藏固定导航并提供新的静态页面路由。
- 未修改 `CustomMenuItem` 后端 DTO、管理端表单、iframe/Markdown 渲染或 Monitor V2 API/统计/刷新合同。
