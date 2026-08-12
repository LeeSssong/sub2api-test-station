# T03-R1 Upstream Cost Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Every task uses a fresh implementer, an independent task reviewer, and a fix/re-review loop before the next task. After all tasks, run a final whole-branch review.

**Goal:** Persist one exact Sub/New native upstream charge result for every post-deployment usage row through the existing response-after usage task, so administrator list/detail views share durable upstream cost and profit without opening a detail to query upstream.

**Architecture:** Extend `usage_logs` with nullable durable result fields, refactor the existing exact Sub/New lookup into a reusable lookup that accepts an in-memory usage row, invoke it once inside the existing asynchronous usage-record task before the final usage insert, and write the result in the same insert. Administrator DTOs and the compatibility `/upstream-cost` endpoint become read-only projections of those columns; ordinary user DTOs remain unchanged.

**Tech Stack:** Go 1.24, PostgreSQL migrations, Ent schema/code generation, SQL repository batch paths, Gin handlers, Vue 3, TypeScript, Vitest, pnpm.

## Global Constraints

- Baseline is `main@747c7fb14d1ded243794a77984778babece7c799`; branch is `codex/t03-r1-upstream-cost-persistence`.
- Process only usage rows generated after T03-R1 deployment. Do not backfill, scan, or mutate historical rows.
- Each new usage task performs at most one native ledger lookup. Do not create delayed lookup, scheduled scan, or business-level upstream retry.
- Sub truth source is exact `/api/v1/usage/records.actual_cost` matching.
- New API truth source is exact `/api/log/token.quota` divided by valid positive `/api/status.quota_per_unit`.
- Exact matched blank, `null`, missing cost field, or empty string means `confirmed` cost `0`; this normalization occurs only after exact record match.
- No exact record, unsupported endpoint, missing credentials, rejected authentication, request/network failure, invalid nonblank response, invalid New API unit, or incomplete pagination means `unavailable` with a stable reason code.
- Profit is `usage_logs.actual_cost - upstream_actual_cost` only for `confirmed` rows.
- Do not estimate, fuzzy-match, use `account_cost` as native cost, add external-primary/relay-ops accounting dependencies, or expose credentials/raw upstream responses.
- Administrator list, detail, and compatibility endpoint must read the same persisted fact. Ordinary users must not receive upstream cost, profit, status, reason, or recorded time.
- Do not modify T05 or add GitHub Actions.
- Candidate execution stops at `READY_FOR_ROOT_REVIEW`; no merge to `main`, push, or deployment without the root task protocol.

## File and Interface Map

- `backend/migrations/221_usage_log_upstream_cost_persistence.sql`: expand-only nullable columns; no historical update and no index unless a proved query needs one.
- `backend/ent/schema/usage_log.go`: Ent definitions for the five nullable fields.
- `backend/internal/service/usage_log.go`: service-domain fields and status constants.
- `backend/internal/repository/usage_log_repo_insert.go`: all single, atomic, batch, and best-effort insert argument lists.
- `backend/internal/repository/usage_log_repo_query.go`: select/scan support for the new columns.
- `backend/internal/service/sub_upstream_cost.go`: reusable exact lookup and persisted-detail projection; no duplicate Sub/New protocol implementation elsewhere.
- `backend/internal/service/openai_gateway_usage.go` and `backend/internal/service/gateway_usage_billing.go`: invoke the lookup once after site `ActualCost` is final and before `CreateBestEffort`.
- `backend/internal/handler/dto/types.go` and `mappers.go`: administrator-only fields.
- `backend/internal/handler/admin/usage_handler.go`: list/detail already use admin DTO; compatibility endpoint reads persisted detail.
- `frontend/src/types/index.ts`, usage detail helper/dialog, and administrator usage list components: render persisted states without a second upstream request.

---

### Task 1: Add the expand-only persistence schema and repository round trip

