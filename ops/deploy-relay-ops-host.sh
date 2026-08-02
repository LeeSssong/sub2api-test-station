#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'relay_ops_host status=failed: %s\n' "$1" >&2; exit 1; }
mode=''; requested_image=''; source_commit=''; source_tree=''; tested_tree=''; migrations_hash=''; deadline_epoch=''
while (($#)); do
  case "$1" in
    --mode) (($# >= 2)) || fail '--mode requires a value'; mode=$2; shift 2 ;;
    --image) (($# >= 2)) || fail '--image requires a value'; requested_image=$2; shift 2 ;;
    --source-commit) (($# >= 2)) || fail '--source-commit requires a value'; source_commit=$2; shift 2 ;;
    --source-tree) (($# >= 2)) || fail '--source-tree requires a value'; source_tree=$2; shift 2 ;;
    --tested-tree) (($# >= 2)) || fail '--tested-tree requires a value'; tested_tree=$2; shift 2 ;;
    --migrations-hash) (($# >= 2)) || fail '--migrations-hash requires a value'; migrations_hash=$2; shift 2 ;;
    --deadline-epoch) (($# >= 2)) || fail '--deadline-epoch requires a value'; deadline_epoch=$2; shift 2 ;;
    *) fail "unknown argument: $1" ;;
  esac
done
[[ "$mode" == production ]] || fail '--mode must be production'
[[ "$requested_image" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]] || fail '--image must be immutable'
[[ "$source_commit" =~ ^[a-f0-9]{40}$ && "$source_tree" =~ ^[a-f0-9]{40}$ && "$tested_tree" == "$source_tree" && "$migrations_hash" =~ ^[a-f0-9]{64}$ ]] || fail 'release identity is invalid'
[[ "$deadline_epoch" =~ ^[1-9][0-9]{9}$ ]] || fail '--deadline-epoch is invalid'
[[ "$(id -u)" == 0 ]] || fail 'production executor must run as root'
check_deadline() { [[ "$(date -u +%s)" -lt "$deadline_epoch" ]] || fail 'release deadline exceeded'; }
check_deadline
canonical_directory() { local p=$1 label=$2; [[ "$p" == /* && -d "$p" && ! -L "$p" ]] || fail "$label is invalid"; [[ "$(cd "$p" && pwd -P)" == "$p" ]] || fail "$label must be canonical"; }
canonical_file() { local p=$1 label=$2 d; [[ "$p" == /* && -f "$p" && -r "$p" && ! -L "$p" ]] || fail "$label is invalid"; d=$(dirname "$p"); [[ "$(cd "$d" && pwd -P)/$(basename "$p")" == "$p" ]] || fail "$label must be canonical"; }
canonical_path() { local p=$1 label=$2 d; [[ "$p" == /* && ! -L "$p" ]] || fail "$label is invalid"; d=$(dirname "$p"); [[ -d "$d" && ! -L "$d" ]] || fail "$label parent is invalid"; [[ "$(cd "$d" && pwd -P)/$(basename "$p")" == "$p" ]] || fail "$label must be canonical"; }
mode_of() { stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"; }
owner_of() { stat -c '%u' "$1" 2>/dev/null || stat -f '%u' "$1"; }
secure_directory() { local p=$1 label=$2 mode; canonical_directory "$p" "$label"; [[ "$(owner_of "$p")" == 0 ]] || fail "$label must be root-owned"; mode=$(mode_of "$p"); (( (8#$mode & 8#022) == 0 )) || fail "$label must not be group/other writable"; }
secure_file() { local p=$1 label=$2 mode; canonical_file "$p" "$label"; [[ "$(owner_of "$p")" == 0 ]] || fail "$label must be root-owned"; mode=$(mode_of "$p"); (( (8#$mode & 8#022) == 0 )) || fail "$label must not be group/other writable"; }
deploy_root=${DEPLOY_ROOT:?DEPLOY_ROOT is required}; base_compose=${BASE_COMPOSE:?BASE_COMPOSE is required}; secret_env=${SECRET_ENV:?SECRET_ENV is required}; release_state=${RELEASE_STATE:?RELEASE_STATE is required}; record_root=${RELEASE_RECORD_ROOT:?RELEASE_RECORD_ROOT is required}
secure_directory "$deploy_root" DEPLOY_ROOT; secure_directory "$record_root" RELEASE_RECORD_ROOT; secure_directory "$(dirname "$base_compose")" BASE_COMPOSE_PARENT; secure_directory "$(dirname "$secret_env")" SECRET_ENV_PARENT; secure_directory "$(dirname "$release_state")" RELEASE_STATE_PARENT
secure_file "$base_compose" BASE_COMPOSE; secure_file "$secret_env" SECRET_ENV
canonical_path "$release_state" RELEASE_STATE; [[ ! -e "$release_state" || "$(owner_of "$release_state")" == 0 ]] || fail 'RELEASE_STATE must be root-owned'; [[ ! -e "$release_state" || "$(mode_of "$release_state")" == 600 ]] || fail 'RELEASE_STATE must be 0600'
[[ "$(uname -s)" == Linux ]] || fail 'production deployment must run on Linux'; [[ -z "${DOCKER_HOST:-}" && "${DOCKER_CONTEXT:-default}" == default ]] || fail 'production Docker context must be local'
docker_bin=${DOCKER_BIN:-docker}; command -v "$docker_bin" >/dev/null 2>&1 || fail 'Docker is required'; command -v perl >/dev/null 2>&1 || fail 'Perl is required'; [[ "$("$docker_bin" context show 2>/dev/null)" == default ]] || fail 'Docker context must be default'
compose=("$docker_bin" compose --project-directory "$deploy_root" --env-file "$secret_env" -f "$base_compose"); export RELAY_OPS_IMAGE="$requested_image"
stage_budget=${RELAY_OPS_STAGE_TIMEOUT_SECONDS:-120}; rollback_budget=${RELAY_OPS_ROLLBACK_TIMEOUT_SECONDS:-120}; health_budget=${RELAY_OPS_HEALTH_TIMEOUT_SECONDS:-120}; poll_seconds=${RELAY_OPS_HEALTH_POLL_SECONDS:-1}
[[ "$stage_budget" =~ ^[1-9][0-9]*$ && "$stage_budget" -le 600 && "$rollback_budget" =~ ^[1-9][0-9]*$ && "$rollback_budget" -le 600 && "$health_budget" =~ ^[1-9][0-9]*$ && "$health_budget" -le 600 && "$poll_seconds" =~ ^[1-9][0-9]*$ && "$poll_seconds" -le 30 ]] || fail 'release timeout configuration is invalid'
tmp=$(mktemp); state_tmp=''; trap 'rm -f -- "$tmp" "$state_tmp"' EXIT
stage_seconds() { local remaining=$((deadline_epoch - $(date -u +%s))); (( remaining > 0 )) || return 1; (( stage_budget < remaining )) && printf '%s' "$stage_budget" || printf '%s' "$remaining"; }
run_quiet() { local seconds; seconds=$(stage_seconds) || return 1; : >"$tmp"; perl -e 'alarm shift @ARGV; exec @ARGV' "$seconds" "$@" >"$tmp" 2>&1; }
run_capture() { local seconds; seconds=$(stage_seconds) || return 1; perl -e 'alarm shift @ARGV; exec @ARGV' "$seconds" "$@"; }
run_rollback_quiet() { : >"$tmp"; perl -e 'alarm shift @ARGV; exec @ARGV' "$rollback_budget" "$@" >"$tmp" 2>&1; }
run_rollback_capture() { perl -e 'alarm shift @ARGV; exec @ARGV' "$rollback_budget" "$@"; }
compose_service_id() { local service=$1 id; local -a ids=(); while IFS= read -r id; do [[ "$id" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]+$ ]] || return 1; ids+=("$id"); done < <(run_capture "${compose[@]}" ps -q "$service" 2>/dev/null); [[ ${#ids[@]} -eq 1 ]] || return 1; printf '%s' "${ids[0]}"; }
compose_service_id_rollback() { local service=$1 id; local -a ids=(); while IFS= read -r id; do [[ "$id" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]+$ ]] || return 1; ids+=("$id"); done < <(run_rollback_capture "${compose[@]}" ps -q "$service" 2>/dev/null); [[ ${#ids[@]} -eq 1 ]] || return 1; printf '%s' "${ids[0]}"; }
inspect_image() { run_capture "$docker_bin" inspect "$1" --format '{{.Config.Image}}' 2>/dev/null | tr -d '[:space:]'; }
inspect_image_rollback() { run_rollback_capture "$docker_bin" inspect "$1" --format '{{.Config.Image}}' 2>/dev/null | tr -d '[:space:]'; }
inspect_health() { run_capture "$docker_bin" inspect "$1" --format '{{.State.Health.Status}}' 2>/dev/null | tr -d '[:space:]'; }
inspect_health_rollback() { run_rollback_capture "$docker_bin" inspect "$1" --format '{{.State.Health.Status}}' 2>/dev/null | tr -d '[:space:]'; }
has_json_status() { local body=$1 expected=$2; printf '%s' "$body" | grep -Eq '"status"[[:space:]]*:[[:space:]]*"'"$expected"'"'; }
wait_for_health() { local id=$1 end=$(( $(date -u +%s) + health_budget )) now health; (( end > deadline_epoch )) && end=$deadline_epoch; while :; do health=$(inspect_health "$id") || return 1; [[ "$health" == healthy ]] && return 0; now=$(date -u +%s); (( now >= end )) && return 1; sleep "$poll_seconds"; done; }
wait_for_health_rollback() { local id=$1 end=$(( $(date -u +%s) + rollback_budget )) now health; while :; do health=$(inspect_health_rollback "$id") || return 1; [[ "$health" == healthy ]] && return 0; now=$(date -u +%s); (( now >= end )) && return 1; sleep "$poll_seconds"; done; }
probe_endpoints() { local runner=$1 health_body ready_body; if [[ "$runner" == rollback ]]; then health_body=$(run_rollback_capture "$docker_bin" exec "$before_caddy" wget -qO- http://relay-ops:8100/healthz 2>/dev/null) || return 1; ready_body=$(run_rollback_capture "$docker_bin" exec "$before_caddy" wget -qO- http://relay-ops:8100/readyz 2>/dev/null) || return 1; else health_body=$(run_capture "$docker_bin" exec "$before_caddy" wget -qO- http://relay-ops:8100/healthz 2>/dev/null) || return 1; ready_body=$(run_capture "$docker_bin" exec "$before_caddy" wget -qO- http://relay-ops:8100/readyz 2>/dev/null) || return 1; fi; has_json_status "$health_body" ok && has_json_status "$ready_body" ready; }
shared_services=(postgres redis caddy sub2api-blue sub2api-green sub2api-worker)
run_quiet "${compose[@]}" config --quiet || fail 'Compose preflight failed'
before_postgres=$(compose_service_id postgres) || fail 'required service is not uniquely running: postgres'
before_redis=$(compose_service_id redis) || fail 'required service is not uniquely running: redis'
before_caddy=$(compose_service_id caddy) || fail 'required service is not uniquely running: caddy'
before_blue=$(compose_service_id sub2api-blue) || fail 'required service is not uniquely running: sub2api-blue'
before_green=$(compose_service_id sub2api-green) || fail 'required service is not uniquely running: sub2api-green'
before_worker=$(compose_service_id sub2api-worker) || fail 'required service is not uniquely running: sub2api-worker'
before_relay_ops=$(compose_service_id relay-ops) || fail 'required service is not uniquely running: relay-ops'
previous_image=$(inspect_image "$before_relay_ops"); [[ "$previous_image" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]] || fail 'previous relay-ops image is not immutable'
state_parent=$(dirname "$release_state"); state_tmp=$(mktemp "$state_parent/.relay-ops-state.XXXXXX"); chmod 0600 "$state_tmp"
write_state() { local current=$1 previous=$2 result=$3 ids_json; ids_json=$(ruby -rjson -e 'h={}; ARGV.each{|x| k,v=x.split("=",2); h[k]=v}; print JSON.generate(h)' "postgres=$before_postgres" "redis=$before_redis" "caddy=$before_caddy" "sub2api-blue=$before_blue" "sub2api-green=$before_green" "sub2api-worker=$before_worker"); ruby -rjson -e 'p,c,pr,r,commit,tree,tested,mig,ids=ARGV; x={schema_version:1,service:"relay-ops",current_image:c,previous_image:pr,result:r,source_commit:commit,source_tree:tree,tested_tree:tested,migrations_hash:mig,shared_container_ids:JSON.parse(ids)}; File.write(p,JSON.generate(x)+"\n")' "$state_tmp" "$current" "$previous" "$result" "$source_commit" "$source_tree" "$tested_tree" "$migrations_hash" "$ids_json"; chmod 0600 "$state_tmp"; mv -f "$state_tmp" "$release_state"; }
write_state "$requested_image" "$previous_image" prechange
post_checks() { check_deadline; local id image; id=$(compose_service_id relay-ops) || return 1; image=$(inspect_image "$id"); [[ "$image" == "$requested_image" ]] || return 1; wait_for_health "$id" || return 1; probe_endpoints normal || return 1; [[ "$(compose_service_id postgres)" == "$before_postgres" && "$(compose_service_id redis)" == "$before_redis" && "$(compose_service_id caddy)" == "$before_caddy" && "$(compose_service_id sub2api-blue)" == "$before_blue" && "$(compose_service_id sub2api-green)" == "$before_green" && "$(compose_service_id sub2api-worker)" == "$before_worker" ]] || return 1; return 0; }
rollback() { export RELAY_OPS_IMAGE="$previous_image"; run_rollback_quiet "${compose[@]}" pull relay-ops || return 1; run_rollback_quiet "${compose[@]}" up -d --no-deps --force-recreate relay-ops || return 1; local id; id=$(compose_service_id_rollback relay-ops) || return 1; [[ "$(inspect_image_rollback "$id")" == "$previous_image" ]] || return 1; wait_for_health_rollback "$id" || return 1; probe_endpoints rollback || return 1; [[ "$(compose_service_id_rollback postgres)" == "$before_postgres" && "$(compose_service_id_rollback redis)" == "$before_redis" && "$(compose_service_id_rollback caddy)" == "$before_caddy" && "$(compose_service_id_rollback sub2api-blue)" == "$before_blue" && "$(compose_service_id_rollback sub2api-green)" == "$before_green" && "$(compose_service_id_rollback sub2api-worker)" == "$before_worker" ]] || return 1; }
if ! run_quiet "${compose[@]}" pull relay-ops || ! run_quiet "${compose[@]}" up -d --no-deps --force-recreate relay-ops || ! post_checks; then
  if rollback; then write_state "$previous_image" "$previous_image" rolled_back; printf '{"schema_version":1,"result":"failed_rolled_back","requested_image":"%s","rollback_proven":true}\n' "$requested_image"; exit 1; fi
  fail 'post-change checks failed and rollback could not be proven'
fi
write_state "$requested_image" "$previous_image" succeeded
printf '{"schema_version":1,"result":"succeeded","requested_image":"%s","previous_image":"%s","migration_startup_verified":true,"shared_services_unchanged":true,"rollback_proven":false}\n' "$requested_image" "$previous_image"
