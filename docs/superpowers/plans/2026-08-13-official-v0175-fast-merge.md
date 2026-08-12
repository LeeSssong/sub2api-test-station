# Official Sub2API v0.1.175 Fast Merge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade the current production-derived Sub2API source snapshot from `v0.1.173` to official `v0.1.175`, semantically reapply existing Xingqiao behavior, and deploy through the reviewed blue-green chain.

**Architecture:** Prepare the upgrade in one isolated worktree. Reproduce the official three-way merge, resolve only the nine known conflict files by preserving both official fixes and existing product contracts, then run the narrow validation matrix from the approved specification before merging to `main` and deploying.

**Tech Stack:** Git, Bash release scripts, Go, Vue 3, Vitest, TypeScript, Docker/Compose blue-green host executor.

## Global Constraints

- Preserve T01-T04 behavior, billing safety, admin visibility, URLs, and update contracts.
- Do not implement T03-R1, externalize services, or use GitHub Actions.
- Do not use whole-file `ours` or `theirs` for semantic conflict files.
- Stop before deployment if conflicts remain, migration is not expand-only, or `downtime_required=true`.
- Keep the existing T03-R1 worktree untouched until the new production baseline is verified.

---

### Task 1: Create the isolated candidate and capture conflict baselines

**Files:**
- Modify: `docs/project/project-progress.md`
- Create: candidate-local release metadata and conflict evidence under a mode-0700 temporary directory

**Interfaces:**
- Consumes: `main@82e97418bd1480e03a869856e0cc872194839477`, official tag `v0.1.175`, official commit `93c32fa1a2450351561abc46156d2e28cb5f74ca`.
- Produces: isolated branch `codex/official-v0175-fast-merge`, exact stage-1/2/3 conflict blobs, and a clean reproducible merge state.

- [ ] Create `.worktrees/official-v0175-fast-merge` from the latest clean `main` and register it in the project ledger.
- [ ] Generate release metadata with `ops/sub2api-release-metadata.rb discover` and verify `base_version=0.1.173`, `version=0.1.175`, and `has_update=true`.
- [ ] Reproduce the three-way merge in a temporary official clone and record conflict paths plus stage blobs before resolving anything.
- [ ] Commit the ledger/baseline evidence without modifying T03-R1.

### Task 2: Semantically merge the nine conflict files

**Files:**
- Modify: the nine paths listed in `docs/superpowers/reports/2026-08-12-official-v0175-conflict-report.md`
- Modify: `upstream/sub2api/XINGQIAO_UPSTREAM.md`
- Modify: `upstream/sub2api/backend/cmd/server/VERSION`
- Test: existing focused Go and Vitest files adjacent to the conflicts

**Interfaces:**
- Consumes: stage-2 Xingqiao version, stage-3 official version, existing product tests, and official release notes.
- Produces: one clean source tree containing official `v0.1.175` plus retained Xingqiao behavior and version identity `0.1.175`.

- [ ] Resolve handler, pricing, scheduling, and stream-error conflicts function-by-function; retain official fixes and existing billing/error contracts.
- [ ] Resolve UsageView/UsageTable conflicts component-by-component; retain existing admin visibility and adopt official request-ID behavior.
- [ ] Regenerate only generated files required by the official merge, then verify there are no unmerged paths.
- [ ] Update provenance and version identity, add minimal regression assertions only where existing tests do not cover the chosen semantic merge, and commit.
- [ ] Run an independent task review; fix any blocking finding in the same candidate.

### Task 3: Qualify, merge, and deploy the candidate

**Files:**
- Modify: `docs/project/project-progress.md`
- Create: `docs/superpowers/reports/2026-08-13-official-v0175-production.md`

**Interfaces:**
- Consumes: clean Task 2 candidate commit.
- Produces: reviewed `main`, pushed source, qualified image, blue-green release record, and production acceptance evidence.

- [ ] Run focused Go tests/vet for affected packages; run UsageView/UsageTable Vitest, frontend typecheck/build, release version identity checks, and `git diff --check`.
- [ ] Classify migrations and run the reviewed release preflight; require `expand_only` and `downtime_required=false`.
- [ ] Run final whole-branch review and address blocking findings.
- [ ] Merge the reviewed candidate into clean `main`, rerun the same focused gates, and push `main` without force.
- [ ] Build/stage the qualified image and deploy through the existing host blue-green chain.
- [ ] Verify `/healthz`, `/readyz`, public version `0.1.175`, authenticated update state, admin usage loading, and protected container identities; roll back on failure.
- [ ] Record production evidence and give T03-R1 the verified new `main` SHA as its required rebase/merge baseline.