**Files:**
- Create: `upstream/sub2api/backend/migrations/221_usage_log_upstream_cost_persistence.sql`
- Create: `upstream/sub2api/backend/migrations/usage_log_upstream_cost_persistence_migration_test.go`
- Modify: `upstream/sub2api/backend/ent/schema/usage_log.go`
- Regenerate: `upstream/sub2api/backend/ent/**`
- Modify: `upstream/sub2api/backend/internal/service/usage_log.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_insert.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_query.go`
- Modify: `upstream/sub2api/backend/internal/repository/migrations_schema_integration_test.go`
- Test: `upstream/sub2api/backend/internal/repository/usage_log_repo_detail_unit_test.go`
- Test: `upstream/sub2api/backend/internal/repository/usage_log_repo_unit_test.go`

**Interfaces:**
- Produces constants:
  - `UsageUpstreamCostStatusConfirmed = "confirmed"`
  - `UsageUpstreamCostStatusUnavailable = "unavailable"`
- Produces `UsageLog` fields:
  - `UpstreamActualCost *float64`
  - `UpstreamCostStatus *string`
  - `UpstreamCostReason *string`
  - `Profit *float64`
  - `UpstreamCostRecordedAt *time.Time`
- Repository `Create`, `CreateBestEffort`, batch inserts, `GetByID`, and list scans round-trip all fields without changing `(request_id, api_key_id)` deduplication.

- [ ] **Step 1: Write failing migration contract tests**

Create `usage_log_upstream_cost_persistence_migration_test.go` that reads migration 221 and asserts these exact statements exist:

```go
for _, fragment := range []string{
    "add column if not exists upstream_actual_cost decimal(20, 10)",
    "add column if not exists upstream_cost_status varchar(16)",
    "add column if not exists upstream_cost_reason varchar(64)",
    "add column if not exists profit decimal(20, 10)",
    "add column if not exists upstream_cost_recorded_at timestamptz",
} {
    require.Contains(t, normalizedSQL, fragment)
}
require.NotContains(t, normalizedSQL, "update usage_logs")
require.NotContains(t, normalizedSQL, "create index")
```

Add `requireColumn` checks to `migrations_schema_integration_test.go`, including numeric precision 20/scale 10 for both money columns and nullable status/reason/time columns.

- [ ] **Step 2: Run migration tests and verify RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./migrations -run 'TestUsageLogUpstreamCostPersistenceMigration' -count=1
```

Expected: FAIL because migration 221 does not exist.

- [ ] **Step 3: Add the expand-only migration and Ent/service fields**

Migration content:

```sql
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS upstream_actual_cost DECIMAL(20, 10),
    ADD COLUMN IF NOT EXISTS upstream_cost_status VARCHAR(16),
    ADD COLUMN IF NOT EXISTS upstream_cost_reason VARCHAR(64),
    ADD COLUMN IF NOT EXISTS profit DECIMAL(20, 10),
    ADD COLUMN IF NOT EXISTS upstream_cost_recorded_at TIMESTAMPTZ;
