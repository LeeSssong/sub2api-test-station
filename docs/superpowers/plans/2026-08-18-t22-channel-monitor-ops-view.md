# T22 官方 Channel Monitor V2 简洁运营视图实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将官方 Channel Monitor V2 调整为默认 24h、首屏聚焦运营状态、低样本不误报健康且详细分析按需展开的响应式视图。

**Architecture:** 继续由 `ChannelStatusV2View.vue` 编排官方 dimensions/snapshot/matrix/models/errors/users API；在现有 `monitorFormat.ts` 增加只解释 `MonitorMetric`/`MonitorHealth` 的 readiness helper，页面和矩阵共享。首屏只加载 dimensions/snapshot/matrix，详细分析首次展开后加载当前 tab，不改后端/API/健康算法/T19 SQL。

**Tech Stack:** Vue 3, TypeScript, Pinia, Vue Router, vue-i18n, Tailwind CSS, Vitest, Vue Test Utils, Vite, Playwright。

**Spec:** `docs/superpowers/specs/2026-08-18-t22-channel-monitor-ops-view-design.md`

## Global Constraints

- 基线固定为 `main@9d5f658d039ae6f076e558c9d60f01d8de7993f7`；只在当前独立 worktree 写入。
- 不修改根 `main`、`docs/project/project-progress.md`、`docs/project/native-sub-task-package-queue.md`、发布证据或生产状态。
- 复用 T18 官方 V2 路由和 `channel_monitor_mode=v1` 回滚；复用 T19 缓存有效样本分母。
- 不新增后端、API、事实源、迁移、配置、依赖或 GitHub Actions。
- 零流量和样本不足为中性状态；样本充足的后端 `warning/critical` 黄红结果不得被覆盖。
- 桌面和 390px 无整页横向溢出；矩阵/表格只允许容器内滚动。
- 只运行直接相关 Vitest、现有 mode tests、typecheck、build、`git diff --check` 和两个视口的本地浏览器验证。

---

## 文件变更地图

- Modify: `upstream/sub2api/frontend/src/features/channel-monitor-v2/monitorFormat.ts` - 提供 `monitorReadiness` 与中性数值格式 helper。
- Modify: `upstream/sub2api/frontend/src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts` - 锁定零流量、低样本和已评分边界。
- Modify: `upstream/sub2api/frontend/src/features/channel-monitor-v2/RelayPulseMatrix.vue` - 分组行和 bucket 显示 readiness 文案与中性色。
- Modify: `upstream/sub2api/frontend/src/features/channel-monitor-v2/__tests__/RelayPulseMatrix.spec.ts` - 锁定分组状态文本和真实异常色。
- Modify: `upstream/sub2api/frontend/src/views/user/ChannelStatusV2View.vue` - 默认 24h、整体中性摘要、详细分析折叠和懒加载。
- Create: `upstream/sub2api/frontend/src/views/user/__tests__/ChannelStatusV2View.ops.spec.ts` - 页面级请求、信息架构和状态回归。
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/channelMonitorV2.ts` - 中文运营状态与详细分析文案。
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/channelMonitorV2.ts` - 英文等价文案。
- Create: `docs/superpowers/reviews/2026-08-18-t22-channel-monitor-ops-implementation-review.md` - 实现范围自审。
- Create: `docs/handoffs/2026-08-18-t22-channel-monitor-ops-handoff.md` - READY_FOR_ROOT_REVIEW 交接。

---

### Task 1: 低样本 readiness 展示合同

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/channel-monitor-v2/monitorFormat.ts`
- Modify: `upstream/sub2api/frontend/src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts`
- Modify: `upstream/sub2api/frontend/src/features/channel-monitor-v2/RelayPulseMatrix.vue`
- Modify: `upstream/sub2api/frontend/src/features/channel-monitor-v2/__tests__/RelayPulseMatrix.spec.ts`

**Interfaces:**
- Consumes: `MonitorMetric.request_count`、`MonitorHealth.minimum_sample`、`MonitorHealth.score` 和既有 `healthScoreClass`。
- Produces: `MonitorReadiness = 'no_traffic' | 'observing' | 'scored'`、`monitorReadiness(metrics, health)`，供矩阵和页面共同使用。

- [ ] **Step 1: 在 `monitorFormat.spec.ts` 写 RED。**

新增断言：

```ts
expect(monitorReadiness(metrics(0), { ...health, score: null })).toBe('no_traffic')
expect(monitorReadiness(metrics(3), { ...health, minimum_sample: 20, score: null })).toBe('observing')
expect(monitorReadiness(metrics(20), { ...health, minimum_sample: 20, score: 42 })).toBe('scored')
```

- [ ] **Step 2: 运行 helper RED。**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts
```

