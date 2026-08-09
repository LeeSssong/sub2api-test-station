# 生图账号固定上游成本 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** 从新请求开始，为生产“生图”分组的 1K/2K/4K 图片分别记录 $0.06/$0.08/$0.10 的最终上游账号成本，并让流水、配额、通知、报表和管理员界面使用同一最终值且不受 Token 倍率影响。

**Architecture:** 在 usage_logs 增加可空的 account_cost 最终成本快照，历史空值继续由旧公式兼容。账号统计定价返回“规则成本 + 是否应用账号倍率”的结构化结果；图片档位规则不应用倍率，其他规则保留现有语义。统一扣费和所有成本消费者优先使用快照，生产配置通过本地/宿主幂等脚本绑定到“生图”分组，不写客户模型价格。

**Tech Stack:** Go 1.24 backend, PostgreSQL migrations/raw SQL, Vue 3 + TypeScript + Vitest, Bash + PostgreSQL psql, existing local/host blue-green release chain.

## Global Constraints

- 仅影响新请求；不回补现有 92 条图片流水，历史 account_cost IS NULL 继续使用旧公式。
- 1K 固定 $0.06/image，2K 固定 $0.08/image，4K 固定 $0.10/image。
- 固定图片最终成本不应用 account_rate_multiplier；Token、普通按次和既有账号统计规则继续应用原倍率语义。
- 未知图片档位、无效图片数量、规则缺失或规则不匹配继续旧回退，不把未知档位默认成 2K。
- 不修改客户 total_cost、actual_cost、用户图片倍率、调度、路由或分组客户图片价格。
- 账号统计专用渠道不包含 channel_model_pricing，apply_pricing_to_account_stats=false，规则按 group ID 覆盖未来加入的账号。
- 不通过修改账号 Token 倍率、反向除倍率或把图片流水倍率改为 1 规避成本。
- 生产发布不使用 GitHub Actions；配置、迁移、发布和回滚走现有本地/宿主脚本链。
- 每个任务开始和实质状态变化都更新 docs/project/project-progress.md；每个任务完成后由独立审查代理复核并记录结果。

---

## 文件与接口地图

### 后端持久化与成本合同

- upstream/sub2api/backend/migrations/201_usage_log_account_cost.sql: expand-only 新增 usage_logs.account_cost NUMERIC(20,10)。
- upstream/sub2api/backend/internal/service/usage_log.go: UsageLog.AccountCost *float64 最终成本快照。
- upstream/sub2api/backend/internal/repository/usage_log_repo_insert.go: 新列插入参数、单行/批量/最佳努力写入顺序。
- upstream/sub2api/backend/internal/repository/usage_log_repo_query.go: 新列查询和扫描。
- upstream/sub2api/backend/internal/service/account_stats_pricing.go: 结构化账号成本解析和严格图片档位匹配。
- upstream/sub2api/backend/internal/service/gateway_usage_billing.go: 统一扣费、legacy 扣费、账号配额通知使用最终成本。
- upstream/sub2api/backend/internal/service/openai_gateway_usage.go: OpenAI 图片用量记录路径传递最终成本。

### 后端统计与管理员合同

- upstream/sub2api/backend/internal/repository/account_cost_sql.go: 统一历史兼容 SQL 表达式。
- upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go: 统计聚合。
- upstream/sub2api/backend/internal/repository/usage_log_repo_trend.go: 趋势和账号分组聚合。
- upstream/sub2api/backend/internal/repository/usage_log_repo_dashboard.go: 看板明细。
- upstream/sub2api/backend/internal/repository/dashboard_aggregation_repo.go: 仪表盘聚合。
- upstream/sub2api/backend/internal/service/account_profitability.go: 盈利/relay expense。
- upstream/sub2api/backend/internal/handler/dto/types.go、mappers.go: 管理员 DTO 返回 account_cost，普通用户 DTO 继续隐藏。

### 管理员前端

- upstream/sub2api/frontend/src/types/index.ts: AdminUsageLog.account_cost。
- upstream/sub2api/frontend/src/components/admin/usage/UsageTable.vue: 单行、tooltip、导出均优先最终快照。
- upstream/sub2api/frontend/src/views/admin/UsageView.vue: CSV 导出使用最终快照。
- upstream/sub2api/frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts、upstream/sub2api/frontend/src/api/__tests__/admin.usage.spec.ts: 新字段回归。

