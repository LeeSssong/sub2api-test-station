#!/usr/bin/env bash
set -euo pipefail
umask 077

fail(){ printf 'test_station_backup status=failed: %s\n' "$1" >&2; exit 1; }
stat_mode(){ stat -f '%Lp' "$1" 2>/dev/null || stat -c '%a' "$1"; }

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

compose_file='' env_file='' deploy_root='' timestamp=''
while (($#)); do
  case "$1" in
    --compose) (($# >= 2)) || fail '--compose requires a value'; compose_file=$2; shift 2 ;;
    --env-file) (($# >= 2)) || fail '--env-file requires a value'; env_file=$2; shift 2 ;;
    --deploy-root) (($# >= 2)) || fail '--deploy-root requires a value'; deploy_root=$2; shift 2 ;;
    --timestamp) (($# >= 2)) || fail '--timestamp requires a value'; timestamp=$2; shift 2 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ "$timestamp" =~ ^[0-9]{8}T[0-9]{6}Z$ ]] || fail 'timestamp must use UTC compact format'
deploy_root=$(canonical_directory "$deploy_root" 'deploy root')
compose_file=$(canonical_file "$compose_file" 'Compose file')
env_file=$(canonical_file "$env_file" 'env file')
[[ "$(stat_mode "$env_file")" == 600 ]] || fail 'env file mode must be 0600'
[[ "$compose_file" == "$deploy_root"/* && "$env_file" == "$deploy_root"/* ]] || fail 'active files must be inside deploy root'
grep -Eq '^name:[[:space:]]*sub2api-test-station[[:space:]]*$' "$compose_file" || fail 'Compose project identity mismatch'
grep -q 'sub2api-test-station-network' "$compose_file" || fail 'Compose network identity mismatch'

backup_root="$deploy_root/backups"
reject_symlink_components "$backup_root" 'backup root'
mkdir -p "$backup_root"
backup_root=$(canonical_directory "$backup_root" 'backup root')
chmod 0700 "$backup_root"
partial="$backup_root/.partial-$timestamp-$$"
final="$backup_root/$timestamp"
[[ ! -e "$final" ]] || fail 'backup set already exists'
mkdir "$partial"
chmod 0700 "$partial"

cleanup(){
  local rc=$?
  if [[ -n "${partial:-}" && -d "$partial" ]]; then
    rm -rf -- "$partial"
  fi
  return "$rc"
}
trap cleanup EXIT

docker_bin=${DOCKER_BIN:-docker}
command -v "$docker_bin" >/dev/null 2>&1 || fail 'Docker is required'
compose=("$docker_bin" compose --project-name sub2api-test-station --env-file "$env_file" -f "$compose_file")

"${compose[@]}" exec -T test-station-postgres \
  sh -c 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' >"$partial/postgres.dump"
[[ -s "$partial/postgres.dump" ]] || fail 'PostgreSQL archive is empty'
postgres_container=$("${compose[@]}" ps -q test-station-postgres)
[[ -n "$postgres_container" ]] || fail 'PostgreSQL container is missing'
"$docker_bin" cp "$partial/postgres.dump" "$postgres_container:/tmp/sub2api-test-station-postgres.dump" >/dev/null \
  || fail 'PostgreSQL archive validation staging failed'
"${compose[@]}" exec -T test-station-postgres \
  sh -c 'pg_restore --list /tmp/sub2api-test-station-postgres.dump >/dev/null 2>&1; rc=$?; rm -f /tmp/sub2api-test-station-postgres.dump; exit $rc' \
  || fail 'PostgreSQL archive validation failed'

"${compose[@]}" exec -T test-station-redis \
  sh -c 'redis-cli --no-auth-warning -a "$REDIS_PASSWORD" SAVE >/dev/null' \
  || fail 'Redis SAVE failed'
redis_container=$("${compose[@]}" ps -q test-station-redis)
[[ -n "$redis_container" ]] || fail 'Redis container is missing'
"$docker_bin" cp "$redis_container:/data/dump.rdb" "$partial/redis-dump.rdb" >/dev/null \
  || fail 'Redis snapshot copy failed'
[[ -s "$partial/redis-dump.rdb" ]] || fail 'Redis snapshot is empty'
"$docker_bin" cp "$partial/redis-dump.rdb" "$redis_container:/tmp/sub2api-test-station-redis.rdb" >/dev/null \
  || fail 'Redis snapshot validation staging failed'
"${compose[@]}" exec -T test-station-redis \
  sh -c 'redis-check-rdb /tmp/sub2api-test-station-redis.rdb >/dev/null 2>&1; rc=$?; rm -f /tmp/sub2api-test-station-redis.rdb; exit $rc' \
  || fail 'Redis snapshot validation failed'

api_container=$("${compose[@]}" ps -q test-station-api)
[[ -n "$api_container" ]] || fail 'API container is missing'
mkdir "$partial/app-data"
"$docker_bin" cp "$api_container:/app/data/." "$partial/app-data" >/dev/null \
  || fail 'app-data copy failed'
tar -C "$partial/app-data" -czf "$partial/app-data.tar.gz" .
[[ -s "$partial/app-data.tar.gz" ]] || fail 'app-data archive is empty'
tar -tzf "$partial/app-data.tar.gz" >/dev/null 2>&1 || fail 'app-data archive validation failed'
rm -rf -- "$partial/app-data"

chmod 0600 "$partial/postgres.dump" "$partial/redis-dump.rdb" "$partial/app-data.tar.gz"
(
  cd "$partial"
  sha256sum postgres.dump redis-dump.rdb app-data.tar.gz >SHA256SUMS
  sha256sum -c SHA256SUMS >/dev/null
)
chmod 0600 "$partial/SHA256SUMS"
python3 - "$partial/metadata.json" "$timestamp" <<'PY'
import json,os,sys
path,stamp=sys.argv[1:]
value={"schema_version":1,"created_at":stamp,"project_name":"sub2api-test-station","sha256_verified":True}
with open(path,"w",encoding="utf-8") as f:
    json.dump(value,f,separators=(",",":")); f.write("\n")
os.chmod(path,0o600)
PY

[[ $(find "$partial" -mindepth 1 -maxdepth 1 -type f | wc -l | tr -d ' ') == 5 ]] || fail 'backup set contents are invalid'
(cd "$partial" && sha256sum -c SHA256SUMS >/dev/null) || fail 'backup checksum validation failed'
tar -tzf "$partial/app-data.tar.gz" >/dev/null 2>&1 || fail 'app-data archive validation failed'

mv "$partial" "$final"
partial=''
printf 'test_station_backup status=succeeded backup_dir=%s\n' "$final"
