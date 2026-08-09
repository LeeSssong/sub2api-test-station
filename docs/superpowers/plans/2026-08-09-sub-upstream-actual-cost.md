# Sub Upstream Actual Cost Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show one administrator request's real profit by automatically subtracting the matched upstream Sub `actual_cost` from the site's local `actual_cost`, without relay-ops or additional credentials.

**Architecture:** Complete the Sub-to-Sub request-ID contract first, then add an administrator-only backend service that reads the local usage/account, calls the upstream native usage ledger with the account's stored API Key, matches one exact record, and calculates profit. The frontend consumes that local endpoint and reduces the detail to the three confirmed amounts.

**Tech Stack:** Go 1.x, Gin, native `net/http`, Testify, Vue 3, TypeScript, Vitest.

## Global Constraints

- The only formula is `profit = site actual_cost - upstream Sub actual_cost`.
- Reuse the request account's existing `credentials.api_key` and `base_url`; add no billing credential, account mapping, relay-ops dependency, or five-dataset prerequisite.
- Never substitute standard cost, account cost, multiplier-derived cost, price-table estimates, or allocations for upstream `actual_cost`.
- Never expose or log the stored API Key. Keep this API administrator-only and leave user DTOs/endpoints unchanged.
- Match exact request identifiers inside a bounded time window and bounded pagination; never guess by model, tokens, or amount.
- Do not backfill historical rows or alter customer billing, prices, multipliers, quotas, routing, or aggregate reports.
- Follow strict TDD: add each behavior test, run it and confirm the expected red failure, then write the minimal production change and rerun green.
- Do not mark the project ledger complete until the branch is pushed, deployed, and verified online; this implementation plan itself does not authorize production deployment.

---

### Task 1: Complete The Sub-to-Sub Request-ID Contract

**Files:**

- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_usage.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_record_usage_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/gateway_handler.go`
- Create or modify: focused gateway usage-record handler test under `upstream/sub2api/backend/internal/handler/`

**Interfaces:**

- Consumes: `OpenAIForwardResult.RequestID`, which is populated from upstream HTTP `x-request-id` in the non-WS OpenAI forwarding path.
- Produces: `UsageLog.UpstreamRequestID = optionalTrimmedStringPtr(result.RequestID)` and native `UsageRecord.UpstreamRequestID` JSON mapping.

- [ ] **Step 1: Add the failing OpenAI persistence test.** Extend the existing `RecordUsage` fixture with `RequestID: "upstream-openai-456"`; assert both the stable local `RequestID == "client:openai-client-stable-123"` and `require.Equal(t, "upstream-openai-456", *usageRepo.lastLog.UpstreamRequestID)`. Run:

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/service -run 'TestOpenAIGatewayServiceRecordUsage_PrefersClientRequestIDOverUpstreamRequestID' -count=1
  ```

  Expected red failure: `UpstreamRequestID` is nil.

- [ ] **Step 2: Persist the upstream ID minimally.** In the `UsageLog` literal in `RecordUsage`, add:

  ```go
  UpstreamRequestID: optionalTrimmedStringPtr(result.RequestID),
  ```

  Keep the existing local `requestID` resolution and WS special case unchanged. Rerun Step 1 and require green.

- [ ] **Step 3: Add the failing native usage-record projection test.** Authenticate a gateway request with an API key, return a usage row containing `UpstreamRequestID: ptr("provider-req-789")`, call `/v1/usage/records`, and assert the JSON item contains exactly `"upstream_request_id":"provider-req-789"` along with its existing `actual_cost`. Run the focused handler test and confirm the field is absent before implementation.

- [ ] **Step 4: Map the declared field.** Change the `UsageRecord` construction to include:

  ```go
  UpstreamRequestID: valueOrEmpty(log.UpstreamRequestID),
  ```

  Use an existing pointer helper if available; otherwise add a tiny local nil-safe helper. Preserve API-key scoping and all pagination/time filters. Rerun the focused test and:

  ```bash
  go test ./internal/service ./internal/handler -run 'UpstreamRequestID|UsageRecords|PrefersClientRequestID' -count=1
  ```

