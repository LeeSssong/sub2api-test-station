#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
EXECUTOR="$ROOT/ops/deploy-sub2api-blue-green-host.sh"
FIXTURE=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-blue-green-host.XXXXXX")
FIXTURE=$(cd "$FIXTURE" && pwd -P)
trap 'rm -rf -- "$FIXTURE"' EXIT

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

[[ -f "$EXECUTOR" ]] || fail "host executor does not exist: $EXECUTOR"

REAL_JQ=$(command -v jq)
IMAGE="example.invalid/sub2api@sha256:$(printf 'a%.0s' {1..64})"
SOURCE_COMMIT=$(printf 'b%.0s' {1..40})
SOURCE_TREE=$(printf 'c%.0s' {1..40})
TESTED_TREE=$SOURCE_TREE
MIGRATIONS_HASH=$(printf 'd%.0s' {1..64})
PREVIOUS_IMAGE="example.invalid/sub2api@sha256:$(printf 'e%.0s' {1..64})"

setup_case() {
  CASE_DIR="$FIXTURE/$1"
  rm -rf -- "$CASE_DIR"
  mkdir -p "$CASE_DIR/bin" "$CASE_DIR/deploy" "$CASE_DIR/records"
  EVENT_LOG="$CASE_DIR/events.log"
  : >"$EVENT_LOG"
  printf 'secret=value\n' >"$CASE_DIR/secret.env"
  printf 'admin-test-key\n' >"$CASE_DIR/admin.key"
  printf 'gateway-test-key\n' >"$CASE_DIR/gateway.key"
  printf 'services: {}\n' >"$CASE_DIR/compose.yaml"
  cat >"$CASE_DIR/release.env" <<EOF
UNRELATED_SETTING=preserved
SUB2API_BLUE_IMAGE=$PREVIOUS_IMAGE
SUB2API_GREEN_IMAGE=$PREVIOUS_IMAGE
SUB2API_WORKER_IMAGE=$PREVIOUS_IMAGE
SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080
SUB2API_ACTIVE_SLOT=blue
SUB2API_PREVIOUS_SLOT=green
EOF
  "$REAL_JQ" -n \
    --arg image "$PREVIOUS_IMAGE" \
    --arg source_commit "$(printf 'f%.0s' {1..40})" \
    --arg source_tree "$(printf '1%.0s' {1..40})" \
    --arg migrations_hash "$MIGRATIONS_HASH" '
      {
        schema_version: 1,
        active_slot: "blue",
        active_upstream: "sub2api-blue:8080",
        blue_image: $image,
        green_image: $image,
        worker_image: $image,
        source_commit: $source_commit,
        source_tree: $source_tree,
        migrations_hash: $migrations_hash,
        postgres_id: "postgres-id",
        redis_id: "redis-id",
        caddy_id: "caddy-id"
      }
    ' >"$CASE_DIR/state.json"
  chmod 0600 "$CASE_DIR/state.json" "$CASE_DIR/release.env" "$CASE_DIR/secret.env" \
    "$CASE_DIR/admin.key" "$CASE_DIR/gateway.key"

  cat >"$CASE_DIR/bin/jq" <<EOF
#!/usr/bin/env bash
printf 'jq %s\n' "\$*" >>"\${FAKE_EVENT_LOG:?}"
exec "$REAL_JQ" "\$@"
EOF
  cat >"$CASE_DIR/bin/uname" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "${FAKE_UNAME:-Linux}"
EOF
  cat >"$CASE_DIR/bin/date" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *+%s*) printf '%s\n' "${FAKE_EPOCH:-1785513600}" ;;
  *) printf '%s\n' "${FAKE_DATE:-20260731T160000Z}" ;;
esac
EOF
  cat >"$CASE_DIR/bin/df" <<'EOF'
#!/usr/bin/env bash
printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n'
printf '/dev/fake 99999999 1 %s 1%% /\n' "${FAKE_DISK_KB:-4194304}"
EOF
  cat >"$CASE_DIR/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'curl %s\n' "$*" >>"${FAKE_EVENT_LOG:?}"
case "${FAKE_SCENARIO:-success}:$*" in
  public_failure:*example.invalid*) exit 22 ;;
esac
case "$*" in
  *'/health'*) printf '{"status":"ok"}\n' ;;
  *'/api/v1/admin/system/version'*) printf '{"data":{"version":"1.2.3"}}\n' ;;
  *'/v1/models'*) printf '{"data":[]}\n' ;;
  *) printf '{}\n' ;;