```

Add matching optional/nillable Ent fields. Add the five service fields and the two status constants. Do not add defaults: historical rows must remain `NULL`.

- [ ] **Step 4: Regenerate Ent and add repository RED tests**

Run the repository's documented Ent generation command from `backend` (use the existing `go generate`/Make target discovered in the repository; do not hand-edit generated Ent files). Then add tests that construct a usage row with:

```go
status := service.UsageUpstreamCostStatusConfirmed
cost := 0.004
profit := 0.00288
recordedAt := time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)
```

and assert `prepareUsageLogInsert`, a persisted `GetByID`, and list scanning preserve all five values. Add a historical-row case asserting all five pointers are nil.

- [ ] **Step 5: Run repository tests and verify RED**

Run:

```bash
go test ./internal/repository -run 'Test.*UsageLog.*(UpstreamCost|Detail|Insert|Scan)' -count=1
```

Expected: compile/test failure until insert/select/scan columns are added.

- [ ] **Step 6: Extend every insert and scan path minimally**

Update together, in the exact same order:

1. `usageLogInsertArgTypes`
2. every `INSERT INTO usage_logs` column list and placeholder count
3. `prepareUsageLogInsert().args`
4. `usageLogSelectColumns`
5. `scanUsageLog` variables, `Scan` arguments, and returned `service.UsageLog`

Append the five fields immediately before `created_at` to reduce disruption. Verify single, atomic/outbox, normal batch, and best-effort batch statements all include them.

- [ ] **Step 7: Run Task 1 verification**

Run:

```bash
go test ./migrations -run 'TestUsageLogUpstreamCostPersistenceMigration' -count=1
go test ./internal/repository -run 'Test.*UsageLog.*(UpstreamCost|Detail|Insert|Scan)|TestMigrationsSchema' -count=1
go test ./internal/repository -run 'TestUsageLogRepository.*(Batch|BestEffort|Atomic)' -count=1
git diff --check
```

Expected: PASS; no migration contains historical `UPDATE` or new index.

- [ ] **Step 8: Commit Task 1**

```bash
git add upstream/sub2api/backend/migrations upstream/sub2api/backend/ent upstream/sub2api/backend/internal/service/usage_log.go upstream/sub2api/backend/internal/repository
git commit -m "feat: add upstream cost persistence fields"
```

Stop for independent Task 1 review and fix/re-review before Task 2.

---

### Task 2: Refactor exact Sub/New lookup into a reusable one-shot registrar

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/sub_upstream_cost.go`
- Modify: `upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go`
- Create: `upstream/sub2api/backend/internal/service/usage_upstream_cost.go`
- Create: `upstream/sub2api/backend/internal/service/usage_upstream_cost_test.go`

**Interfaces:**
- Consumes Task 1 status constants and `UsageLog` persistence fields.
- Produces:

```go
type UsageUpstreamCostResult struct {
    UpstreamActualCost *float64
    Status             string
    ReasonCode         string
    RecordedAt         time.Time
}

type UsageUpstreamCostRegistrar interface {
    LookupOnce(ctx context.Context, usage *UsageLog) UsageUpstreamCostResult
}
```

- `SubUpstreamCostService.LookupOnce` performs the existing exact Sub/New native query against an in-memory hydrated usage row.
- `ApplyUsageUpstreamCostResult(usage *UsageLog, result UsageUpstreamCostResult)` sets status/reason/time and calculates profit from the usage's final `ActualCost`.

- [ ] **Step 1: Write RED tests for the reusable result contract**

Add table tests that call `LookupOnce` with an in-memory `UsageLog` and assert:

```go
require.Equal(t, service.UsageUpstreamCostStatusConfirmed, result.Status)
require.InDelta(t, 0.004, *result.UpstreamActualCost, 1e-9)
require.Empty(t, result.ReasonCode)
require.False(t, result.RecordedAt.IsZero())
```

Cover Sub numeric, Sub numeric string, Sub matched blank/null/missing/empty => confirmed zero, New numeric quota/unit conversion, New matched blank/null/missing/empty => confirmed zero, and invalid nonblank values => `unavailable/response_unavailable`.

Add `ApplyUsageUpstreamCostResult` tests:

```go
usage := &UsageLog{ActualCost: 0.00688}
ApplyUsageUpstreamCostResult(usage, confirmedResult(0.004))
require.InDelta(t, 0.00288, *usage.Profit, 1e-9)
```

and unavailable results must nil both cost and profit.

- [ ] **Step 2: Run service tests and verify RED**

```bash
go test ./internal/service -run 'TestUsageUpstreamCost|TestSubUpstreamCostService' -count=1
```

Expected: compile failure because the new interface/functions do not exist.

- [ ] **Step 3: Implement the result object and application helper**

Create `usage_upstream_cost.go` with the exact types above. `ApplyUsageUpstreamCostResult` must enforce:

```go
switch result.Status {
case UsageUpstreamCostStatusConfirmed:
    // cost must be non-nil and finite; profit = ActualCost - cost
case UsageUpstreamCostStatusUnavailable:
    // cost and profit nil; nonempty stable reason required
default:
    // normalize to unavailable/response_unavailable
}
```