### 生产配置与运行手册

- ops/configure-image-account-costs.sh: 默认只读检查，显式 --apply 才写入目标数据库。
- tests/operations/configure_image_account_costs_test.sh: 无真实生产写入的合同测试。
- docs/runbooks/image-account-fixed-cost.md: check/apply、迁移、验证、回滚和自然流量验收。

---

## Task 1: 持久化最终账号成本并保持历史兼容

**Files:**
- Create: upstream/sub2api/backend/migrations/201_usage_log_account_cost.sql
- Modify: upstream/sub2api/backend/internal/service/usage_log.go
- Modify: upstream/sub2api/backend/internal/repository/usage_log_repo_insert.go
- Modify: upstream/sub2api/backend/internal/repository/usage_log_repo_query.go
- Modify: upstream/sub2api/backend/internal/repository/usage_log_repo_detail_unit_test.go
- Modify: upstream/sub2api/backend/internal/repository/usage_log_repo_request_type_test.go
- Modify: upstream/sub2api/backend/internal/repository/usage_log_repo_integration_test.go
- Modify: upstream/sub2api/backend/internal/repository/migrations_schema_integration_test.go
- Modify: docs/project/project-progress.md

**Interfaces:**
- Produces service.UsageLog.AccountCost *float64; nil means pre-migration historical row only.
- usageLogSelectColumns includes account_cost and scanUsageLog assigns it without changing ordinary user DTO behavior.
- Every insert path passes log.AccountCost at the same position in usageLogInsertArgTypes, SQL columns and values.

- [ ] Step 1: Write failing persistence tests

Add an integration/migration assertion that usage_logs.account_cost exists, is numeric(20,10)-compatible and nullable. Add repository round-trip coverage that inserts 0.06, reads it back, and reads SQL NULL as a nil pointer. Extend SQL mock argument order assertions so the new value is required in single, request-type and batch insert paths.

- [ ] Step 2: Run focused tests and verify failure

Run:
~~~
cd upstream/sub2api/backend
go test -tags=unit ./internal/repository -run 'TestUsageLogRepo|TestMigrations' -count=1
~~~
Expected: FAIL because the column, model field and scan/insert positions do not exist.

- [ ] Step 3: Add the expand-only migration and model field

Create 201_usage_log_account_cost.sql:
~~~
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS account_cost NUMERIC(20,10);
~~~
Add AccountCost *float64 immediately after AccountStatsCost in UsageLog. Keep the column nullable and do not update existing rows.

- [ ] Step 4: Update every raw-SQL insert and read path

Insert account_cost after account_stats_cost in usageLogInsertArgTypes, prepareUsageLogInsert, single INSERT, batched CTE INSERT, best-effort INSERT and all placeholder/value lists. Add account_cost to usageLogSelectColumns, scan it into sql.NullFloat64, and assign a pointer only when valid. Preserve idempotency and existing request IDs.

- [ ] Step 5: Run focused tests and migration checks

Run:
~~~
cd upstream/sub2api/backend
go test -tags=unit ./internal/repository -run 'TestUsageLogRepo|TestMigrations' -count=1
go vet ./internal/repository
~~~
Expected: PASS; migration is idempotent and no historical rows are updated.

- [ ] Step 6: Commit and independent review

~~~
git add upstream/sub2api/backend/migrations/201_usage_log_account_cost.sql upstream/sub2api/backend/internal/service/usage_log.go upstream/sub2api/backend/internal/repository docs/project/project-progress.md
git commit -m "feat: persist final account cost snapshots"
~~~
Fresh reviewer checks migration safety, all insert/scan positions and NULL historical behavior before Task 2 starts.

## Task 2: Implement structured image account-cost resolution

**Files:**
- Modify: upstream/sub2api/backend/internal/service/account_stats_pricing.go
- Modify: upstream/sub2api/backend/internal/service/account_stats_pricing_test.go
- Modify: upstream/sub2api/backend/internal/service/gateway_usage_billing.go
- Modify: upstream/sub2api/backend/internal/service/openai_gateway_usage.go
- Modify: upstream/sub2api/backend/internal/service/openai_gateway_record_usage_test.go
- Modify: docs/project/project-progress.md

