#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

require_file() {
  local path=$1
  [[ -f "$path" ]] || { echo "missing native file: $path" >&2; exit 1; }
}

require_text() {
  local needle=$1
  local path=$2
  grep -Fq -- "$needle" "$path" || {
    echo "missing native contract: $needle in $path" >&2
    exit 1
  }
}

require_file upstream/sub2api/backend/internal/server/router.go
require_file upstream/sub2api/backend/internal/server/routes/admin.go
require_file upstream/sub2api/backend/internal/server/routes/auth.go
require_file upstream/sub2api/backend/internal/handler/auth_handler.go
require_file upstream/sub2api/frontend/src/api/client.ts
require_file upstream/sub2api/frontend/src/api/auth.ts
require_file infra/compose.yaml
require_file infra/Caddyfile

require_text 'RegisterAdminRoutes(v1' upstream/sub2api/backend/internal/server/router.go
require_text 'RegisterAuthRoutes(v1' upstream/sub2api/backend/internal/server/router.go
require_text 'baseURL: getAPIBaseURL()' upstream/sub2api/frontend/src/api/client.ts
require_text 'localStorage.getItem('\''auth_token'\'')' upstream/sub2api/frontend/src/api/client.ts
require_text 'reverse_proxy {$SUB2API_ACTIVE_UPSTREAM:sub2api-blue:8080}' infra/Caddyfile
require_text 'DATABASE_HOST: postgres' infra/compose.yaml
require_text 'REDIS_HOST: redis' infra/compose.yaml

# Guard against accidentally introducing a second accounting fact source in the lab.
if find tools/admin-lab upstream/sub2api/backend/internal/lab -type f 2>/dev/null \
    | xargs -r grep -IlE 'actual_cost|user_cost|account_cost|usage_logs' | grep -q .; then
  echo 'lab implementation must not introduce a second accounting fact source' >&2
  exit 1
fi

echo "native reuse inventory: PASS"
printf '%s\n' \
  'auth: reuse native JWT bearer middleware and admin route registration' \
  'admin API: reuse native /api/v1/admin handlers and DTOs' \
  'frontend: reuse native Vue app/router/API client; add lab base and storage namespace only' \
  'data: reuse native migrations in an independent PostgreSQL database' \
  'cache: reuse native Redis integration against an independent lab instance' \
  'proxy: extend existing Caddy routing before the production fallback only'
git rev-parse HEAD
