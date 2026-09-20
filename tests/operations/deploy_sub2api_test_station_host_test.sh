#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
EXECUTOR=${EXECUTOR_UNDER_TEST:-$ROOT/ops/deploy-sub2api-test-station-host.sh}
TMP_BASE=$(cd "${TMPDIR:-/tmp}" && pwd -P)
FIXTURE=$(mktemp -d "$TMP_BASE/test-station-host.XXXXXX")
trap 'rm -rf -- "$FIXTURE"' EXIT
fail(){ printf 'FAIL: %s\n' "$1" >&2; exit 1; }
[[ -f "$EXECUTOR" ]] || fail 'executor missing'

OLD_COMMIT=$(printf '1%.0s' {1..40})
OLD_TREE=$(printf '2%.0s' {1..40})
NEW_COMMIT=$(printf 'a%.0s' {1..40})
NEW_TREE=$(printf 'b%.0s' {1..40})
MIGRATION_SHA=$(printf 'c%.0s' {1..64})
ARCHIVE_SHA=''
IMAGE_ID="sha256:$(printf 'd%.0s' {1..64})"

setup(){
  CASE=$FIXTURE/$1
  DEPLOY_ROOT=$CASE/deploy
  STAGE=$CASE/stage
  OLD_RELEASE=$DEPLOY_ROOT/releases/$OLD_COMMIT
  STATE=$DEPLOY_ROOT/release-state.json
  BIN=$CASE/bin
  EVENT_LOG=$CASE/events.log
  ACTIVE_PHASE=$CASE/active-phase
  SERVICE_COUNT=$CASE/service-count
  PROBE_COUNT=$CASE/probe-count
  REAL_MKTEMP=$(command -v mktemp)
  REAL_MV=$(command -v mv)
  mkdir -p "$STAGE" "$OLD_RELEASE" "$BIN" "$DEPLOY_ROOT/backups"
  printf 'name: sub2api-test-station\nnetworks: {test: {name: sub2api-test-station-network}}\nservices: {}\n' >"$STAGE/compose.yaml"
  cp "$STAGE/compose.yaml" "$OLD_RELEASE/compose.yaml"
  printf ':80 { reverse_proxy test-station-api:8080 }\n' >"$STAGE/Caddyfile"
  cp "$STAGE/Caddyfile" "$OLD_RELEASE/Caddyfile"
  printf 'CLONE_APP_IMAGE=sub2api-test-station-runtime:%s\nCLONE_CADDY_IMAGE=caddy\n' "$OLD_COMMIT" >"$OLD_RELEASE/.env"
  chmod 0600 "$OLD_RELEASE/.env"
  printf 'image-archive\n' >"$STAGE/image.tar"
  ARCHIVE_SHA=$(sha256sum "$STAGE/image.tar" | awk '{print $1}')
  printf '%s\n' "$ARCHIVE_SHA" >"$STAGE/image.sha256"
  cat >"$STATE" <<JSON
{"source_commit":"$OLD_COMMIT","source_tree":"$OLD_TREE","image_digest":"old-archive","release_dir":"$OLD_RELEASE","previous_release_dir":null,"project_name":"sub2api-test-station","result":"succeeded","updated_at":"2026-09-19T00:00:00Z"}
JSON
  chmod 0600 "$STATE"
  printf 'previous\n' >"$ACTIVE_PHASE"
  printf '0\n' >"$SERVICE_COUNT"
  printf '0\n' >"$PROBE_COUNT"
  printf '%s\n' "$IMAGE_ID" >"$CASE/old-image-id"
  : >"$EVENT_LOG"

  cat >"$STAGE/backup.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'backup %s\n' "$*" >>"${EVENT_LOG:?}"
[[ "${FAKE_MODE:-ok}" != backup-fail ]] || exit 71
backup_dir="${DEPLOY_ROOT:?}/backups/20260920T120000Z"
mkdir -p "$backup_dir"
printf 'test_station_backup status=succeeded backup_dir=%s\n' "$backup_dir"
SH
  chmod 0700 "$STAGE/backup.sh"

  cat >"$BIN/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"${EVENT_LOG:?}"
mode=${FAKE_MODE:-ok}
case "${1:-}" in
  ps)
    [[ "$*" == *'com.docker.compose.service=test-station-api'* ]] || exit 61
    if [[ "$*" == *'--format {{.ID}}'* ]]; then printf 'old-api-container\n'; else printf '%s\n' "${OLD_RELEASE:?}/compose.yaml"; fi
    ;;
  load)
    [[ "$*" == "load --input ${STAGE:?}/image.tar" ]] || exit 62
    ;;
  image)
    [[ "${2:-}" == inspect ]] || exit 63
    tag=${*: -1}
    if [[ "$tag" == "sub2api-test-station-runtime:${OLD_COMMIT:?}" ]]; then
      [[ "$mode" != previous-image-missing ]] || exit 1
      if [[ "$mode" == previous-image-mismatch ]]; then printf 'sha256:%064d\n' 8; else cat "${CASE:?}/old-image-id"; fi
    elif [[ "$mode" == image-mismatch ]]; then
      printf 'sha256:%064d\n' 9
    else
      printf '%s\n' "${IMAGE_ID:?}"
    fi
    ;;
  inspect)
    format=${3:-}; container=${4:-}
    if [[ "$format" == '{{.Image}}' && "$container" == old-api-container ]]; then cat "${CASE:?}/old-image-id"; exit 0; fi
    if [[ "$format" == '{{.State.Health.Status}}' ]]; then
      service=${container#*-container}
      if [[ "$mode" == candidate-worker-unhealthy && "$container" == candidate-test-station-worker-container ]]; then printf 'unhealthy\n'; else printf 'healthy\n'; fi
      exit 0
    fi
    if [[ "$format" == '{{.State.Status}}' ]]; then
      if [[ "$mode" == candidate-caddy-unhealthy && "$container" == candidate-test-station-caddy-container ]]; then printf 'exited\n'; else printf 'running\n'; fi
      exit 0
    fi
    exit 63
    ;;
  compose)
    args="$*"
    [[ "$args" == *'--project-name sub2api-test-station'* ]] || exit 64
    if [[ "$args" == *"-f ${OLD_RELEASE:?}/compose.yaml"* ]]; then phase=previous; else phase=candidate; fi
    if [[ "$args" == *' config --quiet'* ]]; then exit 0; fi
    if [[ "$args" == *' up -d --remove-orphans'* ]]; then
      if [[ "$phase" == candidate && "$mode" == candidate-up-fail ]]; then exit 72; fi
      if [[ "$phase" == previous && "$mode" == rollback-fail ]]; then exit 73; fi
      printf '%s\n' "$phase" >"${ACTIVE_PHASE:?}"
      exit 0
    fi
    if [[ "$args" == *' ps -q '* ]]; then
      service=${args##* ps -q }
      if [[ "$phase" == candidate && "$mode" == transient-worker && "$service" == test-station-worker ]]; then
        count=$(cat "${SERVICE_COUNT:?}")
        if [[ "$count" == 0 ]]; then printf '1\n' >"$SERVICE_COUNT"; exit 0; fi
      fi
      printf '%s-%s-container\n' "$phase" "$service"
      exit 0
    fi
    exit 66
    ;;
  *) exit 67 ;;