**Interfaces:**
~~~
type AccountStatsCostResolution struct {
    StatsCost        *float64
    ApplyAccountRate bool
    Matched          bool
}
~~~
resolveAccountStatsCost receives billingMode, raw imageSize, imageCount, token usage, request count and customer totalCost. applyAccountStatsCost sets UsageLog.AccountStatsCost and always sets UsageLog.AccountCost for a new request. For a matched image interval, AccountCost == StatsCost; otherwise it applies the existing account multiplier/fallback.

- [ ] Step 1: Add failing unit tests for strict image intervals

Create table tests for one and multiple images:
~~~
size 1K count 1 wantStats 0.06 wantFinal 0.06
size 2K count 3 wantStats 0.24 wantFinal 0.24
size 4K count 2 wantStats 0.20 wantFinal 0.20
~~~
Run each with accountRateMultiplier=0.5 and 1.75; final cost must remain unchanged. Add tests for dimension inputs classified by ClassifyImageBillingTier, empty/unknown values returning Matched=false, imageCount<=0, Token pricing applying the multiplier, ordinary per-request pricing applying the multiplier, and existing ApplyPricingToAccountStats/LiteLLM fallback behavior.

- [ ] Step 2: Run pricing tests and verify failure

~~~
cd upstream/sub2api/backend
go test -tags=unit ./internal/service -run 'Test(AccountStats|ResolveAccount|CalculateStats|ApplyAccount)' -count=1
~~~
Expected: FAIL because the structured return type and image interval selection are absent.

- [ ] Step 3: Implement the structured resolver

Keep existing rule priority. For billing_mode=image, match only the strict tier returned by ClassifyImageBillingTier; find the interval by TierLabel and calculate per_request_price * imageCount. Return StatsCost=cost, ApplyAccountRate=false, Matched=true. For existing Token/per-request/model-file results return the same cost with ApplyAccountRate=true. A miss returns Matched=false and leaves the old fallback available.

- [ ] Step 4: Resolve and attach the final snapshot

Update both OpenAI usage recording paths so applyAccountStatsCost runs before buildUsageBillingCommand. The final-cost rules are:
~~~
if resolution.Matched && !resolution.ApplyAccountRate:
    final = statsCost
else if resolution.Matched:
    final = statsCost * accountRateMultiplier
else:
    final = totalCost * accountRateMultiplier
~~~
Set UsageLog.AccountCost even when final == 0; do not change TotalCost, ActualCost, RateMultiplier or AccountRateMultiplier.

- [ ] Step 5: Run pricing and record-usage tests

~~~
cd upstream/sub2api/backend
go test -tags=unit ./internal/service -run 'Test(AccountStats|ResolveAccount|CalculateStats|ApplyAccount|GatewayService|RecordUsage)' -count=1
go vet ./internal/service
~~~
Expected: PASS; customer billing assertions remain unchanged and fixed image final cost is independent of account multiplier.

- [ ] Step 6: Commit and independent review

~~~
git add upstream/sub2api/backend/internal/service docs/project/project-progress.md
git commit -m "feat: resolve fixed image account costs"
~~~
Fresh reviewer checks strict unknown-tier behavior, rule priority and that customer-side costs are not read or mutated.

## Task 3: Use the final snapshot for quota deduction and notifications

**Files:**
- Modify: upstream/sub2api/backend/internal/service/gateway_usage_billing.go
- Modify: upstream/sub2api/backend/internal/service/openai_gateway_usage.go
- Modify: upstream/sub2api/backend/internal/service/gateway_service_subscription_billing_test.go
- Modify: upstream/sub2api/backend/internal/service/openai_gateway_record_usage_test.go
- Modify: upstream/sub2api/backend/internal/repository/usage_billing_repo_integration_test.go
- Modify: docs/project/project-progress.md

**Interfaces:**
- postUsageBillingParams.AccountCost float64 is the already-resolved final account cost.
- buildUsageBillingCommand sets AccountQuotaCost from AccountCost, not from TotalCost * AccountRateMultiplier.
- postUsageBilling and notifyAccountQuota consume the same AccountCost; customer balance/API-key/subscription paths continue using Cost.ActualCost.

- [ ] Step 1: Add failing billing-path tests

