#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT="$ROOT/ops/backup-sub2api-release.sh"
PINNED_POSTGRES_IMAGE='postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15'

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
  FIXTURE_ROOT=$(cd "$(mktemp -d)" && pwd -P)
  DEPLOY_ROOT="$FIXTURE_ROOT/deploy"
  BACKUP_ROOT="$FIXTURE_ROOT/backups"
  DATA_ROOT="$FIXTURE_ROOT/app-data"
  FAKE_BIN="$FIXTURE_ROOT/bin"
  INVOCATION_LOG="$FIXTURE_ROOT/invocations.log"
  BASE_COMPOSE="$DEPLOY_ROOT/base.yaml"
  IMAGE_OVERLAY="$DEPLOY_ROOT/image-overlay.yaml"
  RELEASE_ENV="$DEPLOY_ROOT/release.env"
  SECRET_ENV="$DEPLOY_ROOT/secret.env"
  REAL_TAR=$(command -v tar)
  REAL_RM=$(command -v rm)
  REAL_RMDIR=$(command -v rmdir)
  REAL_LN=$(command -v ln)
  mkdir -p "$DEPLOY_ROOT" "$BACKUP_ROOT" "$DATA_ROOT" "$FAKE_BIN"
  : >"$BASE_COMPOSE"
  : >"$IMAGE_OVERLAY"
  : >"$RELEASE_ENV"
  : >"$SECRET_ENV"
  : >"$INVOCATION_LOG"
  printf 'application-state' >"$DATA_ROOT/state.db"

  cat >"$FAKE_BIN/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker\n' >>"${FAKE_INVOCATION_LOG:?}"

validate_compose_prefix() {
  local expected
  local -a prefix=(
    compose
    --project-name "${FAKE_EXPECTED_PROJECT:-sub2api-deploy}"
    --project-directory "${FAKE_EXPECTED_DEPLOY_ROOT:-$FAKE_DEPLOY_ROOT}"
    --env-file "${FAKE_EXPECTED_SECRET_ENV:-$FAKE_SECRET_ENV}"
    --env-file "${FAKE_EXPECTED_RELEASE_ENV:-$FAKE_RELEASE_ENV}"
    -f "${FAKE_EXPECTED_BASE_COMPOSE:-$FAKE_BASE_COMPOSE}"
    -f "${FAKE_EXPECTED_IMAGE_OVERLAY:-$FAKE_IMAGE_OVERLAY}"
  )
  for expected in "${prefix[@]}"; do
    [[ "${1:-}" == "$expected" ]] || exit 61
    shift
  done
  COMPOSE_REMAINDER=("$@")
}

case "${1:-}" in
  compose)
    validate_compose_prefix "$@"
    set -- "${COMPOSE_REMAINDER[@]}"
    [[ "${1:-}" == exec && "${2:-}" == -T && "${3:-}" == postgres ]] || exit 62
    if [[ "$*" == *pg_dump* ]]; then
      [[ "${FAKE_DOCKER_MODE:-}" != dump-fail ]] || exit 41
      [[ "${FAKE_DOCKER_MODE:-}" == dump-empty ]] || printf 'postgres-custom-format-dump'
    else
      [[ "${FAKE_DOCKER_MODE:-}" != counts-fail ]] || exit 42
      printf '{"users":2,"accounts":3,"groups":4,"api_keys":5,"settings":1,"usage_logs":6}\n'
    fi
    ;;
  run)
    [[ "$#" == 15 ]] || exit 63
    [[ "$2" == --rm && "$3" == --network && "$4" == none ]] || exit 64
    [[ "$5" == --read-only && "$6" == --cap-drop && "$7" == ALL ]] || exit 65
    [[ "$8" == --security-opt && "$9" == no-new-privileges:true ]] || exit 66
    [[ "${10}" == --mount && "${11}" == type=bind,src=*,dst=/backup,readonly ]] || exit 67
    [[ "${12}" == 'postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15' ]] || exit 68
    [[ "${13}" == pg_restore && "${14}" == --list && "${15}" == /backup/sub2api.dump ]] || exit 69
    [[ "${FAKE_DOCKER_MODE:-}" != restore-fail ]] || exit 43
    ;;
  *) exit 44 ;;
esac
SH
  chmod 0755 "$FAKE_BIN/docker"

  cat >"$FAKE_BIN/tar" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'tar\n' >>"${FAKE_INVOCATION_LOG:?}"
exec "${FAKE_REAL_TAR:?}" "$@"
SH
  chmod 0755 "$FAKE_BIN/tar"
}