esac
EOF
  cat >"$CASE_DIR/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"${FAKE_EVENT_LOG:?}"
scenario=${FAKE_SCENARIO:-success}
case "$*" in
  'context show') printf '%s\n' "${FAKE_DOCKER_CONTEXT:-default}" ;;
  image\ inspect*)
    qualified=true
    source_commit=${EXPECTED_SOURCE_COMMIT:?}
    source_tree=${EXPECTED_SOURCE_TREE:?}
    tested_tree=${EXPECTED_TESTED_TREE:?}
    migrations=${EXPECTED_MIGRATIONS_HASH:?}
    [[ "$scenario" == label_mismatch ]] && source_tree=$(printf '9%.0s' {1..40})
    cat <<JSON
[{"RepoDigests":["${EXPECTED_IMAGE:?}"],"Config":{"Labels":{"com.xingqiao.sub2api.qualified":"$qualified","com.xingqiao.sub2api.source.commit":"$source_commit","com.xingqiao.sub2api.source.tree":"$source_tree","com.xingqiao.sub2api.tested.tree":"$tested_tree","com.xingqiao.sub2api.migrations.sha256":"$migrations"}}}]
JSON
    ;;
  *' config --format json')
    role=api
    [[ "$scenario" == candidate_role ]] && role=worker
    printf '{"services":{"sub2api-green":{"image":"%s","environment":{"SERVER_PROCESS_ROLE":"%s"}},"sub2api-blue":{"image":"%s","environment":{"SERVER_PROCESS_ROLE":"api"}},"sub2api-worker":{"image":"%s","environment":{"SERVER_PROCESS_ROLE":"worker"}}}}\n' \
      "${EXPECTED_IMAGE:?}" "$role" "${PREVIOUS_IMAGE_FOR_FAKE:?}" "${PREVIOUS_IMAGE_FOR_FAKE:?}"
    ;;
  *' ps -q postgres') printf 'postgres-id\n' ;;
  *' ps -q redis') printf 'redis-id\n' ;;
  *' ps -q caddy') printf 'caddy-id\n' ;;
  *' ps -q sub2api-blue') printf 'blue-id\n' ;;
  *' ps -q sub2api-green') printf 'green-id\n' ;;
  *' ps -q sub2api-worker') printf 'worker-id\n' ;;
  *'exec -T postgres '*'psql'*) printf '%s\n' "${FAKE_DB_HEADROOM:-30}" ;;
  *'pull sub2api-green') : ;;
  *'up --no-deps -d sub2api-green')
    [[ "$scenario" != candidate_up_failure ]] || exit 1
    ;;
  *'run --rm --network '*'/health'*)
    [[ "$scenario" != candidate_health_failure ]] || exit 1
    printf '{"status":"ok"}\n'
    ;;
  *'run --rm --network '*'/api/v1/admin/system/version'*) printf '{"data":{"version":"1.2.3"}}\n' ;;
  *'run --rm --network '*'/api/v1/settings/public'*) printf '{"data":{}}\n' ;;
  *'run --rm --network '*'/v1/models'*) printf '{"data":[]}\n' ;;
  *'exec -T -e SUB2API_ACTIVE_UPSTREAM='*' caddy caddy validate'*)
    [[ "$scenario" != caddy_validate_failure ]] || exit 1
    ;;
  *'exec -T -e SUB2API_ACTIVE_UPSTREAM='*' caddy caddy reload'*)
    if [[ "$scenario" == reload_failure && "$*" == *sub2api-green:8080* ]]; then exit 1; fi
    ;;
  *'up --no-deps -d --force-recreate sub2api-worker')
    if [[ "$scenario" == worker_update_failure && ! -e "${FAKE_EVENT_LOG}.worker-failed" ]]; then
      : >"${FAKE_EVENT_LOG}.worker-failed"
      exit 1
    fi
    ;;
  'inspect worker-id --format {{.State.Health.Status}}')
    [[ "$scenario" != worker_health_failure ]] || { printf 'unhealthy\n'; exit 0; }
    printf 'healthy\n'
    ;;
  *'logs --no-color --tail 200 sub2api-worker')
    [[ "$scenario" != worker_log_failure ]] || { printf 'panic: worker failed\n'; exit 0; }
    printf 'worker ready\n'
    ;;
  *) : ;;
esac
EOF
  chmod +x "$CASE_DIR/bin/"*
}

