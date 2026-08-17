# T15 Native Probe Model Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add native per-account probe-model selection and persistent asynchronous model detection without changing Sub account health, billing, scoring, or scheduling facts.

**Architecture:** Extend the existing account-monitor projection and connection-test path, while storing model-detection settings/runs in separate tables. A worker-owned service performs durable dedupe and fixed-slot scheduling, then calls a private sidecar client that validates and persists only bounded detector summaries. The existing account monitor card receives a compact status row and lazy details dialog.

**Tech Stack:** Go, PostgreSQL migrations, Gin, sqlmock/testify, Vue 3 + TypeScript, Vitest, Tailwind classes.

## Global Constraints

- Work only in `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t15-native-probe-model-detection`; do not modify root queue/progress, `main`, production, release records, or deploy.
- Reuse Sub native account model registration and `AccountTestService.ProbeAccountConnection`.
- Detector credentials are memory-only; never persist API keys, full prompts, full outputs, or upstream URLs.
- Detector results never alter account status, monitor score, scheduler weight, schedulable flag, billing, or profitability.
- Existing detector source is PolyForm Noncommercial; implement the Sub contract and sidecar boundary without copying its core implementation or baselines.

---

### Task 1: Add the independent model-detection data contract and migration

**Files:**
- Create: `upstream/sub2api/backend/migrations/225_account_model_detection.sql`
- Create: `upstream/sub2api/backend/migrations/account_model_detection_migration_test.go`
- Create: `upstream/sub2api/backend/internal/service/account_model_detection_types.go`

**Interfaces:**
- Produces `AccountModelDetectionRun`, `AccountModelDetectionSummary`, `AccountModelDetectionModelOption`, and status constants for repository/service/UI tasks.

- [ ] Step 1: Write the migration contract test asserting both tables, FK behavior, status checks, and partial/slot uniqueness.
- [ ] Step 2: Run `go test ./migrations -run TestAccountModelDetectionMigration -count=1` and observe failure because migration 225 is absent.
- [ ] Step 3: Add idempotent SQL for settings and runs, indexes, bounded JSON columns, and `ON DELETE CASCADE` settings/results cleanup.
- [ ] Step 4: Add Go DTOs with JSON tags and eight UI statuses (`untested`, `queued`, `running`, `normal`, `abnormal`, `insufficient`, `failed`, `unsupported`).
- [ ] Step 5: Run the migration test and `gofmt -w` on new Go files.

### Task 2: Implement native model selection, sidecar client, repository, and service

**Files:**
- Create: `upstream/sub2api/backend/internal/service/account_model_detection.go`
- Create: `upstream/sub2api/backend/internal/service/account_model_detection_sidecar.go`
- Create: `upstream/sub2api/backend/internal/service/account_model_detection_test.go`
- Create: `upstream/sub2api/backend/internal/service/account_model_detection_sidecar_test.go`
- Create: `upstream/sub2api/backend/internal/repository/account_model_detection_repo.go`
- Create: `upstream/sub2api/backend/internal/repository/account_model_detection_repo_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account.go` (shared text-model helper only if needed)

**Interfaces:**
- `AccountModelDetectionRepository` loads/saves settings, enqueues/reuses runs, claims runs, persists bounded results, and lists recent runs.
- `AccountModelDetectionSidecar` exposes `Catalog(ctx)` and `Detect(ctx, request)`.
- `AccountModelDetectionService` exposes `Models`, `SaveModels`, `EnqueueImmediate`, `RunDueSlots`, `Recent`, and `ProjectionForAccount`.

