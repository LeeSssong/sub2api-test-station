# 账号监控：全站经营、分组运营与账号质量 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** 将管理员账号监控改造成“全站经营总览 → 分组运营 → 账号服务质量和投入产出”的工作台，且账号跨分组不会重复统计全站成本、营收或利润。

**Architecture:** relay-ops-service 是真实上游账单、成本覆盖率和按日历史的唯一来源。每个成本 attempt 保存请求实际服务的可空 group_id，按 Attempt/Transaction 聚合，而不是按账号当前分组展开。Sub2API 持有探测健康、原生分组公开状态和每组评分权重；评分和分组排序只用于监控展示，永不写入调度器。

**Tech Stack:** Go、PostgreSQL、Sub2API Gin/service/repository、relay-ops Go HTTP、Vue 3、TypeScript、Vitest。

## Global Constraints

- “接线实时上游账单采集”任务的真实采集、Attempt 匹配、管理刷新和日结闭环是本计划前置条件；不得回退、覆盖或平行重写它正在修改的 importer/reconciliation 文件。
- “上游真实扣费”只来自 relay_ops.upstream_cost_transactions；不得把 today_stats.cost、倍率或前端估算显示成真实扣费。
- 全站按唯一 attempt 统计一次；分组按该 request 在 usage_logs.group_id 的实际归属统计；group_id 为 NULL 只进入全站，并以“未归属请求”解释差额。
- 失败/重试 attempt 若已收费同样进入成本和覆盖分母；请求结束超过 10 分钟仍未对齐显示“对账异常”。
- accounts.priority 是唯一可编辑的全局调度优先级。评分权重不得复用 scheduler_score_weights、priority 或任何分组排序字段。
- 每组评分权重为持久化非负整数 cost/success/ttft/latency，总和 100，默认与重置均为 15/45/20/20；只允许直接输入、保存、恢复默认。
- 分组 Tab 只按原生分组倍率降序、同倍率维持原生稳定顺序；不写入任何顺序字段，不影响路由或用户可见顺序。
- 未对有效用户开放的分组显示“已关闭”/一次运营提示，不持续报无账号故障；真实用户请求失败可覆盖该抑制。
- 实施前第一步必须在 docs/project/project-progress.md 登记“进行中”。只有推送、部署、线上验证均完成才能标记“已完成”。
- 工作树当前有其他 agent 的未提交账单采集改动。每个任务只暂存本任务列出的文件；不得 reset、checkout、批量格式化或提交他人变更。

---

## File Structure

| 文件 | 职责 |
|---|---|
| relay-ops-service/internal/store/migrations/011_reconciliation_group_scope.sql | 为成本 Attempt 保存请求实际分组，建立按组/时间查询索引。 |
| relay-ops-service/internal/sub2api、reconciliation、store | 读取 usage_logs.group_id、幂等落库、按 scope 聚合真实经营结果。 |
| relay-ops-service/internal/http/server.go、reconciliation.go | 暴露受保护的运营总览和按日账务历史 API。 |
| upstream/sub2api/backend/migrations/188_account_monitor_group_score_weights.sql | 保存每组账号评分权重，独立于调度器。 |
| upstream/sub2api/backend/internal/service、repository、handler | 返回分组开放状态、倍率、评分规则、质量证据和只读分数。 |
| upstream/sub2api/frontend/src/api/admin | 定义 native monitor 与 relay 账本的前端合同。 |
| upstream/sub2api/frontend/src/components/admin/account-monitor | 深色监控卡片、评分规则弹窗、轻量历史抽屉。 |
| upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue | 全站首屏、固定摘要、分组 Tab 和卡片排序。 |

### Task 1: 将实际 group_id 固化到真实账单 Attempt

**Precondition:** 真实上游账单采集任务已稳定其 UsageImporter / AttemptInput 合同；若仍在改动，暂停本任务，先对齐其最终提交。