Construct a fixed image UsageLog with AccountCost=0.24, TotalCost=0.90, AccountRateMultiplier=0.5. Assert the unified command, legacy increment and quota notification all receive 0.24; assert balance and API key quota still receive ActualCost. Add a regression test that existing Token usage with no final snapshot still resolves to TotalCost * AccountRateMultiplier before command construction.

- [ ] Step 2: Run billing tests and verify failure

~~~
cd upstream/sub2api/backend
go test -tags=unit ./internal/service ./internal/repository -run 'Test(BuildUsageBilling|PostUsageBilling|NotifyAccountQuota|UsageBilling)' -count=1
~~~
Expected: FAIL because the command and legacy paths still recompute the old formula.

- [ ] Step 3: Thread the final cost through all billing paths

Set AccountCost in both postUsageBillingParams construction sites from usageLog.AccountCost, falling back to the legacy formula only for callers/tests that provide no snapshot. Replace the three account-cost computations in buildUsageBillingCommand, postUsageBilling and notifyAccountQuota with the shared value while preserving all existing >0, account-type and quota-limit guards.

- [ ] Step 4: Run focused billing tests

~~~
cd upstream/sub2api/backend
go test -tags=unit ./internal/service ./internal/repository -run 'Test(BuildUsageBilling|PostUsageBilling|NotifyAccountQuota|UsageBilling)' -count=1
go vet ./internal/service ./internal/repository
~~~
Expected: PASS; idempotency, customer deduction and account quota guards remain unchanged.

- [ ] Step 5: Commit and independent review

~~~
git add upstream/sub2api/backend/internal/service upstream/sub2api/backend/internal/repository/usage_billing_repo_integration_test.go docs/project/project-progress.md
git commit -m "fix: bill account quotas from resolved account cost"
~~~
Fresh reviewer traces one fixed-image request through unified, legacy and notification paths and verifies no customer quota path changed.

## Task 4: Unify backend aggregations and administrator presentation

**Files:**
- Create: upstream/sub2api/backend/internal/repository/account_cost_sql.go
- Modify: upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go
- Modify: upstream/sub2api/backend/internal/repository/usage_log_repo_trend.go
- Modify: upstream/sub2api/backend/internal/repository/usage_log_repo_dashboard.go
- Modify: upstream/sub2api/backend/internal/repository/dashboard_aggregation_repo.go
- Modify: upstream/sub2api/backend/internal/service/account_profitability.go
- Modify: upstream/sub2api/backend/internal/service/account_profitability_test.go
- Modify: upstream/sub2api/backend/internal/handler/dto/types.go
- Modify: upstream/sub2api/backend/internal/handler/dto/mappers.go
- Modify: upstream/sub2api/backend/internal/handler/dto/mappers_usage_test.go
- Modify: upstream/sub2api/frontend/src/types/index.ts
- Modify: upstream/sub2api/frontend/src/components/admin/usage/UsageTable.vue
- Modify: upstream/sub2api/frontend/src/views/admin/UsageView.vue
- Modify: upstream/sub2api/frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts
- Modify: upstream/sub2api/frontend/src/api/__tests__/admin.usage.spec.ts
- Modify: docs/project/project-progress.md

**Interfaces:**
- Backend helper effectiveAccountCostSQL(columnPrefix string) returns:
~~~
COALESCE(<prefix>account_cost,
         COALESCE(<prefix>account_stats_cost, <prefix>total_cost) * COALESCE(<prefix>account_rate_multiplier, 1))
~~~
- Admin DTO exposes AccountCost *float64; ordinary user DTO JSON must not contain account_cost, account_stats_cost or account_rate_multiplier.
- Frontend accountBilled(row) returns row.account_cost when non-null, otherwise the legacy formula.

- [ ] Step 1: Add failing backend and frontend regression tests

Backend tests compare a new row (account_cost=0.24, account_stats_cost=0.24, multiplier 0.5) and historical row (account_cost=NULL, total_cost=0.9, multiplier 0.5) across profitability and usage aggregations. Frontend tests assert table, tooltip and CSV use 0.24 for the new row and 0.45 for historical fallback. DTO tests assert admin includes account_cost and user JSON excludes all admin cost fields.

- [ ] Step 2: Run focused tests and verify failure

