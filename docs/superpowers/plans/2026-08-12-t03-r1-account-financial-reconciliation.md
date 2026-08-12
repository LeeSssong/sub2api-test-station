# T03-R1 账号财务、逐笔上游成本与异常核对 MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax. Assign a fresh implementer to each task, obtain an independent task review after each, and obtain a final whole-branch review before handoff.

**Goal:** 保持官方 `usage_logs` 结构和插入语义不变，为功能启用后的 Sub/New 官方流水持久化独立逐笔成本证据，并提供管理员财务首页、异常核对、OAuth 每日成本和本地证据详情。

**Architecture:** 先以两笔显式、可审查的 Git revert 移除冻结的直接 `usage_logs` 实现。然后用四个独立 Ent 模型保存一对一 evidence、一对一 review、账号北京日值/覆盖和功能启用边界。现有响应后 usage 任务在官方流水成功插入后仅执行一次有界精确登记；财务报告、异常 Tab 和详情只能从本地 evidence/review/day-value 事实读取。

**Tech Stack:** Go 1.24、PostgreSQL、Ent、Gin、既有 usage worker、Vue 3、TypeScript、Vitest、pnpm。

## Global Constraints

- 日期统一为 `2026-08-12`；时区 `Asia/Shanghai`；核算币种为 CNY，本站 `actual_cost` 与 Sub/New 原生账单按人民币 `1:1`。
- Task 1 必须按逆依赖顺序显式反转 `ce5691527a54cb2e7f8b3dabf624eb65e93fc177`、`1c3e8768458c7c46725725e9f828fbcaba403f16`；仅反转实现、测试、生成代码和迁移，保留旧规格、旧计划和 Git 历史。
- 禁止新增或保留 `usage_logs.upstream_actual_cost`、`usage_logs.profit`、状态、原因、登记时间或等价字段；不得修改官方 usage 的插入、扫描、历史数据或既有迁移。
- 新表只覆盖：一对一 `usage_log_id` evidence、一对一 review、`account_id + business_date` OAuth 成本/每日覆盖、功能启用设置。迁移只能 expand-only，不得历史 `UPDATE`、回填、收缩、重写既有迁移。
- 非 OAuth 流水仅在本次响应后的现有 usage 任务内登记一次；禁止历史回填、延迟补查、定时扫描、业务级上游重试、页面打开查上游、模糊匹配、估算或以 `account_cost` 冒充原生账单。
- `confirmed` 且非零成本立即纳入；`confirmed_zero` 与 `unavailable` 在人工核对前完全隔离。启用后非 OAuth 缺证据通过 `LEFT JOIN` 投影为 `unavailable/evidence_not_registered`。
- 只有字面账号类型 `oauth` 不查询上游且不创建逐笔异常；未填写北京日成本的 OAuth 账号日整体排除全站营收、支出、利润、利润率。
- 全部新接口管理员专属；普通用户导航、DTO、列表、详情、导出不含 evidence、review、上游成本、利润或审计字段。
- 复用 `audit_logs`；不新增审计 UI、撤销、`external-primary`、relay-ops 主账务、GitHub Actions、定时汇总表或 WebSocket。
- 每个任务由 fresh implementer 独占列出的写入路径；跨任务文件冲突必须停止并拆出新窄任务，不得并行共享编辑。
- 全部验证及审查后只能报告 `READY_FOR_ROOT_REVIEW`。没有精确目标 `main` SHA 的 `AUTHORIZE_MERGE_TO_MAIN` 不得合并、推送或部署。

## File and Interface Map

