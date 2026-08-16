# T12 Native Probe Cost Design Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an isolated, append-only record of本站 probe cost and expose it beside the existing native `usage_logs` financial metrics without changing user billing facts or the existing unconsumed-balance field/API/DTO.

**Architecture:** Keep `usage_logs` as the only user financial fact source. Add `account_probe_cost_logs` as a separate append-only operational ledger with `account_id ON DELETE RESTRICT`; the account-test service records explicit `monitor`, `scheduled`, or `manual` attempts through one injected recorder, while the account-financial reader aggregates user rows and probe rows in one repeatable-read snapshot. Probe query failures are represented by `probe_data_error` and `probe_error_code`, never by `unavailable` or zero.

**Tech Stack:** Go services and SQL migrations, PostgreSQL, Ent-backed Sub2API repositories, Gin admin API, Vue 3/TypeScript/Vitest frontend.

## Global Constraints

- User financial facts remain exclusively in native `usage_logs`; probe rows never enter `usage_logs` and never affect balance, user usage, account cost, user cost, profit, or margin.
- `account_probe_cost_logs.account_id` is `BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT`; `CASCADE` is forbidden and probe history is append-only.
- Probe completeness uses `complete`, `partial`, and `unknown`; missing usage or pricing writes a nullable cost and never an estimate.
- `probe_cost_status=unavailable` means a successful query found no rows in the requested window/dimension only. Query failures use `probe_data_error=true` and `probe_error_code="probe_aggregate_unavailable"`; probe values/status are null on that path.
- Existing unconsumed-balance field, API name, and DTO remain unchanged. Only the operations page formats that existing value as USD with two displayed decimals; no alias, deprecation, global balance migration, or other balance page change.
- Internal decimal precision is retained until presentation; every external amount on this page—including non-zero probe amounts smaller than `0.00000001`, unconsumed balance, and all six financial amounts—uses ordinary USD formatting with exactly two decimal places. Database values, API/DTO values, and aggregate calculations are never truncated or rounded early.
- No historical probe backfill, usage-log rewrite, user-data rewrite, scheduling change, external control plane, GitHub Actions, production access, merge, push, or deployment occurs before root approval of the implementation plan and subsequent task gates.
- T13 retains the next integration/deployment lane; T12 may implement and review after plan approval but cannot enter `INTEGRATING`, `DEPLOYING`, or `VERIFYING` ahead of T13.
- Each task ends with focused tests, `git diff --check`, a scoped review, and a task commit. A fresh implementer and an independent read-only reviewer are required for each approved task; final whole-branch review happens before `READY_FOR_ROOT_REVIEW`.

---

## File Map

