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
  mkdir -p "$CASE_DIR/bin" "$CASE_DIR/repo/upstream/sub2api/backend/migrations" "$CASE_DIR/repo/ops"
  : >"$CASE_DIR/docker.log"
  : >"$CASE_DIR/ssh.log"
  printf 'FROM scratch\n' >"$CASE_DIR/repo/Dockerfile"
  printf '\nCREATE TABLE example ();\n' >"$CASE_DIR/repo/upstream/sub2api/backend/migrations/001_init.sql"
  printf '#!/usr/bin/env bash\nprintf "host executor\\n"\n' >"$CASE_DIR/repo/ops/deploy-sub2api-blue-green-host.sh"
  chmod 0755 "$CASE_DIR/repo/ops/deploy-sub2api-blue-green-host.sh"
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
  'image inspect '*)
    printf '%s\n' "${FAKE_IMAGE_ID:?}"
    exit 0
    ;;
  'image tag '*) exit 0 ;;
  'image save '*)
    output=''
    for ((index=1; index <= $#; index++)); do
      if [[ "${!index}" == --output ]]; then
        next=$((index + 1))
        output=${!next}
      fi
    done
    [[ -n "$output" ]] || exit 64
    printf 'preloaded archive\n' >"$output"
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
if [[ "$*" == *'verify_executor_directory_chain'* && "${FAKE_UNSAFE_EXECUTOR_PARENT:-false}" == true ]]; then
  exit 1
fi
if [[ "$*" == *'verify_executor_path_chain'* && "${FAKE_EXECUTOR_ABSENT:-false}" == true \
    && ! -e "${FAKE_EXECUTOR_INSTALLED_FILE:?}" ]]; then
  exit 1
fi
if [[ "$*" == *'verify_executor_directory_chain'* || "$*" == *'verify_executor_path_chain'* ]]; then
  exit 0
fi
case "$*" in
  *'sudo -n install -o root -g root -m 0755 '* )
    printf 'install\n' >>"${FAKE_EXECUTOR_STAGE_LOG:?}"
    [[ "${FAKE_EXECUTOR_STAGE_RESULT:-success}" != install_fail ]] || exit 1
    : >"${FAKE_EXECUTOR_STAGED_DESTINATION:?}"
    if [[ "${FAKE_EXECUTOR_STAGE_RESULT:-success}" == syntax_fail ]]; then
      [[ -f "${FAKE_EXECUTOR_STAGED_DESTINATION}" ]] || exit 64
      [[ "$*" == *'bash -n '* ]] || exit 64
      printf 'bash-n\n' >>"${FAKE_EXECUTOR_STAGE_LOG:?}"
      exit 1
    fi
    printf '0:0:755\n'
    exit 0
    ;;
  *'sudo -n mv -f -- '* )
    [[ "${FAKE_EXECUTOR_ABSENT:-false}" != true ]] || : >"${FAKE_EXECUTOR_INSTALLED_FILE:?}"
    printf '0:0:755\n'
    exit 0
    ;;
  *'stat -c '* )
    printf '0:0:755\n'
    exit 0
    ;;
  *'mktemp -p /tmp .sub2api-host-executor-'*)
    printf '%s\n' "${FAKE_EXECUTOR_TMP:?}"
    exit 0
    ;;
  *'mktemp -p /usr/local/libexec .deploy-sub2api-blue-green-host.sh.'*)
    printf '%s\n' "${FAKE_EXECUTOR_DEST_TMP:?}"
    exit 0
    ;;
  *'mktemp -p /tmp .sub2api-'*)
    printf '%s\n' "${FAKE_ARCHIVE_TMP:?}"
    exit 0
    ;;
  *'sha256sum '*)
    printf '%s  %s\n' "${FAKE_EXECUTOR_SHA256:?}" "${FAKE_HOST_EXECUTOR_PATH:?}"
    exit 0
    ;;
esac
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
  cat >"$CASE_DIR/bin/scp" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'scp %s\n' "$*" >>"${FAKE_SCP_LOG:?}"
[[ "${FAKE_SCP_RESULT:-success}" == success ]] || exit 1
SH
  chmod +x "$CASE_DIR/bin/scp"
}

write_evidence() {
  EVIDENCE="$CASE_DIR/evidence.json"
  (
    cd "$CASE_DIR/repo"
    bash "$WRITER" --output "$EVIDENCE" --command 'bash tests/operations/release_sub2api_blue_green_test.sh'
  )
}