| Area | Files | Owner |
| --- | --- | --- |
| 冻结实现反转 | `upstream/sub2api/backend/{ent,migrations,internal/{repository,service}}/**` | Task 1 |
| 独立存储 | `backend/ent/schema/{usage_upstream_cost_evidence,usage_cost_review,account_daily_financial_value,account_financial_setting}.go`、`backend/migrations/222_account_financial_reconciliation.sql`、生成 `ent/**` | Task 2 |
| 单次登记 | `backend/internal/{service,repository}/usage_cost_evidence*.go`、`service/sub_upstream_cost.go`、usage task tests | Task 3 |
| 汇总/复核/覆盖 | `backend/internal/{service,repository}/account_financial*.go` | Task 4 |
| 管理员 API | `backend/internal/handler/admin/account_financial*.go`、`routes/admin.go`、Wire | Task 5 |
| 财务首页 | `frontend/src/api/admin/accountFinancial.ts`、`AccountProfitabilityView.vue` 和测试 | Task 6 |
| 异常 Tab/详情 | `frontend/src/components/admin/usage/CostExceptionTable.vue`、`UsageView.vue`、`UsageDetailDialog.vue` 和测试 | Task 7 |
| 验证/审查 | `docs/superpowers/reports/2026-08-12-t03-r1-account-financial-reconciliation-*.md` | Task 8 |

## Cross-task Interfaces

Task 2 establishes the only evidence types; later tasks must not add equivalent `usage_logs` fields:

```go
type UsageCostEvidenceStatus string
const (
    UsageCostEvidenceStatusConfirmed UsageCostEvidenceStatus = "confirmed"
    UsageCostEvidenceStatusConfirmedZero UsageCostEvidenceStatus = "confirmed_zero"
    UsageCostEvidenceStatusUnavailable UsageCostEvidenceStatus = "unavailable"
)
type UsageCostEvidence struct {
    ID int64; UsageLogID int64; Source string; UpstreamRequestID string
    SubActualCost *float64; NewAPIQuota *float64; NewAPIQuotaPerUnit *float64
    NormalizedCostCNY *float64; ProfitCNY *float64
    EvidenceStatus UsageCostEvidenceStatus; ReasonCode string; RecordedAt time.Time
}
type UsageCostReviewInput struct { UsageLogID int64; ManualCostCNY *float64; ReviewedBy int64; RequestID string }
```

Task 4 exposes exact typed methods to Task 5: `GetReport`, `ListExceptions`, `ReviewOne`, `ReviewSelected`, `ReviewFiltered`, `SetOAuthDailyCost`, `SetTodayOverride`, and `GetUsageEvidence`. All take `context.Context`, typed input, and return typed DTO/error.

---

### Task 1: Explicitly revert frozen direct-`usage_logs` implementation

**Files:**
- Modify only through Git revert: `upstream/sub2api/backend/ent/**`
- Modify only through Git revert: `upstream/sub2api/backend/internal/repository/usage_log_repo_*.go`
- Modify only through Git revert: `upstream/sub2api/backend/internal/service/usage_log.go`
- Delete only through Git revert: `upstream/sub2api/backend/migrations/221_usage_log_upstream_cost_persistence.sql`
- Preserve: `docs/superpowers/specs/2026-08-12-t03-r1-upstream-cost-persistence-design.md`
- Preserve: `docs/superpowers/plans/2026-08-12-t03-r1-upstream-cost-persistence.md`

**Interfaces:** Consumes the two frozen commits. Produces a candidate with no direct upstream-cost persistence in official usage logs.

- [ ] **Step 1: Write failing legacy-direction guard**

Create `upstream/sub2api/backend/migrations/t03_r1_legacy_usage_log_fields_absent_test.go`:

```go
for _, forbidden := range []string{
    "upstream_actual_cost", "upstream_cost_status", "upstream_cost_reason",
    "upstream_cost_recorded_at", "profit",
} { require.NotContains(t, usageSchemaAndSQL, forbidden) }
require.NoFileExists(t, "221_usage_log_upstream_cost_persistence.sql")
```

- [ ] **Step 2: Run RED**

Run: `cd upstream/sub2api/backend && go test ./migrations -run TestT03R1LegacyUsageLogFieldsAreAbsent -count=1`

Expected: FAIL because frozen migration/generated fields exist.

- [ ] **Step 3: Revert in auditable order**

```bash
git revert --no-edit ce5691527a54cb2e7f8b3dabf624eb65e93fc177
git revert --no-edit 1c3e8768458c7c46725725e9f828fbcaba403f16
```

Resolve only mechanical conflicts with official `v0.1.175`. Do not reset, amend historical commits, delete old docs, or retain renamed equivalents.

