#!/usr/bin/env bash
set -euo pipefail
umask 077
fail(){ printf 'test_station_release status=failed: %s\n' "$1" >&2; exit 1; }
sha256_file(){ if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1"|awk '{print $1}'; else shasum -a 256 "$1"|awk '{print $1}'; fi; }

worktree=${RELEASE_WORKTREE:-$(pwd -P)}
[[ "$worktree" == /* && -d "$worktree" ]] || fail 'worktree is invalid'
worktree=$(cd "$worktree" && pwd -P)
[[ "$(git -C "$worktree" branch --show-current)" == main ]] || fail 'release must originate from main'
[[ -z "$(git -C "$worktree" status --porcelain)" ]] || fail 'worktree is dirty'
git -C "$worktree" fetch origin main >/dev/null 2>&1 || fail 'origin fetch failed'
[[ "$(git -C "$worktree" rev-parse HEAD)" == "$(git -C "$worktree" rev-parse origin/main)" ]] || fail 'main is not equal to origin/main'
source_commit=$(git -C "$worktree" rev-parse HEAD)
source_tree=$(git -C "$worktree" rev-parse 'HEAD^{tree}')
[[ "$source_commit" =~ ^[a-f0-9]{40}$ && "$source_tree" =~ ^[a-f0-9]{40}$ ]] || fail 'source identity is invalid'

target=${TEST_STATION_SSH_TARGET:-sub2api-test-station}
[[ "$target" == sub2api-test-station ]] || fail 'unsafe SSH target'
deploy_root=/opt/sub2api-test-station
build_context="$worktree/upstream/sub2api"
migrations_dir="$build_context/backend/migrations"
compose_source="$worktree/infra/independent-test-station/compose.yaml"
caddy_source="$worktree/infra/independent-test-station/Caddyfile"
host_executor="$worktree/ops/deploy-sub2api-test-station-host.sh"
backup_helper="$worktree/ops/backup-sub2api-test-station-host.sh"
[[ -d "$build_context" && ! -L "$build_context" ]] || fail 'build context missing'
[[ -d "$migrations_dir" && ! -L "$migrations_dir" ]] || fail 'migration directory is invalid'
for path in "$compose_source" "$caddy_source" "$host_executor" "$backup_helper"; do
  [[ -f "$path" && ! -L "$path" ]] || fail 'release source file is invalid'
done
command -v docker >/dev/null 2>&1 || fail 'Docker is required'
command -v ssh >/dev/null 2>&1 || fail 'SSH is required'
command -v scp >/dev/null 2>&1 || fail 'SCP is required'
command -v ruby >/dev/null 2>&1 || fail 'Ruby is required'

# Release steps can spend several minutes in quiet Docker/Compose operations.
# Keep the control connection alive explicitly instead of relying on a user's
# local SSH config, and allow transient packet loss before declaring failure.
ssh_opts=(
  -o BatchMode=yes
  -o ConnectTimeout=15
  -o ServerAliveInterval=10
  -o ServerAliveCountMax=18
  -o TCPKeepAlive=yes
  -o StrictHostKeyChecking=yes
)
release_reconcile_attempts=${TEST_STATION_RELEASE_RECONCILE_ATTEMPTS:-6}
release_reconcile_interval=${TEST_STATION_RELEASE_RECONCILE_INTERVAL_SECONDS:-10}
[[ "$release_reconcile_attempts" =~ ^[1-9][0-9]*$ ]] || fail 'release reconciliation attempts are invalid'
[[ "$release_reconcile_interval" =~ ^[0-9]+$ ]] || fail 'release reconciliation interval is invalid'

remote_release_succeeded(){
  ssh -T "${ssh_opts[@]}" "$target" \
    "sudo -n python3 -c 'import json,sys; value=json.load(open(\"/opt/sub2api-test-station/release-state.json\",encoding=\"utf-8\")); raise SystemExit(0 if value.get(\"source_commit\")==sys.argv[1] and value.get(\"source_tree\")==sys.argv[2] and value.get(\"result\")==\"succeeded\" and value.get(\"rolled_back\") is False else 1)' '$source_commit' '$source_tree'" \
    >/dev/null 2>&1
}

migration_set_sha256=$(ruby -rdigest -e '
  directory = ARGV.fetch(0)
  go_space = /[\u0009-\u000D\u0020\u0085\u00A0\u1680\u2000-\u200A\u2028\u2029\u202F\u205F\u3000]/
  files = Dir.children(directory).select { |name| name.end_with?(".sql") }.sort
  digest = Digest::SHA256.new
  files.each do |name|
    content = File.binread(File.join(directory, name)).force_encoding(Encoding::UTF_8)
    abort "migration is not valid UTF-8: #{name}" unless content.valid_encoding?
    content = content.sub(/\A#{go_space}+/, "").sub(/#{go_space}+\z/, "")
    next if content.empty?
    digest << name << "\0" << Digest::SHA256.hexdigest(content) << "\n"
  end
  print digest.hexdigest
' "$migrations_dir") || fail 'could not compute migration hash'
[[ "$migration_set_sha256" =~ ^[a-f0-9]{64}$ ]] || fail 'migration hash is invalid'

tmp=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-test-station-release.XXXXXX")
trap 'rm -rf -- "$tmp"' EXIT
image="sub2api-test-station-runtime:$source_commit"
docker buildx build --platform linux/amd64 --load -t "$image" "$build_context" >/dev/null
image_id=$(docker image inspect --format '{{.Id}}' "$image" 2>/dev/null | tr -d '[:space:]')
[[ "$image_id" =~ ^sha256:[a-f0-9]{64}$ ]] || fail 'built image identity is invalid'
docker save -o "$tmp/image.tar" "$image"
archive_sha256=$(sha256_file "$tmp/image.tar")
[[ "$archive_sha256" =~ ^[a-f0-9]{64}$ ]] || fail 'image archive checksum is invalid'
cp "$compose_source" "$tmp/compose.yaml"
cp "$caddy_source" "$tmp/Caddyfile"
cp "$backup_helper" "$tmp/backup-sub2api-test-station-host.sh"
cp "$host_executor" "$tmp/deploy-sub2api-test-station-host.sh"
chmod 0700 "$tmp/backup-sub2api-test-station-host.sh" "$tmp/deploy-sub2api-test-station-host.sh"
printf '%s\n' "$archive_sha256" >"$tmp/image.sha256"

remote=$(ssh -T "${ssh_opts[@]}" "$target" 'mktemp -d /var/tmp/sub2api-test-station-release.XXXXXX') || fail 'remote staging failed'
cleanup_remote(){ ssh -T "${ssh_opts[@]}" "$target" "rm -rf -- '$remote'" >/dev/null 2>&1 || true; }
trap 'cleanup_remote; rm -rf -- "$tmp"' EXIT
scp -q "${ssh_opts[@]}" "$tmp/image.tar" "$tmp/image.sha256" "$tmp/compose.yaml" "$tmp/Caddyfile" \
  "$tmp/backup-sub2api-test-station-host.sh" "$tmp/deploy-sub2api-test-station-host.sh" \
  "$target:$remote/" || fail 'bundle transfer failed'
if ! ssh -T "${ssh_opts[@]}" "$target" \
  "sudo -n bash '$remote/deploy-sub2api-test-station-host.sh' --staging-root '$remote' --image-archive '$remote/image.tar' --image-sha256 '$archive_sha256' --image-id '$image_id' --compose '$remote/compose.yaml' --caddy '$remote/Caddyfile' --backup-script '$remote/backup-sub2api-test-station-host.sh' --source-commit '$source_commit' --source-tree '$source_tree' --migration-set-sha256 '$migration_set_sha256' --deploy-root '$deploy_root'"; then
  reconciled=false
  for ((attempt=1; attempt<=release_reconcile_attempts; attempt++)); do
    if remote_release_succeeded; then
      reconciled=true
      break
    fi
    ((attempt < release_reconcile_attempts && release_reconcile_interval > 0)) && sleep "$release_reconcile_interval"
  done
  [[ "$reconciled" == true ]] || fail 'remote executor failed and release state could not be reconciled'
fi
printf 'test_station_release status=succeeded source_commit=%s source_tree=%s\n' "$source_commit" "$source_tree"
