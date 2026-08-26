# T77 Licensed v4.1.1 Detector Reset Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy the commercially authorized v4.1.1 detector through the native sidecar contract and delete only the confirmed incompatible history after a recoverable export.

**Architecture:** The existing Sub scheduler, sidecar client, storage, and UI remain authoritative. A token-gated Python adapter loads the pinned v4.1.1 release inside the existing image and maps its bounded report into the existing sidecar response. A host-only purge script backs up and deletes non-v4.1.1 rows under an exact-count transaction guard.

**Tech Stack:** Go, Python 3 standard library, Node runtime, Docker, Bash, PostgreSQL, existing blue-green release scripts.

**Spec:** `docs/superpowers/specs/2026-08-27-t77-licensed-v411-detector-reset-design.md`

## Global Constraints

- The v4.1.1 release URL and SHA-256 are pinned; required notices remain in the image.
- No authorization text, token, key, URL, prompt, output, or raw retention is committed or exposed.
- The run table is the sole record source; no migration or backfill is allowed.
- Production deletion must export first, expect exactly 3,676 target rows, lock the table, and fail closed on any drift.
- No GitHub Actions; deployment is from clean `main` using the existing blue-green scripts.

### Task 1: Define and prove the adapter contract

**Files:**
- Create: `upstream/sub2api/deploy/model-detector-v411-adapter.py`
- Create: `upstream/sub2api/deploy/model-detector-v411-adapter_test.py`
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection_sidecar.go`
- Test: `upstream/sub2api/backend/internal/service/account_model_detection_sidecar_test.go`

- [ ] Write failing tests for a v4.1.1 report mapping, invalid token rejection, bounded response output, and a high-profile-compatible timeout.
- [ ] Run the focused Go/Python tests and observe the intended failures.
- [ ] Implement the token-gated `/healthz`, `/v1/catalog`, and `/v1/detect` adapter, with per-run temporary state cleanup and report-to-contract mapping.
- [ ] Increase the native sidecar client timeout only as required for high-profile execution.
- [ ] Re-run focused tests green and commit the adapter slice.

### Task 2: Pin the licensed release in the image and preserve release topology

**Files:**
- Modify: `upstream/sub2api/Dockerfile`
- Modify: `infra/compose.yaml`
- Modify: `infra/compose.sub2api-rehearsal.yaml`
- Modify: `ops/deploy-sub2api-blue-green-host.sh`
- Test: `tests/operations/model_detector_compose_contract_test.sh`
- Test: `tests/operations/deploy_sub2api_blue_green_host_test.sh`

- [ ] Write failing Docker/Compose contract assertions for pinned artifact checksum, wrapper command, Python/Node runtime, version `4.1.1`, and unchanged private detector topology.
- [ ] Run the focused shell tests and observe failure.
- [ ] Download, checksum-verify, unpack, normalize, and retain the full v4.1.1 release plus notices during image build; install the wrapper at `/app/model-detector`.
- [ ] Update deployment defaults and release-topology tests without changing credentials or public ports.
- [ ] Build the image and run all direct adapter/Compose/release-contract tests green.

### Task 3: Add the recoverable, exact-target history purge operation

**Files:**
- Create: `ops/purge-account-model-detection-runs.sh`
- Create: `tests/operations/purge_account_model_detection_runs_test.sh`
- Modify: `docs/superpowers/specs/2026-08-27-t77-licensed-v411-detector-reset-design.md`

- [ ] Write failing shell tests for missing expected count, unsafe backup path, foreign-key presence, count drift, and a successful transaction script shape.
- [ ] Run the shell test and observe failure.
- [ ] Implement a host-only guard that emits a restricted `pg_dump`, validates it, locks only the target table, deletes only non-v4.1.1 rows, and proves zero remaining target rows.
- [ ] Run the purge-script tests and syntax check green; commit the cleanup slice.

### Task 4: Candidate verification and handoff

**Files:**
- Create: `docs/handoffs/2026-08-27-t77-licensed-v411-detector-reset-handoff.md`

- [ ] Run targeted Go, Python, shell, image-build, Compose, frontend, typecheck/build and diff checks.
- [ ] Record artifact SHA, candidate SHA, test outputs, no-migration result, expected release behavior, deletion sequence, rollback path, and remaining risk.
- [ ] Commit all candidate work and report `READY_FOR_ROOT_REVIEW`.

### Task 5: Root integration, deletion, release, and validation

- [ ] Merge the verified candidate into clean root `main`, run the required root checks and release preflight.
- [ ] Before data mutation, execute the purge script on production with `T77_EXPECTED_ROWS=3676`; retain its backup and evidence without exposing secrets.
- [ ] If and only if the release preflight reports `downtime_required=false`, publish and promote through the existing blue-green chain.
- [ ] Verify public health, sidecar version `4.1.1`, a live bounded detection run with nonempty fingerprint result, and an empty legacy-result count.
- [ ] Update release evidence, task queue, and project ledger only after production validation succeeds.
