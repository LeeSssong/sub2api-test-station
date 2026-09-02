# T113 流式请求全链路可观测与根因诊断实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变调度、重试、计费或上游协议的前提下，为 Sub2API OpenAI 流式请求补齐关键生命周期日志、稳定关联 ID、脱敏传输错误分类、蓝绿运行身份和管理员只读诊断。

**Architecture:** 复用现有 request middleware、context、zap/sink logger、`ops_error_logs`、`usage_logs` 和管理员 Ops/Usage 详情。新增一个小型流观测状态对象负责事件字段、SSE 生命周期和错误分类；诊断快照作为现有错误日志的脱敏 JSON 扩展保存，不新增数据库表或迁移。Caddy/Compose 仅增加结构化访问日志字段和日志保留合同。

**Tech Stack:** Go 1.27、Gin、zap、PostgreSQL、Vitest、Vue 3、Caddy、Docker Compose、Bash contract tests。

**Spec:** `docs/superpowers/specs/2026-09-02-stream-observability-root-cause-diagnosis-design.md`

## Global Constraints

- 只复用 Sub2API 原生请求、错误、usage、管理员详情和蓝绿发布链，不建设外部 tracing 平台或第二业务事实源。
- 不改变调度、重试预算、计费、服务档位、账号状态、上游协议或用户错误响应兼容性。
- Authorization、API Key、Cookie、OAuth token、代理密码、请求/响应正文和完整 SSE data 永不落盘或渲染。
- request/logical/attempt/upstream/response 关联 ID、账号/模型、协议元数据和非敏感底层错误不做无意义泛化或截断。
- 不记录每个 token/delta；只记录 accepted、headers、first event、first output、terminal、error/disconnect、completed/failed 等关键节点。
- 本任务不新增迁移、不修改生产数据、不产生真实上游流量；部署授权和 `downtime_required` 由根总控后续发布预检决定。

### Task 1: 流观测领域对象、错误分类与脱敏合同

**Files:**
- Create: `upstream/sub2api/backend/internal/service/stream_observability.go`
- Test: `upstream/sub2api/backend/internal/service/stream_observability_test.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_port.go`

**Interfaces:**
- Produces `StreamLifecycleStage`, `StreamErrorClass`, `StreamFailureStage`, `StreamObservation`, `NewStreamObservation`, `RecordEvent`, `RecordHeaders`, `RecordTerminal`, `RecordFailure`, `RecordClientDisconnect`, `Snapshot`, `ClassifyStreamError`, and `SanitizeStreamErrorChain`.
- `StreamObservation.Snapshot()` returns a JSON-marshalable struct containing the exact spec fields, omitting empty optional values and setting `correlation_degraded=true` when required correlation values are absent.
- `OpsUpstreamErrorEvent` gains a backward-compatible optional `StreamObservation` JSON field so existing `upstream_errors` storage carries the diagnostic payload without schema changes.

- [ ] **Step 1: Write failing unit tests for stage and error contracts**

```go
func TestClassifyStreamError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage string
		err   error
		want  StreamErrorClass
	}{
		{"unexpected eof", "upstream_body_read", io.ErrUnexpectedEOF, StreamErrorClassUpstreamEOF},
		{"client cancel", "client_write", context.Canceled, StreamErrorClassClientDisconnected},
		{"reset", "upstream_body_read", syscall.ECONNRESET, StreamErrorClassUpstreamConnectionReset},
		{"malformed", "sse_decode", ErrMalformedSSE, StreamErrorClassUpstreamSSEMalformed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyStreamError(tc.stage, tc.err, false)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSanitizeStreamErrorChainPreservesTransportButRemovesSecrets(t *testing.T) {
	got := SanitizeStreamErrorChain(errors.New("Bearer secret request_id=req-1 https://provider.invalid/path?api_key=hidden: unexpected EOF"))
	require.NotContains(t, got, "secret")
	require.NotContains(t, got, "api_key=hidden")
	require.Contains(t, got, "unexpected EOF")
}
```