cleanup_fixture() {
  rm -rf -- "$FIXTURE_ROOT"
}

run_backup() {
  env PATH="$FAKE_BIN:$PATH" \
    FAKE_INVOCATION_LOG="$INVOCATION_LOG" \
    FAKE_REAL_TAR="$REAL_TAR" \
    FAKE_REAL_RM="$REAL_RM" \
    FAKE_REAL_RMDIR="$REAL_RMDIR" \
    FAKE_REAL_LN="$REAL_LN" \
    FAKE_DEPLOY_ROOT="$DEPLOY_ROOT" \
    FAKE_SECRET_ENV="$SECRET_ENV" \
    FAKE_RELEASE_ENV="$RELEASE_ENV" \
    FAKE_BASE_COMPOSE="$BASE_COMPOSE" \
    FAKE_IMAGE_OVERLAY="$IMAGE_OVERLAY" \
    DEPLOY_ROOT="$DEPLOY_ROOT" \
    SECRET_ENV="$SECRET_ENV" \
    RELEASE_ENV="$RELEASE_ENV" \
    BASE_COMPOSE="$BASE_COMPOSE" \
    IMAGE_OVERLAY="$IMAGE_OVERLAY" \
    COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME_OVERRIDE:-sub2api-deploy}" \
    SUB2API_BACKUP_ROOT="$BACKUP_ROOT" \
    SUB2API_DATA_DIR="$DATA_ROOT" \
    SUB2API_BACKUP_NOW="${SUB2API_BACKUP_NOW:-20260724T100000Z}" \
    /bin/bash "$SCRIPT"
}

test_rehearsal_project_never_falls_back_to_production() {
  new_fixture
  output=$(COMPOSE_PROJECT_NAME_OVERRIDE=sub2api-official-rehearsal \
    FAKE_EXPECTED_PROJECT=sub2api-official-rehearsal run_backup 2>&1) \
    || fail "rehearsal project failed: $output"
  [[ "$output" == release_backup_set=* ]] || fail 'rehearsal backup did not report its set'
  cleanup_fixture

  new_fixture
  if COMPOSE_PROJECT_NAME_OVERRIDE=unapproved-project run_backup >/dev/null 2>&1; then
    fail 'backup accepted an unapproved Compose project'
  fi
  [[ ! -s "$INVOCATION_LOG" ]] || fail 'invalid project invoked Docker or tar'
  cleanup_fixture
}

create_verified_set() {
  local stamp=$1 set_path="$BACKUP_ROOT/$1"
  mkdir "$set_path"
  printf 'old-postgres-dump' >"$set_path/sub2api.dump"
  tar -C "$DATA_ROOT" -czf "$set_path/app-data.tar.gz" .
  printf '{"users":1,"accounts":1,"groups":1,"api_keys":1,"settings":1,"usage_logs":1}\n' \
    >"$set_path/record-counts.json"
  (
    cd "$set_path"
    sha256sum sub2api.dump app-data.tar.gz record-counts.json >SHA256SUMS
  )
  dump_bytes=$(wc -c <"$set_path/sub2api.dump" | tr -d ' ')
  app_bytes=$(wc -c <"$set_path/app-data.tar.gz" | tr -d ' ')
  counts_bytes=$(wc -c <"$set_path/record-counts.json" | tr -d ' ')
  printf '{"schema_version":1,"created_at":"%s","sha256_verified":true,"sub2api_dump_bytes":%s,"app_data_bytes":%s,"record_counts_bytes":%s}\n' \
    "$stamp" "$dump_bytes" "$app_bytes" "$counts_bytes" >"$set_path/metadata.json"
}

