# T21 Model Detector Sidecar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Distinguish detector service availability from model support, pass the existing sidecar configuration into Sub API/worker processes, and verify a configured sidecar through the existing T15 contract.

**Architecture:** Extend the existing T15 catalog cache with an explicit `ready/unconfigured/unavailable` state and project it through the current admin API. The Vue dialog and card render that state while preserving native connection tests. Compose only passes operator-controlled URL/token values; the detector remains an external replaceable service.

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

### Task 3: Pass sidecar configuration through Compose

**Files:**
- Modify: `infra/compose.yaml`
- Create: `tests/operations/model_detector_compose_contract_test.sh`

**Interfaces:**
- Passes `SUB2API_MODEL_DETECTOR_URL` and `SUB2API_MODEL_DETECTOR_TOKEN` from the operator `.env` to blue, green, and worker through the existing shared environment anchor.
- Exposes no sidecar port and writes no secret value into the repository.

- [ ] **Step 1: Write a failing shell contract test** that renders Compose with sentinel URL/token values and asserts all three Sub services receive both values while no published detector port or literal token appears in `infra/compose.yaml`.
- [ ] **Step 2: Run RED test:** `bash tests/operations/model_detector_compose_contract_test.sh`.
- [ ] **Step 3: Add the two interpolated environment entries** to `x-sub2api-environment`.
- [ ] **Step 4: Run GREEN contract test** and `git diff --check`.

### Task 4: Candidate verification and handoff

**Files:**
- Create: `docs/handoffs/2026-08-18-t21-model-detector-sidecar-handoff.md`

**Interfaces:**
- Produces a `READY_FOR_ROOT_REVIEW` candidate with exact config/license prerequisites and rollback steps.

- [ ] **Step 1: Run direct backend tests** for model selection, catalog state, sidecar validation, enqueue/reuse, and worker execution.
- [ ] **Step 2: Run direct frontend tests**, `vue-tsc --noEmit`, `pnpm build`, the Compose contract test, backend compile-only, `gofmt`, and `git diff --check`.
- [ ] **Step 3: Record the base SHA, candidate SHA, changed files, tests, no-migration result, expected `downtime_required=false`, sidecar artifact/config prerequisite, and rollback in the handoff.**
- [ ] **Step 4: Commit the candidate** and stop at `READY_FOR_ROOT_REVIEW`; root alone may merge, push, configure the host, deploy, and perform production catalog/detection verification.
