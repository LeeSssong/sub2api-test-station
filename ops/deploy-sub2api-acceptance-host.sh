#!/usr/bin/env bash
set -euo pipefail

umask 077

fail() {
  printf 'acceptance_host_deploy status=failed: %s\n' "$1" >&2
  exit 1
}

mode_of() {
  stat -c '%a' -- "$1" 2>/dev/null || stat -f '%Lp' -- "$1"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$1" | awk '{print $1}'
  else
    shasum -a 256 -- "$1" | awk '{print $1}'
  fi
}

canonical_path() {
  cd -P "$(dirname "$1")" && printf '%s/%s\n' "$PWD" "$(basename "$1")"
}

usage() {
  fail 'usage: deploy-sub2api-acceptance-host.sh --staging-root DIR --image-archive FILE --image-sha256 FILE --compose FILE --caddy FILE --env-file FILE --source-commit SHA --source-tree SHA --deploy-root DIR'
}

staging_root=
image_archive=
image_sha256=
compose_source=
caddy_source=
env_source=
source_commit=
source_tree=
deploy_root=

while [[ $# -gt 0 ]]; do
  case "$1" in
    --staging-root) staging_root=${2:-}; shift 2 ;;
    --image-archive) image_archive=${2:-}; shift 2 ;;
    --image-sha256) image_sha256=${2:-}; shift 2 ;;
    --compose) compose_source=${2:-}; shift 2 ;;
    --caddy) caddy_source=${2:-}; shift 2 ;;
    --env-file) env_source=${2:-}; shift 2 ;;
    --source-commit) source_commit=${2:-}; shift 2 ;;
    --source-tree) source_tree=${2:-}; shift 2 ;;
    --deploy-root) deploy_root=${2:-}; shift 2 ;;
    *) usage ;;
  esac
done

[[ $EUID -eq 0 ]] || fail 'host executor must run as root'
[[ "$staging_root" =~ ^/var/tmp/sub2api-acceptance-release\.[A-Za-z0-9._-]+$ ]] || fail 'staging root is invalid'
[[ -d "$staging_root" && ! -L "$staging_root" ]] || fail 'staging root is invalid'
staging_root=$(canonical_path "$staging_root")
[[ "$(mode_of "$staging_root")" == 700 ]] || fail 'staging root must be mode 0700'
cleanup_staging() {
  rm -rf -- "$staging_root"
}
trap cleanup_staging EXIT
[[ "$deploy_root" =~ ^/opt/sub2api/acceptance-[A-Za-z0-9._-]+$ && "$deploy_root" != *..* ]] \
  || fail 'deploy root must be a canonical acceptance-only path'
for protected_path in /opt /opt/sub2api "$deploy_root"; do
  [[ ! -L "$protected_path" ]] || fail 'deploy root or parent component must not be a symlink'
done
[[ ! -L "$deploy_root" ]] || fail 'deploy root must not be a symlink'
if [[ -e "$deploy_root" ]]; then
  canonical_deploy_root=$(cd -P "$deploy_root" && pwd -P)
  [[ "$canonical_deploy_root" == "$deploy_root" ]] || fail 'deploy root must resolve to its canonical acceptance-only path'
fi

require_staged_file() {
  local path=$1
  [[ "$path" == "$staging_root/"* && -f "$path" && ! -L "$path" ]] \
    || fail 'all deployment inputs must be regular files inside staging root'
  path=$(canonical_path "$path")
  [[ "$path" == "$staging_root/"* ]] || fail 'all deployment inputs must remain inside staging root'
  printf '%s\n' "$path"
}

image_archive=$(require_staged_file "$image_archive")
image_sha256=$(require_staged_file "$image_sha256")
compose_source=$(require_staged_file "$compose_source")
caddy_source=$(require_staged_file "$caddy_source")
env_source=$(require_staged_file "$env_source")
[[ "$(mode_of "$env_source")" == 600 ]] || fail 'acceptance env must be mode 0600'
[[ "$source_commit" =~ ^[a-f0-9]{40}$ && "$source_tree" =~ ^[a-f0-9]{40}$ ]] || fail 'source identity is invalid'

expected_sha=$(awk 'NR == 1 { print $1 }' "$image_sha256")
[[ "$expected_sha" =~ ^[a-f0-9]{64}$ ]] || fail 'image checksum file is invalid'
[[ "$(sha256_file "$image_archive")" == "$expected_sha" ]] || fail 'image archive checksum mismatch'

load_acceptance_env() {
  local line key value
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]] || fail 'acceptance env contains an invalid assignment'
    key=${BASH_REMATCH[1]}
    value=${BASH_REMATCH[2]}
    [[ "$key" == ACCEPTANCE_* ]] || fail 'acceptance env may only contain ACCEPTANCE_ variables'
    printf -v "$key" '%s' "$value"
    export "$key"
  done <"$env_source"
}

