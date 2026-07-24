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

extract_caddy_block() {
  local marker=$1
  awk -v marker="$marker" '
    $0 == marker { capture = 1 }
    capture {
      print
      opens += gsub(/\{/, "{")
      closes += gsub(/\}/, "}")
      if (opens > 0 && opens == closes) exit
    }
  ' infra/Caddyfile
}

require_in_block() {
  local needle=$1
  local block=$2
  local name=$3
  [[ "$block" == *"$needle"* ]] || fail "missing expected value in $name block: $needle"
}

require_file infra/compose.yaml
require_file infra/compose.bootstrap.yaml
require_file infra/.env.example
require_file infra/Caddyfile
require_file infra/Dockerfile.caddy
require_file infra/Caddyfile.bootstrap
require_file infra/Dockerfile.relay-ops
require_file tests/relay_ops/validate_relay_ops_contract.sh
require_file config/releases/sub2api.env
require_file infra/compose.sub2api-release.yaml
require_file tests/infra/validate-official-sub2api-release.sh
require_file tests/infra/audit-public-links.sh
test -x tests/infra/audit-public-links.sh || fail 'public link audit must be executable'

require_fixed '@docs_root path /docs' infra/Caddyfile
require_fixed 'redir @docs_root /docs/ 308' infra/Caddyfile
require_fixed '@docs_index path /docs/' infra/Caddyfile
require_fixed 'rewrite * /docs/index.html' infra/Caddyfile
require_fixed '@docs_assets path /docs/*' infra/Caddyfile
require_fixed "script-src 'none'" infra/Caddyfile

docs_line=$(rg -n -F '@docs_root path /docs' infra/Caddyfile | head -n1 | cut -d: -f1)
proxy_line=$(rg -n -F 'reverse_proxy sub2api:8080' infra/Caddyfile | head -n1 | cut -d: -f1)
[[ -n "$docs_line" && -n "$proxy_line" && "$docs_line" -lt "$proxy_line" ]] || \
  fail 'docs handlers must appear before the Sub2API fallback proxy'

docker compose \
  --project-name sub2api-deploy \
  --project-directory "$ROOT" \
  --env-file infra/.env.example \
  --env-file config/releases/sub2api.env \
  -f infra/compose.yaml \
  -f infra/compose.sub2api-release.yaml \
  config --quiet || fail 'docker compose config failed'

bash tests/infra/validate-official-sub2api-release.sh || fail 'official Sub2API release contract failed'

SITE_ADDRESS=api.example.com docker compose \
  -f infra/compose.bootstrap.yaml \
  config --quiet || fail 'bootstrap docker compose config failed'

images=(
  'postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15'
  'redis:8-alpine@sha256:9d317178eceac8454a2284a9e6df2466b93c745529947f0cd42a0fa9609d7005'
  'xingqiao-caddy:homepage-20260724-v6-full-width-signal'
)

for image in "${images[@]}"; do
  require_fixed "image: $image" infra/compose.yaml
done

require_fixed 'image: ${SUB2API_IMAGE:?SUB2API_IMAGE is required}' infra/compose.yaml
require_fixed 'SUB2API_IMAGE=weishaw/sub2api:0.1.164@sha256:a94c25fb4c50c3bf21155142d745ff11a8d9199e4cf72d9a2424d75ccbfc1659' config/releases/sub2api.env

if rg -n '^[[:space:]]*SUB2API_IMAGE[[:space:]]*=' infra/.env.example; then
  fail 'release env must be the sole SUB2API_IMAGE source'
fi

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
  'GATEWAY_TEXT_MAX_BODY_SIZE=16777216' \
  'SUB2API_DATA_DIR=./data' \
  'POSTGRES_DATA_DIR=./postgres_data' \
  'REDIS_DATA_DIR=./redis_data' \
  'SECURITY_URL_ALLOWLIST_ENABLED=false' \
  'SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP=true' \
  'SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS=true' \
  'SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS='; do
  require_fixed "$setting" infra/.env.example
