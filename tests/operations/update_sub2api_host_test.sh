#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT="$ROOT/ops/update-sub2api-host.sh"
IMAGE='weishaw/sub2api:0.1.164@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
OLD_IMAGE='xingqiao-sub2api:rollback-20260724-contact-v1'
OLD_ID='sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
NEW_ID='sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc'

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

new_fixture() {
  local temporary
  temporary=$(mktemp -d)
  FIXTURE_ROOT=$(cd "$temporary" && pwd -P)
  FIXTURE_BIN="$FIXTURE_ROOT/bin"
  FIXTURE_DEPLOY="$FIXTURE_ROOT/deploy"
  FIXTURE_RECORDS="$FIXTURE_ROOT/release-records/host-updater"
  FIXTURE_LOG="$FIXTURE_ROOT/operations.log"
  FIXTURE_TRACE="$FIXTURE_ROOT/trace.log"
  FIXTURE_STATE="$FIXTURE_ROOT/state"
  REAL_JQ_PATH=$(command -v jq)
  mkdir -p "$FIXTURE_BIN" "$FIXTURE_DEPLOY" "$FIXTURE_RECORDS" "$FIXTURE_DEPLOY/app-data"
  printf 'initial\n' >"$FIXTURE_STATE"
  : >"$FIXTURE_LOG"
  : >"$FIXTURE_TRACE"
  : >"$FIXTURE_DEPLOY/.env"
  : >"$FIXTURE_DEPLOY/compose.yaml"
  printf 'admin-secret\n' >"$FIXTURE_DEPLOY/admin.key"
  printf 'gateway-secret\n' >"$FIXTURE_DEPLOY/gateway.key"
  printf 'services:\n  sub2api:\n    image: %s\n' "$OLD_IMAGE" >"$FIXTURE_DEPLOY/compose.yaml"
  printf 'fixture-data\n' >"$FIXTURE_DEPLOY/app-data/example.txt"

  cat >"$FIXTURE_BIN/uname" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "${FAKE_UNAME:-Linux}"
SH
  cat >"$FIXTURE_BIN/date" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "${FAKE_DATE:-20260725T000000Z}"
SH
  cat >"$FIXTURE_BIN/sleep" <<'SH'
#!/usr/bin/env bash
exit 0
SH
cat >"$FIXTURE_BIN/df" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' 'Filesystem 1024-blocks Used Available Capacity Mounted on'
printf '%s\n' "fixture 8388608 1024 ${FAKE_AVAILABLE_KB:-8387584} 1% /fixture"
SH
  cat >"$FIXTURE_BIN/sha256sum" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'checksum\n' >>"${FAKE_LOG:?}"
exec /usr/bin/shasum -a 256 "$@"
SH
  cat >"$FIXTURE_BIN/tar" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'tar\n' >>"${FAKE_LOG:?}"
exec /usr/bin/tar "$@"
SH
  cat >"$FIXTURE_BIN/jq" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'jq\n' >>"${FAKE_LOG:?}"
  exec "${REAL_JQ_PATH:?}" "$@"
SH
  cat >"$FIXTURE_BIN/curl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'curl %s\n' "$*" >>"${FAKE_LOG:?}"
[[ "$*" != *'127.0.0.1:8080'* ]] || exit 22
case "$*" in
  */api/v1/admin/system/version*|*/api/v1/admin/system/update*|*/api/v1/admin/system/rollback*)
    found_header=false
    for argument in "$@"; do
      if [[ "$argument" == @* ]] && grep -Fqx 'X-API-Key: admin-secret' "${argument#@}"; then
        found_header=true
      fi
    done
    [[ "$found_header" == true ]] || exit 22
    [[ "$*" != *'admin-secret'* ]] || exit 22
    ;;
  */v1/models*)
    found_header=false
    for argument in "$@"; do
      if [[ "$argument" == @* ]] && grep -Fqx 'Authorization: Bearer gateway-secret' "${argument#@}"; then
        found_header=true
      fi
    done
    [[ "$found_header" == true ]] || exit 22
    [[ "$*" != *'gateway-secret'* ]] || exit 22
    ;;