load_acceptance_env
site_address=${ACCEPTANCE_SITE_ADDRESS:-}
project_name=${ACCEPTANCE_PROJECT_NAME:-}
network_name=${ACCEPTANCE_NETWORK_NAME:-}
env_deploy_root=${ACCEPTANCE_DEPLOY_ROOT:-}
loopback_port=${ACCEPTANCE_LOOPBACK_PORT:-}
payment_provider=${ACCEPTANCE_PAYMENT_PROVIDER:-}
upstream_provider=${ACCEPTANCE_UPSTREAM_PROVIDER:-}
notification_transport=${ACCEPTANCE_NOTIFICATION_TRANSPORT:-}
totp_encryption_key=${ACCEPTANCE_TOTP_ENCRYPTION_KEY:-}
[[ "$project_name" == sub2api-acceptance ]] || fail 'acceptance project identity is invalid'
[[ "$network_name" == sub2api-acceptance-network ]] || fail 'acceptance network identity is invalid'
[[ "$env_deploy_root" == "$deploy_root" ]] || fail 'acceptance deploy root does not match staged env'
[[ "$site_address" == api.xingqiaolab.top ]] || fail 'ACCEPTANCE_SITE_ADDRESS must be api.xingqiaolab.top'
[[ "$loopback_port" =~ ^[1-9][0-9]{3,4}$ && "$loopback_port" -le 65535 && "$loopback_port" -ne 443 ]] \
  || fail 'ACCEPTANCE_LOOPBACK_PORT is invalid'
[[ "$totp_encryption_key" =~ ^[A-Fa-f0-9]{64}$ ]] \
  || fail 'ACCEPTANCE_TOTP_ENCRYPTION_KEY must be 64 hexadecimal characters'
case "$site_address:$deploy_root:$project_name:$network_name" in
  *shop.xingqiaolab.top*|*/opt/sub2api/production*|*sub2api_default*|*':sub2api:'*)
    fail 'production identity is forbidden'
    ;;
esac
case "$payment_provider:$upstream_provider:$notification_transport" in
  *mock*|*lab-outbox*) fail 'mock flow is forbidden' ;;
esac
[[ "${ACCEPTANCE_REAL_FLOW_ACK:-}" == I_UNDERSTAND_REAL_CHARGES ]] || fail 'ACCEPTANCE_REAL_FLOW_ACK is required'

grep -Fq 'name: sub2api-acceptance' "$compose_source" || fail 'staged compose is not an acceptance topology'
grep -Fq 'sub2api-acceptance-network' "$compose_source" || fail 'staged compose network is invalid'
if grep -En 'sub2api_default|sub2api-blue|sub2api-green|mock-upstream|lab-outbox' "$compose_source" "$caddy_source"; then
  fail 'staged topology contains a forbidden production or lab identity'
fi

extraction_root=$(mktemp -d /var/tmp/sub2api-acceptance-extract.XXXXXX)
previous_root=
had_previous=false
deployment_started=false

install -d -m 700 -o root -g root "$extraction_root/runtime"
install -m 600 -o root -g root "$compose_source" "$extraction_root/runtime/compose.acceptance.yaml"
install -m 600 -o root -g root "$caddy_source" "$extraction_root/runtime/Caddyfile.acceptance"
install -m 600 -o root -g root "$env_source" "$extraction_root/runtime/.env"

loaded_image=$(docker load --input "$image_archive" | awk -F': ' '/Loaded image: / { print $2; exit }')
[[ -n "$loaded_image" ]] || fail 'image archive did not load a tagged image'
[[ "$loaded_image" != *$'\n'* && "$loaded_image" != *$'\r'* ]] || fail 'loaded image tag is invalid'
awk -v image="$loaded_image" '
  /^ACCEPTANCE_IMAGE=/ { print "ACCEPTANCE_IMAGE=" image; replaced=1; next }
  { print }
  END { if (!replaced) print "ACCEPTANCE_IMAGE=" image }
' "$extraction_root/runtime/.env" >"$extraction_root/runtime/.env.next"
install -m 600 -o root -g root "$extraction_root/runtime/.env.next" "$extraction_root/runtime/.env"
rm -f "$extraction_root/runtime/.env.next"

