#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
fixture=$(mktemp -d)
real_docker=$(command -v docker || true)
postgres_container=''

cleanup() {
  if [[ -n "$postgres_container" && -n "$real_docker" ]]; then
    "$real_docker" rm --force --volumes "$postgres_container" >/dev/null 2>&1 || true
  fi
  rm -rf "$fixture"
}
trap cleanup EXIT INT TERM

mkdir -p "$fixture/bin" "$fixture/data/pages" "$fixture/project"
: > "$fixture/compose.yaml"
: > "$fixture/compose.image.yaml"
: > "$fixture/secret.env"
: > "$fixture/release.env"
: > "$fixture/docker-invocations"
printf '%s\n' '{"settings":{"custom_menu_items":[{"id":"xingqiao-storefront","label":"充值/订阅","url":"https://catfk.com/shop/DLK8SNUJ","visibility":"user","sort_order":90},{"id":"xingqiao-support","url":"md:obsolete","visibility":"user"}],"payment_enabled":true}}' > "$fixture/db.json"

cat > "$fixture/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

fixture=${SUPPORT_TEST_FIXTURE:?}
project=${SUB2API_COMPOSE_PROJECT:?}
[[ "$project" == 'sub2api-deploy' || "$project" == 'sub2api-official-rehearsal' ]] || exit 89
expected="compose --project-name $project --project-directory $fixture/project --env-file $fixture/secret.env --env-file $fixture/release.env -f $fixture/compose.yaml -f $fixture/compose.image.yaml exec -T postgres sh -c exec psql -v ON_ERROR_STOP=1 -v VERBOSITY=verbose -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\""
[[ "$*" == "$expected" ]] || {
  printf 'unexpected Compose identity: %s\n' "$*" >&2
  exit 90
}
printf '%s\n' "$*" >> "$fixture/docker-invocations"
invocation=$(wc -l < "$fixture/docker-invocations" | tr -d ' ')
sql_file="$fixture/sql.$invocation"
cat > "$sql_file"

