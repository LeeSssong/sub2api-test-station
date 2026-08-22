#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

(
  cd upstream/sub2api/backend
  go test ./internal/lab -run 'TestValidateEgressTarget' -count=1
)
grep -Fq 'SECURITY_URL_ALLOWLIST_ENABLED: "true"' infra/compose.admin-lab.yaml
grep -Fq 'UPSTREAM_PROVIDER: mock-upstream' infra/compose.admin-lab.yaml
grep -Fq 'PAYMENT_PROVIDER: mock' infra/compose.admin-lab.yaml
if grep -RInE 'api\.openai\.com|api\.anthropic\.com|smtp://|feishu\.cn' infra/admin-lab tools/admin-lab; then
  echo 'real external endpoint present in lab mock implementation' >&2
  exit 1
fi
echo 'admin lab mock egress contract: PASS'
python3 tools/admin-lab/mock_server_test.py
