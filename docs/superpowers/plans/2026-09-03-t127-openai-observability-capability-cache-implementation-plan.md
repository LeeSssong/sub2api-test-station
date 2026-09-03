# OpenAI Stream Observability, Capability Cache, And Cache Metric Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the native OpenAI request, scheduling, Monitor V4, and blue/green release contracts so model capability cooldowns, incomplete streams, lifecycle evidence, cache metrics, and deployment identity remain consistent without changing billing or replay semantics.

**Architecture:** Reuse the existing T113 stream observation/runtime and the existing model-not-found and shared-health scheduler state. Add only missing capability-state projection and explicit cache-token denominator fields, then tighten the existing release preflight and log contracts around immutable image identity. All lifecycle writes remain bounded best-effort asynchronous observations and never gate a request.

**Tech Stack:** Go, Gin, PostgreSQL query projections, Vue 3/TypeScript, POSIX shell, jq, existing Sub2API scheduler and blue/green scripts.

**Spec:** `docs/superpowers/specs/2026-09-03-openai-stream-observability-model-capability-cache-metric-design.md`

## Global Constraints

- Preserve existing pre-first-output 502/503 retry and account-switch boundaries.
- Do not replay requests after output, usage, tool calls, or other side effects.
- Do not add credentials, request/response bodies, SSE data, external tracing, or a second billing source.
- Keep lifecycle logging bounded, asynchronous, lossy under pressure, and non-blocking.
- Keep Monitor V4 success/real-request/probe filtering and the `2026-08-31` to `2026-09-02` exclusion window unchanged.
- Do not modify root `main`, project progress, task queue, release evidence, production state, or deploy any environment from this candidate.

### Task 1: Capability-state contract and scheduler filtering

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_shared_health.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/ratelimit_service.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_projection_test.go`
- Test: `upstream/sub2api/backend/internal/service/model_not_found_error_test.go`

**Interfaces:**
- Consume existing `OpenAISharedHealthKey`, `OpenAISharedHealthSnapshot`, `ClassifyOpenAINotFound`, and `HandleUpstreamModelNotFound`.
- Produce a model-isolated capability read/filter helper and regression coverage for unknown, supported, unsupported, cooldown, expiry recovery, and model isolation.

- [ ] Write failing tests for filtering `unsupported`/active `cooldown`, allowing `unknown`/expired cooldown, and preserving another model on the same account.
- [ ] Run the focused scheduler tests and confirm the new assertions fail because no capability filter exists.
- [ ] Implement the smallest read-only capability predicate using the existing shared-health/account-model key and existing model-rate-limit cooldown state; do not add a new database table.
- [ ] Add the 404 semantic assertion for `model_not_found`/`model_not_supported` and ensure cooldown writes use canonical mapped model keys.
- [ ] Run the focused tests and verify all capability cases pass.
- [ ] Run `gofmt` and commit the task changes.

### Task 2: Stream incomplete event and lifecycle queue hardening

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/stream_observability.go`
- Modify: `upstream/sub2api/backend/internal/service/stream_observability_runtime.go`
- Modify: the existing OpenAI Responses SSE forwarding implementation located by `rg -l "ObserveSSEEvent|ObserveReadFailure|FinishStreamObservation" upstream/sub2api/backend/internal/service`
- Test: `upstream/sub2api/backend/internal/service/stream_observability_test.go`
- Test: the existing Responses SSE forwarding test file covering EOF, decoder errors, client cancellation, and terminal events.

**Interfaces:**
- Consume the existing `StreamObservation` methods and bounded logging sink.
- Produce one `openai.stream_incomplete` observation for EOF/reset/decode/client-disconnect before a terminal event, while preserving terminal-plus-edge semantics and never changing retry/account/billing behavior.

- [ ] Add failing tests for EOF, connection reset, malformed SSE, client cancellation, and post-terminal client-write errors; assert distinct error class/client-disconnected/terminal flags.
- [ ] Run the focused stream tests and confirm the incomplete-event assertions fail.
- [ ] Implement the minimal transition/event naming and idempotence guard so incomplete is emitted once per attempt and only before terminal absence is established.
- [ ] Verify redaction removes bearer/token/cookie/query secrets while retaining transport classification.
- [ ] Run focused stream tests, including queue-full/write-failure non-blocking tests, and confirm request behavior is unchanged.
- [ ] Run `gofmt` and commit the task changes.

### Task 3: Monitor V4 native cache-token projection

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v4.go`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/types.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/api.ts`
- Test: `upstream/sub2api/backend/internal/service/monitor_v4_test.go`
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- Test: `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/api.spec.ts`

**Interfaces:**
- Consume existing successful real-request cache token columns.
- Produce `cache_read_tokens`, `cache_creation_tokens`, `cache_hit_denominator`, and nullable `cache_hit_rate`, with rate equal to `read / (creation + read)` and null for zero denominator.

- [ ] Add failing backend and frontend contract tests for numerator/denominator, ordinary input-token independence, and zero-denominator null/`--` behavior.
- [ ] Run focused tests and confirm they fail against the current input-token denominator and missing fields.
- [ ] Change the SQL projection and DTOs to return the two raw sums plus denominator and nullable rate without changing sample filtering or date exclusions.
- [ ] Update frontend validation/types/rendering to consume the server rate and never recompute from `input_tokens`.
- [ ] Run backend tests and frontend Monitor V4 tests; verify no NaN, Infinity, or false zero percent.
- [ ] Run `gofmt`, frontend formatter/typecheck as applicable, and commit the task changes.

### Task 4: Blue/green identity fail-closed contract

**Files:**
- Modify: `ops/deploy-sub2api-blue-green-host.sh`
- Modify: `ops/release-sub2api-blue-green.sh`
- Modify: `infra/Caddyfile`
- Modify: `infra/Caddyfile.acceptance`
- Test: `tests/operations/deploy_sub2api_blue_green_host_test.sh`
- Test: `tests/operations/release_sub2api_blue_green_test.sh`
- Test: `tests/operations/sub2api_blue_green_topology_test.sh`

**Interfaces:**
- Consume existing source commit/tree, immutable image labels/digests, active upstream, and container metadata.
- Produce fail-closed preflight when blue/green/worker/model-detector identity differs, and ensure Caddy/application logs expose environment, deployment commit, slot, container ID, image digest, and active upstream.

- [ ] Add failing shell fixtures for differing image digests, source commits, slots, and active upstream identity.
- [ ] Run the focused shell tests and confirm inconsistent fixtures are currently accepted.
- [ ] Implement validation using existing jq/docker inspection paths; do not introduce a new release path or GitHub Actions workflow.
- [ ] Add the required log fields to both Caddy contracts and runtime environment wiring where absent.
- [ ] Run focused shell contract tests and `git diff --check`.
- [ ] Commit the task changes.

### Task 5: Integrated verification and handoff

**Files:**
- Create: `docs/handoffs/2026-09-03-t127-openai-observability-capability-cache-handoff.md`

- [ ] Run direct related Go tests for Tasks 1-3.
- [ ] Run frontend Monitor V4 tests, typecheck, and build only if the changed frontend contract requires them.
- [ ] Run shell contract tests for Task 4 and `git diff --check`.
- [ ] Run `go build ./cmd/server` from the candidate source tree.
- [ ] Record baseline SHA, candidate commit/tree, changed files, tests, unverified items, migration/config/deploy status, rollback, and residual risks in the handoff.
- [ ] Leave the candidate at `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or alter root `main`.
