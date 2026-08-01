#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
WRITER=${WRITER_UNDER_TEST:-"$ROOT/ops/write-sub2api-test-evidence.sh"}
CONTROLLER=${CONTROLLER_UNDER_TEST:-"$ROOT/ops/release-sub2api-blue-green.sh"}
FIXTURE=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-release-controller.XXXXXX")
FIXTURE=$(cd "$FIXTURE" && pwd -P)
trap 'rm -rf -- "$FIXTURE"' EXIT

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

[[ -f "$WRITER" ]] || fail "evidence writer does not exist: $WRITER"
[[ -f "$CONTROLLER" ]] || fail "release controller does not exist: $CONTROLLER"

SHA256=$(printf 'a%.0s' {1..64})

setup_case() {
  CASE_DIR="$FIXTURE/$1"
  rm -rf -- "$CASE_DIR"
  mkdir -p "$CASE_DIR/bin" "$CASE_DIR/repo/upstream/sub2api/backend/migrations"
  : >"$CASE_DIR/docker.log"
  : >"$CASE_DIR/ssh.log"
  printf 'FROM scratch\n' >"$CASE_DIR/repo/Dockerfile"
  printf '\nCREATE TABLE example ();\n' >"$CASE_DIR/repo/upstream/sub2api/backend/migrations/001_init.sql"
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
    [[ "${FAKE_DOCKER_MODE:-}" != slow ]] || /bin/sleep 2
    exit 0
    ;;
  'buildx imagetools inspect') printf 'sha256:%s\n' "${FAKE_DIGEST:?}"; exit 0 ;;
esac
exit 64
SH
  cat >"$CASE_DIR/bin/ssh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'ssh %s\n' "$*" >>"${FAKE_SSH_LOG:?}"
if [[ "${FAKE_HOST_RESULT:-success}" == downtime ]]; then
  printf '{"schema_version":1,"downtime_required":true,"reason_code":"migration_set_changed"}\n'
  exit 2
fi
printf '{"schema_version":1,"downtime_required":false,"result":"succeeded"}\n'
SH
  cat >"$CASE_DIR/bin/monotonic" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
sequence=${FAKE_CLOCK_SEQUENCE:-0}
count_file="${FAKE_CLOCK_COUNT:?}"
count=0
[[ -f "$count_file" ]] && count=$(cat "$count_file")
count=$((count + 1))
printf '%s\n' "$count" >"$count_file"
printf '%s\n' "$sequence" | awk -F ',' -v position="$count" '{ if (position <= NF) print $position; else print $NF }'
SH
  chmod +x "$CASE_DIR/bin/docker" "$CASE_DIR/bin/ssh" "$CASE_DIR/bin/monotonic"
}

write_evidence() {
  EVIDENCE="$CASE_DIR/evidence.json"
  (
    cd "$CASE_DIR/repo"
    bash "$WRITER" --output "$EVIDENCE" --command 'bash tests/operations/release_sub2api_blue_green_test.sh'
  )
}

run_controller() {
  env \
    PATH="$CASE_DIR/bin:$PATH" \
    FAKE_DOCKER_LOG="$CASE_DIR/docker.log" \
    FAKE_SSH_LOG="$CASE_DIR/ssh.log" \
    FAKE_DIGEST="$SHA256" \
    FAKE_CLOCK_COUNT="$CASE_DIR/clock-count" \
    RELEASE_WORKTREE="$CASE_DIR/repo" \
    RELEASE_BUILD_CONTEXT="$CASE_DIR/repo" \
    SUB2API_IMAGE_REPOSITORY='example.invalid/xingqiao-sub2api' \
    RELEASE_SSH_BIN="$CASE_DIR/bin/ssh" \
    RELEASE_SSH_TARGET='release@example.invalid' \
    RELEASE_SSH_KEY="$CASE_DIR/id_ed25519" \
    RELEASE_SSH_KNOWN_HOSTS="$CASE_DIR/known_hosts" \
    RELEASE_SSH_PORT=22 \
    RELEASE_MONOTONIC_BIN="$CASE_DIR/bin/monotonic" \
    "$@" bash "$CONTROLLER" --mode production --evidence "$EVIDENCE"
}