- [ ] **Step 2: Run the focused tests and verify the expected RED failure**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'Test(ClassifyStreamError|SanitizeStreamErrorChain)' -count=1`

Expected: FAIL because the new observation types and classifier do not exist yet.

- [ ] **Step 3: Implement the minimal observation state and classifiers**

Implement the fixed stage/class enums, bounded string normalization, secret/credential redaction using the existing `util/logredact` helpers, event index/last event/byte counters, response and terminal flags, and JSON snapshot serialization. Do not log event payloads.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'Test(ClassifyStreamError|SanitizeStreamErrorChain|StreamObservation)' -count=1`

Expected: PASS with no warnings.

- [ ] **Step 5: Add edge-case tests for correlation degradation and root-cause evidence**

Cover missing logical/attempt IDs, `response.completed` plus downstream write failure, `response.failed`, proxy/DNS markers, and evidence-insufficient output. Assert that only the allowed root-cause values are produced.

- [ ] **Step 6: Run the expanded focused tests**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'Test(StreamObservation|ClassifyStreamError|SanitizeStreamErrorChain|RootCause)' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit the domain contract**

```bash
git add upstream/sub2api/backend/internal/service/stream_observability.go \
  upstream/sub2api/backend/internal/service/stream_observability_test.go \
  upstream/sub2api/backend/internal/service/ops_port.go
git commit -m "feat: add stream observability contracts"
```

### Task 2: 请求上下文与生命周期关联 ID

**Files:**
- Modify: `upstream/sub2api/backend/internal/pkg/ctxkey/ctxkey.go`
- Modify: `upstream/sub2api/backend/internal/server/middleware/client_request_id.go`
- Modify: `upstream/sub2api/backend/internal/server/middleware/request_logger.go`
- Modify: `upstream/sub2api/backend/internal/server/middleware/wire.go`
- Test: `upstream/sub2api/backend/internal/server/middleware/stream_correlation_test.go`

**Interfaces:**
- Adds typed context keys/accessors for `thread_id`, `window_id`, `session_id`, `logical_request_id`, `attempt_id`, `upstream_request_id`, and `response_id`.
- Middleware reads `X-Client-Request-Id`, `X-Codex-Window-Id`, `X-Thread-Id`/`X-Codex-Thread-Id`, and `X-Session-Id` with existing bounded UTF-8 semantics; missing optional headers remain empty, while server request ID remains generated as today.
- Every stream observation can obtain `request_id`, client IDs, environment metadata, and correlation degradation from the request context without using time-based matching.

- [ ] **Step 1: Write failing middleware tests**

```go
func TestStreamCorrelationMiddlewarePreservesBoundedClientMetadata(t *testing.T) {
	r := gin.New()
	r.Use(ClientRequestID(), RequestLogger())
	r.GET("/responses", func(c *gin.Context) {
		require.NotEmpty(t, c.Request.Context().Value(ctxkey.RequestID))
		require.Equal(t, "window-1", c.Request.Context().Value(ctxkey.WindowID))
		require.Equal(t, "thread-1", c.Request.Context().Value(ctxkey.ThreadID))
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/responses", nil)
	req.Header.Set("X-Codex-Window-Id", "window-1")
	req.Header.Set("X-Codex-Thread-Id", "thread-1")
	r.ServeHTTP(httptest.NewRecorder(), req)
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `cd upstream/sub2api/backend && go test ./internal/server/middleware -run TestStreamCorrelationMiddlewarePreservesBoundedClientMetadata -count=1`

Expected: FAIL because the context keys are not populated.

- [ ] **Step 3: Implement context extraction and logger field propagation**

Add exact typed keys, normalize each optional header with `NormalizeCorrelationID`, set context values before `RequestLogger` creates the request-scoped logger, and include non-empty values in structured request logs. Do not accept or log Authorization, Cookie, or request bodies.

- [ ] **Step 4: Run middleware tests and verify GREEN**

Run: `cd upstream/sub2api/backend && go test ./internal/server/middleware -run 'Test(StreamCorrelation|ClientRequestID|RequestLogger)' -count=1`

Expected: PASS.

- [ ] **Step 5: Add tests for invalid UTF-8, overlong IDs, and missing optional values**

Assert invalid/overlong optional headers are omitted or marked degraded, server request ID is still valid, and no sensitive header value appears in captured logger fields.

- [ ] **Step 6: Commit the correlation layer**

```bash
git add upstream/sub2api/backend/internal/pkg/ctxkey/ctxkey.go \
  upstream/sub2api/backend/internal/server/middleware/client_request_id.go \
  upstream/sub2api/backend/internal/server/middleware/request_logger.go \
  upstream/sub2api/backend/internal/server/middleware/wire.go \
  upstream/sub2api/backend/internal/server/middleware/stream_correlation_test.go