run_executor() {
  env \
    PATH="$CASE_DIR/bin:$PATH" \
    FAKE_EVENT_LOG="$EVENT_LOG" \
    RELEASE_EVENT_LOG="$EVENT_LOG" \
    EXPECTED_IMAGE="$IMAGE" \
    EXPECTED_SOURCE_COMMIT="$SOURCE_COMMIT" \
    EXPECTED_SOURCE_TREE="$SOURCE_TREE" \
    EXPECTED_TESTED_TREE="$TESTED_TREE" \
    EXPECTED_MIGRATIONS_HASH="$MIGRATIONS_HASH" \
    PREVIOUS_IMAGE_FOR_FAKE="$PREVIOUS_IMAGE" \
    DEPLOY_ROOT="$CASE_DIR/deploy" \
    BASE_COMPOSE="$CASE_DIR/compose.yaml" \
    SECRET_ENV="$CASE_DIR/secret.env" \
    RELEASE_ENV="$CASE_DIR/release.env" \
    RELEASE_STATE="$CASE_DIR/state.json" \
    RELEASE_RECORD_ROOT="$CASE_DIR/records" \
    BASE_URL="https://example.invalid" \
    ADMIN_API_KEY_FILE="$CASE_DIR/admin.key" \
    GATEWAY_API_KEY_FILE="$CASE_DIR/gateway.key" \
    MEMINFO_FILE="$CASE_DIR/meminfo" \
    COMPOSE_PROJECT_NAME=sub2api \
    "$@" bash "$EXECUTOR" \
      --mode production \
      --image "$IMAGE" \
      --source-commit "$SOURCE_COMMIT" \
      --source-tree "$SOURCE_TREE" \
      --tested-tree "$TESTED_TREE" \
      --migrations-hash "$MIGRATIONS_HASH"
}

write_meminfo() {
  printf 'MemAvailable: %s kB\n' "${1:-2097152}" >"$CASE_DIR/meminfo"
}

assert_no_mutation() {
  ! grep -Eq 'docker .*compose .* up |caddy caddy reload' "$EVENT_LOG" || fail "$1 mutated Docker/Caddy"
  grep -q '^SUB2API_ACTIVE_SLOT=blue$' "$CASE_DIR/release.env" || fail "$1 rewrote release env"
}

expect_failure() {
  local label=$1
  shift
  if "$@" >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail "$label unexpectedly succeeded"
  fi
}

test_validation_failures() {
  setup_case validation
  write_meminfo
  local saved_image=$IMAGE
  IMAGE=bad
  expect_failure malformed run_executor
  IMAGE=$saved_image
  assert_no_mutation malformed

  setup_case label
  write_meminfo
  expect_failure label_mismatch run_executor FAKE_SCENARIO=label_mismatch
  assert_no_mutation label_mismatch

  setup_case trees
  write_meminfo
  TESTED_TREE=$(printf '2%.0s' {1..40})
  expect_failure tree_mismatch run_executor
  TESTED_TREE=$SOURCE_TREE
  assert_no_mutation tree_mismatch

  setup_case nonlinux
  write_meminfo
  expect_failure non_linux run_executor FAKE_UNAME=Darwin
  assert_no_mutation non_linux

  setup_case context
  write_meminfo
  expect_failure context run_executor FAKE_DOCKER_CONTEXT=remote
  assert_no_mutation context

  setup_case symlink
  write_meminfo
  mv "$CASE_DIR/state.json" "$CASE_DIR/real-state.json"
  ln -s "$CASE_DIR/real-state.json" "$CASE_DIR/state.json"
  expect_failure symlink run_executor
  assert_no_mutation symlink

  setup_case duplicate
  write_meminfo
  sed 's/"active_slot": "blue"/"active_slot": "blue", "active_slot": "green"/' "$CASE_DIR/state.json" >"$CASE_DIR/duplicate.json"
  mv "$CASE_DIR/duplicate.json" "$CASE_DIR/state.json"
  chmod 0600 "$CASE_DIR/state.json"
  expect_failure duplicate run_executor
  assert_no_mutation duplicate

  setup_case invalid_key
  write_meminfo
  "$REAL_JQ" '.unexpected=true' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"
  chmod 0600 "$CASE_DIR/state.json"
  expect_failure invalid_key run_executor
  assert_no_mutation invalid_key

  setup_case stale_partial
  write_meminfo
  printf '{"schema_version":1,"started_epoch":1}\n' >"$CASE_DIR/records/stale.partial"
  chmod 0600 "$CASE_DIR/records/stale.partial"
  expect_failure stale_partial run_executor
  assert_no_mutation stale_partial

  setup_case lock
  write_meminfo
  mkdir "$CASE_DIR/records/.blue-green.lock"
  expect_failure concurrent_lock run_executor
  assert_no_mutation concurrent_lock

  setup_case malformed_lock
  write_meminfo
  mkdir "$CASE_DIR/records/.blue-green.lock"
  printf 'not-a-pid\n' >"$CASE_DIR/records/.blue-green.lock/owner.pid"
  chmod 0600 "$CASE_DIR/records/.blue-green.lock/owner.pid"
  expect_failure malformed_lock run_executor
  assert_no_mutation malformed_lock

  setup_case live_lock
  write_meminfo
  mkdir "$CASE_DIR/records/.blue-green.lock"
  printf '%s\n' "$$" >"$CASE_DIR/records/.blue-green.lock/owner.pid"
  chmod 0600 "$CASE_DIR/records/.blue-green.lock/owner.pid"
  expect_failure live_lock run_executor
  assert_no_mutation live_lock
}

