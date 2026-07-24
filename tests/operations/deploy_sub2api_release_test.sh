#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT="$ROOT/ops/deploy-sub2api-release.sh"
REQUESTED_DIGEST='sha256:a94c25fb4c50c3bf21155142d745ff11a8d9199e4cf72d9a2424d75ccbfc1659'
REQUESTED_IMAGE="weishaw/sub2api:0.1.164@$REQUESTED_DIGEST"

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
  FIXTURE_RECORDS="$FIXTURE_ROOT/records"
  FIXTURE_BACKUPS="$FIXTURE_ROOT/backups"
  FIXTURE_LOG="$FIXTURE_ROOT/operations.log"
  FIXTURE_SPACE_LOG="$FIXTURE_ROOT/space.log"
  FIXTURE_CONTAINER_LOG="$FIXTURE_ROOT/container-resolution.log"
  mkdir -p "$FIXTURE_BIN" "$FIXTURE_DEPLOY/data" "$FIXTURE_DEPLOY/postgres" "$FIXTURE_DEPLOY/redis" \
    "$FIXTURE_RECORDS" "$FIXTURE_BACKUPS"
  : >"$FIXTURE_LOG"
  : >"$FIXTURE_SPACE_LOG"
  : >"$FIXTURE_CONTAINER_LOG"
  : >"$FIXTURE_DEPLOY/base.yml"
  : >"$FIXTURE_DEPLOY/overlay.yml"
  printf 'SUB2API_IMAGE=%s\n' "$REQUESTED_IMAGE" >"$FIXTURE_DEPLOY/release.env"
  : >"$FIXTURE_DEPLOY/secret.env"
  printf 'admin-test-key\n' >"$FIXTURE_DEPLOY/admin.key"
  printf 'gateway-test-key\n' >"$FIXTURE_DEPLOY/gateway.key"

  cat >"$FIXTURE_BIN/df" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "${2:?}" >>"${FAKE_SPACE_LOG:?}"
printf '%s\n' 'Filesystem 1024-blocks Used Available Capacity Mounted on'
printf '%s\n' 'fixture 8388608 1024 8387584 1% /fixture'
SH

  cat >"$FIXTURE_BIN/uname" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "${FAKE_UNAME:-Linux}"
SH

  cat >"$FIXTURE_BIN/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

log=${FAKE_LOG:?}
requested=${FAKE_REQUESTED_IMAGE:?}
state_file=${FAKE_STATE_FILE:?}
deploy_root=${FAKE_DEPLOY:?}
expected_project=${FAKE_EXPECTED_PROJECT:?}
previous='xingqiao-sub2api:v0.1.164-contact-v1'
requested_id='sha256:2222222222222222222222222222222222222222222222222222222222222222'
previous_id='sha256:1111111111111111111111111111111111111111111111111111111111111111'
case "$(cat "$state_file")" in
  initial) sub2api_id='compose-generated-sub2api-id-initial' ;;
  requested) sub2api_id='compose-generated-sub2api-id-requested' ;;
  previous) sub2api_id='compose-generated-sub2api-id-rollback' ;;
  *) die 'unknown fake container state' ;;
esac
postgres_id='compose-generated-postgres-id'
redis_id='compose-generated-redis-id'

die() {
  printf 'forbidden docker command: %s\n' "$*" >>"$log"
  exit 64
}

if [[ "$1" == context && "$2" == show ]]; then
  printf '%s\n' "${FAKE_DOCKER_CONTEXT:-default}"
  exit 0
fi

if [[ "$1" == inspect ]]; then
  if [[ $(cat "$state_file") != initial ]]; then
    [[ "$#" -eq 2 && "$2" == "$sub2api_id" ]] || die "$@"
    image=$(cat "$state_file")
    if [[ "$image" == previous ]]; then
      current_image=$previous
      current_id=$previous_id
      if [[ "${FAKE_ROLLBACK_HEALTH_FAILURE:-false}" == true ]]; then
        health=unhealthy
        printf 'rollback wait failed\n' >>"$log"
      else
        health=healthy
      fi
    else
      current_image=$requested
      current_id=$requested_id
      if [[ "${FAKE_REQUESTED_HEALTH_FAILURE:-false}" == true ]]; then
        health=unhealthy
        printf 'wait failed\n' >>"$log"
      else
        health=healthy
        printf 'wait\n' >>"$log"
      fi
    fi
    if [[ "${FAKE_PROMOTED_CONFIG_MISMATCH:-false}" == true && "$image" == requested ]]; then
      runtime_configs="$deploy_root/base.yml"
    else
      runtime_configs="$deploy_root/base.yml,$deploy_root/overlay.yml"
    fi
    cat <<JSON
