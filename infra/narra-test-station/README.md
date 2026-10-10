# Narra test-station workspace

Pinned source and AGPL license: workstations/narra/UPSTREAM.md.
Mount /image-workstation through test-station Caddy, /image-studio inside existing authenticated Vue shell.
Dedicated queue DB and role, protected /opt/sub2api-test-station/narra/.env, local /opt/sub2api-test-station/narra/media.
No production operations. Build runtime from clean test-station main. Images 512MiB with reserve 768MiB; session 6h; cleaner 60s. Admin current-image list 200.
Local storage uses upstream S3 mediaStorage enum to preserve schema compatibility; local protected URL and storageKey identify local files. Storage replacement must configure both Node and Go adapters.
Runtime commands: node server.js, /app/narra-worker, node site-cleanup.mjs. Worker concurrency 1 / MaxAttempts 1; uncertain upstream requests never retried. Site billing unchanged.

## Release source and maintenance

Only the isolated test-station repository clean main, equal to fetched origin/main (origin points to LeeSssong/sub2api-test-station), may supply release inputs. Never push the production origin.
`ops/deploy-narra-test-station.py` wraps the existing API controller for the user-authorized maintenance release: restore rehearsal, stop API/worker, consistent backup, native migration-only, new API/worker, public probes, route promotion. Compatible post-migration application rollback preserves new data after workers/traffic resume. Pre-worker failure restores stopped database backup.
Narra DB initialization uses `prisma migrate deploy` against narra_image only; dedicated role cannot access site DB.
