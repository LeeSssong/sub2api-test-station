# Homepage Logo First-Load Optimization Design

## Objective

Reduce the Xingqiao public homepage's largest first-load image from the
current 945,132-byte, 1254-by-1254 PNG to a visually equivalent 256-by-256
WebP no larger than 50 KiB. This phase changes only repository assets and
their public references. It does not purchase a service, change DNS, alter
production routing, or deploy to production.

## Current State

- `homepage/public/home-assets/xingqiao-logo.png` is a 1254-by-1254 PNG with
  no alpha channel and a file size of 945,132 bytes.
- The public homepage displays the logo at small UI sizes in the header,
  model-flow chip, and request-journey node.
- The homepage and static beginner guide use the same PNG as their favicon.
- `/home-assets/*` is already served with
  `Cache-Control: public, max-age=31536000, immutable`.
- The deployed path is produced by the Vite homepage build and copied into
  the custom Caddy image.

## Approved Design

### Image production

Generate a 256-by-256 WebP from the existing PNG using deterministic local
image tooling. Preserve the square composition and use high visual quality.
The published WebP must be no larger than 50 KiB.

Use a content-versioned public filename. The version must change when the
image bytes change because the current Caddy policy makes `/home-assets/*`
immutable for one year. Keep the original high-resolution PNG outside the
published `homepage/public/` tree as a source master so it remains available
for future brand work without being copied into the production build.

### Reference migration

Update all public homepage and beginner-guide references from
`/home-assets/xingqiao-logo.png` to the new WebP path:

- `homepage/src/components/Header.tsx`
- `homepage/src/sections/ModelFlow.tsx`
- `homepage/src/sections/RequestJourney.tsx`
- `homepage/index.html`
- `homepage/public/docs/index.html`

Set favicon MIME types to `image/webp`. Do not add a second fallback download;
the supported browser baseline already supports WebP.

### Regression protection

Add a focused executable asset-contract test that checks the real published
asset rather than grepping source text. It must fail when the WebP is absent,
not 256-by-256, larger than 50 KiB, or when the legacy 945 KiB PNG remains in
the public directory.

Existing component tests continue to cover rendering. The complete homepage
test suite, TypeScript check, and production build must pass. After building,
the same size and dimension contract must hold in `homepage/dist` and the
built HTML/JavaScript must not request the legacy PNG.

## Verification

The phase is complete only when all of the following evidence exists:

1. The new public WebP is exactly 256 by 256 pixels and no larger than 51,200
   bytes.
2. `homepage/public/home-assets/xingqiao-logo.png` no longer exists.
3. The original PNG is retained as a non-published source master.
4. The focused asset-contract test was observed failing before the asset and
   references changed, then passing after implementation.
5. `npm run test:run`, `npm run typecheck`, and `npm run build` pass in
   `homepage/`.
6. The built `homepage/dist` contains the WebP, omits the legacy PNG, and has
   no remaining request reference to `xingqiao-logo.png`.
7. A local visual check confirms that header, flow, journey, and favicon uses
   remain sharp at their rendered sizes.

## Deferred Work

The following work is explicitly outside this implementation:

- EdgeOne, CDN, COS, or Hong Kong server purchases.
- DNSPod record changes.
- Production deployment or Caddy image switching.
- CDN trusted-proxy/firewall changes.
- JavaScript bundle splitting and QR-code optimization.

A separate read-only research task will compare the lowest-cost unfiled-domain
option for a later second phase. Its findings do not authorize implementation.
