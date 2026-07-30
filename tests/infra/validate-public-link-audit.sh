#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

require_fixed() {
  local needle=$1
  rg -Fq -- "$needle" tests/infra/audit-public-links.sh || \
    fail "missing public audit contract: $needle"
}

bash -n tests/infra/audit-public-links.sh

require_fixed "'GET /relay-ops/api/ops-view'"
require_fixed "'POST /relay-ops/api/incidents/ack'"
require_fixed "'POST /relay-ops/api/feishu/events'"
require_fixed '--request "$retired_method"'
require_fixed 'read -r retired_method retired_path <<< "$retired_endpoint"'
require_fixed 'audit_retired_endpoint "$retired_method" "$retired_path"'
require_fixed 'retired_status=$(curl --disable "${curl_args[@]}" "${BASE_ORIGIN}${retired_path}")'

retired_helper=$(
  sed -n '/^audit_retired_endpoint() {$/,/^}$/p' tests/infra/audit-public-links.sh
)
[[ -n "$retired_helper" ]] || fail 'missing audit_retired_endpoint helper'
grep -Fq -- '--max-redirs 0' <<< "$retired_helper" || \
  fail 'retired endpoint audit must disable redirects'
if grep -Fq -- '--location' <<< "$retired_helper"; then
  fail 'retired endpoint audit must not follow redirects'
fi

printf 'PASS: public link audit static contracts\n'
