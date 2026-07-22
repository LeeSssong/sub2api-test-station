# Xingqiao Public Homepage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a Caddy-hosted Xingqiao homepage that faithfully reproduces the reference page's visible layout and motion while preserving every existing Sub2API and relay-ops route.

**Architecture:** A self-contained Vite/React application builds into a custom Caddy image and owns only `/` plus `/home-assets/*`. Pure TypeScript modules own public-config parsing and Sub2API session resolution; focused React sections own content and motion. The existing relay-ops `/ops` page reads the same public JSON to show or clear the MODELOC reminder.

**Tech Stack:** React 19.2.8, TypeScript 7.0.2, Vite 8.1.5, Motion 12.42.2, Lenis 1.3.25, Lucide React 1.25.0, Vitest 4.1.10, Testing Library 16.3.2, Playwright 1.61.1, Caddy 2.10.2, Go 1.24 relay-ops tests.

## Global Constraints

- Implement `docs/superpowers/specs/2026-07-22-xingqiao-homepage-design.md` without widening scope.
- The visible reference layout and every visible motion are hard acceptance targets; generic fade-ins are not substitutes for the scroll scenes.
- First-screen copy is exactly `星桥 / 链接世界顶尖模型 / 韩国首尔节点，国内直连。/ 兼容 OpenAI 与 Anthropic API。/ 无需翻墙，只需更改基础 URL。`.
- Public pricing copy is exactly `官方价格 100% / 星桥价格 官方价格的 0.1–0.3 倍 / 额度换算 1 元 = 1 美元额度` and remains independent of the current `1.0x` production multiplier.
- Support displays only QQ group `1080152144`.
- MODELOC starts empty, hides publicly, and produces a persistent `/ops` reminder until a valid report is configured.
- Guest CTA is `立即获取密钥 -> /register`; user CTA is `前往控制台 -> /dashboard`; administrator CTA is `前往控制台 -> /admin/dashboard`.
- `/pricing` and `/monitor` remain existing pages. Documentation and about remain homepage anchors.
- The reference bundle is evidence only. Do not copy it into the repository or ship it.
- No change may mutate registration, users, balances, multipliers, prices, routing, upstreams, keys, or operational evidence.
- Preserve all pre-existing dirty-worktree changes. Inspect shared-file diffs before and after every edit and never stage unrelated hunks.
- Use TDD for logic and routing contracts. Run a failing test before each implementation slice.
- Use only original Xingqiao assets from `output/imagegen/xingqiao-brand/`.

## File Structure

```text
homepage/
  package.json                         # isolated scripts and pinned dependencies
  package-lock.json                    # reproducible npm resolution
  tsconfig.json                        # strict browser/test TypeScript
  vite.config.ts                       # /home-assets output and Vitest setup
  index.html                           # metadata, favicon, root mount
  public/home-assets/site-config.json  # runtime config, MODELOC initially empty
  public/home-assets/xingqiao-logo.png # approved logo copy
  src/main.tsx                         # entrypoint and Lenis lifecycle
  src/App.tsx                          # semantic section composition
  src/styles.css                       # tokens, responsive layout, motion
  src/domain/siteConfig.ts             # config schema and validation
  src/domain/session.ts                # Sub2API session flow
  src/domain/clipboard.ts              # copy fallback
  src/hooks/useSiteConfig.ts           # async config state
  src/hooks/useSession.ts              # async CTA state
  src/components/Header.tsx            # navigation
  src/components/CopyControl.tsx       # accessible copy action
  src/components/Reveal.tsx            # intersection entrance
  src/sections/HeroSection.tsx         # hero, CTA, endpoint, scroll cue
  src/sections/ValueSections.tsx       # reports, price, channel, network, QQ
  src/sections/BoundarySection.tsx      # service promises
  src/sections/StatementSection.tsx     # word reveal
  src/sections/RequestJourney.tsx       # sticky request flow
  src/sections/IntegrationSection.tsx   # API integration
  src/sections/BrandReveal.tsx          # canvas footer reveal
  src/test/setup.ts                     # test polyfills
  src/**/*.test.ts(x)                   # unit/component tests
  e2e/homepage.spec.ts                  # browser flows and motion checks
infra/Dockerfile.caddy                 # node build plus Caddy runtime
infra/Caddyfile                        # exact homepage ownership
infra/compose.yaml                     # custom Caddy image
tests/infra/validate-baseline.sh        # image/route contracts
tests/relay_ops/validate_relay_ops_contract.sh
relay-ops-service/internal/http/templates/ops.html
relay-ops-service/internal/http/static/ops-admin.js
relay-ops-service/internal/http/server_test.go
```

