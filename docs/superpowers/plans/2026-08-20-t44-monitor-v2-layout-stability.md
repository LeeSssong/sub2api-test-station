# T44 Monitor V2 Layout Stability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Monitor V2 timeline bars fill desktop card width without excess whitespace while keeping mobile scrolling internal and removing hover-induced card jitter.

**Architecture:** Keep `MonitorV2Timeline` and `MonitorV2GroupCard` as the only production components touched. Use CSS Grid with a `--timeline-count` custom property on desktop, a fixed-width flex track under the mobile breakpoint, and geometry-neutral color/shadow transitions on cards and points. Existing props, data attributes, tooltip logic, and API contracts remain unchanged.

**Tech Stack:** Vue 3 SFC, Tailwind utility classes, scoped CSS, Vitest + Vue Test Utils, pnpm/vue-tsc/Vite.

**Spec:** `docs/superpowers/specs/2026-08-20-t44-monitor-v2-layout-stability-design.md`

## Global Constraints

- Do not modify Monitor V2 v7 JSON, API types, native probe source, or 24/28/30 bucket generation.
- Do not modify backend, migrations, configuration, production data, or `.github/workflows`.
- Keep mobile horizontal scrolling inside `data-timeline-scroll`; no page-level overflow.
- Keep one-pixel card/point borders and fixed tooltip-row geometry so hover cannot change layout.
- Stop at `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or edit global queue/progress ledgers.

### Task 1: Lock the new responsive timeline and stable-hover contracts with RED tests

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`

**Interfaces:**
- Consumes: existing `MonitorV2Timeline` props and `MonitorV2View` fixture.
- Produces: failing assertions that require `--timeline-count`, responsive track classes, no geometry transforms, and stable card transitions.

- [ ] **Step 1: Replace the old fixed-width/hover-lift assertions.**

In the first timeline test, assert the point keeps `h-5`, has no `hover:-translate-y-1`, no `hover:scale-y-110`, and the track exposes a count style for one point. In the dense timeline test, assert the scroll wrapper remains `overflow-x-auto`, the track exposes a mobile minimum width, and the point has a mobile fixed-width contract rather than the desktop-only `w-[6px]` class. Add a test with 24, 28, and 30 points that checks `data-timeline-track` style contains `--timeline-count: N` for each N.

- [ ] **Step 2: Add a card stability assertion.**

Update `MonitorV2View.spec.ts` to require `transition-colors` and `transition-shadow`, retain hover background/focus styling, and assert the card does not contain `transition-all` or `hover:-translate-y-0.5`.

- [ ] **Step 3: Run the focused tests and verify RED.**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts src/features/monitor-v2/__tests__/MonitorV2View.spec.ts
```

Expected: FAIL because the current components still use fixed `w-[6px]`, fixed desktop width, and transform/`transition-all` hover classes.

### Task 2: Implement the responsive timeline track without changing data behavior

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2Timeline.vue`

**Interfaces:**
- Consumes: `points.length` and existing point event/tooltip handlers.
- Produces: `data-timeline-track` with `--timeline-count`, desktop equal-width columns, mobile fixed-width flex track, and unchanged point semantics.

- [ ] **Step 1: Add a deterministic track style computed from point count.**

Add:

```ts
const trackStyle = computed(() => ({
  '--timeline-count': String(Math.max(props.points.length, 1)),
} as Record<string, string>))
```

Bind `:style="trackStyle"` to `data-timeline-track`; leave an empty timeline with count 1 so the placeholder remains visible.

- [ ] **Step 2: Replace fixed desktop sizing with responsive classes and scoped rules.**

Change the root to `relative w-full min-w-0 max-w-full`; remove `sm:w-[620px] sm:flex-shrink-0`. Keep the scroll wrapper as `max-w-full overflow-x-auto overflow-y-hidden overscroll-x-contain`.

Use a track class that can be selected by scoped CSS, retaining padding and gap but not `min-w-max` on desktop. Add scoped rules:

```css
[data-timeline-track] {
  display: grid;
  grid-template-columns: repeat(var(--timeline-count), minmax(0, 1fr));
  min-width: 0;
}

[data-timeline-point] {
  width: auto;
  min-width: 4px;
}

@media (max-width: 639px) {
  [data-timeline-track] {
    display: flex;
    min-width: 620px;
  }

  [data-timeline-point] {
    width: 6px;
    flex: 0 0 6px;
  }
}
```

Keep the no-data placeholder full-width; when points are present, each bar fills its grid cell on desktop and remains 6px on mobile. Do not modify tooltip handlers or point state mapping.

