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
grep -Fq '@admin_lab_login path /admin/lab/login' infra/Caddyfile
grep -Fq '@admin_lab_no_session' infra/Caddyfile
grep -Fq 'not header Cookie *sub2api_lab_session=*' infra/Caddyfile
grep -Fq 'forward_auth admin-lab-gateway:8088' infra/Caddyfile
grep -Fq 'uri /api/v1/auth/lab-session' infra/Caddyfile
grep -Fq 'RequireLabAdmin' upstream/sub2api/backend/internal/lab/auth_policy.go
grep -Fq 'SetAdminSessionCookie' upstream/sub2api/backend/internal/handler/auth_handler.go

if grep -RInE "VITE_AUTH_STORAGE_PREFIX=($|sub2api_)|ADMIN_LAB_JWT_SECRET=.*prod|ADMIN_LAB_COOKIE_NAME=sub2api_session" infra upstream/sub2api/frontend/src 2>/dev/null; then
  echo 'production auth namespace leaked into lab' >&2
  exit 1
fi
echo 'admin lab auth isolation contract: PASS'
