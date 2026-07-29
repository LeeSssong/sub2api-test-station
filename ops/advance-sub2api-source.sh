#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'sub2api_source_advance status=failed\n' >&2
  exit 1
}

bundle=
base_sha=
candidate_sha=
remote=
output=
while (($# > 0)); do
  case "$1" in
    --bundle) bundle=${2:-}; shift 2 ;;
    --base-sha) base_sha=${2:-}; shift 2 ;;
    --candidate-sha) candidate_sha=${2:-}; shift 2 ;;
    --remote) remote=${2:-}; shift 2 ;;
    --output) output=${2:-}; shift 2 ;;
    *) fail ;;
  esac
done

[[ "$bundle" = /* && "$output" = /* && -f "$bundle" && ! -e "$output" ]] || fail
[[ -n "$remote" ]] || fail
sha_pattern='^[0-9a-f]{40}$'
[[ "$base_sha" =~ $sha_pattern && "$candidate_sha" =~ $sha_pattern ]] || fail
command -v git >/dev/null 2>&1 || fail
command -v ruby >/dev/null 2>&1 || fail

remote_main() {
  git ls-remote "$remote" refs/heads/main | awk 'NR == 1 { print $1 }'
}

initial_main=$(remote_main)
[[ "$initial_main" == "$base_sha" || "$initial_main" == "$candidate_sha" ]] || fail

temporary=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-source-advance.XXXXXX")
cleanup() {
  rm -rf -- "$temporary"
}
trap cleanup EXIT
chmod 0700 "$temporary"

git init -q "$temporary/git"
git -C "$temporary/git" fetch -q "$bundle" candidate-artifact:candidate-artifact
[[ "$(git -C "$temporary/git" rev-parse candidate-artifact)" == "$candidate_sha" ]] || fail
[[ "$(git -C "$temporary/git" rev-parse "${candidate_sha}^")" == "$base_sha" ]] || fail
git -C "$temporary/git" merge-base --is-ancestor "$base_sha" "$candidate_sha" || fail

current_main=$(remote_main)
[[ "$current_main" == "$base_sha" || "$current_main" == "$candidate_sha" ]] || fail
if [[ "$current_main" == "$base_sha" ]]; then
  GIT_ALTERNATE_OBJECT_DIRECTORIES="$temporary/git/.git/objects" \
    git push -q "$remote" "$candidate_sha:refs/heads/main"
fi
[[ "$(remote_main)" == "$candidate_sha" ]] || fail

umask 077
ruby -rjson -rtempfile -e '
  output = File.expand_path(ARGV.fetch(0))
  directory = File.dirname(output)
  abort unless File.directory?(directory)
  value = {
    "previous_main" => ARGV.fetch(1),
    "current_main" => ARGV.fetch(2)
  }
  Tempfile.create([".sub2api-advance-", ".json"], directory) do |file|
    file.chmod(0o600)
    file.write(JSON.pretty_generate(value) + "\n")
    file.flush
    file.fsync
    File.rename(file.path, output)
  end
' "$output" "$initial_main" "$candidate_sha"
chmod 0600 "$output"
printf 'sub2api_source_advance status=succeeded current_main=%s\n' "$candidate_sha"
