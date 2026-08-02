#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
WRAPPER=${WRAPPER_UNDER_TEST:-"$ROOT/ops/provision-billing-source-host.sh"}
FIXTURE=$(mktemp -d "${TMPDIR:-/tmp}/relay-ops-billing-host.XXXXXX")
FIXTURE=$(cd "$FIXTURE" && pwd -P)
trap 'rm -rf -- "$FIXTURE"' EXIT
fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
[[ -f "$WRAPPER" ]] || fail "wrapper does not exist: $WRAPPER"

setup_case() {
  CASE_DIR="$FIXTURE/$1"
  rm -rf -- "$CASE_DIR"
  mkdir -p "$CASE_DIR/bin" "$CASE_DIR/secrets" "$CASE_DIR/sessions"
  printf 'postgres://relay\n' >"$CASE_DIR/db-url"
  printf 'admin-key\n' >"$CASE_DIR/admin-key"
  printf 'bearer\n' >"$CASE_DIR/sessions/upstream-a"
  cat >"$CASE_DIR/declaration.json" <<'JSON'
{"version":1,"actor_user_id":1,"base_url":"https://api.example.invalid","source_name":"example","source_kind":"openai_compatible","billing_account_id":7,"bearer_secret_filename":"upstream-a"}
JSON
  chmod 0600 "$CASE_DIR/db-url" "$CASE_DIR/admin-key" "$CASE_DIR/sessions/upstream-a" "$CASE_DIR/declaration.json"
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
printf '%s\n' "$*" >>"${FAKE_DOCKER_LOG:?}"
case "$*" in
  'context show') printf 'default\n' ;;
  *'image inspect '* ) printf 'sha256:%s\n' "$(printf 'a%.0s' {1..64})" ;;
  *'run --rm'*) [[ "$*" == *'--network sub2api_default'* ]] || exit 1; [[ "$*" == *'--entrypoint /provision-billing-source'* ]] || exit 1; [[ "$*" == *'--read-only'* && "$*" == *'--cap-drop ALL'* ]] || exit 1; printf '{"status":"configured","upstream_id":11,"billing_account_id":7}\n' ;;
  *) exit 64 ;;
esac
SH
  chmod +x "$CASE_DIR/bin/id" "$CASE_DIR/bin/uname" "$CASE_DIR/bin/stat" "$CASE_DIR/bin/docker"
}

run_wrapper() {
  env PATH="$CASE_DIR/bin:$PATH" FAKE_DOCKER_LOG="$CASE_DIR/docker.log" DOCKER_BIN="$CASE_DIR/bin/docker" \
    RELAY_OPS_BILLING_DB_URL_FILE="$CASE_DIR/db-url" RELAY_OPS_BILLING_ADMIN_KEY_FILE="$CASE_DIR/admin-key" \
    RELAY_OPS_BILLING_SESSIONS_DIR="$CASE_DIR/sessions" "$@" bash "$WRAPPER" \
    --image "example.invalid/xingqiao-relay-ops@sha256:$(printf 'a%.0s' {1..64})" --declaration "$CASE_DIR/declaration.json"
}

test_runs_restricted_provision_container() {
  setup_case success
  run_wrapper >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail "wrapper failed: $(cat "$CASE_DIR/stderr")"
  grep -F -- '--network sub2api_default' "$CASE_DIR/docker.log" >/dev/null || fail 'network isolation missing'
  grep -F -- '--entrypoint /provision-billing-source' "$CASE_DIR/docker.log" >/dev/null || fail 'provision entrypoint missing'
  grep -F -- '--user 0:0' "$CASE_DIR/docker.log" >/dev/null || fail 'root container user missing'
  grep -F -- '--read-only' "$CASE_DIR/docker.log" >/dev/null || fail 'read-only container missing'
  grep -F -- '--security-opt no-new-privileges:true' "$CASE_DIR/docker.log" >/dev/null || fail 'no-new-privileges missing'
  grep -F -- '--cap-drop ALL' "$CASE_DIR/docker.log" >/dev/null || fail 'cap drop missing'
  grep -F -- '/run/secrets/billing-source-declaration.json:ro' "$CASE_DIR/docker.log" >/dev/null || fail 'declaration mount missing'
}

test_rejects_mutable_image() {
  setup_case mutable
  if env PATH="$CASE_DIR/bin:$PATH" DOCKER_BIN="$CASE_DIR/bin/docker" RELAY_OPS_BILLING_DB_URL_FILE="$CASE_DIR/db-url" RELAY_OPS_BILLING_ADMIN_KEY_FILE="$CASE_DIR/admin-key" RELAY_OPS_BILLING_SESSIONS_DIR="$CASE_DIR/sessions" bash "$WRAPPER" --image example.invalid/relay-ops:latest --declaration "$CASE_DIR/declaration.json" >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'mutable image unexpectedly accepted'
  fi
  [[ ! -s "$CASE_DIR/docker.log" ]] || fail 'docker invoked before image validation'
}

test_runs_restricted_provision_container
test_rejects_mutable_image
printf 'PASS: relay-ops billing provision host wrapper\n'