test_downtime_gates() {
  local scenario reason
  for scenario in migration legacy disk memory db active_pair identity candidate_role; do
    setup_case "gate-$scenario"
    write_meminfo
    reason=$scenario
    case "$scenario" in
      migration)
        MIGRATIONS_HASH=$(printf '7%.0s' {1..64})
        ;;
      legacy) rm -f "$CASE_DIR/state.json" ;;
      disk) export FAKE_DISK_KB=1024 ;;
      memory) write_meminfo 1024 ;;
      db) export FAKE_DB_HEADROOM=1 ;;
      active_pair)
        "$REAL_JQ" '.active_upstream="sub2api-green:8080"' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
        mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"; chmod 0600 "$CASE_DIR/state.json"
        ;;
      identity)
        "$REAL_JQ" '.postgres_id="different"' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
        mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"; chmod 0600 "$CASE_DIR/state.json"
        ;;
    esac
    if [[ "$scenario" == candidate_role ]]; then
      expect_failure "$scenario" run_executor FAKE_SCENARIO=candidate_role
    else
      expect_failure "$scenario" run_executor
    fi
    grep -q '"downtime_required"' "$CASE_DIR/stdout" || fail "$scenario did not print a JSON gate: $(cat "$CASE_DIR/stderr")"
    grep -q 'true' "$CASE_DIR/stdout" || fail "$scenario gate was not true"
    assert_no_mutation "$scenario gate"
    MIGRATIONS_HASH=$(printf 'd%.0s' {1..64})
    unset FAKE_DISK_KB FAKE_DB_HEADROOM || true
  done
}

test_success_order_and_atomic_records() {
  setup_case success
  write_meminfo
  run_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail "success path failed: $(cat "$CASE_DIR/stderr")"
  local previous=0 line current pattern
  for pattern in \
    'docker image inspect' \
    'docker compose .* ps -q postgres' \
    'docker compose .* pull sub2api-green' \
    'docker compose .* up --no-deps -d sub2api-green' \
    'docker run --rm --network sub2api_default .*health' \
    'caddy caddy validate' \
    'caddy caddy reload' \
    'curl .*https://example.invalid/health' \
    'persist release-env' \
    'persist release-state' \
    'docker compose .* up --no-deps -d --force-recreate sub2api-worker' \
    'docker inspect worker-id' \
    'docker compose .* ps -q postgres' \
    'persist success-record'; do
    line=$(awk -v pattern="$pattern" -v after="$previous" 'NR > after && $0 ~ pattern { print NR; exit }' "$EVENT_LOG")
    [[ -n "$line" && "$line" -gt "$previous" ]] || fail "successful order missing/out of order: $pattern"
    previous=$line
  done
  ! grep -Eq 'compose .* down( |$)|volume (rm|prune)|compose .* (up|rm|stop) .*postgres|compose .* (up|rm|stop) .*redis|compose .* (up|rm|stop) .*caddy|compose .*stop sub2api-blue|database restore' "$EVENT_LOG" \
    || fail 'success path used a prohibited destructive operation'
  grep -q '^UNRELATED_SETTING=preserved$' "$CASE_DIR/release.env" || fail 'release env lost unrelated settings'
  grep -q '^SUB2API_ACTIVE_SLOT=green$' "$CASE_DIR/release.env" || fail 'release env did not persist green'
  "$REAL_JQ" -e '.active_slot == "green" and .active_upstream == "sub2api-green:8080" and .worker_image == $image' \
    --arg image "$IMAGE" "$CASE_DIR/state.json" >/dev/null || fail 'state was not promoted atomically'
  [[ "$(stat -f '%Lp' "$CASE_DIR/state.json" 2>/dev/null || stat -c '%a' "$CASE_DIR/state.json")" == 600 ]] || fail 'state mode is not 0600'
  record=$(find "$CASE_DIR/records" -maxdepth 1 -type f -name '*.json' -print -quit)
  [[ -n "$record" ]] || fail 'success record missing'
  [[ "$(stat -f '%Lp' "$record" 2>/dev/null || stat -c '%a' "$record")" == 600 ]] || fail 'record mode is not 0600'
  "$REAL_JQ" -e '.result == "succeeded" and .state == "promoted"' "$record" >/dev/null || fail 'success record invalid'
  [[ -z "$(find "$CASE_DIR/records" -maxdepth 1 -name '*.partial' -print -quit)" ]] || fail 'partial record remained after success'
}