- [ ] **Step 4: Run GREEN**

```bash
cd upstream/sub2api/backend
go test ./migrations -run TestT03R1LegacyUsageLogFieldsAreAbsent -count=1
go test ./internal/repository -run 'TestUsageLog.*(Insert|Detail|RequestType|SessionID)|TestMigrationsSchema' -count=1
git diff --check
```

Expected: PASS; direct fields/migration absent and existing inserts pass.

- [ ] **Step 5: Independent review checkpoint**

Reviewer verifies both exact commits are reversed rather than hidden, historical docs remain, no cost field/SQL/generated Ent survives, and no release/production path changed.

**Migration / Ent generation:** Do not generate before reversal; generated removal must be part of Git reversions.

---

### Task 2: Add independent Ent evidence, review, daily-value, and setting tables

**Files:**
- Create: `upstream/sub2api/backend/ent/schema/usage_upstream_cost_evidence.go`
- Create: `upstream/sub2api/backend/ent/schema/usage_cost_review.go`
- Create: `upstream/sub2api/backend/ent/schema/account_daily_financial_value.go`
- Create: `upstream/sub2api/backend/ent/schema/account_financial_setting.go`
- Create: `upstream/sub2api/backend/migrations/222_account_financial_reconciliation.sql`
- Create: `upstream/sub2api/backend/migrations/account_financial_reconciliation_migration_test.go`
- Modify generated only: `upstream/sub2api/backend/ent/**`
- Modify: `upstream/sub2api/backend/internal/repository/migrations_schema_integration_test.go`

**Interfaces:** Produces four Ent entities. Setting key `t03_r1_account_financial` has nullable `enabled_at` until activation.

- [ ] **Step 1: Write RED migration/schema tests**

```go
require.Contains(t, sql, "create table if not exists usage_upstream_cost_evidence")
require.Contains(t, sql, "usage_log_id bigint not null unique")
require.Contains(t, sql, "create table if not exists usage_cost_reviews")
require.Contains(t, sql, "create table if not exists account_daily_financial_values")
require.Contains(t, sql, "unique (account_id, business_date)")
require.Contains(t, sql, "create table if not exists account_financial_settings")
require.NotContains(t, sql, "alter table usage_logs")
require.NotContains(t, sql, "update usage_logs")
```

Assert `NUMERIC(20,10)`, evidence/review uniqueness, `DATE business_date`, and separate revenue/cost evidence/review cutoff IDs.

- [ ] **Step 2: Run RED**

```bash
cd upstream/sub2api/backend
go test ./migrations -run TestAccountFinancialReconciliationMigration -count=1
go test ./internal/repository -run TestMigrationsSchema -count=1
```

Expected: FAIL because schemas/migration 222 do not exist.

- [ ] **Step 3: Implement independent storage**

Evidence has unique usage ID, `sub|newapi`, exact request ID, structured source values, normalized cost/profit, status/reason/time. Review has unique usage ID, `reviewed`, manual cost/profit/actor/time; absence is pending. Daily value has unique account/date, OAuth cost, independent revenue/cost override values and cutoff IDs. Setting stores enable boundary. Add only indexes evidence `(evidence_status, usage_log_id)`, review `(usage_log_id)`, daily `(account_id, business_date)`. Store no raw response/body/credential/API key.

- [ ] **Step 4: Generate Ent**

```bash
cd upstream/sub2api/backend
make generate
git diff --check
```

Expected: generated Ent contains four entities and unchanged `UsageLog`.

- [ ] **Step 5: Run GREEN and commit**

```bash
cd upstream/sub2api/backend
go test ./migrations -run 'Test(AccountFinancialReconciliationMigration|T03R1LegacyUsageLogFieldsAreAbsent)' -count=1
go test ./internal/repository -run TestMigrationsSchema -count=1
go test ./ent/schema -count=1
git add ent migrations internal/repository/migrations_schema_integration_test.go
git commit -m "feat: add independent account financial evidence tables"
```

Expected: PASS. Reviewer rejects `usage_logs` alteration, backfill/scheduler, manual generated Ent edits, missing cutoffs, or non-expand-only SQL.

