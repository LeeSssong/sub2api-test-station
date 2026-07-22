# Xingqiao Homepage Motion Reconstruction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Deliver an original Xingqiao motion system with the full visible motion inventory established in the approved design.

**Architecture:** A dedicated canvas and entry hook drive the hero. Shared reveal variants handle viewport choreography. Motion scroll values drive route and statement scenes directly, while reduced motion and mobile use stable final states.

**Tech Stack:** React 19, TypeScript, Motion 12, Lenis 1.3.25, Vitest, Testing Library, Vite.

## Global Constraints

- Preserve Xingqiao copy, Seoul direct-connect claims, OpenAI/Anthropic compatibility, QQ `1080152144`, and current session CTA behavior.
- Do not vendor, serve, import, or reference Shuai API code, classes, assets, or logos.
- Use only `/home-assets/xingqiao-logo.png` for the site mark.
- Keep desktop sticky scenes out of the mobile and reduced-motion layouts.
- Clean up every animation frame, interval, event listener, and observer.
- Maintain a semantic static fallback for every canvas or animated surface.

---

### Task 1: Hero Motion Primitives

**Files:**
- Create: `homepage/src/components/HeroSignalCanvas.tsx`
- Create: `homepage/src/hooks/useHeroEntry.ts`
- Modify: `homepage/src/sections/HeroSection.tsx`
- Modify: `homepage/src/sections/HeroSection.test.tsx`
- Modify: `homepage/src/styles.css`

**Interfaces:**
- `HeroSignalCanvas` accepts `active: boolean` and emits `data-canvas-active`.
- `useHeroEntry` returns `{ started: boolean; origin: { x: string; y: string }; start(event?: PointerEvent): void }`.
- `HeroSection` retains `apiOrigin` and `session` props.

- [ ] **Step 1: Write failing tests**

```tsx
expect(screen.getByLabelText('星桥实时信号背景')).toHaveAttribute('data-canvas-active', 'false')
expect(screen.getByLabelText('星桥首页首屏')).toHaveAttribute('data-entry-state', 'final')
```

- [ ] **Step 2: Run the focused test**

Run: `cd homepage && npm run test:run -- src/sections/HeroSection.test.tsx`

Expected: FAIL because the hero lacks canvas and entry-state contracts.

- [ ] **Step 3: Implement original background and entry hook**

Create an offscreen-canvas row field from API-themed local strings. Cap DPR at two, randomize only the supplied local row descriptors, expose static fallback text, start/stop drawing through `IntersectionObserver`, and add pointer disturbance. Add the once-only pointer origin hook with coarse/fine delay and reduced-motion final state.

- [ ] **Step 4: Compose the hero and styles**

Replace `pre.hero-signal` with `HeroSignalCanvas`, apply entry CSS variables, ambient/grid/ripple layers, hero scroll transforms, pulsing status dot, endpoint typing cadence, cursor blink, and a static no-motion branch.

- [ ] **Step 5: Verify focused test**

Run: `cd homepage && npm run test:run -- src/sections/HeroSection.test.tsx`

Expected: PASS.

### Task 2: Shared Reveal And Model Flow

**Files:**
- Modify: `homepage/src/components/Reveal.tsx`
- Modify: `homepage/src/sections/ValueSections.tsx`
- Create: `homepage/src/sections/ModelFlow.tsx`
- Modify: `homepage/src/sections/ValueSections.test.tsx`
- Modify: `homepage/src/styles.css`

**Interfaces:**
- `Reveal` accepts `animation?: 'fade-up' | 'fade-in' | 'scale-in' | 'mask-rise' | 'rule-line'`, `delay?: number`, `threshold?: number`, `once?: boolean`.
- `ModelFlow` renders the unchanged six model names and an accessible `模型路由示意` label.

- [ ] **Step 1: Write failing tests**

```tsx
expect(screen.getByLabelText('模型路由示意')).toHaveAttribute('data-motion-state', 'final')
expect(screen.getByText('OpenAI').parentElement).toHaveStyle({ '--flow-stagger': '0s' })
expect(screen.getByText('GLM').parentElement).toHaveStyle({ '--flow-stagger': '2.25s' })
```

- [ ] **Step 2: Run focused tests**

