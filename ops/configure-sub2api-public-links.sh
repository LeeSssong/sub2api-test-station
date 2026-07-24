#!/usr/bin/env bash
set -euo pipefail

: "${SUB2API_COMPOSE_PROJECT:?Set SUB2API_COMPOSE_PROJECT to the Compose project identity}"
: "${SUB2API_PROJECT_DIRECTORY:?Set SUB2API_PROJECT_DIRECTORY to the explicit Compose project directory}"
: "${SUB2API_SECRET_ENV_FILE:?Set SUB2API_SECRET_ENV_FILE to the protected deployment environment file}"
: "${SUB2API_RELEASE_ENV_FILE:?Set SUB2API_RELEASE_ENV_FILE to the release environment file}"
: "${SUB2API_COMPOSE_FILE:?Set SUB2API_COMPOSE_FILE to the explicit base Compose file}"
: "${SUB2API_IMAGE_OVERLAY:?Set SUB2API_IMAGE_OVERLAY to the explicit image overlay Compose file}"

require_readable_file() {
  local label=$1 path=$2
  [[ -f "$path" && -r "$path" && ! -L "$path" ]] || {
    printf '%s must be a readable regular non-symlink file: %s\n' "$label" "$path" >&2
    exit 1
  }
}

require_readable_directory() {
  local label=$1 path=$2
  [[ -d "$path" && -r "$path" && ! -L "$path" ]] || {
    printf '%s must be a readable non-symlink directory: %s\n' "$label" "$path" >&2
    exit 1
  }
}

case "$SUB2API_COMPOSE_PROJECT" in
  sub2api-deploy|sub2api-official-rehearsal) ;;
  *)
    printf 'Compose project identity must be sub2api-deploy or sub2api-official-rehearsal\n' >&2
    exit 1
    ;;
esac
require_readable_directory 'Compose project directory' "$SUB2API_PROJECT_DIRECTORY"
require_readable_file 'Secret environment file' "$SUB2API_SECRET_ENV_FILE"
require_readable_file 'Release environment file' "$SUB2API_RELEASE_ENV_FILE"
require_readable_file 'Base Compose file' "$SUB2API_COMPOSE_FILE"
require_readable_file 'Image overlay Compose file' "$SUB2API_IMAGE_OVERLAY"
command -v docker >/dev/null || { printf 'docker is required\n' >&2; exit 1; }

compose=(docker compose
  --project-name "$SUB2API_COMPOSE_PROJECT"
  --project-directory "$SUB2API_PROJECT_DIRECTORY"
  --env-file "$SUB2API_SECRET_ENV_FILE"
  --env-file "$SUB2API_RELEASE_ENV_FILE"
  -f "$SUB2API_COMPOSE_FILE"
  -f "$SUB2API_IMAGE_OVERLAY")

run_database_transaction() {
  "${compose[@]}" exec -T postgres \
    sh -c 'exec psql -v ON_ERROR_STOP=1 -v VERBOSITY=verbose -U "$POSTGRES_USER" -d "$POSTGRES_DB"' >/dev/null <<'SQL'
\set VERBOSITY verbose

BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;

SET LOCAL application_name = 'configure-sub2api-public-links';

SELECT pg_advisory_xact_lock(hashtext('sub2api:settings:public-links'));

INSERT INTO settings (key, value, updated_at)
VALUES
  ('doc_url', 'https://api.xingqiaolab.top/docs/', NOW()),
  ('balance_low_notify_recharge_url', 'https://api.xingqiaolab.top/custom/xingqiao-storefront', NOW())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = EXCLUDED.updated_at;

COMMIT;
SQL
}

database_error=$(mktemp)
cleanup() {
  rm -f -- "$database_error"
}
trap cleanup EXIT

max_attempts=5
for ((attempt = 1; attempt <= max_attempts; attempt++)); do
  : > "$database_error"
  if run_database_transaction 2>"$database_error"; then
    break
  else
    transaction_status=$?
  fi

  database_failure=$(<"$database_error")
  if [[ "$database_failure" =~ ERROR:[[:space:]]+(40001|40P01): ]] && ((attempt < max_attempts)); then
    sleep "0.$attempt"
    continue
  fi

  printf '%s\n' "$database_failure" >&2
  exit "$transaction_status"
done

printf 'Configured Sub2API public links\n'