**Files:**
- Create: relay-ops-service/internal/store/migrations/011_reconciliation_group_scope.sql
- Modify: relay-ops-service/internal/sub2api/types.go
- Modify: relay-ops-service/internal/sub2api/client.go
- Modify: relay-ops-service/internal/reconciliation/model.go
- Modify: relay-ops-service/internal/reconciliation/importer.go
- Modify: relay-ops-service/internal/store/reconciliation.go
- Test: relay-ops-service/internal/sub2api/client_test.go
- Test: relay-ops-service/internal/reconciliation/importer_test.go
- Test: relay-ops-service/internal/store/reconciliation_test.go
- Modify: docs/project/project-progress.md

**Interfaces:**
- Produces UsageLog.GroupID *int64, AttemptInput.GroupID *int64, Attempt.GroupID *int64.
- nil means “未归属请求”, never group zero. Re-importing one attempt_id with a different group returns store.ErrConflict.

- [ ] **Step 1: 登记总账为进行中**

在 docs/project/project-progress.md 的“当前最重要进行中事项”新增：

~~~markdown
**账号监控全站经营、分组运营与账号质量**：已确认产品规格；待真实账单采集链路稳定后实施分组归属、运营聚合、每组评分规则与管理端重构。**状态：进行中（未部署）**。
~~~

不得改变“已完成”统计，也不得改写真实账单采集事项。

- [ ] **Step 2: 写失败测试**

在 importer fixture 加：

~~~go
sub2api.UsageLog{
    ID: 31, AccountID: 8, RequestID: "req-group-3",
    GroupID: ptrInt64(3), Model: "gpt-5",
    TotalCost: 2, CreatedAt: start.Add(time.Minute),
}
~~~

断言：

~~~go
require.Equal(t, int64(3), *attempts.inputs[0].GroupID)
~~~

另加 GroupID:nil 断言，确认不会变为 0。client 测试覆盖 group_id:3 成功、group_id:null 成功、缺失或 0 返回 errSchemaMismatch；store 测试覆盖相同 attempt 改变 group 返回冲突。

- [ ] **Step 3: 运行测试确认失败**

Run:

~~~bash
(cd relay-ops-service && go test ./internal/reconciliation ./internal/sub2api ./internal/store -run 'Test.*(Importer|UsageLogs|Reconciliation).*Group' -count=1)
~~~

Expected: FAIL，因为 GroupID 和迁移尚不存在。

- [ ] **Step 4: 实现最小归属链**

创建：

~~~sql
ALTER TABLE relay_ops.upstream_cost_attempts
    ADD COLUMN IF NOT EXISTS group_id BIGINT NULL;

CREATE INDEX IF NOT EXISTS upstream_cost_attempts_group_window_idx
    ON relay_ops.upstream_cost_attempts (group_id, completed_at, id)
    WHERE group_id IS NOT NULL;
~~~

扩展类型：

~~~go
type UsageLog struct {
    // existing fields
    GroupID *int64 "json:\"group_id\""
}
type AttemptInput struct {
    // existing fields
    GroupID *int64
}
~~~

ListUsageLogs 允许 nil，否则要求正数；importer 直接传 GroupID: log.GroupID；RecordUpstreamCostAttempt 的 INSERT、SELECT、scan 和 sameAttempt 都加入 group_id，并用现有 sameInt64Ptr 比较。不得根据账号“当前所属组”推断历史 request 分组。

- [ ] **Step 5: 验证并提交**

Run:

~~~bash
(cd relay-ops-service && go test ./internal/reconciliation ./internal/sub2api ./internal/store -count=1)
git add docs/project/project-progress.md relay-ops-service/internal/store/migrations/011_reconciliation_group_scope.sql relay-ops-service/internal/sub2api/types.go relay-ops-service/internal/sub2api/client.go relay-ops-service/internal/reconciliation/model.go relay-ops-service/internal/reconciliation/importer.go relay-ops-service/internal/store/reconciliation.go relay-ops-service/internal/sub2api/client_test.go relay-ops-service/internal/reconciliation/importer_test.go relay-ops-service/internal/store/reconciliation_test.go
git diff --cached --check
git commit -m "feat: retain reconciliation group scope"
~~~

