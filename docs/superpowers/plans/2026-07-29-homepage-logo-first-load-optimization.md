# Homepage Logo First-Load Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the 945 KiB published Xingqiao PNG with a visually equivalent 256-by-256 WebP no larger than 50 KiB and prevent the oversized asset from returning.

**Architecture:** Keep a high-resolution source master outside `homepage/public`, publish one versioned WebP under `/home-assets/`, and update every React and static-document consumer to the new URL. A real-file Vitest contract verifies dimensions, size, and legacy-public-file removal; normal homepage tests and the production build verify integration.

**Tech Stack:** React 19, TypeScript, Vite 8, Vitest 4, Pillow 12.2 with WebP support, Caddy static assets

## Global Constraints

- The published file is `homepage/public/home-assets/xingqiao-logo-256-v1.webp`.
- The published image is exactly 256 by 256 pixels and no larger than 51,200 bytes.
- The source master is retained at `homepage/assets/source/xingqiao-logo-master.png` and is not copied into `homepage/dist`.
- `homepage/public/home-assets/xingqiao-logo.png` is removed.
- DNS, EdgeOne, production deployment, Caddy routing, API behavior, SSE behavior, and QR-code assets are unchanged.
- The unrelated working-tree change in `监控日报-2026-07-28.md` is never staged or modified by this plan.

---

### Task 1: Add the published-logo asset contract

**Files:**
- Create: `homepage/src/domain/logoAsset.test.ts`
- Test: `homepage/src/domain/logoAsset.test.ts`

**Interfaces:**
- Consumes: the repository paths `homepage/public/home-assets/xingqiao-logo-256-v1.webp`, `homepage/public/home-assets/xingqiao-logo.png`, and `homepage/assets/source/xingqiao-logo-master.png`
- Produces: an executable Vitest contract for WebP dimensions, published byte size, legacy removal, and source-master retention

- [ ] **Step 1: Write the failing real-file test**

Create a Vitest test that reads the actual files with `node:fs`. Parse the WebP RIFF container directly and accept the `VP8X`, `VP8 `, or `VP8L` dimension encodings. Assert the hand-derived literals `256`, `256`, and `51_200`; assert that the source master exists and the old public PNG does not.

```ts
import { existsSync, readFileSync, statSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const projectRoot = resolve(import.meta.dirname, '../..')
const publishedLogo = resolve(projectRoot, 'public/home-assets/xingqiao-logo-256-v1.webp')
const legacyLogo = resolve(projectRoot, 'public/home-assets/xingqiao-logo.png')
const sourceMaster = resolve(projectRoot, 'assets/source/xingqiao-logo-master.png')

function readWebpDimensions(buffer: Buffer): { width: number; height: number } {
  if (buffer.toString('ascii', 0, 4) !== 'RIFF' || buffer.toString('ascii', 8, 12) !== 'WEBP') {
    throw new Error('not a WebP RIFF file')
  }

  const chunk = buffer.toString('ascii', 12, 16)
  if (chunk === 'VP8X') {
    return {
      width: 1 + buffer.readUIntLE(24, 3),
      height: 1 + buffer.readUIntLE(27, 3),
    }
  }
  if (chunk === 'VP8 ') {
    return {
      width: buffer.readUInt16LE(26) & 0x3fff,
      height: buffer.readUInt16LE(28) & 0x3fff,
    }
  }
  if (chunk === 'VP8L') {
    const bits = buffer.readUInt32LE(21)
    return {
      width: 1 + (bits & 0x3fff),
      height: 1 + ((bits >> 14) & 0x3fff),
    }
  }
  throw new Error(`unsupported WebP chunk ${chunk}`)
}

describe('published Xingqiao logo', () => {
  it('ships only a 256px WebP no larger than 50 KiB', () => {
    expect(existsSync(sourceMaster)).toBe(true)
    expect(existsSync(legacyLogo)).toBe(false)
    expect(existsSync(publishedLogo)).toBe(true)

    const dimensions = readWebpDimensions(readFileSync(publishedLogo))
    expect(dimensions).toEqual({ width: 256, height: 256 })
    expect(statSync(publishedLogo).size).toBeLessThanOrEqual(51_200)
  })
})
```

- [ ] **Step 2: Run the focused test and observe the expected failure**

Run:

```bash
cd homepage
npm run test:run -- src/domain/logoAsset.test.ts
```

Expected: FAIL because the source master and new WebP do not exist while the legacy PNG still exists.

### Task 2: Produce the optimized asset and migrate all consumers

**Files:**
- Create: `homepage/assets/source/xingqiao-logo-master.png`
- Create: `homepage/public/home-assets/xingqiao-logo-256-v1.webp`
- Delete: `homepage/public/home-assets/xingqiao-logo.png`
- Modify: `homepage/src/components/Header.tsx`
- Modify: `homepage/src/sections/ModelFlow.tsx`
- Modify: `homepage/src/sections/RequestJourney.tsx`
- Modify: `homepage/index.html`
- Modify: `homepage/public/docs/index.html`
- Test: `homepage/src/domain/logoAsset.test.ts`

