#!/usr/bin/env bash
set -euo pipefail

umask 077

production_root=${D04_PRODUCTION_ROOT:-/opt/sub2api/production}
backup_root=${D04_BACKUP_ROOT:-$production_root/backups/d04-account-data}
sub2api_compose_file=${SUB2API_COMPOSE_FILE:-$production_root/compose.yaml}
d04_image=${D04_IMAGE:-sub2api-internal-test:d04-lightweight-launch-20260722-v2}
d04_volume=${D04_VOLUME:-sub2api_d04_internal_test_data}
timestamp=${D04_BACKUP_NOW:-$(date -u +%Y%m%dT%H%M%SZ)}

fail() {
  printf 'account backup failed: %s\n' "$1" >&2
  exit 1
}

case "$backup_root" in
  /*) ;;
  *) fail 'backup root must be absolute' ;;
esac
[[ "$timestamp" =~ ^[0-9]{8}T[0-9]{6}Z$ ]] || fail 'timestamp must use UTC compact format'
[[ -n "$sub2api_compose_file" && -n "$d04_image" && -n "$d04_volume" ]] \
  || fail 'backup inputs must be non-empty'

mkdir -p "$backup_root"
chmod 0700 "$backup_root"

lock_dir="$backup_root/.backup.lock"
if ! mkdir "$lock_dir" 2>/dev/null; then
  fail 'another backup is already running'
fi
lock_owned=true
temporary_set="$backup_root/.partial-$timestamp-$$"

cleanup() {
  exit_code=$?
  if [[ -n "${temporary_set:-}" && -d "$temporary_set" ]]; then
    case "$temporary_set" in
      "$backup_root"/.partial-*)
        find "$temporary_set" -mindepth 1 -maxdepth 1 \( -type f -o -type l \) -delete
        rmdir "$temporary_set" 2>/dev/null || true
        ;;
    esac
  fi
  if [[ "${lock_owned:-false}" == true ]]; then
    rmdir "$lock_dir" 2>/dev/null || true
  fi
  return "$exit_code"
}
trap cleanup EXIT

final_set="$backup_root/$timestamp"
[[ ! -e "$final_set" ]] || fail 'a backup set already exists for this timestamp'
mkdir "$temporary_set"
chmod 0700 "$temporary_set"

docker compose -f "$sub2api_compose_file" exec -T postgres \
  sh -c 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
  >"$temporary_set/sub2api.dump"
[[ -s "$temporary_set/sub2api.dump" ]] || fail 'PostgreSQL archive is empty'

docker run --rm --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges:true --user 0:0 \
  --mount "type=volume,src=$d04_volume,dst=/var/lib/internal-test,readonly" \
  --mount "type=bind,src=$temporary_set,dst=/backup" \
  "$d04_image" backup-sqlite \
  /var/lib/internal-test/internal-test.db /backup/d04.sqlite
[[ -s "$temporary_set/d04.sqlite" ]] || fail 'D04 SQLite archive is empty'

chmod 0600 "$temporary_set/sub2api.dump" "$temporary_set/d04.sqlite"
(
  cd "$temporary_set"
  sha256sum sub2api.dump d04.sqlite >SHA256SUMS
  sha256sum -c SHA256SUMS >/dev/null
)
chmod 0600 "$temporary_set/SHA256SUMS"

postgres_bytes=$(wc -c <"$temporary_set/sub2api.dump" | tr -d ' ')
d04_bytes=$(wc -c <"$temporary_set/d04.sqlite" | tr -d ' ')
printf '{"schema_version":1,"created_at":"%s","includes_sub2api_postgres":true,"includes_d04_sqlite":true,"sha256_verified":true,"sub2api_bytes":%s,"d04_bytes":%s}\n' \
  "$timestamp" "$postgres_bytes" "$d04_bytes" >"$temporary_set/metadata.json"
chmod 0600 "$temporary_set/metadata.json"

mv "$temporary_set" "$final_set"
temporary_set=''

verified_sets=()
while IFS= read -r candidate; do
  [[ -f "$candidate/SHA256SUMS" && -f "$candidate/metadata.json" ]] || continue
  verified_sets+=("$candidate")
done < <(find "$backup_root" -mindepth 1 -maxdepth 1 -type d -name '????????T??????Z' | LC_ALL=C sort)

while (( ${#verified_sets[@]} > 3 )); do
  victim=${verified_sets[0]}
  victim_parent=$(dirname "$victim")
  victim_name=$(basename "$victim")
  [[ "$victim_parent" == "$backup_root" ]] || fail 'retention target escaped backup root'
  [[ "$victim_name" =~ ^[0-9]{8}T[0-9]{6}Z$ ]] || fail 'retention target is not a backup set'
  find "$victim" -mindepth 1 -maxdepth 1 \( -type f -o -type l \) -delete
  rmdir "$victim"
  verified_sets=("${verified_sets[@]:1}")
done

printf 'account_backup_set=%s\n' "$final_set"
