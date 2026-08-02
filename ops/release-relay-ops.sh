#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'relay_ops_release status=failed: %s\n' "$1" >&2; exit 1; }
mode=''; evidence=''
while (($#)); do
  case "$1" in
    --mode) (($# >= 2)) || fail '--mode requires a value'; [[ -z "$mode" ]] || fail '--mode may be supplied once'; mode=$2; shift 2 ;;
    --evidence) (($# >= 2)) || fail '--evidence requires a value'; [[ -z "$evidence" ]] || fail '--evidence may be supplied once'; evidence=$2; shift 2 ;;
    *) fail "unknown argument: $1" ;;
  esac
done
[[ "$mode" == production ]] || fail '--mode must be production'
mode_of() { stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"; }
owner_of() { stat -c '%u' "$1" 2>/dev/null || stat -f '%u' "$1"; }
secure_parent() { local p=$1 label=$2 mode; [[ -d "$p" && ! -L "$p" ]] || fail "$label is invalid"; [[ "$(cd "$p" && pwd -P)" == "$p" ]] || fail "$label must be canonical"; mode=$(mode_of "$p"); (( (8#$mode & 8#022) == 0 )) || fail "$label must not be group/other writable"; }
secure_credential() { local p=$1 label=$2; [[ "$p" == /* && -f "$p" && -r "$p" && ! -L "$p" ]] || fail "$label is invalid"; secure_parent "$(dirname "$p")" "$label parent"; [[ "$(cd "$(dirname "$p")" && pwd -P)/$(basename "$p")" == "$p" ]] || fail "$label must be canonical"; [[ "$(mode_of "$p")" == 600 ]] || fail "$label must be 0600"; [[ "$(owner_of "$p")" == "$(id -u)" ]] || fail "$label must be owned by the release user"; }
[[ "$evidence" == /* && -f "$evidence" && -r "$evidence" && ! -L "$evidence" ]] || fail '--evidence is invalid'
evidence_parent=$(dirname "$evidence"); secure_parent "$evidence_parent" '--evidence parent'; [[ "$(cd "$evidence_parent" && pwd -P)/$(basename "$evidence")" == "$evidence" ]] || fail '--evidence must be canonical'; [[ "$(mode_of "$evidence")" == 600 ]] || fail '--evidence mode must be 0600'

worktree=${RELEASE_WORKTREE:-$(pwd -P)}; [[ "$worktree" == /* && -d "$worktree" && ! -L "$worktree" ]] || fail 'RELEASE_WORKTREE is invalid'; [[ "$(cd "$worktree" && pwd -P)" == "$worktree" ]] || fail 'RELEASE_WORKTREE must be canonical'; [[ -d "$worktree/.git" || -f "$worktree/.git" ]] || fail 'RELEASE_WORKTREE is not a Git worktree'; [[ -z "$(git -C "$worktree" status --porcelain)" ]] || fail 'worktree is dirty'
source_commit=$(git -C "$worktree" rev-parse HEAD) || fail 'could not resolve source commit'; source_tree=$(git -C "$worktree" rev-parse 'HEAD^{tree}') || fail 'could not resolve source tree'
migrations_dir="$worktree/relay-ops-service/internal/store/migrations"; [[ -d "$migrations_dir" && ! -L "$migrations_dir" ]] || fail 'relay-ops migrations directory is invalid'
migrations_hash=$(ruby -rdigest -e 'dir=ARGV.fetch(0); ws=/[\u0009-\u000D\u0020\u0085\u00A0\u1680\u2000-\u200A\u2028\u2029\u202F\u205F\u3000]/; d=Digest::SHA256.new; Dir.children(dir).select{|n| n.end_with?(".sql")}.sort.each{|n| c=File.binread(File.join(dir,n)).force_encoding(Encoding::UTF_8); abort unless c.valid_encoding?; c=c.sub(/\A#{ws}+/,"").sub(/#{ws}+\z/,""); next if c.empty?; d << n << "\0" << Digest::SHA256.hexdigest(c) << "\n"}; print d.hexdigest' "$migrations_dir") || fail 'could not compute migrations hash'
ruby -rjson -e 'p,commit,tree,migrations=ARGV; v=JSON.parse(File.binread(p)); keys=%w[commands created_at migrations_hash result schema_version source_commit tested_tree]; abort unless v.is_a?(Hash)&&v.keys.sort==keys&&v["schema_version"]==1&&v["result"]=="passed"&&v["commands"].is_a?(Array)&&!v["commands"].empty?&&v["source_commit"]==commit&&v["tested_tree"]==tree&&v["migrations_hash"]==migrations' "$evidence" "$source_commit" "$source_tree" "$migrations_hash" || fail 'evidence validation failed'

docker_bin=${RELEASE_DOCKER_BIN:-docker}; ssh_bin=${RELEASE_SSH_BIN:-ssh}; image_repository=${RELAY_OPS_IMAGE_REPOSITORY:-}; build_context=${RELEASE_BUILD_CONTEXT:-$worktree}; ssh_target=${RELEASE_SSH_TARGET:-}; ssh_key=${RELEASE_SSH_KEY:-}; ssh_known_hosts=${RELEASE_SSH_KNOWN_HOSTS:-}; ssh_port=${RELEASE_SSH_PORT:-}; host_executor=${RELEASE_HOST_EXECUTOR_PATH:-/usr/local/libexec/deploy-relay-ops-host.sh}
[[ "$image_repository" =~ ^[a-z0-9][a-z0-9._/-]*/[a-z0-9][a-z0-9._/-]*$ ]] || fail 'RELAY_OPS_IMAGE_REPOSITORY is invalid'; [[ "$build_context" == /* && -d "$build_context" && ! -L "$build_context" && "$(cd "$build_context" && pwd -P)" == "$build_context" ]] || fail 'RELEASE_BUILD_CONTEXT is invalid'; case "$build_context" in "$worktree"|"$worktree"/*) ;; *) fail 'RELEASE_BUILD_CONTEXT must be inside RELEASE_WORKTREE' ;; esac
[[ "$ssh_target" =~ ^[A-Za-z0-9._-]+@[A-Za-z0-9._-]+$ ]] || fail 'RELEASE_SSH_TARGET is invalid'; secure_credential "$ssh_key" RELEASE_SSH_KEY; secure_credential "$ssh_known_hosts" RELEASE_SSH_KNOWN_HOSTS; [[ "$ssh_port" =~ ^[1-9][0-9]{0,4}$ && "$ssh_port" -le 65535 ]] || fail 'RELEASE_SSH_PORT is invalid'; [[ "$host_executor" =~ ^/[A-Za-z0-9._/-]+$ ]] || fail 'RELEASE_HOST_EXECUTOR_PATH is invalid'; command -v "$docker_bin" >/dev/null 2>&1 || fail 'Docker Buildx is required'; command -v "$ssh_bin" >/dev/null 2>&1 || fail 'SSH is required'; command -v perl >/dev/null 2>&1 || fail 'Perl is required'
tag="$image_repository:release-$source_commit"; timeout=${RELEASE_STAGE_TIMEOUT_SECONDS:-600}; [[ "$timeout" =~ ^[1-9][0-9]*$ && "$timeout" -le 600 ]] || fail 'RELEASE_STAGE_TIMEOUT_SECONDS is invalid'
perl -e 'alarm shift @ARGV; exec @ARGV' "$timeout" "$docker_bin" buildx build --platform linux/amd64 --provenance=false --sbom=false --push --file "$worktree/infra/Dockerfile.relay-ops" --label com.xingqiao.relay-ops.qualified=true --label "com.xingqiao.relay-ops.source.commit=$source_commit" --label "com.xingqiao.relay-ops.source.tree=$source_tree" --label "com.xingqiao.relay-ops.tested.tree=$source_tree" --label "com.xingqiao.relay-ops.migrations.sha256=$migrations_hash" --tag "$tag" "$build_context" >/dev/null || fail 'image build or push failed'
digest=$(
  perl -e 'alarm shift @ARGV; exec @ARGV' "$timeout" "$docker_bin" buildx imagetools inspect --format '{{.Manifest.Digest}}' "$tag" 2>/dev/null | tr -d '[:space:]'
) || fail 'digest resolution failed'
[[ "$digest" =~ ^sha256:[a-f0-9]{64}$ ]] || fail 'published image did not resolve to an immutable sha256 digest'
deadline=$(( $(date -u +%s) + timeout )); host_output=$(
  perl -e 'alarm shift @ARGV; exec @ARGV' "$timeout" "$ssh_bin" -T -i "$ssh_key" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o "UserKnownHostsFile=$ssh_known_hosts" -p "$ssh_port" "$ssh_target" bash "$host_executor" --mode production --image "$image_repository@$digest" --source-commit "$source_commit" --source-tree "$source_tree" --tested-tree "$source_tree" --migrations-hash "$migrations_hash" --deadline-epoch "$deadline" 2>/dev/null
) || { status=$?; printf '%s\n' "$host_output"; exit "$status"; }
ruby -rjson -e 'v=JSON.parse(STDIN.read); abort unless v.is_a?(Hash)&&v["result"]=="succeeded"' <<<"$host_output" || fail 'host executor did not report success'
printf '%s\n' "$host_output"
