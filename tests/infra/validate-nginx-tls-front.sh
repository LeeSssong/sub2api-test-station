#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

require_fixed() {
  local needle=$1
  local file=$2
  rg -Fq -- "$needle" "$file" || fail "missing expected value in $file: $needle"
}

[[ -f infra/nginx-tls-front.conf.template ]] || fail 'missing nginx TLS front template'
[[ -f infra/nginx-tls-front-entrypoint.sh ]] || fail 'missing nginx TLS front entrypoint'

for expected in \
  'listen 443 ssl;' \
  'ssl_protocols TLSv1.2 TLSv1.3;' \
  'location ^~ /.well-known/acme-challenge/' \
  'proxy_pass http://caddy_http;' \
  'proxy_pass https://caddy_https;' \
  'server caddy-tls-origin:443 resolve;' \
  'proxy_ssl_trusted_certificate /etc/ssl/cert.pem;' \
  'proxy_ssl_verify on;' \
  'proxy_set_header Authorization $http_authorization;' \
  'proxy_set_header Upgrade $http_upgrade;' \
  'proxy_buffering off;' \
  'proxy_request_buffering off;' \
  'proxy_read_timeout 1800s;' \
  'client_max_body_size 128m;'; do
  require_fixed "$expected" infra/nginx-tls-front.conf.template
done

require_fixed 'trusted_proxies static {$CADDY_TRUSTED_PROXIES:172.30.0.3/32}' infra/Caddyfile

for expected in \
  'NGINX_TLS_CERT_ROOT' \
  'NGINX_TLS_CERT_FILE' \
  'NGINX_TLS_KEY_FILE' \
  'discover_certificate_pair' \
  'sha256sum' \
  'watch_certificates &' \
  'wait "$nginx_pid"' \
  'nginx -s reload'; do
  require_fixed "$expected" infra/nginx-tls-front-entrypoint.sh
done

fixture=$(mktemp -d)
docker_fixture=$(mktemp -d "$ROOT/.nginx-tls-front-test.XXXXXX")
trap 'rm -rf "$fixture" "$docker_fixture"' EXIT

cat >"$fixture/secret.env" <<'EOF'
SITE_ADDRESS=api.example.com
POSTGRES_USER=sub2api
POSTGRES_PASSWORD=test-postgres-password
POSTGRES_DB=sub2api
REDIS_PASSWORD=test-redis-password
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=test-admin-password
JWT_SECRET=test-jwt-secret
TOTP_ENCRYPTION_KEY=test-totp-key
SUB2API_DATA_DIR=./data
POSTGRES_DATA_DIR=./postgres
REDIS_DATA_DIR=./redis
EOF

cat >"$fixture/release.env" <<'EOF'
SUB2API_BLUE_IMAGE=example.invalid/sub2api-blue@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
SUB2API_GREEN_IMAGE=example.invalid/sub2api-green@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
SUB2API_WORKER_IMAGE=example.invalid/sub2api-worker@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080
SUB2API_ACTIVE_SLOT=blue
SUB2API_PREVIOUS_SLOT=green
EOF

config=$(env SUB2API_RELEASE_ENV_FILE="$fixture/release.env" docker compose \
  --env-file "$fixture/secret.env" \
  --env-file "$fixture/release.env" \
  -f infra/compose.yaml \
  config --format json)

jq -e '
  .services.caddy.ports == null and
  (.services.caddy.expose | sort) == ["443", "80", "8081"] and
  .services.caddy.networks.tls_front.ipv4_address == "172.30.0.2" and
  (.services.caddy.healthcheck.test | join(" ")) == "CMD wget -q -T 5 -O /dev/null http://127.0.0.1:8081/health" and
  (.services["nginx-tls-front"].ports | map(.target) | sort) == [80, 443] and
  .services["nginx-tls-front"].depends_on.caddy.condition == "service_healthy" and
  .services["nginx-tls-front"].networks.tls_front.ipv4_address == "172.30.0.3" and
  (.services["nginx-tls-front"].healthcheck.test | join(" ")) == "CMD-SHELL wget --no-check-certificate --header=\"Host: $$SITE_ADDRESS\" -q -T 5 -O /dev/null https://127.0.0.1/health" and
  (.services["nginx-tls-front"].volumes | any(.type == "volume" and .source == "caddy_data" and .target == "/data" and .read_only == true))
' <<<"$config" >/dev/null || fail 'rendered nginx/Caddy TLS-front topology is invalid'

openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$docker_fixture/api.example.com.key" \
  -out "$docker_fixture/api.example.com.crt" \
  -subj '/CN=api.example.com' \
  -days 1 >/dev/null 2>&1

docker run --rm \
  --env SITE_ADDRESS=api.example.com \
  --volume "$ROOT/infra/nginx-tls-front.conf.template:/etc/nginx/templates/default.conf.template:ro" \
  --volume "$docker_fixture:/run/nginx-tls:ro" \
  nginx:1.27-alpine@sha256:65645c7bb6a0661892a8b03b89d0743208a18dd2f3f17a54ef4b76fb8e2f2a10 \
  /bin/sh -ec '/docker-entrypoint.d/20-envsubst-on-templates.sh; nginx -t' \
  >/dev/null || fail 'nginx TLS-front template failed to render or validate'

printf 'PASS: nginx TLS front contract\n'
