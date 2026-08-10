#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT="$ROOT/ops/smoke-sub2api-release.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

new_fixture() {
  FIXTURE_ROOT=$(cd "$(mktemp -d)" && pwd -P)
  DEPLOY_ROOT="$FIXTURE_ROOT/deploy"
  FAKE_BIN="$FIXTURE_ROOT/bin"
  COUNTS_FILE="$FIXTURE_ROOT/record-counts.json"
  ADMIN_KEY_FILE="$FIXTURE_ROOT/admin.key"
  GATEWAY_KEY_FILE="$FIXTURE_ROOT/gateway.key"
  CURL_LOG="$FIXTURE_ROOT/curl.log"
  BASE_COMPOSE="$DEPLOY_ROOT/base.yaml"
  IMAGE_OVERLAY="$DEPLOY_ROOT/image-overlay.yaml"
  RELEASE_ENV="$DEPLOY_ROOT/release.env"
  SECRET_ENV="$DEPLOY_ROOT/secret.env"
  mkdir -p "$DEPLOY_ROOT" "$FAKE_BIN"
  printf '{"users":2,"accounts":3,"groups":4,"api_keys":5,"settings":1,"usage_logs":6}\n' >"$COUNTS_FILE"
  printf 'admin-secret-not-for-output\n' >"$ADMIN_KEY_FILE"
  printf 'gateway-secret-not-for-output\n' >"$GATEWAY_KEY_FILE"
  : >"$BASE_COMPOSE"
  : >"$IMAGE_OVERLAY"
  : >"$RELEASE_ENV"
  : >"$SECRET_ENV"
  : >"$CURL_LOG"

  cat >"$FAKE_BIN/curl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

require_pair() {
  local option=$1 value=$2
  shift 2
  while (($#)); do
    if [[ "$1" == "$option" ]]; then
      [[ "${2:-}" == "$value" ]] || exit 70
      return 0
    fi
    shift
  done
  exit 71
}

require_pair --connect-timeout 5 "$@"
require_pair --max-time 15 "$@"
require_pair --max-filesize 1048576 "$@"

url=${!#}
mode=${FAKE_SMOKE_MODE:-success}
printf '%s\n' "$url" >>"${FAKE_CURL_LOG:?}"
case "$url" in
  */health)
    printf '{"status":"ok"}\n'
    ;;
  */api/v1/admin/system/version)
    if [[ "$mode" == wrong-version ]]; then
      printf '{"data":{"version":"wrong-version"}}\n'
    else
      printf '{"data":{"version":"v0.1.164"}}\n'
    fi
    ;;
  */api/v1/settings/public)
    if [[ "$mode" == missing-support ]]; then
      printf '{"data":{"custom_menu_items":[]}}\n'
    else
      printf '{"data":{"custom_menu_items":[{"id":"xingqiao-support","url":"md:support"}]}}\n'
    fi
    ;;
  */api/v1/admin/system/update|*/api/v1/admin/system/rollback)
    body=''
    for ((index = 1; index <= $#; index++)); do
      if [[ "${!index}" == -o ]]; then
        next=$((index + 1))
        body=${!next}
      fi
    done
    [[ -n "$body" ]] || exit 72
    endpoint=update
    [[ "$url" == */update ]] || endpoint=rollback
    status=409
    response='{"code":"DOCKER_DEPLOYMENT_UPDATE_REQUIRED"}'
    [[ "$mode" != "$endpoint-status" ]] || status=200
    [[ "$mode" != "$endpoint-malformed" ]] || response='{'
    [[ "$mode" != "$endpoint-body" ]] || response='{"code":"WRONG_GUARD"}'
    printf '%s\n' "$response" >"$body"
    printf '%s' "$status"
    ;;
  */v1/models)
    [[ "$mode" != gateway-fail ]] || exit 22
    printf '{"data":[]}\n'
    ;;
  *) exit 23 ;;
esac
SH

  cat >"$FAKE_BIN/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