Use a cloned UTC `RecordedAt`; never persist response bodies or error text.

- [ ] **Step 4: Refactor the existing lookup without changing protocol rules**

Move the core of `GetByUsageID` behind `LookupOnce(ctx, usage)`. Preserve exactly:

- 10-minute request time window
- limit 1000, maximum 10 pages, 2 MiB body cap, 10-second HTTP cap
- exact match priority already implemented by `subUsageRecordMatchRank` and `newAPIUsageRecordMatchRank`
- 404-only Sub-to-New fallback
- stable reason codes

`GetByUsageID` may remain temporarily for Task 4 compatibility, but it must delegate to `LookupOnce`; do not duplicate HTTP logic.

- [ ] **Step 5: Add explicit no-retry tests**

For `record_not_found`, `endpoint_unsupported`, authentication rejection, network failure, invalid JSON, and pagination exhaustion, count HTTP requests and assert only the existing bounded endpoint/fallback sequence occurs. There must be no sleep, timer, recursive retry, or second business lookup.

- [ ] **Step 6: Run Task 2 verification**

```bash
go test ./internal/service -run 'TestUsageUpstreamCost|TestSubUpstreamCostService' -count=1
go test ./internal/service -run 'TestSubUpstreamCostService' -count=20
go vet ./internal/service
git diff --check
```

- [ ] **Step 7: Commit Task 2**

```bash
git add upstream/sub2api/backend/internal/service/sub_upstream_cost.go upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go upstream/sub2api/backend/internal/service/usage_upstream_cost.go upstream/sub2api/backend/internal/service/usage_upstream_cost_test.go
git commit -m "refactor: expose one-shot upstream cost lookup"
```

Stop for independent Task 2 review and fix/re-review before Task 3.

---

### Task 3: Register upstream cost inside existing usage tasks before persistence

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_service.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_usage.go`
- Modify: `upstream/sub2api/backend/internal/service/gateway_service.go`
- Modify: `upstream/sub2api/backend/internal/service/gateway_usage_billing.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Regenerate: `upstream/sub2api/backend/cmd/server/wire_gen.go`
- Test: `upstream/sub2api/backend/internal/service/openai_gateway_record_usage_test.go`
- Test: `upstream/sub2api/backend/internal/service/gateway_record_usage_test.go`
- Test: `upstream/sub2api/backend/internal/handler/usage_record_submit_task_test.go`

**Interfaces:**
- Consumes `UsageUpstreamCostRegistrar.LookupOnce` and `ApplyUsageUpstreamCostResult` from Task 2.
- `OpenAIGatewayService` and `GatewayService` receive an optional registrar dependency.
- New helper:

```go
func registerUsageUpstreamCost(ctx context.Context, registrar UsageUpstreamCostRegistrar, usage *UsageLog)
```

It performs one lookup only when registrar and usage/account are present, then applies a terminal result before repository insertion.

- [ ] **Step 1: Write RED OpenAI usage tests**

Extend the existing usage repo stub to capture the final log. Add registrar stubs with a call counter. Test confirmed, confirmed-zero, and unavailable results:

```go
require.Equal(t, 1, registrar.calls)
require.Equal(t, UsageUpstreamCostStatusConfirmed, *usageRepo.lastLog.UpstreamCostStatus)
require.InDelta(t, 0.004, *usageRepo.lastLog.UpstreamActualCost, 1e-9)
require.InDelta(t, usageRepo.lastLog.ActualCost-0.004, *usageRepo.lastLog.Profit, 1e-9)
```

Assert lookup occurs after `ActualCost` is finalized and before `CreateBestEffort` sees the row. Add a nil-registrar compatibility case with all new fields nil.

- [ ] **Step 2: Run OpenAI tests and verify RED**

```bash
go test ./internal/service -run 'TestOpenAIGatewayServiceRecordUsage_.*UpstreamCost' -count=1
```

