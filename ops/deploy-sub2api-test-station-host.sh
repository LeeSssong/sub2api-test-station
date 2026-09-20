#!/usr/bin/env bash
set -euo pipefail
umask 077

fail(){ printf 'test_station_host status=failed: %s\n' "$1" >&2; exit 1; }
sha256_file(){ if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1"|awk '{print $1}'; else shasum -a 256 "$1"|awk '{print $1}'; fi; }
stat_mode(){
  local mode
  if mode=$(stat -c '%a' "$1" 2>/dev/null); then
    printf '%s\n' "$mode"
    return 0
  fi
  stat -f '%Lp' "$1"
}

reject_symlink_components(){
  local value=$1 label=$2 current='' component
  local -a components
  [[ "$value" == /* ]] || fail "$label must be absolute"
  IFS='/' read -r -a components <<<"${value#/}"
  for component in "${components[@]}"; do
    [[ -n "$component" && "$component" != . && "$component" != .. ]] || fail "$label must be canonical"
    current="$current/$component"
    [[ ! -L "$current" ]] || fail "$label must not contain symlinks"
  done
}

canonical_directory(){
  local value=$1 label=$2 physical
  reject_symlink_components "$value" "$label"
  [[ -d "$value" && ! -L "$value" ]] || fail "$label is invalid"
  physical=$(cd "$value" && pwd -P)
  [[ "$physical" == "$value" ]] || fail "$label must be canonical"
  printf '%s\n' "$physical"
}

canonical_file(){
  local value=$1 label=$2 parent physical
  reject_symlink_components "$value" "$label"
  [[ -f "$value" && -r "$value" && ! -L "$value" ]] || fail "$label is invalid"
  parent=$(dirname "$value")
  physical=$(cd "$parent" && pwd -P)/$(basename "$value")
  [[ "$physical" == "$value" ]] || fail "$label must be canonical"
  printf '%s\n' "$value"
}

staging_root='' image_archive='' image_sha256='' image_id='' compose_file='' caddy_file=''
backup_script='' source_commit='' source_tree='' migration_set_sha256='' deploy_root=''
while (($#)); do
  case "$1" in
    --staging-root) (($# >= 2)) || fail '--staging-root requires a value'; staging_root=$2; shift 2 ;;
    --image-archive) (($# >= 2)) || fail '--image-archive requires a value'; image_archive=$2; shift 2 ;;
    --image-sha256) (($# >= 2)) || fail '--image-sha256 requires a value'; image_sha256=$2; shift 2 ;;
    --image-id) (($# >= 2)) || fail '--image-id requires a value'; image_id=$2; shift 2 ;;
    --compose) (($# >= 2)) || fail '--compose requires a value'; compose_file=$2; shift 2 ;;
    --caddy) (($# >= 2)) || fail '--caddy requires a value'; caddy_file=$2; shift 2 ;;
    --backup-script) (($# >= 2)) || fail '--backup-script requires a value'; backup_script=$2; shift 2 ;;
    --source-commit) (($# >= 2)) || fail '--source-commit requires a value'; source_commit=$2; shift 2 ;;
    --source-tree) (($# >= 2)) || fail '--source-tree requires a value'; source_tree=$2; shift 2 ;;
    --migration-set-sha256) (($# >= 2)) || fail '--migration-set-sha256 requires a value'; migration_set_sha256=$2; shift 2 ;;
    --deploy-root) (($# >= 2)) || fail '--deploy-root requires a value'; deploy_root=$2; shift 2 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

staging_root=$(canonical_directory "$staging_root" 'staging root')
for value in "$image_archive" "$compose_file" "$caddy_file" "$backup_script"; do
  [[ "$value" == "$staging_root"/* ]] || fail 'bundle file is outside staging root'
done
image_archive=$(canonical_file "$image_archive" 'image archive')
compose_file=$(canonical_file "$compose_file" 'Compose file')
caddy_file=$(canonical_file "$caddy_file" 'Caddyfile')
backup_script=$(canonical_file "$backup_script" 'backup script')
[[ "$image_sha256" =~ ^[a-f0-9]{64}$ && "$(sha256_file "$image_archive")" == "$image_sha256" ]] || fail 'image archive checksum mismatch'
[[ "$image_id" =~ ^sha256:[a-f0-9]{64}$ ]] || fail 'image ID is invalid'
[[ "$source_commit" =~ ^[a-f0-9]{40}$ && "$source_tree" =~ ^[a-f0-9]{40}$ ]] || fail 'source identity is invalid'
[[ "$migration_set_sha256" =~ ^[a-f0-9]{64}$ ]] || fail 'migration set identity is invalid'
if [[ "${TEST_STATION_TEST_MODE:-false}" != true ]]; then
  [[ "$deploy_root" == /opt/sub2api-test-station ]] || fail 'deploy root is not the independent test station'
fi
deploy_root=$(canonical_directory "$deploy_root" 'deploy root')
grep -Eq '^name:[[:space:]]*sub2api-test-station[[:space:]]*$' "$compose_file" || fail 'Compose project identity mismatch'
grep -q 'sub2api-test-station-network' "$compose_file" || fail 'Compose network identity mismatch'

docker_bin=${DOCKER_BIN:-docker}
command -v "$docker_bin" >/dev/null 2>&1 || fail 'Docker is required'
state=${RELEASE_STATE:-$deploy_root/release-state.json}
state=$(canonical_file "$state" 'release state')
[[ "$(stat_mode "$state")" == 600 ]] || fail 'release state mode must be 0600'

active_api_container=$($docker_bin ps \
  --filter label=com.docker.compose.project=sub2api-test-station \
  --filter label=com.docker.compose.service=test-station-api \
  --format '{{.ID}}' | head -n 1)
[[ -n "$active_api_container" ]] || fail 'active API container is missing'
active_compose=$($docker_bin ps \
  --filter label=com.docker.compose.project=sub2api-test-station \
  --filter label=com.docker.compose.service=test-station-api \
  --format '{{.Label "com.docker.compose.project.config_files"}}' | head -n 1)
[[ "$active_compose" == "$deploy_root"/releases/*/compose.yaml ]] || fail 'active API Compose is invalid'
active_compose=$(canonical_file "$active_compose" 'active API Compose')
previous_release_dir=$(dirname "$active_compose")

IFS=$'\t' read -r previous_commit previous_tree state_release state_project < <(
  python3 - "$state" <<'PY'
import json,sys
try:
    value=json.load(open(sys.argv[1],encoding="utf-8"))
    fields=[value.get("source_commit"),value.get("source_tree"),value.get("release_dir"),value.get("project_name")]
    if not all(isinstance(v,str) and v for v in fields): raise ValueError("invalid state")
    print("\t".join(fields))
except Exception:
    raise SystemExit(1)
PY
) || fail 'release state is invalid'
[[ "$previous_commit" =~ ^[a-f0-9]{40}$ && "$previous_tree" =~ ^[a-f0-9]{40}$ ]] || fail 'previous source identity is invalid'
[[ "$state_project" == sub2api-test-station ]] || fail 'release state project mismatch'
[[ "$state_release" == "$previous_release_dir" ]] || fail 'release state does not match active API Compose'
for name in compose.yaml Caddyfile .env; do
  canonical_file "$previous_release_dir/$name" "previous $name" >/dev/null
done
previous_env="$previous_release_dir/.env"
[[ "$(stat_mode "$previous_env")" == 600 ]] || fail 'previous env mode must be 0600'
grep -Eq '^name:[[:space:]]*sub2api-test-station[[:space:]]*$' "$active_compose" || fail 'previous Compose project mismatch'
previous_image=$(awk -F= '/^CLONE_APP_IMAGE=/{value=substr($0,index($0,"=")+1)} END{print value}' "$previous_env")
[[ "$previous_image" == "sub2api-test-station-runtime:$previous_commit" ]] || fail 'previous image tag does not match release state'
previous_image_id=$($docker_bin image inspect --format '{{.Id}}' "$previous_image" 2>/dev/null | tr -d '[:space:]')
[[ "$previous_image_id" =~ ^sha256:[a-f0-9]{64}$ ]] || fail 'previous image is missing'
active_image_id=$($docker_bin inspect --format '{{.Image}}' "$active_api_container" 2>/dev/null | tr -d '[:space:]')
[[ "$active_image_id" == "$previous_image_id" ]] || fail 'previous image does not match active API container'

backup_timestamp=${TEST_STATION_BACKUP_TIMESTAMP:-$(date -u +%Y%m%dT%H%M%SZ)}
backup_output=$(EVENT_LOG="${EVENT_LOG:-}" FAKE_MODE="${FAKE_MODE:-}" DEPLOY_ROOT="$deploy_root" \
  bash "$backup_script" --compose "$active_compose" --env-file "$previous_env" \
  --deploy-root "$deploy_root" --timestamp "$backup_timestamp") || fail 'pre-release backup failed'
[[ "$backup_output" == 'test_station_backup status=succeeded backup_dir='* ]] || fail 'backup helper output is invalid'
backup_dir=${backup_output#test_station_backup status=succeeded backup_dir=}
[[ "$backup_dir" == "$deploy_root"/backups/* && -d "$backup_dir" && ! -L "$backup_dir" ]] || fail 'backup directory is invalid'

release_dir="$deploy_root/releases/$source_commit"
[[ ! -e "$release_dir" ]] || fail 'candidate release already exists'
mkdir "$release_dir"
chmod 0700 "$release_dir"
cp "$compose_file" "$release_dir/compose.yaml"
cp "$caddy_file" "$release_dir/Caddyfile"
awk '!/^CLONE_SOURCE_COMMIT=|^CLONE_APP_IMAGE=/' "$previous_env" >"$release_dir/.env"
printf 'CLONE_SOURCE_COMMIT=%s\nCLONE_APP_IMAGE=sub2api-test-station-runtime:%s\n' "$source_commit" "$source_commit" >>"$release_dir/.env"
chmod 0600 "$release_dir/.env"

"$docker_bin" load --input "$image_archive" >/dev/null || fail 'image load failed'
image_tag="sub2api-test-station-runtime:$source_commit"
actual_image_id=$($docker_bin image inspect --format '{{.Id}}' "$image_tag" 2>/dev/null | tr -d '[:space:]')
[[ "$actual_image_id" == "$image_id" ]] || fail 'loaded image identity mismatch'

candidate_compose=("$docker_bin" compose --project-name sub2api-test-station --env-file "$release_dir/.env" -f "$release_dir/compose.yaml")
previous_compose=("$docker_bin" compose --project-name sub2api-test-station --env-file "$previous_env" -f "$active_compose")
"${candidate_compose[@]}" config --quiet >/dev/null || fail 'candidate Compose preflight failed'
"${previous_compose[@]}" config --quiet >/dev/null || fail 'previous Compose preflight failed'

probe_bin=${TEST_STATION_PROBE_BIN:-}
probe_attempts=${TEST_STATION_PROBE_ATTEMPTS:-12}
probe_consecutive=${TEST_STATION_PROBE_CONSECUTIVE:-3}
probe_interval=${TEST_STATION_PROBE_INTERVAL_SECONDS:-5}
service_attempts=${TEST_STATION_SERVICE_ATTEMPTS:-30}
service_interval=${TEST_STATION_SERVICE_INTERVAL_SECONDS:-2}
[[ "$probe_attempts" =~ ^[1-9][0-9]*$ && "$probe_consecutive" =~ ^[1-9][0-9]*$ ]] || fail 'probe counts are invalid'
[[ "$service_attempts" =~ ^[1-9][0-9]*$ ]] || fail 'service attempts are invalid'
[[ "$probe_interval" =~ ^[0-9]+$ && "$service_interval" =~ ^[0-9]+$ ]] || fail 'wait intervals are invalid'
if [[ "${TEST_STATION_TEST_MODE:-false}" != true ]]; then
  ((probe_interval > 0 && service_interval > 0)) || fail 'wait intervals must be positive'
fi

probe_request(){
  local path=$1 output status content_type body
  if [[ -n "$probe_bin" ]]; then
    output=$($probe_bin "http://127.0.0.1$path") || return 1
  else
    output=$(curl --silent --show-error --max-time 5 \
      --write-out $'\n%{http_code}\t%{content_type}' "http://127.0.0.1$path") || return 1
    body=${output%$'\n'*}
    output=${output##*$'\n'}
    output+=$'\t'"$body"
  fi
  IFS=$'\t' read -r status content_type body <<<"$output"
  [[ "$status" == 200 && "$content_type" == application/json* ]] || return 1
  python3 - "$path" "$body" <<'PY' >/dev/null 2>&1
import json,sys
path,raw=sys.argv[1:]
value=json.loads(raw)
expected="ok" if path=="/health" else "ready"
raise SystemExit(0 if value=={"status":expected} else 1)
PY
}

wait_for_probes(){
  local attempt consecutive=0
  for ((attempt=1; attempt<=probe_attempts; attempt++)); do
    if probe_request /health && probe_request /readyz; then
      consecutive=$((consecutive+1))
      ((consecutive >= probe_consecutive)) && return 0
    else
      consecutive=0
    fi
    ((attempt < probe_attempts)) && sleep "$probe_interval"
  done
  return 1
}

check_services(){
  local target=$1 service container_id health_value status_value
  for service in test-station-api test-station-worker test-station-detector test-station-postgres test-station-redis; do
    if [[ "$target" == candidate ]]; then
      container_id=$("${candidate_compose[@]}" ps -q "$service" 2>/dev/null || true)
    else
      container_id=$("${previous_compose[@]}" ps -q "$service" 2>/dev/null || true)
    fi
    [[ -n "$container_id" && "$container_id" != *$'\n'* ]] || return 1
    health_value=$($docker_bin inspect --format '{{.State.Health.Status}}' "$container_id" 2>/dev/null || true)
    [[ "$health_value" == healthy ]] || return 1
  done
  if [[ "$target" == candidate ]]; then
    container_id=$("${candidate_compose[@]}" ps -q test-station-caddy 2>/dev/null || true)
  else
    container_id=$("${previous_compose[@]}" ps -q test-station-caddy 2>/dev/null || true)
  fi
  [[ -n "$container_id" && "$container_id" != *$'\n'* ]] || return 1
  status_value=$($docker_bin inspect --format '{{.State.Status}}' "$container_id" 2>/dev/null || true)
  [[ "$status_value" == running ]]
}

wait_for_services(){
  local target=$1 attempt
  for ((attempt=1; attempt<=service_attempts; attempt++)); do
    check_services "$target" && return 0
    ((attempt < service_attempts)) && sleep "$service_interval"
  done
  return 1
}

write_failure(){
  local stage=$1 rolled_back=$2 tmp
  tmp=$(mktemp "$release_dir/.failure.XXXXXX")
  chmod 0600 "$tmp"
  python3 - "$tmp" "$stage" "$rolled_back" "$source_commit" "$source_tree" "$release_dir" "$previous_release_dir" "$backup_dir" <<'PY'
import datetime,json,os,sys
path,stage,rolled,commit,tree,release,previous,backup=sys.argv[1:]
value={"schema_version":1,"result":"failed","stage":stage,"rolled_back":rolled=="true","source_commit":commit,"source_tree":tree,"release_dir":release,"previous_release_dir":previous,"backup_dir":backup,"updated_at":datetime.datetime.now(datetime.timezone.utc).isoformat()}
with open(path,"w",encoding="utf-8") as f: json.dump(value,f,separators=(",",":")); f.write("\n")
os.chmod(path,0o600)
PY
  mv -f "$tmp" "$release_dir/failure.json"
}

restore_previous(){
  "${previous_compose[@]}" up -d --remove-orphans >/dev/null || return 1
  wait_for_services previous || return 1
  wait_for_probes || return 1
}

candidate_started=false
release_committed=false
rollback_in_progress=false
failure_stage=candidate_start

rollback_uncommitted_candidate(){
  local rc=$? rolled_back=false
  if [[ "$candidate_started" == true && "$release_committed" != true && "$rollback_in_progress" != true ]]; then
    rollback_in_progress=true
    if restore_previous; then rolled_back=true; fi
    write_failure "$failure_stage" "$rolled_back" || true
  fi
  return "$rc"
}
trap rollback_uncommitted_candidate EXIT

candidate_failed(){
  local stage=$1 rolled_back=false
  rollback_in_progress=true
  if restore_previous; then rolled_back=true; fi
  write_failure "$stage" "$rolled_back" || true
  candidate_started=false
  if [[ "$rolled_back" == true ]]; then
    printf 'test_station_host status=failed stage=%s rolled_back=true\n' "$stage" >&2
  else
    printf 'test_station_host status=failed stage=%s rolled_back=false\n' "$stage" >&2
  fi
  exit 1
}

candidate_started=true
if ! "${candidate_compose[@]}" up -d --remove-orphans >/dev/null; then
  candidate_failed compose_start
fi
if ! wait_for_services candidate; then
  candidate_failed service_health
fi
if ! wait_for_probes; then
  candidate_failed readiness
fi

failure_stage=state_commit
state_dir=$(dirname "$state")
tmp_state=$(mktemp "$state_dir/.release-state.XXXXXX")
chmod 0600 "$tmp_state"
python3 - "$tmp_state" "$source_commit" "$source_tree" "$migration_set_sha256" "$image_sha256" "$image_id" "$image_tag" "$release_dir" "$previous_release_dir" "$backup_dir" <<'PY'
import datetime,json,os,sys
path,commit,tree,migrations,archive,image_id,image_tag,release,previous,backup=sys.argv[1:]
value={"schema_version":2,"source_commit":commit,"source_tree":tree,"migration_set_sha256":migrations,"image_archive_sha256":archive,"image_id":image_id,"image_tag":image_tag,"release_dir":release,"previous_release_dir":previous,"backup_dir":backup,"project_name":"sub2api-test-station","result":"succeeded","rolled_back":False,"updated_at":datetime.datetime.now(datetime.timezone.utc).isoformat()}
with open(path,"w",encoding="utf-8") as f: json.dump(value,f,separators=(",",":")); f.write("\n")
os.chmod(path,0o600)
PY
mv -f "$tmp_state" "$state"
release_committed=true
candidate_started=false
trap - EXIT
printf 'test_station_host status=succeeded source_commit=%s source_tree=%s release_dir=%s backup_dir=%s\n' "$source_commit" "$source_tree" "$release_dir" "$backup_dir"
