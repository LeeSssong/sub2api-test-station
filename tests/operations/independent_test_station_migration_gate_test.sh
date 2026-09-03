#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
COMPOSE="$ROOT/infra/independent-test-station/compose.yaml"
[[ -f "$COMPOSE" ]] || { echo 'FAIL: compose missing'; exit 1; }
python3 - "$COMPOSE" <<'PY'
import sys
from pathlib import Path

text = Path(sys.argv[1]).read_text()
api = text.split("  test-station-api:\n", 1)[1].split("  test-station-worker:\n", 1)[0]
if "test-station-worker: {condition: service_healthy}" not in api:
    raise SystemExit("FAIL: API is not gated on worker migration readiness")
print("PASS: independent test station migration gate")
PY