[{"Name":"/$expected_project-sub2api-1","Config":{"Image":"$current_image","Labels":{"com.docker.compose.project":"$expected_project","com.docker.compose.project.working_dir":"$deploy_root","com.docker.compose.project.config_files":"$runtime_configs"}},"Image":"$current_id","State":{"Health":{"Status":"$health"}}}]
JSON
  else
    [[ "$#" -eq 4 && "$2" == "$sub2api_id" && "$3" == "$postgres_id" && "$4" == "$redis_id" ]] \
      || die "$@"
    printf 'inspect\n' >>"$log"
    cat <<JSON
[{"Name":"/$expected_project-sub2api-1","Config":{"Image":"$previous","Labels":{"com.docker.compose.project":"$expected_project","com.docker.compose.project.working_dir":"$deploy_root","com.docker.compose.project.config_files":"$deploy_root/base.yml"}},"Image":"$previous_id","State":{"Health":{"Status":"healthy"}}},{"Name":"/$expected_project-postgres-1","Config":{"Labels":{"com.docker.compose.project":"$expected_project","com.docker.compose.project.working_dir":"$deploy_root","com.docker.compose.project.config_files":"$deploy_root/base.yml"}},"State":{"Health":{"Status":"healthy"}}},{"Name":"/$expected_project-redis-1","Config":{"Labels":{"com.docker.compose.project":"$expected_project","com.docker.compose.project.working_dir":"$deploy_root","com.docker.compose.project.config_files":"$deploy_root/base.yml"}},"State":{"Health":{"Status":"healthy"}}}]
JSON
  fi
  exit 0
fi

if [[ "$1" == image && "$2" == inspect && "$#" -eq 3 && "$3" == "$requested" ]]; then
  printf '[{"Id":"%s","RepoDigests":["%s"]}]\n' "$requested_id" "$requested"
  exit 0
fi

[[ "$1" == compose ]] || die "$@"
expected_prefix=(compose --project-name "$expected_project" --project-directory "$deploy_root"
  --env-file "$deploy_root/secret.env" --env-file "$deploy_root/release.env"
  -f "$deploy_root/base.yml" -f "$deploy_root/overlay.yml")
(( $# >= ${#expected_prefix[@]} )) || die "$@"
for ((index = 0; index < ${#expected_prefix[@]}; index++)); do
  position=$((index + 1))
  [[ "${!position}" == "${expected_prefix[$index]}" ]] || die "$@"
done
shift "${#expected_prefix[@]}"

case "${1:-}" in
  config)
    [[ "$#" -eq 3 && "$2" == --format && "$3" == json ]] || die "$@"
    printf 'compose config\n' >>"$log"
    cat <<JSON
{"services":{"sub2api":{"volumes":[{"type":"bind","source":"$deploy_root/data","target":"/app/data"}]},"postgres":{"volumes":[{"type":"bind","source":"$deploy_root/postgres","target":"/var/lib/postgresql/data"}]},"redis":{"volumes":[{"type":"bind","source":"$deploy_root/redis","target":"/data"}]}}}
JSON
    ;;
  ps)
    [[ "$#" -eq 3 && "$2" == -q ]] || die "$@"
    case "$3" in
      sub2api) resolved_id=$sub2api_id ;;
      postgres) resolved_id=$postgres_id ;;
      redis) resolved_id=$redis_id ;;
      *) die "$@" ;;
    esac
    printf '%s=%s\n' "$3" "$resolved_id" >>"${FAKE_CONTAINER_LOG:?}"
    [[ "${FAKE_EMPTY_CONTAINER_SERVICE:-}" != "$3" ]] || exit 0
    if [[ "${FAKE_DUPLICATE_CONTAINER_SERVICE:-}" == "$3" ]]; then
      printf '%s\n' duplicate-id
    fi
    printf '%s\n' "$resolved_id"
    ;;
  pull)
    [[ "$#" -eq 2 && "$2" == sub2api && "${SUB2API_IMAGE:-}" == "$requested" ]] || die "$@"
    printf 'pull\n' >>"$log"
    ;;
  up)
    [[ "$#" -eq 5 && "$2" == -d && "$3" == --no-deps && "$4" == --force-recreate && "$5" == sub2api ]] \
      || die "$@"
    if [[ "${SUB2API_IMAGE:-}" == "$previous" ]]; then
      printf 'compose up sub2api(previous image)\n' >>"$log"
      printf 'previous\n' >"$state_file"
      [[ "${FAKE_ROLLBACK_UP_FAILURE:-false}" != true ]] || exit 1
    elif [[ "${SUB2API_IMAGE:-}" == "$requested" ]]; then
      if [[ "${FAKE_REQUESTED_UP_FAILURE:-false}" == true ]]; then
        printf 'compose up sub2api failed\n' >>"$log"
        printf 'requested\n' >"$state_file"
        exit 1
      fi
      printf 'compose up sub2api\n' >>"$log"
      printf 'requested\n' >"$state_file"
    else
      die "$@"
    fi
    ;;
  down|exec|run|start|stop|restart|postgres|redis) die "$@" ;;
  *) die "$@" ;;
