# T11-R1 Sub 原生计费聚合经营页纠偏 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 使现有管理员经营页的六项经营指标只由 `usage_logs` 官方值聚合，保留当前页面结构、时间范围、状态、刷新和用户未消费余额卡片，删除已批准的人工覆盖与异常入口。

**Architecture:** 在现有 `usageLogRepository` 上新增窄的只读 `AccountFinancialUsageReader`，以 `(group_id, account_id)` 为互斥基础粒度在 PostgreSQL 内完成聚合，并同时返回活动账号/分组元数据与原有用户余额快照。`AccountFinancialService.GetReport` 仅消费该快照折叠全站、分组、全站账号和分组账号；旧 `AccountFinancialRepository` 仍仅供异常、复核和人工写接口使用。前端保留同一 GET 路径，展示六项 USD 指标和独立 CNY 余额卡，并完整呈现 loading/data/empty/error/retry/refreshing。

**Tech Stack:** Go 1.x、Ent + `database/sql`、PostgreSQL、Gin、Google Wire；Vue 3 `<script setup>`、TypeScript、Vitest + Vue Test Utils、pnpm。

## Global Constraints

- 实现基线固定为 `main@6289c22a31a9c6a53836e2086f2f356c13be1c1b`，候选分支为 `codex/t11-r1-native-accounting-profitability`。
- 本文件按移交指定使用未来日期文件名 `2026-08-16`；当前日期为 2026-08-15。
- 六项经营指标为 `requests`、`tokens`、`cost`、`user_cost`、`profit`、`margin`，金额保持 USD。
- `requests = COUNT(*)`。
- `tokens = SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens)`，各项用 `COALESCE(..., 0)` 保护。
- `cost = SUM(COALESCE(account_cost, COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)))`。
- `user_cost = SUM(actual_cost)`，`profit = user_cost - cost`，`user_cost == 0` 时 `margin = null`。
- 时间范围全部为 `[from,to)`：今日从 Asia/Shanghai 00:00 到生成时刻，24h 为滚动 24 小时，7d/31d 为北京当日加前 6/30 个自然日。
- 聚合不得应用 `account_financial_settings.enabled_at`，不得读取 evidence、review、OAuth 日成本或 today override。
- 保留 `user_unconsumed_balance_cny`的现有只读 CNY 语义；它不参与 `range`、profit、margin 或用量守恒。
- 保留全站摘要、分组 Tab、账号行、今日/24h/7d/31d、手动刷新、60 秒刷新以及 loading/data/empty/error/retry/refreshing。
- 删除白色人工营收/成本/OAuth 成本输入、“今日覆盖”操作、“异常流水”卡、账号异常数量/操作与成本异常页跳转；不删除未消费余额卡。
- 保留历史 T03/T03-R1 表、数据、异常页、API、写入、审计和测试证据；不做破坏性删除。
- 不新增 schema/migration、配置、环境变量、依赖、汇率、估算、补查、重试、回填、计费写入或 GitHub Actions。
- 本窗口不修改根 `main`、`docs/project/project-progress.md`、`docs/project/native-sub-task-package-queue.md`、发布证据或生产状态，不合并、推送或部署。
- 计划批准后，每个任务由 fresh implementer 实施，紧接一个独立只读 reviewer；全部任务后再由 fresh reviewer 完成全分支审查。
- 如发现范围漂移、冲突、不可逆数据操作、需要 `downtime_required=true` 或本任务无法解决的问题，立即停止并报告。

---

## File Responsibility Map