- [ ] Step 1: Write failing tests for Sol-first/first-text fallback, native-model∩catalog options, OAuth unsupported, duplicate immediate reuse, slot dedupe, and 30-minute late window.
- [ ] Step 2: Run focused service tests and observe failures for missing service/types.
- [ ] Step 3: Implement native model extraction from `GetModelMapping` plus platform defaults, deterministic sort, Sol-first selection, and catalog intersection.
- [ ] Step 4: Write failing sidecar tests for redacted request logging, response enum/length validation, non-2xx, timeout, and malformed JSON.
- [ ] Step 5: Implement private HTTP sidecar client with configurable endpoint/token, no URL/API-key logging, bounded JSON decoding, and fail-closed catalog behavior.
- [ ] Step 6: Implement repository SQL and transaction-safe unique-run reuse; claim only queued runs and update terminal results without touching account rows.
- [ ] Step 7: Implement service execution: re-read account before sidecar call, skip OAuth/deleted/model-invalid accounts, invoke sidecar once, map result status, and persist sanitized summary.
- [ ] Step 8: Implement due-slot computation in `Asia/Shanghai` and `RunDueSlots`; use `slot_key` as durable idempotency key and 30-minute grace.
- [ ] Step 9: Run focused service/repository tests, `gofmt`, and `git diff --check`.

### Task 3: Wire worker scheduling and admin HTTP APIs into native monitor

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_runner.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/backend/cmd/server/wire_gen.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go`
- Create: `upstream/sub2api/backend/internal/server/routes/account_model_detection_routes_test.go`
- Create: `upstream/sub2api/backend/internal/handler/admin/account_model_detection_handler_test.go`

**Interfaces:**
- Adds model/detection fields to the existing monitor projection without removing legacy fields.
- Adds `/admin/account-monitors/:account_id/models`, `/models` PUT, `/detection` POST, and `/detection` GET.

- [ ] Step 1: Write failing route/handler tests for model reads/writes, immediate enqueue response, recent history, and admin authentication path.
- [ ] Step 2: Add optional detection service to the native monitor service/handler so legacy test constructors remain valid when nil.
- [ ] Step 3: Project detection data into each `AccountMonitorAccount`; leave scoring/ranking code unchanged.
- [ ] Step 4: Add a worker-owned fixed-slot loop that calls `RunDueSlots` and preserves existing connection-monitor cadence; stop it through existing cleanup.
- [ ] Step 5: Wire repository, sidecar, detection service, and runner using existing provider sets; keep API processes read-only and start detector loop only for singleton worker roles.
- [ ] Step 6: Add handlers/routes and run focused backend route tests plus package compile-only checks.

### Task 4: Add card state row, details dialog, and model controls

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Create: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountModelDetectionDialog.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Create: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/accounts.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/accounts.ts`

**Interfaces:**
- Adds API methods/types for model options, save, enqueue, and recent results.
- Card emits model edit/detect events to the view; dialog never handles credentials.

- [ ] Step 1: Write failing Vitest cases for each status label, default-collapsed row, details dialog fields, unsupported disabled option, and immediate-detect action.
- [ ] Step 2: Run `npm run test -- src/components/admin/account-monitor/AccountMonitorCard.spec.ts` and observe red tests.
- [ ] Step 3: Add typed API methods and projection fields.
- [ ] Step 4: Implement the dialog and compact card row with bounded text, explicit “检测器观察到异常” wording, and no global summary.
- [ ] Step 5: Wire view actions, loading/error states, and refresh projection after enqueue/model save.
- [ ] Step 6: Run focused Vitest, frontend typecheck/build, and diff-check.

### Task 5: Candidate handoff

**Files:**
- Create: `docs/handoffs/2026-08-17-t15-native-probe-model-detection-handoff.md`
- Create: `docs/superpowers/reviews/2026-08-17-t15-implementation-plan-self-review.md`

- [ ] Step 1: Re-read the spec and checklist every contract against changed files/tests.
- [ ] Step 2: Record migration/config changes, license gate, unverified production detector status, no-deploy instruction, commit SHA, and rollback approach.
- [ ] Step 3: Run final direct tests, `git diff --check`, and `git status`; stop at `READY_FOR_ROOT_REVIEW` without merge, push, or deploy.
