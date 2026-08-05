# Sub2API 0.1.171 Release Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development` and execute every implementation
> task with a fresh implementer followed by an independent reviewer.

**Goal:** Deterministically merge official Sub2API `v0.1.171`, publish one
qualified immutable image, adapt the host updater to the existing blue-green
topology, and complete the authorized production cutover with focused online
acceptance.

**Architecture:** The generic release merge remains fail-closed. A separate
resolver handles only the exact recorded transition from the current Xingqiao
provenance to the annotated `v0.1.171` release when target identity, conflict
set, and every conflict preimage match. The host updater validates the staged
image and delegates to the installed blue-green executor with preloaded-image
mode and canonical production paths. GitHub Actions performs the single full
qualification; local and production checks stay focused on affected contracts.

**Release identity:** tag `v0.1.171`, tag object
`afd154b92aac36c6dafb1fa8e181ca827c78c465`, official commit
`f0e7a9c7a23a7d02fb159b62fa809621eb0475a6`.

**Global constraints:**

- Preserve Xingqiao account monitoring, managed/manual multiplier, procurement
  cost, and related behavior while incorporating official `0.1.170/0.1.171`
  changes.
- Unknown target identities, conflict sets, or preimages must fail before a
  candidate commit or image is produced.
- Regenerate `backend/cmd/server/wire_gen.go` and `backend/go.sum`; do not
  accept opaque conflict-side choices for generated files.
- Production uses the installed blue-green executor. Do not rewrite Compose,
  recreate shared services, or fall back to the retired single-service update.
- PostgreSQL, Redis, and Caddy identities must remain unchanged.
- Mark the project ledger complete only after source push, production deploy,
  and focused online verification all succeed.

---

### Task 1: Add The Version-Pinned Release Conflict Resolver

**Files:**

- Modify: `ops/merge-sub2api-release.sh`
- Create: `ops/resolve-sub2api-release-conflicts.sh`
- Create/Modify: resolver records under `ops/sub2api-release-resolutions/`
- Modify: `tests/operations/merge_sub2api_release_test.rb`

**Interfaces:** Consumes the synthetic merge repository, current provenance,
target tag object/commit, exact unresolved paths, and preimage hashes. Produces
only a fully resolved Git index/worktree or a non-zero fail-closed result.

- [ ] Add failing tests for exact `v0.1.171` dispatch, target mismatch,
  conflict-set mismatch, preimage mismatch, and an unknown future release.
- [ ] Implement a minimal resolver interface that receives explicit release
  identities and repository path without weakening the generic merge gate.
- [ ] Record deterministic postimages for the exact `v0.1.171` transition,
  verifying all identities before writing any resolution.
- [ ] Regenerate `wire_gen.go` and `go.sum`, then reject conflict markers and
  unmerged index entries.
- [ ] Run `ruby tests/operations/merge_sub2api_release_test.rb` and shell syntax
  checks for the two scripts.
- [ ] Commit the task and obtain independent review before Task 2.

### Task 2: Prove The Resolved 0.1.171 Semantics

**Files:**

- Modify only the resolver records or release-focused tests if Task 1 review
  reveals semantic omissions.
- Test: affected backend packages and frontend specs in the resolved candidate.

**Interfaces:** Consumes the exact synthetic merge result. Produces focused
evidence that Xingqiao behavior and official `0.1.170/0.1.171` behavior coexist
and that generated dependencies are reproducible.

- [ ] Reproduce the full exact merge from a clean temporary clone and assert
  the resolved tree has no conflict markers or unmerged entries.
- [ ] Compare each resolved conflict against base, Xingqiao, and official sides;
  add focused behavioral tests for account monitoring, multiplier/procurement,
  profit control, authentication/settings, and Codex identity overlap.
- [ ] Run affected Go package tests and generation checks only; run affected
  frontend tests/build checks only where a conflict changed frontend behavior.
- [ ] Confirm a second clean reproduction produces the same candidate tree.
- [ ] Commit any corrections and obtain independent review before Task 3.

### Task 3: Adapt The Host Updater To Blue-Green Production

**Files:**

- Modify: `ops/update-sub2api-host.sh`
- Modify: `tests/operations/update_sub2api_host_test.sh`
- Modify only if required: `sub2api-updater/internal/updater/*`

**Interfaces:** Continues to accept `--image sha256:...`, `--version`,
`--operation-id`, and the updater contract version. Resolves one approved GHCR
RepoDigest plus qualification labels, then invokes
`/usr/local/libexec/deploy-sub2api-blue-green-host.sh` with production mode,
preloaded-image mode, exact source/test/migration identity, canonical paths,
and an absolute deadline.

