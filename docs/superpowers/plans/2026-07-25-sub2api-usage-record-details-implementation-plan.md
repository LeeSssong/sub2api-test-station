# Sub2API Usage Record Details Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add explicit, secure request-detail actions for successful and failed records on both administrator and ordinary-user Sub2API usage pages.

**Architecture:** Successful records use one shared Vue dialog with an explicit `user` or `admin` scope. The user scope reuses the ownership-protected native endpoint, while the administrator scope adds one protected handler that reuses the same service query and administrator DTO. Error records keep their existing APIs and dialogs, with the user detail whitelist extended only for the owned request ID.

**Tech Stack:** Go 1.24, Gin, Sub2API service/DTO layers, Vue 3 Composition API, TypeScript 5.6, Tailwind CSS, Vitest, Vue Test Utils, vue-i18n.

## Global Constraints

- Reuse existing `usage_logs` persistence and `UsageService.GetByID`; add no migration, repository, or duplicate logging path.
- Ordinary users call only `GET /api/v1/usage/:id` for successful records and retain its user ownership check.
- Administrators call `GET /api/v1/admin/usage/:id` through the existing protected administrator route group.
- Ordinary-user responses must not contain account, channel, upstream endpoint/model, model mapping, billing tier, account multiplier, account statistics cost, or upstream request identifiers.
- Extend the ordinary-user error-detail projection with only the owned request ID; do not add it to the paginated error list.
- Do not capture or expose successful request bodies, generated responses, credentials, authorization headers, cookies, raw headers, account secrets, or Base URLs.
- Use existing dependencies, `BaseDialog`, `Icon`, formatters, clipboard feedback, table components, light/dark theme tokens, and responsive conventions.
- Keep the detail column always visible at the right edge on both successful-record pages.
- Preserve unrelated dirty-worktree changes, especially the account-monitor edits already present in `backend/internal/server/routes/admin.go`.

## File Map

### Backend

- Modify `upstream/sub2api/backend/internal/handler/admin/usage_handler.go`: add administrator successful-record lookup.
- Create `upstream/sub2api/backend/internal/handler/admin/usage_handler_detail_test.go`: handler validation and admin DTO coverage.
- Modify `upstream/sub2api/backend/internal/server/routes/admin.go`: register `GET /admin/usage/:id` after static usage routes.
- Modify `upstream/sub2api/backend/internal/service/ops_user_error.go`: add request ID only to the owned user error-detail projection.
- Modify `upstream/sub2api/backend/internal/service/ops_user_error_test.go`: verify request-ID selection and redaction boundary.

### Frontend contracts

- Modify `upstream/sub2api/frontend/src/api/admin/usage.ts`: add `adminUsageAPI.getById`.
- Create `upstream/sub2api/frontend/src/api/__tests__/admin.usage.spec.ts`: verify the administrator detail URL.
- Modify `upstream/sub2api/frontend/src/types/index.ts`: add `request_id` only to `UserErrorRequestDetail`.
- Modify `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`: add Chinese detail labels and states.
- Modify `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`: add matching English detail labels and states.

### Frontend successful-record detail

- Create `upstream/sub2api/frontend/src/components/usage/usageDetail.ts`: pure cost, token-price, request-type, and admin-cost projections for the dialog.
- Create `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue`: shared successful-record dialog and scope-aware fetch lifecycle.
- Create `upstream/sub2api/frontend/src/components/usage/__tests__/usageDetail.spec.ts`: pure projection tests.
- Create `upstream/sub2api/frontend/src/components/usage/__tests__/UsageDetailDialog.spec.ts`: endpoint selection, visibility, state, and stale-response tests.
- Modify `upstream/sub2api/frontend/src/components/admin/usage/UsageTable.vue`: render the detail action and emit `detailClick`.
- Modify `upstream/sub2api/frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`: verify the emitted ID and accessible action.
- Modify `upstream/sub2api/frontend/src/views/admin/UsageView.vue`: always-visible detail column and admin dialog state.
- Modify `upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts`: verify administrator event wiring and scope.
- Modify `upstream/sub2api/frontend/src/views/user/UsageView.vue`: always-visible detail column and user dialog state.
- Modify `upstream/sub2api/frontend/src/views/user/__tests__/UsageView.spec.ts`: verify user event wiring and scope.

