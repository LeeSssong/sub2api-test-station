#!/usr/bin/env bash
set -euo pipefail

umask 077

fail() {
  printf 'blue-green deploy failed: %s\n' "$1" >&2
  exit 1
}

trace_event() {
  [[ -n "${RELEASE_EVENT_LOG:-}" ]] || return 0
  printf '%s\n' "$1" >>"$RELEASE_EVENT_LOG"
}

mode=''
requested_image=''
source_commit=''
source_tree=''
tested_tree=''
migrations_hash=''
deadline_epoch=''

while (($#)); do
  case "$1" in
    --mode) (($# >= 2)) || fail '--mode requires a value'; [[ -z "$mode" ]] || fail '--mode may be supplied once'; mode=$2; shift 2 ;;
    --image) (($# >= 2)) || fail '--image requires a value'; [[ -z "$requested_image" ]] || fail '--image may be supplied once'; requested_image=$2; shift 2 ;;
    --source-commit) (($# >= 2)) || fail '--source-commit requires a value'; [[ -z "$source_commit" ]] || fail '--source-commit may be supplied once'; source_commit=$2; shift 2 ;;
    --source-tree) (($# >= 2)) || fail '--source-tree requires a value'; [[ -z "$source_tree" ]] || fail '--source-tree may be supplied once'; source_tree=$2; shift 2 ;;
    --tested-tree) (($# >= 2)) || fail '--tested-tree requires a value'; [[ -z "$tested_tree" ]] || fail '--tested-tree may be supplied once'; tested_tree=$2; shift 2 ;;
    --migrations-hash) (($# >= 2)) || fail '--migrations-hash requires a value'; [[ -z "$migrations_hash" ]] || fail '--migrations-hash may be supplied once'; migrations_hash=$2; shift 2 ;;
		--deadline-epoch) (($# >= 2)) || fail '--deadline-epoch requires a value'; [[ -z "$deadline_epoch" ]] || fail '--deadline-epoch may be supplied once'; deadline_epoch=$2; shift 2 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ "$mode" == rehearsal || "$mode" == production ]] || fail '--mode must be rehearsal or production'
[[ "$requested_image" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]] || fail '--image must be an immutable repository sha256 digest'
[[ "$source_commit" =~ ^[a-f0-9]{40}$ ]] || fail '--source-commit must be 40 lowercase hex'
[[ "$source_tree" =~ ^[a-f0-9]{40}$ ]] || fail '--source-tree must be 40 lowercase hex'
[[ "$tested_tree" =~ ^[a-f0-9]{40}$ ]] || fail '--tested-tree must be 40 lowercase hex'
[[ "$migrations_hash" =~ ^[a-f0-9]{64}$ ]] || fail '--migrations-hash must be 64 lowercase hex'
[[ "$deadline_epoch" =~ ^[1-9][0-9]{9}$ ]] || fail '--deadline-epoch must be a Unix epoch'
[[ "$source_tree" == "$tested_tree" ]] || fail 'source tree does not equal tested tree'

for required_command in docker curl jq df awk date stat mktemp find sort uniq chmod mv mkdir cp tr grep rm dirname basename sleep perl; do
  command -v "$required_command" >/dev/null 2>&1 || fail "$required_command is required"
done

check_deadline() {
	local now
	now=$(date -u +%s) || fail 'release deadline clock failed'
	[[ "$now" =~ ^[0-9]+$ && "$now" -lt "$deadline_epoch" ]] || fail 'release exceeded its end-to-end deadline'
}

check_deadline
deadline_remaining=$((deadline_epoch - $(date -u +%s)))
(( deadline_remaining > 0 )) || fail 'release exceeded its end-to-end deadline'
parent_pid=$$
perl -e '($pid, $seconds) = @ARGV; sleep $seconds; kill "TERM", $pid' "$parent_pid" "$deadline_remaining" &
deadline_watchdog_pid=$!

stop_deadline_watchdog() {
	if [[ -n "${deadline_watchdog_pid:-}" ]]; then
		kill "$deadline_watchdog_pid" 2>/dev/null || true
		wait "$deadline_watchdog_pid" 2>/dev/null || true
		deadline_watchdog_pid=''
	fi
}
trap stop_deadline_watchdog EXIT

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

canonical_optional_file_path() {
  local value=$1 label=$2 parent physical canonical
  [[ "$value" == /* && ! -L "$value" ]] || fail "$label must be absolute and non-symlinked"
  parent=$(dirname "$value")
  [[ -d "$parent" && ! -L "$parent" ]] || fail "$label parent must be a non-symlink directory"
  physical=$(cd "$parent" && pwd -P)
  canonical="$physical/$(basename "$value")"
  [[ "$canonical" == "$value" ]] || fail "$label must be canonical"
  printf '%s\n' "$value"
}

mode_of() {
  stat -f '%Lp' "$1" 2>/dev/null || stat -c '%a' "$1"
}

deploy_root=$(canonical_directory "${DEPLOY_ROOT:?DEPLOY_ROOT is required}" 'DEPLOY_ROOT')
base_compose=$(canonical_file "${BASE_COMPOSE:?BASE_COMPOSE is required}" 'BASE_COMPOSE')
secret_env=$(canonical_file "${SECRET_ENV:?SECRET_ENV is required}" 'SECRET_ENV')
release_env=$(canonical_file "${RELEASE_ENV:?RELEASE_ENV is required}" 'RELEASE_ENV')
release_state=$(canonical_optional_file_path "${RELEASE_STATE:?RELEASE_STATE is required}" 'RELEASE_STATE')
record_root=$(canonical_directory "${RELEASE_RECORD_ROOT:?RELEASE_RECORD_ROOT is required}" 'RELEASE_RECORD_ROOT')
admin_key_file=$(canonical_file "${ADMIN_API_KEY_FILE:?ADMIN_API_KEY_FILE is required}" 'ADMIN_API_KEY_FILE')
gateway_key_file=$(canonical_file "${GATEWAY_API_KEY_FILE:?GATEWAY_API_KEY_FILE is required}" 'GATEWAY_API_KEY_FILE')
base_url=${BASE_URL:?BASE_URL is required}

[[ "$(mode_of "$release_env")" == 600 ]] || fail 'RELEASE_ENV mode must be 0600'
[[ "$(mode_of "$secret_env")" == 600 ]] || fail 'SECRET_ENV mode must be 0600'
[[ "$(mode_of "$admin_key_file")" == 600 ]] || fail 'ADMIN_API_KEY_FILE mode must be 0600'
[[ "$(mode_of "$gateway_key_file")" == 600 ]] || fail 'GATEWAY_API_KEY_FILE mode must be 0600'

if [[ "$mode" == production ]]; then
	[[ "$base_url" == https://* ]] || fail 'production BASE_URL must be HTTPS'
  [[ "$(uname -s)" == Linux ]] || fail 'production deployment must run on Linux'
  [[ -z "${DOCKER_HOST:-}" ]] || fail 'production deployment must not use DOCKER_HOST'
  [[ "${DOCKER_CONTEXT:-default}" == default ]] || fail 'production DOCKER_CONTEXT must be default'
  [[ "$(docker context show)" == default ]] || fail 'production Docker context must be default'
  compose_project=${COMPOSE_PROJECT_NAME:-sub2api}
  [[ "$compose_project" == sub2api ]] || fail 'production COMPOSE_PROJECT_NAME must be sub2api'
else
  compose_project=${COMPOSE_PROJECT_NAME:-sub2api-blue-green-rehearsal}
	[[ "$compose_project" == sub2api-blue-green-rehearsal ]] \
		|| fail 'rehearsal COMPOSE_PROJECT_NAME must be sub2api-blue-green-rehearsal'
	rehearsal_root=$(canonical_directory "${REHEARSAL_ROOT:?REHEARSAL_ROOT is required in rehearsal mode}" 'REHEARSAL_ROOT')
	[[ "$base_compose" == "$deploy_root/compose.sub2api-rehearsal.yaml" ]] \
		|| fail 'rehearsal BASE_COMPOSE must be the isolated rehearsal topology'
	case "$secret_env" in "$rehearsal_root"/*) ;; *) fail 'rehearsal SECRET_ENV must be inside REHEARSAL_ROOT' ;; esac
	case "$release_env" in "$rehearsal_root"/*) ;; *) fail 'rehearsal RELEASE_ENV must be inside REHEARSAL_ROOT' ;; esac
	case "$release_state" in "$rehearsal_root"/*) ;; *) fail 'rehearsal RELEASE_STATE must be inside REHEARSAL_ROOT' ;; esac
	case "$record_root" in "$rehearsal_root"/*) ;; *) fail 'rehearsal RELEASE_RECORD_ROOT must be inside REHEARSAL_ROOT' ;; esac
	case "$admin_key_file" in "$rehearsal_root"/*) ;; *) fail 'rehearsal ADMIN_API_KEY_FILE must be inside REHEARSAL_ROOT' ;; esac
	case "$gateway_key_file" in "$rehearsal_root"/*) ;; *) fail 'rehearsal GATEWAY_API_KEY_FILE must be inside REHEARSAL_ROOT' ;; esac
	[[ "$release_state" == "$record_root/release-state.json" ]] \
		|| fail 'rehearsal RELEASE_STATE must use the rehearsal record namespace'
	[[ "$base_url" =~ ^https?://(localhost|127\.0\.0\.1)(:[1-9][0-9]{0,4})?$ ]] \
		|| fail 'rehearsal BASE_URL must be localhost-only'
fi

network_curl_image=${NETWORK_CURL_IMAGE:-}
network_curl_allowlist=${NETWORK_CURL_IMAGE_ALLOWLIST:-}
if [[ "$mode" == production ]]; then
  [[ "$network_curl_image" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]] \
    || fail 'production NETWORK_CURL_IMAGE must be an approved immutable sha256 digest'
  [[ -n "$network_curl_allowlist" ]] || fail 'production NETWORK_CURL_IMAGE_ALLOWLIST is required'
  network_curl_approved=false
  while IFS= read -r approved_network_curl_image; do
    [[ -z "$approved_network_curl_image" ]] && continue
    [[ "$approved_network_curl_image" == "$network_curl_image" ]] && network_curl_approved=true
  done <<<"$network_curl_allowlist"
  [[ "$network_curl_approved" == true ]] || fail 'production NETWORK_CURL_IMAGE is not allowlisted'
else
  network_curl_image=${network_curl_image:-curlimages/curl:8.12.1}
fi

lock_dir="$record_root/.blue-green.lock"
lock_owner_path="$lock_dir/owner.pid"
lock_owned=false

cleanup_lock() {
	stop_deadline_watchdog
  if [[ "${lock_owned:-false}" == true ]]; then
    rm -f -- "$lock_owner_path"
    rmdir "$lock_dir" 2>/dev/null || true
    lock_owned=false
  fi
}

persist_lock_owner() {
  local temporary="$lock_dir/.owner.pid.$$"
  if ! { printf '%s\n' "$$" >"$temporary" && chmod 0600 "$temporary" && mv "$temporary" "$lock_owner_path"; }; then
    rm -f -- "$temporary"
    rmdir "$lock_dir" 2>/dev/null || true
    lock_owned=false
    fail 'could not persist blue-green lock ownership'
  fi
}

acquire_lock() {
  local owner_pid
  if mkdir "$lock_dir" 2>/dev/null; then
    lock_owned=true
    persist_lock_owner
    return 0
  fi

  [[ -d "$lock_dir" && ! -L "$lock_dir" ]] || fail 'blue-green release lock is invalid'
  if [[ ! -e "$lock_owner_path" ]]; then
    fail 'blue-green release lock owner is missing; manual recovery is required'
  fi
  [[ -f "$lock_owner_path" && ! -L "$lock_owner_path" ]] || fail 'blue-green release lock owner is invalid'
  [[ "$(mode_of "$lock_owner_path")" == 600 ]] || fail 'blue-green release lock owner mode must be 0600'
  owner_pid=$(awk 'NR == 1 { owner=$0 } END { if (NR != 1) exit 1; print owner }' "$lock_owner_path") \
    || fail 'blue-green release lock owner is invalid'
  [[ "$owner_pid" =~ ^[1-9][0-9]*$ ]] || fail 'blue-green release lock owner is invalid'
  if kill -0 "$owner_pid" 2>/dev/null; then
    fail 'another blue-green release is in progress'
  fi
  fail 'blue-green release lock owner is stale; manual recovery is required'
}

acquire_lock
partial_path=''
record_finalized=false
cutover_attempted=false
cutover_applied=false
state_persisted=false
persistence_started=false
worker_update_started=false
rollback_completed=false
failure_reason='unexpected_exit'
candidate_slot=''
candidate_upstream=''
previous_slot=''
previous_upstream=''
previous_worker_image=''
rollback_blue_image=''
rollback_green_image=''
rollback_source_commit=''
rollback_source_tree=''
rollback_migrations_hash=''
rollback_postgres_id=''
rollback_redis_id=''
rollback_caddy_id=''
candidate_env=''
rollback_env=''
admin_header=''
gateway_header=''
attempt_id="$(date -u +%Y%m%dT%H%M%SZ)-$mode-$$"
started_epoch=$(date -u +%s)
record_path="$record_root/$attempt_id.json"

compose_current=(docker compose --project-name "$compose_project" --project-directory "$deploy_root"
  --env-file "$secret_env" --env-file "$release_env" -f "$base_compose")
export SUB2API_RELEASE_ENV_FILE="$release_env"

write_final_record() {
  local result=$1 state=$2 reason=$3 temporary
  [[ "$record_finalized" == false ]] || return 0
  temporary="$record_root/.$attempt_id.record.tmp"
  jq -n \
    --arg attempt_id "$attempt_id" \
    --arg mode "$mode" \
    --arg image "$requested_image" \
    --arg source_commit "$source_commit" \
    --arg source_tree "$source_tree" \
    --arg tested_tree "$tested_tree" \
    --arg migrations_hash "$migrations_hash" \
    --arg result "$result" \
    --arg state "$state" \
    --arg reason "$reason" \
    --argjson rolled_back "$rollback_completed" \
    '{schema_version:1, attempt_id:$attempt_id, mode:$mode,
      requested:{image:$image, source_commit:$source_commit, source_tree:$source_tree,
        tested_tree:$tested_tree, migrations_hash:$migrations_hash},
      result:$result, state:$state, reason:$reason, rolled_back:$rolled_back}' >"$temporary"
  chmod 0600 "$temporary"
  mv "$temporary" "$record_path"
  record_finalized=true
}

write_release_env_values_to() {
  local target=$1 blue_image=$2 green_image=$3 worker_image=$4 active_upstream=$5 active_slot=$6 previous=$7 temporary
  temporary="$(dirname "$target")/.$(basename "$target").$attempt_id.tmp"
  awk \
    -v blue="$blue_image" -v green="$green_image" -v worker="$worker_image" \
    -v upstream="$active_upstream" -v active="$active_slot" -v previous="$previous" '
    BEGIN {
      values["SUB2API_BLUE_IMAGE"] = blue
      values["SUB2API_GREEN_IMAGE"] = green
      values["SUB2API_WORKER_IMAGE"] = worker
      values["SUB2API_ACTIVE_UPSTREAM"] = upstream
      values["SUB2API_ACTIVE_SLOT"] = active
      values["SUB2API_PREVIOUS_SLOT"] = previous
    }
    /^[[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]*=/ {
      key=$0
      sub(/^[[:space:]]*/, "", key)
      sub(/[[:space:]]*=.*/, "", key)
      if (key in values) {
        print key "=" values[key]
        seen[key]=1
        next
      }
    }
    { print }
    END {
      for (key in values) if (!(key in seen)) print key "=" values[key]
    }
  ' "$release_env" >"$temporary"
  chmod 0600 "$temporary"
  mv "$temporary" "$target"
}

write_release_env_values() {
  write_release_env_values_to "$release_env" "$@"
}

write_state_values() {
  local active_slot=$1 active_upstream=$2 blue_image=$3 green_image=$4 worker_image=$5 \
    commit=$6 tree=$7 migrations=$8 postgres_id=$9
  shift 9
  local redis_id=$1 caddy_id=$2 temporary
  temporary="$(dirname "$release_state")/.$(basename "$release_state").$attempt_id.tmp"
  jq -n \
    --arg active_slot "$active_slot" --arg active_upstream "$active_upstream" \
    --arg blue_image "$blue_image" --arg green_image "$green_image" --arg worker_image "$worker_image" \
    --arg source_commit "$commit" --arg source_tree "$tree" --arg migrations_hash "$migrations" \
    --arg postgres_id "$postgres_id" --arg redis_id "$redis_id" --arg caddy_id "$caddy_id" \
    '{schema_version:1, active_slot:$active_slot, active_upstream:$active_upstream,
      blue_image:$blue_image, green_image:$green_image, worker_image:$worker_image,
      source_commit:$source_commit, source_tree:$source_tree, migrations_hash:$migrations_hash,
      postgres_id:$postgres_id, redis_id:$redis_id, caddy_id:$caddy_id}' >"$temporary"
  chmod 0600 "$temporary"
  mv "$temporary" "$release_state"
}

write_partial() {
  local phase=$1 temporary
  [[ -n "$partial_path" ]] || return 0
  temporary="$partial_path.tmp"
  jq -n \
    --arg attempt_id "$attempt_id" --arg mode "$mode" --argjson started_epoch "$started_epoch" \
    --arg phase "$phase" --argjson cutover_attempted "$cutover_attempted" \
    --argjson cutover_applied "$cutover_applied" \
    --argjson worker_updated "$worker_update_started" \
    --arg previous_slot "$previous_slot" --arg previous_upstream "$previous_upstream" \
    --arg previous_blue_image "$rollback_blue_image" --arg previous_green_image "$rollback_green_image" \
    --arg previous_worker_image "$previous_worker_image" \
    --arg previous_source_commit "$rollback_source_commit" --arg previous_source_tree "$rollback_source_tree" \
    --arg previous_migrations_hash "$rollback_migrations_hash" \
    --arg previous_postgres_id "$rollback_postgres_id" --arg previous_redis_id "$rollback_redis_id" \
    --arg previous_caddy_id "$rollback_caddy_id" \
    --arg candidate_slot "$candidate_slot" --arg candidate_upstream "$candidate_upstream" \
    --arg candidate_image "$requested_image" \
    '{schema_version:1, attempt_id:$attempt_id, mode:$mode, started_epoch:$started_epoch,
      phase:$phase, cutover_attempted:$cutover_attempted, cutover_applied:$cutover_applied,
      worker_updated:$worker_updated,
      previous:{active_slot:$previous_slot, active_upstream:$previous_upstream,
        blue_image:$previous_blue_image, green_image:$previous_green_image, worker_image:$previous_worker_image,
        source_commit:$previous_source_commit, source_tree:$previous_source_tree,
        migrations_hash:$previous_migrations_hash, postgres_id:$previous_postgres_id,
        redis_id:$previous_redis_id, caddy_id:$previous_caddy_id},
      candidate:{slot:$candidate_slot, upstream:$candidate_upstream, image:$candidate_image}}' >"$temporary"
  chmod 0600 "$temporary"
  mv "$temporary" "$partial_path"
}

gate() {
  local reason_code=$1 reason=$2 seconds=${3:-300}
  jq -n --arg reason_code "$reason_code" --arg reason "$reason" --argjson seconds "$seconds" '
    {schema_version:1, downtime_required:true, reason_code:$reason_code, reason:$reason,
      estimated_unavailable_seconds:$seconds,
      rollback:["keep current active slot", "do not start candidate", "prepare an authorized maintenance release"]}'
  cleanup_lock
  exit 2
}

validate_upstream() {
  case "$1" in
    sub2api-blue:8080|sub2api-green:8080) return 0 ;;
    *) return 1 ;;
  esac
}

managed_env_value() {
  local key=$1 count value
  count=$(awk -v key="$key" '
    $0 ~ "^[[:space:]]*" key "[[:space:]]*=" { count++ }
    END { print count + 0 }
  ' "$release_env")
  [[ "$count" == 1 ]] || fail "RELEASE_ENV must contain exactly one $key assignment"
  value=$(awk -v key="$key" '
    $0 ~ "^[[:space:]]*" key "[[:space:]]*=" {
      value=$0
      sub("^[[:space:]]*" key "[[:space:]]*=[[:space:]]*", "", value)
      sub(/[[:space:]]+$/, "", value)
      print value
    }
  ' "$release_env")
  printf '%s\n' "$value"
}

resolve_container_id() {
  local service=$1 ids count
  ids=$("${compose_current[@]}" ps -q "$service") || return 1
  count=$(printf '%s\n' "$ids" | awk 'NF { count++ } END { print count + 0 }')
  [[ "$count" == 1 ]] || return 1
  printf '%s\n' "$ids" | awk 'NF { print; exit }'
}

wait_for_worker_healthy() {
  local timeout=${WORKER_HEALTH_TIMEOUT_SECONDS:-90} poll=${WORKER_HEALTH_POLL_SECONDS:-1}
  local deadline now remaining attempts=0 max_attempts worker_status
  [[ "$timeout" =~ ^[1-9][0-9]*$ && "$poll" =~ ^[1-9][0-9]*$ ]] || return 1
  deadline=$(( $(date -u +%s) + timeout ))
  max_attempts=$((timeout / poll + 1))
  while true; do
    worker_status=$(docker inspect "$(resolve_container_id sub2api-worker)" --format '{{.State.Health.Status}}') || return 1
    [[ "$worker_status" == healthy ]] && return 0
    attempts=$((attempts + 1))
    [[ "$attempts" -lt "$max_attempts" ]] || return 1
    now=$(date -u +%s)
    [[ "$now" =~ ^[0-9]+$ && "$now" -lt "$deadline" ]] || return 1
    remaining=$((deadline - now))
    if [[ "$poll" -lt "$remaining" ]]; then sleep "$poll"; else sleep "$remaining"; fi
  done
}

wait_for_candidate_healthy() {
	local service=$1 timeout=${CANDIDATE_HEALTH_TIMEOUT_SECONDS:-90} poll=${CANDIDATE_HEALTH_POLL_SECONDS:-1}
	local deadline now remaining attempts=0 max_attempts candidate_status candidate_id
	[[ "$timeout" =~ ^[1-9][0-9]*$ && "$poll" =~ ^[1-9][0-9]*$ ]] || return 1
	deadline=$(( $(date -u +%s) + timeout ))
	max_attempts=$((timeout / poll + 1))
	while true; do
		check_deadline
		candidate_id=$(resolve_container_id "$service") || return 1
		candidate_status=$(docker inspect "$candidate_id" --format '{{.State.Health.Status}}') || return 1
		[[ "$candidate_status" == healthy ]] && return 0
		[[ "$candidate_status" != unhealthy ]] || return 1
		attempts=$((attempts + 1))
		[[ "$attempts" -lt "$max_attempts" ]] || return 1
		now=$(date -u +%s)
		[[ "$now" =~ ^[0-9]+$ && "$now" -lt "$deadline" ]] || return 1
		remaining=$((deadline - now))
		if [[ "$poll" -lt "$remaining" ]]; then sleep "$poll"; else sleep "$remaining"; fi
	done
}

container_role() {
	local container_id=$1
	docker inspect "$container_id" --format '{{range .Config.Env}}{{println .}}{{end}}' | awk -F= '
		$1 == "SERVER_PROCESS_ROLE" { value=$2; count++ }
		END { if (count != 1) exit 1; print value }
	'
}

live_caddy_upstream() {
	local config
	config=$("${compose_current[@]}" exec -T caddy wget -qO- http://127.0.0.1:2019/config/) || return 1
	jq -er '
		[.. | objects | .dial? // empty |
		 select(. == "sub2api-blue:8080" or . == "sub2api-green:8080")] |
		unique | if length == 1 then .[0] else error("active upstream is not unique") end
	' <<<"$config"
}

write_acceptance_headers() {
	[[ -n "$admin_header" && -n "$gateway_header" ]] && return 0
	admin_header="$record_root/.$attempt_id.admin.header"
	gateway_header="$record_root/.$attempt_id.gateway.header"
	printf 'X-API-Key: %s\n' "$(tr -d '\r\n' <"$admin_key_file")" >"$admin_header"
	printf 'Authorization: Bearer %s\n' "$(tr -d '\r\n' <"$gateway_key_file")" >"$gateway_header"
	chmod 0600 "$admin_header" "$gateway_header"
}

public_acceptance() {
	write_acceptance_headers || return 1
	curl -fsS --connect-timeout 5 --max-time 15 "$base_url/health" | jq -e '.status == "ok"' >/dev/null || return 1
	curl -fsS --connect-timeout 5 --max-time 15 -H "@$admin_header" "$base_url/api/v1/admin/system/version" | \
		jq -e '(.data // .).version | type == "string" and length > 0' >/dev/null || return 1
	curl -fsS --connect-timeout 5 --max-time 15 -H "@$gateway_header" "$base_url/v1/models" | \
		jq -e '.data | type == "array"' >/dev/null || return 1
}

worker_logs_are_acceptable() {
  local worker_logs
  worker_logs=$("${compose_current[@]}" logs --no-color --tail 200 sub2api-worker) || return 1
  printf '%s\n' "$worker_logs" | grep -Eiq 'panic:|fatal:|migration.*failed|worker.*failed' && return 1
  return 0
}

restore_previous() {
  local rollback_ok=true current_blue current_green previous_previous
  if [[ "$cutover_attempted" == true ]]; then
    if validate_upstream "$previous_upstream"; then
      "${compose_current[@]}" exec -T -e "SUB2API_ACTIVE_UPSTREAM=$previous_upstream" caddy \
        caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null 2>&1 || rollback_ok=false
      "${compose_current[@]}" exec -T -e "SUB2API_ACTIVE_UPSTREAM=$previous_upstream" caddy \
        caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null 2>&1 || rollback_ok=false
    else
      rollback_ok=false
    fi
  fi
  if [[ "$rollback_ok" == true && "$worker_update_started" == true ]]; then
    rollback_env="$record_root/.$attempt_id.rollback.env"
    write_release_env_values_to "$rollback_env" "$rollback_blue_image" "$rollback_green_image" "$previous_worker_image" \
      "$previous_upstream" "$previous_slot" "$candidate_slot" || rollback_ok=false
    if [[ "$rollback_ok" == true ]]; then
      compose_rollback=(docker compose --project-name "$compose_project" --project-directory "$deploy_root"
        --env-file "$secret_env" --env-file "$rollback_env" -f "$base_compose")
      "${compose_rollback[@]}" up --no-deps -d --force-recreate sub2api-worker >/dev/null 2>&1 || rollback_ok=false
      [[ "$rollback_ok" == false ]] || wait_for_worker_healthy || rollback_ok=false
      [[ "$rollback_ok" == false ]] || worker_logs_are_acceptable || rollback_ok=false
    fi
  fi
	if [[ "$rollback_ok" == true && "$cutover_attempted" == true ]]; then
		[[ "$(live_caddy_upstream)" == "$previous_upstream" ]] || rollback_ok=false
		[[ "$rollback_ok" == false ]] || public_acceptance || rollback_ok=false
	fi
	if [[ "$rollback_ok" == true ]]; then
		[[ "$(resolve_container_id postgres)" == "$rollback_postgres_id" ]] || rollback_ok=false
		[[ "$rollback_ok" == false || "$(resolve_container_id redis)" == "$rollback_redis_id" ]] || rollback_ok=false
		[[ "$rollback_ok" == false || "$(resolve_container_id caddy)" == "$rollback_caddy_id" ]] || rollback_ok=false
	fi
  if [[ "$rollback_ok" == true && ( "$persistence_started" == true || "$state_persisted" == true || "$worker_update_started" == true ) ]]; then
    current_blue=$rollback_blue_image
    current_green=$rollback_green_image
    previous_previous=$candidate_slot
    write_release_env_values "$current_blue" "$current_green" "$previous_worker_image" \
      "$previous_upstream" "$previous_slot" "$previous_previous" || rollback_ok=false
    write_state_values "$previous_slot" "$previous_upstream" "$current_blue" "$current_green" \
      "$previous_worker_image" "$rollback_source_commit" "$rollback_source_tree" "$rollback_migrations_hash" \
      "$rollback_postgres_id" "$rollback_redis_id" "$rollback_caddy_id" || rollback_ok=false
  fi
  [[ "$rollback_ok" == true ]] || return 1
  rollback_completed=true
  return 0
}

on_exit() {
  local status=$?
  trap - EXIT HUP INT TERM
  set +e
  [[ -n "$candidate_env" ]] && rm -f -- "$candidate_env"
  [[ -n "$rollback_env" ]] && rm -f -- "$rollback_env"
  [[ -n "$admin_header" ]] && rm -f -- "$admin_header"
  [[ -n "$gateway_header" ]] && rm -f -- "$gateway_header"
  if [[ "$status" -ne 0 && "$record_finalized" == false && -n "$partial_path" && -e "$partial_path" ]]; then
    if restore_previous; then
      write_final_record failed rolled_back "$failure_reason"
    else
      write_final_record failed rollback_failed "$failure_reason"
    fi
    [[ "$rollback_completed" == true ]] && rm -f -- "$partial_path"
  fi
  cleanup_lock
  exit "$status"
}
trap on_exit EXIT
trap 'failure_reason=interrupted; exit 130' HUP INT TERM

partial_record_is_valid() {
  local existing=$1
  [[ -f "$existing" && ! -L "$existing" && "$(mode_of "$existing")" == 600 ]] || return 1
  jq -e --arg mode "$mode" '
    type == "object" and
    (keys | sort) == ["attempt_id","candidate","cutover_applied","cutover_attempted","mode","phase","previous","schema_version","started_epoch","worker_updated"] and
    .schema_version == 1 and (.attempt_id | type == "string" and length > 0) and
    .mode == $mode and
    (.started_epoch | type == "number" and floor == .) and
    (.phase | type == "string" and length > 0) and
    (.cutover_attempted | type == "boolean") and (.cutover_applied | type == "boolean") and
    (.worker_updated | type == "boolean") and
    (.previous | type == "object" and
      (keys | sort) == ["active_slot","active_upstream","blue_image","caddy_id","green_image","migrations_hash","postgres_id","redis_id","source_commit","source_tree","worker_image"] and
      (.active_slot == "blue" or .active_slot == "green") and
      ((.active_slot == "blue" and .active_upstream == "sub2api-blue:8080") or
       (.active_slot == "green" and .active_upstream == "sub2api-green:8080")) and
      ([.blue_image,.green_image,.worker_image] | all(type == "string" and test("^[^[:space:]@]+@sha256:[a-f0-9]{64}$"))) and
      (.source_commit | type == "string" and test("^[a-f0-9]{40}$")) and
      (.source_tree | type == "string" and test("^[a-f0-9]{40}$")) and
      (.migrations_hash | type == "string" and test("^[a-f0-9]{64}$")) and
      ([.postgres_id,.redis_id,.caddy_id] | all(type == "string" and length > 0))) and
    (.candidate | type == "object" and (keys | sort) == ["image","slot","upstream"] and
      (.slot == "blue" or .slot == "green") and
      ((.slot == "blue" and .upstream == "sub2api-blue:8080") or
       (.slot == "green" and .upstream == "sub2api-green:8080")) and
      (.image | type == "string" and test("^[^[:space:]@]+@sha256:[a-f0-9]{64}$"))) and
    .previous.active_slot != .candidate.slot
  ' "$existing" >/dev/null 2>&1
}

recover_partial() {
  local existing=$1 now age recovery_cutover_attempted recovery_cutover recovery_worker
  partial_record_is_valid "$existing" || fail 'stale or invalid partial release record is present'
  now=$(date -u +%s)
  age=$((now - $(jq -r '.started_epoch' "$existing")))
  [[ "$age" -ge 0 && "$age" -le 1800 ]] || fail 'stale partial release record is present'
  previous_slot=$(jq -r '.previous.active_slot' "$existing")
  previous_upstream=$(jq -r '.previous.active_upstream' "$existing")
  previous_worker_image=$(jq -r '.previous.worker_image' "$existing")
  rollback_blue_image=$(jq -r '.previous.blue_image' "$existing")
  rollback_green_image=$(jq -r '.previous.green_image' "$existing")
  rollback_source_commit=$(jq -r '.previous.source_commit' "$existing")
  rollback_source_tree=$(jq -r '.previous.source_tree' "$existing")
  rollback_migrations_hash=$(jq -r '.previous.migrations_hash' "$existing")
  rollback_postgres_id=$(jq -r '.previous.postgres_id' "$existing")
  rollback_redis_id=$(jq -r '.previous.redis_id' "$existing")
  rollback_caddy_id=$(jq -r '.previous.caddy_id' "$existing")
  candidate_slot=$(jq -r '.candidate.slot' "$existing")
  candidate_upstream=$(jq -r '.candidate.upstream' "$existing")
  recovery_cutover_attempted=$(jq -r '.cutover_attempted' "$existing")
  recovery_cutover=$(jq -r '.cutover_applied' "$existing")
  recovery_worker=$(jq -r '.worker_updated' "$existing")
  validate_upstream "$previous_upstream" || fail 'partial record previous upstream is invalid'
  [[ "$previous_worker_image" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]] || fail 'partial record previous worker image is invalid'
  cutover_attempted=$recovery_cutover_attempted
  cutover_applied=$recovery_cutover
  state_persisted=$recovery_cutover
  persistence_started=$recovery_cutover
  worker_update_started=$recovery_worker
  failure_reason=interrupted_release_recovered
  partial_path=$existing
  if restore_previous; then
    write_final_record failed rolled_back "$failure_reason"
    rm -f -- "$existing"
  else
    write_final_record failed rollback_failed "$failure_reason"
  fi
  cleanup_lock
  exit 1
}

partial_is_committed_success() {
  local existing=$1 partial_attempt partial_slot partial_upstream partial_image success_record state_image
  partial_record_is_valid "$existing" || return 1
  partial_attempt=$(jq -r '.attempt_id // empty' "$existing" 2>/dev/null) || return 1
  partial_slot=$(jq -r '.candidate.slot // empty' "$existing" 2>/dev/null) || return 1
  partial_upstream=$(jq -r '.candidate.upstream // empty' "$existing" 2>/dev/null) || return 1
  partial_image=$(jq -r '.candidate.image // empty' "$existing" 2>/dev/null) || return 1
  [[ "$partial_attempt" =~ ^[A-Za-z0-9._-]+$ ]] || return 1
  case "$partial_slot:$partial_upstream" in
    blue:sub2api-blue:8080) state_image=$state_blue_image ;;
    green:sub2api-green:8080) state_image=$state_green_image ;;
    *) return 1 ;;
  esac
  [[ "$state_active_slot" == "$partial_slot" && "$state_active_upstream" == "$partial_upstream" && "$state_image" == "$partial_image" ]] \
    || return 1
  success_record="$record_root/$partial_attempt.json"
  [[ -f "$success_record" && ! -L "$success_record" && "$(mode_of "$success_record")" == 600 ]] || return 1
  jq -e --arg attempt_id "$partial_attempt" --arg mode "$mode" --arg image "$partial_image" '
    type == "object" and .schema_version == 1 and .attempt_id == $attempt_id and .mode == $mode and
    .result == "succeeded" and .state == "promoted" and (.requested | type == "object") and .requested.image == $image
  ' "$success_record" >/dev/null 2>&1
}

existing_partials=$(find "$record_root" -maxdepth 1 -type f -name '*.partial' -print)
partial_count=$(printf '%s\n' "$existing_partials" | awk 'NF { count++ } END { print count + 0 }')
[[ "$partial_count" -le 1 ]] || fail 'multiple partial release records are present'

if [[ ! -e "$release_state" ]]; then
  gate legacy_topology_bootstrap 'steady-state blue-green release state is absent; bootstrap requires maintenance authorization' 600
fi
[[ -f "$release_state" && ! -L "$release_state" ]] || fail 'RELEASE_STATE must be a regular non-symlink file'
[[ "$(mode_of "$release_state")" == 600 ]] || fail 'RELEASE_STATE mode must be 0600'

duplicate_state_keys=$(jq -r --stream 'select(length == 2 and (.[0] | length) == 1) | .[0][0]' "$release_state" 2>/dev/null | sort | uniq -d)
[[ -z "$duplicate_state_keys" ]] || fail 'RELEASE_STATE contains duplicate top-level keys'
jq -e '
  type == "object" and
  (keys | sort) == ["active_slot","active_upstream","blue_image","caddy_id","green_image","migrations_hash","postgres_id","redis_id","schema_version","source_commit","source_tree","worker_image"] and
  .schema_version == 1 and
  (.active_slot == "blue" or .active_slot == "green") and
  (.active_upstream == "sub2api-blue:8080" or .active_upstream == "sub2api-green:8080") and
  ([.blue_image,.green_image,.worker_image] | all(type == "string" and test("^[^[:space:]@]+@sha256:[a-f0-9]{64}$"))) and
  (.source_commit | type == "string" and test("^[a-f0-9]{40}$")) and
  (.source_tree | type == "string" and test("^[a-f0-9]{40}$")) and
  (.migrations_hash | type == "string" and test("^[a-f0-9]{64}$")) and
  ([.postgres_id,.redis_id,.caddy_id] | all(type == "string" and length > 0))
' "$release_state" >/dev/null 2>&1 || fail 'RELEASE_STATE schema is invalid'

state_active_slot=$(jq -r '.active_slot' "$release_state")
state_active_upstream=$(jq -r '.active_upstream' "$release_state")
state_blue_image=$(jq -r '.blue_image' "$release_state")
state_green_image=$(jq -r '.green_image' "$release_state")
state_worker_image=$(jq -r '.worker_image' "$release_state")
state_source_commit=$(jq -r '.source_commit' "$release_state")
state_source_tree=$(jq -r '.source_tree' "$release_state")
state_migrations_hash=$(jq -r '.migrations_hash' "$release_state")
state_postgres_id=$(jq -r '.postgres_id' "$release_state")
state_redis_id=$(jq -r '.redis_id' "$release_state")
state_caddy_id=$(jq -r '.caddy_id' "$release_state")

if [[ "$partial_count" == 1 ]]; then
  if partial_is_committed_success "$existing_partials"; then
    rm -f -- "$existing_partials"
    partial_count=0
  else
    recover_partial "$existing_partials"
  fi
fi

# Recovery above depends only on protected checkpoint/state data. New image
# availability and provenance must never block repairing an interrupted release.
image_json=$(docker image inspect "$requested_image") || fail 'could not inspect requested image'
jq -e \
  --arg image "$requested_image" --arg source_commit "$source_commit" \
  --arg source_tree "$source_tree" --arg tested_tree "$tested_tree" \
  --arg migrations_hash "$migrations_hash" '
  length == 1 and
  (.[0].RepoDigests | type == "array" and index($image) != null) and
  .[0].Config.Labels["com.xingqiao.sub2api.qualified"] == "true" and
  .[0].Config.Labels["com.xingqiao.sub2api.source.commit"] == $source_commit and
  .[0].Config.Labels["com.xingqiao.sub2api.source.tree"] == $source_tree and
  .[0].Config.Labels["com.xingqiao.sub2api.tested.tree"] == $tested_tree and
  .[0].Config.Labels["com.xingqiao.sub2api.migrations.sha256"] == $migrations_hash
' <<<"$image_json" >/dev/null || fail 'requested image labels do not match qualified source/test evidence'

case "$state_active_slot:$state_active_upstream" in
  blue:sub2api-blue:8080) candidate_slot=green; candidate_upstream=sub2api-green:8080 ;;
  green:sub2api-green:8080) candidate_slot=blue; candidate_upstream=sub2api-blue:8080 ;;
  *) gate invalid_active_slot_upstream 'active slot and Caddy upstream are not an allowlisted matching pair' 300 ;;
esac
previous_slot=$state_active_slot
previous_upstream=$state_active_upstream
previous_worker_image=$state_worker_image
rollback_blue_image=$state_blue_image
rollback_green_image=$state_green_image
rollback_source_commit=$state_source_commit
rollback_source_tree=$state_source_tree
rollback_migrations_hash=$state_migrations_hash
rollback_postgres_id=$state_postgres_id
rollback_redis_id=$state_redis_id
rollback_caddy_id=$state_caddy_id
validate_upstream "$candidate_upstream" || gate invalid_candidate_upstream 'candidate Caddy upstream is not allowlisted' 300

live_upstream=$(live_caddy_upstream) || fail 'live Caddy upstream is not uniquely resolvable'
[[ "$live_upstream" == "$state_active_upstream" ]] || fail 'live Caddy upstream does not match release state'

[[ "$(managed_env_value SUB2API_BLUE_IMAGE)" == "$state_blue_image" ]] || fail 'RELEASE_ENV blue image does not match state'
[[ "$(managed_env_value SUB2API_GREEN_IMAGE)" == "$state_green_image" ]] || fail 'RELEASE_ENV green image does not match state'
[[ "$(managed_env_value SUB2API_WORKER_IMAGE)" == "$state_worker_image" ]] || fail 'RELEASE_ENV worker image does not match state'
[[ "$(managed_env_value SUB2API_ACTIVE_UPSTREAM)" == "$state_active_upstream" ]] || fail 'RELEASE_ENV active upstream does not match state'
[[ "$(managed_env_value SUB2API_ACTIVE_SLOT)" == "$state_active_slot" ]] || fail 'RELEASE_ENV active slot does not match state'
state_previous_slot=green
[[ "$state_active_slot" == green ]] && state_previous_slot=blue
[[ "$(managed_env_value SUB2API_PREVIOUS_SLOT)" == "$state_previous_slot" ]] || fail 'RELEASE_ENV previous slot does not match state'

if [[ "$migrations_hash" != "$state_migrations_hash" ]]; then
  gate migration_set_changed 'candidate migration set differs from the active release' 300
fi

postgres_id=$(resolve_container_id postgres) || gate legacy_topology_bootstrap 'PostgreSQL container identity is not uniquely resolvable' 600
redis_id=$(resolve_container_id redis) || gate legacy_topology_bootstrap 'Redis container identity is not uniquely resolvable' 600
caddy_id=$(resolve_container_id caddy) || gate legacy_topology_bootstrap 'Caddy container identity is not uniquely resolvable' 600
[[ "$postgres_id" == "$state_postgres_id" && "$redis_id" == "$state_redis_id" && "$caddy_id" == "$state_caddy_id" ]] \
  || gate shared_container_identity_changed 'PostgreSQL, Redis, or Caddy identity differs from the active release state' 600

active_service="sub2api-$state_active_slot"
active_image=$state_blue_image
[[ "$state_active_slot" == green ]] && active_image=$state_green_image
active_container_id=$(resolve_container_id "$active_service") \
	|| gate invalid_runtime_cardinality 'active API container identity is not uniquely resolvable' 600
worker_container_id=$(resolve_container_id sub2api-worker) \
	|| gate invalid_runtime_cardinality 'worker container identity is not uniquely resolvable' 600
[[ "$(docker inspect "$active_container_id" --format '{{.Config.Image}}')" == "$active_image" ]] \
	|| gate active_runtime_drift 'active API image differs from release state' 600
[[ "$(docker inspect "$worker_container_id" --format '{{.Config.Image}}')" == "$state_worker_image" ]] \
	|| gate active_runtime_drift 'worker image differs from release state' 600
[[ "$(container_role "$active_container_id")" == api ]] \
	|| gate invalid_runtime_role 'active API runtime role is not api' 600
[[ "$(container_role "$worker_container_id")" == worker ]] \
	|| gate invalid_runtime_role 'worker runtime role is not worker' 600
legacy_all_ids=$(docker ps -q --filter "label=com.docker.compose.project=$compose_project" \
	--filter label=com.docker.compose.service=sub2api) || fail 'legacy runtime lookup failed'
[[ -z "$legacy_all_ids" ]] || gate invalid_runtime_cardinality 'legacy all-role runtime is still present' 600

available_kb=$(df -Pk "$deploy_root" | awk 'NR == 2 { print $4 }')
[[ "$available_kb" =~ ^[0-9]+$ && "$available_kb" -ge "${MIN_FREE_KB:-2097152}" ]] \
  || gate insufficient_disk_headroom 'less than 2 GiB disk headroom is available for parallel release operation' 300

meminfo_file=${MEMINFO_FILE:-/proc/meminfo}
[[ -f "$meminfo_file" && -r "$meminfo_file" && ! -L "$meminfo_file" ]] || fail 'memory headroom source is invalid'
memory_kb=$(awk '$1 == "MemAvailable:" { print $2; exit }' "$meminfo_file")
[[ "$memory_kb" =~ ^[0-9]+$ && "$memory_kb" -ge "${MIN_AVAILABLE_MEMORY_KB:-1048576}" ]] \
  || gate insufficient_memory_headroom 'less than 1 GiB available memory remains for the parallel API slot' 300

db_headroom=$("${compose_current[@]}" exec -T postgres sh -c \
  'exec psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "select current_setting('"'"'max_connections'"'"')::int - count(*)::int from pg_stat_activity"') \
  || gate insufficient_db_connection_headroom 'database connection headroom could not be proved' 300
db_headroom=$(printf '%s' "$db_headroom" | tr -d '[:space:]')
[[ "$db_headroom" =~ ^[0-9]+$ && "$db_headroom" -ge "${MIN_DB_CONNECTION_HEADROOM:-10}" ]] \
  || gate insufficient_db_connection_headroom 'fewer than 10 PostgreSQL connections remain available' 300

candidate_env="$record_root/.$attempt_id.candidate.env"
cp "$release_env" "$candidate_env"
chmod 0600 "$candidate_env"
candidate_blue=$state_blue_image
candidate_green=$state_green_image
if [[ "$candidate_slot" == blue ]]; then candidate_blue=$requested_image; else candidate_green=$requested_image; fi
awk \
  -v blue="$candidate_blue" -v green="$candidate_green" '
  /^SUB2API_BLUE_IMAGE=/ { print "SUB2API_BLUE_IMAGE=" blue; next }
  /^SUB2API_GREEN_IMAGE=/ { print "SUB2API_GREEN_IMAGE=" green; next }
  { print }
' "$candidate_env" >"$candidate_env.tmp"
chmod 0600 "$candidate_env.tmp"
mv "$candidate_env.tmp" "$candidate_env"
compose_candidate=(docker compose --project-name "$compose_project" --project-directory "$deploy_root"
  --env-file "$secret_env" --env-file "$candidate_env" -f "$base_compose")
candidate_config=$("${compose_candidate[@]}" config --format json) || gate invalid_candidate_topology 'candidate Compose topology could not be rendered' 600
active_image=$state_blue_image
[[ "$state_active_slot" == green ]] && active_image=$state_green_image
jq -e --arg service "sub2api-$candidate_slot" --arg active_service "sub2api-$state_active_slot" \
  --arg image "$requested_image" --arg active_image "$active_image" --arg worker_image "$state_worker_image" '
  .services[$service].image == $image and
  .services[$active_service].image == $active_image and
  .services["sub2api-worker"].image == $worker_image
' <<<"$candidate_config" >/dev/null 2>&1 || gate invalid_candidate_topology 'candidate Compose image selection is not exact' 600
jq -e --arg service "sub2api-$candidate_slot" --arg active_service "sub2api-$state_active_slot" '
  .services[$service].environment.SERVER_PROCESS_ROLE == "api" and
  .services[$active_service].environment.SERVER_PROCESS_ROLE == "api" and
  .services["sub2api-worker"].environment.SERVER_PROCESS_ROLE == "worker"
' <<<"$candidate_config" >/dev/null 2>&1 || gate candidate_role_not_api 'inactive candidate slot is not configured with SERVER_PROCESS_ROLE=api' 600
rm -f -- "$candidate_env"
candidate_env=''

partial_path="$record_root/$attempt_id.partial"
write_partial preflight_complete

failure_reason=candidate_pull_failed
candidate_env="$record_root/.$attempt_id.candidate.env"
cp "$release_env" "$candidate_env"
chmod 0600 "$candidate_env"
awk -v blue="$candidate_blue" -v green="$candidate_green" '
  /^SUB2API_BLUE_IMAGE=/ { print "SUB2API_BLUE_IMAGE=" blue; next }
  /^SUB2API_GREEN_IMAGE=/ { print "SUB2API_GREEN_IMAGE=" green; next }
  { print }
' "$candidate_env" >"$candidate_env.tmp"
chmod 0600 "$candidate_env.tmp"
mv "$candidate_env.tmp" "$candidate_env"
compose_candidate=(docker compose --project-name "$compose_project" --project-directory "$deploy_root"
  --env-file "$secret_env" --env-file "$candidate_env" -f "$base_compose")
"${compose_candidate[@]}" pull "sub2api-$candidate_slot" >/dev/null

failure_reason=candidate_start_failed
"${compose_candidate[@]}" up --no-deps -d "sub2api-$candidate_slot" >/dev/null
write_partial candidate_started
	wait_for_candidate_healthy "sub2api-$candidate_slot" || fail 'candidate did not become healthy before timeout'

write_acceptance_headers
network_name="${compose_project}_default"
candidate_url="http://sub2api-$candidate_slot:8080"
failure_reason=candidate_acceptance_failed
docker run --rm --network "$network_name" "$network_curl_image" -fsS --connect-timeout 5 --max-time 15 "$candidate_url/health" | \
  jq -e '.status == "ok"' >/dev/null
docker run --rm --network "$network_name" -v "$admin_header:/run/key:ro" "$network_curl_image" \
  -fsS --connect-timeout 5 --max-time 15 -H @/run/key "$candidate_url/api/v1/admin/system/version" | \
  jq -e '(.data // .).version | type == "string" and length > 0' >/dev/null
docker run --rm --network "$network_name" "$network_curl_image" -fsS --connect-timeout 5 --max-time 15 \
  "$candidate_url/api/v1/settings/public" | jq -e 'type == "object"' >/dev/null
docker run --rm --network "$network_name" -v "$gateway_header:/run/key:ro" "$network_curl_image" \
  -fsS --connect-timeout 5 --max-time 15 -H @/run/key "$candidate_url/v1/models" | \
  jq -e '.data | type == "array"' >/dev/null
write_partial candidate_accepted

failure_reason=caddy_validate_failed
"${compose_current[@]}" exec -T -e "SUB2API_ACTIVE_UPSTREAM=$candidate_upstream" caddy \
  caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null
write_partial caddy_validated

failure_reason=caddy_reload_failed
cutover_attempted=true
write_partial cutover_attempted
"${compose_current[@]}" exec -T -e "SUB2API_ACTIVE_UPSTREAM=$candidate_upstream" caddy \
  caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null
cutover_applied=true
write_partial cutover_applied

failure_reason=public_acceptance_failed
public_acceptance
write_partial public_accepted

failure_reason=state_persist_failed
persistence_started=true
write_partial state_persisting
write_release_env_values "$candidate_blue" "$candidate_green" "$requested_image" \
  "$candidate_upstream" "$candidate_slot" "$previous_slot"
trace_event 'persist release-env'
write_state_values "$candidate_slot" "$candidate_upstream" "$candidate_blue" "$candidate_green" \
  "$requested_image" "$source_commit" "$source_tree" "$migrations_hash" \
  "$postgres_id" "$redis_id" "$caddy_id"
trace_event 'persist release-state'
state_persisted=true
write_partial state_persisted
[[ "$(live_caddy_upstream)" == "$candidate_upstream" ]] || fail 'persisted route does not match live Caddy upstream'

failure_reason=worker_update_failed
worker_update_started=true
write_partial worker_updating
"${compose_current[@]}" up --no-deps -d --force-recreate sub2api-worker >/dev/null
wait_for_worker_healthy || fail 'worker did not become healthy before timeout'
worker_logs_are_acceptable || fail 'worker logs contain a startup failure'
write_partial worker_accepted

failure_reason=final_identity_check_failed
[[ "$(resolve_container_id postgres)" == "$postgres_id" ]] || fail 'PostgreSQL identity changed during release'
[[ "$(resolve_container_id redis)" == "$redis_id" ]] || fail 'Redis identity changed during release'
[[ "$(resolve_container_id caddy)" == "$caddy_id" ]] || fail 'Caddy identity changed during release'

write_final_record succeeded promoted ''
trace_event 'persist success-record'
record_finalized=true
rm -f -- "$partial_path" "$candidate_env" "$admin_header" "$gateway_header"
partial_path=''
candidate_env=''
cleanup_lock
printf '{"schema_version":1,"downtime_required":false,"result":"succeeded","active_slot":"%s","active_upstream":"%s","image":"%s"}\n' \
  "$candidate_slot" "$candidate_upstream" "$requested_image"
