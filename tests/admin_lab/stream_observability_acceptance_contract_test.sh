#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
grep -Fq 'SUB2API_ENVIRONMENT: acceptance' "$ROOT/infra/compose.acceptance.yaml"
grep -Fq 'SUB2API_DEPLOYMENT_COMMIT' "$ROOT/infra/compose.acceptance.yaml"
grep -Fq 'log_append environment' "$ROOT/infra/Caddyfile.acceptance"
grep -Fq 'log_append client_request_id' "$ROOT/infra/Caddyfile.acceptance"
if grep -RInE 'log_append (authorization|cookie|api_key|token)' "$ROOT/infra/Caddyfile.acceptance"; then
  echo 'acceptance Caddy must not log credentials' >&2
  exit 1
fi
printf 'PASS: acceptance stream observability contract\n'