Expected: PASS；重复 import 不增 attempt；暂存区没有未列出的账单采集文件。

### Task 2: 建立唯一 Attempt 的经营汇总和按日历史

**Files:**
- Create: relay-ops-service/internal/reconciliation/operations.go
- Create: relay-ops-service/internal/reconciliation/operations_test.go
- Modify: relay-ops-service/internal/reconciliation/model.go
- Modify: relay-ops-service/internal/store/reconciliation.go
- Test: relay-ops-service/internal/store/reconciliation_test.go

**Interfaces:**

~~~go
type OperationsScope struct {
    GroupID *int64
    AccountID *int64
    Start, End time.Time
    Currency, Timezone string
}
type OperationsSummary struct {
    TotalAttempts, MatchedAttempts, PendingAttempts, ExceptionAttempts int64
    UpstreamCost, UserCharge, PaperProfit decimal.Decimal
    ProfitMargin *decimal.Decimal
    CoverageKnown bool
    CoverageRatio decimal.Decimal
    UnattributedAttempts int64
    UnattributedUserCharge, UnattributedUpstreamCost decimal.Decimal
    ObservedAt time.Time
}
func (s *Store) ReadOperationsSummary(context.Context, reconciliation.OperationsScope) (reconciliation.OperationsSummary, error)
func (s *Store) ListOperationsDaily(context.Context, reconciliation.OperationsScope) ([]reconciliation.OperationsDailyRow, error)
~~~

Global scope has nil IDs; group scope filters a.group_id=id; account scope filters a.account_id=id; both are their intersection.

- [ ] **Step 1: 写失败测试覆盖去重、未归属和失败已收费**

在同一账号插入 group 3、group 8、NULL group_id 和一个 request_status=failed 但含有效成本 transaction 的四个 attempt：

~~~go
global, _ := st.ReadOperationsSummary(ctx, globalScope)
require.EqualValues(t, 4, global.TotalAttempts)
require.True(t, global.UpstreamCost.Equal(decimal.RequireFromString("0.80")))
require.EqualValues(t, 1, global.UnattributedAttempts)

group3, _ := st.ReadOperationsSummary(ctx, group3Scope)
require.EqualValues(t, 1, group3.TotalAttempts)
require.True(t, group3.UserCharge.Equal(decimal.RequireFromString("2.00")))
~~~

再测两天 ListOperationsDaily：分组和不含未归属，全站含未归属；UserCharge 为 0 时 ProfitMargin 为 nil。

- [ ] **Step 2: 运行失败测试**

Run:

~~~bash
(cd relay-ops-service && go test ./internal/reconciliation ./internal/store -run 'Test.*Operations(.*Group|.*Daily|.*Unattributed)' -count=1)
~~~

Expected: FAIL，因为 scope 和读模型不存在。

- [ ] **Step 3: 实现验证及单一聚合 SQL**

~~~go
func ValidateOperationsScope(scope OperationsScope) (OperationsScope, error) {
    if scope.GroupID != nil && *scope.GroupID <= 0 { return OperationsScope{}, fmt.Errorf("group_id must be positive") }
    if scope.AccountID != nil && *scope.AccountID <= 0 { return OperationsScope{}, fmt.Errorf("account_id must be positive") }
    if !scope.Start.Before(scope.End) { return OperationsScope{}, fmt.Errorf("time window is invalid") }
    scope.Currency = strings.ToUpper(strings.TrimSpace(scope.Currency))
    if len(scope.Currency) != 3 { return OperationsScope{}, fmt.Errorf("currency must be a three-letter code") }
    return scope, nil
}
~~~

SQL 先以 upstream_cost_attempts.id 建 attempts CTE，再 JOIN effective transaction；禁止 JOIN account_groups 或按账号当前 group 展开。matched 为 matched/manual，pending 为 pending/exception，ExceptionAttempts 仅数 completed_at 不晚于 now-10m 的未对齐记录。日聚合用 date_trunc(day, completed_at AT TIME ZONE timezone)。

