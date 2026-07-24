# Xingqiao Beginner Guide And Link Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish a licensed TK API-style beginner guide at `https://api.xingqiaolab.top/docs/`, replace every guide screenshot and brand value with Xingqiao production evidence, and remove erroneous production links to other relay sites or obsolete hosts.

**Architecture:** The guide is a self-contained static HTML page plus local PNG assets under `homepage/public/docs/`; Caddy owns `/docs` routing before the Sub2API fallback. A separate idempotent production script updates only `doc_url` and `balance_low_notify_recharge_url` in PostgreSQL. Homepage navigation, Caddy routing, static content, database settings, browser behavior, and production deployment each have independent verification and rollback.

**Tech Stack:** Static HTML/CSS, React 19, Vitest/jsdom, Bash, PostgreSQL, Caddy 2.10.2, Docker Compose, Playwright CLI.

## Global Constraints

- The approved design is `docs/superpowers/specs/2026-07-25-xingqiao-beginner-guide-and-link-audit-design.md`.
- The reference page may be recreated because the user confirmed permission to copy its page design and content.
- Production origin is exactly `https://api.xingqiaolab.top`.
- Guide origin is exactly `https://api.xingqiaolab.top/docs/`.
- QQ group is exactly `1080152144`.
- Do not start, stop, rebuild, or modify local Colima, local Sub2API, local databases, or local containers.
- Do not change the Sub2API image or recreate production `sub2api`, `postgres`, `redis`, `relay-ops`, or `internal-test-service`.
- Do not publish registration, email verification, invitation rebate, affiliate, or unverified download instructions.
- Do not expose API keys, tokens, email addresses, balances, order data, or user identifiers in screenshots, logs, Git, or final responses.
- CatFK is the only approved user-flow third-party service; the upstream Sub2API GitHub link remains admin-only.
- Existing unrelated worktree changes must not be staged, reverted, or included in feature commits.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `homepage/public/docs/index.html` | Licensed static guide, exact reference layout, Xingqiao-only content and links. |
| `homepage/public/docs/assets/01-create-key.png` | Sanitized production create-key dialog. |
| `homepage/public/docs/assets/02-select-group.png` | Sanitized production group selector. |
| `homepage/public/docs/assets/03-key-actions.png` | Sanitized production key action area. |
| `homepage/public/docs/assets/04-ccswitch.png` | Sanitized CC Switch Xingqiao active state. |
| `homepage/public/docs/assets/05-usage-and-billing.png` | Sanitized production usage and billing table. |
| `homepage/src/docs/BeginnerGuide.test.ts` | Static content, asset, forbidden-copy, and link-allowlist contract. |
| `homepage/src/components/Header.tsx` | Same-origin `/docs/` navigation target. |
| `homepage/src/components/Header.test.tsx` | Desktop/mobile navigation regression test. |
| `infra/Caddyfile` | Exact `/docs` redirect, index, asset, CSP, and 404 boundary. |
| `tests/infra/validate-baseline.sh` | Static Caddy route ordering and handler contract. |
| `ops/configure-sub2api-public-links.sh` | Idempotent transaction updating the two approved production settings. |
| `tests/operations/configure_sub2api_public_links_test.sh` | Script context, SQL scope, retry, idempotence, and secret-safety contract. |
| `tests/infra/audit-public-links.sh` | HTTP/final-origin audit for public and guide links. |
| `docs/superpowers/reports/2026-07-25-xingqiao-beginner-guide-production-verification.md` | Non-secret production pre-state, deployment, link audit, screenshots, and rollback evidence. |

---

### Task 1: Capture And Sanitize Xingqiao-Owned Screenshots

**Files:**
- Create in temporary evidence first: `output/playwright/xingqiao-beginner-guide/raw/*.png`
- Create after review: `homepage/public/docs/assets/01-create-key.png`
- Create after review: `homepage/public/docs/assets/02-select-group.png`
- Create after review: `homepage/public/docs/assets/03-key-actions.png`
- Create after review: `homepage/public/docs/assets/04-ccswitch.png`
- Create after review: `homepage/public/docs/assets/05-usage-and-billing.png`