Run: `cd homepage && npm run test:run -- src/sections/ValueSections.test.tsx`

Expected: FAIL because no dedicated model-flow state or stagger variables exist.

- [ ] **Step 3: Implement contracts and styles**

Give observer variants exact root margin `0px 0px -40px 0px`, add the flow-progress Motion value, traveling dash, sequential chip pulse, CSS reveal variants, and only small card hover feedback.

- [ ] **Step 4: Verify focused tests**

Run: `cd homepage && npm run test:run -- src/sections/ValueSections.test.tsx`

Expected: PASS.

### Task 3: Request Sequence Fidelity

**Files:**
- Modify: `homepage/src/sections/RequestJourney.tsx`
- Modify: `homepage/src/sections/RequestJourney.test.tsx`
- Modify: `homepage/src/styles.css`

**Interfaces:**
- `RequestJourney` preserves `aria-label="一次 API 请求的完整旅程"`.
- Its root exposes `data-journey-phase` with `send`, `route`, `observe`, or `static`.

- [ ] **Step 1: Extend tests for final and sequential states**

```tsx
expect(screen.getByLabelText('一次 API 请求的完整旅程')).toHaveAttribute('data-journey-mode', 'static')
expect(screen.getByText('187')).toBeVisible()
expect(screen.getByText('2,148')).toBeVisible()
```

- [ ] **Step 2: Run focused tests**

Run: `cd homepage && npm run test:run -- src/sections/RequestJourney.test.tsx`

Expected: FAIL because the explicit journey-mode contract does not exist.

- [ ] **Step 3: Implement Motion-value progress**

Use interpolated values for fill, dot positions, opacity, provider lighting, gateway ring, metric count-up, and phase text. Use 340svh/sticky only above 680px and when motion is allowed. Make every mobile/reduced state complete without scroll-dependent hidden content.

- [ ] **Step 4: Verify focused tests**

Run: `cd homepage && npm run test:run -- src/sections/RequestJourney.test.tsx`

Expected: PASS.

### Task 4: Footer Canvas, Smooth Scroll, And Integration Cursor

**Files:**
- Modify: `homepage/src/sections/BrandReveal.tsx`
- Modify: `homepage/src/sections/BrandReveal.test.tsx`
- Modify: `homepage/src/sections/IntegrationSection.tsx`
- Modify: `homepage/src/App.tsx`
- Modify: `homepage/src/styles.css`

**Interfaces:**
- `BrandReveal` keeps heading `星桥` and `data-canvas-active`.
- `App` creates Lenis only when reduced motion is not requested and tears it down on unmount.

- [ ] **Step 1: Write failing tests**

```tsx
expect(screen.getByLabelText('星桥品牌揭幕')).toHaveAttribute('data-canvas-active', 'false')
expect(screen.getByText('|')).toHaveClass('terminal-cursor')
```

- [ ] **Step 2: Run focused tests**

Run: `cd homepage && npm run test:run -- src/sections/BrandReveal.test.tsx src/App.test.tsx`

Expected: FAIL because the integration cursor and explicit motion contracts are absent.

- [ ] **Step 3: Implement final motion layers**

Tune the existing brand cell canvas to an offscreen glyph source, pointer-velocity displacement, visibility pause, decay, and responsive word sizing. Add Lenis lifecycle in `App`, terminal cursor styling, and complete reduced-motion overrides.

- [ ] **Step 4: Verify focused tests**

Run: `cd homepage && npm run test:run -- src/sections/BrandReveal.test.tsx src/App.test.tsx`

Expected: PASS.

### Task 5: Whole-Page Validation

**Files:**
- Modify: `homepage/design-qa.md`

- [ ] **Step 1: Run the static suite**

Run: `cd homepage && npm run test:run && npm run typecheck && npm run build`

Expected: all commands exit zero.

- [ ] **Step 2: Run browser motion evidence**

At desktop and `390x844`, capture the hero at two animation frames, desktop request scene at 10%, 45%, and 85% section progress, and reduced-motion static state. Confirm no console errors, hero canvas changes between regular-motion frames, no sticky framing on mobile, and semantic text remains visible.

- [ ] **Step 3: Record result**

Write exact commands, viewport results, remaining risks, and `final result: passed` to `homepage/design-qa.md` only after all checks pass.
