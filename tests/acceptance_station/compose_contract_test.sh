#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'acceptance station compose contract: FAIL: %s\n' "$*" >&2
  exit 1
}

compose_file=infra/compose.acceptance.yaml
caddy_file=infra/Caddyfile.acceptance
env_file=infra/.env.acceptance.example

[[ -f "$compose_file" ]] || fail "$compose_file is missing"
[[ -f "$caddy_file" ]] || fail "$caddy_file is missing"
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
grep -Fq 'response_header_timeout 15m' "$caddy_file" || fail '15-minute upstream timeout is missing'
grep -Fq 'header=\"Host: $$ACCEPTANCE_SITE_ADDRESS\"' "$compose_file" || fail 'caddy healthcheck must use acceptance host header'

if rg -n 'mock-upstream|PAYMENT_PROVIDER: mock|lab-outbox|sub2api_default|sub2api-blue|sub2api-green' \
  "$compose_file" "$caddy_file"; then
  fail 'mock or production topology identifier is forbidden'
fi

if rg -n '/admin/lab/' "$caddy_file"; then
  fail 'admin lab route is forbidden'
fi

printf 'acceptance station compose contract: PASS\n'