esac
SH

  cat >"$FIXTURE_BIN/baseline" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
[[ "${EXPECTED_PROJECT:-}" == "$FAKE_EXPECTED_PROJECT" ]]
[[ "${EXPECTED_IMAGE_ID:-}" == sha256:1111111111111111111111111111111111111111111111111111111111111111 ]]
[[ "${EXPECTED_SUB2API_DATA:-}" == "$FAKE_DEPLOY/data" ]]
[[ "${EXPECTED_POSTGRES_DATA:-}" == "$FAKE_DEPLOY/postgres" ]]
[[ "${EXPECTED_REDIS_DATA:-}" == "$FAKE_DEPLOY/redis" ]]
[[ "${SUB2API_CONTAINER:-}" == compose-generated-sub2api-id-initial ]]
[[ "${POSTGRES_CONTAINER:-}" == compose-generated-postgres-id ]]
[[ "${REDIS_CONTAINER:-}" == compose-generated-redis-id ]]
printf 'baseline\n' >>"${FAKE_LOG:?}"
[[ "${FAKE_BASELINE_FAILURE:-false}" != true ]] || exit 1
if [[ "${FAKE_BASELINE_WORKING_DIR_MISMATCH:-false}" == true ]]; then
  working_dir=/wrong/deploy
else
  working_dir=$FAKE_DEPLOY
fi
if [[ "${FAKE_BASELINE_CONFIG_MISMATCH:-false}" == true ]]; then
  config_files=/wrong/compose.yml
else
  config_files=$FAKE_DEPLOY/base.yml
fi
jq -n --arg image xingqiao-sub2api:v0.1.164-contact-v1 \
  --arg image_id sha256:1111111111111111111111111111111111111111111111111111111111111111 \
  --arg working_dir "$working_dir" --arg config_files "$config_files" \
  '{image:$image,image_id:$image_id,working_dir:$working_dir,config_files:$config_files}'
SH

  cat >"$FIXTURE_BIN/backup" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
[[ "${DEPLOY_ROOT:-}" == "$FAKE_DEPLOY" ]]
[[ "${SECRET_ENV:-}" == "$FAKE_DEPLOY/secret.env" ]]
[[ "${RELEASE_ENV:-}" == "$FAKE_DEPLOY/release.env" ]]
[[ "${BASE_COMPOSE:-}" == "$FAKE_DEPLOY/base.yml" ]]
[[ "${IMAGE_OVERLAY:-}" == "$FAKE_DEPLOY/overlay.yml" ]]
[[ "${COMPOSE_PROJECT_NAME:-}" == "$FAKE_EXPECTED_PROJECT" ]]
printf 'backup\n' >>"${FAKE_LOG:?}"
backup=${SUB2API_BACKUP_ROOT:?}/20260724T000000Z
mkdir -p "$backup"
printf 'backup' >"$backup/sub2api.dump"
printf '{"users":1,"accounts":1,"groups":1,"api_keys":1,"settings":1,"usage_logs":1}\n' >"$backup/record-counts.json"
(cd "$backup" && shasum -a 256 sub2api.dump record-counts.json >SHA256SUMS)
if [[ "${FAKE_BAD_BACKUP_CHECKSUM:-false}" == true ]]; then
  printf 'changed' >>"$backup/sub2api.dump"
