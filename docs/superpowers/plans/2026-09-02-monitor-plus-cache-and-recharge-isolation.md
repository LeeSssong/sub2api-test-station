# Monitor, Plus, Cache, and Recharge Isolation Implementation Plan

**Goal:** Keep T91 quota accounting isolated, merge completed non-recharge work into `main`, correct the monitor `1h` window, and diagnose/fix Plus and cache metrics before a main-site-only release.

**Architecture:** Preserve T91 on `quota-accounting-long-lived` and its dedicated worktree. Integrate only non-recharge commits and apply monitor fixes against native `usage_logs`, account-monitor projections, and existing scheduler evidence. Use read-only evidence for Plus diagnosis and avoid scheduler changes unless a failing behavioral test proves a defect.

**Tech Stack:** Go, Vue/TypeScript, PostgreSQL SQL projections, Vitest, Go tests, existing local/host release scripts.

**Constraints:** Main-site deployment must originate from a clean pushed `main`; no GitHub Actions; no acceptance deployment; acceptance is read-only same-commit reconciliation after successful main-site release; no production business-data writes.

### Task 1: Preserve Recharge Isolation

- [x] Create `quota-accounting-long-lived` from the complete T91 commit.
- [x] Verify the branch contains T91 runtime code and migration `233_quota_accounting_foundation.sql`.
- [x] Verify root `main` does not contain T91 runtime code or migration.

### Task 2: Correct Monitor Window Contract

- [x] Add failing frontend tests for exactly `1h`, `24h`, and `7d` options and query parsing.
- [x] Remove `90m` and `30d` from the user monitor range type, options, parser, bootstrap copy, and polling comments.
- [ ] Run focused frontend tests and typecheck.

### Task 3: Plus and Cache Evidence

- [x] Identify native repository/service queries that produce success rate, latency, cache rate, selected account, and retry evidence.
- [x] Run read-only production evidence collection without printing credentials.
- [x] Add a focused regression only if the evidence demonstrates a calculation or scheduler behavior defect.
- [x] Fix the smallest proven defect and run direct tests.

### Task 4: Non-Recharge Integration

- [x] Confirm completed non-recharge candidates and their exact commits.
- [x] Selectively integrate T113 runtime/docs while excluding T91 files and migrations.
- [ ] Run direct tests, build/typecheck, source guard, and diff checks on clean root `main`.
- [ ] Push `origin/main`.

### Task 5: Main-Site Release and Reconciliation

- [ ] Run the existing main-site preflight from clean `main`.
- [ ] Deploy only the main site using the approved fast-deploy path.
- [ ] Verify health, version/tree identity, monitor `1h`, Plus metrics, and cache output.
- [ ] Read-only reconcile acceptance station commit/tree without deploying it.