~~~
cd upstream/sub2api/backend
go test -tags=unit ./internal/repository ./internal/service ./internal/handler/dto -run 'Test(AccountCost|UsageLogFromService|Profitability|UsageLogRepo)' -count=1
cd ../frontend
pnpm exec vitest run src/components/admin/usage/__tests__/UsageTable.spec.ts src/api/__tests__/admin.usage.spec.ts
~~~
Expected: FAIL because queries, DTOs and UI still calculate the legacy expression.

- [ ] Step 3: Centralize backend cost expressions and update every consumer

Add the helper in the repository package and replace every legacy account-cost occurrence found by rg in stats, trends, dashboard, profitability and detail queries. Keep aliases (account_cost, total_account_cost, actual_cost) stable for API clients. Ensure historical NULL snapshots use the old expression exactly.

- [ ] Step 4: Expose the snapshot only to administrators

Map service.UsageLog.AccountCost to AdminUsageLog. Do not add it to UsageLogFromService or ordinary user types. Add frontend AdminUsageLog.account_cost?: number | null and use it in row display, tooltip data and CSV export; retain legacy fallback for old API responses.

- [ ] Step 5: Run focused backend/frontend tests and static checks

~~~
cd upstream/sub2api/backend
go test -tags=unit ./internal/repository ./internal/service ./internal/handler/dto -run 'Test(AccountCost|UsageLogFromService|Profitability|UsageLogRepo)' -count=1
go vet ./internal/repository ./internal/service ./internal/handler/dto
cd ../frontend
pnpm exec vitest run src/components/admin/usage/__tests__/UsageTable.spec.ts src/api/__tests__/admin.usage.spec.ts
pnpm run typecheck
~~~
Expected: PASS; rg -n 'COALESCE\((ul\.)?account_stats_cost,.*account_rate_multiplier' backend/internal returns no unreviewed production expression.

- [ ] Step 6: Commit and independent review

~~~
git add upstream/sub2api/backend/internal/repository upstream/sub2api/backend/internal/service/account_profitability.go upstream/sub2api/backend/internal/service/account_profitability_test.go upstream/sub2api/backend/internal/handler/dto upstream/sub2api/frontend/src/types/index.ts upstream/sub2api/frontend/src/components/admin/usage upstream/sub2api/frontend/src/views/admin/UsageView.vue upstream/sub2api/frontend/src/api/__tests__/admin.usage.spec.ts docs/project/project-progress.md
git commit -m "fix: use final account cost across reporting"
~~~
Fresh reviewer searches the entire backend/frontend tree for old account-cost calculations and checks ordinary user API redaction.

## Task 5: Add reviewed idempotent production configuration

**Files:**
- Create: ops/configure-image-account-costs.sh
- Create: tests/operations/configure_image_account_costs_test.sh
- Create: docs/runbooks/image-account-fixed-cost.md
- Modify: docs/project/project-progress.md

**Interfaces:**
- bash ops/configure-image-account-costs.sh performs a read-only check by default.
- bash ops/configure-image-account-costs.sh --apply is the only write mode.
- Deployment context follows existing scripts: SUB2API_COMPOSE_PROJECT, SUB2API_PROJECT_DIRECTORY, SUB2API_SECRET_ENV_FILE, SUB2API_RELEASE_ENV_FILE, SUB2API_COMPOSE_FILE, SUB2API_IMAGE_OVERLAY; credentials remain in the env files and never print.
- Stable objects: channel name 生图固定上游成本, rule name 生图分组图片固定成本, group lookup name 生图, model wildcard gpt-image-*, platform openai, billing mode image, intervals 1K/0.06, 2K/0.08, 4K/0.10.

- [ ] Step 1: Write failing shell contract tests

The fixture must fake docker compose exec ... psql, capture SQL, and verify: no arguments means read-only SQL; missing/duplicate target group is rejected before writes; an existing incompatible same-name channel/rule is rejected; --apply executes one SERIALIZABLE transaction; rerunning --apply produces byte-identical SQL state; no UPDATE/DELETE usage_logs, no account multiplier updates, no customer pricing rows, no GitHub Actions reference, and secrets never appear in stdout.

- [ ] Step 2: Run the script contract and verify failure

~~~
bash tests/operations/configure_image_account_costs_test.sh
~~~
Expected: FAIL because the script and runbook do not exist.

