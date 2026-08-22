#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"
CADDY=infra/Caddyfile
for needle in \
  '@admin_lab_root path /admin/lab' \
  '@admin_lab_api path /admin/lab/api/*' \
  '@admin_lab_ws path /admin/lab/ws/*' \
  'reverse_proxy admin-lab-api:8080' \
  'reverse_proxy admin-lab-frontend:4173' \
  'header_up X-Admin-Lab-Only 1'; do
  grep -Fq "$needle" "$CADDY" || { echo "missing Caddy lab route: $needle" >&2; exit 1; }
done
lab_line=$(grep -n '@admin_lab_root' "$CADDY" | head -n1 | cut -d: -f1)
prod_line=$(grep -nF 'reverse_proxy {$SUB2API_ACTIVE_UPSTREAM:sub2api-blue:8080} {' "$CADDY" | tail -n1 | cut -d: -f1)
[[ "$lab_line" -lt "$prod_line" ]] || { echo 'lab route must precede production fallback' >&2; exit 1; }
if grep -nE '/admin/lab.*reverse_proxy \{\$SUB2API_ACTIVE_UPSTREAM' "$CADDY"; then
  echo 'lab matcher points to production upstream' >&2
  exit 1
fi
echo 'admin lab Caddy route contract: PASS'
