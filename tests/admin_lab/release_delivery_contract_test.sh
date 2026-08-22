#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
cd "$ROOT"

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

controller=ops/release-admin-lab.sh
executor=ops/deploy-admin-lab-host.sh
compose=infra/compose.admin-lab.yaml
caddy=infra/Caddyfile
[[ -f "$controller" ]] || fail 'admin lab release controller is missing'
[[ -x "$controller" ]] || fail 'admin lab release controller is not executable'
[[ -f "$executor" ]] || fail 'admin lab host executor is missing'
[[ -x "$executor" ]] || fail 'admin lab host executor is not executable'
[[ -f "$compose" ]] || fail 'admin lab compose is missing'
[[ -f "$caddy" ]] || fail 'Caddyfile is missing'

for needle in \
  'compose.admin-lab.yaml' \
  'infra/Caddyfile' \
  'Dockerfile.frontend' \
  'gateway.conf' \
  'mock_server.py' \
  'ADMIN_LAB_IMAGE' \
  'ADMIN_LAB_FRONTEND_IMAGE' \
  'admin-lab-bundle' \
  'sha256'; do
  grep -Fq "$needle" "$controller" || fail "release controller missing contract: $needle"
done

for needle in '/admin/lab*' 'handle @admin_lab_app'; do
  grep -Fq "$needle" "$caddy" || fail "Caddyfile missing lab routing contract: $needle"
done

for needle in \
  'AUTO_SETUP: "true"' \
  'admin-lab-app-data:/app/data' \
  'admin-lab-worker: {condition: service_healthy}'; do
  grep -Fq "$needle" "$compose" || fail "admin lab compose missing contract: $needle"
done

for needle in \
  'sub2api-admin-lab' \
  'ADMIN_LAB_ENV' \
  'effective_env' \
  'ADMIN_LAB_DB_PASSWORD' \
  'ADMIN_LAB_REDIS_PASSWORD' \
  'install -o root -g root -m 0600' \
  'up -d --no-build --wait' \
  'admin-lab-api' \
  'admin-lab-worker' \
  'admin-lab-gateway' \
  'admin-lab-frontend' \
  '/admin/lab/assets/' \
  'caddy validate' \
  'caddy reload' \
  'Caddyfile.backup' \
  'cat "$stage/infra/Caddyfile" >"$deploy_root/Caddyfile"' \
  '主站 HTML' \
  'rollback'; do
  grep -Fq "$needle" "$executor" || fail "host executor missing contract: $needle"
done

echo 'admin lab release delivery contract: PASS'
