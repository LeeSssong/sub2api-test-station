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
echo 'acceptance auth mode contract: PASS'
