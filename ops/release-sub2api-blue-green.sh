#!/usr/bin/env bash
set -euo pipefail

umask 077

fail() {
  printf 'sub2api_blue_green_release status=failed: %s\n' "$1" >&2
  exit 1
}

mode=''
evidence=''
maintenance_authorized=false
while (($#)); do
  case "$1" in
    --mode)
      (($# >= 2)) || fail '--mode requires a value'
      [[ -z "$mode" ]] || fail '--mode may be supplied once'
      mode=$2
      shift 2
      ;;
    --evidence)
      (($# >= 2)) || fail '--evidence requires a value'
      [[ -z "$evidence" ]] || fail '--evidence may be supplied once'
      evidence=$2
      shift 2
      ;;
		--maintenance-authorized)
			[[ "$maintenance_authorized" == false ]] || fail '--maintenance-authorized may be supplied once'
			maintenance_authorized=true
			shift
			;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ "$mode" == rehearsal || "$mode" == production ]] || fail '--mode must be rehearsal or production'
[[ "$maintenance_authorized" == false || "$mode" == production ]] || fail '--maintenance-authorized is only valid in production mode'
maintenance_from_hash=${RELEASE_MAINTENANCE_FROM_HASH:-}
if [[ "$maintenance_authorized" == true ]]; then
  [[ "$maintenance_from_hash" =~ ^[a-f0-9]{64}$ ]] \
    || fail 'RELEASE_MAINTENANCE_FROM_HASH must be 64 lowercase hex when maintenance is authorized'
else
  [[ -z "$maintenance_from_hash" ]] \
    || fail 'RELEASE_MAINTENANCE_FROM_HASH requires --maintenance-authorized'
fi
[[ "$evidence" == /* && -f "$evidence" && -r "$evidence" && ! -L "$evidence" ]] \
  || fail '--evidence must be an absolute readable non-symlink file'
evidence_parent=$(dirname "$evidence")
[[ -d "$evidence_parent" && ! -L "$evidence_parent" ]] || fail '--evidence parent must be a non-symlink directory'
evidence_parent_physical=$(cd "$evidence_parent" && pwd -P)
[[ "$evidence_parent_physical/$(basename "$evidence")" == "$evidence" ]] || fail '--evidence path must be canonical'
mode_of() { stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"; }
sha256_file() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi; }
[[ "$(mode_of "$evidence")" == 600 ]] || fail '--evidence mode must be 0600'

worktree=${RELEASE_WORKTREE:-$(pwd -P)}
[[ "$worktree" == /* && -d "$worktree" && ! -L "$worktree" ]] || fail 'RELEASE_WORKTREE must be an absolute non-symlink directory'
worktree_physical=$(cd "$worktree" && pwd -P)
[[ "$worktree_physical" == "$worktree" ]] || fail 'RELEASE_WORKTREE must be canonical'
[[ -d "$worktree/.git" || -f "$worktree/.git" ]] || fail 'RELEASE_WORKTREE must be a Git worktree'
[[ -z "$(git -C "$worktree" status --porcelain)" ]] || fail 'worktree is dirty'
source_commit=$(git -C "$worktree" rev-parse HEAD) || fail 'could not resolve source commit'
source_tree=$(git -C "$worktree" rev-parse 'HEAD^{tree}') || fail 'could not resolve source tree'
[[ "$source_commit" =~ ^[a-f0-9]{40}$ && "$source_tree" =~ ^[a-f0-9]{40}$ ]] || fail 'Git identity is invalid'
if [[ "$mode" == production ]]; then
  "$worktree/ops/assert-sub2api-release-source.sh" --mode production --worktree "$worktree" \
    || fail 'production release source freshness check failed'
fi

migrations_dir="$worktree/upstream/sub2api/backend/migrations"
[[ -d "$migrations_dir" && ! -L "$migrations_dir" ]] || fail 'migration directory is invalid'
migrations_hash=$(ruby -rdigest -e '
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
[[ "$migrations_hash" =~ ^[a-f0-9]{64}$ ]] || fail 'migration hash is invalid'

ruby -rjson -rtime -e '
  path, commit, tree, migrations = ARGV
  value = JSON.parse(File.binread(path))
  expected_keys = %w[commands created_at migrations_hash result schema_version source_commit tested_tree]
  abort "evidence keys are invalid" unless value.is_a?(Hash) && value.keys.sort == expected_keys
  abort "evidence schema is invalid" unless value["schema_version"] == 1 && value["result"] == "passed"
  abort "evidence command list is invalid" unless value["commands"].is_a?(Array) && !value["commands"].empty? && value["commands"].all? { |command| command.is_a?(String) && !command.empty? && !command.match?(/[\r\n]/) }
  abort "evidence timestamp is invalid" unless value["created_at"].is_a?(String) && value["created_at"].match?(/\A\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ\z/) && Time.iso8601(value["created_at"]).utc?
  abort "evidence identity is invalid" unless [value["source_commit"], value["tested_tree"]].all? { |id| id.is_a?(String) && id.match?(/\A[0-9a-f]{40}\z/) } && value["migrations_hash"].is_a?(String) && value["migrations_hash"].match?(/\A[0-9a-f]{64}\z/)
  abort "evidence source commit does not match worktree" unless value["source_commit"] == commit
  abort "evidence tested tree does not match worktree" unless value["tested_tree"] == tree
  abort "evidence migrations hash does not match worktree" unless value["migrations_hash"] == migrations
' "$evidence" "$source_commit" "$source_tree" "$migrations_hash" \
  || fail 'evidence validation failed'

docker_bin=${RELEASE_DOCKER_BIN:-docker}
ssh_bin=${RELEASE_SSH_BIN:-ssh}
scp_bin=${RELEASE_SCP_BIN:-scp}
image_repository=${SUB2API_IMAGE_REPOSITORY:-}
build_context=${RELEASE_BUILD_CONTEXT:-"$worktree/upstream/sub2api"}
ssh_target=${RELEASE_SSH_TARGET:-}
ssh_key=${RELEASE_SSH_KEY:-}
ssh_known_hosts=${RELEASE_SSH_KNOWN_HOSTS:-}
ssh_port=${RELEASE_SSH_PORT:-}
readonly fixed_host_executor='/usr/local/libexec/deploy-sub2api-blue-green-host.sh'
host_executor=${RELEASE_HOST_EXECUTOR_PATH:-$fixed_host_executor}
host_executor_source=${RELEASE_HOST_EXECUTOR_SOURCE:-}
transport=${RELEASE_TRANSPORT:-registry}
release_staging_root=${RELEASE_STAGING_ROOT:-/var/lib/sub2api/release-staging}
[[ "$release_staging_root" == /* && "$release_staging_root" != */ && ! -L "$release_staging_root" ]] \
  || fail 'RELEASE_STAGING_ROOT is invalid'
if [[ "$mode" == rehearsal ]]; then
  [[ -n "${REHEARSAL_ROOT:-}" ]] || fail 'REHEARSAL_ROOT is required in rehearsal mode'
  [[ "$REHEARSAL_ROOT" == /* && "$REHEARSAL_ROOT" != */ && ! -L "$REHEARSAL_ROOT" ]] \
    || fail 'REHEARSAL_ROOT is invalid'
fi

[[ "$image_repository" =~ ^[a-z0-9][a-z0-9._/-]*$ && "$image_repository" == */* ]] || fail 'SUB2API_IMAGE_REPOSITORY is invalid'
[[ "$build_context" == /* && -d "$build_context" && ! -L "$build_context" ]] || fail 'RELEASE_BUILD_CONTEXT is invalid'
build_context_physical=$(cd "$build_context" && pwd -P) || fail 'RELEASE_BUILD_CONTEXT cannot be canonicalized'
case "$build_context_physical" in
  "$worktree_physical"|"$worktree_physical"/*) ;;
  *) fail 'RELEASE_BUILD_CONTEXT must be inside RELEASE_WORKTREE' ;;
esac
[[ "$ssh_target" =~ ^[A-Za-z0-9._-]+@[A-Za-z0-9._-]+$ ]] || fail 'RELEASE_SSH_TARGET is invalid'
[[ "$ssh_key" == /* && -f "$ssh_key" && ! -L "$ssh_key" && "$(mode_of "$ssh_key")" == 600 ]] || fail 'RELEASE_SSH_KEY must be a 0600 non-symlink file'
[[ "$ssh_known_hosts" == /* && -f "$ssh_known_hosts" && ! -L "$ssh_known_hosts" && "$(mode_of "$ssh_known_hosts")" == 600 ]] || fail 'RELEASE_SSH_KNOWN_HOSTS must be a 0600 non-symlink file'
[[ "$ssh_port" =~ ^[1-9][0-9]{0,4}$ && "$ssh_port" -le 65535 ]] || fail 'RELEASE_SSH_PORT is invalid'
[[ "$host_executor" == "$fixed_host_executor" ]] || fail 'RELEASE_HOST_EXECUTOR_PATH must use the fixed production executor path'
[[ "$transport" == registry || "$transport" == preloaded ]] || fail 'RELEASE_TRANSPORT must be registry or preloaded'
command -v "$docker_bin" >/dev/null 2>&1 || fail 'Docker Buildx is required'
command -v "$ssh_bin" >/dev/null 2>&1 || fail 'SSH is required'
command -v "$scp_bin" >/dev/null 2>&1 || fail 'SCP is required to install the host executor'
command -v perl >/dev/null 2>&1 || fail 'Perl is required for stage timeouts'

if [[ -z "$host_executor_source" ]]; then
  host_executor_source="$worktree/ops/deploy-sub2api-blue-green-host.sh"
fi
[[ "$host_executor_source" == "$worktree/ops/deploy-sub2api-blue-green-host.sh" ]] \
  || fail 'RELEASE_HOST_EXECUTOR_SOURCE must use the fixed worktree executor path'
[[ "$host_executor_source" == /* && -f "$host_executor_source" && -r "$host_executor_source" && ! -L "$host_executor_source" ]] \
  || fail 'RELEASE_HOST_EXECUTOR_SOURCE must be an absolute readable non-symlink file'
host_executor_source_parent=$(dirname "$host_executor_source")
host_executor_source_physical=$(cd "$host_executor_source_parent" && pwd -P) \
  || fail 'RELEASE_HOST_EXECUTOR_SOURCE parent cannot be canonicalized'
[[ "$host_executor_source_physical/$(basename "$host_executor_source")" == "$host_executor_source" ]] \
  || fail 'RELEASE_HOST_EXECUTOR_SOURCE must be canonical'
case "$host_executor_source" in
  "$worktree_physical"/*) ;;
  *) fail 'RELEASE_HOST_EXECUTOR_SOURCE must be inside RELEASE_WORKTREE' ;;
esac
host_executor_sha256=$(sha256_file "$host_executor_source" 2>/dev/null) || fail 'host executor checksum failed'
[[ "$host_executor_sha256" =~ ^[a-f0-9]{64}$ ]] || fail 'host executor checksum is invalid'

monotonic_bin=${RELEASE_MONOTONIC_BIN:-}
if [[ -n "$monotonic_bin" ]]; then
  [[ "$monotonic_bin" == /* && -x "$monotonic_bin" && ! -L "$monotonic_bin" ]] || fail 'RELEASE_MONOTONIC_BIN is invalid'
fi
monotonic_now() {
  if [[ -n "$monotonic_bin" ]]; then
    "$monotonic_bin"
  else
    ruby -e 'print Process.clock_gettime(Process::CLOCK_MONOTONIC).to_i'
  fi
}
started=$(monotonic_now)
[[ "$started" =~ ^[0-9]+$ ]] || fail 'monotonic clock is invalid'
budget_seconds=1800
# A stage may consume the remaining release budget.  Slow but live preloaded
# transfers must not be rejected by an arbitrary per-stage 600-second cap.
stage_timeout_seconds=${RELEASE_STAGE_TIMEOUT_SECONDS:-$budget_seconds}
[[ "$stage_timeout_seconds" =~ ^[1-9][0-9]*$ ]] \
  || fail 'RELEASE_STAGE_TIMEOUT_SECONDS must be a positive integer'
check_budget() {
  local now elapsed
  now=$(monotonic_now)
  [[ "$now" =~ ^[0-9]+$ ]] || fail 'monotonic clock is invalid'
  (( now >= started )) || fail 'monotonic clock moved backwards'
  elapsed=$((now - started))
  (( elapsed <= budget_seconds )) || fail 'release exceeded the 1800-second total budget'
}
stage_timeout() {
  local now elapsed remaining
  now=$(monotonic_now)
  [[ "$now" =~ ^[0-9]+$ ]] || fail 'monotonic clock is invalid'
  (( now >= started )) || fail 'monotonic clock moved backwards'
  elapsed=$((now - started))
  remaining=$((budget_seconds - elapsed))
  (( remaining > 0 )) || fail 'release exceeded the 1800-second total budget'
  if (( remaining < stage_timeout_seconds )); then
    printf '%s\n' "$remaining"
  else
    printf '%s\n' "$stage_timeout_seconds"
  fi
}
run_stage() {
  local stage=$1 timeout
  shift
  timeout=$(stage_timeout)
  perl -e 'alarm shift @ARGV; exec @ARGV' "$timeout" "$@" || fail "$stage stage failed or exceeded its timeout"
  check_budget
}

verify_remote_executor_directory_chain() {
  local verification_script
  verification_script='# verify_executor_directory_chain
set -eu
path=$1
current=$path
while :; do
  [[ ! -L "$current" ]] || exit 1
  metadata=$(stat -c "%u:%a:%F" -- "$current") || exit 1
  IFS=: read -r owner mode kind <<<"$metadata"
  [[ "$owner" == 0 ]] || exit 1
  [[ "$mode" =~ ^[0-7]+$ ]] || exit 1
  (( (8#$mode & 8#022) == 0 )) || exit 1
  [[ "$kind" == "directory" ]] || exit 1
  [[ "$current" == / ]] && break
  current=${current%/*}
  [[ -n "$current" ]] || current=/
done'
  check_budget
  printf '%s\n' "$verification_script" | perl -e 'alarm shift @ARGV; exec @ARGV' "$(stage_timeout)" "$ssh_bin" -T -i "$ssh_key" \
    -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
    -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" \
    sudo -n bash -s -- "$(dirname "$host_executor")" \
    >/dev/null 2>&1 || fail 'remote host executor parent chain is not root-owned, non-symlink, and non-writable'
  check_budget
}

verify_remote_executor_path_chain() {
  local verification_script
  verification_script='# verify_executor_path_chain
set -eu
path=$1
current=$path
while :; do
  [[ ! -L "$current" ]] || exit 1
  metadata=$(stat -c "%u:%a:%F" -- "$current") || exit 1
  IFS=: read -r owner mode kind <<<"$metadata"
  [[ "$owner" == 0 ]] || exit 1
  [[ "$mode" =~ ^[0-7]+$ ]] || exit 1
  (( (8#$mode & 8#022) == 0 )) || exit 1
  if [[ "$current" == "$path" ]]; then
    [[ "$kind" == "regular file" ]] || exit 1
  else
    [[ "$kind" == "directory" ]] || exit 1
  fi
  [[ "$current" == / ]] && break
  current=${current%/*}
  [[ -n "$current" ]] || current=/
done'
  check_budget
  printf '%s\n' "$verification_script" | perl -e 'alarm shift @ARGV; exec @ARGV' "$(stage_timeout)" "$ssh_bin" -T -i "$ssh_key" \
    -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
    -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" \
    sudo -n bash -s -- "$host_executor" \
    >/dev/null 2>&1 || fail 'remote host executor path chain is not root-owned, non-symlink, and non-writable'
  check_budget
}

registry_tag="$image_repository:release-$source_commit"
archive=''
archive_sha256=''
image_id=''
remote_tmp=''
remote_executor_tmp=''
remote_executor_dest_tmp=''
staged_archive="$release_staging_root/sub2api-$source_commit.tar"
cleanup_archive() { [[ -z "$archive" ]] || rm -f -- "$archive"; }
cleanup_remote_executor_staging() {
  local cleanup_paths=()
  [[ "$remote_tmp" =~ ^/tmp/\.sub2api-$source_commit\.[A-Za-z0-9]{6}$ ]] && cleanup_paths+=("$remote_tmp")
  [[ "$remote_executor_tmp" =~ ^/tmp/\.sub2api-host-executor-$source_commit\.[A-Za-z0-9]{6}$ ]] && cleanup_paths+=("$remote_executor_tmp")
  [[ "$remote_executor_dest_tmp" == "/usr/local/libexec/.deploy-sub2api-blue-green-host.sh."* ]] && cleanup_paths+=("$remote_executor_dest_tmp")
  (( ${#cleanup_paths[@]} > 0 )) || return 0
  perl -e 'alarm shift @ARGV; exec @ARGV' 30 "$ssh_bin" -T -i "$ssh_key" \
    -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
    -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" \
    sudo -n rm -f -- "${cleanup_paths[@]}" >/dev/null 2>&1 || true
}
cleanup() {
  cleanup_remote_executor_staging
  cleanup_archive
}
trap cleanup EXIT

# Install and attest the exact host executor before any image build. A stale or
# tampered remote executor must fail the release before it can consume build
# time or mutate production.
host_executor_dir=$(dirname "$host_executor")
host_executor_basename=$(basename "$host_executor")
verify_remote_executor_directory_chain
check_budget
remote_executor_tmp=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$(stage_timeout)" "$ssh_bin" -T -i "$ssh_key" \
  -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
  -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" \
  umask 077 '&&' mktemp -p /tmp ".sub2api-host-executor-$source_commit.XXXXXX" 2>/dev/null | tr -d '[:space:]') \
  || fail 'remote host executor staging allocation failed'
[[ "$remote_executor_tmp" =~ ^/tmp/\.sub2api-host-executor-$source_commit\.[A-Za-z0-9]{6}$ ]] \
  || fail 'remote host executor staging path is invalid'
run_stage executor-transfer "$scp_bin" -C -q -i "$ssh_key" -o BatchMode=yes -o IdentitiesOnly=yes \
  -o StrictHostKeyChecking=yes -o "UserKnownHostsFile=$ssh_known_hosts" -P "$ssh_port" \
  "$host_executor_source" "$ssh_target:$remote_executor_tmp"
check_budget
remote_executor_dest_tmp=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$(stage_timeout)" "$ssh_bin" -T -i "$ssh_key" \
  -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
  -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" \
  sudo -n mktemp -p "$host_executor_dir" ".${host_executor_basename}.XXXXXX" 2>/dev/null | tr -d '[:space:]') \
  || fail 'remote host executor destination staging allocation failed'
[[ "$remote_executor_dest_tmp" == "$host_executor_dir/.$host_executor_basename."* ]] \
  || fail 'remote host executor destination staging path is invalid'
remote_executor_dest_suffix=${remote_executor_dest_tmp##*.}
[[ "$remote_executor_dest_suffix" =~ ^[A-Za-z0-9]{6}$ ]] \
  || fail 'remote host executor destination staging suffix is invalid'
remote_executor_metadata=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$(stage_timeout)" "$ssh_bin" -T -i "$ssh_key" \
  -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
  -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" \
  sudo -n install -o root -g root -m 0755 "$remote_executor_tmp" "$remote_executor_dest_tmp" '&&' \
  sudo -n bash -n "$remote_executor_dest_tmp" '&&' sudo -n stat -c '%u:%g:%a' "$remote_executor_dest_tmp" 2>/dev/null) \
  || fail 'remote host executor staging installation or syntax validation failed'
check_budget
remote_executor_metadata=$(printf '%s\n' "$remote_executor_metadata" | awk 'NF { print $1; exit }')
[[ "$remote_executor_metadata" == 0:0:755 || "$remote_executor_metadata" == 0:0:700 ]] \
  || fail 'remote staged host executor ownership or mode is invalid'
remote_executor_sha256=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$(stage_timeout)" "$ssh_bin" -T -i "$ssh_key" \
  -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
  -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" \
  sudo -n sha256sum "$remote_executor_dest_tmp" 2>/dev/null) \
  || fail 'remote staged host executor checksum verification failed'
check_budget
remote_executor_sha256=$(printf '%s\n' "$remote_executor_sha256" | awk 'NF { print $1; exit }')
[[ "$remote_executor_sha256" == "$host_executor_sha256" ]] \
  || fail 'remote staged host executor checksum does not match source'
remote_executor_metadata=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$(stage_timeout)" "$ssh_bin" -T -i "$ssh_key" \
  -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
  -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" \
  sudo -n mv -f -- "$remote_executor_dest_tmp" "$host_executor" '&&' \
  sudo -n bash -n "$host_executor" '&&' sudo -n stat -c '%u:%g:%a' "$host_executor" 2>/dev/null) \
  || fail 'remote host executor atomic installation or syntax validation failed'
check_budget
remote_executor_metadata=$(printf '%s\n' "$remote_executor_metadata" | awk 'NF { print $1; exit }')
[[ "$remote_executor_metadata" == 0:0:755 || "$remote_executor_metadata" == 0:0:700 ]] \
  || fail 'remote host executor ownership or mode is invalid'
remote_executor_sha256=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$(stage_timeout)" "$ssh_bin" -T -i "$ssh_key" \
  -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
  -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" \
  sudo -n sha256sum "$host_executor" '&&' sudo -n rm -f -- "$remote_executor_tmp" 2>/dev/null) \
  || fail 'remote host executor checksum verification or staging cleanup failed'
check_budget
remote_executor_sha256=$(printf '%s\n' "$remote_executor_sha256" | awk 'NF { print $1; exit }')
[[ "$remote_executor_sha256" == "$host_executor_sha256" ]] \
  || fail 'remote host executor checksum does not match source'
remote_executor_dest_tmp=''
remote_executor_tmp=''
verify_remote_executor_path_chain

if [[ "$transport" == registry ]]; then
  run_stage build "$docker_bin" buildx build \
    --platform linux/amd64 \
    --provenance=false \
    --sbom=false \
    --push \
    --label com.xingqiao.sub2api.qualified=true \
    --label "com.xingqiao.sub2api.source.commit=$source_commit" \
    --label "com.xingqiao.sub2api.source.tree=$source_tree" \
    --label "com.xingqiao.sub2api.tested.tree=$source_tree" \
    --label "com.xingqiao.sub2api.migrations.sha256=$migrations_hash" \
    --tag "$registry_tag" \
    "$build_context"

  digest_timeout=$(stage_timeout)
  digest=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$digest_timeout" "$docker_bin" buildx imagetools inspect --format '{{.Manifest.Digest}}' "$registry_tag") \
    || fail 'digest resolution failed or exceeded its timeout'
  check_budget
  digest=$(printf '%s' "$digest" | tr -d '[:space:]')
  [[ "$digest" =~ ^sha256:[a-f0-9]{64}$ ]] || fail 'published image did not resolve to an immutable sha256 digest'
  immutable_image="$image_repository@$digest"
  preloaded_args=()
else
  temporary_tag_nonce=$(ruby -rsecurerandom -e 'print SecureRandom.hex(16)') \
    || fail 'could not generate a unique preloaded build tag'
  [[ "$temporary_tag_nonce" =~ ^[a-f0-9]{32}$ ]] || fail 'preloaded build tag nonce is invalid'
  temporary_tag="$image_repository:build-$source_commit-$temporary_tag_nonce"
  archive=$(mktemp "${TMPDIR:-/tmp}/sub2api-$source_commit.XXXXXX")
  run_stage build "$docker_bin" buildx build \
    --platform linux/amd64 \
    --provenance=false \
    --sbom=false \
    --load \
    --label com.xingqiao.sub2api.qualified=true \
    --label "com.xingqiao.sub2api.source.commit=$source_commit" \
    --label "com.xingqiao.sub2api.source.tree=$source_tree" \
    --label "com.xingqiao.sub2api.tested.tree=$source_tree" \
    --label "com.xingqiao.sub2api.migrations.sha256=$migrations_hash" \
    --tag "$temporary_tag" \
    "$build_context"
  image_id=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$(stage_timeout)" "$docker_bin" image inspect --format '{{.Id}}' "$temporary_tag" | tr -d '[:space:]') \
    || fail 'local image ID resolution failed'
  [[ "$image_id" =~ ^sha256:[a-f0-9]{64}$ ]] || fail 'preloaded image did not resolve to an immutable image ID'
  immutable_image="$image_repository:release-$source_commit-${image_id#sha256:}"
  run_stage tag "$docker_bin" image tag "$temporary_tag" "$immutable_image"
  tagged_image_id=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$(stage_timeout)" "$docker_bin" image inspect --format '{{.Id}}' "$immutable_image" | tr -d '[:space:]') \
    || fail 'preloaded image ID-bound tag inspection failed'
  [[ "$tagged_image_id" == "$image_id" ]] || fail 'preloaded image ID-bound tag does not resolve to the built image'
  run_stage archive "$docker_bin" image save --output "$archive" "$immutable_image"
  archive_sha256=$(sha256_file "$archive" 2>/dev/null)
  [[ "$archive_sha256" =~ ^[a-f0-9]{64}$ ]] || fail 'image archive checksum failed'
  remote_tmp=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$(stage_timeout)" "$ssh_bin" -T -i "$ssh_key" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" umask 077 '&&' mktemp -p /tmp ".sub2api-$source_commit.XXXXXX" 2>/dev/null | tr -d '[:space:]') \
    || fail 'remote staging allocation failed'
  [[ "$remote_tmp" =~ ^/tmp/\.sub2api-$source_commit\.[A-Za-z0-9]{6}$ ]] || fail 'remote staging path is invalid'
  run_stage transfer "$scp_bin" -C -q -i "$ssh_key" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o "UserKnownHostsFile=$ssh_known_hosts" -P "$ssh_port" "$archive" "$ssh_target:$remote_tmp"
  run_stage stage "$ssh_bin" -T -i "$ssh_key" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" sudo -n install -o root -g root -m 600 "$remote_tmp" "$staged_archive" '&&' sudo -n rm -f -- "$remote_tmp"
  preloaded_args=(--preloaded-archive "$staged_archive" --preloaded-archive-sha256 "$archive_sha256" --preloaded-image-id "$image_id")
fi

host_output=''
host_status=0
host_timeout=$(stage_timeout)
host_deadline_epoch=$(ruby -e 'print Time.now.utc.to_i + Integer(ARGV.fetch(0))' "$host_timeout") \
  || fail 'could not calculate host deadline'
[[ "$host_deadline_epoch" =~ ^[1-9][0-9]{9}$ ]] || fail 'host deadline is invalid'
set +e
verify_remote_executor_path_chain
host_environment=(
  "DEPLOY_ROOT=${RELEASE_DEPLOY_ROOT:-/opt/sub2api/production}"
  "BASE_COMPOSE=${RELEASE_BASE_COMPOSE:-/opt/sub2api/production/compose.yaml}"
  "SECRET_ENV=${RELEASE_SECRET_ENV:-/opt/sub2api/production/.env}"
  "RELEASE_ENV=${RELEASE_RELEASE_ENV:-/opt/sub2api/production/release.env}"
  "RELEASE_STATE=${RELEASE_STATE_PATH:-/var/lib/sub2api/release-state}"
  "RELEASE_RECORD_ROOT=${RELEASE_RECORD_ROOT_PATH:-/var/lib/sub2api/release-records}"
  "WORKER_HEALTH_TIMEOUT_SECONDS=${RELEASE_WORKER_HEALTH_TIMEOUT_SECONDS:-240}"
  "ADMIN_API_KEY_FILE=${RELEASE_ADMIN_API_KEY_FILE:-/opt/sub2api/production/secrets/sub2api-admin-api-key}"
  "GATEWAY_API_KEY_FILE=${RELEASE_GATEWAY_API_KEY_FILE:-/opt/sub2api/production/secrets/sub2api-gateway-api-key}"
  "BASE_URL=${RELEASE_BASE_URL:-https://api.xingqiaolab.top}"
  "RELEASE_STAGING_ROOT=$release_staging_root"
)
if [[ "$mode" == rehearsal ]]; then
  host_environment+=("REHEARSAL_ROOT=$REHEARSAL_ROOT")
fi
if [[ "$mode" == production ]]; then
  [[ -n "${RELEASE_NETWORK_CURL_IMAGE:-}" ]] || fail 'RELEASE_NETWORK_CURL_IMAGE is required for production'
  [[ -n "${RELEASE_NETWORK_CURL_IMAGE_ALLOWLIST:-}" ]] || fail 'RELEASE_NETWORK_CURL_IMAGE_ALLOWLIST is required for production'
  host_environment+=(
    "NETWORK_CURL_IMAGE=$RELEASE_NETWORK_CURL_IMAGE"
    "NETWORK_CURL_IMAGE_ALLOWLIST=$RELEASE_NETWORK_CURL_IMAGE_ALLOWLIST"
  )
fi
host_args=(
  sudo -n env "${host_environment[@]}"
)
if [[ "$transport" == preloaded ]]; then
  host_args+=(env RELEASE_PRELOADED_IMAGE=true)
fi
host_args+=(
  bash "$host_executor" --mode "$mode" --image "$immutable_image"
  --source-commit "$source_commit" --source-tree "$source_tree" --tested-tree "$source_tree"
  --migrations-hash "$migrations_hash" --deadline-epoch "$host_deadline_epoch"
)
if [[ "$transport" == preloaded ]]; then
  host_args+=("${preloaded_args[@]}")
fi
if [[ "$maintenance_authorized" == true ]]; then
  host_args+=(--maintenance-authorized --maintenance-from-hash \
    "$maintenance_from_hash")
fi
host_output=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$host_timeout" "$ssh_bin" \
  -T -i "$ssh_key" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
  -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" \
  "${host_args[@]}")
host_status=$?
set -e
check_budget

host_downtime=$(ruby -rjson -e '
  value = JSON.parse(STDIN.read)
  abort unless value.is_a?(Hash) && (value["downtime_required"] == true || value["downtime_required"] == false)
  print value["downtime_required"]
' <<<"$host_output") || fail 'host executor output is invalid'
if [[ "$host_downtime" == true ]]; then
  printf '%s\n' "$host_output"
  exit 2
fi
(( host_status == 0 )) || fail 'host executor failed'
ruby -rjson -e '
  value = JSON.parse(STDIN.read)
  abort unless value["downtime_required"] == false && value["result"] == "succeeded"
' <<<"$host_output" || fail 'host executor did not report success'
printf '%s\n' "$host_output"
