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

require_file infra/compose.yaml
require_file infra/compose.bootstrap.yaml
require_file infra/.env.example
require_file infra/Caddyfile
require_file infra/Dockerfile.caddy
require_file infra/Caddyfile.bootstrap
require_file infra/Dockerfile.relay-ops
require_file tests/relay_ops/validate_relay_ops_contract.sh

docker compose \
  --env-file infra/.env.example \
  -f infra/compose.yaml \
  config --quiet || fail 'docker compose config failed'

SITE_ADDRESS=api.example.com docker compose \
  -f infra/compose.bootstrap.yaml \
  config --quiet || fail 'bootstrap docker compose config failed'

images=(
  'weishaw/sub2api@sha256:4467f888bc37bcb297220e3246b22cb55861aea40e319307bf4512b98eac2ce8'
  'postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15'
  'redis:8-alpine@sha256:9d317178eceac8454a2284a9e6df2466b93c745529947f0cd42a0fa9609d7005'
  'xingqiao-caddy:homepage-20260723-v2-headline-spacing'
)

for image in "${images[@]}"; do
  require_fixed "image: $image" infra/compose.yaml
done

if rg -n ':latest([[:space:]]|$)' infra/compose.yaml; then
  fail 'mutable latest tag is forbidden'
fi

ports_owner=$(awk '
  /^  [a-zA-Z0-9_-]+:/ { service=$1 }
  /^    ports:/ { print service }
' infra/compose.yaml)
[[ "$ports_owner" == 'caddy:' ]] || fail 'only caddy may publish host ports'
require_fixed 'ports: ["80:80", "443:443"]' infra/compose.yaml

for setting in \
  'DATABASE_MAX_OPEN_CONNS=20' \
  'DATABASE_MAX_IDLE_CONNS=5' \
  'POSTGRES_MAX_CONNECTIONS=60' \
  'POSTGRES_SHARED_BUFFERS=128MB' \
  'REDIS_MAXCLIENTS=1000' \
  'REDIS_POOL_SIZE=64' \
  'REDIS_MIN_IDLE_CONNS=5' \
  'SERVER_TRUSTED_PROXIES=172.16.0.0/12' \
  'SERVER_MAX_REQUEST_BODY_SIZE=16777216' \
  'GATEWAY_MAX_BODY_SIZE=16777216' \
  'GATEWAY_TEXT_MAX_BODY_SIZE=16777216'; do
  require_fixed "$setting" infra/.env.example
done

for setting in \
  'SECURITY_URL_ALLOWLIST_ENABLED: "true"' \
  'SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP: "false"' \
  'SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS: "false"' \
  'SERVER_TRUSTED_PROXIES: ${SERVER_TRUSTED_PROXIES:-172.16.0.0/12}' \
  'SERVER_MAX_REQUEST_BODY_SIZE: ${SERVER_MAX_REQUEST_BODY_SIZE:-16777216}' \
  'GATEWAY_MAX_BODY_SIZE: ${GATEWAY_MAX_BODY_SIZE:-16777216}' \
  'GATEWAY_TEXT_MAX_BODY_SIZE: ${GATEWAY_TEXT_MAX_BODY_SIZE:-16777216}'; do
  require_fixed "$setting" infra/compose.yaml
done

require_fixed 'reverse_proxy sub2api:8080' infra/Caddyfile
require_fixed 'flush_interval -1' infra/Caddyfile
require_fixed 'reverse_proxy @relay_ops_public relay-ops:8100' infra/Caddyfile
require_fixed 'reverse_proxy @relay_ops_admin relay-ops:8100' infra/Caddyfile
require_fixed 'path / /home /home/' infra/Caddyfile
require_fixed 'path /home-assets/site-config.json' infra/Caddyfile
require_fixed 'path /home-assets/*' infra/Caddyfile
require_fixed 'Cache-Control "no-store, max-age=0"' infra/Caddyfile
require_fixed 'Cache-Control "public, max-age=31536000, immutable"' infra/Caddyfile

require_fixed 'FROM node:22-alpine@sha256:b74031e546d7f4faf561d797ac1b76beccac856a042815ca77db4fd047581605 AS homepage-build' infra/Dockerfile.caddy
require_fixed 'FROM caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d' infra/Dockerfile.caddy
require_fixed 'COPY --from=homepage-build /src/dist /srv/home' infra/Dockerfile.caddy
require_fixed 'dockerfile: infra/Dockerfile.caddy' infra/compose.yaml

require_fixed 'image: caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d' infra/compose.bootstrap.yaml
require_fixed 'ports: ["80:80", "443:443"]' infra/compose.bootstrap.yaml
require_fixed './Caddyfile.bootstrap:/etc/caddy/Caddyfile:ro' infra/compose.bootstrap.yaml
require_fixed '{"status":"ok","phase":"bootstrap"}' infra/Caddyfile.bootstrap

git check-ignore -q infra/.env || fail 'infra/.env is not ignored'
if git ls-files --error-unmatch infra/.env >/dev/null 2>&1; then
  fail 'infra/.env must never be tracked'
fi

if rg -n -i \
  '(sk-[a-z0-9]{16,}|whsec_[a-z0-9]{16,}|BEGIN [A-Z ]*PRIVATE KEY|Bearer[[:space:]]+eyJ|Cookie:[[:space:]]*[^[:space:]])' \
  infra; then
  fail 'possible secret found in controlled infrastructure files'
fi

require_file ops/generate-env.sh

TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT
GENERATED_ENV="$TEMP_DIR/.env"

ops/generate-env.sh "$GENERATED_ENV" >/dev/null
[[ -f "$GENERATED_ENV" ]] || fail 'environment generator did not create target'

get_value() {
  local key=$1
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$GENERATED_ENV"
}

secret_keys=(
  POSTGRES_PASSWORD
  REDIS_PASSWORD
  ADMIN_PASSWORD
  JWT_SECRET
  TOTP_ENCRYPTION_KEY
)

seen_values='|'
for key in "${secret_keys[@]}"; do
  value=$(get_value "$key")
  [[ "$value" =~ ^[0-9a-f]{64}$ ]] || fail "$key is not 64 lowercase hex characters"
  case "$seen_values" in
    *"|$value|"*) fail 'generated secrets must be distinct' ;;
  esac
  seen_values="${seen_values}${value}|"
done

if stat -f '%Lp' "$GENERATED_ENV" >/dev/null 2>&1; then
  mode=$(stat -f '%Lp' "$GENERATED_ENV")
else
  mode=$(stat -c '%a' "$GENERATED_ENV")
fi
[[ "$mode" == '600' ]] || fail "generated environment mode is $mode, expected 600"

if ops/generate-env.sh "$GENERATED_ENV" >/dev/null 2>&1; then
  fail 'environment generator overwrote an existing target'
fi

printf 'PASS: infrastructure baseline contracts\n'
