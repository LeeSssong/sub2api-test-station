# T35 Procurement Audit PostgreSQL Parameter Type Hotfix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add explicit PostgreSQL types to the three procurement audit `jsonb_build_object` calls so save, clear, and settle transactions parse and commit atomically without changing their contracts.

**Architecture:** Keep the existing `AccountProfitabilityService` SQL, transaction boundaries, ledger, idempotency, and audit schema unchanged. Add only value casts in the three production statements; prove parser behavior by invoking the production service against the existing PostgreSQL testcontainers harness, and use focused sqlmock/source-contract tests for fast transaction and exact-SQL coverage.

**Tech Stack:** Go, `database/sql`, PostgreSQL/lib/pq, testcontainers-go integration harness, go-sqlmock, testify, `go build`.

**Spec:** `docs/superpowers/specs/2026-08-20-t35-procurement-audit-parameter-type-hotfix-design.md`

## Global Constraints

- Work only in `/Users/gongtengxinwen/.codex/worktrees/eeb6/sub2api搭建` on `codex/t35-procurement-audit-type-hotfix` from `main@101357776e1af9dbf83df282afd96cdb284ffcf4`.
- The only production SQL changes are `$4::bigint`, `$5/$6::double precision`, and `$5::text` in the three named audit JSON expressions.
- Preserve procurement ledger, transaction, idempotency, audit, cost formulas, handler/frontend contracts, and error semantics.
- No migration, schema, historical backfill, production data write, release evidence, global queue/ledger, root `main`, or GitHub Actions changes.
- PostgreSQL integration evidence is required; sqlmock-only or source-only evidence does not qualify.
- Expected release preflight is `downtime_required=false`; root release control owns merge, push, deploy, and online verification.

## File Map

- Modify: `upstream/sub2api/backend/internal/service/account_procurement_profitability.go` — the three production audit SQL expressions only.
- Modify: `upstream/sub2api/backend/internal/service/account_profitability_test.go` — exact cast source contract and injected audit-failure rollback tests.
- Create: `upstream/sub2api/backend/internal/repository/account_procurement_audit_type_integration_test.go` — build-tag `integration` test that invokes production service methods through the existing PostgreSQL harness.
- Create: `docs/superpowers/plans/2026-08-20-t35-procurement-audit-parameter-type-hotfix.md` — this implementation plan.

### Task 1: Add fast failing exact-SQL and rollback tests

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_profitability_test.go`

**Interfaces:**
- Consumes: `newAccountProfitabilityDB`, `NewAccountProfitabilityService`, `UpdateProcurementConfig`, and `SettleProcurement`.
- Produces: deterministic unit checks for the exact casts and transaction rollback when an audit insert returns an error.

- [ ] **Step 1: Replace the one-occurrence audit source check with exact contracts**

Read `account_procurement_profitability.go` with `os.ReadFile` and assert all three `LEFT($3,64)` occurrences plus these exact fragments:

```go
require.Equal(t, 3, strings.Count(source, "LEFT($3,64)"))
require.Contains(t, source, "jsonb_build_object('account_id',$4::bigint,'reason',$5::text)")
require.Contains(t, source, "jsonb_build_object('account_id',$4::bigint,'cleared',true)")
require.Contains(t, source, "jsonb_build_object('account_id',$4::bigint,'cost_cny',$5::double precision,'quota_usd',$6::double precision)")
require.NotContains(t, source, "jsonb_build_object('account_id',$4,'reason',$5)")
require.NotContains(t, source, "jsonb_build_object('account_id',$4,'cleared',true)")
require.NotContains(t, source, "jsonb_build_object('account_id',$4,'cost_cny',$5,'quota_usd',$6)")
```

- [ ] **Step 2: Add the save audit-failure rollback test**

Copy the existing first-version save expectation sequence, make the audit expectation return `errors.New("audit write failed")`, expect `Rollback`, call `UpdateProcurementConfig`, and assert the returned error contains `audit write failed`. Keep all earlier ledger/projection expectations so the test proves the failure happens after them and still rolls back.

- [ ] **Step 3: Add the clear audit-failure rollback test**

Use the existing clear expectation sequence through the pending-version insert, make its audit expectation return `errors.New("clear audit write failed")`, expect `Rollback`, invoke `UpdateProcurementConfig` with both pointers nil, and assert the exact error. This locks the clear path to the same atomic transaction semantics.

- [ ] **Step 4: Add the settle audit-failure rollback test**

Use the existing permanent-loss settlement expectation sequence, make its audit expectation return `errors.New("settle audit write failed")`, expect `Rollback`, invoke `SettleProcurement` with `Reason: "administrator_confirmed_expired"`, and assert the exact error.

- [ ] **Step 5: Run the unit RED check**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'Test(UpdateProcurementConfig|SettleProcurement|ProcurementAudit)' -count=1
```