esac
if [[ "$*" == *'-w %{http_code}'* || "$*" == *"-w '%{http_code}'"* ]]; then
  printf '409\n'
  exit 0
fi
case "$*" in
  */health*) printf '{"status":"ok"}\n' ;;
  */api/v1/admin/system/version*) printf '{"data":{"version":"0.1.164"}}\n' ;;
  */api/v1/settings/public*) printf '{"data":{"custom_menu_items":[{"id":"xingqiao-support","url":"md:support"}]}}\n' ;;
  */api/v1/admin/system/update*) printf 'guard\n' ;;
  */v1/models*) printf '{"data":[]}\n' ;;
  *) printf '{}\n' ;;
esac
SH
  cat >"$FIXTURE_BIN/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

log=${FAKE_LOG:?}
state=${FAKE_STATE:?}
deploy=${FAKE_DEPLOY:?}
project=${FAKE_PROJECT:?}
old_image=${FAKE_OLD_IMAGE:?}
requested=${FAKE_REQUESTED_IMAGE:?}
old_id=${FAKE_OLD_ID:?}
new_id=${FAKE_NEW_ID:?}

die() {
  printf 'forbidden docker command: %s\n' "$*" >>"$log"
  exit 64
}

if [[ "${1:-}" == context && "${2:-}" == show ]]; then
  printf '%s\n' "${FAKE_DOCKER_CONTEXT:-default}"
  exit 0
fi

if [[ "${1:-}" == image && "${2:-}" == inspect ]]; then
  [[ "${3:-}" == "$requested" ]] || die "$@"
  printf '[{"Id":"%s","RepoDigests":["%s"]}]\n' "$new_id" "$requested"
  exit 0
fi

if [[ "${1:-}" == pull ]]; then
  printf 'pull\n' >>"$log"
  exit 0
fi

if [[ "${1:-}" == tag ]]; then
  printf 'tag\n' >>"$log"
  exit 0
fi

if [[ "${1:-}" == run ]]; then
  printf 'postgres-validator\n' >>"$log"
  exit 0
fi

if [[ "${1:-}" == inspect ]]; then
  if [[ "$(cat "$state")" == initial ]]; then
    expected='sub2api-id postgres-id redis-id caddy-id relay-ops-id'
    [[ "$*" == *"sub2api-id"*"postgres-id"*"redis-id"*"caddy-id"*"relay-ops-id"* ]] || die "$@"
    if [[ "${FAKE_INTERNAL_TEST_UNDEFINED:-false}" == true ]]; then
      [[ "$*" != *"internal-test-service-id"* ]] || die "$@"
      internal_json=''
    else
      [[ "$*" == *"internal-test-service-id"* ]] || die "$@"
      internal_json=',{"Id":"internal-test-service-id","Config":{"Labels":{"com.docker.compose.project":"'"$project"'"}},"Image":"sha256:d04","State":{"Health":{"Status":"healthy"}}}'
    fi
    app_name=sub2api_sub2api_data
    postgres_name=sub2api_postgres_data
    redis_name=sub2api_redis_data
    health=healthy
    caddy_state='"Health":{"Status":"healthy"}'
    [[ "${FAKE_BAD_VOLUME:-false}" == true ]] && app_name=unexpected-volume
    [[ "${FAKE_UNHEALTHY_DEPENDENCY:-false}" == true ]] && health=unhealthy
    [[ "${FAKE_CADDY_NO_HEALTHCHECK:-false}" == true ]] && caddy_state='"Status":"running","Running":true'
    cat <<JSON
