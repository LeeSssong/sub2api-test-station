# Homepage Theme and Layered Signal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Beijing-time light/dark homepage theme with temporary manual overrides and make the canvas signal field visibly layered, theme-aware, accessible, and performant.

**Architecture:** A pure `themeSchedule` domain module owns all time and persistence rules. A focused `useHomepageTheme` hook applies those rules to the document and exposes the toggle to `Header`. The existing single-canvas signal field gains deterministic layer descriptors and listens for theme changes without introducing WebGL or another rendering surface.

**Tech Stack:** React 19, TypeScript 7, Vite 8, Vitest, Testing Library, Canvas 2D, CSS custom properties, Lucide React.

## Global Constraints

- Automatic light mode is 06:00:00–18:59:59 Beijing time; dark mode is 19:00:00–05:59:59.
- A manual choice persists only until the next Beijing-time 06:00 or 19:00 boundary.
- Scope is the homepage only; `/support`, docs, console, and upstream Sub2API UI remain unchanged.
- Do not add WebGL, new Canvas elements, new runtime dependencies, or production deployment steps.
- Preserve the existing brand cyan/blue vocabulary and meet WCAG 2.1 AA for text and interactive controls.
- Keep `prefers-reduced-motion` as a static, meaningful fallback.
- Use the existing `Public Sans`/system typography and existing Lucide icon set.

---

### Task 1: Beijing-Time Theme Domain

**Files:**
- Create: `homepage/src/domain/themeSchedule.ts`
- Create: `homepage/src/domain/themeSchedule.test.ts`

**Interfaces:**
- Produces: `Theme = 'light' | 'dark'`
- Produces: `ThemeOverride = { theme: Theme; expiresAt: number }`
- Produces: `themeForBeijingTime(now: Date): Theme`
- Produces: `nextThemeBoundary(now: Date): Date`
- Produces: `parseThemeOverride(raw: string | null, now: Date): ThemeOverride | null`
- Produces: `createThemeOverride(theme: Theme, now: Date): ThemeOverride`

- [ ] **Step 1: Write failing boundary and override tests**

```ts
it.each([
  ['2026-07-28T21:59:59.000Z', 'dark'],
  ['2026-07-28T22:00:00.000Z', 'light'],
  ['2026-07-29T10:59:59.000Z', 'light'],
  ['2026-07-29T11:00:00.000Z', 'dark'],
])('maps %s to %s in Beijing', (iso, expected) => {
  expect(themeForBeijingTime(new Date(iso))).toBe(expected)
})

it('expires a manual override at the next Beijing boundary', () => {
  const now = new Date('2026-07-29T03:00:00.000Z')
  expect(createThemeOverride('dark', now)).toEqual({
    theme: 'dark',
    expiresAt: Date.parse('2026-07-29T11:00:00.000Z'),
  })
})

it('rejects malformed and expired stored overrides', () => {
  const now = new Date('2026-07-29T11:00:00.000Z')
  expect(parseThemeOverride('not-json', now)).toBeNull()
  expect(parseThemeOverride('{"theme":"light","expiresAt":1753786800000}', now)).toBeNull()
})
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `npm test -- --run src/domain/themeSchedule.test.ts`  
Expected: FAIL because `themeSchedule` does not exist.

- [ ] **Step 3: Implement the pure UTC+8 schedule**

Use UTC getters on `new Date(now.getTime() + 8 * 60 * 60 * 1000)` so behavior is independent of the visitor’s locale. Calculate boundaries in UTC from Beijing calendar fields and validate parsed JSON without throwing.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run: `npm test -- --run src/domain/themeSchedule.test.ts`  
Expected: all theme domain tests PASS.

- [ ] **Step 5: Commit the domain**

```bash
git add homepage/src/domain/themeSchedule.ts homepage/src/domain/themeSchedule.test.ts
git commit -m "feat: add Beijing homepage theme schedule"
```

---

### Task 2: Homepage Theme Runtime and Flash-Free Bootstrap

**Files:**
- Create: `homepage/src/hooks/useHomepageTheme.ts`
- Create: `homepage/src/hooks/useHomepageTheme.test.ts`
- Create: `homepage/src/themeBootstrap.ts`
- Create: `homepage/src/themeBootstrap.test.ts`
- Modify: `homepage/index.html`
- Modify: `homepage/src/main.tsx`

**Interfaces:**
- Consumes: all Task 1 theme-domain exports.
- Produces: `applyHomepageTheme(theme: Theme): void`
- Produces: `bootstrapHomepageTheme(pathname: string, now?: Date): Theme | null`
- Produces: `useHomepageTheme(): { theme: Theme; toggleTheme(): void }`
- Storage key: `xingqiao-home-theme-override`
- Document event: `xingqiao-theme-change` with `{ detail: { theme: Theme } }`

- [ ] **Step 1: Write failing bootstrap and hook tests**

Cover:

```ts
expect(bootstrapHomepageTheme('/', new Date('2026-07-29T03:00:00Z'))).toBe('light')
expect(document.documentElement).toHaveAttribute('data-theme', 'light')
expect(bootstrapHomepageTheme('/support', new Date('2026-07-29T03:00:00Z'))).toBeNull()
```

Render a hook harness with fake timers, click its toggle, assert local storage contains a dark override expiring at 19:00 Beijing, advance to the boundary, and assert the theme returns to dark automatic mode with the override removed.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `npm test -- --run src/themeBootstrap.test.ts src/hooks/useHomepageTheme.test.ts`  
Expected: FAIL because bootstrap and hook modules do not exist.

- [ ] **Step 3: Implement document application and bootstrap**

`applyHomepageTheme` must set:

```ts
document.documentElement.dataset.theme = theme
document.documentElement.style.colorScheme = theme
document.querySelector('meta[name="theme-color"]')
  ?.setAttribute('content', theme === 'dark' ? '#090b10' : '#f4f7fb')
