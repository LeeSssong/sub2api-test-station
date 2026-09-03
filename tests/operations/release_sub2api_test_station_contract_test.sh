#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P); SCRIPT="$ROOT/ops/release-sub2api-test-station.sh"
[[ -x "$SCRIPT" ]] || { echo 'FAIL: orchestrator missing'; exit 1; }
bash -n "$SCRIPT"
grep -q 'target=.*sub2api-test-station' "$SCRIPT"
grep -q 'branch --show-current.*main' "$SCRIPT"
grep -q 'rev-parse origin/main' "$SCRIPT"
grep -q 'deploy-sub2api-test-station-host.sh' "$SCRIPT"
printf 'PASS: independent test station release contract\n'
