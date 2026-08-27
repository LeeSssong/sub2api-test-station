#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
cd "$ROOT"
fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

executor=ops/deploy-sub2api-acceptance-host.sh
[[ -x "$executor" ]] || fail 'acceptance host executor is missing or not executable'
compose=infra/compose.acceptance.yaml
grep -Fq "('backend_mode_enabled', 'true')" "$compose" || fail 'backend mode bootstrap missing'
grep -Fq "('registration_enabled', 'false')" "$compose" || fail 'registration mode bootstrap missing'
grep -Fq 'ON CONFLICT (key) DO UPDATE' "$compose" || fail 'bootstrap must be idempotent'
grep -Fq 'acceptance-bootstrap' "$executor" || fail 'executor must run acceptance bootstrap'
grep -Fq 'registration_enabled' "$executor" || fail 'executor must verify registration mode'
grep -Fq 'backend_mode_enabled' "$executor" || fail 'executor must verify backend mode'
grep -Fq '! -L "$deploy_root"' "$executor" || fail 'executor must reject symlink deploy roots'
grep -Fq '/opt/sub2api' "$executor" || fail 'executor must validate acceptance parent path'
grep -Fq 'ACCEPTANCE_LOOPBACK_PORT' "$executor" || fail 'executor must validate the acceptance loopback port'
grep -Fq '/admin/lab/auth/login' "$executor" || fail 'executor must probe the prefixed login route'
rollback_line=$(grep -n '^rollback()' "$executor" | cut -d: -f1)
trap_line=$(grep -n '^trap cleanup EXIT' "$executor" | cut -d: -f1)
first_install_line=$(grep -n 'install -d -m 700 -o root -g root "$deploy_root"' "$executor" | cut -d: -f1)
[[ "$rollback_line" -lt "$trap_line" && "$trap_line" -lt "$first_install_line" ]] \
  || fail 'rollback trap must be armed after rollback definition and before runtime replacement'
echo 'acceptance auth mode contract: PASS'
