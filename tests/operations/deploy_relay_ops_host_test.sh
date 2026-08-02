#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
EXECUTOR=${EXECUTOR_UNDER_TEST:-"$ROOT/ops/deploy-relay-ops-host.sh"}
FIXTURE=$(mktemp -d "${TMPDIR:-/tmp}/relay-ops-host.XXXXXX")
FIXTURE=$(cd "$FIXTURE" && pwd -P)
trap 'rm -rf -- "$FIXTURE"' EXIT
fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
[[ -f "$EXECUTOR" ]] || fail "executor does not exist: $EXECUTOR"

IMAGE="example.invalid/relay-ops@sha256:$(printf 'a%.0s' {1..64})"
PREVIOUS="example.invalid/relay-ops@sha256:$(printf 'b%.0s' {1..64})"
setup_case() {
  CASE_DIR="$FIXTURE/$1"; rm -rf -- "$CASE_DIR"; mkdir -p "$CASE_DIR/bin" "$CASE_DIR/deploy" "$CASE_DIR/records" "$CASE_DIR/staging"
  printf 'secret=value\n' >"$CASE_DIR/secret.env"; chmod 0600 "$CASE_DIR/secret.env"
  printf 'services: {}\n' >"$CASE_DIR/compose.yaml"
  : >"$CASE_DIR/events.log"
  cat >"$CASE_DIR/bin/id" <<'SH'
#!/usr/bin/env bash
[[ "$1" == -u ]] && printf '0\n' || /usr/bin/id "$@"
SH
  cat >"$CASE_DIR/bin/uname" <<'SH'
#!/usr/bin/env bash
printf 'Linux\n'
SH
  cat >"$CASE_DIR/bin/stat" <<'SH'
#!/usr/bin/env bash
if [[ "$*" == *"%u"* ]]; then printf '0\n'; else /usr/bin/stat "$@"; fi
SH
  cat >"$CASE_DIR/bin/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"${FAKE_EVENT_LOG:?}"
case "$*" in
  'context show') printf 'default\n' ;;
  *' config --quiet') ;;
  *' ps -q postgres') [[ "${FAKE_SCENARIO:-}" == duplicate_postgres ]] && { printf 'postgres-id\npostgres-duplicate-id\n'; exit 0; }; printf 'postgres-id\n' ;;
  *' ps -q redis') printf 'redis-id\n' ;;
  *' ps -q caddy') printf 'caddy-id\n' ;;
  *' ps -q sub2api-blue') printf 'blue-id\n' ;;
  *' ps -q sub2api-green') printf 'green-id\n' ;;
  *' ps -q sub2api-worker') printf 'worker-id\n' ;;
  *' ps -q relay-ops') printf 'relay-id\n' ;;
  *'Config.Image'*) [[ -e "${FAKE_EVENT_LOG}.rollback" ]] && printf '%s\n' "${FAKE_RELAY_IMAGE:?}" || { [[ -e "${FAKE_EVENT_LOG}.new" ]] && printf '%s\n' "${FAKE_REQUESTED_IMAGE:?}" || printf '%s\n' "${FAKE_RELAY_IMAGE:?}"; } ;;
  *'inspect relay-id --format {{.Image}}'*) [[ -e "${FAKE_EVENT_LOG}.rollback" ]] && printf '%s\n' "${FAKE_RELAY_IMAGE_ID:?}" || { [[ -e "${FAKE_EVENT_LOG}.new" ]] && printf '%s\n' "${FAKE_REQUESTED_IMAGE_ID:?}" || printf '%s\n' "${FAKE_RELAY_IMAGE_ID:?}"; } ;;
  *'image inspect '*"{{.Id}}"*) printf '%s\n' "${FAKE_REQUESTED_IMAGE_ID:?}" ;;
  *'image inspect '*"{{json .}}"*)
    if [[ "${FAKE_SCENARIO:-}" == provenance_mismatch ]]; then
      printf '{"Id":"%s","Config":{"Labels":{"com.xingqiao.relay-ops.qualified":"true","com.xingqiao.relay-ops.source.commit":"wrong","com.xingqiao.relay-ops.source.tree":"%s","com.xingqiao.relay-ops.tested.tree":"%s","com.xingqiao.relay-ops.migrations.sha256":"%s"}}}\n' "${FAKE_REQUESTED_IMAGE_ID:?}" "${FAKE_SOURCE_TREE:?}" "${FAKE_TESTED_TREE:?}" "${FAKE_MIGRATIONS_HASH:?}"
    else
      printf '{"Id":"%s","Config":{"Labels":{"com.xingqiao.relay-ops.qualified":"true","com.xingqiao.relay-ops.source.commit":"%s","com.xingqiao.relay-ops.source.tree":"%s","com.xingqiao.relay-ops.tested.tree":"%s","com.xingqiao.relay-ops.migrations.sha256":"%s"}}}\n' "${FAKE_REQUESTED_IMAGE_ID:?}" "${FAKE_SOURCE_COMMIT:?}" "${FAKE_SOURCE_TREE:?}" "${FAKE_TESTED_TREE:?}" "${FAKE_MIGRATIONS_HASH:?}"
    fi
    ;;
  *' load --input '*) ;;
  *'State.Health.Status'*)
    if [[ "${FAKE_SCENARIO:-}" == health_starting && ! -e "${FAKE_EVENT_LOG}.health_once" ]]; then
      : >"${FAKE_EVENT_LOG}.health_once"; printf 'starting\n'
    elif [[ "${FAKE_SCENARIO:-}" == health_failure && -e "${FAKE_EVENT_LOG}.new" && ! -e "${FAKE_EVENT_LOG}.rollback" ]]; then
      printf 'unhealthy\n'
    else
      printf 'healthy\n'
    fi
    ;;
  *' inspect postgres-id --format {{.Id}}') printf 'postgres-id\n' ;;
  *' inspect redis-id --format {{.Id}}') printf 'redis-id\n' ;;
  *' inspect caddy-id --format {{.Id}}') printf 'caddy-id\n' ;;
  *' inspect blue-id --format {{.Id}}') printf 'blue-id\n' ;;
  *' inspect green-id --format {{.Id}}') printf 'green-id\n' ;;
  *' inspect worker-id --format {{.Id}}') printf 'worker-id\n' ;;
  *' pull relay-ops') ;;
  *' up -d --no-deps '*'--force-recreate relay-ops') [[ -e "${FAKE_EVENT_LOG}.new" ]] && : >"${FAKE_EVENT_LOG}.rollback" || : >"${FAKE_EVENT_LOG}.new" ;;
  *'exec caddy-id wget -qO- http://relay-ops:8100/healthz') [[ "${FAKE_SCENARIO:-}" == bad_health ]] && { printf '{"status":"wrong"}\n'; exit 0; }; printf '{"status":"ok"}\n' ;;
  *'exec caddy-id wget -qO- http://relay-ops:8100/readyz') [[ "${FAKE_SCENARIO:-}" == bad_ready ]] && { printf '{"status":"wrong"}\n'; exit 0; }; printf '{"status":"ready"}\n' ;;
  *) exit 0 ;;