test_success_permissions_validation_and_strict_retention() {
  new_fixture
  create_verified_set 20260721T100000Z
  create_verified_set 20260722T100000Z
  create_verified_set 20260723T100000Z
  mkdir "$BACKUP_ROOT/20260725T100000Z"
  printf 'incomplete' >"$BACKUP_ROOT/20260725T100000Z/operator-note.txt"
  create_verified_set 20260726T100000Z
  printf 'operator-extra' >"$BACKUP_ROOT/20260726T100000Z/operator-note.txt"
  mkdir "$BACKUP_ROOT/operator-notes"
  printf 'do not remove' >"$BACKUP_ROOT/operator-notes/note.txt"

  output=$(run_backup 2>&1) || fail "success case failed: $output"

  set_path="$BACKUP_ROOT/20260724T100000Z"
  for required in sub2api.dump app-data.tar.gz record-counts.json SHA256SUMS metadata.json; do
    [[ -f "$set_path/$required" && ! -L "$set_path/$required" ]] || fail "missing regular $required"
    [[ "$(stat_mode "$set_path/$required")" == 600 ]] || fail "$required mode is not 0600"
  done
  [[ "$(stat_mode "$set_path")" == 700 ]] || fail 'set mode is not 0700'
  (cd "$set_path" && sha256sum -c SHA256SUMS >/dev/null) || fail 'checksums do not verify'
  tar -tzf "$set_path/app-data.tar.gz" >/dev/null || fail 'promoted app-data archive is unreadable'
  [[ ! -e "$BACKUP_ROOT/20260721T100000Z" ]] || fail 'oldest verified set was not pruned'
  [[ -d "$BACKUP_ROOT/20260722T100000Z" && -d "$BACKUP_ROOT/20260723T100000Z" ]] \
    || fail 'one of the three newest verified sets was pruned'
  [[ -f "$BACKUP_ROOT/20260725T100000Z/operator-note.txt" ]] || fail 'incomplete timestamp set was pruned'
  [[ -f "$BACKUP_ROOT/20260726T100000Z/operator-note.txt" ]] || fail 'timestamp set with extra file was pruned'
  [[ -f "$BACKUP_ROOT/operator-notes/note.txt" ]] || fail 'operator notes were removed'
  cleanup_fixture
}

test_compose_context_is_exact() {
  local mismatch
  for mismatch in project deploy_root secret_env release_env base_compose image_overlay; do
    new_fixture
    case "$mismatch" in
      project) override=(FAKE_EXPECTED_PROJECT=changed-project) ;;
      deploy_root) override=(FAKE_EXPECTED_DEPLOY_ROOT="$FIXTURE_ROOT/changed-root") ;;
      secret_env) override=(FAKE_EXPECTED_SECRET_ENV="$FIXTURE_ROOT/changed-secret.env") ;;
      release_env) override=(FAKE_EXPECTED_RELEASE_ENV="$FIXTURE_ROOT/changed-release.env") ;;
      base_compose) override=(FAKE_EXPECTED_BASE_COMPOSE="$FIXTURE_ROOT/changed-base.yaml") ;;
      image_overlay) override=(FAKE_EXPECTED_IMAGE_OVERLAY="$FIXTURE_ROOT/changed-overlay.yaml") ;;
    esac
    if (export "${override[@]}"; run_backup >/dev/null 2>&1); then
      fail "backup accepted changed Compose $mismatch context"
    fi
    cleanup_fixture
  done
}

assert_symlink_rejected_without_side_effects() {
  local label=$1 protected_path=$2 protected_mode=$3
  if run_backup >/dev/null 2>&1; then
    fail "$label symlink was accepted"
  fi
  [[ "$(stat_mode "$protected_path")" == "$protected_mode" ]] || fail "$label changed target permissions"
  [[ -f "$protected_path/protected.txt" ]] || fail "$label changed target content"
  [[ ! -s "$INVOCATION_LOG" ]] || fail "$label invoked Docker or tar"
}

test_backup_and_data_symlinks_are_rejected_before_side_effects() {
  local target_mode

  new_fixture
  mkdir "$FIXTURE_ROOT/backup-target"
  printf protected >"$FIXTURE_ROOT/backup-target/protected.txt"
  chmod 0755 "$FIXTURE_ROOT/backup-target"
  target_mode=$(stat_mode "$FIXTURE_ROOT/backup-target")
  rm -rf "$BACKUP_ROOT"
  ln -s "$FIXTURE_ROOT/backup-target" "$BACKUP_ROOT"
  assert_symlink_rejected_without_side_effects 'backup leaf' "$FIXTURE_ROOT/backup-target" "$target_mode"
  cleanup_fixture

  new_fixture
  mkdir "$FIXTURE_ROOT/backup-parent-target"
  printf protected >"$FIXTURE_ROOT/backup-parent-target/protected.txt"
  chmod 0755 "$FIXTURE_ROOT/backup-parent-target"
  target_mode=$(stat_mode "$FIXTURE_ROOT/backup-parent-target")
  BACKUP_ROOT="$FIXTURE_ROOT/backup-parent-link/new-backups"
  ln -s "$FIXTURE_ROOT/backup-parent-target" "$FIXTURE_ROOT/backup-parent-link"
  assert_symlink_rejected_without_side_effects 'backup parent' "$FIXTURE_ROOT/backup-parent-target" "$target_mode"
  [[ ! -e "$FIXTURE_ROOT/backup-parent-target/new-backups" ]] || fail 'backup parent created a directory through symlink'
  cleanup_fixture

  new_fixture
  mkdir "$FIXTURE_ROOT/data-target"
  printf protected >"$FIXTURE_ROOT/data-target/protected.txt"
  chmod 0755 "$FIXTURE_ROOT/data-target"
  target_mode=$(stat_mode "$FIXTURE_ROOT/data-target")
  rm -rf "$DATA_ROOT"
  ln -s "$FIXTURE_ROOT/data-target" "$DATA_ROOT"
  assert_symlink_rejected_without_side_effects 'data leaf' "$FIXTURE_ROOT/data-target" "$target_mode"
  cleanup_fixture

  new_fixture
  mkdir -p "$FIXTURE_ROOT/data-parent-target/app-data"
  printf protected >"$FIXTURE_ROOT/data-parent-target/protected.txt"
  chmod 0755 "$FIXTURE_ROOT/data-parent-target"
  target_mode=$(stat_mode "$FIXTURE_ROOT/data-parent-target")
  DATA_ROOT="$FIXTURE_ROOT/data-parent-link/app-data"
  ln -s "$FIXTURE_ROOT/data-parent-target" "$FIXTURE_ROOT/data-parent-link"
  assert_symlink_rejected_without_side_effects 'data parent' "$FIXTURE_ROOT/data-parent-target" "$target_mode"
  cleanup_fixture
}

