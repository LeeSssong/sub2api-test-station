#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

require_file() {
  [[ -f "$1" ]] || fail "missing $1"
}

require_fixed() {
  local needle=$1
  local file=$2
  rg -Fq -- "$needle" "$file" || fail "missing expected value in $file: $needle"
}

require_file infra/sub2api-update-ui/index.html
require_file infra/sub2api-update-ui/update-ui.js
require_file infra/sub2api-update-ui/update-ui.css
require_file infra/Caddyfile
require_file infra/compose.yaml

require_fixed 'trusted_proxies static {$CADDY_TRUSTED_PROXIES:172.30.0.3/32}' infra/Caddyfile
require_fixed 'trusted_proxies_strict' infra/Caddyfile
require_fixed '@sub2api_official_index path /__sub2api-official-index' infra/Caddyfile
require_fixed 'reverse_proxy {$SUB2API_ACTIVE_UPSTREAM:sub2api-blue:8080}' infra/Caddyfile
require_fixed 'templates' infra/Caddyfile
require_fixed 'httpInclude "/__sub2api-official-index"' infra/sub2api-update-ui/index.html
require_fixed 'src="/xingqiao-update-ui.js?v=20260729-1"' infra/sub2api-update-ui/index.html
require_fixed 'href="/xingqiao-update-ui.css?v=20260726-1"' infra/sub2api-update-ui/index.html
require_fixed '@sub2api_update {' infra/Caddyfile
require_fixed $'\t\tmethod POST' infra/Caddyfile
require_fixed $'\t\tpath /api/v1/admin/system/update' infra/Caddyfile
require_fixed 'reverse_proxy @sub2api_update unix//run/sub2api-updater/updater.sock' infra/Caddyfile
require_fixed 'response_header_timeout 15m' infra/Caddyfile
require_fixed 'reverse_proxy @sub2api_host_update_status unix//run/sub2api-updater/updater.sock' infra/Caddyfile
require_fixed 'reverse_proxy @sub2api_host_update_schedule unix//run/sub2api-updater/updater.sock' infra/Caddyfile
require_fixed 'path /api/v1/admin/system/host-update/readiness' infra/Caddyfile
require_fixed 'reverse_proxy @sub2api_host_update_readiness unix//run/sub2api-updater/updater.sock' infra/Caddyfile
require_fixed 'path /api/v1/admin/system/check-updates' infra/Caddyfile
require_fixed 'path /api/v1/admin/system/rollback' infra/Caddyfile
require_fixed 'path /api/v1/admin/system/restart' infra/Caddyfile
require_fixed 'path / /home /home/' infra/Caddyfile
require_fixed 'path /support /support/' infra/Caddyfile
require_fixed 'reverse_proxy @relay_ops_public relay-ops:8100' infra/Caddyfile
require_fixed '@relay_ops_reconciliation path /relay-ops/api/reconciliation/*' infra/Caddyfile
require_fixed 'reverse_proxy @relay_ops_reconciliation relay-ops:8100' infra/Caddyfile
require_fixed 'not path /relay-ops/api/reconciliation/*' infra/Caddyfile
require_fixed '@legacy_ops path /ops /ops/*' infra/Caddyfile
require_fixed 'redir @legacy_ops /admin/ops 302' infra/Caddyfile
require_fixed '@retired_relay_ops_api path /relay-ops/api/ops-view /relay-ops/api/incidents/ack /relay-ops/api/feishu/events' infra/Caddyfile
require_fixed 'respond @retired_relay_ops_api 404' infra/Caddyfile
require_fixed './sub2api-update-ui:/srv/sub2api-update-ui:ro' infra/compose.yaml
require_fixed '/run/sub2api-updater:/run/sub2api-updater:ro' infra/compose.yaml
require_fixed 'CADDY_TRUSTED_PROXIES: ${CADDY_TRUSTED_PROXIES:-172.30.0.3/32}' infra/compose.yaml
require_fixed 'rewrite * /update-ui.js' infra/Caddyfile
require_fixed 'rewrite * /update-ui.css' infra/Caddyfile

if rg -n -F 'reverse_proxy @relay_ops_admin relay-ops:8100' infra/Caddyfile; then
  fail 'retired relay ops APIs must not be publicly routed'
fi
if rg -n -F 'reverse_proxy @relay_ops_feishu_command relay-ops:8100' infra/Caddyfile; then
  fail 'retired Feishu callback must not be publicly routed'
fi

if rg -n '/(var/)?run/docker\.sock|docker\.sock' infra/compose.yaml infra/Caddyfile; then
  fail 'Docker socket must not be mounted'
fi

update_line=$(rg -n -F '@sub2api_update {' infra/Caddyfile | head -n1 | cut -d: -f1)
retired_api_line=$(rg -n -F '@retired_relay_ops_api path ' infra/Caddyfile | head -n1 | cut -d: -f1)
fallback_line=$(rg -n -F 'reverse_proxy {$SUB2API_ACTIVE_UPSTREAM:sub2api-blue:8080}' infra/Caddyfile | tail -n1 | cut -d: -f1)
[[ "$update_line" -lt "$fallback_line" ]] || fail 'update route must precede the generic Sub2API proxy'
[[ "$retired_api_line" -lt "$fallback_line" ]] || fail 'retired relay APIs must be rejected before the generic Sub2API proxy'

if rg -n -F 'path /api/v1/auth/register /api/v1/auth/login /api/v1/auth/login/2fa' infra/Caddyfile; then
  fail 'native authentication routes must not be intercepted before the generic Sub2API proxy'
fi
if rg -n -F 'path /api/v1/settings/public' infra/Caddyfile; then
  fail 'native public settings route must not be intercepted before the generic Sub2API proxy'
fi

printf 'PASS: Sub2API update UI and routing contracts\n'