expect_failure_before_transport() {
  local label=$1
  shift
  if "$@" >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail "$label unexpectedly succeeded"
  fi
  [[ ! -s "$CASE_DIR/docker.log" ]] || fail "$label invoked Docker before rejecting evidence"
  [[ ! -s "$CASE_DIR/ssh.log" ]] || fail "$label invoked SSH before rejecting evidence"
}

rewrite_evidence() {
  write_evidence
  ruby -rjson -e '
    path, key, value = ARGV
    data = JSON.parse(File.binread(path))
    data[key] = value == "__array_empty__" ? [] : value
    File.write(path, JSON.generate(data))
  ' "$EVIDENCE" "$1" "$2"
  chmod 0600 "$EVIDENCE"
}

test_writer_schema_and_permissions() {
  setup_case writer
  write_evidence
  ruby -rjson -e '
    value = JSON.parse(File.binread(ARGV.fetch(0)))
    abort unless value.keys.sort == %w[commands created_at migrations_hash result schema_version source_commit tested_tree]
    abort unless value.fetch("schema_version") == 1 && value.fetch("result") == "passed"
    abort unless value.fetch("commands") == ["bash tests/operations/release_sub2api_blue_green_test.sh"]
    abort unless value.fetch("source_commit").match?(/\A[0-9a-f]{40}\z/)
    abort unless value.fetch("tested_tree").match?(/\A[0-9a-f]{40}\z/)
    abort unless value.fetch("migrations_hash").match?(/\A[0-9a-f]{64}\z/)
    abort unless value.fetch("created_at").match?(/\A\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ\z/)
  ' "$EVIDENCE" || fail 'writer schema is invalid'
  [[ "$(stat -f '%Lp' "$EVIDENCE" 2>/dev/null || stat -c '%a' "$EVIDENCE")" == 600 ]] || fail 'writer did not create 0600 evidence'
  if (cd "$CASE_DIR/repo" && bash "$WRITER" --output "$CASE_DIR/empty.json") >/dev/null 2>&1; then
    fail 'writer accepted missing command evidence'
  fi
}

test_evidence_rejected_before_transport() {
  local bad_commit
  setup_case dirty
  write_evidence
  printf 'dirty\n' >"$CASE_DIR/repo/dirty.txt"
  expect_failure_before_transport dirty_tree run_controller

  setup_case tested-tree
  write_evidence
  rewrite_evidence tested_tree "$(printf 'b%.0s' {1..40})"
  expect_failure_before_transport tested_tree_mismatch run_controller

  setup_case commit
  write_evidence
  bad_commit=$(printf 'c%.0s' {1..40})
  rewrite_evidence source_commit "$bad_commit"
  expect_failure_before_transport commit_mismatch run_controller

  setup_case migrations
  write_evidence
  rewrite_evidence migrations_hash "$SHA256"
  expect_failure_before_transport migration_hash_mismatch run_controller

  setup_case failed-result
  write_evidence
  rewrite_evidence result failed
  expect_failure_before_transport failed_result run_controller

  setup_case missing-command
  write_evidence
  rewrite_evidence commands __array_empty__
  expect_failure_before_transport missing_commands run_controller

  setup_case permissive
  write_evidence
  chmod 0644 "$EVIDENCE"
  expect_failure_before_transport permissive_evidence run_controller

  setup_case symlink
  write_evidence
  mv "$EVIDENCE" "$CASE_DIR/target.json"
  ln -s "$CASE_DIR/target.json" "$EVIDENCE"
  expect_failure_before_transport symlink_evidence run_controller

  setup_case malformed
  write_evidence
  printf '{bad json\n' >"$EVIDENCE"
  chmod 0600 "$EVIDENCE"
  expect_failure_before_transport malformed_json run_controller

  setup_case unknown
  write_evidence
  ruby -rjson -e 'p = ARGV.fetch(0); x = JSON.parse(File.binread(p)); x["unexpected"] = true; File.write(p, JSON.generate(x))' "$EVIDENCE"
  chmod 0600 "$EVIDENCE"
  expect_failure_before_transport unknown_key run_controller
}

