#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

fail(){ printf 'FAIL: %s\n' "$1" >&2; exit 1; }
require(){ rg -Fq -- "$1" "$2" || fail "missing $1 in $2"; }
require_line(){ rg -Fxq -- "$1" "$2" || fail "missing exact line in $2: $1"; }
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
require_relay_ops 'dockerfile: infra/Dockerfile.relay-ops'
require_relay_ops 'expose: ["8100"]'
require_relay_ops 'read_only: true'
require_relay_ops 'user: "10002:10002"'
require_relay_ops 'security_opt: [no-new-privileges:true]'
require_relay_ops 'cap_drop: [ALL]'
require_relay_ops 'RELAY_OPS_MODE: ${RELAY_OPS_MODE:-read_only}'
require_relay_ops 'RELAY_OPS_ACCOUNT_QUALITY_RESULT_FILE: /run/relay-ops/account-quality/account-quality-result.json'
require_relay_ops 'RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE: ${RELAY_OPS_UPSTREAM_GROUP_MAPPING_FILE:-}'
require_relay_ops 'RELAY_OPS_CANDIDATE_SECRET_DIR: /var/lib/relay-ops/candidate-keys'
require_relay_ops 'RELAY_OPS_FEISHU_WEBHOOK_FILE: ${RELAY_OPS_FEISHU_WEBHOOK_FILE:-}'
require_relay_ops 'RELAY_OPS_FEISHU_APP_ID_FILE: ${RELAY_OPS_FEISHU_APP_ID_FILE:-}'
require_relay_ops 'RELAY_OPS_FEISHU_APP_SECRET_FILE: ${RELAY_OPS_FEISHU_APP_SECRET_FILE:-}'
require_relay_ops 'RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE: ${RELAY_OPS_FEISHU_ALERT_CHAT_ID_FILE:-}'
require_relay_ops 'RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE: ${RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE:-}'
require_relay_ops 'RELAY_OPS_NOTIFICATION_POLICY_FILE: ${RELAY_OPS_NOTIFICATION_POLICY_FILE:-}'
require_relay_ops '${RELAY_OPS_FEISHU_APP_ID_HOST_FILE:-/dev/null}:/run/secrets/feishu-app-id:ro'
require_relay_ops '${RELAY_OPS_FEISHU_APP_SECRET_HOST_FILE:-/dev/null}:/run/secrets/feishu-app-secret:ro'
require_relay_ops '${RELAY_OPS_FEISHU_ALERT_CHAT_ID_HOST_FILE:-/dev/null}:/run/secrets/feishu-alert-chat-id:ro'
require_relay_ops '${RELAY_OPS_FEISHU_ALERT_RECIPIENTS_HOST_FILE:-/dev/null}:/run/secrets/feishu-alert-recipients.json:ro'
require_relay_ops '${RELAY_OPS_NOTIFICATION_POLICY_HOST_FILE:-/dev/null}:/run/relay-ops/notification-policy.json:ro'
require_relay_ops '${RELAY_OPS_ACCOUNT_QUALITY_RESULT_HOST_DIR:-/dev/null}:/run/relay-ops/account-quality:ro'
require_relay_ops '${RELAY_OPS_UPSTREAM_GROUP_MAPPING_HOST_FILE:-/dev/null}:/run/relay-ops/upstream-group-mapping.json:ro'
require_relay_ops '${RELAY_OPS_CANDIDATE_KEYS_HOST_DIR:-./secrets/candidate-keys}:/run/secrets/candidates:ro'
require_relay_ops '${RELAY_OPS_CANDIDATE_MANAGED_KEYS_HOST_DIR:-./secrets/candidate-managed-keys}:/var/lib/relay-ops/candidate-keys:rw'
require_relay_ops 'test: ["CMD", "wget", "-q", "-T", "5", "-O", "/dev/null", "http://localhost:8100/healthz"]'
require_relay_ops 'mem_limit: 384m'
require_relay_ops 'cpus: 0.75'
forbid_relay_ops 'D04'
forbid_relay_ops 'RELAY_OPS_MODEL_RELEASE_RESULT_FILE'
forbid_relay_ops 'RELAY_OPS_MODEL_RELEASE_RESULT_HOST'

for retired in \
  RELAY_OPS_FEISHU_COMMAND_MODE \
  RELAY_OPS_FEISHU_VERIFICATION_TOKEN_FILE \
  RELAY_OPS_FEISHU_ENCRYPT_KEY_FILE \
  RELAY_OPS_FEISHU_ROUTING_FILE
do
  forbid_relay_ops "$retired"
  forbid "$retired" infra/.env.example
done
for retired_mount in \
  feishu-verification-token \
  feishu-encrypt-key \
  feishu-routing.json
do
  forbid_relay_ops "$retired_mount"
done