- `upstream/sub2api/backend/internal/service/account_financial.go`: 定义原生经营快照的窄接口/类型，保留历史异常与写入类型，将 `GetReport` 替换为原生 pair 行折叠。
- `upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go`: 实现 `[from,to)` 原生 SQL 聚合与账号/分组/用户余额只读快照。
- `upstream/sub2api/backend/internal/repository/usage_log_repo_stats_test.go`: 用 SQL mock 锁定聚合公式、参数、扫描和错误传递，并防止引用禁止表。
- `upstream/sub2api/backend/internal/repository/usage_log_repo_stats_integration_test.go`: 用真实 PostgreSQL 验证 NULL/fallback、半开区间、跨分组、未归属、软删除身份和余额合同。
- `upstream/sub2api/backend/internal/repository/usage_log_repo.go`: 仅在需要复用现有 `usageLogRepository` 构造路径时增加窄 reader provider，不扩展庞大 `UsageLogRepository` 接口。
- `upstream/sub2api/backend/internal/repository/wire.go`: 注册窄 `AccountFinancialUsageReader` provider。
- `upstream/sub2api/backend/internal/service/wire.go`: 将窄 reader 注入 `AccountFinancialService`，保留 audit/repository 供历史写接口。
- `upstream/sub2api/backend/cmd/server/wire_gen.go`: 同步 Wire 生成装配；必须与 provider 签名一致。
- `upstream/sub2api/backend/internal/service/account_financial_test.go`: 重建 `GetReport` 单测，锁定四个时间范围、四层折叠、守恒、利润/利润率、历史回退名和 fail-closed；原异常/审计测试保留。
- `upstream/sub2api/backend/internal/handler/admin/account_financial_handler_test.go`: 锁定合法/非法 range、500 和新响应字段，不删除写 API 的现有测试。
- `upstream/sub2api/frontend/src/api/admin/accountFinancial.ts`: 定义六字段 USD 合同、余额字段和新旧响应 normalization；历史写 API export 保留但页面不再调用。
- `upstream/sub2api/frontend/src/api/__tests__/admin.accountFinancial.spec.ts`: 验证 snake_case/PascalCase/旧 CNY 别名的 normalization 与 `margin=null`。
- `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`: 呈现六项 USD 指标、保留 CNY 余额卡、删除已批准控件，完成加载/空/错误/重试/刷新和最新请求胜出。
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`: 验证七张全站卡、六张分组卡、七列账号表、禁止入口消失和所有页面状态。
- `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`: 中文请求/Token/账号计费/用户扣费/利润/利润率、loading/empty/refreshing 文案。
- `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`: 英文对应文案。
- `docs/handoffs/2026-08-16-t11-r1-native-accounting-profitability-candidate-handoff.md`: 候选交接，只记录基线、提交、测试、未验证项、迁移/配置、停机、回滚与风险，并只读引用根目录受保护的源 handoff；不复制、覆盖或提交源 handoff，不修改根总账/队列。

### Task 1: 原生 usage pair 聚合 reader

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_financial.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go`
- Create: `upstream/sub2api/backend/internal/repository/usage_log_repo_stats_test.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_stats_integration_test.go`

**Interfaces:**
- Consumes: 现有 `usageLogRepository{client *ent.Client, sql sqlExecutor}` 与 `newUsageLogRepositoryWithSQL(client, sqlq)`。
- Produces:

```go
type AccountFinancialUsageReader interface {
	ReadAccountFinancialUsage(ctx context.Context, from, to time.Time) (*AccountFinancialUsageSnapshot, error)
}

type AccountFinancialUsageSnapshot struct {
	Accounts       []AccountFinancialUsageAccount
	Groups         []AccountFinancialUsageGroup
	Rows           []AccountFinancialUsageRow
	UserBalanceCNY float64
}

type AccountFinancialUsageAccount struct {
	ID       int64
	Name     string
	Type     string
	Platform string
	Active   bool
}

type AccountFinancialUsageGroup struct {
	ID     int64
	Name   string
	Active bool
}

type AccountFinancialUsageRow struct {
	GroupID        *int64
	GroupName      string
	AccountID      int64
	AccountName    string
	AccountType    string
	AccountPlatform string
	Requests       int64
	Tokens         int64
	Cost           float64
	UserCost       float64
}
```

- Produces constructor: `func NewAccountFinancialUsageReader(client *ent.Client, sqlDB *sql.DB) service.AccountFinancialUsageReader` returning `newUsageLogRepositoryWithSQL(client, sqlDB)`.
- Invariant: `Rows` contains exactly one row per in-range `(group_id, account_id)` pair; active zero-usage accounts/groups exist only in `Accounts`/`Groups`; historical identities may exist in metadata and rows but cannot drop money.

- [ ] **Step 1: Add compile-time contract tests for the narrow reader**

In `usage_log_repo_stats_test.go`, add a compile-time assertion and a SQL-mock test fixture around `newUsageLogRepositoryWithSQL`:

```go
var _ service.AccountFinancialUsageReader = (*usageLogRepository)(nil)

func TestReadAccountFinancialUsageUsesHalfOpenNativeAggregation(t *testing.T) {
	from := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	// Expect the pair query to receive exactly from and to, return one row,
	// and assert requests/tokens/cost/user_cost plus active metadata/balance.
}
```

The expected SQL must require all of these fragments:

```sql
ul.created_at >= $1 AND ul.created_at < $2
GROUP BY ul.group_id, ul.account_id
COUNT(*)
COALESCE(SUM(
  COALESCE(ul.input_tokens, 0)
  + COALESCE(ul.output_tokens, 0)
  + COALESCE(ul.cache_creation_tokens, 0)
  + COALESCE(ul.cache_read_tokens, 0)
), 0)
COALESCE(SUM(COALESCE(
  ul.account_cost,
  COALESCE(ul.account_stats_cost, ul.total_cost)
    * COALESCE(ul.account_rate_multiplier, 1)
)), 0)
COALESCE(SUM(COALESCE(ul.actual_cost, 0)), 0)
```

