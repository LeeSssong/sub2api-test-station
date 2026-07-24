#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'Sub2API release failed: %s\n' "$1" >&2
  exit 1
}

canonical_directory() {
  [[ "$1" == /* ]] || fail "path is not absolute: $1"
  [[ -d "$1" && ! -L "$1" ]] || fail "path is not a non-symlink directory: $1"
  (cd "$1" && pwd -P)
}

canonical_file() {
  [[ "$1" == /* ]] || fail "path is not absolute: $1"
  [[ -f "$1" && -r "$1" && ! -L "$1" ]] || fail "path is not a readable non-symlink file: $1"
  printf '%s/%s\n' "$(cd "$(dirname "$1")" && pwd -P)" "$(basename "$1")"
}

trace() {
  [[ -n "${RELEASE_EVENT_LOG:-}" ]] || return 0
  printf '%s\n' "$1" >>"$RELEASE_EVENT_LOG"
}

mode=
while (($#)); do
  case "$1" in
    --mode)
      (($# >= 2)) || fail '--mode requires rehearsal or production'
      mode=$2
      shift 2
      ;;
    *) fail "unknown argument: $1" ;;
  esac
done
[[ "$mode" == rehearsal || "$mode" == production ]] || fail '--mode must be rehearsal or production'

command -v docker >/dev/null || fail 'docker is required'
command -v jq >/dev/null || fail 'jq is required'
command -v df >/dev/null || fail 'df is required'

deploy_root=$(canonical_directory "${DEPLOY_ROOT:?DEPLOY_ROOT is required}")
base_compose=$(canonical_file "${BASE_COMPOSE:?BASE_COMPOSE is required}")
image_overlay=$(canonical_file "${IMAGE_OVERLAY:?IMAGE_OVERLAY is required}")
release_env=$(canonical_file "${RELEASE_ENV:?RELEASE_ENV is required}")
secret_env=$(canonical_file "${SECRET_ENV:?SECRET_ENV is required}")
sub2api_data=$(canonical_directory "${SUB2API_DATA_DIR:?SUB2API_DATA_DIR is required}")
postgres_data=$(canonical_directory "${POSTGRES_DATA_DIR:?POSTGRES_DATA_DIR is required}")
redis_data=$(canonical_directory "${REDIS_DATA_DIR:?REDIS_DATA_DIR is required}")
admin_key_file=$(canonical_file "${ADMIN_API_KEY_FILE:?ADMIN_API_KEY_FILE is required}")
gateway_key_file=$(canonical_file "${GATEWAY_API_KEY_FILE:?GATEWAY_API_KEY_FILE is required}")
backup_root=$(canonical_directory "${BACKUP_ROOT:?BACKUP_ROOT is required}")
record_root_input=${RELEASE_RECORD_ROOT:?RELEASE_RECORD_ROOT is required}
[[ "$record_root_input" == /* ]] || fail 'RELEASE_RECORD_ROOT must be absolute'
[[ ! -L "$record_root_input" ]] || fail 'RELEASE_RECORD_ROOT must not be a symlink'
mkdir -p "$record_root_input"
record_root=$(canonical_directory "$record_root_input")

script_root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
baseline_script=$(canonical_file "${BASELINE_SCRIPT:-$script_root/capture-sub2api-runtime-baseline.sh}")
backup_script=$(canonical_file "${BACKUP_SCRIPT:-$script_root/backup-sub2api-release.sh}")
smoke_script=$(canonical_file "${SMOKE_SCRIPT:-$script_root/smoke-sub2api-release.sh}")
base_url=${BASE_URL:?BASE_URL is required}
previous_expected_version=${PREVIOUS_EXPECTED_VERSION:-0.1.164}
[[ "$previous_expected_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z]+)*$ ]] \
  || fail 'PREVIOUS_EXPECTED_VERSION must be an application version such as 0.1.164'

if ! requested_image=$(awk '
  /^[[:space:]]*SUB2API_IMAGE[[:space:]]*=/ {
    count++
    value = $0
    sub(/^[[:space:]]*SUB2API_IMAGE[[:space:]]*=[[:space:]]*/, "", value)
    sub(/[[:space:]]+$/, "", value)
  }
  END {
    if (count != 1) exit 1
    print value
  }
' "$release_env"); then
  fail 'RELEASE_ENV must contain exactly one SUB2API_IMAGE assignment'
fi
[[ "$requested_image" =~ ^[^[:space:]@]+:[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]] \
  || fail 'SUB2API_IMAGE in RELEASE_ENV must be an exact image digest'
requested_digest=${requested_image##*@}
requested_ref=${requested_image%%@*}
requested_version=${requested_ref##*:}
[[ -n "$requested_version" ]] || fail 'requested image version is empty'

if [[ "$mode" == production ]]; then
  compose_project=${COMPOSE_PROJECT_NAME:-sub2api}
  [[ "$compose_project" == sub2api ]] \
    || fail 'production COMPOSE_PROJECT_NAME must be sub2api'
  [[ "$(uname -s)" == Linux ]] \
    || fail 'production releases must run on the Linux production host'
  [[ -z "${DOCKER_HOST:-}" ]] \
    || fail 'production releases must not use DOCKER_HOST'
  [[ "${DOCKER_CONTEXT:-default}" == default ]] \
    || fail 'production DOCKER_CONTEXT must be default'
  [[ "$(docker context show)" == default ]] \
    || fail 'production Docker context must be default'
else
  compose_project=${COMPOSE_PROJECT_NAME:-sub2api-official-rehearsal}
  [[ "$compose_project" == sub2api-official-rehearsal ]] \
    || fail 'rehearsal COMPOSE_PROJECT_NAME must be sub2api-official-rehearsal'
fi

if [[ "$mode" == production ]]; then
  [[ "${PRODUCTION_CONFIRMATION:-}" == "$requested_digest" ]] \
    || fail 'PRODUCTION_CONFIRMATION must equal the requested sha256 digest exactly'
  rehearsal_record=$(canonical_file "${REHEARSAL_RECORD:?REHEARSAL_RECORD is required for production}")
  rehearsal_mode=$(stat -f '%Lp' "$rehearsal_record" 2>/dev/null || stat -c '%a' "$rehearsal_record" 2>/dev/null) \
    || fail 'could not read REHEARSAL_RECORD mode'
  [[ "$rehearsal_mode" == 600 ]] || fail 'REHEARSAL_RECORD mode must be 0600'
  jq -e --arg image "$requested_image" --arg digest "$requested_digest" --arg version "$requested_version" '
    type == "object" and
    (keys | sort) == ["backup", "checks", "mode", "previous", "requested", "schema_version", "state"] and
    (.schema_version | type == "number" and floor == .) and .schema_version == 1 and
    (.mode | type == "string") and .mode == "rehearsal" and
    (.state | type == "string") and .state == "promoted" and
    (.previous | type == "object" and (keys | sort) == ["image", "image_id"] and
      (.image | type == "string" and length > 0) and
      (.image_id | type == "string" and test("^sha256:[0-9a-f]{64}$"))) and
    (.requested | type == "object" and (keys | sort) == ["digest", "image", "version"] and
      (.image | type == "string") and .image == $image and
      (.digest | type == "string") and .digest == $digest and
      (.version | type == "string") and .version == $version) and
    (.backup | type == "object" and (keys | sort) == ["path", "sha256_verified"] and
      (.path | type == "string" and startswith("/") and length > 1) and
      (.sha256_verified | type == "boolean") and .sha256_verified == true) and
    (.checks | type == "object" and
      (keys | sort) == ["gateway", "guard", "health", "records", "storage_identity", "support", "version"] and
      all(.[]; type == "boolean" and . == true))
  ' "$rehearsal_record" >/dev/null \
    || fail 'REHEARSAL_RECORD is not complete promoted rehearsal evidence for the requested digest'
fi

partial_record=$(find "$record_root" -maxdepth 1 -name '*.partial' -print -quit)
if [[ -e "$record_root/.release.lock" || -n "$partial_record" ]]; then
  fail 'a partial release lock or record is present'
fi
if ! mkdir "$record_root/.release.lock" 2>/dev/null; then
  fail 'another release is in progress'
fi

lock_owned=true
record_written=false
mutation_started=false
previous_image=''
previous_image_id=''
backup_path=''
backup_verified=false
storage_identity=false
health=false
version=false
records=false
support=false
guard=false
gateway=false
record_path="$record_root/$(date -u +%Y%m%dT%H%M%SZ)-$mode-$$.json"

write_record() {
  local state=$1 temporary
  [[ "$record_written" == false ]] || return 1
  temporary="$record_root/.${record_path##*/}.partial"
  umask 077
  jq -n \
    --arg mode "$mode" \
    --arg previous_image "$previous_image" \
    --arg previous_image_id "$previous_image_id" \
    --arg requested_image "$requested_image" \
    --arg requested_digest "$requested_digest" \
    --arg requested_version "$requested_version" \
    --arg backup_path "$backup_path" \
    --arg state "$state" \
    --argjson backup_verified "$backup_verified" \
    --argjson storage_identity "$storage_identity" \
    --argjson health "$health" \
    --argjson version "$version" \
    --argjson records "$records" \
    --argjson support "$support" \
    --argjson guard "$guard" \
    --argjson gateway "$gateway" \
    '{
      schema_version: 1,
      mode: $mode,
      previous: {image: $previous_image, image_id: $previous_image_id},
      requested: {image: $requested_image, digest: $requested_digest, version: $requested_version},
      backup: {path: $backup_path, sha256_verified: $backup_verified},
      checks: {
        storage_identity: $storage_identity, health: $health, version: $version,
        records: $records, support: $support, guard: $guard, gateway: $gateway
      },
      state: $state
    }' >"$temporary"
  chmod 0600 "$temporary"
  mv "$temporary" "$record_path"
  record_written=true
}

on_exit() {
  local exit_status=$? fallback_state
  trap - EXIT HUP INT TERM
  set +e
  if [[ "$lock_owned" == true && "$record_written" == false ]]; then
    if [[ "$mutation_started" == true && "${ROLLBACK_COMPATIBLE:-false}" == true ]] && attempt_rollback; then
      fallback_state=rolled_back
      write_record "$fallback_state"
      trace rolled_back
    elif [[ "$mutation_started" == true ]]; then
      fallback_state=rollback_failed
      write_record "$fallback_state"
    else
      fallback_state=preflight_failed
      write_record "$fallback_state"
    fi
    [[ "$exit_status" -ne 0 ]] || exit_status=1
  fi
  if [[ "$lock_owned" == true ]]; then
    rmdir "$record_root/.release.lock" 2>/dev/null
  fi
  exit "$exit_status"
}
trap on_exit EXIT
trap 'exit 130' HUP INT TERM

preflight_failed() {
  write_record preflight_failed
  fail "$1"
}

require_free_space() {
  local path=$1 available
  available=$(df -Pk "$path" | awk 'NR == 2 {print $4}')
  [[ "$available" =~ ^[0-9]+$ && "$available" -ge 2097152 ]] \
    || preflight_failed "less than 2 GiB free at $path"
}

compose=(docker compose --project-name "$compose_project" --project-directory "$deploy_root"
  --env-file "$secret_env" --env-file "$release_env"
  -f "$base_compose" -f "$image_overlay")
expected_config_files="$base_compose,$image_overlay"

resolve_container_id() {
  local service=$1 ids count
  if ! ids=$("${compose[@]}" ps -q "$service"); then
    return 1
  fi
  count=$(awk 'NF {count++} END {print count + 0}' <<<"$ids")
  [[ "$count" == 1 ]] || return 1
  printf '%s\n' "$ids"
}

sub2api_container=$(resolve_container_id sub2api) \
  || preflight_failed 'Compose must resolve exactly one sub2api container'
postgres_container=$(resolve_container_id postgres) \
  || preflight_failed 'Compose must resolve exactly one postgres container'
redis_container=$(resolve_container_id redis) \
  || preflight_failed 'Compose must resolve exactly one redis container'
health_timeout=${HEALTH_TIMEOUT_SECONDS:-180}
[[ "$health_timeout" =~ ^[0-9]+$ && "$health_timeout" -le 180 ]] \
  || preflight_failed 'HEALTH_TIMEOUT_SECONDS must be an integer no greater than 180'

wait_for_image_health() {
  local expected_id=$1 deadline current_sub2api inspected_sub2api actual_id health_status runtime_root runtime_configs
  deadline=$((SECONDS + health_timeout))
  while ((SECONDS <= deadline)); do
    current_sub2api=$(resolve_container_id sub2api 2>/dev/null || true)
    if [[ -n "$current_sub2api" ]] && inspected_sub2api=$(docker inspect "$current_sub2api" 2>/dev/null); then
      actual_id=$(jq -er '.[0].Image' <<<"$inspected_sub2api" 2>/dev/null || true)
      health_status=$(jq -er '.[0].State.Health.Status // empty' <<<"$inspected_sub2api" 2>/dev/null || true)
      runtime_root=$(jq -er '.[0].Config.Labels["com.docker.compose.project.working_dir"] // empty' \
        <<<"$inspected_sub2api" 2>/dev/null || true)
      runtime_configs=$(jq -er '.[0].Config.Labels["com.docker.compose.project.config_files"] // empty' \
        <<<"$inspected_sub2api" 2>/dev/null || true)
      if [[ "$actual_id" == "$expected_id" && "$health_status" == healthy &&
        "$runtime_root" == "$deploy_root" && "$runtime_configs" == "$expected_config_files" ]]; then
        return 0
      fi
    fi
    ((SECONDS <= deadline)) || break
    sleep 2
  done
  return 1
}

run_smoke() {
  local image=$1 expected_version=$2
  SUB2API_IMAGE="$image" EXPECTED_VERSION="$expected_version" BASE_URL="$base_url" \
    ADMIN_API_KEY_FILE="$admin_key_file" GATEWAY_API_KEY_FILE="$gateway_key_file" \
    EXPECTED_RECORD_COUNTS_FILE="$backup_path/record-counts.json" COMPOSE_FILE="$base_compose" \
    SUB2API_BACKUP_ROOT="$backup_root" DEPLOY_ROOT="$deploy_root" SECRET_ENV="$secret_env" \
    RELEASE_ENV="$release_env" BASE_COMPOSE="$base_compose" IMAGE_OVERLAY="$image_overlay" \
    COMPOSE_PROJECT_NAME="$compose_project" \
    "$smoke_script"
}

rollback_after_mutation() {
  local reason=$1
  if [[ "${ROLLBACK_COMPATIBLE:-false}" != true ]]; then
    write_record rollback_failed
    fail "$reason; ROLLBACK_COMPATIBLE=true was not supplied"
  fi
  if ! attempt_rollback; then
    write_record rollback_failed
    fail "$reason; the previous image could not be restored and validated"
  fi
  write_record rolled_back
  trace rolled_back
  printf 'Sub2API release rolled back to %s\n' "$previous_image"
  exit 0
}

attempt_rollback() {
  if ! SUB2API_IMAGE="$previous_image" "${compose[@]}" up -d --no-deps --force-recreate sub2api; then
    return 1
  fi
  if ! wait_for_image_health "$previous_image_id"; then
    return 1
  fi
  if ! run_smoke "$previous_image" "$previous_expected_version"; then
    return 1
  fi
  return 0
}

if ! inspected=$(docker inspect "$sub2api_container" "$postgres_container" "$redis_container"); then
  preflight_failed 'could not inspect the running Compose services'
fi
if ! jq -e --arg project "$compose_project" --arg root "$deploy_root" \
  --arg base "$base_compose" --arg full "$expected_config_files" '
  length == 3 and
  all(.[]; .Config.Labels["com.docker.compose.project"] == $project) and
  all(.[];
    .Config.Labels["com.docker.compose.project.working_dir"] == $root and
    (.Config.Labels["com.docker.compose.project.config_files"] == $base or
      .Config.Labels["com.docker.compose.project.config_files"] == $full)) and
  .[1].State.Health.Status == "healthy" and .[2].State.Health.Status == "healthy"
' >/dev/null <<<"$inspected"; then
  preflight_failed 'running services do not have the expected project identity and dependency health'
fi
previous_image=$(jq -er '.[0].Config.Image' <<<"$inspected") || preflight_failed 'could not identify the current image'
previous_image_id=$(jq -er '.[0].Image' <<<"$inspected") || preflight_failed 'could not identify the current image ID'

if ! resolved_config=$("${compose[@]}" config --format json); then
  preflight_failed 'Compose configuration could not be resolved'
fi
if ! jq -e --arg app "$sub2api_data" --arg postgres "$postgres_data" --arg redis "$redis_data" '
  def expected_bind($service; $target; $source):
    any(.services[$service].volumes[]?;
      .type == "bind" and .target == $target and .source == $source);
  expected_bind("sub2api"; "/app/data"; $app) and
  expected_bind("postgres"; "/var/lib/postgresql/data"; $postgres) and
  expected_bind("redis"; "/data"; $redis)
' >/dev/null <<<"$resolved_config"; then
  preflight_failed 'Compose storage identity does not match the canonical data paths'
fi
storage_identity=true
require_free_space "$backup_root"
require_free_space "$sub2api_data"
require_free_space "$postgres_data"
require_free_space "$redis_data"

if ! baseline=$(EXPECTED_PROJECT="$compose_project" EXPECTED_IMAGE_ID="$previous_image_id" \
  EXPECTED_SUB2API_DATA="$sub2api_data" EXPECTED_POSTGRES_DATA="$postgres_data" \
  EXPECTED_REDIS_DATA="$redis_data" SUB2API_CONTAINER="$sub2api_container" \
  POSTGRES_CONTAINER="$postgres_container" REDIS_CONTAINER="$redis_container" "$baseline_script"); then
  preflight_failed 'runtime baseline capture failed'
fi
if ! jq -e --arg image "$previous_image" --arg image_id "$previous_image_id" \
  --arg root "$deploy_root" --arg base "$base_compose" --arg full "$expected_config_files" '
    .image == $image and .image_id == $image_id and .working_dir == $root and
    (.config_files == $base or .config_files == $full)
  ' >/dev/null <<<"$baseline"; then
  preflight_failed 'runtime baseline does not match the inspected image'
fi

if ! backup_output=$(SUB2API_PRODUCTION_ROOT="$deploy_root" SUB2API_BACKUP_ROOT="$backup_root" \
  SUB2API_DATA_DIR="$sub2api_data" DEPLOY_ROOT="$deploy_root" SECRET_ENV="$secret_env" \
  RELEASE_ENV="$release_env" BASE_COMPOSE="$base_compose" IMAGE_OVERLAY="$image_overlay" \
  COMPOSE_PROJECT_NAME="$compose_project" "$backup_script"); then
  preflight_failed 'release backup failed'
fi
backup_path=${backup_output##*release_backup_set=}
[[ "$backup_path" != "$backup_output" && "$backup_path" != *$'\n'* ]] \
  || preflight_failed 'release backup did not report its promoted set'
backup_path=$(canonical_directory "$backup_path")
[[ -r "$backup_path/SHA256SUMS" ]] || preflight_failed 'release backup has no SHA256SUMS manifest'
if command -v sha256sum >/dev/null; then
  (cd "$backup_path" && sha256sum --check SHA256SUMS >/dev/null) \
    || preflight_failed 'release backup checksum verification failed'
else
  (cd "$backup_path" && shasum -a 256 -c SHA256SUMS >/dev/null) \
    || preflight_failed 'release backup checksum verification failed'
fi
backup_verified=true

if ! SUB2API_IMAGE="$requested_image" "${compose[@]}" pull sub2api; then
  preflight_failed 'could not pull the requested image'
fi
if ! requested_image_id=$(docker image inspect "$requested_image" | jq -er --arg digest "$requested_digest" '
  .[0] | select(any(.RepoDigests[]?; endswith($digest))) | .Id
'); then
  preflight_failed 'requested digest did not resolve to a local image ID'
fi

mutation_started=true
if ! SUB2API_IMAGE="$requested_image" "${compose[@]}" up -d --no-deps --force-recreate sub2api; then
  rollback_after_mutation 'requested-image recreation failed'
fi
if ! wait_for_image_health "$requested_image_id"; then
  rollback_after_mutation 'requested image did not become healthy within 180 seconds'
fi
health=true

if ! run_smoke "$requested_image" "$requested_version"; then
  rollback_after_mutation 'requested image failed smoke checks'
fi
version=true
records=true
support=true
guard=true
gateway=true
write_record promoted
trace promoted
printf 'Sub2API %s promoted\n' "$requested_image"