test_empty_pg_dump_never_promotes_timestamp_directory() {
  new_fixture
  if FAKE_DOCKER_MODE=dump-empty run_backup >/dev/null 2>&1; then
    fail 'empty pg_dump returned success'
  fi
  [[ ! -e "$BACKUP_ROOT/20260724T100000Z" ]] || fail 'empty dump was promoted'
  [[ ! -e "$BACKUP_ROOT/.backup.lock" ]] || fail 'failed backup left lock'
  cleanup_fixture
}

test_corrupt_nonempty_app_data_archive_never_promotes() {
  new_fixture
  cat >"$FAKE_BIN/tar" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'tar\n' >>"${FAKE_INVOCATION_LOG:?}"
if [[ "${1:-}" == -C ]]; then
  printf 'not-a-valid-tarball' >"$4"
  exit 0
fi
exit 51
SH
  chmod 0755 "$FAKE_BIN/tar"
  if run_backup >/dev/null 2>&1; then
    fail 'corrupt nonempty app-data archive returned success'
  fi
  [[ ! -e "$BACKUP_ROOT/20260724T100000Z" ]] || fail 'corrupt archive was promoted'
  cleanup_fixture
}

test_post_promotion_retention_failure_rolls_back_only_new_set() {
  new_fixture
  create_verified_set 20260721T100000Z
  create_verified_set 20260722T100000Z
  create_verified_set 20260723T100000Z
  mkdir "$BACKUP_ROOT/20260725T100000Z" "$BACKUP_ROOT/operator-notes"
  printf 'unknown timestamp content' >"$BACKUP_ROOT/20260725T100000Z/unknown.txt"
  printf 'operator note' >"$BACKUP_ROOT/operator-notes/note.txt"

  cat >"$FAKE_BIN/rmdir" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
for argument in "$@"; do
  if [[ "$argument" == "${FAKE_RETENTION_VICTIM:?}" ]]; then
    [[ -d "${FAKE_NEW_FINAL:?}" ]] || exit 81
    printf 'injected\n' >"${FAKE_RETENTION_MARKER:?}"
    exit 82
  fi
done
exec "${FAKE_REAL_RMDIR:?}" "$@"
SH
  chmod 0755 "$FAKE_BIN/rmdir"
  marker="$FIXTURE_ROOT/retention-failure.marker"
  if output=$(FAKE_RETENTION_VICTIM="$BACKUP_ROOT/20260721T100000Z" \
    FAKE_NEW_FINAL="$BACKUP_ROOT/20260724T100000Z" FAKE_RETENTION_MARKER="$marker" \
    run_backup 2>&1); then
    fail 'post-promotion retention failure returned success'
  fi
  [[ -f "$marker" ]] || fail 'retention failure was not injected after promotion'
  [[ ! -e "$BACKUP_ROOT/20260724T100000Z" ]] || fail 'failed retention left the new timestamp set'
  for stamp in 20260721T100000Z 20260722T100000Z 20260723T100000Z; do
    set_path="$BACKUP_ROOT/$stamp"
    [[ -d "$set_path" ]] || fail "retention failure removed existing set $stamp"
    (cd "$set_path" && sha256sum -c SHA256SUMS >/dev/null) \
      || fail "retention failure damaged existing set $stamp"
  done
  [[ -f "$BACKUP_ROOT/20260725T100000Z/unknown.txt" ]] || fail 'retention failure removed unknown content'
  [[ -f "$BACKUP_ROOT/operator-notes/note.txt" ]] || fail 'retention failure removed operator notes'
  cleanup_fixture
}

