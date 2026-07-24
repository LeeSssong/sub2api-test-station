#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"
fail(){ printf 'FAIL: %s\n' "$1" >&2; exit 1; }
require(){ rg -Fq -- "$1" "$2" || fail "missing $1 in $2"; }

require 'internal-test-service:' infra/compose.yaml
require 'dockerfile: infra/Dockerfile.internal-test' infra/compose.yaml
require 'D04_MODE: ${D04_MODE:-read_only}' infra/compose.yaml
require 'D04_TOTAL_BUDGET_USD: ${D04_TOTAL_BUDGET_USD}' infra/compose.yaml
require 'D04_REGISTRATION_OPEN: ${D04_REGISTRATION_OPEN:-false}' infra/compose.yaml
require 'D04_DAILY_LOGIN_CREDIT_USD: ${D04_DAILY_LOGIN_CREDIT_USD:-20}' infra/compose.yaml
require 'D04_COST_POLICY_QUALIFIED: ${D04_COST_POLICY_QUALIFIED:-false}' infra/compose.yaml
require 'D04_FEISHU_APP_ID_FILE: ${D04_FEISHU_APP_ID_FILE:-}' infra/compose.yaml
require 'D04_FEISHU_APP_SECRET_FILE: ${D04_FEISHU_APP_SECRET_FILE:-}' infra/compose.yaml
require 'D04_FEISHU_ALERT_CHAT_ID_FILE: ${D04_FEISHU_ALERT_CHAT_ID_FILE:-}' infra/compose.yaml
require 'internal_test_data:/var/lib/internal-test' infra/compose.yaml
require 'read_only: true' infra/compose.yaml
require 'security_opt: [no-new-privileges:true]' infra/compose.yaml
require 'cap_drop: [ALL]' infra/compose.yaml
require 'test: ["CMD", "/internal-test-service", "healthcheck"]' infra/compose.yaml
require 'golang@sha256:8bee1901f1e530bfb4a7850aa7a479d17ae3a18beb6e09064ed54cfd245b7191' infra/Dockerfile.internal-test
require 'USER 10001:0' infra/Dockerfile.internal-test
require 'method POST' infra/Caddyfile
require 'path /api/v1/auth/register /api/v1/auth/login /api/v1/auth/login/2fa' infra/Caddyfile
require 'reverse_proxy @d04_auth internal-test-service:8090' infra/Caddyfile
require 'path /api/v1/settings/public' infra/Caddyfile
require 'reverse_proxy @d04_public_settings internal-test-service:8090' infra/Caddyfile
require '@d04_retired path /internal-test/*' infra/Caddyfile
require 'respond @d04_retired 404' infra/Caddyfile
require 'D04_REGISTRATION_METHOD_DISABLED' infra/Caddyfile
require '首发计划仅开放邮箱密码注册' infra/Caddyfile
require 'reverse_proxy sub2api:8080' infra/Caddyfile
require 'flush_interval -1' infra/Caddyfile

ports_owner=$(awk '/^  [a-zA-Z0-9_-]+:/{service=$1} /^    ports:/{print service}' infra/compose.yaml)
[[ "$ports_owner" == 'caddy:' ]] || fail 'only caddy may publish host ports'

docker compose \
  --env-file infra/.env.example \
  --env-file config/releases/sub2api.env \
  -f infra/compose.yaml \
  config --quiet || fail 'compose config failed'
docker compose -f infra/compose.d04-read-only.yaml config --quiet || fail 'D04 read-only deployment config failed'
docker compose -f infra/compose.d04-read-only.yaml -f infra/compose.d04-acceptance.yaml config --quiet || fail 'D04 acceptance overlay config failed'
docker compose -f infra/compose.d04-read-only.yaml -f infra/compose.d04-launch.yaml config --quiet || fail 'D04 launch overlay config failed'
require 'D04_MODE: read_only' infra/compose.d04-read-only.yaml
require 'D04_REGISTRATION_OPEN: "false"' infra/compose.d04-read-only.yaml
require 'D04_DAILY_LOGIN_CREDIT_USD: "20"' infra/compose.d04-read-only.yaml
require 'D04_COST_POLICY_QUALIFIED: "false"' infra/compose.d04-read-only.yaml
require 'user: "10001:0"' infra/compose.d04-read-only.yaml
require 'read_only: true' infra/compose.d04-read-only.yaml
require 'nocopy: true' infra/compose.d04-read-only.yaml
require 'external: true' infra/compose.d04-read-only.yaml
require 'D04_MODE: write' infra/compose.d04-acceptance.yaml
require 'D04_REGISTRATION_OPEN: "true"' infra/compose.d04-acceptance.yaml
require 'D04_MAX_USERS: "15"' infra/compose.d04-acceptance.yaml
require 'D04_DAILY_LOGIN_CREDIT_USD: "20"' infra/compose.d04-acceptance.yaml
require 'D04_TOTAL_BUDGET_USD: "2.00"' infra/compose.d04-acceptance.yaml
require 'D04_BUDGET_COST_BPS: "1000"' infra/compose.d04-acceptance.yaml
require 'D04_COST_POLICY_QUALIFIED: "true"' infra/compose.d04-acceptance.yaml
require 'D04_COST_POLICY_ID: d04-acceptance-conservative-1000bps-20260721' infra/compose.d04-acceptance.yaml
require 'D04_MODE: write' infra/compose.d04-launch.yaml
require 'D04_REGISTRATION_OPEN: "true"' infra/compose.d04-launch.yaml
require 'D04_MAX_USERS: "15"' infra/compose.d04-launch.yaml
require 'D04_DAILY_LOGIN_CREDIT_USD: "20"' infra/compose.d04-launch.yaml
require 'D04_TOTAL_BUDGET_USD: "100.00"' infra/compose.d04-launch.yaml
require 'D04_BUDGET_COST_BPS: "1000"' infra/compose.d04-launch.yaml
require 'D04_COST_POLICY_QUALIFIED: "true"' infra/compose.d04-launch.yaml
require 'D04_COST_POLICY_ID: d04-active-upstream-conservative-1000bps-v2' infra/compose.d04-launch.yaml
require 'D04-LIGHTWEIGHT-LAUNCH-v2' docs/runbooks/operations-and-incident-response.md
require 'evaluate-d04-lightweight-launch-readiness.rb' docs/runbooks/operations-and-incident-response.md
require 'backup-d04-account-data.sh' docs/runbooks/operations-and-incident-response.md
require 'launch_approved' docs/runbooks/operations-and-incident-response.md
require 'compose.d04-read-only.yaml' docs/runbooks/operations-and-incident-response.md

if rg -n '/api/v1/admin/(groups|accounts|models)' internal-test-service >/dev/null; then
  fail 'D04 service must not contain route/account/model Admin API access'
fi
if rg -n 'reverse_proxy @d04_internal|/internal-test/(join|api/invitations|api/checkin)' infra/Caddyfile internal-test-service/internal/http/server.go >/dev/null; then
  fail 'retired invitation, join, or manual check-in surface is still routed'
fi
printf 'PASS: internal test service contracts\n'
