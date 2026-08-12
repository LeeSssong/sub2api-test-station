# T02 Native Error Diagnostics Implementation Plan

> **For agentic workers:** Execute inline in this sole-implementer task; use strict test-driven-development and retain RED/GREEN evidence for every behavior.

**Goal:** Add one persisted-field four-class diagnosis shared by user-safe and administrator error details without changing transport protocol behavior.

**Architecture:** A pure backend projector classifies and sanitizes existing `OpsErrorLogDetail` fields. Service read paths attach the admin diagnosis and derive a separate user-safe DTO; the existing shared admin modal renders diagnosis and the user modal renders only safe meaning/suggestion.

**Tech Stack:** Go, Gin/native ops DTOs, Vue 3, TypeScript, vue-i18n, Vitest.

## Global Constraints

- Four classes only: `local_limit`, `upstream_overloaded`, `upstream_failed`, `upload_interrupted`.
- Preserve HTTP/SSE machine-client status, type, code, and body behavior.
- No migration, new page, new configuration system, GitHub Actions, merge, push, deploy, or production access.
- User output must not contain raw upstream evidence or internal account/group/upstream/request-body data.

### Task 1: Backend diagnosis projection

**Files:**
- Create: `upstream/sub2api/backend/internal/service/native_error_diagnostics.go`
- Create: `upstream/sub2api/backend/internal/service/native_error_diagnostics_test.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_models.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_service.go`

- [ ] Add failing table tests for four classes, selected/not-selected, sanitization, fallback, and HTTP/SSE equivalence.
- [ ] Run focused test and retain expected RED output.
- [ ] Implement the minimal pure projector and attach it in the admin service read path.
- [ ] Run focused test and retain GREEN output.

### Task 2: User-safe projection

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/ops_user_error.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_user_error_test.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_service_user_error_test.go`

- [ ] Add failing tests proving safe meaning/suggestion and JSON absence of raw body/status/internal evidence.
- [ ] Run focused test and retain expected RED output.
- [ ] Replace raw user fields with the safe projection while retaining request ID and ordinary owned request metadata.
- [ ] Run focused test and retain GREEN output.

### Task 3: Existing modal UI and types

**Files:**
- Modify: `upstream/sub2api/frontend/src/types/index.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/ops.ts`
- Modify: `upstream/sub2api/frontend/src/components/user/UserErrorDetailModal.vue`
- Modify: `upstream/sub2api/frontend/src/components/user/__tests__/UserErrorDetailModal.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/ops/components/OpsErrorDetailModal.vue`
- Create: `upstream/sub2api/frontend/src/views/admin/ops/components/__tests__/OpsErrorDetailModal.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/{zh,en}/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/{zh,en}/admin/ops.ts`

- [ ] Add failing UI tests for safe user rendering and shared admin diagnosis rendering.
- [ ] Run focused Vitest and retain expected RED output.
- [ ] Add types/i18n and minimally render the new DTO fields in the existing modals.
- [ ] Run focused Vitest and retain GREEN output.

### Task 4: Verification and handoff

- [ ] Run focused and relevant backend tests, `go vet`, backend build.
- [ ] Run focused frontend tests, typecheck, and build.
- [ ] Run `git diff --check`, inspect diff for scope and secrets, update the ledger to implementation-ready review state (still “进行中”).
- [ ] Commit all T02 changes and confirm clean worktree; stop at `READY_FOR_ROOT_REVIEW`.

## Rollback

Revert the T02 commit(s). No data or configuration rollback is required.