**Migration / Ent generation:** Migration creates only tables/indexes; `make generate` mandatory.

---

### Task 3: Register exactly one terminal evidence result after official usage insert

**Files:**
- Create: `upstream/sub2api/backend/internal/service/{usage_cost_evidence.go,usage_cost_evidence_test.go}`
- Create: `upstream/sub2api/backend/internal/repository/{usage_cost_evidence_repo.go,usage_cost_evidence_repo_test.go}`
- Modify: `upstream/sub2api/backend/internal/service/{sub_upstream_cost.go,sub_upstream_cost_test.go}`
- Modify: `upstream/sub2api/backend/internal/handler/{usage_record_submit_task_test.go,usage_record_task_fallback_test.go}`
- Modify only current hooks: `upstream/sub2api/backend/internal/handler/{gateway_helper.go,openai_gateway_handler.go,gemini_v1beta_handler.go}`

**Interfaces:** Produces `UsageCostEvidenceRegistrar.RegisterOnce(ctx, usageLogID) error`; it runs only after official insert and cannot alter usage success.

- [ ] **Step 1: Write RED registrar/task tests**

```go
func TestRegistrarRegistersConfirmedOnce(t *testing.T) {
    require.NoError(t, registrar.RegisterOnce(ctx, usageID))
    require.Equal(t, service.UsageCostEvidenceStatusConfirmed, evidence.Status)
    require.Equal(t, 1, upstream.RequestCount())
}
func TestRegistrarStoresUnavailableWithoutRetry(t *testing.T) {
    _ = registrar.RegisterOnce(ctx, usageID)
    require.Equal(t, "record_not_found", evidence.ReasonCode)
    require.Equal(t, 1, upstream.RequestCount())
}
func TestUsagePersistsWhenRegistrationFails(t *testing.T) {
    require.NoError(t, recordUsage(ctx, input)); require.NotZero(t, usageID)
}
```

Cover Sub/New nonzero, exact zero, blank/`null`/empty only after exact match, missing request ID, record missing, endpoint/auth/network/parse failure, invalid New unit, conflict, stream/nonstream hooks, OAuth zero HTTP/no evidence.

- [ ] **Step 2: Run RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'Test(Registrar|UsageCostEvidence|SubUpstreamCost)' -count=1
go test ./internal/handler -run 'TestUsageRecord.*(Evidence|Fallback)' -count=1
```

Expected: FAIL because registrar/post-insert hook is absent.

- [ ] **Step 3: Implement one-shot terminal registration**

```go
// Never writes usage_logs. Never queues, retries, scans, or backfills.
func (r *UsageCostEvidenceRegistrar) RegisterOnce(ctx context.Context, usageLogID int64) error
```

Refactor current Sub/New parser into registrar result. Exact finite nonzero is confirmed; exact zero/blank/null/empty is confirmed_zero; stable failures unavailable. Invoke only after insert in response-after task; swallow/log registration failure. Missing evidence is read-projected later, never repaired asynchronously.

- [ ] **Step 4: Run GREEN and commit**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'Test(Registrar|UsageCostEvidence|SubUpstreamCost)' -count=1
go test ./internal/repository -run TestUsageCostEvidenceRepository -count=1
go test ./internal/handler -run 'TestUsageRecord.*(Evidence|Fallback)|TestGateway.*Usage' -count=1
git add internal/service internal/repository internal/handler
git commit -m "feat: register one-shot upstream cost evidence"
```

Expected: PASS. Reviewer traces insert-before-lookup, exact-match normalization, one HTTP call, OAuth exclusion, and rejects read-time lookup/retry/usage-log write.

**Migration / Ent generation:** No schema edit; Task 3 must not modify `ent/schema`, generated `ent/**`, or `migrations/**`.

---

### Task 4: Build snapshot report, reviews, OAuth daily cost, overrides, and audit

**Files:**
- Create: `upstream/sub2api/backend/internal/repository/{account_financial_repo.go,account_financial_repo_test.go}`
- Create: `upstream/sub2api/backend/internal/service/{account_financial.go,account_financial_test.go,account_financial_audit.go,account_financial_audit_test.go}`
- Modify: `upstream/sub2api/backend/internal/service/audit_log_service.go`

