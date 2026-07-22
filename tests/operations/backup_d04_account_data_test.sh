#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT="$ROOT/ops/backup-d04-account-data.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

stat_mode() {
  if stat -f '%Lp' "$1" >/dev/null 2>&1; then
    stat -f '%Lp' "$1"
  else
    stat -c '%a' "$1"
  fi
}

new_fixture() {
  FIXTURE_ROOT=$(mktemp -d)
  BACKUP_ROOT="$FIXTURE_ROOT/backups"
  FAKE_BIN="$FIXTURE_ROOT/bin"
  mkdir -p "$BACKUP_ROOT" "$FAKE_BIN"
  cat >"$FAKE_BIN/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  compose)
    [[ "${FAKE_DOCKER_FAIL:-}" != postgres ]] || exit 41
    if [[ "${FAKE_DOCKER_EMPTY:-}" != postgres ]]; then
      printf 'postgres-custom-format-dump'
    fi
    ;;
  run)
    [[ "${FAKE_DOCKER_FAIL:-}" != sqlite ]] || exit 42
    backup_dir=''
    for argument in "$@"; do
      case "$argument" in
        type=bind,src=*,dst=/backup)
          backup_dir=${argument#type=bind,src=}
          backup_dir=${backup_dir%,dst=/backup}
          ;;
      esac
    done
    [[ -n "$backup_dir" ]] || exit 43
    printf 'sqlite-online-backup' >"$backup_dir/d04.sqlite"
    ;;
  *)
    exit 44
    ;;
esac
SH
  chmod 0755 "$FAKE_BIN/docker"
}

cleanup_fixture() {
  rm -rf -- "$FIXTURE_ROOT"
}

run_backup() {
  env \
    PATH="$FAKE_BIN:$PATH" \
    D04_BACKUP_ROOT="$BACKUP_ROOT" \
    SUB2API_COMPOSE_FILE="$FIXTURE_ROOT/compose.yaml" \
    D04_IMAGE="d04-backup:test" \
    D04_VOLUME="d04-test-volume" \
    D04_BACKUP_NOW="${D04_BACKUP_NOW:-20260722T100000Z}" \
    "$SCRIPT"
}

test_success_permissions_checksums_and_retention() {
  new_fixture
  mkdir -p \
    "$BACKUP_ROOT/20260719T100000Z" \
    "$BACKUP_ROOT/20260720T100000Z" \
    "$BACKUP_ROOT/20260721T100000Z" \
    "$BACKUP_ROOT/operator-notes"
  for old_set in "$BACKUP_ROOT"/202607{19,20,21}T100000Z; do
    : >"$old_set/SHA256SUMS"
    : >"$old_set/metadata.json"
  done

  run_backup

  set_path="$BACKUP_ROOT/20260722T100000Z"
  [[ -f "$set_path/sub2api.dump" ]] || fail 'missing PostgreSQL archive'
  [[ -f "$set_path/d04.sqlite" ]] || fail 'missing D04 SQLite archive'
  [[ -f "$set_path/SHA256SUMS" ]] || fail 'missing checksum manifest'
  [[ -f "$set_path/metadata.json" ]] || fail 'missing metadata'
  (cd "$set_path" && sha256sum -c SHA256SUMS >/dev/null) || fail 'checksum verification failed'
  [[ "$(stat_mode "$set_path")" == 700 ]] || fail 'backup directory mode is not 0700'
  [[ "$(stat_mode "$set_path/sub2api.dump")" == 600 ]] || fail 'PostgreSQL archive mode is not 0600'
  [[ "$(stat_mode "$set_path/d04.sqlite")" == 600 ]] || fail 'SQLite archive mode is not 0600'
  rg -Fq '"includes_sub2api_postgres":true' "$set_path/metadata.json" || fail 'PostgreSQL scope missing'
  rg -Fq '"includes_d04_sqlite":true' "$set_path/metadata.json" || fail 'D04 scope missing'

  [[ ! -e "$BACKUP_ROOT/20260719T100000Z" ]] || fail 'oldest verified set was not pruned'
  [[ -d "$BACKUP_ROOT/20260720T100000Z" ]] || fail 'second-newest set was pruned'
  [[ -d "$BACKUP_ROOT/20260721T100000Z" ]] || fail 'newest prior set was pruned'
  [[ -d "$BACKUP_ROOT/operator-notes" ]] || fail 'unrelated directory was removed'
  cleanup_fixture
}

test_failed_backup_never_promotes_partial_set() {
  new_fixture

  if FAKE_DOCKER_FAIL=sqlite run_backup >/dev/null 2>&1; then
    fail 'failed SQLite backup returned success'
  fi
  if find "$BACKUP_ROOT" -mindepth 1 -maxdepth 1 -type d -name '202[0-9]*Z' | rg -q .; then
    fail 'failed backup promoted a timestamp set'
  fi
  [[ ! -e "$BACKUP_ROOT/.backup.lock" ]] || fail 'failed backup left its lock behind'
  cleanup_fixture
}

test_empty_archive_never_promotes_partial_set() {
  new_fixture

  if FAKE_DOCKER_EMPTY=postgres run_backup >/dev/null 2>&1; then
    fail 'empty PostgreSQL backup returned success'
  fi
  if find "$BACKUP_ROOT" -mindepth 1 -maxdepth 1 -type d -name '202[0-9]*Z' | rg -q .; then
    fail 'empty archive promoted a timestamp set'
  fi
  cleanup_fixture
}

test_existing_lock_refuses_concurrent_run() {
  new_fixture
  mkdir "$BACKUP_ROOT/.backup.lock"

  if run_backup >/dev/null 2>&1; then
    fail 'concurrent backup unexpectedly succeeded'
  fi
  [[ -d "$BACKUP_ROOT/.backup.lock" ]] || fail 'existing lock was removed by refused run'
  cleanup_fixture
}

test_success_permissions_checksums_and_retention
test_failed_backup_never_promotes_partial_set
test_empty_archive_never_promotes_partial_set
test_existing_lock_refuses_concurrent_run

git -C "$ROOT" check-ignore -q config/operations/d04-lightweight-launch-snapshot.local.yaml \
  || fail 'local v2 snapshot is not ignored'

printf 'PASS: lightweight D04 account backup contracts\n'