fi
printf 'release_backup_set=%s\n' "$backup"
SH

  cat >"$FIXTURE_BIN/smoke" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
[[ "${DEPLOY_ROOT:-}" == "$FAKE_DEPLOY" ]]
[[ "${SECRET_ENV:-}" == "$FAKE_DEPLOY/secret.env" ]]
[[ "${RELEASE_ENV:-}" == "$FAKE_DEPLOY/release.env" ]]
[[ "${BASE_COMPOSE:-}" == "$FAKE_DEPLOY/base.yml" ]]
[[ "${IMAGE_OVERLAY:-}" == "$FAKE_DEPLOY/overlay.yml" ]]
[[ "${COMPOSE_PROJECT_NAME:-}" == "$FAKE_EXPECTED_PROJECT" ]]
[[ "${EXPECTED_RECORD_COUNTS_FILE:-}" == "$SUB2API_BACKUP_ROOT/20260724T000000Z/record-counts.json" ]]
if [[ "${FAKE_SMOKE_PAUSE:-false}" == true && "${SUB2API_IMAGE:-}" != xingqiao-sub2api:* ]]; then
  printf 'unexpected interruption\n' >>"${FAKE_LOG:?}"
  sleep 1
  exit 143
fi
if [[ "${SUB2API_IMAGE:-}" == xingqiao-sub2api:* ]]; then
  printf 'rollback smoke\n' >>"${FAKE_LOG:?}"
  [[ "${EXPECTED_VERSION:-}" == 0.1.164 ]] || exit 1
  [[ "${FAKE_ROLLBACK_SMOKE_FAILURE:-false}" != true ]] || exit 1
else
  if [[ "${FAKE_SMOKE_FAILURE:-false}" == true ]]; then
    printf 'smoke failed\n' >>"${FAKE_LOG:?}"
    exit 1
  fi
  printf 'smoke\n' >>"${FAKE_LOG:?}"
fi
SH
  chmod 0755 "$FIXTURE_BIN/df" "$FIXTURE_BIN/uname" "$FIXTURE_BIN/docker" "$FIXTURE_BIN/baseline" "$FIXTURE_BIN/backup" "$FIXTURE_BIN/smoke"
  printf 'initial\n' >"$FIXTURE_ROOT/state"
}

cleanup_fixture() {
  rm -rf -- "$FIXTURE_ROOT"
}

run_deploy() {
  local expected_project=sub2api-official-rehearsal argument
  local -a deploy_command
  for argument in "$@"; do
    [[ "$argument" != production ]] || expected_project=sub2api
  done
  deploy_command=(env
    "PATH=$FIXTURE_BIN:$PATH"
    "FAKE_LOG=$FIXTURE_LOG"
    "FAKE_SPACE_LOG=$FIXTURE_SPACE_LOG"
    "FAKE_CONTAINER_LOG=$FIXTURE_CONTAINER_LOG"
    "RELEASE_EVENT_LOG=$FIXTURE_LOG"
    "FAKE_DEPLOY=$FIXTURE_DEPLOY"
    "FAKE_STATE_FILE=$FIXTURE_ROOT/state"
    "FAKE_REQUESTED_IMAGE=$REQUESTED_IMAGE"
    "FAKE_EXPECTED_PROJECT=$expected_project"
    "DEPLOY_ROOT=$FIXTURE_DEPLOY"
    "BASE_COMPOSE=$FIXTURE_DEPLOY/base.yml"
    "IMAGE_OVERLAY=$FIXTURE_DEPLOY/overlay.yml"
    "RELEASE_ENV=$FIXTURE_DEPLOY/release.env"
    "SECRET_ENV=$FIXTURE_DEPLOY/secret.env"
    "SUB2API_DATA_DIR=$FIXTURE_DEPLOY/data"
    "POSTGRES_DATA_DIR=$FIXTURE_DEPLOY/postgres"
    "REDIS_DATA_DIR=$FIXTURE_DEPLOY/redis"
    "ADMIN_API_KEY_FILE=$FIXTURE_DEPLOY/admin.key"
    "GATEWAY_API_KEY_FILE=$FIXTURE_DEPLOY/gateway.key"
    "BACKUP_ROOT=$FIXTURE_BACKUPS"
    "RELEASE_RECORD_ROOT=$FIXTURE_RECORDS"
    'BASE_URL=https://sub2api.example.test'
    "HEALTH_TIMEOUT_SECONDS=${HEALTH_TIMEOUT_SECONDS:-0}"
    "BASELINE_SCRIPT=$FIXTURE_BIN/baseline"
    "BACKUP_SCRIPT=$FIXTURE_BIN/backup"
    "SMOKE_SCRIPT=$FIXTURE_BIN/smoke"
    bash "$SCRIPT" "$@")
  if [[ "${FAKE_EXEC_DEPLOY:-false}" == true ]]; then
    exec "${deploy_command[@]}"
  fi
  "${deploy_command[@]}"
}