require 'RELAY_OPS_NOTIFICATION_POLICY_FILE=' infra/.env.example
require 'RELAY_OPS_NOTIFICATION_POLICY_HOST_FILE=' infra/.env.example
require_line 'RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE=' infra/.env.example
require_line 'RELAY_OPS_FEISHU_ALERT_RECIPIENTS_HOST_FILE=' infra/.env.example
require_line '# RELAY_OPS_FEISHU_ALERT_RECIPIENTS_FILE=/run/secrets/feishu-alert-recipients.json' infra/.env.example
require_line '# RELAY_OPS_FEISHU_ALERT_RECIPIENTS_HOST_FILE=./secrets/feishu-alert-recipients.json' infra/.env.example
policy_file=config/relay-ops/notification-policy.example.json
[[ -f "$policy_file" ]] || fail "missing notification policy example $policy_file"
require '"version": 1' "$policy_file"
require '"delivery_mode": "shadow"' "$policy_file"
for family in \
  group_runtime \
  group_capacity \
  account_impact \
  native_monitor_evidence \
  pricing_notice \
  daily_digest \
  incident_escalation
do
  require "\"${family}_enabled\":" "$policy_file"
done
for family in candidate release usage synthetic
do
  forbid "$family" "$policy_file"
done

for inactive_dir in \
  relay-ops-service/internal/acceptance \
  relay-ops-service/internal/candidates \
  relay-ops-service/internal/qualityreports \
  relay-ops-service/internal/billing
do
  if rg -n 'SendIncident|SendOneShot' "$inactive_dir" --glob '*.go'; then
    fail "inactive notification sender found in $inactive_dir"
  fi
done
forbid 'notifier.SendIncident' relay-ops-service/internal/app/app.go
if rg -n 'Acceptance:.*Notifier:' relay-ops-service/internal/app/app.go; then
  fail 'synthetic acceptance regained a notification sender'
fi
require 'SupersedeLegacyNotificationIncidents(ctx' relay-ops-service/internal/app/app.go

require 'USER 10002:10002' infra/Dockerfile.relay-ops
require 'ENTRYPOINT ["/relay-ops"]' infra/Dockerfile.relay-ops
forbid 'd04-readiness-snapshot' infra/Dockerfile.relay-ops
require 'ops/collect-account-quality-pulse.rb' infra/Dockerfile.relay-ops
require 'ops/analyze-account-monitor.rb' infra/Dockerfile.relay-ops
require 'config/upstream-benchmarks/quality-first-fast-v1.yaml' infra/Dockerfile.relay-ops
require 'path /pricing /relay-ops/static/*' infra/Caddyfile
require '@legacy_ops path /ops /ops/*' infra/Caddyfile
require 'redir @legacy_ops /admin/ops 302' infra/Caddyfile
require 'reverse_proxy @relay_ops_public relay-ops:8100' infra/Caddyfile
require 'reverse_proxy sub2api:8080' infra/Caddyfile
forbid 'internal-test-service' infra/Caddyfile
forbid 'path /api/v1/auth/register /api/v1/auth/login /api/v1/auth/login/2fa' infra/Caddyfile
forbid 'path /api/v1/settings/public' infra/Caddyfile
forbid '@relay_ops_feishu_command' infra/Caddyfile
forbid '@relay_ops_admin' infra/Caddyfile
forbid '/relay-ops/api/feishu/events' infra/Caddyfile
forbid '/relay-ops/api/incidents/ack' infra/Caddyfile
forbid '/relay-ops/api/ops-view' infra/Caddyfile

require 'mux.HandleFunc("GET /relay-ops/static/app.css", s.styles)' relay-ops-service/internal/http/server.go
require 'mux.HandleFunc("GET /pricing", s.pricing)' relay-ops-service/internal/http/server.go
forbid 'mux.Handle("GET /ops"' relay-ops-service/internal/http/server.go
forbid 'mux.Handle("GET /relay-ops/api/ops-view"' relay-ops-service/internal/http/server.go
forbid 'mux.Handle("POST /relay-ops/api/incidents/ack"' relay-ops-service/internal/http/server.go
forbid 'mux.Handle("POST /relay-ops/api/feishu/events"' relay-ops-service/internal/http/server.go
forbid 'static/ops.js' relay-ops-service/internal/http/server.go
forbid 'static/ops-admin.js' relay-ops-service/internal/http/server.go
require 'ListAccountMonitors' relay-ops-service/internal/sub2api/client.go
require '/api/v1/admin/account-monitors' relay-ops-service/internal/sub2api/client.go

ports_owner=$(awk '/^  [a-zA-Z0-9_-]+:/{service=$1} /^    ports:/{print service}' "$COMPOSE_FILE")
[[ "$ports_owner" == 'caddy:' ]] || fail 'only caddy may publish host ports'
docker compose \
  --env-file infra/.env.example \
  --env-file config/releases/sub2api.env \
  -f "$COMPOSE_FILE" \
  config --quiet || fail 'compose config failed'

printf 'PASS: relay-ops outbound-only and native-ops contracts\n'
