#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'sub2api_candidate_publish status=failed\n' >&2
  exit 1
}

archive=
metadata=
report=
bundle=
remote=
output=
while (($# > 0)); do
  case "$1" in
    --archive) archive=${2:-}; shift 2 ;;
    --metadata) metadata=${2:-}; shift 2 ;;
    --report) report=${2:-}; shift 2 ;;
    --bundle) bundle=${2:-}; shift 2 ;;
    --remote) remote=${2:-}; shift 2 ;;
    --output) output=${2:-}; shift 2 ;;
    *) fail ;;
  esac
done

for path in "$archive" "$metadata" "$report" "$bundle" "$output"; do
  [[ "$path" = /* ]] || fail
done
[[ -f "$archive" && -f "$metadata" && -f "$report" && -f "$bundle" ]] || fail
[[ -n "$remote" && ! -e "$output" ]] || fail
command -v docker >/dev/null 2>&1 || fail
command -v git >/dev/null 2>&1 || fail
command -v ruby >/dev/null 2>&1 || fail

json_field() {
  ruby -rjson -e '
    value = JSON.parse(File.binread(ARGV.fetch(0))).fetch(ARGV.fetch(1))
    abort unless value.is_a?(String)
    print value
  ' "$1" "$2"
}

version=$(json_field "$metadata" version)
official_commit=$(json_field "$metadata" official_commit)
base_sha=$(json_field "$metadata" base_sha)
candidate_commit=$(json_field "$report" candidate_commit)
report_version=$(json_field "$report" version)
report_official=$(json_field "$report" official_commit)
report_base=$(json_field "$report" base_sha)

sha_pattern='^[0-9a-f]{40}$'
version_pattern='^[0-9]+([.][0-9]+){1,2}$'
[[ "$version" =~ $version_pattern ]] || fail
[[ "$official_commit" =~ $sha_pattern && "$base_sha" =~ $sha_pattern ]] || fail
[[ "$candidate_commit" =~ $sha_pattern ]] || fail
[[ "$report_version" == "$version" && "$report_official" == "$official_commit" ]] || fail
[[ "$report_base" == "$base_sha" ]] || fail

repository=ghcr.io/leesssong/xingqiao-sub2api
local_reference=xingqiao-sub2api:upstream-"$version"
target="$repository:upstream-$version"
audit_branch=automation/sub2api-upstream-"$version"

temporary=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-candidate-publish.XXXXXX")
cleanup() {
  rm -rf -- "$temporary"
}
trap cleanup EXIT
chmod 0700 "$temporary"

validate_image() {
  local image_json=$1
  ruby -rjson -e '
    image = JSON.parse(File.binread(ARGV.fetch(0)))
    expected = {
      "com.xingqiao.sub2api.qualified" => "true",
      "com.xingqiao.sub2api.upstream.version" => ARGV.fetch(2),
      "com.xingqiao.sub2api.upstream.commit" => ARGV.fetch(3),
      "com.xingqiao.sub2api.source.commit" => ARGV.fetch(4)
    }
    abort unless image.fetch("Id").match?(/\Asha256:[0-9a-f]{64}\z/)
    abort unless image.fetch("Os") == "linux" && image.fetch("Architecture") == "amd64"
    abort unless image.dig("Config", "Labels")&.slice(*expected.keys) == expected
    print image.fetch("Id")
  ' "$image_json" "$repository" "$version" "$official_commit" "$candidate_commit"
}

inspect_image() {
  local reference=$1
  local destination=$2
  umask 077
  docker image inspect --format '{{json .}}' "$reference" >"$destination"
  chmod 0600 "$destination"
}

docker load --input "$archive" >/dev/null
local_json="$temporary/local.json"
inspect_image "$local_reference" "$local_json"
local_image_id=$(validate_image "$local_json")

target_json="$temporary/target.json"
manifest_error="$temporary/manifest-error"
if docker manifest inspect "$target" >/dev/null 2>"$manifest_error"; then
  docker pull --platform linux/amd64 "$target" >/dev/null
  inspect_image "$target" "$target_json"
  target_image_id=$(validate_image "$target_json")
  [[ "$target_image_id" == "$local_image_id" ]] || fail
else
  grep -Eiq 'manifest unknown|no such manifest' "$manifest_error" || fail
  docker tag "$local_reference" "$target"
  docker push "$target" >/dev/null
  inspect_image "$target" "$target_json"
  target_image_id=$(validate_image "$target_json")
  [[ "$target_image_id" == "$local_image_id" ]] || fail
fi

digest=$(
  ruby -rjson -e '
    image = JSON.parse(File.binread(ARGV.fetch(0)))
    prefix = ARGV.fetch(1) + "@sha256:"
    digest = Array(image.fetch("RepoDigests")).find { |item| item.start_with?(prefix) }
    abort unless digest&.match?(/\Aghcr[.]io\/leesssong\/xingqiao-sub2api@sha256:[0-9a-f]{64}\z/)
    print digest
  ' "$target_json" "$repository"
)

git_dir="$temporary/git"
git init -q "$git_dir"
git -C "$git_dir" fetch -q "$bundle" candidate-artifact:candidate-artifact
[[ "$(git -C "$git_dir" rev-parse candidate-artifact)" == "$candidate_commit" ]] || fail
[[ "$(git -C "$git_dir" rev-parse "${candidate_commit}^")" == "$base_sha" ]] || fail
existing_audit=$(git ls-remote "$remote" "refs/heads/$audit_branch" | awk 'NR == 1 { print $1 }')
[[ -z "$existing_audit" || "$existing_audit" == "$candidate_commit" ]] || fail
GIT_ALTERNATE_OBJECT_DIRECTORIES="$git_dir/.git/objects" \
  git push -q "$remote" "$candidate_commit:refs/heads/$audit_branch"
[[ "$(git ls-remote "$remote" "refs/heads/$audit_branch" | awk 'NR == 1 { print $1 }')" == "$candidate_commit" ]] || fail

umask 077
ruby -rjson -rtempfile -e '
  output = File.expand_path(ARGV.fetch(0))
  directory = File.dirname(output)
  abort unless File.directory?(directory)
  value = {
    "version" => ARGV.fetch(1),
    "reference" => ARGV.fetch(2),
    "image_id" => ARGV.fetch(3),
    "source_commit" => ARGV.fetch(4),
    "audit_branch" => ARGV.fetch(5)
  }
  Tempfile.create([".sub2api-publish-", ".json"], directory) do |file|
    file.chmod(0o600)
    file.write(JSON.pretty_generate(value) + "\n")
    file.flush
    file.fsync
    File.rename(file.path, output)
  end
' "$output" "$version" "$digest" "$local_image_id" "$candidate_commit" "$audit_branch"
chmod 0600 "$output"
printf 'sub2api_candidate_publish status=succeeded version=%s\n' "$version"