**Interfaces:**
- Consumes: the user's already authenticated production browser session and existing CC Switch installation/state.
- Produces: five PNG files containing only Xingqiao product UI and no secret or personal data.

- [ ] **Step 1: Record the production screenshot rules before opening authenticated pages**

Use these exact acceptance rules:

```text
01-create-key.png: dialog title and safe empty/default controls only; no created key.
02-select-group.png: group names and explanatory text only; no account/provider identifiers.
03-key-actions.png: crop to action controls; any key value must remain masked.
04-ccswitch.png: Xingqiao provider name, https://api.xingqiaolab.top/, Codex, and “使用中”; no API key.
05-usage-and-billing.png: column labels and representative rows only after masking user, IP, request ID, exact balance, and any prompt text.
```

- [ ] **Step 2: Capture production key UI without mutating data**

Use the approved Playwright CLI browser flow:

```bash
PWCLI="$HOME/.codex/skills/playwright/scripts/playwright_cli.sh"
"$PWCLI" -s=xq-prod tab-new https://api.xingqiaolab.top/keys
"$PWCLI" -s=xq-prod snapshot
```

Open the existing “创建密钥” dialog but do not submit. Capture the empty dialog and group selector. Close the dialog with Escape. Capture only masked key action controls from the existing list; do not click copy, reveal, edit, disable, or delete.

Expected: no POST/PUT/PATCH/DELETE request is emitted during capture.

- [ ] **Step 3: Capture usage UI read-only**

Open `https://api.xingqiaolab.top/usage`, snapshot before any action, and capture the table after masking sensitive cells in the browser page only. Do not change filters if doing so would trigger billable or mutating work.

Expected: screenshot shows the same column language users will see in the guide and contains no prompt, key, token, email, IP, request ID, or exact account balance.

- [ ] **Step 4: Capture CC Switch active state**

Open the existing CC Switch UI. Select the already configured Xingqiao entry only if it is already active; do not import a new key or overwrite configuration. Capture the provider card showing:

```text
星桥AI
https://api.xingqiaolab.top/
Codex
使用中
```

If the configured entry is absent, stop this task and ask the user for the current Xingqiao CC Switch screenshot. Do not manufacture the state.

- [ ] **Step 5: Inspect every image at original resolution**

Use `view_image` on all five raw files. Reject any file with a secret, personal value, loading state, wrong product, crop error, overlap, or unreadable text.

- [ ] **Step 6: Promote accepted screenshots**

Create `homepage/public/docs/assets/` and copy only accepted files to the exact five names. Run:

```bash
file homepage/public/docs/assets/*.png
shasum -a 256 homepage/public/docs/assets/*.png
```

Expected: exactly five valid PNG files and five distinct hashes.

Do not commit yet; Task 2 commits screenshots with the guide and its contract.

---

### Task 2: Build The Static Guide Test-First

**Files:**
- Create: `homepage/src/docs/BeginnerGuide.test.ts`
- Create: `homepage/public/docs/index.html`
- Consume: `homepage/public/docs/assets/*.png`

**Interfaces:**
- Consumes: five accepted Task 1 assets.
- Produces: a JavaScript-independent, responsive guide at built path `homepage/dist/docs/index.html`.

- [ ] **Step 1: Write the failing static guide contract**

Create `homepage/src/docs/BeginnerGuide.test.ts` with these assertions:

```ts
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { JSDOM } from 'jsdom'
import { describe, expect, it } from 'vitest'

const publicRoot = resolve(process.cwd(), 'public')
const guidePath = resolve(publicRoot, 'docs/index.html')
const assetNames = [
  '01-create-key.png',
  '02-select-group.png',
  '03-key-actions.png',
  '04-ccswitch.png',
  '05-usage-and-billing.png',
]

describe('Xingqiao beginner guide', () => {
  it('uses approved Xingqiao content and omits disabled features', () => {
    const html = readFileSync(guidePath, 'utf8')
    for (const required of [
      '星桥AI 小白使用教程',
      'https://api.xingqiaolab.top/',
      '1080152144',
      '充值并确认余额',
      '创建 API 密钥',
      '导入 CC Switch',
      '查看 Token 和扣费',
      '密钥安全建议',
    ]) expect(html).toContain(required)

    for (const forbidden of [
      'tkapi.fun',
      'xmhbao.cn',
      'sslip.io',
      '邮箱验证码',
      '邀请好友',
      '20% 返利',
      'pan.quark.cn',
    ]) expect(html).not.toContain(forbidden)
  })

  it('ships every approved local screenshot', () => {
    for (const name of assetNames) {
      expect(existsSync(resolve(publicRoot, 'docs/assets', name))).toBe(true)
    }
  })

  it('keeps normal user links on the Xingqiao origin', () => {
    const dom = new JSDOM(readFileSync(guidePath, 'utf8'))
    const allowed = new Set([
      '/',
      '/keys',
      '/usage',
      '/custom/xingqiao-storefront',
      '/support',
    ])
    const hrefs = [...dom.window.document.querySelectorAll<HTMLAnchorElement>('a[href]')]
      .map((anchor) => anchor.getAttribute('href'))
    expect(hrefs.length).toBeGreaterThan(5)
    for (const href of hrefs) {
      expect(href).toBeTruthy()
      expect(href === '/' || href?.startsWith('#') || allowed.has(href ?? '')).toBe(true)
    }
  })
})
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
npm --prefix homepage run test:run -- src/docs/BeginnerGuide.test.ts
```

Expected: FAIL because `homepage/public/docs/index.html` does not exist.

- [ ] **Step 3: Create the licensed static guide**

Create `homepage/public/docs/index.html` using the captured reference values exactly:

```css
:root {
  color-scheme: light;
  --ink: #14231f;
  --muted: #60716b;
  --line: #dce6e2;
  --brand: #0d6f5d;
  --brand-dark: #102b25;
  --brand-soft: #e8f6f1;
  --link: #075ea8;
  --warning: #8a3f08;
  --warning-bg: #fff7e8;
  --code: #10211d;
}
```

Use a 58px sticky `.topbar`, `min(1240px, calc(100% - 36px))` content width, `232px minmax(0, 1fr)` desktop grid, 44px gap, and the reference `@media (max-width: 820px)` single-column behavior. Keep all heading IDs and table-of-contents anchors hardcoded so the page needs no JavaScript and can use `script-src 'none'`.

Use this exact section order and links:

```text
H1 星桥AI 小白使用教程
开始前先了解
一、充值并确认余额 -> /custom/xingqiao-storefront
二、创建 API 密钥 -> /keys
三、导入 CC Switch
  一键导入没有反应怎么办
  手动配置位置
四、启动 Codex 并完成首次测试
五、查看 Token 和扣费 -> /usage
六、常见报错
  401、Unauthorized 或 invalid_api_key
  余额不足或无法继续调用
  429、请求过多或并发已满
  模型不可用或 model not supported
  超时、长时间没有首字
七、密钥安全建议
八、完成检查与联系客服 -> /support
```

Use Xingqiao-specific copy from the approved spec. Add the five images after their corresponding paragraphs with absolute local sources `/docs/assets/<name>.png`. Top bar return link is `/`. Footer is `星桥AI · api.xingqiaolab.top · QQ群 1080152144`.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run the Step 2 command.

Expected: 3 tests pass and no forbidden string is reported.

- [ ] **Step 5: Build and verify copied assets**

Run:

```bash
npm --prefix homepage run build
test -f homepage/dist/docs/index.html
test "$(find homepage/dist/docs/assets -type f -name '*.png' | wc -l | tr -d ' ')" = 5
```

Expected: all commands exit 0.

- [ ] **Step 6: Commit the guide unit**

```bash
git add homepage/public/docs homepage/src/docs/BeginnerGuide.test.ts
git commit -m "feat: add xingqiao beginner guide"
```