---

### Task 1: Create The Tested Homepage Domain Core

**Files:**
- Create: `homepage/package.json`
- Create: `homepage/package-lock.json`
- Create: `homepage/tsconfig.json`
- Create: `homepage/vite.config.ts`
- Create: `homepage/index.html`
- Create: `homepage/src/test/setup.ts`
- Create: `homepage/src/domain/siteConfig.ts`
- Create: `homepage/src/domain/session.ts`
- Create: `homepage/src/domain/clipboard.ts`
- Test: `homepage/src/domain/siteConfig.test.ts`
- Test: `homepage/src/domain/session.test.ts`
- Test: `homepage/src/domain/clipboard.test.ts`

**Interfaces:**
- Produces: `loadSiteConfig(fetcher, origin): Promise<SiteConfig>`.
- Produces: `resolveSession(storage, fetcher): Promise<SessionState>`.
- Produces: `copyText(value, clipboard, document): Promise<"copied" | "selected">`.
- Produces: `DEFAULT_SITE_CONFIG`, including QQ group `1080152144` and no reports.

- [ ] **Step 1: Scaffold the isolated package and test runner**

Create scripts `dev`, `build`, `test`, `test:run`, `typecheck`, and `preview`. Configure Vite with `build.assetsDir = "home-assets"`, React plugin, jsdom, `src/test/setup.ts`, and strict TypeScript. Pin the versions from the plan header, run `npm install`, and retain the generated lockfile.

- [ ] **Step 2: Write failing public-config tests**

```ts
import { describe, expect, it, vi } from 'vitest'
import { DEFAULT_SITE_CONFIG, loadSiteConfig } from './siteConfig'

describe('loadSiteConfig', () => {
  it('falls back without reports and retains the Xingqiao QQ group', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('offline'))
    await expect(loadSiteConfig(fetcher, 'https://api.example.com')).resolves.toEqual({
      ...DEFAULT_SITE_CONFIG,
      apiOrigin: 'https://api.example.com',
    })
  })

  it('keeps only complete HTTPS third-party reports', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: 1,
      apiOrigin: '',
      support: { qqGroup: '1080152144' },
      thirdPartyReports: [
        { id: 'modeloc-1', provider: 'MODELOC', title: '模型真实性报告', url: 'https://modeloc.com/r/report', status: 'verified' },
        { id: 'unsafe', provider: 'MODELOC', title: '不安全', url: 'http://example.com', status: 'verified' },
      ],
    })))
    const config = await loadSiteConfig(fetcher, 'https://api.example.com')
    expect(config.thirdPartyReports.map((report) => report.id)).toEqual(['modeloc-1'])
  })
})
```

- [ ] **Step 3: Run the config tests and verify failure**

Run: `cd homepage && npm run test:run -- src/domain/siteConfig.test.ts`

Expected: FAIL because `siteConfig.ts` and its exports do not exist.

- [ ] **Step 4: Implement strict public-config parsing**

Define these types and defaults:

```ts
export type ReportStatus = 'verified' | 'reference' | 'archived'
export interface ThirdPartyReport { id: string; provider: string; title: string; description?: string; url: string; status: ReportStatus }
export interface SiteConfig { version: 1; apiOrigin: string; support: { qqGroup: string }; thirdPartyReports: ThirdPartyReport[] }
export const DEFAULT_SITE_CONFIG: SiteConfig = { version: 1, apiOrigin: '', support: { qqGroup: '1080152144' }, thirdPartyReports: [] }
```

`loadSiteConfig` fetches `/home-assets/site-config.json` with no-store caching, accepts only version `1`, trims strings, resolves empty origin to the browser origin, requires a ten-digit QQ group, accepts only HTTPS reports, and clones defaults on errors.

- [ ] **Step 5: Write failing session tests**

Cover no token, cached user plus successful `/auth/me`, expired access token plus one refresh and retry, failed refresh, and network failure retaining a cached role. Assert:

```ts
expect(await resolveSession(guestStorage, fetcher)).toEqual({ kind: 'guest', ctaLabel: '立即获取密钥', ctaHref: '/register' })
expect(await resolveSession(userStorage, fetcher)).toMatchObject({ kind: 'user', ctaLabel: '前往控制台', ctaHref: '/dashboard' })
expect(await resolveSession(adminStorage, fetcher)).toMatchObject({ kind: 'admin', ctaLabel: '前往控制台', ctaHref: '/admin/dashboard' })
```

