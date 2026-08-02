# OpenAI Model Mapping Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Record the upstream model returned by OpenAI-compatible JSON, SSE, and Responses WebSocket requests in `usage_logs.actual_response_model` without changing request behavior.

**Architecture:** Add one nullable Ent field and migration, expose a repository update-by-request-ID method, and add a small OpenAI-only response model extractor used at existing response forwarding boundaries. The admin usage DTO/table reads the new nullable field; raw response bytes/messages remain untouched.

**Tech Stack:** Go, Gin, Ent, PostgreSQL migrations, Vue 3/TypeScript, Go/Vitest tests.

## Global Constraints

- OpenAI protocol only; do not alter Claude, Gemini, Grok, or other protocol handlers.
- Do not change routing, scheduling, billing, or client-visible response content.
- Persist only a model string with maximum length 100; never persist full request/response/SSE content.
- New migration number must be the next available number: `193`.

### Task 1: Database and usage-log model

**Files:**
- Create: `upstream/sub2api/backend/migrations/193_usage_log_actual_response_model.sql`
- Modify: `upstream/sub2api/backend/ent/schema/usage_log.go`
- Regenerate: `upstream/sub2api/backend/ent/*` generated usage-log files using the repository generator
- Test: `upstream/sub2api/backend/migrations/*` migration regression coverage if required by existing conventions

**Deliverable:** nullable `actual_response_model` field and migration adding `VARCHAR(100)` column.

- [ ] Write a failing schema/migration assertion that the field exists, is nullable, and is capped at 100 characters.
- [ ] Run the focused migration/schema test and confirm it fails because the field is absent.
- [ ] Add the Ent field and SQL migration with no index unless existing query requirements demand one.
- [ ] Regenerate Ent code and run focused backend tests.
- [ ] Commit the task changes.

### Task 2: Response model extraction and persistence

**Files:**
- Create or modify: `upstream/sub2api/backend/internal/service/openai_response_model_audit.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_query.go` or the existing usage-log repository interface/implementation
- Modify: `upstream/sub2api/backend/internal/service/usage_log.go` or the service UsageLog type to carry the nullable field
- Test: `upstream/sub2api/backend/internal/service/openai_response_model_audit_test.go`

**Interfaces:**
- `ExtractOpenAIResponseModelJSON([]byte) string`
- `ExtractOpenAIResponseModelSSEEvent(eventType string, data []byte) string`
- `UpdateActualResponseModelByRequestID(ctx context.Context, requestID, model string) error`

- [ ] Write failing unit tests for nested JSON, top-level JSON, relevant SSE completion events, WebSocket-style JSON messages, missing model, malformed JSON, and max-length safety.
- [ ] Run the focused tests and verify expected failures.
- [ ] Implement minimal extraction helpers that return only a trimmed model string and never retain raw payloads.
- [ ] Add repository update-by-request-ID SQL/Ent operation that ignores empty model values.
- [ ] Run focused tests and then the usage-log repository suite.
- [ ] Commit the task changes.

### Task 3: OpenAI HTTP JSON/SSE/WebSocket integration

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go` and/or the existing OpenAI responses forwarding files where raw upstream JSON/SSE/WS messages are handled
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_service.go` or shared OpenAI forwarding helpers as needed
- Tests: existing OpenAI handler/service test files plus new focused tests

**Interfaces:** consume the extractor and repository update method from Task 2.

- [ ] Add failing integration tests proving request model `gpt-5.6-sol` plus upstream `gpt-5.6-terra` writes the audit field for JSON, SSE, and WebSocket paths.
- [ ] Add failing tests proving missing model leaves the field empty and does not fail the request.
- [ ] Add failing tests asserting client response bytes/messages are exactly unchanged.
- [ ] Run the focused tests and verify red failures.
- [ ] Wire extraction at existing response completion/forwarding points; update usage logs best-effort after model extraction, without buffering or rewriting responses.
- [ ] Run OpenAI handler/service tests and fix only feature-related failures.
- [ ] Commit the task changes.

### Task 4: Admin usage-log display

**Files:**
- Modify: `upstream/sub2api/frontend/src/types/index.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/UsageView.vue`
- Modify: relevant admin usage API/mapper tests

- [ ] Write a failing frontend test that renders `actual_response_model` as a distinct column and leaves it blank when null.
- [ ] Run the focused frontend test and verify failure.
- [ ] Add the nullable field to TypeScript types and table/export mapping.
- [ ] Run focused frontend tests and lint/typecheck.
- [ ] Commit the task changes.

### Task 5: Whole-feature review and verification

**Files:**
- Modify: `docs/project/project-progress.md`
- Review all changed files from Tasks 1-4.

- [ ] Run backend focused tests, migration tests, and frontend focused tests.
- [ ] Run broader OpenAI/backend and frontend suites proportional to runtime.
- [ ] Verify the acceptance SQL uses `model` versus `actual_response_model`.
- [ ] Confirm no raw payload persistence or client response mutation was introduced.
- [ ] Update progress to “工程代码/配置差异待部署” because deployment and online verification are not part of this local task.
- [ ] Perform final whole-branch review before reporting completion.