if [[ -d "$deploy_root" ]]; then
  previous_root=$(mktemp -d "${deploy_root}.previous.XXXXXX")
  if [[ -f "$deploy_root/compose.acceptance.yaml" && ! -L "$deploy_root/compose.acceptance.yaml" &&
        -f "$deploy_root/Caddyfile.acceptance" && ! -L "$deploy_root/Caddyfile.acceptance" &&
        -f "$deploy_root/.env" && ! -L "$deploy_root/.env" ]]; then
    cp -p "$deploy_root/compose.acceptance.yaml" "$previous_root/compose.acceptance.yaml"
    cp -p "$deploy_root/Caddyfile.acceptance" "$previous_root/Caddyfile.acceptance"
    cp -p "$deploy_root/.env" "$previous_root/.env"
    had_previous=true
  fi
fi
compose_cmd=(docker compose --project-name sub2api-acceptance --env-file "$deploy_root/.env" -f "$deploy_root/compose.acceptance.yaml")
rollback() {
  if [[ "$had_previous" == true ]]; then
    install -m 600 -o root -g root "$previous_root/compose.acceptance.yaml" "$deploy_root/compose.acceptance.yaml"
    install -m 600 -o root -g root "$previous_root/Caddyfile.acceptance" "$deploy_root/Caddyfile.acceptance"
    install -m 600 -o root -g root "$previous_root/.env" "$deploy_root/.env"
    docker compose --project-name sub2api-acceptance --env-file "$deploy_root/.env" \
      -f "$deploy_root/compose.acceptance.yaml" up -d --wait --no-build || true
  else
    "${compose_cmd[@]}" stop || true
  fi
}

cleanup() {
  local status=$?
  if [[ $status -ne 0 && "$deployment_started" == true ]]; then
    rollback || true
  fi
  rm -rf "$extraction_root"
  [[ -z "$previous_root" ]] || rm -rf "$previous_root"
  rm -rf -- "$staging_root"
}
trap cleanup EXIT

deployment_started=true
install -d -m 700 -o root -g root "$deploy_root"
install -m 600 -o root -g root "$extraction_root/runtime/compose.acceptance.yaml" "$deploy_root/compose.acceptance.yaml"
install -m 600 -o root -g root "$extraction_root/runtime/Caddyfile.acceptance" "$deploy_root/Caddyfile.acceptance"
install -m 600 -o root -g root "$extraction_root/runtime/.env" "$deploy_root/.env"

docker compose --project-name sub2api-acceptance --env-file "$deploy_root/.env" \
  -f "$deploy_root/compose.acceptance.yaml" up -d --wait --no-build
docker compose --project-name sub2api-acceptance --env-file "$deploy_root/.env" \
  -f "$deploy_root/compose.acceptance.yaml" --profile bootstrap run --rm acceptance-bootstrap

postgres_container=$("${compose_cmd[@]}" ps -q acceptance-postgres)
[[ -n "$postgres_container" ]] || fail 'acceptance postgres is missing after bootstrap'
settings_values=$(docker exec "$postgres_container" sh -ec '
  psql --username="$POSTGRES_USER" --dbname="$POSTGRES_DB" --tuples-only --no-align \
    --command="SELECT key || chr(61) || value FROM settings WHERE key IN (chr(98)||chr(97)||chr(99)||chr(107)||chr(101)||chr(110)||chr(100)||chr(95)||chr(109)||chr(111)||chr(100)||chr(101)||chr(95)||chr(101)||chr(110)||chr(97)||chr(98)||chr(108)||chr(101)||chr(100), chr(114)||chr(101)||chr(103)||chr(105)||chr(115)||chr(116)||chr(114)||chr(97)||chr(116)||chr(105)||chr(111)||chr(110)||chr(95)||chr(101)||chr(110)||chr(97)||chr(98)||chr(108)||chr(101)||chr(100)) ORDER BY key;"
')
[[ "$settings_values" == *'backend_mode_enabled=true'* ]] || fail 'backend_mode_enabled bootstrap verification failed'
[[ "$settings_values" == *'registration_enabled=false'* ]] || fail 'registration_enabled bootstrap verification failed'

for service in acceptance-api acceptance-worker acceptance-detector acceptance-postgres acceptance-redis acceptance-caddy; do
  container=$("${compose_cmd[@]}" ps -q "$service")
  [[ -n "$container" ]] || fail "service is missing: $service"
  [[ "$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$container")" == healthy ]] \
    || fail "service is not healthy: $service"
done

curl --fail --silent --show-error "http://127.0.0.1:$loopback_port/admin/lab/health" >/dev/null
curl --fail --silent --show-error "http://127.0.0.1:$loopback_port/admin/lab/auth/login" >/dev/null

deployment_started=false
printf '{"result":"succeeded","downtime_required":false,"source_commit":"%s","source_tree":"%s","image_sha256":"%s","services":6}\n' \
  "$source_commit" "$source_tree" "$expected_sha"