- [ ] Step 3: Implement check/apply SQL with ownership guards

The SQL must lock groups, channels, channel_groups and account-stats pricing tables; select exactly one groups.name='生图'; create or validate the stable channel; require no customer model pricing and apply_pricing_to_account_stats=false; bind only the target group; create or validate one group-scoped rule with the exact wildcard and three intervals. Existing objects with different status, group binding, customer prices, billing mode, model, platform or prices fail closed. Check mode prints planned object counts only.

- [ ] Step 4: Add runbook and run shell/static checks

Document check, apply, post-apply read-only SQL, migration-before-configuration ordering, removal of the binding/rule for rollback, and natural-request verification. Run:
~~~
bash -n ops/configure-image-account-costs.sh
shellcheck ops/configure-image-account-costs.sh
bash tests/operations/configure_image_account_costs_test.sh
~~~
Expected: PASS with no production database contacted by the fixture run.

- [ ] Step 5: Commit and independent review

~~~
git add ops/configure-image-account-costs.sh tests/operations/configure_image_account_costs_test.sh docs/runbooks/image-account-fixed-cost.md docs/project/project-progress.md
git commit -m "ops: add idempotent image account cost configuration"
~~~
Fresh reviewer checks SQL ownership/conflict guards, default read-only behavior, no history mutation and no release workflow additions.

## Task 6: Whole-branch validation, merge, deploy and natural verification

**Files:**
- Modify: docs/project/project-progress.md
- Create: docs/superpowers/reports/2026-08-09-image-account-fixed-cost-verification.md

**Interfaces:**
- Final evidence records candidate commit/tree, migration hash, focused/full test commands, configuration check/apply output hashes, deployment health, read-only production configuration state and natural image request verification.
- Completion requires main pushed, deployment successful and online verification effective; local tests alone remain 进行中/准备完成.

- [ ] Step 1: Audit all worktrees and review candidate commits

Before merging, list every non-main worktree, preserve the explicitly protected “新建运营界面” and “优化账号卡片” threads, and review whether any non-main branch leads main. Do not touch the unrelated dirty /Users/gongtengxinwen/Documents/sub2api-upstream-resilience-spec worktree.

- [ ] Step 2: Run candidate-wide validation

Run from the candidate worktree:
~~~
cd upstream/sub2api/backend
go test ./...
go vet ./...
cd ../frontend
pnpm run typecheck
pnpm exec vitest run src/components/admin/usage/__tests__/UsageTable.spec.ts src/api/__tests__/admin.usage.spec.ts
cd ../../..
bash -n ops/configure-image-account-costs.sh
shellcheck ops/configure-image-account-costs.sh
bash tests/operations/configure_image_account_costs_test.sh
git diff --check
~~~

- [ ] Step 3: Obtain final whole-branch review

Fresh reviewer checks the full diff against the approved design, searches all account-cost consumers, confirms no customer billing/routing/scheduling changes, and verifies migration ordering and rollback evidence.

- [ ] Step 4: Merge to main and re-run validation on merged main

Merge the reviewed candidate into main; resolve conflicts without overwriting protected worktree changes. On merged main, rerun backend/frontend tests, migration/preflight checks and the configuration script's read-only check. Record the merged commit and tree in the ledger before any production action.

- [ ] Step 5: Deploy only from verified main

Use the existing local/host release chain documented in docs/runbooks/sub2api-blue-green-production-deployment.md; do not add or use GitHub Actions. Apply the database migration before application restart, then run the configuration script with --apply in the authorized production maintenance window. Preserve deployment evidence and keep the candidate worktree until online verification succeeds.

- [ ] Step 6: Verify online without synthetic paid images

Run read-only SQL to confirm the “生图” group binding, exact rule, intervals and no customer model pricing. Wait for the first natural 1K, 2K or 4K request; verify its new usage_logs.account_stats_cost, account_cost, account quota increment, notification input and admin display. Do not manufacture a paid image request. Unobserved tiers remain explicitly pending.

- [ ] Step 7: Publish evidence and close the ledger

Write the verification report with command outputs/hashes and update docs/project/project-progress.md only after main is pushed, deployment is successful and online verification is effective. Retain the candidate worktree and failure evidence if any merge, build, deployment or online check fails; delete it only after the ledger records recovery evidence.