test_build_publish_and_host_invocation() {
  setup_case success
  write_evidence
  run_controller >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail "controller success failed: $(cat "$CASE_DIR/stderr")"
  [[ $(grep -c '^docker buildx build ' "$CASE_DIR/docker.log") -eq 1 ]] || fail 'controller did not build exactly once'
  [[ $(grep -c '^docker buildx imagetools inspect ' "$CASE_DIR/docker.log") -eq 1 ]] || fail 'controller did not resolve the immutable digest exactly once'
  [[ $(wc -l <"$CASE_DIR/ssh.log" | tr -d ' ') -eq 1 ]] || fail 'controller did not invoke the host once'
  grep -F -- '--platform linux/amd64' "$CASE_DIR/docker.log" >/dev/null || fail 'build platform is not Linux AMD64'
  grep -F -- '--label com.xingqiao.sub2api.qualified=true' "$CASE_DIR/docker.log" >/dev/null || fail 'qualified label missing'
  grep -F -- "--label com.xingqiao.sub2api.source.commit=$(git -C "$CASE_DIR/repo" rev-parse HEAD)" "$CASE_DIR/docker.log" >/dev/null || fail 'source commit label missing'
  grep -F -- "--label com.xingqiao.sub2api.source.tree=$(git -C "$CASE_DIR/repo" rev-parse 'HEAD^{tree}')" "$CASE_DIR/docker.log" >/dev/null || fail 'source tree label missing'
  grep -F -- "--label com.xingqiao.sub2api.tested.tree=$(git -C "$CASE_DIR/repo" rev-parse 'HEAD^{tree}')" "$CASE_DIR/docker.log" >/dev/null || fail 'tested tree label missing'
  grep -F -- '--label com.xingqiao.sub2api.migrations.sha256=' "$CASE_DIR/docker.log" >/dev/null || fail 'migrations label missing'
  grep -F -- "--image example.invalid/xingqiao-sub2api@sha256:$SHA256" "$CASE_DIR/ssh.log" >/dev/null || fail 'host did not receive immutable digest'
  grep -F '"result":"succeeded"' "$CASE_DIR/stdout" >/dev/null || fail 'controller did not propagate host result'
}

test_downtime_gate_is_propagated_without_retry() {
  setup_case downtime
  write_evidence
  if run_controller FAKE_HOST_RESULT=downtime >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'downtime gate unexpectedly succeeded'
  fi
  [[ $(wc -l <"$CASE_DIR/ssh.log" | tr -d ' ') -eq 1 ]] || fail 'downtime gate retried the host'
  [[ $(grep -c '^docker buildx build ' "$CASE_DIR/docker.log") -eq 1 ]] || fail 'downtime gate rebuilt the image'
  grep -F '"downtime_required":true' "$CASE_DIR/stdout" >/dev/null || fail 'downtime gate was not propagated'
}

test_total_budget_stops_before_host() {
  setup_case budget
  write_evidence
  if run_controller FAKE_CLOCK_SEQUENCE=0,0,1801 >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'over-budget controller unexpectedly succeeded'
  fi
  [[ ! -s "$CASE_DIR/ssh.log" ]] || fail 'over-budget controller invoked host'
}

test_per_stage_timeout_stops_before_host() {
  setup_case stage-timeout
  write_evidence
  if run_controller FAKE_DOCKER_MODE=slow RELEASE_STAGE_TIMEOUT_SECONDS=1 >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'stage-timeout controller unexpectedly succeeded'
  fi
  [[ ! -s "$CASE_DIR/ssh.log" ]] || fail 'stage-timeout controller invoked host'
}

test_rejects_unattested_build_context_before_transport() {
  setup_case untrusted-context
  write_evidence
  mkdir -p "$CASE_DIR/untrusted-context"
  if run_controller RELEASE_BUILD_CONTEXT="$CASE_DIR/untrusted-context" >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'untrusted build context unexpectedly succeeded'
  fi
  [[ ! -s "$CASE_DIR/docker.log" ]] || fail 'untrusted build context invoked Docker'
  [[ ! -s "$CASE_DIR/ssh.log" ]] || fail 'untrusted build context invoked SSH'
}

test_writer_schema_and_permissions
test_evidence_rejected_before_transport
test_build_publish_and_host_invocation
test_downtime_gate_is_propagated_without_retry
test_total_budget_stops_before_host
test_per_stage_timeout_stops_before_host
test_rejects_unattested_build_context_before_transport
printf 'PASS: tested-tree evidence and blue-green release controller\n'
