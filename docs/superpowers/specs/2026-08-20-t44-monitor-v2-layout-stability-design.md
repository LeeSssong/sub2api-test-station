# T44 Monitor V2 时间线布局稳定性与卡片防抖设计

## 状态与批准记录

- 任务：T44（快速迭代-12）
- 基线：`main@2d7c38de2c932478ed82b415e201855ef75839e4`
- 工作区：`.worktrees/t44-monitor-v2-layout-stability`
- 分支：`codex/t44-monitor-v2-layout-stability`
- 设计批准：根总控转达用户“好的，继续”，并已批准本轮推荐方向；不等待额外真机验收。

## 问题证据与当前行为

`MonitorV2GroupCard` 使用 `transition-all` 与 `hover:-translate-y-0.5`。鼠标在相邻卡片间移动时，卡片整体位移和阴影扩散会改变布局视觉锚点，造成抖动；`transition-all` 还会对不应参与布局的属性做过渡。

`MonitorV2Timeline` 在桌面端固定 `sm:w-[620px]`，柱体固定 `w-[6px]` 并使用 flex，因此 24/28/30 个柱体只占时间线左侧，右侧留下大块空白。移动端需要继续在时间线内部横向滚动，不能让整页横向溢出。Tooltip 已独立在固定最小高度行中，本任务保留该结构并使卡片高度不因 hover 改变。

现有前端测试已锁定旧行为：期待柱体 `w-[6px]`、`hover:-translate-y-1` 与卡片 `hover:-translate-y-0.5`；这些断言需要改成新的稳定布局合同。

## 目标

1. 桌面端（`min-width: 640px`）时间线轨道填充父列可用宽度，24、28、30 桶均匀分配列宽，保留可读间距和最小柱宽。
2. 小屏幕时间线保持组件内部横向滚动，轨道保持可读的固定最小宽度；页面主体不出现横向溢出。
3. 卡片 hover/focus 只改变颜色和阴影，不做整体位移、不改变边框宽度、不触发跨卡片布局抖动。
4. Tooltip 行和时间标签维持固定结构/高度，hover 进入与离开不改变卡片外部高度。
5. 不修改 Monitor V2 v7 API、24/28/30 桶数量、原生探测数据源、刷新语义、业务文案或后端代码。

## 非目标

- 不改 tooltip 文案、定位算法或数据格式，除非为适配稳定轨道所需的最小样式调整。
- 不调整 Monitor V2 卡片信息层级、评分公式、状态语义或 CodexRadar 主题。
- 不增加新依赖、迁移、配置项、生产数据写入或浏览器端数据请求。

## 方案比较

### 方案 A（推荐）：响应式 CSS Grid + 移动端固定轨道

桌面轨道使用 `repeat(var(--timeline-count), minmax(0, 1fr))`，每个桶均匀占用可用宽度；根节点取消桌面固定 620px。小屏切换为固定最小宽度的 flex 轨道，仅滚动内部容器。卡片使用 `transition-colors`/`transition-shadow`，保留恒定 1px border。

优点：布局直接表达“每桶一列”，桌面无空白，移动端行为可控，DOM 和数据契约不变。代价：需要少量 scoped CSS/内联桶数变量。

### 方案 B：桌面继续 flex，按运行时计算柱宽

在 mounted/resize 中计算柱体宽度并写入 CSS 变量。优点是复用现有 flex；代价是引入测量时序、resize 状态和潜在闪烁，且容易重新触发 hover 布局变化。

### 方案 C：放大固定轨道并居中

把固定轨道扩展为更大宽度、居中显示。优点是改动少；代价是不同卡片/窗口仍可能出现空白，小屏和大屏体验不一致，不能真正填充父列。

选择方案 A，因为它无运行时测量、对 24/28/30 桶统一、能保留移动端内部滚动并最小化布局副作用。

## 端到端设计

`MonitorV2GroupCard` 的 `header` 继续使用两列 grid；右列中的 `MonitorV2Timeline` 使用 `w-full min-w-0`，桌面轨道以 points 数量设置 `--timeline-count`，并用 grid 分配每桶宽度。小屏由媒体查询将轨道切回 `display:flex`、`min-width:620px`，滚动容器设置 `overflow-x:auto`，因此滚动范围局限在时间线。

柱体保留 `h-5`、圆角、状态色、无数据虚线表达和可访问性属性；桌面宽度由网格列决定，移动端固定为 6px。hover/focus 仅保留 glow/shadow 与颜色变化，不再使用 translate/scale，避免点位改变导致 tooltip 重新计算和卡片视觉抖动。

Tooltip 继续渲染在 `data-timeline-tooltip-row` 中，行保留固定最小高度；卡片 border 宽度始终 1px，hover/focus 仅改变 border/background/shadow。

## 接口与字段契约

- `MonitorV2Timeline` props 仍为 `points: MonitorV2TimelinePoint[]`。
- `data-timeline-point`、`data-timeline-point-state`、`data-timeline-track`、`data-timeline-scroll` 和 tooltip 数据属性保留。
- 24h、7d、30d 分别继续由 API 提供 24、28、30 个点；前端不修改 validator 或 API 类型。
- 不新增后端字段、不修改 v7 JSON。

## 失败与兼容语义

- points 为空时仍显示占位轨道和“暂无探测记录”，不设置无效的 grid 列。
- 极窄设备保持内部滚动；若 CSS 容器宽度为 0，既有 tooltip 定位 fallback 保留。
- 不支持 CSS grid 的旧浏览器不在项目支持矩阵内；现代浏览器使用现有 Tailwind/Vite 编译链。

## 验收矩阵

| 场景 | 预期 |
|---|---|
| 24/28/30 点，桌面宽卡片 | 轨道使用父列宽度，桶列均匀，无大块右侧空白 |
| 64 点，小屏 | 时间线内部可横向滚动，页面根节点无横向溢出 |
| 鼠标进入/离开柱体 | 柱体不位移、不缩放，tooltip 行高度不变 |
| 鼠标在相邻卡片间移动 | 其他卡片不发生整体位移，border 宽度稳定 |
| focus 柱体/卡片 | 保持可见 focus ring/颜色反馈，不改变几何尺寸 |
| 空时间线 | 占位轨道与文案保持现有语义 |

## 测试策略

- TDD 先修改/新增 `MonitorV2Timeline.spec.ts`：桌面响应式 grid 合同、24/28/30 桶变量、移动端内部滚动、柱体无 translate/scale；tooltip 行固定高度。
- 修改 `MonitorV2View.spec.ts`：卡片仅使用颜色/阴影过渡，明确无 `transition-all`、无 `hover:-translate-y-0.5`。
- 运行 Monitor V2 直接相关 Vitest，随后 `pnpm typecheck`、`pnpm build`、`git diff --check`。
- 不运行无关全仓测试或后端测试，除非前端构建暴露跨层错误。

## 发布、回滚与风险

- 仅前端 Vue/测试/规格计划/交接文档变更；无迁移、无配置、无生产数据写入。
- 预期 `downtime_required=false`，根预检为最终事实；发布按单车道蓝绿链执行。
- 回滚：恢复 T44 提交或切回上一不可变镜像，不需要数据回滚。
- 剩余风险：不同浏览器字体/缩放导致柱宽视觉略有差异；移动端固定轨道宽度仍需用户真机确认，但不影响桌面布局合同或数据正确性。

## 仍待决事项

无。实施按方案 A 执行，完成后停在 `READY_FOR_ROOT_REVIEW`，不合并、不推送、不部署。
