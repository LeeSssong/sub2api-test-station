#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'release_source status=failed: %s\n' "$1" >&2
  exit 1
}

mode=
worktree=
while (($#)); do
  case "$1" in
    --mode) (($# >= 2)) || fail '--mode requires a value'; mode=$2; shift 2 ;;
    --worktree) (($# >= 2)) || fail '--worktree requires a value'; worktree=$2; shift 2 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ "$mode" == rehearsal || "$mode" == production ]] || fail '--mode must be rehearsal or production'
[[ "$worktree" == /* && -d "$worktree" && ! -L "$worktree" ]] || fail '--worktree must be an absolute non-symlink directory'
worktree=$(cd "$worktree" && pwd -P)
[[ -d "$worktree/.git" || -f "$worktree/.git" ]] || fail '--worktree is not a Git worktree'

branch=$(git -C "$worktree" symbolic-ref --quiet --short HEAD 2>/dev/null || true)
[[ "$branch" == main ]] || fail 'releases must use the main branch'
remote_commit=$(git -C "$worktree" rev-parse --verify refs/remotes/origin/main 2>/dev/null) \
  || fail 'origin/main is unavailable; fetch the remote before release'
source_commit=$(git -C "$worktree" rev-parse HEAD) || fail 'could not resolve worktree HEAD'
source_tree=$(git -C "$worktree" rev-parse 'HEAD^{tree}') || fail 'could not resolve worktree tree'
remote_tree=$(git -C "$worktree" rev-parse 'refs/remotes/origin/main^{tree}') \
  || fail 'could not resolve origin/main tree'
[[ "$source_commit" == "$remote_commit" ]] || fail 'release source is not the pushed origin/main commit'
[[ "$source_tree" == "$remote_tree" ]] || fail 'release source tree does not match origin/main'

# Every real Sub2API release must prove that account concurrency remains
# provider-native. Lightweight release-source fixtures may omit the backend,
# so only invoke the guard when the production backend marker is present.
if [[ -f "$worktree/upstream/sub2api/backend/go.mod" ]]; then
  guard="$worktree/ops/assert-native-openai-concurrency-only.sh"
  [[ -x "$guard" && ! -L "$guard" ]] || fail 'native account concurrency guard is missing or not executable'
  "$guard" --worktree "$worktree" || fail 'native account concurrency guard failed'
fi

printf 'release_source status=passed branch=main commit=%s tree=%s\n' "$source_commit" "$source_tree"