### Frontend error detail

- Modify `upstream/sub2api/frontend/src/components/user/UserErrorRequestsTable.vue`: add the always-visible explicit detail action while retaining row click.
- Create `upstream/sub2api/frontend/src/components/user/__tests__/UserErrorRequestsTable.spec.ts`: verify action and row-click behavior.
- Modify `upstream/sub2api/frontend/src/components/user/UserErrorDetailModal.vue`: render and copy the owned request ID.
- Create `upstream/sub2api/frontend/src/components/user/__tests__/UserErrorDetailModal.spec.ts`: verify request-ID rendering, copy, and load failure.
- Verify existing `upstream/sub2api/frontend/src/views/admin/ops/components/OpsErrorLogTable.vue`: its `actions` column already emits `openErrorDetail`; do not duplicate it.

---

### Task 1: Administrator successful-record detail endpoint

**Files:**
- Create: `upstream/sub2api/backend/internal/handler/admin/usage_handler_detail_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/usage_handler.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go:655-666`

**Interfaces:**
- Consumes: `(*service.UsageService).GetByID(ctx, id int64) (*service.UsageLog, error)`.
- Produces: `(*admin.UsageHandler).GetByID(c *gin.Context)` and `GET /api/v1/admin/usage/:id`, returning `dto.AdminUsageLog`.

- [ ] **Step 1: Write the failing handler tests**

Create a repository capture by embedding the existing interface so only `GetByID` must be implemented:

```go
type adminUsageDetailRepo struct {
	service.UsageLogRepository
	record *service.UsageLog
	err    error
	gotID  int64
}

func (r *adminUsageDetailRepo) GetByID(_ context.Context, id int64) (*service.UsageLog, error) {
	r.gotID = id
	return r.record, r.err
}
```

Register `router.GET("/admin/usage/:id", handler.GetByID)` in the test. Cover:

```go
func TestAdminUsageGetByIDReturnsAdminProjection(t *testing.T)
func TestAdminUsageGetByIDRejectsInvalidID(t *testing.T)
func TestAdminUsageGetByIDRejectsNonPositiveID(t *testing.T)
func TestAdminUsageGetByIDPreservesNotFound(t *testing.T)
```

The success fixture must include `RequestID`, `InboundEndpoint`, `UpstreamEndpoint`, `UpstreamModel`, `ChannelID`, `BillingTier`, `AccountRateMultiplier`, and a shallow `Account`. Assert the JSON response includes these administrator fields and `repo.gotID == 42`.

- [ ] **Step 2: Run the focused backend test and confirm RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/handler/admin -run 'TestAdminUsageGetByID' -count=1
```

Expected: build failure because `(*UsageHandler).GetByID` does not exist.

- [ ] **Step 3: Implement the minimal administrator handler**

Add:

```go
// GetByID handles fetching one usage record for an administrator.
// GET /api/v1/admin/usage/:id
func (h *UsageHandler) GetByID(c *gin.Context) {
	usageID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || usageID <= 0 {
		response.BadRequest(c, "Invalid usage ID")
		return
	}

	record, err := h.usageService.GetByID(c.Request.Context(), usageID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.UsageLogFromServiceAdmin(record))
}
```

Do not add authorization logic inside the handler; the existing admin route group owns authorization.

- [ ] **Step 4: Register the route without disturbing static routes**

In `registerUsageRoutes`, keep all current `/stats`, `/search-*`, and cleanup routes intact, including user changes already in the dirty file. Append the parameter route after static entries:

```go
usage.GET("/:id", h.Admin.Usage.GetByID)
```

- [ ] **Step 5: Run handler and package tests**

Run:

```bash
cd upstream/sub2api/backend
gofmt -w internal/handler/admin/usage_handler.go internal/handler/admin/usage_handler_detail_test.go
go test ./internal/handler/admin -run 'TestAdminUsage(GetByID|List|Stats)' -count=1
go test ./internal/server/routes ./internal/handler/admin -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 1 files only**

```bash
git add upstream/sub2api/backend/internal/handler/admin/usage_handler.go \
  upstream/sub2api/backend/internal/handler/admin/usage_handler_detail_test.go \
  upstream/sub2api/backend/internal/server/routes/admin.go
git commit -m "feat: add admin usage detail endpoint"
```

