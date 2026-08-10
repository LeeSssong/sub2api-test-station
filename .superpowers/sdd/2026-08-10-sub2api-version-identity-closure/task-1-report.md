# Task 1 — Candidate version identity contract

## Scope

- Worktree: `.worktrees/fix-release-version-identity`
- Branch: `codex/fix-release-version-identity`
- No production access, deployment, push, credential access, or protected-worktree mutation was performed.

## Root cause reproduced

Release metadata can target `0.1.173` while the merged candidate retains
`upstream/sub2api/backend/cmd/server/VERSION=0.1.172`. The upstream Dockerfile
uses that file when no `VERSION` build argument is supplied, so the stale
candidate source becomes the built binary identity.

The checked-in candidate was also stale at the start of this task:
`XINGQIAO_UPSTREAM.md` recorded `v0.1.173` while `VERSION` contained
`0.1.172`.

## TDD evidence

### RED

Added two real merge fixtures before production-code changes:

1. A `0.1.171 -> 0.1.173` candidate whose source VERSION is `0.1.172` must
   export `0.1.173` in both the checked-out candidate source and bundle commit.
2. A stale candidate VERSION that cannot be safely materialized (a symlink)
   must fail before bundle/report publication and restore the release root.

Command:

```sh
ruby tests/operations/merge_sub2api_release_test.rb --name '/test_(materializes_target_version_in_candidate_source_before_export|fails_closed_when_stale_candidate_version_cannot_be_materialized)/'
```

Observed before the fix: 2 runs, 3 assertions, 2 failures. The first fixture
exported `0.1.172`; the second incorrectly printed
`sub2api_merge status=succeeded`.

Added a checked-in candidate acceptance assertion before changing the live
VERSION file:

```sh
ruby tests/operations/sub2api_release_process_test.rb --name test_current_candidate_source_version_matches_recorded_release_tag
```

Observed before that file update: 1 run, 2 assertions, 1 failure; the
provenance-derived expected value was `0.1.173` and the candidate source value
was `0.1.172`.

### GREEN

The same focused command passed after the implementation: 2 runs, 9
assertions, 0 failures/errors/skips (8 explicit assertions plus one fixture
invariant assertion).

The current-candidate acceptance assertion also passed after updating
`upstream/sub2api/backend/cmd/server/VERSION` to `0.1.173`: 1 run, 2
assertions, 0 failures/errors/skips.

Independent review identified a forced-rebuild compatibility hole. Added an
execution fixture for `has_update=true` with the same official commit and the
VERSION already set to the target version. Before correction, it failed with
`nothing to commit, working tree clean`; after allowing the intentional
materialization audit commit to be empty, it passed: 1 run, 4 assertions, 0
failures/errors/skips.

## Implementation

`ops/merge-sub2api-release.sh` now:

- requires the official target commit to contain a blob at
  `backend/cmd/server/VERSION`;
- rejects a missing or symlinked candidate VERSION path;
- writes the validated metadata target version into that regular file and
  commits the local materialization before copying it to the candidate root;
- re-reads the candidate commit's VERSION before creating the candidate-artifact
  branch, bundle, or publication report.

The current checked-in candidate VERSION is now `0.1.173`, matching its
recorded `v0.1.173` release tag.

The existing provenance, recorded conflict-resolution, migration, candidate
tree, and publication contracts were not relaxed.

## Fresh verification

```text
ruby tests/operations/merge_sub2api_release_test.rb
31 runs, 207 assertions, 0 failures, 0 errors, 0 skips

ruby tests/operations/sub2api_release_metadata_test.rb
6 runs, 39 assertions, 0 failures, 0 errors, 0 skips

ruby tests/operations/sub2api_release_process_test.rb
3 runs, 8 assertions, 0 failures, 0 errors, 0 skips

ruby tests/operations/publish_sub2api_candidate_test.rb
5 runs, 30 assertions, 0 failures, 0 errors, 0 skips

bash -n ops/merge-sub2api-release.sh
bash -n ops/resolve-sub2api-release-conflicts.sh
git diff --check
```

Both Bash syntax checks and `git diff --check` exited 0.

## Remaining work / concerns

- This is only Task 1. It has not been merged into `main`, pushed, built,
  qualified against the real upstream, or deployed.
- Independent review found the forced-rebuild empty-commit case and required
  the follow-up regression and `--allow-empty` fix. Re-review of the final
  amended candidate found no Critical, Important, or Minor issues and marked
  Task 1 ready to merge. Production version identity verification belongs to
  Task 2 after reviewed integration.
