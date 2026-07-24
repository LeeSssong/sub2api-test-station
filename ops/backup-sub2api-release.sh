#!/usr/bin/env bash
set -euo pipefail

umask 077

PINNED_POSTGRES_IMAGE='postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15'

fail() {
  printf 'release backup failed: %s\n' "$1" >&2
  exit 1
}

reject_symlink_components() {
  local path=$1 label=$2 component current=''
  local -a components
  [[ "$path" == /* ]] || fail "$label must be absolute"
  IFS='/' read -r -a components <<<"${path#/}"
  for component in "${components[@]}"; do
    [[ -n "$component" && "$component" != . && "$component" != .. ]] \
      || fail "$label must be canonical"
    current="$current/$component"
    [[ ! -L "$current" ]] || fail "$label must not contain symlinks"
  done
}

canonical_directory() {
  local path=$1 label=$2 physical
  reject_symlink_components "$path" "$label"
  [[ -d "$path" && ! -L "$path" ]] || fail "$label does not exist"
  physical=$(cd "$path" && pwd -P)
  [[ "$physical" == "$path" ]] || fail "$label must be canonical"
  printf '%s\n' "$physical"
}

canonical_file() {
  local path=$1 label=$2 parent physical_parent canonical_parent
  [[ "$path" == /* ]] || fail "$label must be absolute"
  reject_symlink_components "$path" "$label"
  [[ -f "$path" && -r "$path" && ! -L "$path" ]] || fail "$label is not a readable regular file"
  parent=$(dirname "$path")
  physical_parent=$(cd "$parent" && pwd -P)
  canonical_parent=$physical_parent
  [[ "$canonical_parent" != / ]] || canonical_parent=''
  [[ "$canonical_parent/$(basename "$path")" == "$path" ]] || fail "$label must be canonical"
  printf '%s\n' "$path"
}

production_root=${SUB2API_PRODUCTION_ROOT:-/opt/sub2api/production}
backup_root=${SUB2API_BACKUP_ROOT:-$production_root/backups/release}
data_root=${SUB2API_DATA_DIR:-$production_root/data}
timestamp=${SUB2API_BACKUP_NOW:-$(date -u +%Y%m%dT%H%M%SZ)}
postgres_image=${SUB2API_POSTGRES_IMAGE:-$PINNED_POSTGRES_IMAGE}
compose_project=${COMPOSE_PROJECT_NAME:-sub2api-deploy}

[[ "$timestamp" =~ ^[0-9]{8}T[0-9]{6}Z$ ]] || fail 'timestamp must use UTC compact format'
[[ "$postgres_image" == "$PINNED_POSTGRES_IMAGE" ]] || fail 'PostgreSQL validator image must use the approved digest'
[[ "$compose_project" == sub2api-deploy || "$compose_project" == sub2api-official-rehearsal ]] \
  || fail 'Compose project is not approved'

reject_symlink_components "$backup_root" 'backup root'
data_root=$(canonical_directory "$data_root" 'app-data root')
deploy_root=$(canonical_directory "${DEPLOY_ROOT:?DEPLOY_ROOT is required}" 'deploy root')
secret_env=$(canonical_file "${SECRET_ENV:?SECRET_ENV is required}" 'secret env')
release_env=$(canonical_file "${RELEASE_ENV:?RELEASE_ENV is required}" 'release env')
base_compose=$(canonical_file "${BASE_COMPOSE:?BASE_COMPOSE is required}" 'base Compose file')
image_overlay=$(canonical_file "${IMAGE_OVERLAY:?IMAGE_OVERLAY is required}" 'image overlay')

mkdir -p "$backup_root"
backup_root=$(canonical_directory "$backup_root" 'backup root')
chmod 0700 "$backup_root"

compose=(docker compose --project-name "$compose_project" --project-directory "$deploy_root"
  --env-file "$secret_env" --env-file "$release_env"
  -f "$base_compose" -f "$image_overlay")

lock_dir="$backup_root/.backup.lock"
if ! mkdir "$lock_dir" 2>/dev/null; then
  fail 'another backup is already running'
fi
lock_owned=true
partial_set="$backup_root/.partial-$timestamp-$$"
working_set=$partial_set
final_set="$backup_root/$timestamp"
promoted_owned=false
retention_rollback_root=''
preserve_retention_recovery=false
owned_files=(sub2api.dump app-data.tar.gz record-counts.json SHA256SUMS metadata.json)
retention_snapshots=()

remove_owned_files() {
  local set_path=$1 entry
  for entry in "${owned_files[@]}"; do
    rm -f -- "$set_path/$entry" || return 1
  done
}

rollback_promoted_set() {
  [[ "$promoted_owned" == true ]] || return 0
  [[ -d "$final_set" && ! -L "$final_set" && ! -e "$working_set" ]] || return 1
  mv "$final_set" "$working_set" || return 1
  partial_set=$working_set
  promoted_owned=false
}

cleanup() {
  local exit_code=$? snapshot
  if [[ "$promoted_owned" == true ]]; then
    rollback_promoted_set 2>/dev/null || true
  fi
  if [[ -n "${partial_set:-}" && -d "$partial_set" ]]; then
    case "$partial_set" in
      "$backup_root"/.partial-*)
        remove_owned_files "$partial_set" 2>/dev/null || true
        rmdir "$partial_set" 2>/dev/null || true
        ;;
    esac
  fi
  if [[ "$preserve_retention_recovery" != true ]]; then
    for snapshot in ${retention_snapshots[@]+"${retention_snapshots[@]}"}; do
      case "$snapshot" in
        "$backup_root"/.retention-rollback-*/*)
          remove_owned_files "$snapshot" 2>/dev/null || true
          rmdir "$snapshot" 2>/dev/null || true
          ;;
      esac
    done
    if [[ -n "$retention_rollback_root" ]]; then
      case "$retention_rollback_root" in
        "$backup_root"/.retention-rollback-*) rmdir "$retention_rollback_root" 2>/dev/null || true ;;
      esac
    fi
  fi
  if [[ "${lock_owned:-false}" == true ]]; then
    rmdir "$lock_dir" 2>/dev/null || true
  fi
  return "$exit_code"
}
trap cleanup EXIT

