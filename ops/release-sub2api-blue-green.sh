#!/usr/bin/env bash
set -euo pipefail

umask 077

fail() {
  printf 'sub2api_blue_green_release status=failed: %s\n' "$1" >&2
  exit 1
}

mode=''
evidence=''
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
    *) fail "unknown argument: $1" ;;
  esac
done

[[ "$mode" == rehearsal || "$mode" == production ]] || fail '--mode must be rehearsal or production'
[[ "$evidence" == /* && -f "$evidence" && -r "$evidence" && ! -L "$evidence" ]] \
  || fail '--evidence must be an absolute readable non-symlink file'
evidence_parent=$(dirname "$evidence")
[[ -d "$evidence_parent" && ! -L "$evidence_parent" ]] || fail '--evidence parent must be a non-symlink directory'
evidence_parent_physical=$(cd "$evidence_parent" && pwd -P)
[[ "$evidence_parent_physical/$(basename "$evidence")" == "$evidence" ]] || fail '--evidence path must be canonical'
mode_of() { stat -f '%Lp' "$1" 2>/dev/null || stat -c '%a' "$1"; }
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

migrations_dir="$worktree/upstream/sub2api/backend/migrations"
[[ -d "$migrations_dir" && ! -L "$migrations_dir" ]] || fail 'migration directory is invalid'
migrations_hash=$(ruby -rdigest -e '
  directory = ARGV.fetch(0)
  files = Dir.children(directory).select { |name| name.end_with?(".sql") }.sort
  digest = Digest::SHA256.new
  files.each do |name|
    content = File.binread(File.join(directory, name)).strip
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
image_repository=${SUB2API_IMAGE_REPOSITORY:-}
build_context=${RELEASE_BUILD_CONTEXT:-"$worktree/upstream/sub2api"}
ssh_target=${RELEASE_SSH_TARGET:-}
ssh_key=${RELEASE_SSH_KEY:-}
ssh_known_hosts=${RELEASE_SSH_KNOWN_HOSTS:-}
ssh_port=${RELEASE_SSH_PORT:-}
host_executor=${RELEASE_HOST_EXECUTOR_PATH:-/usr/local/libexec/deploy-sub2api-blue-green-host.sh}

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
[[ "$host_executor" =~ ^/[A-Za-z0-9._/-]+$ ]] || fail 'RELEASE_HOST_EXECUTOR_PATH is invalid'
command -v "$docker_bin" >/dev/null 2>&1 || fail 'Docker Buildx is required'
command -v "$ssh_bin" >/dev/null 2>&1 || fail 'SSH is required'
command -v perl >/dev/null 2>&1 || fail 'Perl is required for stage timeouts'

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
stage_timeout_seconds=${RELEASE_STAGE_TIMEOUT_SECONDS:-600}
[[ "$stage_timeout_seconds" =~ ^[1-9][0-9]*$ && "$stage_timeout_seconds" -le 600 ]] \
  || fail 'RELEASE_STAGE_TIMEOUT_SECONDS must be an integer no greater than 600'
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

tag="$image_repository:release-$source_commit"
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
  --tag "$tag" \
  "$build_context"

digest_timeout=$(stage_timeout)
digest=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$digest_timeout" "$docker_bin" buildx imagetools inspect --format '{{.Manifest.Digest}}' "$tag") \
  || fail 'digest resolution failed or exceeded its timeout'
check_budget
digest=$(printf '%s' "$digest" | tr -d '[:space:]')
[[ "$digest" =~ ^sha256:[a-f0-9]{64}$ ]] || fail 'published image did not resolve to an immutable sha256 digest'
immutable_image="$image_repository@$digest"

host_output=''
host_status=0
host_timeout=$(stage_timeout)
set +e
host_output=$(perl -e 'alarm shift @ARGV; exec @ARGV' "$host_timeout" "$ssh_bin" \
  -T -i "$ssh_key" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
  -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" \
  bash "$host_executor" --mode "$mode" --image "$immutable_image" \
  --source-commit "$source_commit" --source-tree "$source_tree" --tested-tree "$source_tree" \
  --migrations-hash "$migrations_hash")
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