test_failures_and_recovery() {
  local scenario
  for scenario in candidate_health_failure caddy_validate_failure reload_failure public_failure worker_update_failure; do
    setup_case "$scenario"
    write_meminfo
    expect_failure "$scenario" run_executor FAKE_SCENARIO="$scenario"
    if [[ "$scenario" == public_failure || "$scenario" == worker_update_failure ]]; then
      grep -q 'SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 caddy caddy reload' "$EVENT_LOG" \
        || fail "$scenario did not reload the previous upstream"
      if [[ "$scenario" == worker_update_failure ]]; then
        count=$(grep -c 'up --no-deps -d --force-recreate sub2api-worker' "$EVENT_LOG")
        [[ "$count" -ge 2 ]] || fail 'worker failure did not restore the prior worker digest'
      fi
      grep -q '^SUB2API_ACTIVE_SLOT=blue$' "$CASE_DIR/release.env" || fail "$scenario did not restore release env"
    fi
    record=$(find "$CASE_DIR/records" -maxdepth 1 -type f -name '*.json' -print -quit)
    [[ -n "$record" ]] || fail "$scenario failure record missing"
    "$REAL_JQ" -e '.result == "failed"' "$record" >/dev/null || fail "$scenario failure record invalid"
  done

  setup_case restart_recovery
  write_meminfo
  cat >"$CASE_DIR/records/restart.partial" <<EOF
{"schema_version":1,"attempt_id":"restart","mode":"production","started_epoch":1785513590,"phase":"worker_update","cutover_applied":true,"worker_updated":true,"previous":{"active_slot":"blue","active_upstream":"sub2api-blue:8080","blue_image":"$PREVIOUS_IMAGE","green_image":"$PREVIOUS_IMAGE","worker_image":"$PREVIOUS_IMAGE","source_commit":"$(printf 'f%.0s' {1..40})","source_tree":"$(printf '1%.0s' {1..40})","migrations_hash":"$MIGRATIONS_HASH","postgres_id":"postgres-id","redis_id":"redis-id","caddy_id":"caddy-id"},"candidate":{"slot":"green","upstream":"sub2api-green:8080","image":"$IMAGE"}}
EOF
  chmod 0600 "$CASE_DIR/records/restart.partial"
  mkdir "$CASE_DIR/records/.blue-green.lock"
  printf '2147483647\n' >"$CASE_DIR/records/.blue-green.lock/owner.pid"
  chmod 0600 "$CASE_DIR/records/.blue-green.lock/owner.pid"
  if run_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'restart recovery should finish the interrupted attempt as failed'
  fi
  grep -q 'SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 caddy caddy reload' "$EVENT_LOG" || fail 'restart did not restore previous upstream'
  grep -q 'up --no-deps -d --force-recreate sub2api-worker' "$EVENT_LOG" || fail 'restart did not restore previous worker'
  [[ ! -e "$CASE_DIR/records/restart.partial" ]] || fail 'recovered partial remains'
  [[ ! -e "$CASE_DIR/records/.blue-green.lock" ]] || fail 'recovered stale lock remains'
}

test_validation_failures
printf 'PASS: fail-closed validation harness\n'
test_downtime_gates
printf 'PASS: downtime gates precede mutation\n'
test_success_order_and_atomic_records
printf 'PASS: successful blue-green command order\n'
test_failures_and_recovery
printf 'PASS: rollback and interruption recovery\n'
