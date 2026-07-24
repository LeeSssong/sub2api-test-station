#!/usr/bin/env bash
set -euo pipefail

umask 077

fail() {
  printf 'release smoke failed\n' >&2
  exit 1
}

canonical_directory() {
  local path=$1 physical
  [[ "$path" == /* && -d "$path" && ! -L "$path" ]] || fail
  physical=$(cd "$path" && pwd -P)
  [[ "$physical" == "$path" ]] || fail
  printf '%s\n' "$physical"
}

canonical_file() {
  local path=$1 parent physical_parent canonical_parent
  [[ "$path" == /* && -f "$path" && -r "$path" && ! -L "$path" ]] || fail
  parent=$(dirname "$path")
  physical_parent=$(cd "$parent" && pwd -P)
  canonical_parent=$physical_parent
  [[ "$canonical_parent" != / ]] || canonical_parent=''
  [[ "$canonical_parent/$(basename "$path")" == "$path" ]] || fail
  printf '%s\n' "$path"
}

base_url=${BASE_URL:-}
expected_version=${EXPECTED_VERSION:-}
compose_project=${COMPOSE_PROJECT_NAME:-sub2api-deploy}
[[ -n "$base_url" && -n "$expected_version" ]] || fail
[[ "$compose_project" == sub2api-deploy || "$compose_project" == sub2api-official-rehearsal ]] || fail

deploy_root=$(canonical_directory "${DEPLOY_ROOT:-}")
secret_env=$(canonical_file "${SECRET_ENV:-}")
release_env=$(canonical_file "${RELEASE_ENV:-}")
base_compose=$(canonical_file "${BASE_COMPOSE:-}")
image_overlay=$(canonical_file "${IMAGE_OVERLAY:-}")
admin_key_file=$(canonical_file "${ADMIN_API_KEY_FILE:-}")
gateway_key_file=$(canonical_file "${GATEWAY_API_KEY_FILE:-}")
expected_counts_file=$(canonical_file "${EXPECTED_RECORD_COUNTS_FILE:-}")

compose=(docker compose --project-name "$compose_project" --project-directory "$deploy_root"
  --env-file "$secret_env" --env-file "$release_env"
  -f "$base_compose" -f "$image_overlay")
curl_bounds=(--connect-timeout 5 --max-time 15 --max-filesize 1048576)

[[ $(wc -c <"$admin_key_file" | tr -d ' ') -le 1024 ]] || fail
[[ $(wc -c <"$gateway_key_file" | tr -d ' ') -le 1024 ]] || fail
[[ $(wc -c <"$expected_counts_file" | tr -d ' ') -le 1048576 ]] || fail
admin_key=$(tr -d '\r\n' <"$admin_key_file")
gateway_key=$(tr -d '\r\n' <"$gateway_key_file")
[[ -n "$admin_key" && -n "$gateway_key" ]] || fail
base_url=${base_url%/}

jq -e '
  type == "object" and
  (keys | sort == ["accounts", "api_keys", "groups", "settings", "usage_logs", "users"]) and
  ([.users, .accounts, .groups, .api_keys, .settings, .usage_logs] |
    all(type == "number" and . >= 0 and floor == .))
' "$expected_counts_file" >/dev/null 2>&1 || fail

guard_body=$(mktemp)
post_counts=$(mktemp)
cleanup() {
  rm -f -- "$guard_body" "$post_counts"
}
trap cleanup EXIT

curl "${curl_bounds[@]}" -fsS "$base_url/health" 2>/dev/null |
  jq -e '.status == "ok"' >/dev/null 2>&1 || fail

curl "${curl_bounds[@]}" -fsS -H "X-API-Key: $admin_key" \
  "$base_url/api/v1/admin/system/version" 2>/dev/null |
  jq -e --arg version "$expected_version" '(.data // .).version == $version' >/dev/null 2>&1 || fail

curl "${curl_bounds[@]}" -fsS "$base_url/api/v1/settings/public" 2>/dev/null |
  jq -e '(.data // .).custom_menu_items | any(.id == "xingqiao-support" and .url == "md:support")' \
  >/dev/null 2>&1 || fail

require_guard() {
  local endpoint=$1 status
  status=$(curl "${curl_bounds[@]}" -sS -o "$guard_body" -w '%{http_code}' -X POST \
    -H "X-API-Key: $admin_key" "$base_url$endpoint" 2>/dev/null) || fail
  [[ "$status" == 409 ]] || fail
  jq -e 'type == "object" and .code == "DOCKER_DEPLOYMENT_UPDATE_REQUIRED"' \
    "$guard_body" >/dev/null 2>&1 || fail
}

require_guard '/api/v1/admin/system/update'
require_guard '/api/v1/admin/system/rollback'

curl "${curl_bounds[@]}" -fsS -H "Authorization: Bearer $gateway_key" \
  "$base_url/v1/models" 2>/dev/null |
  jq -e '.data | type == "array"' >/dev/null 2>&1 || fail

"${compose[@]}" exec -T postgres \
  sh -c 'exec psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At' <<'SQL' >"$post_counts" 2>/dev/null
select json_build_object(
  'users', (select count(*) from users),
  'accounts', (select count(*) from accounts),
  'groups', (select count(*) from groups),
  'api_keys', (select count(*) from api_keys),
  'settings', (select count(*) from settings),
  'usage_logs', (select count(*) from usage_logs)
)::text;
SQL

jq -e --slurpfile expected "$expected_counts_file" '
  . as $actual |
  $expected[0] as $baseline |
  type == "object" and
  (["users", "accounts", "groups", "api_keys", "settings", "usage_logs"] |
    all(.[]; ($actual[.] | type == "number") and $actual[.] >= $baseline[.]))
' "$post_counts" >/dev/null 2>&1 || fail

printf 'release_smoke=passed\n'
