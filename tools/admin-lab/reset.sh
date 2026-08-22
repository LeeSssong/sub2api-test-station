#!/usr/bin/env bash
set -euo pipefail

dry_run=0
while (($#)); do
  case "$1" in
    --dry-run) dry_run=1; shift;;
    --execute) shift;;
    *) echo "usage: $0 [--dry-run|--execute]" >&2; exit 2;;
  esac
done

project=${COMPOSE_PROJECT_NAME:-sub2api-admin-lab}
db=${ADMIN_LAB_DB_NAME:-sub2api_lab}
redis=${ADMIN_LAB_REDIS_HOST:-admin-lab-redis}
lab_only=${LAB_ONLY:-}
[[ "$lab_only" == 1 ]] || { echo 'reset rejected: LAB_ONLY must be 1' >&2; exit 1; }
[[ "$project" == sub2api-admin-lab ]] || { echo 'reset rejected: COMPOSE_PROJECT_NAME must be sub2api-admin-lab' >&2; exit 1; }
[[ "$db" == sub2api_lab ]] || { echo 'reset rejected: ADMIN_LAB_DB_NAME must be sub2api_lab' >&2; exit 1; }
[[ "$redis" == admin-lab-redis ]] || { echo 'reset rejected: ADMIN_LAB_REDIS_HOST must be admin-lab-redis' >&2; exit 1; }

if (( dry_run )); then
  python3 - <<PY
import json
print(json.dumps({"project":"$project","database":"$db","redis":"$redis","seed_version":"v1","result":"dry_run","reset_at":"not-executed"}, separators=(",", ":")))
PY
  exit 0
fi

# Destructive execution is explicit and still fail-closed to the lab Compose project.
command -v docker >/dev/null || { echo 'reset rejected: docker is required' >&2; exit 1; }
compose_file=${ADMIN_LAB_COMPOSE_FILE:-infra/compose.admin-lab.yaml}
env_file=${ADMIN_LAB_ENV_FILE:-infra/.env.admin-lab}
[[ -f "$compose_file" ]] || { echo "reset rejected: compose file not found: $compose_file" >&2; exit 1; }
[[ -f "$env_file" ]] || { echo "reset rejected: env file not found: $env_file" >&2; exit 1; }
# Never use an unscoped docker compose command; project name is pinned above.
docker compose --project-name "$project" --env-file "$env_file" -f "$compose_file" down --volumes --remove-orphans
LAB_ONLY=1 COMPOSE_PROJECT_NAME="$project" ADMIN_LAB_DB_NAME="$db" "$PWD/tools/admin-lab/seed.sh" --version v1
python3 - <<PY
import json, datetime
print(json.dumps({"project":"$project","database":"$db","redis":"$redis","seed_version":"v1","result":"reset","reset_at":datetime.datetime.now(datetime.timezone.utc).isoformat()}, separators=(",", ":")))
PY
