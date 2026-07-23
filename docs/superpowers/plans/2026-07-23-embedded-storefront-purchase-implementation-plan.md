# Embedded Storefront Purchase Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the user-facing Sub2API `/purchase` page with a same-origin page that embeds the Starbridge card shop at `https://catfk.com/shop/DLK8SNUJ`.

**Architecture:** Caddy will match only `/purchase` and `/purchase/` before its catch-all proxy to the immutable Sub2API image. It will serve a self-contained static `purchase.html` from the existing homepage build output. The page has no application JavaScript, has a restrictive page CSP, and embeds the fixed shop URL in a responsive iframe with a visible top-level-window fallback.

**Tech Stack:** Caddy 2.10 configuration, Vite public assets, static HTML and CSS, Bash infrastructure contract test, Docker Compose.

## Global Constraints

- Do not modify the pinned `weishaw/sub2api` image, its database, or payment-provider configuration.
- Match only `/purchase` and `/purchase/`; all other paths continue to reach the current Caddy handlers or Sub2API proxy.
- Embed only `https://catfk.com/shop/DLK8SNUJ`; do not accept a shop URL from query string, local storage, or user input.
- Do not add card redemption, order callback, user-balance adjustment, or any automatic crediting behavior.
- Do not use an iframe load event, client redirect, or external-shop content as proof of payment.
- Do not sandbox the iframe because the external checkout requires its own form, popup, and navigation behavior. Do not attempt to bypass future `X-Frame-Options` or CSP restrictions; provide a visible new-window fallback.
- The static page CSP must prohibit scripts and arbitrary iframe origins while allowing inline CSS and the single `catfk.com` iframe.
- Preserve all unrelated uncommitted work.

---

## File Structure

| Path | Responsibility |
|---|---|
| `homepage/public/purchase.html` | Self-contained responsive purchase shell delivered by Vite to `/srv/home/purchase.html`. |
| `infra/Caddyfile` | Routes exact `/purchase` paths to the static shell before the generic Sub2API reverse proxy; applies no-store and CSP headers. |
| `tests/infra/embedded-storefront-purchase.sh` | Source-level contract test for the fixed shop URL, frame security boundary, responsive shell, and Caddy handler ordering. |
| `docs/superpowers/specs/2026-07-23-embedded-storefront-purchase-design.md` | Approved design and security boundary. |

### Task 1: Add the Static Purchase Shell and Its Contract Test

**Files:**
- Create: `homepage/public/purchase.html`
- Create: `tests/infra/embedded-storefront-purchase.sh`

**Interfaces:**
- Consumes: HTTP requests that Caddy rewrites to `/purchase.html`.
- Produces: A static HTML document with `iframe#storefront-frame`, fixed `src`, a top-level fallback link, and no JavaScript.

- [ ] **Step 1: Write the failing contract test**

Create `tests/infra/embedded-storefront-purchase.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
require() { rg -Fq -- "$1" "$2" || fail "missing $1 in $2"; }
reject() { ! rg -Fq -- "$1" "$2" || fail "forbidden $1 in $2"; }

PAGE=homepage/public/purchase.html
require '<title>星桥 | 充值与订阅</title>' "$PAGE"
require 'id="storefront-frame"' "$PAGE"
require 'src="https://catfk.com/shop/DLK8SNUJ"' "$PAGE"
require 'title="星桥官方兑换卡店铺"' "$PAGE"
require 'target="_blank"' "$PAGE"
require 'rel="noopener noreferrer"' "$PAGE"
require 'min-height: clamp(42rem, 82svh, 72rem);' "$PAGE"
reject '<script' "$PAGE"

printf 'PASS: embedded storefront shell contract\n'
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
bash tests/infra/embedded-storefront-purchase.sh
```

Expected: `FAIL: missing ... homepage/public/purchase.html`.

- [ ] **Step 3: Create the minimal static shell**

Create `homepage/public/purchase.html` with this content:

```html
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <meta name="theme-color" content="#080b12" />
    <title>星桥 | 充值与订阅</title>
    <style>
      :root { color-scheme: dark; font-family: "PingFang SC", "Microsoft YaHei", system-ui, sans-serif; background: #080b12; color: #eef4ff; }
      * { box-sizing: border-box; }
      body { min-width: 320px; min-height: 100svh; margin: 0; background: #080b12; }
      .purchase-shell { width: min(100% - 32px, 1440px); margin: 0 auto; padding: 18px 0 28px; }
      .purchase-bar { display: flex; align-items: center; justify-content: space-between; gap: 16px; min-height: 58px; padding: 0 16px; border: 1px solid rgba(188, 213, 255, .18); background: #101724; }
      .purchase-title { margin: 0; font-size: 16px; font-weight: 700; }
      .purchase-note { margin: 4px 0 0; color: #9ba9bb; font-size: 13px; }
      .open-store { flex: 0 0 auto; display: inline-flex; align-items: center; min-height: 34px; padding: 0 12px; border: 1px solid rgba(86, 211, 194, .6); color: #66e0ce; font-size: 13px; text-decoration: none; }
      .open-store:hover, .open-store:focus-visible { background: rgba(86, 211, 194, .13); }
      .storefront-frame { width: 100%; min-height: clamp(42rem, 82svh, 72rem); margin-top: 14px; border: 1px solid rgba(188, 213, 255, .18); background: #fff; }
      @media (max-width: 640px) {
        .purchase-shell { width: 100%; padding: 0; }
        .purchase-bar { align-items: flex-start; min-height: 76px; padding: 12px; }
        .purchase-title { font-size: 15px; }
        .purchase-note { line-height: 1.45; }
        .open-store { min-height: 36px; padding: 0 10px; white-space: nowrap; }
        .storefront-frame { min-height: calc(100svh - 76px); margin-top: 0; border-left: 0; border-right: 0; }
      }
    </style>
  </head>
  <body>
    <main class="purchase-shell">
      <header class="purchase-bar">
        <div>
          <h1 class="purchase-title">充值与订阅</h1>
          <p class="purchase-note">购买兑换卡后，请返回星桥兑换页面完成到账。</p>
        </div>
        <a class="open-store" href="https://catfk.com/shop/DLK8SNUJ" target="_blank" rel="noopener noreferrer">新窗口打开</a>
      </header>
      <iframe id="storefront-frame" class="storefront-frame" src="https://catfk.com/shop/DLK8SNUJ" title="星桥官方兑换卡店铺" allow="payment" referrerpolicy="strict-origin-when-cross-origin"></iframe>
    </main>
  </body>
</html>
```

- [ ] **Step 4: Run the test to verify it passes**

Run:

```bash
bash tests/infra/embedded-storefront-purchase.sh
```

Expected: `PASS: embedded storefront shell contract`.

- [ ] **Step 5: Commit the independently tested shell**

```bash
git add homepage/public/purchase.html tests/infra/embedded-storefront-purchase.sh
git commit -m "feat: add embedded storefront purchase shell"
```

### Task 2: Route the Native Purchase Path to the Shell

**Files:**
- Modify: `infra/Caddyfile` immediately before `reverse_proxy sub2api:8080`
- Modify: `tests/infra/embedded-storefront-purchase.sh`

**Interfaces:**
- Consumes: GET requests to `/purchase` and `/purchase/`.
- Produces: The static `/srv/home/purchase.html` response with `Cache-Control: no-store` and a restrictive CSP; all unmatched requests proceed to existing routes.

- [ ] **Step 1: Extend the failing contract test**

Append before the final `printf` in `tests/infra/embedded-storefront-purchase.sh`:

```bash
CADDY=infra/Caddyfile
require '@embedded_storefront_purchase path /purchase /purchase/' "$CADDY"
require 'handle @embedded_storefront_purchase {' "$CADDY"
require 'rewrite * /purchase.html' "$CADDY"
require "frame-src https://catfk.com" "$CADDY"
require "default-src 'none'" "$CADDY"
require 'reverse_proxy sub2api:8080' "$CADDY"

handler_line=$(rg -n -F '@embedded_storefront_purchase path /purchase /purchase/' "$CADDY" | cut -d: -f1)
proxy_line=$(rg -n -F 'reverse_proxy sub2api:8080' "$CADDY" | cut -d: -f1)
[[ "$handler_line" -lt "$proxy_line" ]] || fail 'purchase handler must run before Sub2API proxy'
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
bash tests/infra/embedded-storefront-purchase.sh
```

