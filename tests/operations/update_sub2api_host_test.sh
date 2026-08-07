#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT="$ROOT/ops/update-sub2api-host.sh"
IMAGE="sha256:$(printf 'a%.0s' {1..64})"
VERSION=0.1.171
OPERATION_ID=op-test-001
REPOSITORY=ghcr.io/leesssong/xingqiao-sub2api
DIGEST="$REPOSITORY@sha256:$(printf 'b%.0s' {1..64})"
UPSTREAM_COMMIT=$(printf 'c%.0s' {1..40})
SOURCE_COMMIT=$(printf 'd%.0s' {1..40})
SOURCE_TREE=$(printf 'e%.0s' {1..40})
MIGRATIONS_HASH=$(printf 'f%.0s' {1..64})
CURL_IMAGE="curlimages/curl@sha256:$(printf '1%.0s' {1..64})"

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

new_fixture() {
  local temporary
  temporary=$(mktemp -d)
  FIXTURE_ROOT=$(cd "$temporary" && pwd -P)
  FIXTURE_BIN="$FIXTURE_ROOT/bin"
  FIXTURE_DEPLOY="$FIXTURE_ROOT/production"
  FIXTURE_RECORDS="$FIXTURE_ROOT/release-records"
  FIXTURE_STAGING="$FIXTURE_ROOT/release-staging"
  FIXTURE_STATE="$FIXTURE_ROOT/release-state"
  FIXTURE_TRACE="$FIXTURE_ROOT/events.log"
  FIXTURE_CALL="$FIXTURE_ROOT/executor-call"
  FIXTURE_DOCKER="$FIXTURE_ROOT/docker.log"
  REAL_JQ=$(command -v jq)
  mkdir -p "$FIXTURE_BIN" "$FIXTURE_DEPLOY/secrets" "$FIXTURE_RECORDS" "$FIXTURE_STAGING"
  : >"$FIXTURE_TRACE"
  : >"$FIXTURE_DOCKER"
  : >"$FIXTURE_DEPLOY/compose.yaml"
  : >"$FIXTURE_DEPLOY/.env"
  printf 'admin-secret\n' >"$FIXTURE_DEPLOY/secrets/admin-key"
  printf 'gateway-secret\n' >"$FIXTURE_DEPLOY/secrets/gateway-key"
  chmod 0600 "$FIXTURE_DEPLOY/.env" "$FIXTURE_DEPLOY/secrets/admin-key" "$FIXTURE_DEPLOY/secrets/gateway-key"
  write_valid_state

  cat >"$FIXTURE_BIN/date" <<'SH'
#!/usr/bin/env bash
if [[ "$*" == '-u +%s' ]]; then printf '1786000000\n'; else exec /bin/date "$@"; fi
SH
  cat >"$FIXTURE_BIN/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${FAKE_DOCKER_LOG:?}"
if [[ "$1 $2" == 'image inspect' ]]; then
  case "${FAKE_IMAGE_SCENARIO:-valid}" in
    missing_digest) repo_digests='[]' ;;
    ambiguous_digest) repo_digests='["'"${EXPECTED_DIGEST:?}"'","'"${EXPECTED_REPOSITORY:?}"'@sha256:'"$(printf '9%.0s' {1..64})"'"]' ;;
    unapproved_digest) repo_digests='["ghcr.io/other/sub2api@sha256:'"$(printf '8%.0s' {1..64})"'"]' ;;
    *) repo_digests='["'"${EXPECTED_DIGEST:?}"'"]' ;;
  esac
  qualified=true
  version=${EXPECTED_VERSION:?}
  tested=${EXPECTED_SOURCE_TREE:?}
  [[ "${FAKE_IMAGE_SCENARIO:-valid}" != invalid_labels ]] || qualified=false
  [[ "${FAKE_IMAGE_SCENARIO:-valid}" != tree_mismatch ]] || tested=$(printf '7%.0s' {1..40})
  printf '[{"Id":"%s","Os":"linux","Architecture":"amd64","RepoDigests":%s,"Config":{"Labels":{"com.xingqiao.sub2api.qualified":"%s","com.xingqiao.sub2api.upstream.version":"%s","com.xingqiao.sub2api.upstream.commit":"%s","com.xingqiao.sub2api.source.commit":"%s","com.xingqiao.sub2api.source.tree":"%s","com.xingqiao.sub2api.tested.tree":"%s","com.xingqiao.sub2api.migrations.sha256":"%s"}}}]\n' \
    "${EXPECTED_IMAGE:?}" "$repo_digests" "$qualified" "$version" "${EXPECTED_UPSTREAM_COMMIT:?}" \
    "${EXPECTED_SOURCE_COMMIT:?}" "${EXPECTED_SOURCE_TREE:?}" "$tested" "${EXPECTED_MIGRATIONS_HASH:?}"