- [ ] **Step 4: 验证并提交**

Run:

~~~bash
(cd relay-ops-service && go test ./internal/reconciliation ./internal/store -count=1)
git add relay-ops-service/internal/reconciliation/operations.go relay-ops-service/internal/reconciliation/operations_test.go relay-ops-service/internal/reconciliation/model.go relay-ops-service/internal/store/reconciliation.go relay-ops-service/internal/store/reconciliation_test.go
git diff --cached --check
git commit -m "feat: add scoped operations reconciliation summaries"
~~~

Expected: PASS；跨组账号不翻倍，失败但收费 attempt 入成本与覆盖分母。

### Task 3: 公开受保护的全站/分组运营与历史 API

**Files:**
- Modify: relay-ops-service/internal/http/server.go
- Modify: relay-ops-service/internal/http/reconciliation.go
- Test: relay-ops-service/internal/http/server_test.go
- Modify: relay-ops-service/internal/reconciliation/model.go
- Modify: relay-ops-service/internal/store/reconciliation.go

**Interfaces:**

~~~text
GET /relay-ops/api/reconciliation/operations?group_id=&account_id=&start=&end=&currency=&timezone=
GET /relay-ops/api/reconciliation/operations/history?group_id=&account_id=&start=&end=&currency=&timezone=
~~~

Existing summary stays temporarily backward compatible and delegates to global OperationsScope.

- [ ] **Step 1: 写 HTTP 失败测试**

~~~go
request := authenticatedJSONRequest(http.MethodGet,
    "/relay-ops/api/reconciliation/operations?group_id=3&start=2026-08-01T00:00:00Z&end=2026-08-02T00:00:00Z", nil)
response := serve(t, dependencies, request)
require.Equal(t, http.StatusOK, response.Code)
~~~

断言响应含 scope.group_id、profit_margin、unattributed_attempts。group_id=0 返回 400 INVALID_OPERATIONS_SCOPE，未认证仍被 admin middleware 拒绝，history 默认近 30 天且最多 366 天。

- [ ] **Step 2: 运行失败测试**

Run:

~~~bash
(cd relay-ops-service && go test ./internal/http -run 'Test.*Operations' -count=1)
~~~

Expected: FAIL/404。

- [ ] **Step 3: 接线 server dependency 与严格 scope parser**

server dependency 增加：

~~~go
ReadOperationsSummary(context.Context, reconciliation.OperationsScope) (reconciliation.OperationsSummary, error)
ListOperationsDaily(context.Context, reconciliation.OperationsScope) ([]reconciliation.OperationsDailyRow, error)
~~~

operationsScope(request) 只接受 RFC3339、正整数 ID、三位 currency 和有效 IANA timezone。省略 summary 时间时取业务时区当日 [00:00, now)；省略 history 取最近 30 自然日。金钱和比例使用 decimal string JSON，避免 JS 浮点损失。

- [ ] **Step 4: 验证并提交**

Run:

~~~bash
(cd relay-ops-service && go test ./internal/http ./internal/reconciliation ./internal/store -count=1)
git add relay-ops-service/internal/http/server.go relay-ops-service/internal/http/reconciliation.go relay-ops-service/internal/http/server_test.go relay-ops-service/internal/reconciliation/model.go relay-ops-service/internal/store/reconciliation.go
git diff --cached --check
git commit -m "feat: expose scoped operations ledger APIs"
~~~

### Task 4: 在 Sub2API 保存每组评分规则与分组开放语义

**Files:**
- Create: upstream/sub2api/backend/migrations/188_account_monitor_group_score_weights.sql
- Modify: upstream/sub2api/backend/internal/service/account_monitor_types.go
- Modify: upstream/sub2api/backend/internal/service/account_monitor_service.go
- Modify: upstream/sub2api/backend/internal/repository/account_monitor_repo.go
- Modify: upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go
- Modify: upstream/sub2api/backend/internal/server/routes/admin.go
- Test: upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go
- Test: upstream/sub2api/backend/internal/service/account_monitor_service_test.go
- Test: upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go