[
  {"Id":"sub2api-id","Config":{"Image":"$old_image","Labels":{"com.docker.compose.project":"$project","com.docker.compose.project.working_dir":"$deploy","com.docker.compose.project.config_files":"$deploy/compose.yaml"}},"Image":"$old_id","State":{"Health":{"Status":"$health"}},"Mounts":[{"Type":"volume","Name":"$app_name","Source":"$app_name","Destination":"/app/data","RW":true}]},
  {"Id":"postgres-id","Config":{"Labels":{"com.docker.compose.project":"$project","com.docker.compose.project.working_dir":"$deploy","com.docker.compose.project.config_files":"$deploy/compose.yaml"}},"Image":"sha256:postgres","State":{"Health":{"Status":"$health"}},"Mounts":[{"Type":"volume","Name":"$postgres_name","Source":"$postgres_name","Destination":"/var/lib/postgresql/data","RW":true}]},
  {"Id":"redis-id","Config":{"Labels":{"com.docker.compose.project":"$project","com.docker.compose.project.working_dir":"$deploy","com.docker.compose.project.config_files":"$deploy/compose.yaml"}},"Image":"sha256:redis","State":{"Health":{"Status":"$health"}},"Mounts":[{"Type":"volume","Name":"$redis_name","Source":"$redis_name","Destination":"/data","RW":true}]},
  {"Id":"caddy-id","Config":{"Labels":{"com.docker.compose.project":"$project"}},"Image":"sha256:caddy","State":{$caddy_state}},
  {"Id":"relay-ops-id","Config":{"Labels":{"com.docker.compose.project":"$project"}},"Image":"sha256:relay","State":{"Health":{"Status":"$health"}}}$internal_json
]
JSON
  elif [[ "$(cat "$state")" == previous ]]; then
    cat <<JSON
[{"Id":"sub2api-id","Config":{"Image":"$old_image","Labels":{"com.docker.compose.project":"$project","com.docker.compose.project.working_dir":"$deploy","com.docker.compose.project.config_files":"$deploy/compose.yaml"}},"Image":"$old_id","State":{"Health":{"Status":"healthy"}},"Mounts":[{"Type":"volume","Name":"sub2api_sub2api_data","Source":"sub2api_sub2api_data","Destination":"/app/data","RW":true}]}]
JSON
  else
    cat <<JSON
[{"Id":"sub2api-id","Config":{"Image":"$requested","Labels":{"com.docker.compose.project":"$project","com.docker.compose.project.working_dir":"$deploy","com.docker.compose.project.config_files":"$deploy/compose.yaml"}},"Image":"$new_id","State":{"Health":{"Status":"healthy"}},"Mounts":[{"Type":"volume","Name":"sub2api_sub2api_data","Source":"sub2api_sub2api_data","Destination":"/app/data","RW":true}]}]
JSON
  fi
  printf 'inspect\n' >>"$log"
  exit 0
fi

[[ "${1:-}" == compose ]] || die "$@"
shift
[[ "${1:-}" == --project-name && "${2:-}" == "$project" ]] || die "$@"
shift 2
[[ "${1:-}" == --project-directory && "${2:-}" == "$deploy" ]] || die "$@"
shift 2
[[ "${1:-}" == --env-file && "${2:-}" == "$deploy/.env" ]] || die "$@"
shift 2
[[ "${1:-}" == -f && "${2:-}" == "$deploy/compose.yaml" ]] || die "$@"
shift 2

