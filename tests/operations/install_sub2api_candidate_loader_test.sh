#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
INSTALLER="$ROOT/ops/install-sub2api-candidate-loader.sh"
WRAPPER="$ROOT/ops/sub2api-candidate-ssh.sh"
ENVIRONMENT="$ROOT/infra/sub2api-candidate-loader.env.example"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

for file in "$INSTALLER" "$WRAPPER" "$ENVIRONMENT"; do
  [[ -f "$file" ]] || fail "missing ${file#"$ROOT/"}"
done

for required in \
  'uname -s' \
  '/opt/sub2api/production' \
  'docker context show' \
  'SUB2API_CANDIDATE_LOADER_BINARY' \
  'SUB2API_CANDIDATE_PUBLIC_KEY_FILE' \
  '/usr/local/libexec/sub2api-candidate-loader' \
  '/usr/local/libexec/sub2api-candidate-ssh' \
  '/var/lib/sub2api-candidate-loader' \
  '/etc/sub2api/sub2api-candidate-loader.env' \
  '/etc/sudoers.d/sub2api-candidate-loader' \
  'visudo -cf' \
  'restrict,command="/usr/local/libexec/sub2api-candidate-ssh"'; do
  rg -Fq "$required" "$INSTALLER" || fail "installer missing $required"
done

for required in \
  'SUB2API_CANDIDATE_REGISTRY_USER=LeeSssong' \
  'SUB2API_CANDIDATE_REGISTRY=ghcr.io/leesssong/xingqiao-sub2api'; do
  rg -Fq "$required" "$ENVIRONMENT" || fail "environment missing $required"
done

for forbidden in \
  'docker compose' '/system/update' 'psql' 'redis-cli' 'docker restart' \
  'docker stop' 'docker kill' 'docker system prune' 'docker image prune' \
  'Bearer ' 'token=' 'password='; do
  ! rg -Fiq "$forbidden" "$INSTALLER" "$WRAPPER" "$ENVIRONMENT" \
    || fail "packaging contains forbidden content: $forbidden"
done

if [[ "$(uname -s)" != Linux ]]; then
  marker=$(mktemp)
  trap 'rm -f "$marker" "$marker.out"' EXIT
  if SUB2API_CANDIDATE_INSTALL_TEST_MARKER="$marker" bash "$INSTALLER" >"$marker.out" 2>&1; then
    fail 'installer accepted non-Linux host'
  fi
  [[ ! -s "$marker" ]] || fail 'installer mutated before host refusal'
fi

printf 'PASS: candidate loader packaging contracts\n'
