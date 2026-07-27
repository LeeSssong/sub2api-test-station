#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"
fail(){ printf 'FAIL: %s\n' "$1" >&2; exit 1; }
require(){ rg -Fq -- "$1" "$2" || fail "missing $1 in $2"; }
forbid(){ ! rg -Fq -- "$1" "$2" || fail "forbidden $1 in $2"; }
COMPOSE_FILE=${RELAY_OPS_COMPOSE_FILE:-infra/compose.yaml}
[[ -f "$COMPOSE_FILE" ]] || fail "missing Compose file $COMPOSE_FILE"
relay_ops_block=$(awk '
  $0 == "  relay-ops:" { in_relay=1; next }
  in_relay && $0 ~ /^  [a-zA-Z0-9_-]+:/ { exit }
  in_relay { print }
' "$COMPOSE_FILE")
[[ -n "$relay_ops_block" ]] || fail "missing relay-ops service block in $COMPOSE_FILE"
require_relay_ops(){ rg -Fq -- "$1" <<<"$relay_ops_block" || fail "missing $1 in relay-ops service"; }
forbid_relay_ops(){ ! rg -Fq -- "$1" <<<"$relay_ops_block" || fail "forbidden $1 in relay-ops service"; }

[[ -f infra/Dockerfile.relay-ops ]] || fail 'missing relay-ops Dockerfile'
require 'relay-ops:' "$COMPOSE_FILE"
require_relay_ops 'dockerfile: infra/Dockerfile.relay-ops'
require_relay_ops 'expose: ["8100"]'
require_relay_ops 'read_only: true'
require_relay_ops 'user: "10002:10002"'
require_relay_ops 'security_opt: [no-new-privileges:true]'
require_relay_ops 'cap_drop: [ALL]'
require_relay_ops 'RELAY_OPS_MODE: ${RELAY_OPS_MODE:-read_only}'
require_relay_ops 'RELAY_OPS_ACCOUNT_QUALITY_RESULT_FILE: /run/relay-ops/account-quality/account-quality-result.json'
require_relay_ops 'RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE: ${RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE:-}'
forbid_relay_ops 'D04'
forbid_relay_ops 'RELAY_OPS_MODEL_RELEASE_RESULT_FILE'
require_relay_ops 'RELAY_OPS_CANDIDATE_SECRET_DIR: /var/lib/relay-ops/candidate-keys'
require_relay_ops 'RELAY_OPS_FEISHU_WEBHOOK_FILE: ${RELAY_OPS_FEISHU_WEBHOOK_FILE:-}'
require_relay_ops 'RELAY_OPS_FEISHU_COMMAND_MODE: ${RELAY_OPS_FEISHU_COMMAND_MODE:-disabled}'
require_relay_ops 'RELAY_OPS_FEISHU_APP_ID_FILE: ${RELAY_OPS_FEISHU_APP_ID_FILE:-}'
require_relay_ops 'RELAY_OPS_FEISHU_APP_SECRET_FILE: ${RELAY_OPS_FEISHU_APP_SECRET_FILE:-}'
require_relay_ops 'RELAY_OPS_FEISHU_VERIFICATION_TOKEN_FILE: ${RELAY_OPS_FEISHU_VERIFICATION_TOKEN_FILE:-}'
require_relay_ops 'RELAY_OPS_FEISHU_ENCRYPT_KEY_FILE: ${RELAY_OPS_FEISHU_ENCRYPT_KEY_FILE:-}'
require_relay_ops 'RELAY_OPS_FEISHU_ROUTING_FILE: ${RELAY_OPS_FEISHU_ROUTING_FILE:-}'
require_relay_ops 'RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE: ${RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE:-}'
require_relay_ops '${RELAY_OPS_FEISHU_APP_ID_HOST_FILE:-/dev/null}:/run/secrets/feishu-app-id:ro'
require_relay_ops '${RELAY_OPS_FEISHU_WEBHOOK_HOST_FILE:-/dev/null}:/run/secrets/feishu-webhook:ro'
require_relay_ops '${RELAY_OPS_FEISHU_APP_SECRET_HOST_FILE:-/dev/null}:/run/secrets/feishu-app-secret:ro'
require_relay_ops '${RELAY_OPS_FEISHU_VERIFICATION_TOKEN_HOST_FILE:-/dev/null}:/run/secrets/feishu-verification-token:ro'
require_relay_ops '${RELAY_OPS_FEISHU_ENCRYPT_KEY_HOST_FILE:-/dev/null}:/run/secrets/feishu-encrypt-key:ro'
require_relay_ops '${RELAY_OPS_FEISHU_ROUTING_HOST_FILE:-/dev/null}:/run/secrets/feishu-routing.json:ro'
require_relay_ops '${RELAY_OPS_FEISHU_ALERT_CHAT_ID_HOST_FILE:-/dev/null}:/run/secrets/feishu-alert-chat-id:ro'
require_relay_ops '/run/secrets/feishu-app-id:ro'
require_relay_ops '/run/secrets/feishu-app-secret:ro'
require_relay_ops '/run/secrets/feishu-verification-token:ro'
require_relay_ops '/run/secrets/feishu-encrypt-key:ro'
require_relay_ops '/run/secrets/feishu-routing.json:ro'
require_relay_ops '/run/secrets/feishu-alert-chat-id:ro'
require_relay_ops 'mem_limit: 384m'
require_relay_ops 'cpus: 0.75'
require_relay_ops '${RELAY_OPS_CANDIDATE_KEYS_HOST_DIR:-./secrets/candidate-keys}:/run/secrets/candidates:ro'
require_relay_ops '${RELAY_OPS_CANDIDATE_MANAGED_KEYS_HOST_DIR:-./secrets/candidate-managed-keys}:/var/lib/relay-ops/candidate-keys:rw'
require_relay_ops '${RELAY_OPS_ACCOUNT_QUALITY_RESULT_HOST_DIR:-/dev/null}:/run/relay-ops/account-quality:ro'
require_relay_ops '${RELAY_OPS_UPSTREAM_GROUP_MAPPING_HOST_FILE:-/dev/null}:/run/relay-ops/upstream-group-mapping.json:ro'
forbid_relay_ops 'RELAY_OPS_MODEL_RELEASE_RESULT_HOST'
require_relay_ops 'test: ["CMD", "wget", "-q", "-T", "5", "-O", "/dev/null", "http://localhost:8100/healthz"]'
require 'USER 10002:10002' infra/Dockerfile.relay-ops
require 'ENTRYPOINT ["/relay-ops"]' infra/Dockerfile.relay-ops
forbid 'd04-readiness-snapshot' infra/Dockerfile.relay-ops
require 'ops/collect-account-quality-pulse.rb' infra/Dockerfile.relay-ops
require 'ops/analyze-account-monitor.rb' infra/Dockerfile.relay-ops
require 'config/upstream-benchmarks/quality-first-fast-v1.yaml' infra/Dockerfile.relay-ops
require 'path /pricing /relay-ops/static/*' infra/Caddyfile
require 'path /ops /ops/* /relay-ops/api/*' infra/Caddyfile
require '@relay_ops_feishu_command {' infra/Caddyfile
require 'path /relay-ops/api/feishu/events' infra/Caddyfile
require 'reverse_proxy @relay_ops_feishu_command relay-ops:8100' infra/Caddyfile
require 'not {' infra/Caddyfile
require 'reverse_proxy @relay_ops_public relay-ops:8100' infra/Caddyfile
require 'reverse_proxy @relay_ops_admin relay-ops:8100' infra/Caddyfile
require 'reverse_proxy sub2api:8080' infra/Caddyfile
forbid 'internal-test-service' infra/Caddyfile
forbid 'path /api/v1/auth/register /api/v1/auth/login /api/v1/auth/login/2fa' infra/Caddyfile
forbid 'path /api/v1/settings/public' infra/Caddyfile
forbid '内测开放状态' relay-ops-service/internal/http/templates/ops.html
forbid '当前活动上游' relay-ops-service/internal/http/templates/ops.html
require '站内运行' relay-ops-service/internal/http/templates/ops.html
require '公开分组' relay-ops-service/internal/http/templates/ops.html
require '当前调度账号' relay-ops-service/internal/http/templates/ops.html
require '上游账号质量' relay-ops-service/internal/http/templates/ops.html
require '错误率' relay-ops-service/internal/http/templates/ops.html
require '稳定性' relay-ops-service/internal/http/templates/ops.html
require 'TTFT P95' relay-ops-service/internal/http/templates/ops.html
# 倍率列已从账号质量表移除：质量视图只讲稳定性与延时，倍率属成本维度，
# 且原数据源 accounts.rate_multiplier 生产恒为 1、已废弃。倍率监控改由
# opsmonitor 消费 schema v2 的可信倍率（0.05x-0.25x）。
forbid '倍率' relay-ops-service/internal/http/templates/ops.html
forbid '模型版本' relay-ops-service/internal/http/templates/ops.html
require '<details class="technical-details">' relay-ops-service/internal/http/templates/ops.html
require '.ops-main section{border-top:1px solid var(--rule);padding-top:16px;min-width:0}' relay-ops-service/internal/http/static/app.css
require '/relay-ops/static/app.css?v=20260723-site-runtime-1' relay-ops-service/internal/http/templates/ops.html
require '/relay-ops/static/ops-admin.js?v=20260723-site-runtime-1' relay-ops-service/internal/http/templates/ops.html
require 'id="modeloc-reminder"' relay-ops-service/internal/http/templates/ops.html
require 'MODELOC 真实性报告尚未配置' relay-ops-service/internal/http/templates/ops.html
require '/home-assets/site-config.json' relay-ops-service/internal/http/templates/ops.html
require '/relay-ops/static/app.css?v=20260723-site-runtime-1' relay-ops-service/internal/http/templates/ops-bootstrap.html
require '/relay-ops/static/ops.js?v=20260723-site-runtime-1' relay-ops-service/internal/http/templates/ops-bootstrap.html
require "cache: 'no-store'" relay-ops-service/internal/http/static/ops.js
require 'window.setTimeout(refresh, 30000)' relay-ops-service/internal/http/static/ops-admin.js
require '/home-assets/site-config.json' relay-ops-service/internal/http/static/ops-admin.js
require 'thirdPartyReports' relay-ops-service/internal/http/static/ops-admin.js
require 'MODELOC' relay-ops-service/internal/http/static/ops-admin.js
require "protocol === 'https:'" relay-ops-service/internal/http/static/ops-admin.js
require "window.location.replace('/404')" relay-ops-service/internal/http/static/ops.js
require 'RequireHiddenAdmin(dependencies.Auth' relay-ops-service/internal/http/server.go
require 'Native: reader' relay-ops-service/internal/app/app.go
require 'ListAccountMonitors' relay-ops-service/internal/sub2api/client.go
require '/api/v1/admin/account-monitors' relay-ops-service/internal/sub2api/client.go
forbid '<form' relay-ops-service/internal/http/templates/ops.html
forbid '<input' relay-ops-service/internal/http/templates/ops.html
forbid '<select' relay-ops-service/internal/http/templates/ops.html
forbid '<button' relay-ops-service/internal/http/templates/ops.html
forbid 'candidate-source-form' relay-ops-service/internal/http/templates/ops.html
forbid 'production-source-form' relay-ops-service/internal/http/templates/ops.html
forbid 'billing-session-form' relay-ops-service/internal/http/templates/ops.html
forbid '/relay-ops/api/candidates' relay-ops-service/internal/http/static/ops-admin.js
forbid '/relay-ops/api/upstreams' relay-ops-service/internal/http/static/ops-admin.js
forbid '/relay-ops/api/acceptance' relay-ops-service/internal/http/static/ops-admin.js
forbid 'mux.Handle("POST /relay-ops/api/candidates' relay-ops-service/internal/http/server.go
forbid 'mux.Handle("POST /relay-ops/api/upstreams' relay-ops-service/internal/http/server.go
forbid 'mux.Handle("POST /relay-ops/api/acceptance' relay-ops-service/internal/http/server.go
forbid 'mux.Handle("POST /relay-ops/api/model' relay-ops-service/internal/http/server.go
forbid 'mux.Handle("PUT /relay-ops/api/model' relay-ops-service/internal/http/server.go
forbid 'mux.Handle("DELETE /relay-ops/api/model' relay-ops-service/internal/http/server.go

node <<'NODE'
const fs = require('node:fs')
const vm = require('node:vm')

const source = fs.readFileSync('relay-ops-service/internal/http/static/ops-admin.js', 'utf8')
const match = source.match(/(const hasValidModelocReport = \(config\) => \{[\s\S]*?\n  \})\n\n  const updateModelocReminder/)
if (!match) throw new Error('MODELOC validator is not available for contract testing')

const context = { URL }
vm.runInNewContext(`${match[1]}\nglobalThis.validate = hasValidModelocReport`, context)

const valid = {
  version: 1,
  thirdPartyReports: [{
    id: 'modeloc-1', provider: 'MODELOC', title: '模型真实性报告',
    url: 'https://modeloc.com/reports/1', status: ' verified ',
  }],
}
if (!context.validate(valid)) throw new Error('complete HTTPS MODELOC report should clear the reminder')

const incomplete = structuredClone(valid)
delete incomplete.thirdPartyReports[0].title
if (context.validate(incomplete)) throw new Error('incomplete MODELOC report must keep the reminder')

const unsafe = structuredClone(valid)
unsafe.thirdPartyReports[0].url = 'http://modeloc.com/reports/1'
if (context.validate(unsafe)) throw new Error('HTTP MODELOC report must keep the reminder')
NODE

ports_owner=$(awk '/^  [a-zA-Z0-9_-]+:/{service=$1} /^    ports:/{print service}' "$COMPOSE_FILE")
[[ "$ports_owner" == 'caddy:' ]] || fail 'only caddy may publish host ports'
docker compose \
  --env-file infra/.env.example \
  --env-file config/releases/sub2api.env \
  -f "$COMPOSE_FILE" \
  config --quiet || fail 'compose config failed'
printf 'PASS: relay-ops container and routing contracts\n'