Expected: FAIL because the registrar is not wired or invoked.

- [ ] **Step 3: Wire and invoke the registrar in OpenAI usage**

Add the optional dependency to `OpenAIGatewayService`. Immediately before every terminal `writeUsageLogBestEffort` branch—including simple mode, billing failure, and success—invoke the shared helper after the branch has set its final `ActualCost`. The helper must not run in the HTTP response goroutine outside the existing usage task.

Do not let lookup failure return an error from `RecordUsage`; it is represented by the persisted `unavailable` result.

- [ ] **Step 4: Write RED generic Gateway usage tests**

Mirror the OpenAI tests for the generic `GatewayService.RecordUsage` path. Verify the same single call and persisted terminal fields. Include a billing-error branch to prove profit uses the final stored `ActualCost` value.

- [ ] **Step 5: Run generic tests and verify RED**

```bash
go test ./internal/service -run 'TestGatewayServiceRecordUsage_.*UpstreamCost' -count=1
```

- [ ] **Step 6: Wire the registrar into generic Gateway usage and dependency injection**

Use the same helper and registrar instance for both services. Update Wire providers and generated output through the repository's normal generation flow. Avoid adding a second HTTP client or duplicate Sub/New implementation.

- [ ] **Step 7: Prove existing worker and fallback semantics**

Add/extend handler tests showing:

- the lookup runs within the submitted usage task, not before the client response path completes;
- stopped-pool mandatory fallback still runs the task once;
- configured overflow/drop semantics remain unchanged for nonmandatory usage tasks;
- no new goroutine, timer, delayed queue, or outbox is created.

- [ ] **Step 8: Run Task 3 verification**

```bash
go test ./internal/service -run 'Test(OpenAI)?GatewayServiceRecordUsage_.*UpstreamCost|TestUsageUpstreamCost' -count=1
go test ./internal/handler -run 'Test.*UsageRecordTask' -count=1
go vet ./internal/service ./internal/handler
go build ./cmd/server
git diff --check
```

- [ ] **Step 9: Commit Task 3**

```bash
git add \
  upstream/sub2api/backend/internal/service/openai_gateway_service.go \
  upstream/sub2api/backend/internal/service/openai_gateway_usage.go \
  upstream/sub2api/backend/internal/service/gateway_service.go \
  upstream/sub2api/backend/internal/service/gateway_usage_billing.go \
  upstream/sub2api/backend/internal/service/wire.go \
  upstream/sub2api/backend/cmd/server/wire_gen.go \
  upstream/sub2api/backend/internal/service/openai_gateway_record_usage_test.go \
  upstream/sub2api/backend/internal/service/gateway_record_usage_test.go \
  upstream/sub2api/backend/internal/handler/usage_record_submit_task_test.go
git commit -m "feat: persist upstream cost from usage tasks"
```

Stop for independent Task 3 review and fix/re-review before Task 4.

---

