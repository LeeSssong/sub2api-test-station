#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'sub2api_merge status=failed\n' >&2
  exit 1
}

root=
metadata=
official_repository=
bundle=
report=
while (($# > 0)); do
  case "$1" in
    --root) root=${2:-}; shift 2 ;;
    --metadata) metadata=${2:-}; shift 2 ;;
    --official-repository) official_repository=${2:-}; shift 2 ;;
    --bundle) bundle=${2:-}; shift 2 ;;
    --report) report=${2:-}; shift 2 ;;
    *) fail ;;
  esac
done

for path in "$root" "$metadata" "$bundle" "$report"; do
  [[ "$path" = /* ]] || fail
done
[[ -d "$root/.git" && -f "$metadata" && -n "$official_repository" ]] || fail

metadata_field() {
  local field=$1
  ruby -rjson -e '
    value = JSON.parse(File.binread(ARGV.fetch(0))).fetch(ARGV.fetch(1))
    abort unless value.is_a?(String) || value == true || value == false
    print value
  ' "$metadata" "$field"
}

base_sha=$(metadata_field base_sha)
base_version=$(metadata_field base_version)
base_commit=$(metadata_field base_commit)
target_commit=$(metadata_field official_commit)
target_version=$(metadata_field version)
target_tag=$(metadata_field tag)
published_at=$(metadata_field published_at)
has_update=$(metadata_field has_update)

sha_pattern='^[0-9a-f]{40}$'
version_pattern='^[0-9]+([.][0-9]+){1,2}$'
[[ "$base_sha" =~ $sha_pattern && "$base_commit" =~ $sha_pattern ]] || fail
[[ "$base_version" =~ $version_pattern && "$target_commit" =~ $sha_pattern && "$target_version" =~ $version_pattern ]] || fail
[[ "$target_tag" == "v$target_version" || "$target_tag" == "$target_version" ]] || fail
[[ "$published_at" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T ]] || fail
[[ "$has_update" == true ]] || fail

[[ "$(git -C "$root" rev-parse HEAD)" == "$base_sha" ]] || fail
[[ -z "$(git -C "$root" status --porcelain=v1)" ]] || fail
[[ -d "$root/upstream/sub2api" ]] || fail

temporary=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-upstream-merge.XXXXXX")
root_modified=0
completed=0
cleanup() {
  if [[ "$completed" != 1 ]]; then
    rm -f -- "$bundle" "$report"
    if [[ "$root_modified" == 1 ]]; then
      git -C "$root" reset --hard "$base_sha" >/dev/null 2>&1 || true
      git -C "$root" clean -fd -- upstream/sub2api >/dev/null 2>&1 || true
    fi
  fi
  rm -rf -- "$temporary"
}
trap cleanup EXIT

official="$temporary/official"
git clone -q --no-checkout "$official_repository" "$official"
git -C "$official" checkout -q "$base_commit"
[[ "$(git -C "$official" rev-parse HEAD)" == "$base_commit" ]] || fail
[[ "$(git -C "$official" rev-parse "${target_tag}^{commit}")" == "$target_commit" ]] || fail
tag_object=$(git -C "$official" rev-parse "$target_tag")
[[ "$tag_object" =~ $sha_pattern ]] || fail

rsync -a --delete --exclude=.git/ "$root/upstream/sub2api/" "$official/"
git -C "$official" add -A
GIT_AUTHOR_NAME='Xingqiao Release Automation' \
GIT_AUTHOR_EMAIL='release-automation@xingqialab.invalid' \
GIT_COMMITTER_NAME='Xingqiao Release Automation' \
GIT_COMMITTER_EMAIL='release-automation@xingqialab.invalid' \
GIT_AUTHOR_DATE="$published_at" GIT_COMMITTER_DATE="$published_at" \
  git -C "$official" commit -q --allow-empty -m 'chore: overlay Xingqiao customizations'

if ! GIT_AUTHOR_NAME='Xingqiao Release Automation' \
  GIT_AUTHOR_EMAIL='release-automation@xingqialab.invalid' \
  GIT_COMMITTER_NAME='Xingqiao Release Automation' \
  GIT_COMMITTER_EMAIL='release-automation@xingqialab.invalid' \
  GIT_AUTHOR_DATE="$published_at" GIT_COMMITTER_DATE="$published_at" \
    git -C "$official" merge -q --no-ff --no-edit "$target_commit"; then
  conflicts=$(git -C "$official" diff --name-only --diff-filter=U)
  if [[ "$target_version" == "0.1.171" ]]; then
    "$(dirname "$0")/resolve-sub2api-release-conflicts.sh" \
      --repository "$official" \
      --base-version "$base_version" \
      --base-commit "$base_commit" \
      --target-version "$target_version" \
      --target-tag "$target_tag" \
      --target-tag-object "$tag_object" \
      --target-commit "$target_commit"
  else
    [[ "$conflicts" == 'backend/cmd/server/wire_gen.go' ]] || fail

    git -C "$official" checkout --theirs -- backend/cmd/server/wire_gen.go
    GOFLAGS=-mod=mod go -C "$official/backend" generate ./cmd/server
    generated_paths=$(
      {
        git -C "$official" diff --name-only
        git -C "$official" ls-files --others --exclude-standard
      } | sort -u
    )
    printf 'sub2api_merge generated_paths=%q\n' "$generated_paths" >&2
    [[ "$generated_paths" == 'backend/cmd/server/wire_gen.go' ||
      "$generated_paths" == $'backend/cmd/server/wire_gen.go\nbackend/go.sum' ]] || fail
    git -C "$official" add -- backend/cmd/server/wire_gen.go
    if [[ -f "$official/backend/go.sum" ]]; then
      git -C "$official" add -- backend/go.sum
    fi
    [[ -z "$(git -C "$official" diff --name-only --diff-filter=U)" ]] || fail
  fi

  GIT_AUTHOR_NAME='Xingqiao Release Automation' \
  GIT_AUTHOR_EMAIL='release-automation@xingqialab.invalid' \
  GIT_COMMITTER_NAME='Xingqiao Release Automation' \
  GIT_COMMITTER_EMAIL='release-automation@xingqialab.invalid' \
  GIT_AUTHOR_DATE="$published_at" GIT_COMMITTER_DATE="$published_at" \
    git -C "$official" commit -q --no-edit
fi

[[ -z "$(git -C "$official" status --porcelain=v1)" ]] || fail

root_modified=1
rsync -a --delete --exclude=.git/ "$official/" "$root/upstream/sub2api/"
"$(dirname "$0")/sub2api-release-metadata.rb" advance-provenance \
  --metadata "$metadata" \
  --provenance "$root/upstream/sub2api/XINGQIAO_UPSTREAM.md" \
  --imported "${published_at%%T*}" \
  --annotated-tag "$tag_object"

git -C "$root" add -A -- upstream/sub2api
GIT_AUTHOR_NAME='Xingqiao Release Automation' \
GIT_AUTHOR_EMAIL='release-automation@xingqialab.invalid' \
GIT_COMMITTER_NAME='Xingqiao Release Automation' \
GIT_COMMITTER_EMAIL='release-automation@xingqialab.invalid' \
GIT_AUTHOR_DATE="$published_at" GIT_COMMITTER_DATE="$published_at" \
  git -C "$root" commit -q --allow-empty -m "feat: qualify Xingqiao upstream $target_version"

candidate_commit=$(git -C "$root" rev-parse HEAD)
[[ "$(git -C "$root" rev-parse HEAD^)" == "$base_sha" ]] || fail
git -C "$root" branch -f candidate-artifact "$candidate_commit" >/dev/null
umask 077
git -C "$root" bundle create "$bundle" candidate-artifact
ruby -rjson -rshellwords -e '
  report = {
    "candidate_commit" => ARGV.fetch(1),
    "base_sha" => ARGV.fetch(2),
    "version" => ARGV.fetch(3),
    "official_commit" => ARGV.fetch(4),
    "changed_paths" => `git -C #{Shellwords.escape(ARGV.fetch(0))} diff-tree --no-commit-id --name-only -r #{ARGV.fetch(1)}`.lines.map(&:strip)
  }
  File.open(ARGV.fetch(5), File::WRONLY | File::CREAT | File::EXCL, 0o600) do |file|
    file.write(JSON.pretty_generate(report) + "\n")
  end
' "$root" "$candidate_commit" "$base_sha" "$target_version" "$target_commit" "$report"

completed=1
printf 'sub2api_merge status=succeeded candidate_commit=%s\n' "$candidate_commit"
