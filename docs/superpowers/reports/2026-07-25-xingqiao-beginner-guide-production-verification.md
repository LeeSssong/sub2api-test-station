# Xingqiao Beginner Guide Production Verification

**Date:** 2026-07-25 (Asia/Shanghai)

**Result:** `PASS` for the production documentation route and same-origin public links

## Scope

This verification covers only the first requested item: replacing the public documentation destination with the local Xingqiao beginner guide. The homepage pricing copy, account monitoring, and Feishu upstream-monitoring redesign were intentionally left unchanged.

The guide is a static, same-origin page at `/docs/`. It contains Xingqiao-specific copy, five local screenshots, the supported first-use flow, and only the registration/configuration chapters that exist in this service.

## Production Deployment

- Caddy image: `xingqiao-caddy:homepage-20260725-v7-beginner-guide`
- Caddy container: `a8a154236b60` (running, recreated for the Caddy-only release)
- Production Caddyfile SHA-256: `b85a43dd0ab75af076ad59db88bc644a2b10d6c8024f0c985df5af5b4984df43`
- Production Compose SHA-256: `84453a6992982fc73892c63c615a72cae8361950845fefab38701149c87de906`

Only Caddy was rebuilt/recreated. The following running service IDs were unchanged:

```text
Sub2API             30ab69901ac7
PostgreSQL          2db52788ad73
Redis               c45202c0d9e6
relay-ops           84ad983a6be4
internal-test       fe0ecbb9d961
```

Server-side rollback material remains beside the active production files:

```text
compose.yaml.bak-beginner-guide-20260724T203409Z
Caddyfile.bak-beginner-guide-20260724T203409Z
```

No Sub2API, PostgreSQL, Redis, relay-ops, local Docker, or Colima restart/rebuild was performed for this change.

## Public Link Configuration

The public settings endpoint reports the following non-sensitive destinations:

```text
doc_url=https://api.xingqiaolab.top/docs/
balance_low_notify_recharge_url=https://api.xingqiaolab.top/custom/xingqiao-storefront
```

The official settings refresh was used to invalidate the running Sub2API settings cache. The unauthenticated `/login` HTML and the checked public endpoints no longer contain the retired relay domains `api3.xmhbao.cn` or `43-133-75-82.sslip.io`.

## Public HTTP Checks

All checks were read-only and used the production HTTPS endpoint:

| Check | Result |
|---|---|
| `/` | HTTP 200 |
| `/docs` | HTTP 308 to `/docs/` |
| `/docs/` | HTTP 200 |
| Five `/docs/assets/*.png` resources | HTTP 200 each |
| `/docs/does-not-exist` | HTTP 404 |
| `/login`, `/pricing`, `/healthz`, `/readyz`, `/ops` | HTTP 200 each |
| Retired-domain scan across the public pages above | No matches |

The static route terminates before the Sub2API fallback, so unknown documentation paths do not fall through to the application SPA.

## Browser Evidence

The production guide was captured at both required viewport sizes and visually inspected:

```text
output/playwright/production-docs-desktop.png  (1440x1000)
output/playwright/production-docs-mobile.png   (390x844)
```

Both screenshots show the Xingqiao title, same-origin control-panel link, readable section layout, local guide content, and the responsive mobile layout. No browser console error was observed during the capture.

## Local Verification

The release branch also passed the homepage test suite (15 files, 39 tests), TypeScript checking, production build, infrastructure baseline contract, public-link configurator tests, and `git diff --check` before integration.

## Residuals

- Historical git commits, reports, and audit logs may mention the retired relay domains as migration evidence; they are not served as public pages or settings.
- The documentation route is intentionally static. Future content changes require a new Caddy image and the same Caddy-only deployment boundary.