- [ ] **Step 2: Run the focused repository unit test and confirm RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestReadAccountFinancialUsage' -count=1
```

Expected: FAIL because `AccountFinancialUsageReader`, snapshot types, constructor, and method do not exist.

- [ ] **Step 3: Define the narrow service contract without changing historical mutation interfaces**

Add the exact interfaces/types above beside the report types in `account_financial.go`. Do not add the method to `UsageLogRepository` or `AccountFinancialRepository`. Keep `AccountFinancialSnapshot*`, evidence/review/daily-value types and all mutation methods untouched because historical endpoints still require them.

- [ ] **Step 4: Implement the read-only snapshot transaction and pair query**

Implement `ReadAccountFinancialUsage` in `usage_log_repo_stats.go` with a read-only repeatable-read transaction. Within that transaction:

1. query all accounts and groups with soft-delete bypass so historical names can be resolved;
2. mark `Active` from `DeletedAt == nil`;
3. query non-deleted users and sum `balance` into `UserBalanceCNY` without applying `from`, `to`, or `enabled_at`;
4. execute the approved pair aggregate SQL with `[from,to)`;
5. use `LEFT JOIN accounts` and `LEFT JOIN groups` or the already loaded maps to fill names/type/platform, leaving stable ID fallback to the service;
6. check `rows.Err()`, close rows, and commit only after every read succeeds.

The query must not contain any of:

```text
account_financial_settings
usage_upstream_cost_evidence
usage_cost_reviews
account_daily_financial_values
oauth
override
```

- [ ] **Step 5: Run repository unit tests and confirm GREEN**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestReadAccountFinancialUsage|TestAccountFinancial' -count=1
```

Expected: PASS; the SQL mock sees `[from,to)`, scans a nullable group, preserves balance, and propagates query/scan/commit errors.

- [ ] **Step 6: Add real PostgreSQL integration cases**

Extend `usage_log_repo_stats_integration_test.go` with `TestUsageLog_ReadAccountFinancialUsage_NativeContract`. Create through existing helpers:

- one active account with two in-range rows using `account_cost`;
- the same account in a second group using `account_stats_cost * account_rate_multiplier`;
- one row with `account_cost/account_stats_cost` NULL so `total_cost * multiplier` is used;
- one `group_id NULL` row;
- rows exactly at `from` and `to` to prove lower-inclusive/upper-exclusive behavior;
- one soft-deleted account/group with in-range usage;
- one active zero-usage account/group;
- active and soft-deleted users with balances to prove only active balances are summed.

Assert exact requests, four-token sum, cost, user cost, pair cardinality, historical metadata, active metadata, unassigned pair, and unchanged CNY balance.

- [ ] **Step 7: Run the PostgreSQL integration test**

Run when the repository integration database is available:

```bash
cd upstream/sub2api/backend
go test -tags=integration ./internal/repository -run 'TestUsageLog_ReadAccountFinancialUsage_NativeContract' -count=1
```

Expected: PASS. If the environment lacks PostgreSQL/integration prerequisites, record the exact command and failure as an unverified item; do not substitute SQLite or a mock for this gate.

- [ ] **Step 8: Refactor only for local clarity and rerun the task suite**

Extract only small scan/helpers needed to keep the method readable. Do not introduce a new repository abstraction, cache, retry, estimate, migration, or index.

Run:

```bash
cd upstream/sub2api/backend
gofmt -w internal/service/account_financial.go internal/repository/usage_log_repo.go internal/repository/usage_log_repo_stats.go internal/repository/usage_log_repo_stats_test.go internal/repository/usage_log_repo_stats_integration_test.go
go test ./internal/repository -run 'TestReadAccountFinancialUsage|TestAccountFinancial|TestAccountWindowStats' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit Task 1**

```bash
git add upstream/sub2api/backend/internal/service/account_financial.go \
  upstream/sub2api/backend/internal/repository/usage_log_repo.go \
  upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go \
  upstream/sub2api/backend/internal/repository/usage_log_repo_stats_test.go \
  upstream/sub2api/backend/internal/repository/usage_log_repo_stats_integration_test.go
git commit -m "feat: add native financial usage aggregation"
```

- [ ] **Step 10: Independent Task 1 review gate**

Dispatch a fresh read-only reviewer. It must compare the committed diff with the approved formulas, inspect SQL for forbidden table names and `[from,to)`, confirm balance isolation, and rerun the focused unit test. Any finding returns to a fresh implementer for correction and a new review; do not begin Task 2 until approved.

### Task 2: Service folding, API contract, and dependency injection

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_financial.go`
- Modify: `upstream/sub2api/backend/internal/service/account_financial_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_financial_audit_test.go` only for constructor arguments
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_financial_handler_test.go`
- Modify: `upstream/sub2api/backend/internal/repository/wire.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/backend/cmd/server/wire_gen.go`

**Interfaces:**
- Consumes: `AccountFinancialUsageReader.ReadAccountFinancialUsage(ctx, from, to)` from Task 1 and the existing `AccountFinancialRepository` for historical endpoints.
- Produces constructors:

```go
func NewAccountFinancialService(
	repo AccountFinancialRepository,
	usageReader AccountFinancialUsageReader,
	now func() time.Time,
) *AccountFinancialService