Before committing, inspect the staged diff for `admin.go` and confirm existing account-monitor route changes remain present.

---

### Task 2: Owned request ID in ordinary-user error detail

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/ops_user_error.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_user_error_test.go`

**Interfaces:**
- Consumes: owned `*service.OpsErrorLogDetail` after `OpsService.GetUserErrorRequestDetail` has enforced `userID`.
- Produces: `UserErrorRequestDetail.RequestID string` serialized as `request_id`; the base list type remains unchanged.

- [ ] **Step 1: Add failing whitelist tests**

Extend the existing mapper tests with:

```go
func TestToUserErrorRequestDetailSelectsOwnedRequestID(t *testing.T) {
	out := ToUserErrorRequestDetail(&OpsErrorLogDetail{
		OpsErrorLog: OpsErrorLog{
			RequestID:       " req-gateway ",
			ClientRequestID: "req-client",
		},
	})
	require.Equal(t, "req-gateway", out.RequestID)

	fallback := ToUserErrorRequestDetail(&OpsErrorLogDetail{
		OpsErrorLog: OpsErrorLog{ClientRequestID: " req-client "},
	})
	require.Equal(t, "req-client", fallback.RequestID)
}
```

Also marshal `UserErrorRequest{}` and `UserErrorRequestDetail{}` separately. Assert the list JSON does not contain `request_id`, while detail JSON does not contain `account_id`, `upstream_endpoint`, `upstream_model`, or `client_request_id`.

- [ ] **Step 2: Run the mapper tests and confirm RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestToUserErrorRequestDetail' -count=1
```

Expected: build or assertion failure because `RequestID` is absent.

- [ ] **Step 3: Implement the narrow projection**

Add only this field to the detail type:

```go
type UserErrorRequestDetail struct {
	UserErrorRequest
	RequestID          string `json:"request_id,omitempty"`
	ErrorBody          string `json:"error_body"`
	UpstreamStatusCode *int   `json:"upstream_status_code,omitempty"`
}
```

Select and trim the safe ID during mapping:

```go
requestID := strings.TrimSpace(e.RequestID)
if requestID == "" {
	requestID = strings.TrimSpace(e.ClientRequestID)
}
```

Set `RequestID: requestID` only in `ToUserErrorRequestDetail`. Do not change `ToUserErrorRequest`.

- [ ] **Step 4: Run user error and ownership tests**

Run:

```bash
cd upstream/sub2api/backend
gofmt -w internal/service/ops_user_error.go internal/service/ops_user_error_test.go
go test ./internal/service -run 'Test(ToUserErrorRequest|GetUserErrorRequestDetail)' -count=1
```

Expected: PASS, including existing cross-user ownership tests.

- [ ] **Step 5: Commit Task 2**

```bash
git add upstream/sub2api/backend/internal/service/ops_user_error.go \
  upstream/sub2api/backend/internal/service/ops_user_error_test.go
git commit -m "feat: expose owned request id in error detail"
```

---

### Task 3: Frontend API, types, and translations

**Files:**
- Create: `upstream/sub2api/frontend/src/api/__tests__/admin.usage.spec.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/usage.ts`
- Modify: `upstream/sub2api/frontend/src/types/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`

**Interfaces:**
- Produces: `adminUsageAPI.getById(id: number): Promise<AdminUsageLog>`.
- Produces: `UserErrorRequestDetail.request_id?: string`.
- Produces translation keys under `usage.detail.*` and `usage.errors.detail.requestId`.

- [ ] **Step 1: Write the failing administrator API test**

Mock `apiClient.get`, resolve `{ data: record }`, call `adminUsageAPI.getById(42)`, and assert:

```ts
expect(get).toHaveBeenCalledWith('/admin/usage/42')
await expect(result).resolves.toEqual(record)
```

