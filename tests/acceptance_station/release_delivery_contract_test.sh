#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
cd "$ROOT"

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

controller=ops/release-sub2api-acceptance.sh
executor=ops/deploy-sub2api-acceptance-host.sh
source_checker=ops/assert-sub2api-release-source.sh
runbook=docs/runbooks/sub2api-acceptance-station.md

[[ -f "$controller" ]] || fail "$controller is missing"
[[ -x "$controller" ]] || fail "$controller is not executable"
[[ -f "$executor" ]] || fail "$executor is missing"
[[ -x "$executor" ]] || fail "$executor is not executable"
[[ -x "$source_checker" ]] || fail "$source_checker is missing or not executable"
[[ -f "$runbook" ]] || fail 'acceptance runbook is missing'
grep -Fq '本地直接验证 -> 合入并推送根 main -> 从同一 main commit 部署验收站 -> 管理员真实验收 -> 从同一 main commit 部署主站' "$runbook" \
  || fail 'acceptance runbook is missing serial promotion boundary'
grep -Fq '不自动晋级' "$runbook" || fail 'acceptance runbook is missing no-auto-promotion boundary'
grep -Fq '/admin/lab/' "$runbook" || fail 'acceptance runbook is missing admin lab retirement boundary'
grep -Fq 'https://api.xingqiaolab.top/admin/lab/' "$runbook" \
  || fail 'acceptance runbook is missing the shared-domain lab address'
grep -Fq '仅保留脱敏失败证据' "$runbook" || fail 'acceptance runbook is missing failed-staging retention boundary'
grep -Fq '人工回退时必须先在根 `main` 上形成明确的 revert 或前向修复提交' "$runbook" \
  || fail 'acceptance runbook is missing main-only manual rollback path'
! grep -Fq 'sudo -n bash ops/deploy-sub2api-acceptance-host.sh' "$runbook" \
  || fail 'acceptance runbook documents an executor path unavailable on the host'

tmp_root=$(mktemp -d "${TMPDIR:-/tmp}/acceptance-release-contract.XXXXXX")
fixture="$tmp_root/repo"
scratch="$tmp_root/scratch"
trap 'rm -rf "$tmp_root"' EXIT
mkdir -p "$fixture" "$scratch"
mkdir -p "$fixture/upstream/sub2api" "$fixture/ops"
cp "$controller" "$fixture/ops/release-sub2api-acceptance.sh"
cp "$source_checker" "$fixture/ops/assert-sub2api-release-source.sh"
chmod +x "$fixture/ops/release-sub2api-acceptance.sh"
chmod +x "$fixture/ops/assert-sub2api-release-source.sh"
git -C "$fixture" init -q
git -C "$fixture" config user.email contract@example.invalid
git -C "$fixture" config user.name contract
git -C "$fixture" add .
git -C "$fixture" commit -qm fixture
git -C "$tmp_root" init -q --bare remote.git
git -C "$fixture" remote add origin "$tmp_root/remote.git"
git -C "$fixture" branch -M main
git -C "$fixture" push -q -u origin main

env_file="$scratch/acceptance.env"
write_env() {
  cat >"$env_file" <<'EOF'
ACCEPTANCE_SITE_ADDRESS=api.xingqiaolab.top
ACCEPTANCE_DEPLOY_ROOT=/opt/sub2api/acceptance-contract
ACCEPTANCE_PROJECT_NAME=sub2api-acceptance
ACCEPTANCE_NETWORK_NAME=sub2api-acceptance-network
ACCEPTANCE_LOOPBACK_PORT=8181
ACCEPTANCE_PAYMENT_PROVIDER=stripe
ACCEPTANCE_UPSTREAM_PROVIDER=openai
ACCEPTANCE_NOTIFICATION_TRANSPORT=webhook
ACCEPTANCE_REAL_FLOW_ACK=I_UNDERSTAND_REAL_CHARGES
ACCEPTANCE_TOTP_ENCRYPTION_KEY=0000000000000000000000000000000000000000000000000000000000000000
EOF
  chmod 600 "$env_file"
}

assert_refusal() {
  local expected=$1
  shift
  local output status
  set +e
  output=$(env ACCEPTANCE_ENV_FILE="$env_file" RELEASE_WORKTREE="$fixture" "$@" \
    "$ROOT/$controller" 2>&1)
  status=$?
  set -e
  [[ $status -ne 0 ]] || fail "controller unexpectedly accepted: $expected"
  [[ "$output" == *"$expected"* ]] || fail "expected refusal '$expected', got: $output"
}

write_env
git -C "$fixture" switch -q -c candidate
assert_refusal 'releases must use the main branch'
git -C "$fixture" switch -q main

write_env
sed -i.bak 's/^ACCEPTANCE_PROJECT_NAME=.*/ACCEPTANCE_PROJECT_NAME=other-project/' "$env_file"
rm -f "$env_file.bak"
assert_refusal 'ACCEPTANCE_PROJECT_NAME must be sub2api-acceptance'

write_env
sed -i.bak 's/^ACCEPTANCE_NETWORK_NAME=.*/ACCEPTANCE_NETWORK_NAME=other-network/' "$env_file"
rm -f "$env_file.bak"
assert_refusal 'ACCEPTANCE_NETWORK_NAME must be sub2api-acceptance-network'