git commit -m "feat: propagate stream correlation metadata"
```

### Task 3: OpenAI Responses/Chat/Anthropic streaming instrumentation

**Files:**
- Create: `upstream/sub2api/backend/internal/service/stream_observability_runtime.go`
- Test: `upstream/sub2api/backend/internal/service/stream_observability_runtime_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_chat_completions.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_chat_completions_raw.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_messages.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_chat_completions_anthropic_native.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/stream_error_event.go`

**Interfaces:**
- `stream_observability_runtime.go` provides `BeginStreamObservation`, `ObserveUpstreamHeaders`, `ObserveSSEEvent`, `ObserveVisibleOutput`, `ObserveTerminal`, `ObserveReadFailure`, `ObserveClientWriteFailure`, and `FinishStreamObservation`.
- Existing stream handlers call these hooks at response headers, parsed SSE event, first semantic output, terminal event, scanner/read failure, downstream write failure, and deferred completion points.
- Hook failures are swallowed and surfaced only through a bounded `diagnostic_write_failed` logger/metric; they never change response, usage, or failover control flow.

- [ ] **Step 1: Write failing runtime instrumentation tests**

Use existing `httpUpstreamRecorder`, `httptest.ResponseRecorder`, and SSE fixtures to assert normal completion emits headers/first-event/first-output/terminal/completed snapshots; EOF before terminal emits `upstream_eof`; malformed JSON emits `upstream_sse_malformed`; canceled downstream writes emit `client_disconnected`.

- [ ] **Step 2: Run the runtime tests and verify RED**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'TestStreamObservabilityRuntime' -count=1`

Expected: FAIL because runtime hooks are not connected.

- [ ] **Step 3: Implement the runtime recorder and wire the minimal hooks**

Create one request-scoped observer per stream. Count bytes read/forwarded, preserve last event type/index, parse only event type and terminal usage/response IDs, detect semantic output from existing response-event helpers, classify read/write errors by direction and stage, and defer a final status event. Reuse existing `MarkOpsStreamFailure`/Ops context data for error recording; do not introduce retry or failover decisions.

- [ ] **Step 4: Run the runtime tests and verify GREEN**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'Test(StreamObservabilityRuntime|OpenAIGateway.*Streaming|OpenAI.*SSE)' -count=1`

Expected: PASS for the new tests and the directly touched stream suites.

- [ ] **Step 5: Add regression tests for terminal-vs-edge interruption**

Cover valid `response.completed` followed by downstream write failure, upstream `response.failed`, EOF with and without prior output, and `client_disconnected=true`. Assert root-cause remains `insufficient_evidence` unless the evidence contract is satisfied.

- [ ] **Step 6: Run formatting and focused service validation**

Run: `gofmt -w internal/service/stream_observability*.go internal/service/openai_gateway_chat_completions*.go internal/service/openai_gateway_messages.go internal/handler/stream_error_event.go internal/handler/openai_gateway_handler.go && go test ./internal/service ./internal/handler -run 'Test(StreamObservability|OpenAIGateway.*Stream|OpenAI.*SSE)' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit streaming instrumentation**

