#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

require() {
  command -v "$1" >/dev/null || { echo "missing required command: $1" >&2; exit 1; }
}

require docker
require python3

ENV_FILE=infra/.env.admin-lab.example
COMPOSE_FILE=infra/compose.admin-lab.yaml

# Static/in-process smoke is safe to run without starting a service or touching production.
docker compose --project-name sub2api-admin-lab --env-file "$ENV_FILE" -f "$COMPOSE_FILE" config --quiet

for contract in \
  tests/admin_lab/native_reuse_inventory.sh \
  tests/admin_lab/compose_contract_test.sh \
  tests/admin_lab/caddy_route_test.sh \
  tests/admin_lab/auth_isolation_test.sh \
  tests/admin_lab/mock_egress_test.sh \
  tests/admin_lab/seed_reset_test.sh; do
  bash "$contract"
done

seed_output=$(LAB_ONLY=1 COMPOSE_PROJECT_NAME=sub2api-admin-lab ADMIN_LAB_DB_NAME=sub2api_lab \
  tools/admin-lab/seed.sh --version v1 --dry-run)
python3 - "$seed_output" <<'PY'
import json, sys
payload = json.loads(sys.argv[1])
assert payload["result"] == "dry_run"
assert payload["source"] == "LAB_ONLY"
assert payload["project"] == "sub2api-admin-lab"
assert payload["database"] == "sub2api_lab"
assert payload["records"]["users"] == 4
PY

# The lab matcher must be ahead of, and never target, the production fallback.
lab_line=$(grep -n '@admin_lab_root' infra/Caddyfile | head -n1 | cut -d: -f1)
prod_line=$(grep -nF 'reverse_proxy {$SUB2API_ACTIVE_UPSTREAM:sub2api-blue:8080} {' infra/Caddyfile | tail -n1 | cut -d: -f1)
[[ "$lab_line" -lt "$prod_line" ]] || { echo 'lab route is after production fallback' >&2; exit 1; }
grep -Fq 'reverse_proxy admin-lab-gateway:8088' infra/Caddyfile
! grep -Fq '@admin_lab_app.*SUB2API_ACTIVE_UPSTREAM' infra/Caddyfile

if [[ "${LAB_SMOKE_LIVE:-0}" == 1 ]]; then
  require curl
  base=${LAB_SMOKE_BASE_URL:-https://api.example.com}
  status=$(curl -ksS -o /dev/null -w '%{http_code}' "$base/admin/lab/")
  [[ "$status" == 200 || "$status" == 401 || "$status" == 403 || "$status" == 503 ]] || {
    echo "unexpected lab live status: $status" >&2
    exit 1
  }
  prod_status=$(curl -ksS -o /dev/null -w '%{http_code}' "$base/healthz")
  [[ "$prod_status" == 200 ]] || { echo "production health failed during lab smoke: $prod_status" >&2; exit 1; }
fi

echo 'admin lab smoke: PASS (static/in-process)'
