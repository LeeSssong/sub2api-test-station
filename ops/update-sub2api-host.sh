#!/usr/bin/env bash
set -euo pipefail

umask 077

PINNED_POSTGRES_IMAGE='postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15'
EXPECTED_QR_SHA256='35b84b14ab472e117fa413ed5f91357becd01199eeaf3fed469a2d9d3d987c16'

fail() {
  printf 'host update failed: %s\n' "$1" >&2
  exit 1
}

trace() {
  [[ -n "${RELEASE_EVENT_LOG:-}" ]] || return 0
  printf '%s\n' "$1" >>"$RELEASE_EVENT_LOG"
}

canonical_directory() {
  local path=$1 label=$2 physical
  [[ "$path" == /* && -d "$path" && ! -L "$path" ]] || fail "$label must be an absolute non-symlink directory"
  physical=$(cd "$path" && pwd -P)
  [[ "$physical" == "$path" ]] || fail "$label must be canonical"
  printf '%s\n' "$physical"
}

canonical_file() {
  local path=$1 label=$2 parent physical_parent canonical
  [[ "$path" == /* && -f "$path" && -r "$path" && ! -L "$path" ]] || fail "$label must be an absolute readable non-symlink file"
  parent=$(dirname "$path")
  physical_parent=$(cd "$parent" && pwd -P)
  canonical="$physical_parent/$(basename "$path")"
  [[ "$canonical" == "$path" ]] || fail "$label must be canonical"
  printf '%s\n' "$path"
}

mode_of() {
  stat -f '%Lp' "$1" 2>/dev/null || stat -c '%a' "$1"
}

parse_args() {
  requested_image=''
  operation_id=''
  while (($#)); do
    case "$1" in
      --image)
        (($# >= 2)) || fail '--image requires an immutable image reference'
        [[ -z "$requested_image" ]] || fail '--image may be supplied once'
        requested_image=$2
        shift 2
        ;;
      --operation-id)
        (($# >= 2)) || fail '--operation-id requires an identifier'
        [[ -z "$operation_id" ]] || fail '--operation-id may be supplied once'
        operation_id=$2
        shift 2
        ;;
      *) fail "unknown argument: $1" ;;
    esac
  done
  [[ -n "$requested_image" ]] || fail '--image is required'
  [[ -n "$operation_id" && "$operation_id" =~ ^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$ ]] \
    || fail '--operation-id is invalid'
  [[ "$requested_image" =~ ^weishaw/sub2api:[0-9]+\.[0-9]+\.[0-9]+[0-9A-Za-z._-]*@sha256:[a-f0-9]{64}$ ]] \
    || fail '--image must be weishaw/sub2api:<version>@sha256:<digest>'
  requested_digest=${requested_image##*@}
  requested_version=${requested_image#weishaw/sub2api:}
  requested_version=${requested_version%%@*}
}

require_commands() {
  local command
  for command in docker jq curl sha256sum tar df awk date mktemp stat; do
    command -v "$command" >/dev/null 2>&1 || fail "$command is required"
  done
}

parse_args "$@"
require_commands

deploy_root=$(canonical_directory "${SUB2API_PRODUCTION_ROOT:-/opt/sub2api/production}" 'production root')
[[ "$(pwd -P)" == "$deploy_root" ]] || fail 'the current directory must be the production root'
[[ "$(uname -s)" == Linux ]] || fail 'the host must be Linux'
[[ -z "${DOCKER_HOST:-}" ]] || fail 'DOCKER_HOST must be unset'
[[ "${DOCKER_CONTEXT:-default}" == default ]] || fail 'DOCKER_CONTEXT must be default'
[[ "$(docker context show)" == default ]] || fail 'Docker context must be default'

compose_project=${SUB2API_COMPOSE_PROJECT:-sub2api}
[[ "$compose_project" == sub2api ]] || fail 'Compose project must be sub2api'
env_file=$(canonical_file "${SUB2API_ENV_FILE:-$deploy_root/.env}" 'Compose environment file')
compose_file=$(canonical_file "${SUB2API_COMPOSE_FILE:-$deploy_root/compose.yaml}" 'Compose file')

base_url=${SUB2API_BASE_URL:-}
[[ -n "$base_url" ]] || fail 'SUB2API_BASE_URL is required and must point to the Caddy entrypoint'
[[ "$base_url" == https://* ]] || fail 'SUB2API_BASE_URL must be an HTTPS Caddy entrypoint'
case "$base_url" in
  *://127.0.0.1:8080*|*://localhost:8080*|*://sub2api:8080*)
    fail 'SUB2API_BASE_URL must not bypass Caddy'
    ;;
esac

admin_key_file=$(canonical_file "${SUB2API_ADMIN_API_KEY_FILE:-${ADMIN_API_KEY_FILE:-}}" 'admin API key file')
gateway_key_file=$(canonical_file "${SUB2API_GATEWAY_API_KEY_FILE:-${GATEWAY_API_KEY_FILE:-}}" 'gateway API key file')
[[ $(wc -c <"$admin_key_file" | tr -d ' ') -le 1024 ]] || fail 'admin API key file is too large'
[[ $(wc -c <"$gateway_key_file" | tr -d ' ') -le 1024 ]] || fail 'gateway API key file is too large'
admin_key=$(tr -d '\r\n' <"$admin_key_file")
gateway_key=$(tr -d '\r\n' <"$gateway_key_file")
[[ -n "$admin_key" && -n "$gateway_key" ]] || fail 'API key files must not be empty'

record_root_input=${SUB2API_RELEASE_RECORD_ROOT:-$deploy_root/release-records/host-updater}
[[ "$record_root_input" == /* && ! -L "$record_root_input" ]] || fail 'release record root must be absolute and non-symlinked'
mkdir -p "$record_root_input"
record_root=$(canonical_directory "$record_root_input" 'release record root')
chmod 0700 "$record_root"

backup_root_input=${SUB2API_BACKUP_ROOT:-$deploy_root/backups/release}
[[ "$backup_root_input" == /* && ! -L "$backup_root_input" ]] || fail 'backup root must be absolute and non-symlinked'
mkdir -p "$backup_root_input"
backup_root=$(canonical_directory "$backup_root_input" 'backup root')
chmod 0700 "$backup_root"

app_volume=${SUB2API_DATA_VOLUME:-sub2api_sub2api_data}
postgres_volume=${SUB2API_POSTGRES_VOLUME:-sub2api_postgres_data}
redis_volume=${SUB2API_REDIS_VOLUME:-sub2api_redis_data}
[[ "$app_volume" == sub2api_sub2api_data && "$postgres_volume" == sub2api_postgres_data && "$redis_volume" == sub2api_redis_data ]] \
  || fail 'unexpected production volume names'

compose=(docker compose --project-name "$compose_project" --project-directory "$deploy_root"
  --env-file "$env_file" -f "$compose_file")

lock_dir="$record_root/.update.lock"
partial_record=$(find "$record_root" -maxdepth 1 -type f -name '*.partial' -print -quit)
[[ -z "$partial_record" && ! -e "$lock_dir" ]] || fail 'an update lock or partial record already exists'
mkdir "$lock_dir" || fail 'another host update is in progress'
lock_owned=true

record_path="$record_root/$(date -u +%Y%m%dT%H%M%SZ)-$operation_id.json"
[[ ! -e "$record_path" ]] || fail 'a release record already exists for this operation'
record_written=false
mutation_started=false
compose_changed=false
compose_backup=''
backup_path=''
backup_partial=''
previous_image=''
previous_image_id=''
requested_image_id=''
rollback_tag="sub2api-host-updater:rollback-$operation_id"
storage_identity=false
backup_verified=false
health=false
smoke=false

write_record() {
  local state=$1 temporary
  [[ "$record_written" == false ]] || return 0
  temporary="$record_root/.${operation_id}.$$.partial"
  jq -n \
    --arg operation_id "$operation_id" \
    --arg previous_image "$previous_image" \
    --arg previous_image_id "$previous_image_id" \
    --arg requested_image "$requested_image" \
    --arg requested_digest "$requested_digest" \
    --arg requested_version "$requested_version" \
    --arg rollback_tag "$rollback_tag" \
    --arg backup_path "$backup_path" \
    --arg state "$state" \
    --argjson storage_identity "$storage_identity" \
    --argjson backup_verified "$backup_verified" \
    --argjson health "$health" \
    --argjson smoke "$smoke" \
    '{
      schema_version: 1,
      operation_id: $operation_id,
      previous: {image: $previous_image, image_id: $previous_image_id, rollback_tag: $rollback_tag},
      requested: {image: $requested_image, digest: $requested_digest, version: $requested_version},
      backup: {path: $backup_path, sha256_verified: $backup_verified},
      checks: {storage_identity: $storage_identity, backup: $backup_verified, health: $health, smoke: $smoke},
      state: $state
    }' >"$temporary"
  chmod 0600 "$temporary"
  mv "$temporary" "$record_path"
  record_written=true
}

restore_compose_file() {
  [[ "$compose_changed" == true ]] || return 0
  [[ -n "$compose_backup" && -f "$compose_backup" ]] || return 1
  local temporary
  temporary=$(mktemp "$compose_file.restore.XXXXXX")
  chmod 0600 "$temporary"
  cp "$compose_backup" "$temporary"
  mv "$temporary" "$compose_file"
  compose_changed=false
}

resolve_container_id() {
  local service=$1 ids count
  ids=$("${compose[@]}" ps -q "$service") || return 1
  count=$(awk 'NF {n++} END {print n + 0}' <<<"$ids")
  [[ "$count" == 1 ]] || return 1
  awk 'NF {print; exit}' <<<"$ids"
}

inspect_runtime() {
  docker inspect "$@"
}

validate_runtime() {
  local inspected=$1
  jq -e --arg project "$compose_project" --arg root "$deploy_root" --arg config "$compose_file" \
    --arg app "$app_volume" --arg postgres "$postgres_volume" --arg redis "$redis_volume" \
    --arg previous "$previous_image" --arg previous_id "$previous_image_id" '
    length == 6 and
    all(.[]; .Config.Labels["com.docker.compose.project"] == $project) and
    .[0].Config.Labels["com.docker.compose.project.working_dir"] == $root and
    .[0].Config.Labels["com.docker.compose.project.config_files"] == $config and
    .[0].Config.Image == $previous and .[0].Image == $previous_id and
    ([.[1].State.Health.Status, .[2].State.Health.Status, .[3].State.Health.Status,
      .[4].State.Health.Status, .[5].State.Health.Status] | all(. == "healthy")) and
    ([.[0].Mounts[] | select(.Destination == "/app/data")] | length == 1 and
      .[0].Type == "volume" and (.[0].Name // .[0].Source) == $app and .[0].RW == true) and
    ([.[1].Mounts[] | select(.Destination == "/var/lib/postgresql/data")] | length == 1 and
      .[0].Type == "volume" and (.[0].Name // .[0].Source) == $postgres and .[0].RW == true) and
    ([.[2].Mounts[] | select(.Destination == "/data")] | length == 1 and
      .[0].Type == "volume" and (.[0].Name // .[0].Source) == $redis and .[0].RW == true)
  ' <<<"$inspected" >/dev/null
}

validate_compose_storage() {
  local config=$1
  jq -e --arg image "$requested_image" --arg app "$app_volume" \
    --arg postgres "$postgres_volume" --arg redis "$redis_volume" '
    .services.sub2api.image == $image and
    ([.services.sub2api.volumes[] | select(.target == "/app/data")] | length == 1 and
      .[0].type == "volume" and .[0].source == $app) and
    ([.services.postgres.volumes[] | select(.target == "/var/lib/postgresql/data")] | length == 1 and
      .[0].type == "volume" and .[0].source == $postgres) and
    ([.services.redis.volumes[] | select(.target == "/data")] | length == 1 and
      .[0].type == "volume" and .[0].source == $redis)
  ' <<<"$config" >/dev/null
}

require_free_space() {
  local path=$1 available
  available=$(df -Pk "$path" | awk 'NR == 2 {print $4}')
  [[ "$available" =~ ^[0-9]+$ && "$available" -ge 2097152 ]] || fail "less than 2 GiB free at $path"
}

validate_counts() {
  jq -e '
    type == "object" and
    (keys | sort == ["accounts", "api_keys", "groups", "settings", "usage_logs", "users"]) and
    ([.users, .accounts, .groups, .api_keys, .settings, .usage_logs] |
      all(type == "number" and . >= 0 and floor == .))
  ' "$1" >/dev/null
}

validate_counts_monotonic() {
  jq -e --slurpfile baseline "$1" '
    . as $actual | $baseline[0] as $before |
    ($actual.users >= $before.users and
      $actual.accounts >= $before.accounts and
      $actual.groups >= $before.groups and
      $actual.api_keys >= $before.api_keys and
      $actual.settings >= $before.settings and
      $actual.usage_logs >= $before.usage_logs) and
    ([ $actual.users, $actual.accounts, $actual.groups, $actual.api_keys,
       $actual.settings, $actual.usage_logs ] | all(type == "number" and floor == .))
  ' "$2" >/dev/null
}

replace_sub2api_image() {
  local temporary=$1 replaced
  temporary=$(mktemp "$compose_file.update.XXXXXX")
  if ! awk -v image="$requested_image" '
    BEGIN {services = 0; sub2api = 0; replaced = 0}
    /^services:[[:space:]]*$/ {services = 1; print; next}
    services && /^[^[:space:]]/ {services = 0; sub2api = 0}
    services && /^  sub2api:[[:space:]]*$/ {sub2api = 1; print; next}
    sub2api && /^  [^[:space:]]/ {sub2api = 0}
    sub2api && /^    image:[[:space:]]*/ && replaced == 0 {print "    image: " image; replaced = 1; next}
    {print}
    END {exit(replaced == 1 ? 0 : 1)}
  ' "$compose_file" >"$temporary"; then
    rm -f -- "$temporary"
    fail 'Compose file has no services.sub2api.image declaration'
  fi
  chmod 0600 "$temporary"
  mv "$temporary" "$compose_file"
  compose_changed=true
}

backup_release() {
  local timestamp partial
  timestamp=$(date -u +%Y%m%dT%H%M%SZ)
  partial="$backup_root/.partial-$timestamp-$operation_id-$$"
  mkdir "$partial"
  chmod 0700 "$partial"
  backup_partial="$partial"
  backup_path="$backup_root/$timestamp-$operation_id"
  [[ ! -e "$backup_path" ]] || fail 'backup destination already exists'

  trace backup-db
  "${compose[@]}" exec -T postgres sh -c 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
    >"$partial/sub2api.dump"
  [[ -s "$partial/sub2api.dump" ]] || fail 'PostgreSQL dump is empty'
  docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true \
    --mount "type=bind,src=$partial,dst=/backup,readonly" "$PINNED_POSTGRES_IMAGE" \
    pg_restore --list /backup/sub2api.dump >/dev/null

  trace backup-counts
  "${compose[@]}" exec -T postgres sh -c 'exec psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At' <<'SQL' >"$partial/record-counts.json"
select json_build_object(
  'users', (select count(*) from users),
  'accounts', (select count(*) from accounts),
  'groups', (select count(*) from groups),
  'api_keys', (select count(*) from api_keys),
  'settings', (select count(*) from settings),
  'usage_logs', (select count(*) from usage_logs)
)::text;
SQL
  validate_counts "$partial/record-counts.json"

  trace backup-app-data
  "${compose[@]}" exec -T sub2api sh -c 'exec tar -C /app/data -czf - .' >"$partial/app-data.tar.gz"
  [[ -s "$partial/app-data.tar.gz" ]] || fail 'app-data archive is empty'
  tar -tzf "$partial/app-data.tar.gz" >/dev/null

  chmod 0600 "$partial/sub2api.dump" "$partial/record-counts.json" "$partial/app-data.tar.gz"
  trace checksum
  (cd "$partial" && sha256sum sub2api.dump record-counts.json app-data.tar.gz >SHA256SUMS && sha256sum -c SHA256SUMS >/dev/null)
  chmod 0600 "$partial/SHA256SUMS"
  mv "$partial" "$backup_path"
  backup_partial=''
  chmod 0700 "$backup_path"
  backup_verified=true
}

wait_for_requested_health() {
  local deadline=$((SECONDS + health_timeout)) inspected actual_id status image
  while ((SECONDS <= deadline)); do
    inspected=$(inspect_runtime "$sub2api_container" 2>/dev/null || true)
    actual_id=$(jq -er '.[0].Image // empty' <<<"$inspected" 2>/dev/null || true)
    status=$(jq -er '.[0].State.Health.Status // empty' <<<"$inspected" 2>/dev/null || true)
    image=$(jq -er '.[0].Config.Image // empty' <<<"$inspected" 2>/dev/null || true)
    if [[ "$actual_id" == "$requested_image_id" && "$status" == healthy && "$image" == "$requested_image" ]] && \
      jq -e --arg volume "$app_volume" '
        ([.[0].Mounts[] | select(.Destination == "/app/data")] | length == 1 and
          .[0].Type == "volume" and (.[0].Name // .[0].Source) == $volume and .[0].RW == true)
      ' <<<"$inspected" >/dev/null; then
      return 0
    fi
    ((SECONDS < deadline)) || break
    sleep 2
  done
  return 1
}

wait_for_rollback_health() {
  local inspected
  inspected=$(inspect_runtime "$sub2api_container")
  jq -e --arg image "$previous_image" --arg id "$previous_image_id" '
    .[0].Config.Image == $image and .[0].Image == $id and .[0].State.Health.Status == "healthy"
  ' <<<"$inspected" >/dev/null
}

run_smoke() {
  local guard_body status post_counts admin_header gateway_header qr_hash logs
  guard_body=$(mktemp)
  post_counts=$(mktemp)
  admin_header=$(mktemp)
  gateway_header=$(mktemp)
  printf 'X-API-Key: %s\n' "$admin_key" >"$admin_header"
  printf 'Authorization: Bearer %s\n' "$gateway_key" >"$gateway_header"
  chmod 0600 "$guard_body" "$post_counts" "$admin_header" "$gateway_header"
  trap 'rm -f -- "$guard_body" "$post_counts" "$admin_header" "$gateway_header"' RETURN
  curl --connect-timeout 5 --max-time 15 -fsS "$base_url/health" | jq -e '.status == "ok"' >/dev/null
  curl --connect-timeout 5 --max-time 15 -fsS -H "@$admin_header" \
    "$base_url/api/v1/admin/system/version" |
    jq -e --arg version "$requested_version" '(.data // .).version == $version' >/dev/null
  curl --connect-timeout 5 --max-time 15 -fsS "$base_url/api/v1/settings/public" |
    jq -e '(.data // .).custom_menu_items | [ .[] | select(.id == "xingqiao-support" and .url == "md:support") ] | length == 1' >/dev/null
  qr_hash=$("${compose[@]}" exec -T sub2api sha256sum /app/data/pages/support/qq-group-1080152144.png | awk '{print $1}')
  [[ "$qr_hash" == "$EXPECTED_QR_SHA256" ]] || fail 'support QR checksum mismatch'
  "${compose[@]}" exec -T postgres sh -c 'exec psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At' <<'SQL' >"$post_counts"
select json_build_object(
  'users', (select count(*) from users),
  'accounts', (select count(*) from accounts),
  'groups', (select count(*) from groups),
  'api_keys', (select count(*) from api_keys),
  'settings', (select count(*) from settings),
  'usage_logs', (select count(*) from usage_logs)
)::text;
SQL
  validate_counts_monotonic "$backup_path/record-counts.json" "$post_counts"
  status=$(curl --connect-timeout 5 --max-time 15 -sS -o "$guard_body" -w '%{http_code}' \
    -X POST -H "@$admin_header" "$base_url/api/v1/admin/system/update")
  [[ "$status" == 409 ]] || fail 'update guard did not return HTTP 409'
  status=$(curl --connect-timeout 5 --max-time 15 -sS -o "$guard_body" -w '%{http_code}' \
    -X POST -H "@$admin_header" "$base_url/api/v1/admin/system/rollback")
  [[ "$status" == 409 ]] || fail 'rollback guard did not return HTTP 409'
  curl --connect-timeout 5 --max-time 15 -fsS -H "@$gateway_header" \
    "$base_url/v1/models" | jq -e '.data | type == "array"' >/dev/null
  logs=$("${compose[@]}" logs --since 10m --no-color sub2api)
  ! grep -Eiq 'fatal|migration' <<<"$logs" || fail 'recent fatal or migration error found'
}

attempt_rollback() {
  restore_compose_file || return 1
  if ! SUB2API_IMAGE="$previous_image" "${compose[@]}" up -d --no-deps --force-recreate sub2api >/dev/null; then
    return 1
  fi
  wait_for_rollback_health || return 1
  local service current expected
  for service in postgres redis caddy relay-ops internal-test-service; do
    current=$(resolve_container_id "$service") || return 1
    case "$service" in
      postgres) expected=$postgres_container ;;
      redis) expected=$redis_container ;;
      caddy) expected=$caddy_container ;;
      relay-ops) expected=$relay_ops_container ;;
      internal-test-service) expected=$internal_test_container ;;
    esac
    [[ "$current" == "$expected" ]] || return 1
  done
  return 0
}

