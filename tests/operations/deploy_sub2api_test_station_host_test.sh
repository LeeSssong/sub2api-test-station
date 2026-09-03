#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
EXECUTOR=${EXECUTOR_UNDER_TEST:-$ROOT/ops/deploy-sub2api-test-station-host.sh}
FIXTURE=$(mktemp -d "${TMPDIR:-/tmp}/test-station-host.XXXXXX")
trap 'rm -rf -- "$FIXTURE"' EXIT
fail(){ printf 'FAIL: %s\n' "$1" >&2; exit 1; }
[[ -f "$EXECUTOR" ]] || fail 'executor missing'
setup(){
  CASE=$FIXTURE/$1; mkdir -p "$CASE/stage" "$CASE/releases" "$CASE/bin"
  printf 'name: sub2api-test-station\nnetworks: {test: {name: sub2api-test-station-network}}\nservices: {}\n' >"$CASE/stage/compose.yaml"
  printf ':80 { respond "ok" 200 }\n' >"$CASE/stage/Caddyfile"
  printf 'CLONE_APP_IMAGE=old\nCLONE_CADDY_IMAGE=caddy\n' >"$CASE/stage/.env"
  printf 'image\n' >"$CASE/stage/image.tar"
  sha256sum "$CASE/stage/image.tar" | awk '{print $1}' >"$CASE/stage/image.sha256"
  cat >"$CASE/bin/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"${EVENT_LOG:?}"
case "$*" in
  'load --input '* ) printf 'Loaded image: sub2api-test-station-runtime:new\n' ;;
  *' config --quiet'*) ;;
  *' up -d --remove-orphans'*) ;;
  *' ps '* ) printf 'test-station-api healthy\ntest-station-worker healthy\ntest-station-detector healthy\ntest-station-caddy healthy\n';;
  *) ;;
esac
SH
  chmod +x "$CASE/bin/docker"
}
run_exec(){
  env TEST_STATION_TEST_MODE=true PATH="$CASE/bin:$PATH" EVENT_LOG="$CASE/events.log" DOCKER_BIN="$CASE/bin/docker" \
    DEPLOY_ROOT="$CASE/releases" RELEASE_STATE="$CASE/releases/release-state.json" \
    bash "$EXECUTOR" --staging-root "$CASE/stage" --image-archive "$CASE/stage/image.tar" \
    --image-sha256 "$(cat "$CASE/stage/image.sha256")" --compose "$CASE/stage/compose.yaml" \
    --caddy "$CASE/stage/Caddyfile" --env-file "$CASE/stage/.env" \
    --source-commit "$(printf 'a%.0s' {1..40})" --source-tree "$(printf 'b%.0s' {1..40})" \
    --deploy-root "$CASE/releases"
}
test_success_scopes_compose(){
  setup success
  run_exec || fail 'success case failed'
  grep -F -- '--project-name sub2api-test-station' "$CASE/events.log" >/dev/null || fail 'wrong project'
  grep -F -- 'test-station-api' "$CASE/events.log" >/dev/null || fail 'services not checked'
  [[ -f "$CASE/releases/release-state.json" ]] || fail 'state missing'
}
test_rejects_wrong_project_path(){
  setup reject
  if env TEST_STATION_TEST_MODE=true PATH="$CASE/bin:$PATH" DOCKER_BIN="$CASE/bin/docker" DEPLOY_ROOT="$CASE/releases" \
    bash "$EXECUTOR" --staging-root "$CASE/stage" --image-archive "$CASE/stage/image.tar" \
    --image-sha256 "$(cat "$CASE/stage/image.sha256")" --compose "$CASE/stage/compose.yaml" \
    --caddy "$CASE/stage/Caddyfile" --env-file "$CASE/stage/.env" \
    --source-commit "$(printf 'a%.0s' {1..40})" --source-tree "$(printf 'b%.0s' {1..40})" \
    --deploy-root /opt/other; then fail 'unsafe deploy root accepted'; fi
}
test_rejects_bad_checksum(){
  setup checksum
  if env TEST_STATION_TEST_MODE=true PATH="$CASE/bin:$PATH" DOCKER_BIN="$CASE/bin/docker" DEPLOY_ROOT="$CASE/releases" \
    bash "$EXECUTOR" --staging-root "$CASE/stage" --image-archive "$CASE/stage/image.tar" \
    --image-sha256 "$(printf 'c%.0s' {1..64})" --compose "$CASE/stage/compose.yaml" \
    --caddy "$CASE/stage/Caddyfile" --env-file "$CASE/stage/.env" \
    --source-commit "$(printf 'a%.0s' {1..40})" --source-tree "$(printf 'b%.0s' {1..40})" \
    --deploy-root "$CASE/releases"; then fail 'bad checksum accepted'; fi
}
test_success_scopes_compose; test_rejects_wrong_project_path; test_rejects_bad_checksum
printf 'PASS: independent test station host executor\n'
