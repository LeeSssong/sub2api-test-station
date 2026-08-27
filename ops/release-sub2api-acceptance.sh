#!/usr/bin/env bash
set -euo pipefail

umask 077

fail() {
  printf 'acceptance_release status=failed: %s\n' "$1" >&2
  exit 1
}

mode_of() {
  stat -c '%a' -- "$1" 2>/dev/null || stat -f '%Lp' -- "$1"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$1" | awk '{print $1}'
  else
    shasum -a 256 -- "$1" | awk '{print $1}'
  fi
}

load_acceptance_env() {
  local line key value
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]] \
      || fail 'ACCEPTANCE_ENV_FILE contains an invalid assignment'
    key=${BASH_REMATCH[1]}
    value=${BASH_REMATCH[2]}
    [[ "$key" == ACCEPTANCE_* ]] || fail 'ACCEPTANCE_ENV_FILE may only contain ACCEPTANCE_ variables'
    [[ "$value" != *$'\n'* && "$value" != *$'\r'* ]] \
      || fail 'ACCEPTANCE_ENV_FILE contains an invalid value'
    printf -v "$key" '%s' "$value"
    export "$key"
  done <"$acceptance_env"
}

worktree=${RELEASE_WORKTREE:-$(pwd -P)}
[[ "$worktree" == /* && -d "$worktree" && ! -L "$worktree" ]] || fail 'RELEASE_WORKTREE is invalid'
worktree=$(cd "$worktree" && pwd -P)
[[ -d "$worktree/.git" || -f "$worktree/.git" ]] || fail 'RELEASE_WORKTREE is not a Git worktree'

# All local refusal checks happen before command construction or SSH/SCP.
[[ -z "$(git -C "$worktree" status --porcelain)" ]] || fail 'worktree is dirty'

acceptance_env=${ACCEPTANCE_ENV_FILE:-}
[[ "$acceptance_env" == /* && -f "$acceptance_env" && ! -L "$acceptance_env" ]] \
  || fail 'ACCEPTANCE_ENV_FILE must be a 0600 non-symlink file'
[[ "$(mode_of "$acceptance_env")" == 600 ]] \
  || fail 'ACCEPTANCE_ENV_FILE must be a 0600 non-symlink file'

# Parse operator values as literal KEY=VALUE pairs; never evaluate the env file.
load_acceptance_env

real_flow_ack=${ACCEPTANCE_REAL_FLOW_ACK:-}
[[ "$real_flow_ack" == I_UNDERSTAND_REAL_CHARGES ]] || fail 'ACCEPTANCE_REAL_FLOW_ACK is required'

site_address=${ACCEPTANCE_SITE_ADDRESS:-}
deploy_root=${ACCEPTANCE_DEPLOY_ROOT:-}
project_name=${ACCEPTANCE_PROJECT_NAME:-}
network_name=${ACCEPTANCE_NETWORK_NAME:-}
loopback_port=${ACCEPTANCE_LOOPBACK_PORT:-}
case "$site_address:$deploy_root:$project_name:$network_name" in
  *shop.xingqiaolab.top*|*/opt/sub2api/production*|*sub2api_default*|*':sub2api:'*)
    fail 'production identity is forbidden'
    ;;
esac
[[ -n "$site_address" && -n "$deploy_root" && -n "$project_name" && -n "$network_name" ]] \
  || fail 'acceptance identity is incomplete'
[[ "$site_address" == api.xingqiaolab.top ]] || fail 'ACCEPTANCE_SITE_ADDRESS must be api.xingqiaolab.top'
[[ "$project_name" == sub2api-acceptance ]] || fail 'ACCEPTANCE_PROJECT_NAME must be sub2api-acceptance'
[[ "$network_name" == sub2api-acceptance-network ]] || fail 'ACCEPTANCE_NETWORK_NAME must be sub2api-acceptance-network'
[[ "$loopback_port" =~ ^[1-9][0-9]{3,4}$ && "$loopback_port" -le 65535 && "$loopback_port" -ne 443 ]] \
  || fail 'ACCEPTANCE_LOOPBACK_PORT is invalid'
[[ "$deploy_root" =~ ^/opt/sub2api/acceptance-[A-Za-z0-9._-]+$ ]] \
  || fail 'ACCEPTANCE_DEPLOY_ROOT must be a canonical acceptance-only path'
[[ "$deploy_root" != *$'\n'* && "$deploy_root" != *$'\r'* && "$deploy_root" != *'..'* ]] \
  || fail 'ACCEPTANCE_DEPLOY_ROOT must be a canonical acceptance-only path'

payment_provider=${ACCEPTANCE_PAYMENT_PROVIDER:-}
upstream_provider=${ACCEPTANCE_UPSTREAM_PROVIDER:-}
notification_transport=${ACCEPTANCE_NOTIFICATION_TRANSPORT:-}
[[ -n "$payment_provider" && -n "$upstream_provider" && -n "$notification_transport" ]] \
  || fail 'real flow providers are required'
case "$payment_provider:$upstream_provider:$notification_transport" in
  *mock*|*lab-outbox*) fail 'mock flow is forbidden' ;;