on_exit() {
  local exit_status=$? rollback_state
  trap - EXIT HUP INT TERM
  set +e
  if [[ "$record_written" == false ]]; then
    if [[ "$mutation_started" == true ]]; then
      if attempt_rollback; then
        rollback_state=rolled_back
        write_record "$rollback_state"
        trace rolled_back
        printf 'result=rolled_back\n'
        exit_status=0
      else
        rollback_state=rollback_failed
        write_record "$rollback_state"
        trace rollback_failed
        printf 'result=rollback_failed\n'
        exit_status=1
      fi
    else
      restore_compose_file >/dev/null 2>&1 || true
      write_record preflight_failed
    fi
  fi
  [[ -n "${compose_backup:-}" ]] && rm -f -- "$compose_backup"
  if [[ -n "${backup_partial:-}" && "$backup_partial" == "$backup_root"/.partial-* && -d "$backup_partial" ]]; then
    rm -rf -- "$backup_partial"
  fi
  [[ "${lock_owned:-false}" == true ]] && rmdir "$lock_dir" 2>/dev/null || true
  exit "$exit_status"
}
trap on_exit EXIT
trap 'exit 130' HUP INT TERM

trace inspect
sub2api_container=$(resolve_container_id sub2api) || fail 'sub2api container was not resolved uniquely'
for service in postgres redis caddy relay-ops internal-test-service; do
  case "$service" in
    postgres) postgres_container=$(resolve_container_id "$service") || fail "$service container was not resolved uniquely" ;;
    redis) redis_container=$(resolve_container_id "$service") || fail "$service container was not resolved uniquely" ;;
    caddy) caddy_container=$(resolve_container_id "$service") || fail "$service container was not resolved uniquely" ;;
    relay-ops) relay_ops_container=$(resolve_container_id "$service") || fail "$service container was not resolved uniquely" ;;
    internal-test-service) internal_test_container=$(resolve_container_id "$service") || fail "$service container was not resolved uniquely" ;;
  esac