```bash
git add upstream/sub2api/backend/internal/service/stream_observability_runtime.go \
  upstream/sub2api/backend/internal/service/stream_observability_runtime_test.go \
  upstream/sub2api/backend/internal/service/openai_gateway_chat_completions.go \
  upstream/sub2api/backend/internal/service/openai_gateway_chat_completions_raw.go \
  upstream/sub2api/backend/internal/service/openai_gateway_messages.go \
  upstream/sub2api/backend/internal/service/openai_gateway_chat_completions_anthropic_native.go \
  upstream/sub2api/backend/internal/handler/openai_gateway_handler.go \
  upstream/sub2api/backend/internal/handler/stream_error_event.go
git commit -m "feat: instrument native streaming lifecycle"
```

### Task 4: Existing Ops error storage and read-only diagnostic query

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/ops_port.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_user_error.go`
- Modify: `upstream/sub2api/backend/internal/repository/ops_repo.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/ops_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/usage_handler.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go`
- Test: `upstream/sub2api/backend/internal/repository/ops_repo_stream_diagnostic_test.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/ops_stream_diagnostic_test.go`

**Interfaces:**
- Extend existing `OpsErrorLogDetail` with optional `stream_diagnostic` decoded from the existing sanitized `upstream_errors` JSON payload.
- Add read-only `GET /api/v1/admin/ops/stream-diagnostics?request_id=...` and `...?logical_request_id=...`; it returns the spec response shape with `entry`, `attempts`, `final`, and `evidence_missing`, never triggers retry, failover, account changes, or billing changes.
- Usage detail may link the diagnostic query using its existing `request_id`; if no matching Ops evidence exists, return `insufficient_evidence` and `evidence_missing` rather than infer by time.

- [ ] **Step 1: Write failing repository and handler contract tests**

Add sqlmock tests proving the repository reads exact request/logical IDs from existing Ops rows, rejects time-neighbor fallback, decodes sanitized stream snapshots, and returns `correlation_degraded` for missing fields. Add handler tests for 400 when neither query is supplied, 200 for evidence, and 200 with `insufficient_evidence` for no evidence.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `cd upstream/sub2api/backend && go test ./internal/repository ./internal/handler/admin -run 'Test.*StreamDiagnostic' -count=1`

Expected: FAIL because the diagnostic projection and route do not exist.

- [ ] **Step 3: Implement storage decoding and read-only service/handler projection**

Persist the observer snapshot through the existing sanitized `OpsUpstreamErrorEvent` JSON path when the request is already being recorded as an Ops error. Query exact request/logical identifiers only; merge attempts by explicit IDs, preserve environment/slot/container metadata, and compute root cause only from the spec evidence matrix.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `cd upstream/sub2api/backend && go test ./internal/repository ./internal/service ./internal/handler/admin -run 'Test.*(StreamDiagnostic|Ops.*Error)' -count=1`

Expected: PASS.

- [ ] **Step 5: Add no-side-effect and redaction regressions**

Assert the endpoint never invokes retry/failover/account update/billing methods, never returns credentials/body/SSE data, and preserves non-sensitive errors such as `unexpected EOF`, `connection reset by peer`, and proxy/DNS errors.

- [ ] **Step 6: Commit diagnostic query support**

```bash
git add upstream/sub2api/backend/internal/service/ops_port.go \
  upstream/sub2api/backend/internal/service/ops_user_error.go \
  upstream/sub2api/backend/internal/repository/ops_repo.go \
  upstream/sub2api/backend/internal/repository/ops_repo_stream_diagnostic_test.go \
  upstream/sub2api/backend/internal/handler/admin/ops_handler.go \
  upstream/sub2api/backend/internal/handler/admin/usage_handler.go \
  upstream/sub2api/backend/internal/handler/admin/ops_stream_diagnostic_test.go \
  upstream/sub2api/backend/internal/server/routes/admin.go
