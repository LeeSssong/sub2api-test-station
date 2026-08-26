#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
cd "$ROOT"

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

controller=ops/release-sub2api-acceptance.sh
executor=ops/deploy-sub2api-acceptance-host.sh

for file in "$controller" "$executor"; do
  [[ -f "$file" ]] || fail "$file is missing"
  [[ -x "$file" ]] || fail "$file is not executable"
done

for needle in \
  'I_UNDERSTAND_REAL_CHARGES' \
  'api.xingqiaolab.top' \
  'sub2api_default' \
  'worktree is dirty' \
  'ACCEPTANCE_ENV_FILE must be a 0600 non-symlink file' \
  'ACCEPTANCE_REAL_FLOW_ACK is required' \
  'production identity is forbidden' \
  'mock flow is forbidden' \
  'docker buildx build --platform linux/amd64 --load' \
  'docker save' \
  'sha256_file' \
  'scp' \
  'ssh' \
  'sudo -n bash -s'; do
  grep -Fq "$needle" "$controller" || fail "release controller missing contract: $needle"
done

! rg -n 'release-sub2api-blue-green|deploy-sub2api-blue-green|release-admin-lab' \
  "$controller" "$executor" \
  || fail 'acceptance release chain must not invoke a production or lab release script'

echo 'acceptance release delivery contract: PASS'
