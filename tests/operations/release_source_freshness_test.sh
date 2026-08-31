#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
CHECKER="$ROOT/ops/assert-sub2api-release-source.sh"
FIXTURE=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-source-freshness.XXXXXX")
trap 'rm -rf -- "$FIXTURE"' EXIT

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

mkdir -p "$FIXTURE/remote.git"
git -C "$FIXTURE/remote.git" init -q --bare
git -C "$FIXTURE" clone -q "$FIXTURE/remote.git" repo
git -C "$FIXTURE/repo" config user.name Test
git -C "$FIXTURE/repo" config user.email test@example.invalid
printf 'initial\n' >"$FIXTURE/repo/file.txt"
git -C "$FIXTURE/repo" add file.txt
git -C "$FIXTURE/repo" commit -qm initial
git -C "$FIXTURE/repo" branch -M main
git -C "$FIXTURE/repo" push -q -u origin main

for mode in rehearsal production; do
  "$CHECKER" --mode "$mode" --worktree "$FIXTURE/repo" >/dev/null
done

git -C "$FIXTURE/repo" switch -q -c candidate
printf 'candidate\n' >>"$FIXTURE/repo/file.txt"
git -C "$FIXTURE/repo" add file.txt
git -C "$FIXTURE/repo" commit -qm candidate
for mode in rehearsal production; do
  if "$CHECKER" --mode "$mode" --worktree "$FIXTURE/repo" >/dev/null 2>&1; then
    fail "candidate branch was accepted for $mode release"
  fi
done

printf 'PASS: release source freshness\n'
