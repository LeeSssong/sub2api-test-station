# 独立测试站发布控制器 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a fail-closed release controller that deploys the pushed root `main` to the independent test station only.

**Architecture:** A local orchestrator validates Git provenance, builds and stages an immutable application image, then invokes a dedicated remote executor. The remote executor validates bundle identity, operates only the `sub2api-test-station` Compose project, records release metadata, and preserves the previous release on failure.

**Tech Stack:** Bash, Docker Buildx/Compose, SSH, SHA-256, shell contract tests.

**Spec:** `docs/superpowers/specs/2026-09-04-independent-test-station-release-controller-design.md`

## Global Constraints

- Deployments originate only from a clean root `main` whose commit/tree exactly equals `origin/main`.
- The only test station target is SSH alias `sub2api-test-station`, project `sub2api-test-station`, deploy root `/opt/sub2api-test-station/`.
- Do not use the historical acceptance scripts, `/admin/lab`, GitHub Actions, production env, or destructive volume commands.
- Secrets remain in protected files and are never printed or committed.

### Task 1: Contract tests and remote executor

**Files:**
- Create: `ops/deploy-sub2api-test-station-host.sh`
- Test: `tests/operations/deploy_sub2api_test_station_host_test.sh`

- [ ] Write failing shell tests for invalid project/path/checksum, successful compose invocation, and preservation of previous release on health failure.
- [ ] Run the test and confirm failures are caused by the missing executor.
- [ ] Implement argument parsing, bundle validation, image load, release directory creation, Compose project scoping, health wait, metadata persistence, and rollback preservation.
- [ ] Run the focused test until all cases pass.
- [ ] Commit the executor and tests.

### Task 2: Local provenance-gated orchestrator

**Files:**
- Create: `ops/release-sub2api-test-station.sh`
- Test: `tests/operations/release_sub2api_test_station_contract_test.sh`

- [ ] Write failing tests for non-main, dirty tree, origin drift, unsafe target, and correct remote argument construction.
- [ ] Run the test and confirm the missing orchestrator fails closed.
- [ ] Implement source/tree checks, protected-file checks, image archive checksum, isolated bundle creation, and SSH invocation of the dedicated executor.
- [ ] Run focused shell tests and `bash -n` on both scripts.
- [ ] Commit the orchestrator and tests.

### Task 3: Documentation and root integration

**Files:**
- Modify: `docs/runbooks/sub2api-acceptance-station.md`
- Modify: `docs/operations/independent-test-station-handoff.md`

- [ ] Document the new script, required protected paths, release-state fields, health probes, and rollback command shape.
- [ ] Run `git diff --check` and focused contract tests.
- [ ] Merge the candidate into root `main`, push `origin/main`, and verify clean source identity.
- [ ] Run the controller from root `main` to update the test station.
- [ ] Verify release-state source commit/tree, six services, `/health`, `/readyz`, and `/`.