**Interfaces:** Produces Task 5 methods; every report card/row shares one DB snapshot and `generated_at`.

- [ ] **Step 1: Write RED accounting/idempotency tests**

Use fixed Beijing clock. Cover confirmed inclusion, pending exception isolation/count/affected revenue, nil manual cost as zero, duplicate review idempotence, literal OAuth only, missing OAuth day excluded from global four metrics, disabled nondeleted balance included, deleted/frozen excluded, 24h ignores overrides, 7/31 day aggregation, zero revenue margin null.

```go
// Revenue: 100 -> override 95 -> later confirmed 20 -> later review 10 == 125.
// Cost: 40 -> override 35 -> later confirmed 8 -> later review 3 == 46.
```

Prove filtered review freezes filters plus `max_usage_log_id`, returns cutoff/matched/updated/skipped, and cannot update concurrent newer usage.

- [ ] **Step 2: Run RED**

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run TestAccountFinancialRepository -count=1
go test ./internal/service -run 'Test(AccountFinancial|AccountFinancialAudit)' -count=1
```

Expected: FAIL because financial services are absent.

- [ ] **Step 3: Implement local-only financial model**

One snapshot reads `enabled_at`; joins post-enable official usage/local evidence/review; projects missing non-OAuth evidence unavailable/pending; includes confirmed nonzero or reviewed exceptions only; applies separate revenue/cost override bases/cutoffs plus later values; computes `SUM(users.balance) WHERE deleted_at IS NULL`, including disabled and excluding frozen balance. Writes validate finite nonnegative/today/OAuth type, use unique usage ID, freeze filtered cutoff, and add redacted audit actor/request/old-new/account/day/cutoff/result.

- [ ] **Step 4: Run GREEN and commit**

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run TestAccountFinancialRepository -count=1
go test ./internal/service -run 'Test(AccountFinancial|AccountFinancialAudit)' -count=1
go test ./internal/service -run 'TestAccountFinancial.*(Review|Override|OAuth|Beijing|24H|7D|31D|Balance)' -count=1
git add internal/repository/account_financial* internal/service/account_financial* internal/service/audit_log_service.go
git commit -m "feat: add account financial reconciliation service"
```

Expected: PASS. Reviewer recomputes fixture values and verifies no upstream HTTP/read-time dependency or user DTO widening.

**Migration / Ent generation:** No schema work; consume Task 2 generated Ent only.

---

### Task 5: Expose administrator-only financial and local-evidence APIs