func NewAccountFinancialServiceWithAudit(
	repo AccountFinancialRepository,
	usageReader AccountFinancialUsageReader,
	now func() time.Time,
	audit *AccountFinancialAudit,
) *AccountFinancialService
```

- Produces response values:

```go
type FinancialAmounts struct {
	Requests int64    `json:"requests"`
	Tokens   int64    `json:"tokens"`
	Cost     float64  `json:"cost"`
	UserCost float64  `json:"user_cost"`
	Profit   float64  `json:"profit"`
	Margin   *float64 `json:"margin"`
	Revenue  float64  `json:"revenue"` // deprecated alias of UserCost
	Expense  float64  `json:"expense"` // deprecated alias of Cost
}
```

- `AccountFinancialReport` adds `Currency string 'json:"currency"'` and retains `UserBalanceCNY float64 'json:"user_unconsumed_balance_cny"'`; report/account/group structs receive stable snake_case JSON tags and no longer expose exception/completeness fields in `GetReport`.
- `AccountFinancialAccountReport` must expose `ID`, `Name`, `Type`, `Platform`, `Historical bool 'json:"historical"'`, and `Amounts`; `AccountFinancialGroupReport` must expose `ID`, `Name`, `Unassigned bool 'json:"unassigned"'`, `Historical bool 'json:"historical"'`, `Amounts`, and `Accounts`.
- Invariant: for every amounts object, `Revenue == UserCost`, `Expense == Cost`, `Profit == UserCost-Cost`, and `Margin == nil` iff `UserCost == 0`.

- [ ] **Step 1: Replace old GetReport tests with native snapshot RED tests**

Keep tests for `ListExceptions`, reviews, daily values, validation, and audit. Replace only tests whose subject is the old evidence-driven `GetReport` with a `financialUsageReaderStub`:

```go
type financialUsageReaderStub struct {
	snapshot *AccountFinancialUsageSnapshot
	err      error
	from     time.Time
	to       time.Time
	calls    int
}

func (r *financialUsageReaderStub) ReadAccountFinancialUsage(
	ctx context.Context, from, to time.Time,
) (*AccountFinancialUsageSnapshot, error) {
	r.calls++
	r.from, r.to = from, to
	return r.snapshot, r.err
}
```

Add focused tests named:

```text
TestAccountFinancialReportFoldsNativePairsAndConservesTotals
TestAccountFinancialReportPreservesActiveZeroRowsAndHistoricalUsage
TestAccountFinancialReportUsesNullMarginForZeroUserCost
TestAccountFinancialReportBuildsToday24H7D31DHalfOpenWindows
TestAccountFinancialReportFailsClosedWithoutUsageReader
TestAccountFinancialReportPropagatesUsageReaderError
TestAccountFinancialReportDoesNotReadLegacySnapshot
```

Use pair fixtures where one account spans two groups, one group is missing/soft-deleted, one row is unassigned, and one active account/group has zero usage. Assert the additive fields `requests/tokens/cost/user_cost/profit` conserve from pair rows to each group, whole-site account, and site summary; assert each layer recomputes its own margin from its folded `user_cost` and `profit`. Stable fallbacks are `账号 #<id>` / `分组 #<id>`, unassigned is explicit, and `UserBalanceCNY` is unchanged.

- [ ] **Step 2: Run the service tests and confirm RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountFinancialReport' -count=1
```

Expected: FAIL because the service has no usage reader and still applies evidence/review/override logic.

- [ ] **Step 3: Introduce native report DTOs and a single amounts finalizer**

Change only report-facing DTO fields to the interface above. Add:

```go
func finalizeFinancialAmounts(v *FinancialAmounts) {
	v.Profit = v.UserCost - v.Cost
	v.Revenue = v.UserCost
	v.Expense = v.Cost
	if v.UserCost == 0 {
		v.Margin = nil
		return
	}
	margin := v.Profit / v.UserCost
	v.Margin = &margin
}
```

Do not rename historical mutation/evidence structs containing `CNY`; they remain outside the new report contract.

- [ ] **Step 4: Replace GetReport with deterministic pair folding**

Make `GetReport`:

1. calculate one `now := s.now()` and exact `[from,to)`;
2. reject a nil `usageReader` with a non-nil error;
3. call `ReadAccountFinancialUsage(ctx, from, now)` exactly once;
4. seed active accounts and groups as zero rows/tabs;
5. fold every pair into site summary, group amounts, whole-site account by `account_id`, and group account by `(group_id, account_id)`;
6. create the unassigned group only when a NULL-group pair exists;
7. retain historical account/group rows only when a pair references them;
8. use metadata name/type/platform first and stable ID fallback second;
9. finalize every amounts object only after accumulation;
10. return `GeneratedAt=now`, normalized range, `Currency="USD"`, and the snapshot balance.

The report path must not call `repo.ReadSnapshot`; leave that method and all historical endpoint methods available for `ListExceptions` and mutations.

- [ ] **Step 5: Run service tests and confirm GREEN**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountFinancial' -count=1
```

