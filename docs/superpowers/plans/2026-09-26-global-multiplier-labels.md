# Global Multiplier Labels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render backend-provided multiplier values consistently as `{value}x倍率` in Chinese-facing display surfaces across user and admin views.

**Architecture:** Preserve data resolution and all calculations. Centralize the display label in the existing multiplier formatting utility, then migrate display-only callers, including the shared group badge, line selector, monitor, usage, subscription, and admin views. Keep unavailable values unavailable; leave editable numeric fields and raw CSV untouched.

**Tech Stack:** Vue 3, TypeScript, vue-i18n, Vitest, pnpm.

---

### Task 1: Shared Label Contract

**Files:** `upstream/sub2api/frontend/src/utils/formatters.ts`, `upstream/sub2api/frontend/src/utils/__tests__/formatMultiplier.spec.ts`

- [ ] Add failing assertions for `0.12x倍率`, configured integer, small decimal and unavailable values.
- [ ] Run `pnpm exec vitest run src/utils/__tests__/formatMultiplier.spec.ts` and confirm an expected assertion failure.
- [ ] Add a display-only label helper built on the existing numeric formatter; leave its existing numeric API unchanged.
- [ ] Rerun the focused test and confirm pass.

### Task 2: User-Facing Multiplier Displays

**Files:** `upstream/sub2api/frontend/src/components/keys/lineOptions.ts`, `upstream/sub2api/frontend/src/components/common/{GroupBadge,GroupOptionItem}.vue`, `upstream/sub2api/frontend/src/views/user/{DashboardView,PaymentView,SubscriptionsView}.vue`, `upstream/sub2api/frontend/src/components/modelPlaza/{PlazaFilterBar,PlazaModelPricingTable}.vue`, and their directly related specs.

- [ ] Update targeted specs to expect the confirmed label and verify user-specific override plus backend group fallback.
- [ ] Run focused Vitest specs to observe failure on the old markup.
- [ ] Migrate display-only bindings to the shared helper; do not alter rate source or calculations.
- [ ] Rerun focused specs and inspect desktop/390px layout.

### Task 3: Admin and Monitoring Displays

**Files:** display-only multiplier bindings in `upstream/sub2api/frontend/src/{components,features,views/admin}`, their directly related specs, and Chinese locale messages that embed multiplier values.

- [ ] Add focused assertions for representative account, group, usage, and monitor labels.
- [ ] Verify failures, migrate display-only bindings, and preserve source semantics and English locale formatting.
- [ ] Run related component specs, i18n checks, typecheck, build and `git diff --check`.

### Task 4: Integration and Test-Station Release

- [ ] Review the complete diff and exclude unrelated files.
- [ ] Commit candidate after focused verification, integrate into root `main` only through the release-controller boundary.
- [ ] Stop if the root worktree is not clean; do not remove or overwrite pre-existing files to bypass the gate.
- [ ] After `main == origin/main` commit/tree and clean-source checks pass, invoke `ops/release-sub2api-test-station.sh` from the root, then verify release-state, health, logged-in UI, and rollback status.