**Interfaces:**

~~~go
type AccountMonitorScoreWeights struct { Cost, Success, TTFT, Latency int; UpdatedBy int64; UpdatedAt time.Time }
type AccountMonitorGroup struct { ID int64; Name string; RateMultiplier float64; CustomerVisible bool; NativeOrder int; ScoreWeights AccountMonitorScoreWeights }
func (s *AccountMonitorService) GetGroupScoreWeights(context.Context, int64) (AccountMonitorScoreWeights, error)
func (s *AccountMonitorService) UpdateGroupScoreWeights(context.Context, int64, int64, AccountMonitorScoreWeights) (AccountMonitorScoreWeights, error)
func (s *AccountMonitorService) ResetGroupScoreWeights(context.Context, int64, int64) (AccountMonitorScoreWeights, error)
~~~

Routes: GET/PUT/DELETE /api/v1/admin/account-monitors/groups/:group_id/score-weights. DELETE restores defaults, not group data.

- [ ] **Step 1: 写失败测试**

覆盖持久化 15/45/20/20、更新 20/40/20/20、reset 返回默认；服务测试：

~~~go
_, err := svc.UpdateGroupScoreWeights(ctx, 7, 3, AccountMonitorScoreWeights{Cost: 20, Success: 30, TTFT: 20, Latency: 20})
require.ErrorContains(t, err, "sum to 100")
~~~

再构造 active 且非 exclusive 与 exclusive/inactive group，断言 projection 明确 customer_visible，不把后者作为 fault。

- [ ] **Step 2: 运行失败测试**

Run:

~~~bash
(cd upstream/sub2api/backend && go test ./internal/repository ./internal/service ./internal/handler/admin -run 'Test.*AccountMonitor.*(Score|Group)' -count=1)
~~~

Expected: FAIL。

- [ ] **Step 3: 实现独立表、验证和 handler**

~~~sql
CREATE TABLE IF NOT EXISTS account_monitor_group_score_weights (
    group_id BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    cost_weight SMALLINT NOT NULL CHECK (cost_weight >= 0),
    success_weight SMALLINT NOT NULL CHECK (success_weight >= 0),
    ttft_weight SMALLINT NOT NULL CHECK (ttft_weight >= 0),
    latency_weight SMALLINT NOT NULL CHECK (latency_weight >= 0),
    updated_by BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (cost_weight + success_weight + ttft_weight + latency_weight = 100)
);
~~~

~~~go
var DefaultAccountMonitorScoreWeights = AccountMonitorScoreWeights{Cost: 15, Success: 45, TTFT: 20, Latency: 20}
~~~

用服务层校验非负且等于 100；List 返回原生 multiplier、公开状态与 NativeOrder。不写 scheduler_score_weights、account priority 或 groups.sort_order。

- [ ] **Step 4: 验证并提交**

Run:

~~~bash
(cd upstream/sub2api/backend && go test ./internal/repository ./internal/service ./internal/handler/admin -run 'Test.*AccountMonitor' -count=1)
git add upstream/sub2api/backend/migrations/188_account_monitor_group_score_weights.sql upstream/sub2api/backend/internal/service/account_monitor_types.go upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/repository/account_monitor_repo.go upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go upstream/sub2api/backend/internal/server/routes/admin.go upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go upstream/sub2api/backend/internal/service/account_monitor_service_test.go upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go
git diff --cached --check
git commit -m "feat: persist account monitor group score weights"
~~~

### Task 5: 产生每组可解释的账号质量证据

**Files:**
- Modify: upstream/sub2api/backend/internal/service/account_monitor_types.go
- Modify: upstream/sub2api/backend/internal/service/account_monitor_service.go
- Modify: upstream/sub2api/backend/internal/repository/account_monitor_repo.go
- Test: upstream/sub2api/backend/internal/service/account_monitor_service_test.go
- Test: upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go