run_controller() {
  local controller_mode=production
  local argument
  for argument in "$@"; do
    case "$argument" in
      RELEASE_TEST_MODE=*) controller_mode=${argument#RELEASE_TEST_MODE=} ;;
    esac
  done
  env \
    PATH="$CASE_DIR/bin:$PATH" \
    FAKE_DOCKER_LOG="$CASE_DIR/docker.log" \
    FAKE_SSH_LOG="$CASE_DIR/ssh.log" \
    FAKE_SCP_LOG="$CASE_DIR/scp.log" \
    FAKE_EXECUTOR_STAGE_LOG="$CASE_DIR/executor-stage.log" \
    FAKE_EXECUTOR_STAGED_DESTINATION="$CASE_DIR/staged-executor-destination" \
    FAKE_DIGEST="$SHA256" \
    FAKE_IMAGE_ID="sha256:$(printf 'b%.0s' {1..64})" \
    FAKE_EXECUTOR_TMP="/tmp/.sub2api-host-executor-$(git -C "$CASE_DIR/repo" rev-parse HEAD).abc123" \
    FAKE_EXECUTOR_DEST_TMP='/usr/local/libexec/.deploy-sub2api-blue-green-host.sh.abc123' \
    FAKE_EXECUTOR_INSTALLED_FILE="$CASE_DIR/executor-installed" \
    FAKE_ARCHIVE_TMP="/tmp/.sub2api-$(git -C "$CASE_DIR/repo" rev-parse HEAD).abc123" \
    FAKE_EXECUTOR_SHA256="$(sha256sum "$CASE_DIR/repo/ops/deploy-sub2api-blue-green-host.sh" | awk '{print $1}')" \
    FAKE_HOST_EXECUTOR_PATH="${RELEASE_HOST_EXECUTOR_PATH:-/usr/local/libexec/deploy-sub2api-blue-green-host.sh}" \
    FAKE_CLOCK_COUNT="$CASE_DIR/clock-count" \
    RELEASE_WORKTREE="$CASE_DIR/repo" \
    RELEASE_BUILD_CONTEXT="$CASE_DIR/repo" \
    SUB2API_IMAGE_REPOSITORY='example.invalid/xingqiao-sub2api' \
    RELEASE_SSH_BIN="$CASE_DIR/bin/ssh" \
    RELEASE_SCP_BIN="$CASE_DIR/bin/scp" \
    RELEASE_SSH_TARGET='release@example.invalid' \
    RELEASE_SSH_KEY="$CASE_DIR/id_ed25519" \
    RELEASE_SSH_KNOWN_HOSTS="$CASE_DIR/known_hosts" \
    RELEASE_SSH_PORT=22 \
    RELEASE_NETWORK_CURL_IMAGE='example.invalid/curl@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' \
    RELEASE_NETWORK_CURL_IMAGE_ALLOWLIST='example.invalid/curl@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' \
    RELEASE_MONOTONIC_BIN="$CASE_DIR/bin/monotonic" \
    "$@" bash "$CONTROLLER" --mode "$controller_mode" --evidence "$EVIDENCE" \
    ${CONTROLLER_MAINTENANCE_AUTHORIZED:+--maintenance-authorized}
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

test_migration_hash_matches_go_trim_space_for_unicode_whitespace() {
  setup_case unicode-whitespace
  printf '\343\200\200CREATE TABLE example ();\343\200\200\n' >"$CASE_DIR/repo/upstream/sub2api/backend/migrations/001_init.sql"
  git -C "$CASE_DIR/repo" add upstream/sub2api/backend/migrations/001_init.sql
  git -C "$CASE_DIR/repo" commit -qm unicode-whitespace
  write_evidence
  ruby -rjson -e '
    value = JSON.parse(File.binread(ARGV.fetch(0)))
    abort unless value.fetch("migrations_hash") == "6bf880ea3cbbdb3e0e723512267b4e7d3bfacbaf29e9148c85dbc92c867d337e"
  ' "$EVIDENCE" || fail 'migration hash does not match Go strings.TrimSpace normalization'
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
  [[ $(grep -c 'bash /usr/local/libexec/deploy-sub2api-blue-green-host.sh --mode production' "$CASE_DIR/ssh.log" || true) -eq 1 ]] \
    || fail 'controller did not invoke the installed host executor once'
  [[ $(grep -c '^scp ' "$CASE_DIR/scp.log" || true) -eq 1 ]] || fail 'controller did not transfer the host executor'
  grep -F -- 'sudo -n install -o root -g root -m 0755 /tmp/.sub2api-host-executor-' "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'controller did not stage host executor with root ownership and 0755 mode'
  grep -F -- 'sudo -n mv -f -- /usr/local/libexec/.deploy-sub2api-blue-green-host.sh.abc123 /usr/local/libexec/deploy-sub2api-blue-green-host.sh' "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'controller did not atomically promote the validated host executor'
  ! grep -F -- 'sudo -n install -o root -g root -m 0755 /tmp/.sub2api-host-executor-'"$(git -C "$CASE_DIR/repo" rev-parse HEAD)"'.abc123 /usr/local/libexec/deploy-sub2api-blue-green-host.sh' "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'controller directly installed the host executor over the final path'
  grep -F -- "--image example.invalid/xingqiao-sub2api@sha256:$SHA256" "$CASE_DIR/ssh.log" >/dev/null || fail 'host did not receive immutable digest'
  grep -F -- 'DEPLOY_ROOT=/opt/sub2api/production' "$CASE_DIR/ssh.log" >/dev/null || fail 'host did not receive canonical deploy root'
  grep -F -- 'BASE_COMPOSE=/opt/sub2api/production/compose.yaml' "$CASE_DIR/ssh.log" >/dev/null || fail 'host did not receive canonical compose path'
  grep -F -- 'RELEASE_STAGING_ROOT=/var/lib/sub2api/release-staging' "$CASE_DIR/ssh.log" >/dev/null || fail 'host did not receive release staging root'
  grep -F -- 'NETWORK_CURL_IMAGE=example.invalid/curl@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' "$CASE_DIR/ssh.log" >/dev/null || fail 'host did not receive approved network probe image'
  grep -F -- '--platform linux/amd64' "$CASE_DIR/docker.log" >/dev/null || fail 'build platform is not Linux AMD64'
  grep -F -- '--label com.xingqiao.sub2api.qualified=true' "$CASE_DIR/docker.log" >/dev/null || fail 'qualified label missing'
  grep -F -- "--label com.xingqiao.sub2api.source.commit=$(git -C "$CASE_DIR/repo" rev-parse HEAD)" "$CASE_DIR/docker.log" >/dev/null || fail 'source commit label missing'
  grep -F -- "--label com.xingqiao.sub2api.source.tree=$(git -C "$CASE_DIR/repo" rev-parse 'HEAD^{tree}')" "$CASE_DIR/docker.log" >/dev/null || fail 'source tree label missing'
  grep -F -- "--label com.xingqiao.sub2api.tested.tree=$(git -C "$CASE_DIR/repo" rev-parse 'HEAD^{tree}')" "$CASE_DIR/docker.log" >/dev/null || fail 'tested tree label missing'
  grep -F -- '--label com.xingqiao.sub2api.migrations.sha256=' "$CASE_DIR/docker.log" >/dev/null || fail 'migrations label missing'
  grep -E -- '--deadline-epoch [1-9][0-9]{9}($| )' "$CASE_DIR/ssh.log" >/dev/null || fail 'host did not receive an absolute deadline'
  grep -F '"result":"succeeded"' "$CASE_DIR/stdout" >/dev/null || fail 'controller did not propagate host result'
}

test_maintenance_controller_forwards_exact_current_migration_hash() {
  local current_hash=aee795202a3dd14c191c5e395add6beb58942950bf530d9961ae80a359998429
  local retired_hash=ac8b0b33d7ea31a1a4f0117716ba56efec4bd66be9c38267a88d4c512d01bf39

  setup_case maintenance-current-hash
  write_evidence
  CONTROLLER_MAINTENANCE_AUTHORIZED=true RELEASE_MAINTENANCE_FROM_HASH=$current_hash \
    run_controller >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" \
    || fail "maintenance controller failed: $(cat "$CASE_DIR/stderr")"
  grep -F -- "--maintenance-authorized --maintenance-from-hash $current_hash" "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'maintenance controller did not forward the current production migration hash'
  ! grep -F -- "$retired_hash" "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'maintenance controller forwarded the retired migration hash'

  setup_case maintenance-missing-hash
  write_evidence
  if CONTROLLER_MAINTENANCE_AUTHORIZED=true run_controller >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'maintenance controller accepted a missing source migration hash'
  fi
  [[ ! -s "$CASE_DIR/docker.log" ]] || fail 'missing maintenance source hash reached image build'
}

test_downtime_gate_is_propagated_without_retry() {
  setup_case downtime
  write_evidence
  if run_controller FAKE_HOST_RESULT=downtime >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'downtime gate unexpectedly succeeded'
  fi
  [[ $(grep -c 'bash /usr/local/libexec/deploy-sub2api-blue-green-host.sh --mode production' "$CASE_DIR/ssh.log" || true) -eq 1 ]] \
    || fail 'downtime gate retried the host'
  [[ $(grep -c '^docker buildx build ' "$CASE_DIR/docker.log") -eq 1 ]] || fail 'downtime gate rebuilt the image'
  grep -F '"downtime_required":true' "$CASE_DIR/stdout" >/dev/null || fail 'downtime gate was not propagated'
}

assert_executor_install_failure_before_build() {
  local label=$1
  shift
  setup_case "$label"
  write_evidence
  if run_controller "$@" >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail "$label unexpectedly succeeded"
  fi
  ! grep -q '^docker buildx build ' "$CASE_DIR/docker.log" \
    || fail "$label ran buildx after host executor installation failed"
  ! grep -F -- 'sudo -n mv -f -- /usr/local/libexec/.deploy-sub2api-blue-green-host.sh.abc123 /usr/local/libexec/deploy-sub2api-blue-green-host.sh' "$CASE_DIR/ssh.log" >/dev/null \
    || fail "$label overwrote the existing host executor"
  grep -F -- 'sudo -n rm -f --' "$CASE_DIR/ssh.log" >/dev/null \
    || fail "$label did not clean up remote executor staging files after failure"
  if [[ "$label" == executor_remote_bash_syntax_failure ]]; then
    [[ -f "$CASE_DIR/staged-executor-destination" ]] \
      || fail 'bash syntax failure did not have a successfully installed staged destination'
    [[ "$(tr '\n' ' ' <"$CASE_DIR/executor-stage.log")" == 'install bash-n ' ]] \
      || fail 'bash syntax failure did not occur after a successful staged install'
  fi
}

test_executor_install_failures_stop_before_build() {
  assert_executor_install_failure_before_build executor_scp_failure FAKE_SCP_RESULT=fail
  assert_executor_install_failure_before_build executor_remote_install_failure FAKE_EXECUTOR_STAGE_RESULT=install_fail
  assert_executor_install_failure_before_build executor_remote_bash_syntax_failure FAKE_EXECUTOR_STAGE_RESULT=syntax_fail
  assert_executor_install_failure_before_build executor_remote_sha_failure \
    FAKE_EXECUTOR_SHA256="$(printf 'f%.0s' {1..64})"
}

test_executor_parent_chain_rejects_before_build() {
  setup_case executor_insecure_parent
  write_evidence
  if run_controller FAKE_UNSAFE_EXECUTOR_PARENT=true >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'insecure host executor parent chain unexpectedly succeeded'
  fi
  grep -F -- 'verify_executor_directory_chain' "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'controller did not verify the host executor parent chain'
  ! grep -q '^docker buildx build ' "$CASE_DIR/docker.log" \
    || fail 'insecure host executor parent chain reached image build'
}

test_executor_bootstraps_when_final_file_is_absent() {
  setup_case executor_bootstrap
  write_evidence
  run_controller FAKE_EXECUTOR_ABSENT=true >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" \
    || fail "controller could not bootstrap a missing host executor: $(cat "$CASE_DIR/stderr")"
  grep -F -- 'verify_executor_directory_chain' "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'controller did not verify the executor directory chain before bootstrap installation'
  grep -F -- 'verify_executor_path_chain' "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'controller did not verify the installed executor path chain after bootstrap installation'
  directory_line=$(grep -n -m1 'verify_executor_directory_chain' "$CASE_DIR/ssh.log" | cut -d: -f1)
  install_line=$(grep -n -m1 'sudo -n install -o root -g root -m 0755' "$CASE_DIR/ssh.log" | cut -d: -f1)
  path_lines=$(grep -n 'verify_executor_path_chain' "$CASE_DIR/ssh.log" | cut -d: -f1)
  path_count=$(printf '%s\n' "$path_lines" | awk 'NF { count++ } END { print count + 0 }')
  path_line=$(printf '%s\n' "$path_lines" | sed -n '1p')
  final_path_line=$(printf '%s\n' "$path_lines" | tail -n 1)
  execute_line=$(grep -n -m1 'bash /usr/local/libexec/deploy-sub2api-blue-green-host.sh --mode production' "$CASE_DIR/ssh.log" | cut -d: -f1)
  [[ -n "$directory_line" && -n "$install_line" && -n "$path_line" && -n "$final_path_line" && -n "$execute_line" ]] \
    || fail 'bootstrap evidence did not include directory, install, path, and execution stages'
  (( path_count >= 2 )) || fail 'bootstrap evidence did not include the final pre-execution path attestation'
  (( directory_line < install_line && install_line < path_line && path_line < final_path_line && final_path_line < execute_line )) \
    || fail 'bootstrap stages were not ordered as directory-check, install, path-check, execute'
}

test_preloaded_transport_installs_executor_and_forwards_archive() {
  setup_case preloaded
  write_evidence
  run_controller RELEASE_TRANSPORT=preloaded >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" \
    || fail "preloaded controller failed: $(cat "$CASE_DIR/stderr")"
  [[ $(grep -c '^docker buildx build ' "$CASE_DIR/docker.log") -eq 1 ]] || fail 'preloaded controller did not build exactly once'
  [[ $(grep -c '^docker buildx imagetools inspect ' "$CASE_DIR/docker.log" || true) -eq 0 ]] || fail 'preloaded controller resolved a registry digest'
  [[ $(grep -c '^scp ' "$CASE_DIR/scp.log" || true) -eq 2 ]] || fail 'preloaded controller did not transfer archive and executor'
  grep -F -- 'RELEASE_PRELOADED_IMAGE=true' "$CASE_DIR/ssh.log" >/dev/null || fail 'preloaded mode was not forwarded to host'
  grep -F -- '--preloaded-archive /var/lib/sub2api/release-staging/' "$CASE_DIR/ssh.log" >/dev/null || fail 'preloaded archive path was not forwarded to host'
  grep -F -- '--preloaded-image-id sha256:' "$CASE_DIR/ssh.log" >/dev/null || fail 'preloaded image ID was not forwarded to host'
}

test_custom_staging_root_and_rehearsal_root_survive_environment_reset() {
  setup_case custom-paths
  write_evidence
  mkdir -p "$CASE_DIR/custom-staging" "$CASE_DIR/rehearsal-root"
  run_controller RELEASE_TRANSPORT=preloaded RELEASE_STAGING_ROOT="$CASE_DIR/custom-staging" \
    >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" \
    || fail "custom staging root controller failed: $(cat "$CASE_DIR/stderr")"
  grep -F -- "RELEASE_STAGING_ROOT=$CASE_DIR/custom-staging" "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'custom staging root was not forwarded to host'
  grep -F -- "--preloaded-archive $CASE_DIR/custom-staging/sub2api-" "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'custom staging root was not used for the archive path'

  setup_case rehearsal-path
  write_evidence
  mkdir -p "$CASE_DIR/rehearsal-root"
  run_controller RELEASE_TEST_MODE=rehearsal REHEARSAL_ROOT="$CASE_DIR/rehearsal-root" \
    >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" \
    || fail "rehearsal controller failed: $(cat "$CASE_DIR/stderr")"
  grep -F -- "REHEARSAL_ROOT=$CASE_DIR/rehearsal-root" "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'rehearsal root was not forwarded to host'

  setup_case rehearsal-missing-root
  write_evidence
  if run_controller RELEASE_TEST_MODE=rehearsal >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'rehearsal without root unexpectedly succeeded'
  fi
  [[ ! -s "$CASE_DIR/docker.log" && ! -s "$CASE_DIR/ssh.log" ]] || fail 'missing rehearsal root reached transport'
}

test_preloaded_transport_binds_release_reference_to_image_id() {
  local source_commit first_id second_id first_tag second_tag
  setup_case preloaded-image-reference
  write_evidence
  source_commit=$(git -C "$CASE_DIR/repo" rev-parse HEAD)
  first_id="sha256:$(printf 'b%.0s' {1..64})"
  second_id="sha256:$(printf 'c%.0s' {1..64})"
  first_tag="example.invalid/xingqiao-sub2api:release-$source_commit-${first_id#sha256:}"
  second_tag="example.invalid/xingqiao-sub2api:release-$source_commit-${second_id#sha256:}"

  run_controller RELEASE_TRANSPORT=preloaded FAKE_IMAGE_ID="$first_id" >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" \
    || fail "first preloaded controller failed: $(cat "$CASE_DIR/stderr")"
  run_controller RELEASE_TRANSPORT=preloaded FAKE_IMAGE_ID="$second_id" >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" \
    || fail "second preloaded controller failed: $(cat "$CASE_DIR/stderr")"

  [[ $(grep -c '^docker buildx build ' "$CASE_DIR/docker.log") -eq 2 ]] \
    || fail 'preloaded image-ID test did not run both builds'
  ! grep -F -- "--tag example.invalid/xingqiao-sub2api:release-$source_commit " "$CASE_DIR/docker.log" >/dev/null \
    || fail 'preloaded build reused the mutable commit-only release tag'
  ! grep -E -- "example\.invalid/xingqiao-sub2api:release-$source_commit( |$)" "$CASE_DIR/docker.log" >/dev/null \
    || fail 'preloaded rebuild touched the mutable commit-only release tag'
  [[ $(grep -E -c -- "--tag example.invalid/xingqiao-sub2api:build-$source_commit-[a-f0-9]{32}( |$)" "$CASE_DIR/docker.log") -eq 2 ]] \
    || fail 'preloaded build did not use a unique temporary build tag'
  grep -E -- "docker image tag example.invalid/xingqiao-sub2api:build-$source_commit-[a-f0-9]{32} $first_tag$" "$CASE_DIR/docker.log" >/dev/null \
    || fail 'first preloaded build did not bind its release tag to its image ID'
  grep -E -- "docker image tag example.invalid/xingqiao-sub2api:build-$source_commit-[a-f0-9]{32} $second_tag$" "$CASE_DIR/docker.log" >/dev/null \
    || fail 'second preloaded build did not bind its release tag to its image ID'
  grep -F -- "docker image save --output " "$CASE_DIR/docker.log" | grep -F -- "$first_tag" >/dev/null \
    || fail 'first preloaded archive did not save the image-ID-bound tag'
  grep -F -- "docker image save --output " "$CASE_DIR/docker.log" | grep -F -- "$second_tag" >/dev/null \
    || fail 'second preloaded archive did not save the image-ID-bound tag'
  grep -F -- "--image $first_tag" "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'first preloaded host invocation did not receive the image-ID-bound tag'
  grep -F -- "--image $second_tag" "$CASE_DIR/ssh.log" >/dev/null \
    || fail 'second preloaded host invocation did not receive the image-ID-bound tag'
  [[ "$first_tag" != "$second_tag" ]] || fail 'distinct image IDs reused a preloaded release tag'
}

test_total_budget_stops_before_host() {
  setup_case budget
  write_evidence
  if run_controller FAKE_CLOCK_SEQUENCE=0,0,1801 >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'over-budget controller unexpectedly succeeded'
  fi
  ! grep -q 'bash /usr/local/libexec/deploy-sub2api-blue-green-host.sh --mode' "$CASE_DIR/ssh.log" \
    || fail 'over-budget controller invoked host executor'
}

test_per_stage_timeout_stops_before_host() {
  setup_case stage-timeout
  write_evidence
  if run_controller FAKE_DOCKER_MODE=slow RELEASE_STAGE_TIMEOUT_SECONDS=1 >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'stage-timeout controller unexpectedly succeeded'
  fi
  ! grep -q 'bash /usr/local/libexec/deploy-sub2api-blue-green-host.sh --mode' "$CASE_DIR/ssh.log" \
    || fail 'stage-timeout controller invoked host executor'
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
test_migration_hash_matches_go_trim_space_for_unicode_whitespace
test_evidence_rejected_before_transport
test_build_publish_and_host_invocation
test_maintenance_controller_forwards_exact_current_migration_hash
test_downtime_gate_is_propagated_without_retry
test_executor_install_failures_stop_before_build
test_executor_parent_chain_rejects_before_build
test_executor_bootstraps_when_final_file_is_absent
test_total_budget_stops_before_host
test_per_stage_timeout_stops_before_host
test_rejects_unattested_build_context_before_transport
test_preloaded_transport_installs_executor_and_forwards_archive
test_custom_staging_root_and_rehearsal_root_survive_environment_reset
test_preloaded_transport_binds_release_reference_to_image_id
printf 'PASS: tested-tree evidence and blue-green release controller\n'