Expected: PASS, including historical exception/mutation tests and the new native report tests.

- [ ] **Step 6: Add handler JSON contract tests**

Update constructor calls and add assertions for a successful GET response containing:

```json
{
  "currency": "USD",
  "user_unconsumed_balance_cny": 90,
  "summary": {
    "requests": 2,
    "tokens": 10,
    "cost": 1.25,
    "user_cost": 2,
    "profit": 0.75,
    "margin": 0.375,
    "revenue": 2,
    "expense": 1.25
  }
}
```

Also assert invalid range remains 400, a nil/unavailable service remains 500, a reader error remains non-success, and historical mutation handler tests still pass.

- [ ] **Step 7: Run handler tests and confirm GREEN**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/handler/admin -run 'TestAccountFinancial' -count=1
```

Expected: PASS.

- [ ] **Step 8: Wire the narrow reader without broadening UsageLogRepository**

Register `NewAccountFinancialUsageReader` in `repository.ProviderSet`. Change:

```go
func ProvideAccountFinancialService(
	repo AccountFinancialRepository,
	usageReader AccountFinancialUsageReader,
	audit *AuditLogService,
) *AccountFinancialService {
	return NewAccountFinancialServiceWithAudit(repo, usageReader, time.Now, NewAccountFinancialAudit(audit))
}
```

Regenerate Wire with the repository's existing generation command if documented; otherwise make the minimal equivalent update in `cmd/server/wire_gen.go` so it constructs the narrow reader and passes it to the service. Do not alter unrelated providers.

- [ ] **Step 9: Verify backend compile and focused contracts**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestReadAccountFinancialUsage|TestAccountFinancial|TestAccountWindowStats' -count=1
go test ./internal/service -run 'TestAccountFinancial|TestAccountProfitability' -count=1
go test ./internal/handler/admin -run 'TestAccountFinancial|TestDashboardHandlerAccountProfitability' -count=1
go test ./cmd/server -run '^$' -count=1
```

Expected: PASS; production wiring compiles.

- [ ] **Step 10: Commit Task 2**

```bash
git add upstream/sub2api/backend/internal/service/account_financial.go \
  upstream/sub2api/backend/internal/service/account_financial_test.go \
  upstream/sub2api/backend/internal/service/account_financial_audit_test.go \
  upstream/sub2api/backend/internal/handler/admin/account_financial_handler_test.go \
  upstream/sub2api/backend/internal/repository/wire.go \
  upstream/sub2api/backend/internal/service/wire.go \
  upstream/sub2api/backend/cmd/server/wire_gen.go
git commit -m "feat: serve native accounting profitability"
```

- [ ] **Step 11: Independent Task 2 review gate**

Dispatch a fresh read-only reviewer. It must prove conservation from fixtures, inspect all four time windows, verify `margin=nil` only for zero user cost, confirm `GetReport` never reaches `ReadSnapshot`, confirm old mutations/audits remain, and compile production wiring. Findings return to a fresh implementer before Task 3.

### Task 3: Frontend API normalization and operating page states

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountFinancial.ts`
- Modify: `upstream/sub2api/frontend/src/api/__tests__/admin.accountFinancial.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`

**Interfaces:**
- Consumes: Task 2 response with snake_case native fields and deprecated `revenue`/`expense` aliases; also tolerates old PascalCase/CNY-shaped responses during rollback overlap.
- Produces:

```ts
export interface FinancialAmounts {
  requests: number
  tokens: number
  cost: number
  user_cost: number
  profit: number
  margin: number | null
}

export interface FinancialAccount {
  id: number
  name: string
  type: string
  platform: string
  historical: boolean
  amounts: FinancialAmounts
}

export interface FinancialGroup {
  id: number
  name: string
  unassigned: boolean
  historical: boolean
  amounts: FinancialAmounts
  accounts: FinancialAccount[]
}

