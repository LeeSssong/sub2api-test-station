# Narra test-station workspace

Pinned source and AGPL license: workstations/narra/UPSTREAM.md.
Mount /image-workstation through test-station Caddy, /image-studio inside existing authenticated Vue shell.
Dedicated queue DB and role, protected /opt/sub2api-test-station/narra/.env, local /opt/sub2api-test-station/narra/media.
No production operations. Build runtime from clean test-station main. Images 512MiB with reserve 768MiB; session 6h; cleaner 60s. Admin current-image list 200.
Local storage uses upstream S3 mediaStorage enum to preserve schema compatibility; local protected URL and storageKey identify local files. Storage replacement must configure both Node and Go adapters.
Runtime commands: node server.js, /app/narra-worker, node site-cleanup.mjs. Worker concurrency 1 / MaxAttempts 1; uncertain upstream requests never retried. Site billing unchanged.