Expected before the production change: the new exact-SQL contract fails because the casts are absent; the rollback tests may pass because sqlmock intentionally does not parse PostgreSQL SQL. Do not treat that sqlmock result as parser evidence.

- [ ] **Step 6: Commit the focused test changes**

```bash
git add upstream/sub2api/backend/internal/service/account_profitability_test.go
git commit -m "test: pin procurement audit postgres types and rollback"
```

### Task 2: Add and run the production-path PostgreSQL RED regression

**Files:**
- Create: `upstream/sub2api/backend/internal/repository/account_procurement_audit_type_integration_test.go`

**Interfaces:**
- Consumes: integration globals/helpers from `integration_harness_test.go`, Ent account creation, `integrationDB`, and public service constructors/methods.
- Produces: one test named `TestAccountProcurementAuditParametersUsePostgreSQLTypes` with save, clear, and settle subtests that distinguish parser failure from partial writes.

- [ ] **Step 1: Create an integration test using the existing harness**

Start the file with `//go:build integration`, package `repository`, and imports for `context`, `database/sql`, `fmt`, `strings`, `testing`, `time`, `ent`, `service`, and `testify/require`. Use a nanosecond prefix for every account and request id. Create OAuth accounts through `testEntClient(t).Account.Create().SetName(prefix).SetPlatform(service.PlatformOpenAI).SetType(service.AccountTypeOAuth).SetStatus(service.StatusActive).SaveX(ctx)`.

- [ ] **Step 2: Implement the save subtest with RED rollback assertions**

Call:

```go
cost, quota := 120.0, 60.0
err := service.NewAccountProfitabilityService(integrationDB).UpdateProcurementConfig(ctx, service.ProcurementConfigInput{
    AccountID: account.ID, CostCNY: &cost, QuotaUSD: &quota, ActorUserID: 77, RequestID: requestID,
})
```

When `err != nil`, assert it contains `could not determine data type of parameter`, then query `account_procurement_cost_versions`, `accounts.procurement_cost_cny`, and `audit_logs` by the unique request/account and assert zero rows/null projection. Finish with `require.NoError(t, err)` so the uncast production code is a genuine RED. On GREEN, query the active version, account projection, and audit JSON; assert values 9/120/60, `jsonb_typeof(extra->'account_id') = 'number'`, and numeric values in the expected fields.

- [ ] **Step 3: Implement the clear subtest with RED rollback assertions**

Create an OAuth account, seed its projection and one active version with direct `integrationDB.ExecContext`, then call `UpdateProcurementConfig` with nil cost/quota. On RED, assert the parser error and prove the active version/projection remain unchanged with no pending version/audit. On GREEN, assert the old version is ended, a new `cost_pending` version exists, projection columns are null, and the audit JSON has numeric `account_id` and boolean `cleared=true`.

- [ ] **Step 4: Implement the settle subtest with RED rollback assertions**

Create an OAuth account, set its status to `service.StatusError`, seed an active version, then call:

```go
ok, err := service.NewAccountProfitabilityService(integrationDB).SettleProcurement(ctx, service.ProcurementSettlementInput{
    AccountID: account.ID, RequestID: requestID, Reason: "administrator_confirmed_expired", ActorUserID: 77,
})
```

On RED, assert `ok` is false, the parser error is present, the version remains active with no settlement request/loss change, and no audit row exists. On GREEN, assert `ok` true, settled fields/loss are present, and audit JSON contains numeric `account_id` plus string reason.

- [ ] **Step 5: Run the integration RED check with Docker fail-closed**

Run:

```bash
cd upstream/sub2api/backend
docker info
CI=1 go test -tags integration ./internal/repository \
  -run 'TestAccountProcurementAuditParametersUsePostgreSQLTypes' -count=1 -v
```

Expected before the casts: all three service paths reach PostgreSQL parse failure and the rollback assertions pass, followed by the required `require.NoError` failure. The output must show the real testcontainers PostgreSQL harness; a skipped integration suite is not evidence.

- [ ] **Step 6: Commit the RED regression**

```bash
git add upstream/sub2api/backend/internal/repository/account_procurement_audit_type_integration_test.go
git commit -m "test: reproduce procurement audit parameter inference failure"
```