window.dispatchEvent(new CustomEvent('xingqiao-theme-change', { detail: { theme } }))
```

Call `bootstrapHomepageTheme(window.location.pathname)` in `main.tsx` before `createRoot`. Do not apply homepage theme to `/support`.

- [ ] **Step 4: Implement the runtime hook**

Initialize from the already-applied document theme. On toggle, create an override using the current clock. Schedule the next boundary with `window.setTimeout`, re-evaluate on `visibilitychange` and `focus`, and remove every timer/listener during cleanup.

- [ ] **Step 5: Verify focused and entry-point tests**

Run: `npm test -- --run src/themeBootstrap.test.ts src/hooks/useHomepageTheme.test.ts src/main.test.tsx`  
Expected: PASS with no warnings.

- [ ] **Step 6: Commit the runtime**

```bash
git add homepage/index.html homepage/src/main.tsx homepage/src/themeBootstrap.ts homepage/src/themeBootstrap.test.ts homepage/src/hooks/useHomepageTheme.ts homepage/src/hooks/useHomepageTheme.test.ts
git commit -m "feat: run homepage theme on Beijing time"
```

---

### Task 3: Accessible Theme Control

**Files:**
- Modify: `homepage/src/main.tsx`
- Modify: `homepage/src/App.tsx`
- Modify: `homepage/src/components/Header.tsx`
- Modify: `homepage/src/components/Header.test.tsx`
- Modify: `homepage/src/App.test.tsx`

**Interfaces:**
- Consumes: `useHomepageTheme()` from Task 2.
- `App` receives `theme: Theme` and `onToggleTheme: () => void`.
- `Header` receives the same theme control props.

- [ ] **Step 1: Write failing theme-button tests**

```tsx
render(<Header
  session={guest}
  theme="dark"
  onToggleTheme={onToggleTheme}
/>)
await user.click(screen.getByRole('button', { name: '切换到白天模式' }))
expect(onToggleTheme).toHaveBeenCalledOnce()
```

Rerender with `theme="light"` and assert the accessible name becomes `切换到黑夜模式`. Assert the light theme shows the Moon icon (target theme) and the dark theme shows the Sun icon.

- [ ] **Step 2: Run component tests and verify RED**

Run: `npm test -- --run src/components/Header.test.tsx src/App.test.tsx`  
Expected: FAIL because theme props and click behavior are missing.

- [ ] **Step 3: Wire the hook through the homepage**

Call `useHomepageTheme` only inside `HomeRuntimeApp`. Pass the result through `App` to `Header`. Keep `/support` isolated.

- [ ] **Step 4: Implement the 44px accessible control**

Use Lucide `Sun` and `Moon`. The icon and copy describe the action, not the current state. Preserve keyboard focus and do not close or alter the mobile menu when toggling.

- [ ] **Step 5: Run component and homepage tests**

Run: `npm test -- --run src/components/Header.test.tsx src/App.test.tsx src/main.test.tsx`  
Expected: PASS.

- [ ] **Step 6: Commit the control**

```bash
git add homepage/src/main.tsx homepage/src/App.tsx homepage/src/components/Header.tsx homepage/src/components/Header.test.tsx homepage/src/App.test.tsx
git commit -m "feat: add accessible homepage theme control"
```

---

### Task 4: Complete Light and Dark Homepage Tokens

**Files:**
- Modify: `homepage/src/styles.css`
- Modify: `homepage/src/App.test.tsx`

**Interfaces:**
- Consumes: `<html data-theme="light|dark">`.
- Produces semantic CSS roles for page background, elevated surfaces, text, muted text, lines, header, hero overlays, cards, grids, footer, and signal fallbacks.

- [ ] **Step 1: Write a failing theme-contract test**

Read `src/styles.css` and assert it contains both theme selectors and semantic roles:

```ts
expect(styles).toContain('html[data-theme="light"]')
expect(styles).toContain('html[data-theme="dark"]')
for (const token of ['--page-bg', '--surface-1', '--text-strong', '--text-muted', '--border-subtle']) {
  expect(styles).toContain(token)
}
```

- [ ] **Step 2: Run the contract test and verify RED**

Run: `npm test -- --run src/App.test.tsx`  
Expected: FAIL because the light theme and semantic roles do not exist.

- [ ] **Step 3: Introduce semantic theme tokens**

Define dark and light values with OKLCH where practical. Convert the homepage’s hard-coded dark values in header, hero, value sections, statement, journey, integration, and footer to semantic variables. Keep `/support` declarations explicit and separate.

- [ ] **Step 4: Tune transitions and interaction states**

Theme transitions should be 180–240ms for color/background/border only. The theme button must be at least 44×44px. Do not animate layout, Canvas dimensions, or typography. Disable nonessential transitions under reduced motion.

- [ ] **Step 5: Run contract and regression tests**

Run: `npm test -- --run src/App.test.tsx src/components/Header.test.tsx src/sections`  
Expected: PASS.

- [ ] **Step 6: Commit the theme palette**

```bash
git add homepage/src/styles.css homepage/src/App.test.tsx
git commit -m "style: add complete homepage light and dark themes"
```

---

### Task 5: Layered, Theme-Aware Signal Field

**Files:**
- Create: `homepage/src/domain/signalField.ts`
- Create: `homepage/src/domain/signalField.test.ts`
- Modify: `homepage/src/components/HeroSignalCanvas.tsx`
- Create: `homepage/src/components/HeroSignalCanvas.test.tsx`
- Modify: `homepage/src/sections/HeroSection.test.tsx`
- Modify: `homepage/src/styles.css`

**Interfaces:**
- Produces: `SignalLayer = 'far' | 'mid' | 'near'`
- Produces: `SignalThemePalette`
- Produces: `buildSignalRows(options): SignalRow[]`
- Consumes: `xingqiao-theme-change` document event.
- Preserves current `HeroSignalCanvasProps`.

- [ ] **Step 1: Write failing signal-model tests**

Assert a deterministic seeded build includes all three layers, keeps active rows sparse, assigns increasing font size/alpha from far to near, and returns distinct dark/light palettes.

```ts
const rows = buildSignalRows({ width: 1440, height: 900, random: () => 0.42 })
expect(new Set(rows.map((row) => row.layer))).toEqual(new Set(['far', 'mid', 'near']))
expect(rows.filter((row) => row.active).length).toBeLessThan(rows.length / 3)
expect(signalPalette('light')).not.toEqual(signalPalette('dark'))
```

- [ ] **Step 2: Run model tests and verify RED**

Run: `npm test -- --run src/domain/signalField.test.ts`  
Expected: FAIL because the model module does not exist.

- [ ] **Step 3: Implement deterministic row descriptors**

Move row construction decisions out of the component. Each row descriptor includes layer, text seed, y, speed, alpha, font size, active flag, and color role. Keep tiling measurement in the Canvas component because it depends on the 2D context.

- [ ] **Step 4: Write failing Canvas behavior tests**

Mock Canvas 2D methods and IntersectionObserver. Assert the component:

- exposes `data-signal-layers="3"`;
- listens for `xingqiao-theme-change`;
- redraws with the new palette;
- does not request an animation frame under reduced motion;
- still renders a static draw under reduced motion.

- [ ] **Step 5: Run Canvas tests and verify RED**

Run: `npm test -- --run src/components/HeroSignalCanvas.test.tsx src/sections/HeroSection.test.tsx`  
Expected: FAIL because layered metadata and theme handling are missing.

- [ ] **Step 6: Implement the single-canvas layered renderer**

Render far rows first, then mid, then near. Use per-row fonts and colors, a slow time-based pulse only on sparse active rows, pointer proximity as a bounded multiplier, and the existing DPR cap and IntersectionObserver lifecycle. Reduced motion calls one static `draw()` and never starts RAF.

- [ ] **Step 7: Tune the visual safety mask**

Increase signal visibility in open areas while preserving a stronger scrim behind the lower-left headline and CTA. Give light mode its own cool-gray signal fallback and mask values instead of inverting the dark values.

- [ ] **Step 8: Run signal and hero tests**

Run: `npm test -- --run src/domain/signalField.test.ts src/components/HeroSignalCanvas.test.tsx src/sections/HeroSection.test.tsx src/sections/StatementSection.test.tsx`  
Expected: PASS.

- [ ] **Step 9: Commit the signal field**

```bash
git add homepage/src/domain/signalField.ts homepage/src/domain/signalField.test.ts homepage/src/components/HeroSignalCanvas.tsx homepage/src/components/HeroSignalCanvas.test.tsx homepage/src/sections/HeroSection.test.tsx homepage/src/styles.css
git commit -m "feat: layer the homepage signal field"
```

---

### Task 6: Full Verification and Visual QA

**Files:**
- Modify only files required to fix defects found during verification.
- Update relevant tests before every behavioral fix.

**Interfaces:**
- Consumes all previous tasks.
- Produces a locally verified branch; no push and no deployment.

- [ ] **Step 1: Run the full automated suite**

Run:

```bash
npm run typecheck
npm test -- --run
npm run build
```

Expected: all commands exit 0 with no test failures.

- [ ] **Step 2: Start the local Vite server**

Run: `npm run dev -- --host 127.0.0.1`  
Record the local URL. Do not invoke deployment tooling.

- [ ] **Step 3: Inspect desktop states**

Capture and read screenshots at 1440×1000 for:

- scheduled light theme;
- scheduled dark theme;
- manual override in each direction;
- scrolled header and at least one post-hero section.

Check signal depth, headline hierarchy, CTA contrast, nav readability, theme transition, and browser console errors.

- [ ] **Step 4: Inspect responsive and reduced-motion states**

Capture and read screenshots at 390×844 and 768×1024. Emulate reduced motion and verify the signal field is static but visible. Confirm the theme button remains a 44px touch target and the mobile menu still works.

- [ ] **Step 5: Run contrast and detector checks**

Use browser accessibility inspection for body text, muted text, CTA, links, and focus rings. Run the bundled impeccable detector over changed TSX/CSS files and fix only relevant findings.

- [ ] **Step 6: Re-run fresh full verification**

Run:

```bash
npm run typecheck
npm test -- --run
npm run build
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 7: Commit QA fixes if any**

```bash
git add homepage
git commit -m "fix: polish homepage theme and signal states"
```

- [ ] **Step 8: Confirm branch remains local**

Run:

```bash
git status --short
git branch --show-current
git log --oneline --decorate -8
```

Expected: clean worktree on `codex/homepage-theme-signal`; no push or production deployment has occurred.