- [ ] Add failing shell tests for successful delegation and refusal of missing
  RepoDigest, ambiguous/unapproved digest, invalid labels, source/test tree
  mismatch, missing/inconsistent blue-green state, and executor failure.
- [ ] Replace legacy service discovery/Compose mutation with strict blue-green
  state validation and argument/environment translation.
- [ ] Preserve updater event logging and terminal state without leaking API
  keys or accepting an executor result that is not explicitly successful.
- [ ] Run `bash tests/operations/update_sub2api_host_test.sh`, affected updater
  Go tests if touched, and shell syntax checks.
- [ ] Commit the task and obtain independent review before Task 4.

### Task 4: Wire Installer And Production Environment Contracts

**Files:**

- Modify: `infra/systemd/sub2api-updater.env.example`
- Modify: `infra/systemd/sub2api-updater.service` only if sandbox paths require it
- Modify: `ops/install-sub2api-updater.sh`
- Modify: `tests/operations/install_sub2api_updater_test.sh`

**Interfaces:** Installs the updater and adapter with explicit blue-green
executor, release state/record, Compose/environment, API key, base URL, network
curl image, staging root, and deadline settings matching the production host.

- [ ] Add failing packaging tests for every required blue-green environment
  variable and protected path.
- [ ] Update the example and installer validation without embedding secrets.
- [ ] Ensure systemd read/write access covers `/var/lib/sub2api/release-state`,
  `/var/lib/sub2api/release-records`, and staging locations actually mutated by
  the delegated executor.
- [ ] Run installer packaging tests, unit verification where available, and
  shell syntax checks.
- [ ] Commit the task and obtain independent review before Task 5.

### Task 5: Integrate, Push, And Publish The Qualified Candidate

**Files:**

- Modify: `docs/project/project-progress.md` with intermediate evidence.
- Create: `docs/superpowers/reports/2026-08-06-sub2api-0171-release-recovery-verification.md`
- Modify release workflow/tests only if focused integration exposes a contract
  mismatch directly caused by Tasks 1-4.

**Interfaces:** Pushes the reviewed source, triggers one `v0.1.171` release
workflow, and produces a staged immutable image plus source advancement record.

- [ ] Run focused local verification: merge/resolver tests, updater/installer
  tests, affected Go/frontend tests, shell syntax, and `git diff --check`.
- [ ] Run a whole-branch independent review and resolve every blocking finding.
- [ ] Push the implementation branch and integrate it into the canonical remote
  release branch without asking the user to resolve conflicts manually.
- [ ] Trigger exactly one authoritative `v0.1.171` GitHub Actions qualification;
  require discovery, deterministic merge, full backend/frontend qualification,
  Linux AMD64 image build, immutable GHCR publication, production staging, and
  source advancement to succeed.
- [ ] Verify the host has `xingqiao-sub2api:upstream-0.1.171` with the expected
  labels, image ID, and approved RepoDigest before cutover.

### Task 6: Execute Production Cutover And Focused Acceptance

**Files:**

- Modify: `docs/project/project-progress.md`
- Complete: `docs/superpowers/reports/2026-08-06-sub2api-0171-release-recovery-verification.md`

**Interfaces:** Submits the immediate update through the production updater,
which delegates to the blue-green executor. Produces atomic release state and
record evidence or uses the existing rollback checkpoint on failure.

- [ ] Capture pre-cutover active slot, image, release state, and PostgreSQL,
  Redis, and Caddy container IDs.
- [ ] Install/restart the reviewed updater package and verify its executor/env
  contract before submitting the update.
- [ ] Submit the authorized `0.1.171` update and wait for a terminal successful
  blue-green result; if it fails after cutover begins, verify rollback before
  further action.
- [ ] Verify only the approved production matrix: reported version `0.1.171`,
  public health, authenticated admin availability, authenticated `/v1/models`,
  worker health, unchanged PostgreSQL/Redis/Caddy identities, and final release
  state/record matching the promoted immutable image.
- [ ] Commit and push the final evidence and change the ledger from `进行中` to
  `已完成` only after push + deploy + online verification are all true.

## Completion Review

- [ ] Independent final reviewer confirms the branch matches the approved scope,
  no unrelated suites or systems were changed, rollback protection remains
  intact, and the production evidence supports every completion claim.
- [ ] `git status --short`, remote branch identity, workflow conclusion,
  production release state, and the ledger all agree on the final release.