- `upstream/sub2api/backend/migrations/224_account_probe_cost_logs.sql`: add-only probe ledger schema, indexes, checks, and the restrictive account foreign key.
- `upstream/sub2api/backend/migrations/account_probe_cost_logs_migration_test.go`: static migration contract checks, including `ON DELETE RESTRICT` and no `usage_logs` mutation.
- `upstream/sub2api/backend/internal/service/account_probe_cost.go`: probe kinds, completeness/outcome types, recorder/reader interfaces, and aggregation DTOs.
- `upstream/sub2api/backend/internal/repository/account_probe_cost_repo.go`: append-only insert and window aggregation SQL using the existing `*sql.DB` transaction style.
- `upstream/sub2api/backend/internal/repository/account_probe_cost_repo_test.go`: SQL mock coverage for idempotency, nullable costs, half-open windows, and query failures.
- `upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go`: extend the existing repeatable-read snapshot with probe rows and isolated error state.
- `upstream/sub2api/backend/internal/repository/usage_log_repo_stats_test.go`: native/probe separation, aggregation conservation, no-row status, and probe-query-failure fixtures.
- `upstream/sub2api/backend/internal/service/account_test_service.go`: explicit probe-kind execution wrapper and usage observation hooks; keep the existing unclassified test method compatible.
- `upstream/sub2api/backend/internal/service/account_probe_cost_service.go`: translate complete probe observations through native `BillingService.CalculateCostUnified`, apply the account rate multiplier, and delegate only the resulting immutable row to the append-only repository.
- `upstream/sub2api/backend/internal/service/account_monitor_probe.go`: monitor probe wrapper passes `monitor` and records the result without changing monitor response semantics.
- `upstream/sub2api/backend/internal/service/scheduled_test_runner_service.go`: scheduled execution passes `scheduled`; recovery-only calls remain unmetered unless explicitly classified.
- `upstream/sub2api/backend/internal/handler/admin/account_handler.go`: manual admin test invokes the explicit `manual` wrapper while retaining the SSE response contract.
- `upstream/sub2api/backend/internal/service/account_probe_cost_test.go`: service-level tests for all three source labels, fail-open writes, incomplete usage, and no user billing calls.
- `upstream/sub2api/backend/internal/service/account_financial.go`: add nullable probe amounts and report-level `probe_data_error`/`probe_error_code` without changing existing six financial fields.
- `upstream/sub2api/backend/internal/service/account_financial_test.go`: fold probe rows at account/group/summary levels and preserve user-only profit/margin.
- `upstream/sub2api/backend/internal/service/wire.go`: provide the recorder and connect it to `AccountTestService` without changing existing test constructors.
- `upstream/sub2api/backend/cmd/server/wire.go`, `upstream/sub2api/backend/cmd/server/wire_gen.go`: bind the SQL probe repository and service dependencies in the native server graph.
- `upstream/sub2api/backend/internal/handler/admin/account_financial_handler_test.go`: verify the extended JSON contract and nullable probe failure fields.
- `upstream/sub2api/frontend/src/api/admin/accountFinancial.ts`: normalize probe fields and report-level error state while preserving the existing balance property.
- `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`: render probe card/column, USD balance formatting, six-field sorting, and distinct no-data/error states.
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`: card/column/sort/status/mobile regressions.
- `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`, `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`: localized probe labels, status text, and retry/error copy only for this page.

---

### Task 1: Add the isolated probe ledger and repository contract

**Files:**
- Create: `upstream/sub2api/backend/migrations/224_account_probe_cost_logs.sql`
- Create: `upstream/sub2api/backend/migrations/account_probe_cost_logs_migration_test.go`
- Create: `upstream/sub2api/backend/internal/service/account_probe_cost.go`
- Create: `upstream/sub2api/backend/internal/repository/account_probe_cost_repo.go`
- Create: `upstream/sub2api/backend/internal/repository/account_probe_cost_repo_test.go`

**Interfaces:**
- Produces `service.AccountProbeCostRepository.Append(ctx, AccountProbeCostLog) error` and `service.AccountProbeCostRepository.ReadWindow(ctx, from, to) ([]AccountProbeCostAggregate, error)`.
- `AccountProbeCostLog` contains `ProbeRunID`, `AccountID`, optional `GroupID`, `ProbeKind`, `Model`, token counters, nullable `AccountCost`, `UsageCompleteness`, `ProbeOutcome`, `ErrorCode`, and `CreatedAt`.
- `Append` treats a repeated `ProbeRunID` as an idempotent success only when the stored immutable payload matches; a conflicting payload returns a typed conflict error.

- [ ] **Step 1: Write the failing migration contract tests.** Assert the SQL creates `account_probe_cost_logs`, declares `account_id ... ON DELETE RESTRICT`, contains no `ALTER TABLE usage_logs`, has checks for the three probe kinds and `complete/partial/unknown`, and creates the three time/query indexes.
- [ ] **Step 2: Run the migration contract tests and confirm the missing migration fails.** Run `cd upstream/sub2api/backend && go test ./migrations -run TestAccountProbeCostLogsMigration -count=1`; expected failure is a missing migration or missing required clause.
- [ ] **Step 3: Add the expand-only migration.** Use `CREATE TABLE IF NOT EXISTS`, `DECIMAL(20,10)` for nullable `account_cost`, `UNIQUE (probe_run_id)`, `REFERENCES accounts(id) ON DELETE RESTRICT`, no user/API-key columns, and indexes on `created_at`, `(account_id, created_at)`, and `(group_id, created_at)`. Do not touch `usage_logs` or insert historical rows.

```sql
CREATE TABLE IF NOT EXISTS account_probe_cost_logs (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    group_id BIGINT,
    probe_run_id VARCHAR(128) NOT NULL UNIQUE,
    probe_kind VARCHAR(16) NOT NULL CHECK (probe_kind IN ('monitor', 'scheduled', 'manual')),
    model VARCHAR(100) NOT NULL,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    account_cost DECIMAL(20,10),
    usage_completeness VARCHAR(16) NOT NULL CHECK (usage_completeness IN ('complete', 'partial', 'unknown')),
    probe_outcome VARCHAR(16) NOT NULL CHECK (probe_outcome IN ('success', 'failure')),
    error_code VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```
- [ ] **Step 4: Write failing repository tests.** Cover one complete row, one partial row with null cost, duplicate-identical run, duplicate-conflicting run, half-open `[from,to)` aggregation, no rows, and query error propagation.
- [ ] **Step 5: Run the repository tests to capture the red state.** Run `cd upstream/sub2api/backend && go test ./internal/repository -run 'TestAccountProbeCost' -count=1`; expected failure is the absent repository methods.
- [ ] **Step 6: Implement the SQL repository.** Use parameterized SQL, one immutable insert path, `ON CONFLICT (probe_run_id)` read-back comparison, and aggregate sums that preserve nullable cost status rather than converting unknown cost to zero.

```go
type AccountProbeCostRepository interface {
    Append(context.Context, AccountProbeCostLog) error
    ReadWindow(context.Context, time.Time, time.Time) ([]AccountProbeCostAggregate, error)
}
```
- [ ] **Step 7: Run focused green verification and commit.** Run `cd upstream/sub2api/backend && go test ./migrations -run TestAccountProbeCostLogsMigration -count=1`, `cd upstream/sub2api/backend && go test ./internal/repository -run 'TestAccountProbeCost' -count=1`, and `git diff --check`; commit `feat: add isolated probe cost ledger`.

**Independent review gate:** reviewer checks the migration for `ON DELETE RESTRICT`, no `usage_logs` mutation, no cascade path, idempotent append behavior, and correct null/zero distinction before Task 2 starts.

### Task 2: Capture and persist the three explicit probe sources

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_test_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_probe.go`
- Modify: `upstream/sub2api/backend/internal/service/scheduled_test_runner_service.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_handler.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/backend/cmd/server/wire.go`
- Modify: `upstream/sub2api/backend/cmd/server/wire_gen.go`
- Create: `upstream/sub2api/backend/internal/service/account_probe_cost_test.go`
- Create: `upstream/sub2api/backend/internal/service/account_probe_cost_service.go`

**Interfaces:**
- `AccountTestService.SetProbeCostRecorder(AccountProbeCostRecorder)` injects the cost recorder without breaking existing unit-test constructors. `AccountTestService` never calls the repository or billing service directly.
- `AccountProbeCostRecorder` is the service boundary:

```go
type AccountProbeCostRecorder interface {
    Record(context.Context, ProbeRecordInput) error
}

type ProbeRecordInput struct {
    AccountID    int64
    GroupID      *int64
    Group        *Group
    AccountRate  float64
    Kind         ProbeKind
    RunID        string
    Model        string
    Tokens       UsageTokens
    Completeness string
    Outcome      string
    ErrorCode    string
}
```

- `AccountProbeCostService` implements `AccountProbeCostRecorder`, owns `BillingService` plus `AccountProbeCostRepository`, and is the only component allowed to calculate probe cost. `Record` writes a nullable `AccountCost` for `partial`/`unknown` completeness or a native pricing error; it never estimates.
- `TestAccountConnectionWithProbeKind(ctx/gin context, accountID, modelID, prompt, mode, kind, opts...)` is the explicit path; the existing `TestAccountConnection` remains unclassified for callers outside T12.
- `RunTestBackgroundWithProbeKind(ctx, accountID, modelID, kind)` wraps the existing background parser; recovery-only callers use the unclassified wrapper and do not create a T12 row.
- The context observer records one immutable `ProbeObservation` containing model, token counts, completeness, outcome, and stable error code. Provider paths that do not return usable usage write `unknown`/null cost, never an estimate.

- [ ] **Step 1: Add red service tests.** Assert manual, scheduled, and monitor wrappers write exactly one row with the expected `probe_kind`; recorder errors do not change the original test result; no user balance or `usage_logs` repository method is called.
- [ ] **Step 2: Run the red tests.** Run `cd upstream/sub2api/backend && go test ./internal/service -run 'TestAccountProbe' -count=1`; expected failure is missing wrapper/observer behavior.
- [ ] **Step 3: Add the explicit source wrappers.** Manual handler calls `TestAccountConnectionWithProbeKind(..., ProbeKindManual, ...)`; monitor calls `ProbeAccountConnection` with `ProbeKindMonitor`; scheduled runner calls `RunTestBackgroundWithProbeKind(..., ProbeKindScheduled)`; preserve existing SSE and scheduled result persistence.

```go
func (s *AccountTestService) TestAccountConnectionWithProbeKind(
    c *gin.Context, accountID int64, modelID, prompt, mode string,
    kind ProbeKind, opts ...AccountTestOptions,
) error

func (s *AccountTestService) RunTestBackgroundWithProbeKind(
    ctx context.Context, accountID int64, modelID string, kind ProbeKind,
) (*ScheduledTestResult, error)
```
- [ ] **Step 4: Add the context-only usage observer.** Provider response parsers publish observed token counts and completeness to the observer without adding billing fields to SSE. Complete token/price data calls the native billing resolver; absent usage or unsupported media/search/realtime modes produce `unknown` and nullable cost.
- [ ] **Step 5: Implement native cost translation in `account_probe_cost_service.go`.** For a `complete` observation, resolve the existing group context and call exactly:

```go
breakdown, err := s.billingService.CalculateCostUnified(CostInput{
    Ctx:            ctx,
    Model:          input.Model,
    GroupID:        input.GroupID,
    Group:          input.Group,
    Tokens:         input.Tokens,
    RateMultiplier: input.AccountRate,
})
```

Persist `breakdown.ActualCost` only when pricing succeeds and completeness is `complete`; for all other cases persist a null cost with the observed completeness and stable error code. The service delegates persistence to `AccountProbeCostRepository.Append` and does not call any user balance or `usage_logs` writer.
- [ ] **Step 6: Implement fail-open recording at the probe call sites.** Append after the probe attempt is classified, with account/group snapshot from the loaded account, a UUID `probe_run_id`, and no user/API-key identity. Log a stable diagnostic code when recorder or repository append fails, but return the original probe result.
- [ ] **Step 7: Run green focused checks and commit.** Run `cd upstream/sub2api/backend && go test ./internal/service -run 'TestAccountProbe|TestAccountMonitorProbe' -count=1`, `cd upstream/sub2api/backend && go test ./internal/handler/admin -run 'Test.*Account.*Test' -count=1`, `cd upstream/sub2api/backend && go vet ./internal/service ./internal/handler/admin`, and `git diff --check`; commit `feat: record explicit native probe costs`.

**Independent review gate:** reviewer verifies all three source labels, no accidental metering of recovery-only calls, no user billing side effects, and fail-open behavior when the operational ledger is unavailable.

### Task 3: Add isolated probe aggregation to account-financial

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_stats_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_financial.go`
- Modify: `upstream/sub2api/backend/internal/service/account_financial_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_financial_handler_test.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Create: `upstream/sub2api/backend/internal/repository/account_financial_probe_integration_test.go`

**Interfaces:**
- `AccountFinancialUsageSnapshot` gains probe rows plus `ProbeDataError bool` and `ProbeErrorCode *string`; existing user rows and `UserBalanceCNY` remain unchanged.
- `FinancialAmounts` gains nullable `ProbeRequests`, `ProbeTokens`, `ProbeCost`, and `ProbeCostStatus` JSON fields; the existing six fields retain their types and formulas.
- `AccountFinancialReport` gains `ProbeDataError` and `ProbeErrorCode` at report level. On probe query error, user amounts are still finalized and returned; probe fields are null.

- [ ] **Step 1: Write red repository/service/handler tests.** Cover user-only data with `unavailable`, complete probe conservation from account to group to summary, partial probe null cost, probe query failure with six user fields intact and probe fields null, and `probe_error_code="probe_aggregate_unavailable"`.
- [ ] **Step 2: Run the red tests.** Run `cd upstream/sub2api/backend && go test ./internal/repository -run 'Test(ReadAccountFinancialUsage|AccountFinancialProbe)' -count=1` and `cd upstream/sub2api/backend && go test ./internal/service ./internal/handler/admin -run 'TestAccountFinancial.*Probe|TestAccountFinancialReport' -count=1`; expected failures identify missing fields and query folding.
- [ ] **Step 3: Extend the repeatable-read reader.** Read native account/group/balance/user rows as today, then query the probe ledger in the same transaction. If only the probe query/scan fails, preserve the already-read user snapshot, rollback the failed transaction, and return a snapshot marked `ProbeDataError`; account/group metadata remains available for the response. A failure before native user data is available remains the existing full-report error.

```go
type AccountFinancialUsageSnapshot struct {
    Accounts []AccountFinancialUsageAccount
    Groups []AccountFinancialUsageGroup
    Rows []AccountFinancialUsageRow
    ProbeRows []AccountProbeCostAggregate
    ProbeDataError bool
    ProbeErrorCode *string
    UserBalanceCNY float64
}
```
- [ ] **Step 4: Fold probe rows independently.** Match by immutable `(group_id, account_id)` snapshot, preserve unassigned rows, sum raw decimals, set `unavailable` only for successful no-row dimensions, and never feed probe values into cost, user cost, profit, margin, or balance.
- [ ] **Step 5: Add report-level error serialization.** Emit `probe_data_error=false`/null code on success; emit true/code plus null probe fields on probe-query failure while retaining the six native financial values.
- [ ] **Step 6: Run green checks and commit.** Run the focused Go tests from Step 2, `cd upstream/sub2api/backend && go vet ./internal/repository ./internal/service ./internal/handler/admin`, and `git diff --check`; commit `feat: expose isolated probe financial aggregates`.

**Independent review gate:** reviewer checks same-snapshot semantics, no zero masking on probe-query failure, no user-financial formula drift, and stable JSON null/error behavior.

### Task 4: Implement the operations-page card, column, sorting, and USD/error states

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountFinancial.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`

**Interfaces:**
- Preserve `user_unconsumed_balance_cny` in `AccountFinancialReport`; change only its operations-page formatter from `cny()` to the existing USD formatter.
- Normalize nullable probe fields without coercing `null` to zero. The page consumes `probe_data_error` and `probe_error_code` as a separate status channel.
- Sorting state is `{ key: 'requests'|'tokens'|'cost'|'user_cost'|'profit'|'margin'; direction: 'asc'|'desc' }`; probe cost is display-only and cannot be selected as a sort key.

- [ ] **Step 1: Add red Vitest cases.** Cover probe card/column rendering, all six sortable headers with direction toggles and null-margin-last behavior, existing balance field shown with ordinary USD `$0.00`-style two-decimal formatting, a non-zero probe value smaller than `0.00000001` still rendered as ordinary two-decimal USD, every six financial amount rendered with the same two-decimal formatter, successful no-row `unavailable` versus query-error `probe_data_error`, and 390px table containment.
- [ ] **Step 2: Run the red frontend tests.** Run `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts`; expected failures identify missing card, column, sort controls, and error state.
- [ ] **Step 3: Extend API normalization.** Add typed nullable probe fields and report error fields; do not default probe `null` to zero. Keep the existing balance property name and all unrelated endpoint normalization untouched.
- [ ] **Step 4: Add page state and sorting.** Add accessible sort buttons to the six existing financial headers, stable tie-breaking by account ID, and preserve sort key/direction across range, group, and refresh changes. Leave probe fields outside the comparator.

```ts
type FinancialSortKey = 'requests' | 'tokens' | 'cost' | 'user_cost' | 'profit' | 'margin'
type FinancialSort = { key: FinancialSortKey; direction: 'asc' | 'desc' }
```
- [ ] **Step 5: Add visual states and localized copy.** Render the independent probe card and account column; show ordinary USD with exactly two decimals (including `$0.00` plus “暂无探测记录” only for successful `unavailable`), `—` for incomplete cost, and “探测数据暂不可用” with retry for `probe_data_error`. Apply the same two-decimal USD formatter to the unchanged unconsumed-balance field and all six financial amounts, while keeping raw precision in API/DTO/aggregation values. Keep the existing table’s isolated horizontal scrolling and avoid changing other pages.
- [ ] **Step 6: Run green frontend checks and commit.** Run the focused Vitest file, `cd upstream/sub2api/frontend && pnpm typecheck`, `cd upstream/sub2api/frontend && pnpm build`, and `git diff --check`; commit `feat: add probe cost and financial sorting to operations page`.

**Independent review gate:** reviewer checks no balance field rename, no probe value in six-field sorting, exact USD/error states, localized labels, null handling, and 390px layout containment.

### Task 5: Whole-branch validation and root handoff

**Files:**
- Modify only the focused test or migration contract files from Tasks 1–4 when a whole-branch review finds a directly related defect; do not alter unrelated modules.
- Create: `docs/superpowers/reports/2026-08-16-t12-native-probe-cost-implementation-report.md`
- Create: `docs/superpowers/reviews/2026-08-16-t12-native-probe-cost-implementation-review.md`

- [ ] **Step 1: Re-read the approved spec and this plan.** Build a checklist for source isolation, restrictive deletion, idempotency, completeness, error separation, six sort keys, USD display, and no balance DTO changes.
- [ ] **Step 2: Run the minimum combined validation.** Run focused backend tests for probe repository/service/account-financial/handlers, the migration integration test when PostgreSQL/Testcontainers is available, frontend account-profitability Vitest, frontend typecheck/build, `go vet` on touched packages, and `git diff --check`. Do not run unrelated full-repo suites by default.
- [ ] **Step 3: Run scope and safety scans.** Confirm no changed `docs/project/*`, no `usage_logs` schema mutation, no `user_id/api_key_id` placeholders, no `CASCADE`, no historical insert/update, no GitHub Actions, and no production access. Record unavailable environment checks explicitly.
- [ ] **Step 4: Record migration/release posture.** State migration `224_account_probe_cost_logs.sql`, add-only behavior, `downtime_required` result from the existing precheck, rollback as disabling probe writes/display while retaining the table and rows, and that no deployment is authorized from this worktree.
- [ ] **Step 5: Complete independent whole-branch review.** Reviewer reads the full diff and reports findings first; any finding is fixed on this branch and revalidated. The final handoff must remain `READY_FOR_ROOT_REVIEW`, not `DONE`.
- [ ] **Step 6: Commit the reports and handoff.** Commit `docs: hand off T12 implementation candidate` only after all focused validation and review evidence match the committed tree.

**Root gate:** root total control must separately authorize merge to `main`; this plan never authorizes merge, push, deployment, production migration, or online acceptance.

## Plan Self-Review Checklist

- Spec coverage: source isolation, three probe kinds, native pricing reuse, append-only/restrictive persistence, no backfill, ordinary two-decimal USD formatting for every page amount with raw internal precision preserved, six-field sorting, account/group/summary conservation, query failure separation, ordinary-user exclusion, rollback, migration precheck, and mobile layout each map to a task above.
- Placeholder scan: no `TBD`, `TODO`, “implement later”, or unbounded “add tests” steps; each task names files, interfaces, commands, expected red/green behavior, and a commit.
- Type consistency: `AccountProbeCostRepository`/ledger DTOs are introduced in Task 1; `AccountProbeCostRecorder`/`ProbeRecordInput` and its native-pricing implementation are introduced in Task 2; snapshot fields are consumed by Task 3; report-level error fields are introduced in Task 3 and normalized/rendered in Task 4; existing balance property remains unchanged throughout.
- Scope check: no new user billing source, no account-cost formula change, no scheduler/routing change, no global balance migration, and no release action before root authorization.

## Approval Request

This is the minimum implementation plan for the approved T12 specification. Root total control should review and approve the plan before any implementer subagent writes code, migration, or tests.
