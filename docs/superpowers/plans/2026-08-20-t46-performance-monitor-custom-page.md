# T46 性能监测自定义页面挂载实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Sub 原生 Monitor V2 挂载到独立 `/custom/performance-monitor` 页面，新增“性能监测”导航并隐藏固定 `/monitor` 入口。

**Architecture:** 新建轻量 `PerformanceMonitorView.vue` 作为原生路由壳，内部直接渲染既有 `MonitorV2RouteView`。AppSidebar 只改固定导航声明，保留自定义 URL/Markdown 菜单系统不变。

**Tech Stack:** Vue 3 `<script setup>`、Vue Router、TypeScript、Vitest、Vite/pnpm。

**Spec:** `docs/superpowers/specs/2026-08-20-t46-performance-monitor-custom-page-design.md`

## Global Constraints

- 复用 `MonitorV2RouteView`、既有 API/刷新/时间线数据，不新增事实源。
- 隐藏固定 `/monitor` 导航，不新增旧路由重定向。
- 不修改 iframe/Markdown 自定义菜单合同。
- 无迁移、无生产数据写入、禁止 GitHub Actions。

---

### Task 1: 路由与页面壳测试（RED）

**Files:**
- Create: `upstream/sub2api/frontend/src/views/user/PerformanceMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/router/index.ts:388-402`
- Test: `upstream/sub2api/frontend/src/router/__tests__/performance-monitor-route.spec.ts`

**Interfaces:**
- Consumes: existing `MonitorV2RouteView.vue`.
- Produces: named route `PerformanceMonitor`, URL `/custom/performance-monitor`, auth metadata and native page component.

- [ ] **Step 1: Write the failing route contract test**

```ts
import { describe, expect, it } from 'vitest'
import { routes } from '@/router'

describe('performance monitor route', () => {
  it('registers an authenticated native page route', () => {
    const route = routes.find((candidate) => candidate.name === 'PerformanceMonitor')
    expect(route?.path).toBe('/custom/performance-monitor')
    expect(route?.meta).toMatchObject({ requiresAuth: true, requiresAdmin: false, titleKey: 'nav.performanceMonitor' })
    expect(route?.component).toBeTypeOf('function')
  })
})
```

- [ ] **Step 2: Run the route test and verify RED**

Run: `pnpm vitest run src/router/__tests__/performance-monitor-route.spec.ts`
Expected: FAIL because the named route is not registered.

- [ ] **Step 3: Write the minimal page shell and route**

```vue
<template>
  <MonitorV2RouteView />
</template>

<script setup lang="ts">
import MonitorV2RouteView from '@/features/monitor-v2/MonitorV2RouteView.vue'
</script>
```

Add the route immediately after the existing `/custom/:id` route:

```ts
{
  path: '/custom/performance-monitor',
  name: 'PerformanceMonitor',
  component: () => import('@/views/user/PerformanceMonitorView.vue'),
  meta: {
    requiresAuth: true,
    requiresAdmin: false,
    title: 'Performance Monitor',
    titleKey: 'nav.performanceMonitor',
  },
},
```

- [ ] **Step 4: Run the route test and verify GREEN**

Run: `pnpm vitest run src/router/__tests__/performance-monitor-route.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit route/page shell**

```bash
git add upstream/sub2api/frontend/src/router/index.ts upstream/sub2api/frontend/src/router/__tests__/performance-monitor-route.spec.ts upstream/sub2api/frontend/src/views/user/PerformanceMonitorView.vue
git commit -m "feat: add native performance monitor route"
```

### Task 2: Navigation replacement and i18n contract (RED/GREEN)

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/layout/AppSidebar.vue:700-725`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/common.ts:200-220`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/common.ts:200-220`
- Test: `upstream/sub2api/frontend/src/components/layout/__tests__/AppSidebar.performance-monitor.spec.ts`

**Interfaces:**
- Consumes: `nav.performanceMonitor` translation.
- Produces: user and admin-personal navigation item `{ path: '/custom/performance-monitor', label: t('nav.performanceMonitor'), icon: SignalIcon }`; no fixed `/monitor` item.

- [ ] **Step 1: Write the failing navigation contract test**

```ts
import { describe, expect, it } from 'vitest'

describe('performance monitor navigation contract', () => {
  it('uses the custom page path and removes the legacy fixed monitor path', () => {
    const source = readFileSync(new URL('../AppSidebar.vue', import.meta.url), 'utf8')
    expect(source).toContain("path: '/custom/performance-monitor'")
    expect(source).toContain("t('nav.performanceMonitor')")
    expect(source).not.toContain("path: '/monitor', label: t('nav.channelStatus')")
  })
})
```

- [ ] **Step 2: Run the navigation test and verify RED**

Run: `pnpm vitest run src/components/layout/__tests__/AppSidebar.performance-monitor.spec.ts`
Expected: FAIL because the current source still contains the fixed `/monitor` declaration.

- [ ] **Step 3: Implement navigation and translations**

Replace the fixed nav declaration in `buildSelfNavItems` with:

```ts
{ path: '/custom/performance-monitor', label: t('nav.performanceMonitor'), icon: SignalIcon, featureFlag: flagChannelMonitor },
```

Add `performanceMonitor: '性能监测'` to the zh common `nav` object and `performanceMonitor: 'Performance Monitor'` to the en common `nav` object.

- [ ] **Step 4: Run the navigation test and verify GREEN**

Run: `pnpm vitest run src/components/layout/__tests__/AppSidebar.performance-monitor.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit navigation changes**

```bash
git add upstream/sub2api/frontend/src/components/layout/AppSidebar.vue upstream/sub2api/frontend/src/i18n/locales/zh/common.ts upstream/sub2api/frontend/src/i18n/locales/en/common.ts upstream/sub2api/frontend/src/components/layout/__tests__/AppSidebar.performance-monitor.spec.ts
git commit -m "feat: expose performance monitor in custom navigation"
```

### Task 3: Title and affected-suite verification

**Files:**
- Modify: `upstream/sub2api/frontend/src/router/__tests__/title.spec.ts`
- Test: existing route/title and Monitor V2 suites.

- [ ] **Step 1: Add title assertion**

```ts
it('uses the translated performance monitor title', () => {
  expect(resolveDocumentTitle('Performance Monitor', 'Sub2API', 'nav.performanceMonitor'))
    .toBe('性能监测 - Sub2API')
})
```

- [ ] **Step 2: Run affected tests**

Run: `pnpm vitest run src/router/__tests__/title.spec.ts src/router/__tests__/performance-monitor-route.spec.ts src/components/layout/__tests__/AppSidebar.performance-monitor.spec.ts src/features/monitor-v2/__tests__`
Expected: all affected tests PASS.

- [ ] **Step 3: Run typecheck, build, and diff checks**

Run: `pnpm typecheck && pnpm build && git diff --check`
Expected: typecheck, production build, and whitespace checks PASS.

- [ ] **Step 4: Commit verification updates**

```bash
git add upstream/sub2api/frontend/src/router/__tests__/title.spec.ts
git commit -m "test: cover performance monitor title contract"
```

### Task 4: Candidate handoff

- [ ] Record baseline `main@989c072a8`, candidate commits, changed files, test output, no migration/config/data changes, `downtime_required=false` expectation, rollback to previous blue-green slot, and remaining manual desktop/390px login-state visual verification in `docs/handoffs/2026-08-20-t46-performance-monitor-handoff.md`.
- [ ] Set candidate status to `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or edit root ledgers from the candidate.
