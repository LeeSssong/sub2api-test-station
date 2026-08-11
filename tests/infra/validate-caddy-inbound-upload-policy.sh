#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
cd "$ROOT"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

require_fixed() {
  local needle=$1
  local file=$2
  rg -Fq -- "$needle" "$file" || fail "missing expected value in $file: $needle"
}

require_fixed 'read_body 15m' infra/Caddyfile

fallback_line=$(rg -n -F $'\treverse_proxy {$SUB2API_ACTIVE_UPSTREAM:sub2api-blue:8080} {' infra/Caddyfile | tail -n1 | cut -d: -f1)
[[ -n "$fallback_line" ]] || fail 'missing public Sub2API fallback proxy'
fallback_block=$(tail -n +"$fallback_line" infra/Caddyfile | awk '
  {
    print
    opens += gsub(/\{/, "{")
    closes += gsub(/\}/, "}")
    if (opens > 0 && opens == closes) exit
  }
')
[[ "$fallback_block" == *$'\t\t\tresponse_header_timeout 15m'* ]] || \
  fail 'public Sub2API fallback must use a 15-minute response-header window'

if rg -n -F 'response_header_timeout 300s' infra/Caddyfile; then
  fail 'public Sub2API proxy still uses the fixed 300-second response-header window'
fi

# Existing application-side limits remain the source of truth. T01 must not
# weaken or replace them with an incompatible edge-only limit.
for setting in \
  'SERVER_MAX_REQUEST_BODY_SIZE: ${SERVER_MAX_REQUEST_BODY_SIZE:-134217728}' \
  'GATEWAY_MAX_BODY_SIZE: ${GATEWAY_MAX_BODY_SIZE:-134217728}' \
  'GATEWAY_TEXT_MAX_BODY_SIZE: ${GATEWAY_TEXT_MAX_BODY_SIZE:-16777216}'; do
  require_fixed "$setting" infra/compose.yaml
done

if rg -n '^[[:space:]]*request_body[[:space:]]*\{' infra/Caddyfile; then
  fail 'production Caddy must not add a smaller edge request-body limit than the native Sub2API limits'
fi

printf 'PASS: Caddy inbound upload window and native body-size limits are compatible\n'
