#!/usr/bin/env bash
set -euo pipefail

umask 077

readonly HOST_CONTRACT_VERSION=1
readonly APPROVED_IMAGE_REPOSITORY=ghcr.io/leesssong/xingqiao-sub2api

fail() {
  printf 'host update failed: %s\n' "$1" >&2
  exit 1
}

trace() {
  [[ -n "${RELEASE_EVENT_LOG:-}" ]] || return 0
  printf '%s\n' "$1" >>"$RELEASE_EVENT_LOG"
}

canonical_directory() {
  local value=$1 label=$2 physical
  [[ "$value" == /* && -d "$value" && ! -L "$value" ]] || fail "$label must be an absolute non-symlink directory"
  physical=$(cd "$value" && pwd -P)
  [[ "$physical" == "$value" ]] || fail "$label must be canonical"
  printf '%s\n' "$value"
}

canonical_file() {
  local value=$1 label=$2 parent physical canonical
  [[ "$value" == /* && -f "$value" && -r "$value" && ! -L "$value" ]] || fail "$label must be an absolute readable non-symlink file"
  parent=$(dirname "$value")
  physical=$(cd "$parent" && pwd -P)
  canonical="$physical/$(basename "$value")"
  [[ "$canonical" == "$value" ]] || fail "$label must be canonical"
  printf '%s\n' "$value"
}

mode_of() {
  stat -f '%Lp' "$1" 2>/dev/null || stat -c '%a' "$1"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

managed_env_value() {
  local key=$1 count
  count=$(awk -F= -v key="$key" '$1 == key { count++ } END { print count + 0 }' "$release_env")
  [[ "$count" == 1 ]] || fail "release.env must contain exactly one $key assignment"
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$release_env"
}

requested_image=''
requested_version=''
operation_id=''
contract_version=''
while (($#)); do
  case "$1" in
    --image) (($# >= 2)) || fail '--image requires a value'; [[ -z "$requested_image" ]] || fail '--image may be supplied once'; requested_image=$2; shift 2 ;;
    --version) (($# >= 2)) || fail '--version requires a value'; [[ -z "$requested_version" ]] || fail '--version may be supplied once'; requested_version=$2; shift 2 ;;
    --operation-id) (($# >= 2)) || fail '--operation-id requires a value'; [[ -z "$operation_id" ]] || fail '--operation-id may be supplied once'; operation_id=$2; shift 2 ;;
    --contract-version) (($# >= 2)) || fail '--contract-version requires a value'; [[ -z "$contract_version" ]] || fail '--contract-version may be supplied once'; contract_version=$2; shift 2 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ "$contract_version" == "$HOST_CONTRACT_VERSION" ]] \
  || fail "updater contract ${contract_version:-none} does not match executor contract $HOST_CONTRACT_VERSION; reinstall the updater with ops/install-sub2api-updater.sh"
[[ "$requested_image" =~ ^sha256:[a-f0-9]{64}$ ]] || fail '--image must be one local sha256 image ID'
[[ "$requested_version" =~ ^[0-9]+([.][0-9]+){1,2}$ ]] || fail '--version is invalid'
[[ "$operation_id" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ]] || fail '--operation-id is invalid'

for command in docker jq awk date stat mktemp chmod mv dirname basename; do
  command -v "$command" >/dev/null 2>&1 || fail "$command is required"
done
command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 || fail 'sha256sum or shasum is required'

executor=$(canonical_file "${SUB2API_BLUE_GREEN_EXECUTOR:-/usr/local/libexec/deploy-sub2api-blue-green-host.sh}" 'blue-green executor')
[[ -x "$executor" ]] || fail 'blue-green executor must be executable'
deploy_root=$(canonical_directory "${SUB2API_PRODUCTION_ROOT:-/opt/sub2api/production}" 'production root')
base_compose=$(canonical_file "${SUB2API_COMPOSE_FILE:-$deploy_root/compose.yaml}" 'production Compose file')
secret_env=$(canonical_file "${SUB2API_ENV_FILE:-$deploy_root/.env}" 'production secret environment')
release_env=$(canonical_file "${SUB2API_RELEASE_ENV_FILE:-$deploy_root/release.env}" 'blue-green release environment')
release_state=$(canonical_file "${SUB2API_RELEASE_STATE:-/var/lib/sub2api/release-state}" 'blue-green release state')
record_root=$(canonical_directory "${SUB2API_RELEASE_RECORD_ROOT:-/var/lib/sub2api/release-records}" 'release record root')
admin_key_file=$(canonical_file "${SUB2API_ADMIN_API_KEY_FILE:-$deploy_root/secrets/sub2api-admin-api-key}" 'admin API key file')
gateway_key_file=$(canonical_file "${SUB2API_GATEWAY_API_KEY_FILE:-$deploy_root/secrets/sub2api-gateway-api-key}" 'gateway API key file')
staging_root=$(canonical_directory "${SUB2API_RELEASE_STAGING_ROOT:-/var/lib/sub2api/release-staging}" 'release staging root')
base_url=${SUB2API_BASE_URL:-https://api.xingqiaolab.top}
network_curl_image=${SUB2API_NETWORK_CURL_IMAGE:-}
network_curl_allowlist=${SUB2API_NETWORK_CURL_IMAGE_ALLOWLIST:-}
deadline_seconds=${SUB2API_RELEASE_DEADLINE_SECONDS:-1800}

[[ "$base_url" == https://* ]] || fail 'SUB2API_BASE_URL must use HTTPS'
[[ "$network_curl_image" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]] || fail 'SUB2API_NETWORK_CURL_IMAGE must be immutable'
network_curl_approved=false
for approved in $network_curl_allowlist; do
  [[ "$approved" == "$network_curl_image" ]] && network_curl_approved=true
done
[[ "$network_curl_approved" == true ]] || fail 'SUB2API_NETWORK_CURL_IMAGE is not allowlisted'
[[ "$deadline_seconds" =~ ^[0-9]+$ && "$deadline_seconds" -ge 60 && "$deadline_seconds" -le 1800 ]] \
  || fail 'SUB2API_RELEASE_DEADLINE_SECONDS must be between 60 and 1800'
[[ "$(mode_of "$release_env")" == 600 ]] || fail 'blue-green release environment mode must be 0600'
[[ "$(mode_of "$release_state")" == 600 ]] || fail 'blue-green release state mode must be 0600'

jq -e '
  type == "object" and
  (.schema_version == 1 or .schema_version == 2) and
  ((.active_slot == "blue" and .active_upstream == "sub2api-blue:8080") or
   (.active_slot == "green" and .active_upstream == "sub2api-green:8080")) and
  ([.blue_image,.green_image,.worker_image] | all(type == "string" and length > 0)) and
  (.source_commit | type == "string" and test("^[a-f0-9]{40}$")) and
  (.source_tree | type == "string" and test("^[a-f0-9]{40}$")) and
  (.migrations_hash | type == "string" and test("^[a-f0-9]{64}$")) and
  ([.postgres_id,.redis_id,.caddy_id] | all(type == "string" and length > 0))
' "$release_state" >/dev/null || fail 'blue-green release state is invalid or inconsistent'

state_blue=$(jq -r '.blue_image' "$release_state")
state_green=$(jq -r '.green_image' "$release_state")
state_worker=$(jq -r '.worker_image' "$release_state")
state_upstream=$(jq -r '.active_upstream' "$release_state")
state_slot=$(jq -r '.active_slot' "$release_state")
state_previous=green
[[ "$state_slot" == green ]] && state_previous=blue
[[ "$(managed_env_value SUB2API_BLUE_IMAGE)" == "$state_blue" ]] || fail 'release.env blue image does not match release state'
[[ "$(managed_env_value SUB2API_GREEN_IMAGE)" == "$state_green" ]] || fail 'release.env green image does not match release state'
[[ "$(managed_env_value SUB2API_WORKER_IMAGE)" == "$state_worker" ]] || fail 'release.env worker image does not match release state'
[[ "$(managed_env_value SUB2API_ACTIVE_UPSTREAM)" == "$state_upstream" ]] || fail 'release.env active upstream does not match release state'
[[ "$(managed_env_value SUB2API_ACTIVE_SLOT)" == "$state_slot" ]] || fail 'release.env active slot does not match release state'
[[ "$(managed_env_value SUB2API_PREVIOUS_SLOT)" == "$state_previous" ]] || fail 'release.env previous slot does not match release state'

trace 'adapter inspect qualified image'
image_json=$(docker image inspect "$requested_image") || fail 'requested local image could not be inspected'
jq -e --arg id "$requested_image" --arg version "$requested_version" '
  length == 1 and .[0].Id == $id and .[0].Os == "linux" and .[0].Architecture == "amd64" and
  .[0].Config.Labels["com.xingqiao.sub2api.qualified"] == "true" and
  .[0].Config.Labels["com.xingqiao.sub2api.upstream.version"] == $version and
  (.[0].Config.Labels["com.xingqiao.sub2api.upstream.commit"] | type == "string" and test("^[a-f0-9]{40}$")) and
  (.[0].Config.Labels["com.xingqiao.sub2api.source.commit"] | type == "string" and test("^[a-f0-9]{40}$")) and
  (.[0].Config.Labels["com.xingqiao.sub2api.source.tree"] | type == "string" and test("^[a-f0-9]{40}$")) and
  (.[0].Config.Labels["com.xingqiao.sub2api.tested.tree"] | type == "string" and test("^[a-f0-9]{40}$")) and
  (.[0].Config.Labels["com.xingqiao.sub2api.migrations.sha256"] | type == "string" and test("^[a-f0-9]{64}$")) and
  .[0].Config.Labels["com.xingqiao.sub2api.source.tree"] == .[0].Config.Labels["com.xingqiao.sub2api.tested.tree"]
' <<<"$image_json" >/dev/null || fail 'requested image qualification labels are invalid'

repo_digest=$(jq -er --arg repository "$APPROVED_IMAGE_REPOSITORY" '
  .[0].RepoDigests as $digests |
  select(($digests | type) == "array" and ($digests | length) == 1) |
  $digests[0] |
  select(startswith($repository + "@sha256:") and test("@sha256:[a-f0-9]{64}$"))
' <<<"$image_json") || fail 'requested image must have exactly one approved GHCR RepoDigest'

source_commit=$(jq -r '.[0].Config.Labels["com.xingqiao.sub2api.source.commit"]' <<<"$image_json")
source_tree=$(jq -r '.[0].Config.Labels["com.xingqiao.sub2api.source.tree"]' <<<"$image_json")
tested_tree=$(jq -r '.[0].Config.Labels["com.xingqiao.sub2api.tested.tree"]' <<<"$image_json")
migrations_hash=$(jq -r '.[0].Config.Labels["com.xingqiao.sub2api.migrations.sha256"]' <<<"$image_json")
release_image="${repo_digest%@sha256:*}:release-$source_commit-${requested_image#sha256:}"
archive="$staging_root/sub2api-$operation_id-$source_commit.tar"
[[ ! -e "$archive" ]] || fail 'preloaded archive already exists for this operation'
archive_partial=$(mktemp "$staging_root/.sub2api-$operation_id.XXXXXX")
cleanup_staged_archive() {
  [[ -z "${archive_partial:-}" ]] || rm -f -- "$archive_partial"
  [[ -z "${archive:-}" ]] || rm -f -- "$archive"
  return 0
}
trap cleanup_staged_archive EXIT HUP INT TERM

trace 'adapter stage preloaded image'
docker image tag "$requested_image" "$release_image" >/dev/null || fail 'could not create image-ID-bound release tag'
docker image save --output "$archive_partial" "$release_image" || fail 'could not stage the preloaded image archive'
[[ -s "$archive_partial" ]] || fail 'preloaded image archive is empty'
chmod 0600 "$archive_partial"
archive_sha256=$(sha256_file "$archive_partial")
[[ "$archive_sha256" =~ ^[a-f0-9]{64}$ ]] || fail 'preloaded image archive checksum failed'
mv "$archive_partial" "$archive"
archive_partial=''

now=$(date -u +%s) || fail 'release deadline clock failed'
[[ "$now" =~ ^[0-9]+$ ]] || fail 'release deadline clock is invalid'
deadline_epoch=$((now + deadline_seconds))

executor_environment=(
  "DEPLOY_ROOT=$deploy_root"
  "BASE_COMPOSE=$base_compose"
  "SECRET_ENV=$secret_env"
  "RELEASE_ENV=$release_env"
  "RELEASE_STATE=$release_state"
  "RELEASE_RECORD_ROOT=$record_root"
  "ADMIN_API_KEY_FILE=$admin_key_file"
  "GATEWAY_API_KEY_FILE=$gateway_key_file"
  "BASE_URL=$base_url"
  "NETWORK_CURL_IMAGE=$network_curl_image"
  "NETWORK_CURL_IMAGE_ALLOWLIST=$network_curl_allowlist"
  "RELEASE_STAGING_ROOT=$staging_root"
  'RELEASE_PRELOADED_IMAGE=true'
)
[[ -z "${RELEASE_EVENT_LOG:-}" ]] || executor_environment+=("RELEASE_EVENT_LOG=$RELEASE_EVENT_LOG")

set +e
executor_output=$(env "${executor_environment[@]}" "$executor" \
  --mode production --image "$release_image" \
  --preloaded-archive "$archive" --preloaded-archive-sha256 "$archive_sha256" --preloaded-image-id "$requested_image" \
  --source-commit "$source_commit" --source-tree "$source_tree" --tested-tree "$tested_tree" \
  --migrations-hash "$migrations_hash" --deadline-epoch "$deadline_epoch")
executor_status=$?
set -e
if ((executor_status != 0)); then
  [[ -z "$executor_output" ]] || printf '%s\n' "$executor_output"
  exit "$executor_status"
fi
jq -e --arg image "$release_image" '
  type == "object" and .schema_version == 1 and .downtime_required == false and
  .result == "succeeded" and .image == $image and
  ((.active_slot == "blue" and .active_upstream == "sub2api-blue:8080") or
   (.active_slot == "green" and .active_upstream == "sub2api-green:8080"))
' <<<"$executor_output" >/dev/null || fail 'blue-green executor did not emit an explicit successful terminal result'
printf 'result=promoted\n'