- [ ] **Step 6: Run the session tests and verify failure**

Run: `cd homepage && npm run test:run -- src/domain/session.test.ts`

Expected: FAIL because `resolveSession` does not exist.

- [ ] **Step 7: Implement the bounded Sub2API session flow**

Use `auth_token`, `refresh_token`, `auth_user`, and `token_expires_at`. Send Bearer auth to `/api/v1/auth/me`. On the first 401 only, POST `{refresh_token}` to `/api/v1/auth/refresh`, unwrap standard or direct response bodies, persist returned tokens/user, then retry `/auth/me` once. Never recurse or log token values.

- [ ] **Step 8: Add clipboard tests and implementation**

Test successful `navigator.clipboard.writeText` and a rejected clipboard result that selects a temporary readonly input and returns `selected`. Always remove the temporary input.

- [ ] **Step 9: Run the domain suite**

Run `cd homepage && npm run test:run -- src/domain && npm run typecheck`.

Expected: all domain tests pass and TypeScript emits no errors.

- [ ] **Step 10: Commit isolated files when safe**

Stage only Task 1 `homepage/` paths and commit `feat: add Xingqiao homepage domain core`. Do not commit if the staged list includes pre-existing paths.

---

### Task 2: Build The Responsive Branded Page And Content

**Files:**
- Create: `homepage/public/home-assets/site-config.json`
- Create: `homepage/public/home-assets/xingqiao-logo.png`
- Create: `homepage/src/main.tsx`
- Create: `homepage/src/App.tsx`
- Create: `homepage/src/hooks/useSiteConfig.ts`
- Create: `homepage/src/hooks/useSession.ts`
- Create: `homepage/src/components/Header.tsx`
- Create: `homepage/src/components/CopyControl.tsx`
- Create: `homepage/src/components/Reveal.tsx`
- Create: `homepage/src/sections/HeroSection.tsx`
- Create: `homepage/src/sections/ValueSections.tsx`
- Create: `homepage/src/sections/BoundarySection.tsx`
- Create: `homepage/src/sections/IntegrationSection.tsx`
- Create: `homepage/src/styles.css`
- Test: `homepage/src/App.test.tsx`
- Test: `homepage/src/sections/HeroSection.test.tsx`
- Test: `homepage/src/sections/ValueSections.test.tsx`

**Interfaces:**
- Consumes: `SiteConfig`, `SessionState`, and `copyText` from Task 1.
- Produces: semantic anchors `#docs` and `#about`.
- Produces: stable section shells consumed by Task 3.

- [ ] **Step 1: Write failing content and conditional-render tests**

Render with injectable config/session props. Assert exact hero copy, both protocol paths, six navigation items, role-aware CTA, QQ group, fixed price strings, and the report rule:

```tsx
const { rerender } = render(<App config={{...DEFAULT_SITE_CONFIG, apiOrigin: 'https://api.example.com'}} session={guest} />)
expect(screen.queryByText('MODELOC')).not.toBeInTheDocument()
rerender(<App config={{...DEFAULT_SITE_CONFIG, apiOrigin: 'https://api.example.com', thirdPartyReports: [validReport]}} session={guest} />)
expect(screen.getByText('MODELOC')).toBeInTheDocument()
```

- [ ] **Step 2: Run component tests and verify failure**

Run: `cd homepage && npm run test:run -- src/App.test.tsx src/sections`

Expected: FAIL because the page components do not exist.

- [ ] **Step 3: Install the approved logo and public config**

Copy `output/imagegen/xingqiao-brand/xingqiao-logo-master.png` to `homepage/public/home-assets/xingqiao-logo.png`. Create JSON with empty `apiOrigin`, QQ `1080152144`, and `thirdPartyReports: []`.

- [ ] **Step 4: Implement semantic composition**

Compose Header, Hero, value grid, optional reports, transparent price, real channels, Seoul direct-connect, QQ support, boundaries, statement, request journey, integration, footer, and brand reveal in the approved order. Render no empty report wrapper.

- [ ] **Step 5: Implement visual system and responsive layout**

Use neutral light/dark surfaces, deep ink text, cyan-blue accent, and restrained warm gold from the logo. Match the reference maximum width `80rem`, 8px-or-less radii, hairline borders, compact navigation, full-width bands, and stable responsive type. Do not create nested cards or decorative gradient orbs.