test_restore_link_failure_preserves_explicit_recovery_snapshot() {
  new_fixture
  create_verified_set 20260721T100000Z
  create_verified_set 20260722T100000Z
  create_verified_set 20260723T100000Z
  mkdir "$BACKUP_ROOT/20260725T100000Z" "$BACKUP_ROOT/operator-notes"
  printf 'unknown timestamp content' >"$BACKUP_ROOT/20260725T100000Z/unknown.txt"
  printf 'operator note' >"$BACKUP_ROOT/operator-notes/note.txt"

  cat >"$FAKE_BIN/rmdir" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
for argument in "$@"; do
  if [[ "$argument" == "${FAKE_RETENTION_VICTIM:?}" ]]; then
    [[ -d "${FAKE_NEW_FINAL:?}" ]] || exit 81
    exit 82
  fi
done
exec "${FAKE_REAL_RMDIR:?}" "$@"
SH
  cat >"$FAKE_BIN/ln" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
source_path=${1:-}
destination_path=${2:-}
if [[ "$source_path" == "${FAKE_BACKUP_ROOT:?}"/.retention-rollback-*/*/sub2api.dump &&
      "$destination_path" == "${FAKE_RETENTION_VICTIM:?}/sub2api.dump" ]]; then
  exit 83
fi
exec "${FAKE_REAL_LN:?}" "$@"
SH
  chmod 0755 "$FAKE_BIN/rmdir" "$FAKE_BIN/ln"

  if output=$(FAKE_BACKUP_ROOT="$BACKUP_ROOT" \
    FAKE_RETENTION_VICTIM="$BACKUP_ROOT/20260721T100000Z" \
    FAKE_NEW_FINAL="$BACKUP_ROOT/20260724T100000Z" run_backup 2>&1); then
    fail 'secondary restoration failure returned success'
  fi
  [[ ! -e "$BACKUP_ROOT/20260724T100000Z" ]] || fail 'secondary restoration failure left the new timestamp set'
  recovery_roots=("$BACKUP_ROOT"/.retention-rollback-*)
  [[ ${#recovery_roots[@]} == 1 && -d "${recovery_roots[0]}" ]] \
    || fail 'secondary restoration failure removed the recovery root'
  recovery_root=${recovery_roots[0]}
  snapshot="$recovery_root/20260721T100000Z"
  [[ "$(stat_mode "$recovery_root")" == 700 && "$(stat_mode "$snapshot")" == 700 ]] \
    || fail 'recovery directories are not 0700'
  for required in sub2api.dump app-data.tar.gz record-counts.json SHA256SUMS metadata.json; do
    [[ -f "$snapshot/$required" && ! -L "$snapshot/$required" ]] \
      || fail "recovery snapshot lost $required"
  done
  (cd "$snapshot" && sha256sum -c SHA256SUMS >/dev/null) \
    || fail 'recovery snapshot checksums are not recoverable'
  [[ "$output" == *"retention_recovery_path=$recovery_root"* ]] \
    || fail 'secondary restoration failure did not report the recovery path'
  [[ -f "$BACKUP_ROOT/20260725T100000Z/unknown.txt" ]] || fail 'secondary failure removed unknown content'
  [[ -f "$BACKUP_ROOT/operator-notes/note.txt" ]] || fail 'secondary failure removed operator notes'
  cleanup_fixture
}

test_existing_lock_refuses_run_without_removing_lock() {
  new_fixture
  mkdir "$BACKUP_ROOT/.backup.lock"
  if run_backup >/dev/null 2>&1; then
    fail 'existing lock permitted concurrent backup'
  fi
  [[ -d "$BACKUP_ROOT/.backup.lock" ]] || fail 'existing lock was removed'
  cleanup_fixture
}

test_success_permissions_validation_and_strict_retention
test_rehearsal_project_never_falls_back_to_production
test_compose_context_is_exact
test_backup_and_data_symlinks_are_rejected_before_side_effects
test_empty_pg_dump_never_promotes_timestamp_directory
test_corrupt_nonempty_app_data_archive_never_promotes
test_post_promotion_retention_failure_rolls_back_only_new_set
test_restore_link_failure_preserves_explicit_recovery_snapshot
test_existing_lock_refuses_run_without_removing_lock

printf 'PASS: Sub2API release backup contracts\n'
