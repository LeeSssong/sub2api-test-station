# 渠道状态官方聚合/自建监控切换 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 复用现有 `channel_monitor_mode`，让 `/monitor` 在 `v2` 时直接展示官方聚合页，在 `v1` 时保留自建 Monitor V2。

**Architecture:** 只修改 `MonitorV2RouteView.vue` 的入口分流。组件通过现有 `isChannelMonitorV2Mode()` 计算官方模式；官方模式不启动自建快照请求，其他路径保持原行为。

**Tech Stack:** Vue 3 `<script setup>`、TypeScript、Pinia 公共设置、Vitest、Vue Test Utils、pnpm。

## Global Constraints

- 不新增后端配置、API、数据库迁移或平行事实源。
- `channel_monitor_mode=v2` 选择官方页面；`channel_monitor_mode=v1` 选择自建页面。
- 官方模式不得请求 `/api/v1/monitor-v2`。
- 自建模式的成功、加载、错误脱敏和官方回退行为保持不变。
- 不修改根 `main`、全局队列、项目总账或生产；候选只交给唯一发布总控。
- 不使用 GitHub Actions；预期 `downtime_required=false`，最终以根发布预检为准。

---

### Task 1: 在 `/monitor` 包装层按原生模式分流

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2RouteView.vue`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts`

**Interfaces:**
- Consumes: `isChannelMonitorV2Mode(): boolean` from `@/utils/featureFlags`。
- Produces: `MonitorV2RouteView` 在官方模式直接渲染 `ChannelStatusView`，并跳过 `getMonitorV2Snapshot('7d', signal)`。

- [ ] **Step 1: 为模式函数建立可变 mock**

在测试的 hoisted 对象中加入 `isChannelMonitorV2Mode`，并 mock `@/utils/featureFlags`：

```ts
const { getMonitorV2Snapshot, isChannelMonitorV2Mode } = vi.hoisted(() => ({
  getMonitorV2Snapshot: vi.fn(),
  isChannelMonitorV2Mode: vi.fn(),
}))

vi.mock('@/utils/featureFlags', async () => {
  const actual = await vi.importActual<typeof import('@/utils/featureFlags')>(
    '@/utils/featureFlags',
  )
  return {
    ...actual,
    isChannelMonitorV2Mode,
  }
})
```

在 `beforeEach` 中恢复自建默认：

```ts
isChannelMonitorV2Mode.mockReturnValue(false)
```

- [ ] **Step 2: 写官方模式失败测试**

在现有测试前加入：

```ts
it('renders the official aggregated status page without requesting the custom snapshot in v2 mode', async () => {
  isChannelMonitorV2Mode.mockReturnValue(true)

  const wrapper = mountRoute()
  await flushPromises()

  expect(wrapper.find('[data-test="native-channel-status"]').exists()).toBe(true)
  expect(getMonitorV2Snapshot).not.toHaveBeenCalled()
})
```

- [ ] **Step 3: 运行 RED 并确认失败原因**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts
```

Expected: 新测试 FAIL；官方状态 stub 未出现，且当前实现调用了 `getMonitorV2Snapshot`。现有两例保持通过。

- [ ] **Step 4: 写最小生产实现**

在模板首位加入官方模式分支：

```vue
<ChannelStatusView v-if="officialMode" />
```

把现有 fallback 根分支改为 `v-else-if="fallback"`。在脚本中导入 `computed` 和模式函数：

```ts
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { isChannelMonitorV2Mode } from '@/utils/featureFlags'

const officialMode = computed(() => isChannelMonitorV2Mode())
```

挂载时首先跳过官方模式：

```ts
onMounted(async () => {
  if (officialMode.value) return

  const controller = new AbortController()
  // 保留其余现有实现
})
```

- [ ] **Step 5: 运行 GREEN**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts
```

Expected: 1 个测试文件、3 个测试全部 PASS。

- [ ] **Step 6: 运行直接相关门禁**

Run:

```bash
cd upstream/sub2api/frontend
pnpm typecheck
pnpm build
cd ../../..
git diff --check
```

Expected: 所有命令 exit 0；只允许规格、计划、包装层和该 spec 有差异。

- [ ] **Step 7: 提交候选**

```bash
git add \
  docs/superpowers/specs/2026-08-17-channel-status-official-toggle-design.md \
  docs/superpowers/plans/2026-08-17-channel-status-official-toggle.md \
  upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2RouteView.vue \
  upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts
git commit -m "feat: switch monitor page by native mode"
```

### Task 2: 生成根总控交接并排队发布

**Files:**
- Create: `docs/handoffs/2026-08-17-channel-status-official-toggle-handoff.md`

**Interfaces:**
- Consumes: Task 1 的候选 SHA、测试输出和变更清单。
- Produces: 根发布总控可执行的候选交接，包括基线、提交、配置切换、停机属性、验证和回滚。

- [ ] **Step 1: 写交接文件**

交接必须记录：

```text
baseline main SHA
candidate SHA/tree
changed files
test/typecheck/build/diff-check results
migration changes: none
configuration: channel_monitor_enabled=true; channel_monitor_mode=v2
downtime_required: expected false, pending root preflight
rollback parameter: channel_monitor_mode=v1
unverified: production config switch, authenticated /monitor, network request absence, health endpoints
```

- [ ] **Step 2: 做最终范围和工作树验证**

Run:

```bash
git diff --check origin/main...HEAD
git diff --name-only origin/main...HEAD
git status --short
git log -3 --oneline
```

Expected: 无 diff-check 错误；只包含本任务文件；提交后工作树为空。

- [ ] **Step 3: 提交交接**

```bash
git add docs/handoffs/2026-08-17-channel-status-official-toggle-handoff.md
git commit -m "docs: hand off channel status toggle"
```

- [ ] **Step 4: 交给唯一发布总控**

向 `快速迭代-指挥（7）` 提交候选 SHA/tree、基线、验证结果和交接路径。若 T15 仍处于 `INTEGRATING`、`DEPLOYING` 或 `VERIFYING`，保持候选 `READY_FOR_ROOT_REVIEW` 排队；车道释放后由总控合并根 `main`、复验、推送、预检和蓝绿发布。
