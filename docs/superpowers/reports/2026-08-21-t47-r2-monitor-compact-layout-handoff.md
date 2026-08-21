# T47-R2 Monitor V2 紧凑布局交接

- 状态：`READY_FOR_ROOT_REVIEW`
- 基线：`main@67cf4c88d`（候选包含根登记提交 `d9f1c14ce`）
- 候选提交：`c00cca881`
- Worktree：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t47-r2-monitor-layout`
- 分支：`codex/t47-r2-monitor-layout`

## 变更
- `MonitorV2GroupCard.vue`：放大标题/状态/倍率/指标/探测时间，收紧并统一卡片与桌面两列网格，增加稳定的状态/新鲜度测试钩子。
- `MonitorV2Timeline.vue`：柱体改为 `h-8`、间距 `6px`，轨道和时间标签更紧凑；Tooltip 行改为绝对定位悬浮，不再占用 `min-h-[68px]` 常驻高度。
- 直接 Vitest 更新：布局字号、网格、Tooltip 悬浮与柱体尺寸回归断言。

## 验证
- `pnpm vitest run src/features/monitor-v2/__tests__/MonitorV2GroupCard.spec.ts src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts`：10/10 通过。
- `pnpm typecheck`：通过。
- `pnpm build`：通过（Vite 1058 modules）。
- `git diff --check`：通过。

## 未验证项
- 尚未在生产环境进行浏览器视觉验收。
- 尚未合并、推送、部署；无 API、数据库、迁移、配置或生产数据变更。
- 预期发布预检 `downtime_required=false`。

## 回滚
回滚到合并前生产提交/镜像即可；本候选不涉及迁移。