case "${1:-}" in
  ps)
    [[ "${2:-}" == -q ]] || die "$@"
    case "${3:-}" in
      sub2api) printf 'ps sub2api=sub2api-id\n' >>"$log"; printf 'sub2api-id\n' ;;
      postgres) printf 'ps postgres=postgres-id\n' >>"$log"; printf 'postgres-id\n' ;;
      redis) printf 'ps redis=redis-id\n' >>"$log"; printf 'redis-id\n' ;;
      caddy) printf 'ps caddy=caddy-id\n' >>"$log"; printf 'caddy-id\n' ;;
      relay-ops) printf 'ps relay-ops=relay-ops-id\n' >>"$log"; printf 'relay-ops-id\n' ;;
      internal-test-service)
        if [[ "${FAKE_INTERNAL_TEST_UNDEFINED:-false}" == true ]]; then
          printf 'no such service: internal-test-service\n' >&2
          exit 1
        fi
        printf 'ps internal-test-service=internal-test-service-id\n' >>"$log"
        printf 'internal-test-service-id\n'
        ;;
      *) die "$@" ;;
    esac
    ;;
  config)
    if [[ "${2:-}" == --services ]]; then
      printf '%s\n' sub2api postgres redis caddy relay-ops
      [[ "${FAKE_INTERNAL_TEST_UNDEFINED:-false}" == true ]] || printf '%s\n' internal-test-service
      exit 0
    fi
    printf 'compose-validate\n' >>"$log"
    image=$(awk '/^[[:space:]]+image:/ {print $2; exit}' "$deploy/compose.yaml")
    app_name=sub2api_sub2api_data
    [[ "${FAKE_BAD_VOLUME:-false}" == true ]] && app_name=unexpected-volume
    printf '{"services":{"sub2api":{"image":"%s","volumes":[{"type":"volume","source":"%s","target":"/app/data"}]},"postgres":{"volumes":[{"type":"volume","source":"sub2api_postgres_data","target":"/var/lib/postgresql/data"}]},"redis":{"volumes":[{"type":"volume","source":"sub2api_redis_data","target":"/data"}]}}}\n' "$image" "$app_name"
    ;;
  pull)
    printf 'pull\n' >>"$log"
    ;;
  exec)
    if [[ "$*" == *pg_dump* ]]; then
      printf 'backup-db\n' >>"$log"
      printf 'PGDUMP\n'
    elif [[ "$*" == *'postgres'* ]]; then
      printf 'backup-counts\n' >>"$log"
      printf '{"users":1,"accounts":1,"groups":1,"api_keys":1,"settings":1,"usage_logs":1}\n'
    elif [[ "$*" == *'/app/data/pages/support/qq-group-1080152144.png'* ]]; then
      printf '35b84b14ab472e117fa413ed5f91357becd01199eeaf3fed469a2d9d3d987c16  /app/data/pages/support/qq-group-1080152144.png\n'
    elif [[ "$*" == *'tar -C /app/data'* ]]; then
      tar -C "$deploy/app-data" -czf - .
    else
      die "$@"
    fi
    ;;
  up)
    [[ "${2:-}" == -d && "${3:-}" == --no-deps && "${4:-}" == --force-recreate && "${5:-}" == sub2api ]] || die "$@"
    printf 'recreate-sub2api\n' >>"$log"
    if [[ "${SUB2API_IMAGE:-}" == "$old_image" ]]; then
      printf 'previous\n' >"$state"
    else
      printf 'requested\n' >"$state"
    fi
    ;;
  logs)
    printf '%s\n' 'application started'
    ;;
  down|restart|stop|start|rm|volume)
    die "$@"
    ;;
  *) die "$@" ;;