Expected: commit contains only the guide, five screenshots, and guide contract.

---

### Task 3: Point Homepage Documentation Navigation To The Guide

**Files:**
- Modify: `homepage/src/components/Header.test.tsx`
- Modify: `homepage/src/components/Header.tsx`
- Modify: `homepage/src/sections/HeroSection.test.tsx`
- Modify: `homepage/src/sections/HeroSection.tsx`

**Interfaces:**
- Consumes: same-origin `/docs/` from Task 2.
- Produces: every visible homepage “文档” command navigates to `/docs/`.

- [ ] **Step 1: Write failing navigation expectations**

Add to the header test:

```ts
expect(screen.getByRole('link', { name: '文档' })).toHaveAttribute('href', '/docs/')
```

Open the mobile menu and assert the second “文档” link also has `/docs/`. Update the signed-in Hero test to require:

```ts
expect(screen.getByRole('link', { name: '查看文档' })).toHaveAttribute('href', '/docs/')
```

- [ ] **Step 2: Run focused tests and verify RED**

```bash
npm --prefix homepage run test:run -- src/components/Header.test.tsx src/sections/HeroSection.test.tsx
```

Expected: FAIL because current href values are `#docs`.

- [ ] **Step 3: Implement the same-origin links**

In `Header.tsx`, change only:

```ts
{ label: '文档', href: '/docs/' },
```

In `HeroSection.tsx`, change the authenticated secondary CTA to:

```tsx
<a className="secondary-cta" href="/docs/">查看文档</a>
```

Do not remove the existing homepage integration section; only stop presenting it as the documentation target.

- [ ] **Step 4: Run focused and full homepage tests**

```bash
npm --prefix homepage run test:run -- src/components/Header.test.tsx src/sections/HeroSection.test.tsx
npm --prefix homepage run test:run
```

Expected: focused and full suites pass.

- [ ] **Step 5: Commit the navigation unit**

```bash
git add homepage/src/components/Header.tsx homepage/src/components/Header.test.tsx homepage/src/sections/HeroSection.tsx homepage/src/sections/HeroSection.test.tsx
git commit -m "fix: route homepage docs links locally"
```

---

### Task 4: Serve `/docs/` Before The Sub2API Fallback

**Files:**
- Modify: `tests/infra/validate-baseline.sh`
- Modify: `infra/Caddyfile`

**Interfaces:**
- Consumes: `/srv/home/docs/index.html` and `/srv/home/docs/assets/*` from Task 2.
- Produces: `/docs` -> `/docs/` 308, `/docs/` -> static guide 200, assets -> 200, unknown docs paths -> 404.

- [ ] **Step 1: Add failing Caddy contract assertions**

Add exact assertions to `tests/infra/validate-baseline.sh`:

```bash
require_fixed '@docs_root path /docs' infra/Caddyfile
require_fixed 'redir @docs_root /docs/ 308' infra/Caddyfile
require_fixed '@docs_index path /docs/' infra/Caddyfile
require_fixed 'rewrite * /docs/index.html' infra/Caddyfile
require_fixed '@docs_assets path /docs/*' infra/Caddyfile
require_fixed "script-src 'none'" infra/Caddyfile

docs_line=$(rg -n -F '@docs_root path /docs' infra/Caddyfile | head -n1 | cut -d: -f1)
proxy_line=$(rg -n -F 'reverse_proxy sub2api:8080' infra/Caddyfile | head -n1 | cut -d: -f1)
[[ -n "$docs_line" && -n "$proxy_line" && "$docs_line" -lt "$proxy_line" ]] || \
  fail 'docs handlers must appear before the Sub2API fallback proxy'
```

- [ ] **Step 2: Run the infrastructure contract and verify RED**

```bash
bash tests/infra/validate-baseline.sh
```

Expected: FAIL with missing `@docs_root`.

- [ ] **Step 3: Add exact Caddy handlers**

Insert after homepage assets and before support/Sub2API handlers:

```caddyfile
	@docs_root path /docs
	redir @docs_root /docs/ 308

	@docs_index path /docs/
	handle @docs_index {
		root * /srv/home
		rewrite * /docs/index.html
		header Cache-Control "no-store, max-age=0"
		header Content-Security-Policy "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'"
		header X-Frame-Options SAMEORIGIN
		file_server
	}

	@docs_assets path /docs/*
	handle @docs_assets {
		root * /srv/home
		header Cache-Control "public, max-age=86400"
		header Content-Security-Policy "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'"
		header X-Frame-Options SAMEORIGIN
		file_server
	}
```

Because `handle @docs_assets` terminates the route, missing `/docs/*` files return Caddy 404 and never reach Sub2API.

- [ ] **Step 4: Run Caddy and infrastructure verification**

```bash
bash tests/infra/validate-baseline.sh
docker run --rm \
  -v "$PWD/infra/Caddyfile:/etc/caddy/Caddyfile:ro" \
  -e SITE_ADDRESS=api.example.com \
  caddy:2.10.2-alpine \
  caddy adapt --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null
```

Expected: both commands exit 0. If local Docker is unavailable, record the Docker-only check as pending for the remote production-image build; do not start Colima.

- [ ] **Step 5: Commit the routing unit**

```bash
git add infra/Caddyfile tests/infra/validate-baseline.sh
git commit -m "feat: serve beginner guide on docs route"
```

---

### Task 5: Update Only The Two Erroneous Production Settings

**Files:**
- Create: `tests/operations/configure_sub2api_public_links_test.sh`
- Create: `ops/configure-sub2api-public-links.sh`

**Interfaces:**
- Consumes: the established explicit Compose project/file/environment context.
- Produces: exactly two settings:
  - `doc_url=https://api.xingqiaolab.top/docs/`
  - `balance_low_notify_recharge_url=https://api.xingqiaolab.top/custom/xingqiao-storefront`

- [ ] **Step 1: Write the failing script contract**

Model the fixture Docker shim after `tests/operations/configure_sub2api_support_test.sh`. The test must capture SQL and assert:

```bash
rg -Fq 'BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;' "$fixture/sql.1"
rg -Fq "SET LOCAL application_name = 'configure-sub2api-public-links';" "$fixture/sql.1"
rg -Fq "pg_advisory_xact_lock(hashtext('sub2api:settings:public-links'))" "$fixture/sql.1"
rg -Fq "'doc_url', 'https://api.xingqiaolab.top/docs/'" "$fixture/sql.1"
rg -Fq "'balance_low_notify_recharge_url', 'https://api.xingqiaolab.top/custom/xingqiao-storefront'" "$fixture/sql.1"
rg -Fq 'ON CONFLICT (key) DO UPDATE' "$fixture/sql.1"
! rg -n "custom_menu_items|payment_enabled|registration_enabled|affiliate_enabled" "$fixture/sql.1"
```

Run the script twice and require byte-identical SQL. Test the same missing-variable, symlink, changed project identity, five-attempt `40001`, and five-attempt `40P01` cases used by the support configurator. Assert stdout/stderr contain no secret or complete settings response.

- [ ] **Step 2: Run the new test and verify RED**

```bash
bash tests/operations/configure_sub2api_public_links_test.sh
```

Expected: FAIL because `ops/configure-sub2api-public-links.sh` does not exist.

- [ ] **Step 3: Implement the scoped idempotent script**

Use the same context validation as `configure-sub2api-support.sh`, excluding `SUB2API_DATA_DIR`. The transaction body is exactly:

```sql
\set VERBOSITY verbose

BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;

SET LOCAL application_name = 'configure-sub2api-public-links';

SELECT pg_advisory_xact_lock(hashtext('sub2api:settings:public-links'));

INSERT INTO settings (key, value, updated_at)
VALUES
  ('doc_url', 'https://api.xingqiaolab.top/docs/', NOW()),
  ('balance_low_notify_recharge_url', 'https://api.xingqiaolab.top/custom/xingqiao-storefront', NOW())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = EXCLUDED.updated_at;

COMMIT;
```