- [ ] **Step 2: Run the API test and confirm RED**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/api/__tests__/admin.usage.spec.ts
```

Expected: failure because `getById` is not exported.

- [ ] **Step 3: Implement the API and type contract**

Add:

```ts
export async function getById(id: number): Promise<AdminUsageLog> {
  const { data } = await apiClient.get<AdminUsageLog>(`/admin/usage/${id}`)
  return data
}
```

Include `getById` in `adminUsageAPI`. Add only this field to the detail type:

```ts
export interface UserErrorRequestDetail extends UserErrorRequest {
  request_id?: string
  error_body: string
  upstream_status_code?: number
}
```

- [ ] **Step 4: Add exact translation contracts**

Under `usage.detail`, add matching Chinese and English keys for:

```ts
{
  action, title, consumption, requestInfo, requestId, requestTime,
  apiKey, group, model, requestType, responseTime, firstTokenTime,
  tokenSection, billingSection, adminSection, billingMode, billingType,
  inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens,
  inputCost, outputCost, cacheCreationCost, cacheReadCost,
  standardCost, actualCost, groupMultiplier, effectiveInputPrice,
  effectiveOutputPrice, account, channelId, upstreamEndpoint,
  upstreamModel, modelMappingChain, billingTier, accountMultiplier,
  accountCost, loading, loadFailed, retry, copied
}
```

Use `详情`, `日志详情`, `消耗`, `请求信息`, and `计费详情` for the primary Chinese labels. Add `requestId: '请求 ID'` under `usage.errors.detail`.

- [ ] **Step 5: Run API and locale compilation tests**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/api/__tests__/admin.usage.spec.ts \
  src/i18n/__tests__/localesMessageCompile.spec.ts
pnpm typecheck
```

Expected: PASS.

- [ ] **Step 6: Commit Task 3**

```bash
git add upstream/sub2api/frontend/src/api/__tests__/admin.usage.spec.ts \
  upstream/sub2api/frontend/src/api/admin/usage.ts \
  upstream/sub2api/frontend/src/types/index.ts \
  upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts \
  upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts
git commit -m "feat: add usage detail frontend contracts"
```

---

### Task 4: Shared successful-record detail dialog

**Files:**
- Create: `upstream/sub2api/frontend/src/components/usage/usageDetail.ts`
- Create: `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue`
- Create: `upstream/sub2api/frontend/src/components/usage/__tests__/usageDetail.spec.ts`
- Create: `upstream/sub2api/frontend/src/components/usage/__tests__/UsageDetailDialog.spec.ts`

**Interfaces:**
- Consumes: `usageAPI.getById`, `adminUsageAPI.getById`, `UsageLog`, and `AdminUsageLog`.
- Produces: `<UsageDetailDialog :show :usage-id scope @update:show>`.
- Produces pure helpers:

```ts
export type UsageDetailScope = 'user' | 'admin'
export function effectivePerMillion(cost: number | null | undefined, tokens: number | null | undefined): number | null
export function accountBilledCost(row: AdminUsageLog): number
export function hasAdminUsageFields(row: UsageLog | AdminUsageLog): row is AdminUsageLog
```

- [ ] **Step 1: Write failing pure-helper tests**

Cover zero-token division, normal effective price, missing admin fields, and account-cost fallback:

```ts
expect(effectivePerMillion(0.005, 1000)).toBe(5)
expect(effectivePerMillion(0.005, 0)).toBeNull()
expect(accountBilledCost({
  total_cost: 2,
  account_stats_cost: 3,
  account_rate_multiplier: 0.2,
} as AdminUsageLog)).toBeCloseTo(0.6)
```

- [ ] **Step 2: Run helper tests and confirm RED**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/components/usage/__tests__/usageDetail.spec.ts
```

Expected: module-not-found failure.

- [ ] **Step 3: Implement pure projections**

Treat non-finite, missing, negative-token, and zero-token inputs as no effective price. Use `(account_stats_cost ?? total_cost ?? 0) * (account_rate_multiplier ?? 1)` for administrator account cost.

- [ ] **Step 4: Write failing dialog behavior tests**

Mock both APIs and mount with a real or shallow `BaseDialog`. Cover:

1. `scope="user"` calls only `usageAPI.getById`.
2. `scope="admin"` calls only `adminUsageAPI.getById`.
3. User mode shows request ID, endpoint, key, group, model, latency, token, and user-billing fields.
4. User mode does not render account/channel/upstream/model-mapping/admin-cost labels even if a malicious fixture includes those properties.
5. Admin mode renders available administrator fields.
6. Loading clears the previous record.
7. A rejected request renders `usage.detail.loadFailed` and retry calls the same endpoint.
8. Closing emits `update:show=false` and ignores a late response.
9. Changing ID while open allows only the latest response to render.

- [ ] **Step 5: Run dialog tests and confirm RED**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/components/usage/__tests__/UsageDetailDialog.spec.ts
```