**Interfaces:**

~~~go
type AccountMonitorQualityEvidence struct {
    Source string // group, global_fallback, stale
    SampleCount int
    SuccessRate float64
    TTFTP50MS, LatencyP95MS *float64
    ObservedAt time.Time
}
type AccountMonitorGroupAccount struct {
    AccountMonitorAccount
    QualityScore *float64
    GroupRank *int
    Eligible bool
    Evidence AccountMonitorQualityEvidence
}
~~~

成本优势只在服务端算：

~~~go
costAdvantage := clamp(float64(weights.Cost)*(group.RateMultiplier-account.BillingRateMultiplier())/group.RateMultiplier, 0, float64(weights.Cost))
~~~

- [ ] **Step 1: 写失败测试**

同一账号倍率 0.02 放入 1.00x 与 0.10x 两组，各自不同权重；断言第一组成本优势/总分高，priority 相同且不参与总分。覆盖组内样本不足使用 global_fallback、探测过期 Eligible=false、关闭且无账号 operational_state=closed。

- [ ] **Step 2: 运行失败测试**

Run:

~~~bash
(cd upstream/sub2api/backend && go test ./internal/service ./internal/repository -run 'Test.*AccountMonitor.*(Quality|Evidence|Closed)' -count=1)
~~~

Expected: FAIL。

- [ ] **Step 3: 实现证据、资格与稳定排序**

优先用当前组最近五分钟真实请求的成功率、TTFT P50、总耗时 P95；低于命名常量 AccountMonitorGroupEvidenceMinSamples 时才用账户全局样本并标 global_fallback。评分函数必须纯函数，输入为 group multiplier、account multiplier、weights、evidence；不得读写 priority。正常区按 quality_score DESC, account_id ASC；不可用/暂停/成本不合格/过期证据单独返回，关闭组不制造红色故障。

- [ ] **Step 4: 验证并提交**

Run:

~~~bash
(cd upstream/sub2api/backend && go test ./internal/service ./internal/repository -run 'Test.*AccountMonitor' -count=1)
git add upstream/sub2api/backend/internal/service/account_monitor_types.go upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/repository/account_monitor_repo.go upstream/sub2api/backend/internal/service/account_monitor_service_test.go upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go
git diff --cached --check
git commit -m "feat: add group-aware account quality evidence"
~~~

### Task 6: 重建前端合同与深色账号卡片

**Files:**
- Modify: upstream/sub2api/frontend/src/api/admin/accountMonitor.ts
- Modify: upstream/sub2api/frontend/src/api/admin/reconciliation.ts
- Modify: upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue
- Create: upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.vue
- Create: upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorLedgerHistoryDrawer.vue
- Test: upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts
- Test: upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts

**Interfaces:**

~~~ts
export type OperationsScopeParams = { group_id?: number; account_id?: number; start?: string; end?: string; currency?: string; timezone?: string }
export async function operations(params: OperationsScopeParams = {}): Promise<OperationsSummary>
export async function history(params: OperationsScopeParams = {}): Promise<{ items: OperationsDailyRow[] }>
~~~

- [ ] **Step 1: 写组件失败测试**

传入 upstream_cost 0.02、user_charge 1.00、paper_profit 0.98、profit_margin 0.98，断言卡片含“质量评分”“组内第 1”“全局调度优先级”“上游真实扣费”“用户实际计费”“纸面利润”“利润率”。coverage_known 为 false 显示“待对账”而非利润率；关闭组显示“已关闭”且无故障红边。Dialog 填 15/45/20/19 禁用保存，reset 调 API；历史抽屉默认 30 天按日显示账务。

- [ ] **Step 2: 运行失败测试**

Run:

~~~bash
pnpm --dir upstream/sub2api/frontend test -- AccountMonitorCard AccountMonitorGroupScoreDialog
~~~