esac
SH
  chmod 0755 "$FIXTURE_BIN"/*
}

cleanup_fixture() {
  rm -rf -- "$FIXTURE_ROOT"
}

run_update() {
  (cd "${RUN_UPDATE_CWD:-$FIXTURE_DEPLOY}" && env PATH="$FIXTURE_BIN:$PATH" \
    FAKE_DEPLOY="$FIXTURE_DEPLOY" FAKE_PROJECT=sub2api \
    FAKE_STATE="$FIXTURE_STATE" FAKE_LOG="$FIXTURE_LOG" \
    FAKE_REQUESTED_IMAGE="$IMAGE" FAKE_OLD_IMAGE="$OLD_IMAGE" \
    FAKE_OLD_ID="$OLD_ID" FAKE_NEW_ID="$NEW_ID" \
    REAL_JQ_PATH="$REAL_JQ_PATH" \
    SUB2API_PRODUCTION_ROOT="$FIXTURE_DEPLOY" \
    SUB2API_RELEASE_RECORD_ROOT="$FIXTURE_RECORDS" \
    SUB2API_ENV_FILE="$FIXTURE_DEPLOY/.env" \
    SUB2API_COMPOSE_FILE="$FIXTURE_DEPLOY/compose.yaml" \
    SUB2API_DATA_VOLUME=sub2api_sub2api_data \
    SUB2API_POSTGRES_VOLUME=sub2api_postgres_data \
    SUB2API_REDIS_VOLUME=sub2api_redis_data \
    SUB2API_BASE_URL="${SUB2API_BASE_URL_OVERRIDE-https://sub2api.example.test}" \
    ADMIN_API_KEY_FILE="$FIXTURE_DEPLOY/admin.key" \
    GATEWAY_API_KEY_FILE="$FIXTURE_DEPLOY/gateway.key" \
    RELEASE_EVENT_LOG="$FIXTURE_TRACE" \
    bash "$SCRIPT" --image "$IMAGE" --operation-id op-test-001 "$@")
}

assert_trace() {
  local expected=$1 actual
  actual=$(awk 'NR == 1 {value = $0; next} {value = value " -> " $0} END {print value}' "$FIXTURE_TRACE")
  [[ "$actual" == "$expected" ]] || fail "unexpected trace: $actual"
}

assert_no_docker_mutation() {
  ! rg -n 'pull|recreate-sub2api|backup-db|backup-counts|backup-app-data|postgres-validator' "$FIXTURE_LOG" \
    || fail 'Docker mutation or backup occurred before preflight rejection'
}

test_success_trace_and_identity() {
  new_fixture
  local output record
  output=$(run_update)
  [[ "$output" == 'result=promoted' ]] || fail "unexpected success output: $output"
  assert_trace 'inspect -> pull -> backup-db -> backup-counts -> backup-app-data -> checksum -> compose-validate -> recreate-sub2api -> health -> smoke -> promoted'
  rg -n 'sub2api-id|postgres-id|redis-id|caddy-id|relay-ops-id|internal-test-service-id' "$FIXTURE_LOG" >/dev/null \
    || fail 'container identity checks were not recorded'
  ! rg -n 'down|--force-recreate postgres|--force-recreate redis|pg_restore' "$FIXTURE_LOG" \
    || fail 'forbidden dependency or database restore command was used'
  record=$(find "$FIXTURE_RECORDS" -type f -name '*.json' -print -quit)
  [[ -n "$record" ]] || fail 'release record missing'
  [[ $(stat -f '%Lp' "$record" 2>/dev/null || stat -c '%a' "$record") == 600 ]] \
    || fail 'release record is not 0600'
  "$REAL_JQ_PATH" -e --arg image "$IMAGE" --arg operation op-test-001 \
    '.state == "promoted" and .operation_id == $operation and .requested.image == $image and .checks.storage_identity == true' \
    "$record" >/dev/null || fail 'release record is incomplete'
  cleanup_fixture
}

test_missing_optional_service_is_accepted() {
  new_fixture
  local output
  output=$(FAKE_INTERNAL_TEST_UNDEFINED=true run_update)
  [[ "$output" == 'result=promoted' ]] || fail "missing optional service blocked update: $output"
  cleanup_fixture
}

test_running_caddy_without_healthcheck_is_accepted() {
  new_fixture
  local output
  output=$(FAKE_CADDY_NO_HEALTHCHECK=true run_update)
  [[ "$output" == 'result=promoted' ]] || fail "running Caddy without healthcheck blocked update: $output"
  cleanup_fixture
}

test_preflight_rejects_host_and_context() {
  local variable value
  for variable in FAKE_UNAME FAKE_DOCKER_CONTEXT; do
    new_fixture
    if [[ "$variable" == FAKE_UNAME ]]; then value=Darwin; else value=colima; fi
    if [[ "$variable" == FAKE_UNAME ]] && FAKE_UNAME=Darwin run_update >/dev/null 2>&1; then
      fail "$variable preflight rejection was not enforced"
    elif [[ "$variable" == FAKE_DOCKER_CONTEXT ]] && FAKE_DOCKER_CONTEXT=colima run_update >/dev/null 2>&1; then
      fail "$variable preflight rejection was not enforced"
    fi
    assert_no_docker_mutation
    cleanup_fixture
  done
}

test_preflight_rejects_runtime_identity_and_space() {
  local scenario
  for scenario in SUB2API_COMPOSE_PROJECT FAKE_BAD_VOLUME FAKE_UNHEALTHY_DEPENDENCY FAKE_AVAILABLE_KB; do
    new_fixture
    case "$scenario" in
      SUB2API_COMPOSE_PROJECT)
        if SUB2API_COMPOSE_PROJECT=wrong-project run_update >/dev/null 2>&1; then
          fail 'wrong Compose project was accepted'
        fi
        ;;
      FAKE_BAD_VOLUME)
        if FAKE_BAD_VOLUME=true run_update >/dev/null 2>&1; then
          fail 'unexpected named volume was accepted'
        fi
        ;;
      FAKE_UNHEALTHY_DEPENDENCY)
        if FAKE_UNHEALTHY_DEPENDENCY=true run_update >/dev/null 2>&1; then
          fail 'unhealthy dependency was accepted'
        fi
        ;;
      FAKE_AVAILABLE_KB)
        if FAKE_AVAILABLE_KB=1024 run_update >/dev/null 2>&1; then
          fail 'insufficient disk space was accepted'
        fi
        ;;
    esac
    assert_no_docker_mutation
    cleanup_fixture
  done
}

test_preflight_rejects_wrong_directory() {
  new_fixture
  if RUN_UPDATE_CWD="$FIXTURE_ROOT" run_update >/dev/null 2>&1; then
    fail 'wrong production directory was accepted'
  fi
  assert_no_docker_mutation
  cleanup_fixture
}

test_smoke_url_cannot_bypass_caddy() {
  local base
  for base in '' http://127.0.0.1:8080 http://sub2api:8080; do
    new_fixture
    if SUB2API_BASE_URL_OVERRIDE="$base" run_update >/dev/null 2>&1; then
      fail "non-Caddy smoke URL was accepted: $base"
    fi
    assert_no_docker_mutation
    cleanup_fixture
  done
}

test_preflight_rejects_image_and_lock() {
  new_fixture
  if run_update --image wrong/repository:latest >/dev/null 2>&1; then
    fail 'wrong image reference was accepted'
  fi
  assert_no_docker_mutation
  cleanup_fixture

  new_fixture
  if run_update --image weishaw/sub2api:latest >/dev/null 2>&1; then
    fail 'mutable image reference was accepted'
  fi
  assert_no_docker_mutation
  cleanup_fixture

  new_fixture
  mkdir "$FIXTURE_RECORDS/.update.lock"
  if run_update >/dev/null 2>&1; then
    fail 'existing lock was accepted'
  fi
  assert_no_docker_mutation
  cleanup_fixture
}

test_rollback_result_and_no_database_restore() {
  new_fixture
  cat >"$FIXTURE_BIN/curl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'curl failed\n' >>"${FAKE_LOG:?}"
if [[ "$(cat "${FAKE_STATE:?}")" == requested ]]; then
  exit 22
fi
printf '{"status":"ok"}\n'
SH
  chmod 0755 "$FIXTURE_BIN/curl"
  local output
  output=$(ROLLBACK_COMPATIBLE=true run_update)
  [[ "$output" == 'result=rolled_back' ]] || fail "unexpected rollback output: $output"
  rg -n 'recreate-sub2api' "$FIXTURE_LOG" >/dev/null || fail 'rollback recreation missing'
  ! rg -n 'down|pg_restore' "$FIXTURE_LOG" >/dev/null || fail 'rollback restored database or tore down project'
  cleanup_fixture
}

if [[ ! -x "$SCRIPT" ]]; then
  fail 'executor is absent; RED is expected before implementation'
fi

test_success_trace_and_identity
test_running_caddy_without_healthcheck_is_accepted
test_missing_optional_service_is_accepted
test_preflight_rejects_host_and_context
test_preflight_rejects_wrong_directory
test_smoke_url_cannot_bypass_caddy
test_preflight_rejects_runtime_identity_and_space
test_preflight_rejects_image_and_lock
test_rollback_result_and_no_database_restore
printf 'PASS: host-controlled Sub2API update executor\n'
