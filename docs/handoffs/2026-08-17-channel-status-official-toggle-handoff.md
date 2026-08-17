# 渠道状态官方聚合/自建监控切换交接

## 任务

- 名称：渠道状态官方聚合/自建监控可切换
- 目标：复用 `channel_monitor_mode=v1|v2`，让 `/monitor` 在 `v2` 走官方原生聚合，在 `v1` 保留自建 Monitor V2。
- 不打断：`快速迭代-验收问题`（`01a00e93-af05-76c2-bee5-1a80225bd985`）、`快速迭代-指挥（7）`（`01a00e1e-f9e2-7d02-a4d6-af1900384486`）。

## 候选身份

- 最新根主线基线：`027c0c270c8e4eaa95bf2da4cc6377467ec5fa97`
- 功能实现提交：`ee52dd5ea`
- 刷新合并提交：`551f43122f514e5c604075e19c779f9060c5d48b`
- 刷新后候选 tip：`551f43122f514e5c604075e19c779f9060c5d48b`
- 刷新后候选 tree：`0a26230e1c306d402f493793c1171ee80a7a5dba`
- worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/channel-status-official-toggle`
- 分支：`codex/channel-status-official-toggle`
- 候选状态：`READY_FOR_ROOT_REVIEW`
- worktree 状态：干净

## 变更文件

- `docs/superpowers/specs/2026-08-17-channel-status-official-toggle-design.md`
- `docs/superpowers/plans/2026-08-17-channel-status-official-toggle.md`
- `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2RouteView.vue`
- `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts`

运行时只改入口包装层：`isChannelMonitorV2Mode()=true` 时直接渲染 `ChannelStatusView`，并跳过 `getMonitorV2Snapshot`；`v1` 保持自建快照、加载、错误脱敏回退行为。

## 验证证据

在刷新到最新 `origin/main` 后重新执行：

- `pnpm vitest run src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts`：1 文件 / 3 测试通过。
- `pnpm typecheck`：exit 0。
- `pnpm build`：exit 0，Vite 生产构建完成。
- `git diff --check origin/main...HEAD`：通过。
- 相对 `origin/main` 无迁移文件、无 `.github/workflows` 文件。
- 刷新到 `main@027c0c270` 后再次通过同一专项测试、`pnpm typecheck`、`pnpm build` 和 `git diff --check main...HEAD`。

TDD 证据：新增官方模式测试先在旧实现上按预期失败（官方 stub 不出现），最小实现后 3/3 通过。

## 配置与发布

- 迁移变化：无。
- 后端配置键变化：无。
- 上线切换参数：
  - `channel_monitor_enabled=true`
  - `channel_monitor_mode=v2`
- 预期 `downtime_required=false`；以根总控发布预检输出为准。
- 根总控合并并部署后，登录态打开 `/monitor`，确认官方 V2 页面出现、浏览器不请求 `/api/v1/monitor-v2`，并检查 `/healthz`、`/readyz`、`/health` 为 200。
- 回滚：将 `channel_monitor_mode=v1`，刷新即可回到自建 Monitor V2；无需再次发布。

## 未验证项与剩余风险

- 尚未合并根 `main`、推送、生产设置切换或线上验收。
- T15 当前发布车道仍由唯一总控占用，且最新总账已记录停机门禁；用户最新指令为暂不发布，本候选不自行插队或执行生产动作。
- 官方页面自身的被动聚合数据和官方 V2 配置保持现状，本任务不重定义其统计口径。