Expected: FAIL，提示 `monitorReadiness` 尚未导出。

- [ ] **Step 3: 在 `monitorFormat.ts` 写最小实现。**

```ts
export type MonitorReadiness = 'no_traffic' | 'observing' | 'scored'

export function monitorReadiness(metrics: MonitorMetric, health: MonitorHealth): MonitorReadiness {
  if (metrics.request_count <= 0) return 'no_traffic'
  if (health.score == null || metrics.request_count < health.minimum_sample) return 'observing'
  return 'scored'
}
```

- [ ] **Step 4: 运行 helper GREEN。**

Run: `pnpm test:run src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts`

Expected: PASS。

- [ ] **Step 5: 在 `RelayPulseMatrix.spec.ts` 写分组状态 RED。**

构造三行：零请求、3/20 低样本、20/20 且 `overall='critical'`。断言：前两行分别包含“已就绪·暂无流量”“待观察”且使用 `health-unknown`；第三行保留 `health-score*`/critical 色，不含中性文案。bucket tooltip 对零流量和低样本分别显示对应文案。

- [ ] **Step 6: 运行矩阵 RED。**

Run: `pnpm test:run src/features/channel-monitor-v2/__tests__/RelayPulseMatrix.spec.ts`

Expected: FAIL，因为矩阵尚未渲染 readiness 文案。

- [ ] **Step 7: 最小修改 `RelayPulseMatrix.vue`。**

在分组名称下方增加仅中性状态可见的状态文本；成功率、TTFT、缓存率在 `no_traffic` 时显示 `-`，低样本已有值可显示但状态文本为“待观察”。bucket class 继续通过 `healthScoreClass` 保证空分数为中性，tooltip 增加 readiness 行。不得将 `unknown` 改为绿色。

- [ ] **Step 8: 运行 Task 1 GREEN。**

Run:

```bash
pnpm test:run src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts src/features/channel-monitor-v2/__tests__/RelayPulseMatrix.spec.ts
```

Expected: 两个文件全部 PASS。

---

### Task 2: 默认 24h 与简洁首屏/详细分析

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/user/ChannelStatusV2View.vue`
- Create: `upstream/sub2api/frontend/src/views/user/__tests__/ChannelStatusV2View.ops.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/channelMonitorV2.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/channelMonitorV2.ts`

**Interfaces:**
- Consumes: Task 1 的 `monitorReadiness`；现有 `getDimensions/getSnapshot/getMatrix/getModels/getErrors/getUsers`。
- Produces: 缺省 range 24h；`detailsOpen` 控制详细分析；初载只请求 dimensions/snapshot/matrix。

- [ ] **Step 1: 创建页面级 RED 测试夹具。**

mock Router query、Pinia stores、官方 API 和重量组件，返回可控 snapshot/matrix。测试至少覆盖：

```ts
expect(getSnapshot).toHaveBeenCalledWith(expect.objectContaining({ range: '24h' }), ...)
expect(getModels).not.toHaveBeenCalled()
expect(wrapper.text()).toContain('详细分析')
expect(wrapper.get('[data-test="monitor-details-toggle"]').attributes('aria-expanded')).toBe('false')
```

再展开并断言 `getModels` 被调用，切换错误 tab 后 `getErrors` 被调用；合法 `range=7d` 保留；零流量摘要不出现 `100.0%`，低样本显示“待观察”，充足 critical 样本仍给 MetricCell 传 `critical`。

- [ ] **Step 2: 运行页面 RED。**

Run:

```bash
pnpm test:run src/views/user/__tests__/ChannelStatusV2View.ops.spec.ts
```

Expected: FAIL，默认仍为 90m、初载会请求 models 且没有详细分析 toggle。

- [ ] **Step 3: 添加中英文文案。**

新增键：

```ts
readiness: {
  noTraffic: '已就绪·暂无流量',
  observing: '待观察',
}
details: {
  title: '详细分析',
  description: '模型、错误分类与用户排行',
  expand: '展开详细分析',
  collapse: '收起详细分析',
}
```

英文使用 `Ready · no traffic`、`Observing`、`Detailed analysis` 等价文案。

- [ ] **Step 4: 修改默认 range 与整体摘要。**

`parseRange` 非法/缺失值返回 `'24h'`。计算整体 readiness；`no_traffic` 时成功率为“已就绪·暂无流量”、TTFT/缓存为 `-` 且不传健康色，`observing` 时成功率为“待观察”且不传健康色，`scored` 时保留现有值与后端状态。

- [ ] **Step 5: 将三类明细移入可访问折叠区。**

新增 `detailsOpen = ref(false)` 和 toggle button：

```vue
<button
  data-test="monitor-details-toggle"
  :aria-expanded="detailsOpen"
  aria-controls="monitor-detailed-analysis"
  @click="toggleDetails"