Retry only SQLSTATE `40001` and `40P01`, at most five attempts. Print only:

```text
Configured Sub2API public links
```

- [ ] **Step 4: Run focused and shell verification**

```bash
bash tests/operations/configure_sub2api_public_links_test.sh
bash -n ops/configure-sub2api-public-links.sh tests/operations/configure_sub2api_public_links_test.sh
shellcheck ops/configure-sub2api-public-links.sh tests/operations/configure_sub2api_public_links_test.sh
```

Expected: all commands exit 0. If `shellcheck` is unavailable, record that fact and require it in the remote release check.

- [ ] **Step 5: Commit the configuration unit**

```bash
git add ops/configure-sub2api-public-links.sh tests/operations/configure_sub2api_public_links_test.sh
git commit -m "fix: configure same-origin public links"
```

---

### Task 6: Add A Repeatable Public-Link Audit

**Files:**
- Create: `tests/infra/audit-public-links.sh`
- Modify: `tests/infra/validate-baseline.sh`

**Interfaces:**
- Consumes: `BASE_URL`, defaulting to `https://api.xingqiaolab.top`.
- Produces: a non-secret table of source path, status, redirect count, final URL, and classification.

- [ ] **Step 1: Write the audit script**

The script must check these paths with `curl -L --max-redirs 5`:

```bash
paths=(
  /
  /docs
  /docs/
  /docs/assets/01-create-key.png
  /docs/assets/02-select-group.png
  /docs/assets/03-key-actions.png
  /docs/assets/04-ccswitch.png
  /docs/assets/05-usage-and-billing.png
  /docs/does-not-exist
  /support
  /login
  /keys
  /usage
  /custom/xingqiao-storefront
)
```

Rules:

```text
/docs -> one redirect -> same-origin /docs/ -> 200
/docs/ and five assets -> same origin -> 200
/docs/does-not-exist -> same origin -> 404
normal application paths -> same origin and not 5xx
/custom/xingqiao-storefront -> same-origin application page; its embedded CatFK request is allowed but top-level final URL must remain same-origin
```

Fetch `/api/v1/settings/public`, unwrap `.data // .`, and require:

```jq
.doc_url == "https://api.xingqiaolab.top/docs/" and
.balance_low_notify_recharge_url == "https://api.xingqiaolab.top/custom/xingqiao-storefront" and
(.custom_menu_items | any(.id == "xingqiao-storefront"))
```

Fail if response text contains `api3.xmhbao.cn`, `43-133-75-82.sslip.io`, or another relay hostname.

- [ ] **Step 2: Add repository presence checks**

Add to `validate-baseline.sh`:

```bash
require_file tests/infra/audit-public-links.sh
test -x tests/infra/audit-public-links.sh || fail 'public link audit must be executable'
```

- [ ] **Step 3: Run static validation**

```bash
bash -n tests/infra/audit-public-links.sh
bash tests/infra/validate-baseline.sh
```

Expected: both exit 0. Do not run the production audit until Task 8 updates production settings.

- [ ] **Step 4: Commit the audit unit**

```bash
git add tests/infra/audit-public-links.sh tests/infra/validate-baseline.sh
git commit -m "test: audit public link destinations"
```

---

### Task 7: Complete Local Source Verification Without Starting Local Sub2API

**Files:**
- Verify all Task 1-6 files.
- Create ignored artifacts: `output/playwright/xingqiao-beginner-guide/*.png`

**Interfaces:**
- Consumes: complete guide and routing/configuration units.
- Produces: verified static build and desktop/mobile guide evidence without using local Colima or local Sub2API.

- [ ] **Step 1: Run all source tests**

