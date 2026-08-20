# T41 Monitor V2 Timeline视觉与 Tooltip 交互优化交接

状态：`READY_FOR_ROOT_REVIEW`

任务包：T41
基线 `main`：`3ac10d8473923a9b017c4826024680c4361e8323`
工作区：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t41-monitor-v2-timeline`
分支：`codex/t41-monitor-v2-timeline`
功能提交：`ad526074d`（后续 handoff 文档提交见本文件最终 HEAD）

## 交付内容

- `MonitorV2Timeline.vue` 将 Tooltip 放入 `data-timeline-tooltip-row` 独立布局行，移除柱体上方覆盖层；Tooltip 最小宽度由 168px 提升为 196px，定位夹紧半宽同步为 98px。
- 时间线柱体调整为 `6px × 20px`、`5px` 间距，保留 `min-w-max`、内部 `overflow-x-auto`、`shrink-0`，移动端整页不溢出。
- `unavailable + latency_ms=null` 桶以 `data-timeline-point-state="no-data"`、灰色虚线样式、`NO DATA` 标题和“无探测数据”文案呈现；aria-label 使用“无探测数据桶”。
- 中英文 dashboard locale 增加 `timeline.noDataBucket` 与 `timeline.noDataBucketLabel`；v7 数据类型、24/28/30 桶契约和原生探测字段不变。
- 规格书与实施计划：
  - `docs/superpowers/specs/2026-08-20-t41-monitor-v2-timeline-visual-design.md`
  - `docs/superpowers/plans/2026-08-20-t41-monitor-v2-timeline-visual.md`

## TDD 与验证

- RED：在实现前运行 `pnpm vitest run src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts`，5 项中 4 项按预期失败（旧尺寸、无 tooltip row、无 no-data state）。
- GREEN：同命令 `5 tests passed`。
- Monitor V2 专项：`pnpm vitest run src/features/monitor-v2/__tests__` → 8 files / 37 tests passed。
- 类型：`pnpm typecheck` → exit 0。
- 构建：`pnpm build` → Vite 1055 modules transformed，`✓ built in 11.73s`；仅既有 Browserslist/dynamic-import 警告。
- `git diff --check` → 通过。

## 范围/发布属性

- 变更仅涉及 MonitorV2Timeline 组件、直接 Vitest、英文/中文 dashboard locale、T41 spec/plan/handoff；无后端、API、迁移、配置、生产数据或发布链变更。
- `downtime_required=false`（最终值以根总控合并后的发布预检为准）。
- 未推送、未合并 `main`、未部署、未生成生产证据。

## 未验证项与风险

- 未执行真实浏览器 390px/桌面截图；需根总控在合并后按既有 Monitor V2 视觉验收流程确认 Tooltip 行高度和边界。
- Tooltip 行固定最小高度约 68px，会增加卡片纵向空间；若线上视觉需要，可在同一任务候选上微调行高而不改变数据语义。
- no-data 判定沿用现有 v7 字段语义（`unavailable` 且 `latency_ms=null`），没有新增后端状态值。

## 回滚

回退候选提交 `ad526074d`（及本 handoff 文档提交）即可恢复基线时间线布局；无迁移回滚动作。
