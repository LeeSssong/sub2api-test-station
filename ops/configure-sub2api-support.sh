#!/usr/bin/env bash
set -euo pipefail

: "${SUB2API_COMPOSE_PROJECT:?Set SUB2API_COMPOSE_PROJECT to the Compose project identity}"
: "${SUB2API_PROJECT_DIRECTORY:?Set SUB2API_PROJECT_DIRECTORY to the explicit Compose project directory}"
: "${SUB2API_SECRET_ENV_FILE:?Set SUB2API_SECRET_ENV_FILE to the protected deployment environment file}"
: "${SUB2API_RELEASE_ENV_FILE:?Set SUB2API_RELEASE_ENV_FILE to the release environment file}"
: "${SUB2API_COMPOSE_FILE:?Set SUB2API_COMPOSE_FILE to the explicit base Compose file}"
: "${SUB2API_IMAGE_OVERLAY:?Set SUB2API_IMAGE_OVERLAY to the explicit image overlay Compose file}"
: "${SUB2API_DATA_DIR:?Set SUB2API_DATA_DIR to the writable Sub2API data directory}"

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
require_readable_directory 'Sub2API data directory' "$SUB2API_DATA_DIR"
[[ -w "$SUB2API_DATA_DIR" ]] || { printf 'Sub2API data directory is not writable: %s\n' "$SUB2API_DATA_DIR" >&2; exit 1; }
command -v docker >/dev/null || { printf 'docker is required\n' >&2; exit 1; }
command -v sha256sum >/dev/null || { printf 'sha256sum is required\n' >&2; exit 1; }

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
pages_dir="$SUB2API_DATA_DIR/pages"
support_assets_dir="$pages_dir/support"
support_asset="$support_assets_dir/qq-group-1080152144.png"
support_asset_source="$repo_root/homepage/public/support/qq-group-1080152144.png"
support_asset_sha256='35b84b14ab472e117fa413ed5f91357becd01199eeaf3fed469a2d9d3d987c16'
[[ ! -L "$pages_dir" ]] || { printf 'Refusing symlinked pages directory: %s\n' "$pages_dir" >&2; exit 1; }
mkdir -p "$pages_dir"
[[ ! -L "$pages_dir/support.md" ]] || { printf 'Refusing symlinked support page: %s\n' "$pages_dir/support.md" >&2; exit 1; }
[[ ! -L "$support_assets_dir" ]] || { printf 'Refusing symlinked support assets directory: %s\n' "$support_assets_dir" >&2; exit 1; }
mkdir -p "$support_assets_dir"
[[ -d "$support_assets_dir" && ! -L "$support_assets_dir" ]] || {
  printf 'Support assets path is not a directory: %s\n' "$support_assets_dir" >&2
  exit 1
}
[[ ! -L "$support_asset" ]] || { printf 'Refusing symlinked support asset: %s\n' "$support_asset" >&2; exit 1; }
require_readable_file 'Support QR source' "$support_asset_source"
[[ "$(sha256sum "$support_asset_source" | awk '{print $1}')" == "$support_asset_sha256" ]] || {
  printf 'Support QR source hash mismatch\n' >&2
  exit 1
}

temporary_page="$pages_dir/.support.md.$$"
temporary_asset="$support_assets_dir/.qq-group-1080152144.png.$$"
database_error=$(mktemp)
cleanup() {
  rm -f -- "$temporary_page" "$temporary_asset" "$database_error"
}
trap cleanup EXIT
install -m 0644 "$support_asset_source" "$temporary_asset"
[[ "$(sha256sum "$temporary_asset" | awk '{print $1}')" == "$support_asset_sha256" ]] || {
  printf 'Installed support QR hash mismatch\n' >&2
  exit 1
}
mv "$temporary_asset" "$support_asset"
install -m 0644 "$repo_root/config/sub2api/support.md" "$temporary_page"
mv "$temporary_page" "$pages_dir/support.md"

compose=(docker compose
  --project-name "$SUB2API_COMPOSE_PROJECT"
  --project-directory "$SUB2API_PROJECT_DIRECTORY"
  --env-file "$SUB2API_SECRET_ENV_FILE"
  --env-file "$SUB2API_RELEASE_ENV_FILE"
  -f "$SUB2API_COMPOSE_FILE"
  -f "$SUB2API_IMAGE_OVERLAY")

run_database_transaction() {
  "${compose[@]}" exec -T postgres \
    sh -c 'exec psql -v ON_ERROR_STOP=1 -v VERBOSITY=verbose -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
\set VERBOSITY verbose

BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;

SET LOCAL application_name = 'configure-sub2api-support';

SELECT pg_advisory_xact_lock(hashtext('sub2api:settings:custom_menu_items'));

WITH current_value AS MATERIALIZED (
  SELECT CASE
    WHEN pg_input_is_valid(value, 'jsonb') THEN
      CASE
        WHEN jsonb_typeof(value::jsonb) = 'array' THEN value::jsonb
        ELSE '[]'::jsonb
      END
    ELSE '[]'::jsonb
  END AS items
  FROM settings
  WHERE key = 'custom_menu_items'
  FOR UPDATE
), existing_items AS (
  SELECT item, ordinal
  FROM jsonb_array_elements(
    COALESCE((SELECT items FROM current_value), '[]'::jsonb)
  ) WITH ORDINALITY AS existing(item, ordinal)
  WHERE item ->> 'id' IS DISTINCT FROM 'xingqiao-support'
), merged_items AS (
  SELECT item, ordinal
  FROM existing_items
  UNION ALL
  SELECT jsonb_build_object(
    'id', 'xingqiao-support',
    'label', '联系客服',
    'icon_svg', '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a4 4 0 0 1-4 4H8l-5 3V7a4 4 0 0 1 4-4h10a4 4 0 0 1 4 4z"/></svg>',
    'url', 'md:support',
    'page_slug', 'support',
    'visibility', 'user',
    'sort_order', 80
  ), 9223372036854775807::bigint
)
INSERT INTO settings (key, value, updated_at)
SELECT 'custom_menu_items', jsonb_agg(item ORDER BY ordinal)::text, NOW()
FROM merged_items
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = EXCLUDED.updated_at;

COMMIT;
SQL
}

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

printf 'Configured Sub2API support page and menu\n'