validate_postgres_dump() {
  local set_path=$1
  docker run --rm --network none --read-only --cap-drop ALL \
    --security-opt no-new-privileges:true \
    --mount "type=bind,src=$set_path,dst=/backup,readonly" \
    "$PINNED_POSTGRES_IMAGE" pg_restore --list /backup/sub2api.dump >/dev/null 2>&1
}

validate_record_counts() {
  jq -e '
    type == "object" and
    (keys | sort == ["accounts", "api_keys", "groups", "settings", "usage_logs", "users"]) and
    ([.users, .accounts, .groups, .api_keys, .settings, .usage_logs] |
      all(type == "number" and . >= 0 and floor == .))
  ' "$1" >/dev/null 2>&1
}

validate_manifest() {
  local set_path=$1 names
  [[ $(wc -l <"$set_path/SHA256SUMS" | tr -d ' ') == 3 ]] || return 1
  names=$(awk '
    length($1) != 64 || $1 !~ /^[0-9a-f]+$/ || NF != 2 { exit 1 }
    $2 != "sub2api.dump" && $2 != "app-data.tar.gz" && $2 != "record-counts.json" { exit 1 }
    { print $2 }
  ' "$set_path/SHA256SUMS" | LC_ALL=C sort) || return 1
  [[ "$names" == $'app-data.tar.gz\nrecord-counts.json\nsub2api.dump' ]] || return 1
  (cd "$set_path" && sha256sum -c SHA256SUMS >/dev/null 2>&1)
}

validate_metadata() {
  local set_path=$1 stamp=$2 dump_bytes app_bytes counts_bytes
  dump_bytes=$(wc -c <"$set_path/sub2api.dump" | tr -d ' ')
  app_bytes=$(wc -c <"$set_path/app-data.tar.gz" | tr -d ' ')
  counts_bytes=$(wc -c <"$set_path/record-counts.json" | tr -d ' ')
  jq -e --arg stamp "$stamp" --argjson dump "$dump_bytes" --argjson app "$app_bytes" --argjson counts "$counts_bytes" '
    (keys | sort == ["app_data_bytes", "created_at", "record_counts_bytes", "schema_version",
      "sha256_verified", "sub2api_dump_bytes"]) and
    .schema_version == 1 and .created_at == $stamp and .sha256_verified == true and
    .sub2api_dump_bytes == $dump and .app_data_bytes == $app and .record_counts_bytes == $counts
  ' "$set_path/metadata.json" >/dev/null 2>&1
}

is_verified_set() {
  local set_path=$1 stamp=${2:-$(basename "$1")} entry expected_count=0
  [[ -d "$set_path" && ! -L "$set_path" ]] || return 1
  for entry in sub2api.dump app-data.tar.gz record-counts.json SHA256SUMS metadata.json; do
    [[ -f "$set_path/$entry" && ! -L "$set_path/$entry" && -r "$set_path/$entry" ]] || return 1
    expected_count=$((expected_count + 1))
  done
  [[ $(find "$set_path" -mindepth 1 -maxdepth 1 -print | wc -l | tr -d ' ') == "$expected_count" ]] || return 1
  validate_manifest "$set_path" || return 1
  validate_metadata "$set_path" "$stamp" || return 1
  validate_record_counts "$set_path/record-counts.json" || return 1
  validate_postgres_dump "$set_path" || return 1
  tar -tzf "$set_path/app-data.tar.gz" >/dev/null 2>&1 || return 1
}

[[ ! -e "$final_set" ]] || fail 'a backup set already exists for this timestamp'
mkdir "$partial_set"
chmod 0700 "$partial_set"

"${compose[@]}" exec -T postgres \
  sh -c 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
  >"$partial_set/sub2api.dump"
[[ -s "$partial_set/sub2api.dump" ]] || fail 'PostgreSQL archive is empty'
validate_postgres_dump "$partial_set" || fail 'PostgreSQL archive validation failed'

"${compose[@]}" exec -T postgres \
  sh -c 'exec psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At' <<'SQL' >"$partial_set/record-counts.json"
select json_build_object(
  'users', (select count(*) from users),
  'accounts', (select count(*) from accounts),
  'groups', (select count(*) from groups),
  'api_keys', (select count(*) from api_keys),
  'settings', (select count(*) from settings),
  'usage_logs', (select count(*) from usage_logs)
)::text;
SQL
validate_record_counts "$partial_set/record-counts.json" || fail 'record counts are invalid'

tar -C "$data_root" -czf "$partial_set/app-data.tar.gz" .
[[ -s "$partial_set/app-data.tar.gz" ]] || fail 'app-data archive is empty'
tar -tzf "$partial_set/app-data.tar.gz" >/dev/null 2>&1 || fail 'app-data archive validation failed'

chmod 0600 "$partial_set/sub2api.dump" "$partial_set/record-counts.json" "$partial_set/app-data.tar.gz"
(
  cd "$partial_set"
  sha256sum sub2api.dump app-data.tar.gz record-counts.json >SHA256SUMS
  sha256sum -c SHA256SUMS >/dev/null
)
chmod 0600 "$partial_set/SHA256SUMS"

dump_bytes=$(wc -c <"$partial_set/sub2api.dump" | tr -d ' ')
app_data_bytes=$(wc -c <"$partial_set/app-data.tar.gz" | tr -d ' ')
record_counts_bytes=$(wc -c <"$partial_set/record-counts.json" | tr -d ' ')
printf '{"schema_version":1,"created_at":"%s","sha256_verified":true,"sub2api_dump_bytes":%s,"app_data_bytes":%s,"record_counts_bytes":%s}\n' \
  "$timestamp" "$dump_bytes" "$app_data_bytes" "$record_counts_bytes" >"$partial_set/metadata.json"
chmod 0600 "$partial_set/metadata.json"
is_verified_set "$partial_set" "$timestamp" || fail 'completed backup set validation failed'

mv "$partial_set" "$final_set"
partial_set=''
promoted_owned=true

verified_sets=()
while IFS= read -r candidate; do
  is_verified_set "$candidate" || continue
  verified_sets+=("$candidate")
done < <(find "$backup_root" -mindepth 1 -maxdepth 1 -type d -name '????????T??????Z' | LC_ALL=C sort)

victim_count=$((${#verified_sets[@]} - 3))
if ((victim_count > 0)); then
  # Hard-link snapshots make active retention reversible after promotion.
  victims=("${verified_sets[@]:0:victim_count}")
  retention_rollback_root="$backup_root/.retention-rollback-$timestamp-$$"
  mkdir "$retention_rollback_root" || fail 'could not create retention rollback area'
  chmod 0700 "$retention_rollback_root"

  for victim in "${victims[@]}"; do
    victim_name=$(basename "$victim")
    [[ "$(dirname "$victim")" == "$backup_root" && "$victim_name" =~ ^[0-9]{8}T[0-9]{6}Z$ ]] \
      || fail 'retention target is invalid'
    is_verified_set "$victim" || fail 'retention target changed during validation'
    snapshot="$retention_rollback_root/$victim_name"
    mkdir "$snapshot" || fail 'could not stage retention rollback'
    chmod 0700 "$snapshot"
    retention_snapshots+=("$snapshot")
    for entry in "${owned_files[@]}"; do
      ln "$victim/$entry" "$snapshot/$entry" || fail 'could not stage retention rollback'
    done
  done

  retention_failed=false
  for victim in "${victims[@]}"; do
    if ! is_verified_set "$victim"; then
      retention_failed=true
      break
    fi
    if ! remove_owned_files "$victim" || ! rmdir "$victim"; then
      retention_failed=true
      break
    fi
  done

  if [[ "$retention_failed" == true ]]; then
    restore_failed=false
    for index in "${!victims[@]}"; do
      victim=${victims[$index]}
      snapshot=${retention_snapshots[$index]}
      if [[ ! -d "$victim" ]]; then
        mkdir "$victim" || restore_failed=true
        chmod 0700 "$victim" 2>/dev/null || restore_failed=true
      fi
      if [[ -d "$victim" ]]; then
        for entry in "${owned_files[@]}"; do
          if [[ ! -e "$victim/$entry" ]]; then
            ln "$snapshot/$entry" "$victim/$entry" || restore_failed=true
          fi
        done
      fi
    done
    if [[ "$restore_failed" == false ]]; then
      for victim in "${victims[@]}"; do
        if ! is_verified_set "$victim"; then
          restore_failed=true
          break
        fi
      done
    fi
    if [[ "$restore_failed" == true ]]; then
      preserve_retention_recovery=true
      printf 'retention_recovery_path=%s\n' "$retention_rollback_root" >&2
    fi
    rollback_promoted_set || fail 'retention failed and the new promotion could not be rolled back'
    [[ "$restore_failed" == false ]] || fail 'retention failed and recovery snapshots were preserved'
    fail 'retention failed; the new promotion was rolled back'
  fi
fi

promoted_owned=false

printf 'release_backup_set=%s\n' "$final_set"