Expected: component-not-found failure.

- [ ] **Step 6: Implement the scope-aware fetch lifecycle**

Use a monotonic request sequence:

```ts
let requestSequence = 0

async function loadDetail() {
  const id = props.usageId
  if (!props.show || id == null || id <= 0) return
  const sequence = ++requestSequence
  loading.value = true
  loadError.value = false
  detail.value = null
  try {
    const result = props.scope === 'admin'
      ? await adminUsageAPI.getById(id)
      : await usageAPI.getById(id)
    if (sequence === requestSequence && props.show) detail.value = result
  } catch {
    if (sequence === requestSequence && props.show) loadError.value = true
  } finally {
    if (sequence === requestSequence) loading.value = false
  }
}
```

Increment `requestSequence` and clear state when closing. Watch `[show, usageId, scope]`.

- [ ] **Step 7: Implement the responsive dialog sections**

Use `BaseDialog width="wide"`. Use an unframed two-column definition grid on desktop and one column on mobile. Use bordered sections for Token and billing breakdowns without nesting cards. The request ID row uses a copy icon button with a tooltip/title and `useClipboard`.

Render:

- header status `usage.detail.consumption`;
- common request summary;
- token/image section using stored fields;
- billing section with six-decimal component costs and effective per-million prices only when calculable;
- administrator section only when `scope === 'admin'` and safe admin fields exist.

Use existing request-type, billing-mode, date, reasoning-effort, and duration formatters where available. Do not render raw JSON except the sanitized image-size breakdown.

- [ ] **Step 8: Run component tests and typecheck**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/components/usage/__tests__/usageDetail.spec.ts \
  src/components/usage/__tests__/UsageDetailDialog.spec.ts
pnpm typecheck
```

Expected: PASS.

- [ ] **Step 9: Commit Task 4**

```bash
git add upstream/sub2api/frontend/src/components/usage
git commit -m "feat: add shared usage detail dialog"
```

---

### Task 5: Successful-record detail actions on both usage pages

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/admin/usage/UsageTable.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/UsageView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/user/UsageView.vue`
- Modify: `upstream/sub2api/frontend/src/views/user/__tests__/UsageView.spec.ts`

**Interfaces:**
- Consumes: `UsageDetailDialog` from Task 4.
- Produces: `UsageTable` event `detailClick: [usageID: number]`.
- Produces: always-visible `detail` column on administrator and user successful-record tables.

- [ ] **Step 1: Write the failing UsageTable event test**

Update the local `DataTableStub` to render `cell-detail`, mount one row with `id: 42`, click the button, and assert:

```ts
expect(wrapper.emitted('detailClick')).toEqual([[42]])
expect(wrapper.get('[data-testid="usage-detail-action"]').attributes('title'))
  .toBe('Details')
```

- [ ] **Step 2: Run the table test and confirm RED**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/components/admin/usage/__tests__/UsageTable.spec.ts
```

Expected: no detail action or emitted event.

- [ ] **Step 3: Implement the table action**

Add:

```vue
<template #cell-detail="{ row }">
  <button
    type="button"
    data-testid="usage-detail-action"
    class="inline-flex items-center gap-1 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
    :title="t('usage.detail.action')"
    @click.stop="emit('detailClick', row.id)"
  >
    <Icon name="eye" size="sm" />
    {{ t('usage.detail.action') }}
  </button>
</template>
```

Add `detailClick: [usageID: number]` to `defineEmits`.

- [ ] **Step 4: Write failing view-wiring tests**

For each view, use a `UsageTable` stub that emits `detailClick` and a `UsageDetailDialog` stub exposing its props. Assert:

```ts
expect(dialog.props('show')).toBe(true)
expect(dialog.props('usageId')).toBe(42)
expect(dialog.props('scope')).toBe('user') // or 'admin'
```

Also assert the computed column order ends in `detail` and `detail` does not appear in the column-settings toggle list.

- [ ] **Step 5: Run both view tests and confirm RED**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/views/user/__tests__/UsageView.spec.ts \
  src/views/admin/__tests__/UsageView.spec.ts
```

