#!/usr/bin/env bash
set -euo pipefail

umask 077

fail() {
  printf 'T77 detection-history purge failed: %s\n' "$1" >&2
  exit 1
}

expected=${T77_EXPECTED_ROWS:-}
container=${T77_POSTGRES_CONTAINER:-sub2api-postgres-1}
backup_dir=${T77_BACKUP_DIR:-}
[[ "$expected" == 3676 ]] || fail 'T77_EXPECTED_ROWS must be the user-confirmed value 3676'
[[ "$container" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$ ]] || fail 'T77_POSTGRES_CONTAINER is invalid'
[[ "$backup_dir" == /* && "$backup_dir" != / && ! -L "$backup_dir" ]] || fail 'T77_BACKUP_DIR must be an absolute non-symlink directory'

install -d -m 700 "$backup_dir"
[[ -d "$backup_dir" && ! -L "$backup_dir" ]] || fail 'T77_BACKUP_DIR is unsafe'

psql=(docker exec "$container" psql -U sub2api -d sub2api -X -v ON_ERROR_STOP=1 -At)
target_count=$("${psql[@]}" -c "SELECT count(*) FROM account_model_detection_runs WHERE detector_version IS DISTINCT FROM '4.1.1';")
[[ "$target_count" =~ ^[0-9]+$ && "$target_count" -eq "$expected" ]] || fail "target count drifted: expected $expected, got ${target_count:-invalid}"
foreign_keys=$("${psql[@]}" -c "SELECT count(*) FROM pg_constraint WHERE contype = 'f' AND confrelid = 'account_model_detection_runs'::regclass;")
[[ "$foreign_keys" == 0 ]] || fail 'foreign-key guard rejected purge'

stamp=$(date -u +%Y%m%dT%H%M%SZ)
archive="$backup_dir/t77-account-model-detection-runs-$stamp.sql.gz"
[[ ! -e "$archive" ]] || fail 'backup path already exists'
docker exec "$container" pg_dump -U sub2api -d sub2api --table=public.account_model_detection_runs | gzip -c >"$archive"
chmod 600 "$archive"
gzip -t "$archive"
archive_sha256=$(sha256sum "$archive" | awk '{print $1}')
[[ "$archive_sha256" =~ ^[a-f0-9]{64}$ ]] || fail 'backup checksum failed'

docker exec -i "$container" psql -U sub2api -d sub2api -X -v ON_ERROR_STOP=1 <<SQL
BEGIN;
LOCK TABLE account_model_detection_runs IN ACCESS EXCLUSIVE MODE;
DO \$\$
DECLARE
  actual_count bigint;
  foreign_key_count bigint;
  deleted_count bigint;
  remaining_count bigint;
BEGIN
  SELECT count(*) INTO actual_count FROM account_model_detection_runs WHERE detector_version IS DISTINCT FROM '4.1.1';
  IF actual_count <> $expected THEN
    RAISE EXCEPTION 'target count drifted: expected %, got %', $expected, actual_count;
  END IF;
  SELECT count(*) INTO foreign_key_count FROM pg_constraint WHERE contype = 'f' AND confrelid = 'account_model_detection_runs'::regclass;
  IF foreign_key_count <> 0 THEN
    RAISE EXCEPTION 'foreign-key guard rejected purge';
  END IF;
  DELETE FROM account_model_detection_runs WHERE detector_version IS DISTINCT FROM '4.1.1';
  GET DIAGNOSTICS deleted_count = ROW_COUNT;
  IF deleted_count <> $expected THEN
    RAISE EXCEPTION 'deleted count drifted: expected %, got %', $expected, deleted_count;
  END IF;
  SELECT count(*) INTO remaining_count FROM account_model_detection_runs WHERE detector_version IS DISTINCT FROM '4.1.1';
  IF remaining_count <> 0 THEN
    RAISE EXCEPTION 'non-v4.1.1 rows remain: %', remaining_count;
  END IF;
END
\$\$;
COMMIT;
SQL

printf 'T77 detection-history purge succeeded rows=%s backup=%s sha256=%s\n' "$expected" "$archive" "$archive_sha256"