write_env
sed -i.bak 's|^ACCEPTANCE_DEPLOY_ROOT=.*|ACCEPTANCE_DEPLOY_ROOT=/opt/sub2api/acceptance-contract;id|' "$env_file"
rm -f "$env_file.bak"
assert_refusal 'ACCEPTANCE_DEPLOY_ROOT must be a canonical acceptance-only path'

write_env
sed -i.bak 's/^ACCEPTANCE_TOTP_ENCRYPTION_KEY=.*/ACCEPTANCE_TOTP_ENCRYPTION_KEY=000000000000000000000000000000000000000000000000/' "$env_file"
rm -f "$env_file.bak"
assert_refusal 'ACCEPTANCE_TOTP_ENCRYPTION_KEY must be 64 hexadecimal characters'

write_env
assert_refusal 'RELEASE_BUILD_CONTEXT must equal canonical upstream/sub2api' \
  RELEASE_BUILD_CONTEXT=/tmp/not-the-upstream-context

write_env
trace="$scratch/transport.trace"
fake_bin="$scratch/fake-bin"
mkdir -p "$fake_bin"
for binary in docker ssh scp; do
  printf '#!/usr/bin/env bash\nprintf invoked >>%q\n' "$trace" >"$fake_bin/$binary"
  chmod +x "$fake_bin/$binary"
done
[[ ! -e "$trace" ]] || fail 'controller contract fixture must not contact transport'

for needle in \
  'I_UNDERSTAND_REAL_CHARGES' \
  'api.xingqiaolab.top' \
  'sub2api_default' \
  'worktree is dirty' \
  'ACCEPTANCE_ENV_FILE must be a 0600 non-symlink file' \
  'ACCEPTANCE_REAL_FLOW_ACK is required' \
  'ACCEPTANCE_PROJECT_NAME must be sub2api-acceptance' \
  'ACCEPTANCE_NETWORK_NAME must be sub2api-acceptance-network' \
  'ACCEPTANCE_DEPLOY_ROOT must be a canonical acceptance-only path' \
  'RELEASE_BUILD_CONTEXT must equal canonical upstream/sub2api' \
  'production identity is forbidden' \
  'mock flow is forbidden' \
  'ACCEPTANCE_SITE_ADDRESS must be api.xingqiaolab.top' \
  'ACCEPTANCE_LOOPBACK_PORT is invalid' \
  'ACCEPTANCE_TOTP_ENCRYPTION_KEY must be 64 hexadecimal characters' \
  '--build-arg VITE_APP_BASE_PATH=/admin/lab/' \
  '--build-arg VITE_API_BASE_URL=/admin/lab/api/v1' \
  '--build-arg VITE_AUTH_STORAGE_PREFIX=admin_lab_' \
  'docker buildx build --platform linux/amd64 --load' \
  'docker save' \
  'sha256_file' \
  'cleanup_remote_stage' \
  'sub2api-image.tar' \
  '.env.acceptance' \
  'scp' \
  'ssh' \
  'sudo -n bash -s'; do
  grep -Fq -- "$needle" "$controller" || fail "release controller missing contract: $needle"
done

targets=("$controller" "$executor")
grep -Fq 'docker compose --project-name sub2api-acceptance' "$executor" || fail 'executor missing compose project'
grep -Fq 'if ! ssh -i "$ssh_key"' "$controller" \
  || fail 'controller must surface remote staging cleanup failure'
grep -Fq 'return 1' "$controller" || fail 'controller must return cleanup failure'
grep -Fq 'rm -rf -- "$staging_root"' "$executor" || fail 'executor must clean remote staging'
grep -Fq 'acceptance-bootstrap' "$executor" || fail 'executor missing bootstrap'
grep -Fq 'rollback' "$executor" || fail 'executor missing rollback'
grep -Fq 'health' "$executor" || fail 'executor missing health checks'
grep -Fq 'mktemp' "$executor" || fail 'executor missing isolated extraction'
grep -Fq 'docker load' "$executor" || fail 'executor missing image load'
grep -Fq 'downtime_required' "$executor" || fail 'executor missing result contract'
grep -Fq 'ACCEPTANCE_SITE_ADDRESS must be api.xingqiaolab.top' "$executor" \
  || fail 'executor must bind acceptance to the main domain path'
grep -Fq 'ACCEPTANCE_LOOPBACK_PORT is invalid' "$executor" \
  || fail 'executor must validate the loopback listener port'
grep -Fq 'ACCEPTANCE_TOTP_ENCRYPTION_KEY must be 64 hexadecimal characters' "$executor" \
  || fail 'executor must validate the TOTP encryption key'
grep -Fq 'http://127.0.0.1:$loopback_port/admin/lab/health' "$executor" \
  || fail 'executor must probe the prefixed loopback health route'
! grep -En 'release-sub2api-blue-green|deploy-sub2api-blue-green|release-admin-lab' "${targets[@]}" \
  || fail 'acceptance release chain must not invoke a production or lab release script'

echo 'acceptance release delivery contract: PASS'
