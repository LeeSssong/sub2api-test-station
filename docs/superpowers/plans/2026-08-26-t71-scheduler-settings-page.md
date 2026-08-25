# T71 Scheduler Settings Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the OpenAI operational scheduler policy into a dedicated administrator page with visible selection feedback and live scenario previews.

**Architecture:** Extract the existing T68 form state, policy compatibility normalization, serialization, and save path from `SettingsView` into a focused scheduler page/component. Route and sidebar use existing administrator patterns, while the component continues to send the existing settings payload and relies on the existing server-side compilation path.

**Tech Stack:** Vue 3 Composition API, TypeScript, Vue Router, Pinia, Vue I18n, Vitest, Tailwind utility classes.

**Spec:** `docs/superpowers/specs/2026-08-26-t71-scheduler-settings-page-design.md`

## Global Constraints

- Reuse the existing Sub2API settings API and `openai_advanced_scheduler_*` fields.
- Keep service continuity as a fixed non-configurable guard.
- Do not add a backend API, migration, scheduler, config store, dependency, or production-data write path.
- Preserve legacy policy fallback and server-controlled compiled snapshots.
- No GitHub Actions; production release only via the local/host blue-green script after root authorization.

---

### Task 1: Extract Testable Scheduler Policy State

**Files:**
- Create: `upstream/sub2api/frontend/src/views/admin/scheduler/schedulerPolicy.ts`
- Create: `upstream/sub2api/frontend/src/views/admin/scheduler/__tests__/schedulerPolicy.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/SettingsView.vue`

- [ ] Write failing tests for valid recommended priorities, selected option state, and all three scenario preview branches.
- [ ] Run the focused Vitest test and confirm it fails because the module is missing.
- [ ] Move pure priority validation, recommendation, summary, and scenario derivation into `schedulerPolicy.ts`.
- [ ] Run focused Vitest and confirm it passes.
- [ ] Remove the now-unused pure helpers from `SettingsView` without changing unrelated settings behavior.
- [ ] Commit the tested extraction.

### Task 2: Build the Dedicated Scheduler Workbench

**Files:**
- Create: `upstream/sub2api/frontend/src/views/admin/SchedulerSettingsView.vue`
- Create: `upstream/sub2api/frontend/src/views/admin/__tests__/SchedulerSettingsView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/SettingsView.vue`

- [ ] Write failing component tests for group selection, numeric selected styles, three segment selected styles, balance-preview updates, save success, and save failure.
- [ ] Run focused Vitest and confirm expected failures.
- [ ] Implement the page with existing settings/groups APIs and current policy serialization semantics.
- [ ] Implement keyboard/focus/disabled/loading states and responsive light/dark styling matching the confirmed workbench design.
- [ ] Remove the visible T68 panel from `SettingsView` and its unused state/imports.
- [ ] Run focused Vitest and confirm it passes.
- [ ] Commit the page and regression coverage.

### Task 3: Register the Administrator Entry

**Files:**
- Modify: `upstream/sub2api/frontend/src/router/index.ts`
- Modify: `upstream/sub2api/frontend/src/components/layout/AppSidebar.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/*.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/*.ts`
- Create or modify: direct router/sidebar Vitest tests adjacent to established tests

- [ ] Write failing tests for the admin-only route and visible admin navigation label.
- [ ] Run the focused tests and confirm they fail before registrations exist.
- [ ] Add the guarded route, localized document title and administrator sidebar entry.
- [ ] Run focused tests and confirm they pass.
- [ ] Commit route, navigation, locale, and test changes.

### Task 4: Verify Visual and Build Quality

**Files:**
- Modify only where direct visual/test findings require correction.

- [ ] Start the frontend development server and inspect desktop and narrow-screen page states in a real browser.
- [ ] Verify focus rings, selected states, balance/high-peak/session previews, loading and save failure behavior.
- [ ] Apply only findings-backed visual fixes; re-inspect screenshots after each material change.
- [ ] Run `pnpm vitest run` for direct scheduler/router/sidebar tests, `pnpm typecheck`, `pnpm build`, and `git diff --check`.
- [ ] Record baseline SHA, candidate SHA, changed files, tests, no-migration/no-config-schema statement, deployment precheck requirement, rollback commit, and residual risks in the task handoff.
