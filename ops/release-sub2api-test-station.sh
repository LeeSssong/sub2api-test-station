#!/usr/bin/env bash
set -euo pipefail
umask 077
fail(){ printf 'test_station_release status=failed: %s\n' "$1" >&2; exit 1; }
worktree=${RELEASE_WORKTREE:-$(pwd -P)}; [[ "$worktree" == /* && -d "$worktree" ]] || fail 'worktree is invalid'; worktree=$(cd "$worktree" && pwd -P)
[[ "$(git -C "$worktree" branch --show-current)" == main ]] || fail 'release must originate from main'
[[ -z "$(git -C "$worktree" status --porcelain)" ]] || fail 'worktree is dirty'
git -C "$worktree" fetch origin main >/dev/null 2>&1 || fail 'origin fetch failed'
[[ "$(git -C "$worktree" rev-parse HEAD)" == "$(git -C "$worktree" rev-parse origin/main)" ]] || fail 'main is not equal to origin/main'
source_commit=$(git -C "$worktree" rev-parse HEAD); source_tree=$(git -C "$worktree" rev-parse 'HEAD^{tree}')
command -v docker >/dev/null 2>&1 || fail 'Docker is required'; command -v ssh >/dev/null 2>&1 || fail 'SSH is required'
target=${TEST_STATION_SSH_TARGET:-sub2api-test-station}; [[ "$target" == sub2api-test-station ]] || fail 'unsafe SSH target'
deploy_root=/opt/sub2api-test-station; build_context="$worktree/upstream/sub2api"; [[ -d "$build_context" ]] || fail 'build context missing'
tmp=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-test-station-release.XXXXXX"); trap 'rm -rf -- "$tmp"' EXIT
image="sub2api-test-station-runtime:$source_commit"; docker buildx build --platform linux/amd64 --load -t "$image" "$build_context" >/dev/null
docker save -o "$tmp/image.tar" "$image"; digest=$(sha256sum "$tmp/image.tar" | awk '{print $1}')
cp "$worktree/infra/independent-test-station/compose.yaml" "$tmp/compose.yaml"; cp "$worktree/infra/independent-test-station/Caddyfile" "$tmp/Caddyfile"; printf '%s\n' "$digest" >"$tmp/image.sha256"
remote=$(ssh -T -o BatchMode=yes -o StrictHostKeyChecking=yes "$target" 'mktemp -d /var/tmp/sub2api-test-station-release.XXXXXX') || fail 'remote staging failed'
cleanup_remote(){ ssh -T -o BatchMode=yes -o StrictHostKeyChecking=yes "$target" "rm -rf -- '$remote'" >/dev/null 2>&1 || true; }
trap 'cleanup_remote; rm -rf -- "$tmp"' EXIT
scp -q "$tmp/image.tar" "$tmp/image.sha256" "$tmp/compose.yaml" "$tmp/Caddyfile" "$target:$remote/" || fail 'bundle transfer failed'
ssh -T -o BatchMode=yes -o StrictHostKeyChecking=yes "$target" "sudo -n bash -s -- --staging-root '$remote' --image-archive '$remote/image.tar' --image-sha256 '$digest' --compose '$remote/compose.yaml' --caddy '$remote/Caddyfile' --source-commit '$source_commit' --source-tree '$source_tree' --deploy-root '$deploy_root'" <"$worktree/ops/deploy-sub2api-test-station-host.sh" || fail 'remote executor failed'
printf 'test_station_release status=succeeded source_commit=%s source_tree=%s\n' "$source_commit" "$source_tree"