export interface AccountFinancialReport {
  generated_at: string
  range: FinancialRange
  currency: 'USD'
  summary: FinancialAmounts
  accounts: FinancialAccount[]
  groups: FinancialGroup[]
  user_unconsumed_balance_cny: number
}
```

- Normalization precedence: `user_cost`/`UserCost`, then `revenue`/`RevenueCNY`; `cost`/`Cost`, then `expense`/`ExpenseCNY`/`CostCNY`; calculate missing profit/margin only from normalized `user_cost` and `cost`; never convert currency.
- UI selectors: `financial-loading`, `financial-empty`, `financial-load-error`, `financial-retry`, `financial-refreshing`, `summary-requests`, `summary-tokens`, `summary-cost`, `summary-user-cost`, `summary-profit`, `summary-margin`, `summary-unconsumed-balance`.

- [ ] **Step 1: Rewrite API normalization tests for the new contract**

Add tests that feed:

1. canonical snake_case native values;
2. PascalCase Go values;
3. old `RevenueCNY`/`CostCNY` values;
4. explicit `margin: null` and missing derived values;
5. `user_unconsumed_balance_cny` in CNY.

Assert exact normalized values, `currency: 'USD'`, no exception/completeness properties, and no FX conversion.

- [ ] **Step 2: Run the API test and confirm RED**

Run:

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/api/__tests__/admin.accountFinancial.spec.ts
```

Expected: FAIL because current types and normalization expose revenue/exception fields and omit requests/tokens/user_cost/currency.

- [ ] **Step 3: Implement minimal API types and normalization**

Replace report-facing interfaces with the exact types above. Keep `TodayOverridePayload`, `OAuthCostPayload`, `setTodayOverride`, and `setOAuthCost` exports for historical consumers, but remove them from the page imports and calls. Implement normalization with small `read`, `numberValue`, and `nullableNumber` helpers; normalize all summary/group/account amounts through one function.

- [ ] **Step 4: Run the API test and confirm GREEN**

Run:

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/api/__tests__/admin.accountFinancial.spec.ts
```

Expected: PASS.

- [ ] **Step 5: Rewrite component tests around the approved visible contract**

Use a native report fixture and add separate tests for:

- first load shows `financial-loading` and not fabricated zero cards;
- success shows seven whole-site cards: six native USD metrics plus CNY unconsumed balance;
- selected group shows six native metric cards and only backend-provided pair rows;
- table headers are Account, Requests, Tokens, Account cost, User charge, Profit, Margin;
- tiny USD values remain non-zero after formatting;
- `margin=null` renders `—`;
- successful zero-usage response shows `financial-empty` while range controls, refresh, groups, and balance remain available;
- error shows error/retry, not empty or zero success;
- refresh with existing data preserves cards/rows and shows `financial-refreshing`;
- two deferred requests resolved out of order leave the newest range visible;
- manual and 60-second refresh still call only `getReport`;
- no override inputs, exception card/column/button/action, router jump, incomplete notice, or unallocated notice exist;
- source has no `/xingqiao`, control-plane, `tab=cost-exceptions`, `setTodayOverride`, or `setOAuthCost` reference;
- zh/en locale keys resolve and a 390px container has no page-level forced width (table wrapper may scroll).

- [ ] **Step 6: Run the component test and confirm RED**

Run:

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts
```

Expected: FAIL on old six-card layout, old CNY operating values, missing loading/empty/refreshing, old inputs and exception jump.

- [ ] **Step 7: Implement explicit UI state and latest-request-wins behavior**

Use these state rules:

```ts
const loading = ref(false)
const refreshing = ref(false)
const loadError = ref('')
const hasLoaded = ref(false)
let requestSequence = 0

async function load() {
  const sequence = ++requestSequence
  if (hasLoaded.value) refreshing.value = true
  else loading.value = true
  loadError.value = ''
  try {
    const next = await adminAPI.accountFinancial.getReport({ range: activeRange.value })
    if (sequence !== requestSequence) return
    report.value = next
    hasLoaded.value = true
    if (activeScope.value.kind === 'group' && !selectedGroup.value) activeScope.value = { kind: 'all' }
  } catch {
    if (sequence === requestSequence) loadError.value = t('admin.accountProfitability.loadError')
  } finally {
    if (sequence === requestSequence) {
      loading.value = false
      refreshing.value = false
    }
  }
}
```

The empty predicate must use native usage only, for example summary requests/tokens and account/group amounts, and must not treat a non-zero CNY balance as usage data.

- [ ] **Step 8: Render the approved metrics and remove only approved controls**

Implement:

- USD formatter using `Intl.NumberFormat` with enough maximum fraction digits to avoid false `$0.00` for small non-zero values;
- separate existing-style CNY formatter for only `user_unconsumed_balance_cny`;
- number/compact formatter for requests and tokens;
- seven whole-site cards and six group cards;
- seven table columns;
- visible loading, empty, error/retry, and lightweight refreshing nodes;
- unchanged range/scope navigation and refresh interval.

Delete page imports/functions/templates for router exception jump and manual/OAuth overrides. Do not remove API modules, backend routes, exception pages, locale namespaces outside the page, or the CNY balance card.

- [ ] **Step 9: Update zh/en production copy**

Use equivalent localized concepts:

```text
requests / Requests
tokens / Tokens
accountCost / Account cost
userCost / User charge
profit / Profit
margin / Margin
loading / Loading profitability data…
refreshing / Refreshing…
empty / No usage in this range.
unconsumedBalance / User unconsumed balance
```

