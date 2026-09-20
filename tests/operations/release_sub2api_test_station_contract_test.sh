#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
SCRIPT=${SCRIPT_UNDER_TEST:-$ROOT/ops/release-sub2api-test-station.sh}
TMP_BASE=$(cd "${TMPDIR:-/tmp}" && pwd -P)
FIXTURE=$(mktemp -d "$TMP_BASE/test-station-release.XXXXXX")
trap 'rm -rf -- "$FIXTURE"' EXIT
fail(){ printf 'FAIL: %s\n' "$1" >&2; exit 1; }
[[ -x "$SCRIPT" ]] || fail 'orchestrator missing'
COMMIT=$(printf 'a%.0s' {1..40})
TREE=$(printf 'b%.0s' {1..40})
IMAGE_ID="sha256:$(printf 'c%.0s' {1..64})"

setup(){
  CASE=$FIXTURE/$1
  WORKTREE=$CASE/worktree
  BIN=$CASE/bin
  EVENT_LOG=$CASE/events.log
  mkdir -p "$WORKTREE/upstream/sub2api/backend/migrations" "$WORKTREE/infra/independent-test-station" "$WORKTREE/ops" "$BIN"
  printf 'select 1;\n' >"$WORKTREE/upstream/sub2api/backend/migrations/001.sql"
  printf 'FROM scratch\n' >"$WORKTREE/upstream/sub2api/Dockerfile"
  printf 'name: sub2api-test-station\nnetworks: {test: {name: sub2api-test-station-network}}\n' >"$WORKTREE/infra/independent-test-station/compose.yaml"
  printf ':80 { respond "ok" 200 }\n' >"$WORKTREE/infra/independent-test-station/Caddyfile"
  printf '#!/usr/bin/env bash\n' >"$WORKTREE/ops/deploy-sub2api-test-station-host.sh"
  printf '#!/usr/bin/env bash\n' >"$WORKTREE/ops/backup-sub2api-test-station-host.sh"
  chmod +x "$WORKTREE/ops/"*.sh
  : >"$EVENT_LOG"

  cat >"$BIN/git" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'git %s\n' "$*" >>"${EVENT_LOG:?}"
[[ "${1:-}" == -C && "${2:-}" == "${WORKTREE:?}" ]] || exit 61
shift 2
case "$*" in
  'branch --show-current') [[ "${FAKE_MODE:-ok}" == non-main ]] && printf 'feature\n' || printf 'main\n' ;;
  'status --porcelain') [[ "${FAKE_MODE:-ok}" == dirty ]] && printf ' M file\n' || true ;;
  'fetch origin main') [[ "${FAKE_MODE:-ok}" != fetch-fail ]] ;;
  'rev-parse HEAD') printf '%s\n' "${COMMIT:?}" ;;
  'rev-parse origin/main') [[ "${FAKE_MODE:-ok}" == origin-drift ]] && printf '%040d\n' 9 || printf '%s\n' "${COMMIT:?}" ;;
  "rev-parse HEAD^{tree}") printf '%s\n' "${TREE:?}" ;;
  *) exit 62 ;;
esac
SH
  chmod +x "$BIN/git"

  cat >"$BIN/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"${EVENT_LOG:?}"
case "${1:-}" in
  buildx) [[ "$*" == *'build --platform linux/amd64 --load -t sub2api-test-station-runtime:'* ]] || exit 63 ;;
  save)
    [[ "${2:-}" == -o ]] || exit 64
    printf 'image-archive' >"${3:?}"
    ;;
  image)
    [[ "${2:-}" == inspect ]] || exit 65
    if [[ "${FAKE_MODE:-ok}" == malformed-image ]]; then printf 'not-an-image-id\n'; else printf '%s\n' "${IMAGE_ID:?}"; fi
    ;;
  *) exit 66 ;;
esac
SH
  chmod +x "$BIN/docker"

  cat >"$BIN/ssh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'ssh %s\n' "$*" >>"${EVENT_LOG:?}"
if [[ "$*" == *'mktemp -d /var/tmp/sub2api-test-station-release.XXXXXX'* ]]; then
  printf '/var/tmp/sub2api-test-station-release.TEST\n'
