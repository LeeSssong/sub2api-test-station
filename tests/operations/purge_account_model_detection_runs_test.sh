#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
SCRIPT="$ROOT/ops/purge-account-model-detection-runs.sh"
fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

[[ -f "$SCRIPT" ]] || fail 'purge script is missing'
bash -n "$SCRIPT"
grep -Fq 'T77_EXPECTED_ROWS' "$SCRIPT" || fail 'expected-row guard is missing'
grep -Fq 'T77_BACKUP_DIR' "$SCRIPT" || fail 'backup directory guard is missing'
grep -Fq 'pg_dump' "$SCRIPT" || fail 'recoverable export is missing'
grep -Fq 'ACCESS EXCLUSIVE' "$SCRIPT" || fail 'table lock is missing'
grep -Fq "detector_version IS DISTINCT FROM '4.1.1'" "$SCRIPT" || fail 'target predicate is missing'
grep -Fq 'account_model_detection_runs' "$SCRIPT" || fail 'target table is missing'
grep -Fq 'foreign-key' "$SCRIPT" || fail 'foreign-key guard is missing'
grep -Fq 'ON_ERROR_STOP=1' "$SCRIPT" || fail 'transaction failure guard is missing'
grep -Fq 'COMMIT;' "$SCRIPT" || fail 'transaction commit is missing'
printf 'purge account model detection runs contract passed\n'