assert_log() {
  local expected=$1 actual
  actual=$(awk 'NR == 1 {value = $0; next} {value = value " -> " $0} END {print value}' "$FIXTURE_LOG")
  [[ "$actual" == "$expected" ]] || fail "unexpected operation order: $actual"
}

assert_one_record() {
  local expected_state=$1 count record
  count=$(find "$FIXTURE_RECORDS" -name '*.json' -type f | wc -l | tr -d ' ')
  [[ "$count" == 1 ]] || fail "expected one terminal record, got $count"
  record=$(find "$FIXTURE_RECORDS" -name '*.json' -type f)
  jq -e --arg state "$expected_state" '.state == $state' "$record" >/dev/null \
    || fail "expected terminal state $expected_state"
}

make_rehearsal_record() {
  local path=$1
  jq -n --arg image "$REQUESTED_IMAGE" --arg digest "$REQUESTED_DIGEST" '{
    schema_version: 1,
    mode: "rehearsal",
    previous: {image: "custom:previous", image_id: "sha256:1111111111111111111111111111111111111111111111111111111111111111"},
    requested: {image: $image, digest: $digest, version: "0.1.164"},
    backup: {path: "/verified/backup", sha256_verified: true},
    checks: {storage_identity: true, health: true, version: true, records: true,
      support: true, guard: true, gateway: true},
    state: "promoted"
  }' >"$path"
  chmod 0600 "$path"
}

test_rehearsal_promotes_with_fixed_context() {
  new_fixture
  run_deploy --mode rehearsal >/dev/null
  assert_log 'inspect -> compose config -> baseline -> backup -> pull -> compose up sub2api -> wait -> smoke -> promoted'
  [[ $(cat "$FIXTURE_CONTAINER_LOG") == $'sub2api=compose-generated-sub2api-id-initial\npostgres=compose-generated-postgres-id\nredis=compose-generated-redis-id\nsub2api=compose-generated-sub2api-id-requested' ]] \
    || fail 'containers were not re-resolved uniquely through Compose ps after recreation'
  assert_one_record promoted
  record=$(find "$FIXTURE_RECORDS" -name '*.json' -type f)
  [[ $(stat -f '%Lp' "$record") == 600 ]] || fail 'release record is not 0600'
  jq -e --arg image "$REQUESTED_IMAGE" --arg digest "$REQUESTED_DIGEST" '
    .schema_version == 1 and .mode == "rehearsal" and .requested.image == $image and
    .requested.digest == $digest and .requested.version == "0.1.164" and
    .backup.sha256_verified == true and (.checks | all(.[]; . == true))
  ' "$record" >/dev/null || fail 'promoted record omitted required evidence'
  [[ $(LC_ALL=C sort "$FIXTURE_SPACE_LOG") == $(printf '%s\n' "$FIXTURE_BACKUPS" "$FIXTURE_DEPLOY/data" "$FIXTURE_DEPLOY/postgres" "$FIXTURE_DEPLOY/redis" | LC_ALL=C sort) ]] \
    || fail 'free space was not checked on all four storage roots'
  ! rg -n 'test-key|POSTGRES_PASSWORD|JWT_SECRET' "$FIXTURE_RECORDS" || fail 'release record leaked a secret'
  cleanup_fixture
}