### Task 4: Make administrator APIs read the persisted fact and preserve user isolation

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/dto/types.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/mappers.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/mappers_usage_test.go`
- Modify: `upstream/sub2api/backend/internal/service/sub_upstream_cost.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/usage_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/usage_handler_detail_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/usage_handler_request_type_test.go`

**Interfaces:**
- `AdminUsageLog` adds administrator-only JSON fields:
  - `upstream_actual_cost`
  - `upstream_cost_status`
  - `upstream_cost_reason`
  - `profit`
  - `upstream_cost_recorded_at`
- `UsageLog` and `UserUsageDetail` remain byte-for-byte free of those JSON keys.
- `SubUpstreamCostService.GetByUsageID` becomes a persisted projection and performs zero HTTP calls.

- [ ] **Step 1: Write RED DTO isolation tests**

Construct one service log with all new fields. Assert administrator JSON contains all five keys and ordinary user/list/detail JSON contains none:

```go
require.Contains(t, string(adminJSON), `"upstream_actual_cost":0.004`)
for _, key := range []string{"upstream_actual_cost", "upstream_cost_status", "upstream_cost_reason", "profit", "upstream_cost_recorded_at"} {
    require.NotContains(t, string(userJSON), key)
    require.NotContains(t, string(userDetailJSON), key)
}
```

- [ ] **Step 2: Run DTO tests and verify RED**

```bash
go test ./internal/handler/dto -run 'TestUsageLogFromService.*UpstreamCost' -count=1
```

- [ ] **Step 3: Add admin-only DTO mapping**

Map the five pointers only in `UsageLogFromServiceAdmin`. Do not add them to embedded ordinary `UsageLog` because that would expose them through user APIs.

- [ ] **Step 4: Write RED compatibility endpoint tests**

Use a persisted usage stub with confirmed, unavailable, and all-null historical states. Inject an HTTP client/server that fails the test if called. Assert:

- confirmed response includes persisted cost/profit/status;
- unavailable includes persisted stable reason;
- historical null maps to a distinct safe response, with reason code `historical_not_recorded` and null numbers;
- HTTP call count remains zero.

- [ ] **Step 5: Run handler/service tests and verify RED**

```bash
go test ./internal/service ./internal/handler/admin -run 'Test.*PersistedUpstreamCost|TestAdminUsageGetUpstreamCost' -count=1
```

- [ ] **Step 6: Convert `GetByUsageID` to persisted projection**

Read one hydrated usage row and construct `SubUpstreamCostDetail` only from persisted fields. Keep the administrator route and response shape compatible. Remove any runtime path from this method to `LookupOnce`; only usage creation may call `LookupOnce`.

- [ ] **Step 7: Verify administrator list/detail consistency and user isolation**

Add list and detail handler tests asserting the same persisted values and reason codes. Re-run user handler contract tests and API contract tests to prove no field exposure.

- [ ] **Step 8: Run Task 4 verification**

```bash
go test ./internal/handler/dto -run 'TestUsageLogFromService' -count=1
go test ./internal/service -run 'Test.*PersistedUpstreamCost|TestSubUpstreamCostService' -count=1
go test ./internal/handler/admin -run 'TestAdminUsage(GetByID|GetUpstreamCost|List)' -count=1
go test ./internal/handler -run 'TestUsageHandler.*Detail|TestUsageHandler.*RequestType' -count=1
go test ./internal/server -run 'TestAPIContract' -count=1
git diff --check
```

- [ ] **Step 9: Commit Task 4**

```bash
git add upstream/sub2api/backend/internal/handler upstream/sub2api/backend/internal/service/sub_upstream_cost.go upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go
git commit -m "feat: expose persisted upstream cost to admins"
```

Stop for independent Task 4 review and fix/re-review before Task 5.

---

### Task 5: Update administrator frontend to use persisted list/detail data

**Files:**
- Modify: `upstream/sub2api/frontend/src/types/index.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/usage.ts`
- Modify: `upstream/sub2api/frontend/src/api/__tests__/admin.usage.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/usage/usageDetail.ts`
- Modify: `upstream/sub2api/frontend/src/components/usage/__tests__/usageDetail.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue`
- Modify: `upstream/sub2api/frontend/src/components/usage/__tests__/UsageDetailDialog.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/UsageView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/usage/UsageTable.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`

**Interfaces:**
- `AdminUsageLog` TypeScript type carries persisted fields.
- `AdminUsageCostDetail` retains compatibility endpoint typing and adds `recorded_at`/historical reason if backend response includes them.
- Detail dialog does not call `adminUsageAPI.getUpstreamCost` after `getById` already returns persisted fields.

- [ ] **Step 1: Write RED type/helper tests**

Add helper cases for:

- `confirmed` positive cost/profit;
- `confirmed` zero cost;
- `unavailable` returns no display numbers and maps the stable reason;
- `status == null` renders “历史记录未登记”.

Use explicit fixtures rather than `as any` so TypeScript requires the new fields.

- [ ] **Step 2: Run focused helper tests and verify RED**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/usage/__tests__/usageDetail.spec.ts
```

