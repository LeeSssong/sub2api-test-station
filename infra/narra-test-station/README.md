# Narra test-station workspace

Pinned source and AGPL license: workstations/narra/UPSTREAM.md.
Mount /image-workstation through test-station Caddy, /image-studio inside existing authenticated Vue shell.
Dedicated queue DB and role, protected /opt/sub2api-test-station/narra/.env, local /opt/sub2api-test-station/narra/media.
No production operations. Build runtime from clean test-station main. Images 512MiB with reserve 768MiB; session 6h; cleaner 60s. Admin current-image list 200.
Local storage reports LOCAL in the existing string mediaStorage field; protected URL and storageKey identify local files. Storage replacement must configure both Node and Go adapters.
Runtime commands: node server.js, /app/narra-worker, node site-cleanup.mjs. Worker concurrency 1 / MaxAttempts 1; uncertain upstream requests never retried. Site billing unchanged.

## Release source and maintenance

Only the isolated test-station repository clean main, equal to fetched origin/main (origin points to LeeSssong/sub2api-test-station), may supply release inputs. Never push the production origin.
`ops/deploy-narra-test-station.py` wraps the existing API controller for the user-authorized maintenance release: restore rehearsal, stop API/worker, consistent backup, native migration-only, new API/worker, public probes, route promotion. Compatible post-migration application rollback preserves new data after workers/traffic resume. Pre-worker failure restores stopped database backup.
Narra DB initialization uses `prisma migrate deploy` against narra_image only; dedicated role cannot access site DB.

## One key across GPT and Grok

Use Sub2API's native admin Groups UI: create a Composite group, enable image generation, associate suitable OpenAI and Grok accounts, and add exact model routes scoped to `images`. Map `gpt-image-2` to the OpenAI account's real model and `grok-imagine` to the Grok account's supported image model. Assign the key to this group and keep its model allowlist consistent with public route IDs. Existing native pricing, permissions and accounting remain authoritative; the workstation does not alter them or grant cross-group access.

The workstation fetches `/v1/models` with the supplied key, then presents supported image families. The catalogue is discovery, not a successful-generation guarantee. Unknown compatible image families use conservative defaults. For custom route aliases, set protected `WORKSTATION_MODEL_FAMILIES={"your-alias":"grok"}` (families: gpt-image, grok, dalle, compatible) in the Narra environment and pass it to the app. This only declares UI/request capability, never native permission or pricing.

Grok geometry is sent as size and converted by the native gateway into supported aspect ratio and resolution. GPT-only quality/format fields are hidden and rejected for Grok. Browser entry validates/refreshes the native session before exchange, accepts the actual same-origin proxy host/protocol, and syncs light/dark theme without reloading the iframe.
