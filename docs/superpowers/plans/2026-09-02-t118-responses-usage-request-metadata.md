# T118 Responses 流水请求元数据恢复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore Sub-native request metadata capture for new OpenAI Responses usage records without changing historical rows or billing behavior.

**Architecture:** Keep metadata capture in the Responses handler request goroutine, before the bounded async usage-record task is submitted. Add one unexported helper that applies the already-established native sources to an existing `OpenAIRecordUsageInput`; leave service, repository, frontend, schema, and migrations unchanged.

**Tech Stack:** Go, Gin, existing `ip.GetClientIP`, OpenAI gateway handler tests, `go test`, `go build`.

**Spec:** `docs/superpowers/specs/2026-09-02-t118-responses-usage-request-metadata-design.md`

## Global Constraints

- Only new records after deployment are repaired; historical `NULL` values are never backfilled.
- Use the existing Sub-native sources: `GetInboundEndpoint(c)`, `resolveOpenAIUpstreamEndpoint(c, account, result)`, `ip.GetClientIP(c)`, `c.GetHeader("User-Agent")`, `service.ExtractClientSessionID(c)`, and `service.HashUsageRequestPayload(body)`.
- Capture values before async submission; the worker must not access `gin.Context`.
- Preserve logical request, attempt, quota platform, pricing, channel, billing, scheduling, retry, account-state, and upstream-protocol behavior.
- Do not modify frontend code, DTOs, repository SQL, Ent schema, migrations, production data, configuration, or deployment authorization.

### Task 1: Restore Responses metadata snapshot and regression coverage

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go:1407-1414,3653-3659`
- Test: `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`

**Interfaces:**
- Consumes: existing `*gin.Context`, request body `[]byte`, `*service.Account`, `*service.OpenAIForwardResult`, and an existing `*service.OpenAIRecordUsageInput`.
- Produces: unexported `applyOpenAIUsageRequestMetadata(c *gin.Context, body []byte, account *service.Account, result *service.OpenAIForwardResult, input *service.OpenAIRecordUsageInput)` that mutates only the six request metadata fields on `input`; Responses success calls it before `submitOpenAIUsageRecordTask`.

- [ ] **Step 1: Write the failing unit test for native metadata application**

Add a Gin context with request path `/responses`, body `{"model":"gpt-5.6-sol"}`, and a trusted test client address, then assert the helper populates the input snapshot:

```go
func TestApplyOpenAIUsageRequestMetadataUsesNativeSources(t *testing.T) {

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{"model":"gpt-5.6-sol"}`))
	c.Request.RemoteAddr = "203.0.113.10:443"
	c.Set("_gateway_inbound_endpoint", EndpointResponses)

	input := &service.OpenAIRecordUsageInput{}
	account := &service.Account{Platform: service.PlatformOpenAI}
	result := &service.OpenAIForwardResult{Model: "gpt-5.6-sol"}
	applyOpenAIUsageRequestMetadata(c, []byte(`{"model":"gpt-5.6-sol"}`), account, result, input)

	require.Equal(t, "/v1/responses", input.InboundEndpoint)
	require.NotEmpty(t, input.UpstreamEndpoint)
	require.Equal(t, "203.0.113.10", input.IPAddress)
	require.Equal(t, "", input.UserAgent)
	require.Empty(t, input.SessionID)
	require.NotEmpty(t, input.RequestPayloadHash)
}
```

Use the existing handler test package imports and do not add production-only test dependencies.

- [ ] **Step 2: Run the focused test and verify RED**

Run from `upstream/sub2api/backend`:

```bash
go test ./internal/handler -run TestApplyOpenAIUsageRequestMetadataUsesNativeSources -count=1
```

Expected: FAIL because `applyOpenAIUsageRequestMetadata` is not yet defined.

- [ ] **Step 3: Implement the minimal native snapshot helper**

Add the helper beside the existing successful/failed input builders:

```go
func applyOpenAIUsageRequestMetadata(c *gin.Context, body []byte, account *service.Account, result *service.OpenAIForwardResult, input *service.OpenAIRecordUsageInput) {
	if input == nil {
		return
	}
	input.UserAgent = c.GetHeader("User-Agent")
	input.IPAddress = ip.GetClientIP(c)
	input.RequestPayloadHash = service.HashUsageRequestPayload(body)
	input.InboundEndpoint = GetInboundEndpoint(c)
	input.UpstreamEndpoint = resolveOpenAIUpstreamEndpoint(c, account, result)
	input.SessionID = service.ExtractClientSessionID(c)
}
```

In the Responses success branch, call the helper immediately after constructing `recordInput` and before assigning existing API key, quota, channel, pricing, and cyber fields. Do not call it from the service layer or from the async closure. Leave the Messages path unchanged except for using the same helper only if doing so is mechanically identical and does not alter its behavior; the minimum required change is the Responses path.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
go test ./internal/handler -run 'TestApplyOpenAIUsageRequestMetadataUsesNativeSources|TestBuildSuccessfulOpenAIUsageRecordInput' -count=1
```

Expected: PASS.

- [ ] **Step 5: Add a regression assertion for optional metadata and run it**

Extend the test with `User-Agent: codex-test/1.0`, `X-Session-Id: session-t118`, and assert exact propagation of `UserAgent`, `SessionID`, and a stable non-empty `RequestPayloadHash`. Add a second subtest with no optional headers and assert only those optional fields are empty while endpoint and IP extraction still do not panic or block.

Run:

```bash
go test ./internal/handler -run TestApplyOpenAIUsageRequestMetadataUsesNativeSources -count=1
```

Expected: PASS.

- [ ] **Step 6: Run direct verification gates**

Run:

```bash
gofmt -w internal/handler/openai_gateway_handler.go internal/handler/openai_gateway_handler_test.go
go test ./internal/handler -run 'TestApplyOpenAIUsageRequestMetadataUsesNativeSources|TestBuildSuccessfulOpenAIUsageRecordInput' -count=1
go build ./cmd/server
git diff --check
```

Expected: all commands exit 0. Confirm `git diff --name-only` contains only the two handler files and no migration/frontend/config changes.

- [ ] **Step 7: Commit the task-local implementation**

```bash
git add upstream/sub2api/backend/internal/handler/openai_gateway_handler.go \
  upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go
git commit -m "fix: restore Responses usage request metadata"
```

After the commit, report the candidate SHA, focused test output, build result, unverified items, and confirm no historical data or deployment action was performed. Candidate status becomes `READY_FOR_ROOT_REVIEW`; do not merge, push, or deploy without root authorization.
