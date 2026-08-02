#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'relay_ops_test_evidence status=failed: %s\n' "$1" >&2; exit 1; }
output=''; migrations_dir=''; commands=()
while (($#)); do
  case "$1" in
    --output) (($# >= 2)) || fail '--output requires a value'; [[ -z "$output" ]] || fail '--output may be supplied once'; output=$2; shift 2 ;;
    --migrations-dir) (($# >= 2)) || fail '--migrations-dir may be supplied once'; [[ -z "$migrations_dir" ]] || fail '--migrations-dir may be supplied once'; migrations_dir=$2; shift 2 ;;
    --command) (($# >= 2)) || fail '--command requires a value'; [[ -n "$2" && "$2" != *$'\n'* && "$2" != *$'\r'* ]] || fail '--command must be one completed single-line command'; commands+=("$2"); shift 2 ;;
    *) fail "unknown argument: $1" ;;
  esac
done
[[ "$output" == /* && ! -L "$output" ]] || fail '--output must be an absolute non-symlink path'
[[ -n "$migrations_dir" ]] || migrations_dir="$(pwd -P)/relay-ops-service/internal/store/migrations"
[[ "$migrations_dir" == /* && -d "$migrations_dir" && ! -L "$migrations_dir" ]] || fail '--migrations-dir is invalid'
[[ ${#commands[@]} -gt 0 ]] || fail 'at least one completed --command is required'
parent=$(dirname "$output"); [[ -d "$parent" && ! -L "$parent" ]] || fail 'output parent is invalid'
canonical_parent=$(cd "$parent" && pwd -P); [[ "$canonical_parent/$(basename "$output")" == "$output" ]] || fail 'output path must be canonical'
worktree=$(pwd -P); [[ -d "$worktree/.git" || -f "$worktree/.git" ]] || fail 'must run from a Git worktree'
[[ -z "$(git -C "$worktree" status --porcelain)" ]] || fail 'worktree must be clean before writing evidence'
source_commit=$(git -C "$worktree" rev-parse HEAD) || fail 'could not resolve source commit'
tested_tree=$(git -C "$worktree" rev-parse 'HEAD^{tree}') || fail 'could not resolve tested tree'
migrations_hash=$(ruby -rdigest -e '
  dir = ARGV.fetch(0); ws=/[\u0009-\u000D\u0020\u0085\u00A0\u1680\u2000-\u200A\u2028\u2029\u202F\u205F\u3000]/
  files=Dir.children(dir).select { |n| n.end_with?(".sql") }.sort; d=Digest::SHA256.new
  files.each { |n| c=File.binread(File.join(dir,n)).force_encoding(Encoding::UTF_8); abort unless c.valid_encoding?; c=c.sub(/\A#{ws}+/,"").sub(/#{ws}+\z/,""); next if c.empty?; d << n << "\0" << Digest::SHA256.hexdigest(c) << "\n" }
  print d.hexdigest
' "$migrations_dir") || fail 'could not compute migrations hash'
[[ "$source_commit" =~ ^[a-f0-9]{40}$ && "$tested_tree" =~ ^[a-f0-9]{40}$ && "$migrations_hash" =~ ^[a-f0-9]{64}$ ]] || fail 'invalid Git or migration identity'
tmp=$(mktemp "$parent/.relay-ops-test-evidence.XXXXXX"); trap 'rm -f -- "$tmp"' EXIT; chmod 0600 "$tmp"
ruby -rjson -rtime -e 'o,commit,tree,migrations,*commands=ARGV; File.binwrite(o, JSON.generate({schema_version:1,source_commit:commit,tested_tree:tree,migrations_hash:migrations,created_at:Time.now.utc.strftime("%Y-%m-%dT%H:%M:%SZ"),commands:commands,result:"passed"})+"\n")' "$tmp" "$source_commit" "$tested_tree" "$migrations_hash" "${commands[@]}" || fail 'could not write evidence'
chmod 0600 "$tmp"; mv -f "$tmp" "$output"; trap - EXIT
printf 'relay_ops_test_evidence status=succeeded output=%s\n' "$output"