esac
SH
  chmod +x "$CASE_DIR/bin/id" "$CASE_DIR/bin/uname" "$CASE_DIR/bin/stat" "$CASE_DIR/bin/docker"
}
run_executor() {
  env PATH="$CASE_DIR/bin:$PATH" FAKE_EVENT_LOG="$CASE_DIR/events.log" FAKE_RELAY_IMAGE="$PREVIOUS" FAKE_REQUESTED_IMAGE="$IMAGE" FAKE_RELAY_IMAGE_ID="sha256:$(printf 'b%.0s' {1..64})" FAKE_REQUESTED_IMAGE_ID="sha256:$(printf 'a%.0s' {1..64})" FAKE_SOURCE_COMMIT="$(printf 'c%.0s' {1..40})" FAKE_SOURCE_TREE="$(printf 'd%.0s' {1..40})" FAKE_TESTED_TREE="$(printf 'd%.0s' {1..40})" FAKE_MIGRATIONS_HASH="$(printf 'e%.0s' {1..64})" DEPLOY_ROOT="$CASE_DIR/deploy" BASE_COMPOSE="$CASE_DIR/compose.yaml" SECRET_ENV="$CASE_DIR/secret.env" RELAY_OPS_HEALTH_TIMEOUT_SECONDS=2 RELAY_OPS_HEALTH_POLL_SECONDS=1 \
    RELEASE_STATE="$CASE_DIR/records/release-state.json" RELEASE_RECORD_ROOT="$CASE_DIR/records" RELEASE_IMAGE="$IMAGE" \
    DOCKER_BIN="$CASE_DIR/bin/docker" "$@" bash "$EXECUTOR" --mode production --image "$IMAGE" \
    --source-commit "$(printf 'c%.0s' {1..40})" --source-tree "$(printf 'd%.0s' {1..40})" --tested-tree "$(printf 'd%.0s' {1..40})" \
    --migrations-hash "$(printf 'e%.0s' {1..64})" --deadline-epoch "$(( $(date +%s) + 300 ))"
}