elif [[ "$1 $2" == 'image tag' ]]; then
  exit 0
elif [[ "$1 $2" == 'image save' ]]; then
  [[ "$3" == --output && -n "${4:-}" ]] || exit 64
  printf 'preloaded image archive\n' >"$4"
else
  exit 64
fi
SH
  cat >"$FIXTURE_BIN/sha256sum" <<'SH'
#!/usr/bin/env bash
exec /usr/bin/shasum -a 256 "$@"
SH
  cat >"$FIXTURE_BIN/blue-green-executor" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
executor_args=("$@")
preloaded_archive=''
preloaded_archive_sha256=''
preloaded_image_id=''
while (($#)); do
  case "$1" in
    --preloaded-archive) preloaded_archive=${2:?}; shift 2 ;;
    --preloaded-archive-sha256) preloaded_archive_sha256=${2:?}; shift 2 ;;
    --preloaded-image-id) preloaded_image_id=${2:?}; shift 2 ;;
    *) shift ;;
  esac
done
[[ -f "$preloaded_archive" ]] || { printf 'preloaded archive missing during executor\n' >&2; exit 65; }
actual_archive_sha256=$(/usr/bin/shasum -a 256 "$preloaded_archive" | awk '{print $1}')
[[ "$actual_archive_sha256" == "$preloaded_archive_sha256" ]] || { printf 'preloaded archive checksum mismatch\n' >&2; exit 66; }
[[ "$preloaded_image_id" == "${EXPECTED_IMAGE:?}" ]] || { printf 'preloaded image ID mismatch\n' >&2; exit 67; }
archive_mode=$(stat -f '%Lp' "$preloaded_archive" 2>/dev/null || stat -c '%a' "$preloaded_archive")
{
  printf 'args:'; printf ' <%s>' "${executor_args[@]}"; printf '\n'
  printf 'preloaded-archive=%s\n' "$preloaded_archive"
  printf 'preloaded-archive-sha256=%s\n' "$preloaded_archive_sha256"
  printf 'archive-sha256=%s\n' "$actual_archive_sha256"
  printf 'preloaded-image-id=%s\n' "$preloaded_image_id"
  printf 'archive-mode=%s\n' "$archive_mode"
  for key in DEPLOY_ROOT BASE_COMPOSE SECRET_ENV RELEASE_ENV RELEASE_STATE RELEASE_RECORD_ROOT ADMIN_API_KEY_FILE GATEWAY_API_KEY_FILE BASE_URL NETWORK_CURL_IMAGE NETWORK_CURL_IMAGE_ALLOWLIST RELEASE_STAGING_ROOT RELEASE_PRELOADED_IMAGE RELEASE_EVENT_LOG; do
    printf '%s=%s\n' "$key" "${!key-}"
  done
} >"${FAKE_EXECUTOR_CALL:?}"
printf 'executor event\n' >>"${RELEASE_EVENT_LOG:?}"
if [[ "${FAKE_EXECUTOR_RESULT:-success}" == failure ]]; then
  printf 'executor failed\n' >&2
  exit 23