Expected: missing dialog state and missing detail column.

- [ ] **Step 6: Wire the administrator view**

Import `UsageDetailDialog`. Add:

```ts
const showUsageDetail = ref(false)
const selectedUsageID = ref<number | null>(null)

function openUsageDetail(id: number) {
  selectedUsageID.value = id
  showUsageDetail.value = true
}
```

Append `{ key: 'detail', label: t('usage.detail.action'), sortable: false }`, add `detail` to `ALWAYS_VISIBLE`, pass `@detailClick="openUsageDetail"`, and render:

```vue
<UsageDetailDialog
  v-model:show="showUsageDetail"
  :usage-id="selectedUsageID"
  scope="admin"
/>
```

- [ ] **Step 7: Wire the ordinary-user view**

Apply the same state and column contract with `scope="user"`. Keep `show-account-billing="false"` and `show-upstream-endpoint="false"` unchanged.

- [ ] **Step 8: Run focused table/view tests and typecheck**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/components/admin/usage/__tests__/UsageTable.spec.ts \
  src/views/user/__tests__/UsageView.spec.ts \
  src/views/admin/__tests__/UsageView.spec.ts
pnpm typecheck
```

Expected: PASS.

- [ ] **Step 9: Commit Task 5**

```bash
git add upstream/sub2api/frontend/src/components/admin/usage/UsageTable.vue \
  upstream/sub2api/frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts \
  upstream/sub2api/frontend/src/views/admin/UsageView.vue \
  upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts \
  upstream/sub2api/frontend/src/views/user/UsageView.vue \
  upstream/sub2api/frontend/src/views/user/__tests__/UsageView.spec.ts
git commit -m "feat: open usage details from record tables"
```

---

### Task 6: Explicit ordinary-user error detail action and request ID

**Files:**
- Create: `upstream/sub2api/frontend/src/components/user/__tests__/UserErrorRequestsTable.spec.ts`
- Create: `upstream/sub2api/frontend/src/components/user/__tests__/UserErrorDetailModal.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/user/UserErrorRequestsTable.vue`
- Modify: `upstream/sub2api/frontend/src/components/user/UserErrorDetailModal.vue`

**Interfaces:**
- Consumes: `UserErrorRequestDetail.request_id` and the existing `getMyErrorDetail`.
- Produces: always-visible `detail` action in the user error table.
- Preserves: row click opens the same detail dialog; administrator `OpsErrorLogTable` already has its explicit `actions` column.

- [ ] **Step 1: Write failing user error-table interaction tests**

Mount with one row. Assert both interactions select the same ID:

```ts
await wrapper.get('[data-testid="user-error-detail-action"]').trigger('click')
expect(wrapper.getComponent(UserErrorDetailModal).props('errorId')).toBe(7)

await wrapper.get('[data-testid="error-row"]').trigger('click')
expect(wrapper.getComponent(UserErrorDetailModal).props('errorId')).toBe(7)
```

The `DataTable` stub must expose row click and the `cell-detail` slot. Assert the detail action remains in `columns` even when `visibleColumnKeys` omits it.

- [ ] **Step 2: Run the table test and confirm RED**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/components/user/__tests__/UserErrorRequestsTable.spec.ts
```

Expected: no explicit detail action.

- [ ] **Step 3: Add the always-visible detail cell**

Append:

```ts
{ key: 'detail', label: t('usage.detail.action') }
```

Filter optional columns while retaining it:

```ts
const columns = computed(() =>
  props.visibleColumnKeys
    ? allColumns.value.filter((column) =>
        column.key === 'detail' || props.visibleColumnKeys!.includes(column.key))
    : allColumns.value
)
```

The detail button calls `openDetail(row.id)` with `@click.stop`, an eye icon, visible `详情` text, and an accessible title.

- [ ] **Step 4: Write failing user error-modal tests**

Mock `getMyErrorDetail` to return `request_id: 'req-user-error-7'`. Assert the modal renders the ID and a copy button. Mock `useClipboard.copyToClipboard` and assert it receives the exact ID. Also cover rejected loading and absence of the request-ID row for historical responses without the field.

