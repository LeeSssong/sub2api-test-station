# Homepage Full-Width Code Background Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every animated code row cover the complete homepage hero width at desktop, tablet, and mobile sizes.

**Architecture:** Add a small pure tiling helper that converts a measured code segment into enough repeated text for the current canvas width. `HeroSignalCanvas` will measure each segment during its existing build/resize phase, store the generated tile and wrap width per row, and draw the prepared full-width row on every frame.

**Tech Stack:** React 19, TypeScript 7, Canvas 2D, Vitest 4, Vite 8, Docker, Caddy

## Global Constraints

- Keep the existing row vocabulary, opacity range, vertical spacing, direction, pointer-controlled speed, and reduced-motion fallback.
- Do not change hero copy, navigation, calls to action, grid, ambient color, shade, or layout.
- Recalculate row coverage whenever the hero canvas is resized.
- Do not rebuild or restart Sub2API, PostgreSQL, Redis, or relay-ops during deployment.

---

### Task 1: Width-Aware Signal Tiling

**Files:**
- Create: `homepage/src/domain/signalTiling.ts`
- Create: `homepage/src/domain/signalTiling.test.ts`

**Interfaces:**
- Consumes: a rendered segment string, its measured pixel width, and the viewport width.
- Produces: `buildSignalTile(segment: string, segmentWidth: number, viewportWidth: number): SignalTile`.

- [ ] **Step 1: Write the failing unit test**

```ts
import { describe, expect, it } from 'vitest'
import { buildSignalTile } from './signalTiling'

describe('buildSignalTile', () => {
  it('repeats a measured segment far enough to cover a wide viewport with wrap overflow', () => {
    const tile = buildSignalTile('XQ SIGNAL   ', 320, 1920)

    expect(tile.repetitions).toBe(8)
    expect(tile.text).toBe('XQ SIGNAL   '.repeat(8))
    expect(tile.coveredWidth).toBeGreaterThanOrEqual(1920 + 640)
  })

  it('keeps enough overflow on a narrow viewport for seamless movement', () => {
    const tile = buildSignalTile('XQ SIGNAL   ', 320, 390)

    expect(tile.repetitions).toBe(4)
    expect(tile.coveredWidth).toBeGreaterThanOrEqual(390 + 640)
  })
})
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd homepage && npm run test:run -- src/domain/signalTiling.test.ts
```

Expected: FAIL because `./signalTiling` does not exist.

- [ ] **Step 3: Implement the pure tiling helper**

```ts
export interface SignalTile {
  text: string
  segmentWidth: number
  repetitions: number
  coveredWidth: number
}

export function buildSignalTile(
  segment: string,
  segmentWidth: number,
  viewportWidth: number,
): SignalTile {
  const safeSegmentWidth = Math.max(1, segmentWidth)
  const safeViewportWidth = Math.max(1, viewportWidth)
  const repetitions = Math.max(2, Math.ceil(safeViewportWidth / safeSegmentWidth) + 2)

  return {
    text: segment.repeat(repetitions),
    segmentWidth: safeSegmentWidth,
    repetitions,
    coveredWidth: repetitions * safeSegmentWidth,
  }
}
```

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
cd homepage && npm run test:run -- src/domain/signalTiling.test.ts
```

Expected: 2 tests pass.

### Task 2: Integrate Adaptive Tiling Into The Canvas

**Files:**
- Modify: `homepage/src/components/HeroSignalCanvas.tsx:1-83`
- Test: `homepage/src/domain/signalTiling.test.ts`

**Interfaces:**
- Consumes: `buildSignalTile` from Task 1.
- Produces: prepared `SignalRow.text` and `SignalRow.segmentWidth` values that always cover the canvas.

- [ ] **Step 1: Add the helper import and measured row state**

Add:

```ts
import { buildSignalTile } from '../domain/signalTiling'

