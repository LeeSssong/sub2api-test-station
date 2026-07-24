#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'Sub2API runtime baseline capture failed: %s\n' "$1" >&2
  exit 1
}

canonical_directory() {
  case "$1" in
    /*) ;;
    *) fail "path is not absolute: $1" ;;
  esac
  (cd "$1" && pwd -P) || fail "path is not a readable directory: $1"
}

command -v docker >/dev/null || fail 'docker is required'
command -v jq >/dev/null || fail 'jq is required'

expected_project=${EXPECTED_PROJECT:-sub2api-deploy}
expected_image_id=${EXPECTED_IMAGE_ID:-sha256:939e6f88068e82fd65f212bcc7b28b9ef2a9af27b8cce64e0b819a8b65fc3220}
expected_sub2api_data=${EXPECTED_SUB2API_DATA:?EXPECTED_SUB2API_DATA is required}
expected_postgres_data=${EXPECTED_POSTGRES_DATA:?EXPECTED_POSTGRES_DATA is required}
expected_redis_data=${EXPECTED_REDIS_DATA:?EXPECTED_REDIS_DATA is required}
sub2api_container=${SUB2API_CONTAINER:-sub2api}
postgres_container=${POSTGRES_CONTAINER:-sub2api-postgres}
redis_container=${REDIS_CONTAINER:-sub2api-redis}

expected_sub2api_data=$(canonical_directory "$expected_sub2api_data")
expected_postgres_data=$(canonical_directory "$expected_postgres_data")
expected_redis_data=$(canonical_directory "$expected_redis_data")

inspect_mount() {
  docker inspect "$1" | jq -er --arg destination "$2" '
    .[0].Mounts[] | select(.Type == "bind" and .Destination == $destination) |
    {type: .Type, source: .Source, destination: .Destination, rw: .RW}
  '
}

validate_project() {
  local container=$1
  local project
  project=$(docker inspect "$container" | jq -er '.[] | .Config.Labels["com.docker.compose.project"]')
  [[ "$project" == "$expected_project" ]] || fail "unexpected Compose project on $container"
}

validate_mount() {
  local container=$1
  local destination=$2
  local expected_source=$3
  local mount
  local source
  local canonical_source

  mount=$(inspect_mount "$container" "$destination") || fail "missing writable bind at $destination on $container"
  source=$(jq -er '.source' <<<"$mount")
  canonical_source=$(canonical_directory "$source")
  [[ "$canonical_source" == "$expected_source" ]] || fail "unexpected source at $destination on $container"
  jq -e '.rw == true' >/dev/null <<<"$mount" || fail "data bind is read-only at $destination on $container"
  jq -cn --arg type bind --arg source "$canonical_source" --arg destination "$destination" \
    '{type: $type, source: $source, destination: $destination, rw: true}'
}

validate_project "$sub2api_container"
validate_project "$postgres_container"
validate_project "$redis_container"

sub2api_inspect=$(docker inspect "$sub2api_container")
image=$(jq -er '.[0].Config.Image' <<<"$sub2api_inspect")
image_id=$(jq -er '.[0].Image' <<<"$sub2api_inspect")
[[ "$image_id" == "$expected_image_id" ]] || fail 'unexpected Sub2API image ID'

config_files=$(jq -er '.[0].Config.Labels["com.docker.compose.project.config_files"]' <<<"$sub2api_inspect")
working_dir=$(jq -er '.[0].Config.Labels["com.docker.compose.project.working_dir"]' <<<"$sub2api_inspect")
[[ "$config_files" == /* && "$working_dir" == /* ]] || fail 'Compose metadata is not absolute'

sub2api_mount=$(validate_mount "$sub2api_container" /app/data "$expected_sub2api_data")
postgres_mount=$(validate_mount "$postgres_container" /var/lib/postgresql/data "$expected_postgres_data")
redis_mount=$(validate_mount "$redis_container" /data "$expected_redis_data")

network_names=$(printf '%s\n' "$sub2api_inspect" | jq -cer '.[0].NetworkSettings.Networks | keys | sort')
postgres_network_names=$(docker inspect "$postgres_container" | jq -cer '.[0].NetworkSettings.Networks | keys | sort')
redis_network_names=$(docker inspect "$redis_container" | jq -cer '.[0].NetworkSettings.Networks | keys | sort')
[[ "$network_names" == "$postgres_network_names" && "$network_names" == "$redis_network_names" ]] \
  || fail 'Compose services do not share the same networks'

captured_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
jq -n \
  --arg project "$expected_project" \
  --arg config_files "$config_files" \
  --arg working_dir "$working_dir" \
  --arg image "$image" \
  --arg image_id "$image_id" \
  --arg captured_at "$captured_at" \
  --argjson sub2api_mount "$sub2api_mount" \
  --argjson postgres_mount "$postgres_mount" \
  --argjson redis_mount "$redis_mount" \
  --argjson network_names "$network_names" \
  '{
    project: $project,
    config_files: $config_files,
    working_dir: $working_dir,
    image: $image,
    image_id: $image_id,
    mounts: {
      sub2api: $sub2api_mount,
      postgres: $postgres_mount,
      redis: $redis_mount
    },
    network_names: $network_names,
    captured_at: $captured_at
  }'
