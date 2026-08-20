# T41 Monitor V2 Timeline视觉与 Tooltip 交互优化规格书

- 任务包：T41
- 基线：`main@3ac10d8473923a9b017c4826024680c4361e8323`
- 候选工作区：`.worktrees/t41-monitor-v2-timeline`
- 状态：`DESIGNING`（代审批准后进入实现）
- 代审记录：根总控已授权按推荐方案直接推进，无需等待用户在线回复。

## 1. 现状与证据

`MonitorV2Timeline.vue` 当前使用 `top-[4.25rem]` 的绝对定位 Tooltip，同时给滚动区加 `pt-12`。柱体与 Tooltip 在同一可视层，窄屏或末端点位时 Tooltip 会覆盖柱体并造成视觉拥挤。柱体固定为 `5px × 16px`，Tooltip 最小宽度 168px；无数据仅在整个数组为空时显示“该时段暂无探测记录”，单个无探测桶没有独立视觉语义。T31/T37/T38 已确定 v7 合同、24/28/30 桶长度、原生探测来源、时间线内部横滚和移动端整页不溢出约束。

## 2. 目标与非目标

### 目标

1. Tooltip 移到柱体下方的独立布局行，任何点位、滚动位置和键盘聚焦状态都不遮挡柱体。
2. 适度放大柱体和 Tooltip，保持密集时间线可扫读；Tooltip 在时间线可视宽度内夹紧。
3. 将 `status=unavailable` 且 `latency_ms=null` 的桶表达为“无探测数据”而非故障 DOWN，并同步中文/英文文案和可访问名称。
4. 保留 v7、24/28/30 桶、原生探测字段、键盘 Tooltip 和移动端仅组件内横滚。

### 非目标

- 不改 API、后端探测、时间桶生成、`MonitorV2TimelinePoint` 字段或 v7 合同。
- 不改 Monitor V2 卡片、窗口选择、刷新、生产配置、迁移或发布链。
- 不新增依赖或全局 CSS。

## 3. 方案比较

### 方案 A（推荐）：Tooltip 独立布局行

在滚动轨道后增加固定高度 `data-timeline-tooltip-row`，Tooltip 在该行内绝对定位；柱体轨道去掉为 Tooltip 预留的顶部 padding。优点是几何上保证不重叠、横向滚动仍可通过根节点矩形夹紧，键盘/鼠标复用现有状态。代价是时间线总高度增加约 68px。

### 方案 B：Tooltip 通过 transform 放在轨道下方

保留同一 DOM 层，使用 `top:100%` 和额外 margin。实现改动小，但 Tooltip 仍可能与时间标签、相邻卡片发生层叠，且滚动边界计算更脆弱。

### 方案 C：原生 `title`/浏览器 Popover

移除自绘 Tooltip，依赖浏览器提示。跨浏览器样式、键盘行为和状态文案不可控，无法满足现有视觉比例与无障碍回归。

选择 A：与现有夹紧算法兼容，风险最小且能用 DOM/CSS 直接测试“不覆盖柱体”。

## 4. 设计与数据流

`points` 原样来自 v7 原生快照。组件派生 `activePoint` 与 `isNoDataPoint`；激活点后在 `nextTick` 读取根节点与点位矩形，按 Tooltip 半宽（98px）计算 `left` 并夹紧到 `[98, rootWidth-98]`。横滚事件重新计算。轨道保持 `min-w-max`、`overflow-x-auto`、`shrink-0`，因此窄屏只滚动时间线内部。

柱体尺寸调整为约 `6px × 20px`、`5px` 间距；无数据桶增加灰色/虚线边框和 `data-timeline-point-state="no-data"`。Tooltip 宽度约 196px、内距和字号略增，行内箭头朝上连接柱体下方。

状态展示：
- operational：`UP`、运行中、可选延迟。
- unavailable 且有延迟：`DOWN`、服务不可用、延迟。
- unavailable 且无延迟：`NO DATA`、无探测数据；不伪造延迟或故障。

## 5. 接口与文案契约

不改 TypeScript 对外接口。新增 i18n 键：
- `monitorV2.timeline.noDataBucket`: 中文“无探测数据”，英文“No probe data”。
- `monitorV2.timeline.noDataBucketLabel`: 中文“无探测数据桶”，英文“No probe data bucket”。

现有 `monitorV2.timeline.noData` 继续用于整个时间线数组为空的 fallback。

## 6. 失败与兼容语义

根节点宽度为 0 或活动点缺失时 Tooltip 左值回退百分比；宽度恢复后按实际矩形重算。Tooltip 永不捕获指针。缺失或非法延迟仍沿用现有 `null` 语义，不触发 API 级错误。`status` 仅接受 v7 已有值。

## 7. 验收矩阵

| 场景 | 预期 |
|---|---|
| 单个 operational 点 hover/focus | Tooltip 在独立行显示 UP，不覆盖柱体，柱体轻微上移 |
| unavailable + latency | 显示 DOWN/服务不可用/延迟 |
| unavailable + null latency | 灰色虚线桶，Tooltip 显示 NO DATA/无探测数据，aria-label 同步 |
| 24/28/30 点数组 | 所有点保留，轨道 `min-w-max`，不改变数组长度 |
| 横向滚动到左右边界 | Tooltip 左值夹紧在根节点可视宽度内 |
| 390px 视口 | 文档无横向溢出，横滚只发生在 `[data-timeline-scroll]` |
| 空数组 | 继续显示“该时段暂无探测记录” |

## 8. 测试与发布

先在 `MonitorV2Timeline.spec.ts` 增加 RED 用例：独立 Tooltip 行/尺寸、不重叠结构、无数据桶文案与样式；运行 Vitest 确认失败后再实现。GREEN 后运行该文件及 `monitor-v2/__tests__` 专项 Vitest、`pnpm typecheck`、必要时 `pnpm build` 与 `git diff --check`。无迁移、配置或停机动作，预期 `downtime_required=false`；根总控负责合并、发布和线上专项验收。回滚为回退候选提交。

## 9. 自审结论

- 占位符/未决项：无。
- 方案、字段、验收与测试一致；仅修改时间线组件、直接测试和中英文 dashboard 文案。
- 范围聚焦单一用户可见垂直功能，不引入新的事实源或 API。
- 代审结论：根总控批准推荐方案并允许进入 writing-plans/TDD 实施。