- [ ] **Step 6: Implement hooks and interactions**

Load config once with unmount cancellation. Expose cached session immediately, then validated state. Give copy controls stable dimensions, Lucide copy/check icons, `aria-live` feedback, and fallback. Mobile navigation uses an icon, `aria-expanded`, and closes after selection.

- [ ] **Step 7: Run component, type, and build checks**

```bash
cd homepage
npm run test:run
npm run typecheck
npm run build
test -f dist/index.html
test -f dist/home-assets/site-config.json
```

Expected: tests pass and Vite emits the entry, hashed assets, and runtime JSON.

- [ ] **Step 8: Commit isolated UI files when safe**

Stage only Task 2 paths, exclude `homepage/dist/`, and commit `feat: build Xingqiao homepage content`.

---

### Task 3: Reproduce The Reference Motion And Canvas Reveal

**Files:**
- Create: `homepage/src/sections/StatementSection.tsx`
- Create: `homepage/src/sections/RequestJourney.tsx`
- Create: `homepage/src/sections/BrandReveal.tsx`
- Modify: `homepage/src/App.tsx`
- Modify: `homepage/src/main.tsx`
- Modify: `homepage/src/styles.css`
- Test: `homepage/src/sections/StatementSection.test.tsx`
- Test: `homepage/src/sections/RequestJourney.test.tsx`
- Test: `homepage/src/sections/BrandReveal.test.tsx`

**Interfaces:**
- Consumes: Task 2 section shells.
- Produces: `data-motion-state`, `data-journey-phase`, and `data-canvas-active` observability.

- [ ] **Step 1: Write failing reduced-motion and fallback tests**

Mock reduced motion and assert all words, send/route/observe phases, `187`, `2,148`, and static `星桥` are visible without an animation frame or opacity-zero content.

- [ ] **Step 2: Run motion tests and verify failure**

Run: `cd homepage && npm run test:run -- src/sections/StatementSection.test.tsx src/sections/RequestJourney.test.tsx src/sections/BrandReveal.test.tsx`

Expected: FAIL because the motion components do not exist.

- [ ] **Step 3: Implement entrance and smooth-scroll behavior**

Initialize one Lenis instance only when reduced motion is off, connect it to `requestAnimationFrame`, and destroy/cancel on unmount. `Reveal` uses IntersectionObserver, shows final content when unsupported, and cannot strand content at opacity zero.

- [ ] **Step 4: Implement hero and section choreography**

Reproduce the full-viewport cover, hero mask reveal, staggered headline/support/CTA/terminal, floating scroll cue and dismissal, mask-rise headings, staggered cards, flow dashes, terminal cursor, and hover translations. Expose durations and delays as CSS custom properties for fixed-time checks.

- [ ] **Step 5: Implement word-by-word statement**

Use Motion `useScroll` with `offset: ['start 0.9', 'start 0.35']`. Map stable keyed words to staggered ranges; animate opacity `0.15 -> 1` and Y `18 -> 0`. Show final text immediately for reduced motion.

- [ ] **Step 6: Implement request journey**

Desktop uses `340svh` with a sticky `100svh` stage. Reproduce send/route/observe text, line drawing, outbound and green return dots, gateway pulse, OpenAI/Claude/Gemini activation, and count-up to `187 ms` and `2,148`. Mobile renders three numbered rows without scroll scrubbing.

- [ ] **Step 7: Implement interactive brand reveal**

Use a DPR-capped canvas and offscreen centered `星桥`. Split into 14-30 CSS pixel cells. Pointer movement displaces nearby cells with radial falloff; multiply offsets by `0.88` per frame and stop below `0.04`. Repaint on resize/theme, pause outside viewport, set `data-canvas-active="true"`, and retain static fallback text.

- [ ] **Step 8: Run tests and build**

Run `cd homepage && npm run test:run && npm run typecheck && npm run build`.

Expected: all checks pass without unhandled timers.

- [ ] **Step 9: Commit the motion slice when safe**

Stage only Task 3 files and commit `feat: reproduce Xingqiao homepage motion`.

---

### Task 4: Add The MODELOC Reminder To Existing Ops

**Files:**
- Modify: `relay-ops-service/internal/http/templates/ops.html`
- Modify: `relay-ops-service/internal/http/static/ops-admin.js`
- Modify: `relay-ops-service/internal/http/server_test.go`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`

**Interfaces:**
- Consumes: `/home-assets/site-config.json`.
- Produces: `#modeloc-reminder`, hidden only when a valid HTTPS MODELOC report exists.

