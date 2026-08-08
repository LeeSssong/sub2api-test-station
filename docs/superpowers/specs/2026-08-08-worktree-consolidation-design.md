# Non-main Worktree Consolidation Design

## Goal

Prevent feature loss caused by deploying from a stale `main` while active workspaces still contain newer or uncommitted changes. Consolidate every non-main worktree except the two active user tasks into one recoverable release candidate, then merge that candidate into `main` before deployment.

## Protected Workspaces

- The current `main` checkout is protected while the active threads “新建运营界面” and “优化账号卡片” continue. Both threads currently use `/Users/gongtengxinwen/Documents/sub2api搭建` and may have uncommitted changes.
- The consolidation task must not reset, clean, checkout, merge, or delete that checkout while either thread is active.
- The two active threads are not included in the consolidation candidate; their eventual commits are merged into `main` first, then the candidate is rebased/merged against the resulting `main`.

## Inventory and Recovery

Every registered non-main worktree is recorded with its path, branch or detached HEAD, commit, relationship to `main`, dirty status, and classification. For dirty worktrees, save both a binary Git diff and every untracked file into a recovery archive before applying any changes. A missing/prunable worktree is recorded as unresolved rather than guessed or silently deleted.

## Consolidation Rules

1. Work from a dedicated branch/worktree based on the newest committed `main` snapshot.
2. Worktree content already reachable from `main` is classified as stale and is not re-merged.
3. The latest production-relevant implementation is the baseline for overlapping runtime files. The 2026-08-08 `codex/fix-relay-admin-auth` line is currently the newest candidate because it contains the account-monitor probe-only and administrator cost-detail work.
4. Older unique capabilities, such as group-aware monitor scoring from `codex/account`, are retained only when they are not already present or superseded. Conflicts are resolved per file and per data flow; never use a repository-wide “theirs” strategy.
5. Documentation, test evidence, and generated release artifacts are preserved in the archive or merged when they describe the final source. They do not by themselves qualify a release as deployed.
6. No GitHub Actions release workflow is added or used. Release preparation, source advancement, publishing, staging, blue-green promotion, and cleanup remain in the reviewed local/host script chain.

## Release Gate

The candidate is not considered complete until all three facts are evidenced: pushed to the server, deployed, and verified online. Until then, the project ledger remains “进行中”. The final sequence is:

```text
active threads finish → merge their commits into main
→ merge consolidation candidate into main
→ focused regression/build/migration checks
→ push main → deploy via local/host blue-green chain
→ online verification → delete integrated worktrees
```

If a merge, build, deployment, or online check fails, retain the consolidation worktree and continue fixing there; do not overwrite `main` or delete recovery artifacts.