Expected: `FAIL: missing @embedded_storefront_purchase ... infra/Caddyfile`.

- [ ] **Step 3: Add the precise Caddy handler**

Insert this block directly before the existing `reverse_proxy sub2api:8080` block in `infra/Caddyfile`:

```caddy
	@embedded_storefront_purchase path /purchase /purchase/
	handle @embedded_storefront_purchase {
		root * /srv/home
		rewrite * /purchase.html
		header {
			Cache-Control "no-store, max-age=0"
			Content-Security-Policy "default-src 'none'; style-src 'unsafe-inline'; frame-src https://catfk.com; base-uri 'none'; form-action 'none'; frame-ancestors 'self'"
		}
		file_server
	}
```

- [ ] **Step 4: Run configuration and contract validation**

Run:

```bash
bash tests/infra/embedded-storefront-purchase.sh
docker compose --env-file infra/.env.example -f infra/compose.yaml config --quiet
docker run --rm -v "$PWD/infra/Caddyfile:/etc/caddy/Caddyfile:ro" -e SITE_ADDRESS=api.example.com caddy:2.10.2-alpine caddy adapt --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null
```

Expected: shell contract prints `PASS`, Compose exits `0`, and Caddy adapts successfully.

- [ ] **Step 5: Commit the routing contract**

```bash
git add infra/Caddyfile tests/infra/embedded-storefront-purchase.sh
git commit -m "feat: route purchase page to embedded storefront"
```

### Task 3: Build and Visually Verify the Purchase Experience

**Files:**
- Verify: `homepage/public/purchase.html`
- Verify: `infra/Caddyfile`
- Verify: `infra/Dockerfile.caddy`

**Interfaces:**
- Consumes: Vite public-asset copy behavior and Caddy static file handler.
- Produces: A built `/srv/home/purchase.html` and proof that desktop and mobile views retain a visible embedded storefront and fallback link.

- [ ] **Step 1: Build and run the existing frontend checks**

Run:

```bash
npm --prefix homepage run test:run
npm --prefix homepage run typecheck
npm --prefix homepage run build
test -f homepage/dist/purchase.html
```

Expected: all commands exit `0`; `homepage/dist/purchase.html` exists.

- [ ] **Step 2: Serve the built static directory locally**

Run:

```bash
npx --yes serve homepage/dist --listen 4174
```

Expected: a local server reports `http://localhost:4174` and stays running for browser verification.

- [ ] **Step 3: Verify desktop behavior through a real browser**

Open `http://localhost:4174/purchase.html` at a 1440x1000 viewport and verify:

```text
- The heading reads “充值与订阅”.
- The “新窗口打开” link targets https://catfk.com/shop/DLK8SNUJ.
- The iframe is visible, has a nonzero rendered height, and displays the shop rather than a blank frame.
- No source page script, payment completion message, account token, or card code appears in the shell.
```

- [ ] **Step 4: Verify mobile behavior through a real browser**

Open the same URL at a 390x844 viewport and verify:

```text
- Header text wraps without overlapping the fallback link.
- The iframe begins directly below the header and extends to the viewport bottom.
- Horizontal scrolling is absent.
```

- [ ] **Step 5: Build the production Caddy image**

Run:

```bash
docker build -f infra/Dockerfile.caddy -t xingqiao-caddy:embedded-storefront-check .
```

Expected: Docker build exits `0`, including the homepage tests, typecheck, build, and static asset copy.

- [ ] **Step 6: Preserve the verified source state**

Do not create a source or documentation change for transient browser output. The two task commits contain every required source change for this feature.

## Plan Self-Review

- Spec coverage: Task 1 implements the fixed responsive in-page shop and top-level fallback; Task 2 implements exact route interception and CSP; Task 3 proves build and viewport behavior. The plan intentionally excludes redemption, crediting, callbacks, and refund automation as required.
- Placeholder scan: no `TODO`, `TBD`, or unspecified command/implementation steps remain.
- Consistency: `purchase.html`, `/purchase`, `@embedded_storefront_purchase`, and `#storefront-frame` use the same names across tasks. The Caddy root matches the Docker image destination `/srv/home`.