test_all_post_mutation_failures_use_compatible_rollback() {
  local scenario expected
  for scenario in FAKE_REQUESTED_UP_FAILURE FAKE_REQUESTED_HEALTH_FAILURE FAKE_SMOKE_FAILURE; do
    new_fixture
    if [[ "$scenario" == FAKE_REQUESTED_UP_FAILURE ]]; then
      expected='inspect -> compose config -> baseline -> backup -> pull -> compose up sub2api failed -> compose up sub2api(previous image) -> rollback smoke -> rolled_back'
    elif [[ "$scenario" == FAKE_REQUESTED_HEALTH_FAILURE ]]; then
      expected='inspect -> compose config -> baseline -> backup -> pull -> compose up sub2api -> wait failed -> compose up sub2api(previous image) -> rollback smoke -> rolled_back'
    else
      expected='inspect -> compose config -> baseline -> backup -> pull -> compose up sub2api -> wait -> smoke failed -> compose up sub2api(previous image) -> rollback smoke -> rolled_back'
    fi
    export "$scenario=true"
    ROLLBACK_COMPATIBLE=true run_deploy --mode rehearsal >/dev/null
    unset "$scenario"
    assert_log "$expected"
    assert_one_record rolled_back
    cleanup_fixture
  done
}

test_rollback_disallowed_or_failed_is_terminal() {
  new_fixture
  if FAKE_SMOKE_FAILURE=true run_deploy --mode rehearsal >/dev/null 2>&1; then
    fail 'failed release succeeded without rollback compatibility'
  fi
  assert_one_record rollback_failed
  cleanup_fixture

  new_fixture
  if FAKE_SMOKE_FAILURE=true FAKE_ROLLBACK_HEALTH_FAILURE=true ROLLBACK_COMPATIBLE=true \
    run_deploy --mode rehearsal >/dev/null 2>&1; then
    fail 'rollback with unhealthy previous image succeeded'
  fi
  assert_one_record rollback_failed
  cleanup_fixture

  new_fixture
  if FAKE_SMOKE_FAILURE=true FAKE_ROLLBACK_SMOKE_FAILURE=true ROLLBACK_COMPATIBLE=true \
    run_deploy --mode rehearsal >/dev/null 2>&1; then
    fail 'rollback smoke failure succeeded'
  fi
  assert_one_record rollback_failed
  cleanup_fixture
}

test_backup_verification_and_preflight_recording() {
  new_fixture
  if FAKE_BAD_BACKUP_CHECKSUM=true run_deploy --mode rehearsal >/dev/null 2>&1; then
    fail 'bad backup checksum was accepted'
  fi
  assert_one_record preflight_failed
  record=$(find "$FIXTURE_RECORDS" -name '*.json' -type f)
  jq -e '.backup.sha256_verified == false' "$record" >/dev/null \
    || fail 'unverified backup was recorded as verified'
  cleanup_fixture

  new_fixture
  if FAKE_BASELINE_FAILURE=true run_deploy --mode rehearsal >/dev/null 2>&1; then
    fail 'baseline failure was accepted'
  fi
  assert_one_record preflight_failed
  cleanup_fixture
}

test_release_env_is_parsed_as_single_data_assignment() {
  new_fixture
  : >"$FIXTURE_DEPLOY/release.env"
  if run_deploy --mode rehearsal >/dev/null 2>&1; then
    fail 'missing release image was accepted'
  fi
  [[ ! -e "$FIXTURE_ROOT/executed" ]]
  cleanup_fixture

  new_fixture
  printf 'SUB2API_IMAGE=%s\nSUB2API_IMAGE=%s\n' "$REQUESTED_IMAGE" "$REQUESTED_IMAGE" >"$FIXTURE_DEPLOY/release.env"
  if run_deploy --mode rehearsal >/dev/null 2>&1; then
    fail 'duplicate release image was accepted'
  fi
  cleanup_fixture

  new_fixture
  printf '%s\n' 'SUB2API_IMAGE=$(touch /tmp/sub2api-release-env-was-executed)' >"$FIXTURE_DEPLOY/release.env"
  if run_deploy --mode rehearsal >/dev/null 2>&1; then
    fail 'executable release env content was accepted'
  fi
  [[ ! -e /tmp/sub2api-release-env-was-executed ]] || fail 'release env was executed as shell code'
  cleanup_fixture

  new_fixture
  SUB2API_IMAGE='attacker/override:latest' EXPECTED_VERSION=9.9.9 run_deploy --mode rehearsal >/dev/null
  record=$(find "$FIXTURE_RECORDS" -name '*.json' -type f)
  jq -e --arg image "$REQUESTED_IMAGE" '.requested.image == $image and .requested.version == "0.1.164"' "$record" >/dev/null \
    || fail 'ambient image or version overrode release-file data'
  cleanup_fixture
}