- [ ] **Step 5: Run the modal test and confirm RED**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/components/user/__tests__/UserErrorDetailModal.spec.ts
```

Expected: request ID and copy action are absent.

- [ ] **Step 6: Render and copy the owned request ID**

Add a request-ID row near the top of `UserErrorDetailModal`. Use `font-mono`, `break-all`, and an icon-only copy button with `title`/`aria-label`. Call:

```ts
await copyToClipboard(detail.value.request_id, t('usage.detail.copied'))
```

Do not render or infer upstream/internal identifiers.

- [ ] **Step 7: Verify existing administrator error action**

Run its focused test unchanged:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/views/admin/ops/components/__tests__/OpsErrorLogTable.spec.ts
```

Expected: PASS and existing `openErrorDetail` behavior remains intact.

- [ ] **Step 8: Run all error-detail tests and typecheck**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/components/user/__tests__/UserErrorRequestsTable.spec.ts \
  src/components/user/__tests__/UserErrorDetailModal.spec.ts \
  src/views/admin/ops/components/__tests__/OpsErrorLogTable.spec.ts
pnpm typecheck
```

Expected: PASS.

- [ ] **Step 9: Commit Task 6**

```bash
git add upstream/sub2api/frontend/src/components/user/UserErrorRequestsTable.vue \
  upstream/sub2api/frontend/src/components/user/UserErrorDetailModal.vue \
  upstream/sub2api/frontend/src/components/user/__tests__/UserErrorRequestsTable.spec.ts \
  upstream/sub2api/frontend/src/components/user/__tests__/UserErrorDetailModal.spec.ts
git commit -m "feat: expose error request details to users"
```

---

### Task 7: Integrated verification and responsive visual check

**Files:**
- Modify only files from Tasks 1-6 if verification exposes a defect.
- Create no test-only production route or persistent mock page.

**Interfaces:**
- Consumes: complete backend/frontend feature.
- Produces: passing focused suites, passing builds, and inspected desktop/mobile dialog states.

- [ ] **Step 1: Run backend verification**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/handler/admin ./internal/service ./internal/server/routes -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 2: Run frontend focused and full verification**

Run:

```bash
cd upstream/sub2api/frontend
pnpm test:run -- src/api/__tests__/admin.usage.spec.ts \
  src/components/usage/__tests__/usageDetail.spec.ts \
  src/components/usage/__tests__/UsageDetailDialog.spec.ts \
  src/components/admin/usage/__tests__/UsageTable.spec.ts \
  src/components/user/__tests__/UserErrorRequestsTable.spec.ts \
  src/components/user/__tests__/UserErrorDetailModal.spec.ts \
  src/views/user/__tests__/UsageView.spec.ts \
  src/views/admin/__tests__/UsageView.spec.ts \
  src/views/admin/ops/components/__tests__/OpsErrorLogTable.spec.ts
pnpm test:run
pnpm typecheck
pnpm build
```

Expected: PASS.

- [ ] **Step 3: Start the frontend and inspect user/admin dialogs**

Run:

```bash
cd upstream/sub2api/frontend
pnpm dev --host 127.0.0.1
```

Use Playwright against the printed local URL. Use browser request interception or an available local backend session to supply one token-billed successful record, one administrator record with upstream/account fields, and one user error detail. Do not add fixture routes to production code.

Inspect and capture:

- desktop `1440x900`, user success detail, light and dark;
- desktop `1440x900`, admin success detail with administrator section;
- mobile `390x844`, user success detail;
- mobile `390x844`, user error detail with long request ID and response body.

Verify no overlap, horizontal viewport overflow, clipped close action, inaccessible copy/detail actions, blank sections, or admin-only fields in user scope.

- [ ] **Step 4: Run final diff and security review**

Run:

```bash
git diff --check
git status --short
git diff -- upstream/sub2api/backend/internal/service/ops_user_error.go \
  upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue \
  upstream/sub2api/frontend/src/components/user/UserErrorDetailModal.vue
```

Confirm:

- user successful details call only `/usage/:id`;
- user error details contain only the new owned `request_id` beyond the existing whitelist;
- no secret, raw header, account credential, or upstream request ID field is added;
- no unrelated dirty-worktree file is staged.

- [ ] **Step 5: Commit verification fixes only if needed**

If verification required code changes, stage only those feature files and commit:

```bash
git commit -m "fix: harden usage detail presentation"
```

If no code changes were required, do not create an empty commit.