esac
SH
  chmod +x "$BIN/docker"


  cat >"$BIN/mktemp" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${FAKE_MODE:-ok}" == state-mktemp-fail && "$*" == *'.release-state.'* ]]; then exit 80; fi
exec "${REAL_MKTEMP:?}" "$@"
SH
  chmod +x "$BIN/mktemp"

  cat >"$BIN/mv" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${FAKE_MODE:-ok}" == state-mv-fail && "${*: -1}" == "${STATE:?}" ]]; then exit 81; fi
exec "${REAL_MV:?}" "$@"
SH
  chmod +x "$BIN/mv"

  cat >"$BIN/probe" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
url=${1:?}
phase=$(cat "${ACTIVE_PHASE:?}")
printf 'probe %s %s\n' "$phase" "$url" >>"${EVENT_LOG:?}"
if [[ "$url" == */health ]]; then
  printf '200\tapplication/json\t{"status":"ok"}\n'
  exit 0
fi
if [[ "$phase" == candidate && "${FAKE_MODE:-ok}" == readiness-html ]]; then
  printf '200\ttext/html\t<html>spa</html>\n'
elif [[ "$phase" == candidate && "${FAKE_MODE:-ok}" == readiness-malformed ]]; then
  printf '200\tapplication/json\tnot-json\n'