**Files:**
- Create: `upstream/sub2api/backend/internal/handler/admin/{account_financial_handler.go,account_financial_handler_test.go}`
- Modify: `upstream/sub2api/backend/internal/handler/admin/{usage_handler.go,usage_handler_detail_test.go}`
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go`
- Modify: `upstream/sub2api/backend/internal/handler/wire.go`
- Modify: `upstream/sub2api/backend/cmd/server/{wire.go,wire_gen.go}`

**Interfaces:** Consumes Task 4. Produces report, exception list/reviews, OAuth/day override, normal evidence projection, and local-only compatibility endpoint.

- [ ] **Step 1: Write RED HTTP tests**

```text
GET  /api/v1/admin/operations/account-financial?range=today
GET  /api/v1/admin/usage/cost-exceptions
POST /api/v1/admin/usage/cost-exceptions/:usageLogID/review
POST /api/v1/admin/usage/cost-exceptions/review-selected
POST /api/v1/admin/usage/cost-exceptions/review-filtered
PUT  /api/v1/admin/accounts/:accountID/financial/oauth-cost
PUT  /api/v1/admin/accounts/:accountID/financial/today-override
GET  /api/v1/admin/usage/:id/upstream-cost
```

Assert ordinary user `401/403`, invalid amount/date/type `400`, batch counts/cutoff, fake upstream HTTP count `0` for detail/compatibility read.

- [ ] **Step 2: Run RED**

```bash
cd upstream/sub2api/backend
go test ./internal/handler/admin -run 'Test(AccountFinancial|AdminUsage.*(Evidence|UpstreamCost|Exception))' -count=1
go test ./internal/server/routes -run 'Test.*AccountFinancial|Test.*AdminUsage' -count=1
```

Expected: FAIL because handlers/routes/local detail do not exist.

- [ ] **Step 3: Implement routes, validation, and local detail**

Report returns summary, rows, exceptions, `user_unconsumed_balance_cny`, one `generated_at`. Do not expose raw source data/credentials. Compatibility endpoint must be:

```go
detail, err := h.accountFinancialService.GetUsageEvidence(c.Request.Context(), usageID)
c.JSON(http.StatusOK, detail)
```

Remove `SubUpstreamCostService` from admin read injection once unused, Wire the new service, and generate. Only admin DTOs may carry local evidence fields.

- [ ] **Step 4: Run GREEN and commit**

```bash
cd upstream/sub2api/backend
make generate
go test ./internal/handler/admin -run 'Test(AccountFinancial|AdminUsage.*(Evidence|UpstreamCost|Exception))' -count=1
go test ./internal/server/routes -run 'Test.*AccountFinancial|Test.*AdminUsage' -count=1
go test ./internal/handler -run 'TestUsage.*Detail' -count=1
git add internal/handler/admin internal/server/routes/admin.go internal/handler/wire.go cmd/server/wire.go cmd/server/wire_gen.go
git commit -m "feat: add admin financial reconciliation APIs"
```

Expected: PASS. Reviewer verifies admin middleware, local-only reads, validation and Wire generation.

**Migration / Ent generation:** No migration; `make generate` required for Wire output.

---

### Task 6: Upgrade existing account profitability into financial home

**Files:**
- Create: `upstream/sub2api/frontend/src/api/admin/accountFinancial.ts`
- Create: `upstream/sub2api/frontend/src/api/__tests__/admin.accountFinancial.spec.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/{AccountProfitabilityView.vue,__tests__/AccountProfitabilityView.spec.ts}`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/{zh-CN,en-US}.json`

**Interfaces:** Consumes Task 5; retains `/admin/operations/account-profitability` as sole financial entry.

- [ ] **Step 1: Write RED UI/API tests**

```ts
expect(wrapper.get('[data-test="financial-generated-at"]').text()).toContain('2026-08-12')
expect(setInterval).toHaveBeenCalledWith(expect.any(Function), 60_000)
expect(api.getReport).toHaveBeenCalledWith({ range: 'today' })
expect(wrapper.get('[data-test="summary-exceptions"]').text()).toContain('3')
```

Cover six global cards, same cutoff, today-only revenue/cost/OAuth edits, OAuth `待填写`, 24h real-time/no-edit, 7/31 read-only, manual/immediate refresh, exception jump retaining range/account, timer cleanup.

- [ ] **Step 2: Run RED**

Run: `cd upstream/sub2api/frontend && pnpm test:run src/api/__tests__/admin.accountFinancial.spec.ts src/views/admin/__tests__/AccountProfitabilityView.spec.ts`

Expected: FAIL because financial API/refresh/edit/jump behavior is absent.

- [ ] **Step 3: Implement local financial home**

Remove legacy external/control-plane display. Render revenue, expense, profit, margin, exceptions/affected revenue, user unconsumed balance, cutoff, countdown/manual refresh. Rows show source/revenue/expense/profit/margin/exceptions/OAuth completeness. Today alone has explicit saves; profit/margin read-only. Exception click opens existing `/admin/usage?tab=cost-exceptions` with range/optional account. Remove `controlPlaneAPI`, `ReadModelStatus`, externalization imports.

- [ ] **Step 4: Run GREEN and commit**

```bash
cd upstream/sub2api/frontend
pnpm test:run src/api/__tests__/admin.accountFinancial.spec.ts src/views/admin/__tests__/AccountProfitabilityView.spec.ts
pnpm typecheck
pnpm build
git add src/api src/views/admin/AccountProfitabilityView.vue src/views/admin/__tests__/AccountProfitabilityView.spec.ts src/i18n/locales
git commit -m "feat: add administrator account financial home"
```