test_production_requires_strict_rehearsal_evidence() {
  local candidate
  new_fixture
  candidate="$FIXTURE_ROOT/rehearsal.json"
  make_rehearsal_record "$candidate"

  for filter in \
    '{requested:{image:$image},state:"promoted"}' \
    '.mode="production"' \
    '.schema_version=2' \
    '.requested.version="0.1.999"' \
    '.backup.sha256_verified=false' \
    '.checks.guard=false' \
    '.checks.extra_gate=false' \
    'del(.previous.image)' \
    '.previous.image_id=1' \
    'del(.backup.path)' \
    '.backup.path=""' \
    '.backup.sha256_verified="true"' \
    'del(.requested.digest)' \
    '.unexpected=true'; do
    make_rehearsal_record "$candidate"
    jq --arg image "$REQUESTED_IMAGE" "$filter" "$candidate" >"$candidate.tmp"
    mv "$candidate.tmp" "$candidate"
    chmod 0600 "$candidate"
    if PRODUCTION_CONFIRMATION="$REQUESTED_DIGEST" REHEARSAL_RECORD="$candidate" \
      run_deploy --mode production >/dev/null 2>&1; then
      fail "production accepted invalid rehearsal evidence: $filter"
    fi
  done

  make_rehearsal_record "$candidate"
  chmod 0644 "$candidate"
  if PRODUCTION_CONFIRMATION="$REQUESTED_DIGEST" REHEARSAL_RECORD="$candidate" run_deploy --mode production >/dev/null 2>&1; then
    fail 'production accepted a non-0600 rehearsal record'
  fi
  make_rehearsal_record "$candidate"
  ln -s "$candidate" "$FIXTURE_ROOT/rehearsal-link.json"
  if PRODUCTION_CONFIRMATION="$REQUESTED_DIGEST" REHEARSAL_RECORD="$FIXTURE_ROOT/rehearsal-link.json" \
    run_deploy --mode production >/dev/null 2>&1; then
    fail 'production accepted a symlinked rehearsal record'
  fi
  assert_log ''
  cleanup_fixture
}

test_container_resolution_and_runtime_metadata_are_exact() {
  local scenario
  for scenario in FAKE_EMPTY_CONTAINER_SERVICE FAKE_DUPLICATE_CONTAINER_SERVICE; do
    new_fixture
    export "$scenario=postgres"
    if run_deploy --mode rehearsal >/dev/null 2>&1; then
      fail "invalid Compose container resolution was accepted: $scenario"
    fi
    unset "$scenario"
    assert_one_record preflight_failed
    cleanup_fixture
  done

  for scenario in FAKE_BASELINE_WORKING_DIR_MISMATCH FAKE_BASELINE_CONFIG_MISMATCH; do
    new_fixture
    export "$scenario=true"
    if run_deploy --mode rehearsal >/dev/null 2>&1; then
      fail "mismatched baseline runtime metadata was accepted: $scenario"
    fi
    unset "$scenario"
    assert_one_record preflight_failed
    cleanup_fixture
  done

  new_fixture
  FAKE_PROMOTED_CONFIG_MISMATCH=true ROLLBACK_COMPATIBLE=true \
    run_deploy --mode rehearsal >/dev/null
  assert_one_record rolled_back
  cleanup_fixture
}

test_production_confirmation_is_digest_only() {
  new_fixture
  candidate="$FIXTURE_ROOT/rehearsal.json"
  make_rehearsal_record "$candidate"
  for confirmation in 0.1.164 "$REQUESTED_IMAGE" sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff; do
    if PRODUCTION_CONFIRMATION="$confirmation" REHEARSAL_RECORD="$candidate" \
      run_deploy --mode production >/dev/null 2>&1; then
      fail "production accepted invalid confirmation: $confirmation"
    fi
  done
  PRODUCTION_CONFIRMATION="$REQUESTED_DIGEST" REHEARSAL_RECORD="$candidate" \
    run_deploy --mode production >/dev/null
  assert_one_record promoted
  cleanup_fixture
}