done
inspected=$(inspect_runtime "$sub2api_container" "$postgres_container" "$redis_container" \
  "$caddy_container" "$relay_ops_container" "$internal_test_container") \
  || fail 'running containers could not be inspected'
previous_image=$(jq -er '.[0].Config.Image' <<<"$inspected") || fail 'previous image was not identified'
previous_image_id=$(jq -er '.[0].Image' <<<"$inspected") || fail 'previous image ID was not identified'
validate_runtime "$inspected" || fail 'runtime project, health, or named-volume identity is invalid'
storage_identity=true

config_before=$("${compose[@]}" config --format json) || fail 'Compose configuration could not be resolved'
jq -e --arg image "$previous_image" '.services.sub2api.image == $image' <<<"$config_before" >/dev/null \
  || fail 'Compose image does not match the running previous image'
health_timeout=${HEALTH_TIMEOUT_SECONDS:-180}
[[ "$health_timeout" =~ ^[0-9]+$ && "$health_timeout" -le 180 ]] || fail 'HEALTH_TIMEOUT_SECONDS must be an integer no greater than 180'
require_free_space "$deploy_root"
require_free_space "$backup_root"

compose_backup=$(mktemp "$record_root/.compose-backup.XXXXXX")
chmod 0600 "$compose_backup"
cp "$compose_file" "$compose_backup"
docker tag "$previous_image_id" "$rollback_tag" >/dev/null