- [ ] **Step 5: Self-review, update the project ledger with red/green evidence, and commit only Task 1.**

### Task 2: Add Automatic Upstream Sub Cost And Profit Lookup

**Files:**

- Create: `upstream/sub2api/backend/internal/service/sub_upstream_cost.go`
- Create: `upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/usage_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/usage_handler_detail_test.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go`
- Regenerate: `upstream/sub2api/backend/cmd/server/wire_gen.go`
- Modify constructor call sites/tests only as required by the added dependency.

**Interfaces:**

- Produces `service.SubUpstreamCostService.GetByUsageID(ctx context.Context, usageID int64) (*service.SubUpstreamCostDetail, error)`.
- `SubUpstreamCostDetail` JSON fields are:

  ```go
  type SubUpstreamCostDetail struct {
      UsageID             int64    `json:"usage_id"`
      LocalRequestID      string   `json:"local_request_id"`
      UpstreamRequestID   *string  `json:"upstream_request_id"`
      SiteActualCost      float64  `json:"site_actual_cost"`
      UpstreamActualCost  *float64 `json:"upstream_actual_cost"`
      Profit              *float64 `json:"profit"`
      Status              string   `json:"status"`
      Reason              string   `json:"reason,omitempty"`
  }
  ```

- Adds `GET /api/v1/admin/usage/:id/upstream-cost` returning that type through the standard success envelope.

- [ ] **Step 1: Write failing service tests with a real `httptest.Server`.** Use a real `UsageService` backed by a focused repository stub whose row contains an API-key account with `base_url`, `api_key`, local/upstream IDs and `ActualCost: 0.00688`. Verify the server receives `Authorization: Bearer stored-upstream-key`, the `/v1/usage/records` path, RFC3339 start/end timestamps and `limit=1000`; return a complete native response containing `actual_cost: 0.004`. Assert `Status == "confirmed"`, upstream cost `0.004`, and profit `0.00288`.

- [ ] **Step 2: Add failing matching and failure-state tests.** Cover all exact matches in this order: local upstream ID to remote upstream ID, local upstream ID to remote local ID, then local ID to remote local ID. Cover zero upstream cost as confirmed. Cover missing credentials, non-2xx response, malformed/oversized JSON, timeout, pagination beyond ten pages, and no exact match; each expected operational failure returns `Status == "unavailable"`, nil upstream cost/profit and a stable reason without secret or raw response text. A missing local usage row remains an error that the handler maps to 404.