git commit -m "feat: expose read-only stream diagnostics"
```

### Task 5: Admin Usage detail presentation

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/usage.ts`
- Modify: `upstream/sub2api/frontend/src/types/index.ts`
- Modify: `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue`
- Modify: `upstream/sub2api/frontend/src/components/usage/__tests__/UsageDetailDialog.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/ops.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/ops.ts`

**Interfaces:**
- Adds typed `StreamDiagnosticResponse` and `adminUsageAPI.getStreamDiagnostic(requestId|logicalRequestId)`.
- Admin-only Usage detail renders a collapsible “流式链路诊断” section with environment/domain/protocol/slot/commit/container, IDs, lifecycle timings, byte counters, last event, terminal/output/usage flags, sanitized error chain, classification, failure stage, client disconnect, failover result, root cause and missing evidence.
- User Usage detail remains unchanged and receives no upstream/account/container/error-chain fields.

- [ ] **Step 1: Write failing component/API tests**

Assert admin scope requests the diagnostic endpoint using the usage record request ID, renders complete and insufficient-evidence states, and never renders Authorization, Cookie, token, request body, response body, or raw SSE content. Assert user scope does not call the endpoint.

- [ ] **Step 2: Run Vitest and verify RED**

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/components/usage/__tests__/UsageDetailDialog.spec.ts`

Expected: FAIL because the API method and diagnostic section do not exist.

- [ ] **Step 3: Implement typed API and compact read-only UI**

Load diagnostics only for admin details, keep loading/error/empty states local to the section, use existing components and i18n, and render long IDs/errors with wrapping and bounded scroll. Do not add a new page or persistent explanatory panel.

- [ ] **Step 4: Run focused Vitest and verify GREEN**

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/components/usage/__tests__/UsageDetailDialog.spec.ts src/api/__tests__/admin.usage.spec.ts`

Expected: PASS.

- [ ] **Step 5: Run frontend typecheck/build and diff check**

Run: `cd upstream/sub2api/frontend && pnpm typecheck && pnpm build`; then `git diff --check` from the worktree root.

Expected: PASS.

- [ ] **Step 6: Commit the admin presentation**

```bash
git add upstream/sub2api/frontend/src/api/admin/usage.ts \
  upstream/sub2api/frontend/src/types/index.ts \
  upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue \
  upstream/sub2api/frontend/src/components/usage/__tests__/UsageDetailDialog.spec.ts \
  upstream/sub2api/frontend/src/i18n/locales/zh/admin/ops.ts \
  upstream/sub2api/frontend/src/i18n/locales/en/admin/ops.ts
git commit -m "feat: show stream diagnostics in admin usage detail"
```

### Task 6: Caddy, Compose, environment identity and release contracts

**Files:**
- Modify: `infra/Caddyfile`
- Modify: `infra/Caddyfile.acceptance`
- Modify: `infra/compose.yaml`
- Modify: `infra/compose.acceptance.yaml`
- Modify: `ops/deploy-sub2api-blue-green-host.sh`
- Test: `tests/operations/stream_observability_caddy_contract_test.sh`
- Test: `tests/admin_lab/stream_observability_acceptance_contract_test.sh`

**Interfaces:**
- Caddy JSON access logs include route/host/protocol/TLS, allowed correlation headers, active upstream/slot, upstream `X-Request-ID`, status, request/response bytes, content metadata and duration, while explicitly excluding auth/cookie headers.
- API/worker/Caddy receive `SUB2API_ENVIRONMENT`, `SUB2API_DEPLOYMENT_COMMIT`, `SUB2API_CONTAINER_SLOT`, and a runtime container identity source; acceptance values are `acceptance` and `acceptance`, never merged with production values.
- Blue-green release keeps old-slot Docker logs for at least five rotated files/24 hours and records the identity fields needed for post-cutover lookup; no new deployment path is introduced.

