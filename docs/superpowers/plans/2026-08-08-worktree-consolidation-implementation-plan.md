# Non-main Worktree Consolidation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recover, classify, consolidate, verify, and release all non-main worktree changes except the two active user threads without losing uncommitted work or allowing stale-source deployment.

**Architecture:** Keep the active `main` checkout protected. Use one dedicated consolidation worktree as the only write target for recovered non-main snapshots and semantic merges. After the active threads finish, merge their commits into `main`, merge the candidate, run focused release verification, then deploy through the existing local/host blue-green chain and clean only integrated worktrees.

**Tech Stack:** Git worktrees, shell recovery scripts, Markdown project ledgers, Sub2API Go/Vue tests and existing local/host release scripts.

## Global Constraints

- Do not modify, reset, clean, switch, merge, or delete the active `main` checkout while the two protected threads are running.
- Before any write, record the item in `docs/project/project-progress.md` with status “进行中”.
- Preserve every dirty diff and untracked file from non-main worktrees before applying or deleting anything.
- Treat only the two referenced active threads as protected exceptions; every other registered non-main worktree must be classified.
- Resolve overlapping runtime changes semantically, using the newest production-relevant implementation as the baseline and retaining older capabilities only when not superseded.
- Do not use GitHub Actions for release preparation or deployment.
- Do not mark the item “已完成” until it is pushed, deployed, and online-verified.

---

### Task 1: Register the operation and create the recovery manifest

**Files:**
- Modify: `docs/project/project-progress.md`
- Create: `.superpowers/sdd/2026-08-08-worktree-consolidation/progress.md`
- Create: `.superpowers/sdd/2026-08-08-worktree-consolidation/worktree-manifest.tsv`
- Create: `.superpowers/sdd/2026-08-08-worktree-consolidation/recovery/` snapshots as needed

**Interfaces:**
- Consumes: `git worktree list --porcelain`, each worktree’s HEAD, branch, status, and diff.
- Produces: a reproducible manifest and recovery archive that can restore every dirty non-main worktree.

- [ ] Add a dated “worktree consolidation” entry to `docs/project/project-progress.md` with status “进行中”, explicitly naming the two protected threads.
- [ ] Create the SDD ledger whose first line identifies this plan.
- [ ] Record every non-main worktree, including detached and missing/prunable entries, with classification and reason.
- [ ] For each dirty non-main worktree, save `git diff --binary`, staged diff, and untracked files without touching the source worktree.
- [ ] Verify the manifest and recovery archive can be read back before any merge.
- [ ] Commit the ledger, manifest, recovery snapshots, and progress registration on the consolidation branch.

### Task 2: Integrate unique non-main changes into the candidate

**Files:**
- Modify: runtime files selected by the manifest and merge review
- Create/modify: `.superpowers/sdd/2026-08-08-worktree-consolidation/merge-log.md`

**Interfaces:**
- Consumes: Task 1 manifest and recovery snapshots.
- Produces: one candidate branch containing the latest compatible runtime code and retained unique capabilities.

- [ ] Merge the latest `codex/fix-relay-admin-auth` implementation into the candidate and record every conflict resolution.
- [ ] Recover `codex/account` committed and dirty changes; retain group-aware scoring only where the latest implementation does not already provide equivalent behavior.
- [ ] Merge `fix/sub2api-0171-release` release-script changes when they are not already in the candidate; keep release code on the local/host chain.
- [ ] Classify documentation-only, generated-artifact, already-merged, and stale worktrees; preserve evidence without deploying obsolete runtime code.
- [ ] Run focused backend/relay-ops/frontend tests for each merged runtime area and record commands/results in `merge-log.md`.
- [ ] Commit the candidate integration as a single reviewable merge sequence.

### Task 3: Add the permanent workspace lifecycle constraints

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Consumes: the approved design and observed failure mode.
- Produces: enforceable repository rules for task registration, worktree freshness, merge-before-deploy, and cleanup.

- [ ] Add rules requiring every new task to register in the global ledger before implementation and update status at material transitions.
- [ ] Require a pre-implementation scan for non-main worktrees ahead of `main`; completed work must be merged to `main` before creating a new task worktree.
- [ ] Require every deployment candidate to merge into `main`, run merge regression, push `main`, deploy from `main`, verify online, and only then delete its worktree.
- [ ] Require failed merge/build/deploy/verification to remain in the candidate worktree for the next fix loop.
- [ ] State that active protected worktrees are excluded only when explicitly named by the user; no broad “ignore all dirty worktrees” exception.
- [ ] Commit the global constraints separately from runtime integration.

### Task 4: Reconcile against the two active threads and prepare release

**Files:**
- Modify: `docs/project/project-progress.md`
- Create: `docs/superpowers/reports/2026-08-08-worktree-consolidation-verification.md`

**Interfaces:**
- Consumes: the protected threads’ final commits and the candidate branch.
- Produces: a `main` branch that includes the active threads plus the consolidated non-main candidate, with release evidence.

- [ ] Wait until both protected threads report completion and their changes are committed or otherwise explicitly handed off.
- [ ] Merge the two active-thread commits into `main` without resetting their work.
- [ ] Merge the consolidation candidate into the updated `main`; resolve final conflicts using the same semantic rules.
- [ ] Run merge regression, focused tests, frontend build/typecheck, migration/diff checks, and existing release preflight.
- [ ] Push `main` to the server and deploy using the reviewed local/host blue-green chain.
- [ ] Verify health/readiness, source/image identity, affected API/UI behavior, and migration state online.
- [ ] Mark the progress item “已完成” only after all three deployment facts are present; otherwise leave it “进行中” with the blocker.

### Task 5: Clean up only integrated worktrees

**Files:**
- Modify: `.superpowers/sdd/2026-08-08-worktree-consolidation/worktree-manifest.tsv`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Consumes: release verification and final merge ancestry.
- Produces: no stale integrated worktrees, with protected or blocked entries explicitly retained.

- [ ] Confirm each deletion target is an integrated branch/worktree with no uncommitted or unarchived changes.
- [ ] Remove only those worktrees and local branches; keep the two protected active workspaces and any failed/blocked candidate.
- [ ] Prune missing worktree registrations only after their snapshots are verified and recorded.
- [ ] Update the manifest and ledger with deletion results and recovery locations.