elif [[ "$phase" == candidate && "${FAKE_MODE:-ok}" == readiness-flaky ]]; then
  count=$(cat "${PROBE_COUNT:?}"); count=$((count+1)); printf '%s\n' "$count" >"$PROBE_COUNT"
  if ((count % 3 == 0)); then printf '503\tapplication/json\t{"status":"not_ready"}\n'; else printf '200\tapplication/json\t{"status":"ready"}\n'; fi
elif [[ "$phase" == candidate && ( "${FAKE_MODE:-ok}" == readiness-not-ready || "${FAKE_MODE:-ok}" == rollback-fail ) ]]; then
  printf '503\tapplication/json\t{"status":"not_ready"}\n'
else
  printf '200\tapplication/json; charset=utf-8\t{"status":"ready"}\n'
fi
SH
  chmod +x "$BIN/probe"
}

run_exec(){
  env TEST_STATION_TEST_MODE=true PATH="$BIN:$PATH" EVENT_LOG="$EVENT_LOG" DOCKER_BIN="$BIN/docker" \
    TEST_STATION_PROBE_BIN="$BIN/probe" TEST_STATION_PROBE_INTERVAL_SECONDS=0 TEST_STATION_SERVICE_INTERVAL_SECONDS=0 \
    ACTIVE_PHASE="$ACTIVE_PHASE" SERVICE_COUNT="$SERVICE_COUNT" PROBE_COUNT="$PROBE_COUNT" OLD_RELEASE="$OLD_RELEASE" OLD_COMMIT="$OLD_COMMIT" CASE="$CASE" STAGE="$STAGE" IMAGE_ID="$IMAGE_ID" REAL_MKTEMP="$REAL_MKTEMP" REAL_MV="$REAL_MV" STATE="$STATE" \
    DEPLOY_ROOT="$DEPLOY_ROOT" RELEASE_STATE="$STATE" FAKE_MODE="${FAKE_MODE:-ok}" \
    bash "$EXECUTOR" --staging-root "$STAGE" --image-archive "$STAGE/image.tar" \
    --image-sha256 "$ARCHIVE_SHA" --image-id "$IMAGE_ID" --compose "$STAGE/compose.yaml" \
    --caddy "$STAGE/Caddyfile" --backup-script "$STAGE/backup.sh" \
    --source-commit "$NEW_COMMIT" --source-tree "$NEW_TREE" \
    --migration-set-sha256 "$MIGRATION_SHA" --deploy-root "$DEPLOY_ROOT"
}

candidate_release(){ printf '%s/releases/%s' "$DEPLOY_ROOT" "$NEW_COMMIT"; }
assert_previous_state_unchanged(){
  python3 - "$STATE" "$OLD_COMMIT" <<'PY' || fail 'previous state changed'
import json,sys
v=json.load(open(sys.argv[1],encoding='utf-8'))
assert v['source_commit']==sys.argv[2]
PY
}
assert_no_destructive_commands(){
  ! grep -Eq 'compose .* down|volume rm| down -v' "$EVENT_LOG" || fail 'destructive Docker command used'
}
assert_rollback_invoked(){
  grep -F -- "-f $OLD_RELEASE/compose.yaml up -d --remove-orphans" "$EVENT_LOG" >/dev/null || fail 'previous release was not restored'
  [[ "$(cat "$ACTIVE_PHASE")" == previous ]] || fail 'active phase is not previous'
  assert_no_destructive_commands
}

test_success_records_previous_backup_and_identity(){
  setup success
  run_exec >/dev/null || fail 'success case failed'
  release=$(candidate_release)
  python3 - "$STATE" "$release" "$OLD_RELEASE" "$DEPLOY_ROOT/backups/20260920T120000Z" "$IMAGE_ID" "$ARCHIVE_SHA" "$MIGRATION_SHA" <<'PY' || fail 'schema v2 state invalid'
import json,sys
v=json.load(open(sys.argv[1],encoding='utf-8'))
assert v['schema_version']==2 and v['result']=='succeeded' and v['rolled_back'] is False
assert v['release_dir']==sys.argv[2] and v['previous_release_dir']==sys.argv[3]
assert v['backup_dir']==sys.argv[4] and v['image_id']==sys.argv[5]
assert v['image_archive_sha256']==sys.argv[6] and v['migration_set_sha256']==sys.argv[7]
PY
  [[ "$(grep -c '^probe candidate .*readyz$' "$EVENT_LOG")" == 3 ]] || fail 'candidate readiness was not consecutive'
  assert_no_destructive_commands
}