run_preloaded_executor() {
  local scenario=${1:-} commit image_id archive_sha image
  commit=$(printf 'c%.0s' {1..40})
  image="example.invalid/relay-ops:release-$commit"
  image_id="sha256:$(printf 'a%.0s' {1..64})"
  printf 'preloaded-image\n' >"$CASE_DIR/staging/image.tar"
  chmod 0600 "$CASE_DIR/staging/image.tar"
  archive_sha=$(shasum -a 256 "$CASE_DIR/staging/image.tar" | awk '{print $1}')
  env PATH="$CASE_DIR/bin:$PATH" FAKE_SCENARIO="$scenario" FAKE_EVENT_LOG="$CASE_DIR/events.log" FAKE_RELAY_IMAGE="$PREVIOUS" FAKE_REQUESTED_IMAGE="$image" FAKE_RELAY_IMAGE_ID="sha256:$(printf 'b%.0s' {1..64})" FAKE_REQUESTED_IMAGE_ID="$image_id" FAKE_SOURCE_COMMIT="$commit" FAKE_SOURCE_TREE="$(printf 'd%.0s' {1..40})" FAKE_TESTED_TREE="$(printf 'd%.0s' {1..40})" FAKE_MIGRATIONS_HASH="$(printf 'e%.0s' {1..64})" DEPLOY_ROOT="$CASE_DIR/deploy" BASE_COMPOSE="$CASE_DIR/compose.yaml" SECRET_ENV="$CASE_DIR/secret.env" RELEASE_STATE="$CASE_DIR/records/release-state.json" RELEASE_RECORD_ROOT="$CASE_DIR/records" RELEASE_PRELOADED_IMAGE=true RELEASE_STAGING_ROOT="$CASE_DIR/staging" RELAY_OPS_HEALTH_TIMEOUT_SECONDS=2 RELAY_OPS_HEALTH_POLL_SECONDS=1 DOCKER_BIN="$CASE_DIR/bin/docker" bash "$EXECUTOR" --mode production --image "$image" --preloaded-archive "$CASE_DIR/staging/image.tar" --preloaded-archive-sha256 "$archive_sha" --preloaded-image-id "$image_id" --source-commit "$commit" --source-tree "$(printf 'd%.0s' {1..40})" --tested-tree "$(printf 'd%.0s' {1..40})" --migrations-hash "$(printf 'e%.0s' {1..64})" --deadline-epoch "$(( $(date +%s) + 300 ))"
}

test_success_only_recreates_relay_ops() {
  setup_case success
  run_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail "executor failed: $(cat "$CASE_DIR/stderr")"
  grep -F -- 'pull relay-ops' "$CASE_DIR/events.log" >/dev/null || fail 'relay-ops pull missing'
  grep -F -- 'up -d --no-deps --force-recreate relay-ops' "$CASE_DIR/events.log" >/dev/null || fail 'relay-ops-only recreate missing'
  ! grep -E 'up .*postgres|up .*redis|up .*caddy|up .*sub2api-' "$CASE_DIR/events.log" >/dev/null || fail 'shared service recreation detected'
  [[ -f "$CASE_DIR/records/release-state.json" ]] || fail 'release state missing'
  [[ "$(stat -f '%Lp' "$CASE_DIR/records/release-state.json" 2>/dev/null || stat -c '%a' "$CASE_DIR/records/release-state.json")" == 600 ]] || fail 'release state is not 0600'
}

test_preloaded_image_skips_pull_and_records_immutable_image_id() {
  setup_case preloaded
  run_preloaded_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail "preloaded executor failed: $(cat "$CASE_DIR/stderr")"
  grep -F -- 'load --input' "$CASE_DIR/events.log" >/dev/null || fail 'preloaded archive was not loaded'
  ! grep -F -- 'pull relay-ops' "$CASE_DIR/events.log" >/dev/null || fail 'preloaded executor unexpectedly pulled relay-ops'
  grep -F -- 'up -d --no-deps --pull never --force-recreate relay-ops' "$CASE_DIR/events.log" >/dev/null || fail 'preloaded relay-only recreate missing'
  ruby -rjson -e 'v=JSON.parse(File.binread(ARGV[0])); abort unless v["current_image_id"] == ARGV[1]' "$CASE_DIR/records/release-state.json" "sha256:$(printf 'a%.0s' {1..64})" || fail 'preloaded image ID was not persisted'
}