### Task 3: Apply the minimal production casts and turn all regressions GREEN

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_procurement_profitability.go`

**Interfaces:**
- Consumes: the existing three `ExecContext` audit statements and their current positional arguments.
- Produces: PostgreSQL-typed audit JSON without changing SQL parameter count/order or transaction control flow.

- [ ] **Step 1: Add only the approved casts**

Change the three `jsonb_build_object` fragments to exactly:

```go
jsonb_build_object('account_id',$4::bigint,'reason',$5::text)
jsonb_build_object('account_id',$4::bigint,'cleared',true)
jsonb_build_object('account_id',$4::bigint,'cost_cny',$5::double precision,'quota_usd',$6::double precision)
```

Keep `LEFT($3,64)`, the surrounding INSERT columns/values, positional argument lists, `defer tx.Rollback()`, and all preceding/following ledger and projection SQL unchanged.

- [ ] **Step 2: Format the production file**

```bash
cd upstream/sub2api/backend
gofmt -w internal/service/account_procurement_profitability.go
```

- [ ] **Step 3: Run focused unit GREEN checks**

```bash
go test ./internal/service -run 'Test(UpdateProcurementConfig|SettleProcurement|ProcurementAudit)' -count=1
go test ./internal/handler/admin -run 'TestAccountHandler.*Procurement' -count=1
```

Expected: exact cast contract, save/clear/settle success and rollback tests, idempotency tests, and procurement handler tests all pass.

- [ ] **Step 4: Run the real PostgreSQL GREEN check**

```bash
CI=1 go test -tags integration ./internal/repository \
  -run 'TestAccountProcurementAuditParametersUsePostgreSQLTypes' -count=1 -v
```

Expected: all save, clear, and settle subtests pass against PostgreSQL; no parser error, no partial transaction, correct typed JSON and preserved ledger/projection semantics.

- [ ] **Step 5: Commit the implementation**

```bash
git add upstream/sub2api/backend/internal/service/account_procurement_profitability.go
git commit -m "fix: cast procurement audit json parameters"
```

### Task 4: Run final focused validation and prepare handoff

**Files:**
- Read-only validation of the files above and the migration/config/frontend scope.

**Interfaces:**
- Consumes: the final T35 implementation HEAD and the approved spec.
- Produces: reproducible validation evidence bound to one clean candidate HEAD for root review.

- [ ] **Step 1: Run the full direct backend checks**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'Test(UpdateProcurementConfig|SettleProcurement|ProcurementAudit)' -count=1
go test ./internal/handler/admin -run 'TestAccountHandler.*Procurement' -count=1
go build ./cmd/server
```

- [ ] **Step 2: Run formatting and diff checks**

```bash
gofmt -d internal/service/account_procurement_profitability.go \
  internal/service/account_profitability_test.go \
  internal/repository/account_procurement_audit_type_integration_test.go
git diff --check
```

Expected: no formatting or whitespace output.

- [ ] **Step 3: Enforce the T35 file and zero-migration scope**

```bash
git diff --name-only 101357776e1af9dbf83df282afd96cdb284ffcf4...HEAD
git diff --exit-code 101357776e1af9dbf83df282afd96cdb284ffcf4...HEAD -- \
  migrations ent/schema frontend .github/workflows
```

Expected: only the T35 spec/plan, two service files, and one repository integration test appear; the forbidden-path diff command exits zero.

- [ ] **Step 4: Verify clean candidate identity**

```bash
git status --short
git branch --show-current
git log --oneline --decorate -5
git rev-parse HEAD
```

Expected: clean `codex/t35-procurement-audit-type-hotfix`; evidence commands and handoff report reference this same HEAD. The candidate remains unmerged, unpushed, undeployed, and outside root release control.

- [ ] **Step 5: Commit the validation handoff if evidence is a tracked task document**

Only if a T35 handoff report is requested by root, create it under `docs/superpowers/reports/` and commit it separately. Do not edit `docs/project/project-progress.md`, `docs/project/native-sub-task-package-queue.md`, release evidence, or production state from this worktree.

## Self-Review Checklist

- Spec coverage: Tasks 1–3 cover exact casts, parser RED/GREEN, rollback, typed JSON, idempotency preservation, and zero migration; Task 4 covers build, diff, scope, and handoff gates.
- Placeholder scan: all commands, files, test names, expected errors, and commit points are concrete; no unresolved implementation item remains.
- Type consistency: service inputs remain `int64`/`float64`/`string`; SQL contracts are `bigint`/`double precision`/`text`; integration assertions use the same account/request identifiers.
- Scope: one production service file plus directly related unit/integration tests; no handler or frontend implementation is planned.