>
```

明细 body 用 `v-if="detailsOpen"`。删除 `loadMetrics()` 中无条件 `await loadTab()`；仅 `detailsOpen` 时刷新明细。`toggleDetails` 首次打开调用 `loadTab()`；tab watcher 仅在打开时请求。

- [ ] **Step 6: 运行页面 GREEN 与 mode 回归。**

Run:

```bash
pnpm test:run src/views/user/__tests__/ChannelStatusV2View.ops.spec.ts src/views/user/__tests__/ChannelStatusView.mode.spec.ts
```

Expected: 全部 PASS，v1/v2 路由行为不变。

---

### Task 3: 直接相关验证、响应式检查与交接

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/channel-monitor-v2/__tests__/designSystem.structure.spec.ts`（仅当现有静态合同需同步详细分析结构）
- Create: `docs/superpowers/reviews/2026-08-18-t22-channel-monitor-ops-implementation-review.md`
- Create: `docs/handoffs/2026-08-18-t22-channel-monitor-ops-handoff.md`

**Interfaces:**
- Consumes: Task 1/2 的完成实现与测试。
- Produces: 可由根总控审查的候选提交和本地验证证据。

- [ ] **Step 1: 运行全部直接相关 Vitest。**

```bash
cd upstream/sub2api/frontend
pnpm test:run \
  src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts \
  src/features/channel-monitor-v2/__tests__/MetricCell.spec.ts \
  src/features/channel-monitor-v2/__tests__/RelayPulseMatrix.spec.ts \
  src/features/channel-monitor-v2/__tests__/designSystem.structure.spec.ts \
  src/views/user/__tests__/ChannelStatusV2View.ops.spec.ts \
  src/views/user/__tests__/ChannelStatusView.mode.spec.ts
```

Expected: 0 failed。

- [ ] **Step 2: 运行必要类型与构建门禁。**

```bash
pnpm typecheck
pnpm build
```

Expected: exit 0。

- [ ] **Step 3: 本地浏览器验证。**

启动本地 Vite（若 API mock 需要，使用浏览器路由拦截提供官方响应），在 1440x900 和 390x844 检查：默认 24h、首屏 KPI/分组趋势、详细分析展开、`document.documentElement.scrollWidth <= clientWidth`。保存截图到不入库的临时目录，并检查页面非空、无控件重叠。

- [ ] **Step 4: 范围和静态检查。**

```bash
git diff --check
git diff --name-only 9d5f658d039ae6f076e558c9d60f01d8de7993f7...HEAD
git status --short
```

Expected: 仅规格、计划、T22 前端文件、审查和交接；无 backend/migrations/config/workflows/global ledger。

- [ ] **Step 5: 写实现自审与交接。**

实现自审逐项核对：默认/深链 range、首屏请求、详细分析懒加载、零/低样本中性、真实异常黄红、390px 溢出、v1 回滚、无迁移/配置。交接记录基线、HEAD/tree、变更文件、每条命令、未验证生产项、`downtime_required=false` 预期和回滚。

- [ ] **Step 6: 提交并停在 READY_FOR_ROOT_REVIEW。**

```bash
git add upstream/sub2api/frontend/src docs/superpowers docs/handoffs
git commit -m "feat: simplify channel monitor operations view"
```

不得合并根 `main`、推送、预检、部署或访问生产。

## 计划自审

- [x] 规格每项均映射到 Task 1、2 或 3。
- [x] helper 类型和页面消费名称一致。
- [x] 每个生产代码步骤之前都有对应 RED，之后都有 GREEN。
- [x] 没有未定义接口、占位实施步骤或范围外后端工作。
- [x] 发布总控专属动作不在功能 worktree 执行。
