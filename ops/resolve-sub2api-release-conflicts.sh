#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'sub2api_release_resolution status=failed reason=%s\n' "${1:-invalid_request}" >&2
  exit 1
}

repository=
base_version=
base_commit=
target_version=
target_tag=
target_tag_object=
target_commit=
records_root="$(cd "$(dirname "$0")" && pwd)/sub2api-release-resolutions"

while (($# > 0)); do
  case "$1" in
    --repository) repository=${2:-}; shift 2 ;;
    --base-version) base_version=${2:-}; shift 2 ;;
    --base-commit) base_commit=${2:-}; shift 2 ;;
    --target-version) target_version=${2:-}; shift 2 ;;
    --target-tag) target_tag=${2:-}; shift 2 ;;
    --target-tag-object) target_tag_object=${2:-}; shift 2 ;;
    --target-commit) target_commit=${2:-}; shift 2 ;;
    --records-root) records_root=${2:-}; shift 2 ;;
    *) fail invalid_request ;;
  esac
done

sha_pattern='^[0-9a-f]{40}$'
version_pattern='^[0-9]+([.][0-9]+){1,2}$'
[[ "$repository" = /* && -d "$repository/.git" ]] || fail invalid_repository
[[ "$records_root" = /* ]] || fail invalid_request
[[ "$base_version" =~ $version_pattern && "$target_version" =~ $version_pattern ]] || fail invalid_request
[[ "$base_commit" =~ $sha_pattern && "$target_tag_object" =~ $sha_pattern && "$target_commit" =~ $sha_pattern ]] || fail invalid_request
[[ "$target_tag" == "v$target_version" ]] || fail target_identity_mismatch

record="$records_root/${base_version}-to-${target_version}"
manifest="$record/manifest.json"
[[ -f "$manifest" ]] || fail record_missing

temporary=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-release-resolution.XXXXXX")
cleanup() {
  rm -rf -- "$temporary"
}
trap cleanup EXIT
normalized="$temporary/manifest.tsv"

if ! ruby -rjson -e '
  manifest = JSON.parse(File.binread(ARGV.fetch(0)))
  scalar_keys = %w[base_version base_commit target_version target_tag target_tag_object target_commit]
  scalar_keys.each do |key|
    value = manifest.fetch(key)
    abort unless value.is_a?(String) && !value.empty? && !value.match?(/[\t\r\n]/)
    puts ["identity", key, value].join("\t")
  end
  generated = manifest.fetch("generated_paths")
  abort unless generated.is_a?(Array) && generated.uniq.length == generated.length
  generated.each do |path|
    abort unless path.is_a?(String) && path.match?(/\A(?!\/)(?!.*(?:\A|\/)\.\.(?:\/|\z))[^\t\r\n]+\z/)
    puts ["generated", path].join("\t")
  end
  resolution_patch = manifest.fetch("resolution_patch")
  resolution_patch_blob = manifest.fetch("resolution_patch_blob")
  abort unless resolution_patch.is_a?(String) && resolution_patch.match?(/\A(?!\/)(?!.*(?:\A|\/)\.\.(?:\/|\z))[^\t\r\n]+\z/)
  abort unless resolution_patch_blob.is_a?(String) && resolution_patch_blob.match?(/\A[0-9a-f]{40}\z/)
  puts ["resolution", resolution_patch, resolution_patch_blob].join("\t")
  conflicts = manifest.fetch("conflicts")
  abort unless conflicts.is_a?(Hash) && !conflicts.empty?
  conflicts.keys.sort.each do |path|
    entry = conflicts.fetch(path)
    abort unless path.is_a?(String) && path.match?(/\A(?!\/)(?!.*(?:\A|\/)\.\.(?:\/|\z))[^\t\r\n]+\z/)
    abort unless entry.is_a?(Hash)
    stages = entry.fetch("stages")
    abort unless stages.is_a?(Hash) && stages.keys.sort == %w[1 2 3]
    blobs = %w[1 2 3].map { |stage| stages.fetch(stage) }
    abort unless blobs.all? { |blob| blob.is_a?(String) && blob.match?(/\A[0-9a-f]{40}\z/) }
    generated_entry = generated.include?(path)
    abort unless entry.keys.sort == ["stages"]
    puts ["conflict", path, *blobs, generated_entry ? "generated" : "semantic"].join("\t")
  end
  abort unless generated.sort == ["backend/cmd/server/wire_gen.go", "backend/go.sum"].sort
  abort unless (generated - conflicts.keys).empty?
' "$manifest" >"$normalized"; then
  fail record_invalid
fi

manifest_value() {
  local key=$1
  awk -F '\t' -v key="$key" '$1 == "identity" && $2 == key { print $3 }' "$normalized"
}

[[ "$(manifest_value base_version)" == "$base_version" && "$(manifest_value base_commit)" == "$base_commit" ]] || fail base_identity_mismatch
[[ "$(manifest_value target_version)" == "$target_version" &&
  "$(manifest_value target_tag)" == "$target_tag" &&
  "$(manifest_value target_tag_object)" == "$target_tag_object" &&
  "$(manifest_value target_commit)" == "$target_commit" ]] || fail target_identity_mismatch

expected_conflicts=$(awk -F '\t' '$1 == "conflict" { print $2 }' "$normalized" | LC_ALL=C sort)
actual_conflicts=$(git -C "$repository" diff --name-only --diff-filter=U | LC_ALL=C sort)
[[ -n "$actual_conflicts" && "$actual_conflicts" == "$expected_conflicts" ]] || fail conflict_set_mismatch

while IFS=$'\t' read -r kind conflict_file stage1 stage2 stage3 _resolution_kind; do
  [[ "$kind" == conflict ]] || continue
  actual_stage1=$(git -C "$repository" ls-files -u -- "$conflict_file" | awk '$3 == 1 { print $2 }')
  actual_stage2=$(git -C "$repository" ls-files -u -- "$conflict_file" | awk '$3 == 2 { print $2 }')
  actual_stage3=$(git -C "$repository" ls-files -u -- "$conflict_file" | awk '$3 == 3 { print $2 }')
  [[ "$actual_stage1" == "$stage1" && "$actual_stage2" == "$stage2" && "$actual_stage3" == "$stage3" ]] || fail preimage_mismatch
done <"$normalized"

resolution_patch=$(awk -F '\t' '$1 == "resolution" { print $2 }' "$normalized")
resolution_patch_blob=$(awk -F '\t' '$1 == "resolution" { print $3 }' "$normalized")
resolution_patch_path="$record/$resolution_patch"
[[ -f "$resolution_patch_path" ]] || fail record_invalid
[[ "$(git hash-object "$resolution_patch_path")" == "$resolution_patch_blob" ]] || fail record_invalid
semantic_conflicts=()
while IFS=$'\t' read -r kind conflict_file _stage1 _stage2 _stage3 resolution_kind; do
  [[ "$kind" == conflict && "$resolution_kind" == semantic ]] || continue
  semantic_conflicts+=("$conflict_file")
done <"$normalized"
[[ "${#semantic_conflicts[@]}" -gt 0 ]] || fail record_invalid
git -C "$repository" checkout --conflict=merge -- "${semantic_conflicts[@]}"
git -C "$repository" apply --check -- "$resolution_patch_path" || fail postimage_mismatch
git -C "$repository" apply -- "$resolution_patch_path"

while IFS=$'\t' read -r kind conflict_file _stage1 _stage2 _stage3 resolution_kind; do
  [[ "$kind" == conflict && "$resolution_kind" == semantic ]] || continue
  git -C "$repository" add -- "$conflict_file"
done <"$normalized"

git -C "$repository" checkout --theirs -- backend/cmd/server/wire_gen.go backend/go.sum
rm -f -- "$repository/backend/go.sum"
GOFLAGS=-mod=mod go -C "$repository/backend" mod tidy || fail generation_failed
GOFLAGS=-mod=mod go -C "$repository/backend" generate ./cmd/server || fail generation_failed
[[ -f "$repository/backend/go.sum" && -f "$repository/backend/cmd/server/wire_gen.go" ]] || fail generation_failed

unexpected_generated=$(git -C "$repository" diff --name-only | grep -Ev '^(backend/cmd/server/wire_gen[.]go|backend/go[.]sum)$' || true)
[[ -z "$unexpected_generated" ]] || fail generation_scope_mismatch
git -C "$repository" add -- backend/cmd/server/wire_gen.go backend/go.sum

conflict_files=()
while IFS= read -r conflict_file; do
  conflict_files+=("$conflict_file")
done <<<"$expected_conflicts"
if git -C "$repository" grep -n -E '^(<<<<<<< |=======|>>>>>>> )' -- "${conflict_files[@]}" >/dev/null; then
  fail conflict_markers_present
fi
[[ -z "$(git -C "$repository" diff --name-only --diff-filter=U)" ]] || fail unmerged_entries_present
git -C "$repository" diff --quiet || fail unstaged_changes_present

printf 'sub2api_release_resolution status=succeeded base_version=%s target_version=%s\n' "$base_version" "$target_version"