test_preloaded_image_works_without_ruby_runtime() {
  setup_case preloaded_without_ruby
  cat >"$CASE_DIR/bin/ruby" <<'SH'
#!/usr/bin/env bash
printf 'ruby runtime intentionally unavailable\n' >&2
exit 127
SH
  chmod +x "$CASE_DIR/bin/ruby"
  run_preloaded_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail "python fallback failed without ruby: $(cat "$CASE_DIR/stderr")"
  grep -F -- 'load --input' "$CASE_DIR/events.log" >/dev/null || fail 'python fallback did not load preloaded archive'
  python3 - "$CASE_DIR/records/release-state.json" "sha256:$(printf 'a%.0s' {1..64})" <<'PY' || fail 'python fallback did not persist the complete release state'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    value = json.load(handle)
expected = {
    "schema_version", "service", "current_image", "current_image_id",
    "previous_image", "previous_image_id", "result", "source_commit",
    "source_tree", "tested_tree", "migrations_hash", "shared_container_ids",
}
if set(value) != expected or value["schema_version"] != 1 or value["service"] != "relay-ops":
    raise SystemExit(1)
if value["result"] != "succeeded" or value["current_image_id"] != sys.argv[2]:
    raise SystemExit(1)
if set(value["shared_container_ids"]) != {"postgres", "redis", "caddy", "sub2api-blue", "sub2api-green", "sub2api-worker"}:
    raise SystemExit(1)
if any(not value["shared_container_ids"][key].endswith("-id") for key in value["shared_container_ids"]):
    raise SystemExit(1)
PY
  [[ "$(stat -f '%Lp' "$CASE_DIR/records/release-state.json" 2>/dev/null || stat -c '%a' "$CASE_DIR/records/release-state.json")" == 600 ]] || fail 'python fallback release state is not 0600'
}

test_preloaded_provenance_mismatch_prevents_recreate_without_ruby_runtime() {
  setup_case preloaded_bad_provenance
  cat >"$CASE_DIR/bin/ruby" <<'SH'
#!/usr/bin/env bash
printf 'ruby runtime intentionally unavailable\n' >&2
exit 127
SH
  chmod +x "$CASE_DIR/bin/ruby"
  if run_preloaded_executor provenance_mismatch >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then fail 'invalid provenance unexpectedly succeeded'; fi
  grep -F -- 'load --input' "$CASE_DIR/events.log" >/dev/null || fail 'candidate archive was not inspected'
  ! grep -F -- 'up -d --no-deps --pull never --force-recreate relay-ops' "$CASE_DIR/events.log" >/dev/null || fail 'invalid provenance recreated relay-ops'
  [[ ! -e "$CASE_DIR/records/release-state.json" ]] || fail 'invalid provenance wrote release state'
}

test_failed_checks_restore_previous_digest() {
  setup_case rollback
  if run_executor FAKE_SCENARIO=health_failure >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then fail 'failed post-check unexpectedly succeeded'; fi
  grep -F '"result":"failed_rolled_back"' "$CASE_DIR/stdout" >/dev/null || fail 'rollback result missing'
  grep -F '"rollback_proven":true' "$CASE_DIR/stdout" >/dev/null || fail 'rollback proof missing'
  ruby -rjson -e 'v=JSON.parse(File.binread(ARGV[0])); abort unless v["result"]=="rolled_back" && v["current_image"]==ARGV[1]' "$CASE_DIR/records/release-state.json" "$PREVIOUS" || fail 'rollback state did not persist previous digest'
}

test_rejects_ambiguous_required_service() {
  setup_case duplicate
  if run_executor FAKE_SCENARIO=duplicate_postgres >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then fail 'ambiguous postgres unexpectedly succeeded'; fi
  ! grep -F -- 'pull relay-ops' "$CASE_DIR/events.log" >/dev/null || fail 'ambiguous preflight pulled relay-ops'
}

test_rejects_nonready_internal_endpoint() {
  setup_case bad_ready
  if run_executor FAKE_SCENARIO=bad_ready >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then fail 'bad readiness unexpectedly succeeded'; fi
  grep -F -- 'up -d --no-deps --force-recreate relay-ops' "$CASE_DIR/events.log" >/dev/null || fail 'candidate release was not attempted'
  grep -F -- 'pull relay-ops' "$CASE_DIR/events.log" >/dev/null || fail 'rollback pull missing'
  ! grep -F -- '"rollback_proven":true' "$CASE_DIR/stdout" >/dev/null || fail 'unverified rollback was reported as proven'
}

test_waits_for_health_transition() {
  setup_case health_starting
  run_executor FAKE_SCENARIO=health_starting >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail "starting health status was not polled: $(cat "$CASE_DIR/stderr")"
}

test_success_only_recreates_relay_ops
test_preloaded_image_skips_pull_and_records_immutable_image_id
test_preloaded_image_works_without_ruby_runtime
test_preloaded_provenance_mismatch_prevents_recreate_without_ruby_runtime
test_failed_checks_restore_previous_digest
test_rejects_ambiguous_required_service
test_rejects_nonready_internal_endpoint
test_waits_for_health_transition
printf 'PASS: relay-ops host executor\n'