expected_args=(
  compose
  --project-name "${FAKE_EXPECTED_PROJECT:-sub2api-deploy}"
  --project-directory "${FAKE_EXPECTED_DEPLOY_ROOT:-$FAKE_DEPLOY_ROOT}"
  --env-file "${FAKE_EXPECTED_SECRET_ENV:-$FAKE_SECRET_ENV}"
  --env-file "${FAKE_EXPECTED_RELEASE_ENV:-$FAKE_RELEASE_ENV}"
  -f "${FAKE_EXPECTED_BASE_COMPOSE:-$FAKE_BASE_COMPOSE}"
  -f "${FAKE_EXPECTED_IMAGE_OVERLAY:-$FAKE_IMAGE_OVERLAY}"
  exec -T postgres
)
for expected in "${expected_args[@]}"; do
  [[ "${1:-}" == "$expected" ]] || exit 31
  shift
done
[[ "$*" == *psql* ]] || exit 32
if [[ "${FAKE_SMOKE_MODE:-}" == lower-count ]]; then
  printf '{"users":1,"accounts":3,"groups":4,"api_keys":5,"settings":1,"usage_logs":6}\n'
else
  printf '{"users":2,"accounts":3,"groups":4,"api_keys":5,"settings":1,"usage_logs":6}\n'
fi
SH
  chmod 0755 "$FAKE_BIN/curl" "$FAKE_BIN/docker"
}

cleanup_fixture() {
  rm -rf -- "$FIXTURE_ROOT"
}

run_smoke() {
  env PATH="$FAKE_BIN:$PATH" \
    FAKE_CURL_LOG="$CURL_LOG" \
    FAKE_DEPLOY_ROOT="$DEPLOY_ROOT" \
    FAKE_SECRET_ENV="$SECRET_ENV" \
    FAKE_RELEASE_ENV="$RELEASE_ENV" \
    FAKE_BASE_COMPOSE="$BASE_COMPOSE" \
    FAKE_IMAGE_OVERLAY="$IMAGE_OVERLAY" \
    DEPLOY_ROOT="$DEPLOY_ROOT" \
    SECRET_ENV="$SECRET_ENV" \
    RELEASE_ENV="$RELEASE_ENV" \
    BASE_COMPOSE="$BASE_COMPOSE" \
    IMAGE_OVERLAY="$IMAGE_OVERLAY" \
    COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME_OVERRIDE:-sub2api-deploy}" \
    BASE_URL='https://sub2api.invalid' \
    EXPECTED_VERSION='v0.1.164' \
    ADMIN_API_KEY_FILE="$ADMIN_KEY_FILE" \
    GATEWAY_API_KEY_FILE="$GATEWAY_KEY_FILE" \
    EXPECTED_RECORD_COUNTS_FILE="$COUNTS_FILE" \
    /bin/bash "$SCRIPT"
}

test_rehearsal_project_never_falls_back_to_production() {
  new_fixture
  output=$(COMPOSE_PROJECT_NAME_OVERRIDE=sub2api-official-rehearsal \
    FAKE_EXPECTED_PROJECT=sub2api-official-rehearsal run_smoke 2>&1) \
    || fail "rehearsal project failed: $output"
  [[ "$output" == release_smoke=passed ]] || fail 'rehearsal smoke did not pass'
  cleanup_fixture

  new_fixture
  if COMPOSE_PROJECT_NAME_OVERRIDE=unapproved-project run_smoke >/dev/null 2>&1; then
    fail 'smoke accepted an unapproved Compose project'
  fi
  [[ ! -s "$CURL_LOG" ]] || fail 'invalid project made HTTP requests'
  cleanup_fixture
}

assert_failure_is_redacted() {
  local output=$1
  [[ "$output" != *admin-secret-not-for-output* ]] || fail 'admin key leaked to output'
  [[ "$output" != *gateway-secret-not-for-output* ]] || fail 'gateway key leaked to output'
}

test_success_requires_bounded_requests_and_both_guards() {
  new_fixture
  output=$(run_smoke 2>&1) || fail "success case failed: $output"
  [[ "$output" == 'release_smoke=passed' ]] || fail "unexpected success output: $output"
  [[ $(rg -c '/api/v1/admin/system/update$' "$CURL_LOG") == 1 ]] || fail 'update guard was not called exactly once'
  [[ $(rg -c '/api/v1/admin/system/rollback$' "$CURL_LOG") == 1 ]] || fail 'rollback guard was not called exactly once'
  [[ $(wc -l <"$CURL_LOG" | tr -d ' ') == 6 ]] || fail 'unexpected smoke request count'
  cleanup_fixture
}

