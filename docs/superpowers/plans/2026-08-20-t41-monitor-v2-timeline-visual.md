# T41 Monitor V2 Timeline视觉与 Tooltip 交互优化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Monitor V2 时间线 Tooltip 下置到独立布局行、放大时间线视觉比例，并明确无探测数据桶。

**Architecture:** 仅扩展 `MonitorV2Timeline.vue` 的现有派生状态、布局和定位算法；数据仍由 v7 原生快照直接驱动。通过 `data-timeline-tooltip-row` 建立不与柱体重叠的布局边界，并以本地化键表达 no-data 桶。

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, Tailwind utility classes, Vitest + Vue Test Utils, vue-i18n。

**Spec:** `docs/superpowers/specs/2026-08-20-t41-monitor-v2-timeline-visual-design.md`

## Global Constraints

- 保持 Monitor V2 contract version `7`。
- 保持 24h/7d/30d 的 24/28/30 桶契约与原生探测来源。
- 保持移动端整页无横向溢出，横滚仅发生在时间线内部。
- 不修改后端/API/迁移/配置/发布链或全局项目账。

### Task 1: RED — 时间线视觉与 no-data 行为测试

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts`

- [ ] **Step 1: Add failing assertions**
  - Assert an active tooltip is rendered inside `[data-timeline-tooltip-row]`, not with the legacy `top-[4.25rem]` class.
  - Assert bars use `h-5`, `w-[6px]`, and `gap-[5px]`.
  - Add an `unavailable` + `latency_ms:null` point and assert `data-timeline-point-state="no-data"`, gray/dashed styling, `NO DATA`, and localized “无探测数据”.
  - Assert the tooltip row exists and precedes labels, providing a dedicated layout boundary.

- [ ] **Step 2: Run RED**
  - Run `pnpm vitest run src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts` from `upstream/sub2api/frontend`.
  - Expected: FAIL because current component has no tooltip row/no-data state and retains old dimensions.

### Task 2: GREEN — Implement component and locale contract

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2Timeline.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts`

- [ ] **Step 1: Add no-data derivation**
  - Implement `isNoDataPoint(point)` as `point.status === 'unavailable' && point.latency_ms === null`.
  - Use it for point state attribute/class, tooltip heading, status copy, and aria label.

- [ ] **Step 2: Move Tooltip to dedicated row**
  - Remove `pt-12` reservation and legacy top offset from the scroll area.
  - Add `[data-timeline-tooltip-row]` with a stable minimum height below the bars and above timestamp labels.
  - Render Tooltip absolutely within that row, with a larger minimum width/padding and upward arrow; preserve `pointer-events-none`.

- [ ] **Step 3: Resize bars and preserve mobile scroll**
  - Set bar classes to `h-5 w-[6px]`, track gap to `[5px]`, retain `min-w-max`, `overflow-x-auto`, and `shrink-0`.
  - Keep empty-array fallback and labels unchanged.

- [ ] **Step 4: Update positioning math**
  - Change tooltip half-width clamp to 98px (matching the 196px minimum width).
  - Keep root/point DOMRect measurement and scroll-triggered repositioning.

- [ ] **Step 5: Add locale strings**
  - Add `timeline.noDataBucket` and `timeline.noDataBucketLabel` in zh/en dashboard locale files.

- [ ] **Step 6: Run GREEN focused tests**
  - Run `pnpm vitest run src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts`.
  - Expected: all tests pass.

### Task 3: Validation and handoff

**Files:**
- Create: `docs/handoffs/2026-08-20-t41-monitor-v2-timeline-visual-handoff.md`

- [ ] **Step 1: Run direct monitor tests**
  - `pnpm vitest run src/features/monitor-v2/__tests__`

- [ ] **Step 2: Run type/build and diff checks**
  - `pnpm typecheck`
  - `pnpm build`
  - `git diff --check`

- [ ] **Step 3: Self-review scope**
  - Confirm `git diff --name-status` contains only T41 component/tests/locales/spec/plan/handoff files.
  - Confirm no API/backend/migration/config/global ledger changes.

- [ ] **Step 4: Commit and write READY handoff**
  - Commit with `git add ... && git commit -m "fix: refine monitor v2 timeline tooltip layout"`.
  - Record baseline SHA, commit SHA/tree, tests, unverified browser evidence, migration/config status, `downtime_required=false`, rollback and residual risk.