Expected: PASS. Reviewer verifies existing route, 60s refresh, today-only edits and no external/ordinary-user dependency.

**Migration / Ent generation:** Frontend-only; reject backend/generated changes.

---

### Task 7: Add admin exception Tab and local evidence detail

**Files:**
- Create: `upstream/sub2api/frontend/src/components/admin/usage/{CostExceptionTable.vue,__tests__/CostExceptionTable.spec.ts}`
- Modify: `upstream/sub2api/frontend/src/api/admin/usage.ts`
- Modify: `upstream/sub2api/frontend/src/api/__tests__/admin.usage.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/{UsageView.vue,__tests__/UsageView.spec.ts}`
- Modify: `upstream/sub2api/frontend/src/components/usage/{UsageDetailDialog.vue,__tests__/UsageDetailDialog.spec.ts}`
- Modify: `upstream/sub2api/frontend/src/{types/index.ts,i18n/locales/{zh-CN,en-US}.json}`

**Interfaces:** Consumes Task 5. Produces `cost-exceptions` as a filter Tab over existing admin usage and local-only detail.

- [ ] **Step 1: Write RED tests**

```ts
expect(route.query.tab).toBe('cost-exceptions')
expect(api.listCostExceptions).toHaveBeenCalledWith(expect.objectContaining({ account_id: 42 }))
await wrapper.get('[data-test="review-selected"]').trigger('click')
expect(api.reviewSelected).toHaveBeenCalledWith({ usage_log_ids: [11, 12] })
expect(api.getUpstreamCost).not.toHaveBeenCalled()
```

Cover reason/provenance, one/selected/filtered reviews, cutoff/matched/updated/skipped, filters/pagination/export/route restoration, user fixtures/types with no fields/tab.

- [ ] **Step 2: Run RED**

```bash
cd upstream/sub2api/frontend
pnpm test:run src/api/__tests__/admin.usage.spec.ts src/components/admin/usage/__tests__/CostExceptionTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts
```

Expected: FAIL because Tab/review/local detail is absent.

- [ ] **Step 3: Implement exception view and local detail**

Add Tab to current `UsageView`; restore tab/range/account/evidence/review query. Reuse filters, pagination, selection, detail/export without copied usage rows. Review actions show server cutoff/counts and refetch after success. Detail renders regular local evidence/review payload and removes `getUpstreamCost`; preserve user scope by type narrowing.

- [ ] **Step 4: Run GREEN and commit**

```bash
cd upstream/sub2api/frontend
pnpm test:run src/api/__tests__/admin.usage.spec.ts src/components/admin/usage/__tests__/CostExceptionTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts
pnpm typecheck
pnpm build
git add src/api/admin/usage.ts src/components/admin/usage src/components/usage src/views/admin/UsageView.vue src/views/admin/__tests__/UsageView.spec.ts src/types src/i18n/locales
git commit -m "feat: add admin usage cost exception review"
```

Expected: PASS. Reviewer verifies one usage view, no upstream source HTTP, cutoff disclosure, non-admin absence by API/type/navigation.

**Migration / Ent generation:** Frontend-only; reject backend/generated changes.

---

### Task 8: Validate, independently review, and apply fail-closed release handoff

