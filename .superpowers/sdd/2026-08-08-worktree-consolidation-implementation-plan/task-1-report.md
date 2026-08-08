# Task 1 report — worktree consolidation registration and recovery manifest

Date: 2026-08-08
Branch: `codex/worktree-consolidation-20260808`

## Status

DONE. Task 1 registered the operation as `进行中`, protected the main checkout used by the active threads `新建运营界面` and `优化账号卡片`, and created a reproducible inventory for every Git-registered worktree. No runtime code was merged.

## Artifacts

- `.superpowers/sdd/2026-08-08-worktree-consolidation/worktree-manifest.tsv`
  - 24 rows, matching all 24 entries returned by `git worktree list --porcelain`.
  - Main is classified `protected-main` and is explicitly excluded from recovery capture.
  - The missing/prunable monitor release worktree is recorded as `unavailable-prunable` with its recorded HEAD and Git reason.
  - The current consolidation worktree is recorded as `consolidation-target`, not as a merge source. Although it was dirty while Task 1 was being assembled, its operation artifacts are the outputs committed on this branch; it is therefore intentionally exempt from recursive recovery capture and is not treated as a dirty source.
- `.superpowers/sdd/2026-08-08-worktree-consolidation/recovery/`
  - `private-tmp-sub2api-release-njYlCa`
  - `codex-worktrees-sub2api-monitor-reliability`
  - `project-worktrees-account`

Each dirty source snapshot contains `status.porcelain`, unstaged `working-tree.diff`, staged `staged.diff`, `untracked-files.txt`, `untracked-files.tar`, and `metadata.tsv` with source HEAD/branch, capture time, and checksums. The consolidation target is the documented scope exception: its own committed operation artifacts are not recursively snapshotted. Source worktrees were read only.

## Verification

- Manifest schema and row count passed (`24`).
- Manifest paths exactly matched the current `git worktree list --porcelain` registry.
- All three recovery archives were listed/read back successfully; archived untracked-file inventories matched their source lists.
- Fresh source statuses matched the captured status files for all three dirty worktrees, confirming snapshotting did not modify them.

## Concerns

- `/private/tmp/sub2api-monitor-release.DzXKrU` is prunable because its gitdir target no longer exists; its working-tree contents cannot be recovered from the filesystem. Its recorded HEAD remains available for later Git-object inspection.
- The protected main checkout may contain uncommitted edits from the two active threads; it was intentionally not snapshotted or used as a recovery source.
