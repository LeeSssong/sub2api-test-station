#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"
SEED=tools/admin-lab/seed.sh
RESET=tools/admin-lab/reset.sh
MANIFEST=tools/admin-lab/seed-manifest.yaml

[[ -x "$SEED" ]] || { echo 'seed script missing or not executable' >&2; exit 1; }
[[ -x "$RESET" ]] || { echo 'reset script missing or not executable' >&2; exit 1; }
grep -Fq 'version: v1' "$MANIFEST"
grep -Fq 'user_a' "$MANIFEST"
grep -Fq 'user_d' "$MANIFEST"
grep -Fq 'stream_interrupted' "$MANIFEST"
grep -Fq 'LAB_ONLY' "$MANIFEST"

out=$(LAB_ONLY=1 COMPOSE_PROJECT_NAME=sub2api-admin-lab ADMIN_LAB_DB_NAME=sub2api_lab \
  "$SEED" --version v1 --dry-run)
grep -Fq '"result":"dry_run"' <<<"$out"
grep -Fq '"seed_version":"v1"' <<<"$out"
grep -Fq '"project":"sub2api-admin-lab"' <<<"$out"

if LAB_ONLY=1 COMPOSE_PROJECT_NAME=sub2api_default ADMIN_LAB_DB_NAME=production "$SEED" --version v1 --dry-run >/dev/null 2>&1; then
  echo 'seed accepted production project' >&2
  exit 1
fi
if LAB_ONLY=1 COMPOSE_PROJECT_NAME=sub2api-admin-lab ADMIN_LAB_DB_NAME=prod "$RESET" --dry-run >/dev/null 2>&1; then
  echo 'reset accepted production database name' >&2
  exit 1
fi

echo 'admin lab seed/reset contract: PASS'