test_preflight_failures_do_not_start_candidate(){
  local case_name
  for case_name in missing-state wrong-project state-mismatch missing-previous previous-image-missing previous-image-mismatch backup-fail image-mismatch; do
    setup "$case_name"
    case "$case_name" in
      missing-state) rm "$STATE" ;;
      wrong-project) python3 - "$STATE" <<'PY'
import json,sys
p=sys.argv[1]; v=json.load(open(p)); v['project_name']='other'; open(p,'w').write(json.dumps(v))
PY
        ;;
      state-mismatch) python3 - "$STATE" <<'PY'
import json,sys
p=sys.argv[1]; v=json.load(open(p)); v['release_dir']=v['release_dir']+'-other'; open(p,'w').write(json.dumps(v))
PY
        ;;
      missing-previous) rm "$OLD_RELEASE/Caddyfile" ;;
      previous-image-missing) FAKE_MODE=previous-image-missing ;;
      previous-image-mismatch) FAKE_MODE=previous-image-mismatch ;;
      backup-fail) FAKE_MODE=backup-fail ;;
      image-mismatch) FAKE_MODE=image-mismatch ;;
    esac
    if FAKE_MODE="${FAKE_MODE:-ok}" run_exec >/dev/null 2>&1; then fail "$case_name returned success"; fi
    ! grep -F -- "-f $(candidate_release)/compose.yaml up -d --remove-orphans" "$EVENT_LOG" >/dev/null || fail "$case_name started candidate"
    [[ "$(cat "$ACTIVE_PHASE")" == previous ]] || fail "$case_name changed active phase"
    assert_no_destructive_commands
    unset FAKE_MODE
  done
}

test_transient_service_health_recovers(){
  setup transient-worker
  FAKE_MODE=transient-worker run_exec >/dev/null || fail 'transient worker did not recover'
  [[ "$(cat "$ACTIVE_PHASE")" == candidate ]] || fail 'transient success did not keep candidate active'
  [[ "$(cat "$SERVICE_COUNT")" == 1 ]] || fail 'transient worker was not retried'
}

test_candidate_failures_restore_previous(){
  local mode
  for mode in candidate-up-fail candidate-worker-unhealthy candidate-caddy-unhealthy readiness-html readiness-malformed readiness-not-ready readiness-flaky; do
    setup "$mode"
    if FAKE_MODE=$mode run_exec >/dev/null 2>&1; then fail "$mode returned success"; fi
    assert_rollback_invoked
    assert_previous_state_unchanged
    failure=$(candidate_release)/failure.json
    [[ -f "$failure" ]] || fail "$mode did not write failure evidence"
    python3 - "$failure" <<'PY' || fail 'rollback evidence invalid'
import json,sys
v=json.load(open(sys.argv[1],encoding='utf-8'))
assert v['result']=='failed' and v['rolled_back'] is True
PY
  done
}

test_state_commit_failures_restore_previous(){
  local mode
  for mode in state-mktemp-fail state-mv-fail; do
    setup "$mode"
    if FAKE_MODE=$mode run_exec >/dev/null 2>&1; then fail "$mode returned success"; fi
    assert_rollback_invoked
    assert_previous_state_unchanged
  done
}

test_rollback_failure_is_recorded(){
  setup rollback-fail
  if FAKE_MODE=rollback-fail run_exec >/dev/null 2>&1; then fail 'rollback failure returned success'; fi
  assert_previous_state_unchanged
  failure=$(candidate_release)/failure.json
  python3 - "$failure" <<'PY' || fail 'rollback failure evidence invalid'
import json,sys
v=json.load(open(sys.argv[1],encoding='utf-8'))
assert v['result']=='failed' and v['rolled_back'] is False
PY
  assert_no_destructive_commands
}

test_rejects_bad_checksum_and_unsafe_root(){
  setup checksum
  if ARCHIVE_SHA=$(printf 'e%.0s' {1..64}) run_exec >/dev/null 2>&1; then fail 'bad checksum accepted'; fi
  setup unsafe
  if DEPLOY_ROOT=/opt/other run_exec >/dev/null 2>&1; then fail 'unsafe deploy root accepted'; fi
}

test_success_records_previous_backup_and_identity
test_preflight_failures_do_not_start_candidate
test_transient_service_health_recovers
test_candidate_failures_restore_previous
test_state_commit_failures_restore_previous
test_rollback_failure_is_recorded
test_rejects_bad_checksum_and_unsafe_root
printf 'PASS: independent test station host executor\n'
