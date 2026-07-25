# Xingqiao Detailed Guide Production Verification

Date: 2026-07-25

## Scope

- Caddy-only deployment of the detailed Xingqiao beginner guide.
- No restart or rebuild of Sub2API, PostgreSQL, Redis, relay-ops, or internal-test-service.

## Deployment

- Release directory: `/opt/sub2api/releases/20260725-detailed-guide-4`
- Image: `xingqiao-caddy:homepage-20260725-v8-detailed-guide`
- Image digest: `sha256:3cd833030e392060bb458fbfd9fece90f57bfe09d08c46824098fff058d28a9a`
- Production compose was backed up to `/opt/sub2api/production/compose.yaml.before-detailed-guide`.
- Production Caddyfile hash remained `b85a43dd0ab75af076ad59db88bc644a2b10d6c8024f0c985df5af5b4984df43`.

## HTTP checks

| Path | Result |
| --- | --- |
| `/docs` | 308 to `/docs/` |
| `/docs/` | 200 |
| `/docs/assets/01-create-key.png` | 200 |
| `/docs/assets/02-select-group.png` | 200 |
| `/docs/assets/03-key-actions.png` | 200 |
| `/docs/assets/04-ccswitch.png` | 200, Xingqiao-only 1365x58 crop |
| `/docs/assets/05-usage-and-billing.png` | 200 |
| `/docs/does-not-exist` | 404 |
| `/`, `/login`, `/pricing`, `/healthz`, `/readyz`, `/ops` | 200 |

The live HTML contains `星桥AI 小白使用教程` and does not contain `tkapi.fun`, `xmhbao.cn`, email-registration text, or referral text.

## Container preservation

The following IDs were unchanged across the deployment:

- Sub2API: `30ab69901ac7`
- PostgreSQL: `2db52788ad73`
- Redis: `c45202c0d9e6`
- relay-ops: `84ad983a6be4`
- internal-test-service: `fe0ecbb9d961`

Only Caddy was recreated, now with container ID `d1db0f185967`.