support_item='{"id":"xingqiao-support","label":"联系客服","icon_svg":"<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\"><path d=\"M21 15a4 4 0 0 1-4 4H8l-5 3V7a4 4 0 0 1 4-4h10a4 4 0 0 1 4 4z\"/></svg>","url":"md:support","page_slug":"support","visibility":"user","sort_order":80}'
jq --argjson support "$support_item" '
  (.settings.custom_menu_items // []) as $current
  | .settings.custom_menu_items = (
      (if ($current | type) == "array" then $current else [] end
        | map(select(.id != "xingqiao-support"))) + [$support]
    )
' "$fixture/db.json" > "$fixture/db.next.json"
mv "$fixture/db.next.json" "$fixture/db.json"
EOF
chmod +x "$fixture/bin/docker"

support_context=(
  "PATH=$fixture/bin:$PATH"
  "SUPPORT_TEST_FIXTURE=$fixture"
  'SUB2API_COMPOSE_PROJECT=sub2api-deploy'
  "SUB2API_PROJECT_DIRECTORY=$fixture/project"
  "SUB2API_SECRET_ENV_FILE=$fixture/secret.env"
  "SUB2API_RELEASE_ENV_FILE=$fixture/release.env"
  "SUB2API_COMPOSE_FILE=$fixture/compose.yaml"
  "SUB2API_IMAGE_OVERLAY=$fixture/compose.image.yaml"
  "SUB2API_DATA_DIR=$fixture/data"
)

run_configure() {
  env "${support_context[@]}" bash "$ROOT/ops/configure-sub2api-support.sh"
}

output=$(run_configure)
cmp "$ROOT/config/sub2api/support.md" "$fixture/data/pages/support.md"
cmp "$ROOT/homepage/public/support/qq-group-1080152144.png" "$fixture/data/pages/support/qq-group-1080152144.png"
! rg -n -i 'token|user_id|src_url|https?://' "$fixture/data/pages/support.md"
rg -Fq '![QQ群 1080152144 二维码](qq-group-1080152144.png)' "$fixture/data/pages/support.md"
rg -Fq 'QQ群号：1080152144' "$fixture/data/pages/support.md"
! rg -n -i '<iframe|/support' "$fixture/data/pages/support.md"
[[ "$(sha256sum "$fixture/data/pages/support/qq-group-1080152144.png" | awk '{print $1}')" == '35b84b14ab472e117fa413ed5f91357becd01199eeaf3fed469a2d9d3d987c16' ]]
[[ $(wc -l < "$fixture/docker-invocations" | tr -d ' ') == 1 ]]
rg -Fq -- "compose --project-name sub2api-deploy --project-directory $fixture/project --env-file $fixture/secret.env --env-file $fixture/release.env -f $fixture/compose.yaml -f $fixture/compose.image.yaml exec -T postgres" "$fixture/docker-invocations"
rg -Fq 'BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;' "$fixture/sql.1"
rg -Fq '\set VERBOSITY verbose' "$fixture/sql.1"
rg -Fq "SET LOCAL application_name = 'configure-sub2api-support';" "$fixture/sql.1"
rg -Fq "FROM settings" "$fixture/sql.1"
rg -Fq "WHERE key = 'custom_menu_items'" "$fixture/sql.1"
rg -Fq "INSERT INTO settings (key, value, updated_at)" "$fixture/sql.1"
rg -Fq "SELECT 'custom_menu_items'" "$fixture/sql.1"
rg -Fq 'ON CONFLICT (key) DO UPDATE' "$fixture/sql.1"
rg -Fq 'updated_at = EXCLUDED.updated_at' "$fixture/sql.1"
rg -Fq "jsonb_typeof(value::jsonb) = 'array'" "$fixture/sql.1"
rg -Fq "item ->> 'id' IS DISTINCT FROM 'xingqiao-support'" "$fixture/sql.1"
rg -Fq 'COMMIT;' "$fixture/sql.1"
! rg -n '(UPDATE[[:space:]]+settings|DELETE[[:space:]]+FROM[[:space:]]+settings)' "$fixture/sql.1"
! rg -n -i 'admin[_ -]?api[_ -]?key|test-admin-key' "$ROOT/ops/configure-sub2api-support.sh" "$fixture/sql.1"
! rg -n -i 'admin[_ -]?api[_ -]?key|test-admin-key' <<< "$output"
jq -e '.settings.payment_enabled == true' "$fixture/db.json" >/dev/null
jq -e '.settings.custom_menu_items | length == 2' "$fixture/db.json" >/dev/null
jq -e '.settings.custom_menu_items | any(.id == "xingqiao-storefront")' "$fixture/db.json" >/dev/null
jq -e '[.settings.custom_menu_items[] | select(.id == "xingqiao-support" and .url == "md:support" and .visibility == "user")] | length == 1' "$fixture/db.json" >/dev/null

run_configure >/dev/null
jq -e '.settings.custom_menu_items | length == 2' "$fixture/db.json" >/dev/null
jq -e '[.settings.custom_menu_items[] | select(.id == "xingqiao-support")] | length == 1' "$fixture/db.json" >/dev/null
cmp "$fixture/sql.1" "$fixture/sql.2"

env "${support_context[@]}" SUB2API_COMPOSE_PROJECT=sub2api-official-rehearsal \
  bash "$ROOT/ops/configure-sub2api-support.sh" >/dev/null
rg -Fq -- 'compose --project-name sub2api-official-rehearsal' "$fixture/docker-invocations"

required_context=(
  SUB2API_COMPOSE_PROJECT
  SUB2API_PROJECT_DIRECTORY
  SUB2API_SECRET_ENV_FILE
  SUB2API_RELEASE_ENV_FILE
  SUB2API_COMPOSE_FILE
  SUB2API_IMAGE_OVERLAY
  SUB2API_DATA_DIR
)
for missing in "${required_context[@]}"; do
  reduced_context=()
  for assignment in "${support_context[@]}"; do
    [[ "$assignment" == "$missing="* ]] || reduced_context+=("$assignment")
  done
  before=$(wc -l < "$fixture/docker-invocations" | tr -d ' ')
  if env -u "$missing" "${reduced_context[@]}" bash "$ROOT/ops/configure-sub2api-support.sh" >/dev/null 2>&1; then
    printf 'FAIL: accepted missing deployment context: %s\n' "$missing" >&2
    exit 1
  fi
  after=$(wc -l < "$fixture/docker-invocations" | tr -d ' ')
  [[ "$before" == "$after" ]]
done

if env "${support_context[@]}" SUB2API_COMPOSE_PROJECT=not-production bash "$ROOT/ops/configure-sub2api-support.sh" >/dev/null 2>&1; then
  printf 'FAIL: accepted changed Compose project identity\n' >&2
  exit 1
fi

assert_context_path_rejected() {
  local variable=$1 value=$2 label=$3 before after
  before=$(wc -l < "$fixture/docker-invocations" | tr -d ' ')
  if env "${support_context[@]}" "$variable=$value" bash "$ROOT/ops/configure-sub2api-support.sh" >/dev/null 2>&1; then
    printf 'FAIL: accepted invalid deployment context path: %s\n' "$label" >&2
    exit 1
  fi
  after=$(wc -l < "$fixture/docker-invocations" | tr -d ' ')
  [[ "$before" == "$after" ]]
}

ln -s "$fixture/secret.env" "$fixture/symlink-secret.env"
ln -s "$fixture/release.env" "$fixture/symlink-release.env"
ln -s "$fixture/compose.yaml" "$fixture/symlink-compose.yaml"
ln -s "$fixture/compose.image.yaml" "$fixture/symlink-overlay.yaml"
ln -s "$fixture/project" "$fixture/symlink-project"
ln -s "$fixture/data" "$fixture/symlink-data"
assert_context_path_rejected SUB2API_SECRET_ENV_FILE "$fixture/symlink-secret.env" 'secret env symlink'
assert_context_path_rejected SUB2API_RELEASE_ENV_FILE "$fixture/symlink-release.env" 'release env symlink'
assert_context_path_rejected SUB2API_COMPOSE_FILE "$fixture/symlink-compose.yaml" 'base Compose symlink'
assert_context_path_rejected SUB2API_IMAGE_OVERLAY "$fixture/symlink-overlay.yaml" 'image overlay symlink'
assert_context_path_rejected SUB2API_PROJECT_DIRECTORY "$fixture/symlink-project" 'project directory symlink'
assert_context_path_rejected SUB2API_DATA_DIR "$fixture/symlink-data" 'data directory symlink'

: > "$fixture/changed-release.env"
assert_context_path_rejected SUB2API_RELEASE_ENV_FILE "$fixture/changed-release.env" 'changed release identity'

mkdir -p "$fixture/retry-bin"
cat > "$fixture/retry-bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
fixture=${SUPPORT_TEST_FIXTURE:?}
cat >/dev/null
printf 'attempt\n' >> "$fixture/retry-attempts"
printf 'ERROR:  40001: could not serialize access due to concurrent update\n' >&2
exit 3
EOF
chmod +x "$fixture/retry-bin/docker"
: > "$fixture/retry-attempts"
if env "${support_context[@]}" PATH="$fixture/retry-bin:$PATH" \
    bash "$ROOT/ops/configure-sub2api-support.sh" > "$fixture/retry-output" 2>&1; then
  printf 'FAIL: serialization retry exhaustion reported success\n' >&2
  exit 1
fi
[[ $(wc -l < "$fixture/retry-attempts" | tr -d ' ') == 5 ]]
! rg -Fq 'Configured Sub2API support page and menu' "$fixture/retry-output"

jq '.settings.custom_menu_items = {"invalid":"not-an-array"}' "$fixture/db.json" > "$fixture/db.next.json"
mv "$fixture/db.next.json" "$fixture/db.json"
run_configure >/dev/null
jq -e '.settings.payment_enabled == true' "$fixture/db.json" >/dev/null
jq -e '.settings.custom_menu_items | length == 1 and .[0].id == "xingqiao-support"' "$fixture/db.json" >/dev/null

jq 'del(.settings.custom_menu_items)' "$fixture/db.json" > "$fixture/db.next.json"
mv "$fixture/db.next.json" "$fixture/db.json"
run_configure >/dev/null
jq -e '.settings.payment_enabled == true' "$fixture/db.json" >/dev/null
jq -e '.settings.custom_menu_items | length == 1 and .[0].id == "xingqiao-support"' "$fixture/db.json" >/dev/null

assert_symlink_rejected_without_db() {
  local data_dir=$1 before after
  before=$(wc -l < "$fixture/docker-invocations" | tr -d ' ')
  if PATH="$fixture/bin:$PATH" \
      SUPPORT_TEST_FIXTURE="$fixture" \
      SUB2API_COMPOSE_PROJECT=sub2api-deploy \
      SUB2API_SECRET_ENV_FILE="$fixture/secret.env" \
      SUB2API_RELEASE_ENV_FILE="$fixture/release.env" \
      SUB2API_COMPOSE_FILE="$fixture/compose.yaml" \
      SUB2API_IMAGE_OVERLAY="$fixture/compose.image.yaml" \
      SUB2API_PROJECT_DIRECTORY="$fixture/project" \
      SUB2API_DATA_DIR="$data_dir" \
      bash "$ROOT/ops/configure-sub2api-support.sh" > "$fixture/symlink-output" 2>&1; then
    printf 'FAIL: accepted symlinked support path\n' >&2
    exit 1
  fi
  after=$(wc -l < "$fixture/docker-invocations" | tr -d ' ')
  [[ "$before" == "$after" ]]
  ! rg -n -i 'admin[_ -]?api[_ -]?key|test-admin-key' "$fixture/symlink-output"
}

mkdir -p "$fixture/symlink-pages-data"
ln -s "$fixture/elsewhere" "$fixture/symlink-pages-data/pages"
assert_symlink_rejected_without_db "$fixture/symlink-pages-data"

mkdir -p "$fixture/symlink-page-data/pages"
ln -s "$fixture/elsewhere" "$fixture/symlink-page-data/pages/support.md"
assert_symlink_rejected_without_db "$fixture/symlink-page-data"

mkdir -p "$fixture/symlink-assets-data/pages"
ln -s "$fixture/elsewhere" "$fixture/symlink-assets-data/pages/support"
assert_symlink_rejected_without_db "$fixture/symlink-assets-data"

mkdir -p "$fixture/symlink-asset-data/pages/support"
ln -s "$fixture/elsewhere" "$fixture/symlink-asset-data/pages/support/qq-group-1080152144.png"
assert_symlink_rejected_without_db "$fixture/symlink-asset-data"

printf 'PASS: lightweight support transaction and deployment identity contracts\n'

[[ -n "$real_docker" ]] || {
  printf 'FAIL: Docker is required for PostgreSQL 18 support transaction integration\n' >&2
  exit 1
}
"$real_docker" info >/dev/null 2>&1 || {
  printf 'FAIL: Docker daemon is unavailable for PostgreSQL 18 support transaction integration\n' >&2
  exit 1
}

postgres_image='postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15'
postgres_container="sub2api-support-pg-$$-$RANDOM"
"$real_docker" run --detach \
  --name "$postgres_container" \
  --network none \
  --env POSTGRES_USER=postgres \
  --env POSTGRES_PASSWORD=support-integration-only \
  --env POSTGRES_DB=postgres \
  "$postgres_image" >/dev/null

postgres_ready=false
for _attempt in $(seq 1 30); do
  if "$real_docker" exec "$postgres_container" pg_isready -U postgres -d postgres >/dev/null 2>&1; then
    postgres_ready=true
    break
  fi
  sleep 1
done
[[ "$postgres_ready" == true ]] || {
  "$real_docker" logs "$postgres_container" >&2 || true
  printf 'FAIL: ephemeral PostgreSQL 18 did not become ready\n' >&2
  exit 1
}

"$real_docker" exec -i "$postgres_container" \
  psql -v ON_ERROR_STOP=1 -U postgres -d postgres >/dev/null <<'SQL'
CREATE TABLE settings (
  id BIGSERIAL PRIMARY KEY,
  key VARCHAR(100) NOT NULL UNIQUE,
  value TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO settings (key, value, updated_at)
VALUES ('unrelated', '{"preserve":"exact"}', '2024-01-02T03:04:05Z');
SQL

integration="$fixture/postgres-integration"
mkdir -p "$integration/bin" "$integration/data/pages" "$integration/project"
: > "$integration/compose.yaml"
: > "$integration/compose.image.yaml"
: > "$integration/secret.env"
: > "$integration/release.env"

cat > "$integration/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

integration=${SUPPORT_INTEGRATION_ROOT:?}
real_docker=${SUPPORT_REAL_DOCKER:?}
container=${SUPPORT_POSTGRES_CONTAINER:?}
expected="compose --project-name sub2api-deploy --project-directory $integration/project --env-file $integration/secret.env --env-file $integration/release.env -f $integration/compose.yaml -f $integration/compose.image.yaml exec -T postgres sh -c exec psql -v ON_ERROR_STOP=1 -v VERBOSITY=verbose -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\""
[[ "$*" == "$expected" ]] || {
  printf 'FAIL: integration wrapper rejected Compose identity: %s\n' "$*" >&2
  exit 90
}
printf 'attempt\n' >> "${SUPPORT_INTEGRATION_LOG:?}"
exec "$real_docker" exec -i "$container" \
  sh -c 'exec psql -v ON_ERROR_STOP=1 -U postgres -d postgres'
EOF
chmod +x "$integration/bin/docker"

integration_context=(
  "PATH=$integration/bin:$PATH"
  "SUPPORT_INTEGRATION_ROOT=$integration"
  "SUPPORT_REAL_DOCKER=$real_docker"
  "SUPPORT_POSTGRES_CONTAINER=$postgres_container"
  "SUPPORT_INTEGRATION_LOG=$integration/invocations"
  'SUB2API_COMPOSE_PROJECT=sub2api-deploy'
  "SUB2API_PROJECT_DIRECTORY=$integration/project"
  "SUB2API_SECRET_ENV_FILE=$integration/secret.env"
  "SUB2API_RELEASE_ENV_FILE=$integration/release.env"
  "SUB2API_COMPOSE_FILE=$integration/compose.yaml"
  "SUB2API_IMAGE_OVERLAY=$integration/compose.image.yaml"
  "SUB2API_DATA_DIR=$integration/data"
)
: > "$integration/invocations"

run_postgres_configure() {
  env "${integration_context[@]}" bash "$ROOT/ops/configure-sub2api-support.sh"
}

pg_sql() {
  "$real_docker" exec "$postgres_container" \
    psql -v ON_ERROR_STOP=1 -U postgres -d postgres -At -c "$1"
}

unrelated_before=$(pg_sql "SELECT row_to_json(s)::text FROM (SELECT key, value, updated_at FROM settings WHERE key = 'unrelated') AS s;")

pg_sql "INSERT INTO settings (key, value, updated_at) VALUES ('custom_menu_items', '[{\"id\":\"first\",\"marker\":1},{\"id\":\"xingqiao-storefront\",\"label\":\"keep\",\"sort_order\":90},{\"id\":\"xingqiao-support\",\"url\":\"md:obsolete\"},{\"id\":\"last\",\"marker\":4}]', NOW()) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at;" >/dev/null
run_postgres_configure >/dev/null
valid_array=$(pg_sql "SELECT value FROM settings WHERE key = 'custom_menu_items';")
jq -e 'map(.id) == ["first", "xingqiao-storefront", "last", "xingqiao-support"]' <<< "$valid_array" >/dev/null
jq -e '.[1] == {"id":"xingqiao-storefront","label":"keep","sort_order":90}' <<< "$valid_array" >/dev/null
jq -e '[.[] | select(.id == "xingqiao-support" and .url == "md:support")] | length == 1' <<< "$valid_array" >/dev/null

run_postgres_configure >/dev/null
idempotent_array=$(pg_sql "SELECT value FROM settings WHERE key = 'custom_menu_items';")
[[ "$valid_array" == "$idempotent_array" ]]

pg_sql "UPDATE settings SET value = 'not-json{' WHERE key = 'custom_menu_items';" >/dev/null
run_postgres_configure >/dev/null
invalid_json=$(pg_sql "SELECT value FROM settings WHERE key = 'custom_menu_items';")
jq -e 'length == 1 and .[0].id == "xingqiao-support"' <<< "$invalid_json" >/dev/null

pg_sql "UPDATE settings SET value = '{\"valid\":\"but-not-array\"}' WHERE key = 'custom_menu_items';" >/dev/null
run_postgres_configure >/dev/null
non_array=$(pg_sql "SELECT value FROM settings WHERE key = 'custom_menu_items';")
jq -e 'length == 1 and .[0].id == "xingqiao-support"' <<< "$non_array" >/dev/null

pg_sql "DELETE FROM settings WHERE key = 'custom_menu_items';" >/dev/null
run_postgres_configure >/dev/null
missing_row=$(pg_sql "SELECT value FROM settings WHERE key = 'custom_menu_items';")
jq -e 'length == 1 and .[0].id == "xingqiao-support"' <<< "$missing_row" >/dev/null

pg_sql "DELETE FROM settings WHERE key = 'custom_menu_items';" >/dev/null
pg_sql "CREATE OR REPLACE FUNCTION block_support_insert() RETURNS trigger LANGUAGE plpgsql AS \$\$ BEGIN IF NEW.key = 'custom_menu_items' AND current_setting('application_name') = 'configure-sub2api-support' THEN PERFORM pg_advisory_xact_lock(741852963); END IF; RETURN NEW; END \$\$; CREATE TRIGGER block_support_insert BEFORE INSERT ON settings FOR EACH ROW EXECUTE FUNCTION block_support_insert();" >/dev/null

"$real_docker" exec "$postgres_container" psql -v ON_ERROR_STOP=1 -U postgres -d postgres \
  -c "SET application_name = 'support-test-lock-holder'; SELECT pg_advisory_lock(741852963); SELECT pg_sleep(60);" \
  > "$integration/lock-holder" 2>&1 &
lock_holder_pid=$!
lock_ready=false
for _attempt in $(seq 1 50); do
  if [[ $(pg_sql "SELECT count(*) FROM pg_stat_activity a JOIN pg_locks l ON l.pid = a.pid WHERE a.application_name = 'support-test-lock-holder' AND l.locktype = 'advisory' AND l.granted;") == 1 ]]; then
    lock_ready=true
    break
  fi
  sleep 0.1
done
[[ "$lock_ready" == true ]]

attempts_before=$(wc -l < "$integration/invocations" | tr -d ' ')
run_postgres_configure > "$integration/concurrent-config" 2>&1 &
configure_pid=$!
configure_blocked=false
for _attempt in $(seq 1 50); do
  if [[ $(pg_sql "SELECT count(*) FROM pg_stat_activity WHERE application_name = 'configure-sub2api-support' AND wait_event = 'advisory';") == 1 ]]; then
    configure_blocked=true
    break
  fi
  sleep 0.1
done
[[ "$configure_blocked" == true ]]

pg_sql "INSERT INTO settings (key, value, updated_at) VALUES ('custom_menu_items', '[{\"id\":\"non-cooperating-menu\",\"source\":\"external\"}]', NOW());" >/dev/null
pg_sql "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name = 'support-test-lock-holder';" >/dev/null
wait "$lock_holder_pid" || true
if ! wait "$configure_pid"; then
  printf 'FAIL: concurrent support configuration did not recover\n' >&2
  sed -n '1,160p' "$integration/concurrent-config" >&2
  exit 1
fi
attempts_after=$(wc -l < "$integration/invocations" | tr -d ' ')
[[ $((attempts_after - attempts_before)) == 2 ]]
concurrent_result=$(pg_sql "SELECT value FROM settings WHERE key = 'custom_menu_items';")
jq -e 'map(.id) == ["non-cooperating-menu", "xingqiao-support"]' <<< "$concurrent_result" >/dev/null
jq -e '.[0] == {"id":"non-cooperating-menu","source":"external"}' <<< "$concurrent_result" >/dev/null
[[ $(pg_sql "SELECT count(*) FROM settings WHERE key = 'custom_menu_items';") == 1 ]]

unrelated_after=$(pg_sql "SELECT row_to_json(s)::text FROM (SELECT key, value, updated_at FROM settings WHERE key = 'unrelated') AS s;")
[[ "$unrelated_before" == "$unrelated_after" ]]
cmp "$ROOT/config/sub2api/support.md" "$integration/data/pages/support.md"
cmp "$ROOT/homepage/public/support/qq-group-1080152144.png" "$integration/data/pages/support/qq-group-1080152144.png"

printf 'PASS: PostgreSQL 18 support transaction integration\n'
