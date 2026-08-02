#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'billing_provision_host status=failed: %s\n' "$1" >&2; exit 1; }
mode_of() { stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"; }
owner_of() { stat -c '%u' "$1" 2>/dev/null || stat -f '%u' "$1"; }
canonical_file() {
  local path=$1 label=$2 dir
  [[ "$path" == /* && -f "$path" && ! -L "$path" ]] || fail "$label is invalid"
  dir=$(dirname "$path")
  [[ -d "$dir" && ! -L "$dir" ]] || fail "$label parent is invalid"
  [[ "$(cd "$dir" && pwd -P)/$(basename "$path")" == "$path" ]] || fail "$label must be canonical"
  [[ "$(owner_of "$path")" == 0 ]] || fail "$label must be root-owned"
  [[ "$(mode_of "$path")" == 600 ]] || fail "$label must be 0600"
}
canonical_directory() {
  local path=$1 label=$2 mode
  [[ "$path" == /* && -d "$path" && ! -L "$path" ]] || fail "$label is invalid"
  [[ "$(cd "$path" && pwd -P)" == "$path" ]] || fail "$label must be canonical"
  [[ "$(owner_of "$path")" == 0 ]] || fail "$label must be root-owned"
  mode=$(mode_of "$path")
  (( (8#$mode & 8#022) == 0 )) || fail "$label must not be group/other writable"
}

[[ "$(uname -s)" == Linux ]] || fail 'host must be Linux'
[[ "$(id -u)" == 0 ]] || fail 'host wrapper must run as root'
[[ -z "${DOCKER_HOST:-}" && "${DOCKER_CONTEXT:-default}" == default ]] || fail 'Docker context must be local default'

image=''; declaration=''
while (($#)); do
  case "$1" in
    --image) (($# >= 2)) || fail '--image requires a value'; image=$2; shift 2 ;;
    --declaration) (($# >= 2)) || fail '--declaration requires a value'; declaration=$2; shift 2 ;;
    *) fail "unknown argument: $1" ;;
  esac
done
[[ "$image" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]] || fail '--image must be an immutable digest'
[[ -n "$declaration" ]] || fail '--declaration is required'

docker_bin=${DOCKER_BIN:-docker}
command -v "$docker_bin" >/dev/null 2>&1 || fail 'Docker is required'
[[ "$("$docker_bin" context show 2>/dev/null)" == default ]] || fail 'Docker context must be default'

db_url_file=${RELAY_OPS_BILLING_DB_URL_FILE:-/opt/sub2api/production/secrets/relay-ops-database-url}
admin_key_file=${RELAY_OPS_BILLING_ADMIN_KEY_FILE:-/opt/sub2api/production/secrets/sub2api-admin-api-key}
sessions_dir=${RELAY_OPS_BILLING_SESSIONS_DIR:-/opt/sub2api/production/secrets/upstream-sessions}
network_name=${RELAY_OPS_BILLING_NETWORK:-sub2api_default}
[[ "$network_name" =~ ^[a-z0-9][a-z0-9_.-]*$ ]] || fail 'network name is invalid'
canonical_file "$declaration" BILLING_DECLARATION
canonical_file "$db_url_file" DATABASE_URL_FILE
canonical_file "$admin_key_file" ADMIN_API_KEY_FILE
canonical_directory "$sessions_dir" UPSTREAM_SESSIONS_DIR

image_id=$("$docker_bin" image inspect "$image" --format '{{.Id}}' 2>/dev/null | tr -d '[:space:]') || fail 'immutable billing image is not locally available'
[[ "$image_id" =~ ^sha256:[a-f0-9]{64}$ ]] || fail 'billing image identity is invalid'

"$docker_bin" run --rm --pull never --network "$network_name" \
  --entrypoint /provision-billing-source \
  --user 0:0 --read-only --tmpfs /tmp:rw,noexec,nosuid,nodev \
  --security-opt no-new-privileges:true --cap-drop ALL \
  --memory 192m --cpus 0.25 \
  -e RELAY_OPS_MODE=closed \
  -e RELAY_OPS_TIMEZONE=Asia/Shanghai \
  -e RELAY_OPS_DATABASE_URL_FILE=/run/secrets/relay-ops-database-url \
  -e RELAY_OPS_SUB2API_ADMIN_KEY_FILE=/run/secrets/sub2api-admin-api-key \
  -v "$db_url_file:/run/secrets/relay-ops-database-url:ro" \
  -v "$admin_key_file:/run/secrets/sub2api-admin-api-key:ro" \
  -v "$sessions_dir:/run/secrets/upstream-sessions:ro" \
  -v "$declaration:/run/secrets/billing-source-declaration.json:ro" \
  "$image"
