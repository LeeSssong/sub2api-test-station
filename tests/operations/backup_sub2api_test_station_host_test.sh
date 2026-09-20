#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
SCRIPT=${SCRIPT_UNDER_TEST:-$ROOT/ops/backup-sub2api-test-station-host.sh}
TMP_BASE=$(cd "${TMPDIR:-/tmp}" && pwd -P)
FIXTURE=$(mktemp -d "$TMP_BASE/test-station-backup.XXXXXX")
trap 'rm -rf -- "$FIXTURE"' EXIT
fail(){ printf 'FAIL: %s\n' "$1" >&2; exit 1; }
[[ -f "$SCRIPT" ]] || fail 'backup helper missing'

stat_mode(){ stat -f '%Lp' "$1" 2>/dev/null || stat -c '%a' "$1"; }

new_fixture(){
  CASE=$FIXTURE/$1
  DEPLOY_ROOT=$CASE/deploy
  ACTIVE_RELEASE=$DEPLOY_ROOT/releases/old
  BACKUP_ROOT=$DEPLOY_ROOT/backups
  BIN=$CASE/bin
  EVENT_LOG=$CASE/events.log
  REAL_TAR=$(command -v tar)
  REAL_SHA256SUM=$(command -v sha256sum)
  mkdir -p "$ACTIVE_RELEASE" "$BACKUP_ROOT" "$BIN"
  printf 'name: sub2api-test-station\nnetworks: {test: {name: sub2api-test-station-network}}\nservices: {}\n' >"$ACTIVE_RELEASE/compose.yaml"
  printf 'ADMIN_LAB_DB_PASSWORD=secret\nADMIN_LAB_REDIS_PASSWORD=secret\n' >"$ACTIVE_RELEASE/.env"
  chmod 0600 "$ACTIVE_RELEASE/.env"
  : >"$EVENT_LOG"

  cat >"$BIN/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"${EVENT_LOG:?}"
mode=${FAKE_DOCKER_MODE:-ok}
if [[ "${1:-}" == compose ]]; then
  shift
  [[ "${1:-}" == --project-name && "${2:-}" == sub2api-test-station ]] || exit 61
  shift 2
  [[ "${1:-}" == --env-file && "${2:-}" == "${ACTIVE_ENV:?}" ]] || exit 62
  shift 2
  [[ "${1:-}" == -f && "${2:-}" == "${ACTIVE_COMPOSE:?}" ]] || exit 63
  shift 2
  case "${1:-} ${2:-} ${3:-}" in
    'exec -T test-station-postgres')
      [[ "$*" == *pg_dump* ]] || exit 64
      [[ "$mode" != postgres-fail ]] || exit 41
      [[ "$mode" == postgres-empty ]] || printf 'PGDUMP-CUSTOM'
      ;;
    'exec -T test-station-redis')
      [[ "$*" == *'redis-cli --no-auth-warning -a "$REDIS_PASSWORD" SAVE'* ]] || exit 65
      [[ "$mode" != redis-save-fail ]] || exit 42
      ;;
    'ps -q test-station-redis') printf 'redis-container\n' ;;
    'ps -q test-station-api') printf 'api-container\n' ;;
    *) exit 66 ;;
  esac
elif [[ "${1:-}" == cp ]]; then
  src=${2:-}; dst=${3:-}
  case "$src" in
    redis-container:/data/dump.rdb)
      [[ "$mode" != redis-copy-fail ]] || exit 43
      [[ "$mode" == redis-empty ]] || printf 'REDIS-RDB' >"$dst"
      ;;
    api-container:/app/data/.)
      [[ "$mode" != app-copy-fail ]] || exit 44
      mkdir -p "$dst"
      printf 'config' >"$dst/config.yaml"
      ;;
    *) exit 67 ;;
  esac
elif [[ "${1:-}" == run ]]; then
  [[ "$*" == *'pg_restore --list /backup/postgres.dump'* ]] || exit 68
  [[ "$mode" != pg-restore-fail ]] || exit 45
else
  exit 69
