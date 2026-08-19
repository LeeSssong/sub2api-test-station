# T31 Monitor V2 视觉放大、时间线交互与 CodexRadar 列对齐交接

状态：`READY_FOR_ROOT_REVIEW`

任务：T31

原始基线：`main@6bf76430c65105232e87fd3cd1cb9eec4b05d010`

刷新主线：`main@3097968e5559af6801e5d00988133507b8aeea4e`

候选功能提交：`8f846c962602af4238c71c817f9c701e798e9937`

候选 tree：`5838434b90d7483b1003ddf18a8b30b51d0bf578`

分支：`codex/t31-monitor-v2-visual-polish-main`

工作区：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t31-monitor-v2-visual-polish`

## 交付内容

- Monitor V2 时间线复刻固定密集柱体形态：每点 `5px × 16px`、固定 `4px` 间距；窄屏保留时间线容器内横向滚动，不挤压柱宽。
- 每个时间点支持鼠标与键盘聚焦，tooltip 显示 `UP / DOWN`、精确到秒的本地时间、中文状态和延迟；横向滚动后重新计算位置并夹在时间线可视区域内。
- 时间线无障碍语义已收口：根节点为有名称的 `group`，每个可聚焦时间点为独立 `img` 且带完整 `aria-label`，可聚焦节点不位于 `aria-hidden=true` 祖先内。
- 分组卡在 hover / focus-within 时整组切换绿色底色、边框、阴影并轻微上浮；倍率改为高对比绿色徽标，并随分组 hover 放大。
- 可用率仅由当前返回的真实 `timeline` 探测点计算：`operational / timeline.length × 100`；空样本显示“暂无可用率数据”，不按目标值补齐或凑 99%。
- 页面最大内容宽度扩大到 `1500px`，同步放大渠道状态、站长推荐和社区测试矩阵的字号、内距、卡片高度与间距。
- CodexRadar 推理强度固定从左到右：`ultra → max → xhigh → high → medium → low`；Sol、Terra、Luna、5.5 按模型族排序，相同 effort 跨模型固定同列。
- 缺档不渲染 placeholder 卡片；仅通过 `gridColumnStart` 保留列轨，使同 effort 对齐且 DOM 中没有占位元素。

## 变更范围

- 仅修改 `upstream/sub2api/frontend/src/features/monitor-v2/` 下 5 个前端组件及 4 个直接测试。
- 仅为真实可用率与时间线语义补充中英文 `dashboard.ts` 文案。
- 未修改采购成本保存、自购 OAuth 报表、CNY 成本录入、T30 后端/API、计费、调度、数据库迁移、配置、发布链、GitHub Actions、根 `main`、全局队列或项目总账。
- 实施期间根 `main` 先前进到 T30 代码提交 `5758e91ad`，随后以纯全局队列/进度文档提交收口到 `3097968e5`；T31 与两次主线推进文件均零重叠，候选已无冲突 rebase 到最新 `main`。刷新前后 Monitor V2 业务补丁 SHA-256 均为 `631184321c5be6d69073d55f58f1f68d0b631b4a545a758a567520a0bc9c405f`，业务差异逐字不变。

## 自动验证

完整业务代码候选上执行：

- `pnpm vitest run src/features/monitor-v2/__tests__`：8 files / 35 tests passed。
- `pnpm typecheck`：退出码 0。
- `pnpm build`：退出码 0，1055 modules transformed；仅存在既有 Browserslist / dynamic-import 构建警告。

刷新到纯文档主线 `main@3097968e5` 后重新执行最小必要门禁：

- `pnpm vitest run src/features/monitor-v2/__tests__`：8 files / 35 tests passed。
- `git diff --check main..HEAD`：通过。
- `git diff --name-status main..HEAD`：仅本交接和 11 个 Monitor V2 前端/直接测试/i18n 文件；全局队列/进度文档只继承主线，无 T31 authored diff；无后端、采购、自购报表、迁移、配置或发布文件。

## 桌面与 390px 视觉证据

证据目录：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t31-monitor-v2-visual-polish/output/playwright/t31-monitor-v2-visual/`

- 桌面完整页：`desktop-1800-full.png`。
- 桌面分组 hover / tooltip：`desktop-channel-hover.png`。
- 390px 完整页：`mobile-390-full.png`。
- 390px 横滚后 tooltip：`mobile-390-timeline-hover.png`。
- 浏览器 fixture：`visual-fixture.js`（未纳入提交）。

实测断言：

- 1800px：文档 `clientWidth=scrollWidth=1800`；时间线 `620/620`；社区矩阵 `1422/1422`。
- 390px：文档 `clientWidth=scrollWidth=390`，无整页横向溢出；4 条时间线各 `clientWidth=298 / scrollWidth=580`；社区矩阵 `clientWidth=316 / scrollWidth=1120`，横滚仅发生在组件自身。
- 390px 将时间线滚到最右后悬停最后一点：tooltip 为 `UP / 2026-08-19 20:00:00 / 运行中 · 1541 ms`，矩形 `left=176 / right=344`，仍在时间线可视边界 `left=46 / right=344` 内。
- 分组 hover 前背景 `rgba(15,23,42,0.75)`，hover 后 `rgba(52,211,153,0.1)`，卡片上移 `2px`，倍率徽标缩放为 `1.05`。
- `xhigh` 在 Sol、Terra、Luna、5.5 四行的实际 `getBoundingClientRect().x` 均为 `153.66`；社区卡片共 19 个、placeholder 0 个。
- 键盘聚焦失败点可显示与鼠标相同的 `DOWN / 2026-08-19 18:15:00 / 服务不可用 · 12000 ms`；时间线根为 `role=group`，失败点为 `role=img` 且无隐藏祖先。

## 发布属性与根总控后续

- 数据库迁移：无。
- API / 数据合同：无变化；可用率在前端仅使用既有真实时间线点计算。
- 配置或配置 schema：无。
- 生产数据写入：无。
- 预期 `downtime_required=false`，最终以根总控合并后发布预检为准。
- 本任务未推送、未部署、未生成或修改生产发布证据。
- 根总控可从 `codex/t31-monitor-v2-visual-polish-main@8f846c962` 审查业务提交并连同本交接合入；合并前仍需确认 `main` 未再次漂移，若漂移则按项目约束刷新后重跑直接门禁。