- [ ] **Step 1: Inspect overlapping diffs**

Run `git diff --` for all four files and preserve every existing change. Do not replace whole files or normalize unrelated formatting.

- [ ] **Step 2: Write failing Go/template tests**

Require the administrator page to contain `modeloc-reminder`, visible text `MODELOC 真实性报告尚未配置`, and `/home-assets/site-config.json`. Require the static script to reference `thirdPartyReports`, compare provider `MODELOC`, validate HTTPS, and fail visibly closed.

- [ ] **Step 3: Run focused test and verify failure**

Run: `cd relay-ops-service && go test ./internal/http -run TestOpsPageIsReadOnlyPlainLanguageAndAutoRefreshes -count=1`

Expected: FAIL because the reminder is absent.

- [ ] **Step 4: Implement the non-blocking reminder**

Add a status section to `ops.html`. In `ops-admin.js`, fetch the config with no-store caching; hide only when a report's trimmed provider uppercases to MODELOC and its URL parses as HTTPS. On 404, malformed JSON, or network failure, leave the reminder visible. Never insert config values as HTML.

- [ ] **Step 5: Update contracts and run tests**

```bash
cd relay-ops-service && go test ./internal/http -count=1
cd .. && tests/relay_ops/validate_relay_ops_contract.sh
```

Expected: Go HTTP tests and shell contract pass.

- [ ] **Step 6: Recheck overlapping diffs**

Verify only MODELOC reminder hunks were added. Do not stage shared files if unrelated pre-existing hunks cannot be isolated safely.

---

### Task 5: Package The Homepage In Caddy And Preserve Routes

