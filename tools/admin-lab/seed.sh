#!/usr/bin/env bash
set -euo pipefail

version=v1
dry_run=0
while (($#)); do
  case "$1" in
    --version) version=${2:?missing version}; shift 2;;
    --dry-run) dry_run=1; shift;;
    *) echo "usage: $0 [--version v1] [--dry-run]" >&2; exit 2;;
  esac
done

project=${COMPOSE_PROJECT_NAME:-sub2api-admin-lab}
db=${ADMIN_LAB_DB_NAME:-sub2api_lab}
lab_only=${LAB_ONLY:-}
manifest=${ADMIN_LAB_SEED_MANIFEST:-$(cd "$(dirname "$0")" && pwd)/seed-manifest.yaml}

reject() { echo "seed rejected: $1" >&2; exit 1; }
[[ "$lab_only" == 1 ]] || reject 'LAB_ONLY must be 1'
[[ "$project" == sub2api-admin-lab ]] || reject 'COMPOSE_PROJECT_NAME must be sub2api-admin-lab'
[[ "$db" == sub2api_lab ]] || reject 'ADMIN_LAB_DB_NAME must be sub2api_lab'
[[ "$version" == v1 ]] || reject 'unsupported seed version'
[[ -f "$manifest" ]] || reject 'seed manifest not found'
grep -Fq "version: $version" "$manifest" || reject 'manifest version mismatch'
grep -Fq 'source: LAB_ONLY' "$manifest" || reject 'manifest is not LAB_ONLY'

python3 - "$project" "$db" "$version" "$dry_run" "$manifest" <<'PY'
import json, sys
project, db, version, dry_run, manifest = sys.argv[1:]
with open(manifest, encoding='utf-8') as f:
    text = f.read()
# Versioned fixture metadata is intentionally data-only; future ledger tasks own schema writes.
required = ["user_a", "user_b", "user_c", "user_d", "stream_interrupted", "crosses_paid_and_bonus"]
missing = [x for x in required if x not in text]
if missing:
    raise SystemExit("seed rejected: manifest missing " + ",".join(missing))
result = {
    "project": project,
    "database": db,
    "seed_version": version,
    "source": "LAB_ONLY",
    "result": "dry_run" if dry_run == "1" else "prepared",
    "fixture_manifest": manifest,
    "records": {"users": 4, "groups": 2, "models": 3, "usage_scenarios": 4, "payment_scenarios": 3, "cost_scenarios": 2},
}
print(json.dumps(result, ensure_ascii=False, separators=(",", ":")))
PY