fi
printf '{"schema_version":1,"downtime_required":false,"result":"succeeded","active_slot":"green","active_upstream":"sub2api-green:8080","image":"%s"}\n' "${EXPECTED_RELEASE_IMAGE:?}"
SH
  chmod 0755 "$FIXTURE_BIN"/*
}

write_valid_state() {
  local current_image="$REPOSITORY@sha256:$(printf '2%.0s' {1..64})"
  "$REAL_JQ" -n --arg image "$current_image" --arg commit "$(printf '3%.0s' {1..40})" \
    --arg tree "$(printf '4%.0s' {1..40})" --arg migrations "$MIGRATIONS_HASH" \
    '{schema_version:1,active_slot:"blue",active_upstream:"sub2api-blue:8080",blue_image:$image,green_image:$image,worker_image:$image,source_commit:$commit,source_tree:$tree,migrations_hash:$migrations,postgres_id:"postgres-id",redis_id:"redis-id",caddy_id:"caddy-id"}' >"$FIXTURE_STATE"
  chmod 0600 "$FIXTURE_STATE"
  cat >"$FIXTURE_DEPLOY/release.env" <<EOF
SUB2API_BLUE_IMAGE=$current_image
SUB2API_GREEN_IMAGE=$current_image
SUB2API_WORKER_IMAGE=$current_image
SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080
SUB2API_ACTIVE_SLOT=blue
SUB2API_PREVIOUS_SLOT=green
EOF
  chmod 0600 "$FIXTURE_DEPLOY/release.env"
}

cleanup_fixture() { rm -rf -- "$FIXTURE_ROOT"; }

run_update() {
  local release_image="$REPOSITORY:release-$SOURCE_COMMIT-${IMAGE#sha256:}"
  env PATH="$FIXTURE_BIN:$PATH" \
    EXPECTED_IMAGE="$IMAGE" EXPECTED_DIGEST="$DIGEST" EXPECTED_REPOSITORY="$REPOSITORY" \
    EXPECTED_VERSION="$VERSION" EXPECTED_UPSTREAM_COMMIT="$UPSTREAM_COMMIT" \
    EXPECTED_SOURCE_COMMIT="$SOURCE_COMMIT" EXPECTED_SOURCE_TREE="$SOURCE_TREE" \
    EXPECTED_MIGRATIONS_HASH="$MIGRATIONS_HASH" EXPECTED_RELEASE_IMAGE="$release_image" \
    FAKE_DOCKER_LOG="$FIXTURE_DOCKER" FAKE_EXECUTOR_CALL="$FIXTURE_CALL" \
    SUB2API_BLUE_GREEN_EXECUTOR="$FIXTURE_BIN/blue-green-executor" \
    SUB2API_PRODUCTION_ROOT="$FIXTURE_DEPLOY" SUB2API_COMPOSE_FILE="$FIXTURE_DEPLOY/compose.yaml" \
    SUB2API_ENV_FILE="$FIXTURE_DEPLOY/.env" SUB2API_RELEASE_ENV_FILE="$FIXTURE_DEPLOY/release.env" \
    SUB2API_RELEASE_STATE="$FIXTURE_STATE" SUB2API_RELEASE_RECORD_ROOT="$FIXTURE_RECORDS" \
    SUB2API_ADMIN_API_KEY_FILE="$FIXTURE_DEPLOY/secrets/admin-key" \
    SUB2API_GATEWAY_API_KEY_FILE="$FIXTURE_DEPLOY/secrets/gateway-key" \
    SUB2API_BASE_URL=https://sub2api.example.test SUB2API_NETWORK_CURL_IMAGE="$CURL_IMAGE" \
    SUB2API_NETWORK_CURL_IMAGE_ALLOWLIST="$CURL_IMAGE" SUB2API_RELEASE_STAGING_ROOT="$FIXTURE_STAGING" \
    SUB2API_RELEASE_DEADLINE_SECONDS=900 RELEASE_EVENT_LOG="$FIXTURE_TRACE" \
    bash "$SCRIPT" --contract-version 1 --image "$IMAGE" --version "$VERSION" --operation-id "$OPERATION_ID"
}

assert_executor_not_called() { [[ ! -e "$FIXTURE_CALL" ]] || fail 'blue-green executor was called after rejection'; }

test_successful_delegation() {
  new_fixture
  local output archive expected_image
  output=$(run_update)
  [[ "$output" == result=promoted ]] || fail "unexpected terminal output: $output"
  expected_image="$REPOSITORY:release-$SOURCE_COMMIT-${IMAGE#sha256:}"
  grep -F -- "args: <--mode> <production> <--image> <$expected_image>" "$FIXTURE_CALL" >/dev/null || fail 'production image delegation is wrong'
  for expected in \
    "<--source-commit> <$SOURCE_COMMIT>" "<--source-tree> <$SOURCE_TREE>" \
    "<--tested-tree> <$SOURCE_TREE>" "<--migrations-hash> <$MIGRATIONS_HASH>" \
    '<--deadline-epoch> <1786000900>' "DEPLOY_ROOT=$FIXTURE_DEPLOY" \
    "BASE_COMPOSE=$FIXTURE_DEPLOY/compose.yaml" "SECRET_ENV=$FIXTURE_DEPLOY/.env" \
    "RELEASE_ENV=$FIXTURE_DEPLOY/release.env" "RELEASE_STATE=$FIXTURE_STATE" \
    "RELEASE_RECORD_ROOT=$FIXTURE_RECORDS" "BASE_URL=https://sub2api.example.test" \
    "NETWORK_CURL_IMAGE=$CURL_IMAGE" "NETWORK_CURL_IMAGE_ALLOWLIST=$CURL_IMAGE" \
    "RELEASE_STAGING_ROOT=$FIXTURE_STAGING" 'RELEASE_PRELOADED_IMAGE=true' \
    "RELEASE_EVENT_LOG=$FIXTURE_TRACE"; do
    grep -F -- "$expected" "$FIXTURE_CALL" >/dev/null || fail "missing executor contract: $expected"
  done
  archive=$(sed -n 's/.*<--preloaded-archive> <\([^>]*\)>.*/\1/p' "$FIXTURE_CALL")
  [[ "$archive" == "$FIXTURE_STAGING"/* ]] || fail 'preloaded archive path is not canonical'
  grep -F -- "preloaded-archive-sha256=" "$FIXTURE_CALL" >/dev/null || fail 'preloaded archive checksum was not passed'
  archive_sha256=$(sed -n 's/^preloaded-archive-sha256=//p' "$FIXTURE_CALL")
  archive_actual_sha256=$(sed -n 's/^archive-sha256=//p' "$FIXTURE_CALL")
  [[ -n "$archive_sha256" && "$archive_sha256" == "$archive_actual_sha256" ]] || fail 'preloaded archive checksum was not validated'
  grep -F -- "preloaded-image-id=$IMAGE" "$FIXTURE_CALL" >/dev/null || fail 'preloaded image ID was not passed'
  grep -F -- 'archive-mode=600' "$FIXTURE_CALL" >/dev/null || fail 'preloaded archive mode is not 0600'
  [[ ! -e "$archive" ]] || fail 'preloaded archive was not cleaned after success'
  grep -F 'executor event' "$FIXTURE_TRACE" >/dev/null || fail 'executor event trace was not preserved'
  ! grep -F 'admin-secret' "$FIXTURE_CALL" "$FIXTURE_TRACE" >/dev/null || fail 'admin API key leaked'
  ! grep -F 'gateway-secret' "$FIXTURE_CALL" "$FIXTURE_TRACE" >/dev/null || fail 'gateway API key leaked'
  cleanup_fixture
}

test_rejects_repo_digest_failures() {
  local scenario
  for scenario in missing_digest ambiguous_digest unapproved_digest; do
    new_fixture
    if FAKE_IMAGE_SCENARIO=$scenario run_update >/dev/null 2>&1; then fail "$scenario was accepted"; fi
    assert_executor_not_called
    cleanup_fixture
  done
}

test_rejects_invalid_provenance() {
  local scenario
  for scenario in invalid_labels tree_mismatch; do
    new_fixture
    if FAKE_IMAGE_SCENARIO=$scenario run_update >/dev/null 2>&1; then fail "$scenario was accepted"; fi
    assert_executor_not_called
    cleanup_fixture
  done
}

test_rejects_missing_or_inconsistent_blue_green_state() {
  new_fixture
  rm "$FIXTURE_STATE"
  if run_update >/dev/null 2>&1; then fail 'missing release state was accepted'; fi
  assert_executor_not_called
  cleanup_fixture

  new_fixture
  "$REAL_JQ" '.active_upstream="sub2api-green:8080"' "$FIXTURE_STATE" >"$FIXTURE_STATE.tmp"
  mv "$FIXTURE_STATE.tmp" "$FIXTURE_STATE"; chmod 0600 "$FIXTURE_STATE"
  if run_update >/dev/null 2>&1; then fail 'inconsistent active slot/upstream was accepted'; fi
  assert_executor_not_called
  cleanup_fixture

  new_fixture
  sed -i.bak 's/SUB2API_ACTIVE_SLOT=blue/SUB2API_ACTIVE_SLOT=green/' "$FIXTURE_DEPLOY/release.env"
  rm "$FIXTURE_DEPLOY/release.env.bak"
  if run_update >/dev/null 2>&1; then fail 'release.env inconsistent with state was accepted'; fi
  assert_executor_not_called
  cleanup_fixture
}

test_executor_failure_is_fail_closed() {
  new_fixture
  local output status=0 archive
  output=$(FAKE_EXECUTOR_RESULT=failure run_update 2>&1) || status=$?
  [[ "$status" -eq 23 ]] || fail "executor failure status was not preserved: $status"
  [[ "$output" != *'result=promoted'* ]] || fail 'executor failure was reported as promoted'
  grep -F 'executor event' "$FIXTURE_TRACE" >/dev/null || fail 'failure event trace was not preserved'
  archive=$(sed -n 's/^preloaded-archive=//p' "$FIXTURE_CALL")
  [[ -n "$archive" && ! -e "$archive" ]] || fail 'preloaded archive was not cleaned after executor failure'
  cleanup_fixture
}

[[ -x "$SCRIPT" ]] || fail 'host updater is absent'
test_successful_delegation
test_rejects_repo_digest_failures
test_rejects_invalid_provenance
test_rejects_missing_or_inconsistent_blue_green_state
test_executor_failure_is_fail_closed
printf 'PASS: Sub2API updater delegates strictly to blue-green production\n'