- [ ] **Step 3: Extend administrator types and projection helpers**

Add only administrator fields to `AdminUsageLog`. Keep `UsageLog` and `UserUsageDetail` unchanged. Project list/detail display from these persisted fields.

- [ ] **Step 4: Write RED detail-dialog network test**

Change the confirmed fixture so `adminGetById` contains the persisted fields, then assert:

```ts
expect(adminGetUpstreamCost).not.toHaveBeenCalled()
expect(valueForLabel(wrapper, 'admin.usageCostDetail.upstreamActualCost')).toBe('$0.004000')
```

Add historical and unavailable fixtures and ordinary-user tests proving no admin section/fields appear.

- [ ] **Step 5: Run dialog tests and verify RED**

```bash
pnpm vitest run src/components/usage/__tests__/UsageDetailDialog.spec.ts src/api/__tests__/admin.usage.spec.ts
```

- [ ] **Step 6: Remove the detail-triggered upstream request**

Delete `loadAdminCost` and its call from the dialog. Build the view model directly from `AdminUsageLog`. Keep `getUpstreamCost` API exported only if another compatibility caller exists; otherwise retain it with a contract test but do not invoke it from list/detail UI.

- [ ] **Step 7: Add or update administrator list columns**

In the existing usage table component, show:

- confirmed: formatted upstream cost and profit, including zero;
- unavailable: `未获取到扣费信息` with the localized safe reason in existing tooltip/detail affordance;
- historical null: `历史记录未登记`.

Do not add these columns to user usage tables. Add focused component tests for all three states.

- [ ] **Step 8: Run Task 5 verification**

```bash
pnpm vitest run src/api/__tests__/admin.usage.spec.ts src/components/usage/__tests__/usageDetail.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts
pnpm vitest run src/views/admin/__tests__/UsageView.spec.ts src/components/admin/usage/__tests__/UsageTable.spec.ts
pnpm typecheck
pnpm build
git diff --check
```

- [ ] **Step 9: Commit Task 5**

```bash
git add upstream/sub2api/frontend/src
git commit -m "feat: show persisted upstream cost in admin usage"
```

Stop for independent Task 5 review and fix/re-review before Task 6.

---

### Task 6: Complete integrated validation, migration gate, and handoff evidence

**Files:**
- Modify: `docs/project/project-progress.md`
- Create: `docs/superpowers/reports/2026-08-12-t03-r1-upstream-cost-persistence.md`
- Modify only if required by an actual test gap: focused backend/frontend tests from Tasks 1–5

**Interfaces:**
- Produces the final candidate evidence and `READY_FOR_ROOT_REVIEW` handoff.
- Does not merge, push, deploy, access paid upstreams, or start T05.

- [ ] **Step 1: Run the complete focused backend suite**

```bash
cd upstream/sub2api/backend
go test ./migrations -run 'TestUsageLogUpstreamCostPersistenceMigration' -count=1
go test ./internal/repository -run 'Test.*UsageLog.*(UpstreamCost|Detail|Insert|Scan)|TestMigrationsSchema' -count=1
go test ./internal/service -run 'TestUsageUpstreamCost|TestSubUpstreamCostService|Test(OpenAI)?GatewayServiceRecordUsage_.*UpstreamCost|Test.*PersistedUpstreamCost' -count=1
go test ./internal/handler/dto -run 'TestUsageLogFromService' -count=1
go test ./internal/handler/admin -run 'TestAdminUsage(GetByID|GetUpstreamCost|List)' -count=1
go test ./internal/handler -run 'Test.*UsageRecordTask|TestUsageHandler.*Detail' -count=1
```

- [ ] **Step 2: Run backend static/build checks**

```bash
go vet ./internal/service ./internal/repository ./internal/handler/... ./internal/server/...
go build ./cmd/server
```

- [ ] **Step 3: Run the complete focused frontend suite and build**

