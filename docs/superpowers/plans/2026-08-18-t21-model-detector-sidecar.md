# T21 Model Detector Sidecar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Distinguish detector service availability from model support, pass the existing sidecar configuration into Sub API/worker processes, and verify a configured sidecar through the existing T15 contract.

**Architecture:** Extend the existing T15 catalog cache with an explicit `ready/unconfigured/unavailable` state and project it through the current admin API. The Vue dialog and card render that state while preserving native connection tests. A separately built `/app/model-detector` binary runs as a private Compose service from the same qualified immutable image; API and worker access it only through the existing replaceable HTTP contract.

**Tech Stack:** Go, Gin service DTOs, Vue 3, TypeScript, Vitest, Docker Compose, shell contract checks.

**Spec:** `docs/superpowers/specs/2026-08-18-t21-model-detector-sidecar-design.md`

## Global Constraints

- Work only in `.worktrees/t21-model-detector-sidecar`; do not modify root queue/progress or production from this worktree.
- Keep T15 persistence, scheduling, connection probes, account status, scoring, billing, profitability, and group recommendation behavior unchanged.
- Do not copy detector core, baselines, or reports from `tools/gpt56_api_detector-git` into the Sub image.
- Do not persist or expose sidecar tokens, account API keys, base URLs, full prompts, or full outputs.
- Add no migration, history backfill, production business-data write, dependency, or GitHub Actions workflow.

---

### Task 1: Add explicit backend detector availability

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection_sidecar.go`
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection.go`
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection_sidecar_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection_test.go`

**Interfaces:**
- Produces `AccountModelDetectorState` values `ready`, `unconfigured`, and `unavailable`.
- Adds `detector_state` to `AccountModelDetectionModelsResponse` and `AccountModelDetectionProjection`.
- Uses stable sentinel errors `ErrAccountModelDetectorNotConfigured` and `ErrAccountModelDetectorUnavailable`.

- [ ] **Step 1: Write failing tests** for an empty URL returning the not-configured sentinel, a transport/catalog failure returning unavailable, a successful catalog returning ready, projection status mapping, cached state, and enqueue rejection while offline.
- [ ] **Step 2: Run RED tests:** `go test ./internal/service -run 'Test(HTTPAccountModelDetectionSidecar|ModelDetectorAvailability|ProjectionUsesDetectorServiceState|EnqueueRejectsOfflineDetector)' -count=1`.
- [ ] **Step 3: Implement the minimal state contract** by caching catalog models and state together, assigning option reasons from the state, and preserving OAuth `unsupported` plus ready-catalog per-model `detector_unsupported`.
- [ ] **Step 4: Run GREEN tests** with the same command, then run `gofmt` on the changed Go files.

### Task 2: Render accurate offline semantics in the native dialog/card

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountModelDetectionDialog.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/accounts.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/accounts.ts`

**Interfaces:**
- Consumes backend `detector_state` and projection statuses `service_unconfigured/service_unavailable`.
- Produces visible copy `检测服务未接入` and `检测服务暂不可用`; `检测器暂不支持` is used only when `detector_state=ready`.

- [ ] **Step 1: Add failing Vitest cases** for unconfigured, unavailable, and ready-but-model-unsupported responses; assert the detection select/run button state and that the connection model select stays enabled.
- [ ] **Step 2: Run RED test:** `pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts` from `upstream/sub2api/frontend`.
- [ ] **Step 3: Add typed states, translations, status copy, and button/select guards** without changing card layout or native connection-test actions.
- [ ] **Step 4: Run GREEN focused Vitest**, then `pnpm exec vue-tsc --noEmit` and `pnpm build`.

### Task 3: Implement the independent sidecar contract

**Files:**
- Create: `upstream/sub2api/backend/cmd/model-detector/main.go`
- Create: `upstream/sub2api/backend/cmd/model-detector/main_test.go`
- Modify: `upstream/sub2api/Dockerfile`

- [x] **Step 1: Write direct tests** for models endpoint normalization, bearer authentication, catalog output, matching model and unavailable/unauthorized upstream outcomes.
- [x] **Step 2: Implement a standard-library HTTP service** with bounded bodies/timeouts and no credential logging or persistence.
- [x] **Step 3: Build the binary in the existing backend builder** and copy it into the qualified runtime image.
- [x] **Step 4: Run focused Go test/build and gofmt.**

### Task 4: Pass sidecar configuration through Compose and the host release chain

**Files:**
- Modify: `infra/compose.yaml`
- Modify: `infra/compose.sub2api-rehearsal.yaml`
- Modify: `ops/deploy-sub2api-blue-green-host.sh`
- Create: `tests/operations/model_detector_compose_contract_test.sh`

**Interfaces:**
- Declares a private `model-detector` service from the same candidate image and passes its URL/token to blue, green, and worker.
- Upgrades legacy rendered production Compose only after read-only release gates and restores Compose/secret files on failure.

- [x] **Step 1: Extend the shell contract test** to require the private service, command, URL/token wiring and no published detector port.
- [x] **Step 2: Add production/rehearsal Compose services** and current production model catalog defaults.
- [x] **Step 3: Extend the reviewed host executor** with atomic legacy JSON Compose patching, token generation, detector health startup, and failure restoration.
- [x] **Step 4: Run Compose rendering, controller/host release contracts, shell syntax and diff-check.**

### Task 5: Candidate verification and handoff

**Files:**
- Create: `docs/handoffs/2026-08-18-t21-model-detector-sidecar-handoff.md`

**Interfaces:**
- Produces a `READY_FOR_ROOT_REVIEW` candidate with exact config/license prerequisites and rollback steps.

- [ ] **Step 1: Run direct backend tests** for model selection, catalog state, sidecar validation, enqueue/reuse, and worker execution.
- [ ] **Step 2: Run direct frontend tests**, `vue-tsc --noEmit`, `pnpm build`, the Compose contract test, backend compile-only, `gofmt`, and `git diff --check`.
- [ ] **Step 3: Record the base SHA, candidate SHA, changed files, tests, no-migration result, expected `downtime_required=false`, sidecar artifact/config prerequisite, and rollback in the handoff.**
- [ ] **Step 4: Commit the candidate** and stop at `READY_FOR_ROOT_REVIEW`; root alone may merge, push, configure the host, deploy, and perform production catalog/detection verification.