**Interfaces:**
- Consumes: the current 1254-by-1254 PNG and the asset contract from Task 1
- Produces: `/home-assets/xingqiao-logo-256-v1.webp` for every homepage, flow, journey, guide, and favicon consumer

- [ ] **Step 1: Move the master out of the published tree**

Create `homepage/assets/source/`, move the existing PNG to
`homepage/assets/source/xingqiao-logo-master.png`, and do not leave a copy in
`homepage/public/home-assets/`.

- [ ] **Step 2: Generate the 256-by-256 WebP**

Run the bundled Pillow runtime with high-quality Lanczos resampling and WebP
quality 88:

```bash
/Users/gongtengxinwen/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/bin/python3 - <<'PY'
from pathlib import Path
from PIL import Image

source = Path('homepage/assets/source/xingqiao-logo-master.png')
target = Path('homepage/public/home-assets/xingqiao-logo-256-v1.webp')

with Image.open(source) as image:
    image = image.convert('RGB')
    image = image.resize((256, 256), Image.Resampling.LANCZOS)
    image.save(target, 'WEBP', quality=88, method=6)
PY
```

If the result exceeds 51,200 bytes, repeat only the save at quality 84. Do not
reduce dimensions below 256 or use a quality below 84.

- [ ] **Step 3: Update React image URLs**

Replace each rendered `/home-assets/xingqiao-logo.png` URL in `Header.tsx`,
`ModelFlow.tsx`, and `RequestJourney.tsx` with
`/home-assets/xingqiao-logo-256-v1.webp`. Preserve current alt text, dimensions,
classes, and layout.

- [ ] **Step 4: Update both favicons**

In `homepage/index.html` and `homepage/public/docs/index.html`, use:

```html
<link rel="icon" type="image/webp" href="/home-assets/xingqiao-logo-256-v1.webp">
```

- [ ] **Step 5: Run the focused test and observe it pass**

Run:

```bash
cd homepage
npm run test:run -- src/domain/logoAsset.test.ts
```

Expected: PASS with the real public WebP at 256 by 256, no larger than 50 KiB,
the source master retained, and the old public PNG absent.

- [ ] **Step 6: Commit the tested asset migration**

Stage only the Task 1 and Task 2 files, then commit:

```bash
git add homepage/assets/source/xingqiao-logo-master.png \
  homepage/public/home-assets/xingqiao-logo-256-v1.webp \
  homepage/public/home-assets/xingqiao-logo.png \
  homepage/src/domain/logoAsset.test.ts \
  homepage/src/components/Header.tsx \
  homepage/src/sections/ModelFlow.tsx \
  homepage/src/sections/RequestJourney.tsx \
  homepage/index.html \
  homepage/public/docs/index.html
git commit -m "perf: shrink homepage logo payload"
```

### Task 3: Verify the complete homepage artifact

**Files:**
- Verify: `homepage/dist/index.html`
- Verify: `homepage/dist/docs/index.html`
- Verify: `homepage/dist/home-assets/xingqiao-logo-256-v1.webp`

**Interfaces:**
- Consumes: the optimized source tree from Task 2
- Produces: passing test, typecheck, build, artifact, and visual evidence without production deployment

- [ ] **Step 1: Run all homepage verification commands**

Run:

```bash
cd homepage
npm run test:run
npm run typecheck
npm run build
```

Expected: all commands exit zero without test warnings or TypeScript errors.

- [ ] **Step 2: Verify the built artifact contract**

Run from the repository root:

```bash
test -f homepage/dist/home-assets/xingqiao-logo-256-v1.webp
test ! -e homepage/dist/home-assets/xingqiao-logo.png
test "$(stat -f %z homepage/dist/home-assets/xingqiao-logo-256-v1.webp)" -le 51200
! rg -n 'xingqiao-logo\.png' homepage/dist
rg -n 'xingqiao-logo-256-v1\.webp' homepage/dist/index.html homepage/dist/docs/index.html homepage/dist/home-assets
```

Expected: all checks exit zero, the new URL appears in the built homepage,
guide, and JavaScript, and the legacy URL is absent.

- [ ] **Step 3: Perform a local visual check**

Serve `homepage/dist` locally, inspect the homepage at desktop and mobile
widths, and inspect `/docs/`. Confirm the logo has no visible blur, crop,
background shift, or broken favicon request. This is visual verification only;
do not edit layout or styling in this phase.

- [ ] **Step 4: Check repository hygiene**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors; only the unrelated pre-existing
`监控日报-2026-07-28.md` change may remain unstaged.
