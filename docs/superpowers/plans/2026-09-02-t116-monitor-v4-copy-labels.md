# T116 Monitor V4 成功率文案收敛 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Monitor V4 分组卡片收敛为“成功率”和“基于 N 次调用”，隐藏内部样本来源文案。

**Architecture:** 复用现有 `MonitorV4Group.request_count` 和 `success_rate`，只调整 hybrid 卡片模板、中文 i18n 键和值及组件测试。不改 API、后端聚合、数据合同、样式结构或其他监控页面。

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, vue-i18n, Vitest, Vue Test Utils, pnpm。

**Spec:** `docs/superpowers/specs/2026-09-02-t116-monitor-v4-copy-labels-design.md`

## Global Constraints

- 仅在 `codex/t116-monitor-v4-copy-labels` 独立 worktree 修改；不得修改根目录 `main`。
- 成功率仍由现有 `success_rate` 展示，调用次数仍由 `request_count` 展示。
- 不修改 Monitor V4 API、后端 DTO、数据库、迁移、调度、计费或部署配置。
- 用户可见文案不得出现“综合成功”“真实请求成功”“探测补足”“空桶”。
- 完成门槛为直接相关测试、必要前端类型检查和 `git diff --check` 通过；不执行无关全仓验证。

### Task 1: Lock the concise copy contract with failing tests

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts`

**Interfaces:**
- Consumes: existing `HybridPerformanceGroupCard` props and i18n keys.
- Produces: assertions requiring `成功率` and `基于 N 次调用`, with no internal source labels.

- [ ] **Step 1: Update the test mock and assertions.**

  Replace the hybrid mock branches for `requestCount`, `realRequestCount`, and `probeFallbackCount` with a `sampleCount` branch returning `基于 {count} 次调用`; return `成功率` for the hybrid `successRate` key. Assert the center label is `成功率`, the footer is `基于 20 次调用`, and the rendered card text does not contain the three removed phrases.

- [ ] **Step 2: Run the focused test to verify it fails.**

  Run from `upstream/sub2api/frontend`:

  ```bash
  pnpm vitest run src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts
  ```

  Expected: FAIL because the current component renders `体验成功率`, `综合成功 17/20 次请求`, `真实请求成功 14/15`, and `探测补足 5 个空桶`.

### Task 2: Implement the minimal component and locale change

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/HybridPerformanceGroupCard.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/channelMonitorV2.ts`

**Interfaces:**
- Consumes: `MonitorV4Group.success_rate` and `MonitorV4Group.request_count`.
- Produces: card text using `channelMonitorV2.hybrid.successRate` and `channelMonitorV2.hybrid.sampleCount`.

- [ ] **Step 1: Change only the card labels and footer.**

  Keep the existing success-rate computation, tones, metrics, and styling. Change the footer to one `data-test="sample-count"` span calling `t('channelMonitorV2.hybrid.sampleCount', { count: group.request_count })`; remove the three source-count spans. Keep all source-count fields in the TypeScript type and API validation untouched.

- [ ] **Step 2: Change the Chinese hybrid translations.**

  Set `successRate` to `成功率`, add `sampleCount: '基于 {count} 次调用'`, and remove the obsolete hybrid translation entries for `requestCount`, `realRequestCount`, and `probeFallbackCount`. Do not alter unrelated `channelMonitorV2.metrics.successRate` or English locale entries.

- [ ] **Step 3: Run the focused test to verify it passes.**

  ```bash
  pnpm vitest run src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts
  ```

  Expected: PASS for all existing and updated component cases.

### Task 3: Run direct verification and commit the candidate

**Files:**
- Inspect: `upstream/sub2api/frontend/src/features/monitor-v4/HybridPerformanceGroupCard.vue`
- Inspect: `upstream/sub2api/frontend/src/i18n/locales/zh/channelMonitorV2.ts`
- Inspect: `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts`

- [ ] **Step 1: Run Monitor V4 direct tests.**

  ```bash
  pnpm vitest run src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts src/features/monitor-v4/__tests__/HybridPerformanceView.spec.ts src/features/monitor-v4/__tests__/api.spec.ts
  ```

- [ ] **Step 2: Run the required frontend type check.**

  ```bash
  pnpm typecheck
  ```

- [ ] **Step 3: Check the diff and forbidden copy.**

  ```bash
  git diff --check
  ! rg -n '综合成功|真实请求成功|探测补足|空桶' upstream/sub2api/frontend/src/features/monitor-v4 upstream/sub2api/frontend/src/i18n/locales/zh/channelMonitorV2.ts
  ```

- [ ] **Step 4: Commit only the T116 implementation and tests.**

  ```bash
  git add upstream/sub2api/frontend/src/features/monitor-v4/HybridPerformanceGroupCard.vue upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts upstream/sub2api/frontend/src/i18n/locales/zh/channelMonitorV2.ts
  git commit -m "fix: simplify monitor v4 success labels"
  ```

  Do not merge, push, deploy, or modify project-wide queue/progress records from this worktree after the candidate commit.