test_previous_expected_version_is_validated() {
  new_fixture
  if PREVIOUS_EXPECTED_VERSION='v0.1.164-contact-v1' run_deploy --mode rehearsal >/dev/null 2>&1; then
    fail 'release accepted an image tag as PREVIOUS_EXPECTED_VERSION'
  fi
  assert_log ''
  [[ ! -e "$FIXTURE_RECORDS/.release.lock" ]] || fail 'invalid previous version acquired the release lock'
  cleanup_fixture
}

test_project_identity_is_mode_specific() {
  new_fixture
  if COMPOSE_PROJECT_NAME=sub2api run_deploy --mode rehearsal >/dev/null 2>&1; then
    fail 'rehearsal accepted the production Compose project'
  fi
  assert_log ''
  cleanup_fixture

  new_fixture
  candidate="$FIXTURE_ROOT/rehearsal.json"
  make_rehearsal_record "$candidate"
  if COMPOSE_PROJECT_NAME=sub2api-official-rehearsal PRODUCTION_CONFIRMATION="$REQUESTED_DIGEST" \
    REHEARSAL_RECORD="$candidate" run_deploy --mode production >/dev/null 2>&1; then
    fail 'production accepted the rehearsal Compose project'
  fi
  assert_log ''
  cleanup_fixture
}

test_production_rejects_non_server_docker_boundary() {
  local candidate

  new_fixture
  candidate="$FIXTURE_ROOT/rehearsal.json"
  make_rehearsal_record "$candidate"
  if FAKE_UNAME=Darwin PRODUCTION_CONFIRMATION="$REQUESTED_DIGEST" REHEARSAL_RECORD="$candidate" \
    run_deploy --mode production >/dev/null 2>&1; then
    fail 'production accepted a non-Linux execution host'
  fi
  assert_log ''
  cleanup_fixture

  new_fixture
  candidate="$FIXTURE_ROOT/rehearsal.json"
  make_rehearsal_record "$candidate"
  if FAKE_DOCKER_CONTEXT=colima PRODUCTION_CONFIRMATION="$REQUESTED_DIGEST" REHEARSAL_RECORD="$candidate" \
    run_deploy --mode production >/dev/null 2>&1; then
    fail 'production accepted the colima Docker context'
  fi
  assert_log ''
  cleanup_fixture
}

test_interruption_after_lock_writes_one_terminal_record() {
  local deploy_pid attempt
  new_fixture
  FAKE_SMOKE_PAUSE=true FAKE_EXEC_DEPLOY=true ROLLBACK_COMPATIBLE=true \
    run_deploy --mode rehearsal >/dev/null 2>&1 &
  deploy_pid=$!
  for ((attempt = 0; attempt < 200; attempt++)); do
    rg -q 'unexpected interruption' "$FIXTURE_LOG" && break
    sleep 0.05
  done
  rg -q 'unexpected interruption' "$FIXTURE_LOG" || fail 'deploy did not reach post-mutation interruption point'
  kill -TERM "$deploy_pid"
  if wait "$deploy_pid"; then
    fail 'interrupted release succeeded'
  fi
  assert_log 'inspect -> compose config -> baseline -> backup -> pull -> compose up sub2api -> wait -> unexpected interruption -> compose up sub2api(previous image) -> rollback smoke -> rolled_back'
  assert_one_record rolled_back
  cleanup_fixture
}

test_rehearsal_promotes_with_fixed_context
test_all_post_mutation_failures_use_compatible_rollback
test_rollback_disallowed_or_failed_is_terminal
test_backup_verification_and_preflight_recording
test_release_env_is_parsed_as_single_data_assignment
test_production_requires_strict_rehearsal_evidence
test_production_confirmation_is_digest_only
test_previous_expected_version_is_validated
test_project_identity_is_mode_specific
test_production_rejects_non_server_docker_boundary
test_container_resolution_and_runtime_metadata_are_exact
test_interruption_after_lock_writes_one_terminal_record

printf 'PASS: Sub2API recreate-only release orchestration\n'