Remove the page's obsolete description of unknown/pending costs and override/unallocated messaging, while leaving historical cost-exception locale keys required by the separate exception UI.

- [ ] **Step 10: Run focused frontend tests and confirm GREEN**

Run:

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run \
  src/api/__tests__/admin.accountFinancial.spec.ts \
  src/views/admin/__tests__/AccountProfitabilityView.spec.ts
```

Expected: PASS.

- [ ] **Step 11: Run frontend static/build verification**

Run:

```bash
cd upstream/sub2api/frontend
pnpm typecheck
pnpm build
```

Expected: PASS with no new dependency or lockfile changes.

- [ ] **Step 12: Commit Task 3**

```bash
git add upstream/sub2api/frontend/src/api/admin/accountFinancial.ts \
  upstream/sub2api/frontend/src/api/__tests__/admin.accountFinancial.spec.ts \
  upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue \
  upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts \
  upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts \
  upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts
git commit -m "feat: show native profitability metrics"
```

- [ ] **Step 13: Independent Task 3 review gate**

Dispatch a fresh read-only reviewer. It must inspect rendered selectors and source, confirm exactly seven whole-site cards/six group cards/seven table columns, verify the balance card remains CNY, validate latest-request-wins and all state transitions, and ensure historical APIs/pages were not deleted. Findings return to a fresh implementer before Task 4.

### Task 4: Whole-candidate verification, forbidden-scope audit, and handoff

**Files:**
- Create: `docs/handoffs/2026-08-16-t11-r1-native-accounting-profitability-candidate-handoff.md`
- Reference read-only, never copy/edit/add: `/Users/gongtengxinwen/Documents/sub2api搭建/docs/handoffs/2026-08-16-native-accounting-profitability-handoff.md`
- Inspect only: every changed file since `6289c22a31a9c6a53836e2086f2f356c13be1c1b`

**Interfaces:**
- Consumes: approved commits from Tasks 1-3.
- Produces: a clean candidate branch and handoff with `baseline_sha`, `candidate_sha`, changed files, commands/results, unverified items, migration/config/dependency changes, `downtime_required`, rollback, remaining risks, and state `READY_FOR_ROOT_REVIEW`.
- Does not produce: root ledger/queue changes, merge authorization, push, deployment, or production verification.

- [ ] **Step 1: Run backend focused and neighboring regressions**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestReadAccountFinancialUsage|TestAccountFinancial|TestAccountWindowStats' -count=1
go test ./internal/service -run 'TestAccountFinancial|TestAccountProfitability' -count=1
go test ./internal/handler/admin -run 'TestAccountFinancial|TestDashboardHandlerAccountProfitability' -count=1
go test ./cmd/server -run '^$' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run the real PostgreSQL contract or record it as unverified**

Run:

```bash
cd upstream/sub2api/backend
go test -tags=integration ./internal/repository -run 'TestUsageLog_ReadAccountFinancialUsage_NativeContract' -count=1
```

Expected: PASS. If infrastructure is unavailable, preserve the exact error in the handoff and mark only this gate unverified.

- [ ] **Step 3: Run frontend focused, type, and build verification**

Run:

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run \
  src/api/__tests__/admin.accountFinancial.spec.ts \
  src/views/admin/__tests__/AccountProfitabilityView.spec.ts
pnpm typecheck
pnpm build
```

Expected: PASS.

- [ ] **Step 4: Run diff and forbidden-scope guards**

Run from repository root:

```bash
git diff --check 6289c22a31a9c6a53836e2086f2f356c13be1c1b...HEAD
git diff --name-only 6289c22a31a9c6a53836e2086f2f356c13be1c1b...HEAD
git diff --name-only 6289c22a31a9c6a53836e2086f2f356c13be1c1b...HEAD -- \
  docs/project/project-progress.md \
  docs/project/native-sub-task-package-queue.md \
  '.github/workflows/**' \
  'upstream/sub2api/backend/ent/migrate/**' \
  'upstream/sub2api/backend/migrations/**'
rg -n 'usage_upstream_cost_evidence|usage_cost_reviews|account_daily_financial_values|account_financial_settings|tab=cost-exceptions|/xingqiao|controlPlane|external-primary' \
  upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go \
  upstream/sub2api/backend/internal/service/account_financial.go \
  upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue
rg -n 'setTodayOverride|setOAuthCost|account-edit-|account-exceptions-|summary-exceptions|unallocated-adjustments|incomplete-group' \
  upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue
```

Expected:

- `diff --check` succeeds;
- protected/global/migration/workflow diff is empty;
- reader/GetReport/page forbidden-source searches return no matches, except historical methods later in `account_financial.go` that are outside `GetReport` and must be documented rather than deleted;
- page-only obsolete-control search returns no matches.

