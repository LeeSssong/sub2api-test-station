# Non-main Worktree Integration and Release Preparation Plan

> **For agentic workers:** Use the review-and-integrate workflow task-by-task; do not delete dirty or missing worktrees.

**Goal:** Review all registered non-`main` worktrees, integrate every clean completed candidate into `main`, push the resulting branch, and prepare a release candidate without mutating production.

**Architecture:** Treat each branch as an independent merge candidate. Clean documentation-only branches can be merged after scope review; code branches require focused tests and conflict review. Dirty worktrees are first captured as recoverable patches and are not merged until their uncommitted changes are explicitly committed and reviewed.

**Tech Stack:** Git worktrees, Go/Vue tests, Bash release contracts, local blue-green release scripts.

## Global Constraints

- Never use GitHub Actions for release preparation or deployment.
- Preserve all user changes, dirty worktrees, missing worktree metadata, and untracked release evidence.
- Merge candidates into `main` before pushing; do not deploy production in this task.
- Before any future production deployment, rerun release qualification and the minimum online health/rollback gate even if this preparation succeeds.

### Task 1: Register and classify all worktrees

**Files:** `docs/project/project-progress.md`, Git worktree metadata.

- [x] Capture path, branch, HEAD, ahead/behind counts, dirty state, and missing/prunable state for every worktree.
- [x] Classify candidates as clean docs-only, clean code, dirty, or missing/prunable.

### Task 2: Review clean candidates

**Candidates:** `codex/gpt-group-baseline-analysis`, `codex/resend-smtp-timeout`, `codex/usage-upstream-actual-cost`.

- [x] Inspect commit scope and branch reports.
- [x] Run the smallest relevant tests for code branches.
- [x] Merge only candidates with no unresolved conflicts or review blockers.

### Task 3: Preserve and review dirty candidate

**Candidate:** `codex/monitor-v2-current-config`.

- [x] Save a binary diff and untracked-file archive before any edits.
- [x] Review whether its uncommitted changes are complete and testable.
- [x] Commit only if the worktree has a coherent completed change; otherwise preserve it and leave it unmerged.

### Task 4: Integrate and push

- [x] Merge approved candidates into `main` in dependency order.
- [x] Run conflict checks, focused regression, build/type checks, and `git diff --check`.
- [x] Push `main` to `origin/main` and record the final release candidate commit/tree/evidence.

### Task 5: Prepare release handoff

- [x] Produce a release manifest listing included commits, excluded/preserved worktrees, migration hash, image repository, and production command parameters.
- [x] Stop without production writes and wait for the user's deployment instruction.