**Files:**
- Create: `infra/Dockerfile.caddy`
- Modify: `infra/Caddyfile`
- Modify: `infra/compose.yaml`
- Modify: `tests/infra/validate-baseline.sh`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`

**Interfaces:**
- Consumes: the `homepage/` production build.
- Produces: `/srv/home/index.html` and `/srv/home/home-assets/*` in `xingqiao-caddy:homepage-20260722`.

- [ ] **Step 1: Inspect infrastructure diffs**

Run `git diff --` for all shared files. Preserve every matcher, proxy, timeout, volume, and security rule.

- [ ] **Step 2: Write failing image and route contracts**

Require:

```text
infra/Dockerfile.caddy
FROM node:22-alpine@sha256:b74031e546d7f4faf561d797ac1b76beccac856a042815ca77db4fd047581605 AS homepage-build
FROM caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d
COPY --from=homepage-build /src/dist /srv/home
dockerfile: infra/Dockerfile.caddy
image: xingqiao-caddy:homepage-20260722
path /
path /home-assets/site-config.json
path /home-assets/*
Cache-Control "no-store, max-age=0"
Cache-Control "public, max-age=31536000, immutable"
```

Also require the existing final Sub2API proxy and `flush_interval -1`.

- [ ] **Step 3: Run contracts and verify failure**

Run: `tests/infra/validate-baseline.sh`

Expected: FAIL because the custom Caddy build and handlers do not exist.

- [ ] **Step 4: Implement multi-stage image**

Copy package manifests first, run `npm ci`, copy homepage source, run tests/typecheck/build, then copy only `dist` into pinned Caddy at `/srv/home`. The runtime contains no Node modules or source tree.

- [ ] **Step 5: Add exact Caddy handlers**

Use an existence-aware matcher for `/` and `/srv/home/index.html`, rewrite to index, apply no-store, and serve. Add specific no-store config handling, then immutable `/home-assets/*`. Missing homepage falls through to Sub2API; missing assets return a static 404.

- [ ] **Step 6: Switch only production Caddy**

Add build context `..`, Dockerfile `infra/Dockerfile.caddy`, and image `xingqiao-caddy:homepage-20260722`. Preserve ports, volumes, dependencies, and logging. Do not change bootstrap Caddy.

- [ ] **Step 7: Run config, contract, and image checks**

```bash
docker compose --env-file infra/.env.example -f infra/compose.yaml config --quiet
tests/infra/validate-baseline.sh
tests/relay_ops/validate_relay_ops_contract.sh
docker build -f infra/Dockerfile.caddy -t xingqiao-caddy:homepage-test .
```

Expected: all checks pass and the image builds.

- [ ] **Step 8: Inspect final infrastructure diffs**

Confirm every existing proxy remains, only exact homepage paths changed owner, and no unrelated compose setting changed.

---

### Task 6: Add Browser Flows And Visual/Motion Acceptance

**Files:**
- Create: `homepage/playwright.config.ts`
- Create: `homepage/e2e/homepage.spec.ts`
- Create: `homepage/e2e/fixtures/session.ts`
- Modify: `homepage/package.json`
- Output only: `output/playwright/xingqiao-homepage-*`

**Interfaces:**
- Consumes: production Vite build and mocked `/api/v1/auth/*` contracts.
- Produces: guest/user/admin, mobile, reduced-motion, canvas, and overflow evidence.

- [ ] **Step 1: Write failing identity browser tests**

Start preview on `127.0.0.1:4173`. Mock `/api/v1/auth/me` per test and assert CTA label/href for guest, user, and admin. Assert navigation, API origin, both protocol strings, QQ, price copy, and absent MODELOC.

- [ ] **Step 2: Run browser test and verify failure**

Run: `cd homepage && npx playwright test e2e/homepage.spec.ts --project=chromium`

Expected: FAIL until preview and page selectors are complete.

- [ ] **Step 3: Add responsive and reduced-motion assertions**

At `1440x900`, `1920x1080`, and `390x844`, assert document width does not exceed viewport, primary controls have nonzero boxes, the next section intersects the first viewport, and text boxes do not overlap incoherently. With reduced motion, every section is visible and the journey uses non-scrubbed content.

- [ ] **Step 4: Add fixed-progress motion assertions**

Scroll by section bounding boxes and fixed ratios. Assert journey phases send -> route -> observe, final counts, nontransparent brand canvas pixels, pointer-driven canvas change, and decay back toward rest.

- [ ] **Step 5: Capture comparison evidence**

Capture reference and local screenshots at approved viewports and scroll frames under `output/playwright/`. Mask changed copy/logo content only, never containers or paths.

- [ ] **Step 6: Iterate until comparison passes**

Adjust homepage-only geometry and motion constants until section rhythm, entry order, sticky release, request path, mobile fallback, and canvas behavior match the reference.

- [ ] **Step 7: Run complete frontend verification**

```bash
cd homepage
npm run test:run
npm run typecheck
npm run build
npx playwright test --project=chromium
```

Expected: all checks pass.

---

### Task 7: Full Regression, Review, And Local Handoff

**Files:**
- Modify only when verification finds a homepage-related defect.
- Update: `docs/superpowers/plans/2026-07-22-xingqiao-homepage-implementation-plan.md` checkboxes as work completes.

**Interfaces:**
- Consumes: all prior tasks.
- Produces: verified local implementation ready for a separately authorized production deployment.

- [ ] **Step 1: Run focused verification**

```bash
cd homepage && npm run test:run && npm run typecheck && npm run build && npx playwright test --project=chromium
cd .. && tests/infra/validate-baseline.sh
tests/relay_ops/validate_relay_ops_contract.sh
```

Expected: all commands pass.

- [ ] **Step 2: Run relay-ops regression**

```bash
cd relay-ops-service
go test ./internal/http ./internal/app -count=1
go vet ./...
```

Expected: tests and vet pass. Widen to `go test ./...` if focused failures indicate shared behavior.

- [ ] **Step 3: Validate local route matrix**

Build the image, start a non-production test stack on unused ports, verify `/`, config, `/register`, `/pricing`, `/monitor`, `/ops`, `/health`, and representative streaming ownership, then stop only that test stack.

- [ ] **Step 4: Review security and scope**

Search for secret-shaped tokens and reference branding. Confirm no remote scripts, bundled reference assets, token logs, unsanitized config HTML, or mutations to registration, balances, routing, pricing configuration, multipliers, users, accounts, or keys.

- [ ] **Step 5: Perform final diff review**

List every modified path, identify pre-existing work in overlapping files, and ensure no user change was reverted. Stage only separable task changes. Leave overlapping untracked files unstaged rather than capturing unrelated work.

- [ ] **Step 6: Request code review and address findings**

Use `superpowers:requesting-code-review` on the complete implementation. Fix correctness, isolation, session, accessibility, responsive, motion, and test findings.

- [ ] **Step 7: Verify before completion**

Use `superpowers:verification-before-completion`, rerun every relevant command, and report fresh outputs. Do not deploy to production under this plan; production deployment remains a separately authorized action.