done

for setting in \
  'SECURITY_URL_ALLOWLIST_ENABLED: ${SECURITY_URL_ALLOWLIST_ENABLED:-false}' \
  'SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP: ${SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP:-true}' \
  'SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS: ${SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS:-true}' \
  'SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS: ${SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS:-}' \
  'SERVER_TRUSTED_PROXIES: ${SERVER_TRUSTED_PROXIES:-172.16.0.0/12}' \
  'SERVER_MAX_REQUEST_BODY_SIZE: ${SERVER_MAX_REQUEST_BODY_SIZE:-16777216}' \
  'GATEWAY_MAX_BODY_SIZE: ${GATEWAY_MAX_BODY_SIZE:-16777216}' \
  'GATEWAY_TEXT_MAX_BODY_SIZE: ${GATEWAY_TEXT_MAX_BODY_SIZE:-16777216}' \
  'source: ${SUB2API_DATA_DIR:?SUB2API_DATA_DIR is required}' \
  'source: ${POSTGRES_DATA_DIR:?POSTGRES_DATA_DIR is required}' \
  'source: ${REDIS_DATA_DIR:?REDIS_DATA_DIR is required}'; do
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
require_fixed 'path /support/*' infra/Caddyfile
require_fixed 'DOCKER_DEPLOYMENT_UPDATE_REQUIRED' infra/Caddyfile
require_fixed "frame-src 'self'" infra/Caddyfile

support_index_block=$(extract_caddy_block $'\t@support_index path /support /support/')
[[ -n "$support_index_block" ]] || fail 'missing exact @support_index matcher'
require_in_block $'\thandle @support_index {' "$support_index_block" '@support_index'
require_in_block $'\t\trewrite * /index.html' "$support_index_block" '@support_index'
require_in_block $'\t\tfile_server' "$support_index_block" '@support_index'

binary_mutation_block=$(extract_caddy_block $'\t@sub2api_binary_mutation {')
[[ -n "$binary_mutation_block" ]] || fail 'missing @sub2api_binary_mutation matcher'
method_lines=$(rg '^[[:space:]]*method ' <<< "$binary_mutation_block")
path_lines=$(rg '^[[:space:]]*path ' <<< "$binary_mutation_block")
[[ "$method_lines" == $'\t\tmethod POST' ]] || fail 'binary mutation matcher must be POST-only'
[[ "$path_lines" == $'\t\tpath /api/v1/admin/system/update /api/v1/admin/system/rollback' ]] || \
  fail 'binary mutation matcher must contain exactly the update and rollback paths'
[[ "$binary_mutation_block" != *'method GET'* && "$binary_mutation_block" != *'/api/v1/admin/system/*'* ]] || \
  fail 'binary mutation matcher must not include GET or near-match wildcard routes'

expected_binary_response=$'\trespond @sub2api_binary_mutation `{"code":"DOCKER_DEPLOYMENT_UPDATE_REQUIRED","message":"Docker 部署仅支持受控 Compose 更新，请使用运维发布流程"}` 409'
binary_response_lines=$(rg '^[[:space:]]*respond @sub2api_binary_mutation' infra/Caddyfile)
[[ "$binary_response_lines" == "$expected_binary_response" ]] || \
  fail 'binary mutation response must be the unique exact JSON 409 response with no near-match responder'

binary_mutation_line=$(rg -n -F '@sub2api_binary_mutation {' infra/Caddyfile | head -n1 | cut -d: -f1)
proxy_line=$(rg -n -F 'reverse_proxy sub2api:8080' infra/Caddyfile | head -n1 | cut -d: -f1)
[[ -n "$binary_mutation_line" && -n "$proxy_line" && "$binary_mutation_line" -lt "$proxy_line" ]] || \
  fail 'binary mutation guard must appear before the Sub2API fallback proxy'

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
