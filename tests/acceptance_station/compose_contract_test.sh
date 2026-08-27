#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'acceptance station compose contract: FAIL: %s\n' "$*" >&2
  exit 1
}

compose_file=infra/compose.acceptance.yaml
caddy_file=infra/Caddyfile.acceptance
production_caddy_file=infra/Caddyfile
env_file=infra/.env.acceptance.example

[[ -f "$compose_file" ]] || fail "$compose_file is missing"
[[ -f "$caddy_file" ]] || fail "$caddy_file is missing"
[[ -f "$production_caddy_file" ]] || fail "$production_caddy_file is missing"
[[ -f "$env_file" ]] || fail "$env_file is missing"

docker compose --project-name sub2api-acceptance \
  --env-file "$env_file" \
  -f "$compose_file" config --quiet

grep -Fq 'name: sub2api-acceptance' "$compose_file" || fail 'independent project name is missing'
grep -Fq 'sub2api-acceptance-network' "$compose_file" || fail 'independent network is missing'
grep -Fq 'sub2api-acceptance-egress-network' "$compose_file" || fail 'dedicated egress network is missing'
grep -Fq 'acceptance-bootstrap' "$compose_file" || fail 'bootstrap profile service is missing'

for service in acceptance-api acceptance-worker acceptance-detector acceptance-postgres acceptance-redis acceptance-caddy; do
  grep -Fq "  $service:" "$compose_file" || fail "$service is missing"
done

for volume in sub2api-acceptance-data sub2api-acceptance-postgres sub2api-acceptance-redis sub2api-acceptance-caddy-data sub2api-acceptance-caddy-config; do
  grep -Fq "$volume" "$compose_file" || fail "$volume is missing"
done

grep -Fq "('backend_mode_enabled', 'true')" "$compose_file" || fail 'backend mode bootstrap setting is missing'
grep -Fq "('registration_enabled', 'false')" "$compose_file" || fail 'registration closure bootstrap setting is missing'
grep -Fq 'ON CONFLICT (key) DO UPDATE' "$compose_file" || fail 'bootstrap upsert is missing'
grep -Fq 'acceptance-worker: {condition: service_healthy}' "$compose_file" || fail 'api must wait for worker auto-setup'
grep -Fq 'response_header_timeout 15m' "$caddy_file" || fail '15-minute upstream timeout is missing'
grep -Fq '${ACCEPTANCE_LOOPBACK_BIND:-0.0.0.0}:${ACCEPTANCE_LOOPBACK_PORT:?ACCEPTANCE_LOOPBACK_PORT is required}:80' "$compose_file" \
  || fail 'acceptance caddy must bind only a configurable dedicated edge port'
grep -Fq 'http://127.0.0.1/admin/lab/health' "$compose_file" || fail 'caddy healthcheck must use the prefixed acceptance route'
grep -Fq ':80 {' "$caddy_file" || fail 'acceptance caddy must only listen for loopback HTTP'
grep -Fq 'handle_path /admin/lab/*' "$caddy_file" || fail 'acceptance caddy must strip the public lab prefix'
grep -Fq 'ARG VITE_APP_BASE_PATH=/' upstream/sub2api/Dockerfile \
  || fail 'acceptance frontend base-path build contract is missing'
grep -Fq 'ARG VITE_API_BASE_URL=/api/v1' upstream/sub2api/Dockerfile \
  || fail 'acceptance frontend API base build contract is missing'
grep -Fq 'ARG VITE_AUTH_STORAGE_PREFIX=' upstream/sub2api/Dockerfile \
  || fail 'acceptance frontend storage isolation build contract is missing'

grep -Fq '@acceptance_lab_root path /admin/lab' "$production_caddy_file" \
  || fail 'production caddy must own the acceptance root redirect'
grep -Fq '@acceptance_lab path /admin/lab /admin/lab/*' "$production_caddy_file" \
  || fail 'production caddy must proxy only the acceptance prefix'
grep -Fq '{$ACCEPTANCE_LAB_UPSTREAM:172.18.0.1:8181}' "$production_caddy_file" \
  || fail 'production caddy must proxy acceptance through the loopback upstream'
if rg -n 'remote_ip|Cf-Connecting-Ip|ACCEPTANCE_LAB_ALLOWED_IPS|respond 403' "$production_caddy_file"; then
  fail 'production caddy must not impose a network-layer ACL on the acceptance station'
fi
if rg -n 'admin-lab-gateway' "$production_caddy_file"; then
  fail 'production caddy must not route to the retired mock gateway'
fi
if rg -n '^\s*(path|handle|redir|reverse_proxy).*\/admin\/accounts' "$production_caddy_file"; then
  fail 'production caddy must not intercept the native admin accounts page'
fi

if rg -n 'mock-upstream|PAYMENT_PROVIDER: mock|lab-outbox|sub2api_default|sub2api-blue|sub2api-green' \
  "$compose_file" "$caddy_file"; then
  fail 'mock or production topology identifier is forbidden'
fi

if rg -n 'ACCEPTANCE_SITE_ADDRESS|https://|:443|80:80|443:443' "$caddy_file" "$compose_file"; then
  fail 'acceptance topology must not expose a second domain or TLS listener'
fi

printf 'acceptance station compose contract: PASS\n'