test_compose_context_is_exact() {
  local mismatch
  for mismatch in project deploy_root secret_env release_env base_compose image_overlay; do
    new_fixture
    case "$mismatch" in
      project) override=(FAKE_EXPECTED_PROJECT=changed-project) ;;
      deploy_root) override=(FAKE_EXPECTED_DEPLOY_ROOT="$FIXTURE_ROOT/changed-root") ;;
      secret_env) override=(FAKE_EXPECTED_SECRET_ENV="$FIXTURE_ROOT/changed-secret.env") ;;
      release_env) override=(FAKE_EXPECTED_RELEASE_ENV="$FIXTURE_ROOT/changed-release.env") ;;
      base_compose) override=(FAKE_EXPECTED_BASE_COMPOSE="$FIXTURE_ROOT/changed-base.yaml") ;;
      image_overlay) override=(FAKE_EXPECTED_IMAGE_OVERLAY="$FIXTURE_ROOT/changed-overlay.yaml") ;;
    esac
    if (export "${override[@]}"; run_smoke >/dev/null 2>&1); then
      fail "smoke accepted changed Compose $mismatch context"
    fi
    cleanup_fixture
  done
}

test_read_only_mismatches_are_redacted() {
  local mode
  for mode in wrong-version missing-support lower-count gateway-fail; do
    new_fixture
    if output=$(FAKE_SMOKE_MODE="$mode" run_smoke 2>&1); then
      fail "$mode returned success"
    fi
    assert_failure_is_redacted "$output"
    cleanup_fixture
  done
}

test_each_guard_rejects_wrong_status_malformed_json_and_wrong_code() {
  local endpoint defect mode
  for endpoint in update rollback; do
    for defect in status malformed body; do
      mode="$endpoint-$defect"
      new_fixture
      if output=$(FAKE_SMOKE_MODE="$mode" run_smoke 2>&1); then
        fail "$mode returned success"
      fi
      assert_failure_is_redacted "$output"
      [[ $(rg -c "/api/v1/admin/system/$endpoint$" "$CURL_LOG") == 1 ]] \
        || fail "$mode did not call its endpoint exactly once"
      cleanup_fixture
    done
  done
}

test_literal_externalization_rehearsal_writes_report_sets_and_rollbacks() {
  local output_root output
  output_root=$(cd "$(mktemp -d)" && pwd -P)
  output=$(REHEARSAL_OUTPUT_DIR="$output_root" /bin/bash "$SCRIPT" --rehearsal --rollback 2>&1) \
    || fail "literal rehearsal failed: $output"
  [[ "$output" == externalization_rehearsal=passed* ]] || fail "unexpected literal rehearsal output: $output"
  [[ -f "$output_root/report-sets.jsonl" && -f "$output_root/cutover-audit.jsonl" && -f "$output_root/summary.json" ]] \
    || fail 'literal rehearsal omitted durable artifacts'
  [[ $(wc -l <"$output_root/report-sets.jsonl" | tr -d ' ') == 4 ]] || fail 'literal rehearsal did not persist four report sets'
  [[ $(wc -l <"$output_root/cutover-audit.jsonl" | tr -d ' ') == 8 ]] || fail 'literal rehearsal did not persist promotion and rollback for four pages'
  jq -e '
    .schema_version == 1 and .environment == "isolated_local_fixture" and
    (.pages | length) == 4 and
    ([.pages[].windows[]] | length) == 12 and
    ([.pages[].windows[]] | all(.kind == "minimum" or .kind == "default" or .kind == "maximum")) and
    ([.pages[]] | all(.operator == "local-rehearsal-operator" and (.compared_at | length > 0) and .promotion_result == "mode_changed" and .rollback_result == "rolled_back" and .active_mode == "legacy_only"))
  ' "$output_root/summary.json" >/dev/null || fail 'literal rehearsal summary is incomplete'
  rm -rf -- "$output_root"
}

test_success_requires_bounded_requests_and_both_guards
test_rehearsal_project_never_falls_back_to_production
test_compose_context_is_exact
test_read_only_mismatches_are_redacted
test_each_guard_rejects_wrong_status_malformed_json_and_wrong_code
test_literal_externalization_rehearsal_writes_report_sets_and_rollbacks

printf 'PASS: Sub2API release smoke contracts\n'