- [ ] **Step 3: Run the new service tests and confirm red failures.**

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/service -run 'TestSubUpstreamCost' -count=1
  ```

- [ ] **Step 4: Implement the minimal bounded client.** Build the native URL with `net/url`: append `/usage/records` when `base_url` already ends in `/v1`, otherwise append `/v1/usage/records`; reject non-HTTP(S) URLs. Query `created_at ± 10m`, page with `next_cursor`, cap at 10 pages, cap response bodies, and use an HTTP client timeout. Parse the native envelope:

  ```go
  type upstreamUsageRecordsResponse struct {
      Data []struct {
          RequestID         string  `json:"request_id"`
          UpstreamRequestID string  `json:"upstream_request_id"`
          ActualCost        float64 `json:"actual_cost"`
      } `json:"data"`
      NextCursor string `json:"next_cursor"`
      HasMore    bool   `json:"has_more"`
  }
  ```

  On an exact match allocate `upstreamActualCost` and `profit := local.ActualCost - upstreamActualCost`; never infer either value on failure.

- [ ] **Step 5: Add the failing admin handler contract test.** Request `/admin/usage/42/upstream-cost`; assert the standard envelope contains usage/local/upstream IDs, numeric site/upstream costs, numeric profit and `status: "confirmed"`. Add invalid-ID and local-not-found cases. Confirm red before adding the route and handler.

- [ ] **Step 6: Wire the endpoint.** Add the service to `service.ProviderSet`, inject it into the admin `UsageHandler`, register `usage.GET("/:id/upstream-cost", ...)` before the generic `/:id` route, update constructor call sites with `nil` where unrelated, and regenerate Wire:

  ```bash
  cd upstream/sub2api/backend
  make -f ../deploy/Makefile wire
  ```

  Rerun service and handler tests, then `go test ./internal/service ./internal/handler/admin ./internal/server -count=1`.

- [ ] **Step 7: Self-review for credential leakage, update the project ledger, and commit only Task 2.**

### Task 3: Simplify The Administrator Detail UI

**Files:**

- Modify: `upstream/sub2api/frontend/src/api/admin/usage.ts`
- Modify: `upstream/sub2api/frontend/src/types/index.ts`
- Modify: `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue`
- Modify: `upstream/sub2api/frontend/src/components/usage/usageDetail.ts`
- Modify: `upstream/sub2api/frontend/src/components/usage/__tests__/UsageDetailDialog.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/usage/__tests__/usageDetail.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`

**Interfaces:**

- Replaces relay-ops `getRequestCost({local_request_id})` with `getUpstreamCost(usageId: number)` calling `/admin/usage/${usageId}/upstream-cost`.
- Frontend `AdminUsageCostDetail` mirrors Task 2 exactly and treats only `status === "confirmed"` with finite numeric upstream cost/profit as displayable.

- [ ] **Step 1: Rewrite the pure-function tests first.** Remove every estimated/standard-cost expectation. Add literals proving confirmed zero cost is valid, confirmed `profit` is read directly, and unavailable/malformed results produce null upstream cost and null profit. Run:

  ```bash
  cd upstream/sub2api/frontend
  npm run test -- src/components/usage/__tests__/usageDetail.spec.ts --run
  ```

  Confirm red because the old evidence helpers still accept estimated values and calculate margin client-side.

- [ ] **Step 2: Add failing component/API behavior tests.** In admin scope assert `getUpstreamCost` is called with numeric usage ID `42`, not a local request ID. Assert the rendered labels are exactly the locale keys for site actual charge, upstream actual charge and profit; assert the administrator section contains no `siteRequestId`, `costSource`, `includedCost`, `estimatedGrossMargin` or `grossMarginStatus` label. Assert user scope never calls the cost endpoint and an unavailable result displays `-` without failing the main detail.

- [ ] **Step 3: Run the component test and confirm the expected red failures.**

  ```bash
  npm run test -- src/components/usage/__tests__/UsageDetailDialog.spec.ts --run
  ```

- [ ] **Step 4: Implement the simplified API/types/helpers.** Use the Task 2 DTO, remove `buildGatewayUrl` and relay-ops auth comments from this API module, and replace estimated evidence helpers with direct finite-value accessors. Do not calculate a different profit in the browser; display the backend `profit` only when confirmed.

- [ ] **Step 5: Simplify the component.** The top administrator summary must be exactly:

  ```text
  本站实际扣费 | 上游实际扣费 | 利润
  ```

  Remove the administrator-section duplicate site request ID and legacy cost-evidence rows. Keep upstream request ID and operational account/channel/upstream fields. Call `getUpstreamCost(id)` after the admin detail loads and retain the existing stale-response guards.

- [ ] **Step 6: Update Chinese and English labels.** Add `profit: '利润'` / `profit: 'Profit'` and an `unavailable` label if the component uses text rather than `-`; remove only translation keys proven unused by repository search.

- [ ] **Step 7: Verify frontend and integrated static contracts.** Run:

  ```bash
  cd upstream/sub2api/frontend
  npm run test -- src/components/usage/__tests__/usageDetail.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts --run
  npm run type-check
  npm run build
  ```

  Then from the repository worktree run `git diff --check` and search the changed frontend files to confirm there is no `/relay-ops/api/reconciliation/request-cost`, duplicate site request ID row, estimated upstream cost, or five-dataset prerequisite.

- [ ] **Step 8: Update the project ledger, self-review, and commit only Task 3.**

After all three task reviews are clean, run a whole-branch review and fresh backend/frontend verification. Keep the global project status “进行中” unless the reviewed branch is merged into `main`, pushed, deployed through the local/host blue-green chain, and verified online.