const SIGNAL_FONT = '500 14px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace'
```

Change `SignalRow` to:

```ts
interface SignalRow {
  text: string
  segmentWidth: number
  x: number
  y: number
  speed: number
  alpha: number
}
```

- [ ] **Step 2: Build each row from the measured segment width**

Inside `build`, set `context.font = SIGNAL_FONT` before creating rows, then replace row creation with:

```ts
rows = Array.from({ length: count }, (_, index) => {
  const rowText = `${rowSeeds[index % rowSeeds.length]}  ${String(index * 73 + 19).padStart(4, '0')}`
  const segment = `${rowText}   `
  const tile = buildSignalTile(segment, context.measureText(segment).width, width)

  return {
    text: tile.text,
    segmentWidth: tile.segmentWidth,
    x: -(Math.random() * tile.segmentWidth),
    y: index * 20,
    speed: -(.62 * Math.random() + .48),
    alpha: .22 * Math.random() + .05,
  }
})
```

- [ ] **Step 3: Draw the prepared tile and wrap by one segment**

Replace the fixed two-copy draw logic with:

```ts
context.font = SIGNAL_FONT
context.textBaseline = 'middle'
velocity += (targetVelocity - velocity) * .055
for (const row of rows) {
  row.x += row.speed * velocity
  if (row.x < -row.segmentWidth) row.x += row.segmentWidth
  context.fillStyle = `rgba(157, 174, 198, ${row.alpha})`
  context.fillText(row.text, row.x, row.y)
}
```

- [ ] **Step 4: Run focused and full homepage verification**

Run:

```bash
cd homepage
npm run test:run -- src/domain/signalTiling.test.ts src/sections/HeroSection.test.tsx
npm run test:run
npm run typecheck
npm run build
```

Expected: all tests pass, TypeScript exits 0, and Vite produces `homepage/dist`.

### Task 3: Browser Visual Verification

**Files:**
- Verify: `homepage/src/components/HeroSignalCanvas.tsx`
- Output: `output/playwright/xingqiao-homepage-full-width-signal-desktop.png`
- Output: `output/playwright/xingqiao-homepage-full-width-signal-mobile.png`

**Interfaces:**
- Consumes: the local Vite preview build from Task 2.
- Produces: desktop and mobile evidence that code glyphs span the entire hero.

- [ ] **Step 1: Start a local preview on an unused port**

Run:

```bash
cd homepage && npm run dev -- --host 127.0.0.1 --port 4174
```

Expected: Vite serves the homepage at `http://127.0.0.1:4174/`.

- [ ] **Step 2: Inspect wide desktop and mobile viewports**

At `1815x822`, verify visible code glyphs in the left, center, and right thirds of the hero. At `390x844`, verify the signal background remains full-width with no horizontal overflow and that the title and button remain legible.

- [ ] **Step 3: Capture screenshots and check the console**

Save the two screenshots listed above. Expected: no browser console errors, no element overlap, and no blank code region on either viewport.

### Task 4: Production Homepage Deployment

**Files:**
- Modify: `infra/compose.yaml` Caddy image tag only
- Modify remotely: `/opt/sub2api/production/compose.yaml` Caddy image tag only

**Interfaces:**
- Consumes: verified homepage source and `infra/Dockerfile.caddy`.
- Produces: `xingqiao-caddy:homepage-20260724-v6-full-width-signal` on `sub2api-prod`.

- [ ] **Step 1: Upload a clean homepage-only build context**

Create an isolated temporary directory containing only `homepage/` and `infra/Dockerfile.caddy`, excluding `node_modules`, `dist`, `.DS_Store`, and every `._*` AppleDouble file. Upload it to an isolated release directory under `/opt/sub2api/releases/`.

- [ ] **Step 2: Build the new image without touching running containers**

Run remotely:

```bash
sudo docker build \
  -f infra/Dockerfile.caddy \
  -t xingqiao-caddy:homepage-20260724-v6-full-width-signal \
  .
```

Expected: the Docker build runs homepage tests, type checking, and production build successfully.

- [ ] **Step 3: Update and recreate only Caddy**

Back up `/opt/sub2api/production/compose.yaml`, replace only the Caddy image tag with `xingqiao-caddy:homepage-20260724-v6-full-width-signal`, validate the Compose file, then run:

```bash
sudo docker compose up -d --no-deps --force-recreate caddy
```

Expected: only the Caddy container ID changes. Sub2API, PostgreSQL, Redis, and relay-ops container IDs and start times remain unchanged.

- [ ] **Step 4: Verify production behavior**

Check:

```bash
curl -fsS https://api.xingqiaolab.top/health
sudo docker compose ps caddy
```

Then open `https://api.xingqiaolab.top/` at `1815x822`, capture the production hero, and confirm code glyphs are present across all three horizontal thirds with no console errors.

- [ ] **Step 5: Preserve rollback information**

Keep the previous Caddy image tag `xingqiao-caddy:homepage-20260724-v5-auto-journey` and the timestamped production Compose backup. If production verification fails, restore that tag and recreate only Caddy.
