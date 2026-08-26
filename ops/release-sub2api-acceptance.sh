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

# The operator-owned file is sourced privately; no values are echoed.
set -a
# shellcheck disable=SC1090
source "$acceptance_env"
set +a

real_flow_ack=${ACCEPTANCE_REAL_FLOW_ACK:-}
[[ "$real_flow_ack" == I_UNDERSTAND_REAL_CHARGES ]] || fail 'ACCEPTANCE_REAL_FLOW_ACK is required'

site_address=${ACCEPTANCE_SITE_ADDRESS:-}
deploy_root=${ACCEPTANCE_DEPLOY_ROOT:-}
project_name=${ACCEPTANCE_PROJECT_NAME:-}
network_name=${ACCEPTANCE_NETWORK_NAME:-}
case "$site_address:$deploy_root:$project_name:$network_name" in
  *api.xingqiaolab.top*|*shop.xingqiaolab.top*|*/opt/sub2api/production*|*sub2api_default*|*':sub2api:'*)
    fail 'production identity is forbidden'
    ;;
esac
[[ -n "$site_address" && -n "$deploy_root" && -n "$project_name" && -n "$network_name" ]] \
  || fail 'acceptance identity is incomplete'

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

build_context=${RELEASE_BUILD_CONTEXT:-$worktree/upstream/sub2api}
[[ "$build_context" == /* && -d "$build_context" && ! -L "$build_context" ]] \
  || fail 'RELEASE_BUILD_CONTEXT is invalid'
build_context=$(cd "$build_context" && pwd -P)
case "$build_context" in
  "$worktree"/*) ;;
  *) fail 'RELEASE_BUILD_CONTEXT must be inside RELEASE_WORKTREE' ;;
esac

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
trap 'rm -rf "$tmp_root"' EXIT
archive="$tmp_root/sub2api-image.tar"
bundle="$tmp_root/bundle"
mkdir -m 700 "$bundle"

# Build one immutable candidate for the acceptance station.
docker buildx build --platform linux/amd64 --load -t "$image_ref" "$build_context"
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
[[ "$remote_stage" == /var/tmp/sub2api-acceptance-release.* ]] || fail 'remote staging directory is invalid'

scp -q -i "$ssh_key" -P "$ssh_port" -o BatchMode=yes -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$ssh_known_hosts" "$archive" "$archive.sha256" \
  "$bundle/compose.acceptance.yaml" "$bundle/Caddyfile.acceptance" "$bundle/.env.acceptance" \
  "$bundle/source-commit" "$bundle/source-tree" "$ssh_target:$remote_stage/" \
  || fail 'acceptance bundle transfer failed'

# The only remote entrypoint is the dedicated acceptance host executor.
executor_source="$worktree/ops/deploy-sub2api-acceptance-host.sh"
[[ -f "$executor_source" ]] || fail 'acceptance host executor is missing'
ssh -i "$ssh_key" -p "$ssh_port" -o BatchMode=yes -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$ssh_known_hosts" "$ssh_target" \
  "sudo -n bash -s -- --staging-root '$remote_stage' --image-archive '$remote_stage/$(basename "$archive")' --image-sha256 '$remote_stage/$(basename "$archive.sha256")' --compose '$remote_stage/compose.acceptance.yaml' --caddy '$remote_stage/Caddyfile.acceptance' --env-file '$remote_stage/.env.acceptance' --source-commit '$source_commit' --source-tree '$source_tree' --deploy-root '$deploy_root'" \
  <"$executor_source" || fail 'acceptance host executor failed'

printf 'acceptance_release status=succeeded source_commit=%s source_tree=%s image_sha256=%s\n' \
  "$source_commit" "$source_tree" "$archive_sha"