- [ ] **Step 3: Remove point transforms while retaining accessible feedback.**

Replace `transition-all duration-200 ease-out hover:-translate-y-1 hover:scale-y-110 hover:shadow... focus-visible:-translate-y-1 focus-visible:scale-y-110` with `transition-[box-shadow,filter,background-color] duration-200 ease-out hover:shadow... focus-visible:outline-none focus-visible:ring-2...`. Preserve cursor, role, tabindex, and state classes.

- [ ] **Step 4: Run timeline tests and verify GREEN.**

Run the focused timeline test command from Task 1. Expected: PASS, including 24/28/30 style contracts, inner scroll, tooltip behavior, and no transform assertions.

### Task 3: Remove card geometry changes and verify the page matrix

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2GroupCard.vue`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`

**Interfaces:**
- Consumes: existing `group` props and status/multiplier classes.
- Produces: geometry-stable card hover/focus behavior.

- [ ] **Step 1: Replace card `transition-all` and translate hover.**

Keep `border`/`dark:border` classes unchanged at one pixel. Replace `transition-all duration-300 ease-out hover:-translate-y-0.5` with `transition-[background-color,border-color,box-shadow] duration-200 ease-out`; retain existing hover/focus border, background, and shadow classes. Keep padding and grid columns unchanged so no content contract changes.

- [ ] **Step 2: Keep timeline column width governed by parent grid.**

Retain the existing two-column desktop header but allow the timeline child to shrink with `min-w-0`; do not reintroduce fixed pixel width. If needed, add `min-w-0` to the timeline grid cell only, without changing breakpoints or content order.

- [ ] **Step 3: Run the Monitor V2 view test.**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/features/monitor-v2/__tests__/MonitorV2View.spec.ts
```

Expected: PASS with no legacy transform assertions and stable card styling.

### Task 4: Full direct validation, documentation, and handoff

**Files:**
- Create: `docs/superpowers/reports/2026-08-20-t44-monitor-v2-layout-stability-verification.md`
- Create: `docs/handoffs/2026-08-20-t44-monitor-v2-layout-stability-handoff.md`

- [ ] **Step 1: Run the direct Monitor V2 Vitest matrix.**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts src/features/monitor-v2/__tests__/MonitorV2View.spec.ts src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts src/features/monitor-v2/__tests__/CodexRadarRecommendations.spec.ts src/features/monitor-v2/__tests__/CodexRadarCommunityMatrix.spec.ts src/features/monitor-v2/__tests__/api.spec.ts
```

Record test files, test counts, and exit codes in the verification report.

- [ ] **Step 2: Run typecheck and production build.**

```bash
cd upstream/sub2api/frontend
pnpm typecheck
pnpm build
```

Record both exit codes and notable output in the report.

- [ ] **Step 3: Run scope and formatting guards.**

```bash
git diff --check 2d7c38de2c932478ed82b415e201855ef75839e4...HEAD --
git diff --name-only 2d7c38de2c932478ed82b415e201855ef75839e4...HEAD -- | grep -E '(^|/)(migrations|\.github/workflows)(/|$)' || true
git diff --name-only 2d7c38de2c932478ed82b415e201855ef75839e4...HEAD -- | grep -E 'docs/project/(project-progress|native-sub-task-package-queue)\.md' || true
```

Expected: `git diff --check` exits 0; the two path scans produce no output. Confirm no backend, migration, config, or production files changed.

- [ ] **Step 4: Write verification report and handoff.**

Bind the report and handoff to the final HEAD/tree. Include baseline, commits, changed files, test evidence, unverified real-device/browser checks, migration/config status, `downtime_required=false` candidate expectation, rollback, and remaining CSS/browser risks. State `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or update global ledgers.

- [ ] **Step 5: Commit implementation and docs.**

```bash
git add upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2Timeline.vue \
  upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2GroupCard.vue \
  upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts \
  upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts \
  docs/superpowers/reports/2026-08-20-t44-monitor-v2-layout-stability-verification.md \
  docs/handoffs/2026-08-20-t44-monitor-v2-layout-stability-handoff.md

git commit -m "fix: stabilize monitor v2 timeline layout"
```

- [ ] **Step 6: Final clean-tree evidence.**

```bash
git status --short --branch
git rev-parse HEAD^{tree}
```

Expected: clean tracked worktree on `codex/t44-monitor-v2-layout-stability`; report the exact SHA/tree to the root task.
