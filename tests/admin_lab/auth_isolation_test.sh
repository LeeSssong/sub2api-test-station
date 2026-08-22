#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

grep -Fq 'VITE_AUTH_STORAGE_PREFIX=admin_lab_' infra/admin-lab/Dockerfile.frontend
grep -Fq 'VITE_API_BASE_URL=/admin/lab/api/v1' infra/admin-lab/Dockerfile.frontend
grep -Fq 'header_up -Cookie' infra/Caddyfile
grep -Fq 'JWT_SECRET: ${ADMIN_LAB_JWT_SECRET' infra/compose.admin-lab.yaml
grep -Fq 'ADMIN_LAB_COOKIE_NAME=sub2api_lab_session' infra/.env.admin-lab.example
grep -Fq 'authStorageGet' upstream/sub2api/frontend/src/api/client.ts
grep -Fq 'authStorageSet' upstream/sub2api/frontend/src/stores/auth.ts

if grep -RInE "VITE_AUTH_STORAGE_PREFIX=($|sub2api_)|ADMIN_LAB_JWT_SECRET=.*prod|ADMIN_LAB_COOKIE_NAME=sub2api_session" infra upstream/sub2api/frontend/src 2>/dev/null; then
  echo 'production auth namespace leaked into lab' >&2
  exit 1
fi
echo 'admin lab auth isolation contract: PASS'