```bash
npm --prefix homepage run test:run
npm --prefix homepage run typecheck
npm --prefix homepage run build
bash tests/operations/configure_sub2api_public_links_test.sh
bash tests/infra/validate-baseline.sh
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 2: Start only an isolated static preview**

```bash
npx --yes serve homepage/dist --listen 4174
```

This serves only built static files. Do not start Colima, Docker Compose, Sub2API, PostgreSQL, Redis, or Caddy locally.

- [ ] **Step 3: Compare reference and implementation at desktop**

Use Playwright at `1440x1000` for both the licensed reference and `http://127.0.0.1:4174/docs/`. Capture top, key creation, usage, and footer positions. Verify the topbar, 232px sticky table of contents, 44px layout gap, title scale, warning block, code styling, image width, and section spacing match the reference.

- [ ] **Step 4: Compare reference and implementation at mobile**

Use `390x844`. Verify the table of contents is hidden, header brand and return link fit, article has no horizontal overflow, images stay within width, long error names wrap, and no text overlaps.

- [ ] **Step 5: Check interaction and console state**

Click every table-of-contents item and every normal link without leaving the local preview for same-origin targets. Confirm no blank page, missing image, console error, or unexpected network request.

- [ ] **Step 6: Stop only the static preview**

Terminate the `serve` process started in Step 2. Confirm `colima status` remains `not running` and do not change it.

---

### Task 8: Deploy Only Production Caddy And Public-Link Settings

**Files:**
- Modify: `infra/compose.yaml` Caddy image tag only.
- Modify remotely: `/opt/sub2api/production/compose.yaml` Caddy image tag only.
- Execute remotely: `ops/configure-sub2api-public-links.sh` with protected production context.
- Create: `docs/superpowers/reports/2026-07-25-xingqiao-beginner-guide-production-verification.md`

**Interfaces:**
- Consumes: verified Tasks 1-7 and SSH host alias `sub2api-prod`.
- Produces: production guide, same-origin settings, link audit evidence, and a precise Caddy-only rollback.

- [ ] **Step 1: Capture non-secret production pre-state**

On `sub2api-prod`, record:

```bash
cd /opt/sub2api/production
sudo docker compose ps --format json
sudo docker inspect sub2api-caddy-1 sub2api-sub2api-1 sub2api-postgres-1 sub2api-redis-1 sub2api-relay-ops-1 \
  --format '{{.Name}} {{.Id}} {{.State.Status}} {{.State.Health.Status}} {{.Config.Image}}'
```

Query only the two setting values through PostgreSQL and do not print environment files. Save the current Caddy tag and a timestamped `compose.yaml` backup path for rollback.

- [ ] **Step 2: Build a clean production Caddy image without touching running containers**

Create an isolated release directory under `/opt/sub2api/releases/20260725-beginner-guide/` containing only `homepage/`, `infra/Dockerfile.caddy`, and the updated `infra/Caddyfile`. Build:

```bash
sudo docker build \
  -f infra/Dockerfile.caddy \
  -t xingqiao-caddy:homepage-20260725-v7-beginner-guide \
  .
```

Expected: image build runs homepage tests, typecheck, and build successfully; running container IDs are unchanged.

- [ ] **Step 3: Update the repository and production Compose tag**

Change only the local and remote Caddy image line to:

```yaml
image: xingqiao-caddy:homepage-20260725-v7-beginner-guide
```

Back up `/opt/sub2api/production/compose.yaml`, run `sudo docker compose config --quiet`, and compare the resolved service set before proceeding.

- [ ] **Step 4: Recreate only production Caddy**

```bash
cd /opt/sub2api/production
sudo docker compose up -d --no-deps --force-recreate caddy
```

Expected: only Caddy container ID changes. Sub2API, PostgreSQL, Redis, relay-ops, and internal-test-service IDs and start times are unchanged.

- [ ] **Step 5: Verify static docs before changing `doc_url`**

Run:

```bash
curl -fsSI https://api.xingqiaolab.top/docs
curl -fsS https://api.xingqiaolab.top/docs/ | rg -F '星桥AI 小白使用教程'
curl -fsSI https://api.xingqiaolab.top/docs/assets/01-create-key.png
test "$(curl -sS -o /dev/null -w '%{http_code}' https://api.xingqiaolab.top/docs/does-not-exist)" = 404
```