esac

source_commit=$(git -C "$worktree" rev-parse HEAD) || fail 'could not resolve source commit'
source_tree=$(git -C "$worktree" rev-parse 'HEAD^{tree}') || fail 'could not resolve source tree'
[[ "$source_commit" =~ ^[a-f0-9]{40}$ && "$source_tree" =~ ^[a-f0-9]{40}$ ]] \
  || fail 'Git identity is invalid'

expected_build_context="$worktree/upstream/sub2api"
if [[ -n "${RELEASE_BUILD_CONTEXT:-}" && "$RELEASE_BUILD_CONTEXT" != "$expected_build_context" ]]; then
  fail 'RELEASE_BUILD_CONTEXT must equal canonical upstream/sub2api'
fi
[[ -d "$expected_build_context" && ! -L "$expected_build_context" ]] \
  || fail 'canonical upstream/sub2api build context is missing'
build_context=$(cd "$expected_build_context" && pwd -P)
[[ "$build_context" == "$expected_build_context" ]] \
  || fail 'canonical upstream/sub2api build context is invalid'

# Validate the local executor before spending time on a build or contacting the host.
executor_source="$worktree/ops/deploy-sub2api-acceptance-host.sh"
[[ -f "$executor_source" && -x "$executor_source" && ! -L "$executor_source" ]] \
  || fail 'acceptance host executor is missing or not executable'

