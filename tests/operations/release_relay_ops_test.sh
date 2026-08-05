#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
CONTROLLER=${CONTROLLER_UNDER_TEST:-"$ROOT/ops/release-relay-ops.sh"}
WRITER=${WRITER_UNDER_TEST:-"$ROOT/ops/write-relay-ops-test-evidence.sh"}
FIXTURE=$(mktemp -d "${TMPDIR:-/tmp}/relay-ops-release-controller.XXXXXX")
FIXTURE=$(cd "$FIXTURE" && pwd -P)
trap 'rm -rf -- "$FIXTURE"' EXIT

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
[[ -f "$CONTROLLER" ]] || fail "controller does not exist: $CONTROLLER"
[[ -f "$WRITER" ]] || fail "evidence writer does not exist: $WRITER"

SHA256=$(printf 'a%.0s' {1..64})
setup_case() {
  CASE_DIR="$FIXTURE/$1"
  rm -rf -- "$CASE_DIR"
  mkdir -p "$CASE_DIR/bin" "$CASE_DIR/repo/infra" "$CASE_DIR/repo/relay-ops-service/internal/store/migrations"
  : >"$CASE_DIR/docker.log"; : >"$CASE_DIR/ssh.log"
  printf 'FROM scratch\n' >"$CASE_DIR/repo/infra/Dockerfile.relay-ops"
  printf 'CREATE TABLE example ();\n' >"$CASE_DIR/repo/relay-ops-service/internal/store/migrations/001_init.sql"
  printf 'private key\n' >"$CASE_DIR/id_ed25519"
  printf 'example.invalid ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest\n' >"$CASE_DIR/known_hosts"
  chmod 0600 "$CASE_DIR/id_ed25519" "$CASE_DIR/known_hosts"
  git -C "$CASE_DIR/repo" init -q
  git -C "$CASE_DIR/repo" config user.name Test
  git -C "$CASE_DIR/repo" config user.email test@example.invalid
  git -C "$CASE_DIR/repo" add .
  git -C "$CASE_DIR/repo" commit -qm initial
  cat >"$CASE_DIR/bin/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"${FAKE_DOCKER_LOG:?}"
case "$1 ${2:-} ${3:-}" in
  'buildx build '*)
    if [[ "${FAKE_TRANSPORT:-registry}" == preloaded ]]; then
      printf 'sha256:%s\n' "${FAKE_IMAGE_ID:?}" >"${FAKE_IMAGE_ID_FILE:?}"
    fi
    exit 0 ;;
  'buildx imagetools inspect') printf 'sha256:%s\n' "${FAKE_DIGEST:?}"; exit 0 ;;
  'image inspect '*) printf '%s\n' "${FAKE_IMAGE_ID:?}"; exit 0 ;;
  'image save '*) : >"${FAKE_ARCHIVE:?}"; exit 0 ;;
esac
exit 64
SH
  cat >"$CASE_DIR/bin/ssh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'ssh %s\n' "$*" >>"${FAKE_SSH_LOG:?}"
if [[ -n "${FAKE_SSH_SLEEP:-}" ]]; then sleep "$FAKE_SSH_SLEEP"; fi
if [[ "$*" == *' mktemp -p /tmp '* ]]; then
  printf '/tmp/.relay-ops-%s.Abc123\n' "${FAKE_SOURCE_COMMIT:?}"
  exit 0
fi
printf '{"schema_version":1,"result":"succeeded","rollback_proven":false}\n'
SH
  cat >"$CASE_DIR/bin/scp" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'scp %s\n' "$*" >>"${FAKE_SSH_LOG:?}"
exit 0
SH
  chmod +x "$CASE_DIR/bin/docker" "$CASE_DIR/bin/ssh" "$CASE_DIR/bin/scp"
}
write_evidence() {
  EVIDENCE="$CASE_DIR/evidence.json"
  (cd "$CASE_DIR/repo" && bash "$WRITER" --output "$EVIDENCE" --command 'bash tests/operations/release_relay_ops_test.sh')
}
run_controller() {
  env PATH="$CASE_DIR/bin:$PATH" FAKE_DOCKER_LOG="$CASE_DIR/docker.log" FAKE_SSH_LOG="$CASE_DIR/ssh.log" FAKE_DIGEST="$SHA256" FAKE_SOURCE_COMMIT="$(git -C "$CASE_DIR/repo" rev-parse HEAD)" FAKE_IMAGE_ID="sha256:$(printf 'c%.0s' {1..64})" FAKE_IMAGE_ID_FILE="$CASE_DIR/image-id" FAKE_ARCHIVE="$CASE_DIR/image.tar" \
    RELEASE_WORKTREE="$CASE_DIR/repo" RELEASE_BUILD_CONTEXT="$CASE_DIR/repo" RELAY_OPS_IMAGE_REPOSITORY='example.invalid/xingqiao-relay-ops' \
    RELEASE_SSH_BIN="$CASE_DIR/bin/ssh" RELEASE_SCP_BIN="$CASE_DIR/bin/scp" RELEASE_SSH_TARGET='release@example.invalid' RELEASE_SSH_KEY="$CASE_DIR/id_ed25519" \
    RELEASE_SSH_KNOWN_HOSTS="$CASE_DIR/known_hosts" RELEASE_SSH_PORT=22 "$@" bash "$CONTROLLER" --mode production --evidence "$EVIDENCE"
}
expect_failure_before_transport() {
  local label=$1
  shift
  if run_controller "$@" >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then fail "$label unexpectedly succeeded"; fi
  [[ ! -s "$CASE_DIR/docker.log" ]] || fail "$label invoked Docker before validation"
  [[ ! -s "$CASE_DIR/ssh.log" ]] || fail "$label invoked SSH before validation"
}