trace pull
docker pull "$requested_image" >/dev/null
requested_image_id=$(docker image inspect "$requested_image" | jq -er --arg digest "$requested_digest" \
  '.[0] | select(any(.RepoDigests[]?; endswith($digest))) | .Id') || fail 'requested image digest was not verified'

backup_release
replace_sub2api_image "$compose_backup"
trace compose-validate
config_after=$("${compose[@]}" config --format json) || fail 'updated Compose configuration could not be resolved'
validate_compose_storage "$config_after" || fail 'updated Compose storage or image identity is invalid'

mutation_started=true
trace recreate-sub2api
"${compose[@]}" up -d --no-deps --force-recreate sub2api >/dev/null
trace health
wait_for_requested_health || fail 'requested image did not become healthy with the expected volume'
health=true

for service in postgres redis caddy relay-ops internal-test-service; do
  current=$(resolve_container_id "$service") || fail "$service container disappeared after update"
  case "$service" in
    postgres) expected=$postgres_container ;;
    redis) expected=$redis_container ;;
    caddy) expected=$caddy_container ;;
    relay-ops) expected=$relay_ops_container ;;
    internal-test-service) expected=$internal_test_container ;;
  esac
  [[ "$current" == "$expected" ]] || fail "$service container identity changed"
done

trace smoke
run_smoke
smoke=true
write_record promoted
trace promoted
printf 'result=promoted\n'
