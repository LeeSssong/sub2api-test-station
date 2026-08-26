#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
cd "$ROOT"

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

controller=ops/release-sub2api-acceptance.sh
executor=ops/deploy-sub2api-acceptance-host.sh

[[ -f "$controller" ]] || fail "$controller is missing"
[[ -x "$controller" ]] || fail "$controller is not executable"
[[ -f "$executor" ]] || fail "$executor is missing"
[[ -x "$executor" ]] || fail "$executor is not executable"

tmp_root=$(mktemp -d "${TMPDIR:-/tmp}/acceptance-release-contract.XXXXXX")
fixture="$tmp_root/repo"
scratch="$tmp_root/scratch"
trap 'rm -rf "$tmp_root"' EXIT
mkdir -p "$fixture" "$scratch"
mkdir -p "$fixture/upstream/sub2api" "$fixture/ops"
cp "$controller" "$fixture/ops/release-sub2api-acceptance.sh"
chmod +x "$fixture/ops/release-sub2api-acceptance.sh"
git -C "$fixture" init -q
git -C "$fixture" config user.email contract@example.invalid
git -C "$fixture" config user.name contract
git -C "$fixture" add .
git -C "$fixture" commit -qm fixture

env_file="$scratch/acceptance.env"
write_env() {
  cat >"$env_file" <<'EOF'
ACCEPTANCE_SITE_ADDRESS=acceptance.example.invalid
ACCEPTANCE_DEPLOY_ROOT=/opt/sub2api/acceptance-contract
ACCEPTANCE_PROJECT_NAME=sub2api-acceptance
ACCEPTANCE_NETWORK_NAME=sub2api-acceptance-network
ACCEPTANCE_PAYMENT_PROVIDER=stripe
ACCEPTANCE_UPSTREAM_PROVIDER=openai
ACCEPTANCE_NOTIFICATION_TRANSPORT=webhook
ACCEPTANCE_REAL_FLOW_ACK=I_UNDERSTAND_REAL_CHARGES
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
  'docker buildx build --platform linux/amd64 --load' \
  'docker save' \
  'sha256_file' \
  'scp' \
  'ssh' \
  'sudo -n bash -s'; do
  grep -Fq "$needle" "$controller" || fail "release controller missing contract: $needle"
done

targets=("$controller" "$executor")
grep -Fq 'docker compose --project-name sub2api-acceptance' "$executor" || fail 'executor missing compose project'
grep -Fq 'acceptance-bootstrap' "$executor" || fail 'executor missing bootstrap'
grep -Fq 'rollback' "$executor" || fail 'executor missing rollback'
grep -Fq 'health' "$executor" || fail 'executor missing health checks'
grep -Fq 'mktemp' "$executor" || fail 'executor missing isolated extraction'
grep -Fq 'docker load' "$executor" || fail 'executor missing image load'
grep -Fq 'downtime_required' "$executor" || fail 'executor missing result contract'
! grep -En 'release-sub2api-blue-green|deploy-sub2api-blue-green|release-admin-lab' "${targets[@]}" \
  || fail 'acceptance release chain must not invoke a production or lab release script'

echo 'acceptance release delivery contract: PASS'