Expected: `/docs` is 308 to `/docs/`; guide and asset are 200; unknown docs path is 404.

- [ ] **Step 6: Apply the scoped production settings transaction**

Run from the uploaded release tools with exact production context:

```bash
SUB2API_COMPOSE_PROJECT=sub2api-deploy \
SUB2API_PROJECT_DIRECTORY=/opt/sub2api/production \
SUB2API_SECRET_ENV_FILE=/opt/sub2api/production/.env \
SUB2API_RELEASE_ENV_FILE=/opt/sub2api/production/config/releases/sub2api.env \
SUB2API_COMPOSE_FILE=/opt/sub2api/production/compose.yaml \
SUB2API_IMAGE_OVERLAY=/opt/sub2api/production/compose.sub2api-release.yaml \
bash /opt/sub2api/releases/20260725-beginner-guide/ops/configure-sub2api-public-links.sh
```

Expected: stdout is exactly `Configured Sub2API public links`.

- [ ] **Step 7: Run production link audit and browser acceptance**

```bash
BASE_URL=https://api.xingqiaolab.top \
bash /opt/sub2api/releases/20260725-beginner-guide/tests/infra/audit-public-links.sh
```

Then use the production browser at `1440x1000` and `390x844`. From the actual Sub2API topbar, click “文档” and prove the final URL is `https://api.xingqiaolab.top/docs/`. Check all guide anchors, screenshot loads, support, keys, usage, and storefront custom page. Record intentional CatFK iframe traffic separately from top-level navigation.

- [ ] **Step 8: Verify unchanged services and data boundaries**

Repeat the Step 1 container identity capture. Require unchanged IDs for Sub2API, PostgreSQL, Redis, relay-ops, and internal-test-service. Confirm registration remains disabled, affiliate remains disabled, payment settings are unchanged, and no user/key/balance/order/channel counts changed because of this deployment.

- [ ] **Step 9: Roll back on any required failure**

Restore the timestamped production `compose.yaml` backup and run:

```bash
sudo docker compose up -d --no-deps --force-recreate caddy
```

For database links, restore captured pre-state values only if they are approved safe values. Because the old values are confirmed erroneous, otherwise set `doc_url` to the empty string and set the recharge notification URL to the same-origin storefront route. Never restore `api3.xmhbao.cn` or the old `sslip.io` URL.

- [ ] **Step 10: Write production verification report**

Record timestamps, commits, new/old Caddy tags, unchanged container IDs, setting before/after classifications, HTTP statuses, final domains, screenshot paths, browser viewport results, and any remaining intentional third-party link. Do not include secrets, full settings responses, personal data, or exact production key values.

- [ ] **Step 11: Commit deployment metadata and report**

```bash
git add infra/compose.yaml docs/superpowers/reports/2026-07-25-xingqiao-beginner-guide-production-verification.md
git commit -m "docs: verify xingqiao beginner guide production"
```

Expected: only the Caddy image tag and non-secret verification report are committed.

---

## Plan Self-Review

- **Spec coverage:** Task 1 makes all five Xingqiao-owned screenshots a blocking gate. Task 2 reproduces the licensed layout and removes disabled chapters. Task 3 fixes homepage links. Task 4 prevents `/docs/` from falling into Sub2API. Task 5 changes only the two erroneous settings. Task 6 detects future empty/external/obsolete destinations. Task 7 verifies desktop/mobile without starting local Sub2API. Task 8 deploys only Caddy plus two production settings and proves unchanged service/data boundaries.
- **Placeholder scan:** No unresolved marker, unspecified image, dummy URL, or deferred error-handling step remains. A missing CC Switch production state is an explicit stop condition, not a placeholder.
- **Consistency:** `/docs/`, five asset names, Caddy image tag `xingqiao-caddy:homepage-20260725-v7-beginner-guide`, the two PostgreSQL keys, Compose project `sub2api-deploy`, and production root `/opt/sub2api/production` are identical across all tasks.