```bash
cd ../frontend
pnpm vitest run src/api/__tests__/admin.usage.spec.ts src/components/usage/__tests__/usageDetail.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts src/views/admin/__tests__/UsageView.spec.ts src/components/admin/usage/__tests__/UsageTable.spec.ts
pnpm typecheck
pnpm build
```

- [ ] **Step 4: Verify scope, migration, and forbidden mechanisms**

From repository root:

```bash
git diff --check
git diff --name-only 747c7fb14d1ded243794a77984778babece7c799...HEAD
rg -n 'time\.Sleep|NewTicker|AfterFunc|delayed|retry.*upstream|external-primary|relay-ops' upstream/sub2api/backend/internal/service/usage_upstream_cost.go upstream/sub2api/backend/internal/service/sub_upstream_cost.go
rg -n 'UPDATE usage_logs|CREATE INDEX' upstream/sub2api/backend/migrations/221_usage_log_upstream_cost_persistence.sql
```

Expected: no historical update/index, no delayed or retry mechanism, and no external accounting dependency.

- [ ] **Step 5: Run migration/release preflight without production mutation**

Use the reviewed local release qualification/preflight scripts for the final tree and record:

- migration set hash before/after;
- migration classification as expand-only;
- `downtime_required=true|false`;
- no GitHub Actions use.

If preflight reports `downtime_required=true`, stop and report the gate. Do not merge or deploy.

- [ ] **Step 6: Run final whole-branch review**

Dispatch a fresh reviewer against the approved spec, this plan, and the full diff from baseline. Required review points:

- exact Sub/New facts and blank-to-zero only after exact match;
- one lookup per new usage, no history/backfill/delayed retry;
- all usage insert paths and final `ActualCost` ordering;
- administrator-only exposure and zero HTTP calls on detail open;
- migration safety, rollback, and task scope.

Fix every P0–P2 finding, rerun affected tests, and obtain re-review approval.

- [ ] **Step 7: Write the validation report and update the ledger**

Report must contain:

- task package `T03-R1`;
- baseline SHA and candidate SHA;
- commits and changed files;
- exact test/build/preflight results;
- migration/config changes;
- `downtime_required`;
- unverified production-only items;
- rollback method and remaining risks;
- explicit statement: no merge, push, deployment, historical backfill, paid request, or T05 work occurred.

Update the ledger status to `进行中 / READY_FOR_ROOT_REVIEW`; never mark completed.

- [ ] **Step 8: Commit the handoff evidence**

```bash
git add docs/project/project-progress.md docs/superpowers/reports/2026-08-12-t03-r1-upstream-cost-persistence.md
git commit -m "docs: record T03-R1 review readiness"
```

- [ ] **Step 9: Verify clean candidate and hand off**

```bash
git status --short --branch
git rev-parse 747c7fb14d1ded243794a77984778babece7c799
git rev-parse HEAD
git log --oneline 747c7fb14d1ded243794a77984778babece7c799..HEAD
```

Handoff only as `READY_FOR_ROOT_REVIEW`, including baseline, candidate SHA, changed files, verification, migration/config, downtime, rollback, and risks. Wait for a root-task instruction in the exact form `AUTHORIZE_MERGE_TO_MAIN` followed by the target full `main` SHA.

## Done When

- Every post-deployment new usage row produced through target Sub/New paths persists either a terminal `confirmed` native cost/profit or a terminal `unavailable` stable reason without an administrator opening details.
- Exact matched blank/null/empty native charge is persisted as confirmed zero; no-match and failures never become zero.
- Administrator list/detail/compatibility endpoint share persisted facts; ordinary users cannot see them.
- Historical rows remain untouched and distinguishable from new unavailable rows.
- No delayed lookup, upstream retry, estimate, fuzzy match, `account_cost` substitution, external accounting dependency, GitHub Actions, T05 work, merge, push, or deployment is introduced.
- All task reviews and final whole-branch review approve the candidate, validation is fresh, worktree is clean, and status is `READY_FOR_ROOT_REVIEW`.