**Files:**
- Create: `docs/superpowers/reports/2026-08-12-t03-r1-account-financial-reconciliation-task-review.md`
- Create: `docs/superpowers/reports/2026-08-12-t03-r1-account-financial-reconciliation-final-review.md`
- Create: `docs/superpowers/reports/2026-08-12-t03-r1-account-financial-reconciliation-release-preflight.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:** Consumes clean Task 1–7 SHA. Produces `READY_FOR_ROOT_REVIEW` or rejection record; never merges, pushes, deploys, touches production, or starts T05.

- [ ] **Step 1: Add RED negative guards**

```bash
test -z "$(git diff --name-only "$BASELINE"..HEAD | rg '^\.github/workflows/')"
! rg -n 'ALTER TABLE usage_logs|upstream_actual_cost|upstream_cost_status|upstream_cost_reason|upstream_cost_recorded_at' upstream/sub2api/backend/{migrations,ent/schema,internal/repository}
! rg -n 'GetByUsageID\(' upstream/sub2api/frontend/src upstream/sub2api/backend/internal/handler/admin
```

Assert only migration 222 is new; previous migration bytes equal target `main`; 222 has no `UPDATE`, `DELETE`, `ALTER TABLE usage_logs`, or destructive SQL.

- [ ] **Step 2: Prove RED against frozen history**

Run same guards against `ce5691527` through temporary worktree or `git show`; record expected legacy failure without changing history.

- [ ] **Step 3: Run GREEN integrated matrix**

```bash
cd upstream/sub2api/backend
go test ./migrations -run 'Test(AccountFinancialReconciliationMigration|T03R1LegacyUsageLogFieldsAreAbsent)' -count=1
go test ./internal/repository -run 'Test(AccountFinancial|UsageCostEvidence|MigrationsSchema|UsageLog)' -count=1
go test ./internal/service -run 'Test(UsageCostEvidenceRegistrar|SubUpstreamCost|AccountFinancial|AccountFinancialAudit)' -count=1
go test ./internal/handler/admin -run 'Test(AccountFinancial|AdminUsage.*(Evidence|UpstreamCost|Exception))' -count=1
go test ./internal/handler -run 'TestUsageRecord.*(Evidence|Fallback)|TestGateway.*Usage|TestUsage.*Detail' -count=1
go vet ./internal/service ./internal/repository ./internal/handler ./internal/handler/admin
make build
cd ../frontend
pnpm test:run src/api/__tests__/admin.accountFinancial.spec.ts src/api/__tests__/admin.usage.spec.ts src/views/admin/__tests__/AccountProfitabilityView.spec.ts src/views/admin/__tests__/UsageView.spec.ts src/components/admin/usage/__tests__/CostExceptionTable.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts
pnpm typecheck
pnpm build
cd ../..
git diff --check
```

Expected: PASS; record output, clean SHA/tree, migration hash, changed paths and warnings.

- [ ] **Step 4: Two-stage independent review**

One reviewer checks each task and another the whole clean branch. Both must verify reverts, unchanged usage logs, one-shot/no retry/no read-time HTTP, expand-only migration, cutoff math, audit/auth, same local source of truth, no GitHub Actions/T05/external-primary. On `REJECT`, preserve candidate/evidence, fix same worktree, rerun RED/GREEN, then re-review.

- [ ] **Step 5: Root-only preflight stop gate**

Only root may release after authorized merge to exact `main`. It must prove expand-only delta, accepted migration hash, clean authorized main, no GitHub Actions, and preflight JSON `downtime_required=false`. If `true`, absent, hash rejected, non-expand-only, or main drift: **stop before migration, stop, restart, or blue-green switch** and retain evidence/candidate.

- [ ] **Step 6: Commit evidence and handoff**

```bash
git add docs/superpowers/reports docs/project/project-progress.md
git commit -m "docs: record T03-R1 financial reconciliation reviews"
```

After approvals, report only `READY_FOR_ROOT_REVIEW`: baseline/candidate SHA, changed files, tests, migration/hash, configuration delta, `downtime_required=unverified until root preflight`, rollback retaining tables, production risk.

**Migration / Ent generation:** No new schema work; verify generated Ent synchronized and root preflight returns `downtime_required=false`.

---

## Execution Order and Self-Review

Execute strictly Task 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8. Tasks 6/7 start only after Task 5 review and only from committed work. Each task has exclusive writes, concrete RED/GREEN commands, migration/Ent directions, and independent review.

Coverage check: Task 1 removes frozen direct fields; Task 2 makes independent models; Task 3 one-time post-completion registration; Task 4 aggregation/review/OAuth/override/audit; Tasks 5–7 admin-only local API/UI, exception Tab/batches, financial home/60-second refresh; Task 8 review and expand-only/no-downtime gate. Placeholder scan complete: no TODO, deferred implementation, undefined interface, or unspecified test.

## Execution Handoff

Plan saved to `docs/superpowers/plans/2026-08-12-t03-r1-account-financial-reconciliation.md`. Required execution: fresh subagent implementer per task, independent review per task, final whole-branch review.
