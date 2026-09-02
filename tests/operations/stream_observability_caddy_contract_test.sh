#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
for file in "$ROOT/infra/Caddyfile" "$ROOT/infra/Caddyfile.acceptance"; do
  grep -Fq $'format json' "$file"
  grep -Fq 'log_append client_request_id' "$file"
  grep -Fq 'log_append window_id' "$file"
  grep -Fq 'log_append thread_id' "$file"
  grep -Fq 'log_append session_id' "$file"
  if grep -Eq 'log_append (authorization|cookie|api_key|token)' "$file"; then
    echo "sensitive header logging is forbidden in $file" >&2
    exit 1
  fi
done
grep -Fq 'max-size: "20m", max-file: "5"' "$ROOT/infra/compose.yaml"
grep -Fq 'max-size: "20m", max-file: "5"' "$ROOT/infra/compose.acceptance.yaml"
grep -Fq 'SUB2API_ENVIRONMENT: acceptance' "$ROOT/infra/compose.acceptance.yaml"
grep -Fq 'SUB2API_CONTAINER_SLOT: blue' "$ROOT/infra/compose.yaml"
grep -Fq 'SUB2API_CONTAINER_SLOT: green' "$ROOT/infra/compose.yaml"
printf 'PASS: stream observability Caddy/Compose contract\n'