Expected: FAIL。

- [ ] **Step 3: 实现卡片和编辑器**

保留 monitor-card、深色双列布局、状态边线和绿色监测柱状序列；只有明确不可用才红，弱但可用仍用绿色深浅，无证据/过期灰色。优先级标签改为“全局调度优先级”，仍 emit updatePriority，不重排质量分。投入产出只读 relay 真实数据；覆盖未知或 pending 非零显示“待对账”。评分弹窗只给四个直接输入、合计校验、保存、恢复默认。

- [ ] **Step 4: 验证并提交**

Run:

~~~bash
pnpm --dir upstream/sub2api/frontend test -- AccountMonitorCard AccountMonitorGroupScoreDialog
pnpm --dir upstream/sub2api/frontend typecheck
git add upstream/sub2api/frontend/src/api/admin/accountMonitor.ts upstream/sub2api/frontend/src/api/admin/reconciliation.ts upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.vue upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorLedgerHistoryDrawer.vue upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorGroupScoreDialog.spec.ts
git diff --cached --check
git commit -m "feat: show account monitor quality and return cards"
~~~

### Task 7: 重建全站首屏与分组 Tab 工作流

**Files:**
- Modify: upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue
- Modify: upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts
- Modify: upstream/sub2api/frontend/src/api/admin/index.ts

**Interfaces:**
- Default selected scope is all;默认业务时区今日，同时请求全站累计。
- Current group gets its own today/lifetime ledger; card in a group requests that group/account ledger.
- Tabs sort only locally by rate_multiplier DESC, native_order ASC.

- [ ] **Step 1: 写失败测试**

~~~ts
expect(wrapper.get('[data-test="operations-overview"]').text()).toContain('全站经营总览')
expect(wrapper.findAll('[data-test="monitor-group-tab"]')[0].text()).toContain('全站')
await wrapper.get('[data-test="monitor-group-tab-3"]').trigger('click')
expect(reconciliationOperations).toHaveBeenLastCalledWith(expect.objectContaining({ group_id: 3 }))
~~~

覆盖倍率排序、同倍率 NativeOrder、权重保存只刷新当前组、priority 修改后仍 quality-score 降序、关闭组空账号、真实失败覆盖抑制、点击“账务历史”按当前 scope 读取数据。

- [ ] **Step 2: 运行失败测试**

Run:

~~~bash
pnpm --dir upstream/sub2api/frontend test -- AccountMonitorView
~~~

Expected: FAIL，因为当前页面平铺分组、使用旧 summary，并写死 30/30/20/20。

- [ ] **Step 3: 单一 view-model 改写数据流**

删除浏览器内硬编码评分和 qualityScore。实现 selectedScope、globalToday、globalLifetime、selectedGroupToday、selectedGroupLifetime。首屏固定分“经营结果”和“服务健康”；选组后把全站缩为固定摘要。账务历史只做轻量按钮打开抽屉。Tab 只做本地稳定排序：

~~~ts
const orderedGroups = computed(() => [...projection.value.groups].sort((a, b) =>
  b.rate_multiplier - a.rate_multiplier || a.native_order - b.native_order,
))
~~~

刷新全部先运行服务探测，再调用真实账单刷新，最后重新取两个投影；不把探测刷新当账务刷新。

- [ ] **Step 4: 验证并提交**

Run:

~~~bash
pnpm --dir upstream/sub2api/frontend test -- AccountMonitorView AccountMonitorCard AccountMonitorGroupScoreDialog
pnpm --dir upstream/sub2api/frontend typecheck
pnpm --dir upstream/sub2api/frontend build
git add upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts upstream/sub2api/frontend/src/api/admin/index.ts
git diff --cached --check
git commit -m "feat: redesign account monitor operations view"
~~~

Expected: PASS；没有“运行正常”误导标题、没有硬编码 30/30/20/20、没有依据账号多组成员关系计算全站账。

### Task 8: 独立审查、部署前回归和生产证据

