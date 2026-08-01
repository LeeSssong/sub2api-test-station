#!/usr/bin/env bash
set -euo pipefail

umask 077

fail() {
  printf 'sub2api_test_evidence status=failed: %s\n' "$1" >&2
  exit 1
}

output=''
commands=()
while (($#)); do
  case "$1" in
    --output)
      (($# >= 2)) || fail '--output requires a value'
      [[ -z "$output" ]] || fail '--output may be supplied once'
      output=$2
      shift 2
      ;;
    --command)
      (($# >= 2)) || fail '--command requires a value'
      [[ -n "$2" && "$2" != *$'\n'* && "$2" != *$'\r'* ]] || fail '--command must be one completed single-line command'
      commands+=("$2")
      shift 2
      ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ "$output" == /* ]] || fail '--output must be an absolute path'
(( ${#commands[@]} > 0 )) || fail 'at least one completed --command is required'
[[ ! -L "$output" ]] || fail '--output must not be a symlink'

output_parent=$(dirname "$output")
[[ -d "$output_parent" && ! -L "$output_parent" ]] || fail '--output parent must be a non-symlink directory'
output_parent_physical=$(cd "$output_parent" && pwd -P)
[[ "$output_parent_physical/$(basename "$output")" == "$output" ]] || fail '--output path must be canonical'

worktree=$(pwd -P)
[[ -d "$worktree/.git" || -f "$worktree/.git" ]] || fail 'must run from a Git worktree'
[[ -z "$(git -C "$worktree" status --porcelain)" ]] || fail 'worktree must be clean before writing evidence'
source_commit=$(git -C "$worktree" rev-parse HEAD) || fail 'could not resolve source commit'
tested_tree=$(git -C "$worktree" rev-parse 'HEAD^{tree}') || fail 'could not resolve tested tree'
[[ "$source_commit" =~ ^[a-f0-9]{40}$ && "$tested_tree" =~ ^[a-f0-9]{40}$ ]] || fail 'Git identity is invalid'

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

temporary=$(mktemp "$output_parent/.sub2api-test-evidence.XXXXXX")
cleanup() { rm -f -- "$temporary"; }
trap cleanup EXIT
chmod 0600 "$temporary"

ruby -rjson -rtime -e '
  output, source_commit, tested_tree, migrations_hash, *commands = ARGV
  value = {
    "schema_version" => 1,
    "source_commit" => source_commit,
    "tested_tree" => tested_tree,
    "migrations_hash" => migrations_hash,
    "created_at" => Time.now.utc.strftime("%Y-%m-%dT%H:%M:%SZ"),
    "commands" => commands,
    "result" => "passed"
  }
  File.binwrite(output, JSON.generate(value) + "\n")
' "$temporary" "$source_commit" "$tested_tree" "$migrations_hash" "${commands[@]}" || fail 'could not write evidence JSON'
chmod 0600 "$temporary"
mv -f "$temporary" "$output"
trap - EXIT
printf 'sub2api_test_evidence status=succeeded output=%s\n' "$output"