- [ ] **Step 1: Write failing shell contract tests**

Assert both Caddy configs use JSON logs, retain the allowed correlation headers, omit Authorization/Cookie logging, pass environment/commit/slot metadata to services, and preserve existing `json-file` `20m/5` retention. Assert release scripts do not remove old-slot logs before the retention window.

- [ ] **Step 2: Run contract tests and verify RED**

Run: `bash tests/operations/stream_observability_caddy_contract_test.sh; bash tests/admin_lab/stream_observability_acceptance_contract_test.sh`

Expected: FAIL on missing metadata/log field contracts.

- [ ] **Step 3: Implement minimal Caddy/Compose/release metadata changes**

Use Caddy access-log field selection and existing environment substitutions; keep request bodies and sensitive headers out of logs. Propagate commit/slot/environment from the existing release environment and active-slot state. Do not alter routing, TLS, timeouts, health behavior, or deployment authorization.

- [ ] **Step 4: Run contracts and syntax checks**

Run: `bash tests/operations/stream_observability_caddy_contract_test.sh && bash tests/admin_lab/stream_observability_acceptance_contract_test.sh && bash -n ops/deploy-sub2api-blue-green-host.sh && git diff --check`

Expected: PASS.

- [ ] **Step 5: Commit infrastructure contracts**

```bash
git add infra/Caddyfile infra/Caddyfile.acceptance infra/compose.yaml \
  infra/compose.acceptance.yaml ops/deploy-sub2api-blue-green-host.sh \
  tests/operations/stream_observability_caddy_contract_test.sh \
  tests/admin_lab/stream_observability_acceptance_contract_test.sh
git commit -m "feat: preserve stream correlation across blue green logs"
```

### Task 7: Integrated direct verification and handoff

**Files:**
- Modify: `docs/superpowers/specs/2026-09-02-stream-observability-root-cause-diagnosis-design.md` only for implementation status/approval record if needed.
- Create: `docs/handoffs/2026-09-02-t113-stream-observability-handoff.md`

- [ ] **Step 1: Run the complete direct backend test set**

Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/handler ./internal/handler/admin ./internal/repository ./internal/server/middleware -run 'Test(StreamObservability|ClassifyStreamError|SanitizeStreamErrorChain|.*StreamDiagnostic|.*Correlation|OpenAIGateway.*Stream|OpenAI.*SSE)' -count=1`

Expected: PASS; unrelated full-package failures are not expanded into this task.

- [ ] **Step 2: Run required build/format checks**

Run: `gofmt -w` on touched Go files, `go test ./cmd/server`, `cd upstream/sub2api/frontend && pnpm typecheck && pnpm build`, and `git diff --check`.

Expected: PASS with no migration files changed.

- [ ] **Step 3: Run native-only and sensitive-field scans**

Run the existing native-only guard plus focused scans proving touched code/config/tests contain no Authorization values, API keys, cookies, tokens, request bodies, response bodies, or full SSE data persistence.

Expected: PASS; any hit must be a test fixture explicitly marked as a non-secret placeholder and reviewed.

- [ ] **Step 4: Write the handoff**

Record task ID T113, baseline `main@5bff30023`, final candidate commit, changed files, focused test commands/results, build/typecheck results, no migration/config data changes, `downtime_required=unverified until root preflight`, rollback to previous blue-green slot/image, no production/acceptance deployment, and residual risk that Caddy/Cloudflare live interruption requires runtime evidence.

- [ ] **Step 5: Final local review**

Run `git status --short --branch`, `git diff --stat main...HEAD`, and `git diff --check`; confirm only T113 files changed and the worktree is clean apart from any intentionally retained evidence.

- [ ] **Step 6: Report `READY_FOR_ROOT_REVIEW`**

Do not merge, push, deploy, or modify global queue/progress files from the candidate worktree. Report the candidate SHA and handoff path to the root release controller.