test_writer_and_build() {
  setup_case success; write_evidence
  ruby -rjson -e 'v=JSON.parse(File.binread(ARGV[0])); abort unless v.keys.sort==%w[commands created_at migrations_hash result schema_version source_commit tested_tree] && v["result"]=="passed"' "$EVIDENCE" || fail 'invalid evidence schema'
  [[ "$(stat -f '%Lp' "$EVIDENCE" 2>/dev/null || stat -c '%a' "$EVIDENCE")" == 600 ]] || fail 'evidence is not 0600'
  run_controller >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail 'controller failed'
  grep -F -- '--platform linux/amd64' "$CASE_DIR/docker.log" >/dev/null || fail 'non-amd64 build'
  grep -F -- '--provenance=false' "$CASE_DIR/docker.log" >/dev/null || fail 'provenance not disabled'
  grep -F -- '--sbom=false' "$CASE_DIR/docker.log" >/dev/null || fail 'sbom not disabled'
  grep -F -- "--image example.invalid/xingqiao-relay-ops@sha256:$SHA256" "$CASE_DIR/ssh.log" >/dev/null || fail 'immutable digest not sent'
  grep -F -- '-o StrictHostKeyChecking=yes' "$CASE_DIR/ssh.log" >/dev/null || fail 'strict host key checking missing'
}

test_rejects_dirty_tree() {
  setup_case dirty; write_evidence; printf dirty >"$CASE_DIR/repo/dirty"
  expect_failure_before_transport dirty_tree
}

test_bounds_host_ssh_stage() {
  setup_case ssh_timeout; write_evidence
  if run_controller RELEASE_STAGE_TIMEOUT_SECONDS=3 FAKE_SSH_SLEEP=6 >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then fail 'hung SSH unexpectedly succeeded'; fi
  grep -F -- 'ssh ' "$CASE_DIR/ssh.log" >/dev/null || fail 'SSH stage was not invoked'
}

test_preloaded_transport_uploads_verified_archive() {
  setup_case preloaded; write_evidence
  run_controller RELEASE_TRANSPORT=preloaded >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail 'preloaded controller failed'
  grep -F -- '--load' "$CASE_DIR/docker.log" >/dev/null || fail 'preloaded build did not load image'
  grep -F -- 'image save' "$CASE_DIR/docker.log" >/dev/null || fail 'preloaded image archive missing'
  grep -F -- 'scp ' "$CASE_DIR/ssh.log" >/dev/null || fail 'preloaded archive was not transferred'
  grep -F -- 'sudo -n' "$CASE_DIR/ssh.log" >/dev/null || fail 'preloaded host executor was not root-gated'
  grep -F -- '--preloaded-archive-sha256' "$CASE_DIR/ssh.log" >/dev/null || fail 'archive checksum was not sent'
}

test_default_build_goproxy_reaches_buildx() {
  setup_case default_build_goproxy; write_evidence
  run_controller >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail 'default build GOPROXY controller failed'
  grep -F -- '--build-arg GOPROXY=https://proxy.golang.org,direct' "$CASE_DIR/docker.log" >/dev/null || fail 'default build GOPROXY was not passed to buildx'
}

test_approved_build_goproxy_reaches_buildx() {
  setup_case approved_build_goproxy; write_evidence
  run_controller RELAY_OPS_BUILD_GOPROXY=https://goproxy.cn,direct >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail 'approved build GOPROXY controller failed'
  grep -F -- '--build-arg GOPROXY=https://goproxy.cn,direct' "$CASE_DIR/docker.log" >/dev/null || fail 'approved build GOPROXY was not passed to buildx'
}

test_rejects_unapproved_build_goproxy_before_transport() {
  setup_case rejected_build_goproxy; write_evidence
  expect_failure_before_transport rejected_build_goproxy RELAY_OPS_BUILD_GOPROXY=https://mirror.example.invalid,direct
}

test_writer_and_build
test_rejects_dirty_tree
test_bounds_host_ssh_stage
test_preloaded_transport_uploads_verified_archive
test_default_build_goproxy_reaches_buildx
test_approved_build_goproxy_reaches_buildx
test_rejects_unapproved_build_goproxy_before_transport
printf 'PASS: relay-ops release controller\n'