**Files:**
- Modify: docs/project/project-progress.md
- Create: docs/superpowers/reports/2026-08-01-account-monitor-group-operations-local-verification.md
- Create: docs/superpowers/reports/2026-08-01-account-monitor-group-operations-production-verification.md

- [ ] **Step 1: 运行跨服务回归**

Run:

~~~bash
(cd relay-ops-service && go test ./internal/reconciliation ./internal/store ./internal/http ./internal/sub2api -count=1)
(cd upstream/sub2api/backend && go test ./internal/repository ./internal/service ./internal/handler/admin -run 'Test.*AccountMonitor' -count=1)
pnpm --dir upstream/sub2api/frontend test -- AccountMonitorView AccountMonitorCard AccountMonitorGroupScoreDialog
pnpm --dir upstream/sub2api/frontend typecheck
pnpm --dir upstream/sub2api/frontend build
git diff --check
~~~

本地报告记录每条命令、fixture、日期和尚未满足的线上条件；只能称“准备完成/待部署”。

- [ ] **Step 2: 独立任务审查与整分支审查**

每个 Task 完成后由新的 reviewer 检查其 diff；整分支 reviewer 逐项核验：唯一 Attempt、NULL 差额、failed-but-billed、10 分钟异常、关闭组抑制、真实失败覆盖、权重和 100、priority 不参与评分、无 scheduler 写入、卡片真实成本来源和深色视觉复用。将结果写入本地报告。

- [ ] **Step 3: 推送、受控部署和线上验证**

部署后以只读管理员会话验证：

~~~text
1. 全站 operations API 含真实成本、覆盖率和未归属差额。
2. group 3 + group 8 + 未归属与全站一致，账号跨组不翻倍。
3. 倍率排序 Tab、每组权重保存/重置、priority 编辑均生效。
4. 关闭分组不重复告警；开放组无可用账号仍异常。
5. 今日/累计和账务历史抽屉与 API 一致。
~~~

生产报告记录时间、版本/commit、匿名化响应摘要、截图和结论。只有推送、部署、所有线上验证均成功时，才将项目总账此事项移动到“生产工程代码/配置已部署并验证”；否则维持“进行中”。

- [ ] **Step 4: 提交验证文档**

Run:

~~~bash
git add docs/project/project-progress.md docs/superpowers/reports/2026-08-01-account-monitor-group-operations-local-verification.md docs/superpowers/reports/2026-08-01-account-monitor-group-operations-production-verification.md
git diff --cached --check
git commit -m "docs: record account monitor operations verification"
~~~

## Self-Review

| 规格 | 任务 |
|---|---|
| 全站经营、今日默认/累计可见、历史抽屉 | 2、3、6、7 |
| 分组 Tab 按倍率排序、只影响展示 | 4、7 |
| 关闭分组语义、不设分组质量分 | 4、5、7 |
| 每组独立 15/45/20/20 直接配置 | 4、6、7 |
| 成本优势与实际利润率分离 | 2、5、6 |
| 跨组不重复、未归属差额 | 1、2、3 |
| failed/retry 成本、覆盖率、10 分钟异常 | 1、2、3 |
| 深色卡片、服务质量/投入产出、priority 可改 | 5、6、7 |
| 项目总账与推送部署线上验证门槛 | 1、8 |

本计划没有未定项、“稍后实现”或“按需处理”占位。归属链为 UsageLog.GroupID → AttemptInput.GroupID → Attempt.GroupID → OperationsScope.GroupID；账务 summary 只由 relay-ops 生成，评分规则和健康证据只由 Sub2API 生成，前端只组合显示。

## Execution Handoff

Plan complete and saved to docs/superpowers/plans/2026-08-01-account-monitor-group-operations-implementation-plan.md.

项目约束要求采用 **Subagent-Driven** 执行：每个任务使用新的实施 agent，任务结束后独立审查，全部任务结束后整分支审查。Task 1 必须等待真实账单采集任务的接口稳定，避免并行修改 importer/reconciliation。