ssh_target=${ACCEPTANCE_SSH_TARGET:-${RELEASE_SSH_TARGET:-}}
ssh_port=${ACCEPTANCE_SSH_PORT:-${RELEASE_SSH_PORT:-22}}
ssh_key=${ACCEPTANCE_SSH_KEY:-${RELEASE_SSH_KEY:-}}
ssh_known_hosts=${ACCEPTANCE_SSH_KNOWN_HOSTS:-${RELEASE_SSH_KNOWN_HOSTS:-}}
[[ "$ssh_target" =~ ^[A-Za-z0-9._-]+@[A-Za-z0-9._-]+$ ]] || fail 'ACCEPTANCE_SSH_TARGET is invalid'
[[ "$ssh_port" =~ ^[1-9][0-9]{0,4}$ && "$ssh_port" -le 65535 ]] || fail 'ACCEPTANCE_SSH_PORT is invalid'
[[ "$ssh_key" == /* && -f "$ssh_key" && ! -L "$ssh_key" && "$(mode_of "$ssh_key")" == 600 ]] \
  || fail 'ACCEPTANCE_SSH_KEY must be a 0600 non-symlink file'
[[ "$ssh_known_hosts" == /* && -f "$ssh_known_hosts" && ! -L "$ssh_known_hosts" && "$(mode_of "$ssh_known_hosts")" == 600 ]] \
  || fail 'ACCEPTANCE_SSH_KNOWN_HOSTS must be a 0600 non-symlink file'

command -v docker >/dev/null 2>&1 || fail 'Docker Buildx is required'
command -v ssh >/dev/null 2>&1 || fail 'SSH is required'
command -v scp >/dev/null 2>&1 || fail 'SCP is required'

image_ref="sub2api-acceptance:$source_commit"
[[ "$image_ref" != *'"'* && "$image_ref" != *"'"* ]] || fail 'ACCEPTANCE_IMAGE is invalid'
tmp_root=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-acceptance-release.XXXXXX")
remote_stage=
cleanup_remote_stage() {
  [[ -n "$remote_stage" ]] || return 0
  if ! ssh -i "$ssh_key" -p "$ssh_port" -o BatchMode=yes -o StrictHostKeyChecking=yes \
    -o UserKnownHostsFile="$ssh_known_hosts" "$ssh_target" \
    "rm -rf -- $(printf '%q' "$remote_stage")" >/dev/null 2>&1; then
    return 1
  fi
  remote_stage=
}
cleanup() {
  local exit_status=$?
  if ! cleanup_remote_stage; then
    printf 'acceptance_release status=warning: remote staging cleanup failed\n' >&2
  fi
  rm -rf "$tmp_root"
  trap - EXIT
  exit "$exit_status"
}
trap cleanup EXIT
archive="$tmp_root/sub2api-image.tar"
bundle="$tmp_root/bundle"
mkdir -m 700 "$bundle"

# Build one immutable candidate for the acceptance station.
docker buildx build --platform linux/amd64 --load \
  --build-arg VITE_APP_BASE_PATH=/admin/lab/ \
  --build-arg VITE_API_BASE_URL=/admin/lab/api/v1 \
  --build-arg VITE_AUTH_STORAGE_PREFIX=admin_lab_ \
  -t "$image_ref" "$build_context"
docker save -o "$archive" "$image_ref"
archive_sha=$(sha256_file "$archive") || fail 'image archive checksum failed'
[[ "$archive_sha" =~ ^[a-f0-9]{64}$ ]] || fail 'image archive checksum is invalid'
printf '%s  %s\n' "$archive_sha" "$(basename "$archive")" >"$archive.sha256"

[[ -f "$worktree/infra/compose.acceptance.yaml" && -f "$worktree/infra/Caddyfile.acceptance" ]] \
  || fail 'acceptance topology files are missing'
cp "$worktree/infra/compose.acceptance.yaml" "$bundle/compose.acceptance.yaml"
cp "$worktree/infra/Caddyfile.acceptance" "$bundle/Caddyfile.acceptance"
awk -v image="$image_ref" '
  /^ACCEPTANCE_IMAGE=/ { print "ACCEPTANCE_IMAGE=" image; replaced=1; next }
  { print }
  END { if (!replaced) print "ACCEPTANCE_IMAGE=" image }
' "$acceptance_env" >"$bundle/.env.acceptance"
chmod 600 "$bundle/.env.acceptance"
printf '%s\n' "$source_commit" >"$bundle/source-commit"
printf '%s\n' "$source_tree" >"$bundle/source-tree"

remote_stage=$(ssh -i "$ssh_key" -p "$ssh_port" -o BatchMode=yes -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$ssh_known_hosts" "$ssh_target" \
  'umask 077; d=$(mktemp -d /var/tmp/sub2api-acceptance-release.XXXXXX); chmod 700 "$d"; printf "%s" "$d"') \
  || fail 'could not create remote staging directory'
[[ "$remote_stage" =~ ^/var/tmp/sub2api-acceptance-release\.[A-Za-z0-9._-]+$ ]] \
  || fail 'remote staging directory is invalid'

scp -q -i "$ssh_key" -P "$ssh_port" -o BatchMode=yes -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$ssh_known_hosts" "$archive" "$archive.sha256" \
  "$bundle/compose.acceptance.yaml" "$bundle/Caddyfile.acceptance" "$bundle/.env.acceptance" \
  "$bundle/source-commit" "$bundle/source-tree" "$ssh_target:$remote_stage/" \
  || fail 'acceptance bundle transfer failed'

# The only remote entrypoint is the dedicated acceptance host executor.
shell_quote() { printf '%q' "$1"; }
remote_command="sudo -n bash -s --"
for arg in \
  --staging-root "$remote_stage" \
  --image-archive "$remote_stage/$(basename "$archive")" \
  --image-sha256 "$remote_stage/$(basename "$archive.sha256")" \
  --compose "$remote_stage/compose.acceptance.yaml" \
  --caddy "$remote_stage/Caddyfile.acceptance" \
  --env-file "$remote_stage/.env.acceptance" \
  --source-commit "$source_commit" \
  --source-tree "$source_tree" \
  --deploy-root "$deploy_root"; do
  remote_command+=" $(shell_quote "$arg")"
done
ssh -i "$ssh_key" -p "$ssh_port" -o BatchMode=yes -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$ssh_known_hosts" "$ssh_target" \
  "$remote_command" \
  <"$executor_source" || fail 'acceptance host executor failed'

printf 'acceptance_release status=succeeded source_commit=%s source_tree=%s image_sha256=%s\n' \
  "$source_commit" "$source_tree" "$archive_sha"