fi
SH
  chmod +x "$BIN/docker"

  cat >"$BIN/tar" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${FAKE_DOCKER_MODE:-ok}" == app-corrupt && " $* " == *' -czf '* ]]; then
  args=("$@")
  for ((i=0; i<${#args[@]}; i++)); do
    if [[ "${args[$i]}" == -czf ]]; then printf 'corrupt' >"${args[$((i+1))]}"; exit 0; fi
  done
fi
exec "${REAL_TAR:?}" "$@"
SH
  chmod +x "$BIN/tar"

  cat >"$BIN/sha256sum" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${FAKE_DOCKER_MODE:-ok}" == checksum-fail && "${1:-}" == -c ]]; then exit 51; fi
exec "${REAL_SHA256SUM:?}" "$@"
SH
  chmod +x "$BIN/sha256sum"
}

run_backup(){
  env PATH="$BIN:$PATH" EVENT_LOG="$EVENT_LOG" ACTIVE_ENV="$ACTIVE_RELEASE/.env" \
    ACTIVE_COMPOSE="$ACTIVE_RELEASE/compose.yaml" REAL_TAR="$REAL_TAR" REAL_SHA256SUM="$REAL_SHA256SUM" FAKE_DOCKER_MODE="${FAKE_DOCKER_MODE:-ok}" \
    bash "$SCRIPT" --compose "$ACTIVE_RELEASE/compose.yaml" --env-file "$ACTIVE_RELEASE/.env" \
    --deploy-root "$DEPLOY_ROOT" --timestamp "${BACKUP_TIMESTAMP:-20260920T120000Z}"
}

assert_no_promoted_set(){
  [[ ! -e "$BACKUP_ROOT/${BACKUP_TIMESTAMP:-20260920T120000Z}" ]] || fail 'failed backup promoted a set'
  if find "$BACKUP_ROOT" -mindepth 1 -maxdepth 1 -name '.partial-*' | grep -q .; then
    fail 'failed backup left a partial set'
  fi
}

test_success_creates_verified_set(){
  new_fixture success
  output=$(run_backup) || fail 'success case failed'
  set_dir=$BACKUP_ROOT/20260920T120000Z
  [[ "$output" == "test_station_backup status=succeeded backup_dir=$set_dir" ]] || fail 'success output mismatch'
  [[ "$(stat_mode "$set_dir")" == 700 ]] || fail 'backup directory mode is not 0700'
  for file in postgres.dump redis-dump.rdb app-data.tar.gz SHA256SUMS metadata.json; do
    [[ -f "$set_dir/$file" && ! -L "$set_dir/$file" ]] || fail "missing $file"
    [[ "$(stat_mode "$set_dir/$file")" == 600 ]] || fail "$file mode is not 0600"
  done
  [[ "$(find "$set_dir" -mindepth 1 -maxdepth 1 -type f | wc -l | tr -d ' ')" == 5 ]] || fail 'unexpected backup contents'
  (cd "$set_dir" && sha256sum -c SHA256SUMS >/dev/null) || fail 'checksums failed'
  tar -tzf "$set_dir/app-data.tar.gz" >/dev/null || fail 'app-data archive unreadable'
  python3 - "$set_dir/metadata.json" <<'PY' || fail 'metadata invalid'
import json,sys
v=json.load(open(sys.argv[1],encoding='utf-8'))
assert v == {"schema_version":1,"created_at":"20260920T120000Z","project_name":"sub2api-test-station","sha256_verified":True}
PY
}

test_rejects_wrong_project_before_docker(){
  new_fixture wrong-project
  sed -i.bak 's/sub2api-test-station/other-project/' "$ACTIVE_RELEASE/compose.yaml"; rm "$ACTIVE_RELEASE/compose.yaml.bak"
  if run_backup >/dev/null 2>&1; then fail 'wrong project accepted'; fi
  [[ ! -s "$EVENT_LOG" ]] || fail 'wrong project invoked Docker'
  assert_no_promoted_set
}

test_rejects_symlink_before_docker(){
  new_fixture symlink
  mv "$ACTIVE_RELEASE/.env" "$ACTIVE_RELEASE/real.env"
  ln -s "$ACTIVE_RELEASE/real.env" "$ACTIVE_RELEASE/.env"
  if run_backup >/dev/null 2>&1; then fail 'symlink env accepted'; fi
  [[ ! -s "$EVENT_LOG" ]] || fail 'symlink invoked Docker'
  assert_no_promoted_set
}

test_rejects_permissive_env_before_docker(){
  new_fixture permissive-env
  chmod 0644 "$ACTIVE_RELEASE/.env"
  if run_backup >/dev/null 2>&1; then fail 'permissive env accepted'; fi
  [[ ! -s "$EVENT_LOG" ]] || fail 'permissive env invoked Docker'
  assert_no_promoted_set
}

test_failures_never_promote(){
  local mode
  for mode in postgres-fail postgres-empty pg-restore-fail redis-save-fail redis-copy-fail redis-empty app-copy-fail app-corrupt checksum-fail; do
    new_fixture "failure-$mode"
    if FAKE_DOCKER_MODE=$mode run_backup >/dev/null 2>&1; then fail "$mode returned success"; fi
    assert_no_promoted_set
  done
}

test_existing_final_and_history_are_preserved(){
  new_fixture existing
  mkdir "$BACKUP_ROOT/20260919T120000Z"
  printf 'operator evidence' >"$BACKUP_ROOT/20260919T120000Z/note.txt"
  mkdir "$BACKUP_ROOT/20260920T120000Z"
  printf 'existing' >"$BACKUP_ROOT/20260920T120000Z/existing.txt"
  if run_backup >/dev/null 2>&1; then fail 'existing timestamp accepted'; fi
  [[ "$(cat "$BACKUP_ROOT/20260919T120000Z/note.txt")" == 'operator evidence' ]] || fail 'history changed'
  [[ "$(cat "$BACKUP_ROOT/20260920T120000Z/existing.txt")" == existing ]] || fail 'existing final changed'
}

test_success_creates_verified_set
test_rejects_wrong_project_before_docker
test_rejects_symlink_before_docker
test_rejects_permissive_env_before_docker
test_failures_never_promote
test_existing_final_and_history_are_preserved
printf 'PASS: independent test station verified backup\n'