- [ ] **Step 5: Verify the 390×844 responsive contract when a local authenticated page is available**

Start the existing local frontend/backend test environment without changing configuration, open the administrator profitability route at a 390×844 viewport, and capture:

```js
({
  viewportWidth: document.documentElement.clientWidth,
  documentWidth: document.documentElement.scrollWidth,
  tableClientWidth: document.querySelector('[data-test="account-financial-table"]')?.clientWidth,
  tableScrollWidth: document.querySelector('[data-test="account-financial-table"]')?.scrollWidth,
})
```

Expected: `documentWidth <= viewportWidth`; the table wrapper may have `tableScrollWidth > tableClientWidth` because its own horizontal scrolling is allowed. Record the screenshot/evidence path in the handoff. If no authenticated local page can be started without expanding scope or changing configuration, record this exact check as unverified; do not touch production or invent credentials.

- [ ] **Step 6: Inspect schema/config/dependency and worktree cleanliness**

Run:

```bash
git status --short
git diff --name-only 6289c22a31a9c6a53836e2086f2f356c13be1c1b...HEAD -- \
  'upstream/sub2api/**/go.mod' \
  'upstream/sub2api/**/go.sum' \
  'upstream/sub2api/**/package.json' \
  'upstream/sub2api/**/pnpm-lock.yaml' \
  'upstream/sub2api/**/.env*' \
  'upstream/sub2api/**/config*'
```

Expected: no unexplained changes. Planned result is migration/config/dependency changes = none and `downtime_required=false`.

- [ ] **Step 7: Dispatch the final fresh whole-branch reviewer**

The reviewer is strictly read-only and must compare the entire diff against the approved spec and this plan. Required checklist:

1. exact SQL formulas and `[from,to)`;
2. four projections conserve pair totals;
3. soft-deleted/unassigned/zero-usage identities behave as specified;
4. balance card is preserved and isolated;
5. UI has all states and only approved removals;
6. no forbidden sources, writes, migrations, config, dependencies, workflows, root ledger/queue changes;
7. all claimed tests have captured output.

If findings exist, dispatch a fresh implementer for the smallest correction, rerun affected tests and the final reviewer. Do not self-approve unresolved findings.

- [ ] **Step 8: Write the candidate handoff**

Before writing, capture `git rev-parse HEAD`, `git diff --name-only 6289c22a31a9c6a53836e2086f2f356c13be1c1b...HEAD`, and every verification result. Create `docs/handoffs/2026-08-16-t11-r1-native-accounting-profitability-candidate-handoff.md` with this exact outline, replacing each instruction sentence with the captured concrete value rather than leaving brackets or placeholders. The source handoff is protected untracked root content: reference its absolute path only; never copy its contents into the candidate, edit it, stage it, or reuse its filename:

```markdown
# T11-R1 Native Accounting Profitability Handoff

- State: READY_FOR_ROOT_REVIEW
- Source handoff (read-only, not copied): /Users/gongtengxinwen/Documents/sub2api搭建/docs/handoffs/2026-08-16-native-accounting-profitability-handoff.md
- Baseline SHA: 6289c22a31a9c6a53836e2086f2f356c13be1c1b
- Candidate SHA: use the full output of `git rev-parse HEAD` after implementation commits
- Branch: codex/t11-r1-native-accounting-profitability
- Changed files: insert the complete baseline-to-HEAD `git diff --name-only` list
- Tests: insert every exact command with its PASS, FAIL, or SKIPPED result
- Unverified: write `none`, or the exact PostgreSQL/UI environment limitation and failed command
- Migration changes: none
- Configuration changes: none
- Dependency changes: none
- GitHub Actions changes: none
- downtime_required: false
- Rollback: root controller switches the blue-green deployment back to the previous active application image; no database rollback or data cleanup is required.
- Remaining risks: record 31-day aggregation load, historical missing names, floating-point display, and every observed limitation
- Not performed: merge, push, deploy, production mutation, root ledger/queue update.
```

- [ ] **Step 9: Commit the handoff and verify final cleanliness**

```bash
git add docs/handoffs/2026-08-16-t11-r1-native-accounting-profitability-candidate-handoff.md
git commit -m "docs: hand off T11-R1 native profitability"
git status --short --branch
git log --oneline 6289c22a31a9c6a53836e2086f2f356c13be1c1b..HEAD
```

Expected: clean worktree on `codex/t11-r1-native-accounting-profitability`. Report `READY_FOR_ROOT_REVIEW` only; stop without merge, push, deploy, production verification, or root ledger/queue edits.

## Plan Approval Gate

This plan must be committed and then reviewed in writing by the unique root release controller. Before that approval:

- do not invoke `superpowers:subagent-driven-development` or `superpowers:executing-plans`;
- do not dispatch implementers or reviewers;
- do not modify runtime code or tests;
- do not modify root ledgers/queues or any production/release state.