elif [[ "$*" == *'sudo -n bash -s --'* ]]; then
  cat >/dev/null
else
  :
fi
SH
  chmod +x "$BIN/ssh"

  cat >"$BIN/scp" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'scp %s\n' "$*" >>"${EVENT_LOG:?}"
SH
  chmod +x "$BIN/scp"
}

run_release(){
  env PATH="$BIN:$PATH" EVENT_LOG="$EVENT_LOG" WORKTREE="$WORKTREE" COMMIT="$COMMIT" TREE="$TREE" IMAGE_ID="$IMAGE_ID" \
    FAKE_MODE="${FAKE_MODE:-ok}" RELEASE_WORKTREE="$WORKTREE" TEST_STATION_SSH_TARGET="${TEST_STATION_SSH_TARGET:-sub2api-test-station}" \
    bash "$SCRIPT"
}

assert_no_ssh(){ ! grep -q '^ssh ' "$EVENT_LOG" || fail 'failure contacted SSH'; }
assert_no_docker(){ ! grep -q '^docker ' "$EVENT_LOG" || fail 'failure invoked Docker'; }

test_success_transfers_metadata_and_backup_helper(){
  setup success
  output=$(run_release) || fail 'success case failed'
  [[ "$output" == "test_station_release status=succeeded source_commit=$COMMIT source_tree=$TREE" ]] || fail 'success output mismatch'
  grep -F 'docker image inspect --format {{.Id}}' "$EVENT_LOG" >/dev/null || fail 'image ID not inspected'
  scp_line=$(grep '^scp ' "$EVENT_LOG")
  [[ "$scp_line" == *backup-sub2api-test-station-host.sh* ]] || fail 'backup helper not transferred'
  final_ssh=$(grep '^ssh ' "$EVENT_LOG" | grep 'sudo -n bash -s --')
  [[ "$final_ssh" == *"--image-id '$IMAGE_ID'"* ]] || fail 'image ID not passed'
  [[ "$final_ssh" == *"--backup-script '/var/tmp/sub2api-test-station-release.TEST/backup-sub2api-test-station-host.sh'"* ]] || fail 'backup script path not passed'
  [[ "$final_ssh" =~ --migration-set-sha256\ \'[a-f0-9]{64}\' ]] || fail 'migration hash not passed'
}

test_source_failures_stop_before_build(){
  local mode
  for mode in non-main dirty fetch-fail origin-drift; do
    setup "$mode"
    if FAKE_MODE=$mode run_release >/dev/null 2>&1; then fail "$mode returned success"; fi
    assert_no_docker
    assert_no_ssh
  done
}

test_unsafe_target_and_missing_migrations_stop_early(){
  setup unsafe-target
  if TEST_STATION_SSH_TARGET=other run_release >/dev/null 2>&1; then fail 'unsafe target returned success'; fi
  assert_no_docker; assert_no_ssh

  setup missing-migrations
  rm -rf "$WORKTREE/upstream/sub2api/backend/migrations"
  if run_release >/dev/null 2>&1; then fail 'missing migrations returned success'; fi
  assert_no_docker; assert_no_ssh
}

test_malformed_image_id_stops_before_ssh(){
  setup malformed-image
  if FAKE_MODE=malformed-image run_release >/dev/null 2>&1; then fail 'malformed image ID returned success'; fi
  assert_no_ssh
}

test_invalid_migration_content_stops_before_build(){
  setup invalid-migration
  printf '\377' >"$WORKTREE/upstream/sub2api/backend/migrations/001.sql"
  if run_release >/dev/null 2>&1; then fail 'invalid migration content returned success'; fi
  assert_no_docker
  assert_no_ssh
}

bash -n "$SCRIPT"
test_success_transfers_metadata_and_backup_helper
test_source_failures_stop_before_build
test_unsafe_target_and_missing_migrations_stop_early
test_malformed_image_id_stops_before_ssh
test_invalid_migration_content_stops_before_build
printf 'PASS: independent test station release contract\n'
